package event

import (
	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// The DataField contains data payload consisting of bytes that represent a UTF-8 encoded string.
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
// any newline characters (\n or \n) and create a data field using the provided functions, Raw or Text.
type DataField struct {
	data       []byte
	singleLine bool
}

func (d DataField) apply(e *Event) {
	e.fields = append(e.fields, d)
}

func (d DataField) repr() (field, data []byte, singleLine bool) {
	return fieldBytesData, d.data, d.singleLine
}

// Raw creates a data field from a byte slice.
func Raw(p []byte) DataField {
	return DataField{data: p, singleLine: isSingleLine(p)}
}

// Text creates a data field from a string. It uses the unsafe package to convert the string to a byte slice,
// so no allocations take place. If the string is a constant, the conversion may panic, so use the Raw function
// to create a data field from a copy of the constant instead.
func Text(s string) DataField {
	p := util.Bytes(s)
	return DataField{data: p, singleLine: isSingleLine(p)}
}

func isSingleLine(p []byte) bool {
	s := parser.ChunkScanner{Buffer: p}
	s.Scan()
	_, endsInNewline := s.Chunk()
	return !endsInNewline
}
