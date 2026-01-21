// Package main is the entry point for the CHROTE server
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chrote/server/internal/api"
	"github.com/chrote/server/internal/dashboard"
	"github.com/chrote/server/internal/proxy"
)

// Config holds server configuration
type Config struct {
	Port          int
	TtydPort      int
	APIAuthToken  string
	CORSOrigins   []string
	StartTtyd     bool
}

func main() {
	// Parse flags
	config := Config{}
	flag.IntVar(&config.Port, "port", 8080, "Server port")
	flag.IntVar(&config.TtydPort, "ttyd-port", 7681, "ttyd port")
	flag.StringVar(&config.APIAuthToken, "auth-token", "", "API authentication token")
	flag.BoolVar(&config.StartTtyd, "start-ttyd", true, "Start ttyd child process")
	flag.Parse()

	// Environment overrides
	if port := os.Getenv("PORT"); port != "" {
		fmt.Sscanf(port, "%d", &config.Port)
	}
	if port := os.Getenv("TTYD_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &config.TtydPort)
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

	healthHandler := api.NewHealthHandler()
	healthHandler.RegisterRoutes(mux)

	mailHandler := api.NewMailHandler()
	mailHandler.RegisterRoutes(mux)

	chatHandler := api.NewChatHandler()
	chatHandler.RegisterRoutes(mux)

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
		Addr:         fmt.Sprintf(":%d", config.Port),
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
		log.Printf("CHROTE server starting on port %d", config.Port)
		log.Printf("Dashboard: http://localhost:%d/", config.Port)
		log.Printf("API: http://localhost:%d/api/", config.Port)
		log.Printf("Chat: http://localhost:%d/api/chat/", config.Port)
		log.Printf("Files: http://localhost:%d/api/files/", config.Port)
		log.Printf("Terminal: http://localhost:%d/terminal/", config.Port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-done
	log.Println("Shutting down server...")

	// Stop ttyd
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

// corsMiddleware adds CORS headers
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if len(allowedOrigins) > 0 {
				// Production mode: check against allowlist
				for _, allowed := range allowedOrigins {
					if origin == allowed {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						break
					}
				}
			} else {
				// Development mode: allow all
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}

			w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization, X-Nuke-Confirm")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")

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
