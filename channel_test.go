package pushlet

import (
	"os"
	"testing"
)

func TestResolveRelayChannelConfigured(t *testing.T) {
	if got := ResolveRelayChannel("  replica-a  "); got != "replica-a" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveRelayChannelPodName(t *testing.T) {
	t.Setenv("POD_NAME", "api-7f8b9c")
	t.Setenv("HOSTNAME", "ignored")
	if got := ResolveRelayChannel(""); got != "pushlet-api-7f8b9c" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveRelayChannelHostname(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("HOSTNAME", "")
	host, err := os.Hostname()
	if err != nil {
		t.Skip(err)
	}
	want := "pushlet-" + host
	if got := ResolveRelayChannel(""); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
