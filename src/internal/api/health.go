// Package api provides HTTP handlers for the API
package api

import (
	"net/http"
	"time"

	"github.com/chrote/server/internal/core"
)

// HealthHandler handles health check endpoints
type HealthHandler struct{}

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// RegisterRoutes registers the health routes on the given mux
func (h *HealthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", h.Health)
}

// Health handles GET /api/health
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
