package sse

import (
	"context"
	"time"
)

// A ReplayProvider is a type that can replay older published events to new subscribers.
// Replay providers use event IDs, the topics the events were published to and optionally
// the events' expiration times or any other criteria to determine which are valid for replay.
//
// While providers can require events to have IDs beforehand, they can also set the IDs themselves,
// automatically - it's up to the implementation. Providers should ignore events without IDs,
// if they require IDs to be set.
//
// Replay providers are not required to be thread-safe - server providers are required to ensure only
// one operation is executed on the replay provider at any given time. Server providers may not execute
// replay operation concurrently with other operations, so make sure any action on the replay provider
// blocks for as little as possible. If a replay provider is thread-safe, some operations may be
// run in a separate goroutine - see the interface's method documentation.
//
// Executing actions that require waiting for a long time on I/O, such as HTTP requests or database
// calls must be handled with great care, so the server provider is not blocked. Reducing them to
// the minimum by using techniques such as caching or by executing them in separate goroutines is
// recommended, as long as the implementation fulfills the requirements.
//
// If not specified otherwise, the errors returned are implementation-specific.
type ReplayProvider interface {
	// Put adds a new event to the replay buffer. The message's event may be modified by
	// the provider, if it sets an ID.
	//
	// The Put operation may be executed by the replay provider in another goroutine only if
	// it can ensure that any Replay operation called after the Put goroutine is started
	// can replay the new received message. This also requires the replay provider implementation
	// to be thread-safe.
	//
	// Replay providers are not required to guarantee that after Put returns the new events
	// can be replayed. If an error occurs and retrying the operation would block for too
	// long, it can be aborted. The errors aren't returned as the server providers won't be able
	// to handle them in a useful manner anyway.
	Put(message **Message)
	// Replay sends to a new subscriber all the valid events received by the provider
	// since the event with the listener's ID. If the ID the listener provides
	// is invalid, the provider should not replay any events.
	//
	// Replay operations must be executed in the same goroutine as the one it is called in.
	// Other goroutines may be launched from inside the Replay method, but the events must
	// be sent to the listener in the same goroutine that Replay is called in.
	Replay(subscription Subscription)
	// GC triggers a cleanup. After GC returns, all the events that are invalid according
	// to the provider's criteria should be impossible to replay again.
	//
	// If GC returns an error, the provider is not required to try to trigger another
	// GC ever again. Make sure that before you return a non-nil value you handle
	// temporary errors accordingly, with blocking as shortly as possible.
	// If your implementation does not require GC, return a non-nil error from this method.
	// This way server providers won't try to GC at all, improving their performance.
	//
	// If the replay provider implementation is thread-safe the GC operation can be executed in another goroutine.
	GC() error
}

type (
	subscriber  chan<- *Message
	subscribers map[subscriber]struct{}
)

// Joe is a basic server provider that synchronously executes operations by queueing them in channels.
// Events are also sent synchronously to subscribers, and Joe waits for them to be received - if a
// subscriber is misbehaving Joe might wait forever! Make sure the listener channels are always
// open for receiving.
//
// Joe supports event replaying with the help of a replay provider. As operations are executed
// synchronously, it is guaranteed that no new events will be omitted from sending to a new subscriber
// because older events are still replaying when the event is sent to Joe.
//
// If due to some unexpected scenario (the replay provider has a bug, for example) a panic occurs,
// Joe will close all the subscribers' channels, so requests aren't closed abruptly.
//
// He serves simple use-cases well, as he's light on resources, and does not require any external
// services. Also, he is the default provider for Servers.
type Joe struct {
	message        chan *Message
	subscription   chan Subscription
	unsubscription chan subscriber
	done           chan struct{}
	closed         chan struct{}
	gc             <-chan time.Time
	stopGC         func()
	topics         map[string]subscribers
	subscribers    subscribers
	replay         ReplayProvider
}

// JoeConfig is used to optionally configure a replay provider for Joe.
type JoeConfig struct {
	// An optional replay provider that Joe uses to resend older messages to new subscribers.
	ReplayProvider ReplayProvider
	// An optional interval at which Joe triggers a cleanup of expired messages, if the replay provider supports it.
	// See the desired provider's documentation to determine if periodic cleanup is necessary.
	ReplayGCInterval time.Duration
}

// NewJoe creates and starts a Joe (the default provider for servers).
// You can optionally pass a JoeConfig if you want to use a ReplayProvider with Joe.
func NewJoe(configuration ...JoeConfig) *Joe {
	config := joeConfig(configuration)

	gc, stopGCTicker := ticker(config.ReplayGCInterval)

	j := &Joe{
		message:        make(chan *Message),
		subscription:   make(chan Subscription),
		unsubscription: make(chan subscriber),
		done:           make(chan struct{}),
		closed:         make(chan struct{}),
		gc:             gc,
		stopGC:         stopGCTicker,
		topics:         map[string]subscribers{},
		subscribers:    subscribers{},
		replay:         config.ReplayProvider,
	}

	go j.start()

	return j
}

// Subscribe tells Joe to send new messages to the given channel. The subscriber is removed when the context is done.
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

// Publish tells Joe to send the given message to the subscribers.
func (j *Joe) Publish(msg *Message) error {
	// Waiting on done ensures Publish doesn't block the caller goroutine
	// when Joe is stopped and implements the required Provider behavior.
	select {
	case j.message <- msg:
		return nil
	case <-j.done:
		return ErrProviderClosed
	}
}

// Stop signals Joe to close all subscribers and stop receiving messages.
// It returns when all the subscribers are closed.
//
// Further calls to Stop will return ErrProviderClosed.
func (j *Joe) Stop() error {
	// Waiting on Stop here prevents double-closing and implements the required Provider behavior.
	select {
	case <-j.done:
		return ErrProviderClosed
	default:
		close(j.done)
		<-j.closed
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
	defer close(j.closed)
	// defer closing all subscribers instead of closing them when done is closed
	// so in case of a panic subscribers won't block the request goroutines forever.
	defer j.closeSubscribers()
	defer j.stopGC()

	for {
		select {
		case msg := <-j.message:
			j.replay.Put(&msg)

			for sub := range j.topics[msg.Topic] {
				sub <- msg
			}
		case sub := <-j.subscription:
			if _, ok := j.subscribers[sub.Channel]; ok {
				continue
			}

			j.replay.Replay(sub)

			for _, topic := range sub.Topics {
				j.topic(topic)[sub.Channel] = struct{}{}
			}
			j.subscribers[sub.Channel] = struct{}{}
		case unsub := <-j.unsubscription:
			if _, ok := j.subscribers[unsub]; !ok {
				continue
			}
			for _, subs := range j.topics {
				delete(subs, unsub)
			}

			delete(j.subscribers, unsub)
			close(unsub)
		case <-j.gc:
			if err := j.replay.GC(); err != nil {
				j.stopGC()
			}
		case <-j.done:
			return
		}
	}
}

func (j *Joe) closeSubscribers() {
	for sub := range j.subscribers {
		close(sub)
	}
}

var _ Provider = (*Joe)(nil)

// joeConfig takes the NewJoe function's input and returns a valid configuration.
func joeConfig(input []JoeConfig) JoeConfig {
	cfg := JoeConfig{}
	if len(input) > 0 {
		cfg = input[0]
	}

	if cfg.ReplayProvider == nil {
		if cfg.ReplayGCInterval > 0 {
			cfg.ReplayGCInterval = 0
		}
		cfg.ReplayProvider = noopReplayProvider{}
	}

	return cfg
}

// topics returns a slice containing the default topic if no topics are given.
// It returns the given slice otherwise.
func topics(topics []string) []string {
	if len(topics) == 0 {
		return []string{DefaultTopic}
	}
	return topics
}

func noop() {}

// ticker creates a time.Ticker, if duration is positive, and returns its channel and stop function.
// If the duration is negative, it returns a nil channel and a noop function.
func ticker(duration time.Duration) (ticks <-chan time.Time, stop func()) {
	if duration <= 0 {
		return nil, noop
	}
	t := time.NewTicker(duration)
	return t.C, t.Stop
}
