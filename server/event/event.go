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
)

type field interface {
	name() parser.FieldName
	repr() (p []byte, singleLine bool)

	Option
}

// A Marshaler is the interface types that can be represented as an Event implement.
type Marshaler interface {
	MarshalEvent() *Event
}

// Event is the representation of a single message. Use the New constructor to create one.
// This representation is used only for sending events - there is another event type for the client.
type Event struct {
	expiresAt  time.Time
	fields     []field
	nameIndex  int
	idIndex    int
	retryIndex int
}

// reset sets all the event's internal fields to their default values.
func (e *Event) reset() {
	e.expiresAt = time.Time{}
	e.fields = nil
	e.nameIndex = -1
	e.idIndex = -1
	e.retryIndex = -1
}

// WriteTo writes the standard textual representation of an event to an io.Writer.
// This operation is heavily optimized and does zero allocations, so it is strongly preferred
// over MarshalText or String.
func (e *Event) WriteTo(w io.Writer) (int64, error) {
	fw := &fieldWriter{
		w: w,
		s: parser.ChunkScanner{},
	}

	var err error
	n, m := 0, 0

	for _, f := range e.fields {
		m, err = fw.writeField(f)
		n += m
		if err != nil {
			return int64(n), err
		}
	}

	m, err = w.Write(newline)
	n += m

	return int64(n), err
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
	FieldName string
	// The value of the invalid field. This is a copy of the original value.
	FieldValue string
	Reason     error
}

func (u *UnmarshalError) Error() string {
	if u.FieldName == "" {
		return fmt.Sprintf("unmarshal event error: %s", u.Reason.Error())
	}
	return fmt.Sprintf("unmarshal event error, %s field invalid: %s. contents: %s", u.FieldName, u.Reason.Error(), u.FieldValue)
}

// UnmarshalText extracts the first event found in the given byte slice into the
// receiver. The receiver is always reset to the event's default value before unmarshaling,
// so always use a new Event instance if you don't want to overwrite data.
//
// Unmarshaling ignores comments and fields with invalid names. If no valid fields are found,
// an error is returned.
//
// All returned errors are of type UnmarshalError
func (e *Event) UnmarshalText(b []byte) error {
	e.reset()

	s := parser.NewByteParser(b)

loop:
	for s.Scan() {
		f := s.Field()
		var pf field

		switch f.Name {
		case parser.FieldNameData:
			pf = &LineField{s: string(f.Value)}
		case parser.FieldNameEvent:
			pf = Name(f.Value)
		case parser.FieldNameID:
			pf = ID(f.Value)
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

			pf = &RetryField{buf: append(make([]byte, 0, len(f.Value)), f.Value...)}
		default:
			// event end
			break loop
		}

		pf.apply(e)
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
func (e *Event) ID() ID {
	if e.idIndex == -1 {
		return ""
	}
	return e.fields[e.idIndex].(ID)
}

// ExpiresAt returns the timestamp when the event expires.
func (e *Event) ExpiresAt() time.Time {
	return e.expiresAt
}

// New creates a new event. It takes as parameters the event's desired fields and an expiry time configuration
// (TTL or ExpiresAt). If no expiry time is specified, the event expires immediately.
func New(options ...Option) *Event {
	e := &Event{}
	e.reset()

	for _, option := range options {
		option.apply(e)
	}

	return e
}

// From creates a new event using the provided one as a base. It does not modify the base event.
func From(base *Event, options ...Option) *Event {
	e := &Event{
		nameIndex:  base.nameIndex,
		idIndex:    base.idIndex,
		retryIndex: base.retryIndex,
		expiresAt:  base.expiresAt,
		fields:     make([]field, 0, len(base.fields)),
	}

	e.fields = append(e.fields, base.fields...)

	for _, option := range options {
		option.apply(e)
	}

	return e
}
