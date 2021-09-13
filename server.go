/*
Package sse provides utilities for creating and consuming fully spec-compliant HTML5 server-sent events streams.

The central piece of a server's implementation is the Provider interface. A Provider describes a publish-subscribe
system that can be used to implement messaging for the SSE protocol. This package already has an
implementation, called Joe, that is the default provider for any server. Abstracting the messaging
system implementation away allows servers to use any arbitrary provider under the same interface.
The default provider will work for simple use-cases, but where scalability is required, one will
look at a more suitable solution. Adapters that satisfy the Provider interface can easily be created,
and then plugged into the server instance.
Events themselves are represented using the Message type.

On the client-side, we use the Client struct to create connections to event streams. Using an `http.Request`
we instantiate a Connection. Then we subscribe to incoming events using channels and when subscribed
we establish the connection by calling the Conenction's Connect method.
*/
package sse

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
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
	Channel chan<- *Message
	// An optional last event ID indicating the event to resume the stream from.
	// The events will replay starting from the first valid event sent after the one with the given ID.
	// If the ID is invalid replaying events will be omitted and new events will be sent as normal.
	LastEventID EventID
	// The topics to receive message from. If no topic is specified, a default topic is implied.
	// Topics are orthogonal to event names. They are used to filter what the server sends to each client.
	Topics []string
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
	// Subscribe to the provider. The context is used to remove the listener automatically
	// when the subscriber stops receiving messages.
	Subscribe(ctx context.Context, subscription Subscription) error
	// Publish a message to all the subscribers that are subscribed to the message's topic.
	Publish(message *Message) error
	// Stop the provider. Calling Stop will clean up all the provider's resources and
	// make Subscribe and Publish fail with an error. All the listener channels will be
	// closed and any ongoing publishes will be aborted.
	//
	// Calling Stop multiple times does nothing but return ErrProviderClosed.
	Stop() error
}

// ErrProviderClosed is a sentinel error returned by providers when any operation is attempted after the provider is closed.
var ErrProviderClosed = errors.New("go-sse.server: provider is closed")

// DefaultTopic is the identifier for the topic that is implied when no topics are specified for a Subscription
// or a Message. Providers are required to implement this behavior to ensure handlers don't break if providers
// are changed.
const DefaultTopic = ""

// A ServerOption configures a certain property of a given Server.
type ServerOption func(*Server)

// WithProvider is an option that sets a custom provider for a given Server.
// The default provider is Joe without any ReplayProvider, if this option isn't given.
func WithProvider(provider Provider) ServerOption {
	return func(s *Server) {
		if provider != nil {
			s.provider = provider
		}
	}
}

// A Server is mostly a convenience wrapper around a provider.
// It implements the http.Handler interface and has some methods
// for calling the underlying provider's methods.
//
// When creating a server, if no provider is specified using the WithProvider
// option, the Joe provider found in this package with no replay provider is used.
type Server struct {
	provider Provider
}

// NewServer creates a new server using the specified provider. If no provider is given, Joe with no replay provider is used.
func NewServer(options ...ServerOption) *Server {
	s := &Server{}

	for _, opt := range options {
		opt(s)
	}

	if s.provider == nil {
		s.provider = NewJoe()
	}

	return s
}

// Provider returns the server's underlying provider.
func (s *Server) Provider() Provider {
	return s.provider
}

type writeFlusher interface {
	io.Writer
	http.Flusher
}

// An UpgradedRequest is used to send events to the client.
// Create one using the Upgrade function.
type UpgradedRequest struct {
	w writeFlusher
}

// Send sends the given event to the client. It returns any errors that occurred while writing the event.
func (u *UpgradedRequest) Send(e *Message) error {
	_, err := e.WriteTo(u.w)
	u.w.Flush()
	return err
}

// Upgrade upgrades an HTTP request to support server-sent events.
// It returns a Connection that's used to send events to the client or an
// error if the upgrade failed.
func Upgrade(w http.ResponseWriter) (*UpgradedRequest, error) {
	fw, ok := w.(writeFlusher)
	if !ok {
		return nil, ErrUpgradeUnsupported
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	fw.Flush()

	return &UpgradedRequest{w: fw}, nil
}

// ErrUpgradeUnsupported is returned when a request can't be upgraded to support server-sent events.
var ErrUpgradeUnsupported = errors.New("go-sse.server.connection: unsupported")

// ServeHTTP implements a default HTTP handler for a server.
//
// This handler upgrades the request, subscribes it to the server's provider and
// starts sending incoming events to the client, while logging any write errors.
// It also sends the Last-Event-ID header's value, if present.
//
// If the request isn't upgradeable, it writes a message to the client along with
// an 500 Internal Server ConnectionError response code. If on subscribe the provider returns
// an error, it writes the error message to the client and a 500 Internal Server ConnectionError
// response code.
//
// If you need different behavior, you can create a custom handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Make sure to keep the ServeHTTP implementation line number in sync with the number in the README!

	events, id := make(chan *Message), EventID{}
	// Clients must not send empty Last-Message-ID headers:
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#sse-processing-model
	if h := r.Header.Get("Last-Event-ID"); h != "" {
		// We ignore the validity flag because if the given ID is invalid then an unset ID will be returned,
		// which providers are required to ignore.
		id, _ = NewEventID(h)
	}
	err := s.Subscribe(r.Context(), events, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	conn, err := Upgrade(w)
	if err != nil {
		http.Error(w, "Server-sent events unsupported", http.StatusInternalServerError)
		return
	}

	for e := range events {
		if err = conn.Send(e); err != nil {
			log.Println("go-sse.handler: send error:", err)
			break
		}
	}
}

// Subscribe subscribes the given channel to the specified topics. It is unsubscribed when the context is closed
// or the server is shut down. If no topic is specified, the channel is subscribed to the default topic.
func (s *Server) Subscribe(ctx context.Context, ch chan<- *Message, lastEventID EventID, topics ...string) error {
	return s.Provider().Subscribe(ctx, Subscription{
		Channel:     ch,
		LastEventID: lastEventID,
		Topics:      topics,
	})
}

// Publish sends the event to all subscribes that are subscribed to the topic the event is published to.
// The topic is optional - if none is specified, the event is published to the default topic.
func (s *Server) Publish(e *Message) error {
	return s.Provider().Publish(e)
}

// Shutdown closes all the connections and stops the server. Publish operations will fail
// with the error sent by the underlying provider. NewServer requests will be ignored.
//
// Call this method when shutting down the HTTP server using http.Server's RegisterOnShutdown
// method. Not doing this will result in the server never shutting down or connections being
// abruptly stopped.
//
// The error returned is the one returned by the underlying provider's Stop method.
func (s *Server) Shutdown() error {
	return s.Provider().Stop()
}
