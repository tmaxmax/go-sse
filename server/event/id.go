package event

import (
	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// ID is an event field that sets the event's id.
// It takes a string as its value, given that IDs can be of any value.
//
// Use strconv.Itoa if you want to use integer IDs.
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

func (i ID) repr() []byte {
	return util.Bytes(string(i))
}
