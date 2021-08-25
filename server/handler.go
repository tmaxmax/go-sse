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
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Server-sent events are not supported", http.StatusBadRequest)
		return
	}

	conn := h.connect(r)
	if conn == nil {
		http.Error(w, "Server closed", http.StatusInternalServerError)
		return
	}
	go h.watchDisconnect(conn, r)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	for m := range conn {
		flusher.Flush()

		if _, err := m.(*event.Event).WriteTo(w); err != nil {
			break
		}
	}

	flusher.Flush()
}
