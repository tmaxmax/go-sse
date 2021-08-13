package field

import "io"

// Field is the interface that all event stream field types must adhere to.
// The implementations must be idempotent and safe for concurrent use!
type Field interface {
	// name returns the field's name. It is unexported so implementations with
	// names that do not adhere to the standard are prevented. Implement this
	// method by embedding Data in your custom implementations.
	name() string

	io.WriterTo
}

type singleLine interface {
	Field

	singleLine()
}

// SingleLine is a struct that's embedded in the field implementations which
// have values without line endings. Use this in your own custom data field
// implementations for optimized writes to the client.
//
// Be cautious about mistakenly using this for fields with multiline values.
// This library does not check for newlines in the given data, and if they
// exist and the field is marked as single-line the transfer protocol will
// be broken.
type SingleLine struct{}

func (s SingleLine) singleLine() {}
