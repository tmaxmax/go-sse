package server

import (
	"time"

	"github.com/tmaxmax/go-sse/server/event"
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

func (f *FiniteReplayProvider) Put(message *Message) {
	if f.b.len() == f.count {
		f.b.dequeue()
	}

	f.b.queue(message)
}

func (f *FiniteReplayProvider) GC() error { return nil }

func (f *FiniteReplayProvider) Replay(subscription Subscription) {
	events, err := f.b.slice(subscription.LastEventID)
	if err != nil {
		return
	}

	for _, e := range events[1:] {
		if hasTopic(subscription.Topics, e.Topic) {
			subscription.Channel <- e.Event
		}
	}
}

// ValidReplayProvider is a replay provider that replays all the valid (not expired) previous events.
type ValidReplayProvider struct {
	b buffer
}

func (v *ValidReplayProvider) Put(message *Message) {
	v.b.queue(message)
}

func (v *ValidReplayProvider) GC() error {
	now := time.Now()

	var e *event.Event
	for {
		e = v.b.front()
		if e == nil || e.ExpiresAt().After(now) {
			break
		}
		v.b.dequeue()
	}

	return nil
}

func (v *ValidReplayProvider) Replay(subscription Subscription) {
	events, err := v.b.slice(subscription.LastEventID)
	if err != nil {
		return
	}

	now := time.Now()
	for _, e := range events[1:] {
		if e.Event.ExpiresAt().After(now) && hasTopic(subscription.Topics, e.Topic) {
			subscription.Channel <- e.Event
		}
	}
}

var _ ReplayProvider = (*FiniteReplayProvider)(nil)
var _ ReplayProvider = (*ValidReplayProvider)(nil)
