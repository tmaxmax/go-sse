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

func isSingleLine(p string) bool {
	_, newlineLen := parser.NewlineIndex(p)
	return newlineLen == 0
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
	content   parser.Chunk
	isComment bool
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
	m, err := writeString(w, c.content.Data)
	n += m
	if err != nil || c.content.HasNewline {
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
//
// The Topic and ExpiresAt fields are used only on the server and are not sent to the client.
// They are not part of the protocol.
type Message struct {
	Topic     string
	ExpiresAt time.Time

	chunks     []chunk
	retryValue string
	name       string
	id         EventID
}

func (e *Message) appendText(isComment bool, chunks ...string) {
	for _, c := range chunks {
		var content parser.Chunk

		for c != "" {
			content, c = parser.NextChunk(c)
			e.chunks = append(e.chunks, chunk{content: content, isComment: isComment})
		}
	}
}

// AppendData creates multiple data fields on the message's event from the given strings.
//
// Server-sent events are not suited for binary data: the event fields are delimited by newlines,
// where a newline can be a LF, CR or CRLF sequence. When the client interprets the fields,
// it joins multiple data fields using LF, so information is altered. Here's an example:
//
//	initial payload: This is a\r\nmultiline\rtext.\nIt has multiple\nnewline\r\nvariations.
//	data sent over the wire:
//		data: This is a
//		data: multiline
//		data: text.
//		data: It has multiple
//		data: newline
//		data: variations
//	data received by client: This is a\nmultiline\ntext.\nIt has multiple\nnewline\nvariations.
//
// Each line prepended with "data:" is a field; multiple data fields are joined together using LF as the delimiter.
// If you attempted to send the same payload without prepending the "data:" prefix, like so:
//
//	data: This is a
//	multiline
//	text.
//	It has multiple
//	newline
//	variations
//
// there would be only one data field (the first one). The rest would be different fields, named "multiline", "text.",
// "It has multiple" etc., which are invalid fields according to the protocol.
//
// Besides, the protocol explicitly states that event streams must always be UTF-8 encoded:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream.
//
// If you need to send binary data, you can use a Base64 encoder or any other encoder that does not output
// any newline characters (\r or \n) and then append the resulted data.
func (e *Message) AppendData(chunks ...string) {
	e.appendText(false, chunks...)
}

// Comment creates a comment field on the message's event. If it spans multiple lines,
// new comment lines are created.
func (e *Message) Comment(comments ...string) {
	e.appendText(true, comments...)
}

// SetRetry creates a field on the message's event that tells the client to set the event stream reconnection time to
// the number of milliseconds it provides.
func (e *Message) SetRetry(duration time.Duration) {
	e.retryValue = strconv.FormatInt(duration.Milliseconds(), 10)
}

// ID returns the message's event's ID.
func (e *Message) ID() EventID {
	return e.id
}

func (e *Message) Name() string {
	return e.name
}

// SetID sets the message's event's ID.
// To remove the ID, pass an unset ID to this function.
func (e *Message) SetID(id EventID) {
	e.id = id
}

// SetName sets the message's event's name.
//
// A Name cannot have multiple lines. If it has, the function will return false.
func (e *Message) SetName(name string) bool {
	if !isSingleLine(name) {
		return false
	}
	e.name = name
	return true
}

func (e *Message) writeID(w io.Writer) (int64, error) {
	if !e.id.IsSet() {
		return 0, nil
	}

	n, err := w.Write(fieldBytesID)
	if err != nil {
		return int64(n), err
	}
	m, err := writeString(w, e.id.value)
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
	m, err := writeString(w, e.name)
	n += m
	if err != nil {
		return int64(n), err
	}
	m, err = w.Write(newline)
	return int64(n + m), err
}

func (e *Message) writeRetry(w io.Writer) (int64, error) {
	if e.retryValue == "" {
		return 0, nil
	}

	n, err := w.Write(fieldBytesRetry)
	if err != nil {
		return int64(n), err
	}
	m, err := writeString(w, e.retryValue)
	n += m
	if err != nil {
		return int64(n), err
	}
	m, err = w.Write(newline)
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
	// The value of the invalid field.
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
	e.name = ""
	e.id = EventID{}
	e.retryValue = ""
	e.ExpiresAt = time.Time{}
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

	s := parser.NewFieldParser(string(p))

loop:
	for f := (parser.Field{}); s.Next(&f); {
		switch f.Name {
		case parser.FieldNameRetry:
			if i := strings.IndexFunc(f.Value, func(r rune) bool {
				return r < '0' || r > '9'
			}); i != -1 {
				r, _ := utf8.DecodeRuneInString(f.Value[i:])

				return &UnmarshalError{
					FieldName:  string(f.Name),
					FieldValue: string(f.Value),
					Reason:     fmt.Errorf("contains character %q, which is not an ASCII digit", r),
				}
			}

			e.retryValue = f.Value
		case parser.FieldNameData:
			e.chunks = append(e.chunks, chunk{content: parser.Chunk{Data: f.Value + "\n", HasNewline: true}})
		case parser.FieldNameEvent:
			e.name = f.Value
		case parser.FieldNameID:
			e.id = EventID{value: f.Value, set: true}
		default: // event end
			break loop
		}
	}

	if len(e.chunks) == 0 && e.name == "" && e.retryValue == "" && !e.id.IsSet() || s.Err() != nil {
		e.reset()
		return &UnmarshalError{Reason: ErrUnexpectedEOF}
	}
	return nil
}

// Clone returns a copy of the message.
func (e *Message) Clone() *Message {
	return &Message{
		Topic:     e.Topic,
		ExpiresAt: e.ExpiresAt,
		// The first AppendData will trigger a reallocation.
		// Already appended chunks cannot be modified/removed, so this is safe.
		chunks:     e.chunks[:len(e.chunks):len(e.chunks)],
		retryValue: e.retryValue,
		name:       e.name,
		id:         e.id,
	}
}

func writeString(w io.Writer, s string) (int, error) {
	return w.Write(unsafe.Slice((*byte)(unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data)), len(s)))
}
