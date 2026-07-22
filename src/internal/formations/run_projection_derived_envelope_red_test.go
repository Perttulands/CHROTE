package formations

import (
	"bytes"
	"strings"
	"testing"
)

func TestTask1RunStartedRequiresRootConditionalEnvelopeIdentity(t *testing.T) {
	t.Run("Mission root exact envelope projects", func(t *testing.T) {
		if _, err := ProjectCanonicalRun(schema2ProjectionInput(t, true)); err != nil {
			t.Fatalf("project exact Mission root: %v", err)
		}
	})

	for _, mutation := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "Mission root missing missionId", mutate: func(started map[string]any) { delete(started, "missionId") }},
		{name: "Mission root mismatched missionId", mutate: func(started map[string]any) { started["missionId"] = projectionTestFormationID }},
		{name: "Mission root mismatched beadId", mutate: func(started map[string]any) { started["beadId"] = "ctx-other" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			input := schema2ProjectionInput(t, true)
			schema2RequireRunStartedEnvelopeRejection(t, input, mutation.mutate)
		})
	}

	t.Run("isolated Formation exact envelope projects", func(t *testing.T) {
		input, _ := schema2OpenDispatchLifecycleInput(t, false)
		if _, err := ProjectCanonicalRun(input); err != nil {
			t.Fatalf("project exact isolated Formation root: %v", err)
		}
	})

	for _, mutation := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "isolated Formation forbids missionId", mutate: func(started map[string]any) { started["missionId"] = projectionTestMissionID }},
		{name: "isolated Formation forbids beadId", mutate: func(started map[string]any) { started["beadId"] = "ctx-7i1.1" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			input, _ := schema2OpenDispatchLifecycleInput(t, false)
			schema2RequireRunStartedEnvelopeRejection(t, input, mutation.mutate)
		})
	}
}

func schema2RequireRunStartedEnvelopeRejection(t *testing.T, input CanonicalRunReadInput, mutate func(map[string]any)) {
	t.Helper()
	events := canonicalLedgerEvents(t, input)
	mutate(events[0])
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
	if projection, err := ProjectCanonicalRun(input); err == nil {
		t.Fatalf("contradictory run_started envelope projected: %#v", projection)
	} else {
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	}
}

func TestTask1DirectNodeAuthorityRequiresExactEnvelopeIdentity(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		sequence  uint64
		prepare   func(*testing.T) (projectionState, map[string]any)
		required  []string
	}{
		{
			name: "node input ignored", eventType: "node_input_ignored", sequence: 5,
			prepare: func(t *testing.T) (projectionState, map[string]any) {
				return schema2DerivedEnvelopeAttemptState(t, projectionTestFormationID, "formation"), schema2SecondRepairFixture(t, "node_input_ignored")
			},
			required: []string{"nodeId", "attempt"},
		},
		{
			name: "Formation result", eventType: "formation_result", sequence: 20,
			prepare:  schema2DerivedFormationResultInput,
			required: []string{"nodeId", "attempt"},
		},
		{
			name: "Tool process launch", eventType: "tool_process_launch", sequence: 5,
			prepare: func(t *testing.T) (projectionState, map[string]any) {
				state, _ := schema2LifecyclePublicState(t, "tool")
				return state, schema2RepairToolProcessLaunchData()
			},
			required: []string{"nodeId", "attempt"},
		},
		{
			name: "Tool result", eventType: "tool_result", sequence: 6,
			prepare: func(t *testing.T) (projectionState, map[string]any) {
				state, _ := schema2LifecyclePublicState(t, "tool")
				return state, schema2RepairToolResultData()
			},
			required: []string{"nodeId", "attempt"},
		},
	}

	for _, test := range tests {
		t.Run(test.name+" conformant envelope is accepted", func(t *testing.T) {
			state, data := test.prepare(t)
			if err := schema2DerivedEnvelopeReduce(t, &state, test.sequence, test.eventType, data, nil); err != nil {
				t.Fatalf("reduce conformant %s: %v", test.eventType, err)
			}
		})
		for _, member := range test.required {
			member := member
			for _, mutation := range []struct {
				name   string
				mutate func(map[string]any)
			}{
				{name: "omitted", mutate: func(event map[string]any) { delete(event, member) }},
				{name: "mismatched", mutate: func(event map[string]any) { schema2SetMismatchedEnvelopeIdentity(event, member) }},
			} {
				mutation := mutation
				t.Run(test.name+" rejects "+mutation.name+" "+member+" without mutation", func(t *testing.T) {
					state, data := test.prepare(t)
					schema2DerivedEnvelopeRequireRejection(t, &state, test.sequence, test.eventType, data, mutation.mutate)
				})
			}
		}
	}

	t.Run("node output conformant envelope closes only its selected attempt", func(t *testing.T) {
		state, result := schema2FinalGreenFormationResultState(t)
		data := schema2FinalGreenNodeOutputFromResult("formation", 20, result)
		if err := schema2DerivedEnvelopeReduce(t, &state, 21, "node_output", data, nil); err != nil {
			t.Fatalf("reduce conformant node_output: %v", err)
		}
		schema2FinalGreenRequireClosedAttemptAt(t, &state, projectionTestFormationID, 1, 21, "port_out")
	})

	for _, mutation := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "omitted nodeId", mutate: func(event map[string]any) { delete(event, "nodeId") }},
		{name: "mismatched nodeId", mutate: func(event map[string]any) { event["nodeId"] = projectionTestMissionID }},
	} {
		t.Run("node output rejects "+mutation.name+" without mutation", func(t *testing.T) {
			state, result := schema2FinalGreenFormationResultState(t)
			data := schema2FinalGreenNodeOutputFromResult("formation", 20, result)
			schema2DerivedEnvelopeRequireRejection(t, &state, 21, "node_output", data, mutation.mutate)
		})
	}
}

func schema2DerivedFormationResultInput(t *testing.T) (projectionState, map[string]any) {
	t.Helper()
	state := schema2EpochTestState()
	if err := schema2LifecycleReduce(t, &state, 3, "node_started", schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")); err != nil {
		t.Fatalf("prepare Formation attempt: %v", err)
	}
	return state, schema2RepairFormationResultData()
}

func schema2SetMismatchedEnvelopeIdentity(event map[string]any, member string) {
	switch member {
	case "attempt":
		event[member] = uint64(2)
	case "nodeId":
		event[member] = projectionTestMissionID
	case "slotId":
		event[member] = "slot_other"
	case "gateId":
		event[member] = "gate_other"
	default:
		panic("unsupported envelope identity member: " + member)
	}
}

func TestTask1DerivedGraphAuthorityRequiresExactEnvelopeIdentity(t *testing.T) {
	t.Run("slot_dispatch_conformant_envelope_adds_only_exact_dispatch_and_session_authority", func(t *testing.T) {
		state := schema2DerivedEnvelopeSlotState(t)
		data := schema2RepairSlotDispatchData(t)
		if err := schema2DerivedEnvelopeReduce(t, &state, 6, "slot_dispatch", data, nil); err != nil {
			t.Fatalf("reduce conformant slot_dispatch: %v", err)
		}

		dispatch, ok := state.dispatches["dsp_01KXNP6VY3227H78329V52CKF8"]
		if !ok || len(state.dispatches) != 1 || state.dispatchSeq[dispatch.DispatchID] != 6 ||
			dispatch.NodeID != projectionTestFormationID || dispatch.Attempt != 1 || dispatch.SlotID != "slot_worker" {
			t.Fatalf("slot dispatch authority = dispatch:%#v seq:%#v all:%#v", dispatch, state.dispatchSeq, state.dispatches)
		}
		session := state.sessionByDispatch(dispatch.DispatchID)
		if session == nil || len(state.view.Sessions) != 1 || session.NodeID != projectionTestFormationID ||
			session.Attempt != 1 || session.SlotID != "slot_worker" || session.BindingID != "binding_worker" ||
			session.TargetLeaseID != "lease_01KXNP6VY3227H78329V52CKF8" || session.SessionTargetID != "target_worker" {
			t.Fatalf("slot dispatch session authority = %#v; all=%#v", session, state.view.Sessions)
		}
		if len(state.toolLeases) != 0 || len(state.view.Gates) != 0 {
			t.Fatalf("slot dispatch added unrelated authority: tools=%#v gates=%#v", state.toolLeases, state.view.Gates)
		}
		schema2DerivedEnvelopeRequireTwoOpenAttempts(t, &state, projectionTestFormationID, 3)
	})

	for _, identity := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "omitted_node_id", mutate: func(event map[string]any) { delete(event, "nodeId") }},
		{name: "mismatched_node_id", mutate: func(event map[string]any) { event["nodeId"] = projectionTestMissionID }},
		{name: "omitted_slot_id", mutate: func(event map[string]any) { delete(event, "slotId") }},
		{name: "mismatched_slot_id", mutate: func(event map[string]any) { event["slotId"] = "slot_other" }},
		{name: "omitted_attempt", mutate: func(event map[string]any) { delete(event, "attempt") }},
		{name: "mismatched_attempt", mutate: func(event map[string]any) { event["attempt"] = uint64(2) }},
	} {
		t.Run("slot_dispatch_rejects_"+identity.name+"_without_mutation", func(t *testing.T) {
			state := schema2DerivedEnvelopeSlotState(t)
			schema2DerivedEnvelopeRequireRejection(t, &state, 6, "slot_dispatch", schema2RepairSlotDispatchData(t), identity.mutate)
		})
	}

	t.Run("tool_dispatch_conformant_envelope_adds_only_exact_lease_authority", func(t *testing.T) {
		state := schema2DerivedEnvelopeAttemptState(t, "tool_normalize", "tool")
		beforeView := schema2RepairStructuralFingerprint(t, state.view)
		if err := schema2DerivedEnvelopeReduce(t, &state, 5, "tool_dispatch", schema2RepairToolDispatchData(), nil); err != nil {
			t.Fatalf("reduce conformant tool_dispatch: %v", err)
		}

		lease, ok := state.toolLeases["toollease_01KXNP6VY3227H78329V52CKF8"]
		if !ok || len(state.toolLeases) != 1 || lease.NodeID != "tool_normalize" || lease.Attempt != 1 || lease.DispatchSeq != 5 {
			t.Fatalf("Tool lease authority = %#v; all=%#v", lease, state.toolLeases)
		}
		if afterView := schema2RepairStructuralFingerprint(t, state.view); !bytes.Equal(afterView, beforeView) {
			t.Fatalf("tool_dispatch changed public graph authority\nbefore: %s\nafter:  %s", beforeView, afterView)
		}
		if len(state.dispatches) != 0 || len(state.view.Sessions) != 0 || len(state.view.Gates) != 0 {
			t.Fatalf("tool_dispatch added unrelated authority: dispatches=%#v sessions=%#v gates=%#v", state.dispatches, state.view.Sessions, state.view.Gates)
		}
		schema2DerivedEnvelopeRequireTwoOpenAttempts(t, &state, "tool_normalize", 3)
	})

	for _, identity := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "omitted_node_id", mutate: func(event map[string]any) { delete(event, "nodeId") }},
		{name: "mismatched_node_id", mutate: func(event map[string]any) { event["nodeId"] = projectionTestMissionID }},
		{name: "omitted_attempt", mutate: func(event map[string]any) { delete(event, "attempt") }},
		{name: "mismatched_attempt", mutate: func(event map[string]any) { event["attempt"] = uint64(2) }},
	} {
		t.Run("tool_dispatch_rejects_"+identity.name+"_without_mutation", func(t *testing.T) {
			state := schema2DerivedEnvelopeAttemptState(t, "tool_normalize", "tool")
			schema2DerivedEnvelopeRequireRejection(t, &state, 5, "tool_dispatch", schema2RepairToolDispatchData(), identity.mutate)
		})
	}

	t.Run("gate_evaluating_conformant_envelope_adds_only_exact_gate_authority", func(t *testing.T) {
		state := schema2DerivedEnvelopeAttemptState(t, projectionTestGateID, "gate")
		if err := schema2DerivedEnvelopeReduce(t, &state, 5, "gate_evaluating", schema2FinalGreenGateEvaluatingData(), nil); err != nil {
			t.Fatalf("reduce conformant gate_evaluating: %v", err)
		}

		gate := state.existingGate(projectionTestGateID, 1)
		attempt := state.existingAttempt(projectionTestGateID, 1)
		if gate == nil || len(state.view.Gates) != 1 || gate.GateID != projectionTestGateID || gate.Attempt != 1 || gate.EvaluatingSeq != 5 ||
			attempt == nil || attempt.StartedSeq != 3 || len(state.view.Attempts) != 2 {
			t.Fatalf("Gate evaluation authority = gate:%#v attempt:%#v all gates:%#v attempts:%#v", gate, attempt, state.view.Gates, state.view.Attempts)
		}
		if len(state.dispatches) != 0 || len(state.toolLeases) != 0 || len(state.view.Sessions) != 0 {
			t.Fatalf("gate_evaluating added unrelated authority: dispatches=%#v tools=%#v sessions=%#v", state.dispatches, state.toolLeases, state.view.Sessions)
		}
		schema2DerivedEnvelopeRequireTwoOpenAttempts(t, &state, projectionTestGateID, 3)
	})

	for _, identity := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "omitted_node_id", mutate: func(event map[string]any) { delete(event, "nodeId") }},
		{name: "mismatched_node_id", mutate: func(event map[string]any) { event["nodeId"] = projectionTestMissionID }},
		{name: "omitted_gate_id", mutate: func(event map[string]any) { delete(event, "gateId") }},
		{name: "mismatched_gate_id", mutate: func(event map[string]any) { event["gateId"] = "gate_other" }},
		{name: "omitted_attempt", mutate: func(event map[string]any) { delete(event, "attempt") }},
		{name: "mismatched_attempt", mutate: func(event map[string]any) { event["attempt"] = uint64(2) }},
	} {
		t.Run("gate_evaluating_rejects_"+identity.name+"_without_mutation", func(t *testing.T) {
			state := schema2DerivedEnvelopeAttemptState(t, projectionTestGateID, "gate")
			schema2DerivedEnvelopeRequireRejection(t, &state, 5, "gate_evaluating", schema2FinalGreenGateEvaluatingData(), identity.mutate)
		})
	}
}

func schema2DerivedEnvelopeSlotState(t *testing.T) projectionState {
	t.Helper()
	state := schema2DerivedEnvelopeAttemptState(t, projectionTestFormationID, "formation")
	state.bindings["binding_worker"] = schema2Binding{
		BindingID: "binding_worker", NodeID: projectionTestFormationID, SlotID: "slot_worker",
		AgentID: "worker", Harness: "codex", SessionTargetID: "target_worker",
		TargetFingerprint: strings.Repeat("a", 64), SessionLineageSHA256: strings.Repeat("c", 64),
	}
	observed := schema2SecondRepairFixture(t, "slot_binding_observed")
	observed["relatedSeq"] = uint64(3)
	if err := schema2LifecycleReduce(t, &state, 5, "slot_binding_observed", observed); err != nil {
		t.Fatalf("prepare exact slot binding observation: %v", err)
	}
	return state
}

func schema2DerivedEnvelopeAttemptState(t *testing.T, nodeID, nodeKind string) projectionState {
	t.Helper()
	state := schema2FinalGreenAllNodeState()
	if err := schema2FinalGreenReduceNodeStarted(t, &state, 3, schema2FinalGreenNodeStartedData(nodeID, nodeKind), nil); err != nil {
		t.Fatalf("prepare selected %s attempt: %v", nodeKind, err)
	}
	if err := schema2FinalGreenReduceNodeStarted(t, &state, 4, schema2FinalGreenNodeStartedData(projectionTestMissionID, "mission"), nil); err != nil {
		t.Fatalf("prepare collateral Mission attempt: %v", err)
	}
	return state
}

func schema2DerivedEnvelopeReduce(t *testing.T, state *projectionState, sequence uint64, eventType string, data map[string]any, mutate func(map[string]any)) error {
	t.Helper()
	event := schema2Event(projectionTestRunID, sequence, eventType, cloneAny(data).(map[string]any))
	event["epoch"] = state.view.Identity.Epoch
	if mutate != nil {
		mutate(event)
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

func schema2DerivedEnvelopeRequireRejection(t *testing.T, state *projectionState, sequence uint64, eventType string, data map[string]any, mutate func(map[string]any)) {
	t.Helper()
	before := schema2ReducerStateFingerprint(t, state)
	err := schema2DerivedEnvelopeReduce(t, state, sequence, eventType, data, mutate)
	if err == nil {
		t.Fatalf("%s admitted omitted or mismatched graph envelope identity", eventType)
	}
	requireProjectionError(t, err, ErrRunProjectionInvalid)
	after := schema2ReducerStateFingerprint(t, state)
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected %s mutated complete reducer state\nbefore: %s\nafter:  %s", eventType, before, after)
	}
}

func schema2DerivedEnvelopeRequireTwoOpenAttempts(t *testing.T, state *projectionState, selectedNodeID string, selectedStart uint64) {
	t.Helper()
	selected := state.existingAttempt(selectedNodeID, 1)
	collateral := state.existingAttempt(projectionTestMissionID, 1)
	if len(state.view.Attempts) != 2 || selected == nil || selected.StartedSeq != selectedStart || selected.CompletedSeq != 0 ||
		collateral == nil || collateral.StartedSeq != 4 || collateral.CompletedSeq != 0 {
		t.Fatalf("derived event changed ordinary attempt authority: selected=%#v collateral=%#v all=%#v", selected, collateral, state.view.Attempts)
	}
}
