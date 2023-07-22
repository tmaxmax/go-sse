package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tmaxmax/go-sse"
)

func main() {
	var sub string
	flag.StringVar(&sub, "sub", "all", "The topics to subscribe to. Valid values are: all, numbers, metrics")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, getRequestURL(sub), http.NoBody)
	conn := sse.NewConnection(r)
	// Callbacks are called from separate goroutines, we must synchronize access to stdout.
	// log.Logger does that automatically for us, so we create one that writes to stdout without any prefix or flags.
	out := log.New(os.Stdout, "", 0)

	conn.SubscribeToAll(func(event sse.Event) {
		switch event.Type {
		case "cycles", "ops":
			out.Printf("Metric %s: %s\n", event.Type, event.Data)
		case "close":
			out.Println("Server closed!")
			cancel()
		default: // no event name
			var sum, num big.Int
			for _, n := range strings.Split(event.Data, "\n") {
				_, _ = num.SetString(n, 10)
				sum.Add(&sum, &num)
			}

			out.Printf("Sum of random numbers: %s\n", &sum)
		}
	})

	if err := conn.Connect(); err != nil {
		out.Println(err)
	}
}

func getRequestURL(sub string) string {
	q := url.Values{}
	switch sub {
	case "all":
		q.Add("topic", "numbers")
		q.Add("topic", "metrics")
	case "numbers", "metrics":
		q.Set("topic", sub)
	default:
		panic(fmt.Errorf("unexpected subscription topic %q", sub))
	}

	return "http://localhost:8080/events?" + q.Encode()
}
