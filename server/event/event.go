/*
Package events is used for creating events that are sent from the server side.
*/
package event

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

func isSingleLine(p []byte) bool {
	_, remaining := parser.NewlineIndex(p)
	return remaining == 0
}

// fieldBytes holds the byte representation of each field type along with a colon at the end.
var (
	fieldBytesData    = []byte(parser.FieldNameData + ": ")
	fieldBytesEvent   = []byte(parser.FieldNameEvent + ": ")
	fieldBytesRetry   = []byte(parser.FieldNameRetry + ": ")
	fieldBytesID      = []byte(parser.FieldNameID + ": ")
	fieldBytesComment = []byte{':', ' '}
)

// The ID struct represents any valid event ID value.
type ID struct {
	value string
	set   bool
}

// NewID creates an ID value. It also returns a flag that indicates whether the input
// is a valid ID. A valid ID must not have any newlines. If the input is not valid,
// an unset (invalid) ID is returned.
func NewID(value string) (ID, bool) {
	if !isSingleLine(util.Bytes(value)) {
		return ID{}, false
	}
	return ID{value: value, set: true}, true
}

// MustID is the same as NewID, but it panics if the input isn't a valid ID.
func MustID(value string) ID {
	id, ok := NewID(value)
	if !ok {
		panic(fmt.Sprintf("go-sse.server.event: invalid id value %q, has newlines", value))
	}
	return id
}

// IsSet returns true if the receiver is a valid (set) ID value.
func (i ID) IsSet() bool {
	return i.set
}

// String returns the ID's value. The value may be an empty string,
// make sure to check if the ID is set before using the value.
func (i ID) String() string {
	return i.value
}

type chunk struct {
	data          []byte
	endsInNewline bool
	isComment     bool
}

var newline = []byte{'\n'}

func (c *chunk) WriteTo(w io.Writer) (int64, error) {
	name := fieldBytesData
	if c.isComment {
		name = fieldBytesComment
	}
	n, err := w.Write(name)
	if err != nil {
		return int64(n), err
	}
	m, err := w.Write(c.data)
	n += m
	if err != nil || c.endsInNewline {
		return int64(n), err
	}
	m, err = w.Write(newline)
	return int64(n + m), err
}

// Event is the representation of a single message.
// This representation is used only for sending events - there is another event type for the client.
type Event struct {
	expiresAt  time.Time
	chunks     []chunk
	retryValue []byte
	name       []byte
	id         []byte // nil - not set; non-nil, any length: set
}

func (e *Event) appendText(isComment bool, chunks ...string) {
	s := parser.ChunkScanner{}

	for _, c := range chunks {
		s.Reset(util.Bytes(c))

		for s.Scan() {
			data, endsInNewline := s.Chunk()
			e.chunks = append(e.chunks, chunk{data: data, endsInNewline: endsInNewline, isComment: isComment})
		}
	}
}

// AppendText creates multiple data fields from the given strings. It uses the unsafe package
// to convert the string to a byte slice, so no allocations are made. See the AppendData method's
// documentation for details about the data field type.
func (e *Event) AppendText(chunks ...string) {
	e.appendText(false, chunks...)
}

// AppendData creates multiple data fields from the given byte slices.
//
// Server-sent events are not suited for binary data: the event fields are delimited by newlines,
// where a newline can be a LF, CR or CRLF sequence. When the client interprets the fields,
// it joins multiple data fields using LF, so information is altered. Here's an example:
//
//   initial payload: This is a\r\nmultiline\rtext.\nIt has multiple\nnewline\r\nvariations.
//   data sent over the wire:
//     data: This is a
//     data: multiline
//     data: text.
//     data: It has multiple
//     data: newline
//     data: variations
//   data received by client: This is a\nmultiline\ntext.\nIt has multiple\nnewline\nvariations.
//
// Each line prepended with "data:" is a field; multiple data fields are joined together using LF as the delimiter.
// If you attempted to send the same payload without prepending the "data:" prefix, like so:
//
//   data: This is a
//   multiline
//   text.
//   It has multiple
//   newline
//   variations
//
// there would be only one data field (the first one). The rest would be different fields, named "multiline", "text.",
// "It has multiple" etc., which are invalid fields according to the protocol.
//
// Besides, the protocol explicitly states that event streams must always be UTF-8 encoded:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream.
//
// Still, if you need to send binary data, you can use a Base64 encoder or any other encoder that does not output
// any newline characters (\n or \n) and then append the resulted byte slices.
func (e *Event) AppendData(chunks ...[]byte) {
	s := parser.ChunkScanner{}

	for _, c := range chunks {
		s.Reset(c)

		for s.Scan() {
			data, endsInNewline := s.Chunk()
			e.chunks = append(e.chunks, chunk{data: data, endsInNewline: endsInNewline})
		}
	}
}

// Comment creates an event comment field. If it spans on multiple lines,
// new comment lines are created.
func (e *Event) Comment(comments ...string) {
	e.appendText(true, comments...)
}

// SetRetry creates an event field that tells the client to set the event stream reconnection time to
// the number of milliseconds it provides.
func (e *Event) SetRetry(duration time.Duration) {
	var buf []byte
	if e.retryValue == nil {
		buf = e.retryValue[:0]
	}
	e.retryValue = strconv.AppendInt(buf, duration.Milliseconds(), 10)
	e.retryValue = append(e.retryValue, '\n')
}

// ID returns the event's ID.
func (e *Event) ID() ID {
	if e.id == nil {
		return ID{}
	}
	return ID{value: util.String(e.id), set: true}
}

// SetID sets the event's ID.
func (e *Event) SetID(id ID) {
	if !id.IsSet() {
		e.id = nil
		return
	}
	e.id = util.Bytes(id.String())
}

// SetName sets the event's name.
//
// A Name cannot have multiple lines. If it has, the function will return false
func (e *Event) SetName(name string) bool {
	b := util.Bytes(name)
	if !isSingleLine(b) {
		return false
	}
	e.name = b
	return true
}

// SetExpiry sets the event's expiry time to the given timestamp.
//
// This is not sent to the clients. The expiry time can be used when implementing
// event replay providers, to see if an event is still valid for replay.
func (e *Event) SetExpiry(t time.Time) {
	e.expiresAt = t
}

// SetTTL sets the event's expiry time to a timestamp after the given duration from the current time.
//
// This is not sent to the clients. The expiry time can be used when implementing
// event replay providers, to see if an event is still valid for replay.
func (e *Event) SetTTL(d time.Duration) {
	e.SetExpiry(time.Now().Add(d))
}

// ExpiresAt returns the timestamp when the event expires.
func (e *Event) ExpiresAt() time.Time {
	return e.expiresAt
}

func (e *Event) writeID(w io.Writer) (int64, error) {
	if e.id == nil {
		return 0, nil
	}

	n, err := w.Write(fieldBytesID)
	if err != nil {
		return int64(n), err
	}
	m, err := w.Write(e.id)
	n += m
	if err != nil {
		return int64(n), err
	}
	m, err = w.Write(newline)
	return int64(n + m), err
}

func (e *Event) writeName(w io.Writer) (int64, error) {
	if len(e.name) == 0 {
		return 0, nil
	}

	n, err := w.Write(fieldBytesEvent)
	if err != nil {
		return int64(n), err
	}
	m, err := w.Write(e.name)
	n += m
	if err != nil {
		return int64(n), err
	}
	m, err = w.Write(newline)
	return int64(n + m), err
}

func (e *Event) writeRetry(w io.Writer) (int64, error) {
	if len(e.retryValue) == 0 {
		return 0, nil
	}

	n, err := w.Write(fieldBytesRetry)
	if err != nil {
		return int64(n), err
	}
	m, err := w.Write(e.retryValue)
	return int64(n + m), err
}

// WriteTo writes the standard textual representation of an event to an io.Writer.
// This operation is heavily optimized and does zero allocations, so it is strongly preferred
// over MarshalText or String.
func (e *Event) WriteTo(w io.Writer) (int64, error) {
	n, err := e.writeID(w)
	if err != nil {
		return n, err
	}
	m, err := e.writeName(w)
	n += m
	if err != nil {
		return n, err
	}
	m, err = e.writeRetry(w)
	n += m
	if err != nil {
		return n, err
	}
	for i := range e.chunks {
		m, err = e.chunks[i].WriteTo(w)
		n += m
		if err != nil {
			return n, err
		}
	}
	o, err := w.Write(newline)
	return int64(o) + n, err
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
	b := bytes.Buffer{}
	_, err := e.WriteTo(&b)
	return b.Bytes(), err
}

// String writes the event's standard textual representation to a strings.Builder and returns the resulted string.
// It may panic if the representation is too long to be buffered.
//
// Use the WriteTo method if you don't actually need the string representation.
func (e *Event) String() string {
	s := strings.Builder{}
	_, _ = e.WriteTo(&s)
	return s.String()
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

// ErrUnexpectedEOF is returned when UnmarshalText isn't provided a byte slice that ends in a newline.
var ErrUnexpectedEOF = parser.ErrUnexpectedEOF

func (e *Event) reset() {
	e.chunks = nil
	e.name = []byte{}
	e.id = nil
	e.retryValue = []byte{}
	e.expiresAt = time.Time{}
}

// UnmarshalText extracts the first event found in the given byte slice into the
// receiver. The receiver is always reset to the event's default value before unmarshaling,
// so always use a new Event instance if you don't want to overwrite data.
//
// Unmarshaling ignores comments and fields with invalid names. If no valid fields are found,
// an error is returned. For a field to be valid it must end in a newline - if the last
// field of the event doesn't end in one, an error is returned.
//
// All returned errors are of type UnmarshalError.
func (e *Event) UnmarshalText(p []byte) error {
	e.reset()

	s := parser.NewByteParser(p)

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

			e.retryValue = append(e.retryValue[:0], f.Value...)
			e.retryValue = append(e.retryValue, newline...)
		case parser.FieldNameData:
			var data []byte
			data = append(data, f.Value...)
			data = append(data, newline...)
			e.chunks = append(e.chunks, chunk{data: data, endsInNewline: true})
		case parser.FieldNameEvent:
			e.name = append(e.name[:0], f.Value...)
		case parser.FieldNameID:
			var buf []byte
			if e.id != nil {
				buf = e.id[:0]
			}
			e.id = append(buf, f.Value...)
		default: // event end
			break loop
		}
	}

	if len(e.chunks) == 0 && len(e.name) == 0 && len(e.retryValue) == 0 && e.id == nil || s.Err() != nil {
		e.reset()
		return &UnmarshalError{Reason: ErrUnexpectedEOF}
	}
	return nil
}

// Clone returns a copy of the event.
func (e *Event) Clone() *Event {
	return &Event{
		expiresAt:  e.expiresAt,
		chunks:     append([]chunk(nil), e.chunks...),
		retryValue: util.CloneBytes(e.retryValue),
		name:       e.name,
		id:         e.id,
	}
}
