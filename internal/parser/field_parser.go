package parser

import (
	"bytes"
	"errors"
)

// FieldParser extracts fields from a byte slice.
type FieldParser struct {
	data []byte
	err  error
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func scanSegment(chunk []byte) (name FieldName, data []byte, valid bool) {
	colonPos, l := bytes.IndexByte(chunk, ':'), len(chunk)
	if colonPos > maxFieldNameLength {
		return "", nil, false
	}
	if colonPos == -1 {
		colonPos = l
	}

	name, ok := getFieldName(chunk[:colonPos])
	if ok {
		dataStart := min(colonPos+1, l)
		return name, chunk[dataStart:], true
	}
	return "", nil, false
}

// ErrUnexpectedEOF is returned when the input is completely parsed but no complete field was found at the end.
var ErrUnexpectedEOF = errors.New("go-sse: unexpected end of input")

// Next parses the next available field in the remaining buffer.
// It returns false if there are no more fields to parse.
func (f *FieldParser) Next(r *Field) bool {
	for len(f.data) != 0 {
		var chunk Chunk
		chunk, f.data = NextChunk(f.data)
		if !chunk.HasNewline {
			f.err = ErrUnexpectedEOF
			return false
		}

		name, data, ok := scanSegment(chunk.Data)
		if !ok {
			continue
		}

		r.Name = name
		r.Value = trimData(data)

		return true
	}

	return false
}

// Reset changes the buffer from which fields are parsed.
func (f *FieldParser) Reset(data []byte) {
	f.data = data
	f.err = nil
}

// Err returns the last error encountered by the parser. It is either nil or ErrUnexpectedEOF.
func (f *FieldParser) Err() error {
	return f.err
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

func trimData(c []byte) []byte {
	return trimFirstSpace(trimNewline(c))
}

// NewFieldParser creates a parser that extracts fields from the given byte slice.
func NewFieldParser(data []byte) *FieldParser {
	return &FieldParser{data: data}
}
