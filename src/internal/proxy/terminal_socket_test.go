package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrote/server/internal/core"
)

// TestNewTerminalProxyForSocket_PassesSocketToLaunchEnv proves the formations
// terminal proxy threads the explicit socket to its ttyd launch environment via
// CHROTE_TMUX_SOCKET, so the launch script attaches with `tmux -S <socket>` —
// the SAME socket the formations executor and session API use. A drift here
// would silently attach the browser terminal to the wrong (or empty) socket.
func TestNewTerminalProxyForSocket_PassesSocketToLaunchEnv(t *testing.T) {
	const socket = "/run/user/1000/chrote-formations-tmux/default"
	proxy := NewTerminalProxyForSocket(7684, socket)

	env := proxy.launchEnv()

	want := "CHROTE_TMUX_SOCKET=" + socket
	found := false
	for _, e := range env {
		if e == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("formations launch env does not contain %q; ttyd would not attach to the formations socket\nenv=%v", want, env)
	}
}

// TestNewTerminalProxy_CockpitLeavesSocketUnset locks the unchanged cockpit
// behavior: the cockpit proxy stays env/TMUX_TMPDIR driven and must NOT inject
// CHROTE_TMUX_SOCKET.
func TestNewTerminalProxy_CockpitLeavesSocketUnset(t *testing.T) {
	proxy := NewTerminalProxy(7683)

	for _, e := range proxy.launchEnv() {
		if strings.HasPrefix(e, "CHROTE_TMUX_SOCKET=") && e != "CHROTE_TMUX_SOCKET=" {
			t.Fatalf("cockpit proxy injected %q; cockpit terminal must stay TMUX_TMPDIR-driven", e)
		}
	}
}

// TestLaunchScript_AttachesWithExplicitSocketWhenSet asserts the launch script
// itself uses `tmux -S "$CHROTE_TMUX_SOCKET"` when that variable is set, so the
// ttyd attach cannot silently diverge from the executor socket. The script
// content is the contract between Go (which sets the env) and tmux (which must
// see the -S flag).
func TestLaunchScript_AttachesWithExplicitSocketWhenSet(t *testing.T) {
	t.Setenv("CHROTE_LAUNCH_SCRIPT", "")
	scriptPath := repoLaunchScriptPath(t)

	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read launch script %s: %v", scriptPath, err)
	}
	content := string(raw)

	// Must reference the socket variable and pass it via -S to tmux.
	if !strings.Contains(content, "CHROTE_TMUX_SOCKET") {
		t.Fatalf("launch script %s does not reference CHROTE_TMUX_SOCKET; formations attach cannot scope to the executor socket", scriptPath)
	}
	if !strings.Contains(content, `-S "$CHROTE_TMUX_SOCKET"`) && !strings.Contains(content, `-S "${CHROTE_TMUX_SOCKET}"`) {
		t.Fatalf("launch script %s does not pass -S \"$CHROTE_TMUX_SOCKET\" to tmux; ttyd would attach to the wrong socket", scriptPath)
	}
}

// repoLaunchScriptPath locates the repo's terminal-launch.sh relative to this
// test file (src/internal/proxy -> repo root).
func repoLaunchScriptPath(t *testing.T) string {
	t.Helper()
	// core.GetLaunchScript reads CHROTE_LAUNCH_SCRIPT; in tests it is unset and
	// falls back to /usr/local/bin which may not exist in the working tree.
	_ = core.GetLaunchScript
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// wd == <repo>/src/internal/proxy
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	return filepath.Join(repoRoot, "terminal-launch.sh")
}
