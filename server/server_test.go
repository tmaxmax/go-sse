package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/tmaxmax/go-sse/server/event"

	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	Subscribed bool
	Stopped    bool
	Published  bool
	SubError   error
	Sub        Subscription
	Pub        Message
}

func (m *mockProvider) Subscribe(ctx context.Context, sub Subscription) error {
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
	}()

	return nil
}

func (m *mockProvider) Publish(msg Message) error {
	m.Pub = msg
	m.Published = true
	return nil
}

func (m *mockProvider) Stop() error {
	m.Stopped = true
	return nil
}

var _ Provider = (*mockProvider)(nil)

func TestNew(t *testing.T) {
	t.Parallel()

	s := New()
	_, ok := s.provider.(*Joe)
	require.True(t, ok, "Default provider isn't Joe")

	s = New(&mockProvider{})
	_, ok = s.provider.(*mockProvider)
	require.True(t, ok, "given provider isn't used")
}

func TestGetProvider(t *testing.T) {
	t.Parallel()

	require.Nil(t, GetProvider(context.Background()), "invalid context retrieval")

	s := New()
	h := s.NewHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := GetProvider(r.Context()).(*Joe)
		_, _ = w.Write(strconv.AppendBool(nil, ok))
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("", "http://localhost", nil)

	h.ServeHTTP(rec, req)

	require.Equal(t, "true", rec.Body.String(), "invalid GetProvider return")
}

func TestServer_ShutdownPublish(t *testing.T) {
	t.Parallel()

	p := &mockProvider{}
	s := New(p)

	require.NoError(t, s.Publish(nil), "unexpected Publish error")
	require.True(t, p.Published, "Publish wasn't called")
	require.Equal(t, p.Pub, Message{Topic: DefaultTopic}, "incorrect message")

	p.Published = false
	require.NoError(t, s.Publish(nil, "topic"), "unexpected Publish error")
	require.True(t, p.Published, "Publish wasn't called")
	require.Equal(t, p.Pub, Message{Topic: "topic"}, "incorrect message")

	require.NoError(t, s.Shutdown(), "unexpected Shutdown error")
	require.True(t, p.Stopped, "Stop wasn't called")
}

func TestServer_ServeHTTP(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("", "http://localhost", nil)
	p := &mockProvider{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req = req.WithContext(ctx)
	req.Header.Set("Last-Event-ID", "")

	go cancel()

	New(p).ServeHTTP(rec, req)

	require.True(t, p.Subscribed, "Subscribe wasn't called")
	require.Equal(t, event.MustID(""), p.Sub.LastEventID, "Invalid last event ID received")
	require.Equal(t, "data: hello\n\n", rec.Body.String(), "Invalid response body")
	require.Equal(t, http.StatusOK, rec.Code, "invalid response code")
}

type noFlusher struct {
	http.ResponseWriter
}

func TestServer_ServeHTTP_unsupportedRespWriter(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("", "http://localhost", nil)
	p := &mockProvider{}

	New(p).ServeHTTP(noFlusher{rec}, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "invalid response code")
	require.Equal(t, "Server-sent events unsupported\n", rec.Body.String(), "invalid response body")
}

func TestServer_ServeHTTP_subscribeError(t *testing.T) {
	t.Parallel()

	err := errors.New("can't subscribe")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("", "http://localhost", nil)
	p := &mockProvider{SubError: err}

	New(p).ServeHTTP(rec, req)

	require.Equal(t, err.Error()+"\n", rec.Body.String(), "invalid response body")
	require.Equal(t, http.StatusInternalServerError, rec.Code, "invalid response code")
}
