package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	osuser "os/user"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chrote/server/internal/core"
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
	mu           sync.Mutex
	calls        []systemctlCall
	states       map[string]systemdUnitState
	loadStates   map[string]string
	errorsByUser map[string]error
	errorsByVerb map[string]error
	err          error
}

func (f *fakeSystemctl) run(_ context.Context, unixUser string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, systemctlCall{UnixUser: unixUser, Args: append([]string(nil), args...)})
	if err := f.errorsByUser[unixUser]; err != nil {
		return "", err
	}
	if len(args) > 0 {
		if err := f.errorsByVerb[args[0]]; err != nil {
			return "", err
		}
	}
	if f.err != nil {
		return "", f.err
	}
	if len(args) > 0 && args[0] == "show" {
		if slices.Contains(args, "--property=LoadState") {
			state := f.loadStates[unixUser]
			if state == "" {
				state = "loaded"
			}
			return "LoadState=" + state + "\n", nil
		}
		unit := args[len(args)-1]
		state, ok := f.states[unit]
		if !ok {
			return "ActiveState=inactive\nSubState=dead\nUnitFileState=disabled\nExecMainStartTimestampMonotonic=0\nInvocationID=\n", nil
		}
		return "ActiveState=" + state.ActiveState +
			"\nSubState=" + state.SubState +
			"\nUnitFileState=" + state.UnitFileState +
			"\nExecMainStartTimestampMonotonic=1\nInvocationID=current-invocation\n", nil
	}
	return "", nil
}

func TestAgentUnitController_PreflightExercisesEachRealUserManagerAndFailsClosed(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	fake.loadStates = map[string]string{"alice": "loaded", "bob": "not-found"}

	errs := controller.Preflight(context.Background(), []string{"alice", "bob"})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "bob") {
		t.Fatalf("preflight errors = %v, want one error naming bob", errs)
	}
	if err := controller.RequireCapability("alice"); err != nil {
		t.Fatalf("loaded target-user template must be available: %v", err)
	}
	if err := controller.RequireCapability("bob"); err == nil || !strings.Contains(err.Error(), "not loaded") {
		t.Fatalf("missing target-user template must be unavailable, got %v", err)
	}
	for _, user := range []string{"alice", "bob"} {
		found := false
		for _, call := range fake.calls {
			if call.UnixUser == user && slices.Contains(call.Args, "--property=LoadState") {
				found = true
			}
		}
		if !found {
			t.Fatalf("preflight never exercised %s's user manager: %+v", user, fake.calls)
		}
	}
}

func TestAgentUnitController_AnnotateCapabilityMakesUnavailableStateExplicit(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	fake.loadStates = map[string]string{"alice": "not-found"}
	controller.Preflight(context.Background(), []string{"alice"})

	sessions := controller.AnnotateCapability([]core.Session{{Name: "codex-alpha", UnixUser: "alice"}})
	if sessions[0].PersistentAvailable == nil || *sessions[0].PersistentAvailable {
		t.Fatalf("missing grant/template must project explicit unavailable state: %+v", sessions[0])
	}
	if !strings.Contains(sessions[0].PersistentCapabilityDetail, "not loaded") {
		t.Fatalf("capability detail must explain the unavailable state: %+v", sessions[0])
	}
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
		"CHROTE_AGENT_RECEIPT_PATH=" + filepath.Join(controller.receiptRoot, "alice", "codex-alpha.receipt.json"),
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
	if mode := info.Mode().Perm(); mode != 0o640 {
		t.Fatalf("config mode = %o, want 640 ACL mask", mode)
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

func TestAgentUnitController_ConfigAndReceiptPathsUseSeparateOwnershipDomains(t *testing.T) {
	controller, _, _ := newTestAgentUnitController(t)
	configPath, err := controller.configPath("codex-alpha", "alice")
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	receiptPath, err := controller.receiptPath("codex-alpha", "alice")
	if err != nil {
		t.Fatalf("receiptPath: %v", err)
	}
	if filepath.Dir(configPath) == filepath.Dir(receiptPath) {
		t.Fatalf("target-writable receipt directory must not contain CHROTE-owned config: %q", configPath)
	}
	if strings.HasPrefix(filepath.Clean(receiptPath), filepath.Clean(controller.root)+string(os.PathSeparator)) {
		t.Fatalf("receipt %q must be outside CHROTE's config ownership root %q", receiptPath, controller.root)
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

func TestAgentUnitController_RejectsUnsafeConfiguredStateRoot(t *testing.T) {
	controller := newAgentUnitController("/tmp/chrote-agent-units;touch", func(context.Context, string, ...string) (string, error) {
		return "", nil
	})
	if _, err := controller.configPath("codex-alpha", "alice"); err == nil {
		t.Fatal("unsafe configured state root must not produce a config path")
	}
}

func TestAgentUnitController_EnableRefusesHostileUnixUser(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	for _, user := range []string{"alice; rm -rf /", "--user", "a/b", "Alice", strings.Repeat("u", 33)} {
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
	bad = base
	bad.AgentKind = "codex"
	bad.AgentSessionID = "019f4baa-e368-7ea0-8912-fb2c6f99785c"
	bad.AgentBin = "/opt/bin/codex;touch"
	if err := controller.Enable(context.Background(), bad); err == nil {
		t.Fatal("an executable path containing shell metacharacters must be refused")
	}
}

func TestReadAgentUnitConfigRejectsDuplicateTypedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.conf")
	raw := strings.Join([]string{
		"CHROTE_AGENT_SESSION=codex-alpha",
		"CHROTE_AGENT_KIND=codex",
		"CHROTE_AGENT_KIND=claude",
	}, "\n")
	if err := os.WriteFile(path, []byte(raw), 0o640); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := readAgentUnitConfigFile(path); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate typed key must be rejected consistently with the launcher, got %v", err)
	}
}

func TestAgentUnitController_AnnotateHealthUsesOneConcurrentRequestBudget(t *testing.T) {
	var mu sync.Mutex
	active := 0
	maxActive := 0
	blockShows := false
	runner := func(ctx context.Context, _ string, args ...string) (string, error) {
		if len(args) == 0 || args[0] != "show" || !blockShows {
			return "", nil
		}
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		<-ctx.Done()
		mu.Lock()
		active--
		mu.Unlock()
		return "", ctx.Err()
	}
	controller := newAgentUnitController(filepath.Join(t.TempDir(), "units"), runner)
	sessions := make([]core.Session, 8)
	for i := range sessions {
		name := fmt.Sprintf("codex-%d", i)
		if err := controller.Enable(context.Background(), agentUnitConfig{
			Session: name, UnixUser: "alice", AgentKind: "codex",
			AgentSessionID: fmt.Sprintf("session-%d", i), AgentBin: "/opt/bin/codex",
			TmuxBin: "/opt/bin/tmux", TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
		}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		sessions[i] = core.Session{Name: name, UnixUser: "alice", Persistent: true}
	}
	blockShows = true
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	annotated := controller.AnnotateHealth(ctx, sessions)
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("health annotation ignored the overall request deadline: %v", elapsed)
	}
	if maxActive < 2 || maxActive > agentHealthConcurrency {
		t.Fatalf("health lookups max concurrency = %d, want 2..%d", maxActive, agentHealthConcurrency)
	}
	for _, session := range annotated {
		if session.PersistentHealth != agentHealthDegraded {
			t.Fatalf("timed-out lock must degrade independently: %+v", session)
		}
	}
}

func TestAgentUnitController_AnnotateHealthCachesBriefly(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	sessions := []core.Session{}
	for _, name := range []string{"codex-alpha", "codex-beta"} {
		if err := controller.Enable(context.Background(), agentUnitConfig{
			Session: name, UnixUser: "alice", AgentKind: "codex",
			AgentSessionID: "session-" + name, AgentBin: "/opt/bin/codex",
			TmuxBin: "/opt/bin/tmux", TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
		}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		unit, _ := agentUnitName(name)
		fake.states[unit] = systemdUnitState{ActiveState: "failed", SubState: "failed", UnitFileState: "enabled"}
		sessions = append(sessions, core.Session{Name: name, UnixUser: "alice", Persistent: true})
	}
	fake.calls = nil
	controller.AnnotateHealth(context.Background(), sessions)
	controller.AnnotateHealth(context.Background(), sessions)
	shows := 0
	for _, call := range fake.calls {
		if len(call.Args) > 0 && call.Args[0] == "show" {
			shows++
		}
	}
	if shows != len(sessions) {
		t.Fatalf("two immediate annotations made %d status reads, want one per lock", shows)
	}
}

func TestAgentUnitController_DifferentUnitsDoNotSerializeControlCalls(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	var wait sync.WaitGroup
	runner := func(_ context.Context, _ string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "enable" {
			started <- args[len(args)-1]
			<-release
		}
		return "", nil
	}
	controller := newAgentUnitController(filepath.Join(t.TempDir(), "units"), runner)
	enable := func(name string) {
		defer wait.Done()
		_ = controller.Enable(context.Background(), agentUnitConfig{
			Session: name, UnixUser: "alice", AgentKind: "codex", AgentSessionID: "session-" + name,
			AgentBin: "/opt/bin/codex", TmuxBin: "/opt/bin/tmux",
			TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
		})
	}
	wait.Add(2)
	go enable("codex-alpha")
	go enable("codex-beta")
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(500 * time.Millisecond):
			close(release)
			wait.Wait()
			t.Fatal("control of one unit serialized an unrelated unit")
		}
	}
	close(release)
	wait.Wait()
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
	controller, fake, _ := newTestAgentUnitController(t)
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
	receiptPath := filepath.Join(controller.receiptRoot, "alice", "codex-alpha.receipt.json")
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

func TestAgentUnitController_StatusRejectsPriorInvocationAndDeadProcessReceipts(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	sessionID := "019f4baa-e368-7ea0-8912-fb2c6f99785c"
	config := agentUnitConfig{
		Session: "codex-alpha", UnixUser: "alice", AgentKind: "codex",
		AgentSessionID: sessionID, AgentBin: "/opt/bin/codex", TmuxBin: "/opt/bin/tmux",
		TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
	}
	if err := controller.Enable(context.Background(), config); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	fake.states["chrote-agent@codex-alpha.service"] = systemdUnitState{
		ActiveState: "active", SubState: "running", UnitFileState: "enabled",
	}
	receiptPath := filepath.Join(controller.receiptRoot, "alice", "codex-alpha.receipt.json")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o770); err != nil {
		t.Fatalf("mkdir receipt dir: %v", err)
	}

	writeObservedReceipt := func(invocation string, pid int, processStart, attestedAt uint64) {
		t.Helper()
		body := fmt.Sprintf(`{"session":"codex-alpha","agentKind":"codex","agentSessionId":%q,"paneId":"%%7","panePid":%d,"agentPid":%d,"processStartTicks":%d,"invocationId":%q,"attestedAtMonotonic":%d,"startedAt":"2026-08-05T12:00:00Z"}`,
			sessionID, pid, pid, processStart, invocation, attestedAt)
		if err := os.WriteFile(receiptPath, []byte(body), 0o640); err != nil {
			t.Fatalf("write receipt: %v", err)
		}
	}

	writeObservedReceipt("prior-invocation", os.Getpid(), 1, 2)
	status, err := controller.Status(context.Background(), "codex-alpha", "alice")
	if err != nil {
		t.Fatalf("Status prior invocation: %v", err)
	}
	if status.Health != agentHealthDegraded {
		t.Fatalf("receipt from prior unit invocation must be degraded, got %q", status.Health)
	}

	currentStart, startErr := processStartTicks(os.Getpid())
	if startErr != nil {
		t.Fatalf("current process start: %v", startErr)
	}
	writeObservedReceipt("current-invocation", os.Getpid(), currentStart, 0)
	status, err = controller.Status(context.Background(), "codex-alpha", "alice")
	if err != nil {
		t.Fatalf("Status stale monotonic receipt: %v", err)
	}
	if status.Health != agentHealthDegraded || !strings.Contains(status.Detail, "predates") {
		t.Fatalf("receipt older than unit start must be degraded, got %#v", status)
	}

	writeObservedReceipt("current-invocation", 1<<22-1, 1, 2)
	status, err = controller.Status(context.Background(), "codex-alpha", "alice")
	if err != nil {
		t.Fatalf("Status dead process: %v", err)
	}
	if status.Health != agentHealthDegraded {
		t.Fatalf("receipt for a dead process must be degraded, got %q", status.Health)
	}
}

func TestAgentUnitController_StatusReportsFailedAndInactiveVerbatim(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	sessionID := "019f4baa-e368-7ea0-8912-fb2c6f99785c"
	config := agentUnitConfig{
		Session: "codex-alpha", UnixUser: "alice", AgentKind: "codex",
		AgentSessionID: sessionID, AgentBin: "/opt/bin/codex", TmuxBin: "/opt/bin/tmux",
		TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
	}
	if err := controller.Enable(context.Background(), config); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	writeTestReceipt(t, filepath.Join(controller.receiptRoot, "alice", "codex-alpha.receipt.json"), "codex-alpha", "codex", sessionID)
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
	pid := os.Getpid()
	start, err := processStartTicks(pid)
	if err != nil {
		t.Fatalf("process start: %v", err)
	}
	body := fmt.Sprintf(`{"session":%q,"agentKind":%q,"agentSessionId":%q,"paneId":"%%7","panePid":%d,"agentPid":%d,"processStartTicks":%d,"invocationId":"current-invocation","attestedAtMonotonic":2,"startedAt":"2026-08-03T12:00:00Z"}`,
		session, kind, sessionID, pid, pid, start)
	if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
}

func TestReadAgentUnitReceiptRejectsSymlinksAndUnsafeMode(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "real.json")
	writeTestReceipt(t, realPath, "codex-alpha", "codex", "019f4baa-e368-7ea0-8912-fb2c6f99785c")

	linkPath := filepath.Join(dir, "link.json")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := readAgentUnitReceipt(linkPath); err == nil {
		t.Fatal("receipt reader must never follow a symlink")
	}

	if err := os.Chmod(realPath, 0o666); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := readAgentUnitReceipt(realPath); err == nil {
		t.Fatal("group-writable or world-readable receipt must be rejected")
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

// The unit file and this controller must agree on where a config lives. They are
// two files that nothing forced into agreement, and when they disagreed the lock
// reported success while supervising nothing: `enable --now` on a Type=simple
// unit returns as soon as the launcher forks, so the launcher's "config file
// does not exist" failure arrived seconds after a 200 response.
func TestAgentUnitFileConfigPathMatchesController(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "services", "chrote-agent@.service"))
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	var execStart string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "ExecStart=") {
			execStart = strings.TrimPrefix(line, "ExecStart=")
		}
	}
	if execStart == "" {
		t.Fatal("unit has no ExecStart")
	}
	_, unitPath, found := strings.Cut(execStart, "--config ")
	if !found {
		t.Fatalf("ExecStart passes no --config: %q", execStart)
	}
	unitPath = strings.TrimSpace(unitPath)

	// A config path under %h would be inside the SUPERVISED account's home,
	// where the account being supervised could rewrite what its own unit obeys
	// while CHROTE kept reading a different file.
	if strings.Contains(unitPath, "%h") {
		t.Fatalf("unit config path is inside the supervised account's home: %q", unitPath)
	}

	stateRaw, err := os.ReadFile(filepath.Join("..", "..", "..", "services", "chrote-agent-state.env"))
	if err != nil {
		t.Fatalf("read shared agent state config: %v", err)
	}
	stateRoot := systemdProperty(string(stateRaw), "CHROTE_AGENT_UNITS_DIR")
	if stateRoot != defaultAgentUnitsDir {
		t.Fatalf("shared state config root = %q, want controller default %q", stateRoot, defaultAgentUnitsDir)
	}
	if !strings.Contains(string(raw), "EnvironmentFile=-/etc/chrote/chrote-agent-state.env") {
		t.Fatal("agent unit does not read the shared state configuration")
	}
	serverRaw, err := os.ReadFile(filepath.Join("..", "..", "..", "services", "chrote-srv.service"))
	if err != nil {
		t.Fatalf("read server unit: %v", err)
	}
	if !strings.Contains(string(serverRaw), "EnvironmentFile=-/etc/chrote/chrote-agent-state.env") {
		t.Fatal("server unit does not read the same state configuration as agent units")
	}
	controller := newAgentUnitController(stateRoot, func(context.Context, string, ...string) (string, error) {
		return "", nil
	})
	want, err := controller.configPath("codex-alpha", "alice")
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	// Resolve the unit's systemd specifiers the way systemd would for this pair.
	got := strings.NewReplacer("${CHROTE_AGENT_UNITS_DIR}", stateRoot, "%u", "alice", "%i", "codex-alpha").Replace(unitPath)
	if got != want {
		t.Fatalf("unit reads %q but the controller writes %q", got, want)
	}
}

// An absent unixUser is the common case, not an attack: a session on CHROTE's
// own account carries none. Treating it as invalid made those sessions
// impossible to lock and, worse, impossible to UNLOCK -- the unlock handler
// returned before reaching Forget, so the registry entry could never be cleared.
func TestAgentUnitController_EmptyUnixUserResolvesToTheServersOwnAccount(t *testing.T) {
	controller, fake, dir := newTestAgentUnitController(t)
	current, err := osuser.Current()
	if err != nil {
		t.Skipf("cannot determine current user: %v", err)
	}
	config := agentUnitConfig{
		Session: "codex-alpha", UnixUser: "", AgentKind: "codex",
		AgentSessionID: "019f4baa-e368-7ea0-8912-fb2c6f99785c", AgentBin: "/opt/bin/codex",
		TmuxBin: "/opt/bin/tmux", TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
	}
	if err := controller.Enable(context.Background(), config); err != nil {
		t.Fatalf("a session with no unixUser must be lockable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, current.Username, "codex-alpha.conf")); err != nil {
		t.Fatalf("config must land under the resolved account: %v", err)
	}
	if got := fake.calls[len(fake.calls)-1].UnixUser; got != current.Username {
		t.Fatalf("systemctl targeted %q, want the resolved account %q", got, current.Username)
	}
	// And it must be unlockable by the same absent-user request.
	if err := controller.Disable(context.Background(), "codex-alpha", ""); err != nil {
		t.Fatalf("a session locked without a unixUser must be unlockable the same way: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, current.Username, "codex-alpha.conf")); !os.IsNotExist(err) {
		t.Fatalf("unlock must remove the config, stat err = %v", err)
	}
}

// A failed `enable --now` may have already installed the wants-symlink. Undo it
// before dropping our config, or the unit becomes an orphan CHROTE can never
// see or stop: OwnsUnit needs the config, and Disable no-ops without it.
func TestAgentUnitController_FailedEnableDisablesBeforeDroppingConfig(t *testing.T) {
	controller, fake, _ := newTestAgentUnitController(t)
	fake.err = errors.New("Job for chrote-agent@codex-alpha.service failed")
	config := agentUnitConfig{
		Session: "codex-alpha", UnixUser: "alice", AgentKind: "codex",
		AgentSessionID: "019f4baa-e368-7ea0-8912-fb2c6f99785c", AgentBin: "/opt/bin/codex",
		TmuxBin: "/opt/bin/tmux", TmuxSocket: "/run/user/1234/pool/default", Workdir: "/opt/work",
	}
	if err := controller.Enable(context.Background(), config); err == nil {
		t.Fatal("a failing enable must fail the lock")
	}
	if fake.argvFor("disable") == nil {
		t.Fatalf("a failed enable must attempt to undo itself; calls: %+v", fake.calls)
	}
}
