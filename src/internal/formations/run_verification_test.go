package formations

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestS4JudgeChainVerdictRoutesGate(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4JudgeChainRunBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{outputs: map[string]string{
		"fmn_j1": "review notes",
		"fmn_j2": "pass",
	}}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusSucceeded {
		t.Fatalf("status = %+v, want succeeded from judge pass verdict", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_j1", "fmn_j2", "fmn_ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want work, judge chain, ship", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdict := eventOfType(t, events, RunEventGateVerdict)
	if verdict.Data["verdict"] != "pass" || verdict.Data["routePort"] != "pass" {
		t.Fatalf("gate verdict = %+v, want pass route from judge output", verdict)
	}
}

func TestLegacyInlineVerificationRejectsBeforeMissionAndFormationRunArtifacts(t *testing.T) {
	for _, mode := range []string{"mission", "formation"} {
		t.Run(mode, func(t *testing.T) {
			store, personas := s4RunFixture(t)
			store.Now = fixedClock()
			personas.Now = fixedClock()
			createS4Persona(t, personas, "scout")
			writeFixture(t, store.BoardPath("session-search"), s4VerificationBoardFixture("block"))
			board, err := store.ReadBoard("session-search")
			if err != nil {
				t.Fatalf("read board: %v", err)
			}
			executor := &fakeRunExecutor{}
			engine := NewRunEngine(store, personas, executor)

			var runErr error
			if mode == "mission" {
				_, runErr = engine.RunMission("session-search", RunStartRequest{
					MissionID:         "mis_showcase",
					Actor:             "agent:test",
					ExpectedBoardETag: board.ETag,
					ExpectedBoardRev:  board.Rev,
					Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
				})
			} else {
				_, runErr = engine.RunFormation("session-search", "fmn_work", FormationRunRequest{
					Actor:  "agent:test",
					Limits: RunLimits{MaxDispatch: 5, MaxAttempts: 2},
				})
			}
			if runErr == nil || !strings.Contains(runErr.Error(), "legacy_inline_verification_requires_migration") {
				t.Fatalf("run error = %v, want stable legacy inline verification migration code", runErr)
			}
			if len(executor.calls) != 0 {
				t.Fatalf("executor calls = %+v, want no execution before compatibility preflight", executor.calls)
			}
			runsDir := filepath.Join(store.Workspace, ".formations", "runs", "session-search")
			entries, readErr := os.ReadDir(runsDir)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatalf("read runs directory: %v", readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("run artifacts = %+v, want none before migration rejection", entries)
			}
		})
	}
}

func TestLegacyInlineVerificationHeaderVariantsRemainVisibleAndFailBeforeRun(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{name: "inline comment", header: "[formation.verification] # legacy"},
		{name: "spaced header", header: "[ formation.verification ]"},
		{name: "spaced dotted path", header: "[ formation . verification ]"},
		{name: "basic quoted path segment", header: `[formation."verification"]`},
		{name: "literal quoted path segment", header: `[formation.'verification']`},
		{name: "implicit parent from descendant table", header: `[formation.verification.extra]`},
		{name: "implicit parent from quoted descendant table", header: `[formation."verification".extra]`},
		{name: "implicit parent from descendant array table", header: `[[formation.verification.extra]]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, personas := s4RunFixture(t)
			store.Now = fixedClock()
			personas.Now = fixedClock()
			createS4Persona(t, personas, "scout")
			raw := strings.Replace(s4VerificationBoardFixture("block"), "[formation.verification]", test.header, 1)
			writeFixture(t, store.BoardPath("session-search"), raw)
			board, err := store.ReadBoard("session-search")
			if err != nil {
				t.Fatalf("read legacy board: %v", err)
			}
			formation, ok := findFormation(board.Formations, "fmn_work")
			if !ok || formation.Verification == nil {
				t.Fatalf("legacy verification parsed as %+v, want visible compatibility input", formation.Verification)
			}
			if findings := findBoardFindings(ValidateBoard(board).Errors, LegacyInlineVerificationMigrationCode); len(findings) != 1 {
				t.Fatalf("migration findings = %+v, want one fail-closed finding", findings)
			}
			engine := NewRunEngine(store, personas, &fakeRunExecutor{})
			_, err = engine.RunMission("session-search", RunStartRequest{
				MissionID: "mis_showcase", Actor: "agent:test", ExpectedBoardETag: board.ETag, ExpectedBoardRev: board.Rev,
			})
			if !errors.Is(err, ErrLegacyInlineVerificationRequiresMigration) {
				t.Fatalf("run error = %v, want legacy migration rejection", err)
			}
			entries, readErr := os.ReadDir(filepath.Join(store.Workspace, ".formations", "runs", "session-search"))
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatalf("read run artifacts: %v", readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("run artifacts = %+v, want none before migration rejection", entries)
			}
		})
	}
}

func TestQuotedDottedVerificationSectionSegmentDoesNotAliasLegacyRoot(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := strings.Replace(
		s4VerificationBoardFixture("block"),
		"[formation.verification]",
		`[formation."verification.extra"]`,
		1,
	)
	writeFixture(t, store.BoardPath("session-search"), raw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read quoted dotted section board: %v", err)
	}
	formation, ok := findFormation(board.Formations, "fmn_work")
	if !ok {
		t.Fatal("formation fmn_work not found")
	}
	if formation.Verification != nil {
		t.Fatalf("quoted single segment aliased verification root: %+v", formation.Verification)
	}
	if findings := findBoardFindings(ValidateBoard(board).Errors, LegacyInlineVerificationMigrationCode); len(findings) != 0 {
		t.Fatalf("quoted single segment migration findings = %+v, want none", findings)
	}
}

func TestLegacyInlineVerificationKeyVariantsRemainVisibleAndFailBeforeRun(t *testing.T) {
	section := "[formation.verification]\n" +
		"id = \"ver_work\"\n" +
		"kinds = [\"code\"]\n" +
		"criterion = \"Work is ready\"\n" +
		"onFail = \"block\""
	tests := []struct {
		name         string
		verification string
	}{
		{
			name: "dotted keys",
			verification: "verification.id = \"ver_work\"\n" +
				"verification.kinds = [\"code\"]\n" +
				"verification.criterion = \"Work is ready\"\n" +
				"verification.onFail = \"block\"",
		},
		{
			name:         "inline table",
			verification: "verification = { id = \"ver_work\", kinds = [\"code\"], criterion = \"Work is ready\", onFail = \"block\" }",
		},
		{
			name:         "quoted inline table key",
			verification: "\"verification\" = { id = \"ver_work\", kinds = [\"code\"], criterion = \"Work is ready\", onFail = \"block\" }",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, personas := s4RunFixture(t)
			store.Now = fixedClock()
			personas.Now = fixedClock()
			createS4Persona(t, personas, "scout")
			raw := strings.Replace(s4VerificationBoardFixture("block"), section, "", 1)
			raw = strings.Replace(raw, "title = \"Work\"", "title = \"Work\"\n"+test.verification, 1)
			raw += "\n[[gate]]\n" +
				"id = \"gate_replacement\"\n" +
				"title = \"Replacement review\"\n" +
				"kinds = [\"human\"]\n" +
				"criterion = \"Review the work\"\n\n" +
				"[[connection]]\n" +
				"id = \"edge_work_replacement\"\n" +
				"from = \"fmn_work:port_work_out\"\n" +
				"to = \"gate_replacement:in\"\n"
			writeFixture(t, store.BoardPath("session-search"), raw)
			board, err := store.ReadBoard("session-search")
			if err != nil {
				t.Fatalf("read legacy board: %v", err)
			}
			formation, ok := findFormation(board.Formations, "fmn_work")
			if !ok || formation.Verification == nil {
				t.Fatalf("legacy verification parsed as %+v, want visible compatibility input", formation.Verification)
			}
			if findings := findBoardFindings(ValidateBoard(board).Errors, LegacyInlineVerificationMigrationCode); len(findings) != 1 {
				t.Fatalf("migration findings = %+v, want one fail-closed finding", findings)
			}
			beforeRaw := readFile(t, store.BoardPath("session-search"))
			_, removeErr := store.RemoveFormationVerification("session-search", FormationVerificationRemovalRequest{
				FormationID:       "fmn_work",
				ReplacementGateID: "gate_replacement",
				UpdatedBy:         "agent:test",
			}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
			if !errors.Is(removeErr, ErrLegacyInlineVerificationRequiresMigration) {
				t.Fatalf("remove error = %v, want stable migration rejection for non-section representation", removeErr)
			}
			if afterRaw := readFile(t, store.BoardPath("session-search")); afterRaw != beforeRaw {
				t.Fatalf("rejected removal mutated board\nbefore:\n%s\nafter:\n%s", beforeRaw, afterRaw)
			}
			engine := NewRunEngine(store, personas, &fakeRunExecutor{})
			_, err = engine.RunMission("session-search", RunStartRequest{
				MissionID: "mis_showcase", Actor: "agent:test", ExpectedBoardETag: board.ETag, ExpectedBoardRev: board.Rev,
			})
			if !errors.Is(err, ErrLegacyInlineVerificationRequiresMigration) {
				t.Fatalf("run error = %v, want legacy migration rejection", err)
			}
			entries, readErr := os.ReadDir(filepath.Join(store.Workspace, ".formations", "runs", "session-search"))
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatalf("read run artifacts: %v", readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("run artifacts = %+v, want none before migration rejection", entries)
			}
		})
	}
}

func TestLegacyInlineVerificationResumeRejectsBeforeRunResumedAppend(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	raw := s4VerificationBoardFixture("block")
	writeFixture(t, store.BoardPath("session-search"), raw)

	runID := newPrefixedID("run")
	snapshot := runArtifactPath("session-search", runID, ".snapshot.toml")
	ledger := runArtifactPath("session-search", runID, ".ndjson")
	writeFixture(t, filepath.Join(store.Workspace, snapshot), raw)
	if err := writeInitialRunEvent(filepath.Join(store.Workspace, ledger), RunEvent{
		RunID: runID, Seq: 1, Type: RunEventStarted, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{
			"boardSlug": "session-search", "snapshot": snapshot, "limits": RunLimits{MaxDispatch: 5, MaxAttempts: 2},
		},
	}); err != nil {
		t.Fatalf("write legacy run start: %v", err)
	}
	if err := appendRunEventLine(filepath.Join(store.Workspace, ledger), RunEvent{
		RunID: runID, Seq: 2, Type: RunEventBlocked, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{
			"reason": "legacy interruption", "resumeAllowed": true, "resumePolicy": "explicit",
		},
	}); err != nil {
		t.Fatalf("write legacy blocked event: %v", err)
	}
	before, err := store.ReadRunEvents(runID)
	if err != nil {
		t.Fatalf("read legacy events: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	_, err = engine.ResumeRun(runID, RunResumeRequest{Actor: "agent:test", Mode: "reattach"})
	if err == nil || !strings.Contains(err.Error(), "legacy_inline_verification_requires_migration") {
		t.Fatalf("resume error = %v, want stable legacy inline verification migration code", err)
	}
	after, readErr := store.ReadRunEvents(runID)
	if readErr != nil {
		t.Fatalf("read events after rejected resume: %v", readErr)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected resume mutated ledger\nbefore=%+v\nafter=%+v", before, after)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("executor calls = %+v, want none on rejected legacy resume", executor.calls)
	}
}

func TestLegacyInlineVerificationHumanVerdictRejectsBeforeLedgerMutation(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	raw := strings.Replace(
		s5HumanGateBoardFixture(),
		"controller = true\n\n[[gate]]",
		"controller = true\n\n[formation.verification]\nid = \"ver_work\"\nkinds = [\"code\"]\ncriterion = \"Work is ready\"\nonFail = \"block\"\n\n[[gate]]",
		1,
	)
	runID := newPrefixedID("run")
	snapshot := runArtifactPath("session-search", runID, ".snapshot.toml")
	ledger := runArtifactPath("session-search", runID, ".ndjson")
	writeFixture(t, filepath.Join(store.Workspace, snapshot), raw)
	if err := writeInitialRunEvent(filepath.Join(store.Workspace, ledger), RunEvent{
		RunID: runID, Seq: 1, Type: RunEventStarted, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{
			"boardSlug": "session-search", "snapshot": snapshot, "limits": RunLimits{MaxDispatch: 5, MaxAttempts: 2},
		},
	}); err != nil {
		t.Fatalf("write legacy run start: %v", err)
	}
	if err := appendRunEventLine(filepath.Join(store.Workspace, ledger), RunEvent{
		RunID: runID, Seq: 2, Type: RunEventHumanInputRequested, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", GateID: "gate_review", NodeID: "gate_review",
		Data: map[string]any{"prompt": "Good enough to ship", "requestedBy": "gate_review"},
	}); err != nil {
		t.Fatalf("write legacy human request: %v", err)
	}
	before, err := store.ReadRunEvents(runID)
	if err != nil {
		t.Fatalf("read legacy events: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	_, err = engine.RecordHumanGateVerdict(runID, HumanGateVerdictRequest{
		GateID: "gate_review", Verdict: "pass", Actor: "human:perttu",
	})
	if err == nil || !strings.Contains(err.Error(), "legacy_inline_verification_requires_migration") {
		t.Fatalf("human verdict error = %v, want stable legacy inline verification migration code", err)
	}
	after, readErr := store.ReadRunEvents(runID)
	if readErr != nil {
		t.Fatalf("read events after rejected human verdict: %v", readErr)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected human verdict mutated ledger\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestLegacyInlineVerificationHumanVerdictRejectsForgedSnapshotBeforeReadOrMutation(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	store := NewStore(workspace)
	store.Now = fixedClock()
	runID := newPrefixedID("run")
	outside := filepath.Join(root, "forged.snapshot.toml")
	writeFixture(t, outside, s4VerificationBoardFixture("block"))
	forgedSnapshot, err := filepath.Rel(workspace, outside)
	if err != nil {
		t.Fatalf("derive forged snapshot path: %v", err)
	}
	ledger := filepath.Join(workspace, runArtifactPath("session-search", runID, ".ndjson"))
	if err := writeInitialRunEvent(ledger, runStartedFixture(runID, "session-search", filepath.ToSlash(forgedSnapshot))); err != nil {
		t.Fatalf("write forged run start: %v", err)
	}
	if err := appendRunEventLine(ledger, RunEvent{
		RunID: runID, Seq: 2, Type: RunEventHumanInputRequested, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", GateID: "gate_review", NodeID: "gate_review",
		Data: map[string]any{"prompt": "Review", "requestedBy": "gate_review"},
	}); err != nil {
		t.Fatalf("write human request: %v", err)
	}
	before, err := store.ReadRunEvents(runID)
	if err != nil {
		t.Fatalf("read forged run: %v", err)
	}
	engine := NewRunEngine(store, NewPersonaStore(filepath.Join(root, "agents")), &fakeRunExecutor{})
	_, err = engine.RecordHumanGateVerdict(runID, HumanGateVerdictRequest{GateID: "gate_review", Verdict: "pass", Actor: "human:test"})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("forged human verdict error = %v, want ErrRunLedgerInvalid before snapshot read", err)
	}
	after, readErr := store.ReadRunEvents(runID)
	if readErr != nil {
		t.Fatalf("read rejected forged run: %v", readErr)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("forged human verdict mutated ledger\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestLegacyInlineVerificationRawResumeAppendIsRejectedButCancelClosesRun(t *testing.T) {
	store, _ := s4RunFixture(t)
	store.Now = fixedClock()
	raw := s4VerificationBoardFixture("block")
	runID := newPrefixedID("run")
	snapshot := runArtifactPath("session-search", runID, ".snapshot.toml")
	ledger := filepath.Join(store.Workspace, runArtifactPath("session-search", runID, ".ndjson"))
	writeFixture(t, filepath.Join(store.Workspace, snapshot), raw)
	if err := writeInitialRunEvent(ledger, RunEvent{
		RunID: runID, Seq: 1, Type: RunEventStarted, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{
			"boardSlug": "session-search", "snapshot": snapshot,
		},
	}); err != nil {
		t.Fatalf("write legacy run start: %v", err)
	}
	if err := appendRunEventLine(ledger, RunEvent{
		RunID: runID, Seq: 2, Type: RunEventBlocked, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{
			"reason": "legacy interruption", "resumeAllowed": true, "resumePolicy": "explicit",
		},
	}); err != nil {
		t.Fatalf("write legacy blocked event: %v", err)
	}
	before, err := store.ReadRunEvents(runID)
	if err != nil {
		t.Fatalf("read legacy run: %v", err)
	}
	err = store.AppendRunEvent(runID, RunEvent{Type: RunEventResumed, Actor: "agent:test"})
	if err == nil || !strings.Contains(err.Error(), "legacy_inline_verification_requires_migration") {
		t.Fatalf("raw resume append error = %v, want stable migration rejection", err)
	}
	after, readErr := store.ReadRunEvents(runID)
	if readErr != nil {
		t.Fatalf("read rejected raw resume: %v", readErr)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected raw resume mutated ledger\nbefore=%+v\nafter=%+v", before, after)
	}
	if err := store.AppendRunEvent(runID, RunEvent{
		Type: RunEventCanceled, Actor: "human:perttu", Data: map[string]any{"reason": "retire legacy run", "final": true},
	}); err != nil {
		t.Fatalf("cancel legacy blocked run: %v", err)
	}
	status, err := store.ProjectRun(runID)
	if err != nil {
		t.Fatalf("project canceled legacy run: %v", err)
	}
	if status.Status != RunStatusCanceled || !status.Final {
		t.Fatalf("canceled legacy status = %+v, want final canceled", status)
	}
}

func TestLegacyInlineVerificationTerminalContainmentIgnoresUnavailableSnapshot(t *testing.T) {
	tests := []struct {
		name       string
		eventType  string
		wantStatus string
		corrupt    bool
	}{
		{name: "cancel with missing snapshot", eventType: RunEventCanceled, wantStatus: RunStatusCanceled},
		{name: "fail with unreadable snapshot", eventType: RunEventFailed, wantStatus: RunStatusFailed, corrupt: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _ := s4RunFixture(t)
			store.Now = fixedClock()
			runID := newPrefixedID("run")
			snapshot := runArtifactPath("session-search", runID, ".snapshot.toml")
			ledger := filepath.Join(store.Workspace, runArtifactPath("session-search", runID, ".ndjson"))
			if test.corrupt {
				if err := os.MkdirAll(filepath.Join(store.Workspace, snapshot), 0o700); err != nil {
					t.Fatalf("create unreadable snapshot directory: %v", err)
				}
			}
			if err := writeInitialRunEvent(ledger, RunEvent{
				RunID: runID, Seq: 1, Type: RunEventStarted, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
				MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{
					"boardSlug": "session-search", "snapshot": snapshot,
				},
			}); err != nil {
				t.Fatalf("write legacy run start: %v", err)
			}
			if err := appendRunEventLine(ledger, RunEvent{
				RunID: runID, Seq: 2, Type: RunEventBlocked, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
				MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{
					"reason": "legacy interruption", "resumeAllowed": true, "resumePolicy": "explicit",
				},
			}); err != nil {
				t.Fatalf("write legacy blocked event: %v", err)
			}
			if err := store.AppendRunEvent(runID, RunEvent{
				Type: test.eventType, Actor: "human:perttu", Data: map[string]any{"reason": "contain legacy run", "final": true},
			}); err != nil {
				t.Fatalf("append terminal containment event: %v", err)
			}
			status, err := store.ProjectRun(runID)
			if err != nil {
				t.Fatalf("project contained legacy run: %v", err)
			}
			if status.Status != test.wantStatus || !status.Final {
				t.Fatalf("contained legacy status = %+v, want final %s", status, test.wantStatus)
			}
		})
	}
}

func TestRunSnapshotReadRejectsNoncanonicalLedgerPath(t *testing.T) {
	store, _ := s4RunFixture(t)
	store.Now = fixedClock()
	raw := s4MissionOnlyBoardFixture()
	runID := newPrefixedID("run")
	noncanonicalSnapshot := filepath.ToSlash(filepath.Join(".formations", "runs", "quarry", runID+".snapshot.toml"))
	ledger := filepath.Join(store.Workspace, runArtifactPath("session-search", runID, ".ndjson"))
	writeFixture(t, filepath.Join(store.Workspace, noncanonicalSnapshot), raw)
	if err := writeInitialRunEvent(ledger, RunEvent{
		RunID: runID, Seq: 1, Type: RunEventStarted, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{
			"boardSlug": "session-search", "snapshot": noncanonicalSnapshot,
		},
	}); err != nil {
		t.Fatalf("write forged run start: %v", err)
	}
	before, err := store.ReadRunEvents(runID)
	if err != nil {
		t.Fatalf("read forged run: %v", err)
	}
	err = store.AppendRunEvent(runID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_work"})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("noncanonical snapshot append error = %v, want ErrRunLedgerInvalid", err)
	}
	after, readErr := store.ReadRunEvents(runID)
	if readErr != nil {
		t.Fatalf("read rejected forged run: %v", readErr)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("noncanonical snapshot rejection mutated ledger\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestRunSnapshotReadRejectsLedgerControlledIdentityBeforeMutation(t *testing.T) {
	tests := []struct {
		name       string
		ledgerSlug string
		setup      func(t *testing.T, store *Store, requestedRunID string) RunEvent
	}{
		{
			name: "traversal board slug",
			setup: func(t *testing.T, store *Store, requestedRunID string) RunEvent {
				t.Helper()
				outside := filepath.Join(filepath.Dir(store.Workspace), "outside")
				runsRoot := filepath.Join(store.Workspace, ".formations", "runs")
				slug, err := filepath.Rel(runsRoot, outside)
				if err != nil {
					t.Fatalf("derive traversal slug: %v", err)
				}
				snapshot := runArtifactPath(filepath.ToSlash(slug), requestedRunID, ".snapshot.toml")
				resolved := filepath.Clean(filepath.Join(store.Workspace, snapshot))
				wantResolved := filepath.Join(outside, requestedRunID+".snapshot.toml")
				if resolved != wantResolved {
					t.Fatalf("resolved traversal snapshot = %q, want %q", resolved, wantResolved)
				}
				writeFixture(t, resolved, s4MissionOnlyBoardFixture())
				return runStartedFixture(requestedRunID, filepath.ToSlash(slug), snapshot)
			},
		},
		{
			name: "first event run id differs from ledger",
			setup: func(t *testing.T, store *Store, _ string) RunEvent {
				t.Helper()
				startedRunID := newPrefixedID("run")
				snapshot := runArtifactPath("session-search", startedRunID, ".snapshot.toml")
				writeFixture(t, filepath.Join(store.Workspace, snapshot), s4MissionOnlyBoardFixture())
				return runStartedFixture(startedRunID, "session-search", snapshot)
			},
		},
		{
			name: "board slug differs from ledger directory",
			setup: func(t *testing.T, store *Store, requestedRunID string) RunEvent {
				t.Helper()
				snapshot := runArtifactPath("other-board", requestedRunID, ".snapshot.toml")
				writeFixture(t, filepath.Join(store.Workspace, snapshot), s4MissionOnlyBoardFixture())
				return runStartedFixture(requestedRunID, "other-board", snapshot)
			},
		},
		{
			name: "snapshot symlink",
			setup: func(t *testing.T, store *Store, requestedRunID string) RunEvent {
				t.Helper()
				outside := filepath.Join(filepath.Dir(store.Workspace), "outside.snapshot.toml")
				writeFixture(t, outside, s4MissionOnlyBoardFixture())
				snapshot := runArtifactPath("session-search", requestedRunID, ".snapshot.toml")
				if err := os.MkdirAll(filepath.Dir(filepath.Join(store.Workspace, snapshot)), 0o755); err != nil {
					t.Fatalf("create snapshot directory: %v", err)
				}
				if err := os.Symlink(outside, filepath.Join(store.Workspace, snapshot)); err != nil {
					t.Fatalf("symlink snapshot: %v", err)
				}
				return runStartedFixture(requestedRunID, "session-search", snapshot)
			},
		},
		{
			name:       "snapshot board slug differs from canonical run directory",
			ledgerSlug: "other-board",
			setup: func(t *testing.T, store *Store, requestedRunID string) RunEvent {
				t.Helper()
				snapshot := runArtifactPath("other-board", requestedRunID, ".snapshot.toml")
				writeFixture(t, filepath.Join(store.Workspace, snapshot), s4MissionOnlyBoardFixture())
				return runStartedFixture(requestedRunID, "other-board", snapshot)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			workspace := filepath.Join(root, "workspace")
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatalf("create workspace: %v", err)
			}
			store := NewStore(workspace)
			store.Now = fixedClock()
			requestedRunID := newPrefixedID("run")
			started := test.setup(t, store, requestedRunID)
			ledgerSlug := test.ledgerSlug
			if ledgerSlug == "" {
				ledgerSlug = "session-search"
			}
			ledger := filepath.Join(store.Workspace, runArtifactPath(ledgerSlug, requestedRunID, ".ndjson"))
			if err := writeInitialRunEvent(ledger, started); err != nil {
				t.Fatalf("write forged run start: %v", err)
			}
			before, err := store.ReadRunEvents(requestedRunID)
			if err != nil {
				t.Fatalf("read forged run: %v", err)
			}
			err = store.AppendRunEvent(requestedRunID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_work"})
			if !errors.Is(err, ErrRunLedgerInvalid) {
				t.Fatalf("forged snapshot append error = %v, want ErrRunLedgerInvalid", err)
			}
			after, readErr := store.ReadRunEvents(requestedRunID)
			if readErr != nil {
				t.Fatalf("read rejected forged run: %v", readErr)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("forged snapshot rejection mutated ledger\nbefore=%+v\nafter=%+v", before, after)
			}
		})
	}
}

func runStartedFixture(runID, boardSlug, snapshot string) RunEvent {
	return RunEvent{
		RunID: runID, Seq: 1, Type: RunEventStarted, BoardID: "brd_01J9_sesssearch", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{
			"boardSlug": boardSlug, "snapshot": snapshot,
		},
	}
}

func TestNewLegacyVerificationVerdictAppendIsRejectedButHistoricalEvidenceProjects(t *testing.T) {
	store, _ := s4RunFixture(t)
	store.Now = fixedClock()
	raw := s4MissionOnlyBoardFixture()
	writeFixture(t, store.BoardPath("session-search"), raw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID: "mis_showcase", Actor: "agent:test", ExpectedBoardETag: board.ETag, ExpectedBoardRev: board.Rev,
	})
	if err != nil {
		t.Fatalf("start clean run: %v", err)
	}
	before, err := store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read clean run: %v", err)
	}
	err = store.AppendRunEvent(started.RunID, RunEvent{Type: RunEventVerificationVerdict, NodeID: "fmn_old"})
	if err == nil || !strings.Contains(err.Error(), "legacy_inline_verification_requires_migration") {
		t.Fatalf("append verification verdict error = %v, want migration rejection", err)
	}
	after, err := store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read rejected run: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected verification verdict mutated ledger\nbefore=%+v\nafter=%+v", before, after)
	}

	historicalRunID := newPrefixedID("run")
	historicalLedger := filepath.Join(store.Workspace, runArtifactPath("session-search", historicalRunID, ".ndjson"))
	if err := writeInitialRunEvent(historicalLedger, RunEvent{
		RunID: historicalRunID, Seq: 1, Type: RunEventStarted, BoardID: "brd_showcase", BoardRev: 7,
		MissionID: "mis_showcase", Actor: "agent:test", Data: map[string]any{"boardSlug": "session-search"},
	}); err != nil {
		t.Fatalf("write historical start: %v", err)
	}
	if err := appendRunEventLine(historicalLedger, RunEvent{
		RunID: historicalRunID, Seq: 2, Type: RunEventVerificationVerdict, NodeID: "fmn_old",
		Data: map[string]any{"verificationId": "ver_old", "verdict": "fail"},
	}); err != nil {
		t.Fatalf("write historical verification evidence: %v", err)
	}
	if err := appendRunEventLine(historicalLedger, RunEvent{
		RunID: historicalRunID, Seq: 3, Type: RunEventSucceeded, Data: map[string]any{"final": true},
	}); err != nil {
		t.Fatalf("write historical final event: %v", err)
	}
	events, err := store.ReadRunEvents(historicalRunID)
	if err != nil {
		t.Fatalf("read historical ledger: %v", err)
	}
	if len(events) != 3 || events[1].Type != RunEventVerificationVerdict {
		t.Fatalf("historical events = %+v, want retained verification evidence", events)
	}
	projection, err := store.ProjectRun(historicalRunID)
	if err != nil {
		t.Fatalf("project historical ledger: %v", err)
	}
	if projection.Status != RunStatusSucceeded || !projection.Final {
		t.Fatalf("historical projection = %+v, want final status determined by run_succeeded", projection)
	}
}

func s4JudgeChainRunBoardFixture() string {
	return s4MissionOnlyBoardFixture() + `
[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[[formation.input]]
id = "port_work_in"
label = "Input"

[[formation.output]]
id = "port_work_out"
label = "Output"

[[formation.slot]]
id = "slot_work"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["code", "formation"]
criterion = "Judge the work"

[[formation]]
id = "fmn_j1"
type = "solo"
title = "Judge 1"

[[formation.input]]
id = "port_j1_in"
label = "Input"

[[formation.output]]
id = "port_j1_out"
label = "Output"

[[formation.slot]]
id = "slot_j1"
label = "Judge"
agentId = "scout"
harness = "openai-codex"
controller = true

[[formation]]
id = "fmn_j2"
type = "solo"
title = "Judge 2"

[[formation.input]]
id = "port_j2_in"
label = "Input"

[[formation.output]]
id = "port_j2_out"
label = "Output"

[[formation.slot]]
id = "slot_j2"
label = "Judge"
agentId = "scout"
harness = "openai-codex"
controller = true

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_in"
label = "Input"

[[formation.output]]
id = "port_ship_out"
label = "Output"

[[formation.slot]]
id = "slot_ship"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_work"
from = "mis_showcase:out"
to = "fmn_work:port_work_in"

[[connection]]
id = "edge_work_gate"
from = "fmn_work:port_work_out"
to = "gate_review:in"

[[connection]]
id = "edge_gate_j1"
from = "gate_review:judge"
to = "fmn_j1:port_j1_in"

[[connection]]
id = "edge_j1_j2"
from = "fmn_j1:port_j1_out"
to = "fmn_j2:port_j2_in"

[[connection]]
id = "edge_j2_gate"
from = "fmn_j2:port_j2_out"
to = "gate_review:judge"

[[connection]]
id = "edge_gate_pass_ship"
from = "gate_review:pass"
to = "fmn_ship:port_ship_in"
`
}

func s4VerificationBoardFixture(onFail string) string {
	return s4MissionOnlyBoardFixture() + `
[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[[formation.input]]
id = "port_work_in"
label = "Input"

[[formation.output]]
id = "port_work_out"
label = "Output"

[[formation.slot]]
id = "slot_work"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[formation.verification]
id = "ver_work"
kinds = ["code"]
criterion = "Work is ready"
onFail = "` + onFail + `"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_in"
label = "Input"

[[formation.output]]
id = "port_ship_out"
label = "Output"

[[formation.slot]]
id = "slot_ship"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_work"
from = "mis_showcase:out"
to = "fmn_work:port_work_in"

[[connection]]
id = "edge_work_ship"
from = "fmn_work:port_work_out"
to = "fmn_ship:port_ship_in"
`
}
