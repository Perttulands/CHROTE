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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chrote/server/internal/api"
	"github.com/chrote/server/internal/dashboard"
	"github.com/chrote/server/internal/proxy"
)

// Version is set at build time or defaults to dev
var Version = "0.2.0"

// Config holds server configuration
type Config struct {
	Host         string
	Port         int
	TtydPort     int
	APIAuthToken string
	CORSOrigins  []string
	StartTtyd    bool
}

func main() {
	// Parse flags
	config := Config{}
	flag.StringVar(&config.Host, "host", "", "Bind address (default all interfaces)")
	flag.IntVar(&config.Port, "port", 8080, "Server port")
	flag.IntVar(&config.TtydPort, "ttyd-port", 7681, "ttyd port")
	flag.StringVar(&config.APIAuthToken, "auth-token", "", "API authentication token")
	flag.BoolVar(&config.StartTtyd, "start-ttyd", true, "Start ttyd child process")
	flag.Parse()

	// Environment overrides
	if host := os.Getenv("HOST"); host != "" {
		config.Host = host
	}
	if port := os.Getenv("PORT"); port != "" {
		config.Port = mustParsePort("PORT", port)
	}
	if port := os.Getenv("TTYD_PORT"); port != "" {
		config.TtydPort = mustParsePort("TTYD_PORT", port)
	}
	if token := os.Getenv("API_AUTH_TOKEN"); token != "" {
		config.APIAuthToken = token
	}
	if origins := os.Getenv("CORS_ORIGINS"); origins != "" {
		config.CORSOrigins = strings.Split(origins, ",")
		for i := range config.CORSOrigins {
			config.CORSOrigins[i] = strings.TrimSpace(config.CORSOrigins[i])
		}
	}

	// Create main mux
	mux := http.NewServeMux()

	// Register API handlers
	tmuxHandler := api.NewTmuxHandler()
	tmuxHandler.RegisterRoutes(mux)

	beadsHandler := api.NewBeadsHandler()
	beadsHandler.RegisterRoutes(mux)

	filesHandler := api.NewFilesHandler()
	filesHandler.RegisterRoutes(mux)

	healthHandler := api.NewHealthHandlerWithVersion(Version)
	healthHandler.RegisterRoutes(mux)

	servicesHandler := api.NewServicesHandler(api.LoadServiceConfigFromEnv())
	servicesHandler.RegisterRoutes(mux)

	oracleHandler := api.NewOracleHandler(tmuxHandler, beadsHandler)
	oracleHandler.RegisterRoutes(mux)

	// Create terminal proxy
	terminalProxy := proxy.NewTerminalProxy(config.TtydPort)
	terminalProxy.RegisterRoutes(mux)

	// Serve embedded dashboard at root
	dashboardHandler := dashboard.Handler()
	mux.Handle("/", dashboardHandler)

	// Wrap with middleware
	handler := corsMiddleware(config.CORSOrigins)(mux)
	handler = authMiddleware(config.APIAuthToken)(handler)
	handler = loggingMiddleware(handler)

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start ttyd if configured
	if config.StartTtyd {
		if err := terminalProxy.Start(); err != nil {
			log.Printf("Warning: failed to start ttyd: %v", err)
			log.Printf("Terminal functionality will not be available")
		}
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
	if config.StartTtyd {
		terminalProxy.Stop()
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server stopped")
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
					w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization, X-Nuke-Confirm")
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

// authMiddleware adds optional bearer token authentication
func authMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if no token configured
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for health check
			if r.URL.Path == "/api/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for non-API routes
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			// Browser CORS preflights do not carry Authorization; let CORS answer them.
			if r.Method == http.MethodOptions && r.Header.Get("Origin") != "" && r.Header.Get("Access-Control-Request-Method") != "" {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"success":false,"error":{"code":"UNAUTHORIZED","message":"Authorization required"}}`, http.StatusUnauthorized)
				return
			}

			providedToken := strings.TrimPrefix(authHeader, "Bearer ")
			if providedToken != token {
				http.Error(w, `{"success":false,"error":{"code":"FORBIDDEN","message":"Invalid token"}}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
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
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

// Hijack implements http.Hijacker interface to support WebSocket upgrades
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
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
