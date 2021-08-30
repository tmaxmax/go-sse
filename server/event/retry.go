package event

import (
	"strconv"
	"time"

	"github.com/tmaxmax/go-sse/internal/parser"
)

// Retry is a field that tells the client to set the event stream reconnection time to
// the number of milliseconds it provides.
type Retry time.Duration

func (r Retry) name() parser.FieldName {
	return parser.FieldNameRetry
}

func (r Retry) apply(e *Event) {
	if e.retryIndex == -1 {
		e.retryIndex = len(e.fields)
		e.fields = append(e.fields, r)
	} else {
		e.fields[e.retryIndex] = r
	}
}

func (r Retry) repr() []byte {
	return strconv.AppendInt(nil, time.Duration(r).Milliseconds(), 10)
}
