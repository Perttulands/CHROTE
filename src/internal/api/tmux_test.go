package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	osuser "os/user"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/chrote/server/internal/core"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestTmuxHandler_NewTmuxHandler(t *testing.T) {
	handler := NewTmuxHandler()

	if handler == nil {
		t.Fatal("NewTmuxHandler() returned nil")
	}
	if handler.cache == nil {
		t.Error("Handler cache is nil")
	}
	if handler.colorRegex == nil {
		t.Error("Handler colorRegex is nil")
	}
}

func TestTmuxHandler_RunTmuxBoundsAggregateOutput(t *testing.T) {
	installScriptedTmux(t, `
yes x | head -c 1049600
`)
	_, err := NewTmuxHandler().runTmuxOnSocket("/tmp/tmux-a", "list-sessions")
	if !errors.Is(err, errTmuxCommandOutputLimit) {
		t.Fatalf("error = %v, want bounded-output failure", err)
	}
}

func TestTmuxHandler_ValidateColor(t *testing.T) {
	handler := NewTmuxHandler()

	tests := []struct {
		name    string
		color   string
		isValid bool
	}{
		{"hex 3 digit", "#fff", true},
		{"hex 6 digit", "#ff00ff", true},
		{"named color", "red", true},
		{"named color blue", "blue", true},
		{"default", "default", true},
		{"invalid hex", "#gggggg", false},
		{"invalid chars", "red@blue", false},
		{"empty is not matched by regex but handled separately", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.colorRegex.MatchString(tt.color)
			if result != tt.isValid {
				t.Errorf("colorRegex.MatchString(%q) = %v, expected %v", tt.color, result, tt.isValid)
			}
		})
	}
}

func TestTmuxHandler_CreateSession_InvalidJSON(t *testing.T) {
	handler := NewTmuxHandler()

	// Test with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBufferString("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_CreateSession_InvalidName(t *testing.T) {
	handler := NewTmuxHandler()

	body := CreateSessionRequest{Name: "invalid name with spaces"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}

	var response map[string]interface{}
	json.Unmarshal(recorder.Body.Bytes(), &response)

	if response["success"] != false {
		t.Error("Response should indicate failure")
	}
}

func TestTmuxHandler_DeleteSession_InvalidName(t *testing.T) {
	handler := NewTmuxHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/invalid@name", nil)
	req.SetPathValue("name", "invalid@name")
	recorder := httptest.NewRecorder()

	handler.DeleteSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_DeleteAllSessions_NoConfirmHeader(t *testing.T) {
	handler := NewTmuxHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all", nil)
	// Intentionally NOT setting X-Nuke-Confirm header
	recorder := httptest.NewRecorder()

	handler.DeleteAllSessions(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("Status code = %d, expected %d (Forbidden)", recorder.Code, http.StatusForbidden)
	}

	var response map[string]interface{}
	json.Unmarshal(recorder.Body.Bytes(), &response)

	if response["success"] != false {
		t.Error("Response should indicate failure")
	}
}

func TestTmuxHandler_RenameSession_InvalidNewName(t *testing.T) {
	handler := NewTmuxHandler()

	body := RenameSessionRequest{NewName: "invalid name!"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/oldsession", bytes.NewBuffer(bodyBytes))
	req.SetPathValue("name", "oldsession")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.RenameSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_ListSessionsFiltersReservedProbeSessions(t *testing.T) {
	installScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$1\tchrote-probe-123\t1\t0\t/workspaces/alice\n$2\twork\t1\t0\t/workspaces/alice\n' ;;
esac
`)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
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
  *pane_current_path*) printf '$9\twork\t1\t0\t/workspaces/alice/live\tcodex\n$10\tno-cwd\t1\t0\t\tbash\n' ;;
esac
`)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
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
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
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

func TestTmuxHandler_ApplyAppearance_InvalidColor(t *testing.T) {
	handler := NewTmuxHandler()

	body := AppearanceRequest{
		StatusBg: "invalidcolor@#$",
		StatusFg: "red",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tmux/appearance", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ApplyAppearance(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_RegisterRoutes(t *testing.T) {
	handler := NewTmuxHandler()
	mux := http.NewServeMux()

	// This should not panic
	handler.RegisterRoutes(mux)
}

func TestTmuxHandler_ListSessions_ReturnsValidJSON(t *testing.T) {
	handler := NewTmuxHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()

	handler.ListSessions(recorder, req)

	// Should return valid JSON even if tmux isn't running
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	// Should have a timestamp
	if response.Timestamp == "" {
		t.Error("Response should include timestamp")
	}

	// Sessions should be initialized (not nil)
	if response.Sessions == nil {
		t.Error("Sessions should be initialized slice, not nil")
	}

	// Grouped should be initialized (not nil)
	if response.Grouped == nil {
		t.Error("Grouped should be initialized map, not nil")
	}
}

func TestTmuxHandler_DefaultProfileUsesConfiguredSocketAndWorkDir(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "/tmp/tmux-2002/default")
	t.Setenv("CHROTE_DEFAULT_TMUX_WORKDIR", "/srv/terminal-three")

	handler := NewTmuxHandler()
	body := CreateSessionRequest{Name: "terminal-three-smoke"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath))
	want := []string{
		"-S /tmp/tmux-2002/default new-session -d -P -F #{session_id} -e CHROTE_CREATION_TOKEN=<token> -s terminal-three-smoke -c /srv/terminal-three",
		"-S /tmp/tmux-2002/default set-option -g mouse on",
		"-S /tmp/tmux-2002/default unbind-key -q -n MouseDown3Pane",
		"-S /tmp/tmux-2002/default unbind-key -q -n MouseDown3Status",
		"-S /tmp/tmux-2002/default unbind-key -q -n MouseDown3StatusLeft",
		"-S /tmp/tmux-2002/default unbind-key -q -n M-MouseDown3Pane",
		"-S /tmp/tmux-2002/default unbind-key -q -n M-MouseDown3Status",
		"-S /tmp/tmux-2002/default unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_CurrentUnixUserHonorsConfiguredDefaultTarget(t *testing.T) {
	current, err := osuser.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "/configured/current-user.sock")
	t.Setenv("CHROTE_DEFAULT_TMUX_WORKDIR", "/srv/current-user")
	t.Setenv("CHROTE_TERMINAL_USERS", current.Username)

	handler := NewTmuxHandler()
	body := CreateSessionRequest{Name: "current-user-smoke", UnixUser: current.Username}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath))
	want := []string{
		"-S /configured/current-user.sock new-session -d -P -F #{session_id} -e CHROTE_CREATION_TOKEN=<token> -s current-user-smoke -c /srv/current-user",
		"-S /configured/current-user.sock set-option -g mouse on",
		"-S /configured/current-user.sock unbind-key -q -n MouseDown3Pane",
		"-S /configured/current-user.sock unbind-key -q -n MouseDown3Status",
		"-S /configured/current-user.sock unbind-key -q -n MouseDown3StatusLeft",
		"-S /configured/current-user.sock unbind-key -q -n M-MouseDown3Pane",
		"-S /configured/current-user.sock unbind-key -q -n M-MouseDown3Status",
		"-S /configured/current-user.sock unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_CreateSessionUsesSelectedUnixUserTarget(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/run/user/2001/chrote-tmux/tmux-1000/default,build=/tmp/tmux-2002/default")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/operator,build=/home/secondary")

	handler := NewTmuxHandler()
	bodyBytes := []byte(`{"name":"build-shell","unixUser":"build","mouseScroll":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath))
	want := []string{
		"-S /tmp/tmux-2002/default new-session -d -P -F #{session_id} -e CHROTE_CREATION_TOKEN=<token> -s build-shell -c /home/secondary",
		"-S /tmp/tmux-2002/default set-option -g mouse off",
		"-S /tmp/tmux-2002/default unbind-key -q -n MouseDown3Pane",
		"-S /tmp/tmux-2002/default unbind-key -q -n MouseDown3Status",
		"-S /tmp/tmux-2002/default unbind-key -q -n MouseDown3StatusLeft",
		"-S /tmp/tmux-2002/default unbind-key -q -n M-MouseDown3Pane",
		"-S /tmp/tmux-2002/default unbind-key -q -n M-MouseDown3Status",
		"-S /tmp/tmux-2002/default unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_ListSessionsAggregatesConfiguredTerminalUsers(t *testing.T) {
	tmpDir := t.TempDir()
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := `#!/bin/sh
case "$*" in
  *"/tmp/tmux-p"*) printf '$1\talice-shell\t1\t0\t/home/operator\n' ;;
  *"/tmp/tmux-t"*) printf '$2\tbuild-shell\t2\t1\t/home/secondary\n' ;;
  *) printf '$3\tunexpected\t1\t0\t/tmp\n' ;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-p,build=/tmp/tmux-t")
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
		t.Fatalf("terminalUsers = %#v, want %#v", response.TerminalUsers, wantUsers)
	}
}

func TestTmuxHandler_ListSessionsReturnsTrimmedDedupedTerminalUsers(t *testing.T) {
	installFakeTmux(t)
	t.Setenv("CHROTE_TERMINAL_USERS", " alice, bob , alice ,,bob ")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a,bob=/tmp/tmux-b")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice,bob=/home/bob")

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
	want := []string{"alice", "bob"}
	if strings.Join(response.TerminalUsers, ",") != strings.Join(want, ",") {
		t.Fatalf("terminalUsers = %#v, want %#v", response.TerminalUsers, want)
	}
}

func TestTmuxHandler_ListSessionsDoesNotAdvertiseImplicitCurrentUser(t *testing.T) {
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
		t.Fatalf("terminalUsers = %#v, want empty when CHROTE_TERMINAL_USERS is unset", response.TerminalUsers)
	}
	for _, session := range response.Sessions {
		if session.UnixUser != "" {
			t.Fatalf("session %q UnixUser = %q, want bare default-session identity", session.Name, session.UnixUser)
		}
	}
}

func TestTmuxHandler_DeleteAllSessionsReportsListErrors(t *testing.T) {
	installFailingTmux(t, "error connecting to /tmp/tmux-2002/default (Permission denied)")
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

func TestTmuxHandler_ApplyAppearanceTargetsConfiguredTerminalUsers(t *testing.T) {
	tmpDir := t.TempDir()
	callsPath := filepath.Join(tmpDir, "tmux.calls")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + callsPath + "\nprintf '%s\\n' '---' >> " + callsPath + "\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-p,build=/tmp/tmux-t")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/operator,build=/home/secondary")

	handler := NewTmuxHandler()
	bodyBytes := []byte(`{"statusBg":"default","statusFg":"#ffffff","paneBorderActive":"#ff00ff"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/appearance", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ApplyAppearance(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read fake tmux calls: %v", err)
	}
	got := string(calls)
	if strings.Count(got, "-S\n/tmp/tmux-p\nset\n-g\n") != 2 {
		t.Fatalf("alice appearance calls = %q, want two commands on /tmp/tmux-p", got)
	}
	if strings.Count(got, "-S\n/tmp/tmux-t\nset\n-g\n") != 2 {
		t.Fatalf("build appearance calls = %q, want two commands on /tmp/tmux-t", got)
	}
	if strings.Contains(got, "statusBg") {
		t.Fatalf("tmux calls leaked JSON keys instead of set args: %q", got)
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
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-p,build=/tmp/tmux-t")
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

func TestTmuxHandler_SetMouseModeRemovesTmuxRightClickMenus(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	want := []string{
		"set-option -g mouse on",
		"unbind-key -q -n MouseDown3Pane",
		"unbind-key -q -n MouseDown3Status",
		"unbind-key -q -n MouseDown3StatusLeft",
		"unbind-key -q -n M-MouseDown3Pane",
		"unbind-key -q -n M-MouseDown3Status",
		"unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath)); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want mouse mode plus no right-click menus %#v", got, want)
	}
}

func TestTmuxHandler_SetMouseModeDoesNotReportPartialPolicyAsApplied(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "")
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
		{"set-option", "-g", "mouse", "on"},
		{"unbind-key", "-q", "-n", "MouseDown3Pane"},
		{"unbind-key", "-q", "-n", "MouseDown3Status"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want fail-fast policy calls %#v", got, want)
	}
}

func TestTmuxHandler_CreateOwnedTmuxSessionCleansAmbiguousCreationByMarker(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
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

	_, err := handler.createOwnedTmuxSession(context.Background(), "", "ambiguous-smoke", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "tmux command failed") || strings.Contains(err.Error(), "after server-side creation") {
		t.Fatalf("create error = %v, want a redacted ambiguous creation error", err)
	}
	want := [][]string{
		{"new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "ambiguous-smoke", "-c", "/tmp"},
		{"if-shell", "-F", "-t", "ambiguous-smoke", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t ambiguous-smoke", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want marker-owned ambiguous cleanup %#v", got, want)
	}
}

func TestTmuxHandler_CreateOwnedTmuxSessionCleansMalformedIDByOwnedName(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
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

	_, err := handler.createOwnedTmuxSession(context.Background(), "", "malformed-id-smoke", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "without a valid session ID") {
		t.Fatalf("create error = %v, want malformed session ID error", err)
	}
	want := [][]string{
		{"new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "malformed-id-smoke", "-c", "/tmp"},
		{"if-shell", "-F", "-t", "malformed-id-smoke", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t malformed-id-smoke", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want name fallback for malformed ID %#v", got, want)
	}
}

func TestTmuxHandler_CreateOwnedTmuxSessionRefusesToCleanUnownedName(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
	argsPath := installScriptedTmux(t, `
case "$*" in
  *new-session*) echo 'duplicate session' >&2; exit 1 ;;
  *if-shell*) printf 'CHROTE_OWNERSHIP_MISMATCH\n'; exit 0 ;;
esac
`)
	handler := NewTmuxHandler()

	_, err := handler.createOwnedTmuxSession(context.Background(), "", "existing-smoke", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "creation token does not match") {
		t.Fatalf("create error = %v, want ownership mismatch joined to create failure", err)
	}
	got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if len(got) != 2 || got[0][0] != "new-session" || got[1][0] != "if-shell" {
		t.Fatalf("tmux calls = %#v, want create plus ownership check and no kill", got)
	}
}

func TestTmuxHandler_CleanupOwnedTmuxSessionJoinsKillFailure(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
	argsPath := installScriptedTmux(t, `
case "$*" in
  *if-shell*) echo 'kill denied' >&2; exit 1 ;;
esac
`)
	handler := NewTmuxHandler()
	cause := errors.New("policy failed")

	err := handler.cleanupOwnedTmuxSessionAfterError("", ownedTmuxSession{ID: "$42", Name: "owned-smoke", Token: "0123456789abcdef01234567"}, cause)
	if err == nil || !strings.Contains(err.Error(), "policy failed") || !strings.Contains(err.Error(), "tmux command failed") || strings.Contains(err.Error(), "kill denied") {
		t.Fatalf("cleanup error = %v, want original and redacted cleanup failures joined", err)
	}
	want := [][]string{
		{"if-shell", "-F", "-t", "$42", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t $42", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want ID-based verified cleanup %#v", got, want)
	}
}

func TestTmuxHandler_SetMouseModeRejectsInvalidJSON(t *testing.T) {
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestTmuxHandler_SetMouseModeRejectsMissingEnabled(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no side effect for missing enabled", got)
	}
}

func TestTmuxHandler_RegisterRoutesWiresMouseMode(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	want := []string{
		"set-option -g mouse on",
		"unbind-key -q -n MouseDown3Pane",
		"unbind-key -q -n MouseDown3Status",
		"unbind-key -q -n MouseDown3StatusLeft",
		"unbind-key -q -n M-MouseDown3Pane",
		"unbind-key -q -n M-MouseDown3Status",
		"unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath)); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_SendToSessionStoresDropAndPastesViaBuffer(t *testing.T) {
	h := newSendHarness(t, "$7\talice-shell\t%42\t111\t9001\n")
	dropsDir := h.dropsDir
	argsPath := h.tmuxLog

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("text", "Please inspect this screenshot."); err != nil {
		t.Fatalf("write text field: %v", err)
	}
	if err := writer.WriteField("submit", "true"); err != nil {
		t.Fatalf("write submit field: %v", err)
	}
	if err := writer.WriteField("unixUser", "alice"); err != nil {
		t.Fatalf("write unixUser field: %v", err)
	}
	fileWriter, err := writer.CreateFormFile("files", "../clipboard image.png")
	if err != nil {
		t.Fatalf("create file field: %v", err)
	}
	if _, err := fileWriter.Write([]byte("fake png")); err != nil {
		t.Fatalf("write file payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/alice-shell/send", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != true || response["session"] != "alice-shell" || response["unixUser"] != "alice" || response["pane"] != "%42" || response["submitKeyDispatched"] != true {
		t.Fatalf("send response = %#v", response)
	}
	dropPath, _ := response["dropPath"].(string)
	if dropPath == "" || !strings.HasPrefix(dropPath, dropsDir) {
		t.Fatalf("dropPath = %q, want under %q", dropPath, dropsDir)
	}
	for _, rel := range []string{"manifest.json", "text.txt", "payload.txt", filepath.Join("files", "clipboard-image.png")} {
		if _, err := os.Stat(filepath.Join(dropPath, rel)); err != nil {
			t.Fatalf("expected drop file %s: %v", rel, err)
		}
	}
	payload, err := os.ReadFile(filepath.Join(dropPath, "payload.txt"))
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	filePath := filepath.Join(dropPath, "files", "clipboard-image.png")
	payloadText := string(payload)
	if !strings.Contains(payloadText, "Please inspect this screenshot.") || !strings.Contains(payloadText, "CHROTE stored this send at:") || !strings.Contains(payloadText, dropPath) || !strings.Contains(payloadText, "Files:") || !strings.Contains(payloadText, filePath) {
		t.Fatalf("payload = %q, want text, drop path %q, and stored file path %q", payloadText, dropPath, filePath)
	}
	if strings.HasSuffix(payloadText, "\n") {
		t.Fatalf("payload has trailing newline; submit=false must not press Enter implicitly: %q", payloadText)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat stored file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("stored file owner-only base mode = %o, want 600 with fake ACL", info.Mode().Perm())
	}

	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	joined := make([]string, 0, len(calls))
	for _, call := range calls {
		joined = append(joined, strings.Join(call, "\x00"))
	}
	wantSnippets := []string{
		strings.Join([]string{"-S", "/tmp/tmux-a", "load-buffer"}, "\x00"),
		strings.Join([]string{"-S", "/tmp/tmux-a", "if-shell", "-F", "-t", "%42"}, "\x00"),
		"paste-buffer -p -d -b chrote-send-",
		"send-keys -t %42 Enter",
		atomicSendSubmitKeyMarker,
	}
	for _, snippet := range wantSnippets {
		found := false
		for _, call := range joined {
			if strings.Contains(call, snippet) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("tmux calls = %#v, missing %q", calls, snippet)
		}
	}
	for _, call := range joined {
		if strings.Contains(call, "Please inspect this screenshot") {
			t.Fatalf("bulk text leaked into tmux argv instead of buffer file: %#v", calls)
		}
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
