package event

import (
	"bytes"
	"io"
	"strings"
	"time"
)

type field interface {
	name() string

	io.WriterTo
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

func (e *Event) WriteTo(w io.Writer) (n int64, err error) {
	fw := &writer{Writer: w}
	var m int64
	defer fw.Close()

	for _, f := range e.fields {
		m, err = fw.WriteField(f)
		n += m
		if err != nil {
			return n, err
		}
	}

	err = fw.Close()

	return
}

func (e *Event) MarshalText() ([]byte, error) {
	b := &bytes.Buffer{}
	_, _ = e.WriteTo(b)

	return b.Bytes(), nil
}

func (e *Event) String() string {
	s := &strings.Builder{}
	_, _ = e.WriteTo(s)

	return s.String()
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
