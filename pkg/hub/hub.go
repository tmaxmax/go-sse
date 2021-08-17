/*
Package hub provides an abstraction over a messaging pattern similar to pub-sub.

A Conn (or subscriber) is at first a link between the consumer and the application.
In order to receive messages it must attach itself to one or more Hub instances. After it's
attached, messages broadcasted to those hubs are received by the Conn.

The Hub is simply a broker that manages all messages, attachments and detachments. You can broadcast messages to
a Hub and all clients will receive, or a connection can broadcast to a Hub it's attached to (if the Hub allows it),
and all the other clients will receive the message.
A Hub also provides the option to add handlers that are run when certain events occur, such as a client attaching or
detaching. This way you can implement custom logic, such as broadcasting a special message when a new connection
is attached.
*/
package hub

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type EventHandler = func(affectedConn *Conn, iterConn *Conn)
type MessageFilterFunction = func(msg interface{}, connInfo interface{}) bool

type attachEvent *Conn
type detachEvent *Conn

type Hub struct {
	broadcast   chan interface{}
	attach      chan *Conn
	detach      chan *Conn
	connections map[*Conn]chan<- interface{}
	running     bool
	ctx         context.Context
	mu          sync.RWMutex
	wg          sync.WaitGroup

	Log *log.Logger
	// OnAttach is a handler which is executed
	// on each previous connection when a new connection attaches.
	// The first parameter is the new connection's information.
	// The second parameter is the connection this handler is currently executed on.
	OnAttach EventHandler
	// OnDetach is a handler which is executed on each remaining connection when another is lost.
	// The parameters are the same as for OnAttach.
	OnDetach EventHandler
	// OnHubStop is a handler which is executed on each connection when the hub is stopped.
	OnHubStop func(*Conn)
	// MessageFilter is a function that is called for every connection when a message is
	// broadcast. It takes as parameters the message itself and the current connection's
	// additional information, if any. If the function returns true the message is sent,
	// else the connection is skipped.
	MessageFilter MessageFilterFunction
	// SendTimeout sets the duration after the hub stops waiting for the client to
	// receive the message. The zero value is equivalent to waiting indefinitely.
	SendTimeout     time.Duration
	MaxRetries      int
	BackoffUnit     time.Duration
	Backoff         BackoffFunction
	BackoffJitter   float64
	BroadcastBuffer int
}

func (h *Hub) logf(format string, args ...interface{}) {
	if h.Log != nil {
		h.Log.Printf(format, args...)
	}
}

func (h *Hub) init(ctx context.Context) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		return false
	}

	if h.BroadcastBuffer == 0 {
		h.BroadcastBuffer = 1
	}
	if h.MaxRetries == 0 {
		h.MaxRetries = 3
	}
	if h.SendTimeout == 0 {
		h.SendTimeout = time.Second
	}
	if h.BackoffUnit == 0 {
		h.BackoffUnit = time.Second
	}
	if h.Backoff == nil {
		h.Backoff = BackoffDefault
	}

	h.ctx = ctx
	h.broadcast = make(chan interface{}, h.BroadcastBuffer)
	h.attach = make(chan *Conn)
	h.detach = make(chan *Conn)
	h.connections = make(map[*Conn]chan<- interface{})
	h.running = true

	h.logf("initialized")

	return true
}

func (h *Hub) detachConnOnSendError(c *Conn, reason string) {
	c.logf("send failed: %s. sending conenction to detach...", reason)
	h.detach <- c
	c.hubDetach(h)
}

func (h *Hub) newContext() (context.Context, context.CancelFunc) {
	if h.SendTimeout != 0 {
		return context.WithTimeout(h.ctx, h.SendTimeout)
	}

	return h.ctx, func() {}
}

func (h *Hub) trySend(c *Conn, m interface{}) bool {
	if h.MessageFilter != nil && !h.MessageFilter(m, c) {
		return true
	}

	ctx, cancel := h.newContext()
	defer cancel()

	if err := c.Send(ctx, m); err != nil {
		if _, ok := err.(*ClosedErr); !ok {
			return false
		}

		h.detachConnOnSendError(c, "already closed")
	}

	return true
}

func (h *Hub) attachConn(c *Conn) {
	in, out := unboundedChannel(1, true)
	h.connections[c] = in
	h.wg.Add(1)

	go func() {
		defer h.wg.Done()
		defer c.logf("handler done!")

		for m := range out {
			switch v := m.(type) {
			case attachEvent:
				h.OnAttach(v, c)
			case detachEvent:
				h.OnDetach(v, c)
			default:
				var i int
				for ; i < h.MaxRetries; i++ {
					if i > 0 {
						select {
						case <-h.ctx.Done():
							c.logf("retry canceled: hub closed")
							return
						case <-c.Context().Done():
							c.logf("retry canceled: connection closed")
							return
						case <-time.After(h.Backoff(h.BackoffUnit, h.BackoffJitter, i)):
						}
					}
					if h.trySend(c, m) {
						break
					}
				}
				if i == h.MaxRetries {
					h.detachConnOnSendError(c, "timed out")
					break
				}
			}
		}

	}()
}

func (h *Hub) detachConn(c *Conn) {
	c.logf("detaching conn...")
	close(h.connections[c])
	delete(h.connections, c)
	c.logf("conn detached!")
}

// Start runs the hub and blocks the calling goroutine. Calling it multiple times afterwards will do nothing.
// Once Start is called, the hub accepts attachments, detachments and handles broadcasts. Do not modify
// the exported fields after it started, as it can result in a data race!
func (h *Hub) Start(ctx context.Context) {
	if !h.init(ctx) {
		return
	}

	for {
		select {
		case c := <-h.attach:
			h.logf("new attachment request")

			if h.OnAttach != nil {
				for _, ch := range h.connections {
					ch <- attachEvent(c)
				}
			}

			h.attachConn(c)

			c.logf("attached to hub")
		case c := <-h.detach:
			h.logf("new detachment request")

			h.detachConn(c)

			if h.OnDetach != nil {
				for _, ch := range h.connections {
					ch <- detachEvent(c)
				}
			}

			c.logf("detached from hub")
		case m := <-h.broadcast:
			for _, ch := range h.connections {
				ch <- m
			}

			h.logf("broadcasted successfully")
		case <-h.ctx.Done():
			h.teardown()

			return
		}
	}
}

// Broadcast sends the given message to all the connections attached to the hub.
func (h *Hub) Broadcast(ctx context.Context, message interface{}) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	select {
	case h.broadcast <- message:
		return nil
	case <-h.ctx.Done():
		return fmt.Errorf("hub done: %w", h.ctx.Err())
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Hub) Context() context.Context {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.ctx
}

func (h *Hub) teardown() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, in := range h.connections {
		close(in)
	}

	h.logf("waiting for attachment goroutines to finish...")
	h.wg.Wait()
	h.logf("detaching connections and executing stop handlers...")

	executeHandler := h.OnHubStop != nil
	if executeHandler {
		h.wg.Add(len(h.connections))
	}

	for c := range h.connections {
		delete(h.connections, c)

		if executeHandler {
			go func(c *Conn) {
				defer h.wg.Done()
				h.OnHubStop(c)
				c.hubDetach(h)
			}(c)
		} else {
			c.hubDetach(h)
		}
	}

	h.logf("waiting for hub operations to finish...")
	h.wg.Wait()
	h.running = false
	h.logf("closed!")
}
