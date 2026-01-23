// chat_e2e_test.go - E2E tests for ChroteChat
// Run with: go test -v ./internal/api/ -run TestChatE2E -tags=e2e

//go:build e2e

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrote/server/internal/core"
)

// TestChatE2E_WorkspaceDetection tests that we can detect Gastown workspaces
// from tmux session paths by walking up to find the daemon/ directory.
func TestChatE2E_WorkspaceDetection(t *testing.T) {
	// Skip if no tmux sessions
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{session_path}")
	cmd.Env = core.GetTmuxEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("No tmux sessions available: %v", err)
	}

	t.Logf("Tmux sessions:\n%s", output)

	// Parse and check each session
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	workspacesFound := make(map[string]bool)

	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			t.Logf("Skipping malformed line: %q", line)
			continue
		}
		sessionName := parts[0]
		sessionPath := parts[1]

		workspace := findGastownWorkspace(sessionPath)
		t.Logf("Session %q at %q -> workspace %q", sessionName, sessionPath, workspace)

		if workspace != "" {
			workspacesFound[workspace] = true

			// Verify daemon/ exists
			daemonPath := filepath.Join(workspace, "daemon")
			if _, err := os.Stat(daemonPath); os.IsNotExist(err) {
				t.Errorf("Workspace %q claims daemon/ exists but it doesn't", workspace)
			}
		}
	}

	t.Logf("Found %d unique workspaces: %v", len(workspacesFound), workspacesFound)
}

// TestChatE2E_ConversationsIncludeWorkspace tests that the conversations API
// returns workspace paths for sessions that are in Gastown workspaces.
func TestChatE2E_ConversationsIncludeWorkspace(t *testing.T) {
	h := NewChatHandler()

	req := httptest.NewRequest("GET", "/api/chat/conversations", nil)
	w := httptest.NewRecorder()

	h.ListConversations(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ListConversations failed: status %d", resp.StatusCode)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Conversations []Conversation `json:"conversations"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !result.Success {
		t.Fatal("Response success=false")
	}

	t.Logf("Got %d conversations", len(result.Data.Conversations))

	conversationsWithWorkspace := 0
	for _, c := range result.Data.Conversations {
		t.Logf("  %s (%s): workspace=%q", c.DisplayName, c.Target, c.Workspace)
		if c.Workspace != "" {
			conversationsWithWorkspace++

			// Verify workspace is valid
			daemonPath := filepath.Join(c.Workspace, "daemon")
			if _, err := os.Stat(daemonPath); os.IsNotExist(err) {
				t.Errorf("Conversation %q has invalid workspace %q (no daemon/)", c.Target, c.Workspace)
			}
		}
	}

	if len(result.Data.Conversations) > 0 && conversationsWithWorkspace == 0 {
		t.Error("No conversations have workspace set - chat messaging will be disabled")
	}
}

// TestChatE2E_SendMessageValidation tests that SendMessage validates inputs correctly.
func TestChatE2E_SendMessageValidation(t *testing.T) {
	h := NewChatHandler()

	// Find a valid workspace first
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_path}")
	cmd.Env = core.GetTmuxEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("No tmux sessions: %v", err)
	}

	var validWorkspace string
	for _, path := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		ws := findGastownWorkspace(path)
		if ws != "" {
			validWorkspace = ws
			break
		}
	}

	if validWorkspace == "" {
		t.Skip("No Gastown workspace found")
	}

	t.Logf("Using workspace: %s", validWorkspace)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "Valid request structure",
			body:       `{"workspace": "` + validWorkspace + `", "target": "mayor", "message": "test"}`,
			wantStatus: http.StatusOK, // May fail at gt command level but passes validation
		},
		{
			name:       "Missing workspace",
			body:       `{"target": "mayor", "message": "test"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_WORKSPACE",
		},
		{
			name:       "Invalid workspace path",
			body:       `{"workspace": "/nonexistent/path", "target": "mayor", "message": "test"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_WORKSPACE",
		},
		{
			name:       "Missing target",
			body:       `{"workspace": "` + validWorkspace + `", "message": "test"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_TARGET",
		},
		{
			name:       "Empty message",
			body:       `{"workspace": "` + validWorkspace + `", "target": "mayor", "message": ""}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "EMPTY_MESSAGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/chat/send", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.SendMessage(w, req)

			resp := w.Result()
			t.Logf("Response status: %d, body: %s", resp.StatusCode, w.Body.String())

			if tt.wantStatus != 0 && resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if tt.wantCode != "" {
				var result struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				json.NewDecoder(strings.NewReader(w.Body.String())).Decode(&result)
				if result.Error.Code != tt.wantCode {
					t.Errorf("error code = %q, want %q", result.Error.Code, tt.wantCode)
				}
			}
		})
	}
}

// TestChatE2E_FullFlow simulates the full chat flow from frontend perspective.
// This test requires an active Gastown workspace with gt command available.
func TestChatE2E_FullFlow(t *testing.T) {
	h := NewChatHandler()

	// Step 1: Get conversations
	t.Log("Step 1: Fetching conversations...")
	req1 := httptest.NewRequest("GET", "/api/chat/conversations", nil)
	w1 := httptest.NewRecorder()
	h.ListConversations(w1, req1)

	var convosResult struct {
		Success bool `json:"success"`
		Data    struct {
			Conversations []Conversation `json:"conversations"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w1.Body).Decode(&convosResult); err != nil {
		t.Fatalf("Failed to decode conversations: %v", err)
	}

	if len(convosResult.Data.Conversations) == 0 {
		t.Skip("No conversations available")
	}

	// Step 2: Find a conversation with a workspace
	var targetConvo *Conversation
	for i := range convosResult.Data.Conversations {
		c := &convosResult.Data.Conversations[i]
		t.Logf("  Conversation: %s, workspace: %q", c.DisplayName, c.Workspace)
		if c.Workspace != "" && targetConvo == nil {
			targetConvo = c
		}
	}

	if targetConvo == nil {
		t.Fatal("No conversation has a workspace - this is the bug we're trying to fix!")
	}

	t.Logf("Step 2: Selected conversation: %s (target=%s, workspace=%s)",
		targetConvo.DisplayName, targetConvo.Target, targetConvo.Workspace)

	// Step 3: Send a test message (dry run - we'll check the response)
	t.Log("Step 3: Sending test message...")
	sendBody := `{"workspace": "` + targetConvo.Workspace + `", "target": "` + targetConvo.Target + `", "message": "E2E test message"}`
	req3 := httptest.NewRequest("POST", "/api/chat/send", strings.NewReader(sendBody))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	h.SendMessage(w3, req3)

	t.Logf("Send response: %s", w3.Body.String())

	var sendResult struct {
		Success bool `json:"success"`
		Data    struct {
			Success   bool   `json:"success"`
			MessageID string `json:"messageId"`
			MailSent  bool   `json:"mailSent"`
			Nudged    bool   `json:"nudged"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(strings.NewReader(w3.Body.String())).Decode(&sendResult); err != nil {
		t.Fatalf("Failed to decode send response: %v", err)
	}

	if !sendResult.Success {
		t.Errorf("Send failed: %s - %s", sendResult.Error.Code, sendResult.Error.Message)
	} else {
		t.Logf("Send succeeded: mailSent=%v, nudged=%v, messageId=%s",
			sendResult.Data.MailSent, sendResult.Data.Nudged, sendResult.Data.MessageID)
	}
}
