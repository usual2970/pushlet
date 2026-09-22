package pushlet

import (
	"testing"
)

func TestRelayEnvelopeRoundTrip(t *testing.T) {
	msg := NewMessage("alerts", "message", "hello")
	data, err := encodeRelayEnvelope("alerts", false, msg)
	if err != nil {
		t.Fatal(err)
	}
	pm, err := decodeRelayEnvelope(data)
	if err != nil {
		t.Fatal(err)
	}
	if pm.Topic != "alerts" || pm.All {
		t.Fatalf("unexpected publish message: %+v", pm)
	}
	if pm.Message.Event != "message" || pm.Message.Data != "hello" {
		t.Fatalf("unexpected inner message: %+v", pm.Message)
	}
}

func TestRelayEnvelopeGlobal(t *testing.T) {
	msg := NewMessage("global", "broadcast", "all")
	data, err := encodeRelayEnvelope("", true, msg)
	if err != nil {
		t.Fatal(err)
	}
	pm, err := decodeRelayEnvelope(data)
	if err != nil {
		t.Fatal(err)
	}
	if !pm.All {
		t.Fatal("expected All=true")
	}
}

func TestRelayEnvelopeInvalidJSON(t *testing.T) {
	if _, err := decodeRelayEnvelope([]byte("{")); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestRelayEnvelopeMissingMessage(t *testing.T) {
	if _, err := decodeRelayEnvelope([]byte(`{"topic":"x","all":false}`)); err == nil {
		t.Fatal("expected error for missing message")
	}
}
