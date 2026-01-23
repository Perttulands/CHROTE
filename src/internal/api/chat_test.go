package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSessionName(t *testing.T) {
	h := NewChatHandler()

	tests := []struct {
		name        string
		session     string
		wantTarget  string
		wantDisplay string
		wantRole    string
	}{
		// HQ sessions
		{"hq-mayor", "hq-mayor", "mayor", "🎩 Mayor", "mayor"},
		{"hq-deacon", "hq-deacon", "deacon", "🐺 Deacon", "deacon"},

		// Gastown rig sessions
		{"gt-Chrote-witness", "gt-Chrote-witness", "witness", "🦉 Witness", "witness"},
		{"gt-Chrote-refinery", "gt-Chrote-refinery", "refinery", "🏭 Refinery", "refinery"},
		{"gt-Chrote-crew-Ronja", "gt-Chrote-crew-Ronja", "Chrote/Ronja", "👷 Ronja", "crew"},
		{"gt-Chrote-crew-Jasper", "gt-Chrote-crew-Jasper", "Chrote/Jasper", "👷 Jasper", "crew"},

		// Polecat patterns
		{"gt-Chrote-polecat-1", "gt-Chrote-polecat-1", "gt-Chrote-polecat-1", "🐱 Polecat 1", "polecat"},
		{"gt-Chrote-pc-2", "gt-Chrote-pc-2", "gt-Chrote-pc-2", "🐱 Polecat 2", "polecat"},

		// Generic worker (fallback)
		{"gt-Chrote-jasper", "gt-Chrote-jasper", "Chrote/jasper", "🐱 Jasper", "polecat"},

		// Real sessions from user's system
		{"gt-boot", "gt-boot", "", "", ""},  // Boot session, not a chat target
		{"shell-mkob6g67", "shell-mkob6g67", "", "", ""},

		// Non-chat sessions (should return empty role)
		{"main", "main", "", "", ""},
		{"shell", "shell", "", "", ""},
		{"my-project", "my-project", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, displayName, role := h.parseSessionName(tt.session)

			if target != tt.wantTarget {
				t.Errorf("parseSessionName(%q) target = %q, want %q", tt.session, target, tt.wantTarget)
			}
			if displayName != tt.wantDisplay {
				t.Errorf("parseSessionName(%q) displayName = %q, want %q", tt.session, displayName, tt.wantDisplay)
			}
			if role != tt.wantRole {
				t.Errorf("parseSessionName(%q) role = %q, want %q", tt.session, role, tt.wantRole)
			}
		})
	}
}

// TestListConversations_Integration tests the full endpoint with real tmux
func TestListConversations_Integration(t *testing.T) {
	h := NewChatHandler()

	req := httptest.NewRequest("GET", "/api/chat/conversations", nil)
	w := httptest.NewRecorder()

	h.ListConversations(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ListConversations returned status %d, want 200", resp.StatusCode)
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
		t.Error("ListConversations returned success=false")
	}

	t.Logf("Found %d conversations:", len(result.Data.Conversations))
	for _, c := range result.Data.Conversations {
		t.Logf("  - %s (%s) target=%q online=%v", c.DisplayName, c.Role, c.Target, c.Online)
	}

	// Should find at least mayor and witness based on current sessions
	foundMayor := false
	foundWitness := false
	for _, c := range result.Data.Conversations {
		if c.Role == "mayor" {
			foundMayor = true
		}
		if c.Role == "witness" {
			foundWitness = true
		}
	}

	if !foundMayor {
		t.Error("Expected to find mayor in conversations")
	}
	if !foundWitness {
		t.Error("Expected to find witness in conversations")
	}
}

func TestSendMessage_Validation(t *testing.T) {
	h := NewChatHandler()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "Missing target",
			body:       `{"message": "hello"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_TARGET",
		},
		{
			name:       "Empty message",
			body:       `{"target": "mayor", "message": ""}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "EMPTY_MESSAGE",
		},
		{
			name:       "Invalid JSON",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/chat/send", strings.NewReader(tt.body))
			w := httptest.NewRecorder()

			h.SendMessage(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			// Parse error code
			var result struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			json.NewDecoder(resp.Body).Decode(&result)
			if result.Error.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", result.Error.Code, tt.wantCode)
			}
		})
	}
}

func TestGetHistory_QueryParams(t *testing.T) {
	h := NewChatHandler()

	// Test with query parameter (slash encoded as %2F)
	req := httptest.NewRequest("GET", "/api/chat/history?target=Chrote%2Fjasper", nil)
	w := httptest.NewRecorder()

	h.GetHistory(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Messages []ChatMessage `json:"messages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !result.Success {
		t.Error("Success = false")
	}
	// Currently returns empty list (mock)
	if len(result.Data.Messages) != 0 {
		t.Error("Expected empty history")
	}
}
