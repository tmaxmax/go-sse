package sse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func msg(tb testing.TB, data, id, topic string) *Message {
	tb.Helper()

	e := &Message{Topic: topic}
	e.AppendData(data)
	if id != "" {
		e.ID = ID(id)
	}

	return e
}

func replay(tb testing.TB, p ReplayProvider, lastEventID EventID, topics ...string) []*Message {
	tb.Helper()

	if len(topics) == 0 {
		topics = []string{DefaultTopic}
	}

	var replayed []*Message
	cb := func(m *Message) bool {
		replayed = append(replayed, m)
		return true
	}

	sub := Subscription{
		Callback:    cb,
		LastEventID: lastEventID,
		Topics:      topics,
	}

	_ = p.Replay(sub)

	sub.LastEventID = EventID{}
	_ = p.Replay(sub)

	sub.LastEventID = ID("mama")
	_ = p.Replay(sub)

	sub.LastEventID = ID("10")
	_ = p.Replay(sub)

	return replayed
}

func testReplayError(tb testing.TB, p ReplayProvider) {
	tb.Helper()

	_, isExpiry := p.(*ValidReplayProvider)

	var now time.Time
	if isExpiry {
		now = time.Now()
		timeNow = func() time.Time { return now.Add(time.Hour) }
	}

	putMessages(p,
		msg(tb, "a", "1", DefaultTopic),
		msg(tb, "b", "2", DefaultTopic),
	)

	cb := func(_ *Message) bool { return false }
	if isExpiry {
		timeNow = func() time.Time { return now }
	}

	success := p.Replay(Subscription{
		Callback:    cb,
		LastEventID: ID("1"),
		Topics:      []string{DefaultTopic},
	})

	require.False(tb, success, "received invalid error")
}

func putMessages(p ReplayProvider, msgs ...*Message) {
	for i := range msgs {
		msgs[i] = p.Put(msgs[i])
	}
}

func TestValidReplayProvider(t *testing.T) {
	t.Parallel()
	t.Cleanup(func() { timeNow = time.Now })

	p := &ValidReplayProvider{
		TTL:     time.Millisecond * 5,
		AutoIDs: true,
	}

	require.NoError(t, p.GC(), "unexpected GC error") // no elements, noop

	now := time.Now()
	initialNow := now
	timeNow = func() time.Time { return now }

	putMessages(p,
		msg(t, "hi", "", DefaultTopic),
		msg(t, "there", "", "t"),
	)
	now = now.Add(p.TTL)
	putMessages(p,
		msg(t, "world", "", DefaultTopic),
		msg(t, "again", "", "t"),
	)
	now = now.Add(p.TTL * 3)
	putMessages(p,
		msg(t, "world", "", DefaultTopic),
		msg(t, "x", "", "t"),
	)
	now = now.Add(p.TTL * 5)
	putMessages(p, msg(t, "again", "", "t"))

	now = initialNow.Add(p.TTL)

	require.NoError(t, p.GC(), "unexpected GC error")

	now = now.Add(p.TTL)

	replayed := replay(t, p, ID("3"))[0]
	require.Equal(t, "id: 4\ndata: world\n\n", replayed.String())

	testReplayError(t, &ValidReplayProvider{})
}

func TestFiniteReplayProvider(t *testing.T) {
	t.Parallel()

	p := NewFiniteReplayProvider(3)

	putMessages(p,
		msg(t, "", "1", ""),
		msg(t, "hello", "2", DefaultTopic),
		msg(t, "there", "3", "t"),
		msg(t, "world", "4", DefaultTopic),
	)

	replayed := replay(t, p, ID("2"))[0]
	require.Equal(t, "id: 4\ndata: world\n\n", replayed.String())

	putMessages(p,
		msg(t, "", "5", "t"),
		msg(t, "", "6", "t"),
		msg(t, "again", "7", DefaultTopic),
	)

	replayed = replay(t, p, ID("4"))[0]
	require.Equal(t, "id: 7\ndata: again\n\n", replayed.String())

	testReplayError(t, NewFiniteReplayProvider(10))
}
