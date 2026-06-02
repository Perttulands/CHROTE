package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTerminalProxy_WebSocketCurrentlyAllowsCrossOriginBrowserOrigin(t *testing.T) {
	mockTtyd := mockTtydServer()
	defer mockTtyd.Close()

	var port int
	if _, err := fmt.Sscanf(mockTtyd.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse mock ttyd URL %q: %v", mockTtyd.URL, err)
	}

	proxy := NewTerminalProxy(port)
	proxyServer := httptest.NewServer(proxy.Handler())
	defer proxyServer.Close()

	wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http") + "/terminal/ws"
	headers := http.Header{}
	headers.Set("Origin", "https://untrusted.example")
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}

	conn, resp, err := dialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("WebSocket dial with cross-origin Origin failed: %v (response: %+v)", err, resp)
	}
	defer conn.Close()

	message := []byte("origin posture baseline")
	if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
		t.Fatalf("write websocket message: %v", err)
	}
	_, got, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket echo: %v", err)
	}
	if string(got) != string(message) {
		t.Fatalf("echo = %q, want %q", got, message)
	}
}

func TestTerminalProxy_WebSocketOriginAllowlistFutureSpec(t *testing.T) {
	t.Skip("Known gap: enable in home-idhj.13 after browser-compatible terminal WebSocket Origin allowlist is implemented")
}
