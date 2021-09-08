package server

import (
	"errors"
	"strconv"

	event "github.com/tmaxmax/go-sse/server/event/v2"
)

// A buffer is the underlying storage for a provider. Its methods are used by the provider to implement
// the Provider interface.
type buffer interface {
	queue(message *Message)
	dequeue()
	front() *event.Event
	len() int
	slice(event.ID) ([]Message, error)
}

type bufferBase struct {
	buf []Message
}

func (b *bufferBase) len() int {
	return len(b.buf)
}

func (b *bufferBase) front() *event.Event {
	if b.len() == 0 {
		return nil
	}
	return b.buf[0].Event
}

type bufferNoID struct {
	lastRemovedID event.ID
	bufferBase
}

func (b *bufferNoID) queue(message *Message) {
	if message.Event.ID().IsSet() {
		b.buf = append(b.buf, *message)
	}
}

func (b *bufferNoID) dequeue() {
	b.lastRemovedID = b.buf[0].Event.ID()
	b.buf = b.buf[1:]
}

func (b *bufferNoID) slice(atID event.ID) ([]Message, error) {
	if atID == b.lastRemovedID && len(b.buf) != 0 {
		return b.buf, nil
	}
	index := -1
	for i := range b.buf {
		if atID == b.buf[i].Event.ID() {
			index = i
			break
		}
	}
	if index == -1 {
		return nil, &ReplayError{ID: atID}
	}
	return b.buf[index:], nil
}

type bufferAutoID struct {
	bufferBase
	firstID int64
	lastID  int64
}

const autoIDBase = 10

func (b *bufferAutoID) queue(message *Message) {
	message.Event = message.Event.Clone()
	message.Event.SetID(event.MustID(strconv.FormatInt(b.lastID, autoIDBase)))
	b.lastID++
	b.buf = append(b.buf, *message)
}

func (b *bufferAutoID) dequeue() {
	b.firstID++
	b.buf = b.buf[1:]
}

func (b *bufferAutoID) slice(atID event.ID) ([]Message, error) {
	if !atID.IsSet() {
		return nil, &ReplayError{ID: atID, Reason: errors.New("ID not set")}
	}
	id, err := strconv.ParseInt(atID.String(), autoIDBase, 64)
	if err != nil {
		return nil, &ReplayError{ID: atID, Reason: err}
	}
	index := id - b.firstID
	if index < 0 || index >= int64(len(b.buf)) {
		return nil, &ReplayError{ID: atID, Reason: errors.New("ID not found")}
	}
	return b.buf[index:], nil
}

func getBuffer(autoIDs bool, capacity int) buffer {
	base := bufferBase{buf: make([]Message, 0, capacity)}
	if autoIDs {
		return &bufferAutoID{bufferBase: base}
	}
	return &bufferNoID{bufferBase: base}
}
