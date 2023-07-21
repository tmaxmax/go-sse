package sse_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse"
	"golang.org/x/exp/slog"
)

type mockProvider struct {
	SubError   error
	Closed     chan struct{}
	Pub        *sse.Message
	PubTopics  []string
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
	e.AppendData("hello")

	if err := sub.Client.Send(e); err != nil {
		return fmt.Errorf("send failed: %w", err)
	}

	if err := sub.Client.Flush(); err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	<-ctx.Done()

	return nil
}

func (m *mockProvider) Publish(msg *sse.Message, topics []string) error {
	m.Pub = msg
	m.PubTopics = topics
	m.Published = true
	return nil
}

func (m *mockProvider) Shutdown(_ context.Context) error {
	m.Stopped = true
	return nil
}

var _ sse.Provider = (*mockProvider)(nil)

func newMockProvider(tb testing.TB, subErr error) *mockProvider {
	tb.Helper()

	return &mockProvider{Closed: make(chan struct{}), SubError: subErr}
}

func newMockLogger(w io.Writer) func(*http.Request) *slog.Logger {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{
		AddSource: false,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindTime {
				return slog.Attr{}
			}
			return a
		},
	})
	l := slog.New(h)

	return func(r *http.Request) *slog.Logger {
		return l
	}
}

func TestServer_ShutdownPublish(t *testing.T) {
	t.Parallel()

	p := &mockProvider{}
	s := &sse.Server{Provider: p}

	_ = s.Publish(&sse.Message{})
	require.True(t, p.Published, "Publish wasn't called")
	require.Equal(t, []any{*p.Pub, p.PubTopics}, []any{sse.Message{}, []string{sse.DefaultTopic}}, "incorrect message")

	p.Published = false
	_ = s.Publish(&sse.Message{}, "topic")
	require.True(t, p.Published, "Publish wasn't called")
	require.Equal(t, []any{*p.Pub, p.PubTopics}, []any{sse.Message{}, []string{"topic"}}, "incorrect message")

	_ = s.Shutdown(context.Background())
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
	sb := &strings.Builder{}
	(&sse.Server{Provider: p, Logger: newMockLogger(sb)}).ServeHTTP(rec, req)

	require.True(t, p.Subscribed, "Subscribe wasn't called")
	require.Equal(t, sse.ID("5"), p.Sub.LastEventID, "Invalid last event ID received")
	require.Equal(t, "data: hello\n\n", rec.Body.String(), "Invalid response body")
	require.Equal(t, http.StatusOK, rec.Code, "invalid response code")
	require.Equal(t, "level=INFO msg=\"sse: starting new session\"\nlevel=INFO msg=\"sse: subscribing session\" topics=<sse:default> lastEventID=5\nlevel=INFO msg=\"sse: session ended\"\n", sb.String(), "invalid log output")
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
	sb := &strings.Builder{}

	(&sse.Server{Provider: p, Logger: newMockLogger(sb)}).ServeHTTP(noFlusher{rec}, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "invalid response code")
	require.Equal(t, "Server-sent events unsupported\n", rec.Body.String(), "invalid response body")
	require.Equal(t, "level=INFO msg=\"sse: starting new session\"\nlevel=ERROR msg=\"sse: unsupported\"\n", sb.String(), "invalid log output")
}

func TestServer_ServeHTTP_subscribeError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("", "http://localhost", http.NoBody)
	p := newMockProvider(t, errors.New("can't subscribe"))
	sb := &strings.Builder{}

	(&sse.Server{Provider: p, Logger: newMockLogger(sb)}).ServeHTTP(rec, req)

	require.Equal(t, p.SubError.Error()+"\n", rec.Body.String(), "invalid response body")
	require.Equal(t, http.StatusInternalServerError, rec.Code, "invalid response code")
	require.Equal(t, "level=INFO msg=\"sse: starting new session\"\nlevel=INFO msg=\"sse: subscribing session\" topics=<sse:default> lastEventID=\"\"\nlevel=ERROR msg=\"sse: subscribe error\" err=\"can't subscribe\"\n", sb.String(), "invalid log output")
}

func TestServer_OnSession(t *testing.T) {
	t.Parallel()

	t.Run("Invalid", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("", "/", http.NoBody)
		p := newMockProvider(t, nil)
		sb := &strings.Builder{}

		(&sse.Server{
			Provider: p,
			Logger:   newMockLogger(sb),
			OnSession: func(s *sse.Session) (sse.Subscription, bool) {
				http.Error(s.Res, "this is invalid", http.StatusBadRequest)
				return sse.Subscription{}, false
			},
		}).ServeHTTP(rec, req)

		require.Equal(t, "this is invalid\n", rec.Body.String(), "invalid response body")
		require.Equal(t, http.StatusBadRequest, rec.Code, "invalid response code")
		require.Equal(t, "level=INFO msg=\"sse: starting new session\"\nlevel=WARN msg=\"sse: invalid subscription\"\n", sb.String(), "invalid log output")
	})
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
	req, _ := http.NewRequest("", "http://localhost", http.NoBody)
	p := newMockProvider(t, nil)

	(&sse.Server{Provider: p}).ServeHTTP(&responseWriterErr{rec}, req)
	_, ok := <-p.Closed
	require.False(t, ok)
}

func getMessage(tb testing.TB) *sse.Message {
	tb.Helper()

	m := &sse.Message{
		ID:   sse.ID(strconv.Itoa(rand.Int())),
		Type: sse.Type("test"),
	}
	m.AppendData("Hello world!", "Nice to see you all.")

	return m
}

type discardResponseWriter struct {
	w io.Writer
	h http.Header
	c int
}

func (d *discardResponseWriter) Header() http.Header         { return d.h }
func (d *discardResponseWriter) Write(b []byte) (int, error) { return d.w.Write(b) }
func (d *discardResponseWriter) WriteHeader(code int)        { d.c = code }
func (d *discardResponseWriter) Flush()                      {}

func getRequest(tb testing.TB) (w *discardResponseWriter, r *http.Request) {
	tb.Helper()

	w = &discardResponseWriter{w: io.Discard, h: make(http.Header)}
	r = httptest.NewRequest("", "http://localhost", http.NoBody)

	return
}

func benchmarkServer(b *testing.B, conns int) {
	b.Helper()

	s := &sse.Server{}
	b.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	m := getMessage(b)

	for i := 0; i < conns; i++ {
		w, r := getRequest(b)
		go s.ServeHTTP(w, r)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		_ = s.Publish(m)
	}
}

func BenchmarkServer(b *testing.B) {
	conns := [...]int{10, 100, 1000, 10000, 20000, 50000, 100000}

	for _, c := range conns {
		b.Run(strconv.Itoa(c), func(b *testing.B) {
			benchmarkServer(b, c)
		})
	}
}
