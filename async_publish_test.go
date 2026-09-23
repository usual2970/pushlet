package pushlet

import (
	"testing"
	"time"
)

func TestPublishNeverBlocksOnHungBroker(t *testing.T) {
	p := New()
	p.EnableAsyncPublish(AsyncPublishOptions{QueueSize: publishQueueSizeTest, StopDrain: 100 * time.Millisecond})
	// Broker not started: worker blocks on first broker.Publish to unbuffered publish chan.
	p.Publish("t", "e", "d")
	time.Sleep(20 * time.Millisecond)

	for i := range publishQueueSizeTest + 64 {
		start := time.Now()
		if err := p.Publish("t", "e", "d"); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
		if d := time.Since(start); d > 50*time.Millisecond {
			t.Fatalf("publish %d blocked %v", i, d)
		}
	}
	if got := p.DroppedEvents(); got == 0 {
		t.Fatal("expected drops when queue full")
	}
	p.Stop()
}

func TestAsyncDisabledPublishSync(t *testing.T) {
	p := New()
	p.Start()
	defer p.Stop()
	if err := p.Publish("t", "ping", "pong"); err != nil {
		t.Fatal(err)
	}
}

const publishQueueSizeTest = 512
