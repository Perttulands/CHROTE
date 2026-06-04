package formations

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestS4MissionRunCascadesReachableChain(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4CascadeBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 10, WallClockSeconds: 60},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want final succeeded", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_frame", "fmn_research", "fmn_ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want %v", got, want)
	}
	if got := executor.calls[0].Inputs[0].Text; got != "Ship a showcase" {
		t.Fatalf("first formation seed input = %q, want mission objective", got)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	startedNodes := eventNodeOrder(events, RunEventNodeStarted)
	if got, want := startedNodes, []string{"mis_showcase", "fmn_frame", "fmn_research", "fmn_ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("node_started order = %v, want %v", got, want)
	}
	if events[len(events)-1].Type != RunEventSucceeded {
		t.Fatalf("last event = %s, want run_succeeded", events[len(events)-1].Type)
	}
}

func TestS4MissionWithoutOutgoingWireFailsBeforeRun(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4MissionOnlyBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})

	_, err = engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
	})
	if err == nil || !strings.Contains(err.Error(), "wire the mission to a step") {
		t.Fatalf("run mission without outgoing wire error = %v, want wire-the-mission failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(store.Workspace, ".formations", "runs", "session-search")); !os.IsNotExist(statErr) {
		t.Fatalf("runs directory error = %v, want no run artifacts when mission cannot start", statErr)
	}
}

func TestS4JoinWaitsUntilAllInputsReady(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4JoinBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	if _, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 10, WallClockSeconds: 60},
	}); err != nil {
		t.Fatalf("run mission: %v", err)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	waitingAt := indexEvent(events, RunEventNodeWaiting, "fmn_join")
	if waitingAt == -1 {
		t.Fatalf("missing node_waiting for join: %#v", events)
	}
	waiting := events[waitingAt]
	if waiting.Data["readyInputs"] != float64(1) || waiting.Data["totalInputs"] != float64(2) {
		t.Fatalf("join waiting data = %#v, want ready 1/2", waiting.Data)
	}
	joinStartedAt := indexEvent(events, RunEventNodeStarted, "fmn_join")
	if joinStartedAt == -1 || joinStartedAt < waitingAt {
		t.Fatalf("join start index = %d, waiting index = %d; want start after waiting", joinStartedAt, waitingAt)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_a", "fmn_b", "fmn_join"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want %v", got, want)
	}
}

type fakeRunExecutor struct {
	calls   []FormationExecution
	outputs map[string]string
}

func (f *fakeRunExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	f.calls = append(f.calls, req)
	text := "output from " + req.NodeID
	if f.outputs != nil && f.outputs[req.NodeID] != "" {
		text = f.outputs[req.NodeID]
	}
	return FormationExecutionResult{
		Status:    "done",
		ReportRef: "refs/" + req.NodeID + ".md",
		Text:      text,
	}, nil
}

func (f *fakeRunExecutor) nodeIDs() []string {
	ids := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		ids = append(ids, call.NodeID)
	}
	return ids
}

func createS4Persona(t *testing.T, personas *PersonaStore, id string) {
	t.Helper()
	if _, err := personas.CreatePersona(CreatePersonaRequest{
		ID:           id,
		Kind:         "specialist",
		Capabilities: []string{"research"},
		Harness:      "openai-codex",
	}); err != nil {
		t.Fatalf("create persona %s: %v", id, err)
	}
}

func findOnlyRunLedger(t *testing.T, store *Store, slug string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(store.Workspace, ".formations", "runs", slug, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob run ledger: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("run ledgers = %v, want exactly one", matches)
	}
	return matches[0]
}

func eventNodeOrder(events []RunEvent, eventType string) []string {
	var order []string
	for _, event := range events {
		if event.Type == eventType {
			order = append(order, event.NodeID)
		}
	}
	return order
}

func indexEvent(events []RunEvent, eventType, nodeID string) int {
	for i, event := range events {
		if event.Type == eventType && event.NodeID == nodeID {
			return i
		}
	}
	return -1
}

func s4MissionOnlyBoardFixture() string {
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
`
}

func s4CascadeBoardFixture() string {
	return s4MissionOnlyBoardFixture() + `
[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

[[formation.input]]
id = "port_frame_in"
label = "Input"

[[formation.output]]
id = "port_frame_out"
label = "Output"

[[formation.slot]]
id = "slot_frame"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

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
label = "Worker"
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
id = "edge_mission_frame"
from = "mis_showcase:out"
to = "fmn_frame:port_frame_in"

[[connection]]
id = "edge_frame_research"
from = "fmn_frame:port_frame_out"
to = "fmn_research:port_research_in"

[[connection]]
id = "edge_research_ship"
from = "fmn_research:port_research_out"
to = "fmn_ship:port_ship_in"
`
}

func s4JoinBoardFixture() string {
	return s4MissionOnlyBoardFixture() + `
[[formation]]
id = "fmn_a"
type = "solo"
title = "A"

[[formation.input]]
id = "port_a_in"
label = "Input"

[[formation.output]]
id = "port_a_out"
label = "Output"

[[formation.slot]]
id = "slot_a"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[formation]]
id = "fmn_b"
type = "solo"
title = "B"

[[formation.input]]
id = "port_b_in"
label = "Input"

[[formation.output]]
id = "port_b_out"
label = "Output"

[[formation.slot]]
id = "slot_b"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[formation]]
id = "fmn_join"
type = "solo"
title = "Join"

[[formation.input]]
id = "port_join_left"
label = "Left"

[[formation.input]]
id = "port_join_right"
label = "Right"

[[formation.output]]
id = "port_join_out"
label = "Output"

[[formation.slot]]
id = "slot_join"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_a"
from = "mis_showcase:out"
to = "fmn_a:port_a_in"

[[connection]]
id = "edge_mission_b"
from = "mis_showcase:out"
to = "fmn_b:port_b_in"

[[connection]]
id = "edge_a_join"
from = "fmn_a:port_a_out"
to = "fmn_join:port_join_left"

[[connection]]
id = "edge_b_join"
from = "fmn_b:port_b_out"
to = "fmn_join:port_join_right"
`
}
