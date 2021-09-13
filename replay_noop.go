package sse

import "errors"

// noopReplayProvider is the default replay provider used if none is given. It does nothing.
// It is used to avoid nil checks for the provider each time it is used.
type noopReplayProvider struct{}

func (n noopReplayProvider) Put(_ **Message)       {}
func (n noopReplayProvider) Replay(_ Subscription) {}
func (n noopReplayProvider) GC() error {
	// We return an error here so server providers know to stop triggering GC operations on this handler.
	return errors.New("noopReplayProvider: gc is a noop")
}

var _ ReplayProvider = (*noopReplayProvider)(nil)
