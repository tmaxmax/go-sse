package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/tmaxmax/go-sse/internal/parser"
	"github.com/tmaxmax/go-sse/internal/util"
)

type (
	subscriber chan<- Event
	eventName  string
)

type subscription struct {
	subscriber subscriber
	event      eventName
	all        bool
}

type Connection struct {
	request *http.Request

	subscribe      chan subscription
	event          chan Event
	unsubscribe    chan subscription
	done           chan struct{}
	subscribers    map[eventName]map[subscriber]struct{}
	subscribersAll map[subscriber]struct{}

	reconnectionTime *time.Duration
	lastEventID      string
	client           Client
	isRetry          bool
	lastEventIDSet   bool
}

func (c *Connection) send(ch chan<- subscription, s subscription) {
	select {
	case ch <- s:
	case <-c.done:
	}
}

// SubscribeMessages subscribes the given channel to all events that are unnamed (no event field is present).
func (c *Connection) SubscribeMessages(ch chan<- Event) {
	c.SubscribeEvent("", ch)
}

// UnsubscribeMessages unsubscribes an already subscribed channel from unnamed events.
// If the channel is not subscribed to any other events, it will be closed.
// Don't call this method from the goroutine that receives from the channel, as it may result in a deadlock!
func (c *Connection) UnsubscribeMessages(ch chan<- Event) {
	c.UnsubscribeEvent("", ch)
}

// SubscribeEvent subscribes the given channel to all the events with the provided name
// (the event field has the value given here).
func (c *Connection) SubscribeEvent(name string, ch chan<- Event) {
	c.send(c.subscribe, subscription{event: eventName(name), subscriber: ch})
}

// UnsubscribeEvent unsubscribes al already subscribed channel from the events with the provided name.
// If the channel is not subscribed to any other events, it will be closed.
// Don't call this method from the goroutine that receives from the channel, as it may result in a deadlock!
func (c *Connection) UnsubscribeEvent(name string, ch chan<- Event) {
	c.send(c.unsubscribe, subscription{event: eventName(name), subscriber: ch})
}

// SubscribeToAll subscribes the given channel to all events, named and unnamed. If the channel is already subscribed
// to some events it won't receive those events twice.
func (c *Connection) SubscribeToAll(ch chan<- Event) {
	c.send(c.subscribe, subscription{subscriber: ch, all: true})
}

// UnsubscribeFromAll unsubscribes an already subscribed channel from all the events it is subscribed to.
// After unsubscription the channel will be closed. The channel doesn't have to be subscribed using
// the SubscribeToAll method in order for this method to unsubscribe it.
// Don't call this method from the goroutine that receives from the channel, as it may result in a deadlock!
func (c *Connection) UnsubscribeFromAll(ch chan<- Event) {
	c.send(c.unsubscribe, subscription{subscriber: ch, all: true})
}

func (c *Connection) addSubscriberToAll(ch chan<- Event) {
	for _, subs := range c.subscribers {
		delete(subs, ch)
	}

	c.subscribersAll[ch] = struct{}{}
}

func (c *Connection) removeSubscriberFromAll(ch chan<- Event) {
	for event, subs := range c.subscribers {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(c.subscribers, event)
		}
	}

	delete(c.subscribersAll, ch)
	close(ch)
}

func (c *Connection) addSubscriber(event eventName, ch chan<- Event) {
	if _, ok := c.subscribers[event]; !ok {
		c.subscribers[event] = map[subscriber]struct{}{}
	}
	if _, ok := c.subscribers[event][ch]; ok {
		return
	}
	c.subscribers[event][ch] = struct{}{}
}

func (c *Connection) removeSubscriber(name eventName, ch chan<- Event) {
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
			for s := range c.subscribers[eventName(e.Name)] {
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
				c.removeSubscriberFromAll(s.subscriber)
			} else {
				c.removeSubscriber(s.event, s.subscriber)
			}
		case <-c.done:
			return
		}
	}
}

func (c *Connection) resetRequest() error {
	if !c.isRetry {
		c.isRetry = true
		return nil
	}
	if err := resetRequestBody(c.request); err != nil {
		return &Error{Req: c.request, Reason: "unable to reset request body", Err: err}
	}
	if c.lastEventID == "" {
		c.request.Header.Del("Last-Event-ID")
	} else {
		c.request.Header.Set("Last-Event-ID", c.lastEventID)
	}
	return nil
}

func (c *Connection) read(r io.Reader) error {
	p := parser.NewReaderParser(r)
	ev := Event{}

	for p.Scan() {
		f := p.Field()

		switch f.Name {
		case parser.FieldNameData:
			ev.Data = append(ev.Data, f.Value...)
			ev.Data = append(ev.Data, '\n')
		case parser.FieldNameEvent:
			ev.Name = string(f.Value)
		case parser.FieldNameID:
			// empty IDs are valid, only IDs that contain the null byte must be ignored:
			// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
			if bytes.IndexByte(f.Value, 0) != -1 {
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
			if l := len(ev.Data); l > 0 {
				ev.Data = ev.Data[:l-1]
			}
			select {
			case c.event <- ev:
				ev = Event{}
			case <-c.request.Context().Done():
				return nil
			}
		}
	}

	err := p.Err()
	if err == nil || err == context.Canceled || err == context.DeadlineExceeded || err == parser.ErrUnexpectedEOF {
		return nil
	}
	return &Error{Req: c.request, Reason: "reading response body failed", Err: err}
}

// Connect sends the request the connection was created with to the server
// and, if successful, it starts receiving events. The caller goroutine
// is blocked until the request's context is done or an error occurs.
//
// Connect returns a nil error on success. On non-nil errors, the connection
// is reattempted for the number of times the Client is configured with,
// using an exponential backoff that has the initial time set to either the
// client's default value or to the retry value received from the server.
// If an error is permanent (e.g. no internet connection), no retries are done.
// All errors returned are of type *client.Error.
//
// After Connect returns, all subscriptions will be closed. Make sure to wait
// for the subscribers' goroutines to exit, as they may still be running after
// Connect has returned. Connect cannot be called twice for the same connection.
func (c *Connection) Connect() error {
	defer close(c.done)

	b, interval := c.client.newBackoff(c.request.Context())

	c.reconnectionTime = interval
	c.request.Header.Set("Accept", "text/event-stream")
	c.request.Header.Set("Connection", "keep-alive")
	c.request.Header.Set("Cache", "no-cache")

	op := func() error {
		if err := c.resetRequest(); err != nil {
			return backoff.Permanent(err)
		}

		res, err := c.client.do(c.request)
		if err != nil {
			e := &Error{Req: c.request, Reason: "unable to execute request", Err: err}
			return e.toPermanent()
		}
		defer res.Body.Close()

		if err := c.client.ResponseValidator(res); err != nil {
			e := &Error{Req: c.request, Reason: "response validation failed", Err: err}
			return e.toPermanent()
		}

		b.Reset()

		return c.read(res.Body)
	}

	return backoff.RetryNotify(op, b, c.client.OnRetry)
}

func resetRequestBody(r *http.Request) error {
	if r.Body == nil {
		return nil
	}
	if r.GetBody == nil {
		return errors.New("the GetBody function doesn't exist on the request")
	}
	body, err := r.GetBody()
	if err != nil {
		return err
	}
	r.Body = body
	return nil
}
