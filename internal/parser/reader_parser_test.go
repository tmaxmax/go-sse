package parser_test

import (
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

var readerErr = errors.New("haha error")

func (e errReader) Read(_ []byte) (int, error) {
	return 0, readerErr
}

func TestReaderParser(t *testing.T) {
	t.Parallel()

	type test struct {
		name     string
		input    io.Reader
		expected []parser.Field
		err      error
	}

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
		},
		{
			name:  "Error",
			input: errReader{nil},
			err:   readerErr,
		},
		{
			name:  "Error from byte parser (no final newline)",
			input: strings.NewReader("data: lmao"),
			err:   parser.ErrUnexpectedEOF,
		},
		{
			name:     "With BOM",
			input:    strings.NewReader("\xEF\xBB\xBFdata: hello\n"),
			expected: []parser.Field{newDataField(t, "hello")},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			p := parser.NewReaderParser(test.input)
			var fields []parser.Field
			if l := len(test.expected); l > 0 {
				fields = make([]parser.Field, 0, l)
			}

			for p.Scan() {
				fields = append(fields, p.Field())
			}

			if err := p.Err(); err != test.err {
				t.Fatalf("invalid error: received %v, expected %v", err, test.err)
			}

			if !reflect.DeepEqual(test.expected, fields) {
				t.Fatalf("parse failed:\nreceived: %#v\nexpected: %#v", fields, test.expected)
			}
		})
	}
}

func BenchmarkReaderParser(b *testing.B) {
	var f parser.Field

	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		r := strings.NewReader(benchmarkText)
		p := parser.NewReaderParser(r)

		for p.Scan() {
			f = p.Field()
		}
	}

	_ = f
}
