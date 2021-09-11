package client

// The Event struct represents an event sent to the helloworld_client by the server.
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
