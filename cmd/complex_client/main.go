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

var w = common.NewConcurrentWriter(os.Stdout)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/events", nil)
	conn := client.NewConnection(r)

	go func() {
		ch := make(chan *client.Event)
		conn.SubscribeEvent("close", ch)

		<-ch

		fmt.Fprintln(w, "Server closed!")

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

			fmt.Fprintf(w, "Sum of random numbers: %s\n", sum)
		}
	}()

	go receiveMetric(conn, "ops")
	go receiveMetric(conn, "cycles")

	if err := conn.Connect(); err != nil && !common.IsContextError(err) {
		log.Println(err)
	}
}

func receiveMetric(c *client.Connection, metric string) {
	ch := make(chan *client.Event)
	c.SubscribeEvent(metric, ch)

	for ev := range ch {
		v, _ := strconv.ParseInt(ev.String(), 10, 64)

		fmt.Fprintf(w, "Metric %s: %d\n", metric, v)
	}
}
