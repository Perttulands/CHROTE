package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormationsRoutesAreNotRegisteredWhenEnvFlagIsOff(t *testing.T) {
	for _, raw := range []string{"", "0", "false", "off", "no", "unexpected"} {
		t.Run("CHROTE_FORMATIONS="+raw, func(t *testing.T) {
			config := Config{TtydPort: 1, FormationsEnabled: formationsEnabled(raw)}
			mux := http.NewServeMux()
			registerRuntimeRoutes(mux, config)
			registerAPIFallback(mux)

			for _, path := range []string{"/api/formations/boards", "/api/agents"} {
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

				if rec.Code != http.StatusNotFound {
					t.Fatalf("%s status = %d, want %d when flag is off; body=%s", path, rec.Code, http.StatusNotFound, rec.Body.String())
				}
			}

			for _, request := range []struct {
				method string
				path   string
			}{
				{http.MethodPatch, "/api/formations/boards/session-search"},
				{http.MethodPost, "/api/formations/runs"},
				{http.MethodPost, "/api/formations/runs/run_test/resume"},
				{http.MethodPost, "/api/formations/runs/run_test/gates/gate_review/verdict"},
				{http.MethodGet, "/api/formations/runs/run_test/escalations"},
				{http.MethodPost, "/api/formations/boards/session-search/formations"},
			} {
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest(request.method, request.path, nil))
				if rec.Code != http.StatusNotFound {
					t.Fatalf("%s %s status = %d, want %d when flag is off; body=%s", request.method, request.path, rec.Code, http.StatusNotFound, rec.Body.String())
				}
			}
		})
	}
}

func TestFormationsRoutesRegisterOnlyWhenEnvFlagIsOn(t *testing.T) {
	for _, raw := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Run("CHROTE_FORMATIONS="+raw, func(t *testing.T) {
			config := Config{TtydPort: 1, FormationsEnabled: formationsEnabled(raw)}
			mux := http.NewServeMux()
			registerRuntimeRoutes(mux, config)
			registerAPIFallback(mux)

			for _, path := range []string{"/api/formations/boards", "/api/agents"} {
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

				if rec.Code != http.StatusOK {
					t.Fatalf("%s status = %d, want %d when flag is on; body=%s", path, rec.Code, http.StatusOK, rec.Body.String())
				}
			}

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", nil))
			if rec.Code == http.StatusNotFound {
				t.Fatalf("contracted S2 board PATCH route was not registered when flag is on; body=%s", rec.Body.String())
			}

			legacyPost := httptest.NewRecorder()
			mux.ServeHTTP(legacyPost, httptest.NewRequest(http.MethodPost, "/api/formations/boards/session-search/formations", nil))
			if legacyPost.Code != http.StatusNotFound {
				t.Fatalf("legacy S2 POST create route status = %d, want %d when flag is on; body=%s", legacyPost.Code, http.StatusNotFound, legacyPost.Body.String())
			}

			runStart := httptest.NewRecorder()
			mux.ServeHTTP(runStart, httptest.NewRequest(http.MethodPost, "/api/formations/runs", nil))
			if runStart.Code == http.StatusNotFound {
				t.Fatalf("S4 run start route was not registered when flag is on; body=%s", runStart.Body.String())
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
					t.Fatalf("%s route was not registered when flag is on; body=%s", request.name, rec.Body.String())
				}
			}
		})
	}
}

func TestAPIFallbackKeepsUnknownAPIRoutesOutOfDashboard(t *testing.T) {
	mux := http.NewServeMux()
	registerRuntimeRoutes(mux, Config{TtydPort: 1})
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
