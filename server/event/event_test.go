package event

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-test/deep"
	"github.com/kylelemons/godebug/diff"
)

func escape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\\n"), "\r", "\\r")
}

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

	if d := deep.Equal(expected, e.fields); d != nil {
		t.Fatalf("Fields set incorrectly:\n%v", d)
	}
}

func TestEvent_WriteTo(t *testing.T) {
	t.Parallel()

	input := []Option{
		Text("This is an example\nOf an event"),
		ID("example_id"),
		Retry(time.Second * 5),
		Raw("raw bytes here"),
		Name("test_event"),
		Comment("This test should pass"),
		Text("Important data\nImportant again\r\rVery important\r\n"),
	}

	expected := escape("data:This is an example\ndata:Of an event\nid:example_id\nretry:5000\ndata:raw bytes here\nevent:test_event\n:This test should pass\ndata:Important data\ndata:Important again\rdata:\rdata:Very important\r\n\n")

	e := New(input...)
	w := &strings.Builder{}

	if err := e.Message(w); err != nil {
		t.Fatalf("Failed to write event: %v", err)
	}

	got := escape(w.String())

	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("Event written incorrectly:\n%v", diff.Diff(expected, got))
	}
}