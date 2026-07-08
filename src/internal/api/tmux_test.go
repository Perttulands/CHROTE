package api

import (
	"bytes"
	"encoding/json"
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
