package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/tmaxmax/go-sse/client"
)

func main() {
	r, _ := http.NewRequest(http.MethodGet, "http://localhost:8000", nil)
	conn := client.NewConnection(r)
	ch := make(chan client.Event)

	conn.SubscribeMessages(ch)

	go func() {
		for ev := range ch {
			fmt.Printf("%s\n\n", ev.Data)
		}
	}()

	if err := conn.Connect(); err != nil {
		log.Println(err)
	}
}
