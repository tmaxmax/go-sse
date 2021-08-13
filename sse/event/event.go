package event

import (
	"io"
	"time"

	"github.com/tmaxmax/go-sse/sse/event/field"
)

type Event struct {
	fields []field.Field
}

func (e *Event) Field(f field.Field) *Event {
	e.fields = append(e.fields, f)

	return e
}

func (e *Event) Name(name string) *Event {
	return e.Field(field.Event{Name: name})
}

func (e *Event) Text(s string) *Event {
	return e.Field(field.Text{Text: s})
}

func (e *Event) Raw(s []byte) *Event {
	return e.Field(field.Raw{Payload: s})
}

func (e *Event) JSON(v interface{}) *Event {
	return e.Field(field.JSON{Value: v})
}

func (e *Event) Base64(s []byte) *Event {
	return e.Field(field.Base64{Payload: s})
}

func (e *Event) ID(id string) *Event {
	return e.Field(field.ID{ID: id})
}

func (e *Event) Retry(after time.Duration) *Event {
	return e.Field(field.Retry{After: after})
}

func (e *Event) Comment(message string) *Event {
	return e.Field(field.Comment{Message: message})
}

func (e *Event) WriteTo(w io.Writer) (n int64, err error) {
	fw := &field.Writer{Writer: w}

	var m int64

	for _, f := range e.fields {
		m, err = fw.WriteField(f)
		n += m
		if err != nil {
			return
		}
	}

	m, err = fw.End()

	return n + m, err
}

func New() *Event {
	return &Event{}
}
