package client

import (
	"fmt"
	"net/http"
	"time"
)

// DefaultValidator is the default response validation function. It checks the content type to be
// text/event-stream and the response status code to be 200 OK.
var DefaultValidator ResponseValidator = func(r *http.Response) error {
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status code %d, received %d", http.StatusOK, r.StatusCode)
	}
	if rc, ec := r.Header.Get("Content-Type"), "text/event-stream"; rc != ec {
		return fmt.Errorf("expected content type %q, received %q", ec, rc)
	}
	return nil
}

// NoopValidator is a validator function that treats all responses as valid.
var NoopValidator ResponseValidator = func(_ *http.Response) error {
	return nil
}

// DefaultClient is the client that is used by the free functions exported by this package.
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
