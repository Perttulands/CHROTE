// Package api provides HTTP handlers for the API
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

// TmuxHandler handles tmux-related API endpoints
type TmuxHandler struct {
	cache      *sessionsCache
	colorRegex *regexp.Regexp
}

type sessionsCache struct {
	mu        sync.RWMutex
	data      *SessionsResponse
	timestamp time.Time
	ttl       time.Duration
}

// SessionsResponse is the response for listing sessions
type SessionsResponse struct {
	Sessions  []core.Session            `json:"sessions"`
	Grouped   map[string][]core.Session `json:"grouped"`
	Timestamp string                    `json:"timestamp"`
	Error     string                    `json:"error,omitempty"`
}

// CreateSessionRequest is the request body for creating a session
type CreateSessionRequest struct {
	Name string `json:"name"`
}

// RenameSessionRequest is the request body for renaming a session
type RenameSessionRequest struct {
	NewName string `json:"newName"`
}

// AppearanceRequest is the request body for tmux appearance settings
type AppearanceRequest struct {
	StatusBg           string `json:"statusBg"`
	StatusFg           string `json:"statusFg"`
	PaneBorderActive   string `json:"paneBorderActive"`
	PaneBorderInactive string `json:"paneBorderInactive"`
	ModeStyleBg        string `json:"modeStyleBg"`
	ModeStyleFg        string `json:"modeStyleFg"`
}

// NewTmuxHandler creates a new TmuxHandler
func NewTmuxHandler() *TmuxHandler {
	return &TmuxHandler{
		cache: &sessionsCache{
			ttl: time.Second,
		},
		colorRegex: regexp.MustCompile(`^#[0-9A-Fa-f]{3,6}$|^[a-zA-Z]+$|^default$`),
	}
}

// RegisterRoutes registers the tmux routes on the given mux
func (h *TmuxHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tmux/sessions", h.ListSessions)
	mux.HandleFunc("POST /api/tmux/sessions", h.CreateSession)
	mux.HandleFunc("DELETE /api/tmux/sessions/all", h.DeleteAllSessions)
	mux.HandleFunc("DELETE /api/tmux/sessions/{name}", h.DeleteSession)
	mux.HandleFunc("PATCH /api/tmux/sessions/{name}", h.RenameSession)
	mux.HandleFunc("POST /api/tmux/appearance", h.ApplyAppearance)
}

// runTmux executes a tmux command with proper environment
func (h *TmuxHandler) runTmux(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.Env = core.GetTmuxEnv()

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %s", err.Error(), string(exitErr.Stderr))
		}
		return "", err
	}
	return string(output), nil
}

// ListSessions handles GET /api/tmux/sessions
func (h *TmuxHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	// Check cache
	h.cache.mu.RLock()
	if h.cache.data != nil && time.Since(h.cache.timestamp) < h.cache.ttl {
		data := h.cache.data
		h.cache.mu.RUnlock()
		core.WriteJSON(w, http.StatusOK, data)
		return
	}
	h.cache.mu.RUnlock()

	// Fetch sessions
	output, err := h.runTmux("list-sessions", "-F", "#{session_name}:#{session_windows}:#{session_attached}")

	response := &SessionsResponse{
		Sessions:  []core.Session{},
		Grouped:   make(map[string][]core.Session),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err != nil {
		// Check for "no server running" type errors
		errStr := err.Error()
		noServerErrors := []string{"no server running", "No such file or directory", "error connecting"}
		isNoServer := false
		for _, msg := range noServerErrors {
			if strings.Contains(errStr, msg) {
				isNoServer = true
				break
			}
		}
		if !isNoServer {
			response.Error = errStr
		}
	} else {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				windows, _ := strconv.Atoi(parts[1])
				if windows == 0 {
					windows = 1
				}
				session := core.Session{
					Name:     parts[0],
					Windows:  windows,
					Attached: parts[2] == "1",
					Group:    core.CategorizeSession(parts[0]),
				}
				response.Sessions = append(response.Sessions, session)
			}
		}

		core.SortSessions(response.Sessions)
		response.Grouped = core.GroupSessions(response.Sessions)
	}

	// Update cache
	h.cache.mu.Lock()
	h.cache.data = response
	h.cache.timestamp = time.Now()
	h.cache.mu.Unlock()

	core.WriteJSON(w, http.StatusOK, response)
}

// invalidateCache clears the sessions cache
func (h *TmuxHandler) invalidateCache() {
	h.cache.mu.Lock()
	h.cache.data = nil
	h.cache.timestamp = time.Time{}
	h.cache.mu.Unlock()
}

// CreateSession handles POST /api/tmux/sessions
func (h *TmuxHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && r.ContentLength > 0 {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	name := req.Name
	if name == "" {
		// Generate a name if not provided
		timestamp := strconv.FormatInt(time.Now().UnixMilli(), 36)
		name = "shell-" + timestamp
	} else {
		// Validate user-provided session name
		valid, errMsg := core.ValidateSessionName(name, "session name")
		if !valid {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
			return
		}
	}

	// Create the session (detached)
	_, err := h.runTmux("new-session", "-d", "-s", name, "-c", core.GetWorkDir())
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "TMUX_ERROR", err.Error())
		return
	}

	h.invalidateCache()

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"session":   name,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// DeleteSession handles DELETE /api/tmux/sessions/{name}
func (h *TmuxHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionName := r.PathValue("name")

	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}

	_, err := h.runTmux("kill-session", "-t", sessionName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}

	h.invalidateCache()

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"killed":    sessionName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// DeleteAllSessions handles DELETE /api/tmux/sessions/all
func (h *TmuxHandler) DeleteAllSessions(w http.ResponseWriter, r *http.Request) {
	// Verify the request came from the dashboard UI
	confirmHeader := r.Header.Get("X-Nuke-Confirm")
	if confirmHeader != "DASHBOARD-NUKE-CONFIRMED" {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Nuke operation requires dashboard confirmation. Use the UI.")
		return
	}

	// Get list of all sessions first
	output, err := h.runTmux("list-sessions", "-F", "#{session_name}")
	var sessionNames []string
	if err == nil {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				sessionNames = append(sessionNames, line)
			}
		}
	}

	if len(sessionNames) == 0 {
		core.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":   true,
			"killed":    0,
			"message":   "No sessions to kill",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	// Kill the tmux server
	_, err = h.runTmux("kill-server")
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}

	h.invalidateCache()

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"killed":    len(sessionNames),
		"sessions":  sessionNames,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// RenameSession handles PATCH /api/tmux/sessions/{name}
func (h *TmuxHandler) RenameSession(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")

	valid, errMsg := core.ValidateSessionName(oldName, "current session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}

	var req RenameSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	valid, errMsg = core.ValidateSessionName(req.NewName, "new session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}

	_, err := h.runTmux("rename-session", "-t", oldName, req.NewName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}

	h.invalidateCache()

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"oldName":   oldName,
		"newName":   req.NewName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ApplyAppearance handles POST /api/tmux/appearance
func (h *TmuxHandler) ApplyAppearance(w http.ResponseWriter, r *http.Request) {
	var req AppearanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	// Validate colors
	colors := map[string]string{
		"statusBg":           req.StatusBg,
		"statusFg":           req.StatusFg,
		"paneBorderActive":   req.PaneBorderActive,
		"paneBorderInactive": req.PaneBorderInactive,
		"modeStyleBg":        req.ModeStyleBg,
		"modeStyleFg":        req.ModeStyleFg,
	}

	for key, val := range colors {
		if val != "" && !h.colorRegex.MatchString(val) {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("Invalid color for %s: %s", key, val))
			return
		}
	}

	// Build tmux set commands
	var commands [][]string
	if req.StatusBg != "" && req.StatusFg != "" {
		commands = append(commands, []string{"set", "-g", "status-style", fmt.Sprintf("bg=%s,fg=%s", req.StatusBg, req.StatusFg)})
	}
	if req.PaneBorderActive != "" {
		commands = append(commands, []string{"set", "-g", "pane-active-border-style", fmt.Sprintf("fg=%s", req.PaneBorderActive)})
	}
	if req.PaneBorderInactive != "" {
		commands = append(commands, []string{"set", "-g", "pane-border-style", fmt.Sprintf("fg=%s", req.PaneBorderInactive)})
	}
	if req.ModeStyleBg != "" && req.ModeStyleFg != "" {
		commands = append(commands, []string{"set", "-g", "mode-style", fmt.Sprintf("bg=%s,fg=%s", req.ModeStyleBg, req.ModeStyleFg)})
	}

	applied := 0
	for _, args := range commands {
		_, err := h.runTmux(args...)
		if err == nil {
			applied++
		}
		// Ignore errors for appearance - tmux server might not be running
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"applied":   applied,
		"total":     len(commands),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
