package pushlet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"sync"
	"time"

	"github.com/usual2970/novaque"
)

// RelayOptions configures the shared relay channel/topic name for distributed mode.
type RelayOptions struct {
	// RelayTopic is the relay channel (Redis) or novaque topic (default pushlet-relay).
	RelayTopic string
}

// DistributedOptions configures novaque-backed distributed mode.
type DistributedOptions struct {
	RelayOptions
	Novaque novaque.Options
	// Channel is the novaque channel on RelayTopic. Each instance needs a distinct
	// channel for multicast. If empty, a unique name is generated per process.
	Channel string
	// RelayPublishTTL is per-message TTL for relay publishes.
	RelayPublishTTL time.Duration
}

// EnableDistributedNovaque opens distributed mode with an embedded novaque client.
func (b *Broker) EnableDistributedNovaque(client *novaque.Client, opts DistributedOptions) error {
	if client == nil {
		return errDistributedNoConnector
	}
	connector, err := NewNovaqueConnector(client, opts)
	if err != nil {
		return err
	}
	return b.EnableDistributedMode(connector)
}

// DefaultDistributedOptions returns relay-friendly defaults. Channel may be left
// empty to auto-generate a unique channel per instance.
func DefaultDistributedOptions() DistributedOptions {
	return DistributedOptions{
		RelayOptions:    RelayOptions{RelayTopic: defaultRelayTopic},
		RelayPublishTTL: 5 * time.Minute,
		Novaque: novaque.Options{
			DefaultTTL: time.Minute,
		},
	}
}

// NovaqueConnector implements DistributedConnector using embedded novaque.
type NovaqueConnector struct {
	client      *novaque.Client
	consumer    *novaque.Consumer
	relayTopic  string
	channelName string
	publishTTL  time.Duration

	messageChan chan *PublishMessage
	ctx         context.Context
	cancel      context.CancelFunc

	mu      sync.Mutex
	running bool
}

// NewNovaqueConnector builds a connector for an opened novaque client.
func NewNovaqueConnector(client *novaque.Client, opts DistributedOptions) (*NovaqueConnector, error) {
	relayTopic := opts.RelayTopic
	if relayTopic == "" {
		relayTopic = defaultRelayTopic
	}
	ttl := opts.RelayPublishTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &NovaqueConnector{
		client:      client,
		relayTopic:  relayTopic,
		channelName: resolveDistributedChannel(opts.Channel),
		publishTTL:  ttl,
		messageChan: make(chan *PublishMessage, 100),
	}, nil
}

func resolveDistributedChannel(channel string) string {
	if channel != "" {
		return channel
	}
	return "pushlet-node-" + newInstanceID()
}

func newInstanceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "pushlet"
	}
	return host + "-" + hex.EncodeToString(b[:])
}

// Start migrates schema, starts the client, and subscribes to the relay topic.
func (nc *NovaqueConnector) Start() error {
	nc.mu.Lock()
	if nc.running {
		nc.mu.Unlock()
		return nil
	}
	nc.ctx, nc.cancel = context.WithCancel(context.Background())
	nc.mu.Unlock()

	ctx := nc.ctx
	if err := nc.client.Migrate(ctx); err != nil {
		return err
	}
	if err := nc.client.Start(ctx); err != nil {
		return err
	}

	cons, err := nc.client.SubscribeAndStart(ctx, nc.relayTopic, nc.channelName, func(_ context.Context, msg *novaque.Message) error {
		pm, err := decodeRelayEnvelope(msg.Body)
		if err != nil {
			log.Printf("pushlet: drop invalid relay envelope on %s/%s: %v", nc.relayTopic, nc.channelName, err)
			return nil
		}
		select {
		case nc.messageChan <- pm:
		case <-ctx.Done():
			return ctx.Err()
		default:
			log.Printf("pushlet: drop relay message: novaque message channel full on %s/%s", nc.relayTopic, nc.channelName)
		}
		return nil
	})
	if err != nil {
		_ = nc.client.Shutdown(context.Background())
		return err
	}

	nc.mu.Lock()
	nc.consumer = cons
	nc.running = true
	nc.mu.Unlock()
	return nil
}

// Stop shuts down the consumer and novaque client.
func (nc *NovaqueConnector) Stop() {
	nc.mu.Lock()
	if !nc.running {
		nc.mu.Unlock()
		return
	}
	nc.running = false
	cons := nc.consumer
	cancel := nc.cancel
	nc.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cons != nil {
		_ = cons.Shutdown(context.Background())
	}
	_ = nc.client.Shutdown(context.Background())
}

// PublishToTopic publishes a topic-scoped message to the relay.
func (nc *NovaqueConnector) PublishToTopic(topic string, msg *Message) error {
	return nc.publish(topic, false, msg)
}

// PublishToAll publishes a global fan-out message to the relay.
func (nc *NovaqueConnector) PublishToAll(msg *Message) error {
	return nc.publish("", true, msg)
}

func (nc *NovaqueConnector) publish(topic string, all bool, msg *Message) error {
	nc.mu.Lock()
	running := nc.running
	nc.mu.Unlock()
	if !running {
		return errConnectorNotRunning
	}
	body, err := encodeRelayEnvelope(topic, all, msg)
	if err != nil {
		return err
	}
	_, err = nc.client.Publish(nc.ctx, nc.relayTopic, body, novaque.PublishOpts{TTL: nc.publishTTL})
	return err
}

// Messages returns the channel of decoded relay messages.
func (nc *NovaqueConnector) Messages() <-chan *PublishMessage {
	return nc.messageChan
}
