package event

import (
	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// ID is an event field that sets the event's id.
// It takes a string as its value, given that IDs can be of any value.
//
// Use strconv.Itoa if you want to use integer IDs.
//
// An ID cannot have multiple lines, ensure it does not contain any newline characters (\n or \r).
// The library does not do any checks so if the condition is not met the protocol will be broken.
type ID string

func (i ID) name() parser.FieldName {
	return parser.FieldNameID
}

func (i ID) apply(e *Event) {
	if e.idIndex == -1 {
		e.idIndex = len(e.fields)
		e.fields = append(e.fields, i)
	} else {
		e.fields[e.idIndex] = i
	}
}

func (i ID) repr() ([]byte, bool) {
	return util.Bytes(string(i)), true
}
