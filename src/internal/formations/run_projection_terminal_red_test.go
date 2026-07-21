package formations

import (
	"strings"
	"testing"
)

func TestSchema2TerminalLifecycleRequiresExactPredecessor(t *testing.T) {
	terminalCases := []struct {
		name      string
		eventType string
		data      func() map[string]any
	}{
		{name: "succeeded", eventType: "run_succeeded", data: schema2TerminalSucceededData},
		{name: "canceled", eventType: "run_canceled", data: func() map[string]any { return schema2TerminalCanceledData(20) }},
		{name: "failed", eventType: "run_failed", data: func() map[string]any { return schema2TerminalFailedData(20) }},
	}

	t.Run("blocked_rejects_every_direct_terminal", func(t *testing.T) {
		for _, test := range terminalCases {
			t.Run(test.name, func(t *testing.T) {
				state := schema2TerminalBlockedState(t)
				err := schema2EpochReduce(t, &state, 21, 0, test.eventType, test.data())
				if err == nil {
					t.Fatalf("blocked run admitted direct %s", test.eventType)
				}
				requireProjectionError(t, err, ErrRunProjectionInvalid)
			})
		}
	})

	t.Run("canceling_accepts_only_its_terminal_or_exact_failure_escalation", func(t *testing.T) {
		t.Run("rejects_structural_execution", func(t *testing.T) {
			state := schema2TerminalCancelingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "node_waiting", schema2SecondRepairFixture(t, "node_waiting"))
			if err == nil {
				t.Fatal("canceling run admitted structural execution")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_run_blocked", func(t *testing.T) {
			state := schema2TerminalCancelingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_blocked", schema2GreenRereviewBlock("run", "new_run_required", nil, false))
			if err == nil {
				t.Fatal("canceling run returned to blocked")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_repeated_cancel_request", func(t *testing.T) {
			state := schema2TerminalCancelingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_cancel_requested", schema2TerminalCancelRequestedData())
			if err == nil {
				t.Fatal("canceling run admitted a second cancel request")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_succeeded", func(t *testing.T) {
			state := schema2TerminalCancelingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_succeeded", schema2TerminalSucceededData())
			if err == nil {
				t.Fatal("canceling run admitted run_succeeded")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_failed_without_failure_start", func(t *testing.T) {
			state := schema2TerminalCancelingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_failed", schema2TerminalFailedData(21))
			if err == nil {
				t.Fatal("canceling run admitted run_failed without failure reconciliation")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_canceled_for_wrong_request", func(t *testing.T) {
			state := schema2TerminalCancelingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_canceled", schema2TerminalCanceledData(20))
			if err == nil {
				t.Fatal("run_canceled did not name the current cancel request")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("accepts_canceled_for_exact_request", func(t *testing.T) {
			state := schema2TerminalCancelingState(t)
			if err := schema2EpochReduce(t, &state, 22, 0, "run_canceled", schema2TerminalCanceledData(21)); err != nil {
				t.Fatalf("matching run_canceled rejected: %v", err)
			}
			if state.view.Status != "canceled" || !state.view.Final {
				t.Fatalf("matching cancellation = status %q final %t", state.view.Status, state.view.Final)
			}
		})

		t.Run("rejects_failure_start_for_wrong_cancel_request", func(t *testing.T) {
			state := schema2TerminalCancelingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_failure_reconciliation_started", schema2TerminalFailureStartedData(20))
			if err == nil {
				t.Fatal("failure reconciliation did not name the current cancel request")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("accepts_exact_failure_escalation_and_matching_failed", func(t *testing.T) {
			state := schema2TerminalCancelingState(t)
			if err := schema2EpochReduce(t, &state, 22, 0, "run_failure_reconciliation_started", schema2TerminalFailureStartedData(21)); err != nil {
				t.Fatalf("matching failure reconciliation rejected: %v", err)
			}
			if err := schema2EpochReduce(t, &state, 23, 0, "run_failed", schema2TerminalFailedData(22)); err != nil {
				t.Fatalf("matching run_failed rejected: %v", err)
			}
			if state.view.Status != "failed" || !state.view.Final {
				t.Fatalf("matching failure = status %q final %t", state.view.Status, state.view.Final)
			}
		})
	})

	t.Run("failing_accepts_only_failed_for_exact_reconciliation", func(t *testing.T) {
		t.Run("rejects_structural_execution", func(t *testing.T) {
			state := schema2TerminalFailingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "node_waiting", schema2SecondRepairFixture(t, "node_waiting"))
			if err == nil {
				t.Fatal("failing run admitted structural execution")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_run_blocked", func(t *testing.T) {
			state := schema2TerminalFailingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_blocked", schema2GreenRereviewBlock("run", "new_run_required", nil, false))
			if err == nil {
				t.Fatal("failing run returned to blocked")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_cancel_request", func(t *testing.T) {
			state := schema2TerminalFailingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_cancel_requested", schema2TerminalCancelRequestedData())
			if err == nil {
				t.Fatal("failing run returned to canceling")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_repeated_failure_start", func(t *testing.T) {
			state := schema2TerminalFailingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_failure_reconciliation_started", schema2TerminalFailureStartedData(0))
			if err == nil {
				t.Fatal("failing run admitted a second failure reconciliation start")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("blocked_failure_start_requires_no_cancel_request", func(t *testing.T) {
			state := schema2TerminalBlockedState(t)
			err := schema2EpochReduce(t, &state, 21, 0, "run_failure_reconciliation_started", schema2TerminalFailureStartedData(20))
			if err == nil {
				t.Fatal("blocked failure reconciliation invented a cancel request")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_succeeded", func(t *testing.T) {
			state := schema2TerminalFailingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_succeeded", schema2TerminalSucceededData())
			if err == nil {
				t.Fatal("failing run admitted run_succeeded")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_canceled", func(t *testing.T) {
			state := schema2TerminalFailingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_canceled", schema2TerminalCanceledData(20))
			if err == nil {
				t.Fatal("failing run admitted run_canceled")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_failed_for_wrong_reconciliation", func(t *testing.T) {
			state := schema2TerminalFailingState(t)
			err := schema2EpochReduce(t, &state, 22, 0, "run_failed", schema2TerminalFailedData(20))
			if err == nil {
				t.Fatal("run_failed did not name the current failure reconciliation")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("accepts_failed_for_exact_reconciliation", func(t *testing.T) {
			state := schema2TerminalFailingState(t)
			if err := schema2EpochReduce(t, &state, 22, 0, "run_failed", schema2TerminalFailedData(21)); err != nil {
				t.Fatalf("matching run_failed rejected: %v", err)
			}
			if state.view.Status != "failed" || !state.view.Final {
				t.Fatalf("matching failure = status %q final %t", state.view.Status, state.view.Final)
			}
		})
	})

	t.Run("running_accepts_only_succeeded_terminal", func(t *testing.T) {
		t.Run("rejects_canceled", func(t *testing.T) {
			state := schema2EpochTestState()
			err := schema2EpochReduce(t, &state, 20, 0, "run_canceled", schema2TerminalCanceledData(19))
			if err == nil {
				t.Fatal("running run admitted run_canceled without cancel request")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("rejects_failed", func(t *testing.T) {
			state := schema2EpochTestState()
			err := schema2EpochReduce(t, &state, 20, 0, "run_failed", schema2TerminalFailedData(19))
			if err == nil {
				t.Fatal("running run admitted run_failed without failure reconciliation")
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})

		t.Run("accepts_succeeded", func(t *testing.T) {
			state := schema2EpochTestState()
			if err := schema2EpochReduce(t, &state, 20, 0, "run_succeeded", schema2TerminalSucceededData()); err != nil {
				t.Fatalf("running run_succeeded rejected: %v", err)
			}
			if state.view.Status != "succeeded" || !state.view.Final {
				t.Fatalf("success = status %q final %t", state.view.Status, state.view.Final)
			}
		})
	})

	t.Run("running_can_enter_exact_cancel_and_failure_paths", func(t *testing.T) {
		t.Run("cancel_request_then_matching_canceled", func(t *testing.T) {
			state := schema2EpochTestState()
			if err := schema2EpochReduce(t, &state, 20, 0, "run_cancel_requested", schema2TerminalCancelRequestedData()); err != nil {
				t.Fatalf("running cancel request rejected: %v", err)
			}
			if state.view.Status != "canceling" || state.view.Identity.Epoch != 0 {
				t.Fatalf("cancel request = status %q epoch %d", state.view.Status, state.view.Identity.Epoch)
			}
			if err := schema2EpochReduce(t, &state, 21, 0, "run_canceled", schema2TerminalCanceledData(20)); err != nil {
				t.Fatalf("matching running-origin cancellation rejected: %v", err)
			}
			if state.view.Status != "canceled" || !state.view.Final || state.view.Identity.Epoch != 0 {
				t.Fatalf("running-origin cancellation = status %q final %t epoch %d", state.view.Status, state.view.Final, state.view.Identity.Epoch)
			}
		})

		t.Run("failure_start_then_matching_failed", func(t *testing.T) {
			state := schema2EpochTestState()
			if err := schema2EpochReduce(t, &state, 20, 0, "run_failure_reconciliation_started", schema2TerminalFailureStartedData(0)); err != nil {
				t.Fatalf("running failure reconciliation rejected: %v", err)
			}
			if state.view.Status != "failing" || state.view.Identity.Epoch != 0 {
				t.Fatalf("failure start = status %q epoch %d", state.view.Status, state.view.Identity.Epoch)
			}
			if err := schema2EpochReduce(t, &state, 21, 0, "run_failed", schema2TerminalFailedData(20)); err != nil {
				t.Fatalf("matching running-origin failure rejected: %v", err)
			}
			if state.view.Status != "failed" || !state.view.Final || state.view.Identity.Epoch != 0 {
				t.Fatalf("running-origin failure = status %q final %t epoch %d", state.view.Status, state.view.Final, state.view.Identity.Epoch)
			}
		})
	})

	t.Run("queued_requires_cancel_or_failure_predecessor", func(t *testing.T) {
		for _, test := range terminalCases {
			t.Run("rejects_direct_"+test.name, func(t *testing.T) {
				state := schema2TerminalQueuedState()
				err := schema2EpochReduce(t, &state, 20, 0, test.eventType, test.data())
				if err == nil {
					t.Fatalf("queued run admitted direct %s", test.eventType)
				}
				requireProjectionError(t, err, ErrRunProjectionInvalid)
			})
		}

		t.Run("cancel_request_then_matching_canceled", func(t *testing.T) {
			state := schema2TerminalQueuedState()
			if err := schema2EpochReduce(t, &state, 20, 0, "run_cancel_requested", schema2TerminalCancelRequestedData()); err != nil {
				t.Fatalf("queued cancel request rejected: %v", err)
			}
			if state.view.Status != "canceling" || state.view.Identity.Epoch != 0 {
				t.Fatalf("queued cancel request = status %q epoch %d", state.view.Status, state.view.Identity.Epoch)
			}
			if err := schema2EpochReduce(t, &state, 21, 0, "run_canceled", schema2TerminalCanceledData(20)); err != nil {
				t.Fatalf("matching queued-origin cancellation rejected: %v", err)
			}
			if state.view.Status != "canceled" || !state.view.Final || state.view.Identity.Epoch != 0 {
				t.Fatalf("queued-origin cancellation = status %q final %t epoch %d", state.view.Status, state.view.Final, state.view.Identity.Epoch)
			}
		})

		t.Run("failure_start_then_matching_failed", func(t *testing.T) {
			state := schema2TerminalQueuedState()
			if err := schema2EpochReduce(t, &state, 20, 0, "run_failure_reconciliation_started", schema2TerminalFailureStartedData(0)); err != nil {
				t.Fatalf("queued failure reconciliation rejected: %v", err)
			}
			if state.view.Status != "failing" || state.view.Identity.Epoch != 0 {
				t.Fatalf("queued failure start = status %q epoch %d", state.view.Status, state.view.Identity.Epoch)
			}
			if err := schema2EpochReduce(t, &state, 21, 0, "run_failed", schema2TerminalFailedData(20)); err != nil {
				t.Fatalf("matching queued-origin failure rejected: %v", err)
			}
			if state.view.Status != "failed" || !state.view.Final || state.view.Identity.Epoch != 0 {
				t.Fatalf("queued-origin failure = status %q final %t epoch %d", state.view.Status, state.view.Final, state.view.Identity.Epoch)
			}
		})
	})

	t.Run("waiting_human_requires_decision_or_cancel", func(t *testing.T) {
		for _, test := range terminalCases {
			t.Run("rejects_direct_"+test.name, func(t *testing.T) {
				state := schema2TerminalWaitingState(t)
				err := schema2EpochReduce(t, &state, 22, 0, test.eventType, test.data())
				if err == nil {
					t.Fatalf("waiting_human run admitted direct %s", test.eventType)
				}
				requireProjectionError(t, err, ErrRunProjectionInvalid)
			})
		}

		t.Run("cancel_request_then_matching_canceled", func(t *testing.T) {
			state := schema2TerminalWaitingState(t)
			if err := schema2EpochReduce(t, &state, 22, 0, "run_cancel_requested", schema2TerminalCancelRequestedData()); err != nil {
				t.Fatalf("waiting_human cancel request rejected: %v", err)
			}
			if state.view.Status != "canceling" || state.view.Identity.Epoch != 0 {
				t.Fatalf("waiting cancel request = status %q epoch %d", state.view.Status, state.view.Identity.Epoch)
			}
			if err := schema2EpochReduce(t, &state, 23, 0, "run_canceled", schema2TerminalCanceledData(22)); err != nil {
				t.Fatalf("matching waiting-origin cancellation rejected: %v", err)
			}
			if state.view.Status != "canceled" || !state.view.Final || state.view.Identity.Epoch != 0 {
				t.Fatalf("waiting-origin cancellation = status %q final %t epoch %d", state.view.Status, state.view.Final, state.view.Identity.Epoch)
			}
		})
	})
}

func TestProjectCanonicalRunRejectsSuccessAfterUnresolvedBlock(t *testing.T) {
	block := schema2Event(projectionTestRunID, 3, "run_blocked", schema2GreenRereviewBlock("run", "new_run_required", nil, false))
	succeeded := schema2Event(projectionTestRunID, 4, "run_succeeded", schema2TerminalSucceededData())
	_, err := ProjectCanonicalRun(schema2ProjectionInput(t, true, block, succeeded))
	if err == nil {
		t.Fatal("canonical projection reported success after unresolved non-resumable block")
	}
	requireProjectionError(t, err, ErrRunProjectionInvalid)
}

func schema2TerminalBlockedState(t *testing.T) projectionState {
	t.Helper()
	state := schema2EpochTestState()
	schema2EpochReduceValidBlock(t, &state)
	return state
}

func schema2TerminalQueuedState() projectionState {
	state := schema2EpochTestState()
	state.view.Status = "queued"
	return state
}

func schema2TerminalWaitingState(t *testing.T) projectionState {
	t.Helper()
	state := schema2EpochTestState()
	if err := schema2EpochReduce(t, &state, 20, 0, "gate_evaluating", schema2SecondRepairFixture(t, "gate_evaluating")); err != nil {
		t.Fatalf("reduce valid gate evaluation: %v", err)
	}
	if err := schema2EpochReduce(t, &state, 21, 0, "human_input_requested", schema2SecondRepairFixture(t, "human_input_requested")); err != nil {
		t.Fatalf("reduce valid human request: %v", err)
	}
	if state.view.Status != "waiting_human" {
		t.Fatalf("prepared human request = status %q", state.view.Status)
	}
	return state
}

func schema2TerminalCancelingState(t *testing.T) projectionState {
	t.Helper()
	state := schema2TerminalBlockedState(t)
	if err := schema2EpochReduce(t, &state, 21, 0, "run_cancel_requested", schema2TerminalCancelRequestedData()); err != nil {
		t.Fatalf("reduce valid cancel request: %v", err)
	}
	return state
}

func schema2TerminalFailingState(t *testing.T) projectionState {
	t.Helper()
	state := schema2TerminalBlockedState(t)
	if err := schema2EpochReduce(t, &state, 21, 0, "run_failure_reconciliation_started", schema2TerminalFailureStartedData(0)); err != nil {
		t.Fatalf("reduce valid failure reconciliation: %v", err)
	}
	return state
}

func schema2TerminalSucceededData() map[string]any {
	return map[string]any{"outputArtifactIds": []any{}, "final": true}
}

func schema2TerminalCancelRequestedData() map[string]any {
	return map[string]any{
		"commandId": projectionTestOtherCmdID, "commandPayloadSha256": strings.Repeat("a", 64),
		"reason": "stop", "requestedBy": "human:test", "openNodeAttempts": []any{},
		"openSlotDispatches": []any{}, "openToolLeases": []any{},
	}
}

func schema2TerminalCanceledData(cancelRequestSeq uint64) map[string]any {
	return map[string]any{
		"cancelRequestSeq": cancelRequestSeq, "reason": "stop", "requestedBy": "human:test",
		"nodeAttemptDispositions": []any{}, "slotDispatchDispositions": []any{},
		"reconciledToolLeases": []any{}, "final": true,
	}
}

func schema2TerminalFailureStartedData(originCancelRequestSeq uint64) map[string]any {
	return map[string]any{
		"originCancelRequestSeq": originCancelRequestSeq, "code": "engine_failed", "reason": "engine failed",
		"unrecoverable": true, "relatedSeq": uint64(20), "failureCause": map[string]any{"kind": "none"},
		"openNodeAttempts": []any{}, "openSlotDispatches": []any{}, "openToolLeases": []any{},
		"recordedBeforeReconciliation": true,
	}
}

func schema2TerminalFailedData(failureReconciliationSeq uint64) map[string]any {
	return map[string]any{
		"failureReconciliationSeq": failureReconciliationSeq, "code": "engine_failed", "reason": "engine failed",
		"unrecoverable": true, "relatedSeq": uint64(20), "failureCause": map[string]any{"kind": "none"},
		"nodeAttemptDispositions": []any{}, "slotDispatchDispositions": []any{},
		"toolLeaseDispositions": []any{}, "final": true,
	}
}
