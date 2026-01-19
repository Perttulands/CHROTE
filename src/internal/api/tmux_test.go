package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTmuxHandler_NewTmuxHandler(t *testing.T) {
	handler := NewTmuxHandler()

	if handler == nil {
		t.Fatal("NewTmuxHandler() returned nil")
	}
	if handler.cache == nil {
		t.Error("Handler cache is nil")
	}
	if handler.colorRegex == nil {
		t.Error("Handler colorRegex is nil")
	}
}

func TestTmuxHandler_ValidateColor(t *testing.T) {
	handler := NewTmuxHandler()

	tests := []struct {
		name    string
		color   string
		isValid bool
	}{
		{"hex 3 digit", "#fff", true},
		{"hex 6 digit", "#ff00ff", true},
		{"named color", "red", true},
		{"named color blue", "blue", true},
		{"default", "default", true},
		{"invalid hex", "#gggggg", false},
		{"invalid chars", "red@blue", false},
		{"empty is not matched by regex but handled separately", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.colorRegex.MatchString(tt.color)
			if result != tt.isValid {
				t.Errorf("colorRegex.MatchString(%q) = %v, expected %v", tt.color, result, tt.isValid)
			}
		})
	}
}

func TestTmuxHandler_CreateSession_InvalidJSON(t *testing.T) {
	handler := NewTmuxHandler()

	// Test with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBufferString("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_CreateSession_InvalidName(t *testing.T) {
	handler := NewTmuxHandler()

	body := CreateSessionRequest{Name: "invalid name with spaces"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}

	var response map[string]interface{}
	json.Unmarshal(recorder.Body.Bytes(), &response)

	if response["success"] != false {
		t.Error("Response should indicate failure")
	}
}

func TestTmuxHandler_DeleteSession_InvalidName(t *testing.T) {
	handler := NewTmuxHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/invalid@name", nil)
	req.SetPathValue("name", "invalid@name")
	recorder := httptest.NewRecorder()

	handler.DeleteSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_DeleteAllSessions_NoConfirmHeader(t *testing.T) {
	handler := NewTmuxHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all", nil)
	// Intentionally NOT setting X-Nuke-Confirm header
	recorder := httptest.NewRecorder()

	handler.DeleteAllSessions(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("Status code = %d, expected %d (Forbidden)", recorder.Code, http.StatusForbidden)
	}

	var response map[string]interface{}
	json.Unmarshal(recorder.Body.Bytes(), &response)

	if response["success"] != false {
		t.Error("Response should indicate failure")
	}
}

func TestTmuxHandler_RenameSession_InvalidNewName(t *testing.T) {
	handler := NewTmuxHandler()

	body := RenameSessionRequest{NewName: "invalid name!"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/oldsession", bytes.NewBuffer(bodyBytes))
	req.SetPathValue("name", "oldsession")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.RenameSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_ApplyAppearance_InvalidColor(t *testing.T) {
	handler := NewTmuxHandler()

	body := AppearanceRequest{
		StatusBg: "invalidcolor@#$",
		StatusFg: "red",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tmux/appearance", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ApplyAppearance(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_RegisterRoutes(t *testing.T) {
	handler := NewTmuxHandler()
	mux := http.NewServeMux()

	// This should not panic
	handler.RegisterRoutes(mux)
}

func TestTmuxHandler_ListSessions_ReturnsValidJSON(t *testing.T) {
	handler := NewTmuxHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()

	handler.ListSessions(recorder, req)

	// Should return valid JSON even if tmux isn't running
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	// Should have a timestamp
	if response.Timestamp == "" {
		t.Error("Response should include timestamp")
	}

	// Sessions should be initialized (not nil)
	if response.Sessions == nil {
		t.Error("Sessions should be initialized slice, not nil")
	}

	// Grouped should be initialized (not nil)
	if response.Grouped == nil {
		t.Error("Grouped should be initialized map, not nil")
	}
}
