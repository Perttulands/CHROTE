package formations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestRuntimeStoreAuthorizesRunStartPastTheGuard proves the enforcement seam no
// longer fail-closes. A runtime store whose formations-data authority root is
// unavailable — the exact configuration the retired slice rejected up front with
// a typed RUNTIME_AUTHORITY_NON_AUTHORIZING error — now authorizes, so a run
// proceeds past RequireRuntimeAuthority and only blocks LATER at execution (here:
// an unresolved persona binding). The trusted operator's runtime effects are
// authorized under the trust model.
func TestRuntimeStoreAuthorizesRunStartPastTheGuard(t *testing.T) {
	workspace := t.TempDir()
	store := NewRuntimeStore(workspace, filepath.Join(t.TempDir(), "missing-formations-authority"))

	if err := store.RequireRuntimeAuthority(); err != nil {
		t.Fatalf("RequireRuntimeAuthority() = %v, want nil (authorized under the trust model)", err)
	}

	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	executor := &countingFormationExecutor{}
	engine := NewRunEngine(store, nil, executor)

	_, err := engine.RunMission("session-search", RunStartRequest{MissionID: "mis_showcase"})
	if err == nil {
		t.Fatal("run start unexpectedly succeeded without a persona store; want a later execution block")
	}
	if errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
		t.Fatalf("run start error = %v, want the guard to authorize and the run to block later at execution", err)
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

	if err := store.RequireRuntimeAuthority(); err != nil {
		t.Fatalf("runtime authority after compatibility-field mutation = %v, want authorized (nil)", err)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, otherWorkspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("runtime workspace binding changed state\nbefore: %#v\nafter:  %#v", before, got)
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

func mustGlob(t *testing.T, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return matches
}
