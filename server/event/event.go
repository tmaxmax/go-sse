package event

import (
	"io"
	"time"
)

type field interface {
	name() string
	Message(io.Writer) error

	Option
}

// Event is the representation of a single message. Use the New constructor to create one.
type Event struct {
	fields []field

	nameIndex  int
	idIndex    int
	retryIndex int

	expiresAt time.Time
}

func (e *Event) Message(w io.Writer) error {
	fw := &writer{Writer: w}
	defer fw.Close()

	for _, f := range e.fields {
		if err := fw.WriteField(f); err != nil {
			return err
		}
	}

	return fw.Close()
}

func (e *Event) Expired() bool {
	return !e.expiresAt.IsZero() && e.expiresAt.Before(time.Now())
}

func New(options ...Option) *Event {
	e := &Event{
		nameIndex:  -1,
		idIndex:    -1,
		retryIndex: -1,
	}

	for _, option := range options {
		option.apply(e)
	}

	return e
}
