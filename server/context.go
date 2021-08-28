package server

import (
	"context"
	"net/http"

	"github.com/tmaxmax/go-sse/server/event"
)

// SetLastEventID returns a context with a custom context key set to the provided event ID.
// Replace the request's original context in a middleware with the returned one to tell the
// handler to use this value as the last seen event ID when the Last-Event-ID header is not
// found.
func SetLastEventID(ctx context.Context, id event.ID) context.Context {
	return context.WithValue(ctx, ctxLastEventID, id)
}

// SetTopics returns a context with a custom context key set to the provided slice of topic identifiers.
// Replace the request's original context in a middleware with the returned one to tell the
// handler to subscribe the new connection only to these topics.
//
// If no topics are set for a request, the connection is subscribed to the default topic.
func SetTopics(ctx context.Context, topics []string) context.Context {
	return context.WithValue(ctx, ctxTopics, topics)
}

type contextKey int

func (c contextKey) String() string {
	return "go-sse.server: " + ctxKeysRepresentation[c]
}

const (
	ctxLastEventID contextKey = iota
	ctxTopics
)

var ctxKeysRepresentation = map[contextKey]string{
	ctxLastEventID: "Last-Event-ID",
	ctxTopics:      "topics",
}

func getLastEventID(r *http.Request) event.ID {
	id := event.ID(r.Header.Get("Last-Event-ID"))
	if id != "" {
		if v, ok := r.Context().Value(ctxLastEventID).(event.ID); ok {
			id = v
		}
	}
	return id
}

func getTopics(r *http.Request) []string {
	topics := defaultTopicSlice
	if v, ok := r.Context().Value(ctxTopics).([]string); ok && len(v) > 0 {
		topics = v
	}
	return topics
}
