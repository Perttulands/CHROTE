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

// installSelectiveTmux writes a fake tmux that fails only for sockets whose path
// contains failFor, and otherwise reports one session named after the socket.
//
// A selective failure expresses the multi-user case: one user's socket is
// unreadable while another's is healthy.
func installSelectiveTmux(t *testing.T, failFor string, stderr string) {
	t.Helper()

	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"sock=\"\"\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  case \"$1\" in\n" +
		"    -S) sock=\"$2\"; shift 2 ;;\n" +
		"    *) shift ;;\n" +
		"  esac\n" +
		"done\n" +
		"if [ -n \"$TMUX_EMPTY_FOR\" ]; then\n" +
		"  case \"$sock\" in *\"$TMUX_EMPTY_FOR\"*) printf 'no server running on %s\\n' \"$sock\" >&2; exit 1 ;; esac\n" +
		"fi\n" +
		"case \"$sock\" in\n" +
		"  *" + failFor + "*) printf '%s\\n' \"$TMUX_STDERR\" >&2; exit 1 ;;\n" +
		"esac\n" +
		"printf '%s	%s	%s	%s	%s	%s	%s	%s	%s	%s	%s	%s	%s\\n' '$7' 'healthy-session' '1' '0' '/workspaces/healthy' 'bash' '1' '120' '40' 'latest' '1' '' '1756900000'\n" +
		"exit 0\n"
	scriptPath := filepath.Join(dir, "tmux")
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write selective fake tmux: %v", err)
	}
	t.Setenv("TMUX_STDERR", stderr)
	t.Setenv("TMUX_EMPTY_FOR", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func installAlwaysFailingTmux(t *testing.T, stderr string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$TMUX_STDERR\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o700); err != nil {
		t.Fatalf("write failing fake tmux: %v", err)
	}
	t.Setenv("TMUX_STDERR", stderr)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// The production path on a multi-user host is the CHROTE_TMUX_SOCKET loop, and
// it is the only place a socket error is prefixed with the Unix user. This test
// drives the partial-inventory branch where one explicit source is unavailable.
// A broken socket for one user must not hide another user's live sessions. The API
// deliberately returns partial success -- healthy sessions plus an error naming the
// failures -- because cross-user ACL breakage is the headline failure mode here, and
// an empty list is indistinguishable from "no sessions". Which user is broken has to
// be identifiable too: an unattributed permission error on a host with several users
// is not actionable, and blaming the healthy one sends the operator to the wrong
// socket.
//
// Fake users are paired with fake workdirs so targetForUnixUser can exercise the
// configured multi-user branch without relying on host accounts.
func TestTmuxHandler_ListSessionsStillReturnsHealthyUsersSessionsWhenOneSocketFails(t *testing.T) {
	installSelectiveTmux(t, "denied", "error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)")
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/fixture-alice/ok.sock,build=/tmp/fixture-build/denied.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

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
	if len(response.Sessions) == 0 {
		t.Fatalf("sessions = empty, want the healthy user's sessions preserved alongside the error %q", response.Error)
	}
	for _, session := range response.Sessions {
		if session.UnixUser == "build" {
			t.Fatalf("sessions contain the failed user %q: %+v", session.UnixUser, session)
		}
	}
	if !strings.Contains(response.Error, "build: tmux source permission denied") {
		t.Fatalf("error = %q, want the redacted permission failure named against its user", response.Error)
	}
	if strings.Contains(response.Error, "alice") {
		t.Fatalf("error = %q, want the healthy user not to be blamed", response.Error)
	}
	payload := decodeJSONMap(t, rec)
	if partial, ok := payload["partial"].(bool); !ok || !partial {
		t.Fatalf("partial = %#v, want true for healthy sessions plus a per-user error", payload["partial"])
	}
}

// A zero-session user is still authoritative: its absence of rows means old
// sessions may be deleted. The response therefore has to name both successful
// and failed users instead of asking clients to infer authority from sessions.
func TestTmuxHandler_ListSessionsPartialNamesSuccessfulAndFailedUsers(t *testing.T) {
	installSelectiveTmux(t, "denied", "error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)")
	t.Setenv("TMUX_EMPTY_FOR", "empty")
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/fixture-alice/empty.sock,build=/tmp/fixture-build/denied.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

	handler := NewTmuxHandler()
	rec := httptest.NewRecorder()
	handler.ListSessions(rec, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))

	payload := decodeJSONMap(t, rec)
	if partial, ok := payload["partial"].(bool); !ok || !partial {
		t.Fatalf("partial = %#v, want true when an empty healthy source accompanies a failed source", payload["partial"])
	}
	if sessions, ok := payload["sessions"].([]interface{}); !ok || len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want no rows from the healthy zero-session user", payload["sessions"])
	}
	assertJSONStrings(t, payload, "successfulUsers", []string{"alice"})
	assertJSONStrings(t, payload, "failedUsers", []string{"build"})
}

func assertJSONStrings(t *testing.T, payload map[string]interface{}, field string, want []string) {
	t.Helper()
	values, ok := payload[field].([]interface{})
	if !ok {
		t.Fatalf("%s = %#v, want a JSON string array", field, payload[field])
	}
	got := make([]string, len(values))
	for index, value := range values {
		var stringValue bool
		got[index], stringValue = value.(string)
		if !stringValue {
			t.Fatalf("%s[%d] = %#v, want a string", field, index, value)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s = %#v, want %#v", field, got, want)
	}
}

// When every configured user's socket fails, none of the returned session data
// is authoritative. The partial marker must stay absent so dashboard clients
// preserve their last-known-good state.
func TestTmuxHandler_ListSessionsDoesNotMarkTotalMultiUserFailurePartial(t *testing.T) {
	installSelectiveTmux(t, "fixture-", "error connecting to /tmp/chrote-tmux-test/socket (Permission denied)")
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/fixture-alice/denied.sock,build=/tmp/fixture-build/denied.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	payload := decodeJSONMap(t, rec)
	if _, ok := payload["partial"]; ok {
		t.Fatalf("partial = %#v, want the marker omitted when every configured user failed", payload["partial"])
	}
	if sessions, ok := payload["sessions"].([]interface{}); !ok || len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want none when every configured user failed", payload["sessions"])
	}
	errorText, _ := payload["error"].(string)
	if !strings.Contains(errorText, "alice:") || !strings.Contains(errorText, "build:") {
		t.Fatalf("error = %q, want both failed fake users named", errorText)
	}
}

// A per-user partial marker covers only tmux socket failures. If another
// sessions-response subsystem also fails, the combined payload is not
// authoritative and must retain total-failure semantics.
func TestTmuxHandler_ListSessionsMultiUserNoServerIsNotAnError(t *testing.T) {
	installSelectiveTmux(t, "empty", "no server running on /tmp/fixture-build/empty.sock")
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/fixture-alice/ok.sock,build=/tmp/fixture-build/empty.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error != "" {
		t.Fatalf("error = %q, want no user-visible error when a user simply has no server", response.Error)
	}
	if len(response.Sessions) == 0 {
		t.Fatalf("sessions = empty, want the running user's sessions listed")
	}
}
