package pushlet

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
)

var errRedisAddrRequired = errors.New("pushlet: redis addr is required")

const defaultRedisMessageBuffer = 100

// RedisOptions configures Redis pub/sub distributed mode.
type RedisOptions struct {
	RelayOptions
	Addr     string
	Password string
	DB       int
}

// DefaultRedisOptions returns relay-friendly Redis defaults.
func DefaultRedisOptions() RedisOptions {
	return RedisOptions{
		RelayOptions: RelayOptions{RelayTopic: defaultRelayTopic},
	}
}

// RedisConnector implements [DistributedConnector] using Redis pub/sub.
type RedisConnector struct {
	addr        string
	password    string
	db          int
	client      *redis.Client
	pubsub      *redis.PubSub
	relayTopic  string
	messageChan chan *PublishMessage
	ctx         context.Context
	cancel      context.CancelFunc

	mu      sync.Mutex
	running bool
}

// NewRedisConnector builds a connector for Redis.
func NewRedisConnector(opts RedisOptions) (*RedisConnector, error) {
	relayTopic := opts.RelayTopic
	if relayTopic == "" {
		relayTopic = defaultRelayTopic
	}
	if opts.Addr == "" {
		return nil, errRedisAddrRequired
	}
	return &RedisConnector{
		addr:        opts.Addr,
		password:    opts.Password,
		db:          opts.DB,
		relayTopic:  relayTopic,
		messageChan: make(chan *PublishMessage, defaultRedisMessageBuffer),
	}, nil
}

// Start connects to Redis and subscribes to the relay channel.
func (rc *RedisConnector) Start() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.running {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	client := redis.NewClient(&redis.Options{
		Addr:     rc.addr,
		Password: rc.password,
		DB:       rc.db,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		cancel()
		_ = client.Close()
		return err
	}

	pubsub := client.Subscribe(ctx, rc.relayTopic)

	rc.ctx = ctx
	rc.cancel = cancel
	rc.client = client
	rc.pubsub = pubsub
	rc.running = true

	go rc.receiveMessages()
	return nil
}

// Stop closes the Redis subscription and client.
func (rc *RedisConnector) Stop() {
	rc.mu.Lock()
	if !rc.running {
		rc.mu.Unlock()
		return
	}
	rc.running = false
	cancel := rc.cancel
	pubsub := rc.pubsub
	client := rc.client
	rc.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if pubsub != nil {
		_ = pubsub.Close()
	}
	if client != nil {
		_ = client.Close()
	}
}

// PublishToTopic publishes a topic-scoped message to the relay channel.
func (rc *RedisConnector) PublishToTopic(topic string, msg *Message) error {
	return rc.publish(topic, false, msg)
}

// PublishToAll publishes a global fan-out message to the relay channel.
func (rc *RedisConnector) PublishToAll(msg *Message) error {
	return rc.publish("", true, msg)
}

func (rc *RedisConnector) publish(topic string, all bool, msg *Message) error {
	rc.mu.Lock()
	running := rc.running
	client := rc.client
	ctx := rc.ctx
	rc.mu.Unlock()
	if !running || client == nil {
		return errConnectorNotRunning
	}
	body, err := encodeRelayEnvelope(topic, all, msg)
	if err != nil {
		return err
	}
	return client.Publish(ctx, rc.relayTopic, body).Err()
}

// Messages returns decoded relay messages from Redis.
func (rc *RedisConnector) Messages() <-chan *PublishMessage {
	return rc.messageChan
}

func (rc *RedisConnector) receiveMessages() {
	rc.mu.Lock()
	pubsub := rc.pubsub
	ctx := rc.ctx
	rc.mu.Unlock()
	if pubsub == nil {
		return
	}

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			pm, err := decodeRelayEnvelope([]byte(msg.Payload))
			if err != nil {
				log.Printf("pushlet: drop invalid relay envelope on redis %s: %v", rc.relayTopic, err)
				continue
			}
			select {
			case rc.messageChan <- pm:
			case <-ctx.Done():
				return
			default:
				log.Printf("pushlet: drop relay message: redis message channel full on %s", rc.relayTopic)
			}
		}
	}
}
