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

func (r Raw) repr() []byte {
	return r
}

// Text is a data payload consisting of a UTF-8 encoded string.
type Text string

func (t Text) name() parser.FieldName {
	return parser.FieldNameData
}

func (t Text) apply(e *Event) {
	e.fields = append(e.fields, t)
}

func (t Text) repr() []byte {
	return util.Bytes(string(t))
}
