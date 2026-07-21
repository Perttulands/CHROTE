package formations

import (
	"bytes"
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

			t.Run("rejects_declared_non_contract_reason", func(t *testing.T) {
				eventType, data := consumer.data(map[string]any{"kind": "text", "text": "clean", "reason": "must not become public"})
				schema2SecondRepairRequirePublicRejection(t, eventType, data)
			})
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
		if err := reduceSchema2Event(&state, raw, safe, nil); err == nil {
			t.Fatal("node_waiting published one selected node identity while mutating another")
		}
		after := schema2RepairStructuralFingerprint(t, state.view)
		if !bytes.Equal(after, before) {
			t.Fatalf("rejected node_waiting changed structural state\nbefore: %s\nafter:  %s", before, after)
		}
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
