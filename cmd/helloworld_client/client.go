package main

import (
	"log"
	"net/http"
	"os"

	"github.com/tmaxmax/go-sse"
)

func main() {
	r, _ := http.NewRequest(http.MethodGet, "http://localhost:8000", http.NoBody)
	conn := sse.NewConnection(r)
	// Callbacks are called from separate goroutines, we must synchronize access to stdout.
	// log.Logger does that automatically for us, so we create one that writes to stdout without any prefix or flags.
	out := log.New(os.Stdout, "", 0)

	conn.SubscribeMessages(func(event sse.Event) {
		out.Printf("%s\n\n", event)
	})

	if err := conn.Connect(); err != nil {
		out.Println(err)
	}
}
