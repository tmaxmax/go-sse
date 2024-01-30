package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/tmaxmax/go-sse"
)

type logger struct {
	l *log.Logger
}

// Log is a logger for sse.Server. It logs events like so:
//
//	level=INFO msg="A message about the logged event"
//	  ua="the user agent" ip="the RemoteAddr" host="the host" origin="the origin"
//	  lastEventID=5 topics=""
//
// It expects that the given context contains a value mapped to loggerCtxKey{},
// which will happen because sse.Server passes the request context and we set
// a middleware up to add these to the context.
func (l logger) Log(ctx context.Context, level sse.LogLevel, msg string, data map[string]any) {
	levelStr := ""
	switch level {
	case sse.LogLevelInfo:
		levelStr = "INFO"
	case sse.LogLevelWarn:
		levelStr = "WARN"
	case sse.LogLevelError:
		levelStr = "ERROR"
	}

	output := fmt.Sprintf("level=%s msg=%q", levelStr, msg)
	if e, ok := data["err"]; ok {
		output += fmt.Sprintf(" err=%q", e.(error).Error())
	}

	delete(data, "err")

	output += "\n  "

	for k, v := range ctx.Value(loggerCtxKey{}).(map[string]string) {
		output += fmt.Sprintf(" %s=%q", k, v)
	}

	output += "\n  "

	for k, v := range data {
		switch s := v.(type) {
		case sse.EventID: // key is "lastEventID"
			output += fmt.Sprintf(" %s=%q", k, s)
		case []string: // key is "topics"
			output += fmt.Sprintf(" %s=%q", k, strings.Join(s, ", "))
		default:
			panic(fmt.Errorf("unexpected value type %T", v))
		}
	}

	l.l.Println(output)
}

type loggerCtxKey struct{}

func withLogger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), loggerCtxKey{}, map[string]string{
			"ua":     r.UserAgent(),
			"ip":     r.RemoteAddr,
			"host":   r.Host,
			"origin": r.Header.Get("Origin"),
		}))
		h.ServeHTTP(w, r)
	})
}
