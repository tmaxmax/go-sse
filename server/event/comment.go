package event

import (
	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// Comment is an event comment field. If it spans on multiple lines,
// new comment lines are created.
type Comment string

func (c Comment) name() parser.FieldName {
	return ""
}

func (c Comment) apply(e *Event) {
	e.fields = append(e.fields, c)
}

func (c Comment) repr() ([]byte, bool) {
	return util.Bytes(string(c)), false
}
