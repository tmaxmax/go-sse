package server

import (
	"net/http"
	"sync"

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

func (h *Handler) connect(_ *http.Request) hub.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return nil
	}

	conn := make(hub.Conn)
	h.hub <- conn
	return conn
}

func (h *Handler) watchDisconnect(c hub.Conn, r *http.Request) {
	<-r.Context().Done()

	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.closed {
		h.hub <- hub.DisconnectAll(c)
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()

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

	conn := h.connect(r)
	if conn == nil {
		http.Error(w, "Server closed", http.StatusInternalServerError)
		return
	}
	go h.watchDisconnect(conn, r)

	for m := range conn {
		if _, err := m.(*event.Event).WriteTo(w); err != nil {
			break
		}

		flusher.Flush()
	}
}
