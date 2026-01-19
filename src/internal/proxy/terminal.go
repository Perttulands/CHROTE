// Package proxy provides reverse proxy functionality for ttyd
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
)

// TerminalProxy manages ttyd process and proxies requests
type TerminalProxy struct {
	ttydPort    int
	ttydCmd     *exec.Cmd
	proxy       *httputil.ReverseProxy
	mu          sync.Mutex
	running     bool
	launchScript string
}

// NewTerminalProxy creates a new TerminalProxy
func NewTerminalProxy(ttydPort int) *TerminalProxy {
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
		log.Printf("Terminal proxy error: %v", err)
		http.Error(w, "Terminal not available", http.StatusBadGateway)
	}

	return &TerminalProxy{
		ttydPort:     ttydPort,
		proxy:        proxy,
		launchScript: core.GetLaunchScript(),
	}
}

// Start starts the ttyd process
func (tp *TerminalProxy) Start() error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if tp.running {
		return nil
	}

	// Build ttyd command
	// ttyd -p PORT -W -a terminal-launch.sh
	tp.ttydCmd = exec.Command(
		"ttyd",
		"-p", fmt.Sprintf("%d", tp.ttydPort),
		"-W", // WebSocket only mode (for better performance)
		"-a", // Allow URL arguments (?arg=sessionName -> $1)
		tp.launchScript,
	)

	// Set environment with TMUX_TMPDIR
	tp.ttydCmd.Env = core.GetTmuxEnv()

	// Pipe stdout/stderr for debugging
	tp.ttydCmd.Stdout = os.Stdout
	tp.ttydCmd.Stderr = os.Stderr

	if err := tp.ttydCmd.Start(); err != nil {
		return fmt.Errorf("failed to start ttyd: %w", err)
	}

	tp.running = true
	log.Printf("Started ttyd on port %d", tp.ttydPort)

	// Monitor the process
	go func() {
		err := tp.ttydCmd.Wait()
		tp.mu.Lock()
		tp.running = false
		tp.mu.Unlock()
		if err != nil {
			log.Printf("ttyd exited with error: %v", err)
		} else {
			log.Printf("ttyd exited normally")
		}
	}()

	// Wait a moment for ttyd to start
	time.Sleep(500 * time.Millisecond)

	return nil
}

// Stop stops the ttyd process
func (tp *TerminalProxy) Stop() error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if !tp.running || tp.ttydCmd == nil || tp.ttydCmd.Process == nil {
		return nil
	}

	log.Printf("Stopping ttyd...")

	// Try graceful shutdown first
	if err := tp.ttydCmd.Process.Signal(os.Interrupt); err != nil {
		// Fall back to kill
		tp.ttydCmd.Process.Kill()
	}

	// Wait for process to exit with timeout
	done := make(chan error, 1)
	go func() {
		done <- tp.ttydCmd.Wait()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case <-done:
		// Process exited
	case <-ctx.Done():
		// Timeout - force kill
		tp.ttydCmd.Process.Kill()
	}

	tp.running = false
	log.Printf("ttyd stopped")
	return nil
}

// IsRunning returns whether ttyd is running
func (tp *TerminalProxy) IsRunning() bool {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.running
}

// Handler returns an http.Handler that proxies to ttyd
func (tp *TerminalProxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip /terminal prefix before proxying
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/terminal")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}

		tp.proxy.ServeHTTP(w, r)
	})
}

// RegisterRoutes registers the terminal proxy route
func (tp *TerminalProxy) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/terminal/", tp.Handler())
}
