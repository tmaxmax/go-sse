package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/tmaxmax/go-sse/client"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/events", nil)
	conn, done, events := client.NewConnection(r), make(chan struct{}), make(chan *client.Event, 1)

	conn.SubscribeToAll(events)

	go func() {
		for event := range events {
			switch event.Name {
			case "cycles", "ops":
				fmt.Printf("Metric %s: %s\n", event.Name, event)
			case "close":
				fmt.Println("Server closed!")
				cancel()
			default: // no event name
				sum := big.NewInt(0)
				for _, n := range strings.Split(event.String(), "\n") {
					v, _ := strconv.ParseInt(n, 10, 64)
					sum.Add(sum, big.NewInt(v))
				}

				fmt.Printf("Sum of random numbers: %s\n", sum)
			}
		}

		close(done)
	}()

	if err := conn.Connect(); err != nil {
		log.Println(err)
	}

	<-done
}
