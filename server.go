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
we instantiate a Connection. Then we subscribe to incoming events using callback functions, and then
we establish the connection by calling the Connection's Connect method.
*/
package sse

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
)

// SubscriptionCallback is a function that is called when a subscriber receives a message.
// If this callback returns false, the provider must remove the subscription.
type SubscriptionCallback func(message *Message) bool

// The Subscription struct is used to subscribe to a given provider.
type Subscription struct {
	// A non-nil function that is called for each new event. The callback of a single subscription will
	// never be called in parallel, but callbacks from multiple subscriptions may be called in parallel.
	Callback SubscriptionCallback
	// An optional last event ID indicating the event to resume the stream from.
	// The events will replay starting from the first valid event sent after the one with the given ID.
	// If the ID is invalid replaying events will be omitted and new events will be sent as normal.
	LastEventID EventID
	// The topics to receive message from. If no topic is specified, a default topic is implied.
	// Topics are orthogonal to event types. They are used to filter what the server sends to each client.
	//
	// If using a Provider directly, without a Server instance, you must specify at least one topic.
	// The Server automatically adds the default topic if no topic is specified.
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
	// Subscribe to the provider. The context is used to remove the subscriber automatically
	// when it is done. Errors returned by the subscription's callback function must be returned
	// by Subscribe.
	//
	// Providers can assume that the topics list for a subscription has at least one topic.
	Subscribe(ctx context.Context, subscription Subscription) error
	// Publish a message to all the subscribers that are subscribed to the given topics.
	// The topics slice must be non-empty, or ErrNoTopic will be raised.
	Publish(message *Message, topics []string) error
	// Stop the provider. Calling Stop will clean up all the provider's resources and
	// make Subscribe and Publish fail with an error. All the listener channels will be
	// closed and any ongoing publishes will be aborted.
	//
	// Calling Stop multiple times does nothing but return ErrProviderClosed.
	Stop() error
}

// ErrProviderClosed is a sentinel error returned by providers when any operation is attempted after the provider is closed.
var ErrProviderClosed = errors.New("go-sse.server: provider is closed")

// ErrNoTopic is a sentinel error returned by providers when a Message is published without any topics.
// It is not an issue to call Server.Publish without topics, because the Server will add the DefaultTopic;
// it is an error to call Provider.Publish without any topics, though.
var ErrNoTopic = errors.New("go-sse.server: no topics specified")

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

// The Logger interface describes an object that can be used for logging.
type Logger interface {
	Printf(format string, args ...interface{})
}

// WithLogger is an option that sets a custom logger for a given Server.
// The default logger is the one provided by the standard log package.
func WithLogger(logger Logger) ServerOption {
	return func(s *Server) {
		if logger != nil {
			s.logger = logger
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
	logger   Logger
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

	if s.logger == nil {
		s.logger = log.New(os.Stderr, "go-sse: ", log.LstdFlags)
	}

	return s
}

type writeFlusher interface {
	http.ResponseWriter
	http.Flusher
}

// An UpgradedRequest is used to send events to the client.
// Create one using the Upgrade function.
type UpgradedRequest struct {
	w          writeFlusher
	didUpgrade bool
}

// Send sends the given event to the client. It returns any errors that occurred while writing the event.
func (u *UpgradedRequest) Send(e *Message) error {
	if !u.didUpgrade {
		u.w.Header()[headerContentType] = headerContentTypeValue
		u.w.Flush()
		u.didUpgrade = true
	}
	if _, err := e.WriteTo(u.w); err != nil {
		return err
	}
	u.w.Flush()
	return nil
}

// Upgrade upgrades an HTTP request to support server-sent events.
// It returns a Connection that's used to send events to the client or an
// error if the upgrade failed.
//
// The headers required by the SSE protocol are only sent when calling
// the Send method for the first time. If other operations are done before
// sending messages, other headers and status codes can safely be set.
func Upgrade(w http.ResponseWriter) (UpgradedRequest, error) {
	fw, ok := w.(writeFlusher)
	if !ok {
		return UpgradedRequest{}, ErrUpgradeUnsupported
	}

	return UpgradedRequest{w: fw}, nil
}

// ErrUpgradeUnsupported is returned when a request can't be upgraded to support server-sent events.
var ErrUpgradeUnsupported = errors.New("go-sse.server: upgrade unsupported")

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

	conn, err := Upgrade(w)
	if err != nil {
		http.Error(w, "Server-sent events unsupported", http.StatusInternalServerError)
		return
	}

	id := EventID{}
	// Clients must not send empty Last-Event-Id headers:
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#sse-processing-model
	if h := r.Header[headerLastEventID]; len(h) != 0 && h[0] != "" {
		// We ignore the validity flag because if the given ID is invalid then an unset ID will be returned,
		// which providers are required to ignore.
		id, _ = NewID(h[0])
	}

	cb := func(m *Message) bool {
		if err := conn.Send(m); err != nil {
			s.logger.Printf("send error: %v", err)
			return false
		}
		return true
	}
	sub := Subscription{
		Callback:    cb,
		LastEventID: id,
		Topics:      []string{DefaultTopic},
	}

	if err = s.provider.Subscribe(r.Context(), sub); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Canonicalized header keys.
const (
	headerLastEventID = "Last-Event-Id"
	headerContentType = "Content-Type"
)

// Pre-allocated header value.
var headerContentTypeValue = []string{"text/event-stream"}

// Publish sends the event to all subscribes that are subscribed to the topic the event is published to.
// The topics are optional - if none are specified, the event is published to the DefaultTopic.
func (s *Server) Publish(e *Message, topics ...string) error {
	return s.provider.Publish(e, getTopics(topics))
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
	return s.provider.Stop()
}

var defaultTopicSlice = []string{DefaultTopic}

func getTopics(initial []string) []string {
	if len(initial) == 0 {
		return defaultTopicSlice
	}

	return initial
}
