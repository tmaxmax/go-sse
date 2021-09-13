package sse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/tmaxmax/go-sse/internal/parser"
)

// The Event struct represents an event sent to the client by a server.
type Event struct {
	// The last non-empty ID of all the events received. This may not be
	// the ID of the latest event!
	LastEventID string
	// The event's name. It is empty if the eventName is unnamed.
	Name string
	// The events's payload in raw form. Use the String method if you need it as a string.
	Data []byte
}

// String copies the data buffer and returns it as a string.
func (e Event) String() string {
	return string(e.Data)
}

type listener struct {
	subscriber chan<- Event
	event      string
	all        bool
}

// Connection is a connection to an events stream. Created using the Client struct,
// a Connection processes the incoming events and sends them to the subscribed channels.
// If the connection to the server temporarily fails, the connection will be reattempted.
// Retry values received from servers will be taken into account.
type Connection struct {
	request *http.Request

	subscribe      chan listener
	event          chan Event
	unsubscribe    chan listener
	done           chan struct{}
	subscribers    map[string]map[chan<- Event]struct{}
	subscribersAll map[chan<- Event]struct{}

	reconnectionTime *time.Duration
	lastEventID      string
	client           Client
	isRetry          bool
}

func (c *Connection) send(ch chan<- listener, s listener) {
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
	c.send(c.subscribe, listener{event: name, subscriber: ch})
}

// UnsubscribeEvent unsubscribes al already subscribed channel from the events with the provided name.
// If the channel is not subscribed to any other events, it will be closed.
// Don't call this method from the goroutine that receives from the channel, as it may result in a deadlock!
func (c *Connection) UnsubscribeEvent(name string, ch chan<- Event) {
	c.send(c.unsubscribe, listener{event: name, subscriber: ch})
}

// SubscribeToAll subscribes the given channel to all events, named and unnamed. If the channel is already subscribed
// to some events it won't receive those events twice.
func (c *Connection) SubscribeToAll(ch chan<- Event) {
	c.send(c.subscribe, listener{subscriber: ch, all: true})
}

// UnsubscribeFromAll unsubscribes an already subscribed channel from all the events it is subscribed to.
// After unsubscription the channel will be closed. The channel doesn't have to be subscribed using
// the SubscribeToAll method in order for this method to unsubscribe it.
// Don't call this method from the goroutine that receives from the channel, as it may result in a deadlock!
func (c *Connection) UnsubscribeFromAll(ch chan<- Event) {
	c.send(c.unsubscribe, listener{subscriber: ch, all: true})
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

func (c *Connection) addSubscriber(event string, ch chan<- Event) {
	if _, ok := c.subscribersAll[ch]; ok {
		return
	}
	if _, ok := c.subscribers[event]; !ok {
		c.subscribers[event] = map[chan<- Event]struct{}{}
	}
	if _, ok := c.subscribers[event][ch]; ok {
		return
	}
	c.subscribers[event][ch] = struct{}{}
}

func (c *Connection) removeSubscriber(name string, ch chan<- Event) {
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
	for _, subs := range c.subscribers {
		for s := range subs {
			c.subscribersAll[s] = struct{}{}
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
				c.removeSubscriberFromAll(s.subscriber)
			} else {
				c.removeSubscriber(s.event, s.subscriber)
			}
		case <-c.done:
			return
		}
	}
}

// ConnectionError is the type that wraps all the connection errors that occur.
type ConnectionError struct {
	// The request for which the connection failed.
	Req *http.Request
	// The reason the operation failed.
	Err error
	// The reason why the request failed.
	Reason string
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("request failed: %s: %v", e.Reason, e.Err)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// Temporary returns whether the underlying error is temporary.
func (e *ConnectionError) Temporary() bool {
	var t interface{ Temporary() bool }
	if errors.As(e.Err, &t) {
		return t.Temporary()
	}
	return false
}

// Timeout returns whether the underlying error is caused by a timeout.
func (e *ConnectionError) Timeout() bool {
	var t interface{ Timeout() bool }
	if errors.As(e.Err, &t) {
		return t.Timeout()
	}
	return false
}

func (e *ConnectionError) toPermanent() error {
	if e.Temporary() || e.Timeout() {
		return e
	}
	return backoff.Permanent(e)
}

func (c *Connection) resetRequest() error {
	if !c.isRetry {
		c.isRetry = true
		return nil
	}
	if err := resetRequestBody(c.request); err != nil {
		return &ConnectionError{Req: c.request, Reason: "unable to reset request body", Err: err}
	}
	if c.lastEventID == "" {
		c.request.Header.Del("Last-Event-ID")
	} else {
		c.request.Header.Set("Last-Event-ID", c.lastEventID)
	}
	return nil
}

func (c *Connection) dispatch(ev Event) {
	if l := len(ev.Data); l > 0 {
		ev.Data = ev.Data[:l-1]
	}
	ev.LastEventID = c.lastEventID
	c.event <- ev
}

func (c *Connection) read(r io.Reader, reset func()) error {
	p := parser.NewReaderParser(r)
	ev, dirty := Event{}, false

	for p.Scan() {
		f := p.Field()

		switch f.Name {
		case parser.FieldNameData:
			ev.Data = append(ev.Data, f.Value...)
			ev.Data = append(ev.Data, '\n')
			dirty = true
		case parser.FieldNameEvent:
			ev.Name = string(f.Value)
			dirty = true
		case parser.FieldNameID:
			// empty IDs are valid, only IDs that contain the null byte must be ignored:
			// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
			if bytes.IndexByte(f.Value, 0) != -1 {
				break
			}

			c.lastEventID = string(f.Value)
			dirty = true
		case parser.FieldNameRetry:
			n, err := strconv.ParseInt(string(f.Value), 10, 64)
			if err != nil {
				break
			}
			if n > 0 {
				*c.reconnectionTime = time.Duration(n) * time.Millisecond
				reset()
			}
			dirty = true
		default:
			c.dispatch(ev)
			ev = Event{}
			dirty = false
		}
	}

	err := p.Err()
	if dirty && err == nil {
		c.dispatch(ev)
	}
	if isSuccess(err) {
		return nil
	}
	e := &ConnectionError{Req: c.request, Reason: "reading response body failed", Err: err}
	return e.toPermanent()
}

func isSuccess(err error) bool {
	return err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, parser.ErrUnexpectedEOF)
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
// All errors returned are of type *ConnectionError.
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
			e := &ConnectionError{Req: c.request, Reason: "unable to execute request", Err: err}
			return e.toPermanent()
		}
		defer res.Body.Close()

		if err := c.client.ResponseValidator(res); err != nil {
			e := &ConnectionError{Req: c.request, Reason: "response validation failed", Err: err}
			return e.toPermanent()
		}

		b.Reset()

		return c.read(res.Body, b.Reset)
	}

	return backoff.RetryNotify(op, b, c.client.OnRetry)
}

// ErrNoGetBody is a sentinel error returned when the connection cannot be reattempted
// due to GetBody not existing on the original request.
var ErrNoGetBody = errors.New("the GetBody function doesn't exist on the request")

func resetRequestBody(r *http.Request) error {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	if r.GetBody == nil {
		return ErrNoGetBody
	}
	body, err := r.GetBody()
	if err != nil {
		return err
	}
	r.Body = body
	return nil
}
