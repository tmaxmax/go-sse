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

import "time"

type OnAttachFunction func(*Connection, *Connection) bool
type OnDetachFunction func(interface{}, *Connection) bool
type OnStopFunction func(*Connection) bool

type Hub struct {
	broadcast         chan Messager
	broadcastFromConn chan connectionMessage
	attach            chan attachment
	detach            chan *Connection
	connections       map[*Connection]interface{}
	done              chan struct{}

	onAttach    OnAttachFunction
	onDetach    OnDetachFunction
	onHubStop   OnStopFunction
	sendTimeout time.Duration
}

func (h *Hub) Broadcast(message Messager) {
	h.broadcast <- message
}

func (h *Hub) Start() {
	for {
		select {
		case a := <-h.attach:
			a.supportsBroadcastsFromConnections <- h.broadcastFromConn != nil

			if h.onAttach != nil {
				for other := range h.connections {
					if !h.onAttach(a.connection, other) {
						break
					}
				}
			}

			h.connections[a.connection] = a.information
		case c := <-h.detach:
			info := h.connections[c]
			delete(h.connections, c)

			if h.onDetach != nil {
				for other := range h.connections {
					if !h.onDetach(info, other) {
						break
					}
				}
			}
		case m := <-h.broadcast:
			for c := range h.connections {
				c.Send(m, h.sendTimeout)
			}
		case m := <-h.broadcastFromConn:
			for c := range h.connections {
				if c != m.connection {
					c.Send(m, h.sendTimeout)
				}
			}
		case <-h.done:
			executeHandler := h.onHubStop != nil

			for c := range h.connections {
				if executeHandler && !h.onHubStop(c) {
					executeHandler = false
				}

				c.detachFromUnsafe(h, true)
				delete(h.connections, c)
			}

			return
		}
	}
}

func (h *Hub) Done() <-chan struct{} {
	return h.done
}

func (h *Hub) Stop() {
	close(h.done)
}

func New(opts ...Option) *Hub {
	h := &Hub{
		broadcast:   make(chan Messager, 1),
		attach:      make(chan attachment),
		detach:      make(chan *Connection),
		connections: make(map[*Connection]interface{}),
		done:        make(chan struct{}),
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}
