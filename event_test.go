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

	events := sse.Read(response, nil)
	events(func(e sse.Event, err error) bool {
		tests.Equal(t, err, nil, "unexpected error")
		recv = append(recv, e)
		return true
	})

	tests.DeepEqual(t, recv, []sse.Event{{Data: "Hello World!"}}, "incorrect result")

	t.Run("Buffer", func(t *testing.T) {
		_, _ = response.Seek(0, io.SeekStart)

		events := sse.Read(response, &sse.ReadConfig{MaxEventSize: 3})
		var err error
		events(func(_ sse.Event, e error) bool { err = e; return err == nil })
		tests.Expect(t, err != nil, "should fail because of too small buffer")
	})

	t.Run("Break", func(t *testing.T) {
		events := sse.Read(strings.NewReader("id: a\n\nid: b\n\nid: c\n"), nil) // also test EOF edge case

		var recv []sse.Event
		events(func(e sse.Event, err error) bool {
			tests.Equal(t, err, nil, "unexpected error")
			recv = append(recv, e)
			return len(recv) < 2
		})

		expected := []sse.Event{{LastEventID: "a"}, {LastEventID: "b"}}
		tests.DeepEqual(t, recv, expected, "iterator didn't stop")

		// Cover break check on EOF edge case
		// NOTE(tmaxmax): Should also test this with EOF return when possible.
		sse.Read(strings.NewReader("data: x\n"), nil)(func(e sse.Event, err error) bool {
			tests.Equal(t, err, nil, "unexpected error")
			tests.Equal(t, e, sse.Event{Data: "x"}, "unexpected event")
			return false
		})
	})
}
