package server

import (
	"context"
	"log"
	"net/http"

	. "github.com/tmaxmax/go-sse/server/internal/client"
)

type Configuration struct {
	Headers    map[string]string
	CloseEvent *Event
}

type Handler struct {
	notifier       chan *Event
	newClients     chan *Client
	closingClients chan *Client
	clients        map[*Client]struct{}

	configuration *Configuration
	closer        <-chan struct{}
}

func NewHandler(configuration *Configuration) *Handler {
	return &Handler{
		notifier:       make(chan *Event, 1),
		newClients:     make(chan *Client),
		closingClients: make(chan *Client),
		clients:        make(map[*Client]struct{}),
		configuration:  configuration,
		// Will be set on start
		closer: nil,
	}
}

func (h *Handler) Send(ev *Event) {
	if ev == nil {
		return
	}

	h.notifier <- ev
}

func (h *Handler) Start(ctx context.Context) {
	if ctx == nil {
		h.StartWithSignal(nil)
	} else {
		h.StartWithSignal(ctx.Done())
	}
}

func (h *Handler) StartWithSignal(cancel <-chan struct{}) {
	var closeEvent *Event
	if cfg := h.configuration; cfg != nil {
		closeEvent = cfg.CloseEvent
	}

	for {
		select {
		case c := <-h.newClients:
			h.clients[c] = struct{}{}
		case c := <-h.closingClients:
			h.removeClient(c, nil)
		case m := <-h.notifier:
			for c, _ := range h.clients {
				c.Send(m)
			}
		case <-cancel:
			for c, _ := range h.clients {
				h.removeClient(c, closeEvent)
			}

			return
		}
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := h.subscribe(w, r)
	if c == nil {
		http.Error(w, "Server-sent events are not supported", http.StatusBadRequest)

		return
	}

	h.newClients <- c

	go func() {
		<-r.Context().Done()

		h.closingClients <- c
	}()

	log.Println(c.Receive())
}

func (h *Handler) subscribe(w http.ResponseWriter, _ *http.Request) *Client {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil
	}

	header := w.Header()

	if h.configuration != nil {
		for key, value := range h.configuration.Headers {
			header.Set(key, value)
		}
	}

	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("Transfer-Encoding", "chunked")

	return NewClient(w, flusher)
}

func (h *Handler) removeClient(c *Client, ev *Event) {
	if ev != nil {
		c.Send(ev)
	}

	c.Close()

	delete(h.clients, c)
}
