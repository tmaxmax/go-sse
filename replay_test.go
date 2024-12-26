package sse_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse"
	"github.com/tmaxmax/go-sse/internal/tests"
)

func replay(tb testing.TB, p sse.Replayer, lastEventID sse.EventID, topics ...string) []*sse.Message {
	tb.Helper()

	if len(topics) == 0 {
		topics = []string{sse.DefaultTopic}
	}

	var replayed []*sse.Message
	cb := mockClient(func(m *sse.Message) error {
		if m != nil {
			replayed = append(replayed, m)
		}
		return nil
	})

	sub := sse.Subscription{
		Client:      cb,
		LastEventID: lastEventID,
		Topics:      topics,
	}

	_ = p.Replay(sub)

	sub.LastEventID = sse.EventID{}
	_ = p.Replay(sub)

	sub.LastEventID = sse.ID("mama")
	_ = p.Replay(sub)

	sub.LastEventID = sse.ID("10")
	_ = p.Replay(sub)

	return replayed
}

func put(tb testing.TB, p sse.Replayer, msg *sse.Message, topics ...string) *sse.Message {
	tb.Helper()

	if len(topics) == 0 {
		topics = []string{sse.DefaultTopic}
	}

	msg, err := p.Put(msg, topics)
	tests.Equal(tb, err, nil, "invalid message")

	return msg
}

func testReplayError(tb testing.TB, p sse.Replayer, tm *tests.Time) {
	tb.Helper()

	tm.Reset()
	tm.Add(time.Hour)

	put(tb, p, msg(tb, "a", "1"))
	put(tb, p, msg(tb, "b", "2"))

	cb := mockClient(func(_ *sse.Message) error { return nil })

	tm.Rewind()

	err := p.Replay(sse.Subscription{
		Client:      cb,
		LastEventID: sse.ID("1"),
		Topics:      []string{sse.DefaultTopic},
	})

	tests.Equal(tb, err, nil, "received invalid error")
}

func TestValidReplayProvider(t *testing.T) {
	t.Parallel()

	tm := &tests.Time{}
	ttl := time.Millisecond * 5

	_, err := sse.NewValidReplayer(0, false)
	tests.Expect(t, err != nil, "replay provider cannot be created with zero or negative TTL")

	p, _ := sse.NewValidReplayer(ttl, true)
	p.GCInterval = 0
	p.Now = tm.Now

	tests.Equal(t, p.Replay(sse.Subscription{}), nil, "replay failed on provider without messages")

	now := time.Now()
	tm.Set(now)

	put(t, p, msg(t, "hi", ""))
	put(t, p, msg(t, "there", ""), "t")
	tm.Add(ttl)
	put(t, p, msg(t, "world", ""))
	put(t, p, msg(t, "again", ""), "t")
	tm.Add(ttl * 3)
	put(t, p, msg(t, "world", ""))
	put(t, p, msg(t, "x", ""), "t")
	tm.Add(ttl * 5)
	put(t, p, msg(t, "again", ""), "t")

	tm.Set(now.Add(ttl))

	p.GC()

	replayed := replay(t, p, sse.ID("3"), sse.DefaultTopic, "topic with no messages")[0]
	tests.Equal(t, replayed.String(), "id: 4\ndata: world\n\n", "invalid message received")

	p.GCInterval = ttl / 5
	// Should trigger automatic GC which should clean up most of the messages.
	tm.Set(now.Add(ttl * 5))
	put(t, p, msg(t, "not again", ""), "t")

	allReplayed := replay(t, p, sse.ID("3"), "t", "topic with no messages")
	tests.Equal(t, len(allReplayed), 2, "there should be two messages in topic 't'")
	tests.Equal(t, allReplayed[0].String(), "id: 6\ndata: again\n\n", "invalid message received")

	tr, err := sse.NewValidReplayer(time.Second, false)
	tests.Equal(t, err, nil, "replay provider should be created")

	testReplayError(t, tr, tm)
}

func TestFiniteReplayProvider(t *testing.T) {
	t.Parallel()

	_, err := sse.NewFiniteReplayer(1, false)
	tests.Expect(t, err != nil, "should not create FiniteReplayProvider with count less than 2")

	p, err := sse.NewFiniteReplayer(3, false)
	tests.Equal(t, err, nil, "should create new FiniteReplayProvider")

	tests.Equal(t, p.Replay(sse.Subscription{}), nil, "replay failed on provider without messages")

	_, err = p.Put(msg(t, "panic", ""), []string{sse.DefaultTopic})
	tests.Expect(t, err != nil, "message without IDs cannot be put in a replay provider")

	_, err = p.Put(msg(t, "panic", "5"), nil)
	tests.ErrorIs(t, err, sse.ErrNoTopic, "incorrect error returned when no topic is provided")

	put(t, p, msg(t, "", "1"))
	put(t, p, msg(t, "hello", "2"))
	put(t, p, msg(t, "there", "3"), "t")
	put(t, p, msg(t, "world", "4"))

	replayed := replay(t, p, sse.ID("2"))[0]
	tests.Equal(t, replayed.String(), "id: 4\ndata: world\n\n", "invalid replayed message")

	put(t, p, msg(t, "", "5"), "t")
	put(t, p, msg(t, "again", "6"))

	replayed = replay(t, p, sse.ID("4"), sse.DefaultTopic, "topic with no messages")[0]
	tests.Equal(t, replayed.String(), "id: 6\ndata: again\n\n", "invalid replayed message")

	idp, err := sse.NewFiniteReplayer(10, true)
	tests.Equal(t, err, nil, "should create new FiniteReplayProvider")

	_, err = idp.Put(msg(t, "should error", "should not have ID"), []string{sse.DefaultTopic})
	tests.Expect(t, err != nil, "messages with IDs cannot be put in an autoID replay provider")

	tr, err := sse.NewFiniteReplayer(10, false)
	tests.Equal(t, err, nil, "should create new FiniteReplayProvider")

	testReplayError(t, tr, nil)
}

func TestFiniteReplayProvider_allocations(t *testing.T) {
	p, err := sse.NewFiniteReplayer(3, false)
	tests.Equal(t, err, nil, "should create new FiniteReplayProvider")

	const runs = 100

	topics := []string{sse.DefaultTopic}
	// Add one to the number of runs to take the warmup run of
	// AllocsPerRun() into account.
	queue := make([]*sse.Message, runs+1)
	lastID := runs

	for i := 0; i < len(queue); i++ {
		queue[i] = msg(t,
			fmt.Sprintf("message %d", i),
			strconv.Itoa(i),
		)
	}

	var run int

	avgAllocs := testing.AllocsPerRun(runs, func() {
		put(t, p, queue[run], topics...)

		run++
	})

	tests.Equal(t, avgAllocs, 0, "no allocations should be made on Put()")

	var replayCount int

	cb := mockClient(func(m *sse.Message) error {
		if m != nil {
			replayCount++
		}

		return nil
	})

	sub := sse.Subscription{
		Client: cb,
		Topics: topics,
	}

	sub.LastEventID = sse.ID(strconv.Itoa(lastID - 3))

	err = p.Replay(sub)
	tests.Equal(t, err, nil, "replay from fourth last should succeed")

	tests.Equal(t, replayCount, 0, "replay from fourth last should not yield messages")

	sub.LastEventID = sse.ID(strconv.Itoa(lastID - 2))

	err = p.Replay(sub)
	tests.Equal(t, err, nil, "replay from third last should succeed")

	tests.Equal(t, replayCount, 2, "replay from third last should yield 2 messages")
}
