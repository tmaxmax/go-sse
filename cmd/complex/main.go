package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/tmaxmax/go-sse"
)

const (
	topicRandomNumbers = "numbers"
	topicMetrics       = "metrics"
)

func newSSE() *sse.Server {
	rp, _ := sse.NewValidReplayProvider(time.Minute*5, true)
	rp.GCInterval = time.Minute

	return &sse.Server{
		Provider: &sse.Joe{ReplayProvider: rp},
		Logger:   logger{log.New(os.Stderr, "", 0)},
		OnSession: func(s *sse.Session) (sse.Subscription, bool) {
			topics := s.Req.URL.Query()["topic"]
			for _, topic := range topics {
				if topic != topicRandomNumbers && topic != topicMetrics {
					fmt.Fprintf(s.Res, "invalid topic %q; supported are %q, %q", topic, topicRandomNumbers, topicMetrics)
					s.Res.WriteHeader(http.StatusBadRequest)
					return sse.Subscription{}, false
				}
			}
			if len(topics) == 0 {
				// Provide default topics, if none are given.
				topics = []string{topicRandomNumbers, topicMetrics}
			}

			return sse.Subscription{
				Client:      s,
				LastEventID: s.LastEventID,
				Topics:      append(topics, sse.DefaultTopic), // the shutdown message is sent on the default topic
			}, true
		},
	}
}

func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h.ServeHTTP(w, r)
	})
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sseHandler := newSSE()

	mux := http.NewServeMux()
	mux.HandleFunc("/stop", func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/", SnapshotHTTPEndpoint)
	mux.Handle("/events", sseHandler)

	s := &http.Server{
		Addr:              "0.0.0.0:8080",
		Handler:           cors(withLogger(mux)),
		ReadHeaderTimeout: time.Second * 10,
	}
	s.RegisterOnShutdown(func() {
		e := &sse.Message{Type: sse.Type("close")}
		// Adding data is necessary because spec-compliant clients
		// do not dispatch events without data.
		e.AppendData("bye")
		// Broadcast a close message so clients can gracefully disconnect.
		_ = sseHandler.Publish(e)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		// We use a context with a timeout so the program doesn't wait indefinitely
		// for connections to terminate. There may be misbehaving connections
		// which may hang for an unknown timespan, so we just stop waiting on Shutdown
		// after a certain duration.
		_ = sseHandler.Shutdown(ctx)
	})

	go recordMetric(ctx, sseHandler, "ops", time.Second*2)
	go recordMetric(ctx, sseHandler, "cycles", time.Millisecond*500)

	go func() {
		duration := func() time.Duration {
			return time.Duration(2000+rand.Intn(1000)) * time.Millisecond
		}

		timer := time.NewTimer(duration())
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				_ = sseHandler.Publish(generateRandomNumbers(), topicRandomNumbers)
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

func recordMetric(ctx context.Context, sseHandler *sse.Server, metric string, frequency time.Duration) {
	ticker := time.NewTicker(frequency)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			v := Inc(metric)

			e := &sse.Message{
				Type: sse.Type(metric),
			}
			e.AppendData(strconv.FormatInt(v, 10))

			_ = sseHandler.Publish(e, topicMetrics)
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

func generateRandomNumbers() *sse.Message {
	e := &sse.Message{}
	count := 1 + rand.Intn(5)

	for i := 0; i < count; i++ {
		e.AppendData(strconv.FormatUint(rand.Uint64(), 10))
	}

	return e
}
