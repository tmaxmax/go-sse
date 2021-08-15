package hub

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

type writeFlusher interface {
	io.Writer
	http.Flusher
}

// A Connection is a link between a client and the server.
// It can attach to multiple Hub instances in order to receive from
// or send broadcasts to.
type Connection struct {
	messages        chan Messager
	done            chan struct{}
	hubs            map[*Hub]bool // true if the hub supports broadcasts from connections
	deferredAttachs map[*Hub]struct{}

	receiving bool
	mu        sync.RWMutex

	// Additional data hubs use in their handlers.
	Info interface{}
	// Messages are written to this writer. If the passed concrete value
	// also satisfies http.Flusher, Flush will be called too.
	// The errors returned by Receive are only writing errors. If this writer
	// doesn't error (for example bytes.Buffer), the value returned from Receive
	// can safely be ignored.
	Writer io.Writer
}

type noopFlusherWriter struct {
	io.Writer
}

func (n *noopFlusherWriter) Flush() {}

func toConnectionWriter(w io.Writer) writeFlusher {
	if cw, ok := w.(writeFlusher); ok {
		return cw
	}

	return &noopFlusherWriter{w}
}

func (c *Connection) tryDeferAttach(h *Hub) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.receiving {
		return false
	}

	if c.deferredAttachs == nil {
		c.deferredAttachs = make(map[*Hub]struct{})
	}
	c.deferredAttachs[h] = struct{}{}

	return true
}

func (c *Connection) sendAttachRequest(h *Hub) bool {
	supportsBcs := make(chan bool, 1)

	h.attach <- attachment{
		connection:                        c,
		supportsBroadcastsFromConnections: supportsBcs,
	}

	return <-supportsBcs

}

// AttachTo attaches the connection to a hub.
//
// The info parameter can be set to custom data about the client.
// You can use it to associate the client with useful information about itself,
// such as the time it has connected, its IP address or any other data deemed useful.
// Access this field in any of Hub's lifecycle methods to implement powerful
// custom logic.
//
// If AttachTo is called before Receive, the attachment itself is deferred to the
// first Receive call.
func (c *Connection) AttachTo(h *Hub) {
	if c.tryDeferAttach(h) {
		return
	}

	supportsBroadcasts := c.sendAttachRequest(h)

	c.mu.Lock()
	c.hubs[h] = supportsBroadcasts
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

	if len(c.hubs) == 0 && c.receiving {
		c.closeUnsafe()
	}
}

func (c *Connection) tryRemoveDeferredDetach(h *Hub) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.receiving {
		return false
	}

	delete(c.deferredAttachs, h)

	return true
}

// DetachFrom detaches from a hub, if the connection is attached to it.
// If after the detachment the connection is attached to no hubs,
// it is closed completely. It does nothing if the connection isn't
// attached to the given hub.
// Calling DetachFrom before Receive attempt to remove the deferred attachments
// of the given hub, if it exists.
func (c *Connection) DetachFrom(h *Hub) {
	if c.tryRemoveDeferredDetach(h) {
		return
	}

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

// BroadcastTo tries to send the given message to a hub, if the connection is attached
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
	if !c.receiving {
		return
	}

	close(c.done)
	c.receiving = false
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

// Done returns a channel that blocks until the connection is closed.
func (c *Connection) Done() <-chan struct{} {
	return c.done
}

func (c *Connection) Information() interface{} {
	return c.Info
}

func (c *Connection) init() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.receiving {
		return false
	}

	c.hubs = make(map[*Hub]bool)
	c.messages = make(chan Messager)
	c.done = make(chan struct{})
	c.receiving = true

	for h := range c.deferredAttachs {
		c.hubs[h] = c.sendAttachRequest(h)

		delete(c.deferredAttachs, h)
	}

	return true
}

// Receive starts accepting and processing messages. Calling it multiple times does nothing.
func (c *Connection) Receive() error {
	if !c.init() {
		return nil
	}

	var (
		err error
		cw  = toConnectionWriter(c.Writer)
	)

	for {
		select {
		case message := <-c.messages:
			if err = message.Message(cw); err != nil {
				c.Close()
			} else {
				cw.Flush()
			}
		case <-c.done:
			close(c.messages)

			return err
		}
	}
}
