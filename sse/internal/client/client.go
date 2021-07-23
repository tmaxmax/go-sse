package client

import (
	"io"
	"net/http"
)

type Client struct {
	messages chan io.WriterTo
	writer   io.Writer
	flusher  http.Flusher
	id       string
}

func New(writer io.Writer, flusher http.Flusher, id string) *Client {
	return &Client{
		messages: make(chan io.WriterTo),
		writer:   writer,
		flusher:  flusher,
		id:       id,
	}
}

func (c *Client) Send(message io.WriterTo) {
	c.messages <- message
}

func (c *Client) ID() string {
	return c.id
}

func (c *Client) Receive(cancel <-chan struct{}) error {
	if c.writer == nil {
		return nil
	}

	for {
		select {
		case <-cancel:
			return nil
		case message := <-c.messages:
			if _, err := message.WriteTo(c.writer); err != nil {
				return err
			}

			if c.flusher != nil {
				c.flusher.Flush()
			}
		}
	}
}

// TODO: Close method?
