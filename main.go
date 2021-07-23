package main

import (
	"context"
	"go-metrics/sse"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"
)

var eventHandler = sse.New(&sse.Configuration{
	Headers: map[string]string{
		"Access-Control-Allow-Origin": "*",
	},
	CloseMessage: &sse.Message{
		Event: "close",
		Data:  []byte("Event stream closed!"),
	},
})

func main() {
	cancel := make(chan struct{})
	cancelMetrics := make(chan struct{})
	cancelSignal := make(chan os.Signal)
	signal.Notify(cancelSignal, os.Interrupt)

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

	server := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}

	go eventHandler.StartWithSignal(cancel)

	go recordMetric("ops", time.Second*2, cancel)
	go recordMetric("cycles", time.Millisecond*500, cancel)
	//go recordMetric("sarmale", time.Microsecond, cancel)

	if err := runServer(server, cancel); err != nil {
		log.Println(err)
	}
}

func recordMetric(metric string, frequency time.Duration, cancel <-chan struct{}) {
	for {
		select {
		case <-time.After(frequency):
			newValue := Inc(metric)

			_ = eventHandler.Send(&sse.Message{
				Event: metric,
				Data:  []byte(strconv.FormatInt(newValue, 10)),
			})
		case <-cancel:
			break
		}
	}
}

func runServer(server *http.Server, cancel <-chan struct{}) error {
	shutdownError := make(chan error)

	go func() {
		<-cancel

		shutdownError <- server.Shutdown(context.Background())
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return <-shutdownError
}
