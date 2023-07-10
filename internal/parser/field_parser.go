package parser

import (
	"errors"
	"strings"
)

// FieldParser extracts fields from a byte slice.
type FieldParser struct {
	data string
	err  error
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func scanSegment(chunk string) (name FieldName, data string, valid bool) {
	colonPos, l := strings.IndexByte(chunk, ':'), len(chunk)
	if colonPos > maxFieldNameLength {
		return "", "", false
	}
	if colonPos == -1 {
		colonPos = l
	}

	name, ok := getFieldName(chunk[:colonPos])
	if ok {
		dataStart := min(colonPos+1, l)
		return name, chunk[dataStart:], true
	}
	return "", "", false
}

// ErrUnexpectedEOF is returned when the input is completely parsed but no complete field was found at the end.
var ErrUnexpectedEOF = errors.New("go-sse: unexpected end of input")

// Next parses the next available field in the remaining buffer.
// It returns false if there are no more fields to parse.
func (f *FieldParser) Next(r *Field) bool {
	for f.data != "" {
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
func (f *FieldParser) Reset(data string) {
	f.data = data
	f.err = nil
}

// Err returns the last error encountered by the parser. It is either nil or ErrUnexpectedEOF.
func (f *FieldParser) Err() error {
	return f.err
}

func trimNewline(c string) string {
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

func trimFirstSpace(c string) string {
	if c != "" && c[0] == ' ' {
		return c[1:]
	}
	return c
}

func trimData(c string) string {
	return trimFirstSpace(trimNewline(c))
}

// NewFieldParser creates a parser that extracts fields from the given string.
func NewFieldParser(data string) *FieldParser {
	return &FieldParser{data: data}
}
