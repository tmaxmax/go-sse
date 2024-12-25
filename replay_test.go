package sse_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse"
	"github.com/tmaxmax/go-sse/internal/tests"
)

func replay(tb testing.TB, p sse.ReplayProvider, lastEventID sse.EventID, topics ...string) []*sse.Message {
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

func testReplayError(tb testing.TB, p sse.ReplayProvider, tm *tests.Time) {
	tb.Helper()

	tm.Reset()
	tm.Add(time.Hour)

	p.Put(msg(tb, "a", "1"), []string{sse.DefaultTopic})
	p.Put(msg(tb, "b", "2"), []string{sse.DefaultTopic})

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

	p, _ := sse.NewValidReplayProvider(ttl, true)
	p.GCInterval = 0
	p.Now = tm.Now

	tests.Equal(t, p.Replay(sse.Subscription{}), nil, "replay failed on provider without messages")

	now := time.Now()
	initialNow := now
	tm.Set(now)

	p.Put(msg(t, "hi", ""), []string{sse.DefaultTopic})
	p.Put(msg(t, "there", ""), []string{"t"})
	tm.Add(ttl)
	p.Put(msg(t, "world", ""), []string{sse.DefaultTopic})
	p.Put(msg(t, "again", ""), []string{"t"})
	tm.Add(ttl * 3)
	p.Put(msg(t, "world", ""), []string{sse.DefaultTopic})
	p.Put(msg(t, "x", ""), []string{"t"})
	tm.Add(ttl * 5)
	p.Put(msg(t, "again", ""), []string{"t"})

	tm.Set(initialNow.Add(ttl))

	p.GC()

	tm.Set(now.Add(ttl))

	replayed := replay(t, p, sse.ID("3"), sse.DefaultTopic, "topic with no messages")[0]
	tests.Equal(t, replayed.String(), "id: 4\ndata: world\n\n", "invalid message received")

	tr, err := sse.NewValidReplayProvider(time.Second, false)
	tests.Equal(t, err, nil, "replay provider should be created")

	testReplayError(t, tr, tm)
}

func TestFiniteReplayProvider(t *testing.T) {
	t.Parallel()

	_, err := sse.NewFiniteReplayProvider(1, false)
	if err == nil {
		t.Fatal("should not create FiniteReplayProvider with count less than 2")
	}

	p, err := sse.NewFiniteReplayProvider(3, false)
	tests.Equal(t, err, nil, "should create new FiniteReplayProvider")

	tests.Equal(t, p.Replay(sse.Subscription{}), nil, "replay failed on provider without messages")

	r := tests.Panics(t, func() {
		p.Put(&sse.Message{Type: sse.Type("panic")}, []string{sse.DefaultTopic})
	}, "messages without IDs cannot be put in a replay provider")
	rerr, isErr := r.(error)
	tests.Expect(t, isErr, "should panic with error")
	tests.Equal(t, rerr.Error(), `message has no ID`, "invalid error message")

	r = tests.Panics(t, func() {
		p.Put(&sse.Message{ID: sse.ID("5"), Type: sse.Type("panic")}, nil)
	}, "messages cannot be put without a topic")
	rerr, isErr = r.(error)
	tests.Expect(t, isErr, "should panic with error")
	tests.Equal(t, rerr.Error(), `go-sse: no topics provided for Message`, "invalid panic error message")

	p.Put(msg(t, "", "1"), []string{sse.DefaultTopic})
	p.Put(msg(t, "hello", "2"), []string{sse.DefaultTopic})
	p.Put(msg(t, "there", "3"), []string{"t"})
	p.Put(msg(t, "world", "4"), []string{sse.DefaultTopic})

	replayed := replay(t, p, sse.ID("2"))[0]
	tests.Equal(t, replayed.String(), "id: 4\ndata: world\n\n", "invalid replayed message")

	p.Put(msg(t, "", "5"), []string{"t"})
	p.Put(msg(t, "again", "6"), []string{sse.DefaultTopic})

	replayed = replay(t, p, sse.ID("4"), sse.DefaultTopic, "topic with no messages")[0]
	tests.Equal(t, replayed.String(), "id: 6\ndata: again\n\n", "invalid replayed message")

	tr, err := sse.NewFiniteReplayProvider(10, false)
	tests.Equal(t, err, nil, "should create new FiniteReplayProvider")

	testReplayError(t, tr, nil)
}

func TestFiniteReplayProvider_allocations(t *testing.T) {
	p, err := sse.NewFiniteReplayProvider(3, false)
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
		_ = p.Put(queue[run], topics)

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
