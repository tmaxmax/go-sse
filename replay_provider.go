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

// NewValidReplayProvider creates a replay Provider that replays all the buffered non-expired events.
// Call its GC method periodically to remove expired events from the buffer and release resources.
// You can use this provider for replaying an infinite number of events, if the events never
// expire.
// The events must have an ID unless the provider is constructed with autoIDs flag as true.
func NewValidReplayProvider(autoIDs ...bool) *ValidReplayProvider {
	return &ValidReplayProvider{b: getBuffer(isAutoIDsSet(autoIDs), 0)}
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
func (f *FiniteReplayProvider) Put(message **Message) {
	if f.b.len() == f.count {
		f.b.dequeue()
	}

	f.b.queue(message)
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

// ValidReplayProvider is a replay provider that replays all the valid (not expired) previous events.
type ValidReplayProvider struct {
	b buffer
}

// Put puts the message into the provider's buffer.
func (v *ValidReplayProvider) Put(message **Message) {
	v.b.queue(message)
}

// GC removes all the expired messages from the provider's buffer.
func (v *ValidReplayProvider) GC() error {
	now := time.Now()

	var e *Message
	for {
		e = v.b.front()
		if e == nil || e.ExpiresAt.After(now) {
			break
		}
		v.b.dequeue()
	}

	return nil
}

// Replay replays all the valid messages to the listener.
func (v *ValidReplayProvider) Replay(subscription Subscription) bool {
	events := v.b.slice(subscription.LastEventID)
	if events == nil {
		return true
	}

	now := time.Now()
	for _, e := range events {
		if e.ExpiresAt.After(now) && hasTopic(subscription.Topics, e.Topic) {
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
