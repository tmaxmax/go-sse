package server

import (
	"log"
	"net/http"

	"github.com/tmaxmax/hub"

	"github.com/tmaxmax/go-sse/server/event"
)

type Handler struct {
	hub hub.Hub
	// OnWriteError is a function that's called when writing an event to the connection fails.
	// Defaults to log.Println, you can use it for custom logging or any other desired error handling.
	OnWriteError func(error)
}

func New() *Handler {
	return &Handler{
		hub: make(hub.Hub),
		OnWriteError: func(err error) {
			log.Println("go-sse.server: write error to connection:", err)
		},
	}
}

func (h *Handler) Start() {
	h.hub.Start()
}

func (h *Handler) Stop() {
	close(h.hub)
}

func (h *Handler) Broadcast(ev *event.Event) {
	h.hub <- ev
}

func (h *Handler) connect(r *http.Request) hub.Conn {
	conn := make(hub.Conn)

	go func() {
		defer func() {
			_ = recover()
		}()

		<-r.Context().Done()
		h.hub <- hub.Disconnect{Conn: conn}
	}()

	h.hub <- conn
	return conn
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Server-sent events are not supported", http.StatusBadRequest)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	flusher.Flush()

	for m := range h.connect(r) {
		_, err := m.(*event.Event).WriteTo(w)
		flusher.Flush()
		if err != nil {
			h.OnWriteError(err)
			break
		}
	}
}
