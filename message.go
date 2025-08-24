package pushlet

import (
	"encoding/json"
	"time"
)

// Message 表示一个 SSE 消息
type Message struct {
	Topic     string    `json:"topic"`
	Event     string    `json:"event"`
	Data      string    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

// NewMessage 创建一个新的消息
func NewMessage(topic, event, data string) *Message {
	return &Message{
		Topic:     topic,
		Event:     event,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// String 将消息转换为字符串
func (m *Message) String() string {
	bytes, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

// FromJSON 从 JSON 字符串创建消息
func MessageFromJSON(jsonStr string) (*Message, error) {
	var msg Message
	err := json.Unmarshal([]byte(jsonStr), &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}
