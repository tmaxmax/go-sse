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

// Stop signals the handler to close all connections and abort all broadcasts.
// Calling Stop does not abruptly end the HTTP request handlers, it only closes the
// channels it sends the events on. Any ongoing writes to the connections will
// complete and the handlers will return successfully. Calling Stop will not trigger
// the OnDisconnect handler.
//
// Register Stop as a http.Server shutdown function, otherwise Shutdown will hang
// indefinitely or close the connections abruptly (if a cancelable context is provided).
func (h *Handler) Stop() {
	close(h.done)
}

// Broadcast sends the given event to all the connections that are subscribed to the specified topics.
// If no topic is specified, the event will be sent to the default topic.
//
// The connections will receive the event only once, even if it is subscribed to more than one of the
// topics the event is sent to.
//
// The broadcast is aborted if Stop is called.
func (h *Handler) Broadcast(e *event.Event, topics ...string) {
	if len(topics) == 0 {
		topics = defaultTopicSlice
	}
	select {
	case h.broadcasts <- broadcast{e, topics}:
	case <-h.done:
	}
}

func (h *Handler) connect(r *http.Request) conn {
	c := make(conn, h.cfg.ConnectionBufferSize)
	connRequest := connect{c, getLastEventID(r), getTopics(r)}

	select {
	case h.connections <- connRequest:
		return c
	case <-h.done:
		return nil
	}
}

func (h *Handler) watchDisconnect(c conn, r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-h.done:
		return
	}

	select {
	case h.disconnects <- c:
	case <-h.done:
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
		http.Error(w, "Event stream closed", http.StatusInternalServerError)
		return
	}
	go h.watchDisconnect(conn, r)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	flusher.Flush()

	for m := range conn {
		_, err := m.WriteTo(w)
		flusher.Flush()
		if err != nil {
			h.cfg.OnWriteError(err)
			break
		}
	}
}
