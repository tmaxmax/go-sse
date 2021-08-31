package event

import (
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse/internal/util"
)

func TestNewEvent(t *testing.T) {
	t.Parallel()

	input := []Option{
		Name("whatever"),
		ID("again"),
		Text("input"),
		Retry(30),
		Raw("amazing"),
		Retry(time.Second),
		ID("lol"),
		Name("x"),
	}

	expected := []field{
		Name("x"),
		ID("lol"),
		Text("input"),
		Retry(time.Second),
		Raw("amazing"),
	}

	e := New(input...)

	if !reflect.DeepEqual(e.fields, expected) {
		t.Fatalf("Fields set incorrectly:\nreceived: %v\nexpected: %v", e.fields, expected)
	}
}

func TestEvent_WriteTo(t *testing.T) {
	t.Parallel()

	input := []Option{
		Text("This is an example\nOf an event"),
		ID("example_id"),
		Retry(time.Second * 5),
		MustRawLine([]byte("raw bytes here")),
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

	if written != expectedWritten {
		t.Fatalf("Written byte count wrong: expected %d, got %d", expectedWritten, written)
	}

	got := util.EscapeNewlines(w.String())

	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("Event written incorrectly:\nexpected: %s\nreceived: %s", expected, got)
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

func BenchmarkEvent_WriteTo_text(b *testing.B) {
	fields := make([]Option, 0, len(benchmarkText))
	for _, t := range benchmarkText {
		fields = append(fields, Text(t))
	}
	ev := New(fields...)

	for n := 0; n < b.N; n++ {
		_, _ = ev.WriteTo(io.Discard)
	}
}

func BenchmarkEvent_WriteTo_line(b *testing.B) {
	fields := make([]Option, 0, len(benchmarkText))
	for _, t := range benchmarkText {
		fields = append(fields, MustLine(t))
	}
	ev := New(fields...)

	for n := 0; n < b.N; n++ {
		_, _ = ev.WriteTo(io.Discard)
	}
}
