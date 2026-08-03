package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The properties tested here are the ones a broken systemd control path would
// violate silently: a session name that becomes a second unit or a flag, a
// config file written outside the state directory, a claim of restart
// capability by a unit CHROTE did not install, and health reported green for an
// agent that resumed the wrong transcript (ADR-0014 decisions 3, 4, 5).

func newTestAgentUnitController(t *testing.T) (*agentUnitController, *fakeSystemctl, string) {
	t.Helper()
	dir := t.TempDir()
	fake := &fakeSystemctl{states: map[string]systemdUnitState{}}
	controller := newAgentUnitController(dir, fake.run)
	return controller, fake, dir
}

type systemctlCall struct {
	UnixUser string
	Args     []string
}

type fakeSystemctl struct {
	calls  []systemctlCall
	states map[string]systemdUnitState
	err    error
}

func (f *fakeSystemctl) run(_ context.Context, unixUser string, args ...string) (string, error) {
	f.calls = append(f.calls, systemctlCall{UnixUser: unixUser, Args: append([]string(nil), args...)})
	if f.err != nil {
		return "", f.err
	}
	if len(args) > 0 && args[0] == "show" {
		unit := args[len(args)-1]
		state, ok := f.states[unit]
		if !ok {
			return "ActiveState=inactive\nSubState=dead\nUnitFileState=disabled\nExecMainStartTimestampMonotonic=0\n", nil
		}
		return "ActiveState=" + state.ActiveState +
			"\nSubState=" + state.SubState +
			"\nUnitFileState=" + state.UnitFileState +
			"\nExecMainStartTimestampMonotonic=1\n", nil
	}
	return "", nil
}

func (f *fakeSystemctl) argvFor(verb string) []string {
	for _, call := range f.calls {
		if len(call.Args) > 0 && call.Args[0] == verb {
			return call.Args
		}
	}
	return nil
}

func TestAgentUnitName_RejectsEveryNameThatCouldInjectAUnitOrFlag(t *testing.T) {
	// core.ValidateSessionName already guards the API surface; this pins that
	// unit-name construction does not trust it, because a single accepted
	// metacharacter here becomes a second systemctl argument.
	hostile := []string{
		"good name",
		"good;rm -rf /",
		"--user",
		"-f",
		"a@b",
		"a.service",
		"../escape",
		"a\nb",
		"a$(id)",
		"",
		strings.Repeat("a", 51),
	}
	for _, name := range hostile {
		if _, err := agentUnitName(name); err == nil {
			t.Fatalf("agentUnitName(%q) must fail; a hostile name may never reach systemctl", name)
		}
	}
	unit, err := agentUnitName("codex-alpha_1")
	if err != nil {
		t.Fatalf("a valid session name must produce a unit: %v", err)
	}
	if unit != "chrote-agent@codex-alpha_1.service" {
		t.Fatalf("unexpected unit name %q", unit)
	}
}

func TestAgentUnitController_EnableWritesTypedConfigAndStartsTheUnit(t *testing.T) {
	controller, fake, dir := newTestAgentUnitController(t)
	config := agentUnitConfig{
		Session:        "codex-alpha",
		UnixUser:       "alice",
		AgentKind:      "codex",
		AgentSessionID: "019f4baa-e368-7ea0-8912-fb2c6f99785c",
		AgentBin:       "/opt/bin/codex",
		TmuxBin:        "/opt/bin/tmux",
		TmuxSocket:     "/run/user/1234/pool/default",
		Workdir:        "/opt/work",
		KeeperUnit:     "pool-keeper.service",
	}
	if err := controller.Enable(context.Background(), config); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	path := filepath.Join(dir, "alice", "codex-alpha.conf")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("per-agent config was not written: %v", err)
	}
	body := string(raw)
	for _, want := range []string{
		"CHROTE_AGENT_SESSION=codex-alpha",
		"CHROTE_AGENT_KIND=codex",
		"CHROTE_AGENT_SESSION_ID=019f4baa-e368-7ea0-8912-fb2c6f99785c",
		"CHROTE_AGENT_BIN=/opt/bin/codex",
		"CHROTE_AGENT_TMUX_SOCKET=/run/user/1234/pool/default",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("config missing %q; got:\n%s", want, body)
		}
	}
	// A resume COMMAND must never be persisted: the launcher renders argv from
	// typed fields so a config cannot smuggle a command into a pane (ADR-0001).
	if strings.Contains(body, "resume ") || strings.Contains(body, "COMMAND") {
		t.Fatalf("config must carry typed fields, never a rendered command:\n%s", body)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("config mode = %o, want 600", mode)
	}

	argv := fake.argvFor("enable")
	if argv == nil {
		t.Fatalf("Enable must enable the unit; calls: %+v", fake.calls)
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "--now") || !strings.Contains(joined, "chrote-agent@codex-alpha.service") {
		t.Fatalf("unexpected enable argv %q", joined)
	}
	if fake.calls[len(fake.calls)-1].UnixUser != "alice" {
		t.Fatalf("systemctl must target the session's own unix user")
	}
}

func TestAgentUnitController_EnableRefusesConfigPathEscape(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	config := agentUnitConfig{
		Session:        "../../etc/evil",
		UnixUser:       "alice",
		AgentKind:      "codex",
		AgentSessionID: "019f4baa-e368-7ea0-8912-fb2c6f99785c",
		AgentBin:       "/opt/bin/codex",
		TmuxBin:        "/opt/bin/tmux",
		TmuxSocket:     "/run/user/1234/pool/default",
		Workdir:        "/opt/work",
	}
	if err := controller.Enable(context.Background(), config); err == nil {
		t.Fatal("a session name that escapes the state directory must be refused")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("nothing may reach systemctl after a rejected config: %+v", fake.calls)
	}
}

func TestAgentUnitController_EnableRefusesHostileUnixUser(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	for _, user := range []string{"alice; rm -rf /", "--user", "", "a/b", strings.Repeat("u", 33)} {
		config := agentUnitConfig{
			Session:        "codex-alpha",
			UnixUser:       user,
			AgentKind:      "codex",
			AgentSessionID: "019f4baa-e368-7ea0-8912-fb2c6f99785c",
			AgentBin:       "/opt/bin/codex",
			TmuxBin:        "/opt/bin/tmux",
			TmuxSocket:     "/run/user/1234/pool/default",
			Workdir:        "/opt/work",
		}
		if err := controller.Enable(context.Background(), config); err == nil {
			t.Fatalf("hostile unix user %q must be refused", user)
		}
	}
	if len(fake.calls) != 0 {
		t.Fatalf("nothing may reach systemctl after a rejected user: %+v", fake.calls)
	}
}

func TestAgentUnitController_EnableRejectsUnsupportedKindAndSessionID(t *testing.T) {
	controller, _, _ := newTestAgentUnitController(t)
	base := agentUnitConfig{
		Session:    "codex-alpha",
		UnixUser:   "alice",
		AgentBin:   "/opt/bin/codex",
		TmuxBin:    "/opt/bin/tmux",
		TmuxSocket: "/run/user/1234/pool/default",
		Workdir:    "/opt/work",
	}
	bad := base
	bad.AgentKind = "shell"
	bad.AgentSessionID = "019f4baa-e368-7ea0-8912-fb2c6f99785c"
	if err := controller.Enable(context.Background(), bad); err == nil {
		t.Fatal("unsupported agent kind must be refused")
	}
	bad = base
	bad.AgentKind = "codex"
	bad.AgentSessionID = "--last"
	if err := controller.Enable(context.Background(), bad); err == nil {
		t.Fatal("a non-canonical session id must be refused; --last is how a flag becomes an argument")
	}
	bad = base
	bad.AgentKind = "codex"
	bad.AgentSessionID = "019f4baa-e368-7ea0-8912-fb2c6f99785c"
	bad.TmuxSocket = "relative/socket"
	if err := controller.Enable(context.Background(), bad); err == nil {
		t.Fatal("a relative socket path must be refused")
	}
}

func TestAgentUnitController_DisableStopsTheUnitAndRemovesConfigButNotTheSession(t *testing.T) {
	controller, fake, dir := newTestAgentUnitController(t)
	config := agentUnitConfig{
		Session:        "codex-alpha",
		UnixUser:       "alice",
		AgentKind:      "codex",
		AgentSessionID: "019f4baa-e368-7ea0-8912-fb2c6f99785c",
		AgentBin:       "/opt/bin/codex",
		TmuxBin:        "/opt/bin/tmux",
		TmuxSocket:     "/run/user/1234/pool/default",
		Workdir:        "/opt/work",
	}
	if err := controller.Enable(context.Background(), config); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	fake.calls = nil
	if err := controller.Disable(context.Background(), "codex-alpha", "alice"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	argv := fake.argvFor("disable")
	if argv == nil || !strings.Contains(strings.Join(argv, " "), "--now") {
		t.Fatalf("Disable must disable --now; calls: %+v", fake.calls)
	}
	for _, call := range fake.calls {
		joined := strings.Join(call.Args, " ")
		if strings.Contains(joined, "kill") {
			t.Fatalf("unlock must not kill anything: %q", joined)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "alice", "codex-alpha.conf")); !os.IsNotExist(err) {
		t.Fatalf("per-agent config must be removed on unlock, stat err = %v", err)
	}
}

func TestAgentUnitController_DisableIsIdempotent(t *testing.T) {
	controller, _, _ := newTestAgentUnitController(t)
	if err := controller.Disable(context.Background(), "never-locked", "alice"); err != nil {
		t.Fatalf("disabling an unlocked session must be a no-op, got %v", err)
	}
}

func TestAgentUnitController_StatusRequiresBothUnitStateAndMatchingReceipt(t *testing.T) {
	controller, fake, dir := newTestAgentUnitController(t)
	sessionID := "019f4baa-e368-7ea0-8912-fb2c6f99785c"
	config := agentUnitConfig{
		Session:        "codex-alpha",
		UnixUser:       "alice",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		AgentBin:       "/opt/bin/codex",
		TmuxBin:        "/opt/bin/tmux",
		TmuxSocket:     "/run/user/1234/pool/default",
		Workdir:        "/opt/work",
	}
	if err := controller.Enable(context.Background(), config); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	unit := "chrote-agent@codex-alpha.service"
	fake.states[unit] = systemdUnitState{ActiveState: "active", SubState: "running", UnitFileState: "enabled"}

	// Active unit, no receipt: an active unit only proves a process started.
	status, err := controller.Status(context.Background(), "codex-alpha", "alice")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Health != agentHealthDegraded {
		t.Fatalf("active unit without a receipt must be degraded, got %q", status.Health)
	}

	// Receipt for a DIFFERENT transcript: the failure this whole mechanism exists
	// to catch -- a unit that cheerfully resumed the wrong session.
	receiptPath := filepath.Join(dir, "alice", "codex-alpha.receipt.json")
	writeTestReceipt(t, receiptPath, "codex-alpha", "codex", "019f0000-0000-7000-8000-000000000000")
	status, err = controller.Status(context.Background(), "codex-alpha", "alice")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Health != agentHealthDegraded {
		t.Fatalf("a receipt naming another transcript must be degraded, got %q", status.Health)
	}

	// Matching receipt: healthy.
	writeTestReceipt(t, receiptPath, "codex-alpha", "codex", sessionID)
	status, err = controller.Status(context.Background(), "codex-alpha", "alice")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Health != agentHealthHealthy {
		t.Fatalf("active unit with a matching receipt must be healthy, got %q", status.Health)
	}
	if !status.Locked {
		t.Fatal("an enabled unit means the session is locked")
	}
}

func TestAgentUnitController_StatusReportsFailedAndInactiveVerbatim(t *testing.T) {
	controller, fake, dir := newTestAgentUnitController(t)
	sessionID := "019f4baa-e368-7ea0-8912-fb2c6f99785c"
	config := agentUnitConfig{
		Session: "codex-alpha", UnixUser: "alice", AgentKind: "codex",
		AgentSessionID: sessionID, AgentBin: "/opt/bin/codex", TmuxBin: "/opt/bin/tmux",
		TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
	}
	if err := controller.Enable(context.Background(), config); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	writeTestReceipt(t, filepath.Join(dir, "alice", "codex-alpha.receipt.json"), "codex-alpha", "codex", sessionID)
	unit := "chrote-agent@codex-alpha.service"

	fake.states[unit] = systemdUnitState{ActiveState: "failed", SubState: "failed", UnitFileState: "enabled"}
	status, err := controller.Status(context.Background(), "codex-alpha", "alice")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Health != agentHealthFailed {
		t.Fatalf("failed unit must report failed, got %q", status.Health)
	}
	if status.ActiveState != "failed" {
		t.Fatalf("systemd state must be reported verbatim, got %q", status.ActiveState)
	}

	fake.states[unit] = systemdUnitState{ActiveState: "inactive", SubState: "dead", UnitFileState: "enabled"}
	status, err = controller.Status(context.Background(), "codex-alpha", "alice")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Health != agentHealthInactive {
		t.Fatalf("inactive unit must report inactive, got %q", status.Health)
	}
}

func TestAgentUnitController_StatusOfAnUnlockedSessionIsNotLocked(t *testing.T) {
	controller, _, _ := newTestAgentUnitController(t)
	status, err := controller.Status(context.Background(), "never-locked", "alice")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Locked {
		t.Fatal("a session with no CHROTE config is not locked, whatever systemd says")
	}
	if status.Health != agentHealthUnlocked {
		t.Fatalf("unlocked session health = %q", status.Health)
	}
}

// ADR-0014 decision 3: restart capability is granted only to units CHROTE
// installed, proven by the unit name matching the template AND a config file
// existing. A hand-written unit claiming our name must not inherit our rights.
func TestAgentUnitController_OnlyChroteInstalledUnitsMayClaimRestartCapability(t *testing.T) {
	controller, _, dir := newTestAgentUnitController(t)
	if controller.OwnsUnit("chrote-agent@codex-alpha.service", "codex-alpha", "alice") {
		t.Fatal("a unit with no CHROTE config must not be treated as CHROTE-installed")
	}
	config := agentUnitConfig{
		Session: "codex-alpha", UnixUser: "alice", AgentKind: "codex",
		AgentSessionID: "019f4baa-e368-7ea0-8912-fb2c6f99785c", AgentBin: "/opt/bin/codex",
		TmuxBin: "/opt/bin/tmux", TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
	}
	if err := controller.Enable(context.Background(), config); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !controller.OwnsUnit("chrote-agent@codex-alpha.service", "codex-alpha", "alice") {
		t.Fatal("a unit we installed, with our config present, is ours")
	}
	for _, foreign := range []string{
		"some-other@codex-alpha.service",
		"chrote-agent@codex-alpha.timer",
		"chrote-agentXcodex-alpha.service",
		"codex-minerva-telegram.service",
	} {
		if controller.OwnsUnit(foreign, "codex-alpha", "alice") {
			t.Fatalf("unit %q is not CHROTE-installed and must stay read-only", foreign)
		}
	}
	_ = dir
}

func TestAgentUnitController_SystemctlFailureSurfacesInsteadOfSilentlySucceeding(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	fake.err = errors.New("Failed to connect to user scope bus: No such file or directory")
	config := agentUnitConfig{
		Session: "codex-alpha", UnixUser: "alice", AgentKind: "codex",
		AgentSessionID: "019f4baa-e368-7ea0-8912-fb2c6f99785c", AgentBin: "/opt/bin/codex",
		TmuxBin: "/opt/bin/tmux", TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
	}
	err := controller.Enable(context.Background(), config)
	if err == nil {
		t.Fatal("a systemctl failure must fail the lock, not report success")
	}
	if !strings.Contains(err.Error(), "user scope bus") {
		t.Fatalf("the operator needs the real reason, got %v", err)
	}
}

func writeTestReceipt(t *testing.T, path, session, kind, sessionID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{"session":"` + session + `","agentKind":"` + kind + `","agentSessionId":"` + sessionID + `","panePid":4242,"startedAt":"2026-08-03T12:00:00Z"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
}

// installFakeSystemctl points the package-level seam at a recorder for the
// duration of one test, so a handler built by the production constructor drives
// a fake rather than a real user manager.
func installFakeSystemctl(t *testing.T) *fakeSystemctl {
	t.Helper()
	fake := &fakeSystemctl{states: map[string]systemdUnitState{}}
	previous := agentSystemctlRun
	agentSystemctlRun = fake.run
	t.Cleanup(func() { agentSystemctlRun = previous })
	return fake
}
