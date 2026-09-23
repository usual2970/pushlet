package pushlet

import (
	"os"
	"strings"
)

// ResolveRelayChannel picks the novaque channel for pushlet-relay on this
// instance. A configured value wins; otherwise POD_NAME, HOSTNAME, or
// os.Hostname() with a pushlet- prefix so restarts reuse the same channel
// when pod/host identity is stable. Empty returns "" and pushlet
// auto-generates a unique channel per process.
func ResolveRelayChannel(configured string) string {
	if s := strings.TrimSpace(configured); s != "" {
		return s
	}
	for _, env := range []string{"POD_NAME", "HOSTNAME"} {
		if s := strings.TrimSpace(os.Getenv(env)); s != "" {
			return "pushlet-" + s
		}
	}
	if host, err := os.Hostname(); err == nil {
		if s := strings.TrimSpace(host); s != "" {
			return "pushlet-" + s
		}
	}
	return ""
}
