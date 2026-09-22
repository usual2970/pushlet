package pushlet

import (
	"testing"
	"time"
)

func TestBrokerLocalPublish(t *testing.T) {
	b := NewBroker()
	b.Start()
	defer b.Stop()

	client := NewClient()
	b.Register(client, "alerts")
	defer b.Unregister(client)

	b.Publish("alerts", NewMessage("alerts", "ping", "1"))

	select {
	case msg := <-client.Send:
		if msg.Data != "1" {
			t.Fatalf("got %q", msg.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for local publish")
	}
}
