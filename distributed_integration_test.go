//go:build integration

package pushlet

import (
	"testing"
	"time"

	"github.com/usual2970/pushlet/internal/testmysql"
)

func TestDistributedCrossInstance(t *testing.T) {
	db := testmysql.Open(t)
	opts := DefaultDistributedOptions()
	opts.Novaque.PollInterval = 50 * time.Millisecond

	bA := NewBroker()
	if err := bA.EnableDistributedMode(db, opts); err != nil {
		t.Fatal(err)
	}
	bA.Start()
	defer bA.Stop()

	bB := NewBroker()
	if err := bB.EnableDistributedMode(db, opts); err != nil {
		t.Fatal(err)
	}
	bB.Start()
	defer bB.Stop()

	clientB := NewClient()
	if err := bB.Register(clientB, "alerts"); err != nil {
		t.Fatal(err)
	}
	defer bB.Unregister(clientB)

	time.Sleep(200 * time.Millisecond)

	if err := bA.Publish("alerts", NewMessage("alerts", "message", "cross")); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-clientB.Send:
		if msg.Data != "cross" {
			t.Fatalf("got %q", msg.Data)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for cross-node delivery")
	}
}

func TestDistributedPublishToAll(t *testing.T) {
	db := testmysql.Open(t)
	opts := DefaultDistributedOptions()
	opts.Novaque.PollInterval = 50 * time.Millisecond

	bA := NewBroker()
	if err := bA.EnableDistributedMode(db, opts); err != nil {
		t.Fatal(err)
	}
	bA.Start()
	defer bA.Stop()

	bB := NewBroker()
	if err := bB.EnableDistributedMode(db, opts); err != nil {
		t.Fatal(err)
	}
	bB.Start()
	defer bB.Stop()

	clientB := NewClient()
	if err := bB.Register(clientB, "other"); err != nil {
		t.Fatal(err)
	}
	defer bB.Unregister(clientB)

	time.Sleep(200 * time.Millisecond)

	if err := bA.PublishToAll(NewMessage("global", "broadcast", "all")); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-clientB.Send:
		if msg.Data != "all" {
			t.Fatalf("got %q", msg.Data)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}
