package field

import (
	"io"
	"strconv"
	"time"
)

// Retry is a field that tells the client to set the event stream reconnection time to
// the number of milliseconds it provides.
type Retry struct {
	SingleLine

	After time.Duration
}

func (r Retry) name() string {
	return "retry"
}

func (r Retry) WriteTo(w io.Writer) (int64, error) {
	after := strconv.FormatInt(r.After.Milliseconds(), 10)
	n, err := w.Write([]byte(after))

	return int64(n), err
}
