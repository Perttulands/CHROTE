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

	t.Run("attempt_bearing_gate_error_selects_only_exact_gate_attempt", func(t *testing.T) {
		state, final := schema2FinalGreenGateFailureFinal(t)
		if err := schema2LifecycleReduce(t, &state, 21, "run_failed", final); err != nil {
			t.Fatalf("exact Gate-selected failure final rejected: %v", err)
		}
		schema2RequireNodeAttemptDisposition(t, &state, projectionTestGateID, "failed", 21)
		schema2RequireNodeAttemptDisposition(t, &state, projectionTestMissionID, "abandoned", 21)
	})

	for _, mutation := range []struct {
		name   string
		nodeID string
		value  string
	}{
		{name: "selected_gate_is_abandoned", nodeID: projectionTestGateID, value: "abandoned_non_authorizing"},
		{name: "collateral_mission_is_selected", nodeID: projectionTestMissionID, value: "failed_non_authorizing"},
	} {
		t.Run("attempt_bearing_gate_error_rejects_"+mutation.name+"_without_mutation", func(t *testing.T) {
			state, final := schema2FinalGreenGateFailureFinal(t)
			for _, member := range final["nodeAttemptDispositions"].([]any) {
				disposition := member.(map[string]any)
				if disposition["nodeId"] == mutation.nodeID {
					disposition["disposition"] = mutation.value
				}
			}
			schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 21, "run_failed", final)
		})
	}

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

func TestTask1FinalGreenFindingsDerivedToolAndGateAuthorityRequireNodeStarted(t *testing.T) {
	t.Run("tool_dispatch_cannot_open_an_ordinary_attempt", func(t *testing.T) {
		state := schema2FinalGreenAllNodeState()
		schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 3, "tool_dispatch", schema2RepairToolDispatchData())
	})

	t.Run("gate_evaluating_cannot_open_an_ordinary_attempt", func(t *testing.T) {
		state := schema2FinalGreenAllNodeState()
		schema2RequireLifecycleReducerErrorWithoutMutation(t, &state, 3, "gate_evaluating", schema2FinalGreenGateEvaluatingData())
	})
}

func TestTask1FinalGreenFindingsMaterializationResultsRemainOpenUntilNodeOutput(t *testing.T) {
	t.Run("formation_result_retains_materialization_but_does_not_close_or_publish_outputs", func(t *testing.T) {
		state, result := schema2FinalGreenFormationResultState(t)
		attempt := state.existingAttempt(projectionTestFormationID, 1)
		if attempt == nil || attempt.Status != "running" || attempt.CompletedSeq != 0 || attempt.Disposition != "" || len(attempt.Outputs) != 0 || len(state.view.Outputs) != 0 {
			t.Fatalf("formation_result closed or published its ordinary attempt: attempt=%#v outputs=%#v result=%#v", attempt, state.view.Outputs, result)
		}
	})

	t.Run("tool_result_closes_public_lease_but_keeps_attempt_open", func(t *testing.T) {
		state, _ := schema2FinalGreenToolResultState(t)
		attempt := state.existingAttempt("tool_normalize", 1)
		if attempt == nil || attempt.Status != "running" || attempt.CompletedSeq != 0 || attempt.Disposition != "" || len(attempt.Outputs) != 0 || len(state.view.Outputs) != 0 {
			t.Fatalf("tool_result closed or published its ordinary attempt: attempt=%#v outputs=%#v", attempt, state.view.Outputs)
		}
		if _, open := state.toolLeases["toollease_01KXNP6VY3227H78329V52CKF8"]; open {
			t.Fatal("tool_result did not close its exact public Tool lease")
		}
	})

	t.Run("tool_result_accepts_and_durably_registers_nonempty_typed_artifacts", func(t *testing.T) {
		state, _ := schema2LifecyclePublicState(t, "tool")
		result := schema2FinalGreenToolResultData()
		if err := schema2LifecycleReduce(t, &state, 5, "tool_result", result); err != nil {
			t.Fatalf("valid nonempty Tool artifact registration rejected: %v", err)
		}
		if _, registered := state.artifactIndex["artifact_tool"]; !registered {
			t.Fatalf("tool_result did not durably register artifact_tool: %#v", state.view.Artifacts)
		}
	})

	t.Run("formation_result_without_node_output_cannot_finalize_success", func(t *testing.T) {
		state, _ := schema2FinalGreenFormationResultState(t)
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 21, "run_succeeded", schema2TerminalSucceededData())
	})

	t.Run("tool_result_without_node_output_cannot_finalize_success", func(t *testing.T) {
		state, _ := schema2FinalGreenToolResultState(t)
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 7, "run_succeeded", schema2TerminalSucceededData())
	})

	t.Run("formation_result_then_exact_node_output_closes_attempt", func(t *testing.T) {
		state, result := schema2FinalGreenFormationResultState(t)
		output := schema2FinalGreenNodeOutputFromResult("formation", 20, result)
		if err := schema2LifecycleReduce(t, &state, 21, "node_output", output); err != nil {
			t.Fatalf("exact Formation node_output rejected: %v", err)
		}
		schema2FinalGreenRequireClosedAttemptAt(t, &state, projectionTestFormationID, 1, 21, "port_out")
	})

	t.Run("tool_result_then_exact_node_output_closes_attempt", func(t *testing.T) {
		state, result := schema2FinalGreenToolResultState(t)
		output := schema2FinalGreenNodeOutputFromResult("tool", 6, result)
		if err := schema2LifecycleReduce(t, &state, 7, "node_output", output); err != nil {
			t.Fatalf("exact Tool node_output rejected: %v", err)
		}
		schema2FinalGreenRequireClosedAttemptAt(t, &state, "tool_normalize", 1, 7, "port_tool_out")
	})
}

func TestTask1FinalGreenFindingsNodeOutputRequiresExactMaterializationParity(t *testing.T) {
	for _, mismatch := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "producer_kind", mutate: func(output map[string]any) {
			output["producedBy"].(map[string]any)["kind"] = "mission"
		}},
		{name: "producer_outcome_sequence", mutate: func(output map[string]any) {
			output["producedBy"].(map[string]any)["outcomeSeq"] = uint64(19)
		}},
		{name: "status", mutate: func(output map[string]any) {
			output["status"] = "needs-review"
		}},
		{name: "output_keys", mutate: func(output map[string]any) {
			outputs := output["outputs"].(map[string]any)
			outputs["port_other"] = outputs["port_out"]
			delete(outputs, "port_out")
		}},
		{name: "output_bytes_and_retained_hash", mutate: func(output map[string]any) {
			output["outputs"].(map[string]any)["port_out"] = schema2RepairWorkProjection("changed after immutable result hash")
		}},
		{name: "report_artifact", mutate: func(output map[string]any) {
			output["reportArtifactId"] = "artifact_alternative"
		}},
		{name: "artifact_ids", mutate: func(output map[string]any) {
			output["artifactIds"] = []any{"artifact_alternative"}
		}},
		{name: "diff_artifact_ids", mutate: func(output map[string]any) {
			output["diffArtifactIds"] = []any{"artifact_alternative"}
		}},
	} {
		t.Run("formation_rejects_"+mismatch.name+"_without_mutation", func(t *testing.T) {
			state, result := schema2FinalGreenFormationResultState(t)
			output := schema2FinalGreenNodeOutputFromResult("formation", 20, result)
			mismatch.mutate(output)
			schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 21, "node_output", output)
		})
	}

	for _, mismatch := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "producer_kind", mutate: func(output map[string]any) {
			output["producedBy"].(map[string]any)["kind"] = "mission"
		}},
		{name: "producer_outcome_sequence", mutate: func(output map[string]any) {
			output["producedBy"].(map[string]any)["outcomeSeq"] = uint64(4)
		}},
		{name: "status", mutate: func(output map[string]any) {
			output["status"] = "needs-review"
		}},
		{name: "output_keys", mutate: func(output map[string]any) {
			outputs := output["outputs"].(map[string]any)
			outputs["port_other"] = outputs["port_tool_out"]
			delete(outputs, "port_tool_out")
		}},
		{name: "output_bytes_and_retained_hash", mutate: func(output map[string]any) {
			output["outputs"].(map[string]any)["port_tool_out"] = schema2RepairWorkProjection(`{"normalized":false}`)
		}},
		{name: "artifact_ids", mutate: func(output map[string]any) {
			output["artifactIds"] = []any{"artifact_alternative"}
		}},
		{name: "invented_report_artifact", mutate: func(output map[string]any) {
			output["reportArtifactId"] = "artifact_alternative"
		}},
		{name: "invented_diff_artifact", mutate: func(output map[string]any) {
			output["diffArtifactIds"] = []any{"artifact_alternative"}
		}},
	} {
		t.Run("tool_rejects_"+mismatch.name+"_without_mutation", func(t *testing.T) {
			state, result := schema2FinalGreenToolResultState(t)
			output := schema2FinalGreenNodeOutputFromResult("tool", 6, result)
			mismatch.mutate(output)
			schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 7, "node_output", output)
		})
	}
}

func TestTask1FinalGreenFindingsDuplicateMissingOrStaleMaterializationCannotCloseAttempt(t *testing.T) {
	t.Run("duplicate_formation_result_is_rejected_without_mutation", func(t *testing.T) {
		state, result := schema2FinalGreenFormationResultState(t)
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 21, "formation_result", result)
	})

	t.Run("duplicate_tool_result_is_rejected_without_mutation", func(t *testing.T) {
		state, result := schema2FinalGreenToolResultState(t)
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 7, "tool_result", result)
	})

	t.Run("formation_node_output_without_result_is_rejected_without_mutation", func(t *testing.T) {
		state := schema2EpochTestState()
		if err := schema2LifecycleReduce(t, &state, 3, "node_started", schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")); err != nil {
			t.Fatalf("prepare Formation attempt: %v", err)
		}
		output := schema2FinalGreenNodeOutputFromResult("formation", 4, schema2RepairFormationResultData())
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 4, "node_output", output)
	})

	t.Run("tool_node_output_without_result_is_rejected_without_mutation", func(t *testing.T) {
		state, _ := schema2LifecyclePublicState(t, "tool")
		output := schema2FinalGreenNodeOutputFromResult("tool", 5, schema2RepairToolResultData())
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 5, "node_output", output)
	})

	t.Run("duplicate_node_output_is_rejected_without_mutation", func(t *testing.T) {
		state, result := schema2FinalGreenFormationResultState(t)
		output := schema2FinalGreenNodeOutputFromResult("formation", 20, result)
		if err := schema2LifecycleReduce(t, &state, 21, "node_output", output); err != nil {
			t.Fatalf("prepare exact Formation node_output: %v", err)
		}
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 22, "node_output", output)
	})

	t.Run("stale_node_output_cannot_close_new_latest_attempt", func(t *testing.T) {
		state, result := schema2FinalGreenFormationResultState(t)
		output := schema2FinalGreenNodeOutputFromResult("formation", 20, result)
		if err := schema2LifecycleReduce(t, &state, 21, "node_output", output); err != nil {
			t.Fatalf("prepare first completed Formation attempt: %v", err)
		}
		second := schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")
		second["attempt"] = uint64(2)
		second["reason"] = "resume"
		if err := schema2FinalGreenReduceNodeStarted(t, &state, 22, second, nil); err != nil {
			t.Fatalf("prepare second Formation attempt: %v", err)
		}
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 23, "node_output", output)
	})

	t.Run("stale_formation_result_cannot_replace_prior_materialization_after_new_attempt", func(t *testing.T) {
		state, result := schema2FinalGreenFormationResultState(t)
		output := schema2FinalGreenNodeOutputFromResult("formation", 20, result)
		if err := schema2LifecycleReduce(t, &state, 21, "node_output", output); err != nil {
			t.Fatalf("prepare first completed Formation attempt: %v", err)
		}
		second := schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")
		second["attempt"] = uint64(2)
		second["reason"] = "resume"
		if err := schema2FinalGreenReduceNodeStarted(t, &state, 22, second, nil); err != nil {
			t.Fatalf("prepare second Formation attempt: %v", err)
		}
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 23, "formation_result", result)
	})

	t.Run("stale_tool_result_cannot_replace_prior_materialization_after_new_attempt", func(t *testing.T) {
		state, result := schema2FinalGreenToolResultState(t)
		output := schema2FinalGreenNodeOutputFromResult("tool", 6, result)
		if err := schema2LifecycleReduce(t, &state, 7, "node_output", output); err != nil {
			t.Fatalf("prepare first completed Tool attempt: %v", err)
		}
		second := schema2FinalGreenNodeStartedData("tool_normalize", "tool")
		second["attempt"] = uint64(2)
		second["reason"] = "resume"
		if err := schema2FinalGreenReduceNodeStarted(t, &state, 8, second, nil); err != nil {
			t.Fatalf("prepare second Tool attempt: %v", err)
		}
		schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 9, "tool_result", result)
	})
}

func TestTask1FinalGreenFindingsRunSuccessRequiresExactRegisteredArtifacts(t *testing.T) {
	t.Run("exact_registered_summary_and_outputs_are_valid", func(t *testing.T) {
		state := schema2EpochTestState()
		schema2FinalGreenRegisterArtifact(t, &state, 3, "artifact_summary")
		schema2FinalGreenRegisterArtifact(t, &state, 4, "artifact_output")
		succeeded := schema2TerminalSucceededData()
		succeeded["summaryArtifactId"] = "artifact_summary"
		succeeded["outputArtifactIds"] = []any{"artifact_output"}
		if err := schema2LifecycleReduce(t, &state, 5, "run_succeeded", succeeded); err != nil {
			t.Fatalf("exact registered success artifacts rejected: %v", err)
		}
	})

	for _, missing := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "summary", mutate: func(data map[string]any) {
			data["summaryArtifactId"] = "artifact_missing"
		}},
		{name: "output", mutate: func(data map[string]any) {
			data["outputArtifactIds"] = []any{"artifact_missing"}
		}},
		{name: "one_output_from_mixed_set", mutate: func(data map[string]any) {
			data["outputArtifactIds"] = []any{"artifact_output", "artifact_missing"}
		}},
	} {
		t.Run("unregistered_"+missing.name+"_is_rejected_without_mutation", func(t *testing.T) {
			state := schema2EpochTestState()
			schema2FinalGreenRegisterArtifact(t, &state, 3, "artifact_summary")
			schema2FinalGreenRegisterArtifact(t, &state, 4, "artifact_output")
			succeeded := schema2TerminalSucceededData()
			succeeded["summaryArtifactId"] = "artifact_summary"
			succeeded["outputArtifactIds"] = []any{"artifact_output"}
			missing.mutate(succeeded)
			schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t, &state, 5, "run_succeeded", succeeded)
		})
	}
}

func TestTask1FinalGreenFindingsWriterFencesStayWithinImmutableAuthority(t *testing.T) {
	t.Run("equal_allocated_historical_fences_are_valid", func(t *testing.T) {
		input := schema2ProjectionInput(t, true)
		projection, err := ProjectCanonicalRun(input)
		if err != nil {
			t.Fatalf("equal allocated writer fences rejected: %v", err)
		}
		if projection.view.Audit.LatestWriterFence != 1 {
			t.Fatalf("latest writer fence = %d, want 1", projection.view.Audit.LatestWriterFence)
		}
	})

	t.Run("higher_allocated_epoch_preserves_exact_authority_generation_chain", func(t *testing.T) {
		input := schema2ProjectionInput(t, true)
		previous, current := schema2FinalGreenAllocateNextWriterFence(t, &input, 3)
		events := canonicalLedgerEvents(t, input)
		events[0]["writerFence"] = uint64(1)
		for _, event := range events[1:] {
			event["writerFence"] = uint64(2)
		}
		input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))

		projection, err := ProjectCanonicalRun(input)
		if err != nil {
			t.Fatalf("allocated higher historical writer fence rejected: %v", err)
		}
		if projection.view.Audit.LatestWriterFence != 2 {
			t.Fatalf("latest writer fence = %d, want 2", projection.view.Audit.LatestWriterFence)
		}
		schema2FinalGreenRequireExactAuthoritySuccessor(t, previous, current, 3)
	})

	for _, test := range []struct {
		name    string
		prepare func(*CanonicalRunReadInput)
		fences  []uint64
	}{
		{
			name: "regression_rejects_the_whole_projection",
			prepare: func(input *CanonicalRunReadInput) {
				schema2FinalGreenAllocateNextWriterFence(t, input, 3)
			},
			fences: []uint64{2, 1},
		},
		{name: "fence_equal_to_next_writer_fence_is_unallocated", fences: []uint64{2, 2}},
		{name: "fence_above_next_writer_fence_is_unallocated", fences: []uint64{3, 3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := schema2ProjectionInput(t, true)
			if test.prepare != nil {
				test.prepare(&input)
			}
			events := canonicalLedgerEvents(t, input)
			if len(events) != len(test.fences) {
				t.Fatalf("fixture event count = %d, want %d fences", len(events), len(test.fences))
			}
			for index := range events {
				events[index]["writerFence"] = test.fences[index]
			}
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
			schema2FinalGreenRequireWholeProjectionInvalid(t, input)
		})
	}
}

func schema2FinalGreenGateFailureFinal(t *testing.T) (projectionState, map[string]any) {
	t.Helper()
	state := schema2EpochTestState()
	if err := schema2FinalGreenReduceNodeStarted(t, &state, 3, schema2FinalGreenNodeStartedData(projectionTestGateID, "gate"), nil); err != nil {
		t.Fatalf("prepare exact opened Gate attempt: %v", err)
	}
	if err := schema2LifecycleReduce(t, &state, 4, "gate_evaluating", schema2FinalGreenGateEvaluatingData()); err != nil {
		t.Fatalf("prepare exact Gate evaluation: %v", err)
	}
	if err := schema2FinalGreenReduceNodeStarted(t, &state, 5, schema2FinalGreenNodeStartedData(projectionTestMissionID, "mission"), nil); err != nil {
		t.Fatalf("prepare collateral Mission attempt: %v", err)
	}

	errorData := schema2FinalGreenErrorData("gate")
	errorData["nodeId"] = projectionTestGateID
	errorData["attempt"] = uint64(1)
	errorData["gateId"] = projectionTestGateID
	errorData["gateAttempt"] = uint64(1)
	errorData["relatedSeq"] = uint64(4)
	raw, safe, err := schema2FinalGreenReduceError(t, &state, 6, errorData, nil)
	if err != nil {
		t.Fatalf("prepare exact attempt-bearing Gate error: %v", err)
	}
	if raw.envelope.NodeID != projectionTestGateID || raw.envelope.GateID != projectionTestGateID || raw.envelope.Attempt != 1 {
		t.Fatalf("Gate error envelope identity = node:%q gate:%q attempt:%d", raw.envelope.NodeID, raw.envelope.GateID, raw.envelope.Attempt)
	}
	schema2FinalGreenRequirePublicErrorShape(t, raw, safe)

	open := schema2FinalGreenOpenAuthority(t, &state)
	assertExactJSONValue(t, "Gate error open-attempt authority", open["openNodeAttempts"], []any{
		map[string]any{
			"nodeId": projectionTestGateID, "nodeKind": "gate", "attempt": uint64(1),
			"startSeq": uint64(3), "phase": "gate_evaluating", "phaseSeq": uint64(4),
		},
		map[string]any{
			"nodeId": projectionTestMissionID, "nodeKind": "mission", "attempt": uint64(1),
			"startSeq": uint64(5), "phase": "started", "phaseSeq": uint64(5),
		},
	})
	start := schema2LifecycleStartWithOpen("failure", open)
	start["relatedSeq"] = uint64(6)
	start["failureCause"] = map[string]any{"kind": "error", "errorSeq": uint64(6)}
	if err := schema2LifecycleReduce(t, &state, 20, "run_failure_reconciliation_started", start); err != nil {
		t.Fatalf("prepare Gate-selected failure authority: %v", err)
	}
	final := schema2LifecycleFinalWithDispositions(t, "failure", 20, open)
	for _, field := range []string{"code", "reason", "unrecoverable", "relatedSeq", "failureCause"} {
		final[field] = cloneAny(start[field])
	}
	for _, member := range final["nodeAttemptDispositions"].([]any) {
		disposition := member.(map[string]any)
		if disposition["nodeId"] == projectionTestGateID {
			disposition["disposition"] = "failed_non_authorizing"
		}
	}
	return state, final
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
		if err := schema2FinalGreenReduceNodeStarted(t, &state, 3, schema2FinalGreenNodeStartedData(projectionTestGateID, "gate"), nil); err != nil {
			t.Fatalf("prepare opened Gate attempt: %v", err)
		}
		if err := schema2LifecycleReduce(t, &state, 4, "gate_evaluating", schema2FinalGreenGateEvaluatingData()); err != nil {
			t.Fatalf("prepare gate attempt: %v", err)
		}
		data["errorScope"] = "gate"
		data["nodeId"] = projectionTestGateID
		data["attempt"] = uint64(1)
		data["gateId"] = projectionTestGateID
		data["gateAttempt"] = uint64(1)
		data["relatedSeq"] = uint64(4)
		return state, 5, data
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

func schema2FinalGreenFormationResultState(t *testing.T) (projectionState, map[string]any) {
	t.Helper()
	state := schema2EpochTestState()
	if err := schema2LifecycleReduce(t, &state, 3, "node_started", schema2FinalGreenNodeStartedData(projectionTestFormationID, "formation")); err != nil {
		t.Fatalf("prepare Formation attempt: %v", err)
	}
	for index, artifactID := range []string{"artifact_report", "artifact_output", "artifact_diff", "artifact_alternative"} {
		schema2FinalGreenRegisterArtifact(t, &state, uint64(4+index), artifactID)
	}
	result := schema2RepairFormationResultData()
	result["reportArtifactId"] = "artifact_report"
	result["artifactIds"] = []any{"artifact_output"}
	result["diffArtifactIds"] = []any{"artifact_diff"}
	schema2SecondRepairNormalizeFormationResult(result)
	if err := schema2LifecycleReduce(t, &state, 20, "formation_result", result); err != nil {
		t.Fatalf("prepare immutable Formation result: %v", err)
	}
	return state, result
}

func schema2FinalGreenToolResultState(t *testing.T) (projectionState, map[string]any) {
	t.Helper()
	state, _ := schema2LifecyclePublicState(t, "tool")
	schema2FinalGreenRegisterArtifact(t, &state, 5, "artifact_alternative")
	result := schema2RepairToolResultData()
	if err := schema2LifecycleReduce(t, &state, 6, "tool_result", result); err != nil {
		t.Fatalf("prepare immutable Tool result: %v", err)
	}
	return state, result
}

func schema2FinalGreenToolResultData() map[string]any {
	result := schema2RepairToolResultData()
	registration := schema2FinalGreenArtifactProjection("artifact_tool")
	result["artifactRegistrations"] = []any{cloneAny(registration)}
	result["artifacts"] = []any{cloneAny(registration)}
	return result
}

func schema2FinalGreenNodeOutputFromResult(kind string, resultSequence uint64, result map[string]any) map[string]any {
	output := map[string]any{
		"nodeId": result["nodeId"], "status": result["status"], "outputs": cloneAny(result["outputs"]),
		"reportArtifactId": "", "artifactIds": []any{}, "diffArtifactIds": []any{},
		"producedBy": map[string]any{"kind": kind, "outcomeSeq": resultSequence},
		"timing": map[string]any{
			"startedAt": "2026-07-20T10:00:00Z", "finishedAt": "2026-07-20T10:00:01Z", "durationMs": uint64(1000),
		},
		"deliveredEdges": []any{},
	}
	switch kind {
	case "formation":
		output["reportArtifactId"] = result["reportArtifactId"]
		output["artifactIds"] = cloneAny(result["artifactIds"])
		output["diffArtifactIds"] = cloneAny(result["diffArtifactIds"])
	case "tool":
		status := result["status"].(string)
		if status == "ok" {
			output["status"] = "done"
		} else {
			output["status"] = "failed"
		}
		output["timing"] = cloneAny(result["timing"])
		artifacts := result["artifacts"].([]any)
		ids := make([]any, 0, len(artifacts))
		for _, artifact := range artifacts {
			ids = append(ids, artifact.(map[string]any)["artifactId"])
		}
		output["artifactIds"] = ids
	}
	return output
}

func schema2FinalGreenRegisterArtifact(t *testing.T, state *projectionState, sequence uint64, artifactID string) {
	t.Helper()
	data := map[string]any{
		"artifactProjection": schema2FinalGreenArtifactProjection(artifactID),
		"source":             map[string]any{"kind": "system", "sourceId": "projection_system"},
	}
	if err := schema2LifecycleReduce(t, state, sequence, "artifact_attached", data); err != nil {
		t.Fatalf("register %s: %v", artifactID, err)
	}
}

func schema2FinalGreenArtifactProjection(artifactID string) map[string]any {
	return map[string]any{
		"artifactId": artifactID, "availability": "available", "name": artifactID + ".md",
		"artifact": map[string]any{
			"artifactId": artifactID, "rootId": "run-artifacts", "ref": "reports/" + artifactID + ".md",
			"mediaType": "text/markdown", "sizeBytes": uint64(6), "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
}

func schema2FinalGreenRequireClosedAttemptAt(t *testing.T, state *projectionState, nodeID string, attemptNumber, sequence uint64, portID string) {
	t.Helper()
	attempt := state.existingAttempt(nodeID, attemptNumber)
	if attempt == nil || attempt.Status != "done" || attempt.Disposition != "done" || attempt.CompletedSeq != sequence {
		t.Fatalf("closed attempt = %#v, want done at %d", attempt, sequence)
	}
	for _, output := range state.view.Outputs {
		if output.NodeID == nodeID && output.Attempt == attemptNumber && output.PortID == portID && output.OutcomeSeq == sequence {
			return
		}
	}
	t.Fatalf("missing exact node_output materialization for %s/%d/%s at %d: %#v", nodeID, attemptNumber, portID, sequence, state.view.Outputs)
}

func schema2FinalGreenRequireLifecycleRejectionWithoutMutation(t *testing.T, state *projectionState, sequence uint64, eventType string, data map[string]any) {
	t.Helper()
	before := schema2ReducerStateFingerprint(t, state)
	err := schema2LifecycleReduce(t, state, sequence, eventType, data)
	if err == nil {
		t.Fatalf("%s admitted contradictory materialization/finality authority", eventType)
	}
	if !errors.Is(err, ErrRunEventUnknown) && !errors.Is(err, ErrRunProjectionInvalid) {
		t.Fatalf("%s rejection = %T %v, want typed event or projection rejection", eventType, err, err)
	}
	after := schema2ReducerStateFingerprint(t, state)
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected %s mutated reducer state\nbefore: %s\nafter:  %s", eventType, before, after)
	}
}

func schema2FinalGreenAllocateNextWriterFence(t *testing.T, input *CanonicalRunReadInput, next uint64) ([]byte, []byte) {
	t.Helper()
	previous := canonicalDocumentByRole(t, *input, CanonicalInputRoleSchema2WorkspaceAuthority).Bytes
	current := nextMutableRecordGeneration(t, previous, func(record map[string]any) {
		record["nextWriterFence"] = next
	})
	*input = replaceCanonicalDocument(t, *input, CanonicalInputRoleSchema2WorkspaceAuthority, current)
	return previous, current
}

func schema2FinalGreenRequireExactAuthoritySuccessor(t *testing.T, previous, current []byte, next uint64) {
	t.Helper()
	prior := decodeCanonicalObject(t, previous)
	successor := decodeCanonicalObject(t, current)
	priorRevision := uint64(prior["recordRev"].(float64))
	if successor["recordRev"] != float64(priorRevision+1) || successor["nextWriterFence"] != float64(next) {
		t.Fatalf("authority successor revision/fence = %#v/%#v, want %d/%d", successor["recordRev"], successor["nextWriterFence"], priorRevision+1, next)
	}
	assertExactJSONValue(t, "authority successor predecessor", successor["priorGeneration"], map[string]any{
		"recordRev": priorRevision, "sha256": projectionSHA256(previous),
	})
}

func schema2FinalGreenRequireWholeProjectionInvalid(t *testing.T, input CanonicalRunReadInput) {
	t.Helper()
	projection, err := ProjectCanonicalRun(input)
	if err == nil {
		t.Fatalf("non-authoritative writer-fence history projected: %#v", ProjectRunView(projection))
	}
	requireProjectionError(t, err, ErrRunProjectionInvalid)
	if projection.latestSeq != 0 || len(projection.events) != 0 || projection.view.Schema != "" {
		t.Fatalf("rejected writer-fence history returned a partial projection: %#v", projection)
	}
}

func schema2FinalGreenAllNodeState() projectionState {
	board := &BoardDocument{
		Missions:   []MissionNode{{ID: projectionTestMissionID}},
		Formations: []FormationNode{{ID: projectionTestFormationID}},
		Gates:      []GateNode{{ID: projectionTestGateID}},
		Tools:      []ToolNode{{ID: "tool_normalize"}},
	}
	state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, board)
	state.view.Status = "running"
	state.view.Identity.Epoch = 0
	return state
}

func schema2FinalGreenGateEvaluatingData() map[string]any {
	data := schema2RepairGateEvaluatingData()
	delete(data, "revisionCycleId")
	delete(data, "triggerFeedbackId")
	delete(data, "priorGateSeq")
	return data
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
