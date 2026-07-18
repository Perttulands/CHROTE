package formations

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestS4GateRoutesPassAndUnwiredFailBlocks(t *testing.T) {
	t.Run("pass routes through pass wire", func(t *testing.T) {
		store, personas := s4RunFixture(t)
		store.Now = fixedClock()
		personas.Now = fixedClock()
		createS4Persona(t, personas, "scout")
		writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
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
		if status.Status != RunStatusSucceeded {
			t.Fatalf("status = %+v, want succeeded", status)
		}
		if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_ship"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("executor nodes = %v, want %v", got, want)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
		verdict := eventOfType(t, events, RunEventGateVerdict)
		if verdict.GateID != "gate_review" || verdict.Data["verdict"] != "pass" || verdict.Data["routePort"] != "pass" {
			t.Fatalf("gate verdict = %+v, want pass through pass route", verdict)
		}
	})

	t.Run("unwired fail records run_blocked", func(t *testing.T) {
		store, personas := s4RunFixture(t)
		store.Now = fixedClock()
		personas.Now = fixedClock()
		createS4Persona(t, personas, "scout")
		writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
		board, err := store.ReadBoard("session-search")
		if err != nil {
			t.Fatalf("read board: %v", err)
		}
		executor := &fakeRunExecutor{}
		engine := NewRunEngine(store, personas, executor)
		engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"fail"}})

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
		if status.Status != RunStatusBlocked || status.Final {
			t.Fatalf("status = %+v, want blocked non-final", status)
		}
		if got, want := executor.nodeIDs(), []string{"fmn_work"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("executor nodes = %v, want only pre-gate work", got)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
		if events[len(events)-1].Type != RunEventBlocked {
			t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
		}
		verdict := eventOfType(t, events, RunEventGateVerdict)
		if verdict.Data["verdict"] != "fail" || verdict.Data["routePort"] != "none" {
			t.Fatalf("gate verdict = %+v, want unwired fail route none", verdict)
		}
	})
}

func TestS4GateFailWirePushesBackWithAttemptLimit(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(true))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"fail", "fail"}})

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
		t.Fatalf("status = %+v, want blocked after revise exhaustion", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_work"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want two work attempts", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	attempts := nodeStartedAttempts(events, "fmn_work")
	if !reflect.DeepEqual(attempts, []int{1, 2}) {
		t.Fatalf("work attempts = %v, want [1 2]", attempts)
	}
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["reason"] != "revise loop exhausted" {
		t.Fatalf("error data = %#v, want revise loop exhausted", errEvent.Data)
	}
}

func TestS4GateEvaluationRejectsPersistedCommandArgvBeforeEvaluator(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), strings.Replace(s4GateBoardFixture(false), `criterion = "Good enough to ship"`, `criterion = "touch should-not-run"`+"\n"+`commandArgv = ["npm", "run", "lint"]`+"\n"+`commandCwd = "dashboard"`, 1))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	evaluator := &fakeGateEvaluator{verdicts: []string{"pass"}}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	engine.SetGateEvaluator(evaluator)

	_, err = engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if !errors.Is(err, ErrLegacyScriptGateRequiresFencedMigration) {
		t.Fatalf("run mission error = %v, want ErrLegacyScriptGateRequiresFencedMigration", err)
	}
	if len(evaluator.calls) != 0 {
		t.Fatalf("gate calls = %+v, want none before migration", evaluator.calls)
	}
	assertNoRunArtifacts(t, store, "session-search")
}

func TestS4RunLimitsRecordAndStop(t *testing.T) {
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
		Limits:            RunLimits{MaxDispatch: 1},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked when max dispatch is exceeded", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_frame"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want only first dispatch before limit", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "max_dispatch_exceeded" {
		t.Fatalf("error data = %#v, want max_dispatch_exceeded", errEvent.Data)
	}
	if events[len(events)-1].Type != RunEventBlocked {
		t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
	}
}

type fakeGateEvaluator struct {
	verdicts []string
	calls    []GateEvaluation
}

func (f *fakeGateEvaluator) EvaluateGate(req GateEvaluation) (GateEvaluationResult, error) {
	f.calls = append(f.calls, req)
	verdict := "pass"
	if len(f.verdicts) > 0 {
		verdict = f.verdicts[0]
		f.verdicts = f.verdicts[1:]
	}
	return GateEvaluationResult{Verdict: verdict, Reason: "fake " + verdict}, nil
}

func eventOfType(t *testing.T, events []RunEvent, eventType string) RunEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == eventType {
			return event
		}
	}
	t.Fatalf("missing event type %s in %#v", eventType, events)
	return RunEvent{}
}

func nodeStartedAttempts(events []RunEvent, nodeID string) []int {
	var attempts []int
	for _, event := range events {
		if event.Type == RunEventNodeStarted && event.NodeID == nodeID {
			attempts = append(attempts, event.Attempt)
		}
	}
	return attempts
}

func s4GateBoardFixture(pushback bool) string {
	failWire := ""
	if pushback {
		failWire = `
[[connection]]
id = "edge_gate_fail_work"
from = "gate_review:fail"
to = "fmn_work:port_work_in"
`
	}
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
kinds = ["code"]
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
` + failWire
}
