package main

import (
	"context"
	"fmt"
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
)

var eventHandler = server.New()

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	cancelSignal := make(chan os.Signal)
	signal.Notify(cancelSignal, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-cancelSignal
		cancel()
	}()

	mux := http.NewServeMux()
	mux.Handle("/stop", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(http.StatusOK)
	}))
	mux.Handle("/", SnapshotHTTPEndpoint)
	mux.Handle("/events", eventHandler)

	s := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}

	go eventHandler.Start(ctx)

	go recordMetric(ctx, "ops", time.Second*2)
	go recordMetric(ctx, "cycles", time.Millisecond*500)

	go func() {
		getDuration := func() time.Duration {
			return time.Duration(2000+rand.Intn(1000)) * time.Millisecond
		}

		var duration time.Duration
		var id int

		for {
			select {
			case <-time.After(duration):
				id++

				count := 1 + rand.Intn(5)
				opts := []event.Option{
					event.ID(strconv.Itoa(id)),
				}

				for i := 0; i < count; i += 1 {
					opts = append(opts, event.Text(strconv.FormatUint(rand.Uint64(), 10)))
				}

				duration = getDuration()
				opts = append(opts, event.TTL(duration))

				eventHandler.Broadcast(ctx, event.New(opts...))
			case <-ctx.Done():
				return
			}

		}
	}()

	if err := runServer(ctx, s); err != nil {
		log.Println("server closed", err)
	}
}

func recordMetric(ctx context.Context, metric string, frequency time.Duration) {
	var id int

	for {
		select {
		case <-time.After(frequency):
			id++

			v := Inc(metric)
			ev := event.New(
				event.Name(metric),
				event.ID(fmt.Sprintf("%s-%d", metric, id)),
				event.Text(strconv.FormatInt(v, 10)),
				event.TTL(frequency),
			)

			eventHandler.Broadcast(ctx, ev)
		case <-ctx.Done():
			break
		}
	}
}

func runServer(ctx context.Context, s *http.Server) error {
	shutdownError := make(chan error)

	go func() {
		<-ctx.Done()
		shutdownError <- s.Shutdown(context.Background())
	}()

	if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return <-shutdownError
}
