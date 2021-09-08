package client

import (
	"context"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// The ResponseValidator type defines the type of the function
// that checks whether server responses are valid, before starting
// to read events from them. See the Client's documentation for more info.
type ResponseValidator func(*http.Response) error

type Client struct {
	// The HTTP client to be used. Defaults to http.DefaultClient.
	HTTPClient *http.Client
	// A callback that's executed whenever a reconnection attempt starts.
	OnRetry backoff.Notify
	// A function to check if the response from the server is valid.
	// Defaults to a function that checks the response's status code is 200
	// and the content type is text/event-stream.
	//
	// If the error type returned has a Temporary or a Timeout method,
	// they will be used to determine whether to reattempt the connection.
	// Otherwise, the error will be considered permanent and no reconnections
	// will be attempted.
	ResponseValidator ResponseValidator
	// The maximum number of reconnection to attempt when an error occurs.
	// If MaxRetries is negative (-1), infinite reconnection attempts will be done.
	// Defaults to 0 (no retries).
	MaxRetries int
	// The initial reconnection delay. Subsequent reconnections use a longer
	// time. This can be overridden by retry values sent by the server.
	// Defaults to 5 seconds.
	DefaultReconnectionTime time.Duration
}

// NewConnection initializes and configures a connection. On connect, the given
// request is sent and if successful the connection starts receiving messages.
// Use the request's context to stop the connection.
//
// If the request has a body, it is necessary to provide a GetBody function in order
// for the connection to be reattempted, in case of an error. Using readers
// such as bytes.Reader, strings.Reader or bytes.Buffer when creating a request
// using http.NewRequestWithContext will ensure this function is present on the request.
func (c *Client) NewConnection(r *http.Request) *Connection {
	if r == nil {
		panic("go-sse.client.NewConnection: request cannot be nil")
	}

	mergeDefaults(c)

	conn := &Connection{
		client:         *c,                   // we clone the client so the config cannot be modified from outside
		request:        r.Clone(r.Context()), // we clone the request so its fields cannot be modified from outside
		subscribers:    map[eventName]map[subscriber]struct{}{},
		subscribersAll: map[subscriber]struct{}{},
		event:          make(chan Event),
		subscribe:      make(chan subscription),
		unsubscribe:    make(chan subscription),
		done:           make(chan struct{}),
	}

	go conn.run()

	return conn
}

func (c *Client) do(r *http.Request) (*http.Response, error) {
	return c.HTTPClient.Do(r)
}

func (c *Client) newBackoff(ctx context.Context) (backoff.BackOff, *time.Duration) {
	base := backoff.NewExponentialBackOff()
	base.InitialInterval = c.DefaultReconnectionTime
	initialReconnectionTime := &base.InitialInterval
	b := backoff.WithContext(base, ctx)
	if c.MaxRetries >= 0 {
		return backoff.WithMaxRetries(b, uint64(c.MaxRetries)), initialReconnectionTime
	}
	return b, initialReconnectionTime
}
