// Package api provides HTTP handlers for the API
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

// Agent status constants
const (
	AgentStatusIdle     = "idle"
	AgentStatusWorking  = "working"
	AgentStatusComplete = "complete"
	AgentStatusError    = "error"
)

// Agent prefixes that indicate agent sessions. This is intentionally
// orchestrator-neutral: CHROTE watches tmux, not Gastown.
var defaultAgentPrefixes = []string{
	"agent-",
	"claude-",
	"codex",
	"gemini-",
	"hermes-",
	"opencode",
}

// OracleAgent represents an enriched agent session
type OracleAgent struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	ContextPct int      `json:"contextPct"`
	BeadID     string   `json:"beadId,omitempty"`
	LastLines  []string `json:"lastLines"`
	StartedAt  string   `json:"startedAt,omitempty"`
	Group      string   `json:"group"`
	Attached   bool     `json:"attached"`
}

// OracleStatus represents aggregate oracle stats
type OracleStatus struct {
	TotalAgents   int `json:"totalAgents"`
	WorkingAgents int `json:"workingAgents"`
	IdleAgents    int `json:"idleAgents"`
	BeadsActive   int `json:"beadsActive"`
	SSEClients    int `json:"sseClients"`
}

// OracleEvent represents an SSE event
type OracleEvent struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp string      `json:"timestamp"`
}

// RalphStatus represents a Ralph project status
type RalphStatus struct {
	Project string          `json:"project"`
	Path    string          `json:"path"`
	Status  json.RawMessage `json:"status"`
}

// agentSnapshot holds a snapshot for diffing
type agentSnapshot struct {
	Name       string
	Status     string
	ContextPct int
}

// OracleHandler handles agent-observability API endpoints. The route name is
// kept as /api/oracle for compatibility with the existing dashboard code.
type OracleHandler struct {
	tmuxHandler  *TmuxHandler
	beadsHandler *BeadsHandler

	// SSE broadcaster
	mu          sync.RWMutex
	clients     map[chan OracleEvent]struct{}
	lastSnap    map[string]agentSnapshot
	stopPoller  chan struct{}
	pollRunning bool

	// Compiled regexes
	contextRegex *regexp.Regexp
	beadRegex    *regexp.Regexp
}

// NewOracleHandler creates a new OracleHandler
func NewOracleHandler(tmux *TmuxHandler, beads *BeadsHandler) *OracleHandler {
	h := &OracleHandler{
		tmuxHandler:  tmux,
		beadsHandler: beads,
		clients:      make(map[chan OracleEvent]struct{}),
		lastSnap:     make(map[string]agentSnapshot),
		stopPoller:   make(chan struct{}),
		contextRegex: regexp.MustCompile(`(\d+)%\s*(?:context|of context)`),
		beadRegex:    regexp.MustCompile(`\b((?:home|pai|chrote)-[a-z0-9]+(?:\.[0-9]+)?)\b`),
	}
	h.startPoller()
	return h
}

// RegisterRoutes registers the oracle routes on the given mux
func (h *OracleHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/oracle/status", h.GetStatus)
	mux.HandleFunc("GET /api/oracle/agents", h.GetAgents)
	mux.HandleFunc("GET /api/oracle/stream", h.Stream)
	mux.HandleFunc("GET /api/oracle/ralph", h.GetRalph)
}

// runTmux delegates to the TmuxHandler's tmux execution
func (h *OracleHandler) runTmux(args ...string) (string, error) {
	return h.tmuxHandler.runTmux(args...)
}

// isAgentSession checks if a session name matches agent prefixes
func isAgentSession(name string) bool {
	for _, prefix := range agentPrefixes() {
		if strings.HasPrefix(name, prefix) || name == prefix {
			return true
		}
	}
	return false
}

func agentPrefixes() []string {
	if raw := os.Getenv("CHROTE_AGENT_PREFIXES"); raw != "" {
		parts := strings.Split(raw, ",")
		prefixes := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				prefixes = append(prefixes, part)
			}
		}
		if len(prefixes) > 0 {
			return prefixes
		}
	}
	return defaultAgentPrefixes
}

// ParseAgentStatus determines agent status from capture-pane output
func ParseAgentStatus(output string) string {
	lines := strings.Split(output, "\n")

	// Walk backwards to find last non-empty line
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Check for completion indicators
		lower := strings.ToLower(line)
		if strings.Contains(lower, "task complete") ||
			strings.Contains(lower, "completed successfully") {
			return AgentStatusComplete
		}

		// Check for error indicators
		if strings.Contains(lower, "error:") ||
			strings.Contains(lower, "fatal:") ||
			strings.Contains(lower, "panic:") {
			return AgentStatusError
		}

		// Check for idle prompt
		if strings.HasSuffix(line, "$") ||
			strings.HasSuffix(line, "#") ||
			strings.Contains(line, "\u276f") || // ❯
			strings.Contains(line, "\u2771") || // ❱
			strings.HasSuffix(line, ">") {
			return AgentStatusIdle
		}

		// If there's content but no prompt, agent is working
		return AgentStatusWorking
	}

	// Empty output means idle or not started
	return AgentStatusIdle
}

// ExtractContextPercent extracts context usage percentage from capture output
func ExtractContextPercent(output string, re *regexp.Regexp) int {
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0
	}
	// Return the last match (most recent)
	last := matches[len(matches)-1]
	var pct int
	fmt.Sscanf(last[1], "%d", &pct)
	if pct > 100 {
		pct = 100
	}
	return pct
}

// ExtractBeadID extracts the most recent bead reference from output
func ExtractBeadID(output string, re *regexp.Regexp) string {
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return ""
	}
	// Return the last match (most recent)
	return matches[len(matches)-1][1]
}

// extractLastLines returns the last N non-empty lines from output
func extractLastLines(output string, n int) []string {
	lines := strings.Split(output, "\n")
	var result []string
	for i := len(lines) - 1; i >= 0 && len(result) < n; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			result = append(result, line)
		}
	}
	// Reverse to maintain chronological order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// getAgentSessions returns all agent sessions from tmux
func (h *OracleHandler) getAgentSessions() ([]core.Session, error) {
	output, err := h.runTmux("list-sessions", "-F", "#{session_name}:#{session_windows}:#{session_attached}:#{session_created}")
	if err != nil {
		// No tmux server is fine — just no agents
		if strings.Contains(err.Error(), "no server running") ||
			strings.Contains(err.Error(), "No such file or directory") ||
			strings.Contains(err.Error(), "server exited unexpectedly") {
			return nil, nil // no error, no sessions = tmux server not running (expected when no agents active)
		}
		return nil, err
	}

	var agents []core.Session
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) < 3 {
			continue
		}
		name := parts[0]
		if !isAgentSession(name) {
			continue
		}
		var windows int
		fmt.Sscanf(parts[1], "%d", &windows)
		if windows == 0 {
			windows = 1
		}
		session := core.Session{
			Name:     name,
			Windows:  windows,
			Attached: parts[2] == "1",
			Group:    core.CategorizeSession(name),
		}
		agents = append(agents, session)
	}
	return agents, nil
}

// enrichAgent fetches capture data and enriches an agent session
func (h *OracleHandler) enrichAgent(session core.Session) OracleAgent {
	agent := OracleAgent{
		Name:     session.Name,
		Status:   AgentStatusIdle,
		Group:    session.Group,
		Attached: session.Attached,
	}

	// Capture scrollback (100 lines for context/bead scanning)
	output, err := h.runTmux("capture-pane", "-t", session.Name, "-p", "-S", "-100")
	if err != nil {
		return agent
	}

	agent.Status = ParseAgentStatus(output)
	agent.ContextPct = ExtractContextPercent(output, h.contextRegex)
	agent.BeadID = ExtractBeadID(output, h.beadRegex)
	agent.LastLines = extractLastLines(output, 5)

	return agent
}

// GetStatus handles GET /api/oracle/status
func (h *OracleHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	agents, err := h.getAgentSessions()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "ORACLE_ERROR", err.Error())
		return
	}

	status := OracleStatus{
		TotalAgents: len(agents),
	}

	// Enrich each to get status counts
	for _, session := range agents {
		agent := h.enrichAgent(session)
		switch agent.Status {
		case AgentStatusWorking:
			status.WorkingAgents++
		case AgentStatusIdle:
			status.IdleAgents++
		}
		if agent.BeadID != "" {
			status.BeadsActive++
		}
	}

	h.mu.RLock()
	status.SSEClients = len(h.clients)
	h.mu.RUnlock()

	core.WriteSuccess(w, status)
}

// GetAgents handles GET /api/oracle/agents
func (h *OracleHandler) GetAgents(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.getAgentSessions()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "ORACLE_ERROR", err.Error())
		return
	}

	agents := make([]OracleAgent, 0, len(sessions))
	for _, session := range sessions {
		agents = append(agents, h.enrichAgent(session))
	}

	core.WriteSuccess(w, map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	})
}

// Stream handles GET /api/oracle/stream (SSE)
func (h *OracleHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		core.WriteError(w, http.StatusInternalServerError, "SSE_ERROR", "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Create client channel
	ch := make(chan OracleEvent, 32)
	h.subscribe(ch)
	defer h.unsubscribe(ch)

	// Send initial connected event
	h.writeSSE(w, flusher, OracleEvent{
		Type:      "connected",
		Data:      map[string]string{"message": "Oracle SSE stream connected"},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			h.writeSSE(w, flusher, event)
		}
	}
}

// writeSSE writes a single SSE event
func (h *OracleHandler) writeSSE(w http.ResponseWriter, flusher http.Flusher, event OracleEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
	flusher.Flush()
}

// subscribe adds a client to the SSE broadcaster
func (h *OracleHandler) subscribe(ch chan OracleEvent) {
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
}

// unsubscribe removes a client from the SSE broadcaster
func (h *OracleHandler) unsubscribe(ch chan OracleEvent) {
	h.mu.Lock()
	delete(h.clients, ch)
	close(ch)
	h.mu.Unlock()
}

// broadcast sends an event to all connected SSE clients
func (h *OracleHandler) broadcast(event OracleEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- event:
		default:
			// Drop if client is slow
		}
	}
}

// startPoller begins the background tmux polling goroutine
func (h *OracleHandler) startPoller() {
	h.mu.Lock()
	if h.pollRunning {
		h.mu.Unlock()
		return
	}
	h.pollRunning = true
	h.mu.Unlock()

	go func() {
		pollTicker := time.NewTicker(10 * time.Second)
		heartbeatTicker := time.NewTicker(30 * time.Second)
		defer pollTicker.Stop()
		defer heartbeatTicker.Stop()

		for {
			select {
			case <-h.stopPoller:
				return
			case <-heartbeatTicker.C:
				h.broadcast(OracleEvent{
					Type:      "heartbeat",
					Data:      map[string]string{"status": "alive"},
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
			case <-pollTicker.C:
				h.pollAndDiff()
			}
		}
	}()
}

// pollAndDiff polls tmux and broadcasts diffs
func (h *OracleHandler) pollAndDiff() {
	sessions, err := h.getAgentSessions()
	if err != nil {
		return
	}

	newSnap := make(map[string]agentSnapshot)
	for _, session := range sessions {
		agent := h.enrichAgent(session)
		newSnap[agent.Name] = agentSnapshot{
			Name:       agent.Name,
			Status:     agent.Status,
			ContextPct: agent.ContextPct,
		}
	}

	h.mu.Lock()
	oldSnap := h.lastSnap
	h.lastSnap = newSnap
	h.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)

	// Check for new agents
	for name, snap := range newSnap {
		if _, existed := oldSnap[name]; !existed {
			h.broadcast(OracleEvent{
				Type:      "agent_new",
				Data:      snap,
				Timestamp: now,
			})
		}
	}

	// Check for status changes
	for name, snap := range newSnap {
		if old, existed := oldSnap[name]; existed {
			if old.Status != snap.Status || old.ContextPct != snap.ContextPct {
				h.broadcast(OracleEvent{
					Type:      "agent_status",
					Data:      snap,
					Timestamp: now,
				})
			}
		}
	}

	// Check for removed agents
	for name, snap := range oldSnap {
		if _, exists := newSnap[name]; !exists {
			h.broadcast(OracleEvent{
				Type:      "agent_removed",
				Data:      snap,
				Timestamp: now,
			})
		}
	}
}

// GetRalph handles GET /api/oracle/ralph
func (h *OracleHandler) GetRalph(w http.ResponseWriter, r *http.Request) {
	var results []RalphStatus

	roots := core.GetAllowedRoots()
	for _, root := range roots {
		// Walk one level deep looking for .ralph/status.json
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			statusPath := filepath.Join(root, entry.Name(), ".ralph", "status.json")
			data, err := os.ReadFile(statusPath)
			if err != nil {
				continue
			}
			// Validate it's valid JSON
			if !json.Valid(data) {
				continue
			}
			results = append(results, RalphStatus{
				Project: entry.Name(),
				Path:    filepath.Join(root, entry.Name()),
				Status:  json.RawMessage(data),
			})
		}
	}

	if results == nil {
		results = []RalphStatus{}
	}

	core.WriteSuccess(w, map[string]interface{}{
		"ralph":   results,
		"count":   len(results),
		"checked": roots,
	})
}

// Stop gracefully stops the poller (for testing/shutdown)
func (h *OracleHandler) Stop() {
	h.mu.Lock()
	if h.pollRunning {
		close(h.stopPoller)
		h.pollRunning = false
	}
	h.mu.Unlock()
}
