package parser

import "bytes"

type ByteParser struct {
	field Field
	cs    ChunkScanner
	chunk []byte
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

// Scan parses the next available filed in the remaining buffer.
// It returns false if there are no fields to parse.
func (b *ByteParser) Scan() bool {
	for {
		if !b.cs.Scan() {
			return false
		}

		b.chunk, _ = b.cs.Chunk()
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
	b.cs.Buffer = p
}

func trimNewline(c []byte) []byte {
	l := len(c)
	if c[l-1] == '\n' {
		c = c[:l-1]
		l--
	}
	if l > 0 && c[l-1] == '\r' {
		c = c[:l-1]
	}

	return c
}

func trimFirstSpace(c []byte) []byte {
	if c[0] == ' ' {
		return c[1:]
	}
	return c
}

func trimChunk(c []byte) []byte {
	if len(c) == 0 {
		return nil
	}
	return trimFirstSpace(trimNewline(c))
}

func NewByteParser(b []byte) *ByteParser {
	return &ByteParser{cs: ChunkScanner{Buffer: b}}
}
