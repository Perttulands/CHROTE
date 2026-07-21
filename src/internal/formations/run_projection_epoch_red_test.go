package formations

import (
	"strconv"
	"strings"
	"testing"
)

func TestSchema2EpochLifecycleIsExactAcrossBlockAndResume(t *testing.T) {
	t.Run("run_started_begins_at_epoch_zero", func(t *testing.T) {
		input := schema2ProjectionInput(t, false)
		events := canonicalLedgerEvents(t, input)
		events[0]["epoch"] = uint64(1)
		input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
		_, err := ProjectCanonicalRun(input)
		if err == nil {
			t.Fatal("schema-2 run_started admitted a nonzero initial epoch")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("resumable_block_names_exactly_current_epoch_plus_one", func(t *testing.T) {
		state := schema2EpochTestState()
		block := schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false)
		block["nextEpoch"] = uint64(2)
		err := schema2EpochReduce(t, &state, 20, 0, "run_blocked", block)
		if err == nil {
			t.Fatal("epoch-0 block admitted nextEpoch=2")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("future_envelope_block_cannot_locally_name_its_next_epoch", func(t *testing.T) {
		state := schema2EpochTestState()
		block := schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false)
		block["nextEpoch"] = uint64(2)
		err := schema2EpochReduce(t, &state, 20, 1, "run_blocked", block)
		if err == nil {
			t.Fatal("epoch-0 run admitted a locally consistent epoch-1 block with nextEpoch=2")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	for _, epoch := range []uint64{0, 2} {
		t.Run("resume_requires_exact_next_epoch_"+strconv.FormatUint(epoch, 10), func(t *testing.T) {
			state := schema2EpochTestState()
			schema2EpochReduceValidBlock(t, &state)
			resume := schema2GreenRereviewResume("retry-failed-producer", nil, []any{schema2RepairRetryTarget()})
			err := schema2EpochReduce(t, &state, 21, epoch, "run_resumed", resume)
			if err == nil {
				t.Fatalf("nextEpoch=1 block admitted resume envelope epoch %d", epoch)
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	t.Run("running_event_cannot_advance_epoch_without_resume", func(t *testing.T) {
		state := schema2EpochTestState()
		err := schema2EpochReduce(t, &state, 20, 1, "node_waiting", schema2SecondRepairFixture(t, "node_waiting"))
		if err == nil {
			t.Fatal("ordinary running event advanced epoch without a block/resume transition")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	for _, epoch := range []uint64{0, 1} {
		t.Run("blocked_epoch_rejects_structural_execution_"+strconv.FormatUint(epoch, 10), func(t *testing.T) {
			state := schema2EpochTestState()
			schema2EpochReduceValidBlock(t, &state)
			err := schema2EpochReduce(t, &state, 21, epoch, "node_waiting", schema2SecondRepairFixture(t, "node_waiting"))
			if err == nil {
				t.Fatalf("blocked run admitted node_waiting at epoch %d before resume", epoch)
			}
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	t.Run("blocked_epoch_allows_current_non_authorizing_observation", func(t *testing.T) {
		state := schema2EpochTestState()
		schema2EpochReduceValidBlock(t, &state)
		if err := schema2EpochReduce(t, &state, 21, 0, "slot_binding_observed", schema2SecondRepairFixture(t, "slot_binding_observed")); err != nil {
			t.Fatalf("current-epoch non-authorizing observation rejected while blocked: %v", err)
		}
		if state.view.Status != "blocked" || state.view.Identity.Epoch != 0 {
			t.Fatalf("observation changed blocked lifecycle: status=%q epoch=%d", state.view.Status, state.view.Identity.Epoch)
		}
	})

	t.Run("blocked_epoch_rejects_future_non_authorizing_observation", func(t *testing.T) {
		state := schema2EpochTestState()
		schema2EpochReduceValidBlock(t, &state)
		err := schema2EpochReduce(t, &state, 21, 1, "slot_binding_observed", schema2SecondRepairFixture(t, "slot_binding_observed"))
		if err == nil {
			t.Fatal("epoch-0 block admitted a non-authorizing observation from unopened epoch 1")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("post_resume_event_cannot_regress_epoch", func(t *testing.T) {
		state := schema2EpochTestState()
		schema2EpochReduceValidBlock(t, &state)
		schema2EpochReduceValidResume(t, &state, 21)
		err := schema2EpochReduce(t, &state, 22, 0, "node_waiting", schema2SecondRepairFixture(t, "node_waiting"))
		if err == nil {
			t.Fatal("post-resume structural event regressed current epoch")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("post_resume_event_uses_current_epoch", func(t *testing.T) {
		state := schema2EpochTestState()
		schema2EpochReduceValidBlock(t, &state)
		schema2EpochReduceValidResume(t, &state, 21)
		if err := schema2EpochReduce(t, &state, 22, 1, "node_waiting", schema2SecondRepairFixture(t, "node_waiting")); err != nil {
			t.Fatalf("current-epoch structural event rejected after resume: %v", err)
		}
		if state.view.Identity.Epoch != 1 {
			t.Fatalf("current epoch = %d, want 1", state.view.Identity.Epoch)
		}
	})

	t.Run("second_block_resume_cycle_uses_relative_next_epoch", func(t *testing.T) {
		state := schema2EpochTestState()
		schema2EpochReduceValidBlock(t, &state)
		schema2EpochReduceValidResume(t, &state, 21)
		schema2EpochReduceValidBlockAt(t, &state, 22, 1, 2)
		schema2EpochReduceValidResumeAt(t, &state, 23, 2)
		if err := schema2EpochReduce(t, &state, 24, 2, "node_waiting", schema2SecondRepairFixture(t, "node_waiting")); err != nil {
			t.Fatalf("current epoch-2 structural event rejected after second resume: %v", err)
		}
		if state.view.Status != "running" || state.view.Identity.Epoch != 2 {
			t.Fatalf("second cycle lifecycle = status %q epoch %d, want running/2", state.view.Status, state.view.Identity.Epoch)
		}
	})

	t.Run("second_block_resume_cycle_rejects_prior_epoch", func(t *testing.T) {
		state := schema2EpochTestState()
		schema2EpochReduceValidBlock(t, &state)
		schema2EpochReduceValidResume(t, &state, 21)
		schema2EpochReduceValidBlockAt(t, &state, 22, 1, 2)
		schema2EpochReduceValidResumeAt(t, &state, 23, 2)
		err := schema2EpochReduce(t, &state, 24, 1, "node_waiting", schema2SecondRepairFixture(t, "node_waiting"))
		if err == nil {
			t.Fatal("second resume admitted a structural event from prior epoch 1")
		}
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	t.Run("blocked_run_can_cancel_and_finalize_in_current_epoch", func(t *testing.T) {
		state := schema2EpochTestState()
		schema2EpochReduceValidBlock(t, &state)
		cancel := map[string]any{
			"commandId": projectionTestOtherCmdID, "commandPayloadSha256": strings.Repeat("a", 64),
			"reason": "stop", "requestedBy": "human:test", "openNodeAttempts": []any{},
			"openSlotDispatches": []any{}, "openToolLeases": []any{},
		}
		if err := schema2EpochReduce(t, &state, 21, 0, "run_cancel_requested", cancel); err != nil {
			t.Fatalf("cancel request rejected from blocked run: %v", err)
		}
		if state.view.Status != "canceling" || state.view.Identity.Epoch != 0 {
			t.Fatalf("cancel request lifecycle = status %q epoch %d, want canceling/0", state.view.Status, state.view.Identity.Epoch)
		}
		canceled := map[string]any{
			"cancelRequestSeq": uint64(21), "reason": "stop", "requestedBy": "human:test",
			"nodeAttemptDispositions": []any{}, "slotDispatchDispositions": []any{},
			"reconciledToolLeases": []any{}, "final": true,
		}
		if err := schema2EpochReduce(t, &state, 22, 0, "run_canceled", canceled); err != nil {
			t.Fatalf("run_canceled rejected after blocked cancel request: %v", err)
		}
		if state.view.Status != "canceled" || !state.view.Final || state.view.Identity.Epoch != 0 {
			t.Fatalf("canceled lifecycle = status %q final %t epoch %d, want canceled/true/0", state.view.Status, state.view.Final, state.view.Identity.Epoch)
		}
	})
}

func schema2EpochTestState() projectionState {
	board := &BoardDocument{
		Missions:   []MissionNode{{ID: projectionTestMissionID}},
		Formations: []FormationNode{{ID: projectionTestFormationID}},
		Gates:      []GateNode{{ID: projectionTestGateID}},
	}
	state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, board)
	state.view.Status = "running"
	state.view.Identity.Epoch = 0
	return state
}

func schema2EpochReduceValidBlock(t *testing.T, state *projectionState) {
	t.Helper()
	schema2EpochReduceValidBlockAt(t, state, 20, 0, 1)
}

func schema2EpochReduceValidBlockAt(t *testing.T, state *projectionState, sequence, epoch, nextEpoch uint64) {
	t.Helper()
	block := schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false)
	block["nextEpoch"] = nextEpoch
	if err := schema2EpochReduce(t, state, sequence, epoch, "run_blocked", block); err != nil {
		t.Fatalf("reduce valid epoch-%d block: %v", epoch, err)
	}
}

func schema2EpochReduceValidResume(t *testing.T, state *projectionState, sequence uint64) {
	t.Helper()
	schema2EpochReduceValidResumeAt(t, state, sequence, 1)
}

func schema2EpochReduceValidResumeAt(t *testing.T, state *projectionState, sequence, epoch uint64) {
	t.Helper()
	resume := schema2GreenRereviewResume("retry-failed-producer", nil, []any{schema2RepairRetryTarget()})
	resume["resumedFromSeq"] = state.lastBlockSeq
	if err := schema2EpochReduce(t, state, sequence, epoch, "run_resumed", resume); err != nil {
		t.Fatalf("reduce valid epoch-%d resume: %v", epoch, err)
	}
}

func schema2EpochReduce(t *testing.T, state *projectionState, sequence, epoch uint64, eventType string, data map[string]any) error {
	t.Helper()
	event := schema2Event(projectionTestRunID, sequence, eventType, cloneAny(data).(map[string]any))
	event["epoch"] = epoch
	if eventType == "node_waiting" {
		event["nodeId"] = data["nodeId"]
	}
	raw, err := decodeProjectionEvent(canonicalJSON(t, event), CanonicalRunSourceSchema2, projectionTestRunID)
	if err != nil {
		return err
	}
	safe, err := sanitizeSchema2Event(raw)
	if err != nil {
		return err
	}
	var commands map[string]RunCommandReceipt
	if eventType == "run_resumed" || eventType == "run_cancel_requested" {
		commandID := data["commandId"].(string)
		payloadHash := data["commandPayloadSha256"].(string)
		commandKind := "resume"
		if eventType == "run_cancel_requested" {
			commandKind = "cancel"
		}
		commands = map[string]RunCommandReceipt{
			commandID: RunCommandAppliedReceipt{
				CommandID: commandID, CommandPayloadSHA256: payloadHash, CommandKind: commandKind,
				State: "applied", RunID: projectionTestRunID, EffectSeq: sequence,
			},
		}
	}
	return reduceSchema2Event(state, raw, safe, commands)
}
