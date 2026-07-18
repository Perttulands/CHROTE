package formations

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		Data:      map[string]any{"reason": "interrupted", "resumeAllowed": true},
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
			"boardSlug": boardSlug,
			"snapshot":  runArtifactPath(boardSlug, runID, ".snapshot.toml"),
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
