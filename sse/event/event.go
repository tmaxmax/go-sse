package event

import (
	"io"
	"time"

	"github.com/tmaxmax/go-sse/sse/event/field"
)

// Event is the representation of a single message. Use the New constructor to create one.
type Event struct {
	fields []field.Field

	nameIndex  int
	retryIndex int
	idIndex    int
}

// AddField appends a field to the event. It can be a predefined one or your custom event field implementations.
func (e *Event) AddField(f field.Field) *Event {
	var fieldIndex *int

	switch f.(type) {
	case field.Name:
		fieldIndex = &e.nameIndex
	case field.ID:
		fieldIndex = &e.idIndex
	case field.Retry:
		fieldIndex = &e.retryIndex
	}

	if fieldIndex != nil {
		if *fieldIndex == -1 {
			*fieldIndex = len(e.fields)
		} else {
			e.fields[*fieldIndex] = f

			return e
		}
	}

	e.fields = append(e.fields, f)

	return e
}

// SetName sets the event's name. Calling it multiple times overwrites the previously set name.
func (e *Event) SetName(name string) *Event {
	return e.AddField(field.Name{Name: name})
}

// AddText adds data fields that contain the given string.
func (e *Event) AddText(s string) *Event {
	return e.AddField(field.Text{Text: s})
}

// AddRaw adds data fields that contain the given bytes.
func (e *Event) AddRaw(s []byte) *Event {
	return e.AddField(field.Raw{Payload: s})
}

// AddJSON adds a data field that contains the given value's JSON representation.
func (e *Event) AddJSON(v interface{}) *Event {
	return e.AddField(field.JSON{Value: v})
}

// AddBase64 adds a data field that contains the given bytes' Base64 representation.
func (e *Event) AddBase64(s []byte) *Event {
	return e.AddField(field.Base64{Payload: s})
}

// SetID sets the event's SetID.
func (e *Event) SetID(id string) *Event {
	return e.AddField(field.ID{ID: id})
}

// SetRetry tells the client to reconnect after the given duration if the connection to the server is lost.
func (e *Event) SetRetry(after time.Duration) *Event {
	return e.AddField(field.Retry{After: after})
}

// AddComment adds comment lines to the event.
func (e *Event) AddComment(message string) *Event {
	return e.AddField(field.Comment{Message: message})
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

// New returns a new event with the required default internal values set.
func New() *Event {
	return &Event{
		nameIndex:  -1,
		retryIndex: -1,
		idIndex:    -1,
	}
}
