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

// installEventTmux is a tmux that knows two sessions, alpha and beta, on
// socket /tmp/tmux-a, and answers has-session and the inventory for them the
// way a real one does.
func installEventTmux(t *testing.T) string {
	t.Helper()
	return installScriptedTmux(t, `
case "$*" in
  *"has-session -t =alpha"*|*"has-session -t =beta"*) exit 0 ;;
  *"has-session -t ="*)
    target="${*##*has-session -t =}"
    printf "can't find session: %s\n" "$target" >&2
    exit 1
    ;;
  *"list-sessions -F "*)
    printf '$1\talpha\t1\t0\t/srv/work\tclaude\t1\t200\t50\tlatest\t1\t\t1725400000\n'
    printf '$2\tbeta\t1\t0\t/srv/work\tcodex\t1\t200\t50\tlatest\t1\t\t1725400000\n'
    exit 0
    ;;
esac
`)
}

func newEventMux(t *testing.T) *http.ServeMux {
	t.Helper()
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	tmux := NewTmuxHandler()
	mux := http.NewServeMux()
	tmux.RegisterRoutes(mux)
	NewAgentEventHandler(tmux).RegisterRoutes(mux)
	return mux
}

func postJSON(t *testing.T, mux *http.ServeMux, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	return recorder
}

func listedSessions(t *testing.T, mux *http.ServeMux) map[string]core.Session {
	t.Helper()
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode sessions: %v; body=%s", err, recorder.Body.String())
	}
	byName := map[string]core.Session{}
	for _, session := range response.Sessions {
		byName[session.Name] = session
	}
	return byName
}

// An event is the session's last event from the moment it is posted until the
// operator looks: the list carries it unseen, seen keeps the record but drops
// the claim, and a later event is news again.
func TestAgentEventIsCarriedByTheSessionListUntilSeen(t *testing.T) {
	installEventTmux(t)
	mux := newEventMux(t)

	recorder := postJSON(t, mux, "/api/agent/event", `{"session":"alpha","event":"finished","summary":"Done: three files changed"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("event status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	sessions := listedSessions(t, mux)
	alpha := sessions["alpha"].LastEvent
	if alpha == nil {
		t.Fatalf("alpha carries no lastEvent: %+v", sessions["alpha"])
	}
	if alpha.Event != "finished" || alpha.Summary != "Done: three files changed" || alpha.Seen || alpha.Time == "" {
		t.Fatalf("alpha lastEvent = %+v, want an unseen finished event with its summary and time", alpha)
	}
	if sessions["beta"].LastEvent != nil {
		t.Fatalf("beta carries an event it never reported: %+v", sessions["beta"].LastEvent)
	}

	recorder = postJSON(t, mux, "/api/agent/event/seen", `{"session":"alpha"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("seen status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	alpha = listedSessions(t, mux)["alpha"].LastEvent
	if alpha == nil || !alpha.Seen || alpha.Event != "finished" {
		t.Fatalf("after seen, alpha lastEvent = %+v, want the finished event marked seen", alpha)
	}

	recorder = postJSON(t, mux, "/api/agent/event", `{"session":"alpha","event":"needs-input"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("second event status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	alpha = listedSessions(t, mux)["alpha"].LastEvent
	if alpha == nil || alpha.Seen || alpha.Event != "needs-input" || alpha.Summary != "" {
		t.Fatalf("after a second event, alpha lastEvent = %+v, want an unseen needs-input with no summary", alpha)
	}
}

// Seen on a session that reported nothing is not a mistake: the operator
// looked at a quiet session.
func TestAgentEventSeenWithoutAnEventIsQuiet(t *testing.T) {
	installEventTmux(t)
	mux := newEventMux(t)

	recorder := postJSON(t, mux, "/api/agent/event/seen", `{"session":"beta"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("seen status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "lastEvent") {
		t.Fatalf("seen invented an event: %s", recorder.Body.String())
	}
}

// A report is refused before tmux is asked when it is not a report, and a
// report about a session tmux does not have is not kept.
func TestAgentEventRefusesWhatIsNotAReport(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		body       string
		wantStatus int
		wantTmux   bool
	}{
		{name: "not JSON", body: `{"session":`, wantStatus: http.StatusBadRequest},
		{name: "no session", body: `{"event":"finished"}`, wantStatus: http.StatusBadRequest},
		{name: "a session name tmux would not accept", body: `{"session":"a:b","event":"finished"}`, wantStatus: http.StatusBadRequest},
		{name: "an event CHROTE does not know", body: `{"session":"alpha","event":"started"}`, wantStatus: http.StatusBadRequest},
		{name: "a Unix user with no socket", body: `{"session":"alpha","event":"finished","unixUser":"mallory"}`, wantStatus: http.StatusBadRequest},
		{name: "a session tmux does not have", body: `{"session":"gamma","event":"finished"}`, wantStatus: http.StatusNotFound, wantTmux: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			argsPath := installEventTmux(t)
			mux := newEventMux(t)

			recorder := postJSON(t, mux, "/api/agent/event", testCase.body)

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
			raw, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatalf("read tmux calls: %v", err)
			}
			if asked := strings.Contains(string(raw), "has-session"); asked != testCase.wantTmux {
				t.Fatalf("tmux asked = %v, want %v; calls=%q", asked, testCase.wantTmux, raw)
			}
			if sessions := listedSessions(t, mux); sessions["alpha"].LastEvent != nil || sessions["beta"].LastEvent != nil {
				t.Fatalf("a refused report was kept: %+v", sessions)
			}
		})
	}
}

// A summary is a hint, so an over-long one is cut rather than refused: a
// refused report would be a lost one.
func TestAgentEventCutsAnOverlongSummary(t *testing.T) {
	installEventTmux(t)
	mux := newEventMux(t)

	long := strings.Repeat("é", maxAgentEventSummaryRunes+10)
	recorder := postJSON(t, mux, "/api/agent/event", `{"session":"alpha","event":"finished","summary":"`+long+`"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	alpha := listedSessions(t, mux)["alpha"].LastEvent
	if alpha == nil || len([]rune(alpha.Summary)) != maxAgentEventSummaryRunes {
		t.Fatalf("summary length = %d runes, want %d", len([]rune(alpha.Summary)), maxAgentEventSummaryRunes)
	}
}

// The store is bounded by the sessions that exist: once a listing no longer
// shows a session, its event is forgotten with it.
func TestAgentEventIsForgottenWithItsSession(t *testing.T) {
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	inventory := filepath.Join(t.TempDir(), "inventory")
	if err := os.WriteFile(inventory, []byte("$1\talpha\t1\t0\t/srv/work\tclaude\t1\t200\t50\tlatest\t1\t\t1725400000\n"), 0o600); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
	t.Setenv("TMUX_INVENTORY_FILE", inventory)
	installScriptedTmux(t, `
case "$*" in
  *"has-session -t =alpha"*) exit 0 ;;
  *"list-sessions -F "*) cat "$TMUX_INVENTORY_FILE"; exit 0 ;;
esac
`)
	tmux := NewTmuxHandler()
	mux := http.NewServeMux()
	tmux.RegisterRoutes(mux)
	NewAgentEventHandler(tmux).RegisterRoutes(mux)

	if recorder := postJSON(t, mux, "/api/agent/event", `{"session":"alpha","event":"finished"}`); recorder.Code != http.StatusOK {
		t.Fatalf("event status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	if listedSessions(t, mux)["alpha"].LastEvent == nil {
		t.Fatal("alpha carries no lastEvent while it exists")
	}

	// The session ends; the next listing does not show it.
	if err := os.WriteFile(inventory, nil, 0o600); err != nil {
		t.Fatalf("empty inventory: %v", err)
	}
	listedSessions(t, mux)
	if _, kept := tmux.events.markSeen("alice", "alpha"); kept {
		t.Fatal("the event outlived its session")
	}
}

func TestAgentEventURLReachesTheBindAddress(t *testing.T) {
	for host, want := range map[string]string{
		"":          "http://127.0.0.1:8094/api/agent/event",
		"0.0.0.0":   "http://127.0.0.1:8094/api/agent/event",
		"::":        "http://127.0.0.1:8094/api/agent/event",
		"127.0.0.1": "http://127.0.0.1:8094/api/agent/event",
		"10.0.0.5":  "http://10.0.0.5:8094/api/agent/event",
		"fd00::5":   "http://[fd00::5]:8094/api/agent/event",
	} {
		if got := agentEventURL(host, 8094); got != want {
			t.Errorf("agentEventURL(%q) = %q, want %q", host, got, want)
		}
	}
}

// LoadAgentHooks looks beside the binary by default and takes an explicit
// path from the environment; a script that is not there turns hooks off with
// a reason rather than stopping the server.
func TestLoadAgentHooksFindsTheScriptOrSaysWhyNot(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "chrote-agent-event")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv(agentEventHookEnv, script)
	t.Setenv(agentHooksDirEnv, filepath.Join(dir, "hooks"))

	hooks, warning := LoadAgentHooks("127.0.0.1", 8094)
	if warning != "" || hooks.Script != script || hooks.SettingsDir != filepath.Join(dir, "hooks") || !hooks.enabled() {
		t.Fatalf("LoadAgentHooks = %+v, %q; want the configured script and directory", hooks, warning)
	}

	t.Setenv(agentEventHookEnv, filepath.Join(dir, "absent"))
	hooks, warning = LoadAgentHooks("127.0.0.1", 8094)
	if warning == "" || hooks.enabled() {
		t.Fatalf("LoadAgentHooks with an absent script = %+v, %q; want hooks off with a reason", hooks, warning)
	}
}

// A launch with hooks configured tells the harness how to report back in
// that harness's own way, gives every session the address to report to, and
// leaves a harness the server does not know, and a launch that asked for
// quiet, exactly as they were.
func TestCreateSessionInstallsTheHarnessCompletionHooks(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		body      string
		session   string
		wantTyped func(settingsDir string) string
		wantFile  bool
		wantNotif bool
	}{
		{
			name:    "Claude Code reads its hooks from a generated settings file",
			body:    `{"name":"claude-2","harness":"claude-code"}`,
			session: "claude-2",
			wantTyped: func(dir string) string {
				return "claude --harness-flag --settings '" + dir + "/alice.claude-2.claude-settings.json'"
			},
			wantFile:  true,
			wantNotif: true,
		},
		{
			name:    "Codex takes the script as its notify program",
			body:    `{"name":"codex-2","harness":"codex"}`,
			session: "codex-2",
			wantTyped: func(string) string {
				return `codex --harness-flag -c 'notify=["/opt/chrote/bin/chrote-agent-event","finished"]'`
			},
			wantNotif: true,
		},
		{
			name:      "a launch that asked for quiet types the command alone",
			body:      `{"name":"claude-3","harness":"claude-code","notify":false}`,
			session:   "claude-3",
			wantTyped: func(string) string { return "claude --harness-flag" },
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, argsPath := installFakeTmux(t)
			t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/srv/default-work")
			settingsDir := filepath.Join(t.TempDir(), "hooks")
			hooks := AgentHooks{
				Script:      "/opt/chrote/bin/chrote-agent-event",
				EventURL:    "http://127.0.0.1:8094/api/agent/event",
				SettingsDir: settingsDir,
			}

			handler := NewTmuxHandlerWithLaunch(launchTestConfig(t), hooks)
			recorder := postCreateSession(t, handler, testCase.body)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
			}
			calls := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath))
			wantCreate := "-S /tmp/tmux-a new-session -d -P -F #{session_id} -e CHROTE_CREATION_TOKEN=<token> " +
				"-e CHROTE_AGENT_EVENT_URL=http://127.0.0.1:8094/api/agent/event -s " + testCase.session + " -c /srv/default-work"
			if len(calls) == 0 || calls[0] != wantCreate {
				t.Fatalf("creation = %q, want %q", calls, wantCreate)
			}
			wantTyped := "-S /tmp/tmux-a send-keys -t $42 -l " + testCase.wantTyped(settingsDir)
			if typed := calls[len(calls)-2]; typed != wantTyped {
				t.Fatalf("typed = %q, want %q", typed, wantTyped)
			}
			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response["notify"] != testCase.wantNotif {
				t.Fatalf("response notify = %v, want %v: %#v", response["notify"], testCase.wantNotif, response)
			}
			if _, warned := response["warning"]; warned {
				t.Fatalf("a launch with hooks warned: %#v", response)
			}

			settingsPath := filepath.Join(settingsDir, "alice."+testCase.session+".claude-settings.json")
			raw, err := os.ReadFile(settingsPath)
			if !testCase.wantFile {
				if err == nil {
					t.Fatalf("a settings file was written for a launch that needs none: %s", raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("read settings: %v", err)
			}
			var settings struct {
				Hooks map[string][]struct {
					Hooks []struct {
						Type    string `json:"type"`
						Command string `json:"command"`
						Timeout int    `json:"timeout"`
					} `json:"hooks"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(raw, &settings); err != nil {
				t.Fatalf("settings are not Claude Code's shape: %v\n%s", err, raw)
			}
			for event, want := range map[string]string{"Stop": "finished", "Notification": "needs-input"} {
				matchers := settings.Hooks[event]
				if len(matchers) != 1 || len(matchers[0].Hooks) != 1 {
					t.Fatalf("%s hooks = %+v, want one command", event, matchers)
				}
				hook := matchers[0].Hooks[0]
				if hook.Type != "command" || hook.Command != "'/opt/chrote/bin/chrote-agent-event' "+want || hook.Timeout != claudeHookTimeoutSeconds {
					t.Fatalf("%s hook = %+v, want the script reporting %s within %ds", event, hook, want, claudeHookTimeoutSeconds)
				}
			}
			info, err := os.Stat(settingsPath)
			if err != nil {
				t.Fatalf("stat settings: %v", err)
			}
			if info.Mode().Perm()&0o044 != 0o044 {
				t.Fatalf("settings mode = %o, want readable by the session's user", info.Mode().Perm())
			}
		})
	}
}

// Without hooks configured nothing about a launch changes: no address in the
// session, no flags on the command.
func TestCreateSessionWithoutHooksTypesTheCommandAlone(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/srv/default-work")

	handler := NewTmuxHandlerWithLaunchConfig(launchTestConfig(t))
	recorder := postCreateSession(t, handler, `{"name":"claude-2","harness":"claude-code"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	for _, call := range readFakeCommandCalls(t, argsPath) {
		if strings.Contains(call, agentEventURLEnv) || strings.Contains(call, "--settings") {
			t.Fatalf("a launch without hooks configured installed one: %q", call)
		}
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["notify"] != false {
		t.Fatalf("response notify = %v, want false: %#v", response["notify"], response)
	}
}
