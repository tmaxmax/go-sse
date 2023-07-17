package sse

import (
	"strconv"
)

// A buffer is the underlying storage for a provider. Its methods are used by the provider to implement
// the Provider interface.
type buffer interface {
	queue(message *Message, topics []string) *Message
	dequeue()
	front() *messageWithTopics
	len() int
	cap() int
	slice(EventID) []messageWithTopics
}

type bufferBase struct {
	buf []messageWithTopics
}

func (b *bufferBase) len() int {
	return len(b.buf)
}

func (b *bufferBase) cap() int {
	return cap(b.buf)
}

func (b *bufferBase) front() *messageWithTopics {
	if b.len() == 0 {
		return nil
	}
	return &b.buf[0]
}

type bufferNoID struct {
	lastRemovedID EventID
	bufferBase
}

func (b *bufferNoID) queue(message *Message, topics []string) *Message {
	if !message.ID.IsSet() {
		return nil
	}

	b.buf = append(b.buf, messageWithTopics{message: message, topics: topics})

	return message
}

func (b *bufferNoID) dequeue() {
	b.lastRemovedID = b.buf[0].message.ID
	b.buf = b.buf[1:]
}

func (b *bufferNoID) slice(atID EventID) []messageWithTopics {
	if !atID.IsSet() {
		return nil
	}
	if atID == b.lastRemovedID {
		return b.buf
	}
	index := -1
	for i := range b.buf {
		if atID == b.buf[i].message.ID {
			index = i
			break
		}
	}
	if index == -1 {
		return nil
	}

	return b.buf[index+1:]
}

type bufferAutoID struct {
	bufferBase
	firstID    int64
	upcomingID int64
}

const autoIDBase = 10

func (b *bufferAutoID) queue(message *Message, topics []string) *Message {
	message = message.Clone()
	message.ID = ID(strconv.FormatInt(b.upcomingID, autoIDBase))
	b.upcomingID++
	b.buf = append(b.buf, messageWithTopics{message: message, topics: topics})

	return message
}

func (b *bufferAutoID) dequeue() {
	b.firstID++
	b.buf = b.buf[1:]
}

func (b *bufferAutoID) slice(atID EventID) []messageWithTopics {
	id, err := strconv.ParseInt(atID.String(), autoIDBase, 64)
	if err != nil {
		return nil
	}
	index := id - b.firstID
	if index < -1 || index >= int64(len(b.buf)) {
		return nil
	}
	return b.buf[index+1:]
}

func getBuffer(autoIDs bool, capacity int) buffer {
	base := bufferBase{buf: make([]messageWithTopics, 0, capacity)}
	if autoIDs {
		return &bufferAutoID{bufferBase: base}
	}
	return &bufferNoID{bufferBase: base}
}
