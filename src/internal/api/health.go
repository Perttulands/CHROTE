// Package api provides HTTP handlers for the API
package api

import (
	"net/http"
	"time"

	"github.com/chrote/server/internal/core"
)

// HealthHandler handles health check endpoints
type HealthHandler struct {
	version string
	commit  string
}

// NewHealthHandlerWithBuildInfo creates a HealthHandler carrying the git commit
// the binary was built from. Commit is empty when the build did not stamp one.
func NewHealthHandlerWithBuildInfo(version, commit string) *HealthHandler {
	if version == "" {
		return &HealthHandler{version: "2.0.0-alpha.2-dev", commit: commit}
	}
	return &HealthHandler{version: version, commit: commit}
}

// RegisterRoutes registers the health routes on the given mux
func (h *HealthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", h.Health)
	mux.HandleFunc("GET /api/version", h.Version)
}

// Health handles GET /api/health
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"version":   h.version,
		"commit":    h.commit,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// Version handles GET /api/version
func (h *HealthHandler) Version(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"version": h.version,
	})
}
