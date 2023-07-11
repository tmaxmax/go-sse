package sse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	// The events's payload.
	Data string
}

// EventCallback is a function that is used to receive events from a Connection.
type EventCallback func(Event)

// EventCallbackRemover is a function that removes an already registered callback
// from a connection. Calling it multiple times is a no-op.
type EventCallbackRemover func()

// Connection is a connection to an events stream. Created using the Client struct,
// a Connection processes the incoming events and sends them to the subscribed channels.
// If the connection to the server temporarily fails, the connection will be reattempted.
// Retry values received from servers will be taken into account.
//
// Connections must not be copied after they are created.
type Connection struct { //nolint:govet // The current order aids readability.
	mu               sync.RWMutex
	wg               sync.WaitGroup
	request          *http.Request
	callbacks        map[string]map[int]EventCallback
	callbacksAll     map[int]EventCallback
	reconnectionTime *time.Duration
	lastEventID      string
	client           Client
	callbackID       int
	isRetry          bool
}

// SubscribeMessages subscribes the given callback to all unnamed events (none or empty event field).
// Remove the callback by calling the returned function.
func (c *Connection) SubscribeMessages(cb EventCallback) EventCallbackRemover {
	return c.SubscribeEvent("", cb)
}

// SubscribeEvent subscribes the given callback to all the events with the provided name
// (the event field has the value given here).
// Remove the callback by calling the returned function.
func (c *Connection) SubscribeEvent(name string, cb EventCallback) EventCallbackRemover {
	return c.addSubscriber(name, cb)
}

// SubscribeToAll subscribes the given callbcak to all events, named and unnamed.
// Remove the callback by calling the returned function.
func (c *Connection) SubscribeToAll(cb EventCallback) EventCallbackRemover {
	return c.addSubscriberToAll(cb)
}

func (c *Connection) addSubscriberToAll(cb EventCallback) EventCallbackRemover {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.callbackID
	c.callbacksAll[id] = cb
	c.callbackID++

	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		delete(c.callbacksAll, id)
	}
}

func (c *Connection) addSubscriber(event string, cb EventCallback) EventCallbackRemover {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.callbacks[event]; !ok {
		c.callbacks[event] = map[int]EventCallback{}
	}

	id := c.callbackID
	c.callbacks[event][id] = cb
	c.callbackID++

	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		delete(c.callbacks[event], id)
		if len(c.callbacks[event]) == 0 {
			delete(c.callbacks, event)
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

func (c *Connection) executeCallback(cb EventCallback, ev Event) {
	go func() {
		defer c.wg.Done()
		cb(ev)
	}()
}

func (c *Connection) dispatch(ev Event) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cbs := c.callbacks[ev.Name]
	cbCount := len(cbs) + len(c.callbacksAll)
	if cbCount == 0 {
		return
	}

	if l := len(ev.Data); l > 0 {
		ev.Data = ev.Data[:l-1]
	}
	ev.LastEventID = c.lastEventID

	c.wg.Add(cbCount)
	for _, cb := range c.callbacks[ev.Name] {
		c.executeCallback(cb, ev)
	}
	for _, cb := range c.callbacksAll {
		c.executeCallback(cb, ev)
	}
}

func (c *Connection) read(r io.Reader, reset func()) error {
	p := parser.New(r)
	ev, dirty := Event{}, false

	for f := (parser.Field{}); p.Next(&f); {
		switch f.Name { //nolint:exhaustive // Comment fields are not parsed.
		case parser.FieldNameData:
			ev.Data += f.Value + "\n"
			dirty = true
		case parser.FieldNameEvent:
			ev.Name = f.Value
			dirty = true
		case parser.FieldNameID:
			// empty IDs are valid, only IDs that contain the null byte must be ignored:
			// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
			if strings.IndexByte(f.Value, 0) != -1 {
				break
			}

			c.lastEventID = f.Value
			dirty = true
		case parser.FieldNameRetry:
			n, err := strconv.ParseInt(f.Value, 10, 64)
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

	err := backoff.RetryNotify(op, b, c.client.OnRetry)
	c.wg.Wait()

	return err
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
