package main

import (
	"context"
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

var p = log.New(os.Stdout, "", 0)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/events", nil)
	conn := client.NewConnection(r)

	go func() {
		ch := make(chan *client.Event)
		conn.SubscribeEvent("close", ch)

		<-ch

		p.Println("Server closed!")

		cancel()
	}()

	go func() {
		ch := make(chan *client.Event)
		conn.SubscribeMessages(ch)

		for ev := range ch {
			numbers := strings.Split(ev.String(), "\n")
			sum := big.NewInt(0)

			for _, n := range numbers {
				v, _ := strconv.ParseInt(n, 10, 64)
				sum = sum.Add(sum, big.NewInt(v))
			}

			p.Printf("Sum of random numbers: %s\n", sum)
		}
	}()

	go receiveMetric(conn, "ops")
	go receiveMetric(conn, "cycles")

	log.Println(conn.Connect())
}

func receiveMetric(c *client.Connection, metric string) {
	ch := make(chan *client.Event)
	c.SubscribeEvent(metric, ch)

	for ev := range ch {
		v, _ := strconv.ParseInt(ev.String(), 10, 64)

		p.Printf("Metric %s: %d\n", metric, v)
	}
}
