/*
Package hub provides an abstraction over a messaging pattern similar to pub-sub.

A Connection (or subscriber) is at first a link between the consumer and the application.
In order to receive messages it must attach itself to one or more Hub instances. After it's
attached, messages broadcasted to those hubs are received by the Connection.

Connections can be created from any type that is an io.Writer. This way the library is
completely agnostic to the source of the connection - it only has to write messages, so nothing more is required.

Messages must implement the Messager interface. It is similar to io.WriterTo, but it is required that it can
be called multiple times and from multiple goroutines.

The Hub is simply a broker that manages all messages, attachments and detachments. You can broadcast messages to
a Hub and all clients will receive, or a connection can broadcast to a Hub it's attached to (if the Hub allows it),
and all the other clients will receive the message.
A Hub also provides the option to add handlers that are run when certain events occur, such as a client attaching or
detaching. This way you can implement custom logic, such as broadcasting a special message when a new connection
is attached.
*/
package hub

import (
	"sync"
	"time"
)

// AttachedConnection holds a set of safe methods to call inside Hub handlers.
// Calling the other exported methods of Connection might break the program inside them.
type AttachedConnection interface {
	AttachTo(*Hub)
	DetachFrom(*Hub)
	Send(Messager, ...time.Duration) bool
	BroadcastTo(*Hub, Messager) error
	Information() interface{}
}

type OnAttachFunction = func(AttachedConnection, AttachedConnection)
type OnDetachFunction = func(interface{}, AttachedConnection)
type OnStopFunction = func(AttachedConnection)
type MessageFilterFunction = func(Messager, interface{}) bool

type Hub struct {
	broadcast         chan Messager
	broadcastFromConn chan connectionMessage
	attach            chan attachment
	detach            chan *Connection
	connections       map[*Connection]struct{}
	done              chan struct{}
	running           bool
	mu                sync.Mutex

	// OnAttach is a handler which is executed
	// on each previous connection when a new connection attaches.
	// The first parameter is the new connection.
	// The second parameter is the connection this handler is currently executed on.
	// If you want to do something with the new connection use the connection returned by the
	// Hub.Connect method.
	// Return false from this function if you want to stop executing the handler for that
	// specific attachment.
	OnAttach OnAttachFunction
	// OnDetach is a handler which is executed
	// on each remaining connection when another is lost.
	// The parameters are the same as for OnAttach, but the first is the info of the
	// lost connection.
	// Return false from this function if you want to stop executing the handler for that
	// specific detachment.
	OnDetach OnDetachFunction
	// OnHubStop is a handler which is executed
	// on each connection when the hub is stopped.
	// Return false from this function if you want to stop executing the handler for the
	// rest of the remaining connections attached. Note that this will not stop the
	// hub from closing!
	OnHubStop OnStopFunction
	// MessageFilter is a function that is called for every connection when a message is
	// broadcast. It takes as parameters the message itself and the current connection's
	// additional information, if any. If the function returns true the message is sent,
	// else the connection is skipped.
	MessageFilter MessageFilterFunction
	// SendTimeout sets the duration after the hub stops waiting for the client to
	// receive the message. The zero value is equivalent to waiting indefinitely.
	SendTimeout time.Duration
	// AllowBroadcastsFromClients adds the ability for connections
	// to broadcast messages to other connections in the hub.
	AllowBroadcastsFromClients bool
}

func (h *Hub) init() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		return false
	}

	if h.AllowBroadcastsFromClients {
		h.broadcastFromConn = make(chan connectionMessage, 1)
	} else {
		h.broadcastFromConn = nil
	}

	h.broadcast = make(chan Messager, 1)
	h.attach = make(chan attachment)
	h.detach = make(chan *Connection)
	h.connections = make(map[*Connection]struct{})
	h.done = make(chan struct{})
	h.running = true

	return true
}

// Broadcast sends the given message to all the connections attached to the hub.
func (h *Hub) Broadcast(message Messager) {
	h.broadcast <- message
}

func (h *Hub) sendMessage(m Messager, c *Connection) {
	if h.MessageFilter != nil && !h.MessageFilter(m, c.Info) {
		return
	}

	// TODO: Implement some retry logic and detach the connection if the send failed too many times

	c.Send(m, h.SendTimeout)
}

// Start runs the hub and blocks the calling goroutine. Calling it multiple times afterwards will do nothing.
// Once Start is called, the hub accepts attachments, detachments and handles broadcasts. Do not modify
// the exported fields after it started, as it can result in a data race!
func (h *Hub) Start() {
	if !h.init() {
		return
	}

	// TODO: Run event handlers in different goroutines?

	for {
		select {
		case a := <-h.attach:
			a.supportsBroadcastsFromConnections <- h.broadcastFromConn != nil

			if h.OnAttach != nil {
				for other := range h.connections {
					h.OnAttach(a.connection, other)
				}
			}

			h.connections[a.connection] = struct{}{}
		case c := <-h.detach:
			// FIXME: hub hangs when OnDetach handler is run

			delete(h.connections, c)

			if h.OnDetach != nil {
				for other := range h.connections {
					h.OnDetach(c.Info, other)
				}
			}
		case m := <-h.broadcast:
			for c := range h.connections {
				h.sendMessage(m, c)
			}
		case m := <-h.broadcastFromConn:
			for c := range h.connections {
				if c == m.connection {
					continue
				}

				h.sendMessage(m, c)
			}
		case <-h.done:
			executeHandler := h.OnHubStop != nil

			for c := range h.connections {
				if executeHandler {
					h.OnHubStop(c)
				}

				c.detachFromUnsafe(h, true)
				delete(h.connections, c)
			}

			return
		}
	}
}

// Done returns a channel that blocks until the hub is stopped.
// Returns a nil channel if the hub wasn't started.
func (h *Hub) Done() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.done
}

// Stop tells the hub to detach all connections and to then stop accepting new ones or handling broadcasts.
// Calling it multiple times does nothing. After calling Stop, exported fields can be modified
// and Start can be called again, all safely.
func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		close(h.done)
		h.running = false
	}
}
