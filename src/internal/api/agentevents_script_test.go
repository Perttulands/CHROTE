package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// hookScriptPath is the script the installer ships beside the server.
func hookScriptPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "scripts", "chrote-agent-event"))
	if err != nil {
		t.Fatalf("locate hook script: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("hook script is not where the installer takes it from: %v", err)
	}
	return path
}

// hookReceiver is a server that records what the script posts.
type hookReceiver struct {
	mu     sync.Mutex
	bodies []map[string]any
	server *httptest.Server
}

func newHookReceiver(t *testing.T) *hookReceiver {
	t.Helper()
	receiver := &hookReceiver{}
	receiver.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read hook body: %v", err)
			return
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("hook posted something that is not JSON: %v: %s", err, raw)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/agent/event" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("hook request = %s %s %q, want a JSON POST to /api/agent/event", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		receiver.mu.Lock()
		receiver.bodies = append(receiver.bodies, body)
		receiver.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.server.Close)
	return receiver
}

func (r *hookReceiver) received() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]map[string]any(nil), r.bodies...)
}

// runHook runs the script the way a harness does: inside a pane (TMUX and
// TMUX_PANE set by tmux), with a tmux on PATH that answers display-message
// with the session name, and whatever stdin and arguments the harness gives.
func runHook(t *testing.T, env map[string]string, stdin string, args ...string) {
	t.Helper()
	dir := t.TempDir()
	fakeTmux := filepath.Join(dir, "tmux")
	script := `#!/bin/sh
case "$*" in
  "-S /tmp/tmux-test/default display-message -p -t %7 #S") printf 'claude-work\n' ;;
  *) echo "unexpected tmux call: $*" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	cmd := exec.Command(hookScriptPath(t), args...)
	cmd.Env = []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TMUX=/tmp/tmux-test/default,1234,0",
		"TMUX_PANE=%7",
	}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stdin = strings.NewReader(stdin)
	if output, err := cmd.CombinedOutput(); err != nil || len(output) != 0 {
		t.Fatalf("hook exited %v with output %q; it must always exit 0 and say nothing", err, output)
	}
}

// The script reports the session tmux names, the user it runs as, the event
// it was told, and the one message the payload carries, reduced to a hint.
func TestHookScriptReportsTheSessionAndEvent(t *testing.T) {
	receiver := newHookReceiver(t)
	env := map[string]string{agentEventURLEnv: receiver.server.URL + "/api/agent/event"}

	// Claude Code: the payload arrives on stdin.
	runHook(t, env, `{"session_id":"abc","hook_event_name":"Notification","message":"Claude needs your permission to use Bash","notification_type":"permission_prompt"}`, "needs-input")
	// Codex: the payload is the last argument, and the message may be long.
	runHook(t, env, "", "finished", `{"type":"agent-turn-complete","turn-id":"t1","input-messages":["fix it"],"last-assistant-message":"Done; I changed \"three\" files. `+strings.Repeat("x", 300)+`"}`)
	// A Stop payload says nothing worth repeating.
	runHook(t, env, `{"session_id":"abc","hook_event_name":"Stop","stop_hook_active":false}`, "finished")

	bodies := receiver.received()
	if len(bodies) != 3 {
		t.Fatalf("received %d reports, want 3: %#v", len(bodies), bodies)
	}
	for _, body := range bodies {
		if body["session"] != "claude-work" {
			t.Fatalf("report names session %v, want claude-work: %#v", body["session"], body)
		}
		if user, ok := body["unixUser"].(string); !ok || user == "" {
			t.Fatalf("report carries no Unix user: %#v", body)
		}
	}
	if bodies[0]["event"] != "needs-input" || bodies[0]["summary"] != "Claude needs your permission to use Bash" {
		t.Fatalf("Notification report = %#v, want needs-input with the message", bodies[0])
	}
	summary, _ := bodies[1]["summary"].(string)
	if bodies[1]["event"] != "finished" || !strings.HasPrefix(summary, "Done; I changed ") || len(summary) != 200 {
		t.Fatalf("Codex report = %#v, want finished with the message cut to 200 characters", bodies[1])
	}
	if bodies[2]["event"] != "finished" {
		t.Fatalf("Stop report = %#v, want finished", bodies[2])
	}
	if _, said := bodies[2]["summary"]; said {
		t.Fatalf("Stop report invented a summary: %#v", bodies[2])
	}
}

// Without an address, or told an event CHROTE does not know, the script does
// nothing and still exits 0: it must never be the reason a harness fails.
func TestHookScriptIsQuietWhenItHasNothingToSay(t *testing.T) {
	receiver := newHookReceiver(t)

	runHook(t, nil, "", "finished")
	runHook(t, map[string]string{agentEventURLEnv: receiver.server.URL + "/api/agent/event"}, "", "started")
	runHook(t, map[string]string{agentEventURLEnv: receiver.server.URL + "/api/agent/event"}, "", "")

	if bodies := receiver.received(); len(bodies) != 0 {
		t.Fatalf("the script reported with nothing to say: %#v", bodies)
	}
}
