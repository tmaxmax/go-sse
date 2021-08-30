package event

import (
	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// Name is the event field that sets the event's type.
//
// A Name cannot have multiple lines. Make sure this condition is met or the protocol will be broken.
type Name string

func (n Name) name() parser.FieldName {
	return parser.FieldNameEvent
}

func (n Name) apply(e *Event) {
	if e.nameIndex == -1 {
		e.nameIndex = len(e.fields)
		e.fields = append(e.fields, n)
	} else {
		e.fields[e.nameIndex] = n
	}
}

func (n Name) repr() ([]byte, bool) {
	return util.Bytes(string(n)), true
}
