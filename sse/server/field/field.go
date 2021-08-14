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
