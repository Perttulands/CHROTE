package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler_Health(t *testing.T) {
	handler := NewHealthHandler()

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
}

func TestHealthHandler_VersionReportsSourceCommit(t *testing.T) {
	// Provenance (home-altx): the build-time source commit must be observable
	// over HTTP so the running binary is traceable to its CHROTE source.
	handler := NewHealthHandlerWithBuild("0.2.0", "ee585b1")

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	recorder := httptest.NewRecorder()
	handler.Version(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Status code = %d, expected %d", recorder.Code, http.StatusOK)
	}
	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if response["version"] != "0.2.0" {
		t.Errorf("version = %q, expected 0.2.0", response["version"])
	}
	if response["commit"] != "ee585b1" {
		t.Errorf("commit = %q, expected source commit ee585b1", response["commit"])
	}
}

func TestHealthHandler_VersionDefaultsCommitToUnknown(t *testing.T) {
	handler := NewHealthHandlerWithVersion("0.2.0")
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	recorder := httptest.NewRecorder()
	handler.Version(recorder, req)

	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if response["commit"] != "unknown" {
		t.Errorf("commit = %q, expected 'unknown' when not stamped", response["commit"])
	}
}

func TestHealthHandler_RegisterRoutes(t *testing.T) {
	handler := NewHealthHandler()
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
