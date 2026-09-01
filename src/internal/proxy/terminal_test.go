package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The tests below never touch a real tmux server. CHROTE_TMUX_BIN pins the
// attach path to a fake that records its argv and then behaves like an ordinary
// program on a tty, which exercises the real pty, the real relay and the real
// hangup without putting a live session anywhere near the suite.

type terminalHarness struct {
	t        *testing.T
	server   *httptest.Server
	argsPath string
}

func newTerminalHarness(t *testing.T, resolve ResolveTarget) *terminalHarness {
	t.Helper()
	dir := t.TempDir()
	harness := &terminalHarness{t: t, argsPath: filepath.Join(dir, "tmux.args")}

	fake := filepath.Join(dir, "fake-tmux")
	script := `#!/bin/bash
printf '%s\n' "$*" >> "$FAKE_TMUX_ARGS"
for arg in "$@"; do
  if [ "$arg" = "has-session" ]; then
    exit "${FAKE_TMUX_HAS_SESSION_STATUS:-0}"
  fi
done
exec bash "$FAKE_TMUX_ATTACH"
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("CHROTE_TMUX_BIN", fake)
	t.Setenv("FAKE_TMUX_ARGS", harness.argsPath)
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript("exit 0"))

	proxy := NewTerminalProxy(resolve)
	harness.server = httptest.NewServer(proxy.Handler())
	t.Cleanup(harness.server.Close)
	return harness
}

// attachScript writes the program the fake tmux execs in place of an attach.
func (h *terminalHarness) attachScript(body string) string {
	h.t.Helper()
	path := filepath.Join(h.t.TempDir(), "attach.sh")
	if err := os.WriteFile(path, []byte("#!/bin/bash\n"+body+"\n"), 0o755); err != nil {
		h.t.Fatalf("write attach script: %v", err)
	}
	return path
}

func (h *terminalHarness) tmuxArgs() string {
	h.t.Helper()
	raw, err := os.ReadFile(h.argsPath)
	if err != nil {
		return ""
	}
	return string(raw)
}

// stampedFrame records when a server frame reached the client, so a test can
// tell output that was already on its way from output released later.
type stampedFrame struct {
	text string
	at   time.Time
}

// terminalClient is a browser, reading in the background the way one does.
type terminalClient struct {
	t      *testing.T
	conn   *websocket.Conn
	frames chan stampedFrame
}

// dial opens a terminal connection and sends the opening handshake, the way the
// dashboard does.
func (h *terminalHarness) dial(query string, cols, rows int) *terminalClient {
	h.t.Helper()
	url := "ws" + strings.TrimPrefix(h.server.URL, "http") + "/terminal/ws?" + query
	conn, _, err := (&websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{"tty"},
	}).Dial(url, nil)
	if err != nil {
		h.t.Fatalf("dial terminal WebSocket: %v", err)
	}
	h.t.Cleanup(func() { conn.Close() })

	handshake, err := json.Marshal(map[string]any{"AuthToken": "", "columns": cols, "rows": rows})
	if err != nil {
		h.t.Fatalf("marshal handshake: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, handshake); err != nil {
		h.t.Fatalf("send handshake: %v", err)
	}

	client := &terminalClient{t: h.t, conn: conn, frames: make(chan stampedFrame, 256)}
	go func() {
		defer close(client.frames)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if len(message) == 0 || message[0] != serverOutput {
				continue
			}
			client.frames <- stampedFrame{text: string(message[1:]), at: time.Now()}
		}
	}()
	return client
}

func (c *terminalClient) send(frame []byte) {
	c.t.Helper()
	if err := c.conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		c.t.Fatalf("send frame %q: %v", frame, err)
	}
}

// readUntil collects output until want appears, and reports when it arrived.
func (c *terminalClient) readUntil(want string) stampedFrame {
	c.t.Helper()
	collected := &strings.Builder{}
	timeout := time.After(15 * time.Second)
	for {
		select {
		case frame, open := <-c.frames:
			if !open {
				c.t.Fatalf("terminal closed before %q arrived; got %q", want, collected.String())
			}
			collected.WriteString(frame.text)
			if strings.Contains(collected.String(), want) {
				return stampedFrame{text: collected.String(), at: frame.at}
			}
		case <-timeout:
			c.t.Fatalf("timed out waiting for %q; got %q", want, collected.String())
		}
	}
}

// drainFor collects everything that arrives over a window, which is how a test
// asserts that something did not arrive.
func (c *terminalClient) drainFor(window time.Duration) string {
	c.t.Helper()
	collected := &strings.Builder{}
	deadline := time.After(window)
	for {
		select {
		case frame, open := <-c.frames:
			if !open {
				return collected.String()
			}
			collected.WriteString(frame.text)
		case <-deadline:
			return collected.String()
		}
	}
}

func (c *terminalClient) waitForClose() {
	c.t.Helper()
	timeout := time.After(15 * time.Second)
	for {
		select {
		case _, open := <-c.frames:
			if !open {
				return
			}
		case <-timeout:
			c.t.Fatal("the terminal socket stayed open after the attach ended")
		}
	}
}

func defaultTarget(socket string) ResolveTarget {
	return func(string) (Target, error) {
		return Target{Socket: socket, UnixUser: "bob"}, nil
	}
}

func TestTerminal_PlainHTTPIsNotServed(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	client := &http.Client{Timeout: 5 * time.Second}
	for _, path := range []string{"/terminal/", "/terminal/token", "/terminal/xterm.css", "/terminal/ws"} {
		resp, err := client.Get(harness.server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for %s, got %d", path, resp.StatusCode)
		}
	}
}

// One sizing client per window is what the flags buy: a tile takes the session
// over with -d, and a peek attaches without ever sizing the window.
func TestTerminal_ViewingModeSelectsTheAttachFlags(t *testing.T) {
	for _, tt := range []struct {
		mode string
		want string
	}{
		{mode: "tile", want: "-S /tmp/tmux-b attach-session -d -t shell-one"},
		{mode: "peek", want: "-S /tmp/tmux-b attach-session -f ignore-size -t shell-one"},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
			t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(`printf 'attached\n'; sleep 10`))

			harness.dial("arg="+tt.mode+"&arg=shell-one&arg=bob", 80, 24).readUntil("attached")

			args := harness.tmuxArgs()
			if !strings.Contains(args, tt.want) {
				t.Fatalf("%s attach args %q do not contain %q", tt.mode, args, tt.want)
			}
			if !strings.Contains(args, "-S /tmp/tmux-b has-session -t shell-one") {
				t.Fatalf("%s did not probe the configured socket first; args=%q", tt.mode, args)
			}
			if strings.Contains(args, "resize-window") {
				t.Fatalf("attach used resize-window, which pins window-size manual; args=%q", args)
			}
		})
	}
}

// A caller that names no mode must not be attached under a guessed one.
func TestTerminal_RefusesAnUnknownViewingMode(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))

	refusal := harness.dial("arg=shell-one&arg=bob", 80, 24).readUntil("viewing mode")

	if !strings.Contains(refusal.text, "CHROTE") {
		t.Fatalf("refusal %q is not attributed to CHROTE", refusal.text)
	}
	if args := harness.tmuxArgs(); strings.TrimSpace(args) != "" {
		t.Fatalf("tmux was invoked despite the unknown mode; args=%q", args)
	}
}

func TestTerminal_RefusesAMissingSessionName(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))

	harness.dial("arg=tile", 80, 24).readUntil("session name")

	if args := harness.tmuxArgs(); strings.TrimSpace(args) != "" {
		t.Fatalf("tmux was invoked without a session name; args=%q", args)
	}
}

// Explicit-socket terminals must fail loud instead of falling back to the
// invoking user's ambient tmux server, which would attach the operator to the
// wrong pool.
func TestTerminal_RefusesASessionMissingFromTheConfiguredSocket(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	t.Setenv("FAKE_TMUX_HAS_SESSION_STATUS", "1")

	refusal := harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24).readUntil("not available")

	for _, want := range []string{"shell-one", "/tmp/tmux-b"} {
		if !strings.Contains(refusal.text, want) {
			t.Fatalf("refusal %q does not name %q", refusal.text, want)
		}
	}
	if args := harness.tmuxArgs(); strings.Contains(args, "attach-session") {
		t.Fatalf("CHROTE attached anyway after the probe failed; args=%q", args)
	}
}

// Socket resolution has one implementation and the transport holds none of it,
// so a resolution failure has to arrive from outside and still fail loud.
func TestTerminal_ReportsAResolutionFailure(t *testing.T) {
	harness := newTerminalHarness(t, func(string) (Target, error) {
		return Target{}, errors.New(`Unix user "ghost" is not allowed for terminal launch`)
	})

	harness.dial("arg=tile&arg=shell-one&arg=ghost", 80, 24).readUntil("not allowed for terminal launch")

	if args := harness.tmuxArgs(); strings.TrimSpace(args) != "" {
		t.Fatalf("tmux was invoked with an unresolved target; args=%q", args)
	}
}

func TestTerminal_ResolvesTheRequestedUnixUser(t *testing.T) {
	requested := make(chan string, 1)
	harness := newTerminalHarness(t, func(unixUser string) (Target, error) {
		requested <- unixUser
		return Target{Socket: "/tmp/tmux-b", UnixUser: unixUser}, nil
	})
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(`printf 'attached\n'; sleep 10`))

	harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24).readUntil("attached")

	select {
	case got := <-requested:
		if got != "bob" {
			t.Fatalf("resolver saw Unix user %q, want %q", got, "bob")
		}
	default:
		t.Fatal("the transport resolved a target without consulting the injected resolver")
	}
}

// The pty is real, so input, echo and output all have to survive the round trip.
func TestTerminal_RelaysInputAndOutput(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(`
printf 'ready\n'
while IFS= read -r line; do printf 'GOT:%s\n' "$line"; done`))

	client := harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24)
	client.readUntil("ready")

	client.send(append([]byte{clientInput}, []byte("hello\r")...))
	client.readUntil("GOT:hello")
}

// The pty is created at the size the handshake reported, and a resize frame
// changes it, which is what raises SIGWINCH inside the session.
func TestTerminal_SizesThePtyFromTheHandshakeAndResizes(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(`while :; do stty size; sleep 0.05; done`))

	client := harness.dial("arg=tile&arg=shell-one&arg=bob", 137, 40)
	client.readUntil("40 137")

	resize, err := json.Marshal(map[string]int{"columns": 100, "rows": 30})
	if err != nil {
		t.Fatalf("marshal resize: %v", err)
	}
	client.send(append([]byte{clientResize}, resize...))
	client.readUntil("30 100")
}

// The tile state model depends on a connection-closed event, so the end of the
// attach has to reach the browser rather than hanging the socket open.
func TestTerminal_ClosesWhenTheAttachExits(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(`printf 'bye\n'`))

	client := harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24)
	client.readUntil("bye")
	client.waitForClose()
}

// A browser that goes away must not leave a tmux client attached to a live
// session. Closing the pty master hangs the attach up; the same hangup is what
// ends every attach when CHROTE exits, which is why terminal attaches do not
// survive a restart (ADR-0013).
func TestTerminal_ClientDisconnectEndsTheAttach(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	pidPath := filepath.Join(t.TempDir(), "attach.pid")
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(
		fmt.Sprintf("printf '%%s\\n' \"$$\" > %q\nprintf 'attached\\n'\nwhile :; do sleep 0.05; done", pidPath)))

	client := harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24)
	client.readUntil("attached")

	raw, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("attach never reported its pid: %v", err)
	}
	pid := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(raw)), "%d", &pid); err != nil || pid <= 0 {
		t.Fatalf("attach pid %q is unusable: %v", raw, err)
	}

	client.conn.Close()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		// Kill(0) still succeeds on a zombie, so this also proves the exit was
		// reaped rather than merely signalled.
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("attach client %d is still running after the browser disconnected", pid)
}

// Flow control is the client's, not ours: while it says stop, CHROTE issues no
// further pty read, and what the session wrote meanwhile arrives on resume. One
// read can already be outstanding when the pause lands, so the guarantee is
// about the next read, not about instant silence.
func TestTerminal_HonoursClientFlowControl(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(
		`printf 'ready\n'; sleep 0.3; printf 'in-flight\n'; sleep 0.3; printf 'held-back\n'; sleep 10`))

	client := harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24)
	client.readUntil("ready")

	client.send([]byte{clientPause})
	if held := client.drainFor(2 * time.Second); strings.Contains(held, "held-back") {
		t.Fatalf("CHROTE issued another pty read while the client had paused it; got %q", held)
	}

	resumedAt := time.Now()
	client.send([]byte{clientResume})
	released := client.readUntil("held-back")
	if released.at.Before(resumedAt) {
		t.Fatal("held-back output arrived before the client resumed the stream")
	}
}

func TestPTY_ResizeRefusesAnUnusableSize(t *testing.T) {
	pty, err := openPTY()
	if err != nil {
		t.Fatalf("openPTY: %v", err)
	}
	defer pty.master.Close()
	defer pty.slave.Close()

	for _, size := range [][2]int{{0, 24}, {80, 0}, {-1, 24}, {80, 70000}} {
		if err := pty.resize(size[0], size[1]); err == nil {
			t.Fatalf("resize accepted %dx%d", size[0], size[1])
		}
	}
	if err := pty.resize(80, 24); err != nil {
		t.Fatalf("resize refused a usable size: %v", err)
	}
}

func TestAttachEnv_PinsTheTerminalTypeAndLocale(t *testing.T) {
	t.Setenv("TERM", "dumb")

	env := attachEnv()
	for _, entry := range env {
		if strings.HasPrefix(entry, "TERM=dumb") {
			t.Fatalf("attach environment still carries %q", entry)
		}
	}
	if !hasEntry(env, "TERM="+terminalTermType) {
		t.Fatalf("attach environment does not pin TERM; env=%v", env)
	}
	if !hasEntry(env, "LANG=en_US.UTF-8") {
		t.Fatalf("attach environment does not pin LANG; env=%v", env)
	}
}

func hasEntry(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
