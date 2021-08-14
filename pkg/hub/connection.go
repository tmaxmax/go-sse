package hub

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

type writer interface {
	io.Writer
	http.Flusher
}

// A Connection is a link between a client and the server.
// It can attach to multiple Hub instances in order to receive from
// or send broadcasts to.
type Connection struct {
	messages chan Messager
	done     chan struct{}
	writer   writer
	hubs     map[*Hub]bool // true if the hub supports broadcasts from connections

	closed bool
	mu     sync.RWMutex
}

type noopFlusherWriter struct {
	io.Writer
}

func (n *noopFlusherWriter) Flush() {}

func toConnectionWriter(w io.Writer) writer {
	if cw, ok := w.(writer); ok {
		return cw
	}

	return &noopFlusherWriter{w}
}

// NewConnection creates a connection that writes to the given writer.
// AttachTo it to a hub in order to Receive broadcasts.
//
// If the given writer also implements http.Flusher, Flush will be called
// automatically after writes.
func NewConnection(w io.Writer) *Connection {
	return &Connection{
		messages: make(chan Messager),
		writer:   toConnectionWriter(w),
		done:     make(chan struct{}),
		hubs:     make(map[*Hub]bool),
	}
}

// AttachTo attaches the connection to a hub.
//
// The info parameter can be set to custom data about the client.
// You can use it to associate the client with useful information about itself,
// such as the time it has connected, its IP address or any other data deemed useful.
// Access this field in any of Hub's lifecycle methods to implement powerful
// custom logic.
func (c *Connection) AttachTo(h *Hub, info interface{}) {
	supportsBcs := make(chan bool, 1)

	h.attach <- attachment{
		connection:                        c,
		information:                       info,
		supportsBroadcastsFromConnections: supportsBcs,
	}

	status := <-supportsBcs

	c.mu.Lock()
	c.hubs[h] = status
	c.mu.Unlock()
}

// detachFromUnsafe is used by Hub when it's shutting down, because
// the connection can't know to detach by itself.
func (c *Connection) detachFromUnsafe(h *Hub, requestFromHub bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !requestFromHub {
		if _, attachedTo := c.hubs[h]; attachedTo {
			h.detach <- c
		}
	}

	delete(c.hubs, h)

	if len(c.hubs) == 0 {
		c.closeUnsafe()
	}
}

// DetachFrom detaches from a hub, if the connection is attached to it.
// If after the detachment the connection is attached to no hubs,
// it is closed completely. It does nothing if the connection isn't
// attached to the given hub.
func (c *Connection) DetachFrom(h *Hub) {
	c.detachFromUnsafe(h, false)
}

func after(timeout ...time.Duration) <-chan time.Time {
	if len(timeout) == 0 || timeout[0] == 0 {
		return nil
	}

	return time.After(timeout[0])
}

// Send a message to the connection. Optionally add a timeout to control
// how long this method can block before returning.
// It returns a flag indicating whether the message was successfully sent.
// If it is false, it means that the connection is closed.
func (c *Connection) Send(message Messager, timeout ...time.Duration) bool {
	select {
	case <-c.done:
		return false
	default:
		// these cases aren't in the select above, so in the case that
		// done is ready and any other channels are the select case isn't
		// picked at random.
		select {
		case c.messages <- message:
			return true
		case <-after(timeout...):
			return false
		}
	}
}

var (
	ErrNotAttachedToHub     = errors.New("not attached to hub")
	ErrHubDoesNotSupportBcs = errors.New("hub does not support broadcasts from connections")
)

// BroadcastTo sends the given message to a hub, if the connection is attached
// to it and the hub supports broadcasts from connections. It returns an error otherwise.
func (c *Connection) BroadcastTo(h *Hub, message Messager) error {
	c.mu.RLock()
	supportsBcs, attachedTo := c.hubs[h]
	c.mu.RUnlock()

	if !attachedTo {
		return ErrNotAttachedToHub
	}
	if !supportsBcs {
		return ErrHubDoesNotSupportBcs
	}

	h.broadcastFromConn <- connectionMessage{c, message}

	return nil
}

// closeUnsafe closes the connection without locking or detaching from hubs.
func (c *Connection) closeUnsafe() {
	if c.closed {
		return
	}

	close(c.done)
	c.closed = true
}

// Close closes the connection. It detaches from all the hubs it's attached to
// and it signals it receives no more messages.
// Calling Close multiple times will do nothing.
func (c *Connection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for h := range c.hubs {
		c.DetachFrom(h)

		delete(c.hubs, h)
	}

	c.closeUnsafe()
}

// Closed returns a channel that blocks until the connection is closed.
func (c *Connection) Closed() <-chan struct{} {
	return c.done
}

// Receive starts accepting messages from
func (c *Connection) Receive() error {
	var err error

	for {
		select {
		case message := <-c.messages:
			if err = message.Message(c.writer); err != nil {
				c.Close()
			} else {
				c.writer.Flush()
			}
		case <-c.done:
			close(c.messages)

			return err
		}
	}
}
