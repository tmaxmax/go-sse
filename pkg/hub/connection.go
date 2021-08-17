package hub

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// A Conn is a link (connection) between a client and the server.
// It can attach to multiple Hub instances in order to receive from
// or send broadcasts to.
type Conn struct {
	messages        chan interface{}
	hubs            map[*Hub]struct{}
	deferredAttachs map[*Hub]struct{}

	receiving bool
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex

	Log                *log.Logger
	MessagesBufferSize int
}

func (c *Conn) logf(format string, args ...interface{}) {
	if c.Log != nil {
		c.Log.Printf(format, args...)
	}
}

func (c *Conn) unsafeTryDeferAttach(h *Hub) bool {
	if c.receiving {
		return false
	}

	if c.deferredAttachs == nil {
		c.deferredAttachs = make(map[*Hub]struct{})
	}
	c.deferredAttachs[h] = struct{}{}

	c.logf("deferred attach to hub")

	return true
}

type ClosedErr struct {
	what string
	err  error
}

func (c *ClosedErr) Error() string {
	return c.what + " closed: " + c.err.Error()
}

func (c *Conn) unsafeAttachTo(h *Hub) error {
	c.logf("sending attachment request to hub")

	hctx := h.Context()

	select {
	case h.attach <- c:
		c.logf("attached successfully")
		c.hubs[h] = struct{}{}
		return nil
	case <-c.ctx.Done():
		c.logf("you are closed, abort")
		return &ClosedErr{what: "connection", err: c.ctx.Err()}
	case <-hctx.Done():
		c.logf("hub is closed, abort")
		return &ClosedErr{what: "hub", err: c.ctx.Err()}
	}
}

// AttachTo attaches the connection to a hub.
//
// The Info parameter can be set to custom data about the client.
// You can use it to associate the client with useful information about itself,
// such as the time it has connected, its IP address or any other data deemed useful.
// Access this field in any of Hub's lifecycle methods to implement powerful
// custom logic.
//
// If AttachTo is called before Receive, the attachment itself is deferred to the
// first Receive call.
func (c *Conn) AttachTo(h *Hub) error {
	c.logf("attaching to hub")

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.unsafeTryDeferAttach(h) {
		return nil
	}
	return c.unsafeAttachTo(h)
}

func (c *Conn) unsafeRemoveAttach(h *Hub) {
	if !c.receiving {
		c.logf("connection not receiving, no attachments to remove")
		return
	}

	delete(c.hubs, h)
	c.logf("detached successfully")

	if len(c.hubs) == 0 {
		c.logf("no other attachments, closing...")
		c.cancel()
	}
}

func (c *Conn) unsafeSendDetach(h *Hub) bool {
	if _, attachedTo := c.hubs[h]; attachedTo {
		c.logf("detach request sent to hub")

		select {
		case h.detach <- c:
			c.logf("hub received detach request")
			return true
		case <-h.ctx.Done():
			c.logf("hub is done, you will be removed automatically")
		case <-c.ctx.Done():
			c.logf("you are closed, abort")
		}
	}

	return false
}

// hubDetach is used by Hub when it's shutting down, because
// the connection can't know to detach by itself.
func (c *Conn) hubDetach(h *Hub) {
	c.logf("hub requested detach")

	c.mu.Lock()
	defer c.mu.Unlock()

	c.unsafeRemoveAttach(h)
}

func (c *Conn) unsafeTryRemoveDeferredAttach(h *Hub) bool {
	if c.receiving {
		return false
	}

	delete(c.deferredAttachs, h)

	return true
}

func (c *Conn) unsafeDetachFrom(h *Hub) {
	if c.unsafeSendDetach(h) {
		c.unsafeRemoveAttach(h)
	}
}

// DetachFrom detaches from a hub, if the connection is attached to it.
// If after the detachment the connection is attached to no hubs,
// it is closed completely. It does nothing if the connection isn't
// attached to the given hub.
// Calling DetachFrom before Receive attempt to remove the deferred attachments
// of the given hub, if it exists.
func (c *Conn) DetachFrom(h *Hub) {
	c.logf("detaching from hub")

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.unsafeTryRemoveDeferredAttach(h) {
		return
	}
	c.unsafeDetachFrom(h)
}

// Send a message to the connection. Optionally add a timeout to control
// how long this method can block before returning.
// It returns a flag indicating whether the message was successfully sent.
// If it is false, it means that the connection is closed.
func (c *Conn) Send(ctx context.Context, message interface{}) error {
	c.logf("receiving message")

	select {
	case <-c.ctx.Done():
		c.logf("connection already closed, quit")
		return &ClosedErr{what: "connection", err: c.ctx.Err()}
	case c.messages <- message:
		c.logf("connection open, message was received")
		return nil
	case <-ctx.Done():
		c.logf("send context done, abort")
		return ctx.Err()
	}
}

func (c *Conn) Context() context.Context {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ctx
}

var ErrConnAlreadyReceiving = fmt.Errorf("connection is currently receiving")

type AttachErr struct {
	Hub *Hub
	Err error
}

func (a *AttachErr) Error() string {
	return a.Err.Error()
}

func (c *Conn) unsafeInit(ctx context.Context) error {
	if c.receiving {
		return ErrConnAlreadyReceiving
	}

	if c.MessagesBufferSize == 0 {
		c.MessagesBufferSize = 1
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.hubs = make(map[*Hub]struct{})
	c.messages = make(chan interface{}, c.MessagesBufferSize)
	c.receiving = true

	for h := range c.deferredAttachs {
		delete(c.deferredAttachs, h)

		if err := c.unsafeAttachTo(h); err != nil {
			for h := range c.hubs {
				c.unsafeDetachFrom(h)
			}

			return &AttachErr{Hub: h, Err: err}
		}
	}

	go func() {
		<-c.ctx.Done()

		c.logf("context done, executing teardown...")

		c.mu.Lock()
		defer c.mu.Unlock()

		for h := range c.deferredAttachs {
			delete(c.deferredAttachs, h)
		}
		for h := range c.hubs {
			c.unsafeSendDetach(h)

			delete(c.hubs, h)
		}

		c.logf("closing messages channel...")
		close(c.messages)
		c.receiving = false
		c.logf("closed!")
	}()

	return nil
}

// Receive starts accepting and processing messages. Calling it multiple times will return a nil channel if
// the connection is receiving.
func (c *Conn) Receive(ctx context.Context) (<-chan interface{}, error) {
	c.logf("starting to receive")

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.unsafeInit(ctx); err != nil {
		return nil, err
	}

	return c.messages, nil
}
