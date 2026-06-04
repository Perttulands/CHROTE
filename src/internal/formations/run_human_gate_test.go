package formations

import (
	"reflect"
	"testing"
)

func TestS5HumanGateRequestsInputAndWaits(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s5HumanGateBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"pass"}})

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusRunning || status.Final {
		t.Fatalf("status = %+v, want running non-final while waiting for human verdict", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want work only before human verdict", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	request := eventOfType(t, events, RunEventHumanInputRequested)
	if request.GateID != "gate_review" || request.NodeID != "gate_review" {
		t.Fatalf("human request envelope = %+v, want gate_review", request)
	}
	if request.Data["prompt"] != "Good enough to ship" || request.Data["requestedBy"] != "gate_review" {
		t.Fatalf("human request data = %#v, want gate criterion prompt", request.Data)
	}
	for _, event := range events {
		if event.Type == RunEventGateVerdict {
			t.Fatalf("gate verdict recorded before human decision: %+v", event)
		}
	}
}

func TestS5HumanGateVerdictRoutesPassWire(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s5HumanGateBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"pass"}})

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	executor.calls = nil
	status, err = engine.RecordHumanGateVerdict(status.RunID, HumanGateVerdictRequest{
		GateID:  "gate_review",
		Verdict: "pass",
		Reason:  "direction is right",
		Actor:   "human:perttu",
	})
	if err != nil {
		t.Fatalf("record human verdict: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded after pass verdict routes downstream", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes after verdict = %v, want only downstream ship", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdict := eventOfType(t, events, RunEventHumanVerdictRecorded)
	if verdict.GateID != "gate_review" || verdict.Data["verdict"] != "pass" || verdict.Data["reason"] != "direction is right" || verdict.Data["decidedBy"] != "human:perttu" {
		t.Fatalf("human verdict event = %+v, want pass reason/actor", verdict)
	}
	gateVerdict := eventOfType(t, events, RunEventGateVerdict)
	if gateVerdict.Data["verdict"] != "pass" || gateVerdict.Data["routePort"] != "pass" {
		t.Fatalf("gate verdict = %+v, want human pass routed through pass wire", gateVerdict)
	}
}

func s5HumanGateBoardFixture() string {
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
kinds = ["code", "human"]
criterion = "Good enough to ship"

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
id = "edge_gate_pass_ship"
from = "gate_review:pass"
to = "fmn_ship:port_ship_in"
`
}
