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
	// socket, when non-empty, scopes every tmux invocation to an explicit
	// socket path via `tmux -S <socket> ...`. This is how the formations side
	// panel targets the EXACT same socket the formations executor dispatches to
	// (CHROTE_FORMATIONS_TMUX_SOCKET). When empty, the cockpit handler relies on
	// TMUX_TMPDIR via core.GetTmuxEnv() and injects no -S flag.
	socket string
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

// MouseModeRequest is the request body for toggling tmux mouse mode
type MouseModeRequest struct {
	Enabled bool `json:"enabled"`
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

// NewTmuxHandler creates a new TmuxHandler bound to the cockpit socket
// (TMUX_TMPDIR-driven via core.GetTmuxEnv). Behavior is unchanged.
func NewTmuxHandler() *TmuxHandler {
	return newTmuxHandler("")
}

// NewTmuxHandlerForSocket creates a TmuxHandler whose every tmux invocation is
// scoped to the explicit socket path via `tmux -S <socket> ...`. Use this for
// the formations side panel so it lists/attaches the SAME sessions the
// formations executor dispatches to. The socket must be the exact path from
// CHROTE_FORMATIONS_TMUX_SOCKET; a mismatch yields an empty side panel.
func NewTmuxHandlerForSocket(socket string) *TmuxHandler {
	return newTmuxHandler(socket)
}

func newTmuxHandler(socket string) *TmuxHandler {
	return &TmuxHandler{
		cache: &sessionsCache{
			ttl: time.Second,
		},
		colorRegex: regexp.MustCompile(`^#[0-9A-Fa-f]{3,6}$|^[a-zA-Z]+$|^default$`),
		socket:     socket,
	}
}

// RegisterRoutes registers the tmux routes under the default cockpit prefix.
func (h *TmuxHandler) RegisterRoutes(mux *http.ServeMux) {
	h.RegisterRoutesWithPrefix(mux, "/api/tmux")
}

// RegisterRoutesWithPrefix registers the tmux routes under the given prefix
// (e.g. "/api/tmux" for the cockpit, "/api/formations/tmux" for the formations
// socket). The prefix must not end with a slash.
func (h *TmuxHandler) RegisterRoutesWithPrefix(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/sessions", h.ListSessions)
	mux.HandleFunc("POST "+prefix+"/sessions", h.CreateSession)
	mux.HandleFunc("DELETE "+prefix+"/sessions/all", h.DeleteAllSessions)
	mux.HandleFunc("DELETE "+prefix+"/sessions/{name}", h.DeleteSession)
	mux.HandleFunc("PATCH "+prefix+"/sessions/{name}", h.RenameSession)
	mux.HandleFunc("GET "+prefix+"/sessions/{name}/capture", h.CapturePane)
	mux.HandleFunc("POST "+prefix+"/appearance", h.ApplyAppearance)
	mux.HandleFunc("POST "+prefix+"/mouse", h.SetMouseMode)
}

// RunTmux satisfies the teams.TmuxRunner interface.
func (h *TmuxHandler) RunTmux(args ...string) (string, error) {
	return h.runTmux(args...)
}

// runTmux executes a tmux command with proper environment.
//
// When the handler is socket-scoped (NewTmuxHandlerForSocket), every invocation
// is prefixed with `-S <socket>` so it targets the EXACT same socket file as the
// formations executor (which also uses `tmux -S <socket> ...`, see
// formations/tmux_executor.go runTmuxCommand). The cockpit handler leaves the
// socket empty and relies on TMUX_TMPDIR via core.GetTmuxEnv().
func (h *TmuxHandler) runTmux(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if h.socket != "" {
		args = append([]string{"-S", h.socket}, args...)
	}

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
				windows, _ := strconv.Atoi(parts[1]) //nolint:errcheck // defaults to 0 on parse failure, corrected to 1 below
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

// protectedSessions is the list of sessions that should not be killed by nuke
var protectedSessions = map[string]bool{}

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
	var protectedNames []string
	if err == nil {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				if protectedSessions[line] {
					protectedNames = append(protectedNames, line)
				} else {
					sessionNames = append(sessionNames, line)
				}
			}
		}
	}

	if len(sessionNames) == 0 {
		core.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":   true,
			"killed":    0,
			"protected": protectedNames,
			"message":   "No sessions to kill (protected sessions preserved)",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	// Kill each session individually instead of kill-server to preserve protected sessions
	var killed []string
	var errors []string
	for _, name := range sessionNames {
		_, err := h.runTmux("kill-session", "-t", name)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
		} else {
			killed = append(killed, name)
		}
	}

	h.invalidateCache()

	response := map[string]interface{}{
		"success":   len(errors) == 0,
		"killed":    len(killed),
		"sessions":  killed,
		"protected": protectedNames,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	core.WriteJSON(w, http.StatusOK, response)
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

// CapturePane handles GET /api/tmux/sessions/{name}/capture
func (h *TmuxHandler) CapturePane(w http.ResponseWriter, r *http.Request) {
	sessionName := r.PathValue("name")

	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}

	// Build capture-pane command args
	args := []string{"capture-pane", "-t", sessionName, "-p"}

	// Support ?lines=N query param for scrollback
	if linesParam := r.URL.Query().Get("lines"); linesParam != "" {
		lines, err := strconv.Atoi(linesParam)
		if err != nil || lines < 0 {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid lines parameter: must be a non-negative integer")
			return
		}
		// -S -N means start from N lines before the current position
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}

	output, err := h.runTmux(args...)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"content": output,
		"session": sessionName,
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

// SetMouseMode handles POST /api/tmux/mouse — toggles tmux's global mouse mode.
// With mouse mode on, the scroll wheel scrolls tmux history (via copy-mode) in
// the browser terminal; it also enables click-to-select-pane and drag-select.
// This is a global tmux option, so it applies to every session at once and
// affects all attached clients. Errors are ignored the same way appearance does:
// the tmux server may not be running yet, which is not a failure worth surfacing.
func (h *TmuxHandler) SetMouseMode(w http.ResponseWriter, r *http.Request) {
	var req MouseModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	value := "off"
	if req.Enabled {
		value = "on"
	}

	_, err := h.runTmux("set", "-g", "mouse", value)
	applied := err == nil

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"mouse":     value,
		"applied":   applied,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
