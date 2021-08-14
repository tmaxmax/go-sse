package event

import (
	"io"
)

// Raw is a multiline data payload consisting of bytes that represent an UTF-8 encoded string.
// Do not use this for binary data!
type Raw []byte

func (r Raw) name() string {
	return "data"
}

func (r Raw) apply(e *Event) {
	e.fields = append(e.fields, r)
}

func (r Raw) Message(w io.Writer) error {
	_, err := w.Write(r)

	return err
}

// Text is a data payload consisting of a UTF-8 encoded string.
type Text string

func (t Text) name() string {
	return "data"
}

func (t Text) apply(e *Event) {
	e.fields = append(e.fields, t)
}

func (t Text) Message(w io.Writer) error {
	_, err := w.Write([]byte(t))

	return err
}
