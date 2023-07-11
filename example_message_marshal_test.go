package sse_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tmaxmax/go-sse"
)

type MessageMarshaler struct {
	*sse.Message
}

type messageMarshalFormat struct {
	Topic     string       `json:"topic"`
	ExpiresAt time.Time    `json:"expiresAt"`
	Message   *sse.Message `json:"message"`
}

func (m MessageMarshaler) MarshalJSON() ([]byte, error) {
	return json.Marshal(messageMarshalFormat{
		Topic:     m.Topic,
		ExpiresAt: m.ExpiresAt,
		Message:   m.Message,
	})
}

func (m *MessageMarshaler) UnmarshalJSON(data []byte) error {
	var msg messageMarshalFormat
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}

	m.Message = msg.Message
	m.Message.Topic = msg.Topic
	m.Message.ExpiresAt = msg.ExpiresAt

	return nil
}

func Example_messageCustomJSONMarshal() {
	m := &sse.Message{Topic: "Hello", ExpiresAt: time.Now().Add(time.Hour)}
	m.AppendData("hello", "world")

	data, _ := json.Marshal(MessageMarshaler{m})

	var um MessageMarshaler
	_ = json.Unmarshal(data, &um)

	fmt.Println(
		m.Topic == um.Topic,
		m.ExpiresAt.Equal(um.ExpiresAt),
		m.String() == um.String(),
	)
	// Output: true true true
}
