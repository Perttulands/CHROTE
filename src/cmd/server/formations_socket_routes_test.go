package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFormationsSocketRoutesRegisteredWhenSocketSet proves that when
// CHROTE_FORMATIONS_TMUX_SOCKET is configured, the socket-scoped session API and
// the second terminal proxy are wired. This is the side-panel backend the
// Phase 4 UI calls; if these routes were missing the panel would 404.
func TestFormationsSocketRoutesRegisteredWhenSocketSet(t *testing.T) {
	t.Setenv("CHROTE_FORMATIONS_TMUX_SOCKET", "/run/user/1000/chrote-formations-tmux/default")

	mux := http.NewServeMux()
	_, formationsProxy := registerRuntimeRoutes(mux, Config{TtydPort: 1, FormationsTtydPort: 2})
	registerAPIFallback(mux)

	if formationsProxy == nil {
		t.Fatal("formations terminal proxy is nil even though the socket is configured")
	}

	// Session API route must resolve to a registered handler (not the API 404
	// fallback). The handler is socket-scoped to the SAME path the executor uses.
	req := httptest.NewRequest(http.MethodGet, "/api/formations/tmux/sessions", nil)
	if h, pattern := mux.Handler(req); h == nil || pattern == "" || pattern == "/api/" {
		t.Fatalf("/api/formations/tmux/sessions not registered (pattern=%q)", pattern)
	}

	// Second ttyd route must be registered under /terminal-formations/.
	wsReq := httptest.NewRequest(http.MethodGet, "/terminal-formations/", nil)
	if h, pattern := mux.Handler(wsReq); h == nil || pattern == "" {
		t.Fatalf("/terminal-formations/ not registered (pattern=%q)", pattern)
	}
}

// TestFormationsSocketRoutesAbsentWhenSocketUnset locks the fail-loud default:
// with no socket configured we register NOTHING rather than silently pointing
// the side panel at the wrong (cockpit) socket.
func TestFormationsSocketRoutesAbsentWhenSocketUnset(t *testing.T) {
	t.Setenv("CHROTE_FORMATIONS_TMUX_SOCKET", "")

	mux := http.NewServeMux()
	_, formationsProxy := registerRuntimeRoutes(mux, Config{TtydPort: 1, FormationsTtydPort: 2})
	registerAPIFallback(mux)

	if formationsProxy != nil {
		t.Fatal("formations terminal proxy was constructed even though the socket is unset")
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/formations/tmux/sessions", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/api/formations/tmux/sessions status = %d, want %d when socket unset; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
