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
// The pre-existing baseline test installs a tmux that fails for EVERY call, which
// cannot express the case that matters on a multi-user host: one user's socket
// unreadable while another's is fine.
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
		"case \"$sock\" in\n" +
		"  *" + failFor + "*) printf '%s\\n' \"$TMUX_STDERR\" >&2; exit 1 ;;\n" +
		"esac\n" +
		"printf '$1:healthy-session:1:0\\n'\n" +
		"exit 0\n"
	scriptPath := filepath.Join(dir, "tmux")
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write selective fake tmux: %v", err)
	}
	t.Setenv("TMUX_STDERR", stderr)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// The production path on a multi-user host is the CHROTE_TERMINAL_USERS loop, and
// it is the ONLY place a socket error is prefixed with the unix user. Without
// CHROTE_TERMINAL_USERS set, configuredTerminalUsers() returns empty and
// ListSessions takes the single-target branch instead -- which is what every
// pre-existing test exercised, so the branch that actually runs in production had
// no coverage and "the error names the effective user" was unproven.
func TestTmuxHandler_ListSessionsNamesTheUnixUserWhoseSocketFailed(t *testing.T) {
	installSelectiveTmux(t, "denied", "error connecting to /run/user/2002/chrote-tmux/default (Permission denied)")
	t.Setenv("CHROTE_TERMINAL_USERS", "daemon,nobody")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "daemon=/tmp/fixture-daemon/ok.sock,nobody=/tmp/fixture-nobody/denied.sock")

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
		t.Fatalf("error = %q, want the permission failure to be visible", response.Error)
	}
	// The whole point: which user is broken must be identifiable. An unattributed
	// permission error on a host with several users is not actionable.
	//
	// `daemon` and `nobody` stand in for the two real operators. They are used because
	// targetForUnixUser does a real user.Lookup, so invented names fail before the code
	// under test runs -- and the real usernames must never appear in a tracked file
	// (scripts/host-neutrality.py). Both accounts exist on any Linux host, CI included.
	if !strings.Contains(response.Error, "nobody") {
		t.Fatalf("error = %q, want it to name the unix user whose socket failed", response.Error)
	}
	if strings.Contains(response.Error, "daemon") {
		t.Fatalf("error = %q, want the healthy user not to be blamed", response.Error)
	}
}

// A broken socket for one user must not hide another user's live sessions. The API
// deliberately returns partial success -- healthy sessions plus an error naming the
// failures -- because cross-user ACL breakage is the headline failure mode here, and
// an empty list is indistinguishable from "no sessions".
func TestTmuxHandler_ListSessionsStillReturnsHealthyUsersSessionsWhenOneSocketFails(t *testing.T) {
	installSelectiveTmux(t, "denied", "error connecting to /run/user/2002/chrote-tmux/default (Permission denied)")
	t.Setenv("CHROTE_TERMINAL_USERS", "daemon,nobody")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "daemon=/tmp/fixture-daemon/ok.sock,nobody=/tmp/fixture-nobody/denied.sock")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) == 0 {
		t.Fatalf("sessions = empty, want the healthy user's sessions preserved alongside the error %q", response.Error)
	}
	for _, session := range response.Sessions {
		if session.UnixUser == "nobody" {
			t.Fatalf("sessions contain the failed user %q: %+v", session.UnixUser, session)
		}
	}
}

// A genuinely absent server is not an error. Same loop, so this asserts the
// no-server path stays quiet in the multi-user branch too -- otherwise a user who
// simply has no tmux running would raise a permanent cockpit error.
func TestTmuxHandler_ListSessionsMultiUserNoServerIsNotAnError(t *testing.T) {
	installSelectiveTmux(t, "empty", "no server running on /tmp/tmux-2002/default")
	t.Setenv("CHROTE_TERMINAL_USERS", "daemon,nobody")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "daemon=/tmp/fixture-daemon/ok.sock,nobody=/tmp/fixture-nobody/empty.sock")

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
