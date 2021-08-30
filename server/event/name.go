package event

import (
	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// Name is the event field that sets the event's type.
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

func (n Name) repr() []byte {
	return util.Bytes(string(n))
}
