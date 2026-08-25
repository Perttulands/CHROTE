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
socket=''
previous=''
for arg in "$@"; do
  if [ "$previous" = '-S' ]; then socket="$arg"; fi
  previous="$arg"
  printf '%s\n' "$arg" >> "$TMUX_ARGS_FILE"
done
printf '%s\n' '---' >> "$TMUX_ARGS_FILE"
case " $* " in
  *" list-sessions "*)
    if [ "$socket" = '/tmp/fixture-alice/tmux.sock' ] && [ -n "$TMUX_SWAP_LINK" ]; then
      count=0
      if [ -f "$TMUX_LIST_CALLS_FILE" ]; then read -r count < "$TMUX_LIST_CALLS_FILE"; fi
      count=$((count + 1))
      printf '%s\n' "$count" > "$TMUX_LIST_CALLS_FILE"
      if [ "$count" -eq 2 ]; then ln -sfn "$TMUX_SWAP_TARGET" "$TMUX_SWAP_LINK"; fi
    fi
    if [ "$socket" = '/tmp/fixture-build/tmux.sock' ] && [ -n "$TMUX_BUILD_LIST_OUTPUT" ]; then
      printf '%b' "$TMUX_BUILD_LIST_OUTPUT"
    elif [ "$socket" = '/tmp/fixture-build/tmux.sock' ]; then
      printf 'no server running on %s\n' "$socket" >&2
      exit 1
    elif [ -n "$TMUX_LIST_OUTPUT" ]; then
      printf '%b' "$TMUX_LIST_OUTPUT"
    else
      printf 'no server running on %s\n' "$socket" >&2
      exit 1
    fi
    ;;
  *" new-session "*) printf '$42\t%%7\t4242\n' ;;
  *" display-message "*) printf '$42\texact-session\t%%7\t4242\t%s\n' "$TMUX_EXPECTED_CWD" ;;
  *" show-environment "*" CHROTE_EXACT_LAUNCH_ID "*)
    if [ -n "$TMUX_SHOW_ERROR" ]; then printf '%s\n' "$TMUX_SHOW_ERROR" >&2; exit 1; fi
    printf 'CHROTE_EXACT_LAUNCH_ID=%s\n' "$TMUX_SHOW_LAUNCH_ID"
    ;;
  *" show-environment "*" CHROTE_EXACT_LAUNCH_DIGEST "*)
    if [ -n "$TMUX_SHOW_ERROR" ]; then printf '%s\n' "$TMUX_SHOW_ERROR" >&2; exit 1; fi
    printf 'CHROTE_EXACT_LAUNCH_DIGEST=%s\n' "$TMUX_SHOW_DIGEST"
    ;;
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

func fakeAuthoritativeServerIdentity(unixUser, pid, socket string) string {
	return pid + "@1700000000@" + socket + "@test-process=" + pid + ";user=" + unixUser
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
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, tmuxSourceGeneration("alice", nil, "absent@/tmp/fixture-alice/tmux.sock"), argv))

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
	argsPath := installExactLaunchTmux(t, "9001	1700000000	/tmp/fixture-alice/tmux.sock	$7	other	1	0	/srv/other\n")
	configureExactLaunchTarget(t, root)

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, tmuxSourceGeneration("alice", nil, "absent@/tmp/fixture-alice/tmux.sock"), []string{"/bin/true", "--"}))

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
	mux.ServeHTTP(rec, exactLaunchRequest(t, filepath.Dir(root), tmuxSourceGeneration("alice", nil, "absent@/tmp/fixture-alice/tmux.sock"), []string{"/bin/true", "--"}))
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
	argsPath := installExactLaunchTmux(t, "9001	1700000000	/tmp/fixture-alice/tmux.sock	$42	exact-session	1	0	"+root+"\n")
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
	generation := tmuxSourceGeneration("alice", []core.Session{{ID: "$42", Name: "exact-session", UnixUser: "alice", Windows: 1, CWD: root}}, fakeAuthoritativeServerIdentity("alice", "9001", "/tmp/fixture-alice/tmux.sock"))
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, generation, argv))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want replay success; body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	if payload["state"] != "replayed" || payload["sessionId"] != "$42" || payload["paneId"] != "%7" || payload["sourceGeneration"] != generation {
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
	argsPath := installExactLaunchTmux(t, "9001	1700000000	/tmp/fixture-alice/tmux.sock	$42	exact-session	1	0	"+root+"\n")
	configureExactLaunchTarget(t, root)
	t.Setenv("TMUX_SHOW_LAUNCH_ID", "11111111-1111-4111-8111-111111111111")
	t.Setenv("TMUX_SHOW_DIGEST", "sha256:different")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	generation := tmuxSourceGeneration("alice", []core.Session{{ID: "$42", Name: "exact-session", UnixUser: "alice", Windows: 1, CWD: root}}, fakeAuthoritativeServerIdentity("alice", "9001", "/tmp/fixture-alice/tmux.sock"))
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, generation, []string{"/usr/bin/true", "--"}))

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT") {
		t.Fatalf("status/body = %d %s, want idempotency conflict", rec.Code, rec.Body.String())
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("idempotency conflict mutated tmux: %#v", call)
		}
	}
}

func TestTmuxHandler_ExactLaunchIDCannotCreateAnotherSessionName(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "9001	1700000000	/tmp/fixture-alice/tmux.sock	$41	original-session	1	0	"+root+"\n")
	configureExactLaunchTarget(t, root)
	t.Setenv("TMUX_SHOW_LAUNCH_ID", "11111111-1111-4111-8111-111111111111")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, tmuxSourceGeneration("alice", []core.Session{{ID: "$41", Name: "original-session", UnixUser: "alice", Windows: 1, CWD: root}}, fakeAuthoritativeServerIdentity("alice", "9001", "/tmp/fixture-alice/tmux.sock")), []string{"/bin/true", "--"}))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT") {
		t.Fatalf("status/body = %d %s, want cross-name idempotency conflict", rec.Code, rec.Body.String())
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("conflicting launch ID mutated tmux: %#v", call)
		}
	}
}

func TestTmuxHandler_ExactLaunchIDCannotCrossConfiguredSources(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "")
	configureExactLaunchTarget(t, root)
	t.Setenv("TMUX_BUILD_LIST_OUTPUT", "9002	1700000000	/tmp/fixture-build/tmux.sock	$51	foreign-session	1	0	"+root+"\n")
	t.Setenv("TMUX_SHOW_LAUNCH_ID", "11111111-1111-4111-8111-111111111111")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, tmuxSourceGeneration("alice", nil, "absent@/tmp/fixture-alice/tmux.sock"), []string{"/bin/true", "--"}))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT") {
		t.Fatalf("status/body = %d %s, want cross-source idempotency conflict", rec.Code, rec.Body.String())
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("cross-source launch ID mutated tmux: %#v", call)
		}
	}
}

func TestTmuxHandler_ExactLaunchRevalidatesPathsImmediatelyBeforeMutation(t *testing.T) {
	root := t.TempDir()
	safe := filepath.Join(root, "safe")
	if err := os.MkdirAll(safe, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	link := filepath.Join(root, "workspace")
	if err := os.Symlink(safe, link); err != nil {
		t.Fatal(err)
	}
	argsPath := installExactLaunchTmux(t, "")
	configureExactLaunchTarget(t, root)
	t.Setenv("TMUX_SWAP_LINK", link)
	t.Setenv("TMUX_SWAP_TARGET", outside)
	t.Setenv("TMUX_LIST_CALLS_FILE", filepath.Join(root, "list-calls"))

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, link, tmuxSourceGeneration("alice", nil, "absent@/tmp/fixture-alice/tmux.sock"), []string{"/bin/true", "--"}))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "EXACT_LAUNCH_PATH_CHANGED") {
		t.Fatalf("status/body = %d %s, want changed path identity rejection", rec.Code, rec.Body.String())
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("changed path identity mutated tmux: %#v", call)
		}
	}
}

func TestTmuxHandler_ExactLaunchFailsClosedWhenOwnershipMarkersCannotBeRead(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "9001\t1700000000\t/tmp/fixture-alice/tmux.sock\t$42\texact-session\t1\t0\t"+root+"\\n")
	configureExactLaunchTarget(t, root)
	t.Setenv("TMUX_SHOW_ERROR", "permission denied: token=must-not-leak")
	generation := tmuxSourceGeneration("alice", []core.Session{{ID: "$42", Name: "exact-session", UnixUser: "alice", Windows: 1, CWD: root}}, fakeAuthoritativeServerIdentity("alice", "9001", "/tmp/fixture-alice/tmux.sock"))

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, generation, []string{"/bin/true", "--"}))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "TMUX_SOURCE_UNAVAILABLE") || strings.Contains(rec.Body.String(), "must-not-leak") {
		t.Fatalf("status/body = %d %s, want redacted authoritative-source failure", rec.Code, rec.Body.String())
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("unreadable ownership marker mutated tmux: %#v", call)
		}
	}
}

func TestExactLaunchArgvRequiresApprovedNonSetIDExecutableAndAnArgument(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "agent")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHROTE_EXACT_LAUNCH_EXECUTABLE_ROOTS", dir)
	if _, err := exactLaunchArgv([]string{executable}, dir); err == nil || err.Code != "EXACT_LAUNCH_ARGV_INVALID" {
		t.Fatalf("single-argument launch error = %#v, want closed structured argv", err)
	}
	if _, err := exactLaunchArgv([]string{executable, "--resume"}, dir); err != nil {
		t.Fatalf("approved executable rejected: %v", err)
	}
	if err := os.Chmod(executable, 0o755|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	if _, err := exactLaunchArgv([]string{executable, "--resume"}, dir); err == nil || err.Code != "EXACT_LAUNCH_EXECUTABLE_INVALID" {
		t.Fatalf("setid executable error = %#v, want rejection", err)
	}
}

func TestTmuxHandler_ExactLaunchRequiresOwnerHomeBeforeLegacyOwnershipCheck(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "")
	configureExactLaunchTarget(t, root)
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, exactLaunchRequest(t, root, tmuxSourceGeneration("alice", nil, "absent@/tmp/fixture-alice/tmux.sock"), []string{"/usr/bin/true", "--"}))

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "EXACT_LAUNCH_OWNER_INVALID") {
		t.Fatalf("status/body = %d %s, want missing owner-home rejection", rec.Code, rec.Body.String())
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("missing owner home mutated tmux: %#v", call)
		}
	}
}

func TestTmuxHandler_ExactLaunchRejectsUnknownFieldsAndTmuxSeparatorsBeforeMutation(t *testing.T) {
	root := t.TempDir()
	argsPath := installExactLaunchTmux(t, "")
	configureExactLaunchTarget(t, root)
	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	tests := []struct {
		name     string
		body     map[string]interface{}
		wantCode string
	}{
		{
			name: "unknown field",
			body: map[string]interface{}{
				"sourceId": "tmux:alice", "sourceGeneration": "sha256:unused", "unixUser": "alice",
				"name": "exact-session", "cwd": root, "argv": []string{"/bin/true", "--"}, "environment": map[string]string{"UNSAFE": "1"},
			},
			wantCode: "BAD_REQUEST",
		},
		{
			name: "tmux separator",
			body: map[string]interface{}{
				"sourceId": "tmux:alice", "sourceGeneration": "sha256:unused", "unixUser": "alice",
				"name": "exact-session", "cwd": root, "argv": []string{"/bin/true", ";"},
			},
			wantCode: "EXACT_LAUNCH_ARGV_INVALID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/tmux/recovery-launches/11111111-1111-4111-8111-111111111111", bytes.NewReader(body))
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.wantCode) {
				t.Fatalf("status/body = %d %s, want %s", rec.Code, rec.Body.String(), tt.wantCode)
			}
		})
	}
	valid, err := json.Marshal(map[string]interface{}{
		"sourceId": "tmux:alice", "sourceGeneration": "sha256:unused", "unixUser": "alice",
		"name": "exact-session", "cwd": root, "argv": []string{"/bin/true", "--"},
	})
	if err != nil {
		t.Fatal(err)
	}
	withoutUser, err := json.Marshal(map[string]interface{}{
		"sourceId": "tmux:alice", "sourceGeneration": "sha256:unused",
		"name": "exact-session", "cwd": root, "argv": []string{"/bin/true", "--"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rawTests := []struct {
		name     string
		body     []byte
		wantCode string
	}{
		{name: "duplicate field", body: bytes.Replace(valid, []byte(`"sourceId":`), []byte(`"sourceId":"tmux:bob","sourceId":`), 1), wantCode: "BAD_REQUEST"},
		{name: "case variant field", body: bytes.Replace(valid, []byte(`"sourceId"`), []byte(`"SourceID"`), 1), wantCode: "BAD_REQUEST"},
		{name: "omitted Unix user", body: withoutUser, wantCode: "EXACT_LAUNCH_USER_REQUIRED"},
	}
	for _, tt := range rawTests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/tmux/recovery-launches/11111111-1111-4111-8111-111111111111", bytes.NewReader(tt.body))
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.wantCode) {
				t.Fatalf("status/body = %d %s, want %s", rec.Code, rec.Body.String(), tt.wantCode)
			}
		})
	}
	for _, call := range readArgvRecordingTmuxCalls(t, argsPath) {
		if containsArg(call, "new-session") {
			t.Fatalf("invalid request mutated tmux: %#v", call)
		}
	}
}
