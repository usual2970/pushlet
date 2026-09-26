package pushlet

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/usual2970/novaque"
)

var okBytes = []byte("OK")

// Pushlet is the main entry point for SSE and WebSocket pub/sub.
type Pushlet struct {
	broker            *Broker
	heartbeatInterval time.Duration // 心跳间隔
	newLogger         NewLogger

	asyncOpts     AsyncPublishOptions
	asyncOut      chan asyncOutbound
	asyncQuit     chan struct{}
	asyncDone     chan struct{}
	asyncDropped  atomic.Uint64
	asyncStopOnce sync.Once
}

// Option configures a [Pushlet] in [New].
type Option func(*Pushlet)

// WithLogger sets the factory used to create loggers for connection handling.
func WithLogger(newLogger NewLogger) Option {
	return func(p *Pushlet) {
		p.newLogger = newLogger
	}
}

// New returns a Pushlet with a started-ready broker and default heartbeat interval.
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

// SetHeartbeatInterval configures SSE comment heartbeats and WebSocket ping intervals.
func (p *Pushlet) SetHeartbeatInterval(interval time.Duration) {
	p.heartbeatInterval = interval
}

// EnableDistributedMode enables multi-instance fan-out via a [DistributedConnector].
func (p *Pushlet) EnableDistributedMode(connector DistributedConnector) error {
	return p.broker.EnableDistributedMode(connector)
}

// EnableDistributedNovaque enables distributed mode via an opened novaque client.
func (p *Pushlet) EnableDistributedNovaque(client *novaque.Client, opts DistributedOptions) error {
	return p.broker.EnableDistributedNovaque(client, opts)
}

// Start runs the internal broker and any distributed relay goroutines.
func (p *Pushlet) Start() {
	p.broker.Start()
}

// Stop shuts down the async publisher (when enabled), then the broker and
// distributed connector.
func (p *Pushlet) Stop() {
	if p == nil {
		return
	}
	p.stopAsyncPublisher()
	p.broker.Stop()
}

// HandleSSE serves a long-lived Server-Sent Events stream.
// The topic is taken from the "topic" query parameter; empty values use "default".
func (p *Pushlet) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	topic := r.URL.Query().Get("topic")
	if topic == "" || topic == "/" {
		topic = "default"
	}
	p.serveSSE(w, r, topic)
}

func (p *Pushlet) serveSSE(w http.ResponseWriter, r *http.Request, topic string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	client := NewClient()
	p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("New client requested connection")

	if err := p.broker.Register(client, topic); err != nil {
		http.Error(w, "broker not ready", http.StatusServiceUnavailable)
		return
	}
	defer p.broker.Unregister(client)

	p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Sending connection message to client:")
	client.SendMessage(NewMessage(topic, "connected", "Connection established"))
	p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Connection message sent to client:")

	ctx := r.Context()
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	heartbeatTicker := time.NewTicker(p.heartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case msg, ok := <-client.Send:
			if !ok {
				p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Client channel closed:")
				return
			}
			eventStr := "event: " + msg.Event + "\n"
			dataStr := "data: " + msg.Data + "\n\n"
			if _, err := w.Write([]byte(eventStr + dataStr)); err != nil {
				p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Error writing to client:", err)
				return
			}
			flusher.Flush()

		case <-heartbeatTicker.C:
			heartbeatMsg := ": heartbeat " + time.Now().Format("2006-01-02 15:04:05") + "\n\n"
			if _, err := w.Write([]byte(heartbeatMsg)); err != nil {
				p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Error writing heartbeat to client:", err)
				return
			}
			flusher.Flush()
			p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("Heartbeat sent to client:")

		case <-ctx.Done():
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
	EnableCompression: true,
}

// HandleWebsocket upgrades the request to a WebSocket and streams JSON [Message] payloads.
// An optional initial topic may be passed via the "topic" query parameter.
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

	if err := p.broker.Register(client, topic); err != nil {
		http.Error(w, "broker not ready", http.StatusServiceUnavailable)
		return
	}
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
				p.newLogger().WithField("client_id", client.ID).WithField("topic", topic).Println("WebSocket client channel closed:")
				p.writeWsMessage(conn, websocket.CloseMessage, []byte{})
				return
			}

			// 发送消息到客户端
			wsMsg := NewMessage(msg.Topic, msg.Event, msg.Data)
			data, err := json.Marshal(wsMsg)
			if err != nil {
				p.newLogger().Println("Error marshalling WebSocket message:", err)
				return
			}

			data = []byte(msg.Topic + " " + string(data))
			if err := p.writeWsMessage(conn, websocket.BinaryMessage, data); err != nil {
				p.newLogger().WithField("client_id", client.ID).WithField("topic", msg.Topic).Println("Error writing to WebSocket client:", err)
				return
			}

		case <-heartbeatTicker.C:
			// 发送 ping 消息作为心跳
			if err := p.writeWsMessage(conn, websocket.PingMessage, nil); err != nil {
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

		// 处理不同类型的消息
		switch messageType {
		case websocket.PingMessage:
			// 收到 ping，发送 pong 回复
			p.newLogger().WithField("client_id", client.ID).Println("Received ping, sending pong")
			if err := p.writeWsMessage(conn, websocket.PongMessage, nil); err != nil {
				p.newLogger().WithField("client_id", client.ID).Println("Error sending pong:", err)
				return
			}
			continue

		case websocket.PongMessage:
			// 收到 pong 消息
			p.newLogger().WithField("client_id", client.ID).Println("Received pong from client")
			continue

		case websocket.CloseMessage:
			// 收到关闭消息
			p.newLogger().WithField("client_id", client.ID).Println("Received close message from client")
			return

		case websocket.TextMessage:
			// 处理文本消息（如果需要支持的话）
			p.newLogger().WithField("client_id", client.ID).Println("Received text message from client:", string(bts))
			continue

		case websocket.BinaryMessage:
			// 处理二进制消息（业务逻辑）
			// 继续执行下面的业务逻辑处理

		default:
			p.newLogger().WithField("client_id", client.ID).Println("Received unknown message type:", messageType)
			continue
		}

		// 只有二进制消息才进行业务逻辑处理
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
		resp, err := p.exec(parts, client)
		if err != nil {
			p.newLogger().WithField("client_id", client.ID).Println("Error executing command from client:", err)
			continue
		}

		if err := p.writeWsMessage(conn, websocket.BinaryMessage, resp); err != nil {
			p.newLogger().WithField("client_id", client.ID).Println("Error writing to WebSocket client:", err)
			continue
		}
		p.newLogger().WithField("client_id", client.ID).Println("Command executed successfully, response sent to client:")
	}
}

func (p *Pushlet) writeWsMessage(conn *websocket.Conn, msgType int, msg []byte) error {
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(msgType, msg); err != nil {
		p.newLogger().Println("Error writing to WebSocket client:", err)
		return err
	}
	p.newLogger().Println("Message sent to WebSocket client successfully:")
	return nil
}

func (p *Pushlet) exec(parts [][]byte, client *Client) ([]byte, error) {
	log := p.newLogger().WithField("client_id", client.ID)
	log.Println("Executing command from client:", parts)
	var err error
	defer func() {
		if err != nil {
			log.Println("Error executing command:", err)
		}
	}()
	switch {
	case bytes.Equal(parts[0], []byte("SUB")):
		// A malformed command must never panic: parts[1] below assumes the
		// frame carried a topic argument.
		if len(parts) < 2 {
			err = errors.New("SUB requires a topic")
			return nil, err
		}
		topic := string(parts[1])
		log.WithField("topic", topic).Println("Client subscribed to topic:")
		p.broker.Subscribe(client, topic)

		return okBytes, nil

	case bytes.Equal(parts[0], []byte("UNSUB")):
		// Same guard as SUB: parts[1] below requires a topic argument.
		if len(parts) < 2 {
			err = errors.New("UNSUB requires a topic")
			return nil, err
		}
		topic := string(parts[1])
		log.WithField("topic", topic).Println("Client unsubscribing from topic:")
		p.broker.Unsubscribe(client, topic)

		return okBytes, nil
	case bytes.Equal(parts[0], []byte("PING")):
		// 处理主题取消订阅（如果需要动态取消订阅功能）
		log.Println("Received PING from client:")
		return okBytes, nil

	default:
		err = errors.New("unknown command")
		return nil, err
	}
}

// Publish sends an event to all clients subscribed to topic.
func (p *Pushlet) Publish(topic, event, data string) error {
	if p == nil {
		return nil
	}
	return p.enqueuePublish(topic, event, data, false)
}

// PublishToAll broadcasts an event to every connected client regardless of topic.
func (p *Pushlet) PublishToAll(event, data string) error {
	if p == nil {
		return nil
	}
	return p.enqueuePublish("", event, data, true)
}
