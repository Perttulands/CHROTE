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
  case "$arg" in
    has-session) exit "${FAKE_TMUX_HAS_SESSION_STATUS:-0}" ;;
    list-clients) printf '%s' "${FAKE_TMUX_CLIENTS:-}"; exit 0 ;;
    refresh-client) exit "${FAKE_TMUX_REFRESH_STATUS:-0}" ;;
  esac
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

// received reports what has already arrived and waits for nothing more. A test
// asserting that output was withheld reads this after an event that proves the
// session already wrote it, rather than after a silence window.
func (c *terminalClient) received() string {
	c.t.Helper()
	collected := &strings.Builder{}
	for {
		select {
		case frame, open := <-c.frames:
			if !open {
				return collected.String()
			}
			collected.WriteString(frame.text)
		default:
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

// The two Origin cases below run through the real handler, because the check
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

// One *sizing* client per window is what the flags buy, and no more than that:
// nothing here ever attaches with -d, so a second viewer watches a session
// instead of evicting whoever is already in it.
func TestTerminal_ViewingModeSelectsTheAttachFlags(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mode    string
		clients string
		want    string
	}{
		{
			name: "a tile takes a sizing seat nobody holds",
			mode: "tile",
			want: "-S /tmp/tmux-b attach-session -t shell-one",
		},
		{
			name:    "a tile observes a session another client already sizes",
			mode:    "tile",
			clients: "/dev/pts/907\tattached,focused,UTF-8\n",
			want:    "-S /tmp/tmux-b attach-session -f ignore-size -t shell-one",
		},
		{
			name:    "a tile takes the seat back from clients that all ignore size",
			mode:    "tile",
			clients: "/dev/pts/907\tattached,ignore-size,UTF-8\n",
			want:    "-S /tmp/tmux-b attach-session -t shell-one",
		},
		{
			name: "a peek never takes the seat",
			mode: "peek",
			want: "-S /tmp/tmux-b attach-session -f ignore-size -t shell-one",
		},
		{
			name:    "a peek never takes the seat from a sizing client either",
			mode:    "peek",
			clients: "/dev/pts/907\tattached,focused,UTF-8\n",
			want:    "-S /tmp/tmux-b attach-session -f ignore-size -t shell-one",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
			t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(`printf 'attached\n'; sleep 10`))
			t.Setenv("FAKE_TMUX_CLIENTS", tt.clients)

			harness.dial("arg="+tt.mode+"&arg=shell-one&arg=bob", 80, 24).readUntil("attached")

			args := harness.tmuxArgs()
			if !strings.Contains(args, tt.want) {
				t.Fatalf("%s attach args %q do not contain %q", tt.mode, args, tt.want)
			}
			if strings.Contains(args, "attach-session -d") {
				t.Fatalf("attach displaced the clients already watching; args=%q", args)
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

// Claiming has to move the sizing seat, not merely take it: clearing this
// client's flag while another client still lacks it leaves two sizing clients
// and brings back the `window-size latest` flapping this model exists to stop.
func TestTerminal_ClaimFlagsTheOtherSizingClientsBeforeClearingItsOwn(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(`printf 'attached\n'; cat`))
	t.Setenv("FAKE_TMUX_CLIENTS", "/dev/pts/907\tattached,focused,UTF-8\n/dev/pts/909\tattached,ignore-size,UTF-8\n")

	client := harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24)
	client.readUntil("attached")
	client.send([]byte("4"))
	// Frames are dispatched one at a time on one goroutine, so an echo of the
	// next frame is proof the claim before it has already been handled.
	client.send([]byte("0claimed\n"))
	client.readUntil("claimed")

	args := harness.tmuxArgs()
	handOver := strings.Index(args, "refresh-client -t /dev/pts/907 -f ignore-size")
	takeOver := strings.Index(args, "-f !ignore-size")
	if handOver < 0 {
		t.Fatalf("claim did not hand the size over from the other sizing client; args=%q", args)
	}
	if takeOver < 0 || takeOver < handOver {
		t.Fatalf("claim cleared its own flag before flagging the other sizer; args=%q", args)
	}
	if strings.Contains(args, "refresh-client -t /dev/pts/909") {
		t.Fatalf("claim flagged a client that was already ignoring size; args=%q", args)
	}
}

// A peek is an observer by construction. A claim frame on one is a client bug,
// and it must not move the sizing seat.
func TestTerminal_PeekCannotClaimTheSize(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(`printf 'attached\n'; cat`))
	t.Setenv("FAKE_TMUX_CLIENTS", "/dev/pts/907\tattached,focused,UTF-8\n")

	client := harness.dial("arg=peek&arg=shell-one&arg=bob", 80, 24)
	client.readUntil("attached")
	client.send([]byte("4"))
	client.send([]byte("0ignored\n"))
	client.readUntil("ignored")

	if args := harness.tmuxArgs(); strings.Contains(args, "refresh-client") {
		t.Fatalf("a peek moved the sizing seat; args=%q", args)
	}
}

// A refusal is an answer, and it has to arrive as one. Each of these ends the
// socket with a close frame rather than an abnormal close, because the browser
// reads an abnormal close as a lost connection and would dial the refusal again.
// None of them may reach tmux beyond the existence probe: a caller that names no
// mode must not be attached under a guessed one, and an explicit-socket terminal
// that fell back to the invoking user's ambient tmux server would attach the
// operator to the wrong pool.
func TestTerminal_RefusesWhatItCannotAttachAndSaysSo(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		resolve   ResolveTarget
		prepare   func(t *testing.T)
		query     string
		want      string
		wantNamed []string
		// probesTmux marks the one refusal that is allowed to have asked tmux
		// anything at all, because the refusal is the probe's answer.
		probesTmux bool
	}{
		{
			name:    "a viewing mode it does not know",
			resolve: defaultTarget("/tmp/tmux-b"),
			query:   "arg=shell-one&arg=bob",
			want:    "viewing mode",
		},
		{
			name:    "no session name at all",
			resolve: defaultTarget("/tmp/tmux-b"),
			query:   "arg=tile",
			want:    "session name",
		},
		{
			name:       "a session the configured socket does not hold",
			resolve:    defaultTarget("/tmp/tmux-b"),
			prepare:    func(t *testing.T) { t.Setenv("FAKE_TMUX_HAS_SESSION_STATUS", "1") },
			query:      "arg=tile&arg=shell-one&arg=bob",
			want:       "not available",
			wantNamed:  []string{"shell-one", "/tmp/tmux-b"},
			probesTmux: true,
		},
		{
			// Socket resolution has one implementation and the transport holds
			// none of it, so a resolution failure arrives from outside and still
			// has to fail loud.
			name: "a target the resolver refuses",
			resolve: func(string) (Target, error) {
				return Target{}, errors.New(`Unix user "ghost" is not allowed for terminal launch`)
			},
			query: "arg=tile&arg=shell-one&arg=ghost",
			want:  "not allowed for terminal launch",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newTerminalHarness(t, testCase.resolve)
			if testCase.prepare != nil {
				testCase.prepare(t)
			}

			client := harness.dial(testCase.query, 80, 24)
			refusal := client.readUntil(testCase.want)

			if !strings.Contains(refusal.text, "CHROTE") {
				t.Fatalf("refusal %q is not attributed to CHROTE", refusal.text)
			}
			for _, named := range testCase.wantNamed {
				if !strings.Contains(refusal.text, named) {
					t.Fatalf("refusal %q does not name %q", refusal.text, named)
				}
			}
			if code := client.closeCode(); code != websocket.CloseNormalClosure {
				t.Fatalf("close code = %d, want %d: a refused attach must not look like a lost connection", code, websocket.CloseNormalClosure)
			}

			args := harness.tmuxArgs()
			if testCase.probesTmux {
				if strings.Contains(args, "attach-session") {
					t.Fatalf("CHROTE attached anyway after the probe failed; args=%q", args)
				}
				return
			}
			if strings.TrimSpace(args) != "" {
				t.Fatalf("tmux was invoked for a request CHROTE refused; args=%q", args)
			}
		})
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
//
// Every step here waits for an event rather than for a duration. The session
// writes only when the client tells it to, and the client sends those prompts
// behind the pause frame: one relay loop dispatches control and input frames in
// order, so a prompt the session has read proves the pause was already in force.
// `in-flight` satisfies the one read that may still be outstanding, and its
// arrival is what proves the next read is the gated one. The session then
// reports off the wire, through a file, that it has finished writing, because
// the whole point is that its writing is not reaching the wire. `end-of-writing`
// is written after `held-back`, so a resumed stream carrying the marker but not
// the line before it would mean output was dropped rather than held.
func TestTerminal_HonoursClientFlowControl(t *testing.T) {
	harness := newTerminalHarness(t, defaultTarget("/tmp/tmux-b"))
	writingDone := filepath.Join(t.TempDir(), "writing-done")
	t.Setenv("FAKE_TMUX_ATTACH", harness.attachScript(fmt.Sprintf(`
stty -echo
printf 'ready\n'
read -r _
printf 'in-flight\n'
read -r _
printf 'held-back\n'
printf 'end-of-writing\n'
: > %q
sleep 10`, writingDone)))

	client := harness.dial("arg=tile&arg=shell-one&arg=bob", 80, 24)
	client.readUntil("ready")

	client.send([]byte{clientPause})
	client.send(append([]byte{clientInput}, []byte("release the outstanding read\r")...))
	client.readUntil("in-flight")

	client.send(append([]byte{clientInput}, []byte("write behind the pause\r")...))
	waitForFile(t, writingDone)
	if withheld := client.received(); strings.Contains(withheld, "held-back") {
		t.Fatalf("CHROTE issued another pty read while the client had paused it; got %q", withheld)
	}

	resumedAt := time.Now()
	client.send([]byte{clientResume})
	released := client.readUntil("end-of-writing")
	if !strings.Contains(released.text, "held-back") {
		t.Fatalf("the resumed stream skipped what the session wrote behind the pause; got %q", released.text)
	}
	if released.at.Before(resumedAt) {
		t.Fatal("output written behind the pause reached the client before it resumed the stream")
	}
}

// waitForFile blocks until the session's off-the-wire receipt appears.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("the session never reported finishing its writes: %s", path)
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

// The terminal type is pinned whatever the service inherited, because the
// browser's terminal is the one being drawn for. The locale is the other way
// round: a UTF-8 one is kept so the operator's own language survives, and one
// that is not UTF-8 is replaced, because tmux draws box characters as mojibake
// under a single-byte locale.
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

	t.Setenv("LANG", "de_DE@euro")

	env = attachEnv()
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
