package formations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeStoreAuthorityBoundaryRemainsNonAuthorizingAfterExactMatch(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	bindRuntimeAuthorityFixtureToOpenedWorkspace(t, &fixture, fixture.workspace)
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)
	store := NewRuntimeStore(fixture.workspace, filepath.Dir(fixture.root))

	err := store.RequireRuntimeAuthority()
	if !errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
		t.Fatalf("runtime authority error = %v, want non-authorizing sentinel", err)
	}
	var authorityErr *RuntimeAuthorityNonAuthorizingError
	if !errors.As(err, &authorityErr) || authorityErr.Reason != RuntimeAuthorityCapabilityDisabled {
		t.Fatalf("runtime authority error = %#v, want typed disabled capability", err)
	}
	if authorityErr.Capability.ID != RuntimeAuthorityGuardCapabilityV1 || authorityErr.Capability.Execution {
		t.Fatalf("runtime authority capability = %+v, want disabled guard capability", authorityErr.Capability)
	}
	if after := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(after, before) {
		t.Fatalf("non-authorizing runtime guard mutated state\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := NewStore(fixture.workspace).RequireRuntimeAuthority(); err != nil {
		t.Fatalf("schema-1 compatibility store unexpectedly requires runtime authority: %v", err)
	}

	whitespaceRoot := NewRuntimeStore(fixture.workspace, " "+filepath.Dir(fixture.root))
	err = whitespaceRoot.RequireRuntimeAuthority()
	var whitespaceErr *RuntimeAuthorityNonAuthorizingError
	if !errors.As(err, &whitespaceErr) || whitespaceErr.Reason != RuntimeAuthorityGuardRejected || whitespaceErr.Code != RuntimeAuthorityGuardNoncanonical {
		t.Fatalf("whitespace-prefixed authority root = %#v, want noncanonical guard rejection", err)
	}

	writeAuthorityFixture(t, fixture.workspaceDB, []byte(`{"authoritySchema":2,"privatePath":"/do/not/expose"}`))
	malformedBefore := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)
	err = store.RequireRuntimeAuthority()
	var malformedErr *RuntimeAuthorityNonAuthorizingError
	if !errors.As(err, &malformedErr) || malformedErr.Reason != RuntimeAuthorityGuardRejected {
		t.Fatalf("malformed authority error = %#v, want sanitized guard rejection", err)
	}
	if message := err.Error(); message != ErrRuntimeAuthorityNonAuthorizing.Error() || strings.Contains(message, fixture.root) || strings.Contains(message, testWorkspaceAuthorityID) || strings.Contains(message, "privatePath") {
		t.Fatalf("malformed authority error leaked private detail: %q", message)
	}
	if after := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(after, malformedBefore) {
		t.Fatalf("malformed authority rejection mutated state\nbefore: %#v\nafter:  %#v", malformedBefore, after)
	}
}

func TestRuntimeStoreUsesImmutableWorkspaceAfterAuthorityBinding(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	bindRuntimeAuthorityFixtureToOpenedWorkspace(t, &fixture, fixture.workspace)
	otherWorkspace := t.TempDir()
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, otherWorkspace)
	store := NewRuntimeStore(fixture.workspace, filepath.Dir(fixture.root))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			store.Workspace = otherWorkspace
		}
	}()
	for i := 0; i < 100; i++ {
		if got, want := store.BoardPath("bound"), filepath.Join(fixture.workspace, ".formations", "boards", "bound.formation.toml"); got != want {
			t.Fatalf("runtime board path = %q, want immutable authority-bound path %q", got, want)
		}
		if got, err := store.workspaceAbsolutePath(); err != nil || got != fixture.workspace {
			t.Fatalf("runtime workspace path = %q, %v, want immutable authority-bound path %q", got, err, fixture.workspace)
		}
	}
	<-done

	err := store.RequireRuntimeAuthority()
	var authorityErr *RuntimeAuthorityNonAuthorizingError
	if !errors.As(err, &authorityErr) || authorityErr.Reason != RuntimeAuthorityCapabilityDisabled {
		t.Fatalf("runtime authority error after compatibility-field mutation = %#v, want bound non-authorizing capability", err)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, otherWorkspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("runtime workspace binding changed state\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestRuntimeAuthorityRejectionPrecedesEngineDispatchAndTmuxSideEffects(t *testing.T) {
	workspace := t.TempDir()
	store := NewRuntimeStore(workspace, filepath.Join(t.TempDir(), "missing-formations-data"))
	executor := &countingFormationExecutor{}
	engine := NewRunEngine(store, nil, executor)
	gateEvaluator := &countingGateEvaluator{}
	engine.SetGateEvaluator(gateEvaluator)

	assertNonAuthorizing := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
			t.Fatalf("%s error = %v, want runtime-authority rejection", name, err)
		}
	}
	if _, err := engine.RunMission("missing", RunStartRequest{}); err != nil {
		assertNonAuthorizing("mission run", err)
	} else {
		t.Fatal("mission run passed unavailable authority")
	}
	if _, err := engine.RunFormation("missing", "formation_missing", FormationRunRequest{}); err != nil {
		assertNonAuthorizing("formation run", err)
	} else {
		t.Fatal("formation run passed unavailable authority")
	}
	if _, err := engine.ResumeRun("run_missing", RunResumeRequest{}); err != nil {
		assertNonAuthorizing("resume", err)
	} else {
		t.Fatal("resume passed unavailable authority")
	}
	if _, err := engine.RecordHumanGateVerdict("run_missing", HumanGateVerdictRequest{}); err != nil {
		assertNonAuthorizing("verdict", err)
	} else {
		t.Fatal("verdict passed unavailable authority")
	}
	if executor.calls != 0 {
		t.Fatalf("formation executor calls = %d, want zero", executor.calls)
	}
	if gateEvaluator.calls != 0 {
		t.Fatalf("gate evaluator calls = %d, want zero", gateEvaluator.calls)
	}

	adapter := &countingDispatchAdapter{}
	dispatcher := NewSlotDispatcher(store, adapter)
	if _, err := dispatcher.DispatchSlot("run_missing", SlotDispatchRequest{}); err != nil {
		assertNonAuthorizing("dispatch", err)
	} else {
		t.Fatal("dispatch passed unavailable authority")
	}
	if adapter.calls != 0 {
		t.Fatalf("dispatch adapter calls = %d, want zero", adapter.calls)
	}
	if err := dispatcher.CompleteFromCapture("run_missing", "dispatch_missing", "<<<CHROTE-DONE run-id=run_missing status=ok>>>"); err != nil {
		assertNonAuthorizing("capture completion", err)
	} else {
		t.Fatal("capture completion passed unavailable authority")
	}
	if recorded, err := store.RecordEscalationFromCapture("run_missing", "formation_missing", "<<<CHROTE-ESCALATE reason=test>>>"); err != nil {
		assertNonAuthorizing("capture escalation", err)
	} else {
		t.Fatalf("capture escalation passed unavailable authority: recorded=%v", recorded)
	}

	labExecutor := NewLabFormationExecutor(store, nil, LabExecutorConfig{})
	if _, err := labExecutor.ExecuteFormation(FormationExecution{}); err != nil {
		assertNonAuthorizing("lab execution", err)
	} else {
		t.Fatal("lab execution passed unavailable authority")
	}

	tmuxClient := &fakeTmuxHarnessClient{}
	tmuxExecutor := newTmuxFormationExecutorWithClient(store, nil, TmuxExecutorConfig{}, tmuxClient)
	if _, err := tmuxExecutor.ExecuteFormation(FormationExecution{}); err != nil {
		assertNonAuthorizing("tmux execution", err)
	} else {
		t.Fatal("tmux execution passed unavailable authority")
	}
	if _, err := tmuxExecutor.ReattachFormationDispatch(FormationReattachRequest{}); err != nil {
		assertNonAuthorizing("tmux reattach", err)
	} else {
		t.Fatal("tmux reattach passed unavailable authority")
	}
	if tmuxClient.listCalls != 0 || tmuxClient.describeCalls != 0 || tmuxClient.sendCalls != 0 || tmuxClient.captureCalls != 0 {
		t.Fatalf("tmux calls = list:%d describe:%d send:%d capture:%d, want zero", tmuxClient.listCalls, tmuxClient.describeCalls, tmuxClient.sendCalls, tmuxClient.captureCalls)
	}

	if _, err := store.StartRun("missing", RunStartRequest{}); err != nil {
		assertNonAuthorizing("store start", err)
	} else {
		t.Fatal("store start passed unavailable authority")
	}
	if err := store.AppendRunEvent("run_missing", RunEvent{Type: RunEventCanceled}); err != nil {
		assertNonAuthorizing("store append", err)
	} else {
		t.Fatal("store append passed unavailable authority")
	}
	if _, err := store.ResumeRun("run_missing", RunResumeRequest{}); err != nil {
		assertNonAuthorizing("store resume", err)
	} else {
		t.Fatal("store resume passed unavailable authority")
	}
	if len(mustGlob(t, filepath.Join(workspace, ".formations", "*"))) != 0 {
		t.Fatalf("runtime rejection created Formations artifacts in %s", workspace)
	}
}

func TestStoreRejectsSchema2LedgerBeforeLegacyProjection(t *testing.T) {
	workspace := t.TempDir()
	runID := "run_01KXNP6VY3227H78329V52CKF8"
	ledger := filepath.Join(workspace, ".formations", "runs", "demo", runID+".ndjson")
	if err := os.MkdirAll(filepath.Dir(ledger), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(fmt.Sprintf(
		`{"schema":2,"authoritySchema":2,"writerFence":1,"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":1,"type":"run_started","actor":"agent:test","data":{"boardSlug":"demo"}}`+"\n",
		runID,
	))
	if err := os.WriteFile(ledger, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewStore(workspace)
	if events, err := store.ReadRunEvents(runID); !errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
		t.Fatalf("schema-2 ReadRunEvents = events:%+v err:%v, want typed non-authorizing rejection", events, err)
	}
	if status, err := store.ProjectRun(runID); !errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
		t.Fatalf("schema-2 ProjectRun = status:%+v err:%v, want typed non-authorizing rejection", status, err)
	}
	if after, err := os.ReadFile(ledger); err != nil || string(after) != string(raw) {
		t.Fatalf("schema-2 rejection mutated ledger: after=%q err=%v", after, err)
	}
}

type countingFormationExecutor struct {
	calls int
}

type countingGateEvaluator struct {
	calls int
}

func (e *countingGateEvaluator) EvaluateGate(GateEvaluation) (GateEvaluationResult, error) {
	e.calls++
	return GateEvaluationResult{}, nil
}

func (e *countingFormationExecutor) ExecuteFormation(FormationExecution) (FormationExecutionResult, error) {
	e.calls++
	return FormationExecutionResult{}, nil
}

type countingDispatchAdapter struct {
	calls int
}

func (a *countingDispatchAdapter) SendSlotDispatch(SlotDispatchPayload) error {
	a.calls++
	return nil
}

func mustGlob(t *testing.T, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return matches
}
