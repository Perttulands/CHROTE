package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func launchTestConfig(t *testing.T) LaunchConfig {
	t.Helper()
	config, err := LoadLaunchConfig(writeLaunchConfigFile(t, testLaunchConfigJSON))
	if err != nil {
		t.Fatalf("load launch config: %v", err)
	}
	return config
}

func postCreateSession(t *testing.T, handler *TmuxHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.CreateSession(recorder, req)
	return recorder
}

// Launching a harness is one create followed by one typed line and one Enter,
// on the socket and in the folder the request named. The whole argv is compared
// because the typed line is what the operator ends up staring at, and a
// requested flags line has to replace the harness defaults rather than join them.
func TestCreateSessionStartsHarnessInRequestedFolder(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		body        string
		session     string
		wantCwd     string
		wantHarness string
		wantTyped   string
		wantFlags   string
	}{
		{
			name:        "a named folder and the harness's own default flags",
			body:        `{"name":"claude-2","cwd":"/srv/work/one","harness":"claude-code"}`,
			session:     "claude-2",
			wantCwd:     "/srv/work/one",
			wantHarness: "claude-code",
			wantTyped:   "claude --harness-flag",
			wantFlags:   "--harness-flag",
		},
		{
			name:        "a requested line replaces the harness defaults",
			body:        `{"name":"claude-3","harness":"claude-code","flags":"--model fast --verbose"}`,
			session:     "claude-3",
			wantCwd:     "/srv/default-work",
			wantHarness: "claude-code",
			wantTyped:   "claude --model fast --verbose",
			wantFlags:   "--model fast --verbose",
		},
		{
			name:        "an empty line types the binary alone",
			body:        `{"name":"claude-4","harness":"claude-code","flags":""}`,
			session:     "claude-4",
			wantCwd:     "/srv/default-work",
			wantHarness: "claude-code",
			wantTyped:   "claude",
			wantFlags:   "",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, argsPath := installFakeTmux(t)
			t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/srv/default-work")

			handler := NewTmuxHandlerWithLaunchConfig(launchTestConfig(t))
			recorder := postCreateSession(t, handler, testCase.body)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			want := append(tmuxCreationCalls("/tmp/tmux-a", testCase.session, testCase.wantCwd, "on"),
				"-S /tmp/tmux-a send-keys -t $42 -l "+testCase.wantTyped,
				"-S /tmp/tmux-a send-keys -t $42 Enter",
			)
			got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath))
			if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
				t.Fatalf("tmux calls = %#v, want %#v", got, want)
			}

			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode create response: %v; body=%s", err, recorder.Body.String())
			}
			if response["cwd"] != testCase.wantCwd || response["harness"] != testCase.wantHarness {
				t.Fatalf("response = %#v, want cwd %q and harness %q", response, testCase.wantCwd, testCase.wantHarness)
			}
			if response["flags"] != testCase.wantFlags {
				t.Fatalf("response = %#v, want flags %q reported back", response, testCase.wantFlags)
			}
			if _, warned := response["warning"]; warned {
				t.Fatalf("response warned about a command that was sent: %#v", response)
			}
		})
	}
}

func TestCreateSessionWithoutHarnessSendsNoKeysAndReportsTheShell(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/srv/default-work")

	handler := NewTmuxHandlerWithLaunchConfig(launchTestConfig(t))
	recorder := postCreateSession(t, handler, `{"name":"shell-2"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	for _, call := range readFakeCommandCalls(t, argsPath) {
		if strings.Contains(call, "send-keys") {
			t.Fatalf("a session with no harness typed into the shell: %q", call)
		}
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v; body=%s", err, recorder.Body.String())
	}
	if response["cwd"] != "/srv/default-work" || response["harness"] != "shell" {
		t.Fatalf("response = %#v, want the configured workdir and the shell harness", response)
	}
	if response["flags"] != "" {
		t.Fatalf("response = %#v, want no flags for the bare shell", response)
	}
}

func TestCreateSessionResolvesTheHomeTokenAgainstTheTargetUser(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	current, err := user.Current()
	if err != nil {
		t.Fatalf("look up the running account: %v", err)
	}
	t.Setenv("CHROTE_TMUX_SOCKET", current.Username+"=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", current.Username+"=/srv/default-work")
	// The server must read the target user's passwd entry, not its own
	// environment, so a misleading HOME changes nothing.
	t.Setenv("HOME", "/srv/not-a-home")

	handler := NewTmuxHandlerWithLaunchConfig(launchTestConfig(t))
	recorder := postCreateSession(t, handler, `{"name":"home-1","cwd":"~/projects"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	wantCwd := filepath.Join(current.HomeDir, "projects")
	calls := readFakeCommandCalls(t, argsPath)
	if len(calls) == 0 || !strings.HasSuffix(calls[0], " -c "+wantCwd) {
		t.Fatalf("first tmux call = %#v, want a new-session in %q", calls, wantCwd)
	}
}

func TestCreateSessionRefusesUnusableLaunchRequestsWithoutTouchingTmux(t *testing.T) {
	tests := []struct {
		name string
		body string
		// wantMessage, when set, is the message the operator must be given.
		wantMessage string
	}{
		{name: "unknown harness", body: `{"name":"refused-1","harness":"emacs"}`},
		{name: "relative cwd", body: `{"name":"refused-2","cwd":"work/one"}`},
		{name: "single tilde cwd", body: `{"name":"refused-3","cwd":"~/work"}`},
		{
			name:        "a flags line with a control character",
			body:        `{"name":"refused-4","harness":"claude-code","flags":"--model fast\nrm -rf /"}`,
			wantMessage: "flags must be one line",
		},
		{
			name: "flags for the harness that starts nothing",
			body: `{"name":"refused-5","harness":"shell","flags":"--model fast"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, argsPath := installFakeTmux(t)
			t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
			handler := NewTmuxHandlerWithLaunchConfig(launchTestConfig(t))

			recorder := postCreateSession(t, handler, tt.body)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if calls := readFakeCommandCalls(t, argsPath); len(calls) != 0 {
				t.Fatalf("a refused launch still ran tmux: %#v", calls)
			}
			if tt.wantMessage == "" {
				return
			}
			var refusal struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &refusal); err != nil {
				t.Fatalf("decode refusal: %v; body=%s", err, recorder.Body.String())
			}
			if refusal.Error.Message != tt.wantMessage {
				t.Fatalf("refusal message = %q, want %q", refusal.Error.Message, tt.wantMessage)
			}
		})
	}
}

func TestCreateSessionKeepsTheSessionWhenTheCommandCannotBeSent(t *testing.T) {
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/srv/default-work")
	argsPath := installScriptedTmux(t, `
case "$*" in
  *send-keys*) echo 'send-keys failed' >&2; exit 1 ;;
esac
`)

	handler := NewTmuxHandlerWithLaunchConfig(launchTestConfig(t))
	recorder := postCreateSession(t, handler, `{"name":"codex-2","harness":"codex"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v; body=%s", err, recorder.Body.String())
	}
	warning, warned := response["warning"].(string)
	if !warned || !strings.Contains(warning, "codex") {
		t.Fatalf("response = %#v, want a warning naming the harness that did not start", response)
	}
	if response["session"] != "codex-2" || response["harness"] != "codex" {
		t.Fatalf("response = %#v, want the created session reported anyway", response)
	}

	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read tmux calls: %v", err)
	}
	if calls := string(raw); strings.Contains(calls, "kill-session") || strings.Contains(calls, "if-shell") {
		t.Fatalf("a session that could not start its harness was cleaned up: %q", calls)
	}
}
