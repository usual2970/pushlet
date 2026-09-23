package pushlet

import (
	"errors"
	"testing"
	"time"
)

func TestEnableDistributedModeNilClient(t *testing.T) {
	b := NewBroker()
	err := b.EnableDistributedMode(nil, DefaultDistributedOptions())
	if !errors.Is(err, errDistributedNoClient) {
		t.Fatalf("got %v", err)
	}
}

func TestResolveDistributedChannel(t *testing.T) {
	if got := resolveDistributedChannel("my-pod"); got != "my-pod" {
		t.Fatalf("explicit: got %q", got)
	}
	a := resolveDistributedChannel("")
	b := resolveDistributedChannel("")
	if a == "" || b == "" || a == b {
		t.Fatalf("auto: got %q %q", a, b)
	}
}

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
