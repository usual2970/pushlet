package pushlet

import (
	"sync"

	"github.com/usual2970/novaque"
)

// Broker manages topic subscriptions and in-process message delivery.
// When distributed mode is enabled, cross-node messages arrive via [DistributedConnector].
type Broker struct {
	// 按主题组织的客户端映射 - 一个topic可以有多个client
	topicClients map[string]map[*Client]bool

	// 按客户端组织的主题映射 - 一个client可以订阅多个topic
	clientTopics map[*Client]map[string]bool

	// 注册客户端的通道
	register chan *ClientRegistration

	// 注销客户端的通道
	unregister chan *ClientUnregistration

	// 订阅主题的通道
	subscribe chan *SubscriptionRequest

	// 取消订阅主题的通道
	unsubscribe chan *UnsubscriptionRequest

	// 发布消息的通道
	publish chan *PublishMessage

	// connector carries cross-node pub/sub when distributed mode is enabled.
	connector DistributedConnector

	// 确保线程安全
	mu sync.RWMutex

	// 停止信号
	stop chan struct{}

	// 是否已启动
	running bool

	// 是否启用分布式模式
	distributedMode bool

	relayStop      chan struct{}
	stopRelayOnce  sync.Once
	startRelayOnce sync.Once
}

// ClientRegistration registers a client and its initial topic subscription.
type ClientRegistration struct {
	Client *Client
	Topic  string
}

// ClientUnregistration removes a client and all of its topic subscriptions.
type ClientUnregistration struct {
	Client *Client
}

// SubscriptionRequest adds a topic subscription for an existing client.
type SubscriptionRequest struct {
	Client *Client
	Topic  string
}

// UnsubscriptionRequest removes a topic subscription for a client.
type UnsubscriptionRequest struct {
	Client *Client
	Topic  string
}

// PublishMessage is an internal publish command consumed by the broker loop.
type PublishMessage struct {
	Topic   string
	Message *Message
	All     bool
}

// NewBroker returns a broker that must be started with [Broker.Start].
func NewBroker() *Broker {
	return &Broker{
		topicClients:    make(map[string]map[*Client]bool),
		clientTopics:    make(map[*Client]map[string]bool),
		register:        make(chan *ClientRegistration),
		unregister:      make(chan *ClientUnregistration),
		subscribe:       make(chan *SubscriptionRequest),
		unsubscribe:     make(chan *UnsubscriptionRequest),
		publish:         make(chan *PublishMessage),
		stop:            make(chan struct{}),
		relayStop:       make(chan struct{}),
		distributedMode: false,
	}
}

// EnableDistributedMode enables cross-node delivery via an opened novaque client.
// Open the client with any novaque driver (MySQL, PostgreSQL, or SQLite) on a
// shared database before calling this method.
func (b *Broker) EnableDistributedMode(client *novaque.Client, opts DistributedOptions) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.distributedMode {
		return errDistributedAlreadyOn
	}
	if client == nil {
		return errDistributedNoClient
	}
	connector, err := NewNovaqueConnector(client, opts)
	if err != nil {
		return err
	}
	if err := connector.Start(); err != nil {
		return err
	}

	b.connector = connector
	b.distributedMode = true

	if b.running {
		b.startDistributedRelay()
	}

	return nil
}

// Start runs the broker event loop until [Broker.Stop] is called.
func (b *Broker) Start() {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	b.running = true
	b.mu.Unlock()

	go b.run()

	b.startDistributedRelay()
}

// Stop stops the broker loop and shuts down the distributed connector when present.
func (b *Broker) Stop() {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return
	}
	b.running = false
	b.mu.Unlock()

	b.stopRelayOnce.Do(func() { close(b.relayStop) })

	if b.distributedMode && b.connector != nil {
		b.connector.Stop()
	}

	b.stop <- struct{}{}
}

func (b *Broker) startDistributedRelay() {
	if !b.distributedMode || b.connector == nil {
		return
	}
	b.startRelayOnce.Do(func() {
		go b.processDistributedMessages()
	})
}

// Register adds a client and subscribes it to topic when topic is non-empty.
// Returns an error if [Broker.Start] has not been called.
func (b *Broker) Register(client *Client, topic string) error {
	b.mu.RLock()
	running := b.running
	b.mu.RUnlock()
	if !running {
		return errBrokerNotRunning
	}
	b.register <- &ClientRegistration{
		Client: client,
		Topic:  topic,
	}
	return nil
}

// Unregister removes the client from all topics and closes its send channel.
func (b *Broker) Unregister(client *Client) {
	b.unregister <- &ClientUnregistration{
		Client: client,
	}
}

// Subscribe attaches client to topic asynchronously.
func (b *Broker) Subscribe(client *Client, topic string) {
	b.subscribe <- &SubscriptionRequest{
		Client: client,
		Topic:  topic,
	}
}

// Unsubscribe removes client from topic asynchronously.
func (b *Broker) Unsubscribe(client *Client, topic string) {
	b.unsubscribe <- &UnsubscriptionRequest{
		Client: client,
		Topic:  topic,
	}
}

// GetClientTopics returns a snapshot of topics subscribed by client.
func (b *Broker) GetClientTopics(client *Client) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	topics := make([]string, 0, len(b.clientTopics[client]))
	if topicMap, ok := b.clientTopics[client]; ok {
		for topic := range topicMap {
			topics = append(topics, topic)
		}
	}
	return topics
}

// GetTopicClients returns a snapshot of clients subscribed to topic.
func (b *Broker) GetTopicClients(topic string) []*Client {
	b.mu.RLock()
	defer b.mu.RUnlock()

	clients := make([]*Client, 0, len(b.topicClients[topic]))
	if clientMap, ok := b.topicClients[topic]; ok {
		for client := range clientMap {
			clients = append(clients, client)
		}
	}
	return clients
}

// Publish delivers msg to local subscribers of topic, or relays when distributed mode is on.
func (b *Broker) Publish(topic string, msg *Message) error {
	if b.distributedMode && b.connector != nil {
		return b.connector.PublishToTopic(topic, msg)
	}
	b.publish <- &PublishMessage{
		Topic:   topic,
		Message: msg,
		All:     false,
	}
	return nil
}

// PublishToAll delivers msg to all local subscribers, or relays a global fan-out when distributed.
func (b *Broker) PublishToAll(msg *Message) error {
	if b.distributedMode && b.connector != nil {
		return b.connector.PublishToAll(msg)
	}
	b.publish <- &PublishMessage{
		Message: msg,
		All:     true,
	}
	return nil
}

func (b *Broker) processDistributedMessages() {
	if !b.distributedMode || b.connector == nil {
		return
	}

	for {
		select {
		case <-b.relayStop:
			return
		case msg, ok := <-b.connector.Messages():
			if !ok {
				return
			}
			select {
			case b.publish <- msg:
			case <-b.relayStop:
				return
			}
		}
	}
}

// run 运行消息代理的主循环
func (b *Broker) run() {
	for {
		select {
		case reg := <-b.register:
			b.registerClient(reg.Client, reg.Topic)

		case unreg := <-b.unregister:
			b.unregisterClient(unreg.Client)

		case sub := <-b.subscribe:
			b.subscribeClientToTopic(sub.Client, sub.Topic)

		case unsub := <-b.unsubscribe:
			b.unsubscribeClientFromTopic(unsub.Client, unsub.Topic)

		case pm := <-b.publish:
			b.publishMessage(pm)

		case <-b.stop:
			b.cleanup()
			return
		}
	}
}

// registerClient 注册客户端并订阅初始主题
func (b *Broker) registerClient(client *Client, topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 初始化客户端的主题映射
	if _, ok := b.clientTopics[client]; !ok {
		b.clientTopics[client] = make(map[string]bool)
	}
	if topic == "" {
		return
	}

	// 订阅初始主题
	b.subscribeClientToTopicUnsafe(client, topic)
}

// unregisterClient 注销客户端并取消所有订阅
func (b *Broker) unregisterClient(client *Client) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 从所有订阅的主题中移除客户端
	if topics, ok := b.clientTopics[client]; ok {
		for topic := range topics {
			b.unsubscribeClientFromTopicUnsafe(client, topic)
		}
		delete(b.clientTopics, client)
	}

	client.CloseSend()
}

// subscribeClientToTopic 订阅客户端到主题
func (b *Broker) subscribeClientToTopic(client *Client, topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscribeClientToTopicUnsafe(client, topic)
}

// subscribeClientToTopicUnsafe 订阅客户端到主题（不加锁版本）
func (b *Broker) subscribeClientToTopicUnsafe(client *Client, topic string) {
	// 将客户端添加到主题的客户端列表中
	if _, ok := b.topicClients[topic]; !ok {
		b.topicClients[topic] = make(map[*Client]bool)
	}
	b.topicClients[topic][client] = true

	// 将主题添加到客户端的主题列表中
	if _, ok := b.clientTopics[client]; !ok {
		b.clientTopics[client] = make(map[string]bool)
	}
	b.clientTopics[client][topic] = true

}

// unsubscribeClientFromTopic 取消客户端对主题的订阅
func (b *Broker) unsubscribeClientFromTopic(client *Client, topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.unsubscribeClientFromTopicUnsafe(client, topic)
}

// unsubscribeClientFromTopicUnsafe 取消客户端对主题的订阅（不加锁版本）
func (b *Broker) unsubscribeClientFromTopicUnsafe(client *Client, topic string) {
	// 从主题的客户端列表中移除客户端
	if clients, ok := b.topicClients[topic]; ok {
		delete(clients, client)
		// 如果该主题下没有客户端了，删除主题
		if len(clients) == 0 {
			delete(b.topicClients, topic)
		}
	}

	// 从客户端的主题列表中移除主题
	if topics, ok := b.clientTopics[client]; ok {
		delete(topics, topic)
	}
}

// publishMessage 发布消息
func (b *Broker) publishMessage(pm *PublishMessage) {
	b.mu.RLock()
	var dropped []*Client

	if pm.All {
		for _, clients := range b.topicClients {
			for client := range clients {
				if !client.SendMessage(pm.Message) {
					dropped = append(dropped, client)
				}
			}
		}
	} else if clients, ok := b.topicClients[pm.Topic]; ok {
		for client := range clients {
			if !client.SendMessage(pm.Message) {
				dropped = append(dropped, client)
			}
		}
	}
	b.mu.RUnlock()

	seen := make(map[*Client]struct{}, len(dropped))
	for _, client := range dropped {
		if _, dup := seen[client]; dup {
			continue
		}
		seen[client] = struct{}{}
		go b.Unregister(client)
	}
}

// cleanup 清理所有连接
func (b *Broker) cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for client := range b.clientTopics {
		client.CloseSend()
	}

	// 清空所有映射
	b.topicClients = make(map[string]map[*Client]bool)
	b.clientTopics = make(map[*Client]map[string]bool)
}
