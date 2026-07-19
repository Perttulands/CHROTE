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

func TestMatchingRuntimeAuthorityRejectsBeforeEngineEffects(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	bindRuntimeAuthorityFixtureToOpenedWorkspace(t, &fixture, fixture.workspace)
	writeFixture(t, filepath.Join(fixture.workspace, ".formations", "boards", "session-search.formation.toml"), s4RunBoardFixture())
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)
	store := NewRuntimeStore(fixture.workspace, filepath.Dir(fixture.root))
	executor := &countingFormationExecutor{}
	engine := NewRunEngine(store, nil, executor)

	_, err := engine.RunMission("session-search", RunStartRequest{MissionID: "mis_showcase"})
	var authorityErr *RuntimeAuthorityNonAuthorizingError
	if !errors.As(err, &authorityErr) || authorityErr.Reason != RuntimeAuthorityCapabilityDisabled {
		t.Fatalf("matching authority run error = %#v, want disabled non-authorizing capability", err)
	}
	_, err = store.StartRun("session-search", RunStartRequest{MissionID: "mis_showcase"})
	authorityErr = nil
	if !errors.As(err, &authorityErr) || authorityErr.Reason != RuntimeAuthorityCapabilityDisabled {
		t.Fatalf("matching authority store start error = %#v, want disabled non-authorizing capability", err)
	}
	if executor.calls != 0 {
		t.Fatalf("matching authority executor calls = %d, want zero", executor.calls)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("matching authority engine rejection changed state\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestRuntimeStartsRejectSelectedDefinitionBeforeUnavailableAuthority(t *testing.T) {
	tests := []struct {
		name      string
		slug      string
		board     string
		malformed bool
		wantErr   error
		start     func(*Store, *RunEngine) error
	}{
		{
			name:    "store missing definition",
			slug:    "missing",
			wantErr: ErrNotFound,
			start: func(store *Store, _ *RunEngine) error {
				_, err := store.StartRun("missing", RunStartRequest{MissionID: "mis_showcase"})
				return err
			},
		},
		{
			name:      "store malformed definition",
			slug:      "malformed",
			board:     malformedRuntimeStartBoardFixture(),
			malformed: true,
			start: func(store *Store, _ *RunEngine) error {
				_, err := store.StartRun("malformed", RunStartRequest{MissionID: "mis_showcase"})
				return err
			},
		},
		{
			name:    "store missing mission",
			slug:    "session-search",
			board:   s4RunBoardFixture(),
			wantErr: ErrNotFound,
			start: func(store *Store, _ *RunEngine) error {
				_, err := store.StartRun("session-search", RunStartRequest{MissionID: "mis_missing"})
				return err
			},
		},
		{
			name:    "store legacy inline verification",
			slug:    "session-search",
			board:   s4VerificationBoardFixture("block"),
			wantErr: ErrLegacyInlineVerificationRequiresMigration,
			start: func(store *Store, _ *RunEngine) error {
				_, err := store.StartRun("session-search", RunStartRequest{MissionID: "mis_showcase"})
				return err
			},
		},
		{
			name:    "store reachable legacy script Gate",
			slug:    "session-search",
			board:   legacyScriptGateBoardFixture(`commandArgv = ["npm", "run", "lint"]`),
			wantErr: ErrLegacyScriptGateRequiresFencedMigration,
			start: func(store *Store, _ *RunEngine) error {
				_, err := store.StartRun("session-search", RunStartRequest{MissionID: "mis_showcase"})
				return err
			},
		},
		{
			name:    "engine mission missing definition",
			slug:    "missing",
			wantErr: ErrNotFound,
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunMission("missing", RunStartRequest{MissionID: "mis_showcase"})
				return err
			},
		},
		{
			name:      "engine mission malformed definition",
			slug:      "malformed",
			board:     malformedRuntimeStartBoardFixture(),
			malformed: true,
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunMission("malformed", RunStartRequest{MissionID: "mis_showcase"})
				return err
			},
		},
		{
			name:    "engine mission missing root",
			slug:    "session-search",
			board:   s4RunBoardFixture(),
			wantErr: ErrNotFound,
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunMission("session-search", RunStartRequest{MissionID: "mis_missing"})
				return err
			},
		},
		{
			name:    "engine mission legacy inline verification",
			slug:    "session-search",
			board:   s4VerificationBoardFixture("block"),
			wantErr: ErrLegacyInlineVerificationRequiresMigration,
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunMission("session-search", RunStartRequest{MissionID: "mis_showcase"})
				return err
			},
		},
		{
			name:    "engine mission reachable legacy script Gate",
			slug:    "session-search",
			board:   legacyScriptGateBoardFixture(`commandArgv = ["npm", "run", "lint"]`),
			wantErr: ErrLegacyScriptGateRequiresFencedMigration,
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunMission("session-search", RunStartRequest{MissionID: "mis_showcase"})
				return err
			},
		},
		{
			name:      "engine formation malformed definition",
			slug:      "malformed",
			board:     malformedRuntimeStartBoardFixture(),
			malformed: true,
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunFormation("malformed", "fmn_work", FormationRunRequest{})
				return err
			},
		},
		{
			name:    "engine formation missing root",
			slug:    "session-search",
			board:   s4RunBoardFixture(),
			wantErr: ErrNotFound,
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunFormation("session-search", "fmn_missing", FormationRunRequest{})
				return err
			},
		},
		{
			name:    "engine formation legacy inline verification",
			slug:    "session-search",
			board:   s4VerificationBoardFixture("block"),
			wantErr: ErrLegacyInlineVerificationRequiresMigration,
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunFormation("session-search", "fmn_work", FormationRunRequest{})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			store := NewRuntimeStore(workspace, filepath.Join(t.TempDir(), "missing-formations-data"))
			if test.board != "" {
				writeFixture(t, store.BoardPath(test.slug), test.board)
			}
			if test.malformed {
				if _, err := store.ReadBoard(test.slug); err == nil || errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
					t.Fatalf("malformed fixture read error = %v, want definition parse rejection", err)
				}
			}
			executor := &countingFormationExecutor{}
			evaluator := &countingGateEvaluator{}
			engine := NewRunEngine(store, nil, executor)
			engine.SetGateEvaluator(evaluator)

			err := test.start(store, engine)
			if err == nil {
				t.Fatal("start passed unavailable authority and invalid selected definition")
			}
			if errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
				t.Fatalf("start error = %v, want selected-definition rejection before runtime authority", err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("start error = %v, want %v", err, test.wantErr)
			}
			if executor.calls != 0 || evaluator.calls != 0 {
				t.Fatalf("start effects = executor:%d evaluator:%d, want zero", executor.calls, evaluator.calls)
			}
			if matches := mustGlob(t, filepath.Join(workspace, ".formations", "runs", "*")); len(matches) != 0 {
				t.Fatalf("rejected start created run artifacts: %v", matches)
			}
		})
	}
}

func malformedRuntimeStartBoardFixture() string {
	return `schema = 2
id = "brd_malformed"
slug = "malformed"
title = "Malformed Tool definition"
rev = 1

[tool]
id = "tool_malformed"
`
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

func TestRuntimeAuthorityRejectionPrecedesRuntimeEffectsAfterStartPreflight(t *testing.T) {
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
