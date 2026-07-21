package formations

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type interposingLedgerReadSeeker struct {
	passes [][]byte
	reader *bytes.Reader
	seeks  int
}

func (r *interposingLedgerReadSeeker) Read(destination []byte) (int, error) {
	if r.reader == nil {
		return 0, io.EOF
	}
	return r.reader.Read(destination)
}

func (r *interposingLedgerReadSeeker) Seek(offset int64, whence int) (int64, error) {
	if offset != 0 || whence != io.SeekStart {
		return 0, errors.New("interposing ledger only supports rewind")
	}
	index := r.seeks
	if index >= len(r.passes) {
		index = len(r.passes) - 1
	}
	r.reader = bytes.NewReader(r.passes[index])
	r.seeks++
	return 0, nil
}

func TestRunLedgerClassificationAndLegacyDecodeConsumeSameBytes(t *testing.T) {
	runID := newPrefixedID("run")
	legacy := testRunLedgerBytes(t, testRunStartedEvent(runID, "session-search"))
	schema2 := []byte(`{"schema":2,"authoritySchema":2,"writerFence":1,"ts":"2026-07-18T12:00:00Z","runId":"` + runID + `","seq":1,"type":"run_failed","actor":"agent:test"}` + "\n")
	ledger := &interposingLedgerReadSeeker{passes: [][]byte{legacy, schema2}}

	events, err := classifyAndReadRunEvents(ledger, runID)
	if err != nil {
		t.Fatalf("classify and decode interposed ledger: %v", err)
	}
	if ledger.seeks != 1 {
		t.Fatalf("ledger rewind count = %d, want one classification/decode pass", ledger.seeks)
	}
	if len(events) != 1 || events[0].Type != RunEventStarted {
		t.Fatalf("decoded events = %#v, want exact legacy bytes classified on first pass", events)
	}
}

func TestConfiguredWorkspaceSymlinkStillSupportsRunInspection(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	workspaceAlias := filepath.Join(root, "workspace-alias")
	runID := newPrefixedID("run")
	events := testLegacyRunEvents(runID, "session-search")
	ledgerPath := filepath.Join(workspace, runArtifactPath("session-search", runID, ".ndjson"))
	writeFixture(t, ledgerPath, string(testRunLedgerBytes(t, events...)))
	writeFixture(t, filepath.Join(workspace, runArtifactPath("session-search", runID, ".snapshot.toml")), s4RunBoardFixture())
	writeFixture(t, filepath.Join(workspace, runArtifactPath("session-search", runID, ".bindings.toml")), `schema = 1
runId = "`+runID+`"
boardId = "brd_01J9_sesssearch"
boardSlug = "session-search"
boardRev = 7
missionId = "mis_showcase"
`)
	if err := os.Symlink(workspace, workspaceAlias); err != nil {
		t.Fatalf("symlink configured workspace: %v", err)
	}

	store := NewStore(workspaceAlias)
	gotEvents, err := store.ReadRunEvents(runID)
	if err != nil {
		t.Fatalf("read run through configured workspace symlink: %v", err)
	}
	if len(gotEvents) != len(events) {
		t.Fatalf("event count = %d, want %d", len(gotEvents), len(events))
	}
	projection, err := store.ProjectRun(runID)
	if err != nil {
		t.Fatalf("project run through configured workspace symlink: %v", err)
	}
	if projection.RunID != runID || projection.BoardSlug != "session-search" || projection.Status != RunStatusBlocked {
		t.Fatalf("projection through configured workspace symlink = %+v", projection)
	}
	runs, err := store.ListRuns(RunListFilter{})
	if err != nil {
		t.Fatalf("list runs through configured workspace symlink: %v", err)
	}
	if len(runs) != 1 || runs[0].RunID != runID {
		t.Fatalf("listed runs through configured workspace symlink = %+v", runs)
	}
}

func TestConfiguredWorkspaceSymlinkStillSupportsRunCreation(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	workspaceAlias := filepath.Join(root, "workspace-alias")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.Symlink(workspace, workspaceAlias); err != nil {
		t.Fatalf("symlink configured workspace: %v", err)
	}
	store := NewStore(workspaceAlias)
	store.Now = fixedClock()
	personas := NewPersonaStore(filepath.Join(root, "agents"))
	personas.Now = fixedClock()
	if _, err := personas.CreatePersona(CreatePersonaRequest{
		ID: "scout", Kind: "specialist", Capabilities: []string{"research"}, Harness: "openai-codex",
	}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board through configured workspace symlink: %v", err)
	}

	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start run through configured workspace symlink: %v", err)
	}
	events, err := store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read created run through configured workspace symlink: %v", err)
	}
	if len(events) != 1 || events[0].Type != RunEventStarted {
		t.Fatalf("created run events = %+v", events)
	}
}

func TestRunInspectionRejectsSymlinkedDescendantOfConfiguredWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	externalRuns := filepath.Join(root, "external-runs")
	runID := newPrefixedID("run")
	externalLedger := filepath.Join(externalRuns, "session-search", runID+".ndjson")
	ledgerBefore := testRunLedgerBytes(t, testRunStartedEvent(runID, "session-search"))
	writeFixture(t, externalLedger, string(ledgerBefore))
	if err := os.MkdirAll(filepath.Join(workspace, ".formations"), 0o755); err != nil {
		t.Fatalf("create workspace formations directory: %v", err)
	}
	if err := os.Symlink(externalRuns, filepath.Join(workspace, ".formations", "runs")); err != nil {
		t.Fatalf("symlink external runs directory: %v", err)
	}

	if _, err := NewStore(workspace).ReadRunEvents(runID); !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("read through descendant symlink error = %v, want ErrRunLedgerInvalid", err)
	}
	if got := readFile(t, externalLedger); got != string(ledgerBefore) {
		t.Fatal("rejected descendant symlink read mutated external ledger")
	}
}

func TestRunLedgerSymlinkCannotMutateVictimOnTerminalAppend(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(workspace)
	store.Now = fixedClock()
	runID := newPrefixedID("run")
	ledgerPath := filepath.Join(workspace, runArtifactPath("session-search", runID, ".ndjson"))
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o770); err != nil {
		t.Fatalf("create run directory: %v", err)
	}
	victimPath := filepath.Join(root, "victim.ndjson")
	victimBefore := testLegacyLedgerBytes(t, runID, "session-search")
	if err := os.WriteFile(victimPath, victimBefore, 0o600); err != nil {
		t.Fatalf("write victim ledger: %v", err)
	}
	if err := os.Symlink(victimPath, ledgerPath); err != nil {
		t.Fatalf("symlink run ledger: %v", err)
	}

	err := store.AppendRunEvent(runID, RunEvent{
		Type: RunEventCanceled, Actor: "human:test", Data: map[string]any{"final": true},
	})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("terminal append through ledger symlink error = %v, want ErrRunLedgerInvalid", err)
	}
	victimAfter, readErr := os.ReadFile(victimPath)
	if readErr != nil {
		t.Fatalf("read victim after rejected append: %v", readErr)
	}
	if string(victimAfter) != string(victimBefore) {
		t.Fatalf("rejected terminal append mutated symlink victim")
	}
}

func TestRunLedgerLockSymlinkCannotMutateVictimOrLedger(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(workspace)
	store.Now = fixedClock()
	runID := newPrefixedID("run")
	ledgerPath := filepath.Join(workspace, runArtifactPath("session-search", runID, ".ndjson"))
	ledgerBefore := testLegacyLedgerBytes(t, runID, "session-search")
	writeFixture(t, ledgerPath, string(ledgerBefore))

	victimPath := filepath.Join(root, "lock-victim")
	victimBefore := []byte("do not lock or chmod me\n")
	if err := os.WriteFile(victimPath, victimBefore, 0o600); err != nil {
		t.Fatalf("write lock victim: %v", err)
	}
	if err := os.Symlink(victimPath, ledgerPath+".lock"); err != nil {
		t.Fatalf("symlink run lock: %v", err)
	}

	err := store.AppendRunEvent(runID, RunEvent{
		Type: RunEventFailed, Actor: "agent:test", Data: map[string]any{"final": true},
	})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("terminal append through lock symlink error = %v, want ErrRunLedgerInvalid", err)
	}
	if got := readFile(t, ledgerPath); got != string(ledgerBefore) {
		t.Fatalf("rejected lock-symlink append mutated ledger")
	}
	victimAfter, readErr := os.ReadFile(victimPath)
	if readErr != nil {
		t.Fatalf("read lock victim: %v", readErr)
	}
	if string(victimAfter) != string(victimBefore) {
		t.Fatalf("rejected lock-symlink append mutated victim bytes")
	}
	info, statErr := os.Stat(victimPath)
	if statErr != nil {
		t.Fatalf("stat lock victim: %v", statErr)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("rejected lock-symlink append changed victim mode to %04o, want %04o", got, want)
	}
}

func TestRunLedgerLockHardlinkCannotMutateVictimOrLedger(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(workspace)
	store.Now = fixedClock()
	runID := newPrefixedID("run")
	ledgerPath := filepath.Join(workspace, runArtifactPath("session-search", runID, ".ndjson"))
	ledgerBefore := testLegacyLedgerBytes(t, runID, "session-search")
	writeFixture(t, ledgerPath, string(ledgerBefore))

	victimPath := filepath.Join(root, "hardlink-victim")
	victimBefore := []byte("do not lock or chmod me\n")
	if err := os.WriteFile(victimPath, victimBefore, 0o600); err != nil {
		t.Fatalf("write hardlink victim: %v", err)
	}
	if err := os.Link(victimPath, ledgerPath+".lock"); err != nil {
		t.Fatalf("hardlink run lock: %v", err)
	}

	err := store.AppendRunEvent(runID, RunEvent{
		Type: RunEventFailed, Actor: "agent:test", Data: map[string]any{"final": true},
	})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("terminal append through lock hardlink error = %v, want ErrRunLedgerInvalid", err)
	}
	if got := readFile(t, ledgerPath); got != string(ledgerBefore) {
		t.Fatalf("rejected lock-hardlink append mutated ledger")
	}
	victimAfter, readErr := os.ReadFile(victimPath)
	if readErr != nil {
		t.Fatalf("read hardlink victim: %v", readErr)
	}
	if string(victimAfter) != string(victimBefore) {
		t.Fatalf("rejected lock-hardlink append mutated victim bytes")
	}
	info, statErr := os.Stat(victimPath)
	if statErr != nil {
		t.Fatalf("stat hardlink victim: %v", statErr)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("rejected lock-hardlink append changed victim mode to %04o, want %04o", got, want)
	}
}

func TestTerminalEventsStillValidateCanonicalLedgerIdentity(t *testing.T) {
	store, _ := s4RunFixture(t)
	store.Now = fixedClock()
	runID := newPrefixedID("run")
	ledgerPath := filepath.Join(store.Workspace, runArtifactPath("session-search", runID, ".ndjson"))
	ledgerBefore := testLegacyLedgerBytes(t, runID, "../outside")
	writeFixture(t, ledgerPath, string(ledgerBefore))

	for _, eventType := range []string{RunEventCanceled, RunEventFailed} {
		err := store.AppendRunEvent(runID, RunEvent{
			Type: eventType, Actor: "human:test", Data: map[string]any{"final": true},
		})
		if !errors.Is(err, ErrRunLedgerInvalid) {
			t.Fatalf("%s with forged board identity error = %v, want ErrRunLedgerInvalid", eventType, err)
		}
	}
	if got := readFile(t, ledgerPath); got != string(ledgerBefore) {
		t.Fatalf("rejected terminal containment events mutated forged ledger")
	}
}

func TestRunLedgerReaderBoundsEachEvent(t *testing.T) {
	store, _ := s4RunFixture(t)
	runID := newPrefixedID("run")
	ledgerPath := filepath.Join(store.Workspace, runArtifactPath("session-search", runID, ".ndjson"))
	events := testLegacyRunEvents(runID, "session-search")
	events[0].Data["oversized"] = strings.Repeat("x", runtimeAuthorityMaxEventBytes)
	writeFixture(t, ledgerPath, string(testRunLedgerBytes(t, events...)))

	if _, err := store.ReadRunEvents(runID); !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("oversized ledger event error = %v, want ErrRunLedgerInvalid", err)
	}
}

func TestRunSnapshotReaderIsBoundedBeforeAuthorizingAppend(t *testing.T) {
	store, _ := s4RunFixture(t)
	store.Now = fixedClock()
	runID := newPrefixedID("run")
	snapshotPath := runArtifactPath("session-search", runID, ".snapshot.toml")
	ledgerPath := filepath.Join(store.Workspace, runArtifactPath("session-search", runID, ".ndjson"))
	oversizedBoard := s4MissionOnlyBoardFixture() + "\n# " + strings.Repeat("x", int(runtimeAuthorityMaxRecordBytes)) + "\n"
	writeFixture(t, filepath.Join(store.Workspace, snapshotPath), oversizedBoard)
	ledgerBefore := testRunLedgerBytes(t, testRunStartedEvent(runID, "session-search"))
	writeFixture(t, ledgerPath, string(ledgerBefore))

	err := store.AppendRunEvent(runID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_work"})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("append with oversized snapshot error = %v, want ErrRunLedgerInvalid", err)
	}
	if got := readFile(t, ledgerPath); got != string(ledgerBefore) {
		t.Fatalf("oversized snapshot rejection mutated ledger")
	}
}

func TestRunSnapshotIdentityRequiresStartedEventAndCanonicalBindingsSnapshot(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(event *RunEvent, runID string)
	}{
		{
			name: "first event must be run started",
			mutate: func(event *RunEvent, _ string) {
				event.Type = RunEventNodeStarted
			},
		},
		{
			name: "bindings snapshot is required",
			mutate: func(event *RunEvent, _ string) {
				delete(event.Data, "bindingsSnapshot")
			},
		},
		{
			name: "bindings snapshot must be canonical",
			mutate: func(event *RunEvent, runID string) {
				event.Data["bindingsSnapshot"] = runArtifactPath("other-board", runID, ".bindings.toml")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _ := s4RunFixture(t)
			store.Now = fixedClock()
			runID := newPrefixedID("run")
			ledgerPath := filepath.Join(store.Workspace, runArtifactPath("session-search", runID, ".ndjson"))
			started := testRunStartedEvent(runID, "session-search")
			started.Data["bindingsSnapshot"] = runArtifactPath("session-search", runID, ".bindings.toml")
			test.mutate(&started, runID)
			ledgerBefore := testRunLedgerBytes(t, started)
			writeFixture(t, ledgerPath, string(ledgerBefore))

			err := store.AppendRunEvent(runID, RunEvent{Type: RunEventCanceled, Actor: "human:test"})
			if !errors.Is(err, ErrRunLedgerInvalid) {
				t.Fatalf("append with forged first event error = %v, want ErrRunLedgerInvalid", err)
			}
			if got := readFile(t, ledgerPath); got != string(ledgerBefore) {
				t.Fatalf("rejected first-event identity mutated ledger")
			}
		})
	}
}

func TestRunSnapshotReaderRejectsHardlinkBeforeAuthorizingAppend(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(workspace)
	store.Now = fixedClock()
	runID := newPrefixedID("run")
	snapshotPath := filepath.Join(workspace, runArtifactPath("session-search", runID, ".snapshot.toml"))
	ledgerPath := filepath.Join(workspace, runArtifactPath("session-search", runID, ".ndjson"))
	victimPath := filepath.Join(root, "snapshot-victim.toml")
	writeFixture(t, victimPath, s4MissionOnlyBoardFixture())
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o770); err != nil {
		t.Fatalf("create snapshot directory: %v", err)
	}
	if err := os.Link(victimPath, snapshotPath); err != nil {
		t.Fatalf("hardlink snapshot victim: %v", err)
	}
	started := testRunStartedEvent(runID, "session-search")
	started.Data["bindingsSnapshot"] = runArtifactPath("session-search", runID, ".bindings.toml")
	ledgerBefore := testRunLedgerBytes(t, started)
	writeFixture(t, ledgerPath, string(ledgerBefore))

	err := store.AppendRunEvent(runID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_work"})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("append with hardlinked snapshot error = %v, want ErrRunLedgerInvalid", err)
	}
	if got := readFile(t, ledgerPath); got != string(ledgerBefore) {
		t.Fatalf("hardlinked snapshot rejection mutated ledger")
	}
}

func TestSchema2RunLedgerNeverFallsThroughLegacyProjection(t *testing.T) {
	store, _ := s4RunFixture(t)
	runID := newPrefixedID("run")
	ledgerPath := filepath.Join(store.Workspace, runArtifactPath("session-search", runID, ".ndjson"))
	raw := `{"schema":2,"authoritySchema":2,"writerFence":1,"ts":"2026-07-18T12:00:00Z","runId":"` + runID + `","seq":1,"type":"run_started","actor":"agent:test","boardId":"brd_01J9_sesssearch","boardRev":7,"missionId":"mis_showcase","data":{"boardSlug":"session-search"}}` + "\n"
	writeFixture(t, ledgerPath, raw)

	if _, err := store.ReadRunEvents(runID); !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("schema-2 legacy read error = %v, want non-authorizing ErrRunLedgerInvalid", err)
	}
	if _, err := store.ReadRunEvents(runID); !errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
		t.Fatalf("schema-2 legacy read error = %v, want ErrRuntimeAuthorityNonAuthorizing", err)
	}
	if _, err := store.ProjectRun(runID); !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("schema-2 legacy projection error = %v, want non-authorizing ErrRunLedgerInvalid", err)
	}
}

func TestAuthorityShapedRunLedgersFailTypedNonAuthorizing(t *testing.T) {
	tests := []struct {
		name  string
		lines func(runID string) string
	}{
		{
			name: "future schema",
			lines: func(runID string) string {
				return `{"schema":3,"authoritySchema":2,"writerFence":1,"ts":"2026-07-18T12:00:00Z","runId":"` + runID + `","seq":1,"type":"run_started","actor":"agent:test"}` + "\n"
			},
		},
		{
			name: "mixed schema",
			lines: func(runID string) string {
				legacy := testRunLedgerBytes(t, testRunStartedEvent(runID, "session-search"))
				schema2 := `{"schema":2,"authoritySchema":2,"writerFence":1,"ts":"2026-07-18T12:00:01Z","runId":"` + runID + `","seq":2,"type":"run_failed","actor":"agent:test"}` + "\n"
				return string(legacy) + schema2
			},
		},
		{
			name: "unknown envelope key",
			lines: func(runID string) string {
				return `{"ts":"2026-07-18T12:00:00Z","runId":"` + runID + `","seq":1,"type":"run_started","actor":"agent:test","unknownAuthorityField":2}` + "\n"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _ := s4RunFixture(t)
			runID := newPrefixedID("run")
			ledgerPath := filepath.Join(store.Workspace, runArtifactPath("session-search", runID, ".ndjson"))
			writeFixture(t, ledgerPath, test.lines(runID))

			_, err := store.ReadRunEvents(runID)
			if !errors.Is(err, ErrRunLedgerInvalid) || !errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
				t.Fatalf("authority-shaped ledger error = %v, want invalid and typed non-authorizing", err)
			}
		})
	}
}

func TestRunEngineDoesNotTrustSnapshotPathInput(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	runID := newPrefixedID("run")
	snapshotPath := runArtifactPath("session-search", runID, ".snapshot.toml")
	outside := filepath.Join(filepath.Dir(store.Workspace), "snapshot-victim.toml")
	writeFixture(t, outside, s4MissionOnlyBoardFixture())
	if err := os.MkdirAll(filepath.Dir(filepath.Join(store.Workspace, snapshotPath)), 0o770); err != nil {
		t.Fatalf("create snapshot directory: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(store.Workspace, snapshotPath)); err != nil {
		t.Fatalf("symlink snapshot: %v", err)
	}

	if _, err := engine.readRunBoard(snapshotPath); !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("engine read with caller-supplied snapshot path error = %v, want ErrRunLedgerInvalid", err)
	}
}

func testLegacyLedgerBytes(t *testing.T, runID, boardSlug string) []byte {
	t.Helper()
	events := testLegacyRunEvents(runID, boardSlug)
	return testRunLedgerBytes(t, events...)
}

func testLegacyRunEvents(runID, boardSlug string) []RunEvent {
	started := testRunStartedEvent(runID, boardSlug)
	blocked := RunEvent{
		Timestamp: "2026-07-18T12:00:01Z",
		RunID:     runID,
		Seq:       2,
		Type:      RunEventBlocked,
		Actor:     "agent:test",
		BoardID:   "brd_01J9_sesssearch",
		BoardRev:  7,
		MissionID: "mis_showcase",
		Data: map[string]any{
			"reason": "interrupted", "code": "interrupted", "boundary": "engine", "blockedNodeId": "", "blockedGateId": "",
			"waitingNodes": []string{}, "recoverable": true, "resumeAllowed": true, "resumePolicy": "explicit",
			"openDispatches": []any{}, "nextEpoch": 1,
		},
	}
	return []RunEvent{started, blocked}
}

func testRunStartedEvent(runID, boardSlug string) RunEvent {
	return RunEvent{
		Timestamp: "2026-07-18T12:00:00Z",
		RunID:     runID,
		Seq:       1,
		Type:      RunEventStarted,
		Actor:     "agent:test",
		BoardID:   "brd_01J9_sesssearch",
		BoardRev:  7,
		MissionID: "mis_showcase",
		Data: map[string]any{
			"boardSlug":        boardSlug,
			"boardRev":         7,
			"missionId":        "mis_showcase",
			"beadId":           "ctx-test",
			"limits":           map[string]any{"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false},
			"snapshot":         runArtifactPath(boardSlug, runID, ".snapshot.toml"),
			"bindingsSnapshot": runArtifactPath(boardSlug, runID, ".bindings.toml"),
		},
	}
}

func testRunLedgerBytes(t *testing.T, events ...RunEvent) []byte {
	t.Helper()
	var builder strings.Builder
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal run event: %v", err)
		}
		builder.Write(raw)
		builder.WriteByte('\n')
	}
	return []byte(builder.String())
}
