package main

import (
	"log"
	"net/http"
	"time"

	"github.com/tmaxmax/go-sse"
)

func main() {
	s := sse.NewServer()

	go func() {
		ev := &sse.Message{}
		ev.AppendText("Hello world")

		for range time.Tick(time.Second) {
			_ = s.Publish(ev)
		}
	}()

	if err := http.ListenAndServe(":8000", s); err != nil {
		log.Fatalln(err)
	}
}
