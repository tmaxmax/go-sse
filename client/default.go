package client

import (
	"fmt"
	"net/http"
	"time"
)

var DefaultClient = &Client{
	HTTPClient:              http.DefaultClient,
	DefaultReconnectionTime: time.Second * 5,
	ResponseValidator: func(r *http.Response) error {
		if r.StatusCode != http.StatusOK {
			return fmt.Errorf("expected status code %d, received %d", http.StatusOK, r.StatusCode)
		}
		if rc, ec := r.Header.Get("Content-Type"), "text/event-stream"; rc != ec {
			return fmt.Errorf("expected content type %q, received %q", ec, rc)
		}
		return nil
	},
}

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
