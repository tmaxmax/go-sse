package sse_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse"
)

type mockProvider struct {
	SubError   error
	Closed     chan struct{}
	Pub        *sse.Message
	Sub        sse.Subscription
	Subscribed bool
	Stopped    bool
	Published  bool
}

func (m *mockProvider) Subscribe(ctx context.Context, sub sse.Subscription) error {
	m.Subscribed = true
	if m.SubError != nil {
		return m.SubError
	}

	defer close(m.Closed)
	m.Sub = sub

	e := &sse.Message{}
	e.AppendText("hello")

	if err := sub.Callback(e); err != nil {
		return err
	}

	<-ctx.Done()

	return nil
}

func (m *mockProvider) Publish(msg *sse.Message) error {
	m.Pub = msg
	m.Published = true
	return nil
}

func (m *mockProvider) Stop() error {
	m.Stopped = true
	return nil
}

var _ sse.Provider = (*mockProvider)(nil)

func newMockProvider(tb testing.TB, subErr error) *mockProvider {
	tb.Helper()

	return &mockProvider{Closed: make(chan struct{}), SubError: subErr}
}

func TestNew(t *testing.T) {
	t.Parallel()

	s := sse.NewServer()
	defer s.Shutdown() //nolint
	_, ok := s.Provider().(*sse.Joe)
	require.True(t, ok, "Default provider isn't Joe")

	s = sse.NewServer(sse.WithProvider(&mockProvider{}))
	_, ok = s.Provider().(*mockProvider)
	require.True(t, ok, "given provider isn't used")
}

func TestServer_ShutdownPublish(t *testing.T) {
	t.Parallel()

	p := &mockProvider{}
	s := sse.NewServer(sse.WithProvider(p))

	require.NoError(t, s.Publish(&sse.Message{}), "unexpected Publish error")
	require.True(t, p.Published, "Publish wasn't called")
	require.Equal(t, *p.Pub, sse.Message{Topic: sse.DefaultTopic}, "incorrect message")

	p.Published = false
	require.NoError(t, s.Publish(&sse.Message{Topic: "topic"}), "unexpected Publish error")
	require.True(t, p.Published, "Publish wasn't called")
	require.Equal(t, *p.Pub, sse.Message{Topic: "topic"}, "incorrect message")

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
	sse.NewServer(sse.WithProvider(p)).ServeHTTP(rec, req)

	require.True(t, p.Subscribed, "Subscribe wasn't called")
	require.Equal(t, sse.MustEventID("5"), p.Sub.LastEventID, "Invalid last event ID received")
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

	sse.NewServer(sse.WithProvider(p)).ServeHTTP(noFlusher{rec}, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "invalid response code")
	require.Equal(t, "Server-sent events unsupported\n", rec.Body.String(), "invalid response body")
}

func TestServer_ServeHTTP_subscribeError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("", "http://localhost", nil)
	p := newMockProvider(t, errors.New("can't subscribe"))

	sse.NewServer(sse.WithProvider(p)).ServeHTTP(rec, req)

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
	req, _ := http.NewRequest("", "http://localhost", nil)
	p := newMockProvider(t, nil)

	sse.NewServer(sse.WithProvider(p)).ServeHTTP(&responseWriterErr{rec}, req)
	_, ok := <-p.Closed
	require.False(t, ok)
}

func TestUpgrade(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	_, err := sse.Upgrade(rec)
	require.NoError(t, err, "unexpected NewConnection error")
	require.True(t, rec.Flushed, "writer wasn't flushed")

	r := rec.Result()
	defer r.Body.Close()

	expectedHeaders := http.Header{
		"Content-Type":      []string{"text/event-stream"},
		"Cache-Control":     []string{"no-cache"},
		"Connection":        []string{"keep-alive"},
		"Transfer-Encoding": []string{"chunked"},
	}

	require.Equal(t, expectedHeaders, r.Header, "invalid response headers")

	_, err = sse.Upgrade(nil)
	require.Equal(t, sse.ErrUpgradeUnsupported, err, "invalid NewConnection error")
}

var errWriteFailed = errors.New("err")

type errorWriter struct {
	Flushed bool
}

func (e *errorWriter) WriteHeader(_ int)           {}
func (e *errorWriter) Header() http.Header         { return http.Header{} }
func (e *errorWriter) Write(_ []byte) (int, error) { return 0, errWriteFailed }
func (e *errorWriter) Flush()                      { e.Flushed = true }

func TestUpgradedRequest_Send(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	conn, err := sse.Upgrade(rec)
	require.NoError(t, err, "unexpected NewConnection error")

	rec.Flushed = false

	ev := sse.Message{}
	ev.AppendText("sarmale")
	expected, _ := ev.MarshalText()

	require.NoError(t, conn.Send(&ev), "unexpected Send error")
	require.True(t, rec.Flushed, "writer wasn't flushed")
	require.Equal(t, expected, rec.Body.Bytes(), "body not written correctly")
}

func TestUpgradedRequest_Send_error(t *testing.T) {
	t.Parallel()

	rec := &errorWriter{}

	conn, err := sse.Upgrade(rec)
	require.NoError(t, err, "unexpected NewConnection error")

	rec.Flushed = false

	require.Equal(t, errWriteFailed, conn.Send(&sse.Message{}), "invalid Send error")
	require.True(t, rec.Flushed, "writer wasn't flushed")
}
