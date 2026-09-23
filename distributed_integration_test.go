//go:build integration

package pushlet

import (
	"database/sql"
	"testing"
	"time"

	"github.com/usual2970/novaque"
	"github.com/usual2970/novaque/driver/mysql"

	"github.com/usual2970/pushlet/internal/testmysql"
)

func openNovaqueClient(t *testing.T, db *sql.DB, opts DistributedOptions) *novaque.Client {
	t.Helper()
	client, err := novaque.Open(mysql.New(db), opts.Novaque)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestDistributedCrossInstance(t *testing.T) {
	db := testmysql.Open(t)
	optsA := DefaultDistributedOptions()
	optsA.Novaque.PollInterval = 50 * time.Millisecond
	optsA.Channel = "test-a"
	optsB := optsA
	optsB.Channel = "test-b"

	bA := NewBroker()
	if err := bA.EnableDistributedNovaque(openNovaqueClient(t, db, optsA), optsA); err != nil {
		t.Fatal(err)
	}
	bA.Start()
	defer bA.Stop()

	bB := NewBroker()
	if err := bB.EnableDistributedNovaque(openNovaqueClient(t, db, optsB), optsB); err != nil {
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
	optsA := DefaultDistributedOptions()
	optsA.Novaque.PollInterval = 50 * time.Millisecond
	optsA.Channel = "test-a"
	optsB := optsA
	optsB.Channel = "test-b"

	bA := NewBroker()
	if err := bA.EnableDistributedNovaque(openNovaqueClient(t, db, optsA), optsA); err != nil {
		t.Fatal(err)
	}
	bA.Start()
	defer bA.Stop()

	bB := NewBroker()
	if err := bB.EnableDistributedNovaque(openNovaqueClient(t, db, optsB), optsB); err != nil {
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
