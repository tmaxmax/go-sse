package client

import (
	"bytes"
	"errors"
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
	all        bool
}

type Connection struct {
	c       *http.Client
	r       *http.Request
	isRetry bool

	backoff backoff.BackOff
	onRetry backoff.Notify

	subscribers    map[eventName]map[subscriber]struct{}
	subscribersAll map[subscriber]struct{}

	event       chan *Event
	subscribe   chan subscription
	unsubscribe chan subscription
	done        chan struct{}

	lastEventID      string
	reconnectionTime *time.Duration
}

func (c *Connection) SubscribeMessages(ch chan<- *Event) {
	c.subscribe <- subscription{"", ch, false}
}

func (c *Connection) UnsubscribeMessages(ch chan<- *Event) {
	c.unsubscribe <- subscription{"", ch, false}
}

func (c *Connection) SubscribeEvent(name string, ch chan<- *Event) {
	if name == "" {
		panic("go-sse.client.SubscribeEvent: name cannot be empty")
	}

	c.subscribe <- subscription{name, ch, false}
}

func (c *Connection) UnsubscribeEvent(name string, ch chan<- *Event) {
	if name == "" {
		panic("go-sse.client.UnsubscribeEvent: name cannot be empty")
	}

	c.unsubscribe <- subscription{name, ch, false}
}

func (c *Connection) SubscribeToAll(ch chan<- *Event) {
	c.subscribe <- subscription{"", ch, true}
}

func (c *Connection) UnsubscribeFromAll(ch chan<- *Event) {
	c.subscribe <- subscription{"", ch, true}
}

func (c *Connection) addSubscriberToAll(ch chan<- *Event) {
	for _, subs := range c.subscribers {
		delete(subs, ch)
	}

	c.subscribersAll[ch] = struct{}{}
}

func (c *Connection) removeSubscriberToAll(ch chan<- *Event) {
	for _, subs := range c.subscribers {
		delete(subs, ch)
	}

	delete(c.subscribersAll, ch)
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
	for sub := range c.subscribersAll {
		close(sub)
	}
}

func (c *Connection) run() {
	defer c.closeSubscribers()

	for {
		select {
		case e := <-c.event:
			for s := range c.subscribers[e.Name] {
				s <- e
			}
			for s := range c.subscribersAll {
				s <- e
			}
		case s := <-c.subscribe:
			if s.all {
				c.addSubscriberToAll(s.subscriber)
			} else {
				c.addSubscriber(s.event, s.subscriber)
			}
		case s := <-c.unsubscribe:
			if s.all {
				c.removeSubscriberToAll(s.subscriber)
			} else {
				c.removeSubscriber(s.event, s.subscriber)
			}
		case <-c.done:
			return
		}
	}
}

var ErrNoGetBody = errors.New("go-sse.client: can't retry request, GetBody not present")

func (c *Connection) req() (*http.Request, error) {
	if !c.isRetry {
		c.isRetry = true

		h := c.r.Header.Clone()
		h.Set("Accept", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")

		r := *c.r
		r.Header = h
		c.r = &r

		return c.r, nil
	}

	if c.lastEventID != "" {
		c.r.Header.Set("Last-Event-ID", c.lastEventID)
	} else if c.r.Header.Get("Last-Event-ID") != "" {
		c.r.Header.Set("Last-Event-ID", "")
	}

	if c.r.Body == nil {
		return c.r, nil
	}

	if c.r.GetBody == nil {
		return nil, ErrNoGetBody
	}

	body, err := c.r.GetBody()
	if err != nil {
		return nil, err
	}

	r := *c.r
	r.Body = body

	return &r, nil
}

func (c *Connection) read() error {
	r, err := c.req()
	if err != nil {
		return backoff.Permanent(err)
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
	defer close(c.done)
	return backoff.RetryNotify(c.read, c.backoff, c.onRetry)
}
