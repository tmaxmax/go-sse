package replay

import (
	"strconv"

	"github.com/tmaxmax/go-sse/server/event"
)

// A buffer is the underlying storage for a provider. Its methods are used by the provider to implement
// the Provider interface.
type buffer interface {
	queue(**event.Event, []string)
	dequeue()
	front() *event.Event
	len() int
	slice(event.ID) ([]*ev, error)
}

type ev struct {
	e      *event.Event
	topics map[string]struct{}
}

func (e *ev) isInTopics(topics []string) bool {
	for _, t := range topics {
		if _, ok := e.topics[t]; ok {
			return true
		}
	}
	return false
}

func newEvent(e *event.Event, topics []string) *ev {
	m := map[string]struct{}{}
	for _, t := range topics {
		m[t] = struct{}{}
	}

	return &ev{e, m}
}

type bufferBase struct {
	buf []*ev
}

func (b *bufferBase) len() int {
	return len(b.buf)
}

func (b *bufferBase) front() *event.Event {
	if b.len() == 0 {
		return nil
	}
	return b.buf[0].e
}

type bufferNoID struct {
	bufferBase
	lastRemoved event.ID
}

func (b *bufferNoID) queue(ep **event.Event, topics []string) {
	e := *ep
	if e.ID() != "" {
		b.buf = append(b.buf, newEvent(e, topics))
	}
}

func (b *bufferNoID) dequeue() {
	b.lastRemoved = b.buf[0].e.ID()
	b.buf[0] = nil
	b.buf = b.buf[1:]
}

func (b *bufferNoID) slice(at event.ID) ([]*ev, error) {
	if at == b.lastRemoved && len(b.buf) != 0 {
		return b.buf, nil
	}
	index := -1
	for i := range b.buf {
		if at == b.buf[i].e.ID() {
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

func (b *bufferAutoID) queue(ep **event.Event, topics []string) {
	*ep = event.From(*ep, event.ID(strconv.FormatInt(b.lastID, autoIDBase)))
	b.lastID++
	b.buf = append(b.buf, newEvent(*ep, topics))
}

func (b *bufferAutoID) dequeue() {
	b.buf[0] = nil
	b.firstID++
	b.buf = b.buf[1:]
}

func (b *bufferAutoID) slice(at event.ID) ([]*ev, error) {
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
	base := bufferBase{buf: make([]*ev, 0, cap)}
	if autoIDs {
		return &bufferAutoID{bufferBase: base}
	}
	return &bufferNoID{bufferBase: base}
}
