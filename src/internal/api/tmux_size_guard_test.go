package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

const guardFormat = "#{client_tty}\t#{client_session}\t#{client_width}\t#{client_height}\t#{client_activity}\t#{client_flags}"

func TestParseTmuxClientsReadsSizeActivityAndFlags(t *testing.T) {
	output := strings.Join([]string{
		"/dev/pts/14\tclaude-iris\t80\t24\t1785114088\tattached,focused,UTF-8",
		"/dev/pts/17\tclaude-vw-1\t152\t68\t1785114088\tattached,focused,ignore-size,UTF-8",
		"garbage line",
		"/dev/pts/23\tcodex-anim\tnot-a-number\t24\t1785114088\tattached",
	}, "\n")

	clients := parseTmuxClients(output)

	if len(clients) != 2 {
		t.Fatalf("parsed %d clients, want the two well-formed ones: %+v", len(clients), clients)
	}
	if clients[0].TTY != "/dev/pts/14" || clients[0].Width != 80 || clients[0].Height != 24 || clients[0].IgnoreSize {
		t.Fatalf("first client = %+v, want the un-negotiated 80x24 client without the flag", clients[0])
	}
	if !clients[1].IgnoreSize || clients[1].Width != 152 {
		t.Fatalf("second client = %+v, want the flagged 152-wide client", clients[1])
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
      printf '/dev/pts/14\tclaude-iris\t80\t24\t1\tattached,focused,UTF-8\n'
      printf '/dev/pts/17\tclaude-vw-1\t152\t68\t1\tattached,focused,UTF-8\n'
      exit 0
      ;;
  esac
done
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
	// list-clients, one refresh-client, and a widen for the session whose only
	// client just lost its say. claude-vw-1 keeps a sizing client, so it is left alone.
	if len(calls) != 3 {
		t.Fatalf("tmux calls = %#v, want list-clients, refresh-client, resize-window", calls)
	}
	if calls[0][2] != "list-clients" || calls[0][4] != guardFormat {
		t.Fatalf("first call = %#v, want the client listing with the guard format", calls[0])
	}
	want := []string{"-S", "/tmp/guard-test.sock", "refresh-client", "-f", "ignore-size", "-t", "/dev/pts/14"}
	if strings.Join(calls[1], "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("second call = %#v, want %#v", calls[1], want)
	}
	if calls[2][2] != "resize-window" || calls[2][4] != "claude-iris" {
		t.Fatalf("third call = %#v, want a widen for claude-iris only", calls[2])
	}
}

func TestSessionsNeedingResizeOnlyCoversFullyIgnoredSessions(t *testing.T) {
	clients := []tmuxClient{
		// Only client on this session, about to be flagged: nothing left to size it.
		{TTY: "/dev/pts/14", Session: "agent-alone"},
		// Two clients; one keeps its say, so tmux still sizes this session.
		{TTY: "/dev/pts/15", Session: "agent-watched"},
		{TTY: "/dev/pts/16", Session: "agent-watched"},
	}
	decisions := []sizeGuardDecision{
		{TTY: "/dev/pts/14", IgnoreSize: true},
		{TTY: "/dev/pts/15", IgnoreSize: true},
	}

	needing := sessionsNeedingResize(clients, decisions)

	if len(needing) != 1 || needing[0] != "agent-alone" {
		t.Fatalf("sessions needing resize = %v, want only agent-alone", needing)
	}
}

func TestApplySizeGuardWidensASessionLeftWithoutASizingClient(t *testing.T) {
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
      printf '/dev/pts/14\tagent-alone\t80\t24\t1\tattached,focused,UTF-8\n'
      exit 0
      ;;
  esac
done
exit 0
`
	if err := osWriteFileExecutable(fakeTmux, script); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	t.Setenv("CHROTE_TERMINAL_UNOBSERVED_COLS", "200")
	t.Setenv("CHROTE_TERMINAL_UNOBSERVED_ROWS", "50")

	handler := NewTmuxHandler()
	if _, err := handler.applySizeGuard(context.Background(), "/tmp/guard-test.sock", time.Unix(1785200000, 0), 5*time.Minute); err != nil {
		t.Fatalf("applySizeGuard returned error: %v", err)
	}

	raw, err := osReadFileString(argsPath)
	if err != nil {
		t.Fatalf("read fake tmux args: %v", err)
	}
	calls := splitScheduledTmuxCalls(raw)
	if len(calls) != 3 {
		t.Fatalf("tmux calls = %#v, want list-clients, refresh-client, resize-window", calls)
	}
	want := []string{"-S", "/tmp/guard-test.sock", "resize-window", "-t", "agent-alone", "-x", "200", "-y", "50"}
	if strings.Join(calls[2], "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("third call = %#v, want %#v", calls[2], want)
	}
}

func TestSizeGuardCanBeDisabledAndTuned(t *testing.T) {
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
