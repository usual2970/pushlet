package pushlet

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublishJSONDeliversFrame(t *testing.T) {
	p := New()
	p.Start()
	defer p.Stop()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.HandleSSE(w, r)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get(srv.URL + "/events?topic=inv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	var body strings.Builder
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body.Write(buf[:n])
		}
		if strings.Contains(body.String(), "event: connected") {
			break
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	type payload struct {
		N int `json:"n"`
	}
	if err := p.PublishJSON("inv", "test.event", payload{N: 7}); err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body.Write(buf[:n])
		}
		s := body.String()
		if strings.Contains(s, "event: test.event") && strings.Contains(s, `"n":7`) {
			return
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("missing json frame: %q", body.String())
}
