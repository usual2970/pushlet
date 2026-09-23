package pushlet

import "encoding/json"

// PublishJSON marshals v as JSON and publishes to topic. When async publish is
// enabled, marshal failures are ignored (no enqueue); otherwise the error is returned.
func (p *Pushlet) PublishJSON(topic, event string, v any) error {
	if p == nil {
		return nil
	}
	body, err := json.Marshal(v)
	if err != nil {
		if p.asyncEnabled() {
			return nil
		}
		return err
	}
	return p.enqueuePublish(topic, event, string(body), false)
}
