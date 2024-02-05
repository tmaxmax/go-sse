package sse

import (
	"errors"
	"strconv"
	"sync"
	"time"
)

// NewFiniteReplayProvider creates a finite replay provider with the given max
// count and auto ID behaviour.
//
// Count is the maximum number of events FiniteReplayProvider should hold as
// valid. It must be greater than zero.
//
// AutoIDs configures FiniteReplayProvider to automatically set the IDs of
// events.
func NewFiniteReplayProvider(
	count int, autoIDs bool,
) (*FiniteReplayProvider, error) {
	if count < 1 {
		return nil, errors.New("count must be greater than zero")
	}

	return &FiniteReplayProvider{
		cap:     count,
		buf:     make([]messageWithTopics, count),
		count:   count,
		autoIDs: autoIDs,
	}, nil
}

// FiniteReplayProvider is a replay provider that replays at maximum a certain number of events.
// The events must have an ID unless the AutoIDs flag is toggled.
type FiniteReplayProvider struct {
	mut       sync.RWMutex
	cap       int
	buf       []messageWithTopics
	head      int
	tail      int
	count     int
	autoIDs   bool
	currentID int64
}

// Put puts a message into the provider's buffer. If there are more messages than the maximum
// number, the oldest message is removed.
func (f *FiniteReplayProvider) Put(message *Message, topics []string) *Message {
	if len(topics) == 0 {
		panic(errors.New(
			"go-sse: no topics provided for Message.\n" +
				formatMessagePanicString(message)))
	}

	f.mut.Lock()
	defer f.mut.Unlock()

	if f.autoIDs {
		f.currentID++

		message.ID = ID(strconv.FormatInt(f.currentID, 10))
	} else if !message.ID.IsSet() {
		panicString := "go-sse: a Message without an ID was given to a provider that doesn't set IDs automatically.\n" + formatMessagePanicString(message)

		panic(errors.New(panicString))
	}

	f.buf[f.tail] = messageWithTopics{message: message, topics: topics}

	f.tail++
	if f.tail >= f.cap {
		f.tail = 0
	}

	if f.tail == f.head {
		f.head = f.tail + 1

		if f.head > f.cap {
			f.head = 0
		}
	}

	return message
}

// Replay replays the messages in the buffer to the listener.
// It doesn't take into account the messages' expiry times.
func (f *FiniteReplayProvider) Replay(subscription Subscription) error {
	f.mut.RLock()

	if f.head == f.tail {
		f.mut.RUnlock()
		return nil
	}

	// The assumption here is that most of the lifetime of an application
	// the circular buffer would be full, so we might as well allocate in
	// one go.
	vs := make([]messageWithTopics, 0, f.cap)

	var n int

	// Copy head to end and start to tail when head is after tail.
	if f.tail < f.head {
		n = f.cap - f.tail
		copy(vs[0:n], f.buf[f.tail:])
		copy(vs[n:n+f.tail], f.buf[0:f.tail])
		// If the head is after the tail the buffer is full.
		n = f.cap
	} else {
		n = f.tail - f.head
		copy(vs[0:n], f.buf[f.head:f.tail])
	}

	f.mut.RUnlock()

	values := vs[0:n]

	for i := range values {
		// This could be done as part of the the initial copy to vs,
		// leaving it as a separate op for now.
		if values[i].message.ID.value == subscription.LastEventID.value {
			if i == len(values)-1 {
				values = nil
			} else {
				values = values[i+1:]
			}

			break
		}
	}

	for _, e := range values {
		if topicsIntersect(subscription.Topics, e.topics) {
			if err := subscription.Client.Send(e.message); err != nil {
				return err
			}
		}
	}

	return subscription.Client.Flush()
}

// ValidReplayProvider is a ReplayProvider that replays all the buffered non-expired events.
// You can use this provider for replaying an infinite number of events, if the events never
// expire.
// The provider removes any expired events when a new event is put and after at least
// a GCInterval period passed.
// The events must have an ID unless the AutoIDs flag is toggled.
type ValidReplayProvider struct {
	// The function used to retrieve the current time. Defaults to time.Now.
	// Useful when testing.
	Now func() time.Time

	lastGC   time.Time
	b        buffer
	expiries []time.Time

	// TTL is for how long a message is valid, since it was added.
	TTL time.Duration
	// After how long the ReplayProvider should attempt to clean up expired events.
	// By default cleanup is done after a fourth of the TTL has passed; this means
	// that messages may be stored for a duration equal to 5/4*TTL. If this is not
	// desired, set the GC interval to a value sensible for your use case or set
	// it to -1 – this disables automatic cleanup, enabling you to do it manually
	// using the GC method.
	GCInterval time.Duration
	// AutoIDs configures ValidReplayProvider to automatically set the IDs of events.
	AutoIDs bool
}

// Put puts the message into the provider's buffer.
func (v *ValidReplayProvider) Put(message *Message, topics []string) *Message {
	now := v.now()
	if v.b == nil {
		v.b = getBuffer(v.AutoIDs, 0)
		v.lastGC = now
	}

	if v.shouldGC(now) {
		v.doGC(now)
		v.lastGC = now
	}

	v.expiries = append(v.expiries, v.now().Add(v.TTL))
	return v.b.queue(message, topics)
}

func (v *ValidReplayProvider) shouldGC(now time.Time) bool {
	if v.GCInterval < 0 {
		return false
	}

	gcInterval := v.GCInterval
	if gcInterval == 0 {
		gcInterval = v.TTL / 4
	}

	return now.Sub(v.lastGC) >= gcInterval
}

// GC removes all the expired messages from the provider's buffer.
func (v *ValidReplayProvider) GC() {
	if v.b != nil {
		v.doGC(v.now())
	}
}

func (v *ValidReplayProvider) doGC(now time.Time) {
	for {
		e := v.b.front()
		if e == nil || v.expiries[0].After(now) {
			break
		}

		v.b.dequeue()
		v.expiries = v.expiries[1:]
	}
}

// Replay replays all the valid messages to the listener.
func (v *ValidReplayProvider) Replay(subscription Subscription) error {
	if v.b == nil {
		return nil
	}

	events := v.b.slice(subscription.LastEventID)
	if len(events) == 0 {
		return nil
	}

	now := v.now()
	expiriesOffset := v.b.len() - len(events)

	for i, e := range events {
		if v.expiries[i+expiriesOffset].After(now) && topicsIntersect(subscription.Topics, e.topics) {
			if err := subscription.Client.Send(e.message); err != nil {
				return err
			}
		}
	}

	return subscription.Client.Flush()
}

func (v *ValidReplayProvider) now() time.Time {
	if v.Now == nil {
		return time.Now()
	}

	return v.Now()
}

// topicsIntersect returns true if the given topic slices have at least one topic in common.
func topicsIntersect(a, b []string) bool {
	for _, at := range a {
		for _, bt := range b {
			if at == bt {
				return true
			}
		}
	}

	return false
}
