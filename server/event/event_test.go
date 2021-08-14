package event

import (
	"encoding/base64"
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

	input := []Field{
		Name("whatever"),
		ID("again"),
		Text("input"),
		Retry(30),
		Raw("amazing"),
		Retry(time.Second),
		ID("lol"),
		Name("x"),
	}

	expected := []Field{
		Name("x"),
		ID("lol"),
		Text("input"),
		Retry(time.Second),
		Raw("amazing"),
	}

	e := NewEvent(input...)

	if d := deep.Equal(expected, e.fields); d != nil {
		t.Fatalf("Fields set incorrectly:\n%v", d)
	}
}

type testJSON struct {
	Key        string `json:"key"`
	AnotherKey int    `json:"anotherKey"`
}

func TestEvent_WriteTo(t *testing.T) {
	t.Parallel()

	input := []Field{
		Text("This is an example\nOf an event"),
		ID("example_id"),
		Retry(time.Second * 5),
		Base64{
			Payload:  []byte("amazing testing experience"),
			Encoding: base64.StdEncoding,
		},
		JSON{Value: testJSON{
			Key:        "value",
			AnotherKey: 5,
		}},
		Name("test_event"),
		Comment("This test should pass"),
		Text("Important data\nImportant again\r\rVery important\r\n"),
	}

	expected := escape("data:This is an example\ndata:Of an event\nid:example_id\nretry:5000\ndata:YW1hemluZyB0ZXN0aW5nIGV4cGVyaWVuY2U=\ndata:{\"key\":\"value\",\"anotherKey\":5}\nevent:test_event\n:This test should pass\ndata:Important data\ndata:Important again\rdata:\rdata:Very important\r\n\n")

	e := NewEvent(input...)
	r := &strings.Builder{}

	_, err := e.WriteTo(r)
	if err != nil {
		t.Fatalf("Failed to write event: %v", err)
	}

	got := escape(r.String())

	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("Event written incorrectly:\n%v", diff.Diff(expected, got))
	}
}
