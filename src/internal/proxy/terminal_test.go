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

// mockTtydServer creates a mock ttyd server that accepts WebSocket connections
func mockTtydServer() *httptest.Server {
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
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>ttyd mock</html>"))
		}
	}))
}

func TestTerminalProxy_WebSocketUpgrade(t *testing.T) {
	// Start mock ttyd server
	mockTtyd := mockTtydServer()
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

	// Test HTTP request works
	t.Run("HTTP request", func(t *testing.T) {
		resp, err := http.Get(proxyServer.URL + "/terminal/")
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
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
