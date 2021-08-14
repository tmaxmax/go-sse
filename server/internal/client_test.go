package internal

import (
	"io"
	"strings"
	"testing"

	"github.com/kylelemons/godebug/diff"
)

func msg(s string) *strings.Reader {
	return strings.NewReader(s)
}

type testWriter struct {
	w   io.Writer
	buf []byte
	tb  testing.TB
}

func (t *testWriter) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)

	return len(p), nil
}

func (t *testWriter) Flush() {
	_, err := t.w.Write(t.buf)
	t.buf = t.buf[:0]

	if err != nil {
		t.tb.Fatalf("Flush error: %v", err)
	}
}

func TestClient(t *testing.T) {
	sb := &strings.Builder{}
	w := &testWriter{
		w:  sb,
		tb: t,
	}
	c := NewClient(w, w)

	messages := []string{"mama", "mea", "e", "super"}
	expected := strings.Join(messages, "")

	go func() {
		for _, m := range messages {
			c.Send(msg(m))
		}

		c.Close()
	}()

	if err := c.Receive(); err != nil {
		t.Fatal(err)
	}

	if sb.String() != expected {
		t.Fatalf("Messages received unsuccessfully:\n%v", diff.Diff(expected, sb.String()))
	}
}
