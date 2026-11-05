package sse_test

import (
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/tmaxmax/go-sse"
)

func TestReadIncompleteFinalEvent(t *testing.T) {
	for name, eol := range map[string]string{"LF": "\n", "CR": "\r", "CRLF": "\r\n"} {
		t.Run(name, func(t *testing.T) {
			complete := "data: complete" + eol + eol
			for _, tc := range []struct {
				name, input string
				count       int
			}{
				{"empty", "", 0},
				{"unterminated line", "data: partial", 0},
				{"terminated line", "data: partial" + eol, 0},
				{"complete", complete, 1},
				{"complete then unterminated", complete + "data: partial", 1},
				{"complete then terminated", complete + "data: partial" + eol, 1},
				{"two complete then partial", complete + complete + "data: partial", 2},
			} {
				t.Run(tc.name, func(t *testing.T) {
					chunks := [][]string{strings.Split(tc.input, "")}
					for split := 0; split <= len(tc.input); split++ {
						chunks = append(chunks, []string{tc.input[:split], tc.input[split:]})
					}
					for i, parts := range chunks {
						t.Run(strconv.Itoa(i), func(t *testing.T) {
							var got []sse.Event
							sse.Read(&eventChunkReader{chunks: parts}, nil)(func(event sse.Event, err error) bool {
								if err != nil {
									t.Errorf("Read: %v", err)
									return false
								}
								got = append(got, event)
								return true
							})
							var want []sse.Event
							for n := 0; n < tc.count; n++ {
								want = append(want, sse.Event{Data: "complete"})
							}
							if !reflect.DeepEqual(got, want) {
								t.Errorf("got %#v, want %#v", got, want)
							}
						})
					}
				})
			}
		})
	}
}

func TestReadPreservesReadError(t *testing.T) {
	failure := errors.New("read failed")
	var got error
	var events int
	sse.Read(&eventChunkReader{chunks: []string{"data: complete\n\ndata: partial"}, err: failure}, nil)(func(_ sse.Event, err error) bool {
		if err != nil {
			got = err
			return false
		}
		events++
		return true
	})
	if !errors.Is(got, failure) {
		t.Errorf("got error %v, want %v", got, failure)
	}
	if events != 1 {
		t.Errorf("got %d events, want 1", events)
	}
}

type eventChunkReader struct {
	chunks []string
	err    error
}

func (r *eventChunkReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		return 0, io.EOF
	}
	n := copy(p, r.chunks[0])
	if n == len(r.chunks[0]) {
		r.chunks = r.chunks[1:]
	} else {
		r.chunks[0] = r.chunks[0][n:]
	}
	return n, nil
}
