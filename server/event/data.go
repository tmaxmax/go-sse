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

func (r Raw) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(r)

	return int64(n), err
}

// Text is a data payload consisting of a UTF-8 encoded string.
type Text string

func (t Text) name() string {
	return "data"
}

func (t Text) apply(e *Event) {
	e.fields = append(e.fields, t)
}

func (t Text) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, string(t))

	return int64(n), err
}
