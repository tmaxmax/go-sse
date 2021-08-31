package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tmaxmax/go-sse/server/event"
)

// A ReplayProvider is a type that can replay older published events to new subscribers.
// Replay providers use event IDs, the topics the events were published to and optionally
// the events' expiration times or any other criteria to determine which are valid for replay.
//
// While providers can require events to have IDs beforehand, they can also set the IDs themselves,
// automatically - it's up to the implementation.
//
// If not specified otherwise, the errors returned are implementation-specific.
type ReplayProvider interface {
	// Put adds a new event to the replay buffer. The message's event may be modified by
	// the provider, if it sets an ID.
	Put(message *Message) error
	// Replay sends to a new subscriber all the valid events received by the provider
	// since the event with the subscription's ID. If the ID the subscription provides
	// is invalid, the provider should not replay any events and return a ReplayError
	Replay(subscription Subscription) error
	// GC triggers a cleanup. After GC returns, all the events that are invalid according
	// to the provider's criteria should be impossible to replay again.
	GC() error
}

// ReplayError is an error returned when a replay failed. It contains the ID for
// which the replay failed and optionally another error value that describes why
// the replay failed.
type ReplayError struct {
	err error
	id  event.ID
}

func (e *ReplayError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("server.replay.Provider: invalid ID %q: %s", e.id, e.err.Error())
	}
	return fmt.Sprintf("server.replay.Provider: ID %q does not exist", e.id)
}

type subscriber = chan<- *event.Event
type subscribers = map[subscriber]struct{}

// Joe is a basic server provider that synchronously executes operations by queueing them in channels.
// Events are also sent synchronously to subscribers - if a subscriber can't receive events when Joe
// sends them then Joe is blocked until the event is sent.
//
// Joe supports event replaying by configuring it with a replay provider. As operations are executed
// synchronously, it is guaranteed that no new events will be omitted from sending to a new subscriber
// because older events are still replaying when the event is sent to Joe.
//
// Joe works well for simple use-cases - it is light on resources, and does not require any external
// services.
type Joe struct {
	message        chan Message
	subscription   chan Subscription
	unsubscription chan subscriber
	done           chan struct{}
	topics         map[string]subscribers
	subscribers    subscribers
	replay         ReplayProvider
	gcInterval     time.Duration
}

// JoeConfig is used to tune Joe to preference.
type JoeConfig struct {
	// Joe receives published events on a dedicated channel. If the publisher's goroutine is blocked
	// because Joe can't keep up with the load, use a bigger buffer. This shouldn't be a concern
	// and if it is other providers might be suited better for your use-case.
	//
	// The buffer size defaults to 1.
	MessageChannelBuffer int
	// An optional replay provider that Joe uses to resend older messages to new subscribers.
	ReplayProvider ReplayProvider
	// An optional interval at which Joe triggers a cleanup of expired messages, if the replay provider supports it.
	// See the desired provider's documentation to determine if periodic cleanup is necessary.
	ReplayGCInterval time.Duration
}

// NewJoe creates and starts a Joe.
func NewJoe(configuration ...JoeConfig) *Joe {
	config := joeConfig(configuration)
	j := &Joe{
		message:        make(chan Message, config.MessageChannelBuffer),
		subscription:   make(chan Subscription),
		unsubscription: make(chan subscriber),
		done:           make(chan struct{}),
		topics:         map[string]subscribers{},
		subscribers:    subscribers{},
		replay:         config.ReplayProvider,
		gcInterval:     config.ReplayGCInterval,
	}

	go j.start()

	return j
}

var ErrJoeClosed = errors.New("joe: provider already closed")

func (j *Joe) Subscribe(ctx context.Context, sub Subscription) error {
	sub.Topics = topics(sub.Topics)

	go func() {
		// We are also waiting on done here so if Joe is stopped but not the HTTP server that
		// serves the request this goroutine isn't hanging.
		select {
		case <-ctx.Done():
		case <-j.done:
			return
		}

		// We are waiting on done here so the goroutine isn't blocked if Joe is stopped when
		// this point is reached.
		select {
		case j.unsubscription <- sub.Channel:
		case <-j.done:
		}
	}()

	// Waiting on done ensures Subscribe behaves as required by the Provider interface
	// if Stop was called. It also ensures Subscribe doesn't block if a new request arrives
	// after Joe is stopped, which would otherwise result in a client waiting forever.
	select {
	case j.subscription <- sub:
		return nil
	case <-j.done:
		return ErrProviderClosed
	}
}

func (j *Joe) Publish(msg Message) error {
	select {
	case j.message <- msg:
		return nil
	case <-j.done:
		return ErrProviderClosed
	}
}

func (j *Joe) Stop() error {
	// Waiting on Stop here prevents double-closing and implements the required Provider behavior.
	select {
	case <-j.done:
		return ErrProviderClosed
	default:
		close(j.done)
		return nil
	}
}

func (j *Joe) topic(identifier string) subscribers {
	if _, ok := j.topics[identifier]; !ok {
		j.topics[identifier] = subscribers{}
	}
	return j.topics[identifier]
}

func (j *Joe) start() {
	gc, stop := ticker(j.gcInterval)
	defer stop()

	for {
		select {
		case msg := <-j.message:
			_ = j.replay.Put(&msg)

			for sub := range j.topics[msg.Topic] {
				sub <- msg.Event
			}
		case sub := <-j.subscription:
			if _, ok := j.subscribers[sub.Channel]; ok {
				continue
			}

			_ = j.replay.Replay(sub)

			for _, topic := range sub.Topics {
				j.topic(topic)[sub.Channel] = struct{}{}
			}
			j.subscribers[sub.Channel] = struct{}{}
		case unsub := <-j.unsubscription:
			for _, subs := range j.topics {
				delete(subs, unsub)
			}

			delete(j.subscribers, unsub)
			close(unsub)
		case <-gc:
			_ = j.replay.GC()
		case <-j.done:
			for sub := range j.subscribers {
				close(sub)
			}
			return
		}
	}
}

var _ Provider = (*Joe)(nil)
