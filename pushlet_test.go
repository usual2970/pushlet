package pushlet

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialTestWebsocket connects to a test server exposing HandleWebsocket at /ws.
func dialTestWebsocket(t *testing.T, srvURL string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srvURL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	return conn
}

// readTestWsCommand reads the next binary frame expected to be a command
// response, failing the test if the server does not answer in time.
func readTestWsCommand(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	msgType, bts, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read command response: %v", err)
	}
	if msgType != websocket.BinaryMessage {
		t.Fatalf("expected binary response, got type %d", msgType)
	}
	return bts
}

// TestWebsocketBareSubUnsubKeepsConnectionAlive is a regression test for a
// crash where "SUB" or "UNSUB" without a topic panicked the per-connection
// read goroutine and took down the whole process.
func TestWebsocketBareSubUnsubKeepsConnectionAlive(t *testing.T) {
	p := New()
	p.Start()
	defer p.Stop()

	srv := httptest.NewServer(http.HandlerFunc(p.HandleWebsocket))
	defer srv.Close()

	conn := dialTestWebsocket(t, srv.URL)
	defer conn.Close()

	// Consume the initial "connected" frame.
	readTestWsCommand(t, conn)

	// Malformed commands: no topic argument. These must be ignored, not
	// crash the server; the test process surviving past them is the point.
	for _, frame := range []string{"SUB\n", "UNSUB\n"} {
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte(frame)); err != nil {
			t.Fatalf("write %q: %v", frame, err)
		}
	}

	// The connection must still serve valid commands afterwards.
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("SUB room\n")); err != nil {
		t.Fatalf("write SUB room: %v", err)
	}
	if resp := readTestWsCommand(t, conn); string(resp) != "OK" {
		t.Fatalf("SUB room after malformed frames: got %q, want %q", resp, "OK")
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("PING")); err != nil {
		t.Fatalf("write PING: %v", err)
	}
	if resp := readTestWsCommand(t, conn); string(resp) != "OK" {
		t.Fatalf("PING after malformed frames: got %q, want %q", resp, "OK")
	}
}

// TestWebsocketSubWithTopicSucceeds is the negative control: a fully formed
// SUB command still gets an OK response.
func TestWebsocketSubWithTopicSucceeds(t *testing.T) {
	p := New()
	p.Start()
	defer p.Stop()

	srv := httptest.NewServer(http.HandlerFunc(p.HandleWebsocket))
	defer srv.Close()

	conn := dialTestWebsocket(t, srv.URL)
	defer conn.Close()

	// Consume the initial "connected" frame.
	readTestWsCommand(t, conn)

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("SUB room\n")); err != nil {
		t.Fatalf("write SUB room: %v", err)
	}
	if resp := readTestWsCommand(t, conn); string(resp) != "OK" {
		t.Fatalf("SUB room response: got %q, want %q", resp, "OK")
	}
}
