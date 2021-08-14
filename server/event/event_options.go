package event

import "time"

// An Option is a type that is used to define an event. A field is an option.
type Option interface {
	apply(*Event)
}

// TTL is an event option which sets the event's expiration timestamp to the one after the specified duration from now.
type TTL time.Duration

func (t TTL) apply(e *Event) {
	e.expiresAt = time.Now().Add(time.Duration(t))
}

// ExpiresAt is an event option which sets the event's expiration timestamp to the value provided.
type ExpiresAt time.Time

func (e ExpiresAt) apply(ev *Event) {
	ev.expiresAt = time.Time(e)
}
