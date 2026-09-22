package pushlet

import (
	"errors"
	"testing"
	"time"
)

func TestBrokerRegisterBeforeStart(t *testing.T) {
	b := NewBroker()
	err := b.Register(NewClient(), "t")
	if !errors.Is(err, errBrokerNotRunning) {
		t.Fatalf("got %v", err)
	}
}

func TestBrokerLocalPublish(t *testing.T) {
	b := NewBroker()
	b.Start()
	defer b.Stop()

	client := NewClient()
	if err := b.Register(client, "alerts"); err != nil {
		t.Fatal(err)
	}
	defer b.Unregister(client)

	if err := b.Publish("alerts", NewMessage("alerts", "ping", "1")); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-client.Send:
		if msg.Data != "1" {
			t.Fatalf("got %q", msg.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for local publish")
	}
}
