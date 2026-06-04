package formations

import (
	"reflect"
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

func TestS4FormationVerificationBlockAndPushback(t *testing.T) {
	t.Run("block stops before downstream formation", func(t *testing.T) {
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
		engine.SetVerificationEvaluator(&fakeVerificationEvaluator{verdicts: []string{"fail"}})

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
		if status.Status != RunStatusBlocked {
			t.Fatalf("status = %+v, want blocked by verification", status)
		}
		if got, want := executor.nodeIDs(), []string{"fmn_work"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("executor nodes = %v, want downstream formation skipped", got)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
		verdict := eventOfType(t, events, RunEventVerificationVerdict)
		if verdict.Data["verdict"] != "fail" || verdict.Data["onFail"] != "block" {
			t.Fatalf("verification verdict = %+v, want fail/block", verdict)
		}
		if events[len(events)-1].Type != RunEventBlocked {
			t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
		}
	})

	t.Run("pushback retries own formation until attempt limit", func(t *testing.T) {
		store, personas := s4RunFixture(t)
		store.Now = fixedClock()
		personas.Now = fixedClock()
		createS4Persona(t, personas, "scout")
		writeFixture(t, store.BoardPath("session-search"), s4VerificationBoardFixture("pushback"))
		board, err := store.ReadBoard("session-search")
		if err != nil {
			t.Fatalf("read board: %v", err)
		}
		executor := &fakeRunExecutor{}
		engine := NewRunEngine(store, personas, executor)
		engine.SetVerificationEvaluator(&fakeVerificationEvaluator{verdicts: []string{"fail", "fail"}})

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
		if status.Status != RunStatusBlocked {
			t.Fatalf("status = %+v, want blocked after verification pushback exhaustion", status)
		}
		if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_work"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("executor nodes = %v, want two own-formation attempts", got)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
		errEvent := eventOfType(t, events, RunEventError)
		if errEvent.Data["reason"] != "verification pushback exhausted" {
			t.Fatalf("error data = %#v, want verification pushback exhausted", errEvent.Data)
		}
	})
}

type fakeVerificationEvaluator struct {
	verdicts []string
	calls    []VerificationEvaluation
}

func (f *fakeVerificationEvaluator) EvaluateVerification(req VerificationEvaluation) (VerificationEvaluationResult, error) {
	f.calls = append(f.calls, req)
	verdict := "pass"
	if len(f.verdicts) > 0 {
		verdict = f.verdicts[0]
		f.verdicts = f.verdicts[1:]
	}
	return VerificationEvaluationResult{Verdict: verdict, Feedback: "fake " + verdict}, nil
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
