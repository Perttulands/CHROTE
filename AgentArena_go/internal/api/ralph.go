// Package api provides HTTP handlers for the API
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/agentarena/server/internal/core"
)

// RalphHandler handles ralph-related API endpoints
type RalphHandler struct {
	ralphCommand     string
	execTimeout      time.Duration
	templatesDir     string
	sessionNameRegex *regexp.Regexp
}

// NewRalphHandler creates a new RalphHandler
func NewRalphHandler() *RalphHandler {
	return &RalphHandler{
		ralphCommand:     "/usr/local/bin/ralph",
		execTimeout:      60 * time.Second,
		templatesDir:     "/vault/ralph-templates",
		sessionNameRegex: regexp.MustCompile(`^[a-zA-Z0-9_-]+$`),
	}
}

// RegisterRoutes registers the ralph routes on the given mux
func (h *RalphHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ralph/health", h.Health)
	mux.HandleFunc("GET /api/ralph/projects", h.ListProjects)
	mux.HandleFunc("GET /api/ralph/status", h.Status)
	mux.HandleFunc("GET /api/ralph/session", h.Session)
	mux.HandleFunc("POST /api/ralph/start", h.Start)
	mux.HandleFunc("POST /api/ralph/stop", h.Stop)
	mux.HandleFunc("POST /api/ralph/circuit-breaker/reset", h.ResetCircuitBreaker)
	mux.HandleFunc("GET /api/ralph/logs", h.Logs)
	mux.HandleFunc("GET /api/ralph/templates", h.Templates)
}

// checkRalphInstalled checks if ralph is available
func (h *RalphHandler) checkRalphInstalled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.ralphCommand, "--help")
	return cmd.Run() == nil
}

// runTmux executes a tmux command with proper environment
func (h *RalphHandler) runTmux(args ...string) (string, error) {
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

// getRalphSessionState reads the .ralph_session file
func (h *RalphHandler) getRalphSessionState(projectPath string) map[string]interface{} {
	sessionFile := filepath.Join(projectPath, ".ralph_session")
	if !core.FileExists(sessionFile) {
		return nil
	}

	content, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil
	}

	// Try JSON first
	var state map[string]interface{}
	if err := json.Unmarshal(content, &state); err == nil {
		return state
	}

	// Try key=value format
	state = make(map[string]interface{})
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			state[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	if len(state) > 0 {
		return state
	}
	return nil
}

// getRalphSessionHistory reads the .ralph_session_history file
func (h *RalphHandler) getRalphSessionHistory(projectPath string, limit int) []map[string]interface{} {
	historyFile := filepath.Join(projectPath, ".ralph_session_history")
	if !core.FileExists(historyFile) {
		return []map[string]interface{}{}
	}

	content, err := os.ReadFile(historyFile)
	if err != nil {
		return []map[string]interface{}{}
	}

	var items []map[string]interface{}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item map[string]interface{}
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			items = append(items, item)
		}
	}

	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

// getRalphLogs reads logs from the logs directory
func (h *RalphHandler) getRalphLogs(projectPath string, limit int) map[string]interface{} {
	logsDir := filepath.Join(projectPath, "logs")
	if !core.FileExists(logsDir) {
		return nil
	}

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "ralph-") && strings.HasSuffix(entry.Name(), ".log") {
			files = append(files, entry.Name())
		}
	}

	if len(files) == 0 {
		return nil
	}

	// Sort and get latest
	latestLog := filepath.Join(logsDir, files[len(files)-1])
	content, err := os.ReadFile(latestLog)
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	return map[string]interface{}{
		"file":       files[len(files)-1],
		"lines":      lines,
		"totalFiles": len(files),
	}
}

// isRalphProject checks if a directory is a ralph project
func (h *RalphHandler) isRalphProject(projectPath string) bool {
	return core.FileExists(filepath.Join(projectPath, "PROMPT.md")) ||
		core.FileExists(filepath.Join(projectPath, ".ralph_session")) ||
		core.FileExists(filepath.Join(projectPath, "@fix_plan.md"))
}

// generateSessionName generates a session name from project path
func (h *RalphHandler) generateSessionName(projectPath string) string {
	projectName := filepath.Base(projectPath)
	projectName = strings.ToLower(projectName)
	projectName = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(projectName, "-")
	projectName = regexp.MustCompile(`-+`).ReplaceAllString(projectName, "-")
	if len(projectName) > 20 {
		projectName = projectName[:20]
	}
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 36)
	if len(timestamp) > 6 {
		timestamp = timestamp[len(timestamp)-6:]
	}
	return "ralph-" + projectName + "-" + timestamp
}

// findRalphSession finds if there's an active ralph session for a project
func (h *RalphHandler) findRalphSession(projectPath string) string {
	output, err := h.runTmux("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	state := h.getRalphSessionState(projectPath)
	if state == nil {
		return ""
	}

	sessionName, _ := state["session_name"].(string)
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" && strings.HasPrefix(name, "ralph-") {
			if name == sessionName {
				return name
			}
		}
	}
	return ""
}

// Health handles GET /api/ralph/health
func (h *RalphHandler) Health(w http.ResponseWriter, r *http.Request) {
	if !h.checkRalphInstalled() {
		core.WriteError(w, http.StatusServiceUnavailable, "RALPH_NOT_INSTALLED",
			"Ralph not found. Mount a checkout at ./vendor/ralph-claude-code.")
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"status":       "ok",
		"installed":    true,
		"allowedRoots": core.AllowedRoots,
		"templatesDir": h.templatesDir,
	})
}

// ListProjects handles GET /api/ralph/projects
func (h *RalphHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	var projects []map[string]interface{}
	var warnings []string

	for _, root := range core.AllowedRoots {
		if !core.FileExists(root) {
			warnings = append(warnings, "Allowed root does not exist: "+root)
			continue
		}

		entries, err := os.ReadDir(root)
		if err != nil {
			warnings = append(warnings, "Cannot read directory "+root+": "+err.Error())
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				projectPath := filepath.Join(root, entry.Name())
				if h.isRalphProject(projectPath) {
					state := h.getRalphSessionState(projectPath)
					projects = append(projects, map[string]interface{}{
						"name":          entry.Name(),
						"path":          projectPath,
						"hasPrompt":     core.FileExists(filepath.Join(projectPath, "PROMPT.md")),
						"hasFixPlan":    core.FileExists(filepath.Join(projectPath, "@fix_plan.md")),
						"hasSession":    state != nil,
						"sessionActive": state != nil && h.findRalphSession(projectPath) != "",
					})
				}
			}
		}

		// Check root itself
		if h.isRalphProject(root) {
			state := h.getRalphSessionState(root)
			projects = append(projects, map[string]interface{}{
				"name":          filepath.Base(root),
				"path":          root,
				"hasPrompt":     core.FileExists(filepath.Join(root, "PROMPT.md")),
				"hasFixPlan":    core.FileExists(filepath.Join(root, "@fix_plan.md")),
				"hasSession":    state != nil,
				"sessionActive": state != nil && h.findRalphSession(root) != "",
			})
		}
	}

	result := map[string]interface{}{"projects": projects}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}
	core.WriteSuccess(w, result)
}

// Status handles GET /api/ralph/status
func (h *RalphHandler) Status(w http.ResponseWriter, r *http.Request) {
	projectPath, code, msg := core.ValidateProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if !h.checkRalphInstalled() {
		core.WriteError(w, http.StatusServiceUnavailable, "RALPH_NOT_INSTALLED", "Ralph not found.")
		return
	}

	state := h.getRalphSessionState(projectPath)
	activeSession := h.findRalphSession(projectPath)

	loopCount := 0
	currentPhase := "idle"
	completionIndicators := 0
	exitSignalReceived := false
	var rateLimitStatus map[string]interface{}

	if state != nil {
		if v, ok := state["loop_count"]; ok {
			loopCount, _ = strconv.Atoi(fmt.Sprintf("%v", v))
		} else if v, ok := state["loopCount"]; ok {
			loopCount, _ = strconv.Atoi(fmt.Sprintf("%v", v))
		}

		if v, ok := state["phase"]; ok {
			currentPhase = fmt.Sprintf("%v", v)
		} else if v, ok := state["status"]; ok {
			currentPhase = fmt.Sprintf("%v", v)
		} else if activeSession != "" {
			currentPhase = "running"
		}

		if v, ok := state["completion_indicators"]; ok {
			completionIndicators, _ = strconv.Atoi(fmt.Sprintf("%v", v))
		}

		exitSignal := fmt.Sprintf("%v", state["exit_signal"])
		exitSignalReceived = exitSignal == "true"
		if !exitSignalReceived {
			if v, ok := state["exitSignal"].(bool); ok {
				exitSignalReceived = v
			}
		}

		if state["calls_used"] != nil || state["callsUsed"] != nil {
			callsUsed := 0
			maxCalls := 100
			if v, ok := state["calls_used"]; ok {
				callsUsed, _ = strconv.Atoi(fmt.Sprintf("%v", v))
			} else if v, ok := state["callsUsed"]; ok {
				callsUsed, _ = strconv.Atoi(fmt.Sprintf("%v", v))
			}
			if v, ok := state["max_calls"]; ok {
				maxCalls, _ = strconv.Atoi(fmt.Sprintf("%v", v))
			} else if v, ok := state["maxCalls"]; ok {
				maxCalls, _ = strconv.Atoi(fmt.Sprintf("%v", v))
			}
			resetTime, _ := state["reset_time"].(string)
			if resetTime == "" {
				resetTime, _ = state["resetTime"].(string)
			}

			rateLimitStatus = map[string]interface{}{
				"callsUsed":      callsUsed,
				"maxCalls":       maxCalls,
				"callsRemaining": maxCalls - callsUsed,
				"resetTime":      resetTime,
			}
		}
	}

	core.WriteSuccess(w, map[string]interface{}{
		"projectPath":          projectPath,
		"installed":            true,
		"sessionActive":        activeSession != "",
		"sessionName":          activeSession,
		"loopCount":            loopCount,
		"currentPhase":         currentPhase,
		"rateLimitStatus":      rateLimitStatus,
		"completionIndicators": completionIndicators,
		"exitSignalReceived":   exitSignalReceived,
		"hasPrompt":            core.FileExists(filepath.Join(projectPath, "PROMPT.md")),
		"hasFixPlan":           core.FileExists(filepath.Join(projectPath, "@fix_plan.md")),
	})
}

// Session handles GET /api/ralph/session
func (h *RalphHandler) Session(w http.ResponseWriter, r *http.Request) {
	projectPath, code, msg := core.ValidateProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	state := h.getRalphSessionState(projectPath)
	history := h.getRalphSessionHistory(projectPath, 10)
	activeSession := h.findRalphSession(projectPath)

	core.WriteSuccess(w, map[string]interface{}{
		"projectPath":   projectPath,
		"state":         state,
		"history":       history,
		"activeSession": activeSession,
	})
}

// StartRequest is the request body for starting ralph
type StartRequest struct {
	Path        string                 `json:"path"`
	SessionName string                 `json:"sessionName"`
	Flags       map[string]interface{} `json:"flags"`
}

// Start handles POST /api/ralph/start
func (h *RalphHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	resolvedPath, code, msg := core.ValidateProjectPath(req.Path)
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	// Check if already running
	existingSession := h.findRalphSession(resolvedPath)
	if existingSession != "" {
		core.WriteError(w, http.StatusConflict, "ALREADY_RUNNING",
			"Ralph is already running for this project in session: "+existingSession)
		return
	}

	// Generate or validate session name
	sessionName := req.SessionName
	if sessionName == "" {
		sessionName = h.generateSessionName(resolvedPath)
	} else {
		valid, errMsg := core.ValidateSessionName(sessionName, "session name")
		if !valid {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
			return
		}
	}

	// Build ralph command with flags
	var ralphArgs []string
	if req.Flags != nil {
		if v, ok := req.Flags["monitor"].(bool); ok && v {
			ralphArgs = append(ralphArgs, "--monitor")
		}
		if v, ok := req.Flags["verbose"].(bool); ok && v {
			ralphArgs = append(ralphArgs, "--verbose")
		}
		if v, ok := req.Flags["timeout"]; ok {
			ralphArgs = append(ralphArgs, "--timeout", fmt.Sprintf("%v", v))
		}
		if v, ok := req.Flags["calls"]; ok {
			ralphArgs = append(ralphArgs, "--calls", fmt.Sprintf("%v", v))
		}
		if v, ok := req.Flags["prompt"].(string); ok && v != "" {
			ralphArgs = append(ralphArgs, "--prompt", v)
		}
		if v, ok := req.Flags["continue"].(bool); ok && !v {
			ralphArgs = append(ralphArgs, "--no-continue")
		}
	}

	ralphCmd := "ralph"
	if len(ralphArgs) > 0 {
		ralphCmd = "ralph " + strings.Join(ralphArgs, " ")
	}

	// Create tmux session and run Ralph
	_, err := h.runTmux("new-session", "-d", "-s", sessionName, "-c", resolvedPath, ralphCmd)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"sessionName": sessionName,
		"projectPath": resolvedPath,
		"flags":       req.Flags,
		"message":     "Ralph session started",
	})
}

// StopRequest is the request body for stopping ralph
type StopRequest struct {
	SessionName string `json:"sessionName"`
}

// Stop handles POST /api/ralph/stop
func (h *RalphHandler) Stop(w http.ResponseWriter, r *http.Request) {
	var req StopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	valid, errMsg := core.ValidateSessionName(req.SessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}

	// Send Ctrl+C to gracefully stop Ralph
	h.runTmux("send-keys", "-t", req.SessionName, "C-c")

	// Kill the session after a delay (non-blocking)
	go func() {
		time.Sleep(time.Second)
		h.runTmux("kill-session", "-t", req.SessionName)
	}()

	core.WriteSuccess(w, map[string]interface{}{
		"sessionName": req.SessionName,
		"message":     "Ralph session stopping",
	})
}

// CircuitBreakerResetRequest is the request body for resetting circuit breaker
type CircuitBreakerResetRequest struct {
	Path string `json:"path"`
}

// ResetCircuitBreaker handles POST /api/ralph/circuit-breaker/reset
func (h *RalphHandler) ResetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	var req CircuitBreakerResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	projectPath, code, msg := core.ValidateProjectPath(req.Path)
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	// Try running ralph --reset-circuit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.ralphCommand, "--reset-circuit")
	cmd.Dir = projectPath
	if err := cmd.Run(); err != nil {
		// Try deleting the circuit breaker file directly
		cbFile := filepath.Join(projectPath, ".ralph_circuit_breaker")
		if core.FileExists(cbFile) {
			os.Remove(cbFile)
		}
	}

	core.WriteSuccess(w, map[string]interface{}{
		"projectPath": projectPath,
		"message":     "Circuit breaker reset",
	})
}

// Logs handles GET /api/ralph/logs
func (h *RalphHandler) Logs(w http.ResponseWriter, r *http.Request) {
	projectPath, code, msg := core.ValidateProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	logs := h.getRalphLogs(projectPath, limit)

	core.WriteSuccess(w, map[string]interface{}{
		"projectPath": projectPath,
		"logs":        logs,
	})
}

// Templates handles GET /api/ralph/templates
func (h *RalphHandler) Templates(w http.ResponseWriter, r *http.Request) {
	var templates []map[string]interface{}

	if core.FileExists(h.templatesDir) {
		entries, err := os.ReadDir(h.templatesDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") && entry.Name() != "README.md" {
					filePath := filepath.Join(h.templatesDir, entry.Name())
					info, _ := entry.Info()

					description := ""
					if content, err := os.ReadFile(filePath); err == nil {
						lines := strings.Split(string(content), "\n")
						if len(lines) > 0 {
							description = strings.TrimPrefix(strings.TrimSpace(lines[0]), "# ")
						}
					}

					size := int64(0)
					if info != nil {
						size = info.Size()
					}

					templates = append(templates, map[string]interface{}{
						"name":        strings.TrimSuffix(entry.Name(), ".md"),
						"file":        entry.Name(),
						"path":        filePath,
						"description": description,
						"size":        size,
					})
				}
			}
		}
	}

	core.WriteSuccess(w, map[string]interface{}{
		"templates":    templates,
		"templatesDir": h.templatesDir,
	})
}
