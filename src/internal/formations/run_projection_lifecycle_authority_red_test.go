package formations

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const schema2TerminalETXInterruptSHA256 = "084fed08b978af4d7d196a7446a86b58009e636b611db16211b65a9aadff29c5"

// These tests remain inside Task 1's public ledger vocabulary. In particular,
// they do not define or consume a nonempty schema-2 run-private-state record.
// They cover structural lifecycle exactness only; recovery predicates, action
// derivation, and reconcile conditions remain Task 2.
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

	t.Run("wrong_waiting_verdict_request_sequence_is_rejected_without_mutation", func(t *testing.T) {
		state := schema2TerminalWaitingState(t)
		data := schema2RepairHumanVerdictData()
		data["requestedSeq"] = uint64(20)
		schema2RequireReducerErrorWithoutMutation(t, &state, 22, "human_verdict_recorded", data)
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
		projection, err := ProjectCanonicalRun(schema2FinalBindingObservationInput(t))
		if err != nil {
			t.Fatalf("current-epoch final binding observation rejected: %v", err)
		}
		view := ProjectRunView(projection)
		schema2RequireSuccessfulFinalOutcome(t, view)
		page, err := ProjectRunEventPage(projection, 3, 1)
		if err != nil {
			t.Fatalf("page final binding observation: %v", err)
		}
		if len(page.Events) != 1 {
			t.Fatalf("final binding observation page = %#v", page.Events)
		}
		observed, ok := page.Events[0].(SafeSchema2SlotBindingObservedEvent)
		if !ok || observed.Data.BindingID != "binding_worker" || observed.Data.SlotID != "slot_worker" || observed.Data.SessionTargetID != "target_worker" {
			t.Fatalf("final binding observation = %#v", page.Events[0])
		}
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
			t.Run(lifecycle+"_exact_nonempty_"+resource+"_snapshot_and_disposition_are_valid", func(t *testing.T) {
				state, open := schema2LifecyclePublicState(t, resource)
				if err := schema2EpochReduce(t, &state, 20, 0, schema2LifecycleStartType(lifecycle), schema2LifecycleStartWithOpen(lifecycle, open)); err != nil {
					t.Fatalf("exact nonempty %s %s start rejected: %v", lifecycle, resource, err)
				}
				if len(open["openSlotDispatches"]) != 0 {
					schema2LifecycleApplyExactCleanup(t, &state, lifecycle)
					session := state.sessionByDispatch("dsp_01KXNP6VY3227H78329V52CKF8")
					if session == nil || session.Occupancy.State != "held" {
						t.Fatalf("exact %s cleanup outcome = %#v", lifecycle, session)
					}
				}
				finalSeq := uint64(21)
				if len(open["openSlotDispatches"]) != 0 {
					finalSeq = 24
				}
				final := schema2LifecycleFinalWithDispositions(t, lifecycle, 20, open)
				if err := schema2EpochReduce(t, &state, finalSeq, 0, schema2LifecycleFinalType(lifecycle), final); err != nil {
					t.Fatalf("exact nonempty %s %s final rejected: %v", lifecycle, resource, err)
				}
				if !state.view.Final || state.view.Status != map[string]string{"cancel": "canceled", "failure": "failed"}[lifecycle] {
					t.Fatalf("exact nonempty %s %s final = %q/%t", lifecycle, resource, state.view.Status, state.view.Final)
				}
				schema2RequireLifecycleProjectedResource(t, &state, lifecycle, resource, finalSeq)
			})

			for _, mutation := range []string{"missing", "duplicate", "extra", "identity_changed"} {
				t.Run(lifecycle+"_start_rejects_"+mutation+"_"+resource+"_snapshot_without_mutation", func(t *testing.T) {
					state, open := schema2LifecyclePublicState(t, resource)
					field := schema2LifecycleOpenField(resource)
					open[field] = schema2LifecycleMutatedMembers(t, resource, open[field], mutation)
					data := schema2LifecycleStartWithOpen(lifecycle, open)
					if resource == "dispatch" && mutation == "duplicate" {
						// Duplicate object identities are rejected by the current
						// sanitizer before the reducer can classify the exact-set
						// mismatch. Either public typed rejection is contract-valid;
						// an untyped error or state mutation is not.
						schema2RequireTypedReducerErrorWithoutMutation(t, &state, 20, schema2LifecycleStartType(lifecycle), data)
						return
					}
					schema2RequireReducerErrorWithoutMutation(t, &state, 20, schema2LifecycleStartType(lifecycle), data)
				})

				t.Run(lifecycle+"_final_rejects_"+mutation+"_"+resource+"_disposition_without_mutation", func(t *testing.T) {
					state, open := schema2LifecyclePublicState(t, resource)
					if err := schema2EpochReduce(t, &state, 20, 0, schema2LifecycleStartType(lifecycle), schema2LifecycleStartWithOpen(lifecycle, open)); err != nil {
						t.Fatalf("prepare exact nonempty %s %s start: %v", lifecycle, resource, err)
					}
					if len(open["openSlotDispatches"]) != 0 {
						schema2LifecycleApplyExactCleanup(t, &state, lifecycle)
					}
					final := schema2LifecycleFinalWithDispositions(t, lifecycle, 20, open)
					field := schema2LifecycleDispositionField(resource, lifecycle)
					final[field] = schema2LifecycleMutatedMembers(t, resource, final[field].([]any), mutation)
					finalSeq := uint64(21)
					if len(open["openSlotDispatches"]) != 0 {
						finalSeq = 24
					}
					schema2RequireReducerErrorWithoutMutation(t, &state, finalSeq, schema2LifecycleFinalType(lifecycle), final)
				})
			}
		}
	}

	t.Run("failure_cause_selects_one_open_attempt_and_abandons_collateral", func(t *testing.T) {
		state := schema2EpochTestState()
		open := map[string][]any{"openNodeAttempts": {}, "openSlotDispatches": {}, "openToolLeases": {}}
		for _, node := range []struct {
			sequence uint64
			nodeID   string
			kind     string
		}{
			{sequence: 3, nodeID: projectionTestMissionID, kind: "mission"},
			{sequence: 4, nodeID: projectionTestFormationID, kind: "formation"},
		} {
			started := map[string]any{
				"nodeId": node.nodeID, "nodeKind": node.kind, "attempt": uint64(1),
				"reason": "initial", "inputRefs": []any{},
			}
			if err := schema2LifecycleReduce(t, &state, node.sequence, "node_started", started); err != nil {
				t.Fatalf("prepare %s selected/collateral attempt: %v", node.kind, err)
			}
			open["openNodeAttempts"] = append(open["openNodeAttempts"], map[string]any{
				"nodeId": node.nodeID, "nodeKind": node.kind, "attempt": uint64(1),
				"startSeq": node.sequence, "phase": "started", "phaseSeq": node.sequence,
			})
		}
		failure := schema2SecondRepairFixture(t, "error")
		failure["errorScope"] = "node"
		failure["nodeId"] = projectionTestFormationID
		failure["relatedSeq"] = uint64(4)
		if err := schema2LifecycleReduce(t, &state, 5, "error", failure); err != nil {
			t.Fatalf("prepare exact selected attempt error: %v", err)
		}
		start := schema2LifecycleStartWithOpen("failure", open)
		start["relatedSeq"] = uint64(5)
		start["failureCause"] = map[string]any{"kind": "error", "errorSeq": uint64(5)}
		if err := schema2LifecycleReduce(t, &state, 20, "run_failure_reconciliation_started", start); err != nil {
			t.Fatalf("prepare selected/collateral failure snapshot: %v", err)
		}
		final := schema2LifecycleFinalWithDispositions(t, "failure", 20, open)
		for _, field := range []string{"code", "reason", "unrecoverable", "relatedSeq", "failureCause"} {
			final[field] = cloneAny(start[field])
		}
		for _, member := range final["nodeAttemptDispositions"].([]any) {
			disposition := member.(map[string]any)
			if disposition["nodeId"] == projectionTestFormationID {
				disposition["disposition"] = "failed_non_authorizing"
			}
		}
		if err := schema2LifecycleReduce(t, &state, 21, "run_failed", final); err != nil {
			t.Fatalf("exact selected/collateral failure final rejected: %v", err)
		}
		if !state.view.Final || state.view.Status != "failed" {
			t.Fatalf("selected/collateral run final = %q/%t", state.view.Status, state.view.Final)
		}
		schema2RequireNodeAttemptDisposition(t, &state, projectionTestFormationID, "failed", 21)
		schema2RequireNodeAttemptDisposition(t, &state, projectionTestMissionID, "abandoned", 21)
	})

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
		for _, revocation := range []struct {
			name   string
			mutate func(map[string]any)
		}{
			{name: "wrong_target_lease", mutate: func(data map[string]any) { data["targetLeaseId"] = "lease_01KXNP6VY3227H78329V52CKF7" }},
			{name: "wrong_binding", mutate: func(data map[string]any) { data["bindingId"] = "binding_other" }},
			{name: "wrong_session_target", mutate: func(data map[string]any) { data["sessionTargetId"] = "target_other" }},
			{name: "wrong_target_fingerprint", mutate: func(data map[string]any) {
				data["targetFingerprint"] = strings.Repeat("f", 64)
			}},
			{name: "wrong_lifecycle_reason", mutate: func(data map[string]any) { data["reason"] = "result_closure" }},
		} {
			t.Run(lifecycle+"_revocation_rejects_"+revocation.name+"_without_mutation", func(t *testing.T) {
				state, exact := schema2LifecyclePreRevocationState(t, lifecycle)
				revocation.mutate(exact)
				schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 21, "slot_peek_capability_revoked", exact)
			})
		}

		t.Run(lifecycle+"_exact_issued_and_closed_steering_snapshot_is_publicly_valid", func(t *testing.T) {
			state, open := schema2LifecycleIssuedSteeringState(t)
			start := schema2LifecycleStartWithOpen(lifecycle, open)
			if err := schema2EpochReduce(t, &state, 20, 0, schema2LifecycleStartType(lifecycle), start); err != nil {
				t.Fatalf("exact %s issued/closed steering snapshot rejected: %v", lifecycle, err)
			}
		})

		for _, lineage := range []struct {
			name   string
			mutate func(map[string]any)
		}{
			{name: "wrong_capability_generation", mutate: func(data map[string]any) { data["capabilityGeneration"] = "2" }},
			{name: "wrong_capability_issued_sequence", mutate: func(data map[string]any) { data["capabilityIssuedSeq"] = uint64(5) }},
			{name: "wrong_steering_generation", mutate: func(data map[string]any) { data["steeringGeneration"] = "2" }},
		} {
			t.Run(lifecycle+"_revocation_rejects_"+lineage.name+"_without_mutation", func(t *testing.T) {
				state, exact := schema2LifecycleIssuedPreRevocationState(t, lifecycle)
				lineage.mutate(exact)
				schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 21, "slot_peek_capability_revoked", exact)
			})
		}

		t.Run(lifecycle+"_exact_issued_revocation_lineage_is_valid", func(t *testing.T) {
			state, exact := schema2LifecycleIssuedPreRevocationState(t, lifecycle)
			if err := schema2LifecycleReduce(t, &state, 21, "slot_peek_capability_revoked", exact); err != nil {
				t.Fatalf("exact %s issued revocation rejected: %v", lifecycle, err)
			}
			session := state.sessionByDispatch("dsp_01KXNP6VY3227H78329V52CKF8")
			if session == nil || session.PeekCapability.State != "revoked" || session.PeekCapability.Generation != "1" || session.PeekCapability.IssuedSeq != 6 || session.Steering.State != "closed" || session.Steering.Generation != "1" || state.revokedDispatch[session.DispatchID] != 21 {
				t.Fatalf("exact %s issued revocation projection = %#v / revoked=%#v", lifecycle, session, state.revokedDispatch)
			}
		})

		t.Run(lifecycle+"_exact_nonzero_issued_cleanup_and_final_are_valid", func(t *testing.T) {
			state, final := schema2LifecycleIssuedFinalState(t, lifecycle)
			if err := schema2EpochReduce(t, &state, 24, 0, schema2LifecycleFinalType(lifecycle), final); err != nil {
				t.Fatalf("exact nonzero %s final rejected: %v", lifecycle, err)
			}
			if !state.view.Final || state.view.Status != map[string]string{"cancel": "canceled", "failure": "failed"}[lifecycle] {
				t.Fatalf("exact nonzero %s run final = %q/%t", lifecycle, state.view.Status, state.view.Final)
			}
			schema2RequireNodeAttemptDisposition(t, &state, projectionTestFormationID, map[string]string{"cancel": "canceled", "failure": "abandoned"}[lifecycle], 24)
			session := state.sessionByDispatch("dsp_01KXNP6VY3227H78329V52CKF8")
			if session == nil || session.Occupancy.State != "quarantined" || session.PeekCapability.State != "revoked" || session.PeekCapability.Generation != "1" || session.PeekCapability.IssuedSeq != 6 || session.Steering.State != "closed" || session.Steering.Generation != "1" || session.Steering.StartedSeq != nil {
				t.Fatalf("exact nonzero %s final session = %#v", lifecycle, session)
			}
		})

		for _, finalMutation := range []struct {
			name   string
			mutate func(map[string]any)
		}{
			{name: "wrong_nonzero_preserved_peek_capability_state", mutate: func(data map[string]any) { data["peekCapabilityState"] = "revoked" }},
			{name: "wrong_nonzero_preserved_capability_generation", mutate: func(data map[string]any) { data["latestCapabilityGeneration"] = "2" }},
			{name: "wrong_nonzero_preserved_capability_issued_sequence", mutate: func(data map[string]any) { data["latestCapabilityIssuedSeq"] = uint64(5) }},
			{name: "wrong_nonzero_preserved_steering_generation", mutate: func(data map[string]any) { data["latestSteeringGeneration"] = "2" }},
			{name: "wrong_nonzero_final_capability_generation", mutate: func(data map[string]any) { data["finalCapabilityGeneration"] = "2" }},
			{name: "wrong_nonzero_final_capability_issued_sequence", mutate: func(data map[string]any) { data["finalCapabilityIssuedSeq"] = uint64(5) }},
			{name: "wrong_nonzero_final_steering_generation", mutate: func(data map[string]any) { data["finalSteeringGeneration"] = "2" }},
			{name: "wrong_nonzero_final_revocation_sequence", mutate: func(data map[string]any) { data["finalPeekCapabilityRevokedSeq"] = uint64(20) }},
		} {
			t.Run(lifecycle+"_typed_final_rejects_"+finalMutation.name+"_without_mutation", func(t *testing.T) {
				state, final := schema2LifecycleIssuedFinalState(t, lifecycle)
				dispositions := final["slotDispatchDispositions"].([]any)
				if len(dispositions) != 1 {
					t.Fatalf("exact nonzero %s final slot cardinality = %d", lifecycle, len(dispositions))
				}
				finalMutation.mutate(dispositions[0].(map[string]any))
				// The typed final path isolates reducer exactness from the known
				// public issued-arm decoder defect. The public exact positive above
				// retains end-to-end decoder coverage.
				schema2RequireTypedLifecycleFinalErrorWithoutMutation(t, &state, 24, lifecycle, final)
			})
		}

		for _, authority := range []struct {
			name   string
			mutate func(map[string]any)
		}{
			{name: "wrong_authority_sequence", mutate: func(data map[string]any) { data["authoritySeq"] = uint64(19) }},
			{name: "wrong_authority_kind", mutate: func(data map[string]any) {
				data["authorityKind"] = map[string]string{"cancel": "failure", "failure": "cancel"}[lifecycle]
			}},
			{name: "wrong_target_lease", mutate: func(data map[string]any) { data["targetLeaseId"] = "lease_01KXNP6VY3227H78329V52CKF7" }},
			{name: "wrong_binding", mutate: func(data map[string]any) { data["bindingId"] = "binding_other" }},
			{name: "wrong_session_target", mutate: func(data map[string]any) { data["sessionTargetId"] = "target_other" }},
			{name: "wrong_target_fingerprint", mutate: func(data map[string]any) {
				data["targetFingerprint"] = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
			}},
			{name: "wrong_interrupt_semantic_hash", mutate: func(data map[string]any) {
				data["interruptSha256"] = strings.Repeat("b", 64)
			}},
		} {
			t.Run(lifecycle+"_cleanup_rejects_"+authority.name+"_without_mutation", func(t *testing.T) {
				state, interrupt := schema2LifecycleCleanupState(t, lifecycle)
				authority.mutate(interrupt)
				schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 22, "slot_reconciliation_interrupt", interrupt)
			})
		}

		t.Run(lifecycle+"_cleanup_rejects_dispatch_outside_frozen_membership_without_mutation", func(t *testing.T) {
			state, _ := schema2LifecycleCleanupState(t, lifecycle)
			interrupt := schema2LifecycleInjectUnsnapshottedDispatch(t, &state, lifecycle)
			schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 22, "slot_reconciliation_interrupt", interrupt)
		})

		for _, outcomeMutation := range []struct {
			name   string
			mutate func(map[string]any)
		}{
			{name: "wrong_requested_sequence", mutate: func(data map[string]any) { data["requestedSeq"] = uint64(21) }},
			{name: "wrong_target_lease", mutate: func(data map[string]any) { data["targetLeaseId"] = "lease_01KXNP6VY3227H78329V52CKF7" }},
			{name: "wrong_target_fingerprint", mutate: func(data map[string]any) { data["targetFingerprint"] = strings.Repeat("f", 64) }},
		} {
			t.Run(lifecycle+"_cleanup_outcome_rejects_"+outcomeMutation.name+"_without_mutation", func(t *testing.T) {
				state, interrupt := schema2LifecycleCleanupState(t, lifecycle)
				if err := schema2LifecycleReduce(t, &state, 22, "slot_reconciliation_interrupt", interrupt); err != nil {
					t.Fatalf("prepare exact %s cleanup request: %v", lifecycle, err)
				}
				outcome := schema2LifecycleInterruptOutcomeData()
				outcomeMutation.mutate(outcome)
				schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 23, "slot_reconciliation_interrupt_outcome", outcome)
			})
		}

		t.Run(lifecycle+"_duplicate_revocation_is_rejected_without_mutation", func(t *testing.T) {
			state, exact := schema2LifecyclePreRevocationState(t, lifecycle)
			if err := schema2LifecycleReduce(t, &state, 21, "slot_peek_capability_revoked", exact); err != nil {
				t.Fatalf("prepare first exact %s revocation: %v", lifecycle, err)
			}
			// The payload is byte-equivalent; only the repeated event envelope
			// sequence changes from 21 to 22.
			schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 22, "slot_peek_capability_revoked", exact)
		})

		t.Run(lifecycle+"_duplicate_interrupt_request_is_rejected_without_mutation", func(t *testing.T) {
			state, exact := schema2LifecycleCleanupState(t, lifecycle)
			if err := schema2LifecycleReduce(t, &state, 22, "slot_reconciliation_interrupt", exact); err != nil {
				t.Fatalf("prepare first exact %s interrupt request: %v", lifecycle, err)
			}
			// The payload is byte-equivalent; only the repeated event envelope
			// sequence changes from 22 to 23.
			schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 23, "slot_reconciliation_interrupt", exact)
		})

		t.Run(lifecycle+"_duplicate_interrupt_outcome_is_rejected_without_mutation", func(t *testing.T) {
			state, interrupt := schema2LifecycleCleanupState(t, lifecycle)
			if err := schema2LifecycleReduce(t, &state, 22, "slot_reconciliation_interrupt", interrupt); err != nil {
				t.Fatalf("prepare exact %s interrupt request: %v", lifecycle, err)
			}
			exact := schema2LifecycleInterruptOutcomeData()
			if err := schema2LifecycleReduce(t, &state, 23, "slot_reconciliation_interrupt_outcome", exact); err != nil {
				t.Fatalf("prepare first exact %s interrupt outcome: %v", lifecycle, err)
			}
			// The payload is byte-equivalent; only the repeated event envelope
			// sequence changes from 23 to 24.
			schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 24, "slot_reconciliation_interrupt_outcome", exact)
		})

		for _, finalMutation := range []struct {
			name   string
			mutate func(map[string]any)
		}{
			{name: "wrong_soft_interrupt_outcome", mutate: func(data map[string]any) { data["softInterrupt"] = "unavailable" }},
			{name: "wrong_soft_interrupt_requested_sequence", mutate: func(data map[string]any) { data["softInterruptRequestedSeq"] = uint64(21) }},
			{name: "wrong_soft_interrupt_outcome_sequence", mutate: func(data map[string]any) { data["softInterruptOutcomeSeq"] = uint64(22) }},
			{name: "wrong_final_capability_generation", mutate: func(data map[string]any) { data["finalCapabilityGeneration"] = "1" }},
			{name: "wrong_final_capability_issued_sequence", mutate: func(data map[string]any) { data["finalCapabilityIssuedSeq"] = uint64(1) }},
			{name: "wrong_final_steering_generation", mutate: func(data map[string]any) { data["finalSteeringGeneration"] = "1" }},
			{name: "wrong_final_revocation_sequence", mutate: func(data map[string]any) { data["finalPeekCapabilityRevokedSeq"] = uint64(20) }},
			{name: "wrong_preserved_request_steering_generation", mutate: func(data map[string]any) { data["latestSteeringGeneration"] = "1" }},
		} {
			t.Run(lifecycle+"_final_slot_disposition_rejects_"+finalMutation.name+"_without_mutation", func(t *testing.T) {
				state, final := schema2LifecycleFinalSlotState(t, lifecycle)
				dispositions := final["slotDispatchDispositions"].([]any)
				if len(dispositions) != 1 {
					t.Fatalf("exact %s final slot disposition cardinality = %d", lifecycle, len(dispositions))
				}
				finalMutation.mutate(dispositions[0].(map[string]any))
				schema2RequireReducerErrorWithoutMutation(t, &state, 24, schema2LifecycleFinalType(lifecycle), final)
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

func schema2FinalBindingObservationInput(t *testing.T) CanonicalRunReadInput {
	t.Helper()
	input, _ := schema2OpenDispatchLifecycleInput(t, false)
	events := canonicalLedgerEvents(t, input)
	observed := schema2FormationEvent(projectionTestRunID, 4, "slot_binding_observed", schema2SecondRepairFixture(t, "slot_binding_observed"))
	observed["nodeId"] = projectionTestFormationID
	observed["slotId"] = "slot_worker"
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t,
		events[0],
		events[1],
		schema2FormationEvent(projectionTestRunID, 3, "run_succeeded", schema2TerminalSucceededData()),
		observed,
	))
	documents := input.Documents[:0]
	for _, document := range input.Documents {
		if document.Role != CanonicalInputRoleSchema2CommandRecord {
			documents = append(documents, document)
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(document.Bytes, &record); err != nil {
			t.Fatalf("decode command record: %v", err)
		}
		if record["commandId"] == projectionTestCommandID {
			documents = append(documents, document)
		}
	}
	input.Documents = documents
	return input
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

func schema2LifecyclePublicState(t *testing.T, resource string) (projectionState, map[string][]any) {
	t.Helper()
	open := map[string][]any{
		"openNodeAttempts":   {},
		"openSlotDispatches": {},
		"openToolLeases":     {},
	}
	state := schema2EpochTestState()
	prepareNode := func(nodeID, nodeKind string) map[string]any {
		data := map[string]any{
			"nodeId": nodeID, "nodeKind": nodeKind, "attempt": uint64(1),
			"reason": "initial", "inputRefs": []any{},
		}
		if err := schema2LifecycleReduce(t, &state, 3, "node_started", data); err != nil {
			t.Fatalf("prepare open %s attempt: %v", nodeKind, err)
		}
		return map[string]any{
			"nodeId": nodeID, "nodeKind": nodeKind, "attempt": uint64(1),
			"startSeq": uint64(3), "phase": "started", "phaseSeq": uint64(3),
		}
	}

	switch resource {
	case "node":
		open["openNodeAttempts"] = []any{prepareNode(projectionTestMissionID, "mission")}
	case "dispatch":
		open["openNodeAttempts"] = []any{prepareNode(projectionTestFormationID, "formation")}
		state.bindings["binding_worker"] = schema2Binding{
			BindingID: "binding_worker", NodeID: projectionTestFormationID, SlotID: "slot_worker",
			AgentID: "worker", Harness: "codex", SessionTargetID: "target_worker",
			TargetFingerprint: strings.Repeat("a", 64), SessionLineageSHA256: strings.Repeat("c", 64),
		}
		observed := schema2SecondRepairFixture(t, "slot_binding_observed")
		observed["relatedSeq"] = uint64(3)
		if err := schema2LifecycleReduce(t, &state, 4, "slot_binding_observed", observed); err != nil {
			t.Fatalf("prepare exact binding observation: %v", err)
		}
		dispatch := schema2RepairSlotDispatchData(t)
		if err := schema2LifecycleReduce(t, &state, 5, "slot_dispatch", dispatch); err != nil {
			t.Fatalf("prepare unmatched dispatch: %v", err)
		}
		current := state.currentSchema2OpenDispatches()
		if len(current) != 1 {
			t.Fatalf("prepared open dispatches = %#v", current)
		}
		open["openSlotDispatches"] = []any{schema2LifecycleMap(t, current[0])}
	case "tool":
		// I5 holds the launched-Tool/private-cleanup shape for ctx-ug7.6.1.
		// This public control is deliberately the already-frozen unlaunched lease.
		board := &BoardDocument{
			Missions: []MissionNode{{ID: projectionTestMissionID}}, Formations: []FormationNode{{ID: projectionTestFormationID}},
			Gates: []GateNode{{ID: projectionTestGateID}}, Tools: []ToolNode{{ID: "tool_normalize"}},
		}
		state = newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, board)
		state.view.Status = "running"
		state.view.Identity.Epoch = 0
		open["openNodeAttempts"] = []any{prepareNode("tool_normalize", "tool")}
		if err := schema2LifecycleReduce(t, &state, 4, "tool_dispatch", schema2RepairToolDispatchData()); err != nil {
			t.Fatalf("prepare unlaunched Tool lease: %v", err)
		}
		open["openToolLeases"] = []any{map[string]any{
			"toolLeaseId": "toollease_01KXNP6VY3227H78329V52CKF8", "nodeId": "tool_normalize",
			"attempt": uint64(1), "dispatchSeq": uint64(4),
		}}
	default:
		t.Fatalf("unknown public lifecycle resource %q", resource)
	}
	return state, open
}

func schema2LifecycleStartWithOpen(lifecycle string, open map[string][]any) map[string]any {
	data := schema2LifecycleStartData(lifecycle)
	for _, field := range []string{"openNodeAttempts", "openSlotDispatches", "openToolLeases"} {
		data[field] = cloneAny(open[field])
	}
	return data
}

func schema2LifecycleFinalWithDispositions(t *testing.T, lifecycle string, predecessor uint64, open map[string][]any) map[string]any {
	t.Helper()
	data := schema2LifecycleFinalData(lifecycle, predecessor)
	nodes := make([]any, 0, len(open["openNodeAttempts"]))
	for _, member := range open["openNodeAttempts"] {
		node := cloneAny(member).(map[string]any)
		node["disposition"] = map[string]string{"cancel": "canceled_non_authorizing", "failure": "abandoned_non_authorizing"}[lifecycle]
		nodes = append(nodes, node)
	}
	dispatches := make([]any, 0, len(open["openSlotDispatches"]))
	for _, member := range open["openSlotDispatches"] {
		disposition := cloneAny(member).(map[string]any)
		disposition["disposition"] = map[string]string{"cancel": "canceled_non_authorizing", "failure": "abandoned_non_authorizing"}[lifecycle]
		disposition["softInterrupt"] = "sent"
		disposition["softInterruptRequestedSeq"] = uint64(22)
		disposition["softInterruptOutcomeSeq"] = uint64(23)
		disposition["targetLeaseState"] = "quarantined"
		disposition["finalPeekCapabilityState"] = "revoked"
		disposition["finalCapabilityGeneration"] = "0"
		disposition["finalCapabilityIssuedSeq"] = uint64(0)
		disposition["finalSteeringGeneration"] = "0"
		disposition["finalPeekCapabilityRevokedSeq"] = uint64(21)
		dispatches = append(dispatches, disposition)
	}
	tools := make([]any, 0, len(open["openToolLeases"]))
	for _, member := range open["openToolLeases"] {
		tool := cloneAny(member).(map[string]any)
		tool["disposition"] = map[string]string{"cancel": "never_launched_cleaned", "failure": "abandoned_private_cleanup_owned"}[lifecycle]
		tools = append(tools, tool)
	}
	data["nodeAttemptDispositions"] = nodes
	data["slotDispatchDispositions"] = dispatches
	if lifecycle == "cancel" {
		data["reconciledToolLeases"] = tools
	} else {
		data["toolLeaseDispositions"] = tools
	}
	return data
}

func schema2RequireLifecycleProjectedResource(t *testing.T, state *projectionState, lifecycle, resource string, completedSeq uint64) {
	t.Helper()
	nodeID := map[string]string{
		"node": projectionTestMissionID, "dispatch": projectionTestFormationID, "tool": "tool_normalize",
	}[resource]
	want := map[string]string{"cancel": "canceled", "failure": "abandoned"}[lifecycle]
	schema2RequireNodeAttemptDisposition(t, state, nodeID, want, completedSeq)
	if resource != "dispatch" {
		return
	}
	session := state.sessionByDispatch("dsp_01KXNP6VY3227H78329V52CKF8")
	if session == nil {
		t.Fatal("final dispatch session is absent")
	}
	if session.Occupancy.State != "quarantined" || session.PeekCapability.State != "revoked" || session.PeekCapability.Generation != "0" || session.PeekCapability.IssuedSeq != 0 || session.Steering.State != "closed" || session.Steering.Generation != "0" || session.Steering.StartedSeq != nil {
		t.Fatalf("final %s dispatch session does not reflect its exact disposition: %#v", lifecycle, session)
	}
}

func schema2RequireNodeAttemptDisposition(t *testing.T, state *projectionState, nodeID, want string, completedSeq uint64) {
	t.Helper()
	node := state.node(nodeID)
	attempt := state.existingAttempt(nodeID, 1)
	if node == nil || attempt == nil {
		t.Fatalf("final resource %q absent: node=%#v attempt=%#v", nodeID, node, attempt)
	}
	if node.Status != want || node.FinalDisposition != want || attempt.Status != want || attempt.Disposition != want || attempt.CompletedSeq != completedSeq {
		t.Fatalf("final resource %q = node{%q,%q} attempt{%q,%q,completed=%d}, want %q at %d", nodeID, node.Status, node.FinalDisposition, attempt.Status, attempt.Disposition, attempt.CompletedSeq, want, completedSeq)
	}
}

func schema2LifecycleOpenField(resource string) string {
	return map[string]string{"node": "openNodeAttempts", "dispatch": "openSlotDispatches", "tool": "openToolLeases"}[resource]
}

func schema2LifecycleDispositionField(resource, lifecycle string) string {
	if resource == "node" {
		return "nodeAttemptDispositions"
	}
	if resource == "dispatch" {
		return "slotDispatchDispositions"
	}
	if lifecycle == "cancel" {
		return "reconciledToolLeases"
	}
	return "toolLeaseDispositions"
}

func schema2LifecycleMutatedMembers(t *testing.T, resource string, exact []any, mutation string) []any {
	t.Helper()
	if len(exact) != 1 {
		t.Fatalf("%s exact-set fixture cardinality = %d, want 1", resource, len(exact))
	}
	member := cloneAny(exact[0]).(map[string]any)
	changed := cloneAny(member).(map[string]any)
	switch resource {
	case "node":
		changed["attempt"] = uint64(2)
	case "dispatch":
		changed["dispatchId"] = "dsp_01KXNP6VY3227H78329V52CKF7"
	case "tool":
		changed["toolLeaseId"] = "toollease_01KXNP6VY3227H78329V52CKF7"
	default:
		t.Fatalf("unknown lifecycle mutation resource %q", resource)
	}
	switch mutation {
	case "missing":
		return []any{}
	case "duplicate":
		return []any{member, cloneAny(member)}
	case "extra":
		return []any{member, changed}
	case "identity_changed":
		return []any{changed}
	default:
		t.Fatalf("unknown lifecycle set mutation %q", mutation)
		return nil
	}
}

func schema2LifecycleCleanupState(t *testing.T, lifecycle string) (projectionState, map[string]any) {
	t.Helper()
	state, revoked := schema2LifecyclePreRevocationState(t, lifecycle)
	if err := schema2LifecycleReduce(t, &state, 21, "slot_peek_capability_revoked", revoked); err != nil {
		t.Fatalf("prepare exact %s capability revocation: %v", lifecycle, err)
	}
	return state, schema2LifecycleInterruptData(lifecycle)
}

func schema2LifecyclePreRevocationState(t *testing.T, lifecycle string) (projectionState, map[string]any) {
	t.Helper()
	state, open := schema2LifecyclePublicState(t, "dispatch")
	if err := schema2EpochReduce(t, &state, 20, 0, schema2LifecycleStartType(lifecycle), schema2LifecycleStartWithOpen(lifecycle, open)); err != nil {
		t.Fatalf("prepare exact %s cleanup snapshot: %v", lifecycle, err)
	}
	return state, schema2LifecycleRevocationData(lifecycle)
}

func schema2LifecycleIssuedSteeringState(t *testing.T) (projectionState, map[string][]any) {
	t.Helper()
	state, open := schema2LifecyclePublicState(t, "dispatch")
	issued := schema2SecondRepairFixture(t, "slot_peek_capability_issued")
	if err := schema2LifecycleReduce(t, &state, 6, "slot_peek_capability_issued", issued); err != nil {
		t.Fatalf("prepare issued capability: %v", err)
	}
	started := schema2SecondRepairFixture(t, "slot_steering_started")
	started["capabilityIssuedSeq"] = uint64(6)
	if err := schema2LifecycleReduce(t, &state, 7, "slot_steering_started", started); err != nil {
		t.Fatalf("prepare open steering: %v", err)
	}
	ended := schema2SecondRepairFixture(t, "slot_steering_ended")
	ended["startedSeq"] = uint64(7)
	if err := schema2LifecycleReduce(t, &state, 8, "slot_steering_ended", ended); err != nil {
		t.Fatalf("prepare closed steering: %v", err)
	}
	session := state.sessionByDispatch("dsp_01KXNP6VY3227H78329V52CKF8")
	if session == nil || session.PeekCapability.State != "issued" || session.PeekCapability.Generation != "1" || session.PeekCapability.IssuedSeq != 6 || session.Steering.State != "closed" || session.Steering.Generation != "1" {
		t.Fatalf("issued/closed steering prefix = %#v", session)
	}
	current := state.currentSchema2OpenDispatches()
	if len(current) != 1 || current[0].PeekCapabilityState != "issued" || current[0].LatestCapabilityGeneration != "1" || current[0].LatestCapabilityIssuedSeq != 6 || current[0].LatestSteeringGeneration != "1" {
		t.Fatalf("issued/closed steering snapshot = %#v", current)
	}
	open["openSlotDispatches"] = []any{schema2LifecycleMap(t, current[0])}
	return state, open
}

func schema2LifecycleIssuedPreRevocationState(t *testing.T, lifecycle string) (projectionState, map[string]any) {
	t.Helper()
	state, open := schema2LifecycleIssuedSteeringState(t)
	start := schema2LifecycleStartWithOpen(lifecycle, open)
	// The current public open-dispatch decoder still rejects the frozen
	// `issued` snapshot arm. Drive a closed typed reducer fixture here so each
	// lineage negative starts after an exact accepted authority event; the
	// public positive above separately keeps that decoder defect RED.
	if err := schema2LifecycleReduceTypedStart(t, &state, 20, lifecycle, start); err != nil {
		t.Fatalf("prepare typed exact %s issued snapshot: %v", lifecycle, err)
	}
	revoked := schema2LifecycleRevocationData(lifecycle)
	revoked["capabilityGeneration"] = "1"
	revoked["capabilityIssuedSeq"] = uint64(6)
	revoked["steeringGeneration"] = "1"
	return state, revoked
}

func schema2LifecycleIssuedFinalState(t *testing.T, lifecycle string) (projectionState, map[string]any) {
	t.Helper()
	state, open := schema2LifecycleIssuedSteeringState(t)
	start := schema2LifecycleStartWithOpen(lifecycle, open)
	if err := schema2LifecycleReduceTypedStart(t, &state, 20, lifecycle, start); err != nil {
		t.Fatalf("prepare typed exact %s nonzero snapshot: %v", lifecycle, err)
	}
	revoked := schema2LifecycleRevocationData(lifecycle)
	revoked["capabilityGeneration"] = "1"
	revoked["capabilityIssuedSeq"] = uint64(6)
	revoked["steeringGeneration"] = "1"
	if err := schema2LifecycleReduce(t, &state, 21, "slot_peek_capability_revoked", revoked); err != nil {
		t.Fatalf("prepare exact nonzero %s revocation: %v", lifecycle, err)
	}
	if err := schema2LifecycleReduce(t, &state, 22, "slot_reconciliation_interrupt", schema2LifecycleInterruptData(lifecycle)); err != nil {
		t.Fatalf("prepare exact nonzero %s interrupt request: %v", lifecycle, err)
	}
	if err := schema2LifecycleReduce(t, &state, 23, "slot_reconciliation_interrupt_outcome", schema2LifecycleInterruptOutcomeData()); err != nil {
		t.Fatalf("prepare exact nonzero %s interrupt outcome: %v", lifecycle, err)
	}
	final := schema2LifecycleFinalWithDispositions(t, lifecycle, 20, open)
	dispositions := final["slotDispatchDispositions"].([]any)
	if len(dispositions) != 1 {
		t.Fatalf("exact nonzero %s final slot cardinality = %d", lifecycle, len(dispositions))
	}
	disposition := dispositions[0].(map[string]any)
	disposition["finalCapabilityGeneration"] = "1"
	disposition["finalCapabilityIssuedSeq"] = uint64(6)
	disposition["finalSteeringGeneration"] = "1"
	disposition["finalPeekCapabilityRevokedSeq"] = uint64(21)
	return state, final
}

func schema2LifecycleReduceTypedStart(t *testing.T, state *projectionState, sequence uint64, lifecycle string, data map[string]any) error {
	t.Helper()
	eventType := schema2LifecycleStartType(lifecycle)
	event := schema2Event(projectionTestRunID, sequence, eventType, cloneAny(data).(map[string]any))
	event["epoch"] = state.view.Identity.Epoch
	raw, err := decodeProjectionEvent(canonicalJSON(t, event), CanonicalRunSourceSchema2, projectionTestRunID)
	if err != nil {
		return err
	}
	var safe SafeRunEvent
	encoded := mustMarshalJSON(t, data)
	var commands map[string]RunCommandReceipt
	if lifecycle == "cancel" {
		var typed SafeSchema2RunCancelRequestedData
		if err := json.Unmarshal(encoded, &typed); err != nil {
			t.Fatalf("decode typed cancellation start: %v", err)
		}
		safe = SafeSchema2RunCancelRequestedEvent{safeEventEnvelope: eventEnvelope(raw), Type: eventType, Data: typed}
		commands = map[string]RunCommandReceipt{
			typed.CommandID: RunCommandAppliedReceipt{
				CommandID: typed.CommandID, CommandPayloadSHA256: typed.CommandPayloadSHA256,
				CommandKind: "cancel", State: "applied", RunID: projectionTestRunID, EffectSeq: sequence,
			},
		}
	} else {
		var typed SafeSchema2RunFailureReconciliationStartedData
		if err := json.Unmarshal(encoded, &typed); err != nil {
			t.Fatalf("decode typed failure start: %v", err)
		}
		safe = SafeSchema2RunFailureReconciliationStartedEvent{safeEventEnvelope: eventEnvelope(raw), Type: eventType, Data: typed}
	}
	return reduceSchema2Event(state, raw, safe, commands)
}

func schema2LifecycleReduceTypedFinal(t *testing.T, state *projectionState, sequence uint64, lifecycle string, data map[string]any) error {
	t.Helper()
	eventType := schema2LifecycleFinalType(lifecycle)
	event := schema2Event(projectionTestRunID, sequence, eventType, cloneAny(data).(map[string]any))
	event["epoch"] = state.view.Identity.Epoch
	raw, err := decodeProjectionEvent(canonicalJSON(t, event), CanonicalRunSourceSchema2, projectionTestRunID)
	if err != nil {
		return err
	}
	encoded := mustMarshalJSON(t, data)
	var safe SafeRunEvent
	if lifecycle == "cancel" {
		var typed SafeSchema2RunCanceledData
		if err := json.Unmarshal(encoded, &typed); err != nil {
			t.Fatalf("decode typed cancellation final: %v", err)
		}
		safe = SafeSchema2RunCanceledEvent{safeEventEnvelope: eventEnvelope(raw), Type: eventType, Data: typed}
	} else {
		var typed SafeSchema2RunFailedData
		if err := json.Unmarshal(encoded, &typed); err != nil {
			t.Fatalf("decode typed failure final: %v", err)
		}
		safe = SafeSchema2RunFailedEvent{safeEventEnvelope: eventEnvelope(raw), Type: eventType, Data: typed}
	}
	return reduceSchema2Event(state, raw, safe, nil)
}

func schema2RequireTypedLifecycleFinalErrorWithoutMutation(t *testing.T, state *projectionState, sequence uint64, lifecycle string, data map[string]any) {
	t.Helper()
	before := schema2ReducerStateFingerprint(t, state)
	err := schema2LifecycleReduceTypedFinal(t, state, sequence, lifecycle, data)
	if err == nil {
		t.Fatalf("typed %s final admitted invalid nonzero lineage", lifecycle)
	}
	requireProjectionError(t, err, ErrRunProjectionInvalid)
	after := schema2ReducerStateFingerprint(t, state)
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected typed %s final mutated reducer state\nbefore: %s\nafter:  %s", lifecycle, before, after)
	}
}

func schema2LifecycleApplyExactCleanup(t *testing.T, state *projectionState, lifecycle string) {
	t.Helper()
	revoked := schema2LifecycleRevocationData(lifecycle)
	if err := schema2LifecycleReduce(t, state, 21, "slot_peek_capability_revoked", revoked); err != nil {
		t.Fatalf("exact %s capability revocation rejected: %v", lifecycle, err)
	}
	if err := schema2LifecycleReduce(t, state, 22, "slot_reconciliation_interrupt", schema2LifecycleInterruptData(lifecycle)); err != nil {
		t.Fatalf("exact %s reconciliation interrupt rejected: %v", lifecycle, err)
	}
	if err := schema2LifecycleReduce(t, state, 23, "slot_reconciliation_interrupt_outcome", schema2LifecycleInterruptOutcomeData()); err != nil {
		t.Fatalf("exact %s reconciliation outcome rejected: %v", lifecycle, err)
	}
}

func schema2LifecycleFinalSlotState(t *testing.T, lifecycle string) (projectionState, map[string]any) {
	t.Helper()
	state, open := schema2LifecyclePublicState(t, "dispatch")
	if err := schema2EpochReduce(t, &state, 20, 0, schema2LifecycleStartType(lifecycle), schema2LifecycleStartWithOpen(lifecycle, open)); err != nil {
		t.Fatalf("prepare exact %s final-slot snapshot: %v", lifecycle, err)
	}
	schema2LifecycleApplyExactCleanup(t, &state, lifecycle)
	return state, schema2LifecycleFinalWithDispositions(t, lifecycle, 20, open)
}

func schema2LifecycleRevocationData(lifecycle string) map[string]any {
	return map[string]any{
		"dispatchId": "dsp_01KXNP6VY3227H78329V52CKF8", "targetLeaseId": "lease_01KXNP6VY3227H78329V52CKF8",
		"bindingId": "binding_worker", "sessionTargetId": "target_worker", "targetFingerprint": strings.Repeat("a", 64),
		"capabilityGeneration": "0", "capabilityIssuedSeq": uint64(0), "steeringGeneration": "0",
		"reason":    map[string]string{"cancel": "cancel", "failure": "failure"}[lifecycle],
		"revokedAt": "2026-07-20T10:00:19Z", "inputClosed": true,
	}
}

func schema2LifecycleInterruptData(lifecycle string) map[string]any {
	return map[string]any{
		"dispatchId": "dsp_01KXNP6VY3227H78329V52CKF8", "targetLeaseId": "lease_01KXNP6VY3227H78329V52CKF8",
		"bindingId": "binding_worker", "sessionTargetId": "target_worker", "targetFingerprint": strings.Repeat("a", 64),
		"authorityKind": lifecycle, "authoritySeq": uint64(20), "interruptEncoding": "terminal-etx-v1",
		"interruptSha256": schema2TerminalETXInterruptSHA256, "recordedBeforeSend": true,
	}
}

func schema2LifecycleInterruptOutcomeData() map[string]any {
	return map[string]any{
		"requestedSeq": uint64(22), "dispatchId": "dsp_01KXNP6VY3227H78329V52CKF8",
		"targetLeaseId": "lease_01KXNP6VY3227H78329V52CKF8", "targetFingerprint": strings.Repeat("a", 64),
		"outcome": "sent", "observedAt": "2026-07-20T10:00:20Z",
	}
}

func schema2LifecycleInjectUnsnapshottedDispatch(t *testing.T, state *projectionState, lifecycle string) map[string]any {
	t.Helper()
	// The reducer state is deliberately reconstructed after the valid snapshot:
	// every target identity is exact, leaving frozen-snapshot membership as the
	// only invalid relation under test.
	const oldDispatchID = "dsp_01KXNP6VY3227H78329V52CKF8"
	const dispatchID = "dsp_01KXNP6VY3227H78329V52CKF7"
	const leaseID = "lease_01KXNP6VY3227H78329V52CKF7"
	dispatch := state.dispatches[oldDispatchID]
	binding := state.bindings[dispatch.BindingID]
	health := state.health[dispatch.BindingID]
	if binding.BindingID == "" || health.BindingID == "" {
		t.Fatalf("prepared dispatch lacks binding authority: binding=%#v health=%#v", binding, health)
	}
	dispatch.DispatchID = dispatchID
	dispatch.TargetLeaseID = leaseID
	dispatch.BindingID = "binding_intruder"
	dispatch.SessionTargetID = "target_intruder"
	dispatch.TargetFingerprint = strings.Repeat("f", 64)
	binding.BindingID = dispatch.BindingID
	binding.SessionTargetID = dispatch.SessionTargetID
	binding.TargetFingerprint = dispatch.TargetFingerprint
	health.BindingID = dispatch.BindingID
	health.SessionTargetID = dispatch.SessionTargetID
	state.bindings[dispatch.BindingID] = binding
	state.health[dispatch.BindingID] = health
	state.dispatches[dispatchID] = dispatch
	state.dispatchSeq[dispatchID] = 19
	state.revokedDispatch[dispatchID] = state.revokedDispatch[oldDispatchID]
	session := *state.sessionByDispatch(oldDispatchID)
	session.DispatchID = dispatchID
	session.TargetLeaseID = leaseID
	session.BindingID = "binding_intruder"
	session.SessionTargetID = "target_intruder"
	session.TargetFingerprintSHA256 = projectionSHA([]byte(dispatch.TargetFingerprint))
	state.view.Sessions = append(state.view.Sessions, session)
	if state.revokedDispatch[dispatchID] != 21 || session.PeekCapability.State != "revoked" || session.PeekCapability.Generation != "0" || session.PeekCapability.IssuedSeq != 0 || session.Steering.State != "closed" || session.Steering.Generation != "0" {
		t.Fatalf("intruder revocation lineage = revoked %#v session %#v", state.revokedDispatch, session)
	}
	return map[string]any{
		"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": "binding_intruder",
		"sessionTargetId": "target_intruder", "targetFingerprint": dispatch.TargetFingerprint,
		"authorityKind": lifecycle, "authoritySeq": uint64(20), "interruptEncoding": "terminal-etx-v1",
		"interruptSha256": schema2TerminalETXInterruptSHA256, "recordedBeforeSend": true,
	}
}

func schema2LifecycleReduce(t *testing.T, state *projectionState, sequence uint64, eventType string, data map[string]any) error {
	t.Helper()
	event := schema2Event(projectionTestRunID, sequence, eventType, cloneAny(data).(map[string]any))
	event["epoch"] = state.view.Identity.Epoch
	if nodeID, ok := data["nodeId"].(string); ok && nodeID != "" {
		event["nodeId"] = nodeID
	}
	if slotID, ok := data["slotId"].(string); ok && slotID != "" {
		event["slotId"] = slotID
	}
	if eventType == "slot_binding_observed" || eventType == "slot_reconciliation_interrupt" || eventType == "slot_reconciliation_interrupt_outcome" || eventType == "slot_peek_capability_revoked" {
		event["nodeId"] = projectionTestFormationID
		event["slotId"] = "slot_worker"
	}
	raw, err := decodeProjectionEvent(canonicalJSON(t, event), CanonicalRunSourceSchema2, projectionTestRunID)
	if err != nil {
		return err
	}
	safe, err := sanitizeSchema2Event(raw)
	if err != nil {
		return err
	}
	return reduceSchema2Event(state, raw, safe, nil)
}

func schema2RequireLifecycleReducerErrorWithoutMutation(t *testing.T, state *projectionState, sequence uint64, eventType string, data map[string]any) {
	t.Helper()
	before := schema2ReducerStateFingerprint(t, state)
	err := schema2LifecycleReduce(t, state, sequence, eventType, data)
	if err == nil {
		t.Fatalf("%s admitted invalid lifecycle evidence", eventType)
	}
	requireProjectionError(t, err, ErrRunProjectionInvalid)
	after := schema2ReducerStateFingerprint(t, state)
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected %s mutated reducer state\nbefore: %s\nafter:  %s", eventType, before, after)
	}
}

func schema2LifecycleMap(t *testing.T, value any) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(mustMarshalJSON(t, value), &result); err != nil {
		t.Fatalf("decode lifecycle fixture: %v", err)
	}
	return result
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

func schema2RequireTypedReducerErrorWithoutMutation(t *testing.T, state *projectionState, sequence uint64, eventType string, data map[string]any) {
	t.Helper()
	before := schema2ReducerStateFingerprint(t, state)
	err := schema2EpochReduce(t, state, sequence, state.view.Identity.Epoch, eventType, data)
	if err == nil {
		t.Fatalf("%s admitted invalid lifecycle evidence", eventType)
	}
	if !errors.Is(err, ErrRunEventUnknown) && !errors.Is(err, ErrRunProjectionInvalid) {
		t.Fatalf("%s error = %T %v, want typed event or projection rejection", eventType, err, err)
	}
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

func TestSchema2ReducerStateFingerprintIsDeterministicAndComplete(t *testing.T) {
	t.Run("repeated_fingerprints_are_byte_identical", func(t *testing.T) {
		state := schema2EpochTestState()
		first := schema2ReducerStateFingerprint(t, &state)
		second := schema2ReducerStateFingerprint(t, &state)
		if !bytes.Equal(first, second) {
			t.Fatalf("repeated reducer fingerprints differ\nfirst:  %s\nsecond: %s", first, second)
		}
	})

	t.Run("board_mutation_changes_fingerprint", func(t *testing.T) {
		state := schema2EpochTestState()
		before := schema2ReducerStateFingerprint(t, &state)
		state.board.Title = "mutated board"
		after := schema2ReducerStateFingerprint(t, &state)
		if bytes.Equal(before, after) {
			t.Fatalf("board mutation did not change reducer fingerprint: %s", after)
		}
	})

	t.Run("nested_view_mutation_changes_fingerprint", func(t *testing.T) {
		state := schema2EpochTestState()
		before := schema2ReducerStateFingerprint(t, &state)
		state.view.Nodes[0].Status = "waiting"
		after := schema2ReducerStateFingerprint(t, &state)
		if bytes.Equal(before, after) {
			t.Fatalf("nested view mutation did not change reducer fingerprint: %s", after)
		}
	})

	t.Run("map_mutation_changes_fingerprint", func(t *testing.T) {
		state := schema2EpochTestState()
		before := schema2ReducerStateFingerprint(t, &state)
		state.dispatchSeq["dsp_01KXNP6VY3227H78329V52CKF8"] = 9
		after := schema2ReducerStateFingerprint(t, &state)
		if bytes.Equal(before, after) {
			t.Fatalf("map mutation did not change reducer fingerprint: %s", after)
		}
	})
}

func schema2ReducerStateFingerprint(t *testing.T, state *projectionState) []byte {
	t.Helper()
	diagnostic := mustMarshalJSON(t, struct {
		View               RunView
		Board              *BoardDocument
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
		View: state.view, Board: state.board,
		NodeIndex: state.nodeIndex, AttemptIndex: state.attemptIndex,
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
	var structural bytes.Buffer
	if err := schema2WriteCanonicalReducerValue(&structural, reflect.ValueOf(state), map[schema2CanonicalVisit]bool{}); err != nil {
		t.Fatalf("fingerprint reducer state: %v", err)
	}
	result := make([]byte, 0, len(diagnostic)+structural.Len()+32)
	result = append(result, "diagnostic="...)
	result = append(result, diagnostic...)
	result = append(result, '\n')
	result = append(result, "structural="...)
	result = append(result, structural.Bytes()...)
	return result
}

type schema2CanonicalVisit struct {
	typ     reflect.Type
	pointer uintptr
}

func schema2WriteCanonicalReducerValue(output *bytes.Buffer, value reflect.Value, active map[schema2CanonicalVisit]bool) error {
	if !value.IsValid() {
		output.WriteString("invalid;")
		return nil
	}
	typ := value.Type()
	output.WriteString(typ.PkgPath())
	output.WriteByte('/')
	output.WriteString(typ.String())
	output.WriteByte(':')

	switch value.Kind() {
	case reflect.Bool:
		output.WriteString(strconv.FormatBool(value.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		output.WriteString(strconv.FormatInt(value.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		output.WriteString(strconv.FormatUint(value.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		float := value.Float()
		if math.IsNaN(float) || math.IsInf(float, 0) {
			return fmt.Errorf("unsupported non-finite reducer float %s", typ)
		}
		output.WriteString(strconv.FormatFloat(float, 'x', -1, typ.Bits()))
	case reflect.String:
		encoded, _ := json.Marshal(value.String())
		output.Write(encoded)
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			output.WriteString("nil")
			break
		}
		if value.Kind() == reflect.Pointer {
			visit := schema2CanonicalVisit{typ: typ, pointer: value.Pointer()}
			if active[visit] {
				return fmt.Errorf("cyclic reducer pointer %s", typ)
			}
			active[visit] = true
			defer delete(active, visit)
		}
		output.WriteByte('(')
		if err := schema2WriteCanonicalReducerValue(output, value.Elem(), active); err != nil {
			return err
		}
		output.WriteByte(')')
	case reflect.Array:
		output.WriteByte('[')
		for index := 0; index < value.Len(); index++ {
			if err := schema2WriteCanonicalReducerValue(output, value.Index(index), active); err != nil {
				return err
			}
			output.WriteByte(';')
		}
		output.WriteByte(']')
	case reflect.Slice:
		if value.IsNil() {
			output.WriteString("nil")
			break
		}
		visit := schema2CanonicalVisit{typ: typ, pointer: value.Pointer()}
		if active[visit] {
			return fmt.Errorf("cyclic reducer slice %s", typ)
		}
		active[visit] = true
		defer delete(active, visit)
		output.WriteByte('[')
		for index := 0; index < value.Len(); index++ {
			if err := schema2WriteCanonicalReducerValue(output, value.Index(index), active); err != nil {
				return err
			}
			output.WriteByte(';')
		}
		output.WriteByte(']')
	case reflect.Map:
		if value.IsNil() {
			output.WriteString("nil")
			break
		}
		visit := schema2CanonicalVisit{typ: typ, pointer: value.Pointer()}
		if active[visit] {
			return fmt.Errorf("cyclic reducer map %s", typ)
		}
		active[visit] = true
		defer delete(active, visit)
		type canonicalMapEntry struct {
			key   []byte
			value reflect.Value
		}
		entries := make([]canonicalMapEntry, 0, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			var key bytes.Buffer
			if err := schema2WriteCanonicalReducerValue(&key, iterator.Key(), active); err != nil {
				return err
			}
			entries = append(entries, canonicalMapEntry{key: key.Bytes(), value: iterator.Value()})
		}
		sort.Slice(entries, func(left, right int) bool { return bytes.Compare(entries[left].key, entries[right].key) < 0 })
		output.WriteByte('{')
		for _, entry := range entries {
			output.Write(entry.key)
			output.WriteByte('=')
			if err := schema2WriteCanonicalReducerValue(output, entry.value, active); err != nil {
				return err
			}
			output.WriteByte(';')
		}
		output.WriteByte('}')
	case reflect.Struct:
		output.WriteByte('{')
		for index := 0; index < value.NumField(); index++ {
			output.WriteString(typ.Field(index).Name)
			output.WriteByte('=')
			if err := schema2WriteCanonicalReducerValue(output, value.Field(index), active); err != nil {
				return err
			}
			output.WriteByte(';')
		}
		output.WriteByte('}')
	default:
		return fmt.Errorf("unsupported reducer state kind %s at %s", value.Kind(), typ)
	}
	output.WriteByte(';')
	return nil
}
