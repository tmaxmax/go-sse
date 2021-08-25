package server

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tmaxmax/hub"

	"github.com/tmaxmax/go-sse/server/event"
)

type Handler struct {
	hub    hub.Hub
	closed bool
	mu     sync.RWMutex
}

func New() *Handler {
	return &Handler{
		hub: make(hub.Hub, 1),
	}
}

func (h *Handler) Start() {
	h.hub.Start()
}

func (h *Handler) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.closed = true
	h.hub <- event.New(event.Name("close"), event.Text("Goodbye!\nWe're done here"))
	close(h.hub)
}

func (h *Handler) Broadcast(ev *event.Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return
	}
	h.hub <- ev
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	query := r.URL.Query()

	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("Transfer-Encoding", "chunked")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Server-sent events are not supported", http.StatusBadRequest)
		return
	}

	flusher.Flush()

	name := query.Get("name")
	timeout, _ := strconv.Atoi(query.Get("timeout"))
	attachMessage := event.New(
		event.Name("welcome"),
		event.Text(fmt.Sprintf("Hello there, %s!\nNice to see you.", name)),
	)

	conn := make(hub.Conn)

	go func() {
		<-r.Context().Done()

		h.mu.RLock()
		defer h.mu.RUnlock()

		if !h.closed {
			h.hub <- hub.DisconnectAll(conn)
		}
	}()

	h.hub <- attachMessage
	h.hub <- conn

	for m := range conn {
		if _, err := m.(*event.Event).WriteTo(w); err != nil {
			break
		}

		flusher.Flush()

		time.Sleep(time.Duration(timeout) * time.Second)
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return
	}

	detachMessage := event.New(
		event.Name("goodbye"),
		event.Text(fmt.Sprintf("So sad %s left.\nThey will be missed!", name)),
	)
	h.hub <- detachMessage
}
