package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestIntegration_FullAPIRouting tests that all routes are properly registered
func TestIntegration_FullAPIRouting(t *testing.T) {
	mux := http.NewServeMux()

	// Register all handlers
	tmuxHandler := NewTmuxHandler()
	tmuxHandler.RegisterRoutes(mux)

	healthHandler := NewHealthHandler()
	healthHandler.RegisterRoutes(mux)

	beadsHandler := NewBeadsHandler()
	beadsHandler.RegisterRoutes(mux)

	filesHandler := NewFilesHandler()
	filesHandler.RegisterRoutes(mux)

	// Test health endpoint
	t.Run("GET /api/health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}

		var response map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &response)
		if response["status"] != "ok" {
			t.Errorf("Expected status ok, got %v", response["status"])
		}
	})

	// Test tmux sessions list endpoint
	t.Run("GET /api/tmux/sessions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Should return valid JSON even if tmux not available
		var response SessionsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Errorf("Invalid JSON response: %v", err)
		}

		if response.Timestamp == "" {
			t.Error("Response should have timestamp")
		}
	})

	// Test beads health endpoint
	t.Run("GET /api/beads/health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/beads/health", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Should return either 200 (installed) or 503 (not installed)
		if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected 200 or 503, got %d", rec.Code)
		}
	})

	// Test files root listing endpoint
	t.Run("GET /api/files/resources/", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/files/resources/", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}

		var response DirectoryResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Errorf("Invalid JSON response: %v", err)
		}

		if !response.IsDir {
			t.Error("Root should be a directory")
		}

		if len(response.Items) != 2 {
			t.Errorf("Expected 2 root items (code, vault), got %d", len(response.Items))
		}
	})
}

// TestIntegration_APIResponseFormat verifies all APIs return consistent JSON format
func TestIntegration_APIResponseFormat(t *testing.T) {
	mux := http.NewServeMux()

	healthHandler := NewHealthHandler()
	healthHandler.RegisterRoutes(mux)

	tmuxHandler := NewTmuxHandler()
	tmuxHandler.RegisterRoutes(mux)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/health"},
		{"GET", "/api/tmux/sessions"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", contentType)
			}

			// Should be valid JSON
			var result interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Errorf("Response is not valid JSON: %v", err)
			}
		})
	}
}

// TestIntegration_ErrorHandling verifies proper error responses
func TestIntegration_ErrorHandling(t *testing.T) {
	mux := http.NewServeMux()

	tmuxHandler := NewTmuxHandler()
	tmuxHandler.RegisterRoutes(mux)

	// Test invalid session name for delete
	t.Run("DELETE with invalid name", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/invalid@name", nil)
		req.SetPathValue("name", "invalid@name")
		rec := httptest.NewRecorder()

		tmuxHandler.DeleteSession(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", rec.Code)
		}

		var response map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &response)

		if response["success"] != false {
			t.Error("Expected success=false for error response")
		}
	})

	// Test nuke without confirmation header
	t.Run("DELETE all without confirmation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all", nil)
		rec := httptest.NewRecorder()

		tmuxHandler.DeleteAllSessions(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("Expected 403, got %d", rec.Code)
		}
	})
}
