package server

import (
	"time"
)

// topics returns a slice containing the default topic if no topics are given.
// It returns the given slice otherwise.
func topics(topics []string) []string {
	if len(topics) == 0 {
		return []string{DefaultTopic}
	}
	return topics
}

func noop() {}

// ticker creates a time.Ticker, if duration is positive, and returns its channel and stop function.
// If the duration is negative, it returns a nil channel and a noop function.
func ticker(duration time.Duration) (<-chan time.Time, func()) {
	if duration <= 0 {
		return nil, noop
	}
	t := time.NewTicker(duration)
	return t.C, t.Stop
}

// joeConfig takes the NewJoe function's input and returns a valid configuration.
func joeConfig(input []JoeConfig) JoeConfig {
	cfg := JoeConfig{}
	if len(input) > 0 {
		cfg = input[0]
	}

	if cfg.MessageChannelBuffer <= 0 {
		cfg.MessageChannelBuffer = 1
	}
	if cfg.ReplayProvider == nil {
		if cfg.ReplayGCInterval > 0 {
			cfg.ReplayGCInterval = 0
		}
		cfg.ReplayProvider = noopReplayProvider{}
	}

	return cfg
}

// hasTopic returns true if the given topic is inside the given topics slice.
func hasTopic(topics []string, topic string) bool {
	for _, t := range topics {
		if topic == t {
			return true
		}
	}
	return false
}
