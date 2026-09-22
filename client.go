package pushlet

import (
	"sync"
	"time"
)

// Client 表示一个 SSE 客户端连接
type Client struct {
	ID   string
	Send chan *Message

	sendMu     sync.Mutex
	sendClosed bool
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
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.sendClosed {
		return
	}
	c.sendClosed = true
	close(c.Send)
}

// SendClosed reports whether the outbound channel is already closed.
func (c *Client) SendClosed() bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.sendClosed
}

// SendMessage delivers msg to the client. Returns false if the client was
// dropped (buffer full or already closed). Send to a closed channel never runs.
func (c *Client) SendMessage(msg *Message) bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.sendClosed {
		return false
	}
	select {
	case c.Send <- msg:
		return true
	default:
		c.sendClosed = true
		close(c.Send)
		return false
	}
}

// 生成唯一ID
func generateID() string {
	return randomString(16)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[randomInt(len(charset))]
	}
	return string(b)
}

func randomInt(max int) int {
	return int(time.Now().UnixNano() % int64(max))
}
