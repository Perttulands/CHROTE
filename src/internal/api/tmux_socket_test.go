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

// installRecordingTmux installs a fake `tmux` on PATH that appends every argv it
// receives (one space-joined line per invocation) to a recording file, then
// prints a single fake session so list parsing succeeds. It returns the path to
// the recording file so a test can assert exactly which arguments tmux saw.
//
// This proves the socket-scoped handler dispatches `tmux -S <socket> ...` and
// NOT a TMUX_TMPDIR-derived path: a mismatch between the API socket and the
// executor socket would surface here as a missing/wrong `-S` argument rather
// than silently returning an empty side-panel.
func installRecordingTmux(t *testing.T, sessionLine string) string {
	t.Helper()

	dir := t.TempDir()
	recordPath := filepath.Join(dir, "tmux-argv.log")
	scriptPath := filepath.Join(dir, "tmux")
	// Record the full argv on one line, then emit the canned session line so the
	// handler's list parser produces a non-empty result.
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"$TMUX_ARGV_LOG\"\n" +
		"printf '%s\\n' \"$TMUX_SESSION_LINE\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake recording tmux command: %v", err)
	}
	t.Setenv("TMUX_ARGV_LOG", recordPath)
	t.Setenv("TMUX_SESSION_LINE", sessionLine)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return recordPath
}

func readArgvLog(t *testing.T, recordPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read tmux argv log: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestNewTmuxHandlerForSocket_ListSessionsTargetsExplicitSocket(t *testing.T) {
	const socket = "/run/user/1000/chrote-formations-tmux/default"
	recordPath := installRecordingTmux(t, "mission-codex:1:0")

	handler := NewTmuxHandlerForSocket(socket)
	req := httptest.NewRequest(http.MethodGet, "/api/formations/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].Name != "mission-codex" {
		t.Fatalf("sessions = %+v, want the single mission-codex session from the formations socket", response.Sessions)
	}

	argv := readArgvLog(t, recordPath)
	if len(argv) == 0 {
		t.Fatal("fake tmux recorded no invocations; the scoped handler never called tmux")
	}
	for _, line := range argv {
		// Every invocation must be scoped to the explicit socket. This is the
		// one source of truth that must match the executor's `tmux -S <socket>`.
		if !strings.HasPrefix(line, "-S "+socket+" ") {
			t.Fatalf("tmux invocation %q is not scoped to -S %s; the side panel would list the WRONG socket", line, socket)
		}
	}
}

func TestNewTmuxHandlerForSocket_CreateSessionTargetsExplicitSocket(t *testing.T) {
	const socket = "/run/user/1000/chrote-formations-tmux/default"
	recordPath := installRecordingTmux(t, "")

	handler := NewTmuxHandlerForSocket(socket)
	req := httptest.NewRequest(http.MethodPost, "/api/formations/tmux/sessions",
		strings.NewReader(`{"name":"mission-new"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.CreateSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	argv := readArgvLog(t, recordPath)
	if len(argv) == 0 {
		t.Fatal("fake tmux recorded no invocations; CreateSession never called tmux")
	}
	got := argv[0]
	if !strings.HasPrefix(got, "-S "+socket+" new-session ") {
		t.Fatalf("new-session argv = %q, want it prefixed with -S %s new-session", got, socket)
	}
}

func TestNewTmuxHandlerForSocket_CaptureTargetsExplicitSocket(t *testing.T) {
	const socket = "/run/user/1000/chrote-formations-tmux/default"
	recordPath := installRecordingTmux(t, "pane contents")

	handler := NewTmuxHandlerForSocket(socket)
	req := httptest.NewRequest(http.MethodGet, "/api/formations/tmux/sessions/mission-codex/capture", nil)
	req.SetPathValue("name", "mission-codex")
	rec := httptest.NewRecorder()

	handler.CapturePane(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	argv := readArgvLog(t, recordPath)
	if len(argv) == 0 {
		t.Fatal("fake tmux recorded no invocations; CapturePane never called tmux")
	}
	got := argv[0]
	if !strings.HasPrefix(got, "-S "+socket+" capture-pane ") {
		t.Fatalf("capture-pane argv = %q, want it prefixed with -S %s capture-pane", got, socket)
	}
}

// TestNewTmuxHandler_CockpitDoesNotInjectSocketFlag locks the unchanged cockpit
// behavior: the default handler relies on TMUX_TMPDIR env (no explicit -S), so
// it must NOT inject a -S flag into tmux argv.
func TestNewTmuxHandler_CockpitDoesNotInjectSocketFlag(t *testing.T) {
	recordPath := installRecordingTmux(t, "shell:1:0")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	argv := readArgvLog(t, recordPath)
	if len(argv) == 0 {
		t.Fatal("fake tmux recorded no invocations; cockpit handler never called tmux")
	}
	for _, line := range argv {
		if strings.HasPrefix(line, "-S ") {
			t.Fatalf("cockpit tmux invocation %q injected an explicit -S socket; cockpit must stay env-driven", line)
		}
	}
}

func TestNewTmuxHandlerForSocket_RegisterRoutesUsesFormationsPrefix(t *testing.T) {
	const socket = "/run/user/1000/chrote-formations-tmux/default"
	handler := NewTmuxHandlerForSocket(socket)
	mux := http.NewServeMux()

	handler.RegisterRoutesWithPrefix(mux, "/api/formations/tmux")

	// A request under the formations prefix must resolve to a registered handler,
	// not the mux's 404.
	req := httptest.NewRequest(http.MethodGet, "/api/formations/tmux/sessions", nil)
	h, pattern := mux.Handler(req)
	if pattern == "" || h == nil {
		t.Fatalf("no handler registered for /api/formations/tmux/sessions (pattern=%q)", pattern)
	}
}
