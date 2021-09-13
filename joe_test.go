package sse_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse"

	"github.com/stretchr/testify/require"
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

func (m *mockReplayProvider) Replay(_ sse.Subscription) {
	m.callsReplay++
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

func TestJoe_SubscribePublish(t *testing.T) {
	rp := &mockReplayProvider{errGC: errors.New("")}
	j := sse.NewJoe(sse.JoeConfig{
		ReplayProvider: rp,
	})

	ch := make(chan *sse.Message, 1)
	sub := sse.Subscription{Channel: ch}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, j.Subscribe(ctx, sub))
	require.NoError(t, j.Subscribe(ctx, sub)) // same subscriber, noop
	require.NoError(t, j.Publish(msg(t, "hello", "", 0, sse.DefaultTopic)))
	cancel()
	time.Sleep(time.Millisecond)
	require.NoError(t, j.Publish(msg(t, "world", "", 0, sse.DefaultTopic)))
	require.Equal(t, "data: hello\n\n", (<-ch).String())
	_, ok := <-ch
	require.False(t, ok)

	ch2 := make(chan *sse.Message)
	sub2 := sse.Subscription{Channel: ch2}

	require.NoError(t, j.Subscribe(context.Background(), sub2))
	require.NoError(t, j.Stop())
	_, ok = <-ch2
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
