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

func TestScanner(t *testing.T) {
	t.Parallel()

	buf := []byte("sarmale")
	s := NewChunkScanner(buf)

	if !s.Scan() {
		t.Fatalf("Scan should return true")
	}

	chunk, endsInNewline := s.Chunk()

	if getByteSliceDataAddress(t, buf) != getByteSliceDataAddress(t, chunk) {
		t.Fatalf("First chunk should always have the same address as the given buffer")
	}

	if !bytes.Equal(buf, chunk) {
		t.Fatalf("Expected chunk %q, got %q", string(buf), string(chunk))
	}

	if endsInNewline {
		t.Fatalf("Ends in newline flag incorrect: expected %t, got %t", false, endsInNewline)
	}

	type result struct {
		chunk         string
		endsInNewline bool
	}

	s.Reset([]byte("sarmale cu\nghimbir\r\nsunt\rsuper\n\ngenial sincer\r\n"))

	expected := []result{
		{"sarmale cu\n", true},
		{"ghimbir\r\n", true},
		{"sunt\r", true},
		{"super\n", true},
		{"\n", true},
		{"genial sincer\r\n", true},
	}

	var got []result

	for s.Scan() {
		chunk, endsInNewline := s.Chunk()

		got = append(got, result{string(chunk), endsInNewline})
	}

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Bad result:\n\texpected %#v\n\treceived %#v", expected, got)
	}
}
