package pushlet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSSEGuardCancelsOnInvalid(t *testing.T) {
	p := New()
	p.Start()
	defer p.Stop()

	var ok atomic.Bool
	ok.Store(true)
	guard := SSEGuard{
		RecheckInterval: 50 * time.Millisecond,
		WriteTimeout:    500 * time.Millisecond,
		IsValid: func(context.Context) bool {
			return ok.Load()
		},
	}

	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		p.HandleSSEGuarded(w, r, guard)
	}))
	defer srv.Close()

	client := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	resp, err := client.Get(srv.URL + "/events?topic=u1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "event: connected") {
		t.Fatalf("no connected frame: %q", string(buf[:n]))
	}

	ok.Store(false)
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after access revoke")
	}
}

func TestSSEGuardNilChecker503(t *testing.T) {
	p := New()
	rec := httptest.NewRecorder()
	p.HandleSSEGuarded(rec, httptest.NewRequest("GET", "/events", nil), SSEGuard{})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
}
