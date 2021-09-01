package client

import (
	"bytes"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

type subscriber = chan<- *Event
type eventName = string

type subscription struct {
	event      eventName
	subscriber subscriber
}

type Connection struct {
	c *http.Client
	r *http.Request

	backoff backoff.BackOff
	onRetry backoff.Notify

	subscribers map[eventName]map[subscriber]struct{}

	event       chan *Event
	subscribe   chan subscription
	unsubscribe chan subscription
	done        chan struct{}
	runDone     chan struct{}

	lastEventID      string
	reconnectionTime *time.Duration
}

func (c *Connection) SubscribeMessages(ch chan<- *Event) {
	c.subscribe <- subscription{"", ch}
}

func (c *Connection) UnsubscribeMessages(ch chan<- *Event) {
	c.unsubscribe <- subscription{"", ch}
}

func (c *Connection) SubscribeEvent(name string, ch chan<- *Event) {
	if name == "" {
		panic("go-sse.client.SubscribeEvent: name cannot be empty")
	}

	c.subscribe <- subscription{name, ch}
}

func (c *Connection) UnsubscribeEvent(name string, ch chan<- *Event) {
	if name == "" {
		panic("go-sse.client.UnsubscribeEvent: name cannot be empty")
	}

	c.unsubscribe <- subscription{name, ch}
}

func (c *Connection) addSubscriber(event string, ch chan<- *Event) {
	if _, ok := c.subscribers[event]; !ok {
		c.subscribers[event] = map[chan<- *Event]struct{}{}
	}
	if _, ok := c.subscribers[event][ch]; ok {
		return
	}
	c.subscribers[event][ch] = struct{}{}
}

func (c *Connection) removeSubscriber(name string, ch chan<- *Event) {
	if _, ok := c.subscribers[name]; !ok {
		return
	}
	if _, ok := c.subscribers[name][ch]; !ok {
		return
	}

	delete(c.subscribers[name], ch)
	if len(c.subscribers[name]) == 0 {
		delete(c.subscribers, name)
	}

	for _, listeners := range c.subscribers {
		if _, ok := listeners[ch]; ok {
			return
		}
	}

	close(ch)
}

func (c *Connection) closeSubscribers() {
	closed := map[subscriber]struct{}{}

	for _, subs := range c.subscribers {
		for s := range subs {
			if _, ok := closed[s]; ok {
				continue
			}

			close(s)
			closed[s] = struct{}{}
		}
	}

	close(c.runDone)
}

func (c *Connection) run() {
	defer c.closeSubscribers()

	for {
		select {
		case e := <-c.event:
			for s := range c.subscribers[e.Name] {
				s <- e
			}
		case s := <-c.subscribe:
			c.addSubscriber(s.event, s.subscriber)
		case s := <-c.unsubscribe:
			c.removeSubscriber(s.event, s.subscriber)
		case <-c.done:
			return
		}
	}
}

func (c *Connection) read() error {
	r := c.r.Clone(c.r.Context())
	r.Header.Set("Accept", "text/event-stream")
	r.Header.Set("Cache-Control", "no-cache")
	r.Header.Set("Connection", "keep-alive")

	if c.lastEventID != "" {
		r.Header.Set("Last-Event-ID", c.lastEventID)
	}

	res, err := c.c.Do(r)
	if err != nil {
		ue := err.(*url.Error)
		if ue.Temporary() {
			return ue
		}
		return backoff.Permanent(ue)
	}
	defer res.Body.Close()

	c.backoff.Reset()

	p := parser.NewReaderParser(res.Body)

loop:
	for {
		ev, firstDataField := &Event{}, true

	event:
		for {
			if !p.Scan() {
				break loop
			}

			f := p.Field()
			switch f.Name {
			case parser.FieldNameData:
				if firstDataField {
					firstDataField = false
				} else {
					ev.Data = append(ev.Data, '\n')
				}

				ev.Data = append(ev.Data, f.Value...)
			case parser.FieldNameEvent:
				ev.Name = string(f.Value)
			case parser.FieldNameID:
				if len(f.Value) == 0 || bytes.IndexByte(f.Value, 0) != -1 {
					break
				}

				ev.ID = string(f.Value)
				c.lastEventID = ev.ID
			case parser.FieldNameRetry:
				n, err := strconv.ParseInt(util.String(f.Value), 10, 64)
				if err != nil {
					break
				}
				if n > 0 {
					*c.reconnectionTime = time.Duration(n) * time.Millisecond
				}
			default:
				select {
				case c.event <- ev:
					break event
				case <-c.r.Context().Done():
					return nil
				}
			}
		}
	}

	return p.Err()
}

func (c *Connection) Connect() error {
	err := backoff.RetryNotify(c.read, c.backoff, c.onRetry)
	close(c.done)
	<-c.runDone
	return err
}
