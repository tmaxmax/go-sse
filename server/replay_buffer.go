package server

import (
	"strconv"

	"github.com/tmaxmax/go-sse/server/event"
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
	bufferBase
	lastRemoved event.ID
}

func (b *bufferNoID) queue(message *Message) {
	if message.Event.ID() != "" {
		b.buf = append(b.buf, *message)
	}
}

func (b *bufferNoID) dequeue() {
	b.lastRemoved = b.buf[0].Event.ID()
	b.buf = b.buf[1:]
}

func (b *bufferNoID) slice(at event.ID) ([]Message, error) {
	if at == b.lastRemoved && len(b.buf) != 0 {
		return b.buf, nil
	}
	index := -1
	for i := range b.buf {
		if at == b.buf[i].Event.ID() {
			index = i
			break
		}
	}
	if index == -1 {
		return nil, &ReplayError{id: at}
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
	message.Event = event.From(message.Event, event.ID(strconv.FormatInt(b.lastID, autoIDBase)))
	b.lastID++
	b.buf = append(b.buf, *message)
}

func (b *bufferAutoID) dequeue() {
	b.firstID++
	b.buf = b.buf[1:]
}

func (b *bufferAutoID) slice(at event.ID) ([]Message, error) {
	id, err := strconv.ParseInt(string(at), autoIDBase, 64)
	if err != nil {
		return nil, &ReplayError{id: at, err: err}
	}
	index := id - b.firstID
	if index < 0 || index >= int64(len(b.buf)) {
		return nil, &ReplayError{id: at}
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
