package formations

import (
	"errors"
	"fmt"
	"testing"
)

// overrideAgentUserResolution pins the three OS-facing lookups the executor uses
// to decide (a) the service uid, (b) a configured agent-user's uid, and (c) the
// uid that owns the tmux socket's backing server. It restores the real
// implementations on cleanup. No real uid change and no real tmux server are
// involved: the whole config-driven agent-user contract is exercised against
// injected identities.
func overrideAgentUserResolution(t *testing.T, serviceUID int, lookup map[string]int, ownerUID int) {
	t.Helper()
	origService := formationServiceUID
	origLookup := formationLookupUID
	origOwner := formationSocketOwnerUID
	t.Cleanup(func() {
		formationServiceUID = origService
		formationLookupUID = origLookup
		formationSocketOwnerUID = origOwner
	})
	formationServiceUID = func() (int, error) { return serviceUID, nil }
	formationLookupUID = func(name string) (int, error) {
		if lookup == nil {
			return 0, fmt.Errorf("unexpected agent-user lookup of %q", name)
		}
		uid, ok := lookup[name]
		if !ok {
			return 0, fmt.Errorf("unknown agent-user %q", name)
		}
		return uid, nil
	}
	formationSocketOwnerUID = func(string) (int, error) { return ownerUID, nil }
}

func requireBoundaryCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("boundary error = nil, want RunExecutionError with code %q", wantCode)
	}
	var executionErr *RunExecutionError
	if !errors.As(err, &executionErr) {
		t.Fatalf("boundary error = %v, want RunExecutionError with code %q", err, wantCode)
	}
	if executionErr.Code != wantCode {
		t.Fatalf("boundary code = %q, want %q", executionErr.Code, wantCode)
	}
	if executionErr.Boundary != "executor" {
		t.Fatalf("boundary = %q, want executor", executionErr.Boundary)
	}
}

// Case 1: default-to-self. An empty CHROTE_FORMATIONS_AGENT_USER defaults the
// expected agent-user to the service user, so a server already running on the
// socket (owned by the service user) is accepted with no lazy-start.
func TestTmuxExecutorDefaultsAgentUserToServiceUserAndAccepts(t *testing.T) {
	overrideAgentUserResolution(t, 4242, nil, 4242)
	cfg := tmuxTestConfig(t)
	cfg.AgentUser = ""
	client := &fakeTmuxHarnessClient{}
	if err := newTmuxFormationExecutorWithClient(nil, nil, cfg, client).validateConfiguredBoundary(); err != nil {
		t.Fatalf("default-to-self boundary error = %v, want nil", err)
	}
	if len(client.keepersStarted) != 0 {
		t.Fatalf("keepersStarted = %v, want none (server already running)", client.keepersStarted)
	}
}

// Case 2: self + no server ⇒ lazy-start is permitted (the server started is owned
// by the service user, which IS the expected agent-user).
func TestTmuxExecutorSelfLazyStartsWhenNoServer(t *testing.T) {
	overrideAgentUserResolution(t, 4242, nil, 4242)
	cfg := tmuxTestConfig(t)
	cfg.AgentUser = ""
	client := &fakeTmuxHarnessClient{noServer: true}
	if err := newTmuxFormationExecutorWithClient(nil, nil, cfg, client).validateConfiguredBoundary(); err != nil {
		t.Fatalf("self lazy-start boundary error = %v, want nil", err)
	}
	if len(client.keepersStarted) != 1 {
		t.Fatalf("keepersStarted = %v, want exactly one lazy-started keeper", client.keepersStarted)
	}
}

// Case 3: configured other-user, socket owned by that user ⇒ accept, and the
// executor never lazy-starts in the other-user path.
func TestTmuxExecutorConfiguredAgentUserOwnerMatchAccepts(t *testing.T) {
	overrideAgentUserResolution(t, 999, map[string]int{"operator": 1000}, 1000)
	cfg := tmuxTestConfig(t)
	cfg.AgentUser = "operator"
	client := &fakeTmuxHarnessClient{}
	if err := newTmuxFormationExecutorWithClient(nil, nil, cfg, client).validateConfiguredBoundary(); err != nil {
		t.Fatalf("configured agent-user match boundary error = %v, want nil", err)
	}
	if len(client.keepersStarted) != 0 {
		t.Fatalf("keepersStarted = %v, want none (never lazy-start for other-user)", client.keepersStarted)
	}
}

// Case 4: configured other-user, socket owned by the SERVICE user (a wrong-owner
// server, e.g. a stray service-user lazy-start) ⇒ fail loud, do not dispatch.
func TestTmuxExecutorConfiguredAgentUserOwnerMismatchFailsLoud(t *testing.T) {
	overrideAgentUserResolution(t, 999, map[string]int{"operator": 1000}, 999)
	cfg := tmuxTestConfig(t)
	cfg.AgentUser = "operator"
	client := &fakeTmuxHarnessClient{}
	err := newTmuxFormationExecutorWithClient(nil, nil, cfg, client).validateConfiguredBoundary()
	requireBoundaryCode(t, err, "agent_user_owner_mismatch")
	if len(client.keepersStarted) != 0 {
		t.Fatalf("keepersStarted = %v, want none on mismatch", client.keepersStarted)
	}
}

// Case 5: configured other-user + no server ⇒ fail loud, NEVER lazy-start (that
// would create a wrong-owner server), and never consult the socket owner (there
// is no server to own the socket).
func TestTmuxExecutorConfiguredAgentUserNoServerFailsLoudWithoutLazyStart(t *testing.T) {
	overrideAgentUserResolution(t, 999, map[string]int{"operator": 1000}, 1000)
	// Guard: the owner lookup must not run when the server is absent in the
	// other-user path — the code must fail loud before any ownership probe.
	formationSocketOwnerUID = func(string) (int, error) {
		t.Fatalf("socket owner lookup ran despite an absent server in the other-user path")
		return 0, nil
	}
	cfg := tmuxTestConfig(t)
	cfg.AgentUser = "operator"
	client := &fakeTmuxHarnessClient{noServer: true}
	err := newTmuxFormationExecutorWithClient(nil, nil, cfg, client).validateConfiguredBoundary()
	requireBoundaryCode(t, err, "agent_user_server_absent")
	if len(client.keepersStarted) != 0 {
		t.Fatalf("keepersStarted = %v, want none — refusing to lazy-start a wrong-owner server", client.keepersStarted)
	}
	for _, op := range client.ops {
		if op.op == "start-keeper" {
			t.Fatalf("executor lazy-started a keeper in the other-user path: %+v", op)
		}
	}
}

// Case 6: the only-own-sessions safety invariant still holds when driving a
// correctly-owned other-user server through a full run — exactly one uniquely
// named owned session is created and torn down, no foreign session is touched,
// and kill-server is never issued.
func TestTmuxExecutorOtherUserRunPreservesOnlyOwnSessionsSafety(t *testing.T) {
	overrideAgentUserResolution(t, 999, map[string]int{"operator": 1000}, 1000)
	cfg := tmuxTestConfig(t)
	cfg.AgentUser = "operator"
	foreign := []string{"sol", "terminal-1", "operator-live-agent"}
	client := &fakeTmuxHarnessClient{
		sessions: append([]string(nil), foreign...),
		artifact: "reports/tmux-agent-user.md",
	}
	status, _ := runTmuxFormationForTestWithConfig(t, client, "", cfg)
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final", status)
	}
	if len(client.created) != 1 {
		t.Fatalf("created sessions = %v, want exactly one owned on-demand session", client.created)
	}
	owned := client.created[0]
	foreignSet := map[string]bool{}
	for _, name := range foreign {
		foreignSet[name] = true
	}
	if foreignSet[owned] {
		t.Fatalf("owned session name %q collides with a foreign session", owned)
	}
	for _, op := range client.ops {
		if op.op == "kill-server" {
			t.Fatalf("executor issued kill-server in the other-user path")
		}
		if op.target == "" || op.target == owned {
			continue
		}
		t.Fatalf("tmux op %q targeted %q; the executor must only ever touch the session it created (%q)", op.op, op.target, owned)
	}
	if got, want := client.killed, []string{owned}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("killed sessions = %v, want only the owned session torn down %v", got, want)
	}
	live := map[string]bool{}
	for _, name := range client.liveSessions() {
		live[name] = true
	}
	for _, name := range foreign {
		if !live[name] {
			t.Fatalf("foreign session %q was disrupted; it must remain live and untouched", name)
		}
	}
}
