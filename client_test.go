package pushlet

import "testing"

func TestClientCloseSendIdempotent(t *testing.T) {
	c := NewClient()
	c.CloseSend()
	c.CloseSend()
	c.CloseSend()
}

func TestUnregisterAfterSendMessageDropDoesNotPanic(t *testing.T) {
	b := NewBroker()
	b.Start()
	defer b.Stop()

	c := NewClient()
	b.Register(c, "t")
	// Fill buffer then one more SendMessage triggers CloseSend in default branch
	for i := 0; i < cap(c.Send)+1; i++ {
		c.SendMessage(NewMessage("t", "e", "x"))
	}
	b.Unregister(c)
}
