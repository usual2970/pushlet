package pushlet

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestNewRedisConnectorEmptyAddr(t *testing.T) {
	_, err := NewRedisConnector(RedisOptions{})
	if !errors.Is(err, errRedisAddrRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestRedisConnectorCrossBroker(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	opts := DefaultRedisOptions()
	opts.Addr = mr.Addr()

	connA, err := NewRedisConnector(opts)
	if err != nil {
		t.Fatal(err)
	}
	connB, err := NewRedisConnector(opts)
	if err != nil {
		t.Fatal(err)
	}

	bA := NewBroker()
	if err := bA.EnableDistributedMode(connA); err != nil {
		t.Fatal(err)
	}
	bA.Start()
	defer bA.Stop()

	bB := NewBroker()
	if err := bB.EnableDistributedMode(connB); err != nil {
		t.Fatal(err)
	}
	bB.Start()
	defer bB.Stop()

	clientB := NewClient()
	if err := bB.Register(clientB, "alerts"); err != nil {
		t.Fatal(err)
	}
	defer bB.Unregister(clientB)

	msg := NewMessage("alerts", "ping", "from-a")
	if err := bA.Publish("alerts", msg); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-clientB.Send:
		if got.Data != "from-a" {
			t.Fatalf("got data %q", got.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for redis relay")
	}
}

func TestRedisConnectorPublishToAll(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	opts := DefaultRedisOptions()
	opts.Addr = mr.Addr()

	connA, err := NewRedisConnector(opts)
	if err != nil {
		t.Fatal(err)
	}
	connB, err := NewRedisConnector(opts)
	if err != nil {
		t.Fatal(err)
	}

	bA := NewBroker()
	if err := bA.EnableDistributedMode(connA); err != nil {
		t.Fatal(err)
	}
	bA.Start()
	defer bA.Stop()

	bB := NewBroker()
	if err := bB.EnableDistributedMode(connB); err != nil {
		t.Fatal(err)
	}
	bB.Start()
	defer bB.Stop()

	clientB := NewClient()
	if err := bB.Register(clientB, "any"); err != nil {
		t.Fatal(err)
	}
	defer bB.Unregister(clientB)

	if err := bA.PublishToAll(NewMessage("", "broadcast", "all")); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-clientB.Send:
		if got.Data != "all" {
			t.Fatalf("got data %q", got.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for publish-to-all")
	}
}

func TestRedisConnectorInvalidEnvelopeDropped(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	opts := DefaultRedisOptions()
	opts.Addr = mr.Addr()

	conn, err := NewRedisConnector(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Start(); err != nil {
		t.Fatal(err)
	}
	defer conn.Stop()

	_ = mr.Publish(opts.RelayTopic, "not-json")

	select {
	case <-conn.Messages():
		t.Fatal("expected invalid envelope to be dropped")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestRedisConnectorPublishBeforeStart(t *testing.T) {
	conn, err := NewRedisConnector(RedisOptions{Addr: "127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	err = conn.PublishToTopic("t", NewMessage("t", "e", "d"))
	if !errors.Is(err, errConnectorNotRunning) {
		t.Fatalf("got %v", err)
	}
}
