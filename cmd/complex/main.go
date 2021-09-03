package main

import (
	"context"
	"errors"
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

var sse = server.New()

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/stop", func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/", SnapshotHTTPEndpoint)
	mux.Handle("/events", sse)

	s := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}
	s.RegisterOnShutdown(func() {
		// Broadcast a close message so clients can gracefully disconnect.
		_ = sse.Publish(event.New(event.Name("close")))
		_ = sse.Shutdown()
	})

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
				_ = sse.Publish(generateRandomNumbers())
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
			)
			ev.SetTTL(frequency)

			_ = sse.Publish(ev)
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

	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return <-shutdownError
}

type randomNumbers struct {
	numbers []uint64
}

func (r *randomNumbers) MarshalEvent() *event.Event {
	var buf []byte

	for _, n := range r.numbers {
		buf = strconv.AppendUint(buf, n, 10)
		buf = append(buf, '\n')
	}

	e := event.New(event.Raw(buf))
	e.SetTTL(time.Second * 30)
	return e
}

func generateRandomNumbers() *randomNumbers {
	count := 1 + rand.Intn(5)

	r := &randomNumbers{numbers: make([]uint64, 0, count)}

	for i := 0; i < count; i++ {
		r.numbers = append(r.numbers, rand.Uint64())
	}

	return r
}
