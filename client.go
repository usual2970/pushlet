package pushlet

import (
	"sync"
	"time"
)

// Client 表示一个 SSE 客户端连接
type Client struct {
	ID   string
	Send chan *Message

	closeSend sync.Once
}

// NewClient 创建新的客户端连接
func NewClient() *Client {
	return &Client{
		ID:   generateID(),
		Send: make(chan *Message, 256), // 缓冲通道以避免阻塞
	}
}

// CloseSend closes the outbound channel at most once.
func (c *Client) CloseSend() {
	c.closeSend.Do(func() {
		close(c.Send)
	})
}

// SendMessage 向客户端发送消息
func (c *Client) SendMessage(msg *Message) {
	select {
	case c.Send <- msg:
	default:
		// 通道已满，可能客户端处理过慢；幂等关闭，避免 Unregister/cleanup 再次 close panic
		c.CloseSend()
	}
}

// 生成唯一ID
func generateID() string {
	// 简单实现，实际可以使用更复杂的方法如 UUID
	return randomString(16)
}

// 生成随机字符串
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[randomInt(len(charset))]
	}
	return string(b)
}

// 生成随机整数
func randomInt(max int) int {
	// 简单实现，实际可以使用 crypto/rand
	return int(time.Now().UnixNano() % int64(max))
}
