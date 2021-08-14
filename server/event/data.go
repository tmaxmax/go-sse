package event

import (
	"io"
)

// Data implements the Field.name method. Embed this struct in
// all your custom data field implementations.
type Data struct{}

func (d Data) name() string {
	return "data"
}

// Raw is a multiline data payload consisting of bytes that represent an UTF-8 encoded string.
// Do not use this for binary data!
type Raw []byte

func (r Raw) name() string {
	return Data{}.name()
}

func (r Raw) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(r)

	return int64(n), err
}

// Text is a data payload consisting of a UTF-8 encoded string.
type Text string

func (t Text) name() string {
	return Data{}.name()
}

func (t Text) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write([]byte(t))

	return int64(n), err
}
