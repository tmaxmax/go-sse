package server

import (
	"net/http"
	"time"

	"github.com/tmaxmax/go-sse/server/replay"

	"github.com/tmaxmax/go-sse/server/event"
)

// DefaultTopic is the topic that's implied when no topics are specified for
// broadcasts or connections.
const DefaultTopic = ""

var defaultTopicSlice = []string{DefaultTopic}

type broadcast struct {
	e      *event.Event
	topics []string
}

type conn = chan *event.Event
type connect struct {
	c           conn
	lastEventID event.ID
	topics      []string
}

type Handler struct {
	cfg *Config

	broadcasts  chan broadcast
	connections chan connect
	disconnects chan conn
	done        chan struct{}

	topics map[string]map[conn]struct{}
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
		broadcasts:  make(chan broadcast, cfg.BroadcastBufferSize),
		connections: make(chan connect),
		disconnects: make(chan conn),
		done:        make(chan struct{}),
		topics:      map[string]map[conn]struct{}{},
		replay:      cfg.ReplayProvider,
	}

	return h
}

func (h *Handler) topic(t string) map[conn]struct{} {
	if _, ok := h.topics[t]; !ok {
		h.topics[t] = map[conn]struct{}{}
	}
	return h.topics[t]
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
			h.replay.Append(&e.e, e.topics)

			seen := map[conn]struct{}{}

			for _, t := range e.topics {
				for c := range h.topics[t] {
					if _, ok := seen[c]; ok {
						continue
					}
					c <- e.e
					seen[c] = struct{}{}
				}
			}
		case c := <-h.connections:
			if c.lastEventID != "" {
				_ = h.replay.Range(c.lastEventID, c.topics, func(e *event.Event) {
					c.c <- e
				})
			}
			for _, t := range c.topics {
				h.topic(t)[c.c] = struct{}{}
			}
		case c := <-h.disconnects:
			close(c)
			for _, conns := range h.topics {
				delete(conns, c)
			}
		case <-gc:
			h.replay.GC()
		case <-h.done:
			for _, conns := range h.topics {
				for c := range conns {
					close(c)
				}
			}
			return
		}
	}
}

func (h *Handler) Stop() {
	close(h.done)
}

// Broadcast sends the given event to all the connections that are subscribed to the specified topics.
// If no topic is specified, the event will be sent to the default topic.
//
// The connections will receive the event only once, regardless of the number of topics that the
// message is sent to and the connection is subscribed to.
func (h *Handler) Broadcast(e *event.Event, topics ...string) {
	if len(topics) == 0 {
		topics = defaultTopicSlice
	}
	select {
	case h.broadcasts <- broadcast{e, topics}:
	case <-h.done:
	}
}

type contextKeyLastEventID struct{}
type contextKeyTopics struct{}

var (
	// ContextKeyLastEventID is used to set the last received event's ID for the request
	// from a custom source, if the Last-Event-ID header doesn't exist. You can use a
	// middleware for this that's executed before the handler.
	//
	// Valid values are of type string or event.ID.
	ContextKeyLastEventID = contextKeyLastEventID{}
	// ContextKeyTopics is used to tell the handler to which topics the client is
	// subscribed to. Set this in a middleware that's executed before the handler.
	// If no topics are provided, the client will still be subscribed to a default
	// topic (identified by the empty string).
	//
	// Valid values are of type string or []string. If a slice is provided, its
	// elements will be copied in order to prevent data races.
	ContextKeyTopics = contextKeyTopics{}
)

func (h *Handler) connect(r *http.Request) conn {
	lastEventID := event.ID(r.Header.Get("Last-Event-ID"))
	if lastEventID == "" {
		switch v := r.Context().Value(ContextKeyLastEventID).(type) {
		case string:
			lastEventID = event.ID(v)
		case event.ID:
			lastEventID = v
		}
	}

	topics := defaultTopicSlice
	switch v := r.Context().Value(ContextKeyTopics).(type) {
	case string:
		topics = []string{v}
	case []string:
		if len(v) == 0 {
			break
		}
		topics = make([]string, len(v))
		copy(topics, v)
	}

	c := make(conn, h.cfg.ConnectionBufferSize)
	h.connections <- connect{c, lastEventID, topics}

	go func() {
		select {
		case <-r.Context().Done():
		case <-h.done:
			return
		}

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

	h.cfg.OnConnect(w.Header(), r)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	flusher.Flush()

	for m := range h.connect(r) {
		_, err := m.WriteTo(w)
		flusher.Flush()
		if err != nil {
			h.cfg.OnWriteError(err)
			break
		}
	}

}
