package sse

import (
	"io"
	"strings"
)

type Messager interface {
	Message() (*Message, error)
}

type Message struct {
	Event string
	Data  []byte
}

var eventPrefix = []byte("event: ")
var dataPrefix = []byte("\ndata: ")
var newlines = []byte("\n\n")

func (m *Message) WriteTo(w io.Writer) (int64, error) {
	if m.Data == nil {
		return 0, nil
	}

	var (
		chunks       [5][]byte
		totalWritten int
	)

	if m.Event != "" {
		chunks[0] = eventPrefix
		chunks[1] = []byte(m.Event)
		chunks[2] = dataPrefix
	} else {
		chunks[2] = dataPrefix[1:]
	}
	chunks[3] = m.Data
	chunks[4] = newlines

	for _, data := range chunks {
		if data == nil {
			continue
		}

		n, err := w.Write(data)
		totalWritten += n
		if err != nil {
			return int64(totalWritten), err
		}
	}

	return int64(totalWritten), nil
}

func (m *Message) String() string {
	sb := &strings.Builder{}

	_, _ = m.WriteTo(sb)

	return sb.String()
}

func (m *Message) Message() (*Message, error) {
	return m, nil
}
