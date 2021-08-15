package hub

import "io"

// A Messager is a type that can write the same data concurrently and multiple times.
type Messager interface {
	// Message sends data to the Connection by writing it to its underlying writer.
	Message(io.Writer) error
}

type connectionMessage struct {
	connection *Connection
	Messager
}

type attachment struct {
	connection                        *Connection
	supportsBroadcastsFromConnections chan<- bool
}
