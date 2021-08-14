package event

import (
	"io"
)

// Event is the representation of a single message. Use the New constructor to create one.
type Event struct {
	fields []Field
}

func (e *Event) WriteTo(w io.Writer) (n int64, err error) {
	fw := &writer{Writer: w}
	defer fw.Close()

	for _, f := range e.fields {
		m, err := fw.WriteField(f)
		n += m
		if err != nil {
			return n, err
		}
	}

	return n, fw.Close()
}

func NewEvent(fields ...Field) *Event {
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
		case Name:
			fieldIndex = &nameIndex
		case ID:
			fieldIndex = &idIndex
		case Retry:
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
