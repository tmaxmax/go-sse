package internal

import (
	"io"
	"net/http"
)

type Client struct {
	messages chan io.WriterTo
	writer   io.Writer
	flusher  http.Flusher
}

func NewClient(writer io.Writer, flusher http.Flusher) *Client {
	return &Client{
		messages: make(chan io.WriterTo),
		writer:   writer,
		flusher:  flusher,
	}
}

func (c *Client) Close() {
	close(c.messages)
}

func (c *Client) Send(message io.WriterTo) {
	if message == nil {
		panic("sse.internal.client: message sent to client can't be nil")
	}

	c.messages <- message
}

func (c *Client) Receive() error {
	if c.writer == nil {
		return nil
	}

	c.flush()

	for {
		message := <-c.messages

		if message == nil {
			return nil
		}

		if _, err := message.WriteTo(c.writer); err != nil {
			return err
		}

		c.flush()
	}
}

func (c *Client) flush() {
	if c.flusher != nil {
		c.flusher.Flush()
	}
}
