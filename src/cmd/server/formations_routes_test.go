package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Formations is always-on: the agents and formations routes register
// unconditionally now that the CHROTE_FORMATIONS feature gate has been retired.
// The executor safety ladder (CHROTE_FORMATIONS_LAB_*/TMUX_*/PROD_SMOKE) is a
// separate boundary that still governs execution promotion, not API availability.
func TestFormationsRoutesAreAlwaysRegistered(t *testing.T) {
	mux := http.NewServeMux()
	registerRuntimeRoutes(mux, Config{TtydPort: 1}, context.Background())
	registerAPIFallback(mux)

	for _, path := range []string{"/api/formations/boards", "/api/agents"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d; body=%s", path, rec.Code, http.StatusOK, rec.Body.String())
		}
	}

	boardPatch := httptest.NewRecorder()
	mux.ServeHTTP(boardPatch, httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", nil))
	if boardPatch.Code == http.StatusNotFound {
		t.Fatalf("contracted S2 board PATCH route was not registered; body=%s", boardPatch.Body.String())
	}

	legacyPost := httptest.NewRecorder()
	mux.ServeHTTP(legacyPost, httptest.NewRequest(http.MethodPost, "/api/formations/boards/session-search/formations", nil))
	if legacyPost.Code != http.StatusNotFound {
		t.Fatalf("legacy S2 POST create route status = %d, want %d; body=%s", legacyPost.Code, http.StatusNotFound, legacyPost.Body.String())
	}

	runStart := httptest.NewRecorder()
	mux.ServeHTTP(runStart, httptest.NewRequest(http.MethodPost, "/api/formations/runs", nil))
	if runStart.Code == http.StatusNotFound {
		t.Fatalf("S4 run start route was not registered; body=%s", runStart.Body.String())
	}

	for _, request := range []struct {
		method string
		path   string
		name   string
	}{
		{http.MethodPost, "/api/formations/runs/run_test/resume", "S5 run resume"},
		{http.MethodPost, "/api/formations/runs/run_test/gates/gate_review/verdict", "S5 gate verdict"},
		{http.MethodGet, "/api/formations/runs/run_test/escalations", "S5 run escalations"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(request.method, request.path, nil))
		if rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), "Unknown API endpoint") {
			t.Fatalf("%s route was not registered; body=%s", request.name, rec.Body.String())
		}
	}
}

func TestScheduledTaskRoutesAreRegistered(t *testing.T) {
	t.Setenv("CHROTE_SCHEDULED_TASKS_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerRuntimeRoutes(mux, Config{TtydPort: 1}, context.Background())
	registerAPIFallback(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scheduled-tasks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scheduled tasks list status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"success":true`) || !strings.Contains(rec.Body.String(), `"tasks"`) {
		t.Fatalf("scheduled tasks response = %s, want success/data envelope with tasks", rec.Body.String())
	}
}

func TestAPIFallbackKeepsUnknownAPIRoutesOutOfDashboard(t *testing.T) {
	mux := http.NewServeMux()
	registerRuntimeRoutes(mux, Config{TtydPort: 1}, context.Background())
	registerAPIFallback(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/not-a-dashboard-route", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestFormationsStoreDeleteDoesNotTouchWorkspace(t *testing.T) {
	workspace := t.TempDir()
	formationsDir := filepath.Join(workspace, ".formations")
	sentinels := []string{
		filepath.Join(workspace, "src", "index.html"),
		filepath.Join(workspace, "sessions", "live-session.marker"),
		filepath.Join(workspace, ".beads", "tasks.db"),
	}
	for _, path := range append([]string{filepath.Join(formationsDir, "boards", "career-web.formation.toml")}, sentinels...) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("sentinel"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if err := os.RemoveAll(formationsDir); err != nil {
		t.Fatalf("remove .formations: %v", err)
	}
	if _, err := os.Stat(formationsDir); !os.IsNotExist(err) {
		t.Fatalf(".formations stat err = %v, want removed directory", err)
	}
	for _, path := range sentinels {
		if raw, err := os.ReadFile(path); err != nil || string(raw) != "sentinel" {
			t.Fatalf("sentinel %s raw=%q err=%v, want untouched", path, string(raw), err)
		}
	}
}
