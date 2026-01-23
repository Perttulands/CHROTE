// Package proxy provides reverse proxy functionality for bv (beads viewer) terminal
package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
	"github.com/gorilla/websocket"
)

// BvTerminalProxy manages a dedicated ttyd process for beads_viewer
type BvTerminalProxy struct {
	ttydPort     int
	ttydCmd      *exec.Cmd
	proxy        *httputil.ReverseProxy
	mu           sync.Mutex
	running      bool
	launchScript string
	currentPath  string // Current project path
}

// NewBvTerminalProxy creates a new BvTerminalProxy on the specified port
func NewBvTerminalProxy(ttydPort int) *BvTerminalProxy {
	target, _ := url.Parse(fmt.Sprintf("http://localhost:%d", ttydPort))

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the director to handle WebSocket upgrade
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Preserve WebSocket headers
		if strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
			req.Header.Set("Connection", "Upgrade")
		}
	}

	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("BV terminal proxy error: %v", err)
		http.Error(w, "BV terminal not available", http.StatusBadGateway)
	}

	return &BvTerminalProxy{
		ttydPort:     ttydPort,
		proxy:        proxy,
		launchScript: core.GetBvLaunchScript(),
	}
}

// Start starts the ttyd process for bv with the specified project path
func (bp *BvTerminalProxy) Start(projectPath string) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// If already running with same path, do nothing
	if bp.running && bp.currentPath == projectPath {
		return nil
	}

	// If running with different path, stop first
	if bp.running {
		bp.stopLocked()
	}

	// Validate project path if provided
	if projectPath == "" {
		projectPath = "/code"
	}

	// Kill any existing process on our port
	killCmd := exec.Command("fuser", "-k", fmt.Sprintf("%d/tcp", bp.ttydPort))
	killCmd.Run()
	time.Sleep(100 * time.Millisecond)

	// Build ttyd command with project path as argument
	// ttyd -p PORT -W -a bv-launch.sh projectPath
	bp.ttydCmd = exec.Command(
		"ttyd",
		"-p", fmt.Sprintf("%d", bp.ttydPort),
		"-W", // WebSocket only mode
		"-a", // Allow URL arguments
		bp.launchScript,
		projectPath,
	)

	// Set environment
	bp.ttydCmd.Env = append(os.Environ(), "LANG=en_US.UTF-8")

	// Pipe stdout/stderr for debugging
	bp.ttydCmd.Stdout = os.Stdout
	bp.ttydCmd.Stderr = os.Stderr

	if err := bp.ttydCmd.Start(); err != nil {
		return fmt.Errorf("failed to start bv ttyd: %w", err)
	}

	bp.running = true
	bp.currentPath = projectPath
	log.Printf("Started bv ttyd on port %d for path: %s", bp.ttydPort, projectPath)

	// Monitor the process
	go func() {
		err := bp.ttydCmd.Wait()
		bp.mu.Lock()
		bp.running = false
		bp.currentPath = ""
		bp.mu.Unlock()
		if err != nil {
			log.Printf("bv ttyd exited with error: %v", err)
		} else {
			log.Printf("bv ttyd exited normally")
		}
	}()

	// Wait for ttyd to start
	time.Sleep(500 * time.Millisecond)

	return nil
}

// stopLocked stops the ttyd process (must be called with lock held)
func (bp *BvTerminalProxy) stopLocked() {
	if !bp.running || bp.ttydCmd == nil || bp.ttydCmd.Process == nil {
		return
	}

	log.Printf("Stopping bv ttyd...")

	// Try graceful shutdown first
	if err := bp.ttydCmd.Process.Signal(os.Interrupt); err != nil {
		bp.ttydCmd.Process.Kill()
	}

	// Wait for process to exit with timeout
	done := make(chan error, 1)
	go func() {
		done <- bp.ttydCmd.Wait()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	select {
	case <-done:
		// Process exited
	case <-ctx.Done():
		// Timeout - force kill
		bp.ttydCmd.Process.Kill()
	}

	bp.running = false
	bp.currentPath = ""
	log.Printf("bv ttyd stopped")
}

// Stop stops the ttyd process
func (bp *BvTerminalProxy) Stop() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.stopLocked()
	return nil
}

// Restart restarts the ttyd process with a new project path
func (bp *BvTerminalProxy) Restart(projectPath string) error {
	return bp.Start(projectPath)
}

// IsRunning returns whether ttyd is running
func (bp *BvTerminalProxy) IsRunning() bool {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return bp.running
}

// GetCurrentPath returns the current project path
func (bp *BvTerminalProxy) GetCurrentPath() string {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return bp.currentPath
}

// Handler returns an http.Handler that proxies to bv ttyd
func (bp *BvTerminalProxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip /bv-terminal prefix before proxying
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/bv-terminal")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}

		// Check if this is a WebSocket upgrade request
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			bp.proxyWebSocket(w, r)
			return
		}

		bp.proxy.ServeHTTP(w, r)
	})
}

// proxyWebSocket handles WebSocket connections by proxying to ttyd
func (bp *BvTerminalProxy) proxyWebSocket(w http.ResponseWriter, r *http.Request) {
	// Connect to ttyd WebSocket
	ttydURL := fmt.Sprintf("ws://127.0.0.1:%d%s", bp.ttydPort, r.URL.RequestURI())

	// Get requested subprotocols from client
	clientSubprotocols := websocket.Subprotocols(r)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     clientSubprotocols,
	}

	// Forward relevant headers to ttyd
	requestHeader := http.Header{}
	if origin := r.Header.Get("Origin"); origin != "" {
		requestHeader.Set("Origin", origin)
	}

	backendConn, resp, err := dialer.Dial(ttydURL, requestHeader)
	if err != nil {
		log.Printf("Failed to connect to bv ttyd WebSocket: %v", err)
		if resp != nil {
			log.Printf("bv ttyd response status: %d", resp.StatusCode)
		}
		http.Error(w, "BV terminal not available", http.StatusBadGateway)
		return
	}
	defer backendConn.Close()

	// Create upgrader with the negotiated subprotocol from backend
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		Subprotocols: []string{backendConn.Subprotocol()},
	}

	// Upgrade the client connection
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade client WebSocket: %v", err)
		return
	}
	defer clientConn.Close()

	// Bidirectional message forwarding
	errChan := make(chan error, 2)

	// Client -> Backend
	go func() {
		for {
			messageType, message, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := backendConn.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Backend -> Client
	go func() {
		for {
			messageType, message, err := backendConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := clientConn.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Wait for either direction to close
	<-errChan
}

// RegisterRoutes registers the bv terminal proxy routes
func (bp *BvTerminalProxy) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/bv-terminal/", bp.Handler())

	// API endpoint to restart with new project path
	mux.HandleFunc("/api/bv-terminal/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		projectPath := r.URL.Query().Get("path")
		if projectPath == "" {
			projectPath = "/code"
		}

		// Validate path is within allowed roots
		resolved, errCode, errMsg := core.ValidateProjectPath(projectPath)
		if errCode != "" {
			core.WriteError(w, http.StatusBadRequest, errCode, errMsg)
			return
		}

		if err := bp.Restart(resolved); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "RESTART_FAILED", err.Error())
			return
		}

		core.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"path":    resolved,
		})
	})

	// API endpoint to get current status
	mux.HandleFunc("/api/bv-terminal/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		core.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"running": bp.IsRunning(),
			"path":    bp.GetCurrentPath(),
			"port":    bp.ttydPort,
		})
	})
}
