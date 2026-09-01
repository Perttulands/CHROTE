package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installSizingRecorderTmux installs a fake tmux that records both its argv and
// the stdin it was given, so a control-mode sizing command can be asserted in
// full.
func installSizingRecorderTmux(t *testing.T) (argsPath, stdinPath string) {
	t.Helper()
	dir := t.TempDir()
	argsPath = filepath.Join(dir, "tmux-args.txt")
	stdinPath = filepath.Join(dir, "tmux-stdin.txt")
	scriptPath := filepath.Join(dir, "tmux")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TMUX_ARGS_FILE"
case "$*" in
  *" -C attach-session"*) cat >> "$TMUX_STDIN_FILE" ;;
  *new-session*) printf '$7\n' ;;
esac
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
		t.Fatalf("write args log: %v", err)
	}
	if err := os.WriteFile(stdinPath, nil, 0o600); err != nil {
		t.Fatalf("write stdin log: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("TMUX_STDIN_FILE", stdinPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsPath, stdinPath
}

func createSessionThroughHandler(t *testing.T, name string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewTmuxHandler()
	body, _ := json.Marshal(CreateSessionRequest{Name: name})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.CreateSession(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	return recorder
}

func TestCreateSessionSizesOnceWithoutPinningTheWindow(t *testing.T) {
	argsPath, stdinPath := installSizingRecorderTmux(t)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")

	createSessionThroughHandler(t, "sizing-smoke")

	calls := readFakeCommandCalls(t, argsPath)
	sizing := []string{}
	for _, call := range calls {
		if strings.Contains(call, "new-session") {
			if strings.Contains(call, " -x ") || strings.Contains(call, " -y ") {
				t.Fatalf("new-session pinned the window at creation: %q", call)
			}
		}
		if strings.Contains(call, "resize-window") {
			t.Fatalf("creation used resize-window, which pins window-size manual: %q", call)
		}
		if strings.Contains(call, "attach-session") {
			sizing = append(sizing, call)
		}
	}
	if len(sizing) != 1 {
		t.Fatalf("sizing attaches = %#v, want exactly one", sizing)
	}
	if want := "-S /tmp/tmux-a -C attach-session -t $7"; sizing[0] != want {
		t.Fatalf("sizing call = %q, want %q", sizing[0], want)
	}

	stdin, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatalf("read recorded stdin: %v", err)
	}
	if got, want := string(stdin), "refresh-client -C 200,50\n"; got != want {
		t.Fatalf("control-mode stdin = %q, want %q", got, want)
	}
}

func TestCreateSessionHonoursConfiguredCanonicalSize(t *testing.T) {
	_, stdinPath := installSizingRecorderTmux(t)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_UNOBSERVED_COLS", "160")
	t.Setenv("CHROTE_TERMINAL_UNOBSERVED_ROWS", "48")

	createSessionThroughHandler(t, "configured-size-smoke")

	stdin, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatalf("read recorded stdin: %v", err)
	}
	if got, want := string(stdin), "refresh-client -C 160,48\n"; got != want {
		t.Fatalf("control-mode stdin = %q, want %q", got, want)
	}
}

func TestCanonicalWindowSizeIgnoresUnusableConfiguration(t *testing.T) {
	// Below the tmux default in either dimension, and any value that is not a
	// number at all, leaves the canonical size alone.
	for _, unusable := range []string{"", "wide", "0", "-1", "23"} {
		t.Setenv("CHROTE_TERMINAL_UNOBSERVED_COLS", unusable)
		t.Setenv("CHROTE_TERMINAL_UNOBSERVED_ROWS", unusable)
		cols, rows := canonicalWindowSize()
		if cols != defaultCanonicalWindowCols || rows != defaultCanonicalWindowRows {
			t.Fatalf("canonical size with %q = %dx%d, want %dx%d",
				unusable, cols, rows, defaultCanonicalWindowCols, defaultCanonicalWindowRows)
		}
	}
}

func TestCreateSessionCleansUpWhenSizingFails(t *testing.T) {
	argsPath := installScriptedTmux(t, `
case "$*" in
  *" -C attach-session"*) echo 'no server running' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")

	handler := NewTmuxHandler()
	body, _ := json.Marshal(CreateSessionRequest{Name: "sizing-failure-smoke"})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code == http.StatusOK {
		t.Fatalf("create succeeded despite a sizing failure; body=%s", recorder.Body.String())
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	cleaned := false
	for _, call := range calls {
		if containsArg(call, "if-shell") && containsArg(call, "kill-session -t $42") {
			cleaned = true
		}
	}
	if !cleaned {
		t.Fatalf("failed creation left its own session behind: %#v", calls)
	}
}
