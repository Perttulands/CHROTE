package formations

// Dedicated-socket live execution (workstream E3).
//
// This drives the REAL TmuxFormationExecutor against a REAL tmux server on a
// dedicated non-cockpit socket with Dedicated=true — the sanctioned always-on
// execution boundary. It proves the dedicated formations socket can run a
// formation end to end over a real agent pane. It reuses the deterministic
// in-pane agent from the broader dogfood tests (a real process, not a mock of
// the code under test).
//
// SAFETY. This NEVER targets the cockpit/default socket: it creates its OWN
// test-owned server on a freshly-made dedicated socket directory and tears that
// server down with kill-server (which can only reach the sessions this harness
// created on its own socket). validateConfiguredBoundary independently refuses
// the cockpit socket in every mode, so a misconfigured run blocks before any
// keystroke. A guard here also asserts the chosen socket is not the cockpit one.
//
// GATING. It spawns tmux on a dedicated path, so it is inert unless the operator
// opts in with CHROTE_FORMATIONS_DEDICATED_RUN=1. A plain `go test ./...` skips
// it. Run it explicitly, or via scripts/formations-dogfood.sh --dedicated-run.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	dedicatedRunOptInEnv   = "CHROTE_FORMATIONS_DEDICATED_RUN"
	dedicatedSessionPrefix = "mission-"
)

// requireDedicatedRun enforces the honest opt-in/environment gate and returns
// the tmux binary path. It skips with a precise message naming exactly what is
// required (an environment gate, never a hidden failure) whenever a prerequisite
// is missing.
func requireDedicatedRun(t *testing.T) string {
	t.Helper()
	if strings.TrimSpace(os.Getenv(dedicatedRunOptInEnv)) == "" {
		t.Skipf("dedicated-socket run disabled; set %s=1 to run it", dedicatedRunOptInEnv)
	}
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		t.Skipf("%s=1 is set but the 'tmux' binary is not on PATH; install tmux (>=3.0) to run it: %v", dedicatedRunOptInEnv, err)
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("%s=1 is set but 'python3' (the in-pane agent's JSON reader) is not on PATH: %v", dedicatedRunOptInEnv, err)
	}
	return tmuxPath
}

// newDedicatedHarness builds a dogfoodHarness on a NON-temp socket+workspace
// under the repo tree (outside the system temp dir), so it exercises the
// Dedicated boundary branch. It refuses to proceed if the chosen socket is the
// cockpit socket, and tears its own server down on exit.
func newDedicatedHarness(t *testing.T, tmuxPath string) *dogfoodHarness {
	t.Helper()
	// A non-temp base under the repo: t.TempDir() lives under the system temp
	// dir and would NOT exercise the dedicated (non-temp) branch.
	base := filepath.Join(repoRootDir(t), "src", "internal", "formations")
	root, err := os.MkdirTemp(base, "dedicated-run-")
	if err != nil {
		t.Fatalf("make non-temp dedicated root under %s: %v", base, err)
	}
	if pathWithinRoot(root, os.TempDir()) {
		t.Skipf("repo path %q is under the system temp dir; cannot exercise the dedicated non-temp boundary", root)
	}
	socketDir := filepath.Join(root, "formations-tmux")
	if err := os.MkdirAll(socketDir, 0o755); err != nil {
		t.Fatalf("make dedicated socket dir: %v", err)
	}
	socket := filepath.Join(socketDir, "default")
	if isDefaultTmuxSocket(socket) {
		t.Fatalf("refusing to run dedicated test on the cockpit socket %q", socket)
	}
	workdir := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("make dedicated workspace: %v", err)
	}

	h := &dogfoodHarness{
		t:        t,
		tmuxPath: tmuxPath,
		socket:   socket,
		workdir:  workdir,
		store:    NewStore(workdir),
		personas: NewPersonaStore(filepath.Join(root, "agents")),
		scripted: map[string]dogfoodAgent{},
	}
	h.store.Now = fixedClock()
	h.personas.Now = fixedClock()

	// Tear the server down no matter how the test exits. kill-server on
	// this dedicated socket can only reach the sessions this harness created.
	t.Cleanup(func() {
		_ = h.tmux(context.Background(), "kill-server")
		_ = os.RemoveAll(root)
	})

	// Start the server with a placeholder holder session so list-sessions works
	// before any agent session exists. It is killed by kill-server in cleanup.
	if err := h.tmux(context.Background(), "new-session", "-d", "-s", "dedicated-holder", "-x", "240", "-y", "60", "bash --noprofile --norc"); err != nil {
		t.Fatalf("start dedicated tmux server: %v", err)
	}
	return h
}

// dedicatedConfig builds the executor config for the dedicated (non-temp) socket
// path with Dedicated enabled.
func dedicatedConfig(socket, workdir string, timeoutSeconds int) TmuxExecutorConfig {
	return TmuxExecutorConfig{
		Harnesses:      []string{"openai-codex"},
		Socket:         socket,
		Cwd:            workdir,
		Roots:          []string{workdir},
		SessionPrefix:  dedicatedSessionPrefix,
		OutputCapBytes: 1 << 20,
		TimeoutSeconds: timeoutSeconds,
		Dedicated:      true,
	}
}

// TestDedicatedSocketRunsFormationOverRealAgent runs a single formation end to
// end against a real agent pane on a dedicated non-cockpit socket with Dedicated
// enabled. It asserts the prompt reached the pane and the fenced chrote-outputs
// payload was captured and routed, proving the dedicated boundary actually
// executes.
func TestDedicatedSocketRunsFormationOverRealAgent(t *testing.T) {
	tmuxPath := requireDedicatedRun(t)
	h := newDedicatedHarness(t, tmuxPath)
	createS4Persona(t, h.personas, "scout")
	writeFixture(t, h.store.BoardPath("session-search"), s4RunBoardFixture())
	h.stageResponse("fmn_research", dogfoodAgent{
		body:         "dedicated-socket answer\n" + fenced(`{"port_research_out":{"text":"RAN-ON-DEDICATED-SOCKET"}}`),
		emitSentinel: true,
	})
	h.startSession(dedicatedSessionPrefix + "scout")

	executor := NewTmuxFormationExecutor(h.store, h.personas, dedicatedConfig(h.socket, h.workdir, 20))
	engine := NewRunEngine(h.store, h.personas, executor)
	status, err := engine.RunFormation("session-search", "fmn_research", FormationRunRequest{
		Actor:  "agent:dedicated",
		Limits: RunLimits{MaxDispatch: 3, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("dedicated-socket run: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final over the dedicated socket", status)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, h.store, "session-search"))
	if !eventsContainType(events, RunEventAdapterSend) {
		t.Fatalf("events = %v, want adapter_send proving the prompt reached the dedicated pane", eventTypes(events))
	}
	out := findNodeOutputEvent(t, events, "fmn_research")
	outputs, ok := out.Data["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("research outputs = %#v, want routing map from dedicated-socket capture", out.Data["outputs"])
	}
	assertOutputPayloadText(t, outputs, "port_research_out", "RAN-ON-DEDICATED-SOCKET")
}
