package event

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

func TestNew(t *testing.T) {
	t.Parallel()

	input := []Field{
		Name("whatever"),
		ID("again"),
		Text("input"),
		Retry(30),
		Raw([]byte("amazing")),
		Retry(time.Second),
		ID("lol"),
		Name("x"),
	}

	e := New(input...)

	if id := e.ID(); id != "lol" {
		t.Fatalf("Invalid event ID: received %q, expected %q", id, "lol")
	}
}

func TestEvent_WriteTo(t *testing.T) {
	t.Parallel()

	input := []Field{
		Text("This is an example\nOf an event"),
		Text(""), // empty fields should not produce any output
		ID("example_id"),
		Retry(time.Second * 5),
		Raw([]byte("raw bytes here")),
		Name("test_event"),
		Comment("This test should pass"),
		Text("Important data\nImportant again\r\rVery important\r\n"),
	}

	output := "data: This is an example\ndata: Of an event\nid: example_id\nretry: 5000\ndata: raw bytes here\nevent: test_event\n: This test should pass\ndata: Important data\ndata: Important again\rdata: \rdata: Very important\r\n\n"
	expectedWritten := int64(len(output))
	expected := util.EscapeNewlines(output)

	e := New(input...)
	w := &strings.Builder{}

	written, err := e.WriteTo(w)
	if err != nil {
		t.Fatalf("Failed to write event: %v", err)
	}

	got := util.EscapeNewlines(w.String())

	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("Event written incorrectly:\nexpected: %s\nreceived: %s", expected, got)
	}

	if written != expectedWritten {
		t.Fatalf("Written byte count wrong: expected %d, got %d", expectedWritten, written)
	}

}

func TestEvent_SetExpiry(t *testing.T) {
	t.Parallel()

	e := New()
	now := time.Now()

	e.SetExpiry(now)

	if e.expiresAt != now {
		t.Fatalf("Failed to set expiry time: got %v, want %v", e.expiresAt, now)
	}
}

func TestFrom(t *testing.T) {
	t.Parallel()

	now := time.Now()
	e := New(Text("A field"))
	e.SetExpiry(now)
	derivate := From(e, Text("Another field")) //nolint
	expected := []Field{Text("A field"), Text("Another field")}

	if derivate.ExpiresAt() != now {
		t.Fatalf("Expiry date not set correctly: expected %v, got %v", now, derivate.ExpiresAt())
	}
	if !reflect.DeepEqual(derivate.fields, expected) {
		t.Fatalf("Fields not set correctly:\nreceived %v\nexpected %v", derivate.fields, expected)
	}
}

func TestEvent_UnmarshalText(t *testing.T) {
	t.Parallel()

	type test struct {
		name        string
		input       string
		expectedErr error
		expected    []Field
	}

	tests := []test{
		{
			name:        "No input",
			expectedErr: &UnmarshalError{Reason: errors.New("unexpected end of input")},
		},
		{
			name:  "Invalid retry field",
			input: "retry: sigma male",
			expectedErr: &UnmarshalError{
				FieldName:  string(parser.FieldNameRetry),
				FieldValue: "sigma male",
				Reason:     fmt.Errorf("contains character %q, which is not an ASCII digit", 's'),
			},
		},
		{
			name:  "Valid input",
			input: "data: raw bytes here\nretry: 1000\nid: 2000\n: no comments\ndata: again raw bytes\ndata: from multiple lines\nevent: my name here\n\ndata: I should be ignored",
			expected: []Field{
				Raw([]byte("raw bytes here")),
				Retry(time.Second),
				ID("2000"),
				Raw([]byte("again raw bytes")),
				Raw([]byte("from multiple lines")),
				Name("my name here"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := &Event{}

			if err := e.UnmarshalText([]byte(test.input)); (test.expectedErr != nil && err.Error() != test.expectedErr.Error()) || (test.expectedErr == nil && err != nil) {
				t.Fatalf("Invalid unmarshal error: got %q, want %q", err, test.expectedErr)
			}
			if !reflect.DeepEqual(e.fields, test.expected) {
				t.Fatalf("Invalid unmarshal fields:\nreceived %v\nexpected %v", e.fields, test.expected)
			}
		})
	}
}

var benchmarkEvent = New(
	Text("Example data\nWith multiple rows\r\nThis is interesting"),
	ID("example_id"),
	Comment("An useless comment here that spans\non\n\nmultiple\nlines"),
	Name("This is the event's name"),
	Retry(time.Minute),
)

func BenchmarkEvent_WriteTo(b *testing.B) {
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		_, _ = benchmarkEvent.WriteTo(io.Discard)
	}
}

var benchmarkText = []string{
	"Lorem ipsum dolor sit amet, consectetur adipiscing elit.",
	"Pellentesque at dui non quam faucibus ultricies.",
	"Quisque non sem gravida, sodales lorem eget, lobortis est.",
	"Quisque porttitor nunc eu mollis congue.",
	"Vivamus sollicitudin tellus ut mi malesuada lacinia.",
	"Aenean aliquet tortor non urna sodales dignissim.",
	"Sed quis diam sed dui feugiat aliquam.",
	"Etiam sit amet neque cursus, semper nibh non, ornare nunc.",
	"Phasellus dignissim lacus vitae felis interdum, eget pharetra augue bibendum.",
	"Sed euismod enim sed ante laoreet, non ullamcorper enim dapibus.",
	"Ut accumsan arcu venenatis, egestas nisi consectetur, dignissim felis.",
	"Praesent lacinia elit ut tristique molestie.",
	"Mauris ut nibh id ante ultricies egestas.",
	"Mauris porttitor augue quis maximus efficitur.",
	"Fusce auctor enim viverra elit imperdiet, non dignissim dolor condimentum.",
	"Fusce scelerisque quam vel erat tempor elementum.",
	"Nullam ac velit in nisl hendrerit rhoncus sed ut dui.",
	"Pellentesque laoreet arcu vitae commodo gravida.",
	"Pellentesque sagittis enim quis sapien mollis tempor.",
	"Phasellus fermentum leo vitae odio efficitur, eu lacinia enim elementum.",
	"Morbi faucibus nisi a velit dictum eleifend.",
}

func getBuffer(tb testing.TB, text []string) []Field {
	tb.Helper()

	return make([]Field, 0, len(text))
}

func getOpts(buf []Field, text []string) []Field {
	for _, t := range text {
		buf = append(buf, Text(t))
	}
	return buf
}

func createEvent(tb testing.TB, text []string) *Event {
	return New(getOpts(getBuffer(tb, text), text)...)
}

func BenchmarkEvent_WriteTo_text(b *testing.B) {
	ev := createEvent(b, benchmarkText)

	for n := 0; n < b.N; n++ {
		_, _ = ev.WriteTo(io.Discard)
	}
}

func BenchmarkNew(b *testing.B) {
	type benchmark struct {
		name string
		text []string
	}

	benchmarks := []benchmark{
		{
			name: "Single-line text",
			text: benchmarkText,
		},
	}

	for _, bench := range benchmarks {
		buf := getBuffer(b, bench.text)

		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()

			var e *Event
			for n := 0; n < b.N; n++ {
				e = New(getOpts(buf, bench.text)...)
			}
			_ = e
		})
	}
}
