package server

import (
	"log"
	"net/http"
	"sync/atomic"

	"github.com/tmaxmax/go-sse/pkg/hub"

	"github.com/tmaxmax/go-sse/server/event"
)

var connCount int64

type Handler struct {
	Headers    map[string]string
	CloseEvent *event.Event

	hub *hub.Hub
}

func New() *Handler {
	h := &Handler{}
	h.hub = hub.New(
		hub.OnHubStop(func(c *hub.Connection) bool {
			c.Send(h.CloseEvent)

			return true
		}),
	)
	return h
}

func (h *Handler) StartWithSignal(cancel <-chan struct{}) {
	go h.hub.Start()

	<-cancel

	h.hub.Stop()
}

func (h *Handler) Broadcast(ev *event.Event) {
	h.hub.Broadcast(ev)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()

	if h.Headers != nil {
		for key, value := range h.Headers {
			header.Set(key, value)
		}
	}

	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("Transfer-Encoding", "chunked")

	var c *hub.Connection

	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()

		c = hub.NewConnection(w)
	}

	if c == nil {
		http.Error(w, "Server-sent events are not supported", http.StatusBadRequest)

		return
	}

	c.AttachTo(h.hub, atomic.AddInt64(&connCount, 1))

	go func() {
		<-r.Context().Done()

		c.Close()

		atomic.AddInt64(&connCount, -1)
	}()

	log.Println(c.Receive())
}
