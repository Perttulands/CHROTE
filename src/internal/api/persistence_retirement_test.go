package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// CHROTE's tmux API inventories and operates sessions; it does not promise to
// supervise agent lifetime. Keeping the retired endpoint absent prevents the
// lock capability from quietly growing its host-control machinery back.
func TestTmuxRoutesDoNotExposeAgentPersistence(t *testing.T) {
	mux := http.NewServeMux()
	NewTmuxHandler().RegisterRoutes(mux)
	for _, route := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/tmux/sessions"},
		{method: http.MethodPost, path: "/api/tmux/sessions"},
		{method: http.MethodPatch, path: "/api/tmux/sessions/example"},
		{method: http.MethodDelete, path: "/api/tmux/sessions/example"},
	} {
		req := httptest.NewRequest(route.method, route.path, nil)
		_, pattern := mux.Handler(req)
		if pattern == "" {
			t.Fatalf("ordinary tmux route %s %s is not registered", route.method, route.path)
		}
	}

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/tmux/sessions/example/persistence", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s persistence endpoint status = %d, want 404", method, rec.Code)
		}
	}
}
