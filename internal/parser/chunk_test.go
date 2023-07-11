package parser

import (
	"reflect"
	"testing"
	"unsafe"
)

func getStringDataAddress(tb testing.TB, b string) uintptr {
	tb.Helper()

	return (*reflect.StringHeader)(unsafe.Pointer(&b)).Data
}

func TestNextChunk(t *testing.T) {
	t.Parallel()

	s := "sarmale"
	chunk, remaining, hasNewline := NextChunk(s)

	if remaining != "" {
		t.Fatalf("No more data should be remaining")
	}

	if getStringDataAddress(t, s) != getStringDataAddress(t, chunk) {
		t.Fatalf("First chunk should always have the same address as the given buffer")
	}

	if s != chunk {
		t.Fatalf("Expected chunk %q, got %q", s, chunk)
	}

	if hasNewline {
		t.Fatalf("Ends in newline flag incorrect: expected %t, got %t", false, hasNewline)
	}

	s = "sarmale cu\nghimbir\r\nsunt\rsuper\n\ngenial sincer\r\n"

	expected := []string{
		"sarmale cu",
		"ghimbir",
		"sunt",
		"super",
		"",
		"genial sincer",
	}

	var got []string

	for s != "" {
		var chunk string
		chunk, s, _ = NextChunk(s)

		got = append(got, chunk)
	}

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Bad result:\n\texpected %#v\n\treceived %#v", expected, got)
	}
}
