package pushlet

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var okBytes = []byte("OK")

// Pushlet 是消息推送系统的主要入口点
type Pushlet struct {
	broker            *Broker
	heartbeatInterval time.Duration // 心跳间隔
	newLogger         NewLogger
}

type Option func(*Pushlet)

func WithLogger(newLogger NewLogger) Option {
	return func(p *Pushlet) {
		p.newLogger = newLogger
	}
}

// New 创建一个新的 Pushlet 实例
func New(options ...Option) *Pushlet {
	p := &Pushlet{
		broker:            NewBroker(),
		heartbeatInterval: 30 * time.Second, // 默认30秒心跳
	}

	for _, opt := range options {
		opt(p)
	}

	if p.newLogger == nil {
		p.newLogger = NewDefaultLogger
	}

	return p
}

// SetHeartbeatInterval 设置心跳间隔
func (p *Pushlet) SetHeartbeatInterval(interval time.Duration) {
	p.heartbeatInterval = interval
}

// EnableDistributedMode 启用分布式模式
func (p *Pushlet) EnableDistributedMode(redisAddr, redisPassword string, redisDB int) error {
	return p.broker.EnableDistributedMode(redisAddr, redisPassword, redisDB)
}

// Start 启动消息代理
func (p *Pushlet) Start() {
	p.broker.Start()
}

// Stop 停止消息代理
func (p *Pushlet) Stop() {
	p.broker.Stop()
}

// HandleSSE 处理 SSE 连接请求
// 路径参数可以用于区分不同的主题
func (p *Pushlet) HandleSSE(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 设置 SSE 相关头部
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// 获取请求路径作为主题
	topic := r.URL.Query().Get("topic")
	if topic == "" || topic == "/" {
		topic = "default"
	}

	// 创建新客户端
	client := NewClient()
	p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("New client requested connection")

	// 注册客户端到代理
	p.broker.Register(client, topic)
	defer p.broker.Unregister(client)

	// 通知客户端连接已建立
	p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Sending connection message to client:")
	client.SendMessage(NewMessage(topic, "connected", "Connection established"))
	p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Connection message sent to client:")
	// 获取请求上下文
	ctx := r.Context()
	flusher := w.(http.Flusher)

	// 创建心跳定时器
	heartbeatTicker := time.NewTicker(p.heartbeatInterval)
	defer heartbeatTicker.Stop()

	// 监听消息、心跳和断开连接
	for {
		select {
		case msg, ok := <-client.Send:
			if !ok {
				// 客户端通道已关闭
				p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Client channel closed:")
				return
			}

			// 写入 SSE 格式的消息
			eventStr := "event: " + msg.Event + "\n"
			dataStr := "data: " + msg.Data + "\n\n"

			_, err := w.Write([]byte(eventStr + dataStr))
			if err != nil {
				p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Error writing to client:", err)
				return
			}
			flusher.Flush()

		case <-heartbeatTicker.C:
			// 发送心跳注释行
			heartbeatMsg := ": heartbeat " + time.Now().Format("2006-01-02 15:04:05") + "\n\n"
			_, err := w.Write([]byte(heartbeatMsg))
			if err != nil {
				p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Error writing heartbeat to client:", err)
				return
			}
			flusher.Flush()
			p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Heartbeat sent to client:")

		case <-ctx.Done():
			// 客户端断开连接
			p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Client disconnected:")
			return
		}
	}
}

// WebSocket 升级器
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，生产环境中应该更严格
	},
}

// HandleWebsocket 处理 WebSocket 连接请求
func (p *Pushlet) HandleWebsocket(w http.ResponseWriter, r *http.Request) {
	// 升级 HTTP 连接到 WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		p.newLogger().Println("WebSocket upgrade failed:", err)
		return
	}
	defer conn.Close()

	// 获取主题
	topic := r.URL.Query().Get("topic")
	if topic == "" || topic == "/" {
		topic = ""
	}

	// 创建新客户端
	client := NewClient()
	p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("New WebSocket client requested connection:")

	// 注册客户端到代理
	p.broker.Register(client, topic)
	defer p.broker.Unregister(client)

	// 设置连接参数
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	connectMsg := NewMessage(topic, "connected", "WebSocket connection established")
	connectBts, _ := json.Marshal(connectMsg)
	if err := conn.WriteMessage(websocket.BinaryMessage, connectBts); err != nil {
		p.newLogger().Println("Error sending connection message:", err)
		return
	}

	// 创建心跳定时器
	heartbeatTicker := time.NewTicker(p.heartbeatInterval)
	defer heartbeatTicker.Stop()

	// 启动读取 goroutine 处理客户端消息
	go p.handleWebSocketReads(conn, client)

	// 主循环处理发送消息和心跳
	for {
		select {
		case msg, ok := <-client.Send:
			if !ok {
				// 客户端通道已关闭
				p.newLogger().WithField("client_id", client.ID).WithField("topic", msg.Topic).Println("WebSocket client channel closed:")
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 发送消息到客户端
			wsMsg := NewMessage(msg.Topic, msg.Event, msg.Data)
			data, err := json.Marshal(wsMsg)
			if err != nil {
				p.newLogger().Println("Error marshalling WebSocket message:", err)
				return
			}

			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			data = []byte(msg.Topic + " " + string(data))
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				p.newLogger().WithField("client_id", client.ID).WithField("topic", msg.Topic).Println("Error writing to WebSocket client:", err)
				return
			}

		case <-heartbeatTicker.C:
			// 发送 ping 消息作为心跳
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				p.newLogger().WithField("client_id", client.ID).Println("Error sending ping to WebSocket client:", err)
				return
			}
			p.newLogger().WithField("client_id", client.ID).Println("Ping sent to WebSocket client:")
		}
	}
}

// handleWebSocketReads 处理从 WebSocket 客户端接收的消息
func (p *Pushlet) handleWebSocketReads(conn *websocket.Conn, client *Client) {
	defer conn.Close()

	for {
		messageType, bts, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				p.newLogger().WithField("client_id", client.ID).Println("WebSocket error:", err)
			}
			p.newLogger().WithField("client_id", client.ID).Println("WebSocket client disconnected:")
			break
		}

		if messageType != websocket.BinaryMessage {
			p.newLogger().WithField("client_id", client.ID).Println("Received non-binary message from client:", bts)
			continue
		}

		lines := bytes.Split(bts, []byte{'\n'})
		// 第一行：命令行 (SUB TOPIC\n)
		commandLine := lines[0]
		p.newLogger().Println("Received command:", commandLine)

		// 解析命令
		parts := bytes.Split(commandLine, []byte{' '})
		if len(parts) < 1 {
			p.newLogger().WithField("client_id", client.ID).Println("Invalid command from client:", commandLine)
			continue
		}

		// 处理客户端消息（可选功能）

		switch {
		case bytes.Equal(parts[0], []byte("SUB")):
			// 响应客户端的 ping
			topic := string(parts[1])
			p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Client subscribed to topic:")
			p.broker.Subscribe(client, topic)

			conn.WriteMessage(websocket.BinaryMessage, okBytes)

		case bytes.Equal(parts[0], []byte("UNSUB")):
			// 处理主题订阅（如果需要动态订阅功能）
			topic := string(parts[1])
			p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Client unsubscribing from topic:")
			p.broker.Unsubscribe(client, topic)

			conn.WriteMessage(websocket.BinaryMessage, okBytes)
		case bytes.Equal(parts[0], []byte("PING")):
			// 处理主题取消订阅（如果需要动态取消订阅功能）
			p.newLogger().WithField("client_id", client.ID).Println("Received PING from client:")
			conn.WriteMessage(websocket.BinaryMessage, okBytes)

		default:
			p.newLogger().WithField("client_id", client.ID).Println("Unknown message type from client:", string(parts[0]))
		}

	}
}

// Publish 向指定主题发布消息
func (p *Pushlet) Publish(topic, event, data string) {
	p.broker.Publish(topic, NewMessage(topic, event, data))
}

// PublishToAll 向所有主题发布消息
func (p *Pushlet) PublishToAll(event, data string) {
	p.broker.PublishToAll(NewMessage("global", event, data))
}
