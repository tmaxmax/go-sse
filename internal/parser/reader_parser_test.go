package parser_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tmaxmax/go-sse/internal/parser"
)

func TestReaderParser(t *testing.T) {
	t.Parallel()

	r := strings.NewReader(`
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
data: still, here's some data: you deserve it`)
	p := parser.NewReaderParser(r)
	expected := []parser.Field{
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
	}
	fields := make([]parser.Field, 0, len(expected))

	for p.Scan() {
		fields = append(fields, p.Field())
	}

	if p.Err() != nil {
		t.Fatalf("unexpected error: %v", p.Err())
	}

	if !reflect.DeepEqual(expected, fields) {
		t.Fatalf("parse failed:\nreceived: %#v\nexpected: %#v", fields, expected)
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
