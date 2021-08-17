package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/tmaxmax/go-sse/pkg/hub"

	"github.com/tmaxmax/go-sse/server/event"
)

type Handler struct {
	hub *hub.Hub
}

func New() *Handler {
	return &Handler{
		hub: &hub.Hub{
			OnAttach: func(attached *hub.Conn, c *hub.Conn) {
				name := attached.Context().Value("name").(string)
				message := fmt.Sprintf("Hello there, %s!\nNice to see you.", name)
				msg := event.New(event.Name("welcome"), event.Text(message))

				_ = c.Send(attached.Context(), msg)
			},
			OnDetach: func(detached *hub.Conn, c *hub.Conn) {
				name := detached.Context().Value("name").(string)
				message := fmt.Sprintf("So sad %s left.\nThey will be missed!", name)
				msg := event.New(event.Name("goodbye"), event.Text(message))

				_ = c.Send(context.Background(), msg)
			},
			OnHubStop: func(c *hub.Conn) {
				msg := event.New(event.Name("close"), event.Text("Goodbye!\nWe're done here"))

				_ = c.Send(context.Background(), msg)
			},
			Log: log.New(os.Stderr, "HUB -> ", log.Ltime|log.Lmsgprefix),
		},
	}
}

func (h *Handler) Start(ctx context.Context) {
	h.hub.Start(ctx)
}

func (h *Handler) Broadcast(ctx context.Context, ev *event.Event) {
	_ = h.hub.Broadcast(ctx, ev)
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

	c := &hub.Conn{Log: log.New(os.Stderr, name+": ", log.Lmsgprefix|log.Ltime)}
	_ = c.AttachTo(h.hub)
	messages, _ := c.Receive(context.WithValue(r.Context(), "name", name))

	for m := range messages {
		if err := m.(*event.Event).Message(w); err != nil {
			break
		}

		flusher.Flush()

		time.Sleep(time.Duration(timeout) * time.Second)
	}
}
