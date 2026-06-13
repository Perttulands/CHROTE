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
	if got, want := executor.calls[1].Inputs[0].Text, "output from fmn_frame"; got != want {
		t.Fatalf("second formation routed input = %q, want %q", got, want)
	}
	if got, want := executor.calls[1].Inputs[0].FromPortID, "port_frame_out"; got != want {
		t.Fatalf("second formation fromPortID = %q, want %q", got, want)
	}
	if got, want := executor.calls[2].Inputs[0].Text, "output from fmn_research"; got != want {
		t.Fatalf("third formation routed input = %q, want %q", got, want)
	}
	if got, want := executor.calls[2].Inputs[0].FromPortID, "port_research_out"; got != want {
		t.Fatalf("third formation fromPortID = %q, want %q", got, want)
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
	joinCalls := callsByNode(executor.calls)["fmn_join"]
	if len(joinCalls) != 1 {
		t.Fatalf("join calls = %+v, want one joined dispatch", joinCalls)
	}
	if got := len(joinCalls[0].Inputs); got != 2 {
		t.Fatalf("join inputs = %d, want both required inputs", got)
	}
	assertRunInputRef(t, joinCalls[0].Inputs[0], RunInputRef{
		EdgeID:     "edge_a_join",
		FromNodeID: "fmn_a",
		FromPortID: "port_a_out",
		ToPortID:   "port_join_left",
		Text:       "output from fmn_a",
		ReportRef:  "refs/fmn_a.md",
	})
	assertRunInputRef(t, joinCalls[0].Inputs[1], RunInputRef{
		EdgeID:     "edge_b_join",
		FromNodeID: "fmn_b",
		FromPortID: "port_b_out",
		ToPortID:   "port_join_right",
		Text:       "output from fmn_b",
		ReportRef:  "refs/fmn_b.md",
	})

	started := findNodeStartedEvent(t, events, "fmn_join")
	inputRefs, ok := started.Data["inputRefs"].([]any)
	if !ok || len(inputRefs) != 2 {
		t.Fatalf("join node_started inputRefs = %#v, want two refs", started.Data["inputRefs"])
	}
	assertRunInputRef(t, runInputRefFromAny(inputRefs[0]), joinCalls[0].Inputs[0])
	assertRunInputRef(t, runInputRefFromAny(inputRefs[1]), joinCalls[0].Inputs[1])
}

func TestS4NamedOutputPortsRouteDistinctPayloads(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4NamedOutputBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{
		outputsByPort: map[string]map[string]FormationOutputPayload{
			"fmn_split": {
				"port_split_left":  {Text: "LEFT-PAYLOAD"},
				"port_split_right": {Text: "RIGHT-PAYLOAD"},
			},
		},
	}
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

	calls := callsByNode(executor.calls)
	leftInputs := calls["fmn_left"][0].Inputs
	if got, want := leftInputs[0].Text, "LEFT-PAYLOAD"; got != want {
		t.Fatalf("left input text = %q, want %q", got, want)
	}
	if got, want := leftInputs[0].FromPortID, "port_split_left"; got != want {
		t.Fatalf("left input fromPortID = %q, want %q", got, want)
	}
	rightInputs := calls["fmn_right"][0].Inputs
	if got, want := rightInputs[0].Text, "RIGHT-PAYLOAD"; got != want {
		t.Fatalf("right input text = %q, want %q", got, want)
	}
	if got, want := rightInputs[0].FromPortID, "port_split_right"; got != want {
		t.Fatalf("right input fromPortID = %q, want %q", got, want)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	outputEvent := findNodeOutputEvent(t, events, "fmn_split")
	outputs, ok := outputEvent.Data["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("split node_output outputs = %#v, want map", outputEvent.Data["outputs"])
	}
	assertOutputPayloadText(t, outputs, "port_split_left", "LEFT-PAYLOAD")
	assertOutputPayloadText(t, outputs, "port_split_right", "RIGHT-PAYLOAD")
}

func TestS4OneNamedOutputFansOutToMultipleInputs(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4FanOutBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{
		outputsByPort: map[string]map[string]FormationOutputPayload{
			"fmn_split": {
				"port_split_left":  {Text: "LEFT-PAYLOAD", ReportRef: "refs/split-left.md"},
				"port_split_right": {Text: "RIGHT-PAYLOAD", ReportRef: "refs/split-right.md"},
			},
		},
	}
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

	calls := callsByNode(executor.calls)
	leftInputs := calls["fmn_left"][0].Inputs
	assertRunInputRef(t, leftInputs[0], RunInputRef{
		EdgeID:     "edge_split_left",
		FromNodeID: "fmn_split",
		FromPortID: "port_split_left",
		ToPortID:   "port_left_in",
		Text:       "LEFT-PAYLOAD",
		ReportRef:  "refs/split-left.md",
	})
	rightInputs := calls["fmn_right"][0].Inputs
	assertRunInputRef(t, rightInputs[0], RunInputRef{
		EdgeID:     "edge_split_left_fanout",
		FromNodeID: "fmn_split",
		FromPortID: "port_split_left",
		ToPortID:   "port_right_in",
		Text:       "LEFT-PAYLOAD",
		ReportRef:  "refs/split-left.md",
	})

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	leftStarted := findNodeStartedEvent(t, events, "fmn_left")
	leftRefs, ok := leftStarted.Data["inputRefs"].([]any)
	if !ok || len(leftRefs) != 1 {
		t.Fatalf("left node_started inputRefs = %#v, want one ref", leftStarted.Data["inputRefs"])
	}
	assertRunInputRef(t, runInputRefFromAny(leftRefs[0]), leftInputs[0])
	rightStarted := findNodeStartedEvent(t, events, "fmn_right")
	rightRefs, ok := rightStarted.Data["inputRefs"].([]any)
	if !ok || len(rightRefs) != 1 {
		t.Fatalf("right node_started inputRefs = %#v, want one ref", rightStarted.Data["inputRefs"])
	}
	assertRunInputRef(t, runInputRefFromAny(rightRefs[0]), rightInputs[0])
}

func TestS4MissingNamedOutputPayloadBlocksInsteadOfBroadcasting(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4NamedOutputBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{
		outputsByPort: map[string]map[string]FormationOutputPayload{
			"fmn_split": {
				"port_split_left": {Text: "LEFT-PAYLOAD"},
			},
		},
	}
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
	if status.Status != RunStatusBlocked || status.Final {
		t.Fatalf("status = %+v, want blocked non-final", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errorAt := indexEvent(events, RunEventError, "fmn_split")
	if errorAt == -1 || events[errorAt].Data["code"] != "missing_output_payload" {
		t.Fatalf("missing output error event = %#v", events)
	}
}

func TestS4SingleOutputWithoutNamedPayloadBlocksInsteadOfBroadcastingText(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4CascadeBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{disableDefaultOutputs: map[string]bool{"fmn_frame": true}}
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
	if status.Status != RunStatusBlocked || status.Final {
		t.Fatalf("status = %+v, want blocked non-final", status)
	}
	if got := executor.nodeIDs(); !reflect.DeepEqual(got, []string{"fmn_frame"}) {
		t.Fatalf("executor nodes = %v, want only text-only producer to run", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errorAt := indexEvent(events, RunEventError, "fmn_frame")
	if errorAt == -1 || events[errorAt].Data["code"] != "missing_output_payload" {
		t.Fatalf("missing output error event = %#v", events)
	}
}

type fakeRunExecutor struct {
	calls                 []FormationExecution
	outputs               map[string]string
	outputsByPort         map[string]map[string]FormationOutputPayload
	disableDefaultOutputs map[string]bool
}

func (f *fakeRunExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	f.calls = append(f.calls, req)
	text := "output from " + req.NodeID
	if f.outputs != nil && f.outputs[req.NodeID] != "" {
		text = f.outputs[req.NodeID]
	}
	outputs := f.outputsByPort[req.NodeID]
	if outputs == nil && !f.disableDefaultOutputs[req.NodeID] {
		outputs = payloadsForFormationOutputs(req.Formation, text, "refs/"+req.NodeID+".md")
	}
	return FormationExecutionResult{
		Status:    "done",
		ReportRef: "refs/" + req.NodeID + ".md",
		Text:      text,
		Outputs:   outputs,
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

func callsByNode(calls []FormationExecution) map[string][]FormationExecution {
	byNode := map[string][]FormationExecution{}
	for _, call := range calls {
		byNode[call.NodeID] = append(byNode[call.NodeID], call)
	}
	return byNode
}

func findNodeOutputEvent(t *testing.T, events []RunEvent, nodeID string) RunEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == RunEventNodeOutput && event.NodeID == nodeID {
			return event
		}
	}
	t.Fatalf("missing node_output for %s", nodeID)
	return RunEvent{}
}

func findNodeStartedEvent(t *testing.T, events []RunEvent, nodeID string) RunEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == RunEventNodeStarted && event.NodeID == nodeID {
			return event
		}
	}
	t.Fatalf("missing node_started for %s", nodeID)
	return RunEvent{}
}

func assertRunInputRef(t *testing.T, got RunInputRef, want RunInputRef) {
	t.Helper()
	if got.EdgeID != want.EdgeID ||
		got.FromNodeID != want.FromNodeID ||
		got.FromPortID != want.FromPortID ||
		got.ToPortID != want.ToPortID ||
		got.Text != want.Text ||
		got.ReportRef != want.ReportRef ||
		got.ArtifactRef != want.ArtifactRef {
		t.Fatalf("input ref = %+v, want %+v", got, want)
	}
	if got.Ref == "" {
		t.Fatalf("input ref = %+v, want non-empty provenance ref", got)
	}
}

func assertOutputPayloadText(t *testing.T, outputs map[string]any, portID, want string) {
	t.Helper()
	raw, ok := outputs[portID].(map[string]any)
	if !ok {
		t.Fatalf("output %s = %#v, want payload map", portID, outputs[portID])
	}
	if got := raw["text"]; got != want {
		t.Fatalf("output %s text = %#v, want %q", portID, got, want)
	}
}

func payloadsForFormationOutputs(formation FormationNode, text, reportRef string) map[string]FormationOutputPayload {
	outputs := make(map[string]FormationOutputPayload, len(formation.Outputs))
	for _, port := range formation.Outputs {
		outputs[port.ID] = FormationOutputPayload{Text: text, ReportRef: reportRef}
	}
	return outputs
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

func s4NamedOutputBoardFixture() string {
	return s4MissionOnlyBoardFixture() + `
[[formation]]
id = "fmn_split"
type = "solo"
title = "Split"

[[formation.input]]
id = "port_split_in"
label = "Input"

[[formation.output]]
id = "port_split_left"
label = "Left"

[[formation.output]]
id = "port_split_right"
label = "Right"

[[formation.slot]]
id = "slot_split"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[formation]]
id = "fmn_left"
type = "solo"
title = "Left consumer"

[[formation.input]]
id = "port_left_in"
label = "Input"

[[formation.output]]
id = "port_left_out"
label = "Output"

[[formation.slot]]
id = "slot_left"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[formation]]
id = "fmn_right"
type = "solo"
title = "Right consumer"

[[formation.input]]
id = "port_right_in"
label = "Input"

[[formation.output]]
id = "port_right_out"
label = "Output"

[[formation.slot]]
id = "slot_right"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_split"
from = "mis_showcase:out"
to = "fmn_split:port_split_in"

[[connection]]
id = "edge_split_left"
from = "fmn_split:port_split_left"
to = "fmn_left:port_left_in"

[[connection]]
id = "edge_split_right"
from = "fmn_split:port_split_right"
to = "fmn_right:port_right_in"
`
}

func s4FanOutBoardFixture() string {
	return s4MissionOnlyBoardFixture() + `
[[formation]]
id = "fmn_split"
type = "solo"
title = "Split"

[[formation.input]]
id = "port_split_in"
label = "Input"

[[formation.output]]
id = "port_split_left"
label = "Left"

[[formation.output]]
id = "port_split_right"
label = "Right"

[[formation.slot]]
id = "slot_split"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[formation]]
id = "fmn_left"
type = "solo"
title = "Left consumer"

[[formation.input]]
id = "port_left_in"
label = "Input"

[[formation.output]]
id = "port_left_out"
label = "Output"

[[formation.slot]]
id = "slot_left"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[formation]]
id = "fmn_right"
type = "solo"
title = "Right consumer"

[[formation.input]]
id = "port_right_in"
label = "Input"

[[formation.output]]
id = "port_right_out"
label = "Output"

[[formation.slot]]
id = "slot_right"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_split"
from = "mis_showcase:out"
to = "fmn_split:port_split_in"

[[connection]]
id = "edge_split_left"
from = "fmn_split:port_split_left"
to = "fmn_left:port_left_in"

[[connection]]
id = "edge_split_left_fanout"
from = "fmn_split:port_split_left"
to = "fmn_right:port_right_in"
`
}
