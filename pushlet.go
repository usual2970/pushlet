package pushlet

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Pushlet 是消息推送系统的主要入口点
type Pushlet struct {
	broker            *Broker
	heartbeatInterval time.Duration // 心跳间隔
}

// New 创建一个新的 Pushlet 实例
func New() *Pushlet {
	return &Pushlet{
		broker:            NewBroker(),
		heartbeatInterval: 30 * time.Second, // 默认30秒心跳
	}
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
	log.Println("New client requested connection:", client.ID, "topic:", topic)

	// 注册客户端到代理
	p.broker.Register(client, topic)
	defer p.broker.Unregister(client)

	// 通知客户端连接已建立
	log.Println("Sending connection message to client:", client.ID, "topic:", topic)
	client.SendMessage(NewMessage(topic, "connected", "Connection established"))
	log.Println("Connection message sent to client:", client.ID, "topic:", topic)

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
				log.Println("Client channel closed:", client.ID, "topic:", topic)
				return
			}

			// 写入 SSE 格式的消息
			eventStr := "event: " + msg.Event + "\n"
			dataStr := "data: " + msg.Data + "\n\n"

			_, err := w.Write([]byte(eventStr + dataStr))
			if err != nil {
				log.Println("Error writing to client:", client.ID, "error:", err)
				return
			}
			flusher.Flush()

		case <-heartbeatTicker.C:
			// 发送心跳注释行
			heartbeatMsg := ": heartbeat " + time.Now().Format("2006-01-02 15:04:05") + "\n\n"
			_, err := w.Write([]byte(heartbeatMsg))
			if err != nil {
				log.Println("Error sending heartbeat to client:", client.ID, "error:", err)
				return
			}
			flusher.Flush()
			log.Printf("Heartbeat sent to client: %s, topic: %s", client.ID, topic)

		case <-ctx.Done():
			// 客户端断开连接
			log.Println("Client disconnected:", client.ID, "topic:", topic)
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
		log.Println("WebSocket upgrade failed:", err)
		return
	}
	defer conn.Close()

	// 获取主题
	topic := r.URL.Query().Get("topic")
	if topic == "" || topic == "/" {
		topic = "default"
	}

	// 创建新客户端
	client := NewClient()
	log.Println("New WebSocket client requested connection:", client.ID, "topic:", topic)

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
	if err := conn.WriteJSON(connectMsg); err != nil {
		log.Println("Error sending connection message:", err)
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
				log.Println("WebSocket client channel closed:", client.ID, "topic:", msg.Topic)
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 发送消息到客户端
			wsMsg := map[string]interface{}{
				"event":     msg.Event,
				"data":      msg.Data,
				"topic":     msg.Topic,
				"timestamp": time.Now().Unix(),
			}

			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(wsMsg); err != nil {
				log.Println("Error writing to WebSocket client:", client.ID, "error:", err)
				return
			}

		case <-heartbeatTicker.C:
			// 发送 ping 消息作为心跳
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Println("Error sending ping to WebSocket client:", client.ID, "error:", err)
				return
			}
			log.Printf("Ping sent to WebSocket client: %s, topic: %s", client.ID, topic)
		}
	}
}

// handleWebSocketReads 处理从 WebSocket 客户端接收的消息
func (p *Pushlet) handleWebSocketReads(conn *websocket.Conn, client *Client) {
	defer conn.Close()

	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for client %s: %v", client.ID, err)
			}
			log.Println("WebSocket client disconnected:", client.ID)
			break
		}

		// 处理客户端消息（可选功能）
		if msgType, ok := msg["type"].(string); ok {
			switch msgType {
			case "ping":
				// 响应客户端的 ping
				pongMsg := map[string]interface{}{
					"type":      "pong",
					"timestamp": time.Now().Unix(),
				}
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				conn.WriteJSON(pongMsg)

			case "subscribe":
				// 处理主题订阅（如果需要动态订阅功能）
				if newTopic, ok := msg["topic"].(string); ok && newTopic != "" {
					log.Printf("Client %s requesting subscription: %s", client.ID, newTopic)
					p.broker.Subscribe(client, newTopic)
				}
			case "unsubscribe":
				// 处理主题取消订阅（如果需要动态取消订阅功能）
				if oldTopic, ok := msg["topic"].(string); ok && oldTopic != "" {
					log.Printf("Client %s requesting topic unsubscription: %s", client.ID, oldTopic)
					p.broker.Unsubscribe(client, oldTopic)
				}

			default:
				log.Printf("Unknown message type from client %s: %s", client.ID, msgType)
			}
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
