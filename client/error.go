package client

import (
	"fmt"
	"net/http"

	"github.com/cenkalti/backoff/v4"
)

// Error is the type that wraps all the connection errors that occur.
type Error struct {
	// The request for which the connection failed.
	Req *http.Request
	// The reason the operation failed.
	Err error
	// The reason why the request failed.
	Reason string
}

func (e *Error) Error() string {
	return fmt.Sprintf("request failed: %s: %v", e.Reason, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) Temporary() bool {
	t, ok := e.Err.(interface{ Temporary() bool })
	return ok && t.Temporary()
}

func (e *Error) Timeout() bool {
	t, ok := e.Err.(interface{ Timeout() bool })
	return ok && t.Timeout()
}

func (e *Error) toPermanent() error {
	if e.Temporary() || e.Timeout() {
		return e
	}
	return backoff.Permanent(e)
}
