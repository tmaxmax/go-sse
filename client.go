package sse

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/cenkalti/backoff/v4"
)

// The ResponseValidator type defines the type of the function
// that checks whether server responses are valid, before starting
// to read events from them. See the Client's documentation for more info.
//
// Returning an error type that implements the Temporary method will
// tell the client to reattempt the connection. This can be useful if
// you're being rate limited (response code 429), for example.
type ResponseValidator func(*http.Response) error

// The Client struct is used to initialize new connections to different servers.
// It is safe for concurrent use.
//
// After connections are created, the Connect method must be called to start
// receiving events.
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
	//
	// This counter is reset if a reconnection attempt is successful.
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
		client:       *c,                   // we clone the client so the config cannot be modified from outside
		request:      r.Clone(r.Context()), // we clone the request so its fields cannot be modified from outside
		callbacks:    map[string]map[int]EventCallback{},
		callbacksAll: map[int]EventCallback{},
	}

	return conn
}

func (c *Client) newBackoff(ctx context.Context) (b backoff.BackOff, setRetry func(time.Duration)) {
	base := backoff.NewConstantBackOff(c.DefaultReconnectionTime)
	b = backoff.WithContext(base, ctx)
	if c.MaxRetries >= 0 {
		rb := backoff.WithMaxRetries(b, uint64(c.MaxRetries))
		return rb, func(d time.Duration) {
			base.Interval = d
			rb.Reset()
		}
	}
	return b, func(d time.Duration) {
		base.Interval = d
		b.Reset()
	}
}

func contentType(header string) string {
	cts := strings.FieldsFunc(header, func(r rune) bool {
		return unicode.IsSpace(r) || r == ';' || r == ','
	})
	if len(cts) == 0 {
		return ""
	}
	return strings.ToLower(cts[0])
}

// DefaultValidator is the default client response validation function. As per the spec,
// It checks the content type to be text/event-stream and the response status code to be 200 OK.
//
// If this validator fails, errors are considered permanent. No retry attempts are made.
//
// See https://html.spec.whatwg.org/multipage/server-sent-events.html#sse-processing-model.
var DefaultValidator ResponseValidator = func(r *http.Response) error {
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status code %d %s, received %d %s", http.StatusOK, http.StatusText(http.StatusOK), r.StatusCode, http.StatusText(r.StatusCode))
	}
	cts := r.Header.Get("Content-Type")
	ct := contentType(cts)
	if expected := "text/event-stream"; ct != expected {
		return fmt.Errorf("expected content type to have %q, received %q", expected, cts)
	}
	return nil
}

// NoopValidator is a client response validator function that treats all responses as valid.
var NoopValidator ResponseValidator = func(_ *http.Response) error {
	return nil
}

// DefaultClient is the client that is used when creating a new connection using the NewConnection function.
// Unset properties on new clients are replaced with the ones set for the default client.
var DefaultClient = &Client{
	HTTPClient:              http.DefaultClient,
	DefaultReconnectionTime: time.Second * 5,
	ResponseValidator:       DefaultValidator,
}

// NewConnection creates a connection using the default client.
func NewConnection(r *http.Request) *Connection {
	return DefaultClient.NewConnection(r)
}

func mergeDefaults(c *Client) {
	if c.HTTPClient == nil {
		c.HTTPClient = DefaultClient.HTTPClient
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = DefaultClient.MaxRetries
	}
	if c.DefaultReconnectionTime <= 0 {
		c.DefaultReconnectionTime = DefaultClient.DefaultReconnectionTime
	}
	if c.ResponseValidator == nil {
		c.ResponseValidator = DefaultClient.ResponseValidator
	}
}
