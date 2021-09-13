package sse_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse/internal/parser"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (r roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

type temporaryError struct {
	error
}

func (t temporaryError) Temporary() bool {
	return true
}

func reqCtx(tb testing.TB, ctx context.Context, method, address string, body io.Reader) *http.Request { //nolint
	tb.Helper()

	r, err := http.NewRequestWithContext(ctx, method, address, body)
	require.NoError(tb, err, "failed to create request")

	return r
}

func req(tb testing.TB, method, address string, body io.Reader) *http.Request { //nolint
	tb.Helper()
	return reqCtx(tb, context.Background(), method, address, body)
}

func toEv(tb testing.TB, s string) (ev sse.Event) {
	tb.Helper()

	defer func() {
		if l := len(ev.Data); l > 0 {
			ev.Data = ev.Data[:l-1]
		}
	}()

	p := parser.NewByteParser([]byte(s))

	for p.Scan() {
		f := p.Field()
		switch f.Name {
		case parser.FieldNameData:
			ev.Data = append(ev.Data, f.Value...)
			ev.Data = append(ev.Data, '\n')
		case parser.FieldNameID:
			ev.LastEventID = string(f.Value)
		case parser.FieldNameEvent:
			ev.Name = string(f.Value)
		case parser.FieldNameRetry:
		default:
			return
		}
	}

	require.NoError(tb, p.Err(), "unexpected toEv fail")

	return
}

func TestClient_NewConnection(t *testing.T) {
	require.Panics(t, func() {
		sse.NewConnection(nil)
	})

	c := sse.Client{}
	r := req(t, "", "", nil)
	_ = c.NewConnection(r)

	require.Equal(t, c.HTTPClient, http.DefaultClient)
}

func TestConnection_Connect_retry(t *testing.T) {
	var firstReconnectionTime time.Duration
	var retryAttempts int

	tempErr := temporaryError{errors.New("a temporary error take it or leave it")}

	c := &sse.Client{
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return nil, tempErr
			}),
		},
		OnRetry: func(_ error, duration time.Duration) {
			retryAttempts++
			if retryAttempts == 1 {
				firstReconnectionTime = duration
			}
		},
		MaxRetries:              3,
		DefaultReconnectionTime: time.Millisecond,
	}
	r, err := http.NewRequest("", "", nil)
	require.NoError(t, err, "failed to create request")
	err = c.NewConnection(r).Connect()

	require.ErrorIs(t, err, tempErr, "invalid error received from Connect")
	require.Equal(t, c.MaxRetries, retryAttempts, "connection was not retried enough times")
	require.InEpsilon(t, c.DefaultReconnectionTime, firstReconnectionTime, backoff.DefaultRandomizationFactor, "reconnection time incorrectly set")
}

type readerWrapper struct {
	io.Reader
}

func TestConnection_Connect_resetBody(t *testing.T) {
	type test struct {
		body    io.Reader
		err     error
		getBody func() (io.ReadCloser, error)
		name    string
	}

	getBodyErr := errors.New("haha")

	tests := []test{
		{
			name: "No body",
		},
		{
			name: "Body for which GetBody is set",
			body: strings.NewReader("nice"),
		},
		{
			name: "Body without GetBody",
			body: readerWrapper{strings.NewReader("haha")},
			err:  sse.ErrNoGetBody,
		},
		{
			name: "GetBody that returns error",
			err:  getBodyErr,
			body: readerWrapper{nil},
			getBody: func() (io.ReadCloser, error) {
				return nil, getBodyErr
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer ts.Close()
	httpClient := ts.Client()
	rt := httpClient.Transport

	c := &sse.Client{
		HTTPClient:              httpClient,
		ResponseValidator:       sse.NoopValidator,
		MaxRetries:              1,
		DefaultReconnectionTime: time.Nanosecond,
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstTry := true

			c.HTTPClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if firstTry {
					firstTry = false
					return nil, temporaryError{errors.New("hehe")}
				}
				return rt.RoundTrip(r)
			})

			r := req(t, "", ts.URL, test.body)
			if test.getBody != nil {
				r.GetBody = test.getBody
			}

			err := c.NewConnection(r).Connect()
			require.ErrorIs(t, err, test.err, "incorrect error received from Connect")
		})
	}
}

func TestConnection_Connect_validator(t *testing.T) {
	validatorErr := errors.New("invalid")

	type test struct {
		err       error
		validator sse.ResponseValidator
		name      string
	}

	tests := []test{
		{
			name:      "No validation error",
			validator: sse.NoopValidator,
		},
		{
			name: "Validation error",
			validator: func(_ *http.Response) error {
				return validatorErr
			},
			err: validatorErr,
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	c := &sse.Client{
		HTTPClient: ts.Client(),
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c.ResponseValidator = test.validator

			err := c.NewConnection(req(t, "", ts.URL, nil)).Connect()
			require.ErrorIs(t, err, test.err, "incorrect error received from Connect")
		})
	}
}

func TestConnection_Connect_defaultValidator(t *testing.T) {
	type test struct {
		handler   http.Handler
		name      string
		expectErr bool
	}

	tests := []test{
		{
			name: "Valid request",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
			}),
		},
		{
			name: "Invalid content type",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, "plain text")
			}),
			expectErr: true,
		},
		{
			name: "Invalid response status code",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusUnauthorized)
			}),
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ts := httptest.NewServer(test.handler)
			defer ts.Close()

			c := &sse.Client{HTTPClient: ts.Client()}
			err := c.NewConnection(req(t, "", ts.URL, nil)).Connect()

			if test.expectErr {
				require.Error(t, err, "expected Connect error")
			}
		})
	}
}

func events(tb testing.TB, c *sse.Connection, topics ...string) (events <-chan []sse.Event, unsubscribe func()) {
	tb.Helper()

	ch := make(chan []sse.Event)
	events = ch
	recv := make(chan sse.Event, 1)

	if l := len(topics); l == 1 {
		if t := topics[0]; t == "" {
			c.SubscribeMessages(recv) // for coverage, SubscribeEvent("", recv) would be equivalent
			unsubscribe = func() { c.UnsubscribeMessages(recv) }
		} else {
			c.SubscribeEvent(t, recv)
			unsubscribe = func() { c.UnsubscribeEvent(t, recv) }
		}
	} else {
		unsubscribe = func() { c.UnsubscribeFromAll(recv) }
		if l == 0 {
			c.SubscribeToAll(recv)
		} else {
			for _, t := range topics {
				c.SubscribeEvent(t, recv)
			}
		}
	}

	go func() {
		defer close(ch)

		var evs []sse.Event

		for ev := range recv {
			evs = append(evs, ev)
		}

		ch <- evs
	}()

	return
}

func TestConnection_Subscriptions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		data := "retry: 1000\n\nevent: test\ndata: something\nid: 1\n\nevent: test2\ndata: something else\n\ndata: unnamed\nid: 2\n\ndata: this shouldn't be received"

		_, _ = io.WriteString(w, data)
	}))
	defer ts.Close()

	c := &sse.Client{
		HTTPClient:        ts.Client(),
		ResponseValidator: sse.NoopValidator,
	}
	conn := c.NewConnection(req(t, "", ts.URL, nil))

	firstEvent := sse.Event{}
	secondEvent := sse.Event{Name: "test", Data: []byte("something"), LastEventID: "1"}
	thirdEvent := sse.Event{Name: "test2", Data: []byte("something else"), LastEventID: "1"}
	fourthEvent := sse.Event{Data: []byte("unnamed"), LastEventID: "2"}

	all, _ := events(t, conn)
	expectedAll := []sse.Event{firstEvent, secondEvent, thirdEvent, fourthEvent}

	test, _ := events(t, conn, "test")
	expectedTest := []sse.Event{secondEvent}

	test2, _ := events(t, conn, "test2")
	expectedTest2 := []sse.Event{thirdEvent}

	messages, _ := events(t, conn, "")
	expectedMessages := []sse.Event{firstEvent, fourthEvent}

	require.NoError(t, conn.Connect(), "unexpected Connect error")
	require.Equal(t, expectedAll, <-all, "unexpected events for all")
	require.Equal(t, expectedTest, <-test, "unexpected events for test")
	require.Equal(t, expectedTest2, <-test2, "unexpected events for test2")
	require.Equal(t, expectedMessages, <-messages, "unexpected events for messages")
}

func TestConnection_Unsubscriptions(t *testing.T) {
	evs := make(chan string)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic(http.ErrAbortHandler)
		}
		for ev := range evs {
			_, _ = io.WriteString(w, ev)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	c := &sse.Client{
		HTTPClient:        ts.Client(),
		ResponseValidator: sse.NoopValidator,
	}
	conn := c.NewConnection(req(t, "", ts.URL, nil))

	all, unsubAll := events(t, conn)
	some, unsubSome := events(t, conn, "a", "b")
	one, unsubOne := events(t, conn, "a")
	messages, unsubMessages := events(t, conn, "")

	type action struct {
		unsub   func()
		message string
	}

	actions := []action{
		{message: "data: unnamed\n\n", unsub: unsubMessages},
		{message: "data: for one and some\nevent: a\n\n", unsub: unsubOne},
		{message: "data: for some\nevent: b\n\n", unsub: unsubSome},
		{message: "data: for one and some again\nevent: a\n\n", unsub: unsubAll},
		{message: "data: unnamed again\n\n"},
		{message: "data: for some again\nevent: b\n\n"},
	}

	firstEvent := toEv(t, actions[0].message)
	secondEvent := toEv(t, actions[1].message)
	thirdEvent := toEv(t, actions[2].message)
	fourthEvent := toEv(t, actions[3].message)

	expectedAll := []sse.Event{firstEvent, secondEvent, thirdEvent, fourthEvent}
	expectedSome := []sse.Event{secondEvent, thirdEvent}
	expectedOne := []sse.Event{secondEvent}
	expectedMessages := []sse.Event{firstEvent}

	go func() {
		defer close(evs)
		for _, action := range actions {
			evs <- action.message
			// we wait for the subscribers to receive the event
			time.Sleep(time.Millisecond)
			if action.unsub != nil {
				action.unsub()
			}
		}
	}()

	require.NoError(t, conn.Connect(), "unexpected Connect error")
	require.Equal(t, expectedAll, <-all, "unexpected events for all")
	require.Equal(t, expectedSome, <-some, "unexpected events for some")
	require.Equal(t, expectedOne, <-one, "unexpected events for one")
	require.Equal(t, expectedMessages, <-messages, "unexpected events for messages")
}

func TestConnection_serverError(t *testing.T) {
	type action struct {
		message string
		cancel  bool
	}
	evs := make(chan action)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic(http.ErrAbortHandler)
		}
		for ev := range evs {
			if ev.cancel {
				panic(http.ErrAbortHandler)
			}
			_, _ = io.WriteString(w, ev.message)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	c := sse.Client{
		HTTPClient:        ts.Client(),
		ResponseValidator: sse.NoopValidator,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := c.NewConnection(reqCtx(t, ctx, "", ts.URL, nil))

	all, _ := events(t, conn)

	actions := []action{
		{message: "data: first\n"},
		{message: "data: second\n\n", cancel: true},
		{message: "data: third\n\n"},
	}
	expected := []sse.Event(nil)

	go func() {
		defer close(evs)
		for _, action := range actions {
			evs <- action
			if action.cancel {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}()

	require.Error(t, conn.Connect(), "expected Connect error")
	require.Equal(t, expected, <-all, "unexpected values for all")
}

type reconnectWriterError bool

func (r reconnectWriterError) Error() string   { return "write error" }
func (r reconnectWriterError) Temporary() bool { return bool(r) }
func (r reconnectWriterError) Timeout() bool   { return !bool(r) }

type reconnectWriter struct {
	event string
}

func (r *reconnectWriter) Read(p []byte) (int, error) {
	if r.event != "" {
		n := copy(p, r.event)
		r.event = r.event[n:]
		return n, nil
	}
	return 0, reconnectWriterError(rand.Intn(2) == 0)
}

func (r *reconnectWriter) Close() error { return nil }

func newReconnectWriter(tb testing.TB, id, retry string) *reconnectWriter {
	tb.Helper()

	var event string
	event += "id: " + id + "\n"
	if retry == "" {
		event += "\n"
	} else {
		event += "retry: " + retry + "\n\n"
	}

	return &reconnectWriter{event: event}
}

func newReconnectResponse(tb testing.TB, r *http.Request, id, retry string) *http.Response {
	tb.Helper()

	code := http.StatusOK
	cr := *r
	cr.Body = nil

	return &http.Response{
		StatusCode:    code,
		Status:        fmt.Sprintf("%d %s", code, http.StatusText(code)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:          newReconnectWriter(tb, id, retry),
		ContentLength: -1,
		Request:       &cr,
	}
}

const reconnectRetries = 3

type reconnectTransport struct {
	tb         testing.TB
	setIDs     [reconnectRetries]string
	setRetries [reconnectRetries]string
	recvIDs    []string
	recvBodies []string
	attempt    int
}

func (r *reconnectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.recvIDs = append(r.recvIDs, req.Header.Get("Last-Event-ID"))
	b, _ := io.ReadAll(req.Body)
	r.recvBodies = append(r.recvBodies, string(b))

	r.attempt++
	if r.attempt > reconnectRetries {
		return nil, errors.New("reconnectTransport: RoundTrip permanently failed")
	}

	return newReconnectResponse(r.tb, req, r.setIDs[r.attempt-1], r.setRetries[r.attempt-1]), nil
}

func (r *reconnectTransport) IDs() []string    { return r.recvIDs }
func (r *reconnectTransport) Bodies() []string { return r.recvBodies }

func newReconnectTransport(tb testing.TB, ids, retries [reconnectRetries]string) *reconnectTransport {
	tb.Helper()

	return &reconnectTransport{
		tb:         tb,
		setIDs:     ids,
		setRetries: retries,
	}
}

func TestConnection_reconnect(t *testing.T) {
	bodyText := "body"
	expectedIDs := []string{"", "1", "1", ""}
	expectedRetries := []time.Duration{time.Millisecond, time.Millisecond, 2 * time.Millisecond}
	expectedBodies := []string{bodyText, bodyText, bodyText, bodyText}

	var recvRetries []time.Duration
	onRetry := func(_ error, duration time.Duration) {
		recvRetries = append(recvRetries, duration)
	}

	rt := newReconnectTransport(t, [3]string{"1", "\000mama", ""}, [3]string{"1", "mama", "2"})
	c := sse.Client{
		HTTPClient: &http.Client{Transport: rt},
		OnRetry:    onRetry,
		MaxRetries: reconnectRetries,
	}
	conn := c.NewConnection(req(t, "", "", strings.NewReader(bodyText)))

	require.Error(t, conn.Connect(), "expected Connect error")
	require.Equal(t, expectedIDs, rt.IDs(), "incorrect Last-Event-IDs received")
	require.Equal(t, expectedBodies, rt.Bodies(), "incorrect bodies received")
	for i := range expectedRetries {
		require.InEpsilon(t, expectedRetries[i], recvRetries[i], backoff.DefaultRandomizationFactor, "invalid retry value")
	}
}

func drain(tb testing.TB, ch <-chan sse.Event) []sse.Event {
	tb.Helper()

	evs := make([]sse.Event, 0, len(ch))
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs
}

func TestConnection_Subscriptions_2(t *testing.T) {
	c := sse.Client{
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				rec := httptest.NewRecorder()
				_, _ = io.WriteString(rec, "event: test\ndata: test data\n\ndata: unnamed\n")
				return rec.Result(), nil
			}),
		},
		ResponseValidator: sse.NoopValidator,
	}
	conn := c.NewConnection(req(t, "", "", nil))

	ch, test := make(chan sse.Event, 2), make(chan sse.Event, 1)
	conn.SubscribeEvent("test", ch)      // subscribe to event with unsubscribed channel
	conn.UnsubscribeEvent("test", test)  // unsubscribe from existent event with unsubscribed channel (noop)
	conn.SubscribeToAll(ch)              // subscribe to all with already existing subscriptions (should remove previous subscriptions)
	conn.SubscribeEvent("test", ch)      // subscribe to event with subscriber to all (noop)
	conn.SubscribeEvent("test", test)    // subscribe to event with unsubscribed channel
	conn.SubscribeEvent("test", test)    // subscribe to event with channel already subscribed to it (noop)
	conn.SubscribeEvent("test2", test)   // subscribe to event with unsubscribed channel
	conn.UnsubscribeEvent("af", test)    // unsubscribe from nonexistent event (noop)
	conn.UnsubscribeEvent("test2", test) // unsubscribe from event with subscriber to multiple events (should not close the channel)

	expected := []sse.Event{
		{
			Name: "test",
			Data: []byte("test data"),
		},
		{Data: []byte("unnamed")},
	}
	expectedTest := expected[:1]

	require.NoError(t, conn.Connect(), "unexpected Connect error")
	require.Equal(t, expected, drain(t, ch), "invalid events received")
	require.Equal(t, expectedTest, drain(t, test), "invalid events received for test")
	require.NotPanics(t, func() {
		conn.SubscribeMessages(make(chan sse.Event)) // sub/unsub after connection is closed (noop, nonblocking)
	})
}
