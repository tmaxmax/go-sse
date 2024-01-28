package sse_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tmaxmax/go-sse"
	"github.com/tmaxmax/go-sse/internal/tests"
)

func TestUpgrade(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Last-Event-Id", "hello")
	rec := httptest.NewRecorder()

	sess, err := sse.Upgrade(rec, req)
	tests.Equal(t, err, nil, "unexpected  error")
	tests.Expect(t, !rec.Flushed, "response writer was flushed")
	tests.Equal(t, sess.Send(&sse.Message{ID: sess.LastEventID}), nil, "unexpected Send error")
	tests.Equal(t, sess.Flush(), nil, "unexpected Flush error")

	r := rec.Result()
	t.Cleanup(func() { _ = r.Body.Close() })

	expectedHeaders := http.Header{
		"Content-Type": []string{"text/event-stream"},
	}
	expectedBody := "id: hello\n\n"

	body, err := io.ReadAll(r.Body)
	tests.Equal(t, err, nil, "failed to read response body")

	tests.DeepEqual(t, r.Header, expectedHeaders, "invalid response headers")
	tests.Equal(t, expectedBody, string(body), "invalid response body (and Last-Event-Id)")

	_, err = sse.Upgrade(nil, nil)
	tests.ErrorIs(t, err, sse.ErrUpgradeUnsupported, "invalid Upgrade error")
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
	tests.Equal(t, err, nil, "unexpected NewConnection error")

	rec.Flushed = false

	ev := sse.Message{}
	ev.AppendData("sarmale")
	expected, _ := ev.MarshalText()

	tests.Equal(t, conn.Send(&ev), nil, "unexpected Send error")
	tests.Expect(t, rec.Flushed, "writer wasn't flushed")
	tests.DeepEqual(t, rec.Body.Bytes(), expected, "body not written correctly")
}

func TestUpgradedRequest_Send_error(t *testing.T) {
	t.Parallel()

	rec := &errorWriter{}

	conn, err := sse.Upgrade(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	tests.Equal(t, err, nil, "unexpected NewConnection error")

	rec.Flushed = false

	tests.ErrorIs(t, conn.Send(&sse.Message{ID: sse.ID("")}), errWriteFailed, "invalid Send error")
	tests.Expect(t, rec.Flushed, "writer wasn't flushed")
}
