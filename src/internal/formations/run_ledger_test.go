package formations

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestS4RunStartAppendsSeq1AndSnapshots(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()

	card, err := personas.CreatePersona(CreatePersonaRequest{
		ID:           "scout",
		DisplayName:  "Scout",
		Kind:         "specialist",
		Capabilities: []string{"research"},
		Harness:      "openai-codex",
	})
	if err != nil {
		t.Fatalf("create persona: %v", err)
	}
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	boardBefore := readFile(t, store.BoardPath("session-search"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
		Limits:            RunLimits{MaxDispatch: 3, WallClockSeconds: 120},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}

	if !strings.HasPrefix(started.RunID, "run_") {
		t.Fatalf("run id = %q, want run_ prefix", started.RunID)
	}
	if got := readFile(t, store.BoardPath("session-search")); got != boardBefore {
		t.Fatalf("run start rewrote current board definition:\n%s", got)
	}
	snapshot := readFile(t, filepath.Join(store.Workspace, started.SnapshotPath))
	if snapshot != boardBefore {
		t.Fatalf("snapshot did not preserve exact board bytes\n--- got ---\n%s\n--- want ---\n%s", snapshot, boardBefore)
	}
	bindings := readFile(t, filepath.Join(store.Workspace, started.BindingsSnapshotPath))
	for _, want := range []string{
		`agentId = "scout"`,
		`harness = "openai-codex"`,
		`sessionStem = "scout"`,
		`slotId = "slot_research"`,
		`cardPath = "` + filepath.ToSlash(personas.PersonaPath("scout")) + `"`,
		`cardSha256 = "` + testSHA256Hex([]byte(card.TOML)) + `"`,
	} {
		if !strings.Contains(bindings, want) {
			t.Fatalf("bindings snapshot missing %q:\n%s", want, bindings)
		}
	}

	events := readRunEvents(t, filepath.Join(store.Workspace, started.LedgerPath))
	if len(events) != 1 {
		t.Fatalf("ledger events = %d, want run_started only: %#v", len(events), events)
	}
	event := events[0]
	if event.Seq != 1 || event.Type != RunEventStarted || event.RunID != started.RunID || event.Actor != "agent:test" {
		t.Fatalf("run_started envelope = %+v, want seq 1 with run id and actor", event)
	}
	if event.BoardID != "brd_01J9_sesssearch" || event.BoardRev != 7 || event.MissionID != "mis_showcase" || event.BeadID != "home-7kc4.7" {
		t.Fatalf("run_started board/mission spine = %+v", event)
	}
	if got := event.Data["objective"]; got != "Ship a showcase" {
		t.Fatalf("run_started objective = %#v, want mission goal", got)
	}
	if got := event.Data["snapshot"]; got != started.SnapshotPath {
		t.Fatalf("run_started snapshot data = %#v, want %q", got, started.SnapshotPath)
	}
	limits, ok := event.Data["limits"].(map[string]any)
	if !ok || limits["maxDispatch"] != float64(3) || limits["wallClockSeconds"] != float64(120) {
		t.Fatalf("run_started limits = %#v, want maxDispatch and wallClockSeconds", event.Data["limits"])
	}
}

func TestRunStartFreezesSelectedCodeGateProfileBinding(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("output_contains", "LINT OK"))
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
		t.Fatalf("start run: %v", err)
	}
	descriptor, ok := LookupCodeGateProfileDescriptor("output_contains", "1")
	if !ok {
		t.Fatal("missing test profile descriptor")
	}
	bindings := readFile(t, filepath.Join(store.Workspace, started.BindingsSnapshotPath))
	for _, want := range []string{
		"[[gateBinding]]",
		`gateId = "gate_lint"`,
		`profileId = "output_contains"`,
		`profileVersion = "1"`,
		`profileSha256 = "` + descriptor.ProfileSHA256 + `"`,
		`evaluatorBundleSha256 = "` + descriptor.EvaluatorBundleSHA256 + `"`,
		`parameters = { value = "LINT OK" }`,
		`parametersSha256 = "` + codeGateSHA256(`{"value":"LINT OK"}`) + `"`,
		`policySha256 = "` + descriptor.PolicySHA256 + `"`,
		`determinismPolicySha256 = "` + descriptor.DeterminismPolicySHA256 + `"`,
		fmt.Sprintf("maxInputBytes = %d", descriptor.MaxInputBytes),
		fmt.Sprintf("maxResultBytes = %d", descriptor.MaxResultBytes),
		fmt.Sprintf("maxOperations = %d", descriptor.MaxOperations),
		`resultEncoding = "decision-result-jcs-v1"`,
	} {
		if !strings.Contains(bindings, want) {
			t.Fatalf("code Gate binding snapshot missing %q:\n%s", want, bindings)
		}
	}
}

func TestS4ProjectRunStatusFromLedgerOnly(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	if _, err := personas.CreatePersona(CreatePersonaRequest{ID: "scout", Kind: "specialist", Capabilities: []string{"research"}, Harness: "openai-codex"}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
		Limits:            RunLimits{MaxDispatch: 1, WallClockSeconds: 30},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := store.AppendRunEvent(started.RunID, RunEvent{
		Type:  RunEventSucceeded,
		Actor: "agent:test",
		Data: map[string]any{
			"summaryRef":   "run.refs/summary.md",
			"outputRefs":   []string{"run.refs/output.md"},
			"artifactRefs": []string{},
			"final":        true,
		},
	}); err != nil {
		t.Fatalf("append success: %v", err)
	}

	if err := os.Remove(store.BoardPath("session-search")); err != nil {
		t.Fatalf("remove current board after run start: %v", err)
	}
	if err := os.Remove(personas.PersonaPath("scout")); err != nil {
		t.Fatalf("remove current persona after run start: %v", err)
	}
	status, err := store.ProjectRun(started.RunID)
	if err != nil {
		t.Fatalf("project run after current files are gone: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want final succeeded", status)
	}
	if status.BoardRev != 7 || status.BoardSlug != "session-search" || status.MissionID != "mis_showcase" || status.EventCount != 2 {
		t.Fatalf("projected spine = %+v, want ledger-derived original board/mission state", status)
	}
}

func testSHA256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func TestS4RejectAppendAfterFinal(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	if _, err := personas.CreatePersona(CreatePersonaRequest{ID: "scout", Kind: "specialist", Capabilities: []string{"research"}, Harness: "openai-codex"}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := store.AppendRunEvent(started.RunID, RunEvent{Type: RunEventSucceeded, Actor: "agent:test", Data: map[string]any{"final": true}}); err != nil {
		t.Fatalf("append terminal event: %v", err)
	}
	if err := store.AppendRunEvent(started.RunID, RunEvent{Type: RunEventNodeStarted, Actor: "agent:test", NodeID: "fmn_research"}); !errors.Is(err, ErrRunFinal) {
		t.Fatalf("append after final error = %v, want ErrRunFinal", err)
	}

	events := readRunEvents(t, filepath.Join(store.Workspace, started.LedgerPath))
	if len(events) != 2 {
		t.Fatalf("append after final changed ledger length = %d, want 2", len(events))
	}
}

func s4RunFixture(t *testing.T) (*Store, *PersonaStore) {
	t.Helper()
	workspace := t.TempDir()
	agentsDir := filepath.Join(t.TempDir(), "agents")
	return NewStore(workspace), NewPersonaStore(agentsDir)
}

func s4RunBoardFixture() string {
	return `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Ship a showcase"
beadId = "home-7kc4.7"

[[formation]]
id = "fmn_research"
type = "solo"
title = "Research"

[[formation.input]]
id = "port_research_in"
label = "Input"

[[formation.output]]
id = "port_research_out"
label = "Output"

[[formation.slot]]
id = "slot_research"
label = "Researcher"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_research"
from = "mis_showcase:out"
to = "fmn_research:port_research_in"
`
}

func readRunEvents(t *testing.T, path string) []RunEvent {
	t.Helper()
	raw := strings.TrimSpace(readFile(t, path))
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	events := make([]RunEvent, 0, len(lines))
	for _, line := range lines {
		var event RunEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode run event %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}
