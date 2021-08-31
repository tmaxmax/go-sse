package server

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/tmaxmax/go-sse/server/event"
)

// The Subscription struct is used to subscribe to a given provider.
type Subscription struct {
	// A non-nil, open channel to send events on. The provider may wait when sending on this channel
	// until the event is received, so start receiving immediately after the provider's Subscribe
	// method returned.
	//
	// Subsequent subscriptions that use the same channel are ignored by providers.
	//
	// Only the provider is allowed to close this channel. Closing it yourself may cause the program to panic!
	Channel chan<- *event.Event
	// The topics to receive message from. If no topic is specified, a default topic is implied.
	Topics []string
	// An optional last event ID indicating the event to resume the stream from.
	// The events will replay starting from the first valid event sent after the one with the given ID.
	// If the ID is invalid replaying events will be omitted and new events will be sent as normal.
	LastEventID event.ID
}

// The Message struct is used to publish a message to a given provider.
type Message struct {
	// The event to publish.
	Event *event.Event
	// The topic to publish the event to. If no topic is specified the default topic is implied.
	Topic string
}

// A Provider is a publish-subscribe system that can be used to implement a HTML5 server-sent events
// protocol. A standard interface is required so HTTP request handlers are agnostic to the provider's implementation.
//
// Providers are required to be thread-safe.
//
// After Stop is called, trying to call any method of the provider must return ErrProviderClosed. The providers
// may return other implementation-specific errors too, but the close error is guaranteed to be the same across
// providers.
type Provider interface {
	// Subscribe to the provider. The context is used to remove the subscription automatically
	// when the subscriber stops receiving messages.
	Subscribe(ctx context.Context, subscription Subscription) error
	// Publish a message to all the subscribers that are subscribed to the message's topic.
	Publish(message Message) error
	// Stop the provider. Calling Stop will clean up all the provider's resources and
	// make Subscribe and Publish fail with an error. All the subscription channels will be
	// closed and any ongoing publishes will be aborted.
	//
	// Calling Stop multiple times does nothing but return ErrProviderClosed.
	Stop() error
}

var ErrProviderClosed = errors.New("go-sse.server: provider is closed")

// DefaultTopic is the identifier for the topic that is implied when no topics are specified for a Subscription
// or a Message. Providers are required to implement this behavior to ensure handlers don't break if providers
// are changed.
const DefaultTopic = ""

// A Server is a convenience wrapper around a provider.
type Server struct {
	provider Provider
}

// New creates a new server using the specified provider. If no provider is given, Joe with no replay provider is used.
func New(provider ...Provider) *Server {
	s := &Server{}
	if len(provider) > 0 {
		s.provider = provider[0]
	} else {
		s.provider = NewJoe()
	}

	return s
}

// Handler is used to create a custom HTTP handler using the server's underlying provider.
func (s *Server) Handler(f func(Provider) http.Handler) http.Handler {
	return f(s.provider)
}

// ServeHTTP implements a default HTTP handler for a server.
//
// This handler upgrades the request, subscribes it to the server's provider and
// starts sending incoming events to the client, while logging any write errors.
//
// If you need different behavior, use the Handler method.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := NewConnection(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	events := make(chan *event.Event)
	err = s.provider.Subscribe(r.Context(), Subscription{
		Channel:     events,
		LastEventID: event.ID(r.Header.Get("Last-Event-ID")),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for e := range events {
		if err = conn.Send(e); err != nil {
			log.Println("go-sse.handler: send error:", err)
			break
		}
	}
}

// Publish sends the event to all subscribes that are subscribed to the topic the event is published to.
// The topic is optional - if none is specified, the event is published to the default topic.
func (s *Server) Publish(e *event.Event, topic ...string) error {
	msg := Message{Event: e}
	if len(topic) > 0 {
		msg.Topic = topic[0]
	}

	return s.provider.Publish(msg)
}

// Shutdown closes all the connections and stops the server. Publish operations will fail
// with the error sent by the underlying provider. New requests will be ignored.
//
// Call this method when shutting down the HTTP server using http.Server's RegisterOnShutdown
// method. Not doing this will result in the server never shutting down or connections being
// abruptly stopped.
//
// The error returned is the one returned by the underlying provider's Stop method.
func (s *Server) Shutdown() error {
	return s.provider.Stop()
}
