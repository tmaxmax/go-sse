package event

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

// A Marshaler is the interface types that can be represented as an Event implement.
type Marshaler interface {
	MarshalEvent() *Event
}

// Event is the representation of a single message. Use the New constructor to create one.
// This representation is used only for sending events - there is another event type for the client.
type Event struct {
	expiresAt time.Time
	fields    []Field
}

// reset sets all the event's internal fields to their default values.
func (e *Event) reset() {
	e.expiresAt = time.Time{}
	e.fields = nil
}

// WriteTo writes the standard textual representation of an event to an io.Writer.
// This operation is heavily optimized and does zero allocations, so it is strongly preferred
// over MarshalText or String.
func (e *Event) WriteTo(w io.Writer) (int64, error) {
	var m, n int64
	var o int
	var err error

	for i := range e.fields {
		m, err = e.fields[i].WriteTo(w)
		n += m
		if err != nil {
			return n, err
		}
	}

	o, err = w.Write(newline)
	n += int64(o)

	return n, err
}

// MarshalText writes the standard textual representation of the event. Marshalling and unmarshalling will not
// result in an event with the same fields: comment fields will not be unmarshalled, expiry time will be lost,
// and data fields won't be of the same type: a multiline Text field will be unmarshalled into multiple Line fields.
//
// Use the WriteTo method if you don't need the byte representation.
//
// The representation is written to a bytes.Buffer, which means the error is always nil.
// If the buffer grows to a size bigger than the maximum allowed, MarshalText will panic.
// See the bytes.Buffer documentation for more info.
func (e *Event) MarshalText() ([]byte, error) {
	b := &bytes.Buffer{}
	_, _ = e.WriteTo(b)

	return b.Bytes(), nil
}

// UnmarshalError is the error returned by the event's UnmarshalText method.
// If the error is related to a specific field, FieldName will be a non-empty string.
// If no fields were found in the target text or any other errors occurred, only
// a Reason will be provided. Reason is always present.
type UnmarshalError struct {
	Reason    error
	FieldName string
	// The value of the invalid field. This is a copy of the original value.
	FieldValue string
}

func (u *UnmarshalError) Error() string {
	if u.FieldName == "" {
		return fmt.Sprintf("unmarshal event error: %s", u.Reason.Error())
	}
	return fmt.Sprintf("unmarshal event error, %s field invalid: %s. contents: %s", u.FieldName, u.Reason.Error(), u.FieldValue)
}

func (u *UnmarshalError) Unwrap() error {
	return u.Reason
}

// UnmarshalText extracts the first event found in the given byte slice into the
// receiver. The receiver is always reset to the event's default value before unmarshaling,
// so always use a new Event instance if you don't want to overwrite data.
//
// Unmarshaling ignores comments and fields with invalid names. If no valid fields are found,
// an error is returned.
//
// All returned errors are of type UnmarshalError.
func (e *Event) UnmarshalText(b []byte) error {
	e.reset()

	s := parser.NewByteParser(b)

loop:
	for s.Scan() {
		f := s.Field()

		switch f.Name {
		case parser.FieldNameRetry:
			if i := bytes.IndexFunc(f.Value, func(r rune) bool {
				return r < '0' || r > '9'
			}); i != -1 {
				r, _ := utf8.DecodeRune(f.Value[i:])

				return &UnmarshalError{
					FieldName:  string(f.Name),
					FieldValue: string(f.Value),
					Reason:     fmt.Errorf("contains character %q, which is not an ASCII digit", r),
				}
			}

			fallthrough
		case parser.FieldNameData, parser.FieldNameEvent, parser.FieldNameID:
			f := Field{nameBytes: fieldBytes[f.Name], data: util.CloneBytes(f.Value), singleLine: true}
			e.fields = append(e.fields, f)
		default: // event end
			break loop
		}
	}

	if len(e.fields) == 0 {
		return &UnmarshalError{Reason: errors.New("unexpected end of input")}
	}

	return nil
}

// MarshalEvent is implemented on the Event type so concrete Event values can be used where a Marshaler is required.
// It returns the receiver itself.
func (e *Event) MarshalEvent() *Event {
	return e
}

// String writes the event's standard textual representation to a strings.Builder and returns the resulted string.
// It may panic if the representation is too long to be buffered.
//
// Use the WriteTo method if you don't actually need the string representation.
func (e *Event) String() string {
	s := &strings.Builder{}
	_, _ = e.WriteTo(s)

	return s.String()
}

// ID returns the event's ID. It returns an empty string if the event doesn't have an ID.
func (e *Event) ID() string {
	for i := len(e.fields) - 1; i >= 0; i-- {
		if f := &e.fields[i]; bytes.Equal(f.nameBytes, fieldBytes[parser.FieldNameID]) {
			return util.String(f.data)
		}
	}
	return ""
}

// ExpiresAt returns the timestamp when the event expires.
func (e *Event) ExpiresAt() time.Time {
	return e.expiresAt
}

func (e *Event) SetExpiry(time time.Time) {
	e.expiresAt = time
}

func (e *Event) SetTTL(duration time.Duration) {
	e.SetExpiry(time.Now().Add(duration))
}

// New creates a new event. It takes as parameters the event's desired fields and an expiry time configuration
// (TTL or ExpiresAt). If no expiry time is specified, the event expires immediately.
//
// If multiple Retry, ID, or Name fields are passed, the value of the last one is set. Multiple Data fields
// are all kept in the order they're passed.
func New(fields ...Field) *Event {
	return &Event{fields: append(make([]Field, 0, len(fields)), fields...)}
}

// From creates a new event using the provided one as a base. It does not modify the base event.
// It is mostly intended to be used by replay providers that set IDs for events, but can serve
// any other use case where adding or modifying event fields is necessary.
//
// Fields are added or set the same way New does.
func From(base *Event, fields ...Field) *Event {
	e := &Event{
		expiresAt: base.expiresAt,
		fields:    make([]Field, 0, len(base.fields)+len(fields)),
	}
	e.fields = append(e.fields, base.fields...)
	e.fields = append(e.fields, fields...)

	return e
}
