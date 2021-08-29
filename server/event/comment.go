package event

import "io"

// Comment is an event comment field. If it spans on multiple lines,
// new comment lines are created.
type Comment string

func (c Comment) name() string {
	return ""
}

func (c Comment) apply(e *Event) {
	e.fields = append(e.fields, c)
}

func (c Comment) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, string(c))

	return int64(n), err
}
