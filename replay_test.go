package sse_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse"
)

func msg(tb testing.TB, data, id string, expiry time.Duration, topic string) *sse.Message {
	tb.Helper()

	e := &sse.Message{Topic: topic, ExpiresAt: time.Now().Add(expiry)}
	e.AppendData(data)
	if id != "" {
		e.SetID(sse.MustEventID(id))
	}

	return e
}

func replay(tb testing.TB, p sse.ReplayProvider, lastEventID sse.EventID, topics ...string) []*sse.Message {
	tb.Helper()

	if len(topics) == 0 {
		topics = []string{sse.DefaultTopic}
	}

	var replayed []*sse.Message
	cb := func(m *sse.Message) bool {
		replayed = append(replayed, m)
		return true
	}

	sub := sse.Subscription{
		Callback:    cb,
		LastEventID: lastEventID,
		Topics:      topics,
	}

	_ = p.Replay(sub)

	sub.LastEventID = sse.EventID{}
	_ = p.Replay(sub)

	sub.LastEventID = sse.MustEventID("mama")
	_ = p.Replay(sub)

	sub.LastEventID = sse.MustEventID("10")
	_ = p.Replay(sub)

	return replayed
}

func testReplayError(tb testing.TB, p sse.ReplayProvider) {
	tb.Helper()

	putMessages(p,
		msg(tb, "a", "1", time.Hour, sse.DefaultTopic),
		msg(tb, "b", "2", time.Hour, sse.DefaultTopic),
	)

	cb := func(_ *sse.Message) bool { return false }

	success := p.Replay(sse.Subscription{
		Callback:    cb,
		LastEventID: sse.MustEventID("1"),
		Topics:      []string{sse.DefaultTopic},
	})

	require.False(tb, success, "received invalid error")
}

func putMessages(p sse.ReplayProvider, msgs ...*sse.Message) {
	for i := range msgs {
		p.Put(&msgs[i])
	}
}

func TestValidReplayProvider(t *testing.T) {
	t.Parallel()

	p := sse.NewValidReplayProvider(true)

	require.NoError(t, p.GC(), "unexpected GC error") // no elements, noop

	exp := time.Millisecond * 5

	putMessages(p,
		msg(t, "hi", "", exp, sse.DefaultTopic),
		msg(t, "there", "", exp, "t"),
		msg(t, "world", "", exp*2, sse.DefaultTopic),
		msg(t, "again", "", exp*2, "t"),
		msg(t, "world", "", exp*5, sse.DefaultTopic),
		msg(t, "x", "", exp*5, "t"),
		msg(t, "again", "", exp*10, "t"),
	)

	time.Sleep(exp)

	require.NoError(t, p.GC(), "unexpected GC error")

	time.Sleep(exp)

	replayed := replay(t, p, sse.MustEventID("3"))[0]
	require.Equal(t, "id: 4\ndata: world\n\n", replayed.String())

	testReplayError(t, sse.NewValidReplayProvider())
}

func TestFiniteReplayProvider(t *testing.T) {
	t.Parallel()

	p := sse.NewFiniteReplayProvider(3)

	putMessages(p,
		msg(t, "", "1", 0, ""),
		msg(t, "hello", "2", 0, sse.DefaultTopic),
		msg(t, "there", "3", 0, "t"),
		msg(t, "world", "4", 0, sse.DefaultTopic),
	)

	replayed := replay(t, p, sse.MustEventID("2"))[0]
	require.Equal(t, "id: 4\ndata: world\n\n", replayed.String())

	putMessages(p,
		msg(t, "", "5", 0, "t"),
		msg(t, "", "6", 0, "t"),
		msg(t, "again", "7", 0, sse.DefaultTopic),
	)

	replayed = replay(t, p, sse.MustEventID("4"))[0]
	require.Equal(t, "id: 7\ndata: again\n\n", replayed.String())

	testReplayError(t, sse.NewFiniteReplayProvider(10))
}
