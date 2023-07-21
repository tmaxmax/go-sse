package sse_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse"
)

func TestUpgrade(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Last-Event-Id", "hello")
	rec := httptest.NewRecorder()

	sess, err := sse.Upgrade(rec, req)
	require.NoError(t, err, "unexpected  error")
	require.False(t, rec.Flushed, "response writer was flushed")
	require.NoError(t, sess.Send(&sse.Message{ID: sess.LastEventID}), "unexpected Send error")
	require.NoError(t, sess.Flush(), "unexpected Flush error")

	r := rec.Result()
	t.Cleanup(func() { _ = r.Body.Close() })

	expectedHeaders := http.Header{
		"Content-Type": []string{"text/event-stream"},
	}
	expectedBody := "id: hello\n\n"

	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	require.Equal(t, expectedHeaders, r.Header, "invalid response headers")
	require.Equal(t, expectedBody, string(body), "invalid response body (and Last-Event-Id)")

	_, err = sse.Upgrade(nil, nil)
	require.ErrorIs(t, err, sse.ErrUpgradeUnsupported, "invalid Upgrade error")
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

	conn, err := sse.Upgrade(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	require.NoError(t, err, "unexpected NewConnection error")

	rec.Flushed = false

	ev := sse.Message{}
	ev.AppendData("sarmale")
	expected, _ := ev.MarshalText()

	require.NoError(t, conn.Send(&ev), "unexpected Send error")
	require.True(t, rec.Flushed, "writer wasn't flushed")
	require.Equal(t, expected, rec.Body.Bytes(), "body not written correctly")
}

func TestUpgradedRequest_Send_error(t *testing.T) {
	t.Parallel()

	rec := &errorWriter{}

	conn, err := sse.Upgrade(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	require.NoError(t, err, "unexpected NewConnection error")

	rec.Flushed = false

	require.ErrorIs(t, conn.Send(&sse.Message{ID: sse.ID("")}), errWriteFailed, "invalid Send error")
	require.True(t, rec.Flushed, "writer wasn't flushed")
}
