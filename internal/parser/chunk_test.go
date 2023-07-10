package parser

import (
	"reflect"
	"testing"
	"unsafe"
)

func getByteSliceDataAddress(tb testing.TB, b string) uintptr {
	tb.Helper()

	return (*reflect.StringHeader)(unsafe.Pointer(&b)).Data
}

func TestNextChunk(t *testing.T) {
	t.Parallel()

	s := "sarmale"
	chunk, remaining := NextChunk(s)

	if remaining != "" {
		t.Fatalf("No more data should be remaining")
	}

	if getByteSliceDataAddress(t, s) != getByteSliceDataAddress(t, chunk.Data) {
		t.Fatalf("First chunk should always have the same address as the given buffer")
	}

	if s != chunk.Data {
		t.Fatalf("Expected chunk %q, got %q", string(s), string(chunk.Data))
	}

	if chunk.HasNewline {
		t.Fatalf("Ends in newline flag incorrect: expected %t, got %t", false, chunk.HasNewline)
	}

	s = "sarmale cu\nghimbir\r\nsunt\rsuper\n\ngenial sincer\r\n"

	expected := []Chunk{
		{"sarmale cu\n", true},
		{"ghimbir\r\n", true},
		{"sunt\r", true},
		{"super\n", true},
		{"\n", true},
		{"genial sincer\r\n", true},
	}

	var got []Chunk

	for s != "" {
		var chunk Chunk
		chunk, s = NextChunk(s)

		got = append(got, chunk)
	}

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Bad result:\n\texpected %#v\n\treceived %#v", expected, got)
	}
}
