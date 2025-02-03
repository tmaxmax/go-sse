package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
)

type loggerCtxKey struct{}

// withLogger is an http middleware that generates a logger with request-specific fields
// added to it and attaches it to the request context for later retrieval with getLogger().
// This is simmilar to how existing packages like https://github.com/go-chi/httplog works.
func withLogger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := slog.Default().With(

			"UserAgent", r.UserAgent(),
			"RemoteAddr", r.RemoteAddr,
			"Host", r.Host,
			"Origin", r.Header.Get("origin"),
		)
		r = r.WithContext(context.WithValue(r.Context(), loggerCtxKey{}, l))
		h.ServeHTTP(w, r)
	})
}

// getLogger retrieves the request-specific logger from a request's context. This is
// similar to how existing packages like https://github.com/go-chi/httplog work.
func getLogger(r *http.Request) (*slog.Logger, error) {
	logger, ok := r.Context().Value(loggerCtxKey{}).(*slog.Logger)
	if !ok {
		return nil, errors.New("logger not available on request")
	}

	return logger, nil
}

// setupLogger just initializes the global logger in whatever way you might prefer. We are
// keeping it simple for this example.
func setupLogger() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(h)
	slog.SetDefault(logger)
}
