package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockTtydServer creates a mock ttyd server that accepts WebSocket connections.
// Any non-WebSocket request it receives is recorded: the relay must never
// forward one, because ttyd's own page is no longer part of the product.
func mockTtydServer(pageRequests *atomic.Int32) *httptest.Server {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws" || strings.HasPrefix(r.URL.Path, "/ws?") {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				http.Error(w, "upgrade failed", http.StatusInternalServerError)
				return
			}
			defer conn.Close()

			// Echo back any message received
			for {
				messageType, p, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if err := conn.WriteMessage(messageType, p); err != nil {
					return
				}
			}
		} else {
			pageRequests.Add(1)
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>ttyd mock</html>"))
		}
	}))
}

func TestTerminalProxyStopBeforeStart(t *testing.T) {
	proxy := NewTerminalProxy(0)
	if err := proxy.Stop(); err != nil {
		t.Fatalf("Stop before Start returned error: %v", err)
	}
}

func TestTerminalProxy_WebSocketUpgrade(t *testing.T) {
	// Start mock ttyd server
	var ttydPageRequests atomic.Int32
	mockTtyd := mockTtydServer(&ttydPageRequests)
	defer mockTtyd.Close()

	// Extract port from mock server URL
	mockURL := mockTtyd.URL
	var port int
	if _, err := fmt.Sscanf(mockURL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("Failed to parse mock server URL: %v", err)
	}

	// Create terminal proxy pointing to mock server
	proxy := NewTerminalProxy(port)

	// Create test server with the proxy handler
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate /terminal/ prefix stripping
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/terminal")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		proxy.Handler().ServeHTTP(w, r)
	}))
	defer proxyServer.Close()

	// The dashboard renders the terminal itself, so ttyd's page must not be
	// reachable through the relay (ADR-0018).
	t.Run("plain HTTP is not served", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		for _, path := range []string{"/terminal/", "/terminal/token", "/terminal/xterm.css"} {
			resp, err := client.Get(proxyServer.URL + path)
			if err != nil {
				t.Fatalf("HTTP request to %s failed: %v", path, err)
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("Expected status 404 for %s, got %d", path, resp.StatusCode)
			}
		}

		if got := ttydPageRequests.Load(); got != 0 {
			t.Errorf("Expected the relay to forward no page requests to ttyd, got %d", got)
		}
	})

	// Test WebSocket upgrade works
	t.Run("WebSocket upgrade", func(t *testing.T) {
		wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http") + "/terminal/ws"

		dialer := websocket.Dialer{
			HandshakeTimeout: 5 * time.Second,
		}

		conn, resp, err := dialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("WebSocket dial failed: %v (response: %+v)", err, resp)
		}
		defer conn.Close()

		// Test echo
		testMessage := []byte("hello ttyd")
		if err := conn.WriteMessage(websocket.TextMessage, testMessage); err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}

		_, received, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read message: %v", err)
		}

		if string(received) != string(testMessage) {
			t.Errorf("Expected echo %q, got %q", testMessage, received)
		}
	})
}
