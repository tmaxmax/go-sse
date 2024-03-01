package sse

import (
	"errors"
	"strconv"
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
	if count < 2 {
		return nil, errors.New("count must be at least 2")
	}

	return &FiniteReplayProvider{
		cap:     count,
		buf:     make([]messageWithTopics, count),
		autoIDs: autoIDs,
	}, nil
}

// FiniteReplayProvider is a replay provider that replays at maximum a certain number of events.
// The events must have an ID unless the AutoIDs flag is toggled.
type FiniteReplayProvider struct {
	buf       []messageWithTopics
	cap       int
	head      int
	tail      int
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
	if f.head == f.tail {
		return nil
	}

	// Replay head to end and start to tail when head is after tail.
	if f.tail < f.head {
		foundFirst, err := replay(subscription, f.buf[f.tail:], false)
		if err != nil {
			return err
		}

		_, err = replay(subscription, f.buf[0:f.tail], foundFirst)
		if err != nil {
			return err
		}
	} else {
		_, err := replay(subscription, f.buf[0:f.tail], false)
		if err != nil {
			return err
		}
	}

	return subscription.Client.Flush()
}

func replay(
	sub Subscription, events []messageWithTopics, foundFirstEvent bool,
) (hasFoundFirstEvent bool, err error) {
	for _, e := range events {
		if !foundFirstEvent && e.message.ID == sub.LastEventID {
			foundFirstEvent = true

			continue
		}

		if foundFirstEvent && topicsIntersect(sub.Topics, e.topics) {
			if err := sub.Client.Send(e.message); err != nil {
				return false, err
			}
		}
	}

	return foundFirstEvent, nil
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
