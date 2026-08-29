package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler_Health(t *testing.T) {
	handler := NewHealthHandlerWithBuildInfo("2.0.0-alpha.2-dev", "")

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recorder := httptest.NewRecorder()

	handler.Health(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("status = %q, expected 'ok'", response["status"])
	}

	if _, ok := response["timestamp"]; !ok {
		t.Error("Response should include timestamp")
	}

	// Unstamped builds must still serve the key, as an empty string.
	if commit, ok := response["commit"]; !ok || commit != "" {
		t.Errorf("commit = %v (present=%v), expected empty string present", commit, ok)
	}
}

func TestHealthHandler_Health_ReportsBuildCommit(t *testing.T) {
	handler := NewHealthHandlerWithBuildInfo("test-version", "abc123def456")

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recorder := httptest.NewRecorder()

	handler.Health(recorder, req)

	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["commit"] != "abc123def456" {
		t.Errorf("commit = %q, expected %q", response["commit"], "abc123def456")
	}
	if response["version"] != "test-version" {
		t.Errorf("version = %q, expected %q", response["version"], "test-version")
	}
}

func TestHealthHandler_RegisterRoutes(t *testing.T) {
	handler := NewHealthHandlerWithBuildInfo("2.0.0-alpha.2-dev", "")
	mux := http.NewServeMux()

	// This should not panic
	handler.RegisterRoutes(mux)

	// Test the route is registered
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Route not registered correctly, got status %d", recorder.Code)
	}
}
