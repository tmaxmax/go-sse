package sse_test

import (
	"io"
	"strings"
	"testing"

	"github.com/tmaxmax/go-sse"
	"github.com/tmaxmax/go-sse/internal/tests"
)

func TestRead(t *testing.T) {
	// NOTE(tmaxmax): Basic test for now, as the functionality
	// is tested through the client tests. Most tests pertaining
	// to client-side event parsing will be moved here when
	// the client will be refactored.
	response := strings.NewReader("id:\000\nretry:x\ndata: Hello World!\n\n") // also test null ID and invalid retry edge cases

	var recv []sse.Event

	events, errf := sse.Read(response, nil)
	events(func(e sse.Event) bool {
		recv = append(recv, e)
		return true
	})

	tests.Equal(t, errf(), nil, "unexpected error")
	tests.DeepEqual(t, recv, []sse.Event{{Data: "Hello World!"}}, "incorrect result")

	t.Run("Buffer", func(t *testing.T) {
		_, _ = response.Seek(0, io.SeekStart)

		events, errf := sse.Read(response, &sse.ReadConfig{MaxEventSize: 3})
		events(func(sse.Event) bool { return true })
		tests.Expect(t, errf() != nil, "should fail because of too small buffer")
	})

	t.Run("Break", func(t *testing.T) {
		events, errf := sse.Read(strings.NewReader("id: a\n\nid: b\n\nid: c\n"), nil) // also test EOF edge case

		var recv []sse.Event
		events(func(e sse.Event) bool {
			recv = append(recv, e)
			return len(recv) < 2
		})

		tests.Equal(t, errf(), nil, "unexpected error")

		expected := []sse.Event{{LastEventID: "a"}, {LastEventID: "b"}}
		tests.DeepEqual(t, recv, expected, "iterator didn't stop")
	})
}
