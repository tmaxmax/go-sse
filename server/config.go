package server

import (
	"log"
	"time"

	"github.com/tmaxmax/go-sse/server/replay"
)

type Config struct {
	// BroadcastBufferSize is the size of the broadcast channel. If you broadcast messages faster than the handler
	// can process them (calling Handler.Broadcast blocks for too long), provide a bigger size. The default is 1.
	BroadcastBufferSize int
	// ConnectionBufferSize is the size of each connection's events channel. The commands are handled
	// sequentially, which means that if a connection can't write messages faster than they're
	// received all the other commands must wait (other connects, disconnects, broadcasts).
	// In that case you should provide a bigger value. The default is 0 (unbuffered).
	ConnectionBufferSize int
	// OnWriteError is a function that's called when writing an event to the connection fails.
	// Defaults to log.Println, you can use it for custom logging or any other desired error handling.
	OnWriteError func(error)
	// ReplayProvider is the desired replay implementation. See the replay package for documentation on
	// each provider.
	ReplayProvider replay.Provider
	// ReplayGCInterval is the interval at which the replay buffer should be cleaned up to release resources.
	// Some providers might not require cleanup, see their documentation.
	ReplayGCInterval time.Duration
}

var defaultConfig = Config{
	BroadcastBufferSize:  1,
	ConnectionBufferSize: 0,
	OnWriteError: func(err error) {
		log.Println("go-sse.server.Handler: write error:", err)
	},
	ReplayProvider:   replay.Noop{},
	ReplayGCInterval: 0,
}

func mergeWithDefault(c *Config) {
	if c.BroadcastBufferSize <= 0 {
		c.BroadcastBufferSize = defaultConfig.BroadcastBufferSize
	}
	if c.ConnectionBufferSize < 0 {
		c.ConnectionBufferSize = defaultConfig.ConnectionBufferSize
	}
	if c.OnWriteError == nil {
		c.OnWriteError = defaultConfig.OnWriteError
	}
	if c.ReplayProvider == nil {
		c.ReplayProvider = defaultConfig.ReplayProvider
	}
	if c.ReplayGCInterval < 0 {
		c.ReplayGCInterval = defaultConfig.ReplayGCInterval
	}
}
