package replay

import (
	"fmt"

	"github.com/tmaxmax/go-sse/server/event"
)

type Error struct {
	err error
	id  event.ID
}

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("server.replay.Provider: invalid ID %q: %s", e.id, e.err.Error())
	}
	return fmt.Sprintf("server.replay.Provider: ID %q does not exist", e.id)
}
