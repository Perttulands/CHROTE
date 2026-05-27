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

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{version: "0.2.0", commit: "unknown"}
}

// NewHealthHandlerWithVersion creates a new HealthHandler with a custom version
func NewHealthHandlerWithVersion(version string) *HealthHandler {
	return &HealthHandler{version: version, commit: "unknown"}
}

// NewHealthHandlerWithBuild creates a HealthHandler with version and the source
// commit the binary was built from, so the running binary's provenance is
// observable over HTTP (the automatic Go VCS stamp is unreliable for the git
// worktree this is built from).
func NewHealthHandlerWithBuild(version, commit string) *HealthHandler {
	if commit == "" {
		commit = "unknown"
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
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// Version handles GET /api/version
func (h *HealthHandler) Version(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"version": h.version,
		"commit":  h.commit,
	})
}
