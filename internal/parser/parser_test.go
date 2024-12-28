package parser_test

import (
	"bufio"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/tmaxmax/go-sse/internal/parser"
)

type errReader struct {
	r io.Reader
}

var errReadFailed = errors.New("haha error")

func (e errReader) Read(_ []byte) (int, error) {
	return 0, errReadFailed
}

func TestParser(t *testing.T) {
	t.Parallel()

	type test struct {
		input    io.Reader
		err      error
		name     string
		expected []parser.Field
	}

	longString := strings.Repeat("abcdefghijklmnopqrstuvwxyz", 193)

	tests := []test{
		{
			name: "Valid input",
			input: strings.NewReader(`
event: scan
data: first chunk
data: second chunk
:comment
data: third chunk
id: 1

: comment
event: anotherScan
data: nice scan
id: 2
retry: 15


event: something glitched before why are there two newlines
data: still, here's some data: you deserve it
`),
			expected: []parser.Field{
				newEventField(t, "scan"),
				newDataField(t, "first chunk"),
				newDataField(t, "second chunk"),
				newDataField(t, "third chunk"),
				newIDField(t, "1"),
				{},
				newEventField(t, "anotherScan"),
				newDataField(t, "nice scan"),
				newIDField(t, "2"),
				newRetryField(t, "15"),
				{},
				newEventField(t, "something glitched before why are there two newlines"),
				newDataField(t, "still, here's some data: you deserve it"),
			},
			err: io.EOF,
		},
		{
			name:  "Valid input with long string",
			input: strings.NewReader("\nid:2\ndata:" + longString + "\n"),
			expected: []parser.Field{
				newIDField(t, "2"),
				newDataField(t, longString),
			},
			err: io.EOF,
		},
		{
			name:  "Error",
			input: errReader{nil},
			err:   errReadFailed,
		},
		{
			name:  "Error from field parser (no final newline)",
			input: strings.NewReader("data: lmao"),
			err:   parser.ErrUnexpectedEOF,
		},
		{
			name: "With BOM",
			// The second BOM should not be removed, which should result in that field being named
			// "\ufeffdata", which is an invalid name and thus the field ending up being ignored.
			input: strings.NewReader("\xEF\xBB\xBFdata: hello\n\n\xEF\xBB\xBFdata: world\n\n"),
			expected: []parser.Field{
				newDataField(t, "hello"),
				{},
				{},
			},
			err: io.EOF,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			p := parser.New(test.input)
			var fields []parser.Field
			if l := len(test.expected); l > 0 {
				fields = make([]parser.Field, 0, l)
			}

			for f := (parser.Field{}); p.Next(&f); {
				fields = append(fields, f)
			}

			if err := p.Err(); err != test.err { //nolint
				t.Fatalf("invalid error: received %v, expected %v", err, test.err)
			}

			if !reflect.DeepEqual(test.expected, fields) {
				t.Fatalf("parse failed:\nreceived: %#v\nexpected: %#v", fields, test.expected)
			}
		})
	}

	t.Run("Buffer", func(t *testing.T) {
		p := parser.New(strings.NewReader("sarmale"))
		p.Buffer(make([]byte, 0, 3), 3)

		if p.Next(nil) {
			t.Fatalf("nothing should be parsed")
		}
		if !errors.Is(p.Err(), bufio.ErrTooLong) {
			t.Fatalf("expected error %v, received %v", bufio.ErrTooLong, p.Err())
		}
	})

	t.Run("Separate CRLF", func(t *testing.T) {
		r, w := io.Pipe()
		t.Cleanup(func() { r.Close() })

		p := parser.New(r)

		go func() {
			defer w.Close()
			_, _ = io.WriteString(w, "data: hello\n\r")
			// This LF should be ignored and yield no results.
			_, _ = io.WriteString(w, "\n")
			_, _ = io.WriteString(w, "data: world\n")
		}()

		var fields []parser.Field
		for f := (parser.Field{}); p.Next(&f); {
			fields = append(fields, f)
		}

		expected := []parser.Field{newDataField(t, "hello"), {}, newDataField(t, "world")}

		if !reflect.DeepEqual(fields, expected) {
			t.Fatalf("unexpected result:\nreceived %v\nexpected %v", fields, expected)
		}
	})
}

func BenchmarkParser(b *testing.B) {
	b.ReportAllocs()

	var f parser.Field

	for n := 0; n < b.N; n++ {
		r := strings.NewReader(benchmarkText)
		p := parser.New(r)

		for p.Next(&f) {
		}
	}

	_ = f
}
