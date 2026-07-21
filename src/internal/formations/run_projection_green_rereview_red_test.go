package formations

import (
	"bytes"
	"strings"
	"testing"
)

func TestSchema2GateEvidenceIsTheExactFrozenUnionInEveryConsumer(t *testing.T) {
	consumers := []struct {
		name string
		data func(map[string]any) (string, map[string]any)
	}{
		{name: "gate_kind_result", data: schema2GreenRereviewGateKindEvidence},
		{name: "judge_result", data: schema2GreenRereviewJudgeEvidence},
		{name: "gate_feedback", data: schema2GreenRereviewFeedbackEvidence},
	}
	validArms := []struct {
		name     string
		evidence map[string]any
	}{
		{name: "artifact", evidence: map[string]any{"kind": "artifact", "artifactId": "artifact_report"}},
		{name: "ledger", evidence: map[string]any{"kind": "ledger", "seq": uint64(18)}},
		{name: "text", evidence: map[string]any{"kind": "text", "text": "clean"}},
	}

	for _, consumer := range consumers {
		t.Run(consumer.name, func(t *testing.T) {
			for _, arm := range validArms {
				t.Run("accepts_exact_"+arm.name, func(t *testing.T) {
					eventType, data := consumer.data(cloneAny(arm.evidence).(map[string]any))
					if _, err := schema2SecondRepairSanitize(eventType, data); err != nil {
						t.Fatalf("exact %s evidence rejected by %s: %v", arm.name, consumer.name, err)
					}
				})
			}

			t.Run("accepts_text_at_bound", func(t *testing.T) {
				eventType, data := consumer.data(map[string]any{"kind": "text", "text": string(bytes.Repeat([]byte{'x'}, 64<<10))})
				if _, err := schema2SecondRepairSanitize(eventType, data); err != nil {
					t.Fatalf("bounded text evidence rejected by %s: %v", consumer.name, err)
				}
			})

			t.Run("rejects_text_over_bound", func(t *testing.T) {
				eventType, data := consumer.data(map[string]any{"kind": "text", "text": string(bytes.Repeat([]byte{'x'}, (64<<10)+1))})
				schema2SecondRepairRequirePublicRejection(t, eventType, data)
			})

			for _, arm := range validArms {
				t.Run("rejects_declared_non_contract_reason_on_"+arm.name, func(t *testing.T) {
					evidence := cloneAny(arm.evidence).(map[string]any)
					evidence["reason"] = "must not become public"
					eventType, data := consumer.data(evidence)
					schema2SecondRepairRequirePublicRejection(t, eventType, data)
				})
			}
		})
	}
}

func TestSchema2NodeWaitingRequiresOneExactSelectedNodeIdentity(t *testing.T) {
	board := &BoardDocument{
		Missions:   []MissionNode{{ID: projectionTestMissionID}},
		Formations: []FormationNode{{ID: projectionTestFormationID}},
	}
	data := map[string]any{
		"nodeId": projectionTestMissionID, "neededInputs": 1, "readyInputs": 0, "totalInputs": 1,
		"waitingFor": []any{"edge_root_work"},
	}

	t.Run("exact_identity_mutates_only_that_node", func(t *testing.T) {
		state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, board)
		state.view.Status = "running"
		event := schema2Event(projectionTestRunID, 3, "node_waiting", cloneAny(data).(map[string]any))
		event["nodeId"] = projectionTestMissionID
		raw, safe := schema2RepairDecodeSafeEvent(t, event)
		if err := reduceSchema2Event(&state, raw, safe, nil); err != nil {
			t.Fatalf("reduce exact node_waiting identity: %v", err)
		}
		if mission := state.node(projectionTestMissionID); mission == nil || mission.Status != "waiting" {
			t.Fatalf("selected mission was not marked waiting: %#v", mission)
		}
		if formation := state.node(projectionTestFormationID); formation == nil || formation.Status == "waiting" {
			t.Fatalf("unselected formation was mutated: %#v", formation)
		}
	})

	t.Run("two_valid_selected_identities_cannot_disagree", func(t *testing.T) {
		state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, board)
		state.view.Status = "running"
		before := schema2RepairStructuralFingerprint(t, state.view)
		event := schema2Event(projectionTestRunID, 3, "node_waiting", cloneAny(data).(map[string]any))
		event["nodeId"] = projectionTestFormationID
		raw, safe := schema2RepairDecodeSafeEvent(t, event)
		err := reduceSchema2Event(&state, raw, safe, nil)
		if err == nil {
			t.Fatal("node_waiting published one selected node identity while mutating another")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
		after := schema2RepairStructuralFingerprint(t, state.view)
		if !bytes.Equal(after, before) {
			t.Fatalf("rejected node_waiting changed structural state\nbefore: %s\nafter:  %s", before, after)
		}
	})
}

func TestSchema2RunBlockedUsesTheFrozenScopeAndPolicyUnion(t *testing.T) {
	open := []any{schema2RepairOpenDispatchSnapshot(t)}
	valid := []struct {
		name string
		data map[string]any
	}{
		{name: "node_retry", data: schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false)},
		{name: "run_reattach", data: schema2GreenRereviewBlock("run", "reattach_only", open, false)},
		{name: "node_reattach", data: schema2GreenRereviewBlock("node", "reattach_only", open, false)},
		{name: "gate_reattach", data: schema2GreenRereviewBlock("gate", "reattach_only", open, false)},
		{name: "gate_reattach_with_causal_node", data: schema2GreenRereviewBlock("gate", "reattach_only", open, true)},
		{name: "run_new_run_empty", data: schema2GreenRereviewBlock("run", "new_run_required", nil, false)},
		{name: "run_new_run_unmatched", data: schema2GreenRereviewBlock("run", "new_run_required", open, false)},
		{name: "node_new_run_empty", data: schema2GreenRereviewBlock("node", "new_run_required", nil, false)},
		{name: "node_new_run_unmatched", data: schema2GreenRereviewBlock("node", "new_run_required", open, false)},
		{name: "gate_new_run_empty", data: schema2GreenRereviewBlock("gate", "new_run_required", nil, false)},
		{name: "gate_new_run_unmatched", data: schema2GreenRereviewBlock("gate", "new_run_required", open, false)},
		{name: "gate_new_run_with_causal_node", data: schema2GreenRereviewBlock("gate", "new_run_required", open, true)},
	}
	for _, test := range valid {
		t.Run("accepts_"+test.name, func(t *testing.T) {
			safe, err := schema2SecondRepairSanitize("run_blocked", cloneAny(test.data).(map[string]any))
			if err != nil {
				t.Fatalf("valid run_blocked arm %s rejected: %v", test.name, err)
			}
			if test.data["resumePolicy"] == "new_run_required" {
				data := schema2SecondRepairPublicData(t, safe)
				schema2SecondRepairRequireJSONMemberAbsent(t, data, "nextEpoch")
			}
		})
	}

	invalid := []struct {
		name   string
		data   map[string]any
		mutate func(map[string]any)
	}{
		{name: "run_scope_forbids_node", data: schema2GreenRereviewBlock("run", "new_run_required", nil, false), mutate: func(data map[string]any) { data["blockedNodeId"] = projectionTestMissionID }},
		{name: "run_scope_forbids_gate", data: schema2GreenRereviewBlock("run", "new_run_required", nil, false), mutate: func(data map[string]any) { data["blockedGateId"] = projectionTestGateID }},
		{name: "node_scope_requires_node", data: schema2GreenRereviewBlock("node", "new_run_required", nil, false), mutate: func(data map[string]any) { delete(data, "blockedNodeId") }},
		{name: "node_scope_forbids_gate", data: schema2GreenRereviewBlock("node", "new_run_required", nil, false), mutate: func(data map[string]any) { data["blockedGateId"] = projectionTestGateID }},
		{name: "gate_scope_requires_gate", data: schema2GreenRereviewBlock("gate", "new_run_required", nil, false), mutate: func(data map[string]any) { delete(data, "blockedGateId") }},
		{name: "retry_is_node_only", data: schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false), mutate: func(data map[string]any) { data["blockScope"] = "run"; delete(data, "blockedNodeId") }},
		{name: "retry_rejects_gate_scope", data: schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false), mutate: func(data map[string]any) {
			data["blockScope"] = "gate"
			delete(data, "blockedNodeId")
			data["blockedGateId"] = projectionTestGateID
		}},
		{name: "retry_requires_resume_allowed", data: schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false), mutate: func(data map[string]any) { data["resumeAllowed"] = false }},
		{name: "retry_requires_empty_dispatches", data: schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false), mutate: func(data map[string]any) { data["openDispatches"] = cloneAny(open) }},
		{name: "retry_requires_one_target", data: schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false), mutate: func(data map[string]any) { data["retryTargets"] = []any{} }},
		{name: "retry_rejects_two_targets", data: schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false), mutate: func(data map[string]any) {
			data["retryTargets"] = []any{schema2RepairRetryTarget(), schema2GreenRereviewOtherRetryTarget()}
		}},
		{name: "retry_requires_next_epoch", data: schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false), mutate: func(data map[string]any) { delete(data, "nextEpoch") }},
		{name: "reattach_requires_resume_allowed", data: schema2GreenRereviewBlock("node", "reattach_only", open, false), mutate: func(data map[string]any) { data["resumeAllowed"] = false }},
		{name: "reattach_requires_nonempty_dispatches", data: schema2GreenRereviewBlock("node", "reattach_only", open, false), mutate: func(data map[string]any) { data["openDispatches"] = []any{} }},
		{name: "reattach_requires_empty_targets", data: schema2GreenRereviewBlock("node", "reattach_only", open, false), mutate: func(data map[string]any) { data["retryTargets"] = []any{schema2RepairRetryTarget()} }},
		{name: "reattach_requires_next_epoch", data: schema2GreenRereviewBlock("run", "reattach_only", open, false), mutate: func(data map[string]any) { delete(data, "nextEpoch") }},
		{name: "new_run_forbids_resume", data: schema2GreenRereviewBlock("run", "new_run_required", nil, false), mutate: func(data map[string]any) { data["resumeAllowed"] = true }},
		{name: "new_run_requires_empty_targets", data: schema2GreenRereviewBlock("run", "new_run_required", nil, false), mutate: func(data map[string]any) { data["retryTargets"] = []any{schema2RepairRetryTarget()} }},
		{name: "new_run_forbids_next_epoch", data: schema2GreenRereviewBlock("node", "new_run_required", nil, false), mutate: func(data map[string]any) { data["nextEpoch"] = uint64(1) }},
	}
	for _, test := range invalid {
		t.Run("rejects_"+test.name, func(t *testing.T) {
			data := cloneAny(test.data).(map[string]any)
			test.mutate(data)
			schema2SecondRepairRequirePublicRejection(t, "run_blocked", data)
		})
	}
}

func TestSchema2RunResumedUsesTheFrozenModeUnion(t *testing.T) {
	open := []any{schema2RepairOpenDispatchSnapshot(t)}
	valid := []struct {
		name string
		data map[string]any
	}{
		{name: "retry_failed_producer", data: schema2GreenRereviewResume("retry-failed-producer", nil, []any{schema2RepairRetryTarget()})},
		{name: "reattach", data: schema2GreenRereviewResume("reattach", open, nil)},
	}
	for _, test := range valid {
		t.Run("accepts_"+test.name, func(t *testing.T) {
			if _, err := schema2SecondRepairSanitize("run_resumed", cloneAny(test.data).(map[string]any)); err != nil {
				t.Fatalf("valid run_resumed mode %s rejected: %v", test.name, err)
			}
		})
	}

	invalid := []struct {
		name string
		data map[string]any
	}{
		{name: "retry_with_open_dispatch", data: schema2GreenRereviewResume("retry-failed-producer", open, []any{schema2RepairRetryTarget()})},
		{name: "retry_without_target", data: schema2GreenRereviewResume("retry-failed-producer", nil, nil)},
		{name: "retry_with_two_targets", data: schema2GreenRereviewResume("retry-failed-producer", nil, []any{schema2RepairRetryTarget(), schema2GreenRereviewOtherRetryTarget()})},
		{name: "reattach_without_dispatch", data: schema2GreenRereviewResume("reattach", nil, nil)},
		{name: "reattach_with_retry_target", data: schema2GreenRereviewResume("reattach", open, []any{schema2RepairRetryTarget()})},
	}
	for _, test := range invalid {
		t.Run("rejects_"+test.name, func(t *testing.T) {
			schema2SecondRepairRequirePublicRejection(t, "run_resumed", cloneAny(test.data).(map[string]any))
		})
	}
}

func TestSchema2RunResumeMustMatchTheExactPriorBlock(t *testing.T) {
	t.Run("existing_reattach_pair_is_valid", func(t *testing.T) {
		input, _ := schema2OpenDispatchLifecycleInput(t, false)
		if _, err := ProjectCanonicalRun(input); err != nil {
			t.Fatalf("valid reattach block/resume pair rejected: %v", err)
		}
	})

	t.Run("resumed_from_sequence_names_the_block", func(t *testing.T) {
		input, _ := schema2OpenDispatchLifecycleInput(t, false)
		events := canonicalLedgerEvents(t, input)
		resumed := events[len(events)-1]["data"].(map[string]any)
		resumed["resumedFromSeq"] = uint64(1)
		input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
		projection, err := ProjectCanonicalRun(input)
		if err == nil {
			t.Fatalf("run resumed from a non-block sequence: %#v", ProjectRunView(projection))
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("valid_retry_pair", func(t *testing.T) {
		block := schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false)
		resume := schema2GreenRereviewResume("retry-failed-producer", nil, []any{schema2RepairRetryTarget()})
		if err := schema2GreenRereviewReducePair(t, block, resume); err != nil {
			t.Fatalf("valid retry block/resume pair rejected: %v", err)
		}
	})

	t.Run("retry_targets_are_exact_carry", func(t *testing.T) {
		block := schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false)
		resume := schema2GreenRereviewResume("retry-failed-producer", nil, []any{schema2GreenRereviewOtherRetryTarget()})
		err := schema2GreenRereviewReducePair(t, block, resume)
		if err == nil {
			t.Fatal("run resumed with a different retry target than the frozen block")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("new_run_required_cannot_resume", func(t *testing.T) {
		block := schema2GreenRereviewBlock("run", "new_run_required", nil, false)
		resume := schema2GreenRereviewResume("retry-failed-producer", nil, []any{schema2RepairRetryTarget()})
		err := schema2GreenRereviewReducePair(t, block, resume)
		if err == nil {
			t.Fatal("new_run_required block reopened through run_resumed")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})
}

func schema2GreenRereviewGateKindEvidence(evidence map[string]any) (string, map[string]any) {
	return "gate_kind_result", schema2RepairWithEvidence(schema2RepairGateKindResultData(), evidence)
}

func schema2GreenRereviewJudgeEvidence(evidence map[string]any) (string, map[string]any) {
	data := schema2RepairJudgeResultData()
	result := data["result"].(map[string]any)
	result["evidence"] = []any{evidence}
	data["resultSha256"] = projectionSHA256(mustMarshalJSONNoTest(result))
	return "judge_result", data
}

func schema2GreenRereviewFeedbackEvidence(evidence map[string]any) (string, map[string]any) {
	data := schema2RepairGateVerdictData()
	data["feedbackPayload"].(map[string]any)["evidence"] = []any{evidence}
	return "gate_verdict", data
}

func schema2GreenRereviewBlock(scope, policy string, openDispatches []any, causalNode bool) map[string]any {
	data := map[string]any{
		"reason": "operator action required", "blockScope": scope, "resumeAllowed": policy != "new_run_required",
		"resumePolicy": policy, "openDispatches": cloneAny(openDispatches), "retryTargets": []any{},
	}
	switch scope {
	case "node":
		data["blockedNodeId"] = projectionTestFormationID
	case "gate":
		data["blockedGateId"] = projectionTestGateID
		if causalNode {
			data["blockedNodeId"] = projectionTestFormationID
		}
	}
	switch policy {
	case "retry_failed_producer":
		data["retryTargets"] = []any{schema2RepairRetryTarget()}
		data["nextEpoch"] = uint64(1)
	case "reattach_only":
		data["nextEpoch"] = uint64(1)
	}
	return data
}

func schema2GreenRereviewResume(mode string, openDispatches, retryTargets []any) map[string]any {
	return map[string]any{
		"commandId": projectionTestOtherCmdID, "commandPayloadSha256": strings.Repeat("a", 64),
		"resumedFromSeq": uint64(20), "resumedBy": "human:test", "resumeMode": mode, "reason": "continue",
		"openDispatches": cloneAny(openDispatches), "retryTargets": cloneAny(retryTargets),
	}
}

func schema2GreenRereviewOtherRetryTarget() map[string]any {
	return map[string]any{
		"nodeId": projectionTestMissionID, "attempt": 2, "outputPortIds": []any{"out"},
		"outcomeSeqs": []any{uint64(17)}, "deliveredEdges": []any{},
	}
}

func schema2GreenRereviewReducePair(t *testing.T, blockData, resumeData map[string]any) error {
	t.Helper()
	board := &BoardDocument{
		Missions:   []MissionNode{{ID: projectionTestMissionID}},
		Formations: []FormationNode{{ID: projectionTestFormationID}},
		Gates:      []GateNode{{ID: projectionTestGateID}},
	}
	state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, board)
	state.view.Status = "running"

	blockEvent := schema2Event(projectionTestRunID, 20, "run_blocked", cloneAny(blockData).(map[string]any))
	blockRaw, err := decodeProjectionEvent(canonicalJSON(t, blockEvent), CanonicalRunSourceSchema2, projectionTestRunID)
	if err != nil {
		return err
	}
	blockSafe, err := sanitizeSchema2Event(blockRaw)
	if err != nil {
		return err
	}
	if err := reduceSchema2Event(&state, blockRaw, blockSafe, nil); err != nil {
		return err
	}

	resume := cloneAny(resumeData).(map[string]any)
	resume["resumedFromSeq"] = uint64(20)
	resumeEvent := schema2Event(projectionTestRunID, 21, "run_resumed", resume)
	resumeEvent["epoch"] = uint64(1)
	resumeRaw, err := decodeProjectionEvent(canonicalJSON(t, resumeEvent), CanonicalRunSourceSchema2, projectionTestRunID)
	if err != nil {
		return err
	}
	resumeSafe, err := sanitizeSchema2Event(resumeRaw)
	if err != nil {
		return err
	}
	payloadHash := resume["commandPayloadSha256"].(string)
	commands := map[string]RunCommandReceipt{
		projectionTestOtherCmdID: RunCommandAppliedReceipt{
			CommandID: projectionTestOtherCmdID, CommandPayloadSHA256: payloadHash, CommandKind: "resume",
			State: "applied", RunID: projectionTestRunID, EffectSeq: 21,
		},
	}
	return reduceSchema2Event(&state, resumeRaw, resumeSafe, commands)
}
