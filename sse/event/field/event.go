package field

import "io"

// Event is the event field that sets the event's type.
type Event struct {
	SingleLine

	Name string
}

func (e Event) name() string {
	return "event"
}

func (e Event) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write([]byte(e.Name))

	return int64(n), err
}
