package field

import (
	"encoding/base64"
	"encoding/json"
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
type Raw struct {
	Data

	Payload []byte
}

func (r Raw) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(r.Payload)

	return int64(n), err
}

// Text is a data payload consisting of a UTF-8 encoded string.
type Text struct {
	Data

	Text string
}

func (t Text) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write([]byte(t.Text))

	return int64(n), err
}

// JSON is a single-line data payload consisting of a JSON encoded object.
type JSON struct {
	Data
	SingleLine

	Value interface{}
}

func (j JSON) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}

	err := json.NewEncoder(cw).Encode(j.Value)

	return int64(cw.count), err
}

// Base64 is a single-line data payload consisting of Base64 encoded bytes.
// You can send binary data using it.
type Base64 struct {
	Data
	SingleLine

	Payload  []byte
	Encoding *base64.Encoding
}

func (b Base64) WriteTo(w io.Writer) (int64, error) {
	enc := base64.NewEncoder(b.Encoding, w)
	n, err := enc.Write(b.Payload)

	if err == nil {
		err = enc.Close()
	}

	return int64(n), err
}
