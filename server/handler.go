package server

import (
	"net/http"

	"github.com/tmaxmax/go-sse/pkg/hub"

	"github.com/tmaxmax/go-sse/server/event"
)

type Handler struct {
	hub *hub.Hub
}

func New() *Handler {
	return &Handler{
		hub: &hub.Hub{
			OnAttach: func(new hub.AttachedConnection, other hub.AttachedConnection) {
				msg := event.New(
					event.ID("WELCOME"),
					event.Text("Hello there, "+new.Information().(string)+"!\nNice to see you."),
				)
				other.Send(msg)
			},
			OnDetach: func(detached interface{}, c hub.AttachedConnection) {
				msg := event.New(
					event.ID("GOODBYE"),
					event.Text("So sad "+detached.(string)+"left.\nHe will be missed!"),
				)
				c.Send(msg)
			},
			OnHubStop: func(c hub.AttachedConnection) {
				msg := event.New(
					event.ID("CLOSE"),
					event.Text("Goodbye!\nWe're done here"),
				)
				c.Send(msg)
			},
		},
	}
}

func (h *Handler) StartWithSignal(cancel <-chan struct{}) {
	go h.hub.Start()
	<-cancel
	h.hub.Stop()
}

func (h *Handler) Broadcast(ev *event.Event) {
	h.hub.Broadcast(ev)
}

func (h *Handler) Done() <-chan struct{} {
	return h.hub.Done()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("Transfer-Encoding", "chunked")

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	} else {
		http.Error(w, "Server-sent events are not supported", http.StatusBadRequest)
		return
	}

	c := &hub.Connection{}
	c.Writer = w
	c.Info = r.URL.Query().Get("name")
	c.AttachTo(h.hub)

	go func() {
		<-r.Context().Done()

		c.Close()
	}()

	_ = c.Receive()
}
