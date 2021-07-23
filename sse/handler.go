package sse

import (
	"context"
	"fmt"
	"go-metrics/sse/internal/client"
	"log"
	"net/http"
)

type Configuration struct {
	Headers      map[string]string
	CloseMessage Messager
}

type Handler struct {
	notifier       chan *Message
	newClients     chan *client.Client
	closingClients chan *client.Client
	clients        map[*client.Client]struct{}

	configuration *Configuration
	closer        <-chan struct{}
}

func New(configuration *Configuration) *Handler {
	return &Handler{
		notifier:       make(chan *Message, 1),
		newClients:     make(chan *client.Client),
		closingClients: make(chan *client.Client),
		clients:        make(map[*client.Client]struct{}),
		configuration:  configuration,
		// Will be set on start
		closer: nil,
	}
}

func (h *Handler) Send(message Messager) error {
	raw, err := message.Message()
	if err != nil {
		return err
	}

	h.notifier <- raw

	return nil
}

func (h *Handler) Start(ctx context.Context) error {
	if ctx == nil {
		return h.StartWithSignal(nil)
	} else {
		return h.StartWithSignal(ctx.Done())
	}
}

func (h *Handler) StartWithSignal(cancel <-chan struct{}) (err error) {
	var closeMessage *Message
	if h.configuration != nil && h.configuration.CloseMessage != nil {
		closeMessage, err = h.configuration.CloseMessage.Message()
		if err != nil {
			return
		}

		log.Println("Closing message is", closeMessage.String())
	}

	h.closer = mergeCancelChannels(cancel, nil)

	for {
		select {
		case c := <-h.newClients:
			h.clients[c] = struct{}{}

			log.Println("Received client", c.ID())
		case c := <-h.closingClients:
			delete(h.clients, c)

			log.Println("Removed client", c.ID())

			select {
			case <-h.closer:
				log.Println("There")

				if closeMessage != nil {
					c.Send(closeMessage)

					log.Println("Sent closing message")
				}

				if len(h.clients) == 0 {
					log.Println("Exiting event stream handler")

					return
				}
			default:
				log.Println("Here")
			}
		case m := <-h.notifier:
			log.Printf("Received message: %q", m)

			for c, _ := range h.clients {
				c.Send(m)

				log.Println("Message sent to client", c.ID())
			}
		}
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := h.clientFromRequest(w, r)
	if c == nil {
		http.Error(w, "Server-sent events are not supported", http.StatusBadRequest)

		return
	}

	h.newClients <- c

	defer func() {
		h.closingClients <- c
	}()

	_ = c.Receive(mergeCancelChannels(h.closer, r.Context().Done()))
}

func (h *Handler) clientFromRequest(w http.ResponseWriter, r *http.Request) *client.Client {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil
	}

	if h.configuration != nil {
		for key, value := range h.configuration.Headers {
			r.Header.Set(key, value)
		}
	}

	r.Header.Set("Content-Type", "text/event-stream")
	r.Header.Set("Cache-Control", "no-cache")
	r.Header.Set("Connection", "keep-alive")

	return client.New(w, flusher, fmt.Sprintf("%s: %s", r.RemoteAddr, r.UserAgent()))
}

func mergeCancelChannels(a, b <-chan struct{}) <-chan struct{} {
	cancel := make(chan struct{})

	go func() {
		select {
		case <-a:
			close(cancel)
		case <-b:
			close(cancel)
		}
	}()

	return cancel
}
