package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installFailingTmux(t *testing.T, stderr string) {
	t.Helper()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$TMUX_STDERR\" >&2\nexit 1\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake failing tmux command: %v", err)
	}
	t.Setenv("TMUX_STDERR", stderr)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestTmuxHandler_ListSessionsTreatsKnownNoServerErrorsAsEmptyList(t *testing.T) {
	tests := []string{
		"no server running on /tmp/tmux-2001/default",
		"error connecting to /run/user/2001/tmux/default (No such file or directory)",
	}

	for _, stderr := range tests {
		t.Run(stderr, func(t *testing.T) {
			installFailingTmux(t, stderr)
			handler := NewTmuxHandler()
			req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
			rec := httptest.NewRecorder()

			handler.ListSessions(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			var response SessionsResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(response.Sessions) != 0 {
				t.Fatalf("sessions = %+v, want empty list", response.Sessions)
			}
			if response.Error != "" {
				t.Fatalf("error = %q, want no user-visible error for no-server condition", response.Error)
			}
		})
	}
}

func TestTmuxHandler_ListSessionsReportsBareNoSuchFileAsNonAuthoritative(t *testing.T) {
	installFailingTmux(t, "No such file or directory")
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Error, "No such file or directory") {
		t.Fatalf("error = %q, want bare OS failure to remain non-authoritative", response.Error)
	}
	if len(response.Sources) != 1 || response.Sources[0].Status != tmuxSourceFailed {
		t.Fatalf("sources = %+v, want failed source", response.Sources)
	}
}

func TestTmuxHandler_ListSessionsReportsPermissionDeniedConnectionErrors(t *testing.T) {
	installFailingTmux(t, "error connecting to /run/user/2001/chrote-tmux/default (Permission denied)")
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(response.Error, "Permission denied") {
		t.Fatalf("error = %q, want tmux permission/connectivity error to fail loud", response.Error)
	}
}

func TestTmuxHandler_ListSessionsReportsUnknownConnectionErrors(t *testing.T) {
	installFailingTmux(t, "error connecting to /run/user/2001/chrote-tmux/default")
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(response.Error, "error connecting") {
		t.Fatalf("error = %q, want unknown tmux connection error to fail loud", response.Error)
	}
}

func TestTmuxHandler_ListSessionsCurrentlyReportsServerExitedUnexpectedly(t *testing.T) {
	installFailingTmux(t, "server exited unexpectedly")
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(response.Error, "server exited unexpectedly") {
		t.Fatalf("error = %q, want current tmux handler to report server exited unexpectedly", response.Error)
	}
}

func TestOracleHandler_GetAgentSessionsCurrentlyTreatsServerExitedUnexpectedlyAsNoServer(t *testing.T) {
	installFailingTmux(t, "server exited unexpectedly")
	handler := NewOracleHandler(NewTmuxHandler(), NewBeadsHandler())
	defer handler.Stop()

	sessions, err := handler.getAgentSessions()

	if err != nil {
		t.Fatalf("getAgentSessions error = %v, want nil for current Oracle no-server allowlist", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %+v, want empty list", sessions)
	}
}
