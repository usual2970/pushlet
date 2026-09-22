package pushlet

import (
	"encoding/json"
	"time"
)

// Message is the JSON payload streamed to SSE and WebSocket clients.
type Message struct {
	Topic     string    `json:"topic"`
	Event     string    `json:"event"`
	Data      string    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

// NewMessage builds a message with the current timestamp.
func NewMessage(topic, event, data string) *Message {
	return &Message{
		Topic:     topic,
		Event:     event,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// String returns the JSON encoding of m, or "{}" if marshaling fails.
func (m *Message) String() string {
	bytes, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

// MessageFromJSON parses a message from a JSON object string.
func MessageFromJSON(jsonStr string) (*Message, error) {
	var msg Message
	err := json.Unmarshal([]byte(jsonStr), &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}
