package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	osuser "os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrote/server/internal/core"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "chrote-tmux-tests-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("CHROTE_SESSION_BANK_PATH", filepath.Join(dir, "sessions.json"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
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
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "/tmp/tmux-1001/default")
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
	got := readFakeCommandCalls(t, argsPath)
	want := []string{
		"-S /tmp/tmux-1001/default new-session -d -s terminal-three-smoke -c /srv/terminal-three",
		"-S /tmp/tmux-1001/default set-option -g mouse on",
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
	got := readFakeCommandCalls(t, argsPath)
	want := []string{
		"-S /configured/current-user.sock new-session -d -s current-user-smoke -c /srv/current-user",
		"-S /configured/current-user.sock set-option -g mouse on",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_CreateSessionUsesSelectedUnixUserTarget(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_TERMINAL_USERS", "perttu,tavern")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu=/run/user/1000/chrote-tmux/tmux-1000/default,tavern=/tmp/tmux-1001/default")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/perttu,tavern=/home/tavern")

	handler := NewTmuxHandler()
	bodyBytes := []byte(`{"name":"tavern-shell","unixUser":"tavern","mouseScroll":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	got := readFakeCommandCalls(t, argsPath)
	want := []string{
		"-S /tmp/tmux-1001/default new-session -d -s tavern-shell -c /home/tavern",
		"-S /tmp/tmux-1001/default set-option -g mouse off",
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
  *"/tmp/tmux-p"*) printf 'perttu-shell:1:0\n' ;;
  *"/tmp/tmux-t"*) printf 'tavern-shell:2:1\n' ;;
  *) printf 'unexpected:1:0\n' ;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TERMINAL_USERS", "perttu,tavern")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu=/tmp/tmux-p,tavern=/tmp/tmux-t")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/perttu,tavern=/home/tavern")

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
	if usersBySession["perttu-shell"] != "perttu" || usersBySession["tavern-shell"] != "tavern" {
		t.Fatalf("session users = %#v, want perttu-shell/perttu and tavern-shell/tavern", usersBySession)
	}
	wantUsers := []string{"perttu", "tavern"}
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
	installFailingTmux(t, "error connecting to /tmp/tmux-1001/default (Permission denied)")
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all", nil)
	req.Header.Set("X-Nuke-Confirm", "DASHBOARD-NUKE-CONFIRMED")
	recorder := httptest.NewRecorder()

	handler.DeleteAllSessions(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Permission denied") {
		t.Fatalf("body = %q, want permission error to fail loud", recorder.Body.String())
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
	t.Setenv("CHROTE_TERMINAL_USERS", "perttu,tavern")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu=/tmp/tmux-p,tavern=/tmp/tmux-t")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/perttu,tavern=/home/tavern")

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
		t.Fatalf("perttu appearance calls = %q, want two commands on /tmp/tmux-p", got)
	}
	if strings.Count(got, "-S\n/tmp/tmux-t\nset\n-g\n") != 2 {
		t.Fatalf("tavern appearance calls = %q, want two commands on /tmp/tmux-t", got)
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
	t.Setenv("CHROTE_TERMINAL_USERS", "perttu,tavern")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu=/tmp/tmux-p,tavern=/tmp/tmux-t")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/perttu,tavern=/home/tavern")

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
		t.Fatalf("perttu mouse calls = %q, want mouse off command on /tmp/tmux-p", got)
	}
	if strings.Count(got, "-S\n/tmp/tmux-t\nset-option\n-g\nmouse\noff\n") != 1 {
		t.Fatalf("tavern mouse calls = %q, want mouse off command on /tmp/tmux-t", got)
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
	if got := readFakeCommandCalls(t, argsPath); len(got) != 0 {
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
	want := []string{"set-option -g mouse on"}
	if got := readFakeCommandCalls(t, argsPath); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_SessionBankDoesNotMutateOnFilteredScans(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := `#!/bin/sh
case "$*" in
  *"/tmp/tmux-a"*list-sessions*) printf '$1:codex-alpha:1:0\n' ;;
  *"/tmp/tmux-b"*list-sessions*) printf '$2:claude-beta:1:0\n' ;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,bob")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a,bob=/tmp/tmux-b")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice,bob=/home/bob")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("full scan status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions?unixUser=alice", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("filtered scan status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var filtered SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decode filtered response: %v", err)
	}
	var bob *SessionBankEntry
	for i := range filtered.Banked {
		if filtered.Banked[i].Name == "claude-beta" && filtered.Banked[i].UnixUser == "bob" {
			bob = &filtered.Banked[i]
			break
		}
	}
	if bob == nil {
		t.Fatalf("filtered bank = %+v, want existing bob entry preserved", filtered.Banked)
	}
	if !bob.Live {
		t.Fatalf("filtered bank bob entry = %+v, want live unchanged by alice-only scan", *bob)
	}
}

func TestTmuxHandler_SessionBankKeepsRestartResumeHints(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "offline")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\ncase \"$*\" in\n  *list-sessions*)\n    if [ ! -f " + statePath + " ]; then printf '$7:codex-alpha:1:0\\n'; fi\n    ;;\nesac\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var liveResponse SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &liveResponse); err != nil {
		t.Fatalf("decode live response: %v", err)
	}
	if len(liveResponse.Banked) != 1 || !liveResponse.Banked[0].Live {
		t.Fatalf("banked live response = %+v, want one live banked session", liveResponse.Banked)
	}
	if liveResponse.Banked[0].ID != "$7" {
		t.Fatalf("banked session id = %q, want tmux id", liveResponse.Banked[0].ID)
	}
	if liveResponse.Banked[0].ResumeCommand != "/resume codex-alpha" {
		t.Fatalf("resume command = %q, want /resume codex-alpha", liveResponse.Banked[0].ResumeCommand)
	}

	rawBank, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatalf("read bank file: %v", err)
	}
	tampered := strings.Replace(string(rawBank), `"resumeCommand": "/resume codex-alpha"`, `"resumeCommand": "rm -rf /"`, 1)
	if tampered == string(rawBank) {
		t.Fatalf("bank fixture did not contain expected resume command: %s", rawBank)
	}
	if err := os.WriteFile(bankPath, []byte(tampered), 0o660); err != nil {
		t.Fatalf("tamper bank file: %v", err)
	}

	if err := os.WriteFile(statePath, []byte("offline"), 0o644); err != nil {
		t.Fatalf("write offline state: %v", err)
	}
	handler = NewTmuxHandler()
	recorder = httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	var offlineResponse SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &offlineResponse); err != nil {
		t.Fatalf("decode offline response: %v", err)
	}
	if len(offlineResponse.Sessions) != 0 {
		t.Fatalf("live sessions = %+v, want none after restart/offline scan", offlineResponse.Sessions)
	}
	if len(offlineResponse.Banked) != 1 || offlineResponse.Banked[0].Live {
		t.Fatalf("banked offline response = %+v, want one offline banked session", offlineResponse.Banked)
	}
	if offlineResponse.Banked[0].LastSeen == "" || offlineResponse.Banked[0].FirstSeen == "" {
		t.Fatalf("bank timestamps missing: %+v", offlineResponse.Banked[0])
	}
	if offlineResponse.Banked[0].ResumeCommand != "/resume codex-alpha" {
		t.Fatalf("offline resume command = %q, want sanitized /resume codex-alpha", offlineResponse.Banked[0].ResumeCommand)
	}
}

func TestTmuxHandler_EnablePersistentAgentRenamesStoresAndAnnotatesLiveSession(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf 'codex:/home/alice/project\n' ;;
  *list-sessions*) printf '$7:codex-vw-codex1:1:0\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBufferString(`{"newName":"codex-vw-codex1","identity":"Maintains the VW Codex lane.","agentKind":"codex","agentSessionId":"` + sessionID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != true || response["session"] != "codex-vw-codex1" || response["persistent"] != true || response["agentKind"] != "codex" || response["agentSessionId"] != sessionID || response["resumeCommand"] != "codex resume "+sessionID || response["cwd"] != "/home/alice/project" || response["identity"] != "Maintains the VW Codex lane." {
		t.Fatalf("persistent response = %#v", response)
	}
	wantPrefix := [][]string{
		{"-S", "/tmp/tmux-a", "display-message", "-p", "-t", "codex-alpha", "#{pane_pid}:#{pane_current_command}:#{pane_current_path}"},
		{"-S", "/tmp/tmux-a", "rename-session", "-t", "codex-alpha", "codex-vw-codex1"},
	}
	got := readArgvRecordingTmuxCalls(t, argsPath)
	if len(got) < len(wantPrefix) || !equalArgvCalls(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("tmux calls prefix = %#v, want %#v", got, wantPrefix)
	}

	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var sessions SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(sessions.Sessions) != 1 {
		t.Fatalf("sessions = %+v, want one", sessions.Sessions)
	}
	session := sessions.Sessions[0]
	if !session.Persistent || session.PersistentIdentity != "Maintains the VW Codex lane." || session.PersistentAgentKind != "codex" || session.PersistentAgentSessionID != sessionID || session.PersistentResumeCommand != "codex resume "+sessionID {
		t.Fatalf("persistent session metadata = %+v", session)
	}
}

func TestTmuxHandler_EnablePersistentAgentInfersCodexMetadataFromLiveProcess(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	originalReadProcessTable := readPersistentAgentProcessTable
	readPersistentAgentProcessTable = func(context.Context) ([]processInfo, error) {
		return []processInfo{{
			pid:  "42",
			ppid: "1",
			comm: "node",
			args: "node /usr/bin/codex resume --no-alt-screen -C /home/alice/project " + sessionID,
		}}, nil
	}
	t.Cleanup(func() { readPersistentAgentProcessTable = originalReadProcessTable })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBufferString(`{"identity":"Maintains the VW Codex lane."}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["agentKind"] != "codex" || response["agentSessionId"] != sessionID || response["resumeCommand"] != "codex resume "+sessionID || response["cwd"] != "/home/alice/project" {
		t.Fatalf("persistent response = %#v", response)
	}
}

func TestTmuxHandler_EnablePersistentAgentFallsBackToOwnerProbeWhenArgsLackResumeID(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	originalReadProcessTable := readPersistentAgentProcessTable
	readPersistentAgentProcessTable = func(context.Context) ([]processInfo, error) {
		return []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex --no-alt-screen"}}, nil
	}
	t.Cleanup(func() { readPersistentAgentProcessTable = originalReadProcessTable })
	originalProbe := probePersistentAgentOwnerMetadata
	probePersistentAgentOwnerMetadata = func(ctx context.Context, h *TmuxHandler, target tmuxTarget, pane paneInspection, requestedKind string) (inferredPersistentAgentMetadata, error) {
		if target.unixUser != "alice" || target.socket != "/tmp/tmux-a" || pane.PID != "42" || pane.CWD != "/home/alice/project" || requestedKind != "" {
			t.Fatalf("probe args target=%+v pane=%+v requestedKind=%q", target, pane, requestedKind)
		}
		return inferredPersistentAgentMetadata{Kind: "codex", SessionID: sessionID, Source: "owner-probe"}, nil
	}
	t.Cleanup(func() { probePersistentAgentOwnerMetadata = originalProbe })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBufferString(`{"identity":"Maintains the VW Codex lane."}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["agentKind"] != "codex" || response["agentSessionId"] != sessionID || response["resumeCommand"] != "codex resume "+sessionID {
		t.Fatalf("persistent response = %#v", response)
	}
}

func TestTmuxHandler_EnablePersistentAgentFailsClearlyWhenSessionIDCannotBeInferred(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	originalReadProcessTable := readPersistentAgentProcessTable
	readPersistentAgentProcessTable = func(context.Context) ([]processInfo, error) {
		return []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex --no-alt-screen"}}, nil
	}
	t.Cleanup(func() { readPersistentAgentProcessTable = originalReadProcessTable })
	originalProbe := probePersistentAgentOwnerMetadata
	probePersistentAgentOwnerMetadata = func(context.Context, *TmuxHandler, tmuxTarget, paneInspection, string) (inferredPersistentAgentMetadata, error) {
		return inferredPersistentAgentMetadata{}, context.Canceled
	}
	t.Cleanup(func() { probePersistentAgentOwnerMetadata = originalProbe })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewBufferString(`{"identity":"Maintains the VW Codex lane."}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Could not infer Codex/Claude session id") {
		t.Fatalf("error body = %s", recorder.Body.String())
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "[]" && strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
}

func TestTmuxHandler_ListSessionsDoesNotWritePersistentLiveSessionsIntoBank(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$7:codex-alpha:1:0\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	writePersistentAgentSeed(t, persistentPath, []PersistentAgentEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Identity:       "Maintains the repo.",
		AgentKind:      "codex",
		AgentSessionID: "019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		CWD:            "/home/alice/project",
	}})

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(response.Sessions) != 1 || !response.Sessions[0].Persistent {
		t.Fatalf("sessions = %+v, want one persistent live session", response.Sessions)
	}
	if len(response.Banked) != 0 {
		t.Fatalf("banked = %+v, want persistent live session excluded from bank", response.Banked)
	}
	entries, err := handler.bank.Read()
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("bank store entries = %+v, want none for persistent live session", entries)
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsNonAgentPaneWithoutPersisting(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf 'bash:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewBufferString(`{"agentKind":"codex","agentSessionId":"019f45ec-f88b-7f70-88dc-b5b99a9e94c6"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	want := [][]string{{"-S", "/tmp/tmux-a", "display-message", "-p", "-t", "codex-alpha", "#{pane_pid}:#{pane_current_command}:#{pane_current_path}"}}
	if got := readArgvRecordingTmuxCalls(t, argsPath); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "[]" && strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
}

func TestPersistentAgentProcessDetectionAcceptsNodeWrappedCodex(t *testing.T) {
	if !processLooksLikeAgent("node", "node /usr/bin/codex --no-alt-screen", "codex") {
		t.Fatal("node-wrapped Codex process should count as live codex")
	}
	if processLooksLikeAgent("python", "python /tmp/codex-helper.py", "codex") {
		t.Fatal("non-node helper mentioning codex should not count as live codex")
	}
	if processLooksLikeAgent("node", "node /usr/bin/claude", "codex") {
		t.Fatal("node-wrapped Claude process should not count as live codex")
	}
}

func TestPersistentAgentProcessTreeChecksPaneRootProcess(t *testing.T) {
	infos := []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex"}}
	if !processTreeContainsAgentInTable(infos, "42", "codex") {
		t.Fatal("pane root node-wrapped Codex process should count as live codex")
	}
}

func TestPersistentAgentMetadataInferenceParsesClaudeResumeArgs(t *testing.T) {
	const sessionID = "9ed1181c-b2a3-4ef2-96ea-a84e51e79dc4"
	infos := []processInfo{{pid: "50", ppid: "1", comm: "claude", args: "claude --dangerously-skip-permissions --resume " + sessionID + " READ-ONLY"}}
	metadata, foundAgent, foundSessionID := inferPersistentAgentMetadataInTable(infos, "50", "")
	if !foundAgent || !foundSessionID || metadata.Kind != "claude" || metadata.SessionID != sessionID {
		t.Fatalf("metadata=%+v foundAgent=%v foundSessionID=%v", metadata, foundAgent, foundSessionID)
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsRecreatesMissingCodexSession(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	writePersistentAgentSeed(t, persistentPath, []PersistentAgentEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Identity:       "Maintains the repo.",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		ResumeCommand:  "rm -rf /",
		CWD:            "/home/alice/project",
	}})

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "recreated" || results[0].Error != "" {
		t.Fatalf("reconcile results = %+v", results)
	}
	want := [][]string{
		{"-S", "/tmp/tmux-a", "has-session", "-t", "codex-alpha"},
		{"-S", "/tmp/tmux-a", "new-session", "-d", "-s", "codex-alpha", "-c", "/home/alice/project"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "codex-alpha", "-l", "--", "codex resume " + sessionID},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "codex-alpha", "Enter"},
	}
	if got := readArgvRecordingTmuxCalls(t, argsPath); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_DisablePersistentAgentRemovesDesiredStateWithoutCallingTmux(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installArgvRecordingTmux(t)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	writePersistentAgentSeed(t, persistentPath, []PersistentAgentEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		AgentKind:      "codex",
		AgentSessionID: "019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		CWD:            "/home/alice/project",
	}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := readArgvRecordingTmuxCalls(t, argsPath); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none", got)
	}
	entries, err := handler.persistent.Read()
	if err != nil {
		t.Fatalf("read persistent store: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("persistent entries = %+v, want empty", entries)
	}
}

func TestTmuxHandler_ListSessionsPreservesAgentRecoveryMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	seed := []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		Windows:        1,
		Attached:       false,
		Live:           true,
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		ResumeCommand:  "rm -rf /",
		CWD:            "/home/alice/project",
		TranscriptPath: "/home/alice/.codex/sessions/rollout-" + sessionID + ".jsonl",
	}}
	raw, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal bank seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(bankPath), 0o755); err != nil {
		t.Fatalf("mkdir bank: %v", err)
	}
	if err := os.WriteFile(bankPath, raw, 0o660); err != nil {
		t.Fatalf("write bank seed: %v", err)
	}
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

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
	if len(response.Sessions) != 0 {
		t.Fatalf("live sessions = %+v, want none", response.Sessions)
	}
	if len(response.Banked) != 1 {
		t.Fatalf("banked = %+v, want one entry", response.Banked)
	}
	entry := response.Banked[0]
	if entry.Live {
		t.Fatalf("entry.Live = true, want offline after empty tmux scan: %+v", entry)
	}
	if entry.AgentKind != "codex" || entry.AgentSessionID != sessionID || entry.CWD != "/home/alice/project" || entry.TranscriptPath == "" {
		t.Fatalf("agent recovery metadata not preserved: %+v", entry)
	}
	if entry.RecoveryKind != "agent" {
		t.Fatalf("recovery kind = %q, want agent", entry.RecoveryKind)
	}
	if entry.ResumeCommand != "codex resume "+sessionID {
		t.Fatalf("resume command = %q, want canonical codex command", entry.ResumeCommand)
	}
	if calls := readArgvRecordingTmuxCalls(t, argsPath); len(calls) != 1 {
		t.Fatalf("tmux calls = %#v, want one list-sessions call", calls)
	}
}

func TestSessionBankCanonicalAgentResumeCommand(t *testing.T) {
	const codexID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	const claudeID = "9ed1181c-b2a3-4ef2-96ea-a84e51e79dc4"
	tests := []struct {
		name string
		kind string
		id   string
		want string
		ok   bool
	}{
		{name: "codex", kind: "codex", id: codexID, want: "codex resume " + codexID, ok: true},
		{name: "claude", kind: "claude", id: claudeID, want: "claude --resume " + claudeID, ok: true},
		{name: "normalizes kind", kind: " Codex ", id: codexID, want: "codex resume " + codexID, ok: true},
		{name: "unknown kind", kind: "pi", id: codexID, ok: false},
		{name: "empty id", kind: "codex", id: "", ok: false},
		{name: "space in id", kind: "codex", id: codexID + " extra", ok: false},
		{name: "semicolon in id", kind: "codex", id: codexID + ";touch-pwn", ok: false},
		{name: "newline in id", kind: "codex", id: codexID + "\nwhoami", ok: false},
		{name: "dollar in id", kind: "codex", id: "$(whoami)", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := canonicalAgentResumeCommand(tt.kind, tt.id)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("canonicalAgentResumeCommand(%q, %q) = %q, %v; want %q, %v", tt.kind, tt.id, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestTmuxHandler_RecoverBankedCodexSessionCreatesShellAndSendsResumeCommandLiterally(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	writeBankSeed(t, bankPath, []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		Windows:        1,
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		ResumeCommand:  "rm -rf /",
		CWD:            "/home/alice/project",
	}})
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recover?unixUser=alice", bytes.NewBufferString(`{"mouseScroll":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	wantCalls := [][]string{
		{"-S", "/tmp/tmux-a", "new-session", "-d", "-s", "codex-alpha", "-c", "/home/alice/project"},
		{"-S", "/tmp/tmux-a", "set-option", "-g", "mouse", "on"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "codex-alpha", "-l", "codex resume " + sessionID},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "codex-alpha", "Enter"},
	}
	if got := readArgvRecordingTmuxCalls(t, argsPath); !equalArgvCalls(got, wantCalls) {
		t.Fatalf("tmux calls = %#v, want %#v", got, wantCalls)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != true || response["session"] != "codex-alpha" || response["agentKind"] != "codex" || response["agentSessionId"] != sessionID || response["resumeCommand"] != "codex resume "+sessionID {
		t.Fatalf("recover response = %#v", response)
	}
	if _, hasData := response["data"]; hasData {
		t.Fatalf("recover response should be flat JSON, got data envelope: %#v", response)
	}
}

func TestTmuxHandler_SendToSessionStoresDropAndPastesViaBuffer(t *testing.T) {
	tmpDir := t.TempDir()
	dropsDir := filepath.Join(tmpDir, "drops")
	argsPath := installArgvRecordingTmux(t)
	t.Setenv("CHROTE_SESSION_DROPS_DIR", dropsDir)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("text", "Please inspect this screenshot."); err != nil {
		t.Fatalf("write text field: %v", err)
	}
	if err := writer.WriteField("submit", "true"); err != nil {
		t.Fatalf("write submit field: %v", err)
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
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/alice-shell/send?unixUser=alice", body)
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
	if response["success"] != true || response["session"] != "alice-shell" || response["unixUser"] != "alice" {
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
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("stored file mode = %o, want 644", info.Mode().Perm())
	}

	calls := readArgvRecordingTmuxCalls(t, argsPath)
	joined := make([]string, 0, len(calls))
	for _, call := range calls {
		joined = append(joined, strings.Join(call, "\x00"))
	}
	wantSnippets := []string{
		strings.Join([]string{"-S", "/tmp/tmux-a", "load-buffer"}, "\x00"),
		strings.Join([]string{"-S", "/tmp/tmux-a", "paste-buffer", "-d"}, "\x00"),
		strings.Join([]string{"-S", "/tmp/tmux-a", "send-keys", "-t", "alice-shell", "Enter"}, "\x00"),
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

func TestTmuxHandler_UpdateBankedRecoveryMetadataMakesLiveSessionRecoverableAfterRestart(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "offline")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\ncase \"$*\" in\n  *list-sessions*)\n    if [ ! -f " + statePath + " ]; then printf '$7:codex-alpha:1:0\\n'; fi\n    ;;\nesac\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("initial list status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBufferString(`{"agentKind":"codex","agentSessionId":"` + sessionID + `","cwd":"/home/alice/project","transcriptPath":"/home/alice/.codex/sessions/rollout-` + sessionID + `.jsonl"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recovery?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("recovery metadata status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	if err := os.WriteFile(statePath, []byte("offline"), 0o644); err != nil {
		t.Fatalf("write offline marker: %v", err)
	}
	handler = NewTmuxHandler()
	recorder = httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("offline list status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode offline response: %v", err)
	}
	if len(response.Banked) != 1 {
		t.Fatalf("banked = %+v, want one recoverable bank entry", response.Banked)
	}
	entry := response.Banked[0]
	if entry.RecoveryKind != "agent" || entry.AgentKind != "codex" || entry.AgentSessionID != sessionID || entry.ResumeCommand != "codex resume "+sessionID || entry.CWD != "/home/alice/project" {
		t.Fatalf("offline recovery entry = %+v, want persisted agent metadata", entry)
	}
}

func TestTmuxHandler_RecoverBankedSessionDropsUnsafeTmuxFormatCWD(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	writeBankSeed(t, bankPath, []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		CWD:            "/tmp/#(touch /tmp/chrote-pwned)",
	}})
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recover?unixUser=alice", bytes.NewBufferString(`{"mouseScroll":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	calls := readArgvRecordingTmuxCalls(t, argsPath)
	if len(calls) == 0 || strings.Join(calls[0], "\x00") != strings.Join([]string{"-S", "/tmp/tmux-a", "new-session", "-d", "-s", "codex-alpha", "-c", "/home/alice"}, "\x00") {
		t.Fatalf("first tmux call = %#v, want unsafe cwd dropped and configured workdir used", calls)
	}
	for _, call := range calls {
		if strings.Contains(strings.Join(call, " "), "#(") {
			t.Fatalf("tmux calls include unsafe format cwd: %#v", calls)
		}
	}
}

func TestTmuxHandler_RecoverBankedSessionRejectsUnsafeAgentMetadataWithoutTmuxSideEffects(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	writeBankSeed(t, bankPath, []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: "019f45ec-f88b-7f70-88dc-b5b99a9e94c6;touch-pwn",
		ResumeCommand:  "rm -rf /",
	}})
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recover?unixUser=alice", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if got := readArgvRecordingTmuxCalls(t, argsPath); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none for unsafe recovery metadata", got)
	}
}

func TestTmuxHandler_ForgetBankedAgentSessionDoesNotCallTmux(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	writeBankSeed(t, bankPath, []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: "019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		ResumeCommand:  "codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
	}})
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-bank/codex-alpha?unixUser=alice", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := readArgvRecordingTmuxCalls(t, argsPath); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want metadata-only forget", got)
	}
}

func TestSessionBankForgetRemovesExactUserEntry(t *testing.T) {
	store := newSessionBankStore(filepath.Join(t.TempDir(), "session-bank", "sessions.json"))
	_, err := store.Snapshot([]core.Session{
		{Name: "codex-alpha", ID: "$7", UnixUser: "alice", Group: "codex", Windows: 1},
		{Name: "codex-alpha", ID: "$8", UnixUser: "bob", Group: "codex", Windows: 1},
	})
	if err != nil {
		t.Fatalf("snapshot bank: %v", err)
	}

	removed, err := store.Forget("codex-alpha", "alice")
	if err != nil {
		t.Fatalf("forget bank entry: %v", err)
	}
	if !removed {
		t.Fatal("removed = false, want true")
	}

	entries, err := store.Read()
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "codex-alpha" || entries[0].UnixUser != "bob" {
		t.Fatalf("bank entries after forget = %+v, want only bob/codex-alpha", entries)
	}

	removed, err = store.Forget("missing", "alice")
	if err != nil {
		t.Fatalf("forget missing bank entry: %v", err)
	}
	if removed {
		t.Fatal("removed missing = true, want false")
	}
}

func TestTmuxHandler_RegisterRoutesWiresSessionBankForget(t *testing.T) {
	bankPath := filepath.Join(t.TempDir(), "session-bank", "sessions.json")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)

	handler := NewTmuxHandler()
	_, err := handler.bank.Snapshot([]core.Session{{Name: "codex-alpha", ID: "$7", UnixUser: "alice", Group: "codex", Windows: 1}})
	if err != nil {
		t.Fatalf("snapshot bank: %v", err)
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-bank/codex-alpha?unixUser=alice", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode forget response: %v", err)
	}
	if response["removed"] != true {
		t.Fatalf("removed response = %#v, want true", response["removed"])
	}
	entries, err := handler.bank.Read()
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("bank entries = %+v, want empty after forget", entries)
	}
}

func writeBankSeed(t *testing.T, path string, entries []SessionBankEntry) {
	t.Helper()
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal bank seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir bank dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o660); err != nil {
		t.Fatalf("write bank seed: %v", err)
	}
}

func installArgvRecordingTmux(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-argv.txt")
	scriptPath := filepath.Join(dir, "tmux")
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$TMUX_ARGS_FILE"
done
printf '%s\n' '---' >> "$TMUX_ARGS_FILE"
case "$*" in
  *list-sessions*) printf '' ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
		t.Fatalf("write args log: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsPath
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

func writePersistentAgentSeed(t *testing.T, path string, entries []PersistentAgentEntry) {
	t.Helper()
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal persistent seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir persistent dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o660); err != nil {
		t.Fatalf("write persistent seed: %v", err)
	}
}

func installPersistentAgentScriptedTmux(t *testing.T, behavior string) string {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-argv.txt")
	scriptPath := filepath.Join(dir, "tmux")
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$TMUX_ARGS_FILE"
done
printf '%s\n' '---' >> "$TMUX_ARGS_FILE"
` + behavior + `
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
		t.Fatalf("write args log: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsPath
}
