package pushlet

import "encoding/json"

const defaultRelayTopic = "pushlet-relay"

// DistributedConnector carries cross-node publish and subscribe for the broker.
type DistributedConnector interface {
	Start() error
	Stop()
	PublishToTopic(topic string, msg *Message) error
	PublishToAll(msg *Message) error
	Messages() <-chan *PublishMessage
}

type relayEnvelope struct {
	Topic   string   `json:"topic"`
	All     bool     `json:"all"`
	Message *Message `json:"message"`
}

func encodeRelayEnvelope(topic string, all bool, msg *Message) ([]byte, error) {
	env := relayEnvelope{
		Topic:   topic,
		All:     all,
		Message: msg,
	}
	return json.Marshal(env)
}

func decodeRelayEnvelope(data []byte) (*PublishMessage, error) {
	var env relayEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Message == nil {
		return nil, errInvalidRelayEnvelope
	}
	return &PublishMessage{
		Topic:   env.Topic,
		Message: env.Message,
		All:     env.All,
	}, nil
}
