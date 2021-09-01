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
	"github.com/tmaxmax/go-sse/cmd/common"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/events", nil)
	conn := client.NewConnection(r)
	done := make(chan struct{})

	go func() {
		ch := make(chan *client.Event, 1)
		conn.SubscribeToAll(ch)

		for ev := range ch {
			switch ev.Name {
			case "cycles", "ops":
				handleMetric(ev.Name, ev.String())
			case "close":
				fmt.Println("Server closed!")
				cancel()
			default: // no event name
				numbers := strings.Split(ev.String(), "\n")
				sum := big.NewInt(0)

				for _, n := range numbers {
					v, _ := strconv.ParseInt(n, 10, 64)
					sum = sum.Add(sum, big.NewInt(v))
				}

				fmt.Printf("Sum of random numbers: %s\n", sum)
			}
		}

		close(done)
	}()

	if err := conn.Connect(); err != nil && !common.IsContextError(err) {
		log.Println(err)
	}

	<-done
}

func handleMetric(metric string, value string) {
	v, _ := strconv.ParseInt(value, 10, 64)
	fmt.Printf("Metric %s: %d\n", metric, v)
}
