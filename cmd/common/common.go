package common

import (
	"context"
	"errors"
	"io"
	"sync"
)

type ConcurrentWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (m *ConcurrentWriter) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.w.Write(p)
}

func (m *ConcurrentWriter) WriteString(s string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return io.WriteString(m.w, s)
}

func NewConcurrentWriter(w io.Writer) *ConcurrentWriter {
	return &ConcurrentWriter{w: w}
}

func IsContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
