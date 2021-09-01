package client

import (
	"net/http"
	"time"
)

var DefaultClient = &Client{
	HTTPClient:              http.DefaultClient,
	MaxRetries:              3,
	DefaultReconnectionTime: time.Second * 5,
}

func NewConnection(r *http.Request) *Connection {
	return DefaultClient.NewConnection(r)
}
