package event

import (
	"strconv"
	"time"

	"github.com/tmaxmax/go-sse/internal/parser"
)

// Retry is a field that tells the client to set the event stream reconnection time to
// the number of milliseconds it provides.
func Retry(t time.Duration) *RetryField {
	return &RetryField{buf: strconv.AppendInt(nil, t.Milliseconds(), 10)}
}

// RetryField holds the representation in bytes of a retry field. This way when writing an Event
// the duration isn't converted to bytes on each write and no allocations are made.
type RetryField struct {
	buf []byte
}

func (r *RetryField) name() parser.FieldName {
	return parser.FieldNameRetry
}

func (r *RetryField) apply(e *Event) {
	if e.retryIndex == -1 {
		e.retryIndex = len(e.fields)
		e.fields = append(e.fields, r)
	} else {
		e.fields[e.retryIndex] = r
	}
}

func (r *RetryField) repr() ([]byte, bool) {
	return r.buf, true
}
