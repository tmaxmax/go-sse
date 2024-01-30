package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/tmaxmax/go-sse"
)

func main() {
	r, _ := http.NewRequest(http.MethodGet, "http://localhost:8000", http.NoBody)
	conn := sse.NewConnection(r)

	conn.SubscribeMessages(func(event sse.Event) {
		fmt.Printf("%s\n\n", event.Data)
	})

	if err := conn.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
