package sse

import (
	"time"
)

func isAutoIDsSet(input []bool) bool {
	return len(input) > 0 && input[0]
}

// NewFiniteReplayProvider creates a replay provider that can hold a maximum number of events at once.
// The events' expiry times are not considered, as the oldest events are removed
// anyway when the provider has buffered the maximum number of events.
// The events must have an ID unless the provider is constructed with the autoIDs flag.
func NewFiniteReplayProvider(count int, autoIDs ...bool) *FiniteReplayProvider {
	return &FiniteReplayProvider{count: count, b: getBuffer(isAutoIDsSet(autoIDs), count)}
}

// FiniteReplayProvider is a replay provider that replays at maximum a certain number of events.
// GC is a no-op for this provider, as when the maximum number of values is reached
// and a new value has to be appended, old values are removed from the buffer.
type FiniteReplayProvider struct {
	b     buffer
	count int
}

// Put puts a message into the provider's buffer. If there are more messages than the maximum
// number, the oldest message is removed.
func (f *FiniteReplayProvider) Put(message *Message) *Message {
	if f.b.len() == f.count {
		f.b.dequeue()
	}

	return f.b.queue(message)
}

// Replay replays the messages in the buffer to the listener.
// It doesn't take into account the messages' expiry times.
func (f *FiniteReplayProvider) Replay(subscription Subscription) bool {
	events := f.b.slice(subscription.LastEventID)
	if events == nil {
		return true
	}

	for _, e := range events {
		if hasTopic(subscription.Topics, e.Topic) {
			if !subscription.Callback(e) {
				return false
			}
		}
	}

	return true
}

// ValidReplayProvider is a ReplayProvider that replays all the buffered non-expired events.
// Call its GC method periodically to remove expired events from the buffer and release resources.
// You can use this provider for replaying an infinite number of events, if the events never
// expire.
// The events must have an ID unless the AutoIDs flag is toggled.
type ValidReplayProvider struct {
	b        buffer
	expiries []time.Time

	// TTL is for how long a message is valid, since it was added.
	TTL time.Duration
	// AutoIDs configures ValidReplayProvider to automatically set the IDs of events.
	AutoIDs bool
}

// timeNow is used by tests to get a deterministic time.
var timeNow = time.Now

// Put puts the message into the provider's buffer.
func (v *ValidReplayProvider) Put(message *Message) *Message {
	if v.b == nil {
		v.b = getBuffer(v.AutoIDs, 0)
	}

	v.expiries = append(v.expiries, timeNow().Add(v.TTL))
	return v.b.queue(message)
}

// GC removes all the expired messages from the provider's buffer.
func (v *ValidReplayProvider) GC() error {
	if v.b == nil {
		return nil
	}

	now := timeNow()

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
func (v *ValidReplayProvider) Replay(subscription Subscription) bool {
	if v.b == nil {
		return true
	}

	events := v.b.slice(subscription.LastEventID)
	if events == nil {
		return true
	}

	now := timeNow()
	expiriesOffset := v.b.len() - len(events)

	for i, e := range events {
		if v.expiries[i+expiriesOffset].After(now) && hasTopic(subscription.Topics, e.Topic) {
			if !subscription.Callback(e) {
				return false
			}
		}
	}

	return true
}

var (
	_ ReplayProvider = (*FiniteReplayProvider)(nil)
	_ ReplayProvider = (*ValidReplayProvider)(nil)
)

// hasTopic returns true if the given topic is inside the given topics slice.
func hasTopic(topics []string, topic string) bool {
	for i := 0; i < len(topics); i++ {
		if topics[i] == topic {
			return true
		}
	}
	return false
}
