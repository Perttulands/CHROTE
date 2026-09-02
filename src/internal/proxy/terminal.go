// Package proxy owns CHROTE's terminal transport. It attaches to a configured
// tmux session on a pty it allocates itself and relays that pty over its own
// WebSocket (ADR-0018). There is no ttyd: no child web server, no vendored
// binary, and no shell launch script duplicating socket resolution.
//
// The wire protocol is unchanged from the one the dashboard already speaks.
// Frames are binary with a one-byte ASCII command prefix. The client opens with
// an unprefixed JSON handshake and then sends `0` input, `1` resize as JSON and
// `2`/`3` flow control; the server sends `0` output.
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chrote/server/internal/core"
	"github.com/gorilla/websocket"
)

const (
	clientInput  = '0'
	clientResize = '1'
	clientPause  = '2'
	clientResume = '3'
	// clientClaim asks for this connection to become the session's one sizing
	// client. It is CHROTE's own addition to the wire protocol; ttyd never
	// defined a `4` frame, and nothing else in the dashboard sends one.
	clientClaim = '4'

	serverOutput = '0'
)

// terminalTermType is the TERM the attach client runs under. The dashboard
// renders xterm.js, so this describes what is actually on the other end.
const terminalTermType = "xterm-256color"

// fallbackAttachLang is the locale the attach client runs under when the
// service inherited no UTF-8 locale of its own.
const fallbackAttachLang = "en_US.UTF-8"

// Target is where one terminal attach runs: the tmux socket to attach on, the
// directory the attach client starts in, and the Unix user both belong to.
type Target struct {
	Socket   string
	WorkDir  string
	UnixUser string
}

// ResolveTarget maps a requested Unix user to its configured tmux socket and
// working directory. CHROTE resolves that in exactly one place; the server
// injects it here so this package carries no second copy of the rules.
type ResolveTarget func(unixUser string) (Target, error)

// TerminalProxy serves the terminal WebSocket.
type TerminalProxy struct {
	resolve  ResolveTarget
	upgrader websocket.Upgrader
}

// NewTerminalProxy creates the TerminalProxy served at /terminal/. It takes the
// same configured browser origins the CORS middleware takes, because the CORS
// headers that middleware writes do not constrain a WebSocket handshake; see
// originPolicy for what the check is and is not.
func NewTerminalProxy(resolve ResolveTarget, allowedOrigins []string) *TerminalProxy {
	policy := newOriginPolicy(allowedOrigins)
	return &TerminalProxy{
		resolve: resolve,
		upgrader: websocket.Upgrader{
			CheckOrigin:  policy.allows,
			Subprotocols: []string{"tty"},
		},
	}
}

// viewingMode is how a connection views a session, and it decides the attach
// flags. Neither mode displaces anyone: CHROTE does not attach with -d, so a
// second device watches a session live rather than evicting the first
// (ADR-0017 decision 1). A tile takes the sizing seat when it is free and
// watches at the current size when it is not, and `Claim` is how the operator
// takes that seat afterwards. A peek never takes it. Input is not suppressed in
// either mode.
type viewingMode string

const (
	modeTile viewingMode = "tile"
	modePeek viewingMode = "peek"
)

func (m viewingMode) valid() bool {
	return m == modeTile || m == modePeek
}

// attachFlags decides how this connection attaches, from what was already
// sizing the window when it arrived. A window nobody sizes is sized by whoever
// attaches to it whatever the flag says, so the flagless attach is not what
// makes a first viewer the sizer — it is what stops a *later* viewer taking the
// window from it.
func (m viewingMode) attachFlags(sizedByAnother bool) []string {
	if m == modeTile && !sizedByAnother {
		return nil
	}
	return []string{"-f", ignoreSizeFlag}
}

// attachRequest is what the browser asked for, before any of it is resolved.
// The mode leads because the Unix user is optional, so a trailing mode would be
// positionally ambiguous.
type attachRequest struct {
	mode     viewingMode
	session  string
	unixUser string
}

func parseAttachRequest(args []string) (attachRequest, error) {
	request := attachRequest{}
	if len(args) > 0 {
		request.mode = viewingMode(strings.TrimSpace(args[0]))
	}
	if len(args) > 1 {
		request.session = strings.TrimSpace(args[1])
	}
	if len(args) > 2 {
		request.unixUser = strings.TrimSpace(args[2])
	}
	if !request.mode.valid() {
		return attachRequest{}, fmt.Errorf("CHROTE terminal requires a viewing mode of 'tile' or 'peek', got %q", string(request.mode))
	}
	if request.session == "" {
		return attachRequest{}, errors.New("CHROTE terminal requires a tmux session name")
	}
	return request, nil
}

// clientHandshake is the client's opening JSON frame. AuthToken is accepted and
// ignored: CHROTE's trust boundary is the network perimeter, and the browser
// origins allowed onto this socket are settled at the upgrade by originPolicy.
type clientHandshake struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
}

// clientResizeFrame is the payload of a `1` frame.
type clientResizeFrame struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
}

// Handler returns an http.Handler serving the terminal WebSocket. Only the
// upgrade is served: the dashboard is the terminal client, so nothing under
// /terminal answers a plain HTTP request.
func (tp *TerminalProxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.NotFound(w, r)
			return
		}
		tp.serve(w, r)
	})
}

// RegisterRoutes registers the terminal route.
func (tp *TerminalProxy) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/terminal/", tp.Handler())
}

func (tp *TerminalProxy) serve(w http.ResponseWriter, r *http.Request) {
	request, requestErr := parseAttachRequest(r.URL.Query()["arg"])

	conn, err := tp.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already written its own response. A refused Origin lands
		// here too, before any tmux socket is touched.
		log.Printf("terminal WebSocket upgrade failed: %v", err)
		return
	}
	session := &terminalConn{conn: conn}
	defer session.conn.Close()

	if requestErr != nil {
		session.refuse(requestErr)
		return
	}

	size, err := session.readHandshake()
	if err != nil {
		log.Printf("terminal handshake failed for %q: %v", request.session, err)
		return
	}

	target, err := tp.resolve(request.unixUser)
	if err != nil {
		session.refuse(err)
		return
	}
	if err := hasSession(target.Socket, request.session); err != nil {
		session.refuse(err)
		return
	}
	if err := session.attach(request, target, size); err != nil {
		session.refuse(err)
	}
}

// terminalConn is one browser connection. Every WebSocket write goes through
// send, because gorilla permits only one concurrent writer.
type terminalConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (c *terminalConn) send(frame []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, frame)
}

// refuse reports a refusal in the terminal itself and logs it. Failing loud in
// the pane is what the retired launch script's stderr did, and it is the only
// place an operator looking at a blank tile will see the reason.
//
// It closes with a close frame for the same reason the end of a terminal does:
// the browser reads an abnormal close as the connection being lost and dials
// again, and a refusal is an answer, not a lost connection.
func (c *terminalConn) refuse(reason error) {
	log.Printf("terminal attach refused: %v", reason)
	message := append([]byte{serverOutput}, []byte("CHROTE: "+reason.Error()+"\r\n")...)
	if err := c.send(message); err != nil {
		log.Printf("failed to report terminal refusal to the client: %v", err)
	}
	_ = c.conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "attach refused"),
		time.Now().Add(time.Second))
}

// readHandshake reads the client's opening frame, which carries the size the
// pty must be created at. No pty exists until it arrives.
func (c *terminalConn) readHandshake() (clientHandshake, error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, message, err := c.conn.ReadMessage()
	if err != nil {
		return clientHandshake{}, fmt.Errorf("read opening handshake: %w", err)
	}
	_ = c.conn.SetReadDeadline(time.Time{})

	handshake := clientHandshake{}
	if err := json.Unmarshal(message, &handshake); err != nil {
		return clientHandshake{}, fmt.Errorf("opening handshake is not JSON: %w", err)
	}
	if handshake.Columns < 1 || handshake.Rows < 1 {
		return clientHandshake{}, fmt.Errorf("opening handshake reported an unusable size %dx%d", handshake.Columns, handshake.Rows)
	}
	return handshake, nil
}

// attach runs the tmux attach on a pty and relays it until either end closes.
// It returns an error only when the attach could not be started; once bytes are
// flowing, the end of the session is not a failure.
func (c *terminalConn) attach(request attachRequest, target Target, size clientHandshake) error {
	// Read who is sizing the window before attaching, because the answer is
	// about the clients that were already there. Two connections opening the
	// same session in the same instant can both read an empty seat and both
	// take it; that is one `Claim` away from settled and is not worth a lock.
	sizing, err := sizingClientTTYs(target.Socket, request.session)
	if err != nil {
		return err
	}
	flags := request.mode.attachFlags(len(sizing) > 0)

	pty, err := openPTY()
	if err != nil {
		return err
	}
	defer pty.master.Close()
	if err := pty.resize(size.Columns, size.Rows); err != nil {
		pty.slave.Close()
		return err
	}

	args := append([]string{"-S", target.Socket, "attach-session"}, flags...)
	args = append(args, "-t", request.session)
	cmd := exec.Command(core.TmuxBin(), args...)
	cmd.Dir = startDir(target.WorkDir)
	cmd.Env = attachEnv()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pty.slave, pty.slave, pty.slave
	// The attach client leads its own session with the pty as its controlling
	// terminal. That is what makes closing the master hang it up, and it is why
	// the tmux server, which is not a child of CHROTE, is never in reach.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}

	if err := cmd.Start(); err != nil {
		pty.slave.Close()
		return fmt.Errorf("start tmux attach for session %q: %w", request.session, err)
	}
	// CHROTE holds no slave fd, so the master reads end when the attach exits.
	pty.slave.Close()

	flow := &flowGate{}
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		c.relayOutput(pty, flow)
		// Wake the input loop: the session ended, not the browser.
		_ = c.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "terminal ended"),
			time.Now().Add(time.Second))
		_ = c.conn.Close()
	}()

	c.relayInput(pty, flow, func() {
		if request.mode != modeTile {
			log.Printf("terminal claim ignored: a %s connection observes and never takes the size of session %q", request.mode, request.session)
			return
		}
		// A failed claim needs no reply frame: a claim that lands is visible in
		// the pane, because tmux redraws the window at this client's size.
		if err := claimSizing(target.Socket, request.session, pty.name); err != nil {
			log.Printf("terminal claim failed for session %q: %v", request.session, err)
		}
	})

	// Closing the master hangs up the attach client's controlling terminal, so
	// a browser that went away does not leave a tmux client behind. The same
	// hangup happens when CHROTE itself exits, by any means, because the kernel
	// closes this fd: terminal attaches do not survive a restart (ADR-0013).
	pty.master.Close()
	flow.resume()
	<-outputDone
	waitForExit(cmd)
	return nil
}

// relayOutput pumps pty output to the browser until the pty hangs up.
func (c *terminalConn) relayOutput(pty *pty, flow *flowGate) {
	buffer := make([]byte, 32*1024)
	for {
		flow.wait()
		n, err := pty.master.Read(buffer)
		if n > 0 {
			frame := make([]byte, n+1)
			frame[0] = serverOutput
			copy(frame[1:], buffer[:n])
			if err := c.send(frame); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// relayInput dispatches client frames onto the pty until the browser goes away.
// `claim` runs on the reading goroutine, so a claim cannot overlap another one
// on the same connection.
func (c *terminalConn) relayInput(pty *pty, flow *flowGate, claim func()) {
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if len(message) == 0 {
			continue
		}
		switch message[0] {
		case clientInput:
			if _, err := pty.master.Write(message[1:]); err != nil {
				return
			}
		case clientResize:
			resize := clientResizeFrame{}
			if err := json.Unmarshal(message[1:], &resize); err != nil {
				log.Printf("terminal resize frame is not JSON: %v", err)
				continue
			}
			if err := pty.resize(resize.Columns, resize.Rows); err != nil {
				log.Printf("terminal resize refused: %v", err)
			}
		case clientPause:
			flow.pause()
		case clientResume:
			flow.resume()
		case clientClaim:
			claim()
		}
	}
}

// waitForExit reaps the attach client. The hangup normally ends it at once; the
// kill is the bound on a client that ignores it. Only the exact process CHROTE
// started is signalled — never a process group, and never the tmux server.
func waitForExit(cmd *exec.Cmd) {
	kill := time.AfterFunc(2*time.Second, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer kill.Stop()
	_ = cmd.Wait()
}

// flowGate implements the client's `2`/`3` flow control: while paused, CHROTE
// stops reading the pty, so a firehose cannot outrun the renderer.
type flowGate struct {
	mu      sync.Mutex
	paused  bool
	resumed chan struct{}
}

func (g *flowGate) wait() {
	g.mu.Lock()
	if !g.paused {
		g.mu.Unlock()
		return
	}
	resumed := g.resumed
	g.mu.Unlock()
	<-resumed
}

func (g *flowGate) pause() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paused {
		return
	}
	g.paused = true
	g.resumed = make(chan struct{})
}

func (g *flowGate) resume() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.paused {
		return
	}
	g.paused = false
	close(g.resumed)
	g.resumed = nil
}

// hasSession refuses an attach the configured socket cannot serve. Terminals
// must fail loud rather than fall back to the invoking user's ambient tmux
// server, which would attach the operator to the wrong pool.
func hasSession(socket, session string) error {
	if strings.TrimSpace(socket) == "" {
		return errors.New("no tmux socket is configured for this terminal")
	}
	ctx, cancel := context.WithTimeout(context.Background(), tmuxProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, core.TmuxBin(), "-S", socket, "has-session", "-t", session)
	cmd.Env = attachEnv()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux session %q is not available on configured socket %q", session, socket)
	}
	return nil
}

// attachEnv is the environment every tmux invocation on the terminal path runs
// under: the server's own, with TERM pinned and LANG held to a UTF-8 locale.
// Nothing else is added or removed.
func attachEnv() []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "TERM=") || strings.HasPrefix(entry, "LANG=") {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "TERM="+terminalTermType, "LANG="+attachLang(os.Getenv("LANG")))
}

// attachLang keeps an inherited LANG that already names a UTF-8 locale and
// falls back to the pinned default otherwise. The terminal has to be UTF-8 or
// box drawing and agent TUI output break, but tmux exits rather than degrades
// when LANG names a locale the host has not generated, so naming one specific
// locale would kill the terminal on every host that generated a different one.
func attachLang(inherited string) string {
	lowered := strings.ToLower(inherited)
	if strings.HasSuffix(lowered, ".utf-8") || strings.HasSuffix(lowered, ".utf8") {
		return inherited
	}
	return fallbackAttachLang
}

// startDir is the directory the attach client runs in, falling back the way the
// retired launch script did rather than failing an attach over a missing
// directory.
func startDir(preferred string) string {
	for _, candidate := range []string{preferred, os.Getenv("HOME")} {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}
