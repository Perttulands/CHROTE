package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/chrote/server/internal/core"
)

func TestTmuxHandler_RunTmuxBoundsAggregateOutput(t *testing.T) {
	installScriptedTmux(t, `
yes x | head -c 1049600
`)
	_, err := NewTmuxHandler().runTmuxOnSocket("/tmp/tmux-a", "list-sessions")
	if !errors.Is(err, errTmuxCommandOutputLimit) {
		t.Fatalf("error = %v, want bounded-output failure", err)
	}
}

// A malformed request is refused before tmux is asked to do anything. The fake
// tmux is installed and a socket configured on purpose: with neither in place a
// handler refuses for want of configuration, and the test would pass without
// touching the validation it is here to pin. The empty argv log is the proof.
func TestTmuxHandler_RefusesMalformedRequestsBeforeReachingTmux(t *testing.T) {
	argsPath := installScriptedTmux(t, "")
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	handler := NewTmuxHandler()

	for _, testCase := range []struct {
		name     string
		method   string
		path     string
		body     string
		pathName string
		call     func(http.ResponseWriter, *http.Request)
	}{
		{
			name:   "create a session from malformed JSON",
			method: http.MethodPost,
			path:   "/api/tmux/sessions",
			body:   "{invalid}",
			call:   handler.CreateSession,
		},
		{
			name:   "create a session whose name has spaces",
			method: http.MethodPost,
			path:   "/api/tmux/sessions",
			body:   `{"name":"invalid name with spaces"}`,
			call:   handler.CreateSession,
		},
		{
			name:     "delete a session whose name has punctuation",
			method:   http.MethodDelete,
			path:     "/api/tmux/sessions/invalid@name",
			pathName: "invalid@name",
			call:     handler.DeleteSession,
		},
		{
			name:     "rename a session to a name with punctuation",
			method:   http.MethodPatch,
			path:     "/api/tmux/sessions/oldsession",
			body:     `{"newName":"invalid name!"}`,
			pathName: "oldsession",
			call:     handler.RenameSession,
		},
		{
			name:   "set mouse mode from malformed JSON",
			method: http.MethodPost,
			path:   "/api/tmux/mouse",
			body:   "{invalid}",
			call:   handler.SetMouseMode,
		},
		{
			name:   "set mouse mode without saying which way",
			method: http.MethodPost,
			path:   "/api/tmux/mouse",
			body:   `{}`,
			call:   handler.SetMouseMode,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
				t.Fatalf("reset tmux argv log: %v", err)
			}
			req := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			if testCase.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if testCase.pathName != "" {
				req.SetPathValue("name", testCase.pathName)
			}
			recorder := httptest.NewRecorder()

			testCase.call(recorder, req)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			var response core.APIResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode refusal: %v; body=%s", err, recorder.Body.String())
			}
			if response.Success || response.Error == nil || response.Error.Code == "" {
				t.Fatalf("refusal = %s, want an unsuccessful envelope carrying an error code", recorder.Body.String())
			}
			if calls := readArgvRecordingTmuxCalls(t, argsPath); len(calls) != 0 {
				t.Fatalf("tmux calls = %#v, want the request refused before any tmux command", calls)
			}
		})
	}
}

func TestTmuxHandler_ListSessionsFiltersReservedProbeSessions(t *testing.T) {
	installScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$1\tchrote-probe-123\t1\t0\t/workspaces/alice\tbash\t1\t120\t40\tlatest\t1\t\t1756900000\n$2\twork\t1\t0\t/workspaces/alice\tbash\t1\t120\t40\tlatest\t1\t\t1756900000\n' ;;
esac
`)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].Name != "work" {
		t.Fatalf("sessions = %+v, want only non-reserved work session", response.Sessions)
	}
}

func TestTmuxHandler_ListSessionsReportsLiveActivePaneCWDAndCommand(t *testing.T) {
	installScriptedTmux(t, `
case "$*" in
  *pane_current_path*) printf '$9\twork\t1\t0\t/workspaces/alice/live\tcodex\t1\t120\t40\tlatest\t1\t\t1756900000\n$10\tno-cwd\t1\t0\t\tbash\t1\t120\t40\tlatest\t1\t\t1756900000\n' ;;
esac
`)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 2 {
		t.Fatalf("sessions = %+v, want live sessions with and without cwd", response.Sessions)
	}
	byName := make(map[string]core.Session, len(response.Sessions))
	for _, session := range response.Sessions {
		byName[session.Name] = session
	}
	if got := byName["work"].CWD; got != "/workspaces/alice/live" {
		t.Fatalf("live session cwd = %q, want active pane cwd", got)
	}
	if got := byName["no-cwd"].CWD; got != "" {
		t.Fatalf("empty live session cwd = %q, want empty cwd without dropping session", got)
	}
	if got := byName["work"].CurrentCommand; got != "codex" {
		t.Fatalf("live session command = %q, want active pane command", got)
	}
	if got := byName["no-cwd"].CurrentCommand; got != "bash" {
		t.Fatalf("shell session command = %q, want shell foreground evidence", got)
	}
}

func TestTmuxHandler_CreateAndRenameRejectReservedInternalSessionNames(t *testing.T) {
	argsPath := installScriptedTmux(t, "")
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")

	handler := NewTmuxHandler()
	createBody := bytes.NewBufferString(`{"name":"chrote-probe-user","unixUser":"alice"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.CreateSession(createRec, createReq)
	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("create status code = %d, expected %d; body=%s", createRec.Code, http.StatusBadRequest, createRec.Body.String())
	}

	renameBody := RenameSessionRequest{NewName: "chrote-probe-renamed"}
	renameBytes, _ := json.Marshal(renameBody)
	renameReq := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/work?unixUser=alice", bytes.NewBuffer(renameBytes))
	renameReq.SetPathValue("name", "work")
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	handler.RenameSession(renameRec, renameReq)
	if renameRec.Code != http.StatusBadRequest {
		t.Fatalf("rename status code = %d, expected %d; body=%s", renameRec.Code, http.StatusBadRequest, renameRec.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want reserved-name rejection before tmux", got)
	}
}

func TestTmuxHandler_CreateSessionReportsDuplicateNameClearly(t *testing.T) {
	argsPath := installScriptedTmux(t, `
case "$*" in
  *new-session*) echo 'duplicate session: existing-smoke' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBufferString(`{"name":"existing-smoke"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	var response core.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode API response: %v", err)
	}
	if response.Error == nil || response.Error.Code != "SESSION_NAME_CONFLICT" {
		t.Fatalf("error = %#v, want SESSION_NAME_CONFLICT", response.Error)
	}
	if !strings.Contains(response.Error.Message, "existing-smoke") || !strings.Contains(response.Error.Message, "already in use") {
		t.Fatalf("error message = %q, want name and actionable duplicate message", response.Error.Message)
	}
	for _, call := range normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)) {
		if containsArg(call, "kill-session") {
			t.Fatalf("duplicate create attempted to kill an existing session: %#v", call)
		}
	}
}

// Creating a session must land on the socket and the working directory
// configured for the chosen Unix user, and must carry the mouse policy the
// request asked for. The whole argv is compared, because a session created on
// the wrong socket is invisible to the operator who asked for it.
func TestTmuxHandler_CreateSessionUsesTheConfiguredTargetForTheChosenUser(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		sockets   string
		workdirs  string
		body      string
		wantCalls []string
	}{
		{
			name:      "a sole configured socket carries its own working directory",
			sockets:   "alice=/tmp/tmux-2002/default",
			workdirs:  "alice=/srv/terminal-three",
			body:      `{"name":"terminal-three-smoke"}`,
			wantCalls: tmuxCreationCalls("/tmp/tmux-2002/default", "terminal-three-smoke", "/srv/terminal-three", "on"),
		},
		{
			name:      "a named Unix user picks its own socket out of several",
			sockets:   "alice=/run/user/2001/chrote-tmux/tmux-1000/default,build=/tmp/tmux-2002/default",
			workdirs:  "alice=/home/operator,build=/home/secondary",
			body:      `{"name":"build-shell","unixUser":"build","mouseScroll":false}`,
			wantCalls: tmuxCreationCalls("/tmp/tmux-2002/default", "build-shell", "/home/secondary", "off"),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, argsPath := installFakeTmux(t)
			t.Setenv("CHROTE_TMUX_SOCKET", testCase.sockets)
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", testCase.workdirs)

			handler := NewTmuxHandler()
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", strings.NewReader(testCase.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			handler.CreateSession(recorder, req)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath))
			if strings.Join(got, "\x00") != strings.Join(testCase.wantCalls, "\x00") {
				t.Fatalf("tmux calls = %#v, want %#v", got, testCase.wantCalls)
			}
		})
	}
}

// The socket specification is read the way an operator writes it, spaces and
// all, because listing and attaching resolve it separately and must agree.
func TestTmuxHandler_ListSessionsAggregatesConfiguredTerminalUsers(t *testing.T) {
	tmpDir := t.TempDir()
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := `#!/bin/sh
case "$*" in
  *"/tmp/tmux-p"*) printf '$1\talice-shell\t1\t0\t/home/operator\tbash\t1\t120\t40\tlatest\t1\t\t1756900000\n' ;;
  *"/tmp/tmux-t"*) printf '$2\tbuild-shell\t2\t1\t/home/secondary\tbash\t1\t120\t40\tlatest\t1\t\t1756900000\n' ;;
  *) printf '$3\tunexpected\t1\t0\t/tmp\tbash\t1\t120\t40\tlatest\t1\t\t1756900000\n' ;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TMUX_SOCKET", " alice=/tmp/tmux-p, build = /tmp/tmux-t ")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/operator,build=/home/secondary")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()

	handler.ListSessions(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 2 {
		t.Fatalf("sessions = %+v, want two configured-user sessions", response.Sessions)
	}
	usersBySession := map[string]string{}
	for _, session := range response.Sessions {
		usersBySession[session.Name] = session.UnixUser
	}
	if usersBySession["alice-shell"] != "alice" || usersBySession["build-shell"] != "build" {
		t.Fatalf("session users = %#v, want alice-shell/alice and build-shell/build", usersBySession)
	}
	wantUsers := []string{"alice", "build"}
	if strings.Join(response.TerminalUsers, ",") != strings.Join(wantUsers, ",") {
		t.Fatalf("terminalUsers = %#v, want %#v in configured order with the padding trimmed", response.TerminalUsers, wantUsers)
	}
}

func TestTmuxHandler_ZeroMappingsReturnEmptyInventoryAndRoutingError(t *testing.T) {
	installFakeTmux(t)
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()

	handler.ListSessions(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.TerminalUsers) != 0 {
		t.Fatalf("terminalUsers = %#v, want empty inventory when CHROTE_TMUX_SOCKET is unset", response.TerminalUsers)
	}
	if len(response.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want no routed inventory with zero socket mappings", response.Sessions)
	}
	// An empty inventory serialises as an empty array and an empty object, never
	// as null: the dashboard iterates both without a guard.
	if response.Sessions == nil || response.Grouped == nil {
		t.Fatalf("sessions=%#v grouped=%#v, want empty collections rather than null", response.Sessions, response.Grouped)
	}
	if response.Timestamp == "" {
		t.Fatal("empty inventory carried no timestamp")
	}
	if _, err := handler.targetForUnixUser(""); err == nil || !strings.Contains(err.Error(), "no tmux sockets are configured") {
		t.Fatalf("zero-mapping target error = %v, want an explicit configuration error", err)
	}
}

func TestTmuxHandler_DeleteAllSessionsReportsListErrors(t *testing.T) {
	installAlwaysFailingTmux(t, "error connecting to /tmp/tmux-2002/default (Permission denied)")
	t.Setenv("CHROTE_TMUX_SOCKET", "build=/tmp/tmux-2002/default")
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all", nil)
	req.Header.Set("X-Nuke-Confirm", "DASHBOARD-NUKE-CONFIRMED")
	recorder := httptest.NewRecorder()

	handler.DeleteAllSessions(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "tmux source unavailable") || strings.Contains(recorder.Body.String(), "Permission denied") {
		t.Fatalf("body = %q, want a fail-loud redacted source error", recorder.Body.String())
	}
}

func TestTmuxHandler_SetMouseModeTargetsConfiguredTerminalUsers(t *testing.T) {
	tmpDir := t.TempDir()
	callsPath := filepath.Join(tmpDir, "tmux.calls")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + callsPath + "\nprintf '%s\\n' '---' >> " + callsPath + "\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-p,build=/tmp/tmux-t")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/operator,build=/home/secondary")

	handler := NewTmuxHandler()
	bodyBytes := []byte(`{"enabled":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["mouse"] != "off" || response["applied"] != float64(2) || response["total"] != float64(2) {
		t.Fatalf("response = %#v, want mouse off applied/total 2", response)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read fake tmux calls: %v", err)
	}
	got := string(calls)
	if strings.Count(got, "-S\n/tmp/tmux-p\nset-option\n-g\nmouse\noff\n") != 1 {
		t.Fatalf("alice mouse calls = %q, want mouse off command on /tmp/tmux-p", got)
	}
	if strings.Count(got, "-S\n/tmp/tmux-t\nset-option\n-g\nmouse\noff\n") != 1 {
		t.Fatalf("build mouse calls = %q, want mouse off command on /tmp/tmux-t", got)
	}
}

// tmux's own right-click menus sit on top of the terminal and swallow the
// browser's, so turning the mouse on has to unbind every one of them. The
// request travels through the mux, because a policy nothing routes to is a
// policy the operator never gets.
func TestTmuxHandler_SetMouseModeRemovesTmuxRightClickMenus(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	mux := http.NewServeMux()
	NewTmuxHandler().RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	want := tmuxMousePolicyCalls("/tmp/tmux-a", "on")
	if got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath)); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want mouse mode plus no right-click menus %#v", got, want)
	}
}

func TestTmuxHandler_SetMouseModeDoesNotReportPartialPolicyAsApplied(t *testing.T) {
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	argsPath := installScriptedTmux(t, `
case "$*" in
  *"unbind-key -q -n MouseDown3Status"*) echo 'unbind failed' >&2; exit 1 ;;
esac
`)
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Applied int  `json:"applied"`
		Total   int  `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Success || response.Applied != 0 || response.Total != 1 {
		t.Fatalf("response = %+v, want failed policy application with applied=0 total=1", response)
	}
	want := [][]string{
		{"-S", "/tmp/tmux-a", "set-option", "-g", "mouse", "on"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Status"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want fail-fast policy calls %#v", got, want)
	}
}

func TestTmuxHandler_CreateOwnedTmuxSessionCleansAmbiguousCreationByMarker(t *testing.T) {
	argsPath := installScriptedTmux(t, `
case "$*" in
  *new-session*)
    for arg in "$@"; do
      case "$arg" in
        CHROTE_CREATION_TOKEN=*) printf '%s' "$arg" > "$TMUX_SESSION_MARKER_FILE" ;;
      esac
    done
    echo 'context deadline exceeded after server-side creation' >&2
    exit 1
    ;;
esac
`)
	handler := NewTmuxHandler()

	_, err := handler.createOwnedTmuxSession(context.Background(), "/tmp/tmux-a", "ambiguous-smoke", "/tmp", nil)
	if err == nil || !strings.Contains(err.Error(), "tmux command failed") || strings.Contains(err.Error(), "after server-side creation") {
		t.Fatalf("create error = %v, want a redacted ambiguous creation error", err)
	}
	want := [][]string{
		{"-S", "/tmp/tmux-a", "new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "ambiguous-smoke", "-c", "/tmp"},
		{"-S", "/tmp/tmux-a", "if-shell", "-F", "-t", "ambiguous-smoke", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t ambiguous-smoke", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want marker-owned ambiguous cleanup %#v", got, want)
	}
}

func TestTmuxHandler_CreateOwnedTmuxSessionCleansMalformedIDByOwnedName(t *testing.T) {
	argsPath := installScriptedTmux(t, `
case "$*" in
  *new-session*)
    for arg in "$@"; do
      case "$arg" in
        CHROTE_CREATION_TOKEN=*) printf '%s' "$arg" > "$TMUX_SESSION_MARKER_FILE" ;;
      esac
    done
    printf 'not-a-session-id\n'
    exit 0
    ;;
esac
`)
	handler := NewTmuxHandler()

	_, err := handler.createOwnedTmuxSession(context.Background(), "/tmp/tmux-a", "malformed-id-smoke", "/tmp", nil)
	if err == nil || !strings.Contains(err.Error(), "without a valid session ID") {
		t.Fatalf("create error = %v, want malformed session ID error", err)
	}
	want := [][]string{
		{"-S", "/tmp/tmux-a", "new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "malformed-id-smoke", "-c", "/tmp"},
		{"-S", "/tmp/tmux-a", "if-shell", "-F", "-t", "malformed-id-smoke", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t malformed-id-smoke", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want name fallback for malformed ID %#v", got, want)
	}
}

func TestTmuxHandler_CreateOwnedTmuxSessionRefusesToCleanUnownedName(t *testing.T) {
	argsPath := installScriptedTmux(t, `
case "$*" in
  *new-session*) echo 'duplicate session' >&2; exit 1 ;;
  *if-shell*) printf 'CHROTE_OWNERSHIP_MISMATCH\n'; exit 0 ;;
esac
`)
	handler := NewTmuxHandler()

	_, err := handler.createOwnedTmuxSession(context.Background(), "/tmp/tmux-a", "existing-smoke", "/tmp", nil)
	if err == nil || !strings.Contains(err.Error(), "creation token does not match") {
		t.Fatalf("create error = %v, want ownership mismatch joined to create failure", err)
	}
	got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if len(got) != 2 || !containsArg(got[0], "new-session") || !containsArg(got[1], "if-shell") {
		t.Fatalf("tmux calls = %#v, want create plus ownership check and no kill", got)
	}
}

func TestTmuxHandler_CleanupOwnedTmuxSessionJoinsKillFailure(t *testing.T) {
	argsPath := installScriptedTmux(t, `
case "$*" in
  *if-shell*) echo 'kill denied' >&2; exit 1 ;;
esac
`)
	handler := NewTmuxHandler()
	cause := errors.New("policy failed")

	err := handler.cleanupOwnedTmuxSessionAfterError("/tmp/tmux-a", ownedTmuxSession{ID: "$42", Name: "owned-smoke", Token: "0123456789abcdef01234567"}, cause)
	if err == nil || !strings.Contains(err.Error(), "policy failed") || !strings.Contains(err.Error(), "tmux command failed") || strings.Contains(err.Error(), "kill denied") {
		t.Fatalf("cleanup error = %v, want original and redacted cleanup failures joined", err)
	}
	want := [][]string{
		{"-S", "/tmp/tmux-a", "if-shell", "-F", "-t", "$42", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t $42", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want ID-based verified cleanup %#v", got, want)
	}
}

func readArgvRecordingTmuxCalls(t *testing.T, argsPath string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read tmux argv log: %v", err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	calls := [][]string{}
	current := []string{}
	for _, line := range lines {
		if line == "---" {
			if len(current) > 0 {
				calls = append(calls, current)
				current = []string{}
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		calls = append(calls, current)
	}
	return calls
}

func equalArgvCalls(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.Join(a[i], "\x00") != strings.Join(b[i], "\x00") {
			return false
		}
	}
	return true
}

func containsArg(call []string, want string) bool {
	for _, arg := range call {
		if arg == want {
			return true
		}
	}
	return false
}

var (
	tmuxCreationTokenTestPattern = regexp.MustCompile(`CHROTE_CREATION_TOKEN=[0-9a-f]+`)
	tmuxRawTokenTestPattern      = regexp.MustCompile(`\b[0-9a-f]{24}\b`)
)

func normalizeTmuxCreationToken(value string) string {
	value = tmuxCreationTokenTestPattern.ReplaceAllString(value, "CHROTE_CREATION_TOKEN=<token>")
	return tmuxRawTokenTestPattern.ReplaceAllString(value, "<token>")
}

func normalizeFakeTmuxCreationTokens(calls []string) []string {
	normalized := make([]string, len(calls))
	for i, call := range calls {
		normalized[i] = normalizeTmuxCreationToken(call)
	}
	return normalized
}

func normalizeArgvTmuxCreationTokens(calls [][]string) [][]string {
	normalized := make([][]string, len(calls))
	for i, call := range calls {
		normalized[i] = make([]string, len(call))
		for j, arg := range call {
			normalized[i][j] = normalizeTmuxCreationToken(arg)
		}
	}
	return normalized
}

func installScriptedTmux(t *testing.T, behavior string) string {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-argv.txt")
	scriptPath := filepath.Join(dir, "tmux")
	markerPath := filepath.Join(dir, "tmux-session-marker.txt")
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$TMUX_ARGS_FILE"
done
printf '%s\n' '---' >> "$TMUX_ARGS_FILE"
` + behavior + `
case "$*" in
  *new-session*)
    for arg in "$@"; do
      case "$arg" in
        CHROTE_CREATION_TOKEN=*) printf '%s' "$arg" > "$TMUX_SESSION_MARKER_FILE" ;;
      esac
    done
    printf '$42\n'
    ;;
  *if-shell*)
    if [ -f "$TMUX_SESSION_MARKER_FILE" ]; then
      rm -f "$TMUX_SESSION_MARKER_FILE"
    else
      echo "can't find session" >&2
      exit 1
    fi
    ;;
esac
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
		t.Fatalf("write args log: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("TMUX_SESSION_MARKER_FILE", markerPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsPath
}

func TestAPIEnvelopeContract_FlatTmuxEndpointsDoNotUseDataEnvelope(t *testing.T) {
	installFakeTmux(t)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	handler := NewTmuxHandler()

	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		pathName  string
		headerKey string
		headerVal string
		call      func(http.ResponseWriter, *http.Request)
		wantKeys  []string
	}{
		{
			name:     "list sessions",
			method:   http.MethodGet,
			path:     "/api/tmux/sessions",
			call:     handler.ListSessions,
			wantKeys: []string{"grouped", "sessions", "terminalUsers", "timestamp"},
		},
		{
			name:     "create session",
			method:   http.MethodPost,
			path:     "/api/tmux/sessions",
			body:     `{"name":"baseline-session"}`,
			call:     handler.CreateSession,
			wantKeys: []string{"cwd", "flags", "harness", "notify", "session", "success", "timestamp"},
		},
		{
			name:     "delete session",
			method:   http.MethodDelete,
			path:     "/api/tmux/sessions/baseline-session",
			pathName: "baseline-session",
			call:     handler.DeleteSession,
			wantKeys: []string{"killed", "success", "timestamp"},
		},
		{
			name:      "delete all sessions",
			method:    http.MethodDelete,
			path:      "/api/tmux/sessions/all",
			headerKey: "X-Nuke-Confirm",
			headerVal: "DASHBOARD-NUKE-CONFIRMED",
			call:      handler.DeleteAllSessions,
			wantKeys:  []string{"killed", "protected", "sessions", "success", "timestamp"},
		},
		{
			name:     "rename session",
			method:   http.MethodPatch,
			path:     "/api/tmux/sessions/old-session",
			pathName: "old-session",
			body:     `{"newName":"new-session"}`,
			call:     handler.RenameSession,
			wantKeys: []string{"newName", "oldName", "success", "timestamp"},
		},
		{
			name:     "capture pane",
			method:   http.MethodGet,
			path:     "/api/tmux/sessions/baseline-session/capture",
			pathName: "baseline-session",
			call:     handler.CapturePane,
			wantKeys: []string{"content", "session"},
		},
		{
			name:     "set mouse mode",
			method:   http.MethodPost,
			path:     "/api/tmux/mouse",
			body:     `{"enabled":true}`,
			call:     handler.SetMouseMode,
			wantKeys: []string{"applied", "mouse", "success", "timestamp", "total"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tt.pathName != "" {
				req.SetPathValue("name", tt.pathName)
			}
			if tt.headerKey != "" {
				req.Header.Set(tt.headerKey, tt.headerVal)
			}
			rec := httptest.NewRecorder()

			tt.call(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			response := decodeJSONMap(t, rec)
			assertTopLevelKeys(t, response, tt.wantKeys)
			assertNoTopLevelKey(t, response, "data")
		})
	}
}

func TestTmuxHandler_DeleteAllSessionsRequiresExactNukeConfirmationHeader(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	handler := NewTmuxHandler()

	tests := []struct {
		name        string
		headerValue string
		wantStatus  int
		wantCalls   []string
	}{
		{
			name:       "missing confirmation",
			wantStatus: http.StatusForbidden,
			wantCalls:  nil,
		},
		{
			name:        "wrong confirmation",
			headerValue: "DASHBOARD-NUKE",
			wantStatus:  http.StatusForbidden,
			wantCalls:   nil,
		},
		{
			name:        "wrong case confirmation",
			headerValue: "dashboard-nuke-confirmed",
			wantStatus:  http.StatusForbidden,
			wantCalls:   nil,
		},
		{
			name:        "confirmation with trailing space",
			headerValue: "DASHBOARD-NUKE-CONFIRMED ",
			wantStatus:  http.StatusForbidden,
			wantCalls:   nil,
		},
		{
			name:        "exact confirmation",
			headerValue: "DASHBOARD-NUKE-CONFIRMED",
			wantStatus:  http.StatusOK,
			wantCalls: []string{
				"-S /tmp/tmux-a list-sessions -F #{session_name}",
				"-S /tmp/tmux-a kill-session -t alpha",
				"-S /tmp/tmux-a kill-session -t beta",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(argsPath, nil, 0600); err != nil {
				t.Fatalf("reset tmux args: %v", err)
			}
			req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all", nil)
			if tt.headerValue != "" {
				req.Header.Set("X-Nuke-Confirm", tt.headerValue)
			}
			rec := httptest.NewRecorder()

			handler.DeleteAllSessions(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if gotCalls := readFakeCommandCalls(t, argsPath); !reflect.DeepEqual(gotCalls, tt.wantCalls) {
				t.Fatalf("tmux calls = %#v, want %#v", gotCalls, tt.wantCalls)
			}
		})
	}
}

func installFakeTmux(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-args.txt")
	scriptPath := filepath.Join(dir, "tmux")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TMUX_ARGS_FILE"
socket=""
if [ "${1:-}" = "-S" ]; then socket="$2"; shift 2; fi
case "$*" in
  *"list-sessions -F #{session_id}"*)
	printf 'no server running on %s\n' "$socket" >&2
    exit 1
    ;;
  "list-sessions -F #{session_name}:#{session_windows}:#{session_attached}")
    printf 'alpha:1:0\nbeta:2:1\n'
    ;;
  "list-sessions -F #{session_name}")
    printf 'alpha\nbeta\n'
    ;;
  capture-pane*)
    printf 'line one\nline two\n'
    ;;
  *new-session*)
    printf '$42\n'
    ;;
  *)
    printf ''
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake tmux command: %v", err)
	}
	if err := os.WriteFile(argsPath, nil, 0600); err != nil {
		t.Fatalf("write fake tmux args file: %v", err)
	}

	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return scriptPath, argsPath
}

func readFakeCommandCalls(t *testing.T, argsPath string) []string {
	t.Helper()

	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake command calls: %v", err)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	return strings.Split(string(raw), "\n")
}

func decodeJSONMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON response: %v\nbody: %s", err, rec.Body.String())
	}
	return response
}

func assertTopLevelKeys(t *testing.T, response map[string]interface{}, want []string) {
	t.Helper()

	got := make([]string, 0, len(response))
	for key := range response {
		got = append(got, key)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("top-level keys = %#v, want %#v", got, want)
	}
}

func assertNoTopLevelKey(t *testing.T, response map[string]interface{}, key string) {
	t.Helper()

	if _, ok := response[key]; ok {
		t.Fatalf("unexpected top-level %q key in response %#v", key, response)
	}
}

// tmuxCreationCalls is the whole argv a create produces: the detached session,
// the control-mode attach that keeps it alive, and the mouse policy.
func tmuxCreationCalls(socket, session, workdir, mouse string) []string {
	calls := []string{
		"-S " + socket + " new-session -d -P -F #{session_id} -e CHROTE_CREATION_TOKEN=<token> -s " + session + " -c " + workdir,
		"-S " + socket + " -C attach-session -t $42",
	}
	return append(calls, tmuxMousePolicyCalls(socket, mouse)...)
}

// tmuxMousePolicyCalls is the mouse setting plus every right-click binding that
// has to go, in the order CHROTE issues them.
func tmuxMousePolicyCalls(socket, mouse string) []string {
	calls := []string{"-S " + socket + " set-option -g mouse " + mouse}
	for _, binding := range []string{
		"MouseDown3Pane", "MouseDown3Status", "MouseDown3StatusLeft",
		"M-MouseDown3Pane", "M-MouseDown3Status", "M-MouseDown3StatusLeft",
	} {
		calls = append(calls, "-S "+socket+" unbind-key -q -n "+binding)
	}
	return calls
}
