package pushlet

import (
	"testing"
	"time"
)

func TestClientCloseSendIdempotent(t *testing.T) {
	c := NewClient()
	c.CloseSend()
	c.CloseSend()
}

func TestSendMessageAfterCloseIsNoOp(t *testing.T) {
	c := NewClient()
	c.CloseSend()
	if c.SendMessage(NewMessage("t", "e", "x")) {
		t.Fatal("expected false after close")
	}
}

func TestUnregisterAfterSendMessageDropDoesNotPanic(t *testing.T) {
	b := NewBroker()
	b.Start()
	defer b.Stop()

	c := NewClient()
	if err := b.Register(c, "t"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)

	for i := 0; i < cap(c.Send)+1; i++ {
		c.SendMessage(NewMessage("t", "e", "x"))
	}
	b.Unregister(c)
}

func TestBrokerPublishAfterSlowClientDropped(t *testing.T) {
	b := NewBroker()
	b.Start()
	defer b.Stop()

	c := NewClient()
	if err := b.Register(c, "alerts"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)

	for i := 0; i < cap(c.Send)+1; i++ {
		c.SendMessage(NewMessage("alerts", "e", "fill"))
	}

	// Would panic on send-to-closed without sendClosed guard + must not panic on fan-out
	_ = b.Publish("alerts", NewMessage("alerts", "e", "after-drop"))
	_ = b.PublishToAll(NewMessage("global", "e", "broadcast"))
	time.Sleep(50 * time.Millisecond)
}
