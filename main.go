package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/tmaxmax/go-sse/sse"
	"github.com/tmaxmax/go-sse/sse/event"
)

var eventHandler = sse.NewHandler(&sse.Configuration{
	Headers: map[string]string{
		"Access-Control-Allow-Origin": "*",
	},
	CloseEvent: event.New().AddText("we are done here\ngoodbye y'all!").SetID("CLOSE"),
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

	go func() {
		for {
			r := 100 + rand.Int63n(1400)

			select {
			case <-time.After(time.Millisecond * time.Duration(r)):
				b := event.New().SetName("Random numbers")
				count := 1 + rand.Intn(5)

				for i := 0; i < count; i += 1 {
					b.AddText(strconv.FormatUint(rand.Uint64(), 10))
				}

				eventHandler.Send(b)
			case <-cancel:
				return
			}
		}
	}()

	if err := runServer(server, cancel); err != nil {
		log.Println(err)
	}
}

func recordMetric(metric string, frequency time.Duration, cancel <-chan struct{}) {
	for {
		select {
		case <-time.After(frequency):
			eventHandler.Send(event.New().SetName(metric).AddText(strconv.FormatInt(Inc(metric), 10)))
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
