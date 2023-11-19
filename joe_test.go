package sse_test

import (
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse"
)

type mockReplayProvider struct {
	errGC       error
	shouldPanic string
	callsGC     int
	callsPut    int
	callsReplay int
}

func (m *mockReplayProvider) Put(msg *sse.Message, _ []string) *sse.Message {
	m.callsPut++
	if strings.Contains(m.shouldPanic, "put") {
		panic("panicked")
	}

	return msg
}

func (m *mockReplayProvider) Replay(_ sse.Subscription) error {
	m.callsReplay++
	if strings.Contains(m.shouldPanic, "replay") {
		panic("panicked")
	}

	return nil
}

func (m *mockReplayProvider) GC() error {
	m.callsGC++
	if strings.Contains(m.shouldPanic, "gc") {
		panic("panicked")
	}

	return m.errGC
}

var _ sse.ReplayProvider = (*mockReplayProvider)(nil)

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

func TestJoe_Shutdown(t *testing.T) {
	t.Parallel()

	rp := &mockReplayProvider{errGC: errors.New("")}
	j := &sse.Joe{
		ReplayProvider: rp,
	}

	require.NoError(t, j.Shutdown(context.Background()))
	require.ErrorIs(t, j.Shutdown(context.Background()), sse.ErrProviderClosed)
	require.ErrorIs(t, j.Subscribe(context.Background(), sse.Subscription{}), sse.ErrProviderClosed)
	require.ErrorIs(t, j.Publish(nil, nil), sse.ErrNoTopic)
	require.ErrorIs(t, j.Publish(nil, []string{sse.DefaultTopic}), sse.ErrProviderClosed)
	require.Zero(t, rp.callsPut)
	require.Zero(t, rp.callsReplay)
	require.Zero(t, rp.callsGC)

	j = &sse.Joe{}
	// trigger internal initialization, so the concurrent Shutdowns aren't serialized by the internal sync.Once.
	_ = j.Publish(&sse.Message{}, []string{sse.DefaultTopic})
	//nolint
	require.NotPanics(t, func() {
		go j.Shutdown(context.Background())
		j.Shutdown(context.Background())
	})

	log.Println()

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
	require.ErrorIs(t, j.Shutdown(sctx), context.DeadlineExceeded)

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

	return &mockContext{Context: ctx, waitingOnDone: make(chan struct{})}, cancel
}

func TestJoe_SubscribePublish(t *testing.T) {
	t.Parallel()

	rp := &mockReplayProvider{errGC: errors.New("")}
	j := &sse.Joe{
		ReplayProvider: rp,
	}
	defer j.Shutdown(context.Background()) //nolint:errcheck // irrelevant

	ctx, cancel := newMockContext(t)
	defer cancel()

	sub := subscribe(t, j, ctx)
	<-ctx.waitingOnDone
	require.NoError(t, j.Publish(msg(t, "hello", ""), []string{sse.DefaultTopic}))
	cancel()
	require.NoError(t, j.Publish(msg(t, "world", ""), []string{sse.DefaultTopic}))
	msgs := <-sub
	require.Equal(t, "data: hello\n\n", msgs[0].String())

	ctx2, cancel2 := newMockContext(t)
	defer cancel2()

	sub2 := subscribe(t, j, ctx2)
	<-ctx2.waitingOnDone

	require.NoError(t, j.Shutdown(context.Background()))
	msgs = <-sub2
	require.Zero(t, msgs, "unexpected messages received")
	require.Equal(t, 2, rp.callsPut, "invalid put calls")
	require.Equal(t, 2, rp.callsReplay, "invalid replay calls")
}

func TestJoe_Subscribe_multipleTopics(t *testing.T) {
	t.Parallel()

	j := &sse.Joe{}
	defer j.Shutdown(context.Background()) //nolint:errcheck // irrelevant

	ctx, cancel := newMockContext(t)
	defer cancel()

	sub := subscribe(t, j, ctx, sse.DefaultTopic, "another topic")
	<-ctx.waitingOnDone

	_ = j.Publish(msg(t, "hello", ""), []string{sse.DefaultTopic, "another topic"})
	_ = j.Publish(msg(t, "world", ""), []string{"another topic"})

	_ = j.Shutdown(context.Background())

	msgs := <-sub

	expected := `data: hello

data: world

`
	require.Equal(t, expected, msgs[0].String()+msgs[1].String(), "unexpected data received")
}

func TestJoe_errors(t *testing.T) {
	t.Parallel()

	j := &sse.Joe{
		ReplayProvider: &sse.FiniteReplayProvider{Count: 1},
	}
	defer j.Shutdown(context.Background()) //nolint:errcheck // irrelevant

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

	err := j.Subscribe(context.Background(), sse.Subscription{
		Client:      client,
		LastEventID: sse.ID("0"),
		Topics:      []string{sse.DefaultTopic},
	})
	require.Equal(t, callErr, err, "error not received from replay")

	_ = j.Publish(msg(t, "world", "2"), []string{sse.DefaultTopic})

	require.Equal(t, 1, called, "callback was called after subscribe returned")

	called = 0
	ctx, cancel := newMockContext(t)
	defer cancel()
	done := make(chan struct{})

	go func() {
		defer close(done)

		<-ctx.waitingOnDone

		_ = j.Publish(msg(t, "", "3"), []string{sse.DefaultTopic})
		_ = j.Publish(msg(t, "", "4"), []string{sse.DefaultTopic})
	}()

	err = j.Subscribe(ctx, sse.Subscription{Client: client, Topics: []string{sse.DefaultTopic}})
	require.Equal(t, callErr, err, "error not received from send")
	require.Equal(t, 1, called, "callback was called after subscribe returned")

	<-done
}

func TestJoe_GCInterval(t *testing.T) {
	t.Parallel()

	rp := &mockReplayProvider{}
	interval := time.Millisecond * 6
	j := &sse.Joe{
		ReplayProvider:   rp,
		ReplayGCInterval: interval,
	}
	// trigger internal initialization, so GC is started.
	_ = j.Publish(&sse.Message{}, []string{sse.DefaultTopic})

	expected := 2

	time.Sleep(interval*time.Duration(expected) + interval/2)
	require.NoError(t, j.Shutdown(context.Background()))
	require.Equal(t, expected, rp.callsGC)
}

func TestJoe_ReplayPanic(t *testing.T) {
	t.Parallel()

	rp := &mockReplayProvider{shouldPanic: "replay"}
	j := &sse.Joe{ReplayProvider: rp}

	err := j.Subscribe(context.Background(), sse.Subscription{})
	time.Sleep(time.Millisecond)
	require.Equal(t, 1, rp.callsReplay, "replay wasn't called")
	require.ErrorIs(t, err, sse.ErrReplayFailed, "wrong error returned")

	go func() { _ = j.Subscribe(context.Background(), sse.Subscription{}) }()
	time.Sleep(time.Millisecond)
	require.Equal(t, 1, rp.callsReplay, "replay was called")

	require.NoError(t, j.Shutdown(context.Background()))
}
