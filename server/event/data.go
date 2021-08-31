package event

import (
	"fmt"

	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// Raw is a multiline data payload consisting of bytes that represent a UTF-8 encoded string.
//
// It is not suited for binary data. In general the server-sent events protocol is not suitable
// for sending binary data: the event fields are delimited by newlines, where a newline can be
// a LF, CR or CRLF sequence. When the client interprets the fields, it joins multiple data
// fields using LF, so information is altered. Here's an example:
//
//   initial payload: This is a\r\nmultiline\rtext.\nIt has multiple\nnewline\r\nvariations.
//   data sent over the wire:
//     data: This is a
//     data: multiline
//     data: text.
//     data: It has multiple
//     data: newline
//     data: variations
//   data received by client: This is a\nmultiline\ntext.\nIt has multiple\nnewline\nvariations.
//
// Each line prepended with "data:" is a field; multiple data fields are joined together using LF as the delimiter.
// If you attempted to send the same payload without prepending the "data:" prefix, like so:
//
//   data: This is a
//   multiline
//   text.
//   It has multiple
//   newline
//   variations
//
// there would be only one data field (the first one). The rest would be different fields, named "multiline", "text.",
// "It has multiple" etc., which are invalid fields according to the protocol.
//
// Besides, the protocol explicitly states that event streams must always be UTF-8 encoded:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream.
//
// Still, if you need to send binary data, you can use a Base64 encoder or any other encoder that does not output
// any newline characters (\n or \n) and create a data field using the Line or RawLine functions with the encoded output.
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

// Line creates a data field that is guaranteed to not have any newline sequences (\n, \r\n or \r).
// It checks the input beforehand, and if it's valid, it returns a value containing the data and a truthy boolean value.
// If the data is not valid, it returns the zero value for the field type and a false boolean value.
//
// It is recommended to use this utility over using Text if your data doesn't have multiple lines,
// as writing it will be heavily optimized.
func Line(s string) (LineField, bool) {
	if !checkLine(util.Bytes(s)) {
		return LineField{}, false
	}

	return LineField{s: s}, true
}

// MustLine is a convenience wrapper for Line that panics if the input is not valid. Use it if you're sure
// the input has only a single line.
func MustLine(s string) LineField {
	line, ok := Line(s)
	if !ok {
		panic(fmt.Sprintf("tried to create line field with multiline input: %q", s))
	}

	return line
}

// LineField is a checked data field which contains a single line of text.
type LineField struct {
	s string
}

func (s LineField) name() parser.FieldName {
	return parser.FieldNameData
}

func (s LineField) apply(e *Event) {
	e.fields = append(e.fields, s)
}

func (s LineField) repr() ([]byte, bool) {
	return util.Bytes(s.s), true
}

// RawLine creates a data field that is guaranteed to not have any newline sequences (\n, \r\n or \r).
// It checks the input beforehand, and if it's valid, it returns a value containing the data and a truthy boolean value.
// If the data is not valid, it returns the zero value for the field type and a false boolean value.
//
// It is recommended to use this utility over using Raw if your data doesn't have multiple lines,
// as writing it will be heavily optimized.
func RawLine(p []byte) (RawLineField, bool) {
	if !checkLine(p) {
		return RawLineField{}, false
	}

	return RawLineField{buf: p}, true
}

// MustRawLine is a convenience wrapper for RawLine that panics if the input is not valid. Use it if you're sure
// the input has only a single line.
func MustRawLine(p []byte) RawLineField {
	line, ok := RawLine(p)
	if !ok {
		panic(fmt.Sprintf("tried to create raw line field with multiline input: %q", string(p)))
	}

	return line
}

// RawLineField is a checked data field which contains a byte slice that can have only a single newline at the end.
type RawLineField struct {
	buf []byte
}

func (r RawLineField) name() parser.FieldName {
	return parser.FieldNameData
}

func (r RawLineField) apply(e *Event) {
	e.fields = append(e.fields, r)
}

func (r RawLineField) repr() ([]byte, bool) {
	return r.buf, true
}
