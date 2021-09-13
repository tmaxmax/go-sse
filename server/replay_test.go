package server_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse/server"
)

func msg(tb testing.TB, data, id string, expiry time.Duration, topic string) *server.Message {
	tb.Helper()

	e := &server.Message{Topic: topic}
	e.AppendText(data)
	e.SetTTL(expiry)
	if id != "" {
		e.SetID(server.MustEventID(id))
	}

	return e
}

func putMessages(p server.ReplayProvider, msgs ...*server.Message) {
	for i := range msgs {
		p.Put(&msgs[i])
	}
}

func testNoopReplays(p server.ReplayProvider, ch chan<- *server.Message) {
	p.Replay(server.Subscription{ // unset ID, noop
		Channel:     ch,
		LastEventID: server.EventID{},
	})
	p.Replay(server.Subscription{ // invalid ID, noop
		Channel:     ch,
		LastEventID: server.MustEventID("mama"),
	})
	p.Replay(server.Subscription{ // nonexistent ID, noop
		Channel:     ch,
		LastEventID: server.MustEventID("10"),
	})
}

func TestValidReplayProvider(t *testing.T) {
	t.Parallel()

	p := server.NewValidReplayProvider(true)
	ch := make(chan *server.Message, 2)

	require.NoError(t, p.GC(), "unexpected GC error") // no elements, noop

	exp := time.Millisecond * 5

	putMessages(p,
		msg(t, "hi", "", exp, server.DefaultTopic),
		msg(t, "there", "", exp, "t"),
		msg(t, "world", "", exp*2, server.DefaultTopic),
		msg(t, "again", "", exp*2, "t"),
		msg(t, "world", "", exp*5, server.DefaultTopic),
		msg(t, "again", "", exp*5, "t"),
	)

	time.Sleep(exp)

	require.NoError(t, p.GC(), "unexpected GC error")

	time.Sleep(exp)

	p.Replay(server.Subscription{
		Channel:     ch,
		LastEventID: server.MustEventID("3"),
		Topics:      []string{server.DefaultTopic},
	})
	testNoopReplays(p, ch)

	data := (<-ch).String()

	require.Equal(t, "id: 4\ndata: world\n\n", data)
}

func TestFiniteReplayProvider(t *testing.T) {
	t.Parallel()

	p := server.NewFiniteReplayProvider(3)
	ch := make(chan *server.Message, 1)

	require.NoError(t, p.GC(), "unexpected GC error") // GC is not required, noop

	putMessages(p,
		msg(t, "", "1", 0, ""),
		msg(t, "hello", "2", 0, server.DefaultTopic),
		msg(t, "there", "3", 0, "t"),
		msg(t, "world", "4", 0, server.DefaultTopic),
	)

	p.Replay(server.Subscription{
		Channel:     ch,
		LastEventID: server.MustEventID("2"),
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
		LastEventID: server.MustEventID("4"),
		Topics:      []string{server.DefaultTopic},
	})

	data = (<-ch).String()

	require.Equal(t, "id: 7\ndata: again\n\n", data)
}
