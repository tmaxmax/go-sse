package event

import (
	"io"
	"time"

	field2 "github.com/tmaxmax/go-sse/sse/server/event/field"
)

// Event is the representation of a single message. Use the New constructor to create one.
type Event struct {
	fields []field2.Field
}

// Field appends a field to the event. It can be a predefined one or your custom event field implementations.
func (e *Event) Field(f field2.Field) *Event {
	e.fields = append(e.fields, f)

	return e
}

// Name sets the event's name. Calling it multiple times overwrites the previously set name.
func (e *Event) Name(name string) *Event {
	return e.Field(field2.Name{Name: name})
}

// Text adds data fields that contain the given string.
func (e *Event) Text(s string) *Event {
	return e.Field(field2.Text{Text: s})
}

// Raw adds data fields that contain the given bytes.
func (e *Event) Raw(s []byte) *Event {
	return e.Field(field2.Raw{Payload: s})
}

// JSON adds a data field that contains the given value's JSON representation.
func (e *Event) JSON(v interface{}) *Event {
	return e.Field(field2.JSON{Value: v})
}

// Base64 adds a data field that contains the given bytes' Base64 representation.
func (e *Event) Base64(s []byte) *Event {
	return e.Field(field2.Base64{Payload: s})
}

// ID sets the event's ID.
func (e *Event) ID(id string) *Event {
	return e.Field(field2.ID{ID: id})
}

// Retry tells the client to reconnect after the given duration if the connection to the server is lost.
func (e *Event) Retry(after time.Duration) *Event {
	return e.Field(field2.Retry{After: after})
}

// Comment adds comment lines to the event.
func (e *Event) Comment(message string) *Event {
	return e.Field(field2.Comment{Message: message})
}

func (e *Event) WriteTo(w io.Writer) (n int64, err error) {
	fw := &field2.Writer{Writer: w}

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
