package server_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse/server"
	"github.com/tmaxmax/go-sse/server/event"
)

type mockProvider struct {
	SubError   error
	Closed     chan struct{}
	Pub        server.Message
	Sub        server.Subscription
	Subscribed bool
	Stopped    bool
	Published  bool
}

func (m *mockProvider) Subscribe(ctx context.Context, sub server.Subscription) error {
	m.Subscribed = true
	if m.SubError != nil {
		return m.SubError
	}

	m.Sub = sub

	e := &event.Event{}
	e.AppendText("hello")

	go func() {
		sub.Channel <- e
		<-ctx.Done()
		close(sub.Channel)
		close(m.Closed)
	}()

	return nil
}

func (m *mockProvider) Publish(msg server.Message) error {
	m.Pub = msg
	m.Published = true
	return nil
}

func (m *mockProvider) Stop() error {
	m.Stopped = true
	return nil
}

var _ server.Provider = (*mockProvider)(nil)

func newMockProvider(tb testing.TB, subErr error) *mockProvider {
	tb.Helper()

	return &mockProvider{Closed: make(chan struct{}), SubError: subErr}
}

func TestNew(t *testing.T) {
	t.Parallel()

	s := server.New()
	defer s.Shutdown() //nolint
	_, ok := s.Provider().(*server.Joe)
	require.True(t, ok, "Default provider isn't Joe")

	s = server.New(&mockProvider{})
	_, ok = s.Provider().(*mockProvider)
	require.True(t, ok, "given provider isn't used")
}

func TestServer_ShutdownPublish(t *testing.T) {
	t.Parallel()

	p := &mockProvider{}
	s := server.New(p)

	require.NoError(t, s.Publish(nil), "unexpected Publish error")
	require.True(t, p.Published, "Publish wasn't called")
	require.Equal(t, p.Pub, server.Message{Topic: server.DefaultTopic}, "incorrect message")

	p.Published = false
	require.NoError(t, s.Publish(nil, "topic"), "unexpected Publish error")
	require.True(t, p.Published, "Publish wasn't called")
	require.Equal(t, p.Pub, server.Message{Topic: "topic"}, "incorrect message")

	require.NoError(t, s.Shutdown(), "unexpected Shutdown error")
	require.True(t, p.Stopped, "Stop wasn't called")
}

func request(tb testing.TB, method, address string, body io.Reader) (*http.Request, context.CancelFunc) { //nolint
	tb.Helper()

	r := httptest.NewRequest(method, address, body)
	ctx, cancel := context.WithCancel(r.Context())
	return r.WithContext(ctx), cancel
}

func TestServer_ServeHTTP(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req, cancel := request(t, "", "http://localhost", nil)
	defer cancel()
	p := newMockProvider(t, nil)
	req.Header.Set("Last-Event-ID", "5")

	go cancel()

	server.New(p).ServeHTTP(rec, req)
	cancel()

	require.True(t, p.Subscribed, "Subscribe wasn't called")
	require.Equal(t, event.MustID("5"), p.Sub.LastEventID, "Invalid last event ID received")
	require.Equal(t, "data: hello\n\n", rec.Body.String(), "Invalid response body")
	require.Equal(t, http.StatusOK, rec.Code, "invalid response code")
}

type noFlusher struct {
	http.ResponseWriter
}

func TestServer_ServeHTTP_unsupportedRespWriter(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req, cancel := request(t, "", "http://localhost", nil)
	defer cancel()
	p := newMockProvider(t, nil)

	server.New(p).ServeHTTP(noFlusher{rec}, req)
	cancel()

	require.Equal(t, http.StatusInternalServerError, rec.Code, "invalid response code")
	require.Equal(t, "Server-sent events unsupported\n", rec.Body.String(), "invalid response body")
}

func TestServer_ServeHTTP_subscribeError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req, cancel := request(t, "", "http://localhost", nil)
	defer cancel()
	p := newMockProvider(t, errors.New("can't subscribe"))

	server.New(p).ServeHTTP(rec, req)
	cancel()

	require.Equal(t, p.SubError.Error()+"\n", rec.Body.String(), "invalid response body")
	require.Equal(t, http.StatusInternalServerError, rec.Code, "invalid response code")
}

type flushResponseWriter interface {
	http.Flusher
	http.ResponseWriter
}

type responseWriterErr struct {
	flushResponseWriter
}

func (r *responseWriterErr) Write(p []byte) (int, error) {
	n, _ := r.flushResponseWriter.Write(p)
	return n, errors.New("")
}

func TestServer_ServeHTTP_connectionError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req, cancel := request(t, "", "http://localhost", nil)
	defer cancel()
	p := newMockProvider(t, nil)

	server.New(p).ServeHTTP(&responseWriterErr{rec}, req)
	cancel()
	_, ok := <-p.Closed
	require.False(t, ok)
}
