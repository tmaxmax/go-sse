package sse_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse"
)

type mockReplayProvider struct {
	errGC       error
	callsGC     int
	callsPut    int
	callsReplay int
}

func (m *mockReplayProvider) Put(_ **sse.Message) {
	m.callsPut++
}

func (m *mockReplayProvider) Replay(_ sse.Subscription) error {
	m.callsReplay++
	return nil
}

func (m *mockReplayProvider) GC() error {
	m.callsGC++
	return m.errGC
}

var _ sse.ReplayProvider = (*mockReplayProvider)(nil)

func TestNewJoe(t *testing.T) {
	rp := &mockReplayProvider{errGC: errors.New("")}
	j := sse.NewJoe(sse.JoeConfig{
		ReplayProvider:   rp,
		ReplayGCInterval: time.Millisecond,
	})

	time.Sleep(time.Millisecond * 2)

	require.NoError(t, j.Stop())
	require.Equal(t, rp.callsGC, 1)

	//nolint
	require.NotPanics(t, func() {
		s := sse.NewJoe(sse.JoeConfig{
			ReplayGCInterval: -5,
		})
		defer s.Stop()
		t := sse.NewJoe(sse.JoeConfig{
			ReplayGCInterval: 5,
		})
		defer t.Stop()
	})
}

func TestJoe_Stop(t *testing.T) {
	rp := &mockReplayProvider{errGC: errors.New("")}
	j := sse.NewJoe(sse.JoeConfig{
		ReplayProvider: rp,
	})

	require.NoError(t, j.Stop())
	require.ErrorIs(t, j.Stop(), sse.ErrProviderClosed)
	require.ErrorIs(t, j.Subscribe(context.Background(), sse.Subscription{}), sse.ErrProviderClosed)
	require.ErrorIs(t, j.Publish(nil), sse.ErrProviderClosed)
	require.Zero(t, rp.callsPut)
	require.Zero(t, rp.callsReplay)
	require.Zero(t, rp.callsGC)

	j = sse.NewJoe()
	//nolint
	require.NotPanics(t, func() {
		go j.Stop()
		j.Stop()
	})
}

// TODO: Fix tests

func subscribe(t testing.TB, p sse.Provider, ctx context.Context) <-chan []*sse.Message {
	t.Helper()

	ch := make(chan []*sse.Message, 1)

	go func() {
		defer close(ch)

		var msgs []*sse.Message

		fn := func(m *sse.Message) error {
			msgs = append(msgs, m)
			return nil
		}

		_ = p.Subscribe(ctx, sse.Subscription{Callback: fn})

		ch <- msgs
	}()

	return ch
}

func TestJoe_SubscribePublish(t *testing.T) {
	t.SkipNow()

	rp := &mockReplayProvider{errGC: errors.New("")}
	j := sse.NewJoe(sse.JoeConfig{
		ReplayProvider: rp,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := subscribe(t, j, ctx)
	time.Sleep(time.Millisecond * 10)

	require.NoError(t, j.Publish(msg(t, "hello", "", 0, sse.DefaultTopic)))
	time.Sleep(time.Millisecond * 10)
	cancel()
	require.NoError(t, j.Publish(msg(t, "world", "", 0, sse.DefaultTopic)))
	msgs, ok := <-sub
	require.Equal(t, "data: hello\n\n", msgs[0].String())

	sub2 := subscribe(t, j, ctx)

	require.NoError(t, j.Stop())
	_, ok = <-sub2
	require.False(t, ok)
	require.Equal(t, 2, rp.callsPut)
	require.Equal(t, 2, rp.callsReplay)
}

func TestJoe_GCInterval(t *testing.T) {
	rp := &mockReplayProvider{}
	interval := time.Millisecond * 6
	j := sse.NewJoe(sse.JoeConfig{
		ReplayProvider:   rp,
		ReplayGCInterval: interval,
	})

	expected := 2

	time.Sleep(interval*time.Duration(expected) + interval/2)
	require.NoError(t, j.Stop())
	require.Equal(t, expected, rp.callsGC)
}
