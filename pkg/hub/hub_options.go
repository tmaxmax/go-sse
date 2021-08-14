package hub

import "time"

type Option func(*Hub)

// OnAttach is an option that sets a handler which is executed
// on each previous connection when a new connection attaches.
// The first parameter is the new connection.
// The second parameter is the connection this handler is currently executed on.
// If you want to do something with the new connection use the connection returned by the
// Hub.Connect method.
// Return false from this function if you want to stop executing the handler for that
// specific attachment.
func OnAttach(fn OnAttachFunction) Option {
	return func(h *Hub) {
		h.onAttach = fn
	}
}

// OnDetach is an option that sets a handler on the hub which is executed
// on each remaining connection when another is lost.
// The parameters are the same as for onAttach, but the first is the info of the
// lost connection.
// Return false from this function if you want to stop executing the handler for that
// specific detachment.
func OnDetach(fn OnDetachFunction) Option {
	return func(h *Hub) {
		h.onDetach = fn
	}
}

// OnHubStop is an option that sets a handler which is executed
// on each connection when the hub is stopped.
// Return false from this function if you want to stop executing the handler for the
// rest of the remaining connections attached. Note that this will not stop the
// hub from closing!
func OnHubStop(fn OnStopFunction) Option {
	return func(h *Hub) {
		h.onHubStop = fn
	}
}

// AllowBroadcastsFromClients is an option which adds the ability for connections
// to broadcast messages to other clients in the hub.
func AllowBroadcastsFromClients() Option {
	return func(h *Hub) {
		h.broadcastFromConn = make(chan connectionMessage, 1)
	}
}

// SendTimeout is an option that tells the hub to abort sending the message to
// a client after a certain duration, if the message isn't sent already.
func SendTimeout(duration time.Duration) Option {
	return func(h *Hub) {
		h.sendTimeout = duration
	}
}
