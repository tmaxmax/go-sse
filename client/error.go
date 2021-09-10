package client

import (
	"errors"
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

// Temporary returns whether the underlying error is temporary.
func (e *Error) Temporary() bool {
	var t interface{ Temporary() bool }
	if errors.As(e.Err, &t) {
		return t.Temporary()
	}
	return false
}

// Timeout returns whether the underlying error is caused by a timeout.
func (e *Error) Timeout() bool {
	var t interface{ Timeout() bool }
	if errors.As(e.Err, &t) {
		return t.Timeout()
	}
	return false
}

func (e *Error) toPermanent() error {
	if e.Temporary() || e.Timeout() {
		return e
	}
	return backoff.Permanent(e)
}
