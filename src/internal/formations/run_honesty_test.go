package formations

import (
	"reflect"
	"testing"
)

// TestS4ReachableJoinWithUnavailableInputDoesNotFalselySucceed pins the run-state
// honesty contract: a mission run must never project run_succeeded while a
// reachable required formation is still node_waiting because one of its inputs
// has no producer. Here fmn_join requires {left,right} but only left is wired,
// so the join is reached (left delivered) yet can never run. WHY it matters: a
// false run_succeeded tells the operator/CLI/API/UI the work is done when a
// required downstream step never executed, hiding unfinished work.
func TestS4ReachableJoinWithUnavailableInputDoesNotFalselySucceed(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4DanglingJoinBoardFixture())
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

	// Primary honesty assertion: the run must NOT report success.
	if status.Status == RunStatusSucceeded {
		t.Fatalf("status = %+v, want NOT succeeded while fmn_join is still node_waiting", status)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %q, want blocked when a reachable join can never become ready", status.Status)
	}
	if status.Final {
		t.Fatalf("status.Final = true, want non-final blocked run that surfaces unfinished work")
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))

	// The join must be on record as waiting, and that wait must be unresolved.
	waitingAt := indexEvent(events, RunEventNodeWaiting, "fmn_join")
	if waitingAt == -1 {
		t.Fatalf("missing node_waiting for fmn_join: %#v", events)
	}
	if joinStartedAt := indexEvent(events, RunEventNodeStarted, "fmn_join"); joinStartedAt != -1 {
		t.Fatalf("fmn_join started at %d but its right input has no producer; it must never run", joinStartedAt)
	}

	// The ledger evidence itself must be honest: no run_succeeded was emitted.
	if idx := indexEventType(events, RunEventSucceeded); idx != -1 {
		t.Fatalf("ledger emitted run_succeeded at %d while fmn_join is unresolved-waiting", idx)
	}
	if last := events[len(events)-1].Type; last != RunEventBlocked {
		t.Fatalf("last event = %s, want run_blocked", last)
	}

	// Only the reachable, fully-fed formation ran.
	if got, want := executor.nodeIDs(), []string{"fmn_a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want only fmn_a (join never runnable)", got)
	}
}

func indexEventType(events []RunEvent, eventType string) int {
	for i, event := range events {
		if event.Type == eventType {
			return i
		}
	}
	return -1
}

// s4DanglingJoinBoardFixture wires mission -> fmn_a -> fmn_join:left, but
// fmn_join also requires port_join_right which has no incoming connection, so
// the join is reachable yet permanently starved of a required input.
func s4DanglingJoinBoardFixture() string {
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
id = "edge_a_join"
from = "fmn_a:port_a_out"
to = "fmn_join:port_join_left"
`
}
