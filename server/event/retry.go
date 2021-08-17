package event

import (
	"io"
	"strconv"
	"time"
)

// Retry is a field that tells the client to set the event stream reconnection time to
// the number of milliseconds it provides.
type Retry time.Duration

func (r Retry) name() string {
	return "retry"
}

func (r Retry) apply(e *Event) {
	if e.retryIndex == -1 {
		e.retryIndex = len(e.fields)
		e.fields = append(e.fields, r)
	} else {
		e.fields[e.retryIndex] = r
	}
}

func (r Retry) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write([]byte(strconv.FormatInt(time.Duration(r).Milliseconds(), 10)))

	return int64(n), err
}
