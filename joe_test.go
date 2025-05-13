package sse_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse"
	"github.com/tmaxmax/go-sse/internal/tests"
)

type mockReplayer struct {
	putc        chan struct{}
	replayc     chan struct{}
	shouldPanic string
}

func (m *mockReplayer) Put(msg *sse.Message, _ []string) (*sse.Message, error) {
	m.putc <- struct{}{}
	if strings.Contains(m.shouldPanic, "put") {
		panic("panicked")
	}

	return msg, nil
}

func (m *mockReplayer) Replay(_ sse.Subscription) error {
	m.replayc <- struct{}{}
	if strings.Contains(m.shouldPanic, "replay") {
		panic("panicked")
	}

	return nil
}

func (m *mockReplayer) replays() int {
	return len(m.replayc)
}

func (m *mockReplayer) puts() int {
	return len(m.putc)
}

var _ sse.Replayer = (*mockReplayer)(nil)

func newMockReplayer(shouldPanic string, numExpectedCalls int) *mockReplayer {
	return &mockReplayer{
		shouldPanic: shouldPanic,
		putc:        make(chan struct{}, numExpectedCalls),
		replayc:     make(chan struct{}, numExpectedCalls),
	}
}

func msg(tb testing.TB, data, id string) *sse.Message {
	tb.Helper()

	e := &sse.Message{}
	e.AppendData(data)
	if id != "" {
		e.ID = sse.ID(id)
	}

	return e
}

type mockClient func(m *sse.Message) error

func (c mockClient) Send(m *sse.Message) error { return c(m) }
func (c mockClient) Flush() error              { return c(nil) }

func cleanupJoe(tb testing.TB, j *sse.Joe) {
	tb.Helper()
	tb.Cleanup(func() {
		_ = j.Shutdown(context.Background())
	})
}

func TestJoe_Shutdown(t *testing.T) {
	t.Parallel()

	rp := newMockReplayer("", 0)
	j := &sse.Joe{
		Replayer: rp,
	}

	tests.Equal(t, j.Shutdown(context.Background()), nil, "joe should close successfully")
	tests.Equal(t, j.Shutdown(context.Background()), sse.ErrProviderClosed, "joe should already be closed")
	tests.Equal(t, j.Subscribe(context.Background(), sse.Subscription{}), sse.ErrProviderClosed, "no operation should be allowed on closed joe")
	tests.Equal(t, j.Publish(nil, nil), sse.ErrNoTopic, "parameter validation should happen first")
	tests.Equal(t, j.Publish(nil, []string{sse.DefaultTopic}), sse.ErrProviderClosed, "no operation should be allowed on closed joe")
	tests.Equal(t, rp.puts(), 0, "joe should not have used the replay provider")
	tests.Equal(t, rp.replays(), 0, "joe should not have used the replay provider")

	j = &sse.Joe{}
	// trigger internal initialization, so the concurrent Shutdowns aren't serialized by the internal sync.Once.
	_ = j.Publish(&sse.Message{}, []string{sse.DefaultTopic})
	//nolint
	tests.NotPanics(t, func() {
		go j.Shutdown(context.Background())
		j.Shutdown(context.Background())
	}, "concurrent shutdown should work")

	j = &sse.Joe{}
	subctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*15)
	t.Cleanup(cancel)
	go j.Subscribe(subctx, sse.Subscription{ //nolint:errcheck // we don't care about this error
		Topics: []string{sse.DefaultTopic},
		Client: mockClient(func(m *sse.Message) error {
			if m != nil {
				time.Sleep(time.Millisecond * 8)
			}
			return nil
		}),
	})
	time.Sleep(time.Millisecond)

	_ = j.Publish(&sse.Message{}, []string{sse.DefaultTopic})

	sctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*5)
	t.Cleanup(cancel)
	tests.ErrorIs(t, j.Shutdown(sctx), context.DeadlineExceeded, "shutdown should stop on closed context")

	<-subctx.Done()
}

func subscribe(t testing.TB, p sse.Provider, ctx context.Context, topics ...string) <-chan []*sse.Message { //nolint
	t.Helper()

	if len(topics) == 0 {
		topics = []string{sse.DefaultTopic}
	}

	ch := make(chan []*sse.Message, 1)

	go func() {
		defer close(ch)

		var msgs []*sse.Message

		c := mockClient(func(m *sse.Message) error {
			if m != nil {
				msgs = append(msgs, m)
			}
			return nil
		})

		_ = p.Subscribe(ctx, sse.Subscription{Client: c, Topics: topics})

		ch <- msgs
	}()

	return ch
}

type mockContext struct {
	context.Context
	waitingOnDone chan struct{}
}

func (m *mockContext) Done() <-chan struct{} {
	close(m.waitingOnDone)

	return m.Context.Done()
}

func newMockContext(tb testing.TB) (*mockContext, context.CancelFunc) {
	tb.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)

	return &mockContext{Context: ctx, waitingOnDone: make(chan struct{})}, cancel
}

func TestJoe_SubscribePublish(t *testing.T) {
	t.Parallel()

	rp := newMockReplayer("", 2)
	j := &sse.Joe{
		Replayer: rp,
	}
	cleanupJoe(t, j)

	ctx, cancel := newMockContext(t)

	sub := subscribe(t, j, ctx)
	<-ctx.waitingOnDone
	tests.Equal(t, j.Publish(msg(t, "hello", ""), []string{sse.DefaultTopic}), nil, "publish should succeed")
	cancel()
	tests.Equal(t, j.Publish(msg(t, "world", ""), []string{sse.DefaultTopic}), nil, "publish should succeed")
	msgs := <-sub
	tests.Equal(t, "data: hello\n\n", msgs[0].String(), "invalid data received")

	ctx2, _ := newMockContext(t)

	sub2 := subscribe(t, j, ctx2)
	<-ctx2.waitingOnDone

	tests.Equal(t, j.Shutdown(context.Background()), nil, "shutdown should succeed")
	msgs = <-sub2
	tests.Equal(t, len(msgs), 0, "unexpected messages received")
	tests.Equal(t, rp.puts(), 2, "invalid put calls")
	tests.Equal(t, rp.puts(), 2, "invalid replay calls")
}

func TestJoe_Subscribe_multipleTopics(t *testing.T) {
	t.Parallel()

	j := &sse.Joe{}
	cleanupJoe(t, j)

	ctx, _ := newMockContext(t)

	sub := subscribe(t, j, ctx, sse.DefaultTopic, "another topic")
	<-ctx.waitingOnDone

	_ = j.Publish(msg(t, "hello", ""), []string{sse.DefaultTopic, "another topic"})
	_ = j.Publish(msg(t, "world", ""), []string{"another topic"})

	_ = j.Shutdown(context.Background())

	msgs := <-sub

	expected := `data: hello

data: world

`
	tests.Equal(t, expected, msgs[0].String()+msgs[1].String(), "unexpected data received")
}

func TestJoe_errors(t *testing.T) {
	t.Parallel()

	fin, err := sse.NewFiniteReplayer(2, false)
	tests.Equal(t, err, nil, "should create new FiniteReplayProvider")

	j := &sse.Joe{
		Replayer: fin,
	}
	cleanupJoe(t, j)

	_ = j.Publish(msg(t, "hello", "0"), []string{sse.DefaultTopic})
	_ = j.Publish(msg(t, "hello", "1"), []string{sse.DefaultTopic})

	callErr := errors.New("artificial fail")

	var called int
	client := mockClient(func(m *sse.Message) error {
		if m != nil {
			called++
		}
		return callErr
	})

	err = j.Subscribe(context.Background(), sse.Subscription{
		Client:      client,
		LastEventID: sse.ID("0"),
		Topics:      []string{sse.DefaultTopic},
	})
	tests.Equal(t, err, callErr, "error not received from replay")

	_ = j.Publish(msg(t, "world", "2"), []string{sse.DefaultTopic})

	tests.Equal(t, called, 1, "callback was called after subscribe returned")

	called = 0
	ctx, _ := newMockContext(t)
	done := make(chan struct{})

	go func() {
		defer close(done)

		<-ctx.waitingOnDone

		_ = j.Publish(msg(t, "", "3"), []string{sse.DefaultTopic})
		_ = j.Publish(msg(t, "", "4"), []string{sse.DefaultTopic})
	}()

	err = j.Subscribe(ctx, sse.Subscription{Client: client, Topics: []string{sse.DefaultTopic}})
	tests.Equal(t, err, callErr, "error not received from send")
	// Only the first event should be attempted as nothing is replayed.
	tests.Equal(t, called, 1, "callback was called after subscribe returned")

	<-done
}

type mockMessageWriter struct {
	msg chan *sse.Message
}

func (m *mockMessageWriter) Send(msg *sse.Message) error {
	m.msg <- msg
	return nil
}

func (m *mockMessageWriter) Flush() error {
	return nil
}

func TestJoe_ReplayPanic(t *testing.T) {
	t.Parallel()

	rp := newMockReplayer("replay put", 1)
	j := &sse.Joe{Replayer: rp}
	wr := &mockMessageWriter{msg: make(chan *sse.Message, 1)}

	topics := []string{sse.DefaultTopic}
	suberr := make(chan error)
	go func() { suberr <- j.Subscribe(context.Background(), sse.Subscription{Client: wr, Topics: topics}) }()

	_, ok := <-rp.replayc
	tests.Expect(t, ok, "replay wasn't called")

	msg := &sse.Message{ID: sse.ID("hello")}
	tests.Equal(t, j.Publish(msg, topics), nil, "replay put should not be triggered by publishing anymore")
	tests.Equal(t, (<-wr.msg).ID, msg.ID, "message was not sent to client")

	go func() { _ = j.Subscribe(context.Background(), sse.Subscription{}) }()
	time.Sleep(time.Millisecond)
	tests.Equal(t, rp.replays(), 0, "replay was called")

	tests.Equal(t, j.Shutdown(context.Background()), nil, "shutdown should succeed")
	tests.Equal(t, <-suberr, sse.ErrProviderClosed, "expected subscribe error due to forceful shutdown")

	rp = newMockReplayer("put", 1)
	j = &sse.Joe{Replayer: rp}
	go func() { suberr <- j.Subscribe(context.Background(), sse.Subscription{Client: wr, Topics: topics}) }()

	_, ok = <-rp.replayc
	tests.Expect(t, ok, "replay was called")

	tests.Equal(t, j.Publish(msg, topics), nil, "replay put error should not be propagated")
	tests.Equal(t, (<-wr.msg).ID, msg.ID, "message was not sent to client")

	tests.Equal(t, j.Shutdown(context.Background()), nil, "shutdown should succeed")
	tests.Equal(t, <-suberr, sse.ErrProviderClosed, "expected subscribe error due to forceful shutdown")
}

func TestJoe_ClientContextCloseOnError(t *testing.T) {
	t.Parallel()

	ctx, cancel := newMockContext(t)

	mw := mockClient(func(m *sse.Message) error {
		if m == nil {
			cancel()
			time.Sleep(time.Millisecond)
			return errors.New("flush error")
		}

		return nil
	})

	j := &sse.Joe{}
	t.Cleanup(func() { _ = j.Shutdown(context.Background()) })

	errch := make(chan error)
	topics := []string{sse.DefaultTopic}

	go func() {
		errch <- j.Subscribe(ctx, sse.Subscription{Client: mw, Topics: topics})
	}()

	<-ctx.waitingOnDone

	tests.Equal(t, j.Publish(&sse.Message{ID: sse.ID("trigger")}, topics), nil, "unexpected publish error")
	// The error above shouldn't be propagated, as the subscriber's context is done before the replay is done.
	// Subscription should end instantly and not wait for anything else other than the unsubscription signal
	// to be sent successfully.
	tests.Equal(t, <-errch, nil, "unexpected subscribe error")
}
