package formations

import (
	"strconv"
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
	block := schema2GreenRereviewBlock("node", "retry_failed_producer", nil, false)
	if err := schema2EpochReduce(t, state, 20, 0, "run_blocked", block); err != nil {
		t.Fatalf("reduce valid epoch-0 block: %v", err)
	}
}

func schema2EpochReduceValidResume(t *testing.T, state *projectionState, sequence uint64) {
	t.Helper()
	resume := schema2GreenRereviewResume("retry-failed-producer", nil, []any{schema2RepairRetryTarget()})
	if err := schema2EpochReduce(t, state, sequence, 1, "run_resumed", resume); err != nil {
		t.Fatalf("reduce valid epoch-1 resume: %v", err)
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
	if eventType == "run_resumed" {
		commandID := data["commandId"].(string)
		payloadHash := data["commandPayloadSha256"].(string)
		commands = map[string]RunCommandReceipt{
			commandID: RunCommandAppliedReceipt{
				CommandID: commandID, CommandPayloadSHA256: payloadHash, CommandKind: "resume",
				State: "applied", RunID: projectionTestRunID, EffectSeq: sequence,
			},
		}
	}
	return reduceSchema2Event(state, raw, safe, commands)
}
