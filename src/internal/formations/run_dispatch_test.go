package formations

import (
	"errors"
	"strings"
	"testing"
)

func TestS4SlotDispatchLeaseRecordedBeforeSend(t *testing.T) {
	store, started := startS4DispatchRun(t)
	adapter := &fakeDispatchAdapter{
		beforeSend: func(payload SlotDispatchPayload) {
			events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
			last := events[len(events)-1]
			if last.Type != RunEventSlotDispatch {
				t.Fatalf("last event before adapter send = %s, want slot_dispatch", last.Type)
			}
			if last.Data["recordedBeforeSend"] != true || last.Data["nativeAck"] != false {
				t.Fatalf("slot_dispatch data = %#v, want lease-before-send and nativeAck=false", last.Data)
			}
			if last.Data["dispatchId"] != payload.DispatchID {
				t.Fatalf("dispatch id in ledger = %#v, payload = %s", last.Data["dispatchId"], payload.DispatchID)
			}
		},
	}
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
	if lease.DispatchID == "" || !strings.Contains(lease.DispatchID, started.RunID) {
		t.Fatalf("dispatch id = %q, want durable run-scoped lease id", lease.DispatchID)
	}
	if len(adapter.sent) != 1 {
		t.Fatalf("adapter sends = %d, want one", len(adapter.sent))
	}
}

func TestS4CompletionSentinelRequiresMatchingRunID(t *testing.T) {
	runID := "run_test"
	if sentinel, ok := ParseCompletionSentinel("agent says <<<CHROTE-DONE run-id=wrong status=ok artifact=loot.txt>>>", runID); ok || sentinel.RunID != "" {
		t.Fatalf("mismatched sentinel parsed = %+v ok=%v, want ignored", sentinel, ok)
	}
	sentinel, ok := ParseCompletionSentinel("tail\n<<<CHROTE-DONE run-id=run_test status=ok artifact=report.md>>>\n", runID)
	if !ok {
		t.Fatal("matching sentinel was not parsed")
	}
	if sentinel.RunID != runID || sentinel.Status != "ok" || sentinel.Artifact != "report.md" {
		t.Fatalf("sentinel = %+v, want run/status/artifact fields", sentinel)
	}
}

func TestS4DeadPaneAndIdleTimeoutRecordLoudError(t *testing.T) {
	t.Run("dead pane after lease blocks run", func(t *testing.T) {
		store, started := startS4DispatchRun(t)
		dispatcher := NewSlotDispatcher(store, &fakeDispatchAdapter{err: ErrDispatchDeadPane})

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
		if !errors.Is(err, ErrDispatchDeadPane) {
			t.Fatalf("dispatch error = %v, want ErrDispatchDeadPane", err)
		}
		if lease.DispatchID == "" {
			t.Fatal("dead pane dispatch did not return the durable lease id")
		}
		status, err := store.ProjectRun(started.RunID)
		if err != nil {
			t.Fatalf("project run: %v", err)
		}
		if status.Status != RunStatusBlocked {
			t.Fatalf("status = %+v, want blocked", status)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
		if eventOfType(t, events, RunEventError).Data["code"] != "dead_pane" {
			t.Fatalf("events = %#v, want dead_pane error", events)
		}
	})

	t.Run("missing matching sentinel blocks run", func(t *testing.T) {
		store, started := startS4DispatchRun(t)
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
		err = dispatcher.CompleteFromCapture(started.RunID, lease.DispatchID, "<<<CHROTE-DONE run-id=wrong status=ok artifact=fake>>>")
		if !errors.Is(err, ErrDispatchTimeout) {
			t.Fatalf("complete from mismatched capture error = %v, want ErrDispatchTimeout", err)
		}
		status, err := store.ProjectRun(started.RunID)
		if err != nil {
			t.Fatalf("project run: %v", err)
		}
		if status.Status != RunStatusBlocked {
			t.Fatalf("status = %+v, want blocked", status)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
		if eventOfType(t, events, RunEventError).Data["code"] != "completion_sentinel_timeout" {
			t.Fatalf("events = %#v, want completion_sentinel_timeout error", events)
		}
	})
}

func TestS4CompletionForUnknownDispatchFailsLoud(t *testing.T) {
	store, started := startS4DispatchRun(t)
	dispatcher := NewSlotDispatcher(store, &fakeDispatchAdapter{})

	err := dispatcher.CompleteFromCapture(started.RunID, "dsp_unknown", "<<<CHROTE-DONE run-id="+started.RunID+" status=ok artifact=report.md>>>")
	if err == nil || !strings.Contains(err.Error(), "unknown dispatch") {
		t.Fatalf("complete unknown dispatch error = %v, want unknown dispatch failure", err)
	}
	status, err := store.ProjectRun(started.RunID)
	if err != nil {
		t.Fatalf("project run: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked for unknown dispatch completion", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if eventsContainType(events, RunEventSlotResult) {
		t.Fatalf("events include orphan slot_result for unknown dispatch: %+v", events)
	}
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "unknown_dispatch" {
		t.Fatalf("error data = %#v, want unknown_dispatch", errEvent.Data)
	}
}

type fakeDispatchAdapter struct {
	beforeSend func(SlotDispatchPayload)
	sent       []SlotDispatchPayload
	err        error
}

func (f *fakeDispatchAdapter) SendSlotDispatch(payload SlotDispatchPayload) error {
	if f.beforeSend != nil {
		f.beforeSend(payload)
	}
	f.sent = append(f.sent, payload)
	return f.err
}

func startS4DispatchRun(t *testing.T) (*Store, *RunStartResult) {
	t.Helper()
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
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
	return store, started
}
