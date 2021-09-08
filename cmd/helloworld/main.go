package main

import (
	"log"
	"net/http"
	"time"

	"github.com/tmaxmax/go-sse/server"
	event "github.com/tmaxmax/go-sse/server/event/v2"
)

func main() {
	sse := server.New()

	go func() {
		ev := &event.Event{}
		ev.AppendText("Hello world")

		for range time.Tick(time.Second) {
			_ = sse.Publish(ev)
		}
	}()

	if err := http.ListenAndServe(":8000", sse); err != nil {
		log.Fatalln(err)
	}
}
