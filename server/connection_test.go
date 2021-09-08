package server_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tmaxmax/go-sse/server/event"

	"github.com/stretchr/testify/require"

	"github.com/tmaxmax/go-sse/server"
)

func TestNewConnection(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	_, err := server.NewConnection(rec)
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

	_, err = server.NewConnection(nil)
	require.Equal(t, server.ErrUnsupported, err, "invalid NewConnection error")
}

var writerErr = errors.New("err")

type errorWriter struct {
	Flushed bool
}

func (e *errorWriter) WriteHeader(_ int)   {}
func (e *errorWriter) Header() http.Header { return http.Header{} }
func (e *errorWriter) Write(_ []byte) (int, error) {
	return 0, writerErr
}
func (e *errorWriter) Flush() {
	e.Flushed = true
}

func TestConnection_Send(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	conn, err := server.NewConnection(rec)
	require.NoError(t, err, "unexpected NewConnection error")

	rec.Flushed = false
	ev := event.New(event.Text("sarmale"))
	expected, _ := ev.MarshalText()

	require.NoError(t, conn.Send(ev), "unexpected Send error")
	require.True(t, rec.Flushed, "writer wasn't flushed")
	require.Equal(t, expected, rec.Body.Bytes(), "body not written correctly")
}

func TestConnection_Send_error(t *testing.T) {
	t.Parallel()

	rec := &errorWriter{}

	conn, err := server.NewConnection(rec)
	require.NoError(t, err, "unexpected NewConnection error")

	rec.Flushed = false
	ev := event.New(event.Text("sarmale"))

	require.Equal(t, writerErr, conn.Send(ev), "invalid Send error")
	require.True(t, rec.Flushed, "writer wasn't flushed")
}
