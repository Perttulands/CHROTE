package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

const guardFormat = "#{client_tty}\t#{client_session}\t#{window_id}\t#{client_width}\t#{client_height}\t#{client_activity}\t#{client_flags}\t#{@chrote-size-guard}"

func TestParseTmuxClientsReadsSizeActivityAndFlags(t *testing.T) {
	output := strings.Join([]string{
		"/dev/pts/14\tclaude-iris\t@1\t80\t24\t1785114088\tattached,focused,UTF-8",
		"/dev/pts/17\tclaude-vw-1\t@2\t152\t68\t1785114088\tattached,focused,ignore-size,UTF-8\t1",
		"garbage line",
		"/dev/pts/23\tcodex-anim\t@3\tnot-a-number\t24\t1785114088\tattached",
	}, "\n")

	clients := parseTmuxClients(output)

	if len(clients) != 2 {
		t.Fatalf("parsed %d clients, want the two well-formed ones: %+v", len(clients), clients)
	}
	if clients[0].TTY != "/dev/pts/14" || clients[0].WindowID != "@1" || clients[0].Width != 80 || clients[0].Height != 24 || clients[0].IgnoreSize {
		t.Fatalf("first client = %+v, want the un-negotiated 80x24 client without the flag", clients[0])
	}
	if clients[1].WindowID != "@2" || !clients[1].IgnoreSize || !clients[1].GuardOwned || clients[1].Width != 152 {
		t.Fatalf("second client = %+v, want the guard-owned flagged 152-wide client", clients[1])
	}
	if !clients[0].ActivityAt.Equal(time.Unix(1785114088, 0)) {
		t.Fatalf("activity = %v, want the parsed unix timestamp", clients[0].ActivityAt)
	}
}

func TestPlanSizeGuardOnlyDisqualifiesIdleUnnegotiatedClients(t *testing.T) {
	now := time.Unix(1785200000, 0)
	idleAfter := 5 * time.Minute

	clients := []tmuxClient{
		// Hidden keep-alive iframe: still at the ttyd default and untouched since attach.
		{TTY: "/dev/pts/14", Width: 80, Height: 24, ActivityAt: now.Add(-30 * time.Minute)},
		// Operator reading output without typing, but at a real reported viewport.
		{TTY: "/dev/pts/15", Width: 152, Height: 68, ActivityAt: now.Add(-30 * time.Minute)},
		// Freshly attached 80x24 client that has not had time to report yet.
		{TTY: "/dev/pts/16", Width: 80, Height: 24, ActivityAt: now.Add(-10 * time.Second)},
		// Deliberately small terminal that is actively used.
		{TTY: "/dev/pts/17", Width: 80, Height: 24, ActivityAt: now.Add(-1 * time.Second)},
	}

	decisions := planSizeGuard(clients, now, idleAfter)

	if len(decisions) != 1 {
		t.Fatalf("decisions = %+v, want only the idle un-negotiated client", decisions)
	}
	if decisions[0].TTY != "/dev/pts/14" || !decisions[0].IgnoreSize {
		t.Fatalf("decision = %+v, want ignore-size on /dev/pts/14", decisions[0])
	}
}

func TestPlanSizeGuardRestoresAClientThatReportedItsViewport(t *testing.T) {
	now := time.Unix(1785200000, 0)

	// The tab was hidden and flagged; the operator opened it and it reported a
	// real viewport, so it must get its say over window size back.
	clients := []tmuxClient{
		{TTY: "/dev/pts/14", Width: 152, Height: 68, ActivityAt: now.Add(-30 * time.Minute), IgnoreSize: true},
	}

	decisions := planSizeGuard(clients, now, 5*time.Minute)

	if len(decisions) != 1 || decisions[0].IgnoreSize {
		t.Fatalf("decisions = %+v, want the flag cleared once a real size is reported", decisions)
	}
}

func TestPlanSizeGuardIsIdempotent(t *testing.T) {
	now := time.Unix(1785200000, 0)
	clients := []tmuxClient{
		{TTY: "/dev/pts/14", Width: 80, Height: 24, ActivityAt: now.Add(-30 * time.Minute), IgnoreSize: true},
		{TTY: "/dev/pts/15", Width: 152, Height: 68, ActivityAt: now.Add(-30 * time.Minute)},
	}

	if decisions := planSizeGuard(clients, now, 5*time.Minute); len(decisions) != 0 {
		t.Fatalf("decisions = %+v, want no churn when every client is already correct", decisions)
	}
}

func TestApplySizeGuardFlagsIdleClientAndLeavesOthersAlone(t *testing.T) {
	tmpDir := t.TempDir()
	argsPath := tmpDir + "/tmux-argv.txt"
	fakeTmux := tmpDir + "/tmux"
	script := `#!/bin/sh
{
  printf 'CALL\n'
  for arg in "$@"; do
    printf '%s\n' "$arg"
  done
} >> "$TMUX_ARGS_FILE"
for arg in "$@"; do
  case "$arg" in
    list-clients)
      printf '/dev/pts/14\tclaude-iris\t@1\t80\t24\t1\tattached,focused,UTF-8\n'
      printf '/dev/pts/17\tclaude-vw-1\t@2\t152\t68\t1\tattached,focused,UTF-8\n'
      exit 0
      ;;
  esac
done
case "$*" in
  *"show-options"*"window-size"*)
    printf 'latest\n'
    ;;
esac
exit 0
`
	if err := osWriteFileExecutable(fakeTmux, script); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("PATH", tmpDir+":/usr/bin:/bin")

	handler := NewTmuxHandler()
	// Activity of 1 is far in the past relative to now, so the 80x24 client is idle.
	applied, err := handler.applySizeGuard(context.Background(), "/tmp/guard-test.sock", time.Unix(1785200000, 0), 5*time.Minute)
	if err != nil {
		t.Fatalf("applySizeGuard returned error: %v", err)
	}
	if len(applied) != 1 || applied[0].TTY != "/dev/pts/14" || !applied[0].IgnoreSize {
		t.Fatalf("applied = %+v, want ignore-size on the idle 80x24 client only", applied)
	}

	raw, err := osReadFileString(argsPath)
	if err != nil {
		t.Fatalf("read fake tmux args: %v", err)
	}
	calls := splitScheduledTmuxCalls(raw)
	// claude-vw-1 keeps a sizing client and is not guard-owned, so only the
	// newly ignored claude-iris window is marked and widened.
	if len(calls) != 5 {
		t.Fatalf("tmux calls = %#v, want list-clients, refresh-client, policy read, ownership mark, resize-window", calls)
	}
	if calls[0][2] != "list-clients" || calls[0][4] != guardFormat {
		t.Fatalf("first call = %#v, want the client listing with the guard format", calls[0])
	}
	want := []string{"-S", "/tmp/guard-test.sock", "refresh-client", "-f", "ignore-size", "-t", "/dev/pts/14"}
	if strings.Join(calls[1], "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("second call = %#v, want %#v", calls[1], want)
	}
	if calls[2][2] != "show-options" || calls[2][5] != "@1" {
		t.Fatalf("third call = %#v, want a policy read for claude-iris only", calls[2])
	}
	if calls[3][2] != "set-window-option" || calls[3][5] != "@1" {
		t.Fatalf("fourth call = %#v, want an ownership mark for claude-iris only", calls[3])
	}
	if calls[4][2] != "resize-window" || calls[4][4] != "@1" {
		t.Fatalf("fifth call = %#v, want a widen for claude-iris only", calls[4])
	}
}

func TestWindowsBySizeOwnershipSeparatesFullyIgnoredWindows(t *testing.T) {
	clients := []tmuxClient{
		// Only client on this window, about to be flagged: nothing left to size it.
		{TTY: "/dev/pts/14", WindowID: "@1"},
		// Two clients; one keeps its say, so tmux still sizes this window.
		{TTY: "/dev/pts/15", WindowID: "@2"},
		{TTY: "/dev/pts/16", WindowID: "@2"},
	}
	decisions := []sizeGuardDecision{
		{TTY: "/dev/pts/14", IgnoreSize: true},
		{TTY: "/dev/pts/15", IgnoreSize: true},
	}

	sizing, ignored := windowsBySizeOwnership(clients, decisions)

	if len(sizing) != 1 || sizing[0] != "@2" {
		t.Fatalf("windows with sizing clients = %v, want only @2", sizing)
	}
	if len(ignored) != 1 || ignored[0] != "@1" {
		t.Fatalf("fully ignored windows = %v, want only @1", ignored)
	}
}

func applySizeGuardWithFake(t *testing.T, clients, policy string) ([]sizeGuardDecision, [][]string) {
	t.Helper()
	tmpDir := t.TempDir()
	argsPath := tmpDir + "/tmux-argv.txt"
	fakeTmux := tmpDir + "/tmux"
	script := `#!/bin/sh
{
  printf 'CALL\n'
  for arg in "$@"; do
    printf '%s\n' "$arg"
  done
} >> "$TMUX_ARGS_FILE"
case "$3" in
  list-clients)
    printf '%s\n' "$TMUX_CLIENTS"
    ;;
  show-options)
    printf '%s\n' "$TMUX_WINDOW_SIZE"
    ;;
esac
exit 0
`
	if err := osWriteFileExecutable(fakeTmux, script); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("TMUX_CLIENTS", clients)
	t.Setenv("TMUX_WINDOW_SIZE", policy)
	t.Setenv("PATH", tmpDir+":/usr/bin:/bin")

	applied, err := NewTmuxHandler().applySizeGuard(context.Background(), "/tmp/guard-test.sock", time.Unix(1785200000, 0), 5*time.Minute)
	if err != nil {
		t.Fatalf("applySizeGuard returned error: %v", err)
	}
	raw, err := osReadFileString(argsPath)
	if err != nil {
		t.Fatalf("read fake tmux args: %v", err)
	}
	return applied, splitScheduledTmuxCalls(raw)
}

func TestApplySizeGuardOwnsOnlyItsManualFallback(t *testing.T) {
	list := []string{"-S", "/tmp/guard-test.sock", "list-clients", "-F", guardFormat}
	ignore := []string{"-S", "/tmp/guard-test.sock", "refresh-client", "-f", "ignore-size", "-t", "/dev/pts/14"}
	reinstate := []string{"-S", "/tmp/guard-test.sock", "refresh-client", "-f", "!ignore-size", "-t", "/dev/pts/14"}
	policy := []string{"-S", "/tmp/guard-test.sock", "show-options", "-wAv", "-t", "@42", "window-size"}
	mark := []string{"-S", "/tmp/guard-test.sock", "set-window-option", "-q", "-t", "@42", sizeGuardOwnerOption, "1"}
	resize := []string{"-S", "/tmp/guard-test.sock", "resize-window", "-t", "@42", "-x", "200", "-y", "50"}
	latest := []string{"-S", "/tmp/guard-test.sock", "set-window-option", "-q", "-t", "@42", "window-size", "latest"}
	clear := []string{"-S", "/tmp/guard-test.sock", "set-window-option", "-qu", "-t", "@42", sizeGuardOwnerOption}

	tests := []struct {
		name        string
		clients     string
		policy      string
		wantApplied int
		wantCalls   [][]string
	}{
		{
			name:        "automatic window gets an owned unobserved fallback",
			clients:     "/dev/pts/14\tagent-watched\t@42\t80\t24\t1\tattached,focused,UTF-8",
			policy:      "latest",
			wantApplied: 1,
			wantCalls:   [][]string{list, ignore, policy, mark, resize},
		},
		{
			name:        "intentional manual window is not claimed or resized",
			clients:     "/dev/pts/14\tagent-watched\t@42\t80\t24\t1\tattached,focused,UTF-8",
			policy:      "manual",
			wantApplied: 1,
			wantCalls:   [][]string{list, ignore, policy},
		},
		{
			name:        "same client reports its viewport",
			clients:     "/dev/pts/14\tagent-watched\t@42\t152\t68\t1\tattached,focused,ignore-size,UTF-8\t1",
			wantApplied: 1,
			wantCalls:   [][]string{list, reinstate, latest, clear},
		},
		{
			name:        "replacement client arrives already visible",
			clients:     "/dev/pts/19\tagent-watched\t@42\t132\t55\t1\tattached,focused,UTF-8\t1",
			wantApplied: 0,
			wantCalls:   [][]string{list, latest, clear},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applied, calls := applySizeGuardWithFake(t, tt.clients, tt.policy)
			if len(applied) != tt.wantApplied {
				t.Fatalf("applied = %+v, want %d client flag changes", applied, tt.wantApplied)
			}
			if len(calls) != len(tt.wantCalls) {
				t.Fatalf("tmux calls = %#v, want %#v", calls, tt.wantCalls)
			}
			for i := range tt.wantCalls {
				if strings.Join(calls[i], "\x00") != strings.Join(tt.wantCalls[i], "\x00") {
					t.Fatalf("call %d = %#v, want %#v", i+1, calls[i], tt.wantCalls[i])
				}
			}
		})
	}
}

func TestApplySizeGuardGroupsLinkedSessionsByStableWindowID(t *testing.T) {
	// Both clients expose the same linked window. One session's client is
	// becoming ignored, while the other session still has a visible sizing
	// client, so the physical window must never enter the ignored fallback.
	clients := strings.Join([]string{
		"/dev/pts/14\thidden-link\t@42\t80\t24\t1\tattached,focused,UTF-8\t1",
		"/dev/pts/15\tvisible-link\t@42\t132\t55\t1\tattached,focused,UTF-8\t1",
	}, "\n")
	applied, calls := applySizeGuardWithFake(t, clients, "latest")
	if len(applied) != 1 || applied[0].TTY != "/dev/pts/14" || !applied[0].IgnoreSize {
		t.Fatalf("applied = %+v, want only the hidden linked client ignored", applied)
	}
	wantCalls := [][]string{
		{"-S", "/tmp/guard-test.sock", "list-clients", "-F", guardFormat},
		{"-S", "/tmp/guard-test.sock", "refresh-client", "-f", "ignore-size", "-t", "/dev/pts/14"},
		{"-S", "/tmp/guard-test.sock", "set-window-option", "-q", "-t", "@42", "window-size", "latest"},
		{"-S", "/tmp/guard-test.sock", "set-window-option", "-qu", "-t", "@42", sizeGuardOwnerOption},
	}
	if len(calls) != len(wantCalls) {
		t.Fatalf("tmux calls = %#v, want one linked-window restore and no ignored fallback: %#v", calls, wantCalls)
	}
	for i := range wantCalls {
		if strings.Join(calls[i], "\x00") != strings.Join(wantCalls[i], "\x00") {
			t.Fatalf("call %d = %#v, want %#v", i+1, calls[i], wantCalls[i])
		}
	}
}

func TestSizeGuardConfigurationPreservesSocketOrderAndTuning(t *testing.T) {
	t.Setenv("CHROTE_TMUX_SOCKET", "build=/tmp/tmux-build,alice=/tmp/tmux-alice,mirror=/tmp/tmux-build")
	wantSockets := []string{"/tmp/tmux-build", "/tmp/tmux-alice"}
	if got := NewTmuxHandler().guardedSockets(); strings.Join(got, "\x00") != strings.Join(wantSockets, "\x00") {
		t.Fatalf("guarded sockets = %q, want configured order %q", got, wantSockets)
	}

	t.Setenv("CHROTE_TERMINAL_SIZE_GUARD", "off")
	if sizeGuardEnabled() {
		t.Fatal("guard stayed enabled with CHROTE_TERMINAL_SIZE_GUARD=off")
	}
	t.Setenv("CHROTE_TERMINAL_SIZE_GUARD", "")
	if !sizeGuardEnabled() {
		t.Fatal("guard is off by default; it should protect agent panes unless disabled")
	}

	t.Setenv("CHROTE_TERMINAL_SIZE_GUARD_IDLE", "90s")
	if got := sizeGuardIdleAfter(); got != 90*time.Second {
		t.Fatalf("idle threshold = %v, want 90s", got)
	}
	t.Setenv("CHROTE_TERMINAL_SIZE_GUARD_IDLE", "nonsense")
	if got := sizeGuardIdleAfter(); got != defaultSizeGuardIdle {
		t.Fatalf("idle threshold = %v, want the default after invalid input", got)
	}
}

func TestStartTerminalSizeGuardStopsWhenDisabled(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_SIZE_GUARD", "0")
	done := NewTmuxHandler().StartTerminalSizeGuard(context.Background(), nil)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disabled guard did not stop immediately")
	}
}

func TestStartTerminalSizeGuardStopsOnContextCancel(t *testing.T) {
	tmpDir := t.TempDir()
	fakeTmux := tmpDir + "/tmux"
	if err := osWriteFileExecutable(fakeTmux, "#!/bin/sh\nexit 0\n"); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	t.Setenv("CHROTE_TERMINAL_SIZE_GUARD", "")
	t.Setenv("CHROTE_TERMINAL_SIZE_GUARD_INTERVAL", "10ms")
	if _, err := os.Stat(fakeTmux); err != nil {
		t.Fatalf("fake tmux missing: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := NewTmuxHandler().StartTerminalSizeGuard(ctx, nil)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("guard did not stop after context cancellation")
	}
}
