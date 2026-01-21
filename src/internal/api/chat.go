// Package api provides HTTP handlers for the API
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// ChatHandler handles ChroteChat API endpoints
// ChroteChat uses dual-channel delivery: Mail (persistence) + Nudge (real-time signal)
type ChatHandler struct {
	messageIDPattern *regexp.Regexp
}

// NewChatHandler creates a new ChatHandler
func NewChatHandler() *ChatHandler {
	return &ChatHandler{
		messageIDPattern: regexp.MustCompile(`hq-[a-z0-9]+`),
	}
}

// RegisterRoutes registers the chat routes on the given mux
func (h *ChatHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/chat/conversations", h.ListConversations)
	mux.HandleFunc("GET /api/chat/{target}/history", h.GetHistory)
	mux.HandleFunc("POST /api/chat/{target}/send", h.SendMessage)
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
	Target      string       `json:"target"`      // e.g., "mayor/", "Chrote/jasper"
	DisplayName string       `json:"displayName"` // e.g., "Mayor", "Jasper"
	Role        string       `json:"role"`        // "mayor", "polecat", "witness", etc.
	Online      bool         `json:"online"`
	UnreadCount int          `json:"unreadCount"`
	LastMessage *ChatMessage `json:"lastMessage,omitempty"`
}

// SendChatRequest is the request body for sending a chat message
type SendChatRequest struct {
	Message string `json:"message"`
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
// Returns a list of available chat targets with their status
func (h *ChatHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	// Get status to find available recipients
	cmd := exec.Command("gt", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "COMMAND_FAILED", "Failed to get status")
		return
	}

	conversations := h.parseStatusForConversations(string(output))

	core.WriteSuccess(w, map[string]interface{}{
		"conversations": conversations,
	})
}

// parseStatusForConversations parses gt status output to build conversation list
func (h *ChatHandler) parseStatusForConversations(output string) []Conversation {
	conversations := []Conversation{}

	lines := strings.Split(output, "\n")
	currentRig := ""
	currentSection := ""

	rolePattern := regexp.MustCompile(`^(🎩|🐺|🦉|🏭)\s+(\w+)\s+([●○])`)
	memberPattern := regexp.MustCompile(`^\s+(\w+)\s+([●○])`)
	rigPattern := regexp.MustCompile(`^─── (\w+)/ ─`)

	for _, line := range lines {
		// Detect rig section
		if match := rigPattern.FindStringSubmatch(line); match != nil {
			currentRig = match[1]
			continue
		}

		// Detect sections
		if strings.Contains(line, "Crew") {
			currentSection = "crew"
			continue
		}
		if strings.Contains(line, "Polecats") {
			currentSection = "polecat"
			continue
		}

		// Parse main roles (mayor, deacon, witness, refinery)
		if match := rolePattern.FindStringSubmatch(line); match != nil {
			emoji := match[1]
			_ = match[2] // name - not used for main roles
			online := match[3] == "●"

			role := ""
			displayName := ""
			target := ""

			switch emoji {
			case "🎩":
				role = "mayor"
				displayName = "Mayor"
				target = "mayor/"
			case "🐺":
				role = "deacon"
				displayName = "Deacon"
				target = "deacon/"
			case "🦉":
				role = "witness"
				displayName = fmt.Sprintf("Witness (%s)", currentRig)
				target = fmt.Sprintf("%s/witness", currentRig)
			case "🏭":
				role = "refinery"
				displayName = fmt.Sprintf("Refinery (%s)", currentRig)
				target = fmt.Sprintf("%s/refinery", currentRig)
			}

			if role != "" {
				conversations = append(conversations, Conversation{
					Target:      target,
					DisplayName: displayName,
					Role:        role,
					Online:      online,
				})
			}
			// Reset rig-specific sections when we hit a non-rig role
			if emoji == "🎩" || emoji == "🐺" {
				currentRig = ""
			}
			continue
		}

		// Parse crew/polecat members
		if currentSection != "" && currentRig != "" {
			if match := memberPattern.FindStringSubmatch(line); match != nil {
				name := match[1]
				online := match[2] == "●"

				target := fmt.Sprintf("%s/%s", currentRig, name)
				displayName := strings.Title(name)
				if currentSection == "polecat" {
					displayName = fmt.Sprintf("🐱 %s", strings.Title(name))
				} else {
					displayName = fmt.Sprintf("👷 %s", strings.Title(name))
				}

				conversations = append(conversations, Conversation{
					Target:      target,
					DisplayName: displayName,
					Role:        currentSection,
					Online:      online,
				})
			}
		}
	}

	return conversations
}

// GetHistory handles GET /api/chat/{target}/history
// Returns message history for a specific conversation from beads
func (h *ChatHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("target")
	if target == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Target is required")
		return
	}

	// URL decode the target (e.g., "mayor%2F" -> "mayor/")
	target = strings.ReplaceAll(target, "%2F", "/")

	messages := []ChatMessage{}

	// Get messages FROM the target (including archived for full history)
	fromCmd := exec.CommandContext(
		context.Background(),
		"gt", "mail", "search", "", "--from", target, "--json", "--archive",
	)
	fromCmd.Env = core.GetTmuxEnv()
	fromOutput, _ := fromCmd.Output()

	// Parse messages from target
	if len(fromOutput) > 0 {
		fromMessages := h.parseMailJSON(fromOutput, "agent", target)
		messages = append(messages, fromMessages...)
	}

	// Get messages TO the target (our sent messages)
	// Query beads assigned to the target (messages we sent)
	toCmd := exec.CommandContext(
		context.Background(),
		"bd", "list", "--assignee", target, "--label", "message", "--json", "--all",
	)
	toCmd.Env = core.GetTmuxEnv()
	toOutput, _ := toCmd.Output()

	// Parse messages to target (our sent messages)
	if len(toOutput) > 0 {
		toMessages := h.parseBeadsJSON(toOutput, "user")
		messages = append(messages, toMessages...)
	}

	// Sort by timestamp (oldest first for chat display)
	h.sortMessagesByTime(messages)

	core.WriteSuccess(w, map[string]interface{}{
		"target":   target,
		"messages": messages,
	})
}

// MailSearchResult represents a message from gt mail search --json
type MailSearchResult struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Timestamp string `json:"timestamp"`
	Read      bool   `json:"read"`
	ThreadID  string `json:"thread_id"`
}

// parseMailJSON parses JSON output from gt mail search
func (h *ChatHandler) parseMailJSON(output []byte, role string, target string) []ChatMessage {
	messages := []ChatMessage{}

	// Try to parse as JSON array
	var results []MailSearchResult
	if err := json.Unmarshal(output, &results); err != nil {
		// Try parsing line by line as JSONL
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "{") {
				continue
			}
			var result MailSearchResult
			if err := json.Unmarshal([]byte(line), &result); err == nil {
				results = append(results, result)
			}
		}
	}

	for _, r := range results {
		// Determine role based on sender
		msgRole := role
		if strings.Contains(r.From, "crew") || strings.Contains(r.From, "human") || r.From == "" {
			msgRole = "user"
		}

		// Use body if available, otherwise subject
		content := r.Body
		if content == "" {
			content = r.Subject
		}

		// Parse timestamp
		var ts time.Time
		if r.Timestamp != "" {
			ts, _ = time.Parse(time.RFC3339, r.Timestamp)
		}
		if ts.IsZero() {
			ts = time.Now()
		}

		messages = append(messages, ChatMessage{
			ID:        r.ID,
			Role:      msgRole,
			From:      r.From,
			To:        r.To,
			Content:   content,
			Timestamp: ts,
			Read:      r.Read,
		})
	}

	return messages
}

// BeadResult represents an issue from bd list --json
type BeadResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
	Assignee    string `json:"assignee"`
	CreatedAt   string `json:"created_at"`
	Status      string `json:"status"`
}

// parseBeadsJSON parses JSON output from bd list
func (h *ChatHandler) parseBeadsJSON(output []byte, role string) []ChatMessage {
	messages := []ChatMessage{}

	// Try to parse as JSON array
	var results []BeadResult
	if err := json.Unmarshal(output, &results); err != nil {
		// Try parsing line by line as JSONL
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "{") {
				continue
			}
			var result BeadResult
			if err := json.Unmarshal([]byte(line), &result); err == nil {
				results = append(results, result)
			}
		}
	}

	for _, r := range results {
		// Use description if available, otherwise title
		content := r.Description
		if content == "" {
			content = r.Title
		}

		// Parse timestamp
		var ts time.Time
		if r.CreatedAt != "" {
			ts, _ = time.Parse(time.RFC3339, r.CreatedAt)
		}
		if ts.IsZero() {
			ts = time.Now()
		}

		messages = append(messages, ChatMessage{
			ID:        r.ID,
			Role:      role,
			From:      r.Owner,
			To:        r.Assignee,
			Content:   content,
			Timestamp: ts,
			Read:      r.Status == "closed",
		})
	}

	return messages
}

// sortMessagesByTime sorts messages oldest first (for chat display)
func (h *ChatHandler) sortMessagesByTime(messages []ChatMessage) {
	for i := 0; i < len(messages)-1; i++ {
		for j := i + 1; j < len(messages); j++ {
			if messages[i].Timestamp.After(messages[j].Timestamp) {
				messages[i], messages[j] = messages[j], messages[i]
			}
		}
	}
}

// SendMessage handles POST /api/chat/{target}/send
// Implements dual-channel delivery: Mail + Nudge
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("target")
	if target == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Target is required")
		return
	}

	// URL decode the target
	target = strings.ReplaceAll(target, "%2F", "/")

	var req SendChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
		return
	}

	if req.Message == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Message is required")
		return
	}

	response := SendChatResponse{
		Success: true,
	}

	// Channel A: MAIL TRAIN (Persistence)
	// Send via gt mail for durable storage
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mailCmd := exec.CommandContext(ctx, "gt", "mail", "send", target, "-m", req.Message)
	mailCmd.Env = core.GetTmuxEnv()

	if mailOutput, err := mailCmd.CombinedOutput(); err != nil {
		response.MailSent = false
		response.Error = fmt.Sprintf("Mail send failed: %s", strings.TrimSpace(string(mailOutput)))
	} else {
		response.MailSent = true
		// Try to extract message ID from output
		if id := h.messageIDPattern.FindString(string(mailOutput)); id != "" {
			response.MessageID = id
		}
	}

	// Channel B: NUDGE (Real-time Signal)
	// Wake the agent to check their mail
	nudgeMsg := fmt.Sprintf("📬 New message (Check Mail)")
	nudgeCmd := exec.CommandContext(ctx, "gt", "nudge", target, "-m", nudgeMsg)
	nudgeCmd.Env = core.GetTmuxEnv()

	if err := nudgeCmd.Run(); err != nil {
		response.Nudged = false
		// Nudge failure is not critical - mail was still sent
	} else {
		response.Nudged = true
	}

	// Overall success if mail was sent (nudge is best-effort)
	response.Success = response.MailSent

	if response.Success {
		core.WriteSuccess(w, response)
	} else {
		core.WriteError(w, http.StatusInternalServerError, "SEND_FAILED", response.Error)
	}
}
