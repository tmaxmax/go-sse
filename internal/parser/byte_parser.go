package parser

import (
	"bytes"
	"errors"
)

// ByteParser extracts fields from a byte slice.
type ByteParser struct {
	err   error
	field Field
	chunk []byte
	cs    ChunkScanner
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (b *ByteParser) scanSegmentName() (FieldName, bool) {
	colonPos, l := bytes.IndexByte(b.chunk, ':'), len(b.chunk)
	if colonPos > maxFieldNameLength {
		return "", false
	}
	if colonPos == -1 {
		colonPos = l
	}

	name, ok := getFieldName(b.chunk[:colonPos])
	if ok {
		dataStart := min(colonPos+1, l)
		b.chunk = b.chunk[dataStart:]
		return name, true
	}
	return name, false
}

// ErrUnexpectedEOF is returned when the input is completely parsed but no complete field was found at the end.
var ErrUnexpectedEOF = errors.New("go-sse: unexpected end of input")

// Scan parses the next available filed in the remaining buffer.
// It returns false if there are no fields to parse.
func (b *ByteParser) Scan() bool {
	for {
		if !b.cs.Scan() {
			return false
		}

		var endsInNewline bool
		b.chunk, endsInNewline = b.cs.Chunk()
		if !endsInNewline {
			// If the chunk doesn't end in a newline we have reached EOF.
			// Fields should always end in a newline, so we return false, as this is not a complete field.
			b.err = ErrUnexpectedEOF
			return false
		}

		name, ok := b.scanSegmentName()
		if !ok {
			// Ignore the field
			continue
		}

		b.field = Field{Name: name, Value: trimChunk(b.chunk)}

		return true
	}
}

// Field returns the last parsed field.
func (b *ByteParser) Field() Field {
	return b.field
}

// Reset changes the buffer ByteParser parses fields from.
func (b *ByteParser) Reset(p []byte) {
	b.cs.Reset(p)
}

// Err returns the last error encountered by the parser. It is either nil or ErrUnexpectedEOF.
func (b *ByteParser) Err() error {
	return b.err
}

func trimNewline(c []byte) []byte {
	l := len(c)
	if l > 0 && c[l-1] == '\n' {
		c = c[:l-1]
		l--
	}
	if l > 0 && c[l-1] == '\r' {
		c = c[:l-1]
	}

	return c
}

func trimFirstSpace(c []byte) []byte {
	if len(c) == 0 {
		return nil
	}
	if c[0] == ' ' {
		return c[1:]
	}
	return c
}

func trimChunk(c []byte) []byte {
	return trimFirstSpace(trimNewline(c))
}

// NewByteParser creates a parser that extracts fields from the given byte slice.
func NewByteParser(b []byte) *ByteParser {
	return &ByteParser{cs: NewChunkScanner(b)}
}
