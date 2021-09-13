package server_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse/server"
)

type mockReplayProvider struct {
	errGC       error
	callsGC     int
	callsPut    int
	callsReplay int
}

func (m *mockReplayProvider) Put(_ **server.Message) {
	m.callsPut++
}

func (m *mockReplayProvider) Replay(_ server.Subscription) {
	m.callsReplay++
}

func (m *mockReplayProvider) GC() error {
	m.callsGC++
	return m.errGC
}

var _ server.ReplayProvider = (*mockReplayProvider)(nil)

func TestNewJoe(t *testing.T) {
	rp := &mockReplayProvider{errGC: errors.New("")}
	j := server.NewJoe(server.JoeConfig{
		ReplayProvider:   rp,
		ReplayGCInterval: time.Millisecond,
	})

	time.Sleep(time.Millisecond * 2)

	require.NoError(t, j.Stop())
	require.Equal(t, rp.callsGC, 1)

	//nolint
	require.NotPanics(t, func() {
		s := server.NewJoe(server.JoeConfig{
			ReplayGCInterval: -5,
		})
		defer s.Stop()
		t := server.NewJoe(server.JoeConfig{
			ReplayGCInterval: 5,
		})
		defer t.Stop()
	})
}

func TestJoe_Stop(t *testing.T) {
	rp := &mockReplayProvider{errGC: errors.New("")}
	j := server.NewJoe(server.JoeConfig{
		ReplayProvider: rp,
	})

	require.NoError(t, j.Stop())
	require.ErrorIs(t, j.Stop(), server.ErrProviderClosed)
	require.ErrorIs(t, j.Subscribe(context.Background(), server.Subscription{}), server.ErrProviderClosed)
	require.ErrorIs(t, j.Publish(nil), server.ErrProviderClosed)
	require.Zero(t, rp.callsPut)
	require.Zero(t, rp.callsReplay)
	require.Zero(t, rp.callsGC)

	j = server.NewJoe()
	//nolint
	require.NotPanics(t, func() {
		go j.Stop()
		j.Stop()
	})
}

func TestJoe_SubscribePublish(t *testing.T) {
	rp := &mockReplayProvider{errGC: errors.New("")}
	j := server.NewJoe(server.JoeConfig{
		ReplayProvider: rp,
	})

	ch := make(chan *server.Message, 1)
	sub := server.Subscription{Channel: ch}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, j.Subscribe(ctx, sub))
	require.NoError(t, j.Subscribe(ctx, sub)) // same subscriber, noop
	require.NoError(t, j.Publish(msg(t, "hello", "", 0, server.DefaultTopic)))
	cancel()
	time.Sleep(time.Millisecond)
	require.NoError(t, j.Publish(msg(t, "world", "", 0, server.DefaultTopic)))
	require.Equal(t, "data: hello\n\n", (<-ch).String())
	_, ok := <-ch
	require.False(t, ok)

	ch2 := make(chan *server.Message)
	sub2 := server.Subscription{Channel: ch2}

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
	j := server.NewJoe(server.JoeConfig{
		ReplayProvider:   rp,
		ReplayGCInterval: interval,
	})

	expected := 2

	time.Sleep(interval*time.Duration(expected) + interval/2)
	require.NoError(t, j.Stop())
	require.Equal(t, expected, rp.callsGC)
}
