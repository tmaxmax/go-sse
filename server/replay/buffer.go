package replay

import (
	"strconv"

	"github.com/tmaxmax/go-sse/server/event"
)

// A buffer is the underlying storage for a provider. Its methods are used by the provider to implement
// the Provider interface.
type buffer interface {
	queue(**event.Event)
	dequeue()
	front() *event.Event
	len() int
	slice(event.ID) ([]*event.Event, error)
}

type bufferBase struct {
	buf []*event.Event
}

func (b *bufferBase) len() int {
	return len(b.buf)
}

func (b *bufferBase) front() *event.Event {
	if b.len() == 0 {
		return nil
	}
	return b.buf[0]
}

type bufferNoID struct {
	bufferBase
	lastRemoved event.ID
}

func (b *bufferNoID) queue(ep **event.Event) {
	e := *ep
	if e.ID() != "" {
		b.buf = append(b.buf, e)
	}
}

func (b *bufferNoID) dequeue() {
	b.lastRemoved = b.buf[0].ID()
	b.buf[0] = nil
	b.buf = b.buf[1:]
}

func (b *bufferNoID) slice(at event.ID) ([]*event.Event, error) {
	if at == b.lastRemoved && len(b.buf) != 0 {
		return b.buf, nil
	}
	index := -1
	for i := range b.buf {
		if at == b.buf[i].ID() {
			index = i
			break
		}
	}
	if index == -1 {
		return nil, &Error{id: at}
	}
	return b.buf[index:], nil
}

type bufferAutoID struct {
	bufferBase
	firstID int64
	lastID  int64
}

const autoIDBase = 10

func (b *bufferAutoID) queue(ep **event.Event) {
	*ep = event.From(*ep, event.ID(strconv.FormatInt(b.lastID, autoIDBase)))
	b.lastID++
	b.buf = append(b.buf, *ep)
}

func (b *bufferAutoID) dequeue() {
	b.buf[0] = nil
	b.firstID++
	b.buf = b.buf[1:]
}

func (b *bufferAutoID) slice(at event.ID) ([]*event.Event, error) {
	id, err := strconv.ParseInt(string(at), autoIDBase, 64)
	if err != nil {
		return nil, &Error{id: at, err: err}
	}
	index := id - b.firstID
	if index < 0 || index >= int64(len(b.buf)) {
		return nil, &Error{id: at}
	}
	return b.buf[index:], nil
}

func getBuffer(autoIDs bool, cap int) buffer {
	base := bufferBase{buf: make([]*event.Event, 0, cap)}
	if autoIDs {
		return &bufferAutoID{bufferBase: base}
	}
	return &bufferNoID{bufferBase: base}
}
