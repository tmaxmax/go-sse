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

	r := &FiniteReplayProvider{}
	r.buf.buf = make([]messageWithTopics, count)
	if autoIDs {
		r.currentID = new(uint64)
	}

	return r, nil
}

// FiniteReplayProvider is a replay provider that replays at maximum a certain number of events.
// The events must have an ID unless the replay provider is configured to set IDs automatically.
type FiniteReplayProvider struct {
	currentID *uint64
	buf       queue[messageWithTopics]
}

// Put puts a message into the provider's buffer. If there are more messages than the maximum
// number, the oldest message is removed.
func (f *FiniteReplayProvider) Put(message *Message, topics []string) *Message {
	if len(topics) == 0 {
		panic(errors.New("go-sse: no topics provided for Message"))
	}

	message, err := ensureID(message, f.currentID)
	if err != nil {
		panic(err)
	}

	f.buf.enqueue(messageWithTopics{message: message, topics: topics})

	return message
}

// Replay replays the messages in the buffer to the listener.
// It doesn't take into account the messages' expiry times.
func (f *FiniteReplayProvider) Replay(subscription Subscription) error {
	i := findIDInQueue(&f.buf, subscription.LastEventID, f.currentID != nil)
	if i < 0 {
		return nil
	}

	first, second := f.buf.after(i)

	for _, e := range first {
		if topicsIntersect(subscription.Topics, e.topics) {
			if err := subscription.Client.Send(e.message); err != nil {
				return err
			}
		}
	}

	for _, e := range second {
		if topicsIntersect(subscription.Topics, e.topics) {
			if err := subscription.Client.Send(e.message); err != nil {
				return err
			}
		}
	}

	return subscription.Client.Flush()
}

// ValidReplayProvider is a ReplayProvider that replays all the buffered non-expired events.
//
// The provider removes any expired events when a new event is put and after at least
// a GCInterval period passed.
//
// The events must have an ID unless the replay provider is configured to set IDs automatically.
type ValidReplayProvider struct {
	lastGC time.Time

	// The function used to retrieve the current time. Defaults to time.Now.
	// Useful when testing.
	Now func() time.Time

	currentID *uint64
	messages  queue[messageWithTopicsAndExpiry]

	ttl time.Duration
	// After how long the ReplayProvider should attempt to clean up expired events.
	// By default cleanup is done after a fourth of the TTL has passed; this means
	// that messages may be stored for a duration equal to 5/4*TTL. If this is not
	// desired, set the GC interval to a value sensible for your use case or set
	// it to 0 – this disables automatic cleanup, enabling you to do it manually
	// using the GC method.
	GCInterval time.Duration
}

// NewValidReplayProvider creates a valid replay provider with the given message
// lifetime duration (time-to-live) and auto ID behavior.
//
// The TTL must be a positive duration. It is technically possible to use a very
// big duration in order to store and replay every message put for the lifetime
// of the program; this is not recommended, as memory usage becomes effectively
// unbounded which might lead to a crash.
func NewValidReplayProvider(ttl time.Duration, autoIDs bool) (*ValidReplayProvider, error) {
	if ttl <= 0 {
		return nil, errors.New("event TTL must be greater than zero")
	}

	r := &ValidReplayProvider{
		Now:        time.Now,
		GCInterval: ttl / 4,
		ttl:        ttl,
	}

	if autoIDs {
		r.currentID = new(uint64)
	}

	return r, nil
}

// Put puts the message into the provider's buffer.
func (v *ValidReplayProvider) Put(message *Message, topics []string) *Message {
	if len(topics) == 0 {
		panic(errors.New("go-sse: no topics provided for Message"))
	}

	now := v.Now()
	if v.lastGC.IsZero() {
		v.lastGC = now
	}

	if v.shouldGC(now) {
		v.doGC(now)
		v.lastGC = now
	}

	message, err := ensureID(message, v.currentID)
	if err != nil {
		panic(err)
	}

	if v.messages.count == len(v.messages.buf) {
		newCap := len(v.messages.buf) * 2
		if minCap := 4; newCap < minCap {
			newCap = minCap
		}
		v.messages.resize(newCap)
	}

	v.messages.enqueue(messageWithTopicsAndExpiry{messageWithTopics: messageWithTopics{message: message, topics: topics}, exp: now})

	return message
}

func (v *ValidReplayProvider) shouldGC(now time.Time) bool {
	return v.GCInterval > 0 && now.Sub(v.lastGC) >= v.GCInterval
}

// GC removes all the expired messages from the provider's buffer.
func (v *ValidReplayProvider) GC() {
	v.doGC(v.Now())
}

func (v *ValidReplayProvider) doGC(now time.Time) {
	for v.messages.count > 0 {
		e := v.messages.buf[v.messages.head]
		if e.exp.After(now) {
			break
		}

		v.messages.dequeue()
	}

	if v.messages.count <= len(v.messages.buf)/4 {
		newCap := len(v.messages.buf) / 2
		if minCap := 4; newCap < minCap {
			newCap = minCap
		}
		v.messages.resize(newCap)
	}
}

// Replay replays all the valid messages to the listener.
func (v *ValidReplayProvider) Replay(subscription Subscription) error {
	i := findIDInQueue(&v.messages, subscription.LastEventID, v.currentID != nil)
	if i < 0 {
		return nil
	}

	first, second := v.messages.after(i)
	now := v.Now()

	for _, e := range first {
		if e.exp.After(now) && topicsIntersect(subscription.Topics, e.topics) {
			if err := subscription.Client.Send(e.message); err != nil {
				return err
			}
		}
	}

	for _, e := range second {
		if e.exp.After(now) && topicsIntersect(subscription.Topics, e.topics) {
			if err := subscription.Client.Send(e.message); err != nil {
				return err
			}
		}
	}

	return subscription.Client.Flush()
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

func ensureID(m *Message, currentID *uint64) (*Message, error) {
	if currentID == nil {
		if !m.ID.IsSet() {
			return nil, errors.New("message has no ID")
		}

		return m, nil
	}

	if m.ID.IsSet() {
		return nil, errors.New("message already has an ID, can't use generated ID")
	}

	m = m.Clone()
	m.ID = ID(strconv.FormatUint(*currentID, 10))

	(*currentID)++

	return m, nil
}

type queue[T any] struct {
	buf               []T
	head, tail, count int
}

func (q *queue[T]) after(i int) (first, second []T) {
	if i < q.tail {
		return q.buf[i:q.tail], nil
	}

	return q.buf[i:], q.buf[:q.tail]
}

func (q *queue[T]) enqueue(v T) {
	q.buf[q.tail] = v

	q.tail++

	overwritten := false
	if q.tail > q.head && q.count == len(q.buf) {
		q.head = q.tail
		overwritten = true
	} else {
		q.count++
	}

	if q.tail == len(q.buf) {
		q.tail = 0
		if overwritten {
			q.head = 0
		}
	}
}

func (q *queue[T]) dequeue() {
	q.buf[q.head] = *new(T)

	q.head++
	if q.head == len(q.buf) {
		q.head = 0
	}

	q.count--
}

func (q *queue[T]) resize(newSize int) {
	buf := make([]T, newSize)
	if q.head < q.tail {
		copy(buf, q.buf[q.head:q.tail])
	} else {
		n := copy(buf, q.buf[q.head:])
		copy(buf[n:], q.buf[:q.tail])
	}

	q.head = 0
	q.tail = q.count
	q.buf = buf
}

func findIDInQueue[M interface{ ID() EventID }](q *queue[M], id EventID, autoID bool) int {
	if q.count == 0 {
		return -1
	}

	if autoID {
		id, err := strconv.ParseUint(id.String(), 10, 64)
		if err != nil {
			return -1
		}

		firstID, _ := strconv.ParseUint(q.buf[q.head].ID().String(), 10, 64)

		pos := -1
		if delta := id - firstID; id >= firstID {
			if delta >= uint64(q.count) { //nolint:gosec // int always positive
				return -1
			}
			pos = int(delta) //nolint:gosec // delta < q.count, which is an int
		}

		i := pos + q.head + 1
		if i >= len(q.buf) {
			i -= len(q.buf)
		}

		return i
	}

	i := -1
	if q.head < q.tail {
		for j := q.head; i == -1 && j < q.tail; j++ {
			if q.buf[j].ID() == id {
				i = j
			}
		}
	} else {
		for j := q.head; i == -1 && j < len(q.buf); j++ {
			if q.buf[j].ID() == id {
				i = j
			}
		}

		for j := 0; i == -1 && j < q.tail; j++ {
			if q.buf[j].ID() == id {
				i = j
			}
		}
	}

	if i != -1 {
		i++
		if i == len(q.buf) {
			i = 0
		} else if i == q.tail {
			i = -1
		}
	}

	return i
}

func (m messageWithTopics) ID() EventID { return m.message.ID }

type messageWithTopicsAndExpiry struct {
	exp time.Time
	messageWithTopics
}

// noopReplayProvider is the default replay provider used if none is given. It does nothing.
// It is used to avoid nil checks for the provider each time it is used.
type noopReplayProvider struct{}

func (n noopReplayProvider) Put(m *Message, _ []string) *Message { return m }
func (n noopReplayProvider) Replay(_ Subscription) error         { return nil }
