package sse

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/tmaxmax/go-sse/internal/parser"
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

// Message is the representation of a single message sent from the server to its clients.
//
// A Message is made of a single event, which is sent to each client, and other metadata
// about the message itself: its expiry time and topic.
//
// Topics are used to filter which events reach which clients. If a client subscribes to
// a certain set of topics, but a message's topic is not part of that set, then the underlying
// event of the message does not reach the client.
//
// The message's expiry time can be used when replaying messages to new clients. If a message
// is expired, then it is not sent. Replay providers will usually make use of this.
type Message struct {
	Topic string

	expiresAt time.Time
	// DO NOT MUTATE, either original byte slices or unsafely converted from strings
	chunks     []chunk
	retryValue []byte
	// DO NOT MUTATE, unsafely converted from string
	name []byte
	// nil - not set; non-nil, any length: set
	// DO NOT MUTATE, unsafely converted from string
	id []byte
}

func (e *Message) appendText(isComment bool, chunks ...string) {
	s := parser.ChunkScanner{}

	for _, c := range chunks {
		s.Reset(unsafeBytes(c)) // SAFETY: strings are immutable

		for s.Scan() {
			data, endsInNewline := s.Chunk()
			e.chunks = append(e.chunks, chunk{data: data, endsInNewline: endsInNewline, isComment: isComment})
		}
	}
}

// AppendText creates multiple data fields on the message's event from the given strings. It uses the unsafe package
// to convert the string to a byte slice, so no allocations are made. See the AppendData method's
// documentation for details about the data field type.
func (e *Message) AppendText(chunks ...string) {
	e.appendText(false, chunks...)
}

// AppendData creates multiple data fields on the message's event from the given byte slices.
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
// any newline characters (\r or \n) and then append the resulted byte slices.
func (e *Message) AppendData(chunks ...[]byte) {
	s := parser.ChunkScanner{}

	for _, c := range chunks {
		s.Reset(c)

		for s.Scan() {
			data, endsInNewline := s.Chunk()
			e.chunks = append(e.chunks, chunk{data: data, endsInNewline: endsInNewline})
		}
	}
}

// Comment creates a comment field on the message's event. If it spans multiple lines,
// new comment lines are created.
func (e *Message) Comment(comments ...string) {
	e.appendText(true, comments...)
}

// SetRetry creates a field on the message's event that tells the client to set the event stream reconnection time to
// the number of milliseconds it provides.
func (e *Message) SetRetry(duration time.Duration) {
	var buf []byte
	if e.retryValue == nil {
		buf = e.retryValue[:0]
	}
	e.retryValue = strconv.AppendInt(buf, duration.Milliseconds(), 10)
	e.retryValue = append(e.retryValue, '\n')
}

// ID returns the message's event's ID.
func (e *Message) ID() EventID {
	if e.id == nil {
		return EventID{}
	}
	return EventID{value: unsafeString(e.id), set: true} // SAFETY: ids are immutable
}

// SetID sets the message's event's ID.
func (e *Message) SetID(id EventID) {
	if !id.IsSet() {
		e.id = nil
		return
	}
	e.id = unsafeBytes(id.String()) // SAFETY: ids are immutable
}

// SetName sets the message's event's name.
//
// A Name cannot have multiple lines. If it has, the function will return false.
func (e *Message) SetName(name string) bool {
	if !isSingleLine([]byte(name)) {
		return false
	}
	e.name = unsafeBytes(name) // SAFETY: strings are immutable
	return true
}

// SetExpiry sets the message's expiry time to the given timestamp.
//
// This is not sent to the clients. The expiry time can be used when implementing
// event replay providers, to see if an event is still valid for replay.
func (e *Message) SetExpiry(t time.Time) {
	e.expiresAt = t
}

// SetTTL sets the message's expiry time to a timestamp after the given duration from the current time.
//
// This is not sent to the clients. The expiry time can be used when implementing
// event replay providers, to see if an event is still valid for replay.
func (e *Message) SetTTL(d time.Duration) {
	e.SetExpiry(time.Now().Add(d))
}

// ExpiresAt returns the timestamp when the message expires.
func (e *Message) ExpiresAt() time.Time {
	return e.expiresAt
}

func (e *Message) writeID(w io.Writer) (int64, error) {
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

func (e *Message) writeName(w io.Writer) (int64, error) {
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

func (e *Message) writeRetry(w io.Writer) (int64, error) {
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

// WriteTo writes the standard textual representation of the message's event to an io.Writer.
// This operation is heavily optimized and does zero allocations, so it is strongly preferred
// over MarshalText or String.
func (e *Message) WriteTo(w io.Writer) (int64, error) {
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

// MarshalText writes the standard textual representation of the message's event. Marshalling and unmarshalling will not
// result in a message with an event that has the same fields: comment fields will not be unmarshalled, and expiry time and topic will be lost.
//
// Use the WriteTo method if you don't need the byte representation.
//
// The representation is written to a bytes.Buffer, which means the error is always nil.
// If the buffer grows to a size bigger than the maximum allowed, MarshalText will panic.
// See the bytes.Buffer documentation for more info.
func (e *Message) MarshalText() ([]byte, error) {
	b := bytes.Buffer{}
	_, err := e.WriteTo(&b)
	return b.Bytes(), err
}

// String writes the message's event standard textual representation to a strings.Builder and returns the resulted string.
// It may panic if the representation is too long to be buffered.
//
// Use the WriteTo method if you don't actually need the string representation.
func (e *Message) String() string {
	s := strings.Builder{}
	_, _ = e.WriteTo(&s)
	return s.String()
}

// UnmarshalError is the error returned by the Message's UnmarshalText method.
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

// ErrUnexpectedEOF is returned when unmarshaling a Message from an input that doesn't end in a newline.
var ErrUnexpectedEOF = parser.ErrUnexpectedEOF

func (e *Message) reset() {
	e.chunks = nil
	e.name = []byte{}
	e.id = nil
	e.retryValue = []byte{}
	e.expiresAt = time.Time{}
}

// UnmarshalText extracts the first event found in the given byte slice into the
// receiver. The receiver is always reset to the message's default value before unmarshaling,
// so always use a new Message instance if you don't want to overwrite data.
//
// Unmarshaling ignores comments and fields with invalid names. If no valid fields are found,
// an error is returned. For a field to be valid it must end in a newline - if the last
// field of the event doesn't end in one, an error is returned.
//
// All returned errors are of type UnmarshalError.
func (e *Message) UnmarshalText(p []byte) error {
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
			e.id = append(buf, f.Value...) //nolint
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

// Clone returns a deep copy of the message.
func (e *Message) Clone() *Message {
	return &Message{
		expiresAt:  e.expiresAt,
		chunks:     append([]chunk(nil), e.chunks...),
		retryValue: append([]byte(nil), e.retryValue...),
		name:       e.name,
		id:         e.id,
	}
}

func unsafeBytes(s string) []byte {
	b := *(*[]byte)(unsafe.Pointer(&s))
	(*reflect.SliceHeader)(unsafe.Pointer(&b)).Cap = len(s)
	return b
}

func unsafeString(p []byte) string {
	return *(*string)(unsafe.Pointer(&p))
}
