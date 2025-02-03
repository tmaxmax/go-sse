package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
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
	rp, _ := sse.NewValidReplayer(time.Minute*5, true)
	rp.GCInterval = time.Minute

	return &sse.Server{
		Provider: &sse.Joe{Replayer: rp},
		// If you are using a 3rd party library to generate a per-request logger, this
		// can just be a simple wrapper over it.
		Logger: func(r *http.Request) *slog.Logger {
			return getLogger(r.Context())
		},
		OnSession: func(w http.ResponseWriter, r *http.Request) (topics []string, permitted bool) {
			topics = r.URL.Query()["topic"]
			for _, topic := range topics {
				if topic != topicRandomNumbers && topic != topicMetrics {
					fmt.Fprintf(w, "invalid topic %q; supported are %q, %q", topic, topicRandomNumbers, topicMetrics)

					// NOTE: if you are returning false to reject the subscription, we strongly recommend writing
					// your own response code. Clients will receive a 200 code otherwise, which may be confusing.
					w.WriteHeader(http.StatusBadRequest)
					return nil, false
				}
			}
			if len(topics) == 0 {
				// Provide default topics, if none are given.
				topics = []string{topicRandomNumbers, topicMetrics}
			}

			// the shutdown message is sent on the default topic
			return append(topics, sse.DefaultTopic), true
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

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	logMiddleware := withLogger(logger)

	sseHandler := newSSE()

	mux := http.NewServeMux()
	mux.HandleFunc("/stop", func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/", SnapshotHTTPEndpoint)
	mux.Handle("/events", sseHandler)

	httpLogger := slog.NewLogLogger(handler, slog.LevelWarn)
	s := &http.Server{
		Addr:              "0.0.0.0:8080",
		Handler:           cors(logMiddleware(mux)),
		ReadHeaderTimeout: time.Second * 10,
		ErrorLog:          httpLogger,
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

type loggerCtxKey struct{}

// withLogger is a net/http compatable  middleware that generates a logger with request-specific fields
// added to it and attaches it to the request context for later retrieval with getLogger().
// Third party logging packages may offer similar middlewares to add a logger to the request or maybe
// just a helper to add a logger to context; in the second case you can build your own middleware
// function around it, similar to this one.
func withLogger(logger *slog.Logger) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			l := logger.With(

				"UserAgent", r.UserAgent(),
				"RemoteAddr", r.RemoteAddr,
				"Host", r.Host,
				"Origin", r.Header.Get("origin"),
			)
			r = r.WithContext(context.WithValue(r.Context(), loggerCtxKey{}, l))
			h.ServeHTTP(w, r)
		})
	}
}

// getLogger retrieves the request-specific logger from a request's context. This is
// similar to how existing per-request http logging libraries work, just very simplified.
func getLogger(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerCtxKey{}).(*slog.Logger)
	if !ok {
		// We are accepting an arbitrary context object, so it's better to explicitly return
		// nil here since the exact behavior of getting the value of an undefined key is undefined
		return nil
	}
	return logger
}
