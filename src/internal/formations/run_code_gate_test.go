package formations

import (
	"reflect"
	"strings"
	"testing"
)

// codeGateLintBoardFixture wires mission -> work -> machine gate, with the gate
// pass route to ship and the fail route pushed back to work. The gate declares
// an explicit, operator-authored machine check (never the free-text criterion).
func codeGateLintBoardFixture(check, checkValue string) string {
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
id = "gate_lint"
title = "Lint"
kinds = ["code"]
criterion = "touch should-not-run"
check = "` + check + `"
checkValue = "` + checkValue + `"

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
to = "gate_lint:in"

[[connection]]
id = "edge_gate_pass_ship"
from = "gate_lint:pass"
to = "fmn_ship:port_ship_in"

[[connection]]
id = "edge_gate_fail_work"
from = "gate_lint:fail"
to = "fmn_work:port_work_in"
`
}

// attemptOutputExecutor returns different node output per attempt so a machine
// gate can be exercised through a real fail->revise->pass loop deterministically
// without any agent process.
type attemptOutputExecutor struct {
	calls         []FormationExecution
	textByAttempt map[string]map[int]string
}

func (e *attemptOutputExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	e.calls = append(e.calls, req)
	text := "output from " + req.NodeID
	if byAttempt, ok := e.textByAttempt[req.NodeID]; ok {
		if t, ok := byAttempt[req.Attempt]; ok {
			text = t
		}
	}
	return FormationExecutionResult{
		Status:    "done",
		ReportRef: "refs/" + req.NodeID + ".md",
		Text:      text,
		Outputs:   payloadsForFormationOutputs(req.Formation, text, "refs/"+req.NodeID+".md"),
	}, nil
}

func (e *attemptOutputExecutor) nodeIDs() []string {
	ids := make([]string, 0, len(e.calls))
	for _, call := range e.calls {
		ids = append(ids, call.NodeID)
	}
	return ids
}

// A machine (code) gate loops fail->revise->pass deterministically against an
// explicit output check, with no human and no agent process.
func TestCodeGateEvaluatorLoopsUntilLintPasses(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("output_contains", "LINT OK"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &attemptOutputExecutor{textByAttempt: map[string]map[int]string{
		"fmn_work": {
			1: "lint: 3 problems (3 errors, 0 warnings)",
			2: "lint clean — LINT OK",
		},
	}}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(NewCodeGateEvaluator())

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
		t.Fatalf("status = %+v, want succeeded after lint passes", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_work", "fmn_ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want two work attempts then ship", got)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdicts := gateVerdicts(events, "gate_lint")
	if got, want := verdicts, []string{"fail", "pass"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("gate verdicts = %v, want fail then pass", got)
	}
	first := firstGateVerdictEvent(t, events, "gate_lint")
	if reason, _ := first.Data["reason"].(string); !strings.Contains(reason, "output_contains") || !strings.Contains(reason, "LINT OK") {
		t.Fatalf("fail verdict reason = %q, want evidence citing the check and value", first.Data["reason"])
	}
}

// A machine gate with no explicit check must never fall back to the free-text
// criterion (FORMATIONS.md rule 9). It blocks loudly with gate_evaluator_error.
func TestCodeGateEvaluatorRequiresExplicitCheck(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	// No check/checkValue declared: only the free-text criterion is present.
	writeFixture(t, store.BoardPath("session-search"), strings.Replace(
		codeGateLintBoardFixture("output_contains", "LINT OK"),
		"check = \"output_contains\"\ncheckValue = \"LINT OK\"\n",
		"",
		1,
	))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	engine.SetGateEvaluator(NewCodeGateEvaluator())

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
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked without an explicit check", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "gate_evaluator_error" {
		t.Fatalf("error code = %#v, want gate_evaluator_error", errEvent.Data["code"])
	}
	for _, event := range events {
		if event.Type == RunEventGateVerdict {
			t.Fatalf("unexpected gate verdict %#v; missing check must not decide", event)
		}
	}
}

// An unknown check profile is a declaration error, not a silent pass.
func TestCodeGateEvaluatorUnknownProfileBlocks(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("no_such_profile", "LINT OK"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	engine.SetGateEvaluator(NewCodeGateEvaluator())

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
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked for unknown profile", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "gate_evaluator_error" {
		t.Fatalf("error code = %#v, want gate_evaluator_error", errEvent.Data["code"])
	}
}

// The output_absent profile passes only when a forbidden token is absent.
func TestCodeGateEvaluatorOutputAbsentProfile(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("output_absent", "error"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &attemptOutputExecutor{textByAttempt: map[string]map[int]string{
		"fmn_work": {
			1: "test run: 1 error remaining",
			2: "test run: all green",
		},
	}}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(NewCodeGateEvaluator())

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
		t.Fatalf("status = %+v, want succeeded once the forbidden token is absent", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if got, want := gateVerdicts(events, "gate_lint"), []string{"fail", "pass"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("gate verdicts = %v, want fail then pass", got)
	}
}

func gateVerdicts(events []RunEvent, gateID string) []string {
	var verdicts []string
	for _, event := range events {
		if event.Type == RunEventGateVerdict && event.GateID == gateID {
			if v, ok := event.Data["verdict"].(string); ok {
				verdicts = append(verdicts, v)
			}
		}
	}
	return verdicts
}

func firstGateVerdictEvent(t *testing.T, events []RunEvent, gateID string) RunEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == RunEventGateVerdict && event.GateID == gateID {
			return event
		}
	}
	t.Fatalf("no gate verdict for %s", gateID)
	return RunEvent{}
}
