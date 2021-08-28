package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/tmaxmax/go-sse/server"
	"github.com/tmaxmax/go-sse/server/event"
	"github.com/tmaxmax/go-sse/server/replay"
)

var sse = server.New(server.Config{
	ReplayProvider:   replay.NewValidProvider(true),
	ReplayGCInterval: time.Minute,
})

func configSSE(handler *server.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, q := r.Context(), r.URL.Query()
		ctx = server.SetLastEventID(ctx, event.ID(q.Get("lastEventID")))
		ctx = server.SetTopics(ctx, q["topic"])

		w.Header().Set("Access-Control-Allow-Origin", "*")

		handler.Broadcast(event.New(
			event.Name("connect"),
			event.Text(q.Get("name")+" has joined us!"),
		))

		handler.ServeHTTP(w, r.WithContext(ctx))

		handler.Broadcast(event.New(
			event.Name("disconnect"),
			event.Text(q.Get("name")+" left..."),
		))
	})
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	cancelSignal := make(chan os.Signal, 1)
	signal.Notify(cancelSignal, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-cancelSignal
		cancel()
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/", SnapshotHTTPEndpoint)
	mux.Handle("/events", configSSE(sse))

	s := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}
	s.RegisterOnShutdown(func() {
		// Broadcast a close message so clients can gracefully disconnect.
		sse.Broadcast(event.New(event.Name("close")))
		sse.Stop()
	})

	go sse.Start()

	go recordMetric(ctx, "ops", time.Second*2)
	go recordMetric(ctx, "cycles", time.Millisecond*500)

	go func() {
		duration := func() time.Duration {
			return time.Duration(2000+rand.Intn(1000)) * time.Millisecond
		}

		timer := time.NewTimer(duration())
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				count := 1 + rand.Intn(5)
				opts := []event.Option{event.TTL(time.Second * 30)}

				for i := 0; i < count; i += 1 {
					opts = append(opts, event.Text(strconv.FormatUint(rand.Uint64(), 10)))
				}

				sse.Broadcast(event.New(opts...), "numbers", server.DefaultTopic)
			case <-ctx.Done():
				return
			}

			timer.Reset(duration())
		}
	}()

	if err := runServer(ctx, s); err != nil {
		log.Println("server closed", err)
	}
}

func recordMetric(ctx context.Context, metric string, frequency time.Duration) {
	ticker := time.NewTicker(frequency)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			v := Inc(metric)
			ev := event.New(
				event.Name(metric),
				event.Text(strconv.FormatInt(v, 10)),
				event.TTL(frequency),
			)

			sse.Broadcast(ev, metric, server.DefaultTopic)
		case <-ctx.Done():
			return
		}
	}
}

func runServer(ctx context.Context, s *http.Server) error {
	shutdownError := make(chan error)

	go func() {
		<-ctx.Done()

		sctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		shutdownError <- s.Shutdown(sctx)
	}()

	if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return <-shutdownError
}
