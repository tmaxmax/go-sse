package sse

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

// The EventID struct represents any valid event ID value.
// IDs must be passed around as values, not as pointers!
// They can also safely be used as map keys.
type EventID struct {
	value string
	set   bool
}

// NewEventID creates an EventID value. It also returns a flag that indicates whether the input
// is a valid ID. A valid ID must not have any newlines. If the input is not valid,
// an unset (invalid) ID is returned.
func NewEventID(value string) (EventID, error) {
	if !isSingleLine(value) {
		return EventID{}, fmt.Errorf("input is not a valid EventID: %q", value)
	}
	return EventID{value: value, set: true}, nil
}

// MustEventID is the same as NewEventID, but it panics if the input isn't a valid ID.
func MustEventID(value string) EventID {
	id, err := NewEventID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// IsSet returns true if the receiver is a valid (set) ID value.
func (i EventID) IsSet() bool {
	return i.set
}

// String returns the ID's value. The value may be an empty string,
// make sure to check if the ID is set before using the value.
func (i EventID) String() string {
	return i.value
}

// UnmarshalText sets the ID's value to the given string, if valid.
// If the input is invalid, the previous value is discarded.
func (i *EventID) UnmarshalText(data []byte) error {
	*i = EventID{}

	id, err := NewEventID(string(data))
	if err != nil {
		return err
	}

	*i = id

	return nil
}

// UnmarshalJSON sets the ID's value to the given JSON value
// if the value is a string and it doesn't contain any null bytes.
// The previous value is discarded if the operation fails.
func (i *EventID) UnmarshalJSON(data []byte) error {
	*i = EventID{}

	if string(data) == "null" {
		return nil
	}

	var input string

	if err := json.Unmarshal(data, &input); err != nil {
		return err
	}

	id, err := NewEventID(input)
	if err != nil {
		return err
	}

	*i = id

	return nil
}

// ErrIDUnset is returned when calling MarshalText for an unset ID.
var ErrIDUnset = errors.New("tried to marshal to text an unset ID")

// MarshalText returns a copy of the ID's value if it is set.
// It returns an error when trying to marshal an unset ID.
func (i *EventID) MarshalText() ([]byte, error) {
	if i.IsSet() {
		return []byte(i.String()), nil
	}

	return nil, ErrIDUnset
}

// MarshalJSON returns a JSON representation of the ID's value if it is set.
// It otherwise returns the representation of the JSON null value.
func (i *EventID) MarshalJSON() ([]byte, error) {
	if i.IsSet() {
		return json.Marshal(i.String())
	}

	return json.Marshal(nil)
}

// Scan implements the sql.Scanner interface. IDs can be scanned from:
//   - nil interfaces (result: unset ID)
//   - byte slice
//   - string
func (i *EventID) Scan(src interface{}) error {
	*i = EventID{}

	if src == nil {
		return nil
	}

	switch v := src.(type) {
	case []byte:
		i.value = string(v)
	case string:
		i.value = string([]byte(v))
	default:
		return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, *i)
	}

	i.set = true

	return nil
}

// Value implements the driver.Valuer interface.
func (i EventID) Value() (driver.Value, error) {
	if i.IsSet() {
		return i.String(), nil
	}
	return nil, nil
}
