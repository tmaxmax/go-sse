package parser

import (
	"bytes"
	"reflect"
	"testing"
	"unsafe"
)

func getByteSliceDataAddress(tb testing.TB, b []byte) uintptr {
	tb.Helper()

	return (*reflect.SliceHeader)(unsafe.Pointer(&b)).Data
}

func TestNextChunk(t *testing.T) {
	t.Parallel()

	buf := []byte("sarmale")
	chunk, remaining := NextChunk(buf)

	if len(remaining) != 0 {
		t.Fatalf("No more data should be remaining")
	}

	if getByteSliceDataAddress(t, buf) != getByteSliceDataAddress(t, chunk.Data) {
		t.Fatalf("First chunk should always have the same address as the given buffer")
	}

	if !bytes.Equal(buf, chunk.Data) {
		t.Fatalf("Expected chunk %q, got %q", string(buf), string(chunk.Data))
	}

	if chunk.HasNewline {
		t.Fatalf("Ends in newline flag incorrect: expected %t, got %t", false, chunk.HasNewline)
	}

	buf = []byte("sarmale cu\nghimbir\r\nsunt\rsuper\n\ngenial sincer\r\n")

	expected := []Chunk{
		{[]byte("sarmale cu\n"), true},
		{[]byte("ghimbir\r\n"), true},
		{[]byte("sunt\r"), true},
		{[]byte("super\n"), true},
		{[]byte("\n"), true},
		{[]byte("genial sincer\r\n"), true},
	}

	var got []Chunk

	for len(buf) != 0 {
		var chunk Chunk
		chunk, buf = NextChunk(buf)

		got = append(got, chunk)
	}

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Bad result:\n\texpected %#v\n\treceived %#v", expected, got)
	}
}
