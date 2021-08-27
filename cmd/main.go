package main

import (
	"context"
	"log"
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

var eventHandler *server.Handler

func setLastEventID(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("lastEventID")
		ctx := context.WithValue(r.Context(), server.ContextKeyLastEventID, id)

		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func init() {
	eventHandler = server.New(server.Config{
		OnConnect: func(header http.Header, r *http.Request) {
			header.Set("Access-Control-Allow-Origin", "*")

			name := r.URL.Query().Get("name")
			fields := []event.Option{
				event.Name("connect"),
			}

			if name == "" {
				fields = append(fields, event.Text("Someone unknown joined us!"))
			} else {
				fields = append(fields, event.Text(name+" joined us!"))
			}

			eventHandler.Broadcast(event.New(fields...))
		},
		OnDisconnect: func(r *http.Request) {
			name := r.URL.Query().Get("name")
			fields := []event.Option{
				event.Name("disconnect"),
			}

			if name == "" {
				fields = append(fields, event.Text("Someone unknown left us..."))
			} else {
				fields = append(fields, event.Text(name+" left us..."))
			}

			eventHandler.Broadcast(event.New(fields...))
		},
		ReplayProvider:   replay.NewFiniteProvider(50, true),
		ReplayGCInterval: time.Minute,
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
	mux.Handle("/events", setLastEventID(eventHandler))

	s := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}
	s.RegisterOnShutdown(eventHandler.Stop)

	go eventHandler.Start()

	go recordMetric(ctx, "ops", time.Second*2)
	go recordMetric(ctx, "cycles", time.Millisecond*500)

	//go func() {
	//	duration := func() time.Duration {
	//		return time.Duration(2000+rand.Intn(1000)) * time.Millisecond
	//	}
	//
	//	timer := time.NewTimer(duration())
	//	defer timer.Stop()
	//
	//	for {
	//		select {
	//		case <-timer.C:
	//			count := 1 + rand.Intn(5)
	//			opts := []event.Option{event.TTL(time.Second * 30)}
	//
	//			for i := 0; i < count; i += 1 {
	//				opts = append(opts, event.Text(strconv.FormatUint(rand.Uint64(), 10)))
	//			}
	//
	//			eventHandler.Broadcast(event.New(opts...))
	//		case <-ctx.Done():
	//			return
	//		}
	//
	//		timer.Reset(duration())
	//	}
	//}()

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

			eventHandler.Broadcast(ev)
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
