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
)

var eventHandler = server.New()

func main() {
	cancel := make(chan struct{})
	cancelMetrics := make(chan struct{})
	cancelSignal := make(chan os.Signal)
	signal.Notify(cancelSignal, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-cancelMetrics:
			close(cancel)
		case <-cancelSignal:
			close(cancel)
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/stop", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(cancelMetrics)

		w.WriteHeader(http.StatusOK)
	}))
	mux.Handle("/", SnapshotHTTPEndpoint)
	mux.Handle("/events", eventHandler)

	s := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}

	go eventHandler.StartWithSignal(cancel)

	//go recordMetric("ops", time.Second*2, cancel)
	//go recordMetric("cycles", time.Millisecond*500, cancel)

	go func() {
		getDuration := func() time.Duration {
			return time.Duration(100+rand.Int63n(1400)) * time.Millisecond
		}

		var duration time.Duration

		for {
			select {
			case <-time.After(duration):
				var opts []event.Option

				count := 1 + rand.Intn(5)

				for i := 0; i < count; i += 1 {
					opts = append(opts, event.Text(strconv.FormatUint(rand.Uint64(), 10)))
				}

				duration = getDuration()
				opts = append(opts, event.TTL(duration))

				eventHandler.Broadcast(event.New(opts...))
			case <-cancel:
				return
			}

		}
	}()

	if err := runServer(s, cancel); err != nil {
		log.Println(err)
	}

	<-eventHandler.Done()
}

func recordMetric(metric string, frequency time.Duration, cancel <-chan struct{}) {
	for {
		select {
		case <-time.After(frequency):
			v := Inc(metric)
			ev := event.New(
				event.Name(metric),
				event.Text(strconv.FormatInt(v, 10)),
				event.TTL(frequency),
			)

			eventHandler.Broadcast(ev)
		case <-cancel:
			break
		}
	}
}

func runServer(s *http.Server, cancel <-chan struct{}) error {
	shutdownError := make(chan error)

	go func() {
		<-cancel
		shutdownError <- s.Shutdown(context.Background())
	}()

	if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return <-shutdownError
}
