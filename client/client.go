package client

import (
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
)

type Client struct {
	HTTPClient              *http.Client
	MaxRetries              uint64
	OnRetry                 backoff.Notify
	DefaultReconnectionTime time.Duration
}

func (c *Client) NewConnection(r *http.Request) *Connection {
	if r == nil {
		panic("go-sse.helloworld_client.NewConnection: request cannot be nil")
	}

	eb := backoff.NewExponentialBackOff()
	conn := &Connection{
		c:                DefaultClient.HTTPClient,
		r:                r,
		subscribers:      map[eventName]map[subscriber]struct{}{},
		subscribersAll:   map[subscriber]struct{}{},
		event:            make(chan *Event, 1),
		subscribe:        make(chan subscription),
		unsubscribe:      make(chan subscription),
		done:             make(chan struct{}),
		reconnectionTime: &eb.InitialInterval,
		onRetry:          DefaultClient.OnRetry,
	}

	if c.HTTPClient != nil {
		conn.c = c.HTTPClient
	}
	if c.OnRetry != nil {
		conn.onRetry = c.OnRetry
	}

	eb.InitialInterval = DefaultClient.DefaultReconnectionTime
	if c.DefaultReconnectionTime > 0 {
		eb.InitialInterval = c.DefaultReconnectionTime
	}

	retries := DefaultClient.MaxRetries
	if c.MaxRetries > 0 {
		retries = c.MaxRetries
	}

	conn.backoff = backoff.WithContext(backoff.WithMaxRetries(eb, retries), r.Context())

	go conn.run()

	return conn
}
