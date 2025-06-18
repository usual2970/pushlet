package pushlet

import (
	"sync"
)

// Broker 管理客户端连接和消息分发
type Broker struct {
	// 按主题组织的客户端映射
	clients map[string]map[*Client]bool

	// 注册客户端的通道
	register chan *Client

	// 注销客户端的通道
	unregister chan *Client

	// 发布消息的通道
	publish chan *PublishMessage

	// Redis连接器，用于分布式支持
	redisConnector *RedisConnector

	// 确保线程安全
	mu sync.RWMutex

	// 停止信号
	stop chan struct{}

	// 是否已启动
	running bool

	// 是否启用分布式模式
	distributedMode bool
}

// PublishMessage 包含发布信息
type PublishMessage struct {
	Topic   string
	Message *Message
	All     bool
}

// NewBroker 创建一个新的消息代理
func NewBroker() *Broker {
	return &Broker{
		clients:         make(map[string]map[*Client]bool),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		publish:         make(chan *PublishMessage),
		stop:            make(chan struct{}),
		distributedMode: false,
	}
}

// EnableDistributedMode 启用分布式模式
func (b *Broker) EnableDistributedMode(redisAddr, redisPassword string, redisDB int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.redisConnector = NewRedisConnector(redisAddr, redisPassword, redisDB)
	err := b.redisConnector.Start()
	if err != nil {
		return err
	}

	b.distributedMode = true

	// 如果broker已经在运行，则开始处理redis消息
	if b.running {
		go b.processRedisMessages()
	}

	return nil
}

// Start 启动消息代理
func (b *Broker) Start() {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	b.running = true
	b.mu.Unlock()

	go b.run()

	// 如果启用了分布式模式，启动Redis消息处理
	if b.distributedMode {
		go b.processRedisMessages()
	}
}

// Stop 停止消息代理
func (b *Broker) Stop() {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return
	}
	b.running = false
	b.mu.Unlock()

	// 停止Redis连接器
	if b.distributedMode && b.redisConnector != nil {
		b.redisConnector.Stop()
	}

	b.stop <- struct{}{}
}

// Register 注册一个新客户端
func (b *Broker) Register(client *Client) {
	b.register <- client

	// 在分布式模式下，订阅Redis中的主题
	if b.distributedMode && b.redisConnector != nil {
		b.redisConnector.SubscribeTopic(client.Topic)
	}
}

// Unregister 注销一个客户端
func (b *Broker) Unregister(client *Client) {
	b.unregister <- client
}

// Publish 向指定主题发布消息
func (b *Broker) Publish(topic string, msg *Message) {

	// 在分布式模式下，通过Redis发布
	if b.distributedMode && b.redisConnector != nil {
		b.redisConnector.PublishToTopic(topic, msg)
	} else {
		// 本地发布
		b.publish <- &PublishMessage{
			Topic:   topic,
			Message: msg,
			All:     false,
		}
	}
}

// PublishToAll 向所有主题发布消息
func (b *Broker) PublishToAll(msg *Message) {

	// 在分布式模式下，通过Redis发布
	if b.distributedMode && b.redisConnector != nil {
		b.redisConnector.PublishToAll(msg)
	} else {
		// 本地发布
		b.publish <- &PublishMessage{
			Message: msg,
			All:     true,
		}
	}
}

// processRedisMessages 处理从Redis接收的消息
func (b *Broker) processRedisMessages() {
	if !b.distributedMode || b.redisConnector == nil {
		return
	}

	messageChan := b.redisConnector.GetMessageChannel()

	for msg := range messageChan {
		// 将Redis消息转发到本地客户端
		b.publish <- msg
	}
}

// run 运行消息代理的主循环
func (b *Broker) run() {
	for {
		select {
		case client := <-b.register:
			b.mu.Lock()
			if _, ok := b.clients[client.Topic]; !ok {
				b.clients[client.Topic] = make(map[*Client]bool)
			}
			b.clients[client.Topic][client] = true
			b.mu.Unlock()

		case client := <-b.unregister:
			b.mu.Lock()
			if _, ok := b.clients[client.Topic]; ok {
				if _, ok := b.clients[client.Topic][client]; ok {
					delete(b.clients[client.Topic], client)
					close(client.Send)

					// 如果该主题下没有客户端了，可以考虑在Redis中取消订阅
					if len(b.clients[client.Topic]) == 0 {
						delete(b.clients, client.Topic)
						if b.distributedMode && b.redisConnector != nil {
							b.redisConnector.UnsubscribeTopic(client.Topic)
						}
					}
				}
			}
			b.mu.Unlock()

		case pm := <-b.publish:
			b.mu.RLock()
			if pm.All {
				// 发送给所有主题的所有客户端
				for _, clients := range b.clients {
					for client := range clients {
						client.SendMessage(pm.Message)
					}
				}
			} else {
				// 发送给特定主题的客户端
				if clients, ok := b.clients[pm.Topic]; ok {
					for client := range clients {
						client.SendMessage(pm.Message)
					}
				}
			}
			b.mu.RUnlock()

		case <-b.stop:
			// 关闭所有客户端连接
			b.mu.Lock()
			for _, clients := range b.clients {
				for client := range clients {
					close(client.Send)
					delete(clients, client)
				}
			}
			b.clients = make(map[string]map[*Client]bool)
			b.mu.Unlock()
			return
		}
	}
}
