package formations

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestS5ResumeAppendsRunResumedInNextEpoch(t *testing.T) {
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
	if err := store.AppendRunEvent(started.RunID, RunEvent{
		Type:   RunEventBlocked,
		Actor:  "agent:test",
		NodeID: "fmn_research",
		Data: map[string]any{
			"reason":         "awaiting operator resume",
			"resumeAllowed":  true,
			"resumePolicy":   "explicit",
			"openDispatches": []string{"dispatch_01"},
		},
	}); err != nil {
		t.Fatalf("append blocked: %v", err)
	}

	blocked, err := store.ProjectRun(started.RunID)
	if err != nil {
		t.Fatalf("project blocked run: %v", err)
	}
	if blocked.Status != RunStatusBlocked || blocked.Final || !blocked.ResumeAllowed || blocked.Epoch != 0 {
		t.Fatalf("blocked projection = %+v, want blocked non-final epoch 0 with resumeAllowed", blocked)
	}

	resumed, err := store.ResumeRun(started.RunID, RunResumeRequest{
		Actor:  "agent:test",
		Mode:   "reattach",
		Reason: "operator confirmed recovery",
	})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if resumed.Status != RunStatusRunning || resumed.Final || resumed.Epoch != 1 || resumed.ResumeAllowed {
		t.Fatalf("resumed projection = %+v, want running epoch 1 with resume disallowed until next block", resumed)
	}

	events := readRunEvents(t, filepath.Join(store.Workspace, started.LedgerPath))
	if len(events) != 3 {
		t.Fatalf("events = %d, want run_started, run_blocked, run_resumed: %#v", len(events), events)
	}
	resume := events[2]
	if resume.Type != RunEventResumed || resume.Seq != 3 || resume.Epoch != 1 {
		t.Fatalf("resume event = %+v, want run_resumed seq 3 epoch 1", resume)
	}
	if resume.Data["resumedFromSeq"] != float64(2) || resume.Data["resumedBy"] != "agent:test" || resume.Data["resumeMode"] != "reattach" || resume.Data["reason"] != "operator confirmed recovery" {
		t.Fatalf("resume event data = %#v, want resume contract payload", resume.Data)
	}
	openDispatches, ok := resume.Data["openDispatches"].([]any)
	if !ok || len(openDispatches) != 1 || openDispatches[0] != "dispatch_01" {
		t.Fatalf("resume openDispatches = %#v, want blocked-run open dispatches carried forward", resume.Data["openDispatches"])
	}
}

func TestS5ResumeRejectsRunningFinalAndNotAllowedRuns(t *testing.T) {
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

	if _, err := store.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test"}); !errors.Is(err, ErrRunResumeNotAllowed) {
		t.Fatalf("resume running run error = %v, want ErrRunResumeNotAllowed", err)
	}
	if err := store.AppendRunEvent(started.RunID, RunEvent{Type: RunEventBlocked, Data: map[string]any{"resumeAllowed": false, "reason": "manual review disabled"}}); err != nil {
		t.Fatalf("append non-resumable block: %v", err)
	}
	if _, err := store.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test"}); !errors.Is(err, ErrRunResumeNotAllowed) {
		t.Fatalf("resume non-resumable block error = %v, want ErrRunResumeNotAllowed", err)
	}

	finalStore, finalPersonas := s4RunFixture(t)
	finalStore.Now = fixedClock()
	finalPersonas.Now = fixedClock()
	if _, err := finalPersonas.CreatePersona(CreatePersonaRequest{ID: "scout", Kind: "specialist", Capabilities: []string{"research"}, Harness: "openai-codex"}); err != nil {
		t.Fatalf("create final persona: %v", err)
	}
	writeFixture(t, finalStore.BoardPath("session-search"), s4RunBoardFixture())
	finalBoard, err := finalStore.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read final board: %v", err)
	}
	finalStarted, err := finalStore.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: finalBoard.ETag,
		ExpectedBoardRev:  finalBoard.Rev,
		Personas:          finalPersonas,
	})
	if err != nil {
		t.Fatalf("start final run: %v", err)
	}
	if err := finalStore.AppendRunEvent(finalStarted.RunID, RunEvent{Type: RunEventSucceeded, Data: map[string]any{"final": true}}); err != nil {
		t.Fatalf("append final: %v", err)
	}
	if _, err := finalStore.ResumeRun(finalStarted.RunID, RunResumeRequest{Actor: "agent:test"}); !errors.Is(err, ErrRunFinal) {
		t.Fatalf("resume final run error = %v, want ErrRunFinal", err)
	}
}

func TestS5BlockedEpochRejectsContinuationUntilResume(t *testing.T) {
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
	if err := store.AppendRunEvent(started.RunID, RunEvent{Type: RunEventBlocked, Data: map[string]any{"resumeAllowed": true, "resumePolicy": "explicit"}}); err != nil {
		t.Fatalf("append block: %v", err)
	}

	if err := store.AppendRunEvent(started.RunID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_research"}); !errors.Is(err, ErrRunEpochBlocked) {
		t.Fatalf("append after blocked epoch error = %v, want ErrRunEpochBlocked", err)
	}
	if _, err := store.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test", Mode: "operator-input"}); err != nil {
		t.Fatalf("resume blocked run: %v", err)
	}
	if err := store.AppendRunEvent(started.RunID, RunEvent{Type: RunEventNodeStarted, NodeID: "fmn_research"}); err != nil {
		t.Fatalf("append after resume: %v", err)
	}

	events := readRunEvents(t, filepath.Join(store.Workspace, started.LedgerPath))
	last := events[len(events)-1]
	if last.Type != RunEventNodeStarted || last.Epoch != 1 {
		t.Fatalf("post-resume append = %+v, want node_started in resumed epoch 1", last)
	}
}

func TestS5EngineResumeSkipsCompletedNodesAndContinuesFromLedger(t *testing.T) {
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
		Limits:            RunLimits{MaxDispatch: 1, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked || !status.ResumeAllowed {
		t.Fatalf("initial status = %+v, want resumable blocked run", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_frame"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("initial executor nodes = %v, want only completed first node", got)
	}

	executor.calls = nil
	resumed, err := engine.ResumeRun(status.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "continue after dispatch limit"})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if resumed.Status != RunStatusBlocked || resumed.Epoch != 1 || !resumed.ResumeAllowed {
		t.Fatalf("first resume status = %+v, want blocked epoch 1 after one more dispatch", resumed)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_research"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first resume executor nodes = %v, want only next incomplete node", got)
	}

	executor.calls = nil
	resumed, err = engine.ResumeRun(status.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "continue final step"})
	if err != nil {
		t.Fatalf("second resume run: %v", err)
	}
	if resumed.Status != RunStatusSucceeded || !resumed.Final || resumed.Epoch != 2 {
		t.Fatalf("second resume status = %+v, want final succeeded epoch 2", resumed)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second resume executor nodes = %v, want only final incomplete node", got)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if got, want := nodeStartedAttempts(events, "fmn_frame"), []int{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("frame attempts = %v, want completed node never rerun", got)
	}
	if got, want := nodeStartedAttempts(events, "fmn_research"), []int{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("research attempts = %v, want one resumed attempt", got)
	}
	if got, want := nodeStartedAttempts(events, "fmn_ship"), []int{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ship attempts = %v, want one resumed attempt", got)
	}
}

func TestS5EngineResumeRejectsRunsThatAreNotBlocked(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4CascadeBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 10, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusSucceeded {
		t.Fatalf("status = %+v, want succeeded fixture", status)
	}
	if _, err := engine.ResumeRun(status.RunID, RunResumeRequest{Actor: "agent:test"}); !errors.Is(err, ErrRunFinal) {
		t.Fatalf("resume completed run error = %v, want ErrRunFinal", err)
	}
}

func TestS5EngineResumeOpenDispatchRecordsReattachErrorWithoutResend(t *testing.T) {
	store, started := startS4DispatchRun(t)
	adapter := &fakeDispatchAdapter{}
	dispatcher := NewSlotDispatcher(store, adapter)
	lease, err := dispatcher.DispatchSlot(started.RunID, SlotDispatchRequest{
		NodeID:      "fmn_work",
		SlotID:      "slot_work",
		AgentID:     "scout",
		Harness:     "openai-codex",
		SessionStem: "scout",
		SessionRef:  "tmux:scout",
		Prompt:      "Do the work",
		Attempt:     1,
	})
	if err != nil {
		t.Fatalf("dispatch slot: %v", err)
	}
	if err := dispatcher.CompleteFromCapture(started.RunID, lease.DispatchID, "<<<CHROTE-DONE run-id=wrong status=ok artifact=fake>>>"); !errors.Is(err, ErrDispatchTimeout) {
		t.Fatalf("complete with mismatched sentinel error = %v, want ErrDispatchTimeout", err)
	}
	if len(adapter.sent) != 1 {
		t.Fatalf("initial adapter sends = %d, want one original send", len(adapter.sent))
	}

	engine := NewRunEngine(store, nil, &fakeRunExecutor{})
	status, err := engine.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "recover open dispatch"})
	if err != nil {
		t.Fatalf("resume open dispatch: %v", err)
	}
	if status.Status != RunStatusBlocked || status.Epoch != 1 || !status.ResumeAllowed {
		t.Fatalf("status = %+v, want reattach failure to block new epoch", status)
	}
	if len(adapter.sent) != 1 {
		t.Fatalf("adapter sends after resume = %d, want no blind resend", len(adapter.sent))
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	resume := lastEventOfType(t, events, RunEventResumed)
	if resume.Data["resumeMode"] != "reattach" {
		t.Fatalf("resume data = %#v, want reattach mode", resume.Data)
	}
	errEvent := lastEventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "dispatch_reattach_failed" || errEvent.Data["dispatchId"] != lease.DispatchID {
		t.Fatalf("resume recovery error = %#v, want dispatch_reattach_failed for open lease", errEvent.Data)
	}
}

func TestS5EngineResumeOpenDispatchReattachesCompletedCapture(t *testing.T) {
	store, started := startS4DispatchRun(t)
	dispatcher := NewSlotDispatcher(store, &fakeDispatchAdapter{})
	lease, err := dispatcher.DispatchSlot(started.RunID, SlotDispatchRequest{
		NodeID:      "fmn_research",
		SlotID:      "slot_research",
		AgentID:     "scout",
		Harness:     "openai-codex",
		SessionStem: "scout",
		SessionRef:  "tmux:scout",
		Prompt:      "Do the work",
		Attempt:     1,
	})
	if err != nil {
		t.Fatalf("dispatch slot: %v", err)
	}
	if err := dispatcher.CompleteFromCapture(started.RunID, lease.DispatchID, "still working"); !errors.Is(err, ErrDispatchTimeout) {
		t.Fatalf("complete without sentinel error = %v, want ErrDispatchTimeout", err)
	}

	executor := &fakeReattachExecutor{result: FormationExecutionResult{
		Status:    "done",
		ReportRef: "reports/reattached.md",
		Text:      "reattached output",
		Outputs: map[string]FormationOutputPayload{
			"port_research_out": {Text: "reattached output"},
		},
	}}
	engine := NewRunEngine(store, nil, executor)
	status, err := engine.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "recover completed pane"})
	if err != nil {
		t.Fatalf("resume open dispatch: %v", err)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if status.Status != RunStatusSucceeded || status.Final != true {
		t.Fatalf("status = %+v, want succeeded after reattached terminal node; events=%#v", status, events)
	}
	if len(executor.reattachCalls) != 1 || executor.reattachCalls[0].DispatchID != lease.DispatchID {
		t.Fatalf("reattach calls = %+v, want original dispatch", executor.reattachCalls)
	}
	output := lastEventOfType(t, events, RunEventNodeOutput)
	if output.NodeID != "fmn_research" {
		t.Fatalf("node output = %+v, want reattached formation output", output)
	}
	for _, event := range events {
		if event.Type == RunEventError && event.Data["code"] == "dispatch_reattach_failed" {
			t.Fatalf("events = %#v, did not expect dispatch_reattach_failed", events)
		}
	}
}

func TestS5EngineResumeOpenDispatchRoutesReattachedOutputThroughGate(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
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
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	dispatcher := NewSlotDispatcher(store, &fakeDispatchAdapter{})
	lease, err := dispatcher.DispatchSlot(started.RunID, SlotDispatchRequest{
		NodeID:      "fmn_work",
		SlotID:      "slot_work",
		AgentID:     "scout",
		Harness:     "openai-codex",
		SessionStem: "scout",
		SessionRef:  "tmux:scout",
		Prompt:      "Do the work",
		Attempt:     1,
	})
	if err != nil {
		t.Fatalf("dispatch slot: %v", err)
	}
	if err := dispatcher.CompleteFromCapture(started.RunID, lease.DispatchID, "still working"); !errors.Is(err, ErrDispatchTimeout) {
		t.Fatalf("complete without sentinel error = %v, want ErrDispatchTimeout", err)
	}
	executor := &fakeReattachExecutor{
		result: FormationExecutionResult{
			Status: "done",
			Text:   "reattached work output",
			Outputs: map[string]FormationOutputPayload{
				"port_work_out": {Text: "reattached work output"},
			},
		},
		executeResults: map[string]FormationExecutionResult{
			"fmn_ship": {
				Status: "done",
				Text:   "ship output",
				Outputs: map[string]FormationOutputPayload{
					"port_ship_out": {Text: "ship output"},
				},
			},
		},
	}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"pass"}})
	status, err := engine.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "recover completed work"})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded after routed gate", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdict := eventOfType(t, events, RunEventGateVerdict)
	if verdict.GateID != "gate_review" || verdict.Data["verdict"] != "pass" {
		t.Fatalf("gate verdict = %+v, want pass after reattached output", verdict)
	}
	ship := lastEventOfType(t, events, RunEventNodeOutput)
	if ship.NodeID != "fmn_ship" {
		t.Fatalf("last output = %+v, want ship formation output", ship)
	}
}

func TestS5EngineResumeTerminalJudgeGatePassDoesNotReplayJudge(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), terminalJudgePassBoardFixture())
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
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	workInput := RunInputRef{EdgeID: "edge_work_gate", FromNodeID: "fmn_work", FromPortID: "port_work_out", ToPortID: "in", Ref: "ledger://work", Text: "work output"}
	for _, event := range []RunEvent{
		{Type: RunEventNodeStarted, NodeID: "mis_showcase", MissionID: "mis_showcase", Data: map[string]any{"nodeKind": "mission"}},
		{Type: RunEventNodeOutput, NodeID: "mis_showcase", MissionID: "mis_showcase", Data: formationOutputEventData(FormationExecutionResult{Status: "done", Text: "mission objective", Outputs: map[string]FormationOutputPayload{"out": {Text: "mission objective"}}})},
		{Type: RunEventNodeStarted, NodeID: "fmn_work", Attempt: 1, Data: map[string]any{"nodeKind": "formation"}},
		{Type: RunEventNodeOutput, NodeID: "fmn_work", Data: formationOutputEventData(FormationExecutionResult{Status: "done", Text: "work output", Outputs: map[string]FormationOutputPayload{"port_work_out": {Text: "work output"}}})},
		{Type: RunEventGateEvaluating, GateID: "gate_review", NodeID: "gate_review", Data: map[string]any{"gateId": "gate_review", "inputRef": workInput}},
		{Type: RunEventNodeStarted, NodeID: "fmn_j1", Attempt: 1, Data: map[string]any{"nodeKind": "formation", "reason": "judge"}},
		{Type: RunEventNodeOutput, NodeID: "fmn_j1", Data: judgeOutputData("review notes", "port_j1_out")},
		{Type: RunEventNodeStarted, NodeID: "fmn_j2", Attempt: 1, Data: map[string]any{"nodeKind": "formation", "reason": "judge"}},
		{Type: RunEventNodeOutput, NodeID: "fmn_j2", Data: judgeOutputData("pass", "port_j2_out")},
		{Type: RunEventGateVerdict, GateID: "gate_review", NodeID: "gate_review", Data: map[string]any{"verdict": "pass", "perKind": map[string]string{"formation": "pass"}, "routePort": "pass", "routedEdges": []string{}, "reason": "judge chain", "inputRef": workInput}},
		{Type: RunEventBlocked, NodeID: "gate_review", Data: map[string]any{"reason": "crashed before terminal success", "resumeAllowed": true, "resumePolicy": "explicit"}},
	} {
		if err := store.AppendRunEvent(started.RunID, event); err != nil {
			t.Fatalf("append %s/%s: %v", event.Type, event.NodeID, err)
		}
	}
	before := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if !terminalPassReached(before) {
		t.Fatalf("test setup did not create terminal pass verdict: %#v", before)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	status, err := engine.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "close terminal judge pass"})
	if err != nil {
		t.Fatalf("resume terminal judge pass: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want terminal pass resume to succeed", status)
	}
	if got := executor.nodeIDs(); len(got) != 0 {
		t.Fatalf("resume executor nodes = %v, want no judge replay", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if events[len(events)-1].Type != RunEventSucceeded {
		t.Fatalf("last event = %s, want run_succeeded", events[len(events)-1].Type)
	}
	if got := countEventsForNode(events, RunEventGateEvaluating, "gate_review"); got != 1 {
		t.Fatalf("gate_evaluating count = %d, want no duplicate", got)
	}
	if got := countEventsForNode(events, RunEventGateVerdict, "gate_review"); got != 1 {
		t.Fatalf("gate_verdict count = %d, want no duplicate", got)
	}
	if got, want := nodeStartedAttempts(events, "fmn_j1"), []int{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fmn_j1 starts = %v, want original judge start only", got)
	}
	if got, want := nodeStartedAttempts(events, "fmn_j2"), []int{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fmn_j2 starts = %v, want original judge start only", got)
	}
}

func TestS5EngineResumeTerminalFailDoesNotBecomeGraphCompleteSuccess(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), terminalSimpleGateBoardFixture())
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
		Limits:            RunLimits{MaxDispatch: 4, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	workInput := RunInputRef{EdgeID: "edge_work_gate", FromNodeID: "fmn_work", FromPortID: "port_work_out", ToPortID: "in", Ref: "ledger://work", Text: "work output"}
	for _, event := range []RunEvent{
		{Type: RunEventNodeStarted, NodeID: "fmn_work", Attempt: 1, Data: map[string]any{"nodeKind": "formation"}},
		{Type: RunEventNodeOutput, NodeID: "fmn_work", Data: formationOutputEventData(FormationExecutionResult{Status: "done", Text: "work output", Outputs: map[string]FormationOutputPayload{"port_work_out": {Text: "work output"}}})},
		{Type: RunEventGateEvaluating, GateID: "gate_review", NodeID: "gate_review", Data: map[string]any{"gateId": "gate_review", "inputRef": workInput}},
		{Type: RunEventGateVerdict, GateID: "gate_review", NodeID: "gate_review", Data: map[string]any{"verdict": "fail", "perKind": map[string]string{"code": "fail"}, "routePort": "none", "routedEdges": []string{}, "reason": "unwired fail", "inputRef": workInput}},
		{Type: RunEventBlocked, NodeID: "gate_review", Data: map[string]any{"reason": "gate fail is unwired", "resumeAllowed": true, "resumePolicy": "explicit"}},
	} {
		if err := store.AppendRunEvent(started.RunID, event); err != nil {
			t.Fatalf("append %s/%s: %v", event.Type, event.NodeID, err)
		}
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	status, err := engine.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "operator tried resume"})
	if err != nil {
		t.Fatalf("resume terminal fail: %v", err)
	}
	if status.Status == RunStatusSucceeded || status.Final {
		t.Fatalf("status = %+v, unwired fail must not resume to success", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if last := events[len(events)-1]; last.Type == RunEventSucceeded {
		t.Fatalf("last event = %+v, unwired fail must not append run_succeeded", last)
	}
}

func TestS5EngineResumeHonorsMaxAttemptsFromOriginalRun(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4CascadeBoardFixture())
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
		Limits:            RunLimits{MaxAttempts: 1, MaxDispatch: 10},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	for _, event := range []RunEvent{
		{Type: RunEventNodeStarted, NodeID: "fmn_frame", Attempt: 1},
		{Type: RunEventNodeOutput, NodeID: "fmn_frame", Data: formationOutputEventData(FormationExecutionResult{
			Status: "done",
			Text:   "frame output",
			Outputs: map[string]FormationOutputPayload{
				"port_frame_out": {Text: "frame output"},
			},
		})},
		{Type: RunEventNodeStarted, NodeID: "fmn_research", Attempt: 1},
		{Type: RunEventBlocked, NodeID: "fmn_research", Data: map[string]any{"resumeAllowed": true, "resumePolicy": "explicit", "reason": "resume limit check"}},
	} {
		if err := store.AppendRunEvent(started.RunID, event); err != nil {
			t.Fatalf("append %s: %v", event.Type, err)
		}
	}

	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	status, err := engine.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "operator resume"})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if status.Status != RunStatusBlocked || !status.ResumeAllowed {
		t.Fatalf("status = %+v, want resumable blocked after exhausted resume attempt", status)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("executor calls = %v, want no blind re-run after max attempts exhausted", executor.nodeIDs())
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := lastEventOfType(t, events, RunEventError)
	if errEvent.NodeID != "fmn_research" || errEvent.Data["code"] != "resume_attempts_exhausted" {
		t.Fatalf("resume limit error = %+v, want resume_attempts_exhausted for fmn_research", errEvent)
	}
}

func terminalSimpleGateBoardFixture() string {
	return strings.Replace(s4GateBoardFixture(false), `
[[connection]]
id = "edge_gate_pass_ship"
from = "gate_review:pass"
to = "fmn_ship:port_ship_in"
`, "", 1)
}

func terminalJudgePassBoardFixture() string {
	return strings.Replace(s4JudgeChainRunBoardFixture(), `
[[connection]]
id = "edge_gate_pass_ship"
from = "gate_review:pass"
to = "fmn_ship:port_ship_in"
`, "", 1)
}

func judgeOutputData(text, outputPort string) map[string]any {
	data := formationOutputEventData(FormationExecutionResult{
		Status: "done",
		Text:   text,
		Outputs: map[string]FormationOutputPayload{
			outputPort: {Text: text},
		},
	})
	data["reason"] = "judge"
	return data
}

func countEventsForNode(events []RunEvent, eventType, nodeID string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType && event.NodeID == nodeID {
			count++
		}
	}
	return count
}

type fakeReattachExecutor struct {
	result         FormationExecutionResult
	executeResults map[string]FormationExecutionResult
	reattachCalls  []FormationReattachRequest
}

func (f *fakeReattachExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	if f.executeResults != nil {
		if result, ok := f.executeResults[req.NodeID]; ok {
			return result, nil
		}
	}
	return FormationExecutionResult{}, ErrRunExecutorUnavailable
}

func (f *fakeReattachExecutor) ReattachFormationDispatch(req FormationReattachRequest) (FormationExecutionResult, error) {
	f.reattachCalls = append(f.reattachCalls, req)
	return f.result, nil
}

func lastEventOfType(t *testing.T, events []RunEvent, eventType string) RunEvent {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eventType {
			return events[i]
		}
	}
	t.Fatalf("missing event type %s in %#v", eventType, events)
	return RunEvent{}
}
