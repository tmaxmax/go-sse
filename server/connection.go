package server

import (
	"errors"
	"io"
	"net/http"

	"github.com/tmaxmax/go-sse/server/event"
)

type writeFlusher interface {
	io.Writer
	http.Flusher
}

// A Connection is an upgraded request. Use it to send events to the client.
type Connection struct {
	w writeFlusher
}

// Send sends the given event to the client. It returns any errors that occurred while writing the event
func (c *Connection) Send(e *event.Event) error {
	_, err := e.WriteTo(c.w)
	c.w.Flush()
	return err
}

// NewConnection upgrades an HTTP request to support server-sent events.
// It returns a Connection that's used to send events to the client or an
// error if the upgrade failed.
func NewConnection(w http.ResponseWriter) (*Connection, error) {
	fw, ok := w.(writeFlusher)
	if !ok {
		return nil, ErrUnsupported
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	fw.Flush()

	return &Connection{w: fw}, nil
}

// ErrUnsupported is returned when a request can't be upgraded to support server-sent events.
var ErrUnsupported = errors.New("go-sse.server.connection: unsupported")
