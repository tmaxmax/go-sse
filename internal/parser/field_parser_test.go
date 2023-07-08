package parser_test

import (
	"reflect"
	"testing"

	"github.com/tmaxmax/go-sse/internal/parser"
)

func TestByteParser(t *testing.T) {
	t.Parallel()

	type testCase struct {
		err      error
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
			name: "Normal data but no newline at the end",
			data: ":comment\r: another comment\ndata: whatever",
			err:  parser.ErrUnexpectedEOF,
		},
		{
			name: "Fields without data",
			data: "data\ndata  \ndata:\n\n",
			expected: []parser.Field{
				newDataField(t, ""),
				// The second `data  ` should be ignored, as it is not a valid field name
				// (it would be valid without trailing spaces).
				newDataField(t, ""),
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
		{
			name: "Normal data, only one newline at the end",
			data: "data: first chunk\ndata: second chunk\r\n",
			expected: []parser.Field{
				newDataField(t, "first chunk"),
				newDataField(t, "second chunk"),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			p := parser.NewFieldParser([]byte(test.data))
			var segments []parser.Field

			for p.Scan() {
				segments = append(segments, p.Field())
			}

			if p.Err() != test.err { //nolint
				t.Fatalf("invalid error: received %v, expected %v", p.Err(), test.err)
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
		p := parser.NewFieldParser(data)
		for p.Scan() {
			f = p.Field()
		}
	}

	_ = f
}
