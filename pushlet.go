package pushlet

import (
	"net/http"
)

// Pushlet 是消息推送系统的主要入口点
type Pushlet struct {
	broker *Broker
}

// New 创建一个新的 Pushlet 实例
func New() *Pushlet {
	return &Pushlet{
		broker: NewBroker(),
	}
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

	// 获取请求路径作为主题
	topic := r.URL.Query().Get("topic")
	if topic == "" || topic == "/" {
		topic = "default"
	}

	// 创建新客户端
	client := NewClient(topic)

	// 注册客户端到代理
	p.broker.Register(client)
	defer p.broker.Unregister(client)

	// 通知客户端连接已建立
	client.SendMessage(NewMessage("connected", "Connection established"))

	// 保持连接打开直到客户端断开
	ctx := r.Context()
	flusher := w.(http.Flusher)

	// 监听来自客户端的消息
	go func() {
		for msg := range client.Send {
			// 写入 SSE 格式的消息
			eventStr := "event: " + msg.Event + "\n"
			dataStr := "data: " + msg.Data + "\n\n"

			_, err := w.Write([]byte(eventStr + dataStr))
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}()

	// 等待客户端断开连接
	<-ctx.Done()
}

// Publish 向指定主题发布消息
func (p *Pushlet) Publish(topic, event, data string) {
	p.broker.Publish(topic, NewMessage(event, data))
}

// PublishToAll 向所有主题发布消息
func (p *Pushlet) PublishToAll(event, data string) {
	p.broker.PublishToAll(NewMessage(event, data))
}
