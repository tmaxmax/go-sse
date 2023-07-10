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

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse"
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

	p := parser.NewFieldParser([]byte(s))

	for f := (parser.Field{}); p.Next(&f); {
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
	r, err := http.NewRequest("", "", http.NoBody)
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
				w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
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
			name: "Empty content type",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "")
				w.WriteHeader(http.StatusOK)
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

func events(tb testing.TB, c *sse.Connection, topics ...string) (events <-chan []sse.Event, unsubscribe sse.EventCallbackRemover) {
	tb.Helper()

	ch := make(chan []sse.Event)
	recv := make(chan sse.Event, 1)
	done := make(chan struct{})
	var unsub sse.EventCallbackRemover
	cb := func(e sse.Event) {
		select {
		case <-done:
		case recv <- e:
		}
	}
	events = ch

	if l := len(topics); l == 1 {
		if t := topics[0]; t == "" {
			unsub = c.SubscribeMessages(cb) // for coverage, SubscribeEvent("", recv) would be equivalent
		} else {
			unsub = c.SubscribeEvent(t, cb)
		}
	} else {
		if l == 0 {
			unsub = c.SubscribeToAll(cb)
		} else {
			unsubFns := make([]sse.EventCallbackRemover, 0, len(topics))
			for _, t := range topics {
				unsubFns = append(unsubFns, c.SubscribeEvent(t, cb))
			}

			unsub = func() {
				for _, fn := range unsubFns {
					fn()
				}
			}
		}
	}

	unsubscribe = func() {
		defer func() { _ = recover() }()
		defer close(done)
		unsub()
	}

	go func() {
		defer close(ch)

		var evs []sse.Event

		for {
			select {
			case ev := <-recv:
				evs = append(evs, ev)
			case <-done:
				ch <- evs
				return
			}
		}
	}()

	return
}

func TestConnection_Subscriptions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		data := "retry: 1000\n\nevent: test\ndata: something\nid: 1\n\nevent: test2\ndata: something else\n\ndata: unnamed\nid: 2\n\ndata: this shouldn't be received"

		for _, s := range strings.SplitAfter(data, "\n\n") {
			_, _ = io.WriteString(w, s)
			w.(http.Flusher).Flush()
			time.Sleep(time.Millisecond)
		}
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

	all, unsubAll := events(t, conn)
	defer unsubAll()
	expectedAll := []sse.Event{firstEvent, secondEvent, thirdEvent, fourthEvent}

	test, unsubTest := events(t, conn, "test")
	defer unsubTest()
	expectedTest := []sse.Event{secondEvent}

	test2, unsubTest2 := events(t, conn, "test2")
	defer unsubTest2()
	expectedTest2 := []sse.Event{thirdEvent}

	messages, unsubMessages := events(t, conn, "")
	defer unsubMessages()
	expectedMessages := []sse.Event{firstEvent, fourthEvent}

	require.NoError(t, conn.Connect(), "unexpected Connect error")
	unsubAll()
	require.Equal(t, expectedAll, <-all, "unexpected events for all")
	unsubTest()
	require.Equal(t, expectedTest, <-test, "unexpected events for test")
	unsubTest2()
	require.Equal(t, expectedTest2, <-test2, "unexpected events for test2")
	unsubMessages()
	require.Equal(t, expectedMessages, <-messages, "unexpected events for messages")
}

func TestConnection_dispatchDirty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "data: hello\ndata: world\n")
	}))
	defer ts.Close()

	c := &sse.Client{
		HTTPClient:        ts.Client(),
		ResponseValidator: sse.NoopValidator,
		MaxRetries:        -1, // for coverage, test will fail if a retry is actually made
	}
	conn := c.NewConnection(req(t, "", ts.URL, nil))
	expected := sse.Event{Data: []byte("hello\nworld")}
	var got sse.Event

	conn.SubscribeMessages(func(e sse.Event) {
		got = e
	})

	require.NoError(t, conn.Connect(), "unexpected Connect error")
	require.Equal(t, expected, got, "unexpected event received")
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
			time.Sleep(time.Millisecond * 5)
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

	all, unsubAll := events(t, conn)
	defer unsubAll()

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
	unsubAll()
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
