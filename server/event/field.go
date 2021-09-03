package event

import (
	"fmt"
	"strconv"
	"time"

	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// The Field type is a single event field.
type Field struct {
	data       []byte
	nameBytes  []byte
	singleLine bool
}

func isSingleLine(p []byte) bool {
	_, remaining := parser.NewlineIndex(p)
	return remaining == 0
}

func assertSingleLine(s, function string, fieldName parser.FieldName) []byte {
	p := util.Bytes(s)
	if !isSingleLine(p) {
		panic(fmt.Sprintf("go-sse.server.event.%s: %s value %q is not single-line", function, fieldName, s))
	}
	return p
}

// fieldBytes holds the byte representation of each field type along with a colon at the end.
var fieldBytes = map[parser.FieldName][]byte{
	parser.FieldNameData:  []byte(parser.FieldNameData + ": "),
	parser.FieldNameEvent: []byte(parser.FieldNameEvent + ": "),
	parser.FieldNameRetry: []byte(parser.FieldNameRetry + ": "),
	parser.FieldNameID:    []byte(parser.FieldNameID + ": "),
	"":                    {':', ' '},
}

// Raw creates a data field that is a UTF-8 encoded text payload from a byte slice.
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
func Raw(p []byte) Field {
	return Field{
		nameBytes:  fieldBytes[parser.FieldNameData],
		data:       p,
		singleLine: isSingleLine(p),
	}
}

// Text creates a data field from a string. It uses the unsafe package to convert the string to a byte slice,
// so no allocations take place. See the Raw function's documentation for details about the data field type.
func Text(s string) Field {
	return Raw(util.Bytes(s))
}

// Comment creates an event comment field. If it spans on multiple lines,
// new comment lines are created.
func Comment(comment string) Field {
	p := util.Bytes(comment)
	return Field{
		nameBytes:  fieldBytes[""],
		data:       p,
		singleLine: isSingleLine(p),
	}
}

// ID is an event field that sets the event's id.
// It takes a string as its value, given that IDs can be of any value.
// If an ID has a null byte inside its value is ignored by the client.
//
// Use functions from the fmt or strconv packages if you want to use numbers for ID values.
//
// An ID cannot have multiple lines, ensure it does not contain any newline characters (\n or \r).
// The library does not do any checks so if the condition is not met the protocol will be broken.
func ID(id string) Field {
	return Field{
		nameBytes:  fieldBytes[parser.FieldNameID],
		data:       assertSingleLine(id, "ID", parser.FieldNameID),
		singleLine: true,
	}
}

// Name is the event field that sets the event's type.
//
// A Name cannot have multiple lines. Make sure this condition is met or the protocol will be broken.
func Name(name string) Field {
	return Field{
		nameBytes:  fieldBytes[parser.FieldNameEvent],
		data:       assertSingleLine(name, "Name", parser.FieldNameEvent),
		singleLine: true,
	}
}

// Retry creates a field that tells the client to set the event stream reconnection time to
// the number of milliseconds it provides.
func Retry(t time.Duration) Field {
	return Field{
		nameBytes:  fieldBytes[parser.FieldNameRetry],
		data:       strconv.AppendInt(nil, t.Milliseconds(), 10),
		singleLine: true,
	}
}
