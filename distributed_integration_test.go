//go:build integration

package pushlet

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PUSHLET_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("PUSHLET_TEST_MYSQL_DSN not set")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDistributedCrossInstance(t *testing.T) {
	db := openTestDB(t)
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
	bB.Register(clientB, "alerts")
	defer bB.Unregister(clientB)

	time.Sleep(200 * time.Millisecond)

	bA.Publish("alerts", NewMessage("alerts", "message", "cross"))

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
	db := openTestDB(t)
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
	bB.Register(clientB, "other")
	defer bB.Unregister(clientB)

	time.Sleep(200 * time.Millisecond)

	bA.PublishToAll(NewMessage("global", "broadcast", "all"))

	select {
	case msg := <-clientB.Send:
		if msg.Data != "all" {
			t.Fatalf("got %q", msg.Data)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for broadcast")
	}

}
