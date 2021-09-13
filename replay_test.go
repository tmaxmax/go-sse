package sse_test

import (
	"testing"
	"time"

	"github.com/tmaxmax/go-sse"

	"github.com/stretchr/testify/require"
)

func msg(tb testing.TB, data, id string, expiry time.Duration, topic string) *sse.Message {
	tb.Helper()

	e := &sse.Message{Topic: topic}
	e.AppendText(data)
	e.SetTTL(expiry)
	if id != "" {
		e.SetID(sse.MustEventID(id))
	}

	return e
}

func putMessages(p sse.ReplayProvider, msgs ...*sse.Message) {
	for i := range msgs {
		p.Put(&msgs[i])
	}
}

func testNoopReplays(p sse.ReplayProvider, ch chan<- *sse.Message) {
	p.Replay(sse.Subscription{ // unset ID, noop
		Channel:     ch,
		LastEventID: sse.EventID{},
	})
	p.Replay(sse.Subscription{ // invalid ID, noop
		Channel:     ch,
		LastEventID: sse.MustEventID("mama"),
	})
	p.Replay(sse.Subscription{ // nonexistent ID, noop
		Channel:     ch,
		LastEventID: sse.MustEventID("10"),
	})
}

func TestValidReplayProvider(t *testing.T) {
	t.Parallel()

	p := sse.NewValidReplayProvider(true)
	ch := make(chan *sse.Message, 2)

	require.NoError(t, p.GC(), "unexpected GC error") // no elements, noop

	exp := time.Millisecond * 5

	putMessages(p,
		msg(t, "hi", "", exp, sse.DefaultTopic),
		msg(t, "there", "", exp, "t"),
		msg(t, "world", "", exp*2, sse.DefaultTopic),
		msg(t, "again", "", exp*2, "t"),
		msg(t, "world", "", exp*5, sse.DefaultTopic),
		msg(t, "again", "", exp*5, "t"),
	)

	time.Sleep(exp)

	require.NoError(t, p.GC(), "unexpected GC error")

	time.Sleep(exp)

	p.Replay(sse.Subscription{
		Channel:     ch,
		LastEventID: sse.MustEventID("3"),
		Topics:      []string{sse.DefaultTopic},
	})
	testNoopReplays(p, ch)

	data := (<-ch).String()

	require.Equal(t, "id: 4\ndata: world\n\n", data)
}

func TestFiniteReplayProvider(t *testing.T) {
	t.Parallel()

	p := sse.NewFiniteReplayProvider(3)
	ch := make(chan *sse.Message, 1)

	require.NoError(t, p.GC(), "unexpected GC error") // GC is not required, noop

	putMessages(p,
		msg(t, "", "1", 0, ""),
		msg(t, "hello", "2", 0, sse.DefaultTopic),
		msg(t, "there", "3", 0, "t"),
		msg(t, "world", "4", 0, sse.DefaultTopic),
	)

	p.Replay(sse.Subscription{
		Channel:     ch,
		LastEventID: sse.MustEventID("2"),
		Topics:      []string{sse.DefaultTopic},
	})
	testNoopReplays(p, ch)

	data := (<-ch).String()

	require.Equal(t, "id: 4\ndata: world\n\n", data)

	putMessages(p,
		msg(t, "", "5", 0, "t"),
		msg(t, "", "6", 0, "t"),
		msg(t, "again", "7", 0, sse.DefaultTopic),
	)

	p.Replay(sse.Subscription{
		Channel:     ch,
		LastEventID: sse.MustEventID("4"),
		Topics:      []string{sse.DefaultTopic},
	})

	data = (<-ch).String()

	require.Equal(t, "id: 7\ndata: again\n\n", data)
}
