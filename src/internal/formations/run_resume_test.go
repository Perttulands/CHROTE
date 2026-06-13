package formations

import (
	"errors"
	"path/filepath"
	"reflect"
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
