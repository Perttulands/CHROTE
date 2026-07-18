package formations

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const legacyScriptGateMigrationCodeForTest = "legacy_script_gate_requires_fenced_migration"

func TestLegacyScriptGateInspectionIsReadOnlyAndValidationFailsLoud(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		sourceMode   string
		sourceFields string
	}{
		{name: "legacy string", command: `command = "npm run lint"`, sourceMode: "legacy_string", sourceFields: "command"},
		{name: "argv", command: `commandArgv = ["npm", "run", "lint"]` + "\n" + `commandCwd = "dashboard"`, sourceMode: "argv", sourceFields: "commandArgv,commandCwd"},
		{name: "shell", command: `commandShell = "npm run lint"`, sourceMode: "shell", sourceFields: "commandShell"},
		{name: "cwd without executable", command: `commandCwd = "dashboard"`, sourceMode: "cwd_only", sourceFields: "commandCwd"},
		{name: "empty present", command: `commandArgv = []`, sourceMode: "empty_present", sourceFields: "commandArgv"},
		{name: "conflicting modes", command: `commandArgv = ["npm", "run", "lint"]` + "\n" + `commandShell = "npm run lint"`, sourceMode: "conflict", sourceFields: "commandArgv,commandShell"},
		{name: "duplicate field", command: `commandArgv = []` + "\n" + `commandArgv = ["npm", "run", "lint"]`, sourceMode: "conflict", sourceFields: "commandArgv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			board := mustParseValidateBoardFixture(t, legacyScriptGateBoardFixture(tt.command))
			if len(board.Gates) != 1 {
				t.Fatalf("gates = %+v, want one inspectable legacy gate", board.Gates)
			}
			findings := findBoardFindings(ValidateBoard(board).Errors, legacyScriptGateMigrationCodeForTest)
			if len(findings) != 1 || findings[0].NodeID != "gate_review" {
				t.Fatalf("migration findings = %+v, want one stable gate_review finding", findings)
			}
			plan := findings[0].Details
			if plan == nil || plan.SourceMode != tt.sourceMode || plan.Code != legacyScriptGateMigrationCodeForTest {
				t.Fatalf("migration plan = %+v, want source mode %q and stable code", plan, tt.sourceMode)
			}
			if plan.Ready || plan.ApplySupported || plan.TargetKind != "tool_plus_pure_gate" {
				t.Fatalf("migration plan authority = %+v, want non-applying Tool+pure-Gate plan", plan)
			}
			if strings.Join(plan.SourceFields, ",") != tt.sourceFields {
				t.Fatalf("migration source fields = %v, want %s", plan.SourceFields, tt.sourceFields)
			}
			if plan.BoardID != board.ID || plan.BoardRev != board.Rev || plan.BoardETag != board.ETag || plan.GateID != "gate_review" {
				t.Fatalf("migration plan identity = %+v, want exact board/gate CAS identity", plan)
			}
			encoded, err := json.Marshal(plan)
			if err != nil {
				t.Fatalf("encode migration plan: %v", err)
			}
			if !strings.Contains(string(encoded), `"boardETag"`) || strings.Contains(string(encoded), `"boardEtag"`) {
				t.Fatalf("migration plan JSON = %s, want exact boardETag contract key", encoded)
			}
			if strings.Join(plan.IncomingEdgeIDs, ",") != "edge_work_gate" || strings.Join(plan.OutgoingEdgeIDs, ",") != "edge_gate_pass_ship" {
				t.Fatalf("migration plan edges = incoming %v outgoing %v, want exact gate edges", plan.IncomingEdgeIDs, plan.OutgoingEdgeIDs)
			}
			if !strings.Contains(board.TOML, tt.command) {
				t.Fatalf("inspection TOML did not preserve %q", tt.command)
			}
		})
	}
}

func TestNewLegacyScriptGateCommandsAreRejectedWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		req  GateCreateRequest
	}{
		{name: "legacy string", req: GateCreateRequest{Command: "npm run lint"}},
		{name: "argv", req: GateCreateRequest{CommandArgv: []string{"npm", "run", "lint"}}},
		{name: "shell", req: GateCreateRequest{CommandShell: "npm run lint"}},
		{name: "cwd without executable", req: GateCreateRequest{CommandCWD: "dashboard"}},
		{name: "explicit empty field", req: GateCreateRequest{LegacyCommandFieldsPresent: true}},
		{name: "explicit empty argv", req: GateCreateRequest{CommandArgv: []string{}}},
		{name: "whitespace shell", req: GateCreateRequest{CommandShell: " "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := s4RunFixture(t)
			writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
			board, err := store.ReadBoard("session-search")
			if err != nil {
				t.Fatalf("read board: %v", err)
			}
			before := readFile(t, store.BoardPath("session-search"))
			tt.req.Title = "Legacy lint"
			tt.req.Kinds = []string{"code"}
			_, err = store.CreateGate("session-search", tt.req, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
			assertLegacyScriptGateMigrationError(t, err)
			if after := readFile(t, store.BoardPath("session-search")); after != before {
				t.Fatalf("rejected gate create changed board bytes\n--- before ---\n%s\n--- after ---\n%s", before, after)
			}
		})
	}
}

func TestLegacyScriptGateUpdateIsRejectedWithoutMutation(t *testing.T) {
	store, _ := s4RunFixture(t)
	writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	before := readFile(t, store.BoardPath("session-search"))
	_, err = store.UpdateGate("session-search", GateUpdateRequest{
		GateID:      "gate_review",
		CommandArgv: []string{"npm", "run", "lint"},
		CommandCWD:  "dashboard",
	}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	assertLegacyScriptGateMigrationError(t, err)
	if after := readFile(t, store.BoardPath("session-search")); after != before {
		t.Fatalf("rejected gate update changed board bytes\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
}

func TestLegacyScriptGateNonCommandUpdatePreservesLegacySourceFields(t *testing.T) {
	store, _ := s4RunFixture(t)
	legacyFields := `commandArgv = ["npm", "run", "lint"]` + "\n" + `commandCwd = "dashboard"`
	writeFixture(t, store.BoardPath("session-search"), legacyScriptGateBoardFixture(legacyFields))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	updated, err := store.UpdateGate("session-search", GateUpdateRequest{
		GateID:    "gate_review",
		Title:     "Renamed legacy review",
		Kinds:     []string{"human"},
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		t.Fatalf("update non-command fields: %v", err)
	}
	if !strings.Contains(updated.TOML, legacyFields) {
		t.Fatalf("non-command update did not preserve legacy source fields:\n%s", updated.TOML)
	}
	if len(updated.Gates) != 1 || updated.Gates[0].Title != "Renamed legacy review" || updated.Gates[0].LegacyScriptMigration == nil {
		t.Fatalf("updated legacy gate = %+v, want renamed degraded Gate with migration inspection", updated.Gates)
	}
}

func TestLegacyScriptGateMissionStartFailsBeforeRunArtifacts(t *testing.T) {
	store, personas := s4RunFixture(t)
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), legacyScriptGateBoardFixture(`commandArgv = ["npm", "run", "lint"]`))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	_, err = store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	assertLegacyScriptGateMigrationError(t, err)
	assertNoRunArtifacts(t, store, "session-search")
}

func TestLegacyScriptGateEnvironmentCannotReenableGateProcessExecution(t *testing.T) {
	store, personas := s4RunFixture(t)
	createS4Persona(t, personas, "scout")
	marker := filepath.Join(t.TempDir(), "legacy-gate-ran")
	t.Setenv("CHROTE_FORMATIONS_SCRIPT_GATES", "allow")
	t.Setenv("CHROTE_FORMATIONS_GATE_TIMEOUT_SECONDS", "1")
	t.Setenv("CHROTE_FORMATIONS_GATE_OUTPUT_CAP_BYTES", "64")
	writeFixture(t, store.BoardPath("session-search"), legacyScriptGateBoardFixture(`commandShell = "touch `+filepath.ToSlash(marker)+`"`))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	evaluator := &fakeGateEvaluator{verdicts: []string{"pass"}}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	engine.SetGateEvaluator(evaluator)
	_, err = engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	assertLegacyScriptGateMigrationError(t, err)
	if len(evaluator.calls) != 0 {
		t.Fatalf("gate evaluator calls = %+v, want none", evaluator.calls)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy Gate command side effect exists: %v", err)
	}
	assertNoRunArtifacts(t, store, "session-search")
}

func TestLegacyScriptGateOutsideIsolatedFormationRootDoesNotBlockTheRun(t *testing.T) {
	store, personas := s4RunFixture(t)
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), legacyScriptGateBoardFixture(`commandShell = "npm run lint"`))
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	status, err := engine.RunFormation("session-search", "fmn_work", FormationRunRequest{Personas: personas})
	if err != nil {
		t.Fatalf("run isolated formation: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want isolated formation success", status)
	}
}

func TestLegacyScriptGateOutsideMissionRootDoesNotBlockTheRun(t *testing.T) {
	store, personas := s4RunFixture(t)
	createS4Persona(t, personas, "scout")
	boardRaw := s4RunBoardFixture() + `

[[gate]]
id = "gate_unreachable"
title = "Unreachable legacy gate"
kinds = ["code"]
criterion = "Never reached"
commandArgv = ["npm", "run", "lint"]
`
	writeFixture(t, store.BoardPath("session-search"), boardRaw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want mission success with unreachable legacy gate", status)
	}
}

func TestLegacyScriptGateOutsideMissionRootCannotReceiveRawGateEvents(t *testing.T) {
	store, personas := s4RunFixture(t)
	createS4Persona(t, personas, "scout")
	boardRaw := s4GateBoardFixture(false) + `
[[gate]]
id = "gate_unreachable"
title = "Unreachable legacy gate"
kinds = ["human"]
criterion = "Never reached"
commandShell = "npm run lint"
`
	writeFixture(t, store.BoardPath("session-search"), boardRaw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start mission with unreachable legacy gate: %v", err)
	}
	ledgerPath := filepath.Join(store.Workspace, started.LedgerPath)
	before := readFile(t, ledgerPath)
	err = store.AppendRunEvent(started.RunID, RunEvent{
		Type:   RunEventGateEvaluating,
		GateID: "gate_unreachable",
		NodeID: "gate_unreachable",
	})
	assertLegacyScriptGateMigrationError(t, err)
	if after := readFile(t, ledgerPath); after != before {
		t.Fatalf("rejected out-of-root Gate event changed ledger bytes")
	}
}

func TestLegacyScriptGateRunSnapshotFencesResumeAndRawAppendButAllowsCancellation(t *testing.T) {
	t.Run("resume", func(t *testing.T) {
		store, runID, ledgerPath := legacyScriptGateHistoricalBlockedRun(t)
		if _, err := store.ReadRunEvents(runID); err != nil {
			t.Fatalf("inspect historical events: %v", err)
		}
		if _, err := store.ProjectRun(runID); err != nil {
			t.Fatalf("project historical run: %v", err)
		}
		before := readFile(t, ledgerPath)
		_, err := store.ResumeRun(runID, RunResumeRequest{Actor: "agent:test"})
		assertLegacyScriptGateMigrationError(t, err)
		if after := readFile(t, ledgerPath); after != before {
			t.Fatalf("rejected resume changed ledger bytes\n--- before ---\n%s\n--- after ---\n%s", before, after)
		}
	})

	t.Run("raw resumed append", func(t *testing.T) {
		store, runID, ledgerPath := legacyScriptGateHistoricalBlockedRun(t)
		before := readFile(t, ledgerPath)
		err := store.AppendRunEvent(runID, RunEvent{Type: RunEventResumed, Actor: "agent:test"})
		assertLegacyScriptGateMigrationError(t, err)
		if after := readFile(t, ledgerPath); after != before {
			t.Fatalf("rejected raw append changed ledger bytes\n--- before ---\n%s\n--- after ---\n%s", before, after)
		}
	})

	t.Run("raw gate append", func(t *testing.T) {
		store, runID, ledgerPath := legacyScriptGateHistoricalBlockedRun(t)
		before := readFile(t, ledgerPath)
		err := store.AppendRunEvent(runID, RunEvent{Type: RunEventGateVerdict, GateID: "gate_review", NodeID: "gate_review"})
		assertLegacyScriptGateMigrationError(t, err)
		if after := readFile(t, ledgerPath); after != before {
			t.Fatalf("rejected raw Gate append changed ledger bytes")
		}
	})

	t.Run("missing snapshot rejects authorizing append", func(t *testing.T) {
		store, runID, ledgerPath := legacyScriptGateHistoricalBlockedRun(t)
		events := readRunEvents(t, ledgerPath)
		delete(events[0].Data, "snapshot")
		rewriteRunEvents(t, ledgerPath, events)
		before := readFile(t, ledgerPath)
		err := store.AppendRunEvent(runID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_research"})
		if !errors.Is(err, ErrRunLedgerInvalid) {
			t.Fatalf("append error = %v, want ErrRunLedgerInvalid", err)
		}
		if after := readFile(t, ledgerPath); after != before {
			t.Fatalf("rejected append changed ledger bytes")
		}
	})

	t.Run("unknown mission root rejects authorizing append", func(t *testing.T) {
		store, runID, ledgerPath := legacyScriptGateHistoricalBlockedRun(t)
		events := readRunEvents(t, ledgerPath)
		events[0].MissionID = "mis_missing"
		rewriteRunEvents(t, ledgerPath, events)
		before := readFile(t, ledgerPath)
		err := store.AppendRunEvent(runID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_research"})
		if !errors.Is(err, ErrRunLedgerInvalid) {
			t.Fatalf("append error = %v, want ErrRunLedgerInvalid", err)
		}
		if after := readFile(t, ledgerPath); after != before {
			t.Fatalf("rejected append changed ledger bytes")
		}
	})

	t.Run("forged isolated formation mode rejects authorizing append", func(t *testing.T) {
		store, runID, ledgerPath := legacyScriptGateHistoricalBlockedRun(t)
		events := readRunEvents(t, ledgerPath)
		events[0].Data["mode"] = "formation"
		events[0].Data["formationId"] = "fmn_work"
		rewriteRunEvents(t, ledgerPath, events)
		before := readFile(t, ledgerPath)
		err := store.AppendRunEvent(runID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_work"})
		if !errors.Is(err, ErrRunLedgerInvalid) {
			t.Fatalf("append error = %v, want ErrRunLedgerInvalid", err)
		}
		if after := readFile(t, ledgerPath); after != before {
			t.Fatalf("rejected append changed ledger bytes")
		}
	})

	for _, tc := range []struct {
		name      string
		transform func(string) string
	}{
		{name: "snapshot board id mismatch", transform: func(raw string) string {
			return strings.Replace(raw, `id = "brd_01J9_sesssearch"`, `id = "brd_other"`, 1)
		}},
		{name: "snapshot board revision mismatch", transform: func(raw string) string {
			return strings.Replace(raw, "rev = 7", "rev = 8", 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, runID, ledgerPath := legacyScriptGateHistoricalBlockedRun(t)
			events := readRunEvents(t, ledgerPath)
			snapshotPath := stringFromEventData(events[0], "snapshot")
			writeFixture(t, filepath.Join(store.Workspace, snapshotPath), tc.transform(s4GateBoardFixture(false)))
			before := readFile(t, ledgerPath)
			err := store.AppendRunEvent(runID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_work"})
			if !errors.Is(err, ErrRunLedgerInvalid) {
				t.Fatalf("append error = %v, want ErrRunLedgerInvalid", err)
			}
			if after := readFile(t, ledgerPath); after != before {
				t.Fatalf("rejected append changed ledger bytes")
			}
		})
	}

	t.Run("terminal cancellation", func(t *testing.T) {
		store, runID, ledgerPath := legacyScriptGateHistoricalBlockedRun(t)
		if err := store.AppendRunEvent(runID, RunEvent{Type: RunEventCanceled, Actor: "agent:test", Data: map[string]any{"final": true}}); err != nil {
			t.Fatalf("cancel legacy blocked run: %v", err)
		}
		events := readRunEvents(t, ledgerPath)
		if events[len(events)-1].Type != RunEventCanceled {
			t.Fatalf("last event = %s, want run_canceled", events[len(events)-1].Type)
		}
	})

	t.Run("terminal failure", func(t *testing.T) {
		store, runID, ledgerPath := legacyScriptGateHistoricalBlockedRun(t)
		if err := store.AppendRunEvent(runID, RunEvent{Type: RunEventFailed, Actor: "agent:test", Data: map[string]any{"final": true}}); err != nil {
			t.Fatalf("fail legacy blocked run: %v", err)
		}
		events := readRunEvents(t, ledgerPath)
		if events[len(events)-1].Type != RunEventFailed {
			t.Fatalf("last event = %s, want run_failed", events[len(events)-1].Type)
		}
	})
}

func TestLegacyScriptGateSnapshotRejectsHumanVerdictBeforeLedgerMutation(t *testing.T) {
	store, personas := s4RunFixture(t)
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start historical run: %v", err)
	}
	if err := store.AppendRunEvent(started.RunID, RunEvent{
		Type:   RunEventHumanInputRequested,
		GateID: "gate_review",
		NodeID: "gate_review",
		Data:   map[string]any{"inputRef": map[string]any{"ref": "ledger://input"}},
	}); err != nil {
		t.Fatalf("append historical human request: %v", err)
	}
	writeFixture(t, filepath.Join(store.Workspace, started.SnapshotPath), legacyScriptGateBoardFixture(`commandShell = "npm run lint"`))
	ledgerPath := filepath.Join(store.Workspace, started.LedgerPath)
	before := readFile(t, ledgerPath)
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	_, err = engine.RecordHumanGateVerdict(started.RunID, HumanGateVerdictRequest{GateID: "gate_review", Verdict: "pass", Actor: "human:test"})
	assertLegacyScriptGateMigrationError(t, err)
	if after := readFile(t, ledgerPath); after != before {
		t.Fatalf("rejected human verdict changed ledger bytes")
	}
}

func legacyScriptGateHistoricalBlockedRun(t *testing.T) (*Store, string, string) {
	t.Helper()
	store, personas := s4RunFixture(t)
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start historical run: %v", err)
	}
	if err := store.AppendRunEvent(started.RunID, RunEvent{Type: RunEventBlocked, Data: map[string]any{"resumeAllowed": true}}); err != nil {
		t.Fatalf("block historical run: %v", err)
	}
	writeFixture(t, filepath.Join(store.Workspace, started.SnapshotPath), legacyScriptGateBoardFixture(`commandArgv = ["npm", "run", "lint"]`))
	return store, started.RunID, filepath.Join(store.Workspace, started.LedgerPath)
}

func rewriteRunEvents(t *testing.T, path string, events []RunEvent) {
	t.Helper()
	var lines strings.Builder
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("encode run event: %v", err)
		}
		lines.Write(raw)
		lines.WriteByte('\n')
	}
	writeFixture(t, path, lines.String())
}

func legacyScriptGateBoardFixture(command string) string {
	return strings.Replace(
		s4GateBoardFixture(false),
		`criterion = "Good enough to ship"`,
		`criterion = "Good enough to ship"`+"\n"+command,
		1,
	)
}

func assertLegacyScriptGateMigrationError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), legacyScriptGateMigrationCodeForTest) {
		t.Fatalf("error = %v, want %s", err, legacyScriptGateMigrationCodeForTest)
	}
}

func assertNoRunArtifacts(t *testing.T, store *Store, slug string) {
	t.Helper()
	runDir := filepath.Join(store.Workspace, ".formations", "runs", slug)
	entries, err := os.ReadDir(runDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read run artifacts: %v", err)
	}
	if err == nil && len(entries) != 0 {
		t.Fatalf("run artifacts = %v, want none", entries)
	}
}
