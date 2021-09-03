package parser_test

import (
	"reflect"
	"testing"

	"github.com/tmaxmax/go-sse/internal/parser"
)

func TestByteParser(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name     string
		data     string
		expected []parser.Field
	}

	tests := []testCase{
		{
			name: "Normal data",
			data: "event: sarmale\ndata:doresc sarmale\ndata:  multe sarmale  \r\n\n",
			expected: []parser.Field{
				newEventField(t, "sarmale"),
				newDataField(t, "doresc sarmale"),
				newDataField(t, " multe sarmale  "),
				{},
			},
		},
		{
			name: "Normal data with comments",
			data: ":comment\r: another comment\ndata: whatever",
			expected: []parser.Field{
				newDataField(t, "whatever"),
			},
		},
		{
			name: "Fields without colon",
			data: "data\ndata\ndata: some data\n\n",
			expected: []parser.Field{
				newDataField(t, ""),
				newDataField(t, ""),
				newDataField(t, "some data"),
				{},
			},
		},
		{
			name: "Invalid fields",
			data: "i'm an invalid field:\nlmao me too\nretry: 120\nid: 5\r\n\r\n",
			expected: []parser.Field{
				newRetryField(t, "120"),
				newIDField(t, "5"),
				{},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			p := parser.NewByteParser([]byte(test.data))
			var segments []parser.Field

			for p.Scan() {
				segments = append(segments, p.Field())
			}

			if !reflect.DeepEqual(test.expected, segments) {
				t.Fatalf("invalid segments for test %q:\nreceived %#v\nexpected %#v", test.name, segments, test.expected)
			}
		})
	}
}

func BenchmarkByteParser(b *testing.B) {
	var f parser.Field
	data := []byte(benchmarkText)

	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		p := parser.NewByteParser(data)
		for p.Scan() {
			f = p.Field()
		}
	}

	_ = f
}
