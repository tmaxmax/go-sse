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

func scanSegment(chunk string, out *Field) bool {
	colonPos, l := strings.IndexByte(chunk, ':'), len(chunk)
	if colonPos > maxFieldNameLength {
		return false
	}
	if colonPos == -1 {
		colonPos = l
	}

	name, ok := getFieldName(chunk[:colonPos])
	if ok {
		out.Name = name
		out.Value = trimFirstSpace(chunk[min(colonPos+1, l):])
	} else if chunk == "" {
		// scanSegment is called only with chunks which end with a newline in the input.
		// If chunk is empty, it means that this is a blank line which ends the event,
		// so an empty Field needs to be returned.
		out.Name = ""
		out.Value = ""
	}

	return ok || chunk == ""
}

// ErrUnexpectedEOF is returned when the input is completely parsed but no complete field was found at the end.
var ErrUnexpectedEOF = errors.New("go-sse: unexpected end of input")

// Next parses the next available field in the remaining buffer.
// It returns false if there are no more fields to parse.
func (f *FieldParser) Next(r *Field) bool {
	for f.data != "" {
		chunk, rem, hasNewline := NextChunk(f.data)
		if !hasNewline {
			f.err = ErrUnexpectedEOF
			return false
		}

		f.data = rem

		if !scanSegment(chunk, r) {
			continue
		}

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

func trimFirstSpace(c string) string {
	if c != "" && c[0] == ' ' {
		return c[1:]
	}
	return c
}

// NewFieldParser creates a parser that extracts fields from the given string.
func NewFieldParser(data string) *FieldParser {
	return &FieldParser{data: data}
}
