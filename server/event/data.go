package event

import (
	"encoding/base64"
	"encoding/json"
	"io"

	. "github.com/tmaxmax/go-sse/server/event/internal"
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

// JSON is a single-line data payload consisting of a JSON encoded object.
type JSON struct {
	Value interface{}

	Data
}

func (j JSON) WriteTo(w io.Writer) (int64, error) {
	cw := &CountWriter{Writer: w}

	err := json.NewEncoder(cw).Encode(j.Value)

	return int64(cw.Count), err
}

// Base64 is a single-line data payload consisting of Base64 encoded bytes.
// You can send binary data using it.
type Base64 struct {
	Payload  []byte
	Encoding *base64.Encoding

	Data
}

func (b Base64) WriteTo(w io.Writer) (int64, error) {
	enc := base64.NewEncoder(b.Encoding, w)
	n, err := enc.Write(b.Payload)

	if err == nil {
		err = enc.Close()
	}

	return int64(n), err
}
