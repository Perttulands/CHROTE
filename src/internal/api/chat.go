// Package api provides HTTP handlers for the API
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// GastownWorkspace represents a discovered Gastown workspace
type GastownWorkspace struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ChatHandler handles ChroteChat API endpoints
// ChroteChat uses dual-channel delivery: Mail (persistence) + Nudge (real-time signal)
type ChatHandler struct {
	messageIDPattern *regexp.Regexp
}

// NewChatHandler creates a new ChatHandler and ensures chrote-chat session exists
func NewChatHandler() *ChatHandler {
	h := &ChatHandler{
		messageIDPattern: regexp.MustCompile(`hq-[a-z0-9]+`),
	}

	// Ensure chrote-chat session exists at startup
	h.ensureSessionExists()

	return h
}

// ensureSessionExists creates the chrote-chat session if it doesn't exist
func (h *ChatHandler) ensureSessionExists() {
	if h.sessionExists() {
		fmt.Printf("ChroteChat: Session '%s' already exists\n", ChroteChatSession)
		return
	}

	// Create session in home directory (will cd to workspace when sending)
	cmd := exec.Command("tmux", "new-session", "-d", "-s", ChroteChatSession)
	cmd.Env = core.GetTmuxEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("ChroteChat: Failed to create session '%s': %v, output: %s\n", ChroteChatSession, err, string(output))
		return
	}
	fmt.Printf("ChroteChat: Created session '%s' at startup\n", ChroteChatSession)
}

// ChroteChatSession is the name of the dedicated tmux session for ChroteChat
const ChroteChatSession = "chrote-chat"

// RegisterRoutes registers the chat routes on the given mux
func (h *ChatHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/chat/workspaces", h.ListWorkspaces)
	mux.HandleFunc("GET /api/chat/conversations", h.ListConversations)
	mux.HandleFunc("GET /api/chat/history", h.GetHistory)
	mux.HandleFunc("POST /api/chat/send", h.SendMessage)
	mux.HandleFunc("POST /api/chat/nudge", h.NudgeOnly)
	mux.HandleFunc("POST /api/chat/session/init", h.InitSession)
	mux.HandleFunc("POST /api/chat/session/restart", h.RestartSession)
	mux.HandleFunc("GET /api/chat/session/status", h.SessionStatus)
}

// SessionInfo contains session name and its Gastown workspace
type SessionInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Workspace string `json:"workspace,omitempty"` // Gastown workspace root, if found
}

// getSessionWorkspaceMap returns a map of session names to their Gastown workspace
// Uses multiple strategies: session_path, then pane_current_path as fallback
func (h *ChatHandler) getSessionWorkspaceMap() map[string]string {
	result := make(map[string]string)

	// Strategy 1: Try session_path (where session was created)
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{session_path}")
	cmd.Env = core.GetTmuxEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return result
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		sessionName := parts[0]
		sessionPath := parts[1]

		workspace := findGastownWorkspace(sessionPath)
		if workspace != "" {
			result[sessionName] = workspace
		}
	}

	// Strategy 2: For sessions without workspace, try pane_current_path
	// This handles sessions started from ~ that later cd'd into a workspace
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		sessionName := parts[0]

		// Skip if we already found a workspace
		if _, found := result[sessionName]; found {
			continue
		}

		// Get the current working directory of the active pane
		paneCmd := exec.Command("tmux", "display-message", "-t", sessionName, "-p", "#{pane_current_path}")
		paneCmd.Env = core.GetTmuxEnv()
		paneOutput, err := paneCmd.CombinedOutput()
		if err != nil {
			continue
		}

		panePath := strings.TrimSpace(string(paneOutput))
		if panePath != "" {
			workspace := findGastownWorkspace(panePath)
			if workspace != "" {
				result[sessionName] = workspace
			}
		}
	}

	return result
}

// ListWorkspaces handles GET /api/chat/workspaces
// Discovers Gastown workspaces from running tmux sessions
func (h *ChatHandler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	var workspaces []GastownWorkspace
	seen := make(map[string]bool)

	// Get session paths from tmux - this tells us where each session is running
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_path}")
	cmd.Env = core.GetTmuxEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		// No tmux sessions, return empty list
		core.WriteSuccess(w, map[string]interface{}{"workspaces": workspaces})
		return
	}

	// For each session path, walk up to find the Gastown workspace root (has daemon/)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, sessionPath := range lines {
		if sessionPath == "" {
			continue
		}

		// Walk up the directory tree to find a Gastown workspace
		workspace := findGastownWorkspace(sessionPath)
		if workspace != "" && !seen[workspace] {
			seen[workspace] = true
			workspaces = append(workspaces, GastownWorkspace{
				Name: filepath.Base(workspace),
				Path: workspace,
			})
		}
	}

	core.WriteSuccess(w, map[string]interface{}{"workspaces": workspaces})
}

// findGastownWorkspace walks up from a path to find a directory with daemon/
func findGastownWorkspace(startPath string) string {
	// Resolve symlinks to get canonical path
	resolved, err := filepath.EvalSymlinks(startPath)
	if err != nil {
		resolved = startPath
	}

	path := resolved
	for {
		daemonPath := filepath.Join(path, "daemon")
		if core.FileExists(daemonPath) {
			return path
		}

		parent := filepath.Dir(path)
		if parent == path || parent == "/" {
			break
		}
		path = parent
	}
	return ""
}

// ChatMessage represents a message in a conversation
type ChatMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`      // "user" (from UI) or "agent" (from recipient)
	From      string    `json:"from"`      // Sender path
	To        string    `json:"to"`        // Recipient path
	Content   string    `json:"content"`   // Message body
	Timestamp time.Time `json:"timestamp"`
	Read      bool      `json:"read"`
}

// Conversation represents a chat conversation with an agent
type Conversation struct {
	Target      string       `json:"target"`      // e.g., "mayor", "Chrote/jasper"
	DisplayName string       `json:"displayName"` // e.g., "Mayor", "Jasper"
	Role        string       `json:"role"`        // "mayor", "polecat", "witness", etc.
	Online      bool         `json:"online"`
	UnreadCount int          `json:"unreadCount"`
	LastMessage *ChatMessage `json:"lastMessage,omitempty"`
	Workspace   string       `json:"workspace,omitempty"` // Gastown workspace for this agent
}

// SendChatRequest is the request body for sending a chat message
type SendChatRequest struct {
	Workspace string `json:"workspace"` // Gastown workspace path
	Target    string `json:"target"`
	Message   string `json:"message"`
}

// SendChatResponse is the response after sending a chat message
type SendChatResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"messageId,omitempty"`
	MailSent  bool   `json:"mailSent"`
	Nudged    bool   `json:"nudged"`
	Error     string `json:"error,omitempty"`
}

// ListConversations handles GET /api/chat/conversations
// Returns a list of available chat targets with their status and workspace
func (h *ChatHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	conversations := []Conversation{}
	seenTargets := make(map[string]bool)

	// Get session name -> workspace map
	sessionWorkspaces := h.getSessionWorkspaceMap()

	// Get tmux sessions with both name and path
	tmuxCmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{session_path}")
	tmuxCmd.Env = core.GetTmuxEnv()

	output, err := tmuxCmd.CombinedOutput()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			parts := strings.SplitN(line, "|", 2)
			if len(parts) == 0 || parts[0] == "" {
				continue
			}
			sessionName := parts[0]

			// Parse role from session name
			target, displayName, role := h.parseSessionName(sessionName)

			// Only add if we parsed a valid role and haven't seen this target yet
			if role != "" && !seenTargets[target] {
				convo := Conversation{
					Target:      target,
					DisplayName: displayName,
					Role:        role,
					Online:      true,
					UnreadCount: 0,
				}

				// Add workspace if we found one for this session
				if ws, ok := sessionWorkspaces[sessionName]; ok {
					convo.Workspace = ws
				}

				conversations = append(conversations, convo)
				seenTargets[target] = true
			}
		}
	} else {
		fmt.Printf("Chat: 'tmux list-sessions' failed: %v\nOutput: %s\n", err, string(output))
	}

	// Sort: Mayor/Deacon first, then alphabetically
	h.sortConversations(conversations)

	core.WriteSuccess(w, map[string]interface{}{
		"conversations": conversations,
	})
}

// parseSessionName derives chat identity from tmux session name
func (h *ChatHandler) parseSessionName(name string) (target, displayName, role string) {
	// Defaults
	role = ""
	displayName = name
	target = name

	// 1. HQ Role patterns
	if strings.Contains(name, "mayor") {
		return "mayor", "🎩 Mayor", "mayor"
	}
	if strings.Contains(name, "deacon") {
		return "deacon", "🐺 Deacon", "deacon"
	}

	// 2. Gastown Standard Patterns: gt-<rig>-<role>-<name> or gt-<rig>-<role>
	// e.g. "gt-Chrote-crew-Ronja", "gt-Chrote-witness"
	parts := strings.Split(name, "-")

	if len(parts) >= 2 && parts[0] == "gt" {
		// Identify Role
		if strings.Contains(name, "witness") {
			return "witness", "🦉 Witness", "witness"
		}
		if strings.Contains(name, "refinery") {
			return "refinery", "🏭 Refinery", "refinery"
		}

		// Worker / Polecat / Crew detection
		// "gt-Chrote-crew-Ronja" -> parts[2]="crew", parts[3]="Ronja"
		// Crew address format: <rig>/crew/<name> (e.g., "Chrote/crew/Ronja")
		for i, part := range parts {
			if part == "crew" && i+1 < len(parts) {
				workerName := parts[i+1]
				formattedName := strings.Title(workerName)
				rigName := parts[1]
				// Crew workers use rig/crew/name format
				target = fmt.Sprintf("%s/crew/%s", rigName, workerName)

				return target, fmt.Sprintf("👷 %s", formattedName), "crew"
			}
			if part == "polecat" || part == "pc" {
				// Handle "gt-rig-polecat-5"
				suffix := ""
				if i+1 < len(parts) {
					suffix = " " + parts[i+1]
				}
				// Polecats might not have names, use session name or generated ID
				return name, fmt.Sprintf("🐱 Polecat%s", suffix), "polecat"
			}
		}

		// Fallback for generic "gt-Chrote-jasper" -> assume polecat/worker with name "jasper"
		// Must have at least 3 parts: gt-<rig>-<name>
		if len(parts) >= 3 {
			lastPart := parts[len(parts)-1]
			if lastPart != "witness" && lastPart != "refinery" && lastPart != "mayor" && lastPart != "boot" {
				rigName := parts[1]
				target = fmt.Sprintf("%s/%s", rigName, lastPart)
				return target, fmt.Sprintf("🐱 %s", strings.Title(lastPart)), "polecat"
			}
		}
	}

	// 3. Allow all other sessions for debugging?
	// For now, strict strictness to avoid clutter.
	return "", "", ""
}

// sortConversations helper
func (h *ChatHandler) sortConversations(convos []Conversation) {
	score := func(c Conversation) int {
		switch c.Role {
		case "mayor": return 0
		case "deacon": return 1
		case "witness": return 2
		case "refinery": return 3
		case "polecat": return 4
		case "crew": return 5
		default: return 99
		}
	}

	// Simple bubble sort for list (list is small)
	for i := 0; i < len(convos)-1; i++ {
		for j := i + 1; j < len(convos); j++ {
			s1, s2 := score(convos[i]), score(convos[j])
			if s1 > s2 || (s1 == s2 && convos[i].DisplayName > convos[j].DisplayName) {
				convos[i], convos[j] = convos[j], convos[i]
			}
		}
	}
}

// parseStatusForConversations parses gt status output to build conversation list
func (h *ChatHandler) parseStatusForConversations(output string) []Conversation {
	// Dummy parser for now, relying on fallback
	// In future, this parses the 'gt status' rich JSON/Text output
	return []Conversation{}
}


// GetHistory handles GET /api/chat/history?target=...&workspace=...
func (h *ChatHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		core.WriteError(w, http.StatusBadRequest, "MISSING_TARGET", "Target query parameter is required")
		return
	}

	workspace := r.URL.Query().Get("workspace")
	if workspace == "" {
		core.WriteError(w, http.StatusBadRequest, "MISSING_WORKSPACE", "Workspace query parameter is required")
		return
	}

	// Validate workspace
	daemonPath := filepath.Join(workspace, "daemon")
	if !core.FileExists(daemonPath) {
		core.WriteError(w, http.StatusBadRequest, "INVALID_WORKSPACE", "Not a valid Gastown workspace")
		return
	}

	var messages []ChatMessage

	// 1. Get messages TO the target (sent by overseer/user)
	sentMessages := h.getMailboxMessages(workspace, target)
	for _, msg := range sentMessages {
		if msg.From == "overseer" {
			messages = append(messages, ChatMessage{
				ID:        msg.ID,
				Role:      "user",
				From:      msg.From,
				To:        msg.To,
				Content:   msg.Body,
				Timestamp: msg.Timestamp,
				Read:      msg.Read,
			})
		}
	}

	// 2. Get messages FROM the target (replies to overseer)
	receivedMessages := h.getMailboxMessages(workspace, "overseer")
	for _, msg := range receivedMessages {
		// Filter for messages from our target
		if h.normalizeTarget(msg.From) == h.normalizeTarget(target) {
			messages = append(messages, ChatMessage{
				ID:        msg.ID,
				Role:      "agent",
				From:      msg.From,
				To:        msg.To,
				Content:   msg.Body,
				Timestamp: msg.Timestamp,
				Read:      msg.Read,
			})
		}
	}

	// Sort by timestamp (oldest first for chat display)
	for i := 0; i < len(messages)-1; i++ {
		for j := i + 1; j < len(messages); j++ {
			if messages[i].Timestamp.After(messages[j].Timestamp) {
				messages[i], messages[j] = messages[j], messages[i]
			}
		}
	}

	core.WriteSuccess(w, map[string]interface{}{
		"messages": messages,
	})
}

// MailMessage represents a message from gt mail inbox --json
type MailMessage struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Timestamp time.Time `json:"timestamp"`
	Read      bool      `json:"read"`
	Priority  string    `json:"priority"`
	Type      string    `json:"type"`
	ThreadID  string    `json:"thread_id"`
	ReplyTo   string    `json:"reply_to,omitempty"`
}

// getMailboxMessages fetches messages from a mailbox using gt mail inbox
func (h *ChatHandler) getMailboxMessages(workspace, mailbox string) []MailMessage {
	// Build command: gt mail inbox <mailbox> --json
	cmd := exec.Command("gt", "mail", "inbox", mailbox, "--json")
	cmd.Dir = workspace
	cmd.Env = h.getGtEnv()

	fmt.Printf("ChroteChat: Running gt mail inbox %s --json (dir=%s)\n", mailbox, workspace)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("ChroteChat: gt mail inbox %s FAILED: %v\nOutput: %s\n", mailbox, err, string(output))
		return nil
	}

	fmt.Printf("ChroteChat: Got %d bytes from gt mail inbox %s\n", len(output), mailbox)

	var messages []MailMessage
	if err := json.Unmarshal(output, &messages); err != nil {
		fmt.Printf("ChroteChat: JSON parse error: %v\nRaw: %s\n", err, string(output))
		return nil
	}

	fmt.Printf("ChroteChat: Parsed %d messages from %s\n", len(messages), mailbox)
	return messages
}

// getGtEnv returns environment for running gt commands
func (h *ChatHandler) getGtEnv() []string {
	env := core.GetTmuxEnv()
	// Replace PATH to include gt and bd locations (vendor path for deployed system)
	gtPath := "/home/chrote/chrote/vendor/gastown:/home/chrote/.local/bin:/usr/local/bin:/usr/bin:/bin"
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + gtPath
			return env
		}
	}
	// No PATH found, add it
	env = append(env, "PATH="+gtPath)
	return env
}

// normalizeTarget normalizes target addresses for comparison
// e.g., "mayor/" -> "mayor", "Chrote/jasper" -> "Chrote/jasper"
func (h *ChatHandler) normalizeTarget(target string) string {
	return strings.TrimSuffix(strings.TrimSpace(target), "/")
}

// SendMessage handles POST /api/chat/send
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	var req SendChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if req.Workspace == "" {
		core.WriteError(w, http.StatusBadRequest, "MISSING_WORKSPACE", "Workspace is required")
		return
	}

	if req.Target == "" {
		core.WriteError(w, http.StatusBadRequest, "MISSING_TARGET", "Target is required")
		return
	}

	if req.Message == "" {
		core.WriteError(w, http.StatusBadRequest, "EMPTY_MESSAGE", "Message cannot be empty")
		return
	}

	// Validate workspace path exists and has daemon/ directory
	daemonPath := filepath.Join(req.Workspace, "daemon")
	if !core.FileExists(daemonPath) {
		core.WriteError(w, http.StatusBadRequest, "INVALID_WORKSPACE", "Not a valid Gastown workspace: "+req.Workspace)
		return
	}

	// Check that chrote-chat session exists
	if !h.sessionExists() {
		core.WriteError(w, http.StatusPreconditionFailed, "SESSION_NOT_FOUND",
			"chrote-chat session not found. Please restart the chat session.")
		return
	}

	msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	fmt.Printf("\n=== ChroteChat SendMessage ===\n")
	fmt.Printf("Request:\n")
	fmt.Printf("  Workspace: %s\n", req.Workspace)
	fmt.Printf("  Target: %s\n", req.Target)
	fmt.Printf("  Message: %s\n", req.Message)

	// Escape message for shell - replace single quotes with '\''
	escapedMessage := strings.ReplaceAll(req.Message, "'", "'\\''")

	// 1. Send via Mail (Persistence)
	// Build command: cd <workspace> && gt mail send <target> -s 'Chat Message' -m '<message>'
	mailCommand := fmt.Sprintf("cd '%s' && gt mail send '%s' -s 'Chat Message' -m '%s'",
		req.Workspace, req.Target, escapedMessage)
	fmt.Printf("\nMail command: %s\n", mailCommand)

	mailSent := h.sendToSession(mailCommand)
	if mailSent {
		fmt.Printf("Mail command sent to chrote-chat session\n")
	} else {
		fmt.Printf("Mail FAILED to send to session\n")
	}

	// Small delay between commands
	time.Sleep(100 * time.Millisecond)

	// 2. Nudge (Real-time attention)
	nudgeCommand := fmt.Sprintf("cd '%s' && gt nudge '%s' 'New chat message'",
		req.Workspace, req.Target)
	fmt.Printf("\nNudge command: %s\n", nudgeCommand)

	nudged := h.sendToSession(nudgeCommand)
	if nudged {
		fmt.Printf("Nudge command sent to chrote-chat session\n")
	} else {
		fmt.Printf("Nudge FAILED to send to session\n")
	}

	fmt.Printf("\nResult: mailSent=%v, nudged=%v\n", mailSent, nudged)
	fmt.Printf("=== End SendMessage ===\n\n")

	if !mailSent && !nudged {
		core.WriteError(w, http.StatusInternalServerError, "SEND_FAILED", "Failed to send message via any channel")
		return
	}

	core.WriteSuccess(w, SendChatResponse{
		Success:   true,
		MessageID: msgID,
		MailSent:  mailSent,
		Nudged:    nudged,
	})
}

// NudgeRequest is the request body for nudge-only
type NudgeRequest struct {
	Workspace string `json:"workspace"`
	Target    string `json:"target"`
	Message   string `json:"message,omitempty"` // Optional custom message
}

// NudgeResponse is the response for nudge-only
type NudgeResponse struct {
	Success bool `json:"success"`
	Nudged  bool `json:"nudged"`
}

// NudgeOnly sends just a nudge without mail (for quick pings)
func (h *ChatHandler) NudgeOnly(w http.ResponseWriter, r *http.Request) {
	var req NudgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if req.Workspace == "" {
		core.WriteError(w, http.StatusBadRequest, "MISSING_WORKSPACE", "workspace is required")
		return
	}
	if req.Target == "" {
		core.WriteError(w, http.StatusBadRequest, "MISSING_TARGET", "target is required")
		return
	}

	// Validate workspace
	daemonPath := filepath.Join(req.Workspace, "daemon")
	if !core.FileExists(daemonPath) {
		core.WriteError(w, http.StatusBadRequest, "INVALID_WORKSPACE", "Not a valid Gastown workspace: "+req.Workspace)
		return
	}

	// Check session exists
	if !h.sessionExists() {
		core.WriteError(w, http.StatusPreconditionFailed, "SESSION_NOT_FOUND",
			"chrote-chat session not found. Please restart the chat session.")
		return
	}

	// Default nudge message
	nudgeMsg := "Check your mail"
	if req.Message != "" {
		nudgeMsg = strings.ReplaceAll(req.Message, "'", "'\\''")
	}

	fmt.Printf("\n=== ChroteChat NudgeOnly ===\n")
	fmt.Printf("  Workspace: %s\n", req.Workspace)
	fmt.Printf("  Target: %s\n", req.Target)
	fmt.Printf("  Message: %s\n", nudgeMsg)

	nudgeCommand := fmt.Sprintf("cd '%s' && gt nudge '%s' '%s'",
		req.Workspace, req.Target, nudgeMsg)
	fmt.Printf("Nudge command: %s\n", nudgeCommand)

	nudged := h.sendToSession(nudgeCommand)
	fmt.Printf("Result: nudged=%v\n", nudged)
	fmt.Printf("=== End NudgeOnly ===\n\n")

	if !nudged {
		core.WriteError(w, http.StatusInternalServerError, "NUDGE_FAILED", "Failed to nudge target")
		return
	}

	core.WriteSuccess(w, NudgeResponse{
		Success: true,
		Nudged:  true,
	})
}

// sendToSession sends a command to the chrote-chat tmux session via send-keys
func (h *ChatHandler) sendToSession(command string) bool {
	fmt.Printf("ChroteChat: Sending to session '%s': %s\n", ChroteChatSession, command)

	// Use tmux send-keys to inject the command
	cmd := exec.Command("tmux", "send-keys", "-t", ChroteChatSession, command, "Enter")
	cmd.Env = core.GetTmuxEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("ChroteChat: tmux send-keys failed: %v, output: %s\n", err, string(output))
		return false
	}
	fmt.Printf("ChroteChat: tmux send-keys succeeded\n")
	return true
}

// SessionStatusResponse contains chrote-chat session status
type SessionStatusResponse struct {
	Exists    bool   `json:"exists"`
	Workspace string `json:"workspace,omitempty"`
}

// SessionStatus handles GET /api/chat/session/status
func (h *ChatHandler) SessionStatus(w http.ResponseWriter, r *http.Request) {
	exists := h.sessionExists()
	core.WriteSuccess(w, SessionStatusResponse{
		Exists: exists,
	})
}

// InitSessionRequest contains workspace to initialize session in
type InitSessionRequest struct {
	Workspace string `json:"workspace"`
}

// InitSession handles POST /api/chat/session/init
// Creates the chrote-chat tmux session if it doesn't exist
func (h *ChatHandler) InitSession(w http.ResponseWriter, r *http.Request) {
	var req InitSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if req.Workspace == "" {
		core.WriteError(w, http.StatusBadRequest, "MISSING_WORKSPACE", "Workspace is required")
		return
	}

	// Check if session already exists
	if h.sessionExists() {
		core.WriteSuccess(w, map[string]interface{}{
			"created": false,
			"message": "Session already exists",
		})
		return
	}

	// Create the session in the workspace directory
	err := h.createSession(req.Workspace)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "CREATE_FAILED", err.Error())
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"created":   true,
		"workspace": req.Workspace,
	})
}

// RestartSession handles POST /api/chat/session/restart
// Kills and recreates the chrote-chat tmux session
func (h *ChatHandler) RestartSession(w http.ResponseWriter, r *http.Request) {
	var req InitSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if req.Workspace == "" {
		core.WriteError(w, http.StatusBadRequest, "MISSING_WORKSPACE", "Workspace is required")
		return
	}

	// Kill existing session if it exists
	if h.sessionExists() {
		killCmd := exec.Command("tmux", "kill-session", "-t", ChroteChatSession)
		killCmd.Env = core.GetTmuxEnv()
		killCmd.Run() // Ignore error, session might not exist
	}

	// Create fresh session
	err := h.createSession(req.Workspace)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "CREATE_FAILED", err.Error())
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"restarted": true,
		"workspace": req.Workspace,
	})
}

// sessionExists checks if the chrote-chat tmux session exists
func (h *ChatHandler) sessionExists() bool {
	cmd := exec.Command("tmux", "has-session", "-t", ChroteChatSession)
	cmd.Env = core.GetTmuxEnv()
	return cmd.Run() == nil
}

// createSession creates the chrote-chat tmux session in the given workspace
func (h *ChatHandler) createSession(workspace string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", ChroteChatSession, "-c", workspace)
	cmd.Env = core.GetTmuxEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create session: %v, output: %s", err, string(output))
	}
	fmt.Printf("ChroteChat: Created session '%s' in workspace '%s'\n", ChroteChatSession, workspace)
	return nil
}
