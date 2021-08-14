package field

import "io"

// Name is the event field that sets the event's type.
type Name struct {
	SingleLine

	Name string
}

func (e Name) name() string {
	return "event"
}

func (e Name) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write([]byte(e.Name))

	return int64(n), err
}
