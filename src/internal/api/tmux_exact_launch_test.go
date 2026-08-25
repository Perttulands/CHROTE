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

	"github.com/chrote/server/internal/core"
)

func installExactLaunchTmux(t *testing.T, listOutput string) string {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-argv.txt")
	scriptPath := filepath.Join(dir, "tmux")
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$TMUX_ARGS_FILE"
done
printf '%s\n' '---' >> "$TMUX_ARGS_FILE"
case " $* " in
  *" list-sessions "*) printf '%b' "$TMUX_LIST_OUTPUT" ;;
  *" new-session "*) printf '$42\t%%7\t4242\n' ;;
  *" display-message "*) printf '$42\texact-session\t%%7\t4242\t%s\n' "$TMUX_EXPECTED_CWD" ;;
  *" show-environment "*" CHROTE_EXACT_LAUNCH_ID "*) printf 'CHROTE_EXACT_LAUNCH_ID=%s\n' "$TMUX_SHOW_LAUNCH_ID" ;;
  *" show-environment "*" CHROTE_EXACT_LAUNCH_DIGEST "*) printf 'CHROTE_EXACT_LAUNCH_DIGEST=%s\n' "$TMUX_SHOW_DIGEST" ;;
  *" list-panes "*) printf '%b' "$TMUX_PANES_OUTPUT" ;;
  *" if-shell "*) printf '' ;;
esac
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHROTE_TMUX_BIN", scriptPath)
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("TMUX_LIST_OUTPUT", listOutput)
	return argsPath
}

func configureExactLaunchTarget(t *testing.T, root string) {
	t.Helper()
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/fixture-alice/tmux.sock,build=/tmp/fixture-build/tmux.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice="+root+",build="+root)
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice="+root+",build="+root)
	t.Setenv("CHROTE_ROOTS", root)
	t.Setenv("CHROTE_SESSION_BANK_PATH", filepath.Join(root, "session-bank.json"))
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", filepath.Join(root, "managed-status.json"))
	core.ResetConfigForTesting()
	t.Cleanup(core.ResetConfigForTesting)
}

func exactLaunchRequest(t *testing.T, root, generation string, argv []string) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"sourceId":         "tmux:alice",
		"sourceGeneration": generation,
		"unixUser":         "alice",
		"name":             "exact-session",
		"cwd":              root,
		"argv":             argv,
	})
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewRequest(http.MethodPut, "/api/tmux/recovery-launches/11111111-1111-4111-8111-111111111111", bytes.NewReader(body))
}

func TestTmuxHandler_ExactLaunchExecutesStructuredArgvAndReturnsIdentity(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "")
	configureExactLaunchTarget(t, root)
	t.Setenv("TMUX_EXPECTED_CWD", root)
	oldStart := exactLaunchProcessStartTime
	exactLaunchProcessStartTime = func(pid int) (string, error) { return "777", nil }
	t.Cleanup(func() { exactLaunchProcessStartTime = oldStart })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	argv := []string{"/usr/bin/printf", "hello world", "; touch /tmp/must-not-run"}
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, tmuxSourceGeneration("alice", nil), argv))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	if payload["launchId"] != "11111111-1111-4111-8111-111111111111" || payload["sessionId"] != "$42" || payload["paneId"] != "%7" {
		t.Fatalf("identity receipt = %#v", payload)
	}
	if payload["panePid"] != float64(4242) || payload["processStart"] != "777" || payload["cwd"] != root {
		t.Fatalf("process receipt = %#v", payload)
	}

	calls := readArgvRecordingTmuxCalls(t, argsPath)
	var launch []string
	for _, call := range calls {
		if containsArg(call, "new-session") {
			launch = call
		}
		if containsArg(call, "send-keys") {
			t.Fatalf("exact launch typed command text through send-keys: %#v", call)
		}
	}
	if len(launch) == 0 {
		t.Fatalf("tmux calls = %#v, want new-session", calls)
	}
	joined := strings.Join(launch, "\x00")
	wantSuffix := strings.Join(argv, "\x00")
	if !strings.HasSuffix(joined, wantSuffix) {
		t.Fatalf("new-session argv = %#v, want exact structured suffix %#v", launch, argv)
	}
}

func TestTmuxHandler_ExactLaunchRejectsStaleSourceBeforeMutation(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "$7\tother\t1\t0\t/srv/other\n")
	configureExactLaunchTarget(t, root)

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, tmuxSourceGeneration("alice", nil), []string{"/bin/true", "--"}))

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "TMUX_SOURCE_CHANGED") {
		t.Fatalf("status/body = %d %s, want stale-source conflict", rec.Code, rec.Body.String())
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("stale request mutated tmux: %#v", call)
		}
	}
}

func TestTmuxHandler_ExactLaunchRejectsAmbiguousUserAndInvalidCWD(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "")
	configureExactLaunchTarget(t, root)
	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"sourceId":"tmux:default","sourceGeneration":"sha256:bad","name":"exact-session","cwd":"/tmp","argv":["/bin/true","--"]}`
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/tmux/recovery-launches/11111111-1111-4111-8111-111111111111", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Unix user is required") {
		t.Fatalf("ambiguous user status/body = %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, filepath.Dir(root), tmuxSourceGeneration("alice", nil), []string{"/bin/true", "--"}))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "EXACT_LAUNCH_CWD_FORBIDDEN") {
		t.Fatalf("invalid cwd status/body = %d %s", rec.Code, rec.Body.String())
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("invalid request mutated tmux: %#v", call)
		}
	}
}

func TestTmuxHandler_ExactLaunchReplaysItsExactOwnedTargetIdempotently(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "$42	exact-session	1	0	"+root+"\n")
	configureExactLaunchTarget(t, root)
	t.Setenv("TMUX_SHOW_LAUNCH_ID", "11111111-1111-4111-8111-111111111111")
	argv := []string{"/usr/bin/true", "--"}
	t.Setenv("TMUX_SHOW_DIGEST", exactLaunchDigest("tmux:alice", "alice", "exact-session", root, argv))
	t.Setenv("TMUX_PANES_OUTPUT", "$42	exact-session	%7	4242	"+root+"\n")
	oldStart := exactLaunchProcessStartTime
	exactLaunchProcessStartTime = func(pid int) (string, error) { return "777", nil }
	t.Cleanup(func() { exactLaunchProcessStartTime = oldStart })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, tmuxSourceGeneration("alice", nil), argv))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want replay success; body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	if payload["state"] != "replayed" || payload["sessionId"] != "$42" || payload["paneId"] != "%7" {
		t.Fatalf("replay receipt = %#v", payload)
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("idempotent replay created another session: %#v", call)
		}
	}
}

func TestTmuxHandler_ExactLaunchRejectsLaunchIDReuseWithDifferentSpec(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "$42	exact-session	1	0	"+root+"\n")
	configureExactLaunchTarget(t, root)
	t.Setenv("TMUX_SHOW_LAUNCH_ID", "11111111-1111-4111-8111-111111111111")
	t.Setenv("TMUX_SHOW_DIGEST", "sha256:different")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, tmuxSourceGeneration("alice", nil), []string{"/usr/bin/true", "--"}))

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT") {
		t.Fatalf("status/body = %d %s, want idempotency conflict", rec.Code, rec.Body.String())
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("idempotency conflict mutated tmux: %#v", call)
		}
	}
}
