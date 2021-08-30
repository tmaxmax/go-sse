package event

import (
	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// Raw is a multiline data payload consisting of bytes that represent an UTF-8 encoded string.
// Do not use this for binary data!
type Raw []byte

func (r Raw) name() parser.FieldName {
	return parser.FieldNameData
}

func (r Raw) apply(e *Event) {
	e.fields = append(e.fields, r)
}

func (r Raw) repr() ([]byte, bool) {
	return r, false
}

// Text is a data payload consisting of a UTF-8 encoded string.
type Text string

func (t Text) name() parser.FieldName {
	return parser.FieldNameData
}

func (t Text) apply(e *Event) {
	e.fields = append(e.fields, t)
}

func (t Text) repr() ([]byte, bool) {
	return util.Bytes(string(t)), false
}

func checkLine(p []byte) bool {
	s := parser.ChunkScanner{Buffer: p}
	s.Scan()
	_, endsInNewline := s.Chunk()
	return !endsInNewline
}

// Line creates a data field that contains a single line of text.
// Use it if you are sure your input doesn't span over multiple lines, as writing the field will be optimized.
// If the input does not respect the rules (contains either \n or \r), a nil pointer is returned.
func Line(s string) *LineField {
	if !checkLine(util.Bytes(s)) {
		return nil
	}

	return &LineField{s: s}
}

// LineField is a checked data field which contains a single line of text.
type LineField struct {
	s string
}

func (s *LineField) name() parser.FieldName {
	return parser.FieldNameData
}

func (s *LineField) apply(e *Event) {
	e.fields = append(e.fields, s)
}

func (s *LineField) repr() ([]byte, bool) {
	return util.Bytes(s.s), true
}

// RawLine creates a data field that has a value without any newlines except possibly one at the end.
// Use it if you are sure your input doesn't span over multiple lines, as writing the field will be optimized.
// If the input does not respect the rules (contains either \n or \r), a nil pointer is returned.
func RawLine(p []byte) *RawLineField {
	if !checkLine(p) {
		return nil
	}

	return &RawLineField{buf: p}
}

// RawLineField is a checked data field which contains a byte slice that can have only a single newline at the end.
type RawLineField struct {
	buf []byte
}

func (r *RawLineField) name() parser.FieldName {
	return parser.FieldNameData
}

func (r *RawLineField) apply(e *Event) {
	e.fields = append(e.fields, r)
}

func (r *RawLineField) repr() ([]byte, bool) {
	return r.buf, true
}
