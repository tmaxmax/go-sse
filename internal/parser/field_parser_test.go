package parser_test

import (
	"reflect"
	"testing"

	"github.com/tmaxmax/go-sse/internal/parser"
)

func TestFieldParser(t *testing.T) {
	t.Parallel()

	t.Run("Empty input", func(t *testing.T) {
		p := parser.NewFieldParser("")
		if p.Next(nil) {
			t.Fatalf("empty input should not yield data")
		}
		if p.Started() {
			t.Fatalf("parsing empty input should have no effects")
		}
		p.RemoveBOM(true)
		if p.Started() {
			t.Fatalf("BOM shouldn't be removed on empty input")
		}
	})

	type testCase struct {
		err          error
		name         string
		data         string
		expected     []parser.Field
		keepComments bool
	}

	tests := []testCase{
		{
			name: "Normal data",
			data: "event: sarmale\ndata:doresc sarmale\n: comentariu\ndata:  multe sarmale  \r\n\n",
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
		{
			name:         "Normal data with comments",
			data:         "data: hello\ndata: world\r: comm\r\n:other comm\nevent: test\n",
			keepComments: true,
			expected: []parser.Field{
				newDataField(t, "hello"),
				newDataField(t, "world"),
				newCommentField(t, "comm"),
				newCommentField(t, "other comm"),
				newEventField(t, "test"),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			p := parser.NewFieldParser(test.data)
			p.KeepComments(test.keepComments)

			var segments []parser.Field

			for f := (parser.Field{}); p.Next(&f); {
				segments = append(segments, f)
			}

			if !p.Started() {
				t.Fatalf("parsing should be marked as having started")
			}
			if p.Err() != test.err { //nolint
				t.Fatalf("invalid error: received %v, expected %v", p.Err(), test.err)
			}
			if !reflect.DeepEqual(test.expected, segments) {
				t.Fatalf("invalid segments for test %q:\nreceived %#v\nexpected %#v", test.name, segments, test.expected)
			}
		})
	}

	t.Run("BOM", func(t *testing.T) {
		p := parser.NewFieldParser("\xEF\xBB\xBFid: 5\n")
		p.RemoveBOM(true)

		var f parser.Field
		if !p.Next(&f) {
			t.Fatalf("a field should be available (err=%v)", p.Err())
		}

		expectedF := parser.Field{Name: parser.FieldNameID, Value: "5"}
		if f != expectedF {
			t.Fatalf("invalid field: received %v, expected %v", f, expectedF)
		}

		p.Reset("\xEF\xBB\xBF")
		if p.Next(&f) {
			t.Fatalf("no fields should be available")
		}
		if p.Err() != nil {
			t.Fatalf("no error is expected after BOM removal")
		}
		if !p.Started() {
			t.Fatalf("BOM removal should mark parsing as having started")
		}

		p.Reset("data: no BOM\n")
		if p.Started() {
			t.Fatalf("data has no BOM so no advancement should be made")
		}
	})
}

func BenchmarkFieldParser(b *testing.B) {
	b.ReportAllocs()

	var f parser.Field

	for n := 0; n < b.N; n++ {
		p := parser.NewFieldParser(benchmarkText)
		for p.Next(&f) {
		}
	}

	_ = f
}
