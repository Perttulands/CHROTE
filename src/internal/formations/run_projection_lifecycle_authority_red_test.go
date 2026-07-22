package formations

import (
	"bytes"
	"testing"
)

// These tests remain inside Task 1's public ledger vocabulary. In particular,
// they do not define or consume a nonempty schema-2 run-private-state record.
func TestSchema2LifecycleGraphIsClosedAcrossQueuedWaitingAndFinal(t *testing.T) {
	t.Run("queued_rejects_graph_work_before_activation", func(t *testing.T) {
		state := schema2TerminalQueuedState()
		schema2RequireReducerErrorWithoutMutation(t, &state, 20, "node_waiting", schema2SecondRepairFixture(t, "node_waiting"))
	})

	t.Run("activation_is_unique_in_whole_projection", func(t *testing.T) {
		activation := schema2Event(projectionTestRunID, 3, "run_activated", schema2SecondRepairFixture(t, "run_activated"))
		projection, err := ProjectCanonicalRun(schema2ProjectionInput(t, true, activation))
		if err == nil {
			t.Fatalf("whole projector admitted a second activation: %#v", ProjectRunView(projection))
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("activation_from_waiting_is_rejected_without_mutation", func(t *testing.T) {
		state := schema2TerminalWaitingState(t)
		event := schema2Event(projectionTestRunID, 22, "run_activated", schema2SecondRepairFixture(t, "run_activated"))
		raw, safe := schema2RepairDecodeSafeEvent(t, event)
		activation := safe.(SafeSchema2RunActivatedEvent)
		state.view.Audit.WorkspaceAdmissionSeq = activation.Data.WorkspaceAdmissionSeq
		state.view.Audit.AdmissionPolicyRev = activation.Data.AdmissionPolicyRev
		state.view.Audit.AdmissionPolicySHA256 = activation.Data.AdmissionPolicySHA256
		schema2RequireRawReducerErrorWithoutMutation(t, &state, raw, safe)
	})

	t.Run("waiting_request_cannot_be_hidden_by_reblock", func(t *testing.T) {
		state := schema2TerminalWaitingState(t)
		schema2RequireReducerErrorWithoutMutation(t, &state, 22, "run_blocked", schema2GreenRereviewBlock("run", "new_run_required", nil, false))
	})

	t.Run("waiting_request_cannot_execute_graph_work", func(t *testing.T) {
		state := schema2TerminalWaitingState(t)
		schema2RequireReducerErrorWithoutMutation(t, &state, 22, "node_waiting", schema2SecondRepairFixture(t, "node_waiting"))
	})

	t.Run("exact_waiting_verdict_returns_to_running", func(t *testing.T) {
		state := schema2TerminalWaitingState(t)
		data := schema2RepairHumanVerdictData()
		data["requestedSeq"] = uint64(21)
		if err := schema2EpochReduce(t, &state, 22, 0, "human_verdict_recorded", data); err != nil {
			t.Fatalf("exact waiting verdict rejected: %v", err)
		}
		if state.view.Status != "running" || state.view.Final || state.view.Identity.Epoch != 0 {
			t.Fatalf("exact verdict = status %q final %t epoch %d", state.view.Status, state.view.Final, state.view.Identity.Epoch)
		}
	})

	t.Run("canonical_activation_from_queued_still_passes", func(t *testing.T) {
		projection, err := ProjectCanonicalRun(schema2ProjectionInput(t, true))
		if err != nil {
			t.Fatalf("canonical queued activation rejected: %v", err)
		}
		view := ProjectRunView(projection)
		if view.Status != "running" || view.Final {
			t.Fatalf("canonical activation = status %q final %t", view.Status, view.Final)
		}
	})

	t.Run("final_binding_observation_is_current_epoch_and_non_authorizing", func(t *testing.T) {
		observed := schema2Event(projectionTestRunID, 4, "slot_binding_observed", schema2SecondRepairFixture(t, "slot_binding_observed"))
		projection, err := ProjectCanonicalRun(schema2ProjectionInput(t, true,
			schema2Event(projectionTestRunID, 3, "run_succeeded", schema2TerminalSucceededData()),
			observed,
		))
		if err != nil {
			t.Fatalf("current-epoch final binding observation rejected: %v", err)
		}
		schema2RequireSuccessfulFinalOutcome(t, ProjectRunView(projection))
	})

	t.Run("final_artifact_observation_preserves_outcome", func(t *testing.T) {
		projection, err := ProjectCanonicalRun(schema2FinalArtifactObservationInput(t, 0))
		if err != nil {
			t.Fatalf("current-epoch final artifact observation rejected: %v", err)
		}
		view := ProjectRunView(projection)
		schema2RequireSuccessfulFinalOutcome(t, view)
		raw := mustMarshalJSON(t, view.Artifacts)
		if !bytes.Contains(raw, []byte(`"availability":"unavailable"`)) {
			t.Fatalf("final artifact observation was not projected: %s", raw)
		}
	})

	t.Run("final_observation_from_future_epoch_is_rejected", func(t *testing.T) {
		projection, err := ProjectCanonicalRun(schema2FinalArtifactObservationInput(t, 1))
		if err == nil {
			t.Fatalf("future-epoch final observation projected: %#v", ProjectRunView(projection))
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("final_execution_remains_rejected", func(t *testing.T) {
		waiting := schema2Event(projectionTestRunID, 4, "node_waiting", schema2SecondRepairFixture(t, "node_waiting"))
		projection, err := ProjectCanonicalRun(schema2ProjectionInput(t, true,
			schema2Event(projectionTestRunID, 3, "run_succeeded", schema2TerminalSucceededData()),
			waiting,
		))
		if err == nil {
			t.Fatalf("post-final graph execution projected: %#v", ProjectRunView(projection))
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})
}

func TestSchema2TerminalAuthoritySnapshotsHeadersAndMutationAreExact(t *testing.T) {
	for _, lifecycle := range []string{"cancel", "failure"} {
		t.Run("zero_cardinality_"+lifecycle+"_is_valid", func(t *testing.T) {
			state := schema2EpochTestState()
			switch lifecycle {
			case "cancel":
				if err := schema2EpochReduce(t, &state, 20, 0, "run_cancel_requested", schema2TerminalCancelRequestedData()); err != nil {
					t.Fatalf("zero-cardinality cancellation start rejected: %v", err)
				}
				if err := schema2EpochReduce(t, &state, 21, 0, "run_canceled", schema2TerminalCanceledData(20)); err != nil {
					t.Fatalf("zero-cardinality cancellation final rejected: %v", err)
				}
				if state.view.Status != "canceled" || !state.view.Final {
					t.Fatalf("zero-cardinality cancellation = %q/%t", state.view.Status, state.view.Final)
				}
			case "failure":
				if err := schema2EpochReduce(t, &state, 20, 0, "run_failure_reconciliation_started", schema2TerminalFailureStartedData(0)); err != nil {
					t.Fatalf("zero-cardinality failure start rejected: %v", err)
				}
				if err := schema2EpochReduce(t, &state, 21, 0, "run_failed", schema2TerminalFailedData(20)); err != nil {
					t.Fatalf("zero-cardinality failure final rejected: %v", err)
				}
				if state.view.Status != "failed" || !state.view.Final {
					t.Fatalf("zero-cardinality failure = %q/%t", state.view.Status, state.view.Final)
				}
			}
		})

		for _, resource := range []string{"node", "dispatch", "tool"} {
			t.Run(lifecycle+"_start_rejects_invented_"+resource+"_snapshot", func(t *testing.T) {
				state := schema2EpochTestState()
				data := schema2LifecycleStartData(lifecycle)
				field, member := schema2InventedOpenSnapshot(t, lifecycle, resource)
				data[field] = []any{member}
				schema2RequireReducerErrorWithoutMutation(t, &state, 20, schema2LifecycleStartType(lifecycle), data)
			})

			t.Run(lifecycle+"_final_rejects_extra_"+resource+"_disposition", func(t *testing.T) {
				state := schema2EpochTestState()
				if err := schema2EpochReduce(t, &state, 20, 0, schema2LifecycleStartType(lifecycle), schema2LifecycleStartData(lifecycle)); err != nil {
					t.Fatalf("valid zero-cardinality %s start rejected: %v", lifecycle, err)
				}
				data := schema2LifecycleFinalData(lifecycle, 20)
				field, member := schema2InventedDisposition(t, lifecycle, resource)
				data[field] = []any{member}
				schema2RequireReducerErrorWithoutMutation(t, &state, 21, schema2LifecycleFinalType(lifecycle), data)
			})
		}
	}

	for _, header := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "code", mutate: func(data map[string]any) { data["code"] = "different_failure" }},
		{name: "reason", mutate: func(data map[string]any) { data["reason"] = "different reason" }},
		{name: "unrecoverable", mutate: func(data map[string]any) { data["unrecoverable"] = false }},
		{name: "related_sequence", mutate: func(data map[string]any) { data["relatedSeq"] = uint64(18) }},
		{name: "failure_cause", mutate: func(data map[string]any) { data["failureCause"] = map[string]any{"kind": "none"} }},
	} {
		t.Run("run_failed_rejects_changed_"+header.name+"_without_mutation", func(t *testing.T) {
			state, start := schema2FailureHeaderState(t, header.name == "failure_cause")
			failed := schema2FailureFinalFromStart(start, 20)
			header.mutate(failed)
			schema2RequireReducerErrorWithoutMutation(t, &state, 21, "run_failed", failed)
		})
	}

	for _, lifecycle := range []string{"cancel", "failure"} {
		for _, authority := range []struct {
			name string
			kind string
			seq  uint64
		}{
			{name: "exact_authority_but_dispatch_not_snapshotted", kind: lifecycle, seq: 21},
			{name: "wrong_authority_sequence", kind: lifecycle, seq: 999},
			{name: "wrong_authority_kind", kind: map[string]string{"cancel": "failure", "failure": "cancel"}[lifecycle], seq: 21},
		} {
			t.Run(lifecycle+"_cleanup_rejects_"+authority.name, func(t *testing.T) {
				state := schema2LifecycleReconciliationState(t, lifecycle)
				data := schema2SecondRepairFixture(t, "slot_reconciliation_interrupt")
				data["authorityKind"] = authority.kind
				data["authoritySeq"] = authority.seq
				schema2RequireReducerErrorWithoutMutation(t, &state, 22, "slot_reconciliation_interrupt", data)
			})
		}
	}

	t.Run("wrong_terminal_predecessor_does_not_mutate_state", func(t *testing.T) {
		state := schema2TerminalCancelingState(t)
		schema2RequireReducerErrorWithoutMutation(t, &state, 22, "run_canceled", schema2TerminalCanceledData(20))
	})

	t.Run("whole_projector_rejects_changed_failure_header", func(t *testing.T) {
		start := schema2TerminalFailureStartedData(0)
		start["relatedSeq"] = uint64(2)
		failed := schema2FailureFinalFromStart(start, 3)
		failed["code"] = "different_failure"
		projection, err := ProjectCanonicalRun(schema2ProjectionInput(t, true,
			schema2Event(projectionTestRunID, 3, "run_failure_reconciliation_started", start),
			schema2Event(projectionTestRunID, 4, "run_failed", failed),
		))
		if err == nil {
			t.Fatalf("whole projector accepted changed failure header: %#v", ProjectRunView(projection))
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("whole_projector_rejects_invented_failure_snapshot", func(t *testing.T) {
		start := schema2TerminalFailureStartedData(0)
		start["relatedSeq"] = uint64(2)
		start["openNodeAttempts"] = []any{schema2RepairNodeAttemptSnapshot()}
		failed := schema2FailureFinalFromStart(start, 3)
		node := schema2RepairNodeAttemptSnapshot()
		node["disposition"] = "abandoned_non_authorizing"
		failed["nodeAttemptDispositions"] = []any{node}
		projection, err := ProjectCanonicalRun(schema2ProjectionInput(t, true,
			schema2Event(projectionTestRunID, 3, "run_failure_reconciliation_started", start),
			schema2Event(projectionTestRunID, 4, "run_failed", failed),
		))
		if err == nil {
			t.Fatalf("whole projector accepted invented failure snapshot: %#v", ProjectRunView(projection))
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("whole_projector_rejects_success_with_open_attempt", func(t *testing.T) {
		started := schema2Event(projectionTestRunID, 3, "node_started", map[string]any{
			"nodeId": projectionTestMissionID, "nodeKind": "mission", "attempt": uint64(1),
			"reason": "initial", "inputRefs": []any{},
		})
		projection, err := ProjectCanonicalRun(schema2ProjectionInput(t, true,
			started,
			schema2Event(projectionTestRunID, 4, "run_succeeded", schema2TerminalSucceededData()),
		))
		if err == nil {
			t.Fatalf("whole projector accepted success with an open attempt: %#v", ProjectRunView(projection))
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})
}

func schema2RequireSuccessfulFinalOutcome(t *testing.T, view RunView) {
	t.Helper()
	if view.Status != "succeeded" || !view.Final || view.Identity.Epoch != 0 {
		t.Fatalf("final observation changed outcome: status %q final %t epoch %d", view.Status, view.Final, view.Identity.Epoch)
	}
}

func schema2FinalArtifactObservationInput(t *testing.T, epoch uint64) CanonicalRunReadInput {
	t.Helper()
	attached := schema2RepairArtifactAttachedData()
	attached["source"] = map[string]any{"kind": "system", "sourceId": "projection_system"}
	observed := schema2Event(projectionTestRunID, 5, "artifact_observed", map[string]any{
		"artifactId": "artifact_report", "availability": "unavailable", "errorCode": "artifact_unavailable",
		"observedAt": "2026-07-20T10:00:19Z", "relatedSeq": uint64(4),
	})
	observed["epoch"] = epoch
	return schema2ProjectionInput(t, true,
		schema2Event(projectionTestRunID, 3, "artifact_attached", attached),
		schema2Event(projectionTestRunID, 4, "run_succeeded", schema2TerminalSucceededData()),
		observed,
	)
}

func schema2LifecycleStartType(lifecycle string) string {
	if lifecycle == "cancel" {
		return "run_cancel_requested"
	}
	return "run_failure_reconciliation_started"
}

func schema2LifecycleFinalType(lifecycle string) string {
	if lifecycle == "cancel" {
		return "run_canceled"
	}
	return "run_failed"
}

func schema2LifecycleStartData(lifecycle string) map[string]any {
	if lifecycle == "cancel" {
		return schema2TerminalCancelRequestedData()
	}
	return schema2TerminalFailureStartedData(0)
}

func schema2LifecycleFinalData(lifecycle string, predecessor uint64) map[string]any {
	if lifecycle == "cancel" {
		return schema2TerminalCanceledData(predecessor)
	}
	return schema2TerminalFailedData(predecessor)
}

func schema2InventedOpenSnapshot(t *testing.T, lifecycle, resource string) (string, any) {
	t.Helper()
	switch resource {
	case "node":
		return "openNodeAttempts", schema2RepairNodeAttemptSnapshot()
	case "dispatch":
		return "openSlotDispatches", schema2RepairOpenDispatchSnapshot(t)
	case "tool":
		return "openToolLeases", schema2RepairToolLeaseSnapshot(lifecycle == "failure")
	default:
		t.Fatalf("unknown snapshot resource %q", resource)
		return "", nil
	}
}

func schema2InventedDisposition(t *testing.T, lifecycle, resource string) (string, any) {
	t.Helper()
	switch resource {
	case "node":
		node := schema2RepairNodeAttemptSnapshot()
		if lifecycle == "cancel" {
			node["disposition"] = "canceled_non_authorizing"
		} else {
			node["disposition"] = "abandoned_non_authorizing"
		}
		return "nodeAttemptDispositions", node
	case "dispatch":
		disposition := "canceled_non_authorizing"
		if lifecycle == "failure" {
			disposition = "abandoned_non_authorizing"
		}
		return "slotDispatchDispositions", schema2RepairSlotDisposition(t, disposition)
	case "tool":
		tool := schema2RepairToolLeaseSnapshot(lifecycle == "failure")
		if lifecycle == "cancel" {
			tool["disposition"] = "never_launched_cleaned"
			return "reconciledToolLeases", tool
		}
		tool["disposition"] = "abandoned_private_cleanup_owned"
		return "toolLeaseDispositions", tool
	default:
		t.Fatalf("unknown disposition resource %q", resource)
		return "", nil
	}
}

func schema2FailureHeaderState(t *testing.T, errorCause bool) (projectionState, map[string]any) {
	t.Helper()
	state := schema2EpochTestState()
	if err := schema2EpochReduce(t, &state, 19, 0, "error", schema2SecondRepairFixture(t, "error")); err != nil {
		t.Fatalf("prepare failure provenance: %v", err)
	}
	start := schema2TerminalFailureStartedData(0)
	start["relatedSeq"] = uint64(19)
	if errorCause {
		start["failureCause"] = map[string]any{"kind": "error", "errorSeq": uint64(19)}
	}
	if err := schema2EpochReduce(t, &state, 20, 0, "run_failure_reconciliation_started", start); err != nil {
		t.Fatalf("prepare exact failure start: %v", err)
	}
	return state, start
}

func schema2FailureFinalFromStart(start map[string]any, sequence uint64) map[string]any {
	failed := schema2TerminalFailedData(sequence)
	for _, field := range []string{"code", "reason", "unrecoverable", "relatedSeq", "failureCause"} {
		failed[field] = cloneAny(start[field])
	}
	return failed
}

func schema2LifecycleReconciliationState(t *testing.T, lifecycle string) projectionState {
	t.Helper()
	var state projectionState
	if lifecycle == "cancel" {
		state = schema2TerminalCancelingState(t)
	} else {
		state = schema2TerminalFailingState(t)
	}
	data := schema2SecondRepairFixture(t, "slot_reconciliation_interrupt")
	dispatchID := data["dispatchId"].(string)
	state.view.Sessions = []RunSessionView{{
		DispatchID: dispatchID,
		Occupancy:  RunSessionOccupancy{State: "active"},
	}}
	return state
}

func schema2RequireReducerErrorWithoutMutation(t *testing.T, state *projectionState, sequence uint64, eventType string, data map[string]any) {
	t.Helper()
	before := schema2ReducerStateFingerprint(t, state)
	err := schema2EpochReduce(t, state, sequence, state.view.Identity.Epoch, eventType, data)
	if err == nil {
		t.Fatalf("%s admitted invalid lifecycle evidence", eventType)
	}
	requireProjectionError(t, err, ErrRunProjectionInvalid)
	after := schema2ReducerStateFingerprint(t, state)
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected %s mutated reducer state\nbefore: %s\nafter:  %s", eventType, before, after)
	}
}

func schema2RequireRawReducerErrorWithoutMutation(t *testing.T, state *projectionState, raw rawProjectionEvent, safe SafeRunEvent) {
	t.Helper()
	before := schema2ReducerStateFingerprint(t, state)
	err := reduceSchema2Event(state, raw, safe, nil)
	if err == nil {
		t.Fatalf("%T admitted invalid lifecycle evidence", safe)
	}
	requireProjectionError(t, err, ErrRunProjectionInvalid)
	after := schema2ReducerStateFingerprint(t, state)
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected %T mutated reducer state\nbefore: %s\nafter:  %s", safe, before, after)
	}
}

func schema2ReducerStateFingerprint(t *testing.T, state *projectionState) []byte {
	t.Helper()
	return mustMarshalJSON(t, struct {
		View               RunView
		NodeIndex          map[string]int
		AttemptIndex       map[string]int
		GateIndex          map[string]int
		ArtifactIndex      map[string]int
		Dispatches         map[string]SafeSchema2SlotDispatchData
		DispatchSeq        map[string]uint64
		MatchedDispatch    map[string]bool
		RevokedDispatch    map[string]uint64
		Bindings           map[string]schema2Binding
		Health             map[string]SafeSchema2SlotBindingObservedData
		LastBlockJSON      []byte
		LastBlockRetry     []byte
		LastBlockSeq       uint64
		LastBlockPolicy    string
		LastBlockNextEpoch uint64
		CancelRequestSeq   uint64
		FailureStartSeq    uint64
		Terminal           bool
	}{
		View: state.view, NodeIndex: state.nodeIndex, AttemptIndex: state.attemptIndex,
		GateIndex: state.gateIndex, ArtifactIndex: state.artifactIndex,
		Dispatches: state.dispatches, DispatchSeq: state.dispatchSeq,
		MatchedDispatch: state.matchedDispatch, RevokedDispatch: state.revokedDispatch,
		Bindings: state.bindings, Health: state.health,
		LastBlockJSON: state.lastBlockJSON, LastBlockRetry: state.lastBlockRetry,
		LastBlockSeq: state.lastBlockSeq, LastBlockPolicy: state.lastBlockPolicy,
		LastBlockNextEpoch: state.lastBlockNextEpoch,
		CancelRequestSeq:   state.cancelRequestSeq, FailureStartSeq: state.failureStartSeq,
		Terminal: state.terminal,
	})
}
