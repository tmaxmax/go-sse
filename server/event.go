package server

import (
	"io"

	"github.com/tmaxmax/go-sse/server/field"
)

// Event is the representation of a single message. Use the New constructor to create one.
type Event struct {
	fields []field.Field
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

func NewEvent(fields ...field.Field) *Event {
	var (
		nameIndex  = -1
		idIndex    = -1
		retryIndex = -1

		e = &Event{}
	)

	for _, f := range fields {
		var (
			insertIndex = len(e.fields)
			fieldIndex  *int
		)

		switch f.(type) {
		case field.Name:
			fieldIndex = &nameIndex
		case field.ID:
			fieldIndex = &idIndex
		case field.Retry:
			fieldIndex = &retryIndex
		}

		if fieldIndex != nil {
			if i := *fieldIndex; i != -1 {
				e.fields[i] = f

				continue
			} else {
				*fieldIndex = insertIndex
			}
		}

		e.fields = append(e.fields, f)
	}

	return e
}
