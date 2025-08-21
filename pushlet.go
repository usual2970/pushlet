package pushlet

import (
	"log"
	"net/http"
	"time"
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
	client := NewClient(topic)
	log.Println("New client requested connection:", client.ID, "topic:", topic)

	// 注册客户端到代理
	p.broker.Register(client)
	defer p.broker.Unregister(client)

	// 通知客户端连接已建立
	log.Println("Sending connection message to client:", client.ID, "topic:", topic)
	client.SendMessage(NewMessage("connected", "Connection established"))
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

// Publish 向指定主题发布消息
func (p *Pushlet) Publish(topic, event, data string) {
	p.broker.Publish(topic, NewMessage(event, data))
}

// PublishToAll 向所有主题发布消息
func (p *Pushlet) PublishToAll(event, data string) {
	p.broker.PublishToAll(NewMessage(event, data))
}
