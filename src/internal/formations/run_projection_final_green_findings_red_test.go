package formations

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// These tests repair only the Task 1 structural oracle omitted by the final
// GREEN review. Recovery-state predicates, recovery actions, reconcile
// conditions, and schema-2 private-state records remain Task 2 work.
func TestTask1FinalGreenFindingsErrorIdentityUnionIsClosed(t *testing.T) {
	for _, arm := range []string{"run", "opened_node", "pre_attempt_node", "gate", "slot", "slot_without_dispatch", "tool"} {
		t.Run(arm+"_complete_identity_is_valid_and_private_members_stay_private", func(t *testing.T) {
			state, sequence, data := schema2FinalGreenErrorArm(t, arm)
			raw, safe, err := schema2FinalGreenReduceError(t, &state, sequence, data, nil)
			if err != nil {
				t.Fatalf("complete %s error identity rejected: %v", arm, err)
			}
			schema2FinalGreenRequirePublicErrorShape(t, raw, safe)
		})
	}

	for _, arm := range []string{"opened_node", "pre_attempt_node", "gate", "slot", "slot_without_dispatch", "tool"} {
		for _, member := range schema2FinalGreenErrorIdentityMembers(arm) {
			t.Run(arm+"_requires_"+member, func(t *testing.T) {
				state, sequence, valid := schema2FinalGreenErrorArm(t, arm)
				data := cloneAny(valid).(map[string]any)
				delete(data, member)
				schema2FinalGreenRequireErrorRejectionWithoutMutation(t, &state, sequence, data, nil)
			})
		}
	}

	for _, invalid := range []struct {
		name   string
		arm    string
		mutate func(map[string]any)
	}{
		{name: "run_rejects_graph_identity", arm: "run", mutate: func(data map[string]any) {
			data["nodeId"] = projectionTestMissionID
		}},
		{name: "opened_node_rejects_waiting_identity", arm: "opened_node", mutate: func(data map[string]any) {
			data["waitingSeq"] = uint64(3)
		}},
		{name: "pre_attempt_node_rejects_attempt_identity", arm: "pre_attempt_node", mutate: func(data map[string]any) {
			data["attempt"] = uint64(1)
		}},
		{name: "gate_rejects_slot_identity", arm: "gate", mutate: func(data map[string]any) {
			data["slotId"] = "slot_worker"
		}},
		{name: "slot_rejects_tool_identity", arm: "slot", mutate: func(data map[string]any) {
			data["toolLeaseId"] = "toollease_01KXNP6VY3227H78329V52CKF8"
		}},
		{name: "tool_rejects_gate_identity", arm: "tool", mutate: func(data map[string]any) {
			data["gateId"] = projectionTestGateID
		}},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			state, sequence, data := schema2FinalGreenErrorArm(t, invalid.arm)
			invalid.mutate(data)
			schema2FinalGreenRequireErrorRejectionWithoutMutation(t, &state, sequence, data, nil)
		})
	}

	for _, invalid := range []struct {
		name   string
		arm    string
		mutate func(map[string]any)
	}{
		{name: "opened_node_rejects_wrong_attempt", arm: "opened_node", mutate: func(data map[string]any) {
			data["attempt"] = uint64(2)
		}},
		{name: "pre_attempt_node_rejects_wrong_waiting_sequence", arm: "pre_attempt_node", mutate: func(data map[string]any) {
			data["waitingSeq"] = uint64(2)
		}},
		{name: "gate_rejects_wrong_gate_attempt", arm: "gate", mutate: func(data map[string]any) {
			data["gateAttempt"] = uint64(2)
		}},
		{name: "slot_rejects_wrong_binding", arm: "slot", mutate: func(data map[string]any) {
			data["bindingId"] = "binding_other"
		}},
		{name: "slot_rejects_wrong_session_target", arm: "slot", mutate: func(data map[string]any) {
			data["sessionTargetId"] = "target_other"
		}},
		{name: "slot_rejects_wrong_dispatch", arm: "slot", mutate: func(data map[string]any) {
			data["dispatchId"] = "dsp_01KXNP6VY3227H78329V52CKF7"
		}},
		{name: "tool_rejects_wrong_lease", arm: "tool", mutate: func(data map[string]any) {
			data["toolLeaseId"] = "toollease_01KXNP6VY3227H78329V52CKF7"
		}},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			state, sequence, data := schema2FinalGreenErrorArm(t, invalid.arm)
			invalid.mutate(data)
			schema2FinalGreenRequireErrorRejectionWithoutMutation(t, &state, sequence, data, nil)
		})
	}

	for _, mismatch := range []struct {
		name   string
		arm    string
		mutate func(map[string]any)
	}{
		{name: "node_id", arm: "opened_node", mutate: func(event map[string]any) {
			event["nodeId"] = projectionTestMissionID
		}},
		{name: "node_attempt", arm: "opened_node", mutate: func(event map[string]any) {
			event["attempt"] = uint64(2)
		}},
		{name: "gate_id", arm: "gate", mutate: func(event map[string]any) {
			event["gateId"] = "gate_other"
		}},
		{name: "slot_id", arm: "slot", mutate: func(event map[string]any) {
			event["slotId"] = "slot_other"
		}},
	} {
		t.Run("envelope_rejects_"+mismatch.name+"_mismatch", func(t *testing.T) {
			state, sequence, data := schema2FinalGreenErrorArm(t, mismatch.arm)
			schema2FinalGreenRequireErrorRejectionWithoutMutation(t, &state, sequence, data, mismatch.mutate)
		})
	}
}

func TestTask1FinalGreenFindingsFailureCauseErrorSelectsOnlyExactOpenedAttempt(t *testing.T) {
	t.Run("opened_node_selects_exact_attempt_and_abandons_collateral", func(t *testing.T) {
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
			started := schema2FinalGreenNodeStartedData(node.nodeID, node.kind)
			if err := schema2LifecycleReduce(t, &state, node.sequence, "node_started", started); err != nil {
				t.Fatalf("prepare %s attempt: %v", node.kind, err)
			}
			open["openNodeAttempts"] = append(open["openNodeAttempts"], schema2FinalGreenOpenAttempt(node.nodeID, node.kind, node.sequence))
		}

		failure := schema2FinalGreenErrorData("node")
		failure["nodeId"] = projectionTestFormationID
		failure["attempt"] = uint64(1)
		failure["relatedSeq"] = uint64(4)
		if _, _, err := schema2FinalGreenReduceError(t, &state, 5, failure, nil); err != nil {
			t.Fatalf("prepare exact attempt error: %v", err)
		}

		start := schema2LifecycleStartWithOpen("failure", open)
		start["relatedSeq"] = uint64(5)
		start["failureCause"] = map[string]any{"kind": "error", "errorSeq": uint64(5)}
		if err := schema2LifecycleReduce(t, &state, 20, "run_failure_reconciliation_started", start); err != nil {
			t.Fatalf("prepare exact failure authority: %v", err)
		}
		final := schema2LifecycleFinalWithDispositions(t, "failure", 20, open)
		for _, field := range []string{"code", "reason", "unrecoverable", "relatedSeq", "failureCause"} {
			final[field] = cloneAny(start[field])
		}
		for _, member := range final["nodeAttemptDispositions"].([]any) {
			disposition := member.(map[string]any)
			if disposition["nodeId"] == projectionTestFormationID && disposition["attempt"] == uint64(1) {
				disposition["disposition"] = "failed_non_authorizing"
			}
		}
		if err := schema2LifecycleReduce(t, &state, 21, "run_failed", final); err != nil {
			t.Fatalf("exact selected/collateral final rejected: %v", err)
		}
		schema2RequireNodeAttemptDisposition(t, &state, projectionTestFormationID, "failed", 21)
		schema2RequireNodeAttemptDisposition(t, &state, projectionTestMissionID, "abandoned", 21)
	})

	for _, scope := range []string{"run", "pre_attempt_node"} {
		t.Run(scope+"_error_selects_no_open_attempt", func(t *testing.T) {
			state := schema2EpochTestState()
			if err := schema2LifecycleReduce(t, &state, 3, "node_started", schema2FinalGreenNodeStartedData(projectionTestMissionID, "mission")); err != nil {
				t.Fatalf("prepare collateral attempt: %v", err)
			}
			open := map[string][]any{
				"openNodeAttempts":   {schema2FinalGreenOpenAttempt(projectionTestMissionID, "mission", 3)},
				"openSlotDispatches": {},
				"openToolLeases":     {},
			}
			sequence := uint64(4)
			data := schema2FinalGreenErrorData("run")
			if scope == "pre_attempt_node" {
				waiting := schema2SecondRepairFixture(t, "node_waiting")
				waiting["nodeId"] = projectionTestFormationID
				if err := schema2LifecycleReduce(t, &state, 4, "node_waiting", waiting); err != nil {
					t.Fatalf("prepare pre-attempt waiting evidence: %v", err)
				}
				sequence = 5
				data = schema2FinalGreenErrorData("node")
				data["nodeId"] = projectionTestFormationID
				data["waitingSeq"] = uint64(4)
			}
			data["relatedSeq"] = sequence - 1
			if _, _, err := schema2FinalGreenReduceError(t, &state, sequence, data, nil); err != nil {
				t.Fatalf("prepare %s error: %v", scope, err)
			}
			start := schema2LifecycleStartWithOpen("failure", open)
			start["relatedSeq"] = sequence
			start["failureCause"] = map[string]any{"kind": "error", "errorSeq": sequence}
			if err := schema2LifecycleReduce(t, &state, 20, "run_failure_reconciliation_started", start); err != nil {
				t.Fatalf("prepare %s failure authority: %v", scope, err)
			}
			final := schema2LifecycleFinalWithDispositions(t, "failure", 20, open)
			for _, field := range []string{"code", "reason", "unrecoverable", "relatedSeq", "failureCause"} {
				final[field] = cloneAny(start[field])
			}
			if err := schema2LifecycleReduce(t, &state, 21, "run_failed", final); err != nil {
				t.Fatalf("%s error selected collateral attempt: %v", scope, err)
			}
			schema2RequireNodeAttemptDisposition(t, &state, projectionTestMissionID, "abandoned", 21)
		})
	}

	for _, scope := range []string{"slot", "tool"} {
		t.Run(scope+"_error_cannot_back_failure_cause_error", func(t *testing.T) {
			state, sequence, data := schema2FinalGreenErrorArm(t, scope)
			if _, _, err := schema2FinalGreenReduceError(t, &state, sequence, data, nil); err != nil {
				t.Fatalf("prepare %s error: %v", scope, err)
			}
			open := schema2FinalGreenOpenAuthority(t, &state)
			start := schema2LifecycleStartWithOpen("failure", open)
			start["relatedSeq"] = sequence
			start["failureCause"] = map[string]any{"kind": "error", "errorSeq": sequence}
			schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 20, "run_failure_reconciliation_started", start)
		})
	}
}

func TestTask1FinalGreenFindingsNodeStartedOwnsUniqueGraphAttemptIdentity(t *testing.T) {
	for _, valid := range []struct {
		name     string
		nodeID   string
		nodeKind string
	}{
		{name: "mission", nodeID: projectionTestMissionID, nodeKind: "mission"},
		{name: "formation", nodeID: projectionTestFormationID, nodeKind: "formation"},
		{name: "gate", nodeID: projectionTestGateID, nodeKind: "gate"},
	} {
		t.Run(valid.name+"_graph_kind_and_envelope_identity_are_valid", func(t *testing.T) {
			state := schema2EpochTestState()
			data := schema2FinalGreenNodeStartedData(valid.nodeID, valid.nodeKind)
			if err := schema2FinalGreenReduceNodeStarted(t, &state, 3, data, nil); err != nil {
				t.Fatalf("valid %s node_started rejected: %v", valid.name, err)
			}
			attempt := state.existingAttempt(valid.nodeID, 1)
			if attempt == nil || attempt.StartedSeq != 3 || attempt.Status != "running" {
				t.Fatalf("valid %s attempt projection = %#v", valid.name, attempt)
			}
		})
	}

	t.Run("duplicate_same_attempt_is_rejected_without_mutation", func(t *testing.T) {
		state := schema2EpochTestState()
		data := schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")
		if err := schema2FinalGreenReduceNodeStarted(t, &state, 3, data, nil); err != nil {
			t.Fatalf("prepare unique attempt: %v", err)
		}
		schema2FinalGreenRequireNodeStartedRejectionWithoutMutation(t, &state, 4, data, nil)
	})

	t.Run("second_attempt_while_prior_attempt_is_open_is_rejected_without_mutation", func(t *testing.T) {
		state := schema2EpochTestState()
		first := schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")
		if err := schema2FinalGreenReduceNodeStarted(t, &state, 3, first, nil); err != nil {
			t.Fatalf("prepare first open attempt: %v", err)
		}
		second := schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")
		second["attempt"] = uint64(2)
		schema2FinalGreenRequireNodeStartedRejectionWithoutMutation(t, &state, 4, second, nil)
	})

	for _, contradiction := range []struct {
		name     string
		nodeID   string
		nodeKind string
	}{
		{name: "mission_as_formation", nodeID: projectionTestMissionID, nodeKind: "formation"},
		{name: "formation_as_mission", nodeID: projectionTestFormationID, nodeKind: "mission"},
		{name: "gate_as_formation", nodeID: projectionTestGateID, nodeKind: "formation"},
	} {
		t.Run(contradiction.name+"_is_rejected_without_mutation", func(t *testing.T) {
			state := schema2EpochTestState()
			data := schema2FinalGreenNodeStartedData(contradiction.nodeID, contradiction.nodeKind)
			schema2FinalGreenRequireNodeStartedRejectionWithoutMutation(t, &state, 3, data, nil)
		})
	}

	for _, mismatch := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "node_id", mutate: func(event map[string]any) {
			event["nodeId"] = projectionTestMissionID
		}},
		{name: "attempt", mutate: func(event map[string]any) {
			event["attempt"] = uint64(2)
		}},
	} {
		t.Run("envelope_data_"+mismatch.name+"_mismatch_is_rejected_without_mutation", func(t *testing.T) {
			state := schema2EpochTestState()
			data := schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")
			schema2FinalGreenRequireNodeStartedRejectionWithoutMutation(t, &state, 3, data, mismatch.mutate)
		})
	}
}

func schema2FinalGreenErrorArm(t *testing.T, arm string) (projectionState, uint64, map[string]any) {
	t.Helper()
	data := schema2FinalGreenErrorData("run")
	switch arm {
	case "run":
		return schema2EpochTestState(), 3, data
	case "opened_node":
		state := schema2EpochTestState()
		if err := schema2LifecycleReduce(t, &state, 3, "node_started", schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")); err != nil {
			t.Fatalf("prepare opened node: %v", err)
		}
		data["errorScope"] = "node"
		data["nodeId"] = projectionTestFormationID
		data["attempt"] = uint64(1)
		data["relatedSeq"] = uint64(3)
		return state, 4, data
	case "pre_attempt_node":
		state := schema2EpochTestState()
		waiting := schema2SecondRepairFixture(t, "node_waiting")
		waiting["nodeId"] = projectionTestFormationID
		if err := schema2LifecycleReduce(t, &state, 3, "node_waiting", waiting); err != nil {
			t.Fatalf("prepare waiting node: %v", err)
		}
		data["errorScope"] = "node"
		data["nodeId"] = projectionTestFormationID
		data["waitingSeq"] = uint64(3)
		data["relatedSeq"] = uint64(3)
		return state, 4, data
	case "gate":
		state := schema2EpochTestState()
		gate := schema2RepairGateEvaluatingData()
		delete(gate, "revisionCycleId")
		delete(gate, "triggerFeedbackId")
		delete(gate, "priorGateSeq")
		if err := schema2LifecycleReduce(t, &state, 3, "gate_evaluating", gate); err != nil {
			t.Fatalf("prepare gate attempt: %v", err)
		}
		data["errorScope"] = "gate"
		data["nodeId"] = projectionTestGateID
		data["attempt"] = uint64(1)
		data["gateId"] = projectionTestGateID
		data["gateAttempt"] = uint64(1)
		data["relatedSeq"] = uint64(3)
		return state, 4, data
	case "slot", "slot_without_dispatch":
		state := schema2EpochTestState()
		if arm == "slot" {
			state, _ = schema2LifecyclePublicState(t, "dispatch")
		} else {
			if err := schema2LifecycleReduce(t, &state, 3, "node_started", schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")); err != nil {
				t.Fatalf("prepare pre-dispatch slot attempt: %v", err)
			}
			state.bindings["binding_worker"] = schema2Binding{
				BindingID: "binding_worker", NodeID: projectionTestFormationID, SlotID: "slot_worker",
				AgentID: "worker", Harness: "codex", SessionTargetID: "target_worker",
			}
			observed := schema2SecondRepairFixture(t, "slot_binding_observed")
			observed["relatedSeq"] = uint64(3)
			if err := schema2LifecycleReduce(t, &state, 4, "slot_binding_observed", observed); err != nil {
				t.Fatalf("prepare pre-dispatch slot binding: %v", err)
			}
		}
		data["errorScope"] = "slot"
		data["nodeId"] = projectionTestFormationID
		data["attempt"] = uint64(1)
		data["slotId"] = "slot_worker"
		data["bindingId"] = "binding_worker"
		data["sessionTargetId"] = "target_worker"
		if arm == "slot" {
			data["dispatchId"] = "dsp_01KXNP6VY3227H78329V52CKF8"
		}
		data["relatedSeq"] = uint64(5)
		if arm == "slot_without_dispatch" {
			data["relatedSeq"] = uint64(4)
			return state, 5, data
		}
		return state, 6, data
	case "tool":
		state, _ := schema2LifecyclePublicState(t, "tool")
		data["errorScope"] = "tool"
		data["nodeId"] = "tool_normalize"
		data["attempt"] = uint64(1)
		data["toolLeaseId"] = "toollease_01KXNP6VY3227H78329V52CKF8"
		data["relatedSeq"] = uint64(4)
		return state, 5, data
	default:
		t.Fatalf("unknown error arm %q", arm)
		return projectionState{}, 0, nil
	}
}

func schema2FinalGreenReduceNodeStarted(t *testing.T, state *projectionState, sequence uint64, data map[string]any, mutateEvent func(map[string]any)) error {
	t.Helper()
	event := schema2Event(projectionTestRunID, sequence, "node_started", cloneAny(data).(map[string]any))
	event["epoch"] = state.view.Identity.Epoch
	event["nodeId"] = data["nodeId"]
	event["attempt"] = data["attempt"]
	if mutateEvent != nil {
		mutateEvent(event)
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

func schema2FinalGreenRequireNodeStartedRejectionWithoutMutation(t *testing.T, state *projectionState, sequence uint64, data map[string]any, mutateEvent func(map[string]any)) {
	t.Helper()
	before := schema2ReducerStateFingerprint(t, state)
	err := schema2FinalGreenReduceNodeStarted(t, state, sequence, data, mutateEvent)
	if err == nil {
		t.Fatal("contradictory node_started identity was admitted")
	}
	if !errors.Is(err, ErrRunEventUnknown) && !errors.Is(err, ErrRunProjectionInvalid) {
		t.Fatalf("node_started rejection = %T %v, want typed event or projection rejection", err, err)
	}
	after := schema2ReducerStateFingerprint(t, state)
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected node_started mutated reducer state\nbefore: %s\nafter:  %s", before, after)
	}
}

func schema2FinalGreenErrorData(scope string) map[string]any {
	return map[string]any{
		"code": "engine_failed", "message": "engine failed", "boundary": "engine",
		"errorScope": scope, "recoverable": true, "relatedSeq": uint64(2),
	}
}

func schema2FinalGreenNodeStartedData(nodeID, nodeKind string) map[string]any {
	return map[string]any{
		"nodeId": nodeID, "nodeKind": nodeKind, "attempt": uint64(1),
		"reason": "initial", "inputRefs": []any{},
	}
}

func schema2FinalGreenOpenAttempt(nodeID, nodeKind string, sequence uint64) map[string]any {
	return map[string]any{
		"nodeId": nodeID, "nodeKind": nodeKind, "attempt": uint64(1),
		"startSeq": sequence, "phase": "started", "phaseSeq": sequence,
	}
}

func schema2FinalGreenErrorIdentityMembers(arm string) []string {
	switch arm {
	case "opened_node":
		return []string{"nodeId", "attempt"}
	case "pre_attempt_node":
		return []string{"nodeId", "waitingSeq"}
	case "gate":
		return []string{"nodeId", "attempt", "gateId", "gateAttempt"}
	case "slot":
		return []string{"nodeId", "attempt", "slotId", "bindingId", "sessionTargetId", "dispatchId"}
	case "slot_without_dispatch":
		return []string{"nodeId", "attempt", "slotId", "bindingId", "sessionTargetId"}
	case "tool":
		return []string{"nodeId", "attempt", "toolLeaseId"}
	default:
		return nil
	}
}

func schema2FinalGreenErrorEvent(sequence uint64, data map[string]any) map[string]any {
	event := schema2Event(projectionTestRunID, sequence, "error", cloneAny(data).(map[string]any))
	delete(event, "attempt")
	for _, member := range []string{"nodeId", "slotId", "gateId", "attempt"} {
		if value, ok := data[member]; ok {
			event[member] = value
		}
	}
	return event
}

func schema2FinalGreenReduceError(t *testing.T, state *projectionState, sequence uint64, data map[string]any, mutateEvent func(map[string]any)) (rawProjectionEvent, SafeRunEvent, error) {
	t.Helper()
	event := schema2FinalGreenErrorEvent(sequence, data)
	event["epoch"] = state.view.Identity.Epoch
	if mutateEvent != nil {
		mutateEvent(event)
	}
	raw, err := decodeProjectionEvent(canonicalJSON(t, event), CanonicalRunSourceSchema2, projectionTestRunID)
	if err != nil {
		return rawProjectionEvent{}, nil, err
	}
	safe, err := sanitizeSchema2Event(raw)
	if err != nil {
		return raw, nil, err
	}
	return raw, safe, reduceSchema2Event(state, raw, safe, nil)
}

func schema2FinalGreenRequireErrorRejectionWithoutMutation(t *testing.T, state *projectionState, sequence uint64, data map[string]any, mutateEvent func(map[string]any)) {
	t.Helper()
	before := schema2ReducerStateFingerprint(t, state)
	_, _, err := schema2FinalGreenReduceError(t, state, sequence, data, mutateEvent)
	if err == nil {
		t.Fatalf("invalid %s error identity was admitted", data["errorScope"])
	}
	if !errors.Is(err, ErrRunEventUnknown) && !errors.Is(err, ErrRunProjectionInvalid) {
		t.Fatalf("error identity rejection = %T %v, want typed event or projection rejection", err, err)
	}
	after := schema2ReducerStateFingerprint(t, state)
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected error identity mutated reducer state\nbefore: %s\nafter:  %s", before, after)
	}
}

func schema2FinalGreenRequirePublicErrorShape(t *testing.T, raw rawProjectionEvent, safe SafeRunEvent) {
	t.Helper()
	encoded := mustMarshalJSON(t, safe)
	var public struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(encoded, &public); err != nil {
		t.Fatalf("decode public error event: %v", err)
	}
	allowed := stringSet("code", "message", "boundary", "errorScope", "nodeId", "gateId", "slotId", "toolLeaseId", "recoverable", "relatedSeq")
	for member := range public.Data {
		if !allowed[member] {
			t.Fatalf("private error identity member %q escaped public event: %s", member, encoded)
		}
	}
	for _, member := range []string{"attempt", "waitingSeq", "gateAttempt", "bindingId", "sessionTargetId", "dispatchId"} {
		if _, exists := public.Data[member]; exists {
			t.Fatalf("private error identity member %q escaped public data: %s", member, encoded)
		}
	}
	for _, member := range []string{"code", "message", "boundary", "errorScope", "recoverable", "relatedSeq"} {
		if _, exists := public.Data[member]; !exists {
			t.Fatalf("public error omitted required member %q: %s", member, encoded)
		}
	}
	var scope string
	if err := json.Unmarshal(public.Data["errorScope"], &scope); err != nil {
		t.Fatalf("decode public error scope: %v", err)
	}
	identity := map[string]string{"node": "nodeId", "gate": "gateId", "slot": "slotId", "tool": "toolLeaseId"}[scope]
	if identity != "" {
		if _, exists := public.Data[identity]; !exists {
			t.Fatalf("public %s error omitted approved %s: %s", scope, identity, encoded)
		}
	}
	if len(raw.rawData) == 0 {
		t.Fatal("validated error lost its raw semantic source before reduction")
	}
}

func schema2FinalGreenOpenAuthority(t *testing.T, state *projectionState) map[string][]any {
	t.Helper()
	nodes := state.currentOpenNodeAttempts()
	slots := state.currentSchema2OpenDispatches()
	tools := state.currentOpenToolLeases()
	result := map[string][]any{
		"openNodeAttempts":   make([]any, 0, len(nodes)),
		"openSlotDispatches": make([]any, 0, len(slots)),
		"openToolLeases":     make([]any, 0, len(tools)),
	}
	for _, node := range nodes {
		result["openNodeAttempts"] = append(result["openNodeAttempts"], schema2LifecycleMap(t, node))
	}
	for _, slot := range slots {
		result["openSlotDispatches"] = append(result["openSlotDispatches"], schema2LifecycleMap(t, slot))
	}
	for _, tool := range tools {
		result["openToolLeases"] = append(result["openToolLeases"], schema2LifecycleMap(t, tool))
	}
	return result
}
