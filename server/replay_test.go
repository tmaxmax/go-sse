package server_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse/server/event"

	"github.com/tmaxmax/go-sse/server"
)

func msg(tb testing.TB, data, id string, expiry time.Duration, topic string) server.Message {
	tb.Helper()

	e := &event.Event{}
	e.AppendText(data)
	e.SetTTL(expiry)
	if id != "" {
		e.SetID(event.MustID(id))
	}

	return server.Message{
		Event: e,
		Topic: topic,
	}
}

func putMessages(p server.ReplayProvider, msgs ...server.Message) {
	for _, m := range msgs {
		p.Put(&m)
	}
}

func testNoopReplays(p server.ReplayProvider, ch chan<- *event.Event) {
	p.Replay(server.Subscription{ // unset ID, noop
		Channel:     ch,
		LastEventID: event.ID{},
	})
	p.Replay(server.Subscription{ // invalid ID, noop
		Channel:     ch,
		LastEventID: event.MustID("mama"),
	})
	p.Replay(server.Subscription{ // nonexistent ID, noop
		Channel:     ch,
		LastEventID: event.MustID("10"),
	})
}

func TestValidReplayProvider(t *testing.T) {
	p := server.NewValidReplayProvider(true)
	ch := make(chan *event.Event, 1)

	require.NoError(t, p.GC(), "unexpected GC error") // no elements, noop

	putMessages(p,
		msg(t, "hi", "", time.Millisecond, server.DefaultTopic),
		msg(t, "there", "", time.Millisecond, "t"),
		msg(t, "world", "", time.Millisecond*2, server.DefaultTopic),
		msg(t, "again", "", time.Millisecond*2, "t"),
		msg(t, "world", "", time.Millisecond*5, server.DefaultTopic),
		msg(t, "again", "", time.Millisecond*5, "t"),
	)

	time.Sleep(time.Millisecond)

	require.NoError(t, p.GC(), "unexpected GC error")

	time.Sleep(time.Millisecond)

	p.Replay(server.Subscription{
		Channel:     ch,
		LastEventID: event.MustID("3"),
		Topics:      []string{server.DefaultTopic},
	})
	testNoopReplays(p, ch)

	data := (<-ch).String()

	require.Equal(t, "id: 4\ndata: world\n\n", data)
}

func TestFiniteReplayProvider(t *testing.T) {
	p := server.NewFiniteReplayProvider(3)
	ch := make(chan *event.Event, 1)

	require.NoError(t, p.GC(), "unexpected GC error") // GC is not required, noop

	putMessages(p,
		msg(t, "", "1", 0, ""),
		msg(t, "hello", "2", 0, server.DefaultTopic),
		msg(t, "there", "3", 0, "t"),
		msg(t, "world", "4", 0, server.DefaultTopic),
	)

	p.Replay(server.Subscription{
		Channel:     ch,
		LastEventID: event.MustID("2"),
		Topics:      []string{server.DefaultTopic},
	})
	testNoopReplays(p, ch)

	data := (<-ch).String()

	require.Equal(t, "id: 4\ndata: world\n\n", data)

	putMessages(p,
		msg(t, "", "5", 0, "t"),
		msg(t, "", "6", 0, "t"),
		msg(t, "again", "7", 0, server.DefaultTopic),
	)

	p.Replay(server.Subscription{
		Channel:     ch,
		LastEventID: event.MustID("4"),
		Topics:      []string{server.DefaultTopic},
	})

	data = (<-ch).String()

	require.Equal(t, "id: 7\ndata: again\n\n", data)
}
