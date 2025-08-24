package pushlet

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
)

// RedisConnector 管理与Redis的连接和发布/订阅操作
type RedisConnector struct {
	client      *redis.Client
	pubsub      *redis.PubSub
	messageChan chan *PublishMessage
	ctx         context.Context
	cancel      context.CancelFunc
	running     bool
}

// NewRedisConnector 创建新的Redis连接器
func NewRedisConnector(redisAddr string, redisPassword string, redisDB int) *RedisConnector {
	ctx, cancel := context.WithCancel(context.Background())

	return &RedisConnector{
		client: redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
		}),
		ctx:         ctx,
		cancel:      cancel,
		messageChan: make(chan *PublishMessage, 100),
	}
}

// Start 启动Redis连接器并订阅频道
func (rc *RedisConnector) Start() error {
	if rc.running {
		return nil
	}

	// 测试连接
	_, err := rc.client.Ping(rc.ctx).Result()
	if err != nil {
		return err
	}

	// 订阅全局频道
	rc.pubsub = rc.client.Subscribe(rc.ctx, "pushlet:global")

	// 启动消息处理
	go rc.receiveMessages()

	rc.running = true
	return nil
}

// Stop 停止Redis连接器
func (rc *RedisConnector) Stop() {
	if !rc.running {
		return
	}

	rc.cancel()
	if rc.pubsub != nil {
		rc.pubsub.Close()
	}
	rc.client.Close()
	rc.running = false
}

// SubscribeTopic 订阅特定主题
func (rc *RedisConnector) SubscribeTopic(topic string) error {
	err := rc.pubsub.Subscribe(rc.ctx, "pushlet:topic:"+topic)
	return err
}

// UnsubscribeTopic 取消订阅特定主题
func (rc *RedisConnector) UnsubscribeTopic(topic string) error {
	return rc.pubsub.Unsubscribe(rc.ctx, "pushlet:topic:"+topic)
}

// PublishToTopic 发布消息到特定主题
func (rc *RedisConnector) PublishToTopic(topic string, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return rc.client.Publish(rc.ctx, "pushlet:topic:"+topic, data).Err()
}

// PublishToAll 发布消息到所有主题
func (rc *RedisConnector) PublishToAll(msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return rc.client.Publish(rc.ctx, "pushlet:global", data).Err()
}

// GetMessageChannel 获取接收消息的通道
func (rc *RedisConnector) GetMessageChannel() <-chan *PublishMessage {
	return rc.messageChan
}

// receiveMessages 处理从Redis接收的消息
func (rc *RedisConnector) receiveMessages() {
	ch := rc.pubsub.Channel()

	for {
		select {
		case <-rc.ctx.Done():
			return

		case msg, ok := <-ch:
			if !ok {
				return
			}

			// 解析消息
			var message Message
			err := json.Unmarshal([]byte(msg.Payload), &message)
			if err != nil {
				log.Printf("Failed to unmarshal message: %v", err)
				continue
			}

			// 确定主题
			topic := "default"
			channel := msg.Channel
			if channel != "pushlet:global" {
				// 从频道名称中提取主题
				topic = channel[14:] // 删除 "pushlet:topic:" 前缀
			}

			// 创建发布消息
			publishMsg := &PublishMessage{
				Topic:   topic,
				Message: &message,
				All:     channel == "pushlet:global",
			}

			// 发送到消息通道
			rc.messageChan <- publishMsg
		}
	}
}
