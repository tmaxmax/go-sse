//go:build go1.23

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/tmaxmax/go-sse"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	in := bufio.NewScanner(os.Stdin)
	if !in.Scan() {
		fmt.Fprintf(os.Stderr, "message read error: %v\n", in.Err())
		return
	}

	// I've picked ChatGPT for this example just to have a working program.
	// I do not endorse nor have any affiliation with any LLM out there.
	payload := strings.NewReader(fmt.Sprintf(`{
	"model": "gpt-4o",
	"messages": [
		{
			"role": "developer",
			"content": "You are a helpful assistant."
		},
		{
			"role": "user",
			"content": "%s"
		}
    ],
    "stream": true
}`, strconv.Quote(in.Text())))

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", payload)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "request error: %v\n", err)
		return
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "response errored with code %s\n", res.Status)
		return
	}

	for ev, err := range sse.Read(res.Body, nil) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "while reading response body: %v\n", err)
			// Can return – Read stops after first error and no subsequent events are parsed.
			return
		}

		var data struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
			fmt.Fprintf(os.Stderr, "while unmarshalling response: %v\n", err)
			return
		}

		fmt.Printf("%s ", data.Choices[0].Delta.Content)
	}
}
