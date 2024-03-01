package sse_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse"
	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/tests"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (r roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

func reqCtx(tb testing.TB, ctx context.Context, method, address string, body io.Reader) *http.Request { //nolint
	tb.Helper()

	r, err := http.NewRequestWithContext(ctx, method, address, body)
	tests.Equal(tb, err, nil, "failed to create request")

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

	p := parser.NewFieldParser(s)

	for f := (parser.Field{}); p.Next(&f); {
		switch f.Name { //nolint:exhaustive // Comment fields are not parsed.
		case parser.FieldNameData:
			ev.Data += f.Value + "\n"
		case parser.FieldNameID:
			ev.LastEventID = string(f.Value)
		case parser.FieldNameEvent:
			ev.Type = string(f.Value)
		case parser.FieldNameRetry:
		default:
			return
		}
	}

	tests.Equal(tb, p.Err(), nil, "unexpected toEv fail")

	return
}

func TestClient_NewConnection(t *testing.T) {
	tests.Panics(t, func() {
		sse.NewConnection(nil)
	}, "a connection cannot be created without a request")

	c := sse.Client{}
	r := req(t, "", "", nil)
	_ = c.NewConnection(r)

	tests.Equal(t, c.HTTPClient, http.DefaultClient, "incorrect default HTTP client")
}

func TestConnection_Connect_retry(t *testing.T) {
	var firstReconnectionTime time.Duration
	var retryAttempts int

	testErr := errors.New("done")

	c := &sse.Client{
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return nil, testErr
			}),
		},
		OnRetry: func(_ error, duration time.Duration) {
			retryAttempts++
			if retryAttempts == 1 {
				firstReconnectionTime = duration
			}
		},
		Backoff: sse.Backoff{
			MaxRetries:      3,
			InitialInterval: time.Millisecond,
		},
	}
	r := req(t, "", "", http.NoBody)
	err := c.NewConnection(r).Connect()

	tests.ErrorIs(t, err, testErr, "invalid error received from Connect")
	tests.Equal(t, retryAttempts, c.Backoff.MaxRetries, "connection was not retried enough times")

	timeDelta := time.Duration(float64(c.Backoff.InitialInterval) * sse.DefaultClient.Backoff.Jitter)
	tests.Expect(t, c.Backoff.InitialInterval-timeDelta <= firstReconnectionTime && firstReconnectionTime <= c.Backoff.InitialInterval+timeDelta, "reconnection time incorrectly set")
}

func TestConnection_Connect_noRetryCtxErr(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case ts := <-ticker.C:
				fmt.Fprintf(w, "id: %s\n\n", ts)
			case <-r.Context().Done():
				return
			}
		}
	}))
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := &sse.Client{
		HTTPClient:        ts.Client(),
		ResponseValidator: sse.NoopValidator,
	}

	r := reqCtx(t, ctx, "", ts.URL, http.NoBody)
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()
	err := c.NewConnection(r).Connect()
	tests.ErrorIs(t, err, ctx.Err(), "invalid connect error")
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

	tt := []test{
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

	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Millisecond * 5)
	}))
	defer ts.Close()
	httpClient := ts.Client()
	rt := httpClient.Transport

	c := &sse.Client{
		HTTPClient:        httpClient,
		ResponseValidator: sse.NoopValidator,
		Backoff: sse.Backoff{
			MaxRetries:      1,
			InitialInterval: time.Nanosecond,
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			firstTry := true
			c.HTTPClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if firstTry {
					firstTry = false
					return nil, errors.New("fail")
				}
				return rt.RoundTrip(r)
			})

			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*3)
			defer cancel()

			r := reqCtx(t, ctx, "", ts.URL, test.body)
			if test.getBody != nil {
				r.GetBody = test.getBody
			}

			err := c.NewConnection(r).Connect()
			if test.err != nil {
				tests.ErrorIs(t, err, test.err, "incorrect error received from Connect")
			} else {
				tests.Equal(t, err, ctx.Err(), "connection error should be context error")
			}
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

	tt := []test{
		{
			name:      "No validation error",
			validator: sse.NoopValidator,
			err:       io.EOF,
		},
		{
			name: "Validation error",
			validator: func(_ *http.Response) error {
				return validatorErr
			},
			err: validatorErr,
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer ts.Close()

	c := &sse.Client{
		HTTPClient: ts.Client(),
		Backoff: sse.Backoff{
			MaxRetries: -1,
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			c.ResponseValidator = test.validator

			err := c.NewConnection(req(t, "", ts.URL, nil)).Connect()
			tests.ErrorIs(t, err, test.err, "incorrect error received from Connect")
		})
	}
}

func TestConnection_Connect_defaultValidator(t *testing.T) {
	type test struct {
		handler   http.Handler
		name      string
		expectErr bool
	}

	tt := []test{
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

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			ts := httptest.NewServer(test.handler)
			defer ts.Close()

			c := &sse.Client{HTTPClient: ts.Client(), Backoff: sse.Backoff{MaxRetries: -1}}
			err := c.NewConnection(req(t, "", ts.URL, nil)).Connect()

			if test.expectErr {
				tests.Expect(t, err != nil, "expected Connect error")
			} else {
				tests.ErrorIs(t, err, io.EOF, "should propagate EOF error")
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
		Backoff:           sse.Backoff{MaxRetries: -1},
	}
	conn := c.NewConnection(req(t, "", ts.URL, nil))

	firstEvent := sse.Event{}
	secondEvent := sse.Event{Type: "test", Data: "something", LastEventID: "1"}
	thirdEvent := sse.Event{Type: "test2", Data: "something else", LastEventID: "1"}
	fourthEvent := sse.Event{Data: "unnamed", LastEventID: "2"}

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

	tests.ErrorIs(t, conn.Connect(), sse.ErrUnexpectedEOF, "incorrect Connect error")
	unsubAll()
	tests.DeepEqual(t, <-all, expectedAll, "unexpected events for all")
	unsubTest()
	tests.DeepEqual(t, <-test, expectedTest, "unexpected events for test")
	unsubTest2()
	tests.DeepEqual(t, <-test2, expectedTest2, "unexpected events for test2")
	unsubMessages()
	tests.DeepEqual(t, <-messages, expectedMessages, "unexpected events for messages")
}

func TestConnection_dispatchDirty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "data: hello\ndata: world\n")
	}))
	defer ts.Close()

	c := &sse.Client{
		HTTPClient:        ts.Client(),
		ResponseValidator: sse.NoopValidator,
		Backoff: sse.Backoff{
			MaxRetries: -1,
		},
	}
	conn := c.NewConnection(req(t, "", ts.URL, nil))
	expected := sse.Event{Data: "hello\nworld"}
	var got sse.Event

	conn.SubscribeMessages(func(e sse.Event) {
		got = e
	})

	tests.ErrorIs(t, conn.Connect(), io.EOF, "unexpected Connect error")
	tests.Equal(t, got, expected, "unexpected event received")
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
		Backoff: sse.Backoff{
			MaxRetries: -1,
		},
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

	tests.ErrorIs(t, conn.Connect(), io.EOF, "unexpected Connect error")
	tests.DeepEqual(t, <-all, expectedAll, "unexpected events for all")
	tests.DeepEqual(t, <-some, expectedSome, "unexpected events for some")
	tests.DeepEqual(t, <-one, expectedOne, "unexpected events for one")
	tests.DeepEqual(t, <-messages, expectedMessages, "unexpected events for messages")
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
		Backoff:           sse.Backoff{MaxRetries: -1},
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

	tests.Expect(t, conn.Connect() != nil, "expected Connect error")
	unsubAll()
	tests.DeepEqual(t, <-all, expected, "unexpected values for all")
}

func TestConnection_reconnect(t *testing.T) {
	try := 0
	lastEventIDs := []string(nil)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastEventIDs = append(lastEventIDs, r.Header.Get("Last-Event-Id"))
		try++
		fmt.Fprintf(w, "id: %d\n\n", try)
	}))
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	retries := 0
	c := sse.Client{
		HTTPClient: ts.Client(),
		OnRetry: func(_ error, _ time.Duration) {
			retries++
			if retries == 3 {
				cancel()
			}
		},
		Backoff: sse.Backoff{
			InitialInterval: time.Nanosecond,
		},
		ResponseValidator: sse.NoopValidator,
	}

	r := reqCtx(t, ctx, "", ts.URL, http.NoBody)
	err := c.NewConnection(r).Connect()

	tests.Equal(t, err, ctx.Err(), "expected context error")
	tests.DeepEqual(t, lastEventIDs, []string{"", "1", "2"}, "incorrect last event IDs")
}
