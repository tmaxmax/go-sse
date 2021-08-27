package server

import (
	"net/http"
	"time"

	"github.com/tmaxmax/go-sse/server/replay"

	"github.com/tmaxmax/go-sse/server/event"
)

type conn = chan *event.Event

type connect struct {
	c           conn
	lastEventID event.ID
}

type Handler struct {
	cfg *Config

	broadcasts  chan *event.Event
	connections chan connect
	disconnects chan conn
	done        chan struct{}

	conns  map[conn]struct{}
	replay replay.Provider
}

func New(config ...Config) *Handler {
	cfg := &Config{}
	if len(config) > 0 {
		*cfg = config[0]
	}
	mergeWithDefault(cfg)

	h := &Handler{
		cfg:         cfg,
		broadcasts:  make(chan *event.Event, cfg.BroadcastBufferSize),
		connections: make(chan connect),
		disconnects: make(chan conn),
		done:        make(chan struct{}),
		conns:       map[conn]struct{}{},
		replay:      cfg.ReplayProvider,
	}

	return h
}

func (h *Handler) getGCTicker() (<-chan time.Time, func()) {
	if h.cfg.ReplayGCInterval <= 0 {
		return nil, func() {}
	}

	t := time.NewTicker(h.cfg.ReplayGCInterval)
	return t.C, t.Stop
}

func (h *Handler) Start() {
	gc, stop := h.getGCTicker()
	defer stop()

	for {
		select {
		case e := <-h.broadcasts:
			h.replay.Append(&e)
			for c := range h.conns {
				c <- e
			}
		case c := <-h.connections:
			if c.lastEventID != "" {
				_ = h.replay.Range(c.lastEventID, func(e *event.Event) {
					c.c <- e
				})
			}
			h.conns[c.c] = struct{}{}
		case c := <-h.disconnects:
			close(c)
			delete(h.conns, c)
		case <-gc:
			h.replay.GC()
		case <-h.done:
			for c := range h.conns {
				close(c)
			}
			return
		}
	}
}

func (h *Handler) Stop() {
	close(h.done)
}

func (h *Handler) Broadcast(e *event.Event) {
	select {
	case h.broadcasts <- e:
	case <-h.done:
	}
}

type contextKey string

func (c contextKey) String() string { return "go-sse.server: " + string(c) }

const (
	// ContextKeyLastEventID is used to set the last received event's ID for the request
	// from a custom source, if the Last-Event-ID header doesn't exist. You can use a
	// middleware for this that's executed before the handler.
	ContextKeyLastEventID = contextKey("Last-Event-ID")
)

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) conn {
	h.cfg.OnConnect(w.Header(), r)

	lastEventID := event.ID(r.Header.Get("Last-Event-ID"))
	if lastEventID == "" {
		switch v := r.Context().Value(ContextKeyLastEventID).(type) {
		case string:
			lastEventID = event.ID(v)
		case event.ID:
			lastEventID = v
		}
	}

	c := make(conn, h.cfg.ConnectionBufferSize)
	h.connections <- connect{c, lastEventID}

	go func() {
		<-r.Context().Done()

		select {
		case h.disconnects <- c:
			h.cfg.OnDisconnect(r)
		case <-h.done:
		}
	}()

	return c
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Server-sent events are not supported", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	flusher.Flush()

	for m := range h.connect(w, r) {
		_, err := m.WriteTo(w)
		flusher.Flush()
		if err != nil {
			h.cfg.OnWriteError(err)
			break
		}
	}

}
