// Package main is the entry point for the CHROTE server
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chrote/server/internal/api"
	"github.com/chrote/server/internal/dashboard"
	"github.com/chrote/server/internal/proxy"
)

// Version is set at build time or defaults to dev
var Version = "2.0.0-alpha.2-dev"

// BuildCommit is stamped at build time via
// -ldflags "-X main.BuildCommit=$(git rev-parse HEAD)"; empty when unstamped.
var BuildCommit = ""

const (
	defaultBindHost   = "127.0.0.1"
	defaultServerPort = 8094
)

// Config holds server configuration
type Config struct {
	Host               string
	Port               int
	CORSOrigins        []string
	StartSystemHistory bool
}

func main() {
	// Parse flags
	config := Config{StartSystemHistory: true}
	flag.StringVar(&config.Host, "host", defaultBindHost, "Bind address")
	flag.IntVar(&config.Port, "port", defaultServerPort, "Server port")
	flag.BoolVar(&config.StartSystemHistory, "start-system-history", true, "Start system history sampler")
	flag.Parse()

	// Environment overrides
	if host := os.Getenv("HOST"); host != "" {
		config.Host = host
	}
	if port := os.Getenv("PORT"); port != "" {
		config.Port = mustParsePort("PORT", port)
	}
	warnRemovedAccessTokenSetting()
	if err := api.ValidateTerminalUserEnv(); err != nil {
		log.Fatalf("invalid terminal user configuration: %v", err)
	}
	if origins := os.Getenv("CORS_ORIGINS"); origins != "" {
		config.CORSOrigins = strings.Split(origins, ",")
		for i := range config.CORSOrigins {
			config.CORSOrigins[i] = strings.TrimSpace(config.CORSOrigins[i])
		}
	}

	// Create main mux
	mux := http.NewServeMux()
	runtimeCtx, stopRuntime := context.WithCancel(context.Background())

	scheduledTasks, stopRuntimeMaintenance := registerRuntimeRoutes(mux, config, runtimeCtx)
	registerAPIFallback(mux)

	// Serve embedded dashboard at root
	dashboardHandler := dashboard.Handler()
	mux.Handle("/", dashboardHandler)

	// Wrap with middleware
	handler := corsMiddleware(config.CORSOrigins)(mux)
	handler = recoveryMiddleware(handler)
	handler = loggingMiddleware(handler)

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if err := scheduledTasks.StartScheduler(); err != nil {
		log.Printf("Warning: failed to start scheduled tasks: %v", err)
	}

	// Graceful shutdown handling
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("CHROTE v%s starting on port %d", Version, config.Port)
		log.Printf("Dashboard: http://localhost:%d/", config.Port)
		log.Printf("API: http://localhost:%d/api/", config.Port)
		log.Printf("Files: http://localhost:%d/api/files/", config.Port)
		log.Printf("Terminal: http://localhost:%d/terminal/", config.Port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-done
	log.Println("Shutting down server...")

	// Stop subsystems
	stopRuntime()
	stopRuntimeMaintenance()
	scheduledTasks.StopScheduler()

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server stopped")
}

func registerRuntimeRoutes(mux *http.ServeMux, config Config, ctx context.Context) (*api.ScheduledHandler, context.CancelFunc) {
	tmuxHandler := api.NewTmuxHandler()
	tmuxHandler.RegisterRoutes(mux)

	scheduledHandler := api.NewScheduledHandler(tmuxHandler)
	scheduledHandler.RegisterRoutes(mux)

	beadsHandler := api.NewBeadsHandler()
	beadsHandler.RegisterRoutes(mux)

	filesHandler := api.NewFilesHandler()
	filesHandler.RegisterRoutes(mux)

	healthHandler := api.NewHealthHandlerWithBuildInfo(Version, BuildCommit)
	healthHandler.RegisterRoutes(mux)

	themeHandler := api.NewThemeHandler()
	themeHandler.RegisterRoutes(mux)

	servicesHandler := api.NewServicesHandler(api.LoadServiceConfigFromEnv())
	servicesHandler.RegisterRoutes(mux)

	systemHandler := api.NewSystemHandler()
	var stopSystemHistory context.CancelFunc = func() {}
	if config.StartSystemHistory {
		stopSystemHistory = startDefaultSystemHistorySampler(systemHandler, ctx)
	}
	systemHandler.RegisterRoutes(mux)

	// CHROTE owns the terminal transport itself (ADR-0018): the attach runs on
	// a pty this process allocates, resolved by the one implementation of the
	// socket rules that also serves session listing. It gets the same
	// CORS_ORIGINS list as the middleware below, because CORS response headers
	// do not constrain a WebSocket handshake and this socket is shell-grade.
	terminalProxy := proxy.NewTerminalProxy(resolveTerminalTarget, config.CORSOrigins)
	terminalProxy.RegisterRoutes(mux)
	var stopOnce sync.Once
	stopRuntimeMaintenance := func() {
		stopOnce.Do(func() {
			stopSystemHistory()
		})
	}
	return scheduledHandler, stopRuntimeMaintenance
}

// resolveTerminalTarget adapts the API package's resolution to the transport's
// view of it, so neither package has to know the other's types.
func resolveTerminalTarget(unixUser string) (proxy.Target, error) {
	target, err := api.ResolveTerminalTarget(unixUser)
	if err != nil {
		return proxy.Target{}, err
	}
	return proxy.Target{Socket: target.Socket, WorkDir: target.WorkDir, UnixUser: target.UnixUser}, nil
}

var startDefaultSystemHistorySampler = (*api.SystemHandler).StartDefaultHistorySampler

func warnRemovedAccessTokenSetting() {
	if strings.TrimSpace(os.Getenv("API_AUTH_TOKEN")) == "" {
		return
	}
	log.Print("Warning: API_AUTH_TOKEN is no longer supported and does not protect CHROTE; restrict access with localhost and private-network controls")
}

// mustParsePort parses a port string and fatals on invalid values.
func mustParsePort(name, raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		log.Fatalf("invalid %s=%q: %v", name, raw, err)
	}
	if n < 1 || n > 65535 {
		log.Fatalf("invalid %s=%d: must be 1-65535", name, n)
	}
	return n
}

func registerAPIFallback(mux *http.ServeMux) {
	mux.HandleFunc("/api", apiNotFound)
	mux.HandleFunc("/api/", apiNotFound)
}

func apiNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"success":false,"error":{"code":"NOT_FOUND","message":"API route not found"}}`))
}

// corsMiddleware adds CORS headers
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" && len(allowed) > 0 {
				if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, X-Nuke-Confirm")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Add("Vary", "Origin")
				}
			}

			// Handle preflight
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("handler panic recovered: %T\n%s", recovered, debug.Stack())
				if wrapped, ok := w.(*responseWriter); ok && wrapped.wroteHeader {
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				if _, err := w.Write([]byte(`{"success":false,"error":{"code":"INTERNAL_ERROR","message":"Internal server error"}}`)); err != nil {
					log.Printf("failed to write panic response: %v", err)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create response wrapper to capture status
		wrapped := &responseWriter{ResponseWriter: w, status: 200}

		next.ServeHTTP(wrapped, r)

		// Only log API requests and errors
		if strings.HasPrefix(r.URL.Path, "/api/") || wrapped.status >= 400 {
			log.Printf("%s %s %d %v", r.Method, r.URL.Path, wrapped.status, time.Since(start))
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) Write(p []byte) (int, error) {
	if !rw.wroteHeader {
		rw.status = http.StatusOK
		rw.wroteHeader = true
	}
	return rw.ResponseWriter.Write(p)
}

func (rw *responseWriter) WriteHeader(status int) {
	if rw.wroteHeader {
		return
	}
	rw.status = status
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(status)
}

// Hijack implements http.Hijacker interface to support WebSocket upgrades
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		conn, rwbuf, err := hijacker.Hijack()
		if err == nil {
			rw.status = http.StatusSwitchingProtocols
			rw.wroteHeader = true
		}
		return conn, rwbuf, err
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}

// Flush implements http.Flusher interface
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *responseWriter) SetWriteDeadline(deadline time.Time) error {
	return http.NewResponseController(rw.ResponseWriter).SetWriteDeadline(deadline)
}

func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
