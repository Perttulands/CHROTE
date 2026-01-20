// Package api provides HTTP handlers for the API
package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// MailHandler handles mail-related API endpoints
type MailHandler struct{}

// NewMailHandler creates a new MailHandler
func NewMailHandler() *MailHandler {
	return &MailHandler{}
}

// RegisterRoutes registers the mail routes on the given mux
func (h *MailHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/mail/inbox", h.GetInbox)
	mux.HandleFunc("GET /api/mail/recipients", h.GetRecipients)
	mux.HandleFunc("GET /api/mail/status", h.GetStatus)
	mux.HandleFunc("POST /api/mail/send", h.SendMail)
	mux.HandleFunc("POST /api/mail/read/{id}", h.MarkAsRead)
}

// MailMessage represents a mail message
type MailMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Timestamp string `json:"timestamp"`
	Read      bool   `json:"read"`
}

// MailRecipient represents a mail recipient
type MailRecipient struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Path   string `json:"path"`
	Online bool   `json:"online"`
}

// TownStatus represents the town status
type TownStatus struct {
	Name     string `json:"name"`
	Overseer string `json:"overseer"`
}

// SendMailRequest is the request body for sending mail
type SendMailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// GetInbox handles GET /api/mail/inbox
func (h *MailHandler) GetInbox(w http.ResponseWriter, r *http.Request) {
	// Run gt mail inbox command
	cmd := exec.Command("gt", "mail", "inbox")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If command fails, return empty inbox
		core.WriteSuccess(w, map[string]interface{}{
			"messages": []MailMessage{},
		})
		return
	}

	messages := parseInboxOutput(string(output))
	core.WriteSuccess(w, map[string]interface{}{
		"messages": messages,
	})
}

// parseInboxOutput parses the output of gt mail inbox
func parseInboxOutput(output string) []MailMessage {
	messages := []MailMessage{}

	// Parse the inbox output format
	// Format varies, but typically shows message ID, from, subject
	lines := strings.Split(output, "\n")

	// Skip header lines
	inMessages := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Look for message entries
		if strings.HasPrefix(line, "(no messages)") {
			break
		}

		// Skip header line
		if strings.Contains(line, "Inbox:") {
			inMessages = true
			continue
		}

		if inMessages && line != "" && !strings.HasPrefix(line, "(") {
			// Try to parse message line
			// Format: ID | From | Subject | Date
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				msg := MailMessage{
					ID:        strings.TrimSpace(parts[0]),
					From:      strings.TrimSpace(parts[1]),
					Subject:   strings.TrimSpace(parts[2]),
					Body:      "",
					Timestamp: time.Now().Format(time.RFC3339),
					Read:      false,
				}
				if len(parts) >= 4 {
					msg.Timestamp = strings.TrimSpace(parts[3])
				}
				messages = append(messages, msg)
			}
		}
	}

	return messages
}

// GetRecipients handles GET /api/mail/recipients
func (h *MailHandler) GetRecipients(w http.ResponseWriter, r *http.Request) {
	// Run gt status to get available recipients
	cmd := exec.Command("gt", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "COMMAND_FAILED", "Failed to get status")
		return
	}

	recipients := parseStatusForRecipients(string(output))

	// Always add human option
	recipients = append(recipients, MailRecipient{
		ID:     "human",
		Name:   "Human Overseer",
		Role:   "human",
		Path:   "--human",
		Online: true,
	})

	core.WriteSuccess(w, map[string]interface{}{
		"recipients": recipients,
	})
}

// parseStatusForRecipients parses gt status output to extract recipients
func parseStatusForRecipients(output string) []MailRecipient {
	recipients := []MailRecipient{}

	lines := strings.Split(output, "\n")
	currentSection := ""

	// Regex patterns for parsing
	rolePattern := regexp.MustCompile(`^(🎩|🐺|🦉|🏭)\s+(\w+)\s+([●○])`)
	memberPattern := regexp.MustCompile(`^\s+(\w+)\s+([●○])`)

	for _, line := range lines {
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
			name := match[2]
			online := match[3] == "●"

			role := ""
			switch emoji {
			case "🎩":
				role = "mayor"
			case "🐺":
				role = "deacon"
			case "🦉":
				role = "witness"
			case "🏭":
				role = "refinery"
			}

			if role != "" {
				recipients = append(recipients, MailRecipient{
					ID:     name,
					Name:   name,
					Role:   role,
					Path:   role + "/",
					Online: online,
				})
			}
			continue
		}

		// Parse crew/polecat members
		if currentSection != "" {
			if match := memberPattern.FindStringSubmatch(line); match != nil {
				name := match[1]
				online := match[2] == "●"

				recipients = append(recipients, MailRecipient{
					ID:     fmt.Sprintf("%s-%s", currentSection, name),
					Name:   name,
					Role:   currentSection,
					Path:   fmt.Sprintf("%s/%s", currentSection, name),
					Online: online,
				})
			}
		}
	}

	return recipients
}

// GetStatus handles GET /api/mail/status
func (h *MailHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("gt", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "COMMAND_FAILED", "Failed to get status")
		return
	}

	status := parseTownStatus(string(output))
	core.WriteSuccess(w, status)
}

// parseTownStatus parses gt status output for town info
func parseTownStatus(output string) TownStatus {
	status := TownStatus{}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Town:") {
			status.Name = strings.TrimSpace(strings.TrimPrefix(line, "Town:"))
		}
		if strings.Contains(line, "Overseer:") {
			parts := strings.Split(line, "Overseer:")
			if len(parts) > 1 {
				status.Overseer = strings.TrimSpace(parts[1])
			}
		}
	}

	return status
}

// SendMail handles POST /api/mail/send
func (h *MailHandler) SendMail(w http.ResponseWriter, r *http.Request) {
	var req SendMailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
		return
	}

	if req.To == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Recipient is required")
		return
	}
	if req.Subject == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Subject is required")
		return
	}
	if req.Body == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Message body is required")
		return
	}

	// Build gt mail send command
	args := []string{"mail", "send", req.To, "-s", req.Subject, "-m", req.Body}

	// Handle --human flag
	if req.To == "--human" {
		args = []string{"mail", "send", "--human", "-s", req.Subject, "-m", req.Body}
	}

	cmd := exec.Command("gt", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "SEND_FAILED",
			fmt.Sprintf("Failed to send message: %s", strings.TrimSpace(string(output))))
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"messageId": fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		"status":    "sent",
	})
}

// MarkAsRead handles POST /api/mail/read/{id}
func (h *MailHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Message ID is required")
		return
	}

	// Run gt mail read command
	cmd := exec.Command("gt", "mail", "read", id)
	_, err := cmd.CombinedOutput()
	if err != nil {
		// Silently succeed even if marking fails
	}

	core.WriteSuccess(w, map[string]interface{}{
		"id":   id,
		"read": true,
	})
}

// Helper to run gt command and capture output line by line
func runGtCommand(args ...string) ([]string, error) {
	cmd := exec.Command("gt", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var lines []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := cmd.Wait(); err != nil {
		return lines, err
	}

	return lines, nil
}
