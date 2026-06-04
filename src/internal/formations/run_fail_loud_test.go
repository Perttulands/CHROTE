package formations

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunEngineMissingExecutorRecordsBlockedRun(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4CascadeBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, nil)

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
	if status.Status != RunStatusBlocked || !status.ResumeAllowed {
		t.Fatalf("status = %+v, want blocked resumable run when executor is missing", status)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "missing_executor" {
		t.Fatalf("run_error data = %#v, want missing_executor", errEvent.Data)
	}
	if events[len(events)-1].Type != RunEventBlocked {
		t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
	}
	if containsFakeOutput(events) {
		t.Fatalf("events contain fake executor output: %#v", events)
	}
}

func TestRunEngineMissingGateEvaluatorBlocksInsteadOfPassing(t *testing.T) {
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
		t.Fatalf("status = %+v, want blocked when gate evaluator is missing", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want only pre-gate work", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "missing_gate_evaluator" {
		t.Fatalf("run_error data = %#v, want missing_gate_evaluator", errEvent.Data)
	}
	if eventsContainType(events, RunEventGateVerdict) {
		t.Fatalf("events include gate verdict despite missing evaluator: %#v", events)
	}
	if events[len(events)-1].Type != RunEventBlocked {
		t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
	}
}

func TestRunEngineMissingVerificationEvaluatorBlocksInsteadOfPassing(t *testing.T) {
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
		t.Fatalf("status = %+v, want blocked when verification evaluator is missing", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want only verified formation attempt", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "missing_verification_evaluator" {
		t.Fatalf("run_error data = %#v, want missing_verification_evaluator", errEvent.Data)
	}
	if eventsContainType(events, RunEventVerificationVerdict) {
		t.Fatalf("events include verification verdict despite missing evaluator: %#v", events)
	}
	if events[len(events)-1].Type != RunEventBlocked {
		t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
	}
}

func TestRunEngineWallClockLimitBlocksSlowExecutor(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4CascadeBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &slowRunExecutor{delay: 1500 * time.Millisecond}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2, WallClockSeconds: 1},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked by wall-clock timeout", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_frame"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want only timed-out first dispatch", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "wall_clock_exceeded" {
		t.Fatalf("run_error data = %#v, want wall_clock_exceeded", errEvent.Data)
	}
	if events[len(events)-1].Type != RunEventBlocked {
		t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
	}
}

type slowRunExecutor struct {
	delay time.Duration
	calls []FormationExecution
}

func (f *slowRunExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	f.calls = append(f.calls, req)
	time.Sleep(f.delay)
	return FormationExecutionResult{
		Status:    "done",
		ReportRef: "refs/" + req.NodeID + ".md",
		Text:      "slow output from " + req.NodeID,
	}, nil
}

func (f *slowRunExecutor) nodeIDs() []string {
	ids := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		ids = append(ids, call.NodeID)
	}
	return ids
}

func containsFakeOutput(events []RunEvent) bool {
	for _, event := range events {
		if event.Type != RunEventNodeOutput || event.Data == nil {
			continue
		}
		if text, ok := event.Data["text"].(string); ok && strings.HasPrefix(text, "output from ") {
			return true
		}
	}
	return false
}

func eventsContainType(events []RunEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
