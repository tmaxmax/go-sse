package sse

import (
	"time"
)

// FiniteReplayProvider is a replay provider that replays at maximum a certain number of events.
// GC is a no-op for this provider, as when the maximum number of values is reached
// and a new value has to be appended, old values are removed from the buffer.
// The events must have an ID unless the AutoIDs flag is toggled.
type FiniteReplayProvider struct {
	b buffer

	// Count is the maximum number of events FiniteReplayProvider should hold as valid.
	// It must be a positive integer, or the code will panic.
	Count int
	// AutoIDs configures FiniteReplayProvider to automatically set the IDs of events.
	AutoIDs bool
}

// Put puts a message into the provider's buffer. If there are more messages than the maximum
// number, the oldest message is removed.
func (f *FiniteReplayProvider) Put(message *Message, topics []string) *Message {
	if f.b == nil {
		f.b = getBuffer(f.AutoIDs, f.Count)
	}

	if f.b.len() == f.b.cap() {
		f.b.dequeue()
	}

	return f.b.queue(message, topics)
}

// Replay replays the messages in the buffer to the listener.
// It doesn't take into account the messages' expiry times.
func (f *FiniteReplayProvider) Replay(subscription Subscription) error {
	if f.b == nil {
		return nil
	}

	events := f.b.slice(subscription.LastEventID)
	if len(events) == 0 {
		return nil
	}

	for _, e := range events {
		if topicsIntersect(subscription.Topics, e.topics) {
			if err := subscription.Client.Send(e.message); err != nil {
				return err
			}
		}
	}

	return subscription.Client.Flush()
}

// ValidReplayProvider is a ReplayProvider that replays all the buffered non-expired events.
// Call its GC method periodically to remove expired events from the buffer and release resources.
// You can use this provider for replaying an infinite number of events, if the events never
// expire.
// The events must have an ID unless the AutoIDs flag is toggled.
type ValidReplayProvider struct {
	// The function used to retrieve the current time. Defaults to time.Now.
	// Useful when testing.
	Now func() time.Time

	b        buffer
	expiries []time.Time

	// TTL is for how long a message is valid, since it was added.
	TTL time.Duration
	// AutoIDs configures ValidReplayProvider to automatically set the IDs of events.
	AutoIDs bool
}

// Put puts the message into the provider's buffer.
func (v *ValidReplayProvider) Put(message *Message, topics []string) *Message {
	if v.b == nil {
		v.b = getBuffer(v.AutoIDs, 0)
	}

	v.expiries = append(v.expiries, v.now().Add(v.TTL))
	return v.b.queue(message, topics)
}

// GC removes all the expired messages from the provider's buffer.
func (v *ValidReplayProvider) GC() error {
	if v.b == nil {
		return nil
	}

	now := v.now()

	for {
		e := v.b.front()
		if e == nil || v.expiries[0].After(now) {
			break
		}

		v.b.dequeue()
		v.expiries = v.expiries[1:]
	}

	return nil
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
