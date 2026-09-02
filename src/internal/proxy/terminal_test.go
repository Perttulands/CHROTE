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
	return newTerminalHarnessWithOrigins(t, resolve, nil)
}

func newTerminalHarnessWithOrigins(t *testing.T, resolve ResolveTarget, allowedOrigins []string) *terminalHarness {
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

	proxy := NewTerminalProxy(resolve, allowedOrigins)
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
	// closed carries the error that ended the read loop. The browser reads a
	// close frame as the terminal ending and its absence as a lost connection,
	// so which one arrived is part of the contract.
	closed chan error
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

	client := &terminalClient{t: h.t, conn: conn, frames: make(chan stampedFrame, 256), closed: make(chan error, 1)}
	go func() {
		defer close(client.frames)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				client.closed <- err
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

// closeCode reports the WebSocket close code the server sent, or
// websocket.CloseAbnormalClosure when it sent no close frame at all.
func (c *terminalClient) closeCode() int {
	c.t.Helper()
	select {
	case err := <-c.closed:
		var closeErr *websocket.CloseError
		if errors.As(err, &closeErr) {
			return closeErr.Code
		}
		c.t.Fatalf("the terminal socket ended without a close frame: %v", err)
		return 0
	case <-time.After(15 * time.Second):
		c.t.Fatal("the terminal socket stayed open")
		return 0
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

// handshakeFrom performs only the WebSocket handshake, the way a page does when
// it opens the socket, and reports the status CHROTE answered with. An empty
// origin sends no Origin header at all, which is what a non-browser client does.
func (h *terminalHarness) handshakeFrom(origin string) (int, error) {
	h.t.Helper()
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	url := "ws" + strings.TrimPrefix(h.server.URL, "http") + "/terminal/ws?arg=tile&arg=work"
	conn, response, err := (&websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{"tty"},
	}).Dial(url, header)
	if conn != nil {
		conn.Close()
	}
	if response == nil {
		h.t.Fatalf("no handshake response for origin %q: %v", origin, err)
	}
	return response.StatusCode, err
}

func defaultTarget(socket string) ResolveTarget {
	return func(string) (Target, error) {
		return Target{Socket: socket, UnixUser: "bob"}, nil
	}
}

// The four Origin cases below run through the real handler, because the check
// lives in the upgrade and a unit test of the policy alone would not prove the
// handler consults it.

func TestTerminal_SameOriginIsServedWithNoConfiguredOrigins(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-origin"))

	status, err := harness.handshakeFrom(harness.server.URL)
	if err != nil {
		t.Fatalf("the dashboard's own origin was refused: %v (status %d)", err, status)
	}
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("same-origin handshake status = %d, want %d", status, http.StatusSwitchingProtocols)
	}
}

func TestTerminal_ForeignOriginIsRefusedBeforeTmux(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-origin"))

	status, err := harness.handshakeFrom("https://evil.example")
	if err == nil {
		t.Fatal("a foreign browser origin opened a terminal socket")
	}
	if status != http.StatusForbidden {
		t.Fatalf("foreign-origin handshake status = %d, want %d", status, http.StatusForbidden)
	}
	if args := harness.tmuxArgs(); args != "" {
		t.Fatalf("a refused origin still reached tmux: %q", args)
	}
}

func TestTerminal_ConfiguredOriginIsServed(t *testing.T) {
	harness := newTerminalHarnessWithOrigins(t, defaultTarget("/tmp/tmux-origin"),
		[]string{"https://chrote.example", " https://second.example "})

	for _, origin := range []string{"https://chrote.example", "https://second.example"} {
		status, err := harness.handshakeFrom(origin)
		if err != nil {
			t.Fatalf("configured origin %q was refused: %v (status %d)", origin, err, status)
		}
		if status != http.StatusSwitchingProtocols {
			t.Fatalf("configured origin %q handshake status = %d, want %d", origin, status, http.StatusSwitchingProtocols)
		}
	}

	status, err := harness.handshakeFrom("https://evil.example")
	if err == nil {
		t.Fatal("configuring origins let an unconfigured one through")
	}
	if status != http.StatusForbidden {
		t.Fatalf("unconfigured-origin handshake status = %d, want %d", status, http.StatusForbidden)
	}
}

func TestTerminal_AbsentOriginIsServed(t *testing.T) {
	harness := newTerminalHarnessWithOrigins(t, defaultTarget("/tmp/tmux-origin"),
		[]string{"https://chrote.example"})

	status, err := harness.handshakeFrom("")
	if err != nil {
		t.Fatalf("a client sending no Origin was refused: %v (status %d)", err, status)
	}
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("no-Origin handshake status = %d, want %d", status, http.StatusSwitchingProtocols)
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

// The browser tells "the terminal ended" from "the connection was lost" by the
// close frame alone, and acts on the difference: a lost connection is dialled
// again, an ended one is not. Both ends of that contract are asserted here,
// because a restart or a network drop reaches the browser as an abnormal close
// precisely by CHROTE sending nothing.
func TestTerminal_EndOfTheAttachClosesWithACloseFrame(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(`printf 'bye\n'`))

	client := harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24)
	client.readUntil("bye")

	if code := client.closeCode(); code != websocket.CloseNormalClosure {
		t.Fatalf("close code = %d, want %d: a terminal that ended must not look like a lost connection", code, websocket.CloseNormalClosure)
	}
}

// A refusal is an answer too. Left as an abnormal close it would read as a lost
// connection, and the tile would dial the refusal again.
func TestTerminal_RefusalClosesWithACloseFrame(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	t.Setenv("FAKE_TMUX_HAS_SESSION_STATUS", "1")

	client := harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24)
	client.readUntil("not available")

	if code := client.closeCode(); code != websocket.CloseNormalClosure {
		t.Fatalf("close code = %d, want %d: a refused attach must not look like a lost connection", code, websocket.CloseNormalClosure)
	}
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

func TestAttachEnv_PinsTheTerminalTypeAndKeepsAUTF8Locale(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("LANG", "fi_FI.UTF-8")

	env := attachEnv()
	for _, entry := range env {
		if strings.HasPrefix(entry, "TERM=dumb") {
			t.Fatalf("attach environment still carries %q", entry)
		}
	}
	if !hasEntry(env, "TERM="+terminalTermType) {
		t.Fatalf("attach environment does not pin TERM; env=%v", env)
	}
	if !hasEntry(env, "LANG=fi_FI.UTF-8") {
		t.Fatalf("attach environment did not keep the inherited UTF-8 LANG; env=%v", env)
	}
	if hasEntry(env, "LANG="+fallbackAttachLang) {
		t.Fatalf("attach environment overrode an inherited UTF-8 LANG; env=%v", env)
	}
}

func TestAttachEnv_FallsBackWhenTheInheritedLocaleIsNotUTF8(t *testing.T) {
	t.Setenv("LANG", "de_DE@euro")

	env := attachEnv()
	if !hasEntry(env, "LANG="+fallbackAttachLang) {
		t.Fatalf("attach environment did not fall back to the pinned LANG; env=%v", env)
	}
	if hasEntry(env, "LANG=de_DE@euro") {
		t.Fatalf("attach environment kept a non-UTF-8 LANG; env=%v", env)
	}
}

func TestAttachLang(t *testing.T) {
	for _, testCase := range []struct {
		inherited string
		want      string
	}{
		{inherited: "en_US.UTF-8", want: "en_US.UTF-8"},
		{inherited: "fi_FI.UTF-8", want: "fi_FI.UTF-8"},
		{inherited: "en_GB.utf8", want: "en_GB.utf8"},
		{inherited: "C.UTF-8", want: "C.UTF-8"},
		{inherited: "de_DE.utf-8", want: "de_DE.utf-8"},
		{inherited: "", want: fallbackAttachLang},
		{inherited: "C", want: fallbackAttachLang},
		{inherited: "POSIX", want: fallbackAttachLang},
		{inherited: "de_DE@euro", want: fallbackAttachLang},
		{inherited: "fi_FI.ISO-8859-1", want: fallbackAttachLang},
	} {
		if got := attachLang(testCase.inherited); got != testCase.want {
			t.Errorf("attachLang(%q) = %q, want %q", testCase.inherited, got, testCase.want)
		}
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
