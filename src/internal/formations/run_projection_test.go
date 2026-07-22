package formations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	projectionTestRunID       = "run_01KXNP6VY3227H78329V52CKF8"
	projectionTestOtherRunID  = "run_01KXNP6VY3227H78329V52CKF9"
	projectionTestWorkspaceID = "wsa_01KXNP6VY3227H78329V52CKF8"
	projectionTestAuthorityID = "auth_01KXNP6VY3227H78329V52CKF8"
	projectionTestCommandID   = "cmd_01KXNP6VY3227H78329V52CKF8"
	projectionTestOtherCmdID  = "cmd_01KXNP6VY3227H78329V52CKF9"
	projectionTestBoardID     = "brd_projection"
	projectionTestBoardSlug   = "projection"
	projectionTestMissionID   = "mis_root"
	projectionTestFormationID = "fmn_work"
	projectionTestGateID      = "gate_review"
)

var lowercaseSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestProjectCanonicalRunStructuralTransitions(t *testing.T) {
	tests := []struct {
		name             string
		events           []map[string]any
		wantStatus       string
		wantFinal        bool
		wantNodeStatus   string
		wantGateStatus   string
		wantBlocks       int
		wantEscalations  int
		wantConsumed     uint64
		wantCurrentEpoch uint64
	}{
		{
			name:         "start",
			events:       []map[string]any{schema1StartedEvent(projectionTestRunID)},
			wantStatus:   "running",
			wantConsumed: 1,
		},
		{
			name: "node waiting",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1Event(projectionTestRunID, 2, RunEventNodeWaiting, map[string]any{
					"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"},
				}),
			},
			wantStatus:     "running",
			wantNodeStatus: "waiting",
			wantConsumed:   2,
		},
		{
			name: "node start",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1NodeStartedEvent(projectionTestRunID, 2),
			},
			wantStatus:     "running",
			wantNodeStatus: "running",
			wantConsumed:   2,
		},
		{
			name: "node terminal",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1NodeStartedEvent(projectionTestRunID, 2),
				schema1NodeOutputEvent(projectionTestRunID, 3, "done"),
			},
			wantStatus:     "running",
			wantNodeStatus: "done",
			wantConsumed:   3,
		},
		{
			name: "gate open",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1GateEvaluatingEvent(projectionTestRunID, 2),
			},
			wantStatus:     "running",
			wantGateStatus: "evaluating",
			wantConsumed:   2,
		},
		{
			name: "gate verdict",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1GateEvaluatingEvent(projectionTestRunID, 2),
				schema1GateVerdictEvent(projectionTestRunID, 3, "pass"),
			},
			wantStatus:     "running",
			wantGateStatus: "passed",
			wantConsumed:   3,
		},
		{
			name: "escalation open",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1EscalationEvent(projectionTestRunID, 2, true),
				schema1BlockedEvent(projectionTestRunID, 3, true, []any{}),
			},
			wantStatus:      "blocked",
			wantBlocks:      1,
			wantEscalations: 1,
			wantConsumed:    3,
		},
		{
			name: "escalation block resolved by resume while evidence remains",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1EscalationEvent(projectionTestRunID, 2, true),
				schema1BlockedEvent(projectionTestRunID, 3, true, []any{}),
				schema1ResumedEvent(projectionTestRunID, 4, 3, []any{}),
			},
			wantStatus:       "running",
			wantBlocks:       1,
			wantEscalations:  1,
			wantConsumed:     4,
			wantCurrentEpoch: 1,
		},
		{
			name: "run canceled",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1Event(projectionTestRunID, 2, RunEventCanceled, map[string]any{
					"reason": "operator stop", "requestedBy": "human:test", "softInterruptedSlots": []string{}, "final": true,
				}),
			},
			wantStatus:   "canceled",
			wantFinal:    true,
			wantConsumed: 2,
		},
		{
			name: "run failed",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1Event(projectionTestRunID, 2, RunEventFailed, map[string]any{
					"code": "projection_fixture", "reason": "failed", "boundary": "engine", "recoverable": false, "relatedSeq": 1, "final": true,
				}),
			},
			wantStatus:   "failed",
			wantFinal:    true,
			wantConsumed: 2,
		},
		{
			name: "run succeeded",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1Event(projectionTestRunID, 2, RunEventSucceeded, map[string]any{"final": true}),
			},
			wantStatus:   "succeeded",
			wantFinal:    true,
			wantConsumed: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t, test.events...))
			view := ProjectRunView(projection)
			if view.Status != test.wantStatus || view.Final != test.wantFinal {
				t.Fatalf("status/final = %q/%v, want %q/%v", view.Status, view.Final, test.wantStatus, test.wantFinal)
			}
			if view.Cursor != test.wantConsumed || view.Audit.ConsumedEventCount != test.wantConsumed {
				t.Fatalf("cursor/audit count = %d/%d, want %d", view.Cursor, view.Audit.ConsumedEventCount, test.wantConsumed)
			}
			if view.Audit.StartSeq != 1 || view.Audit.EventSchema != 1 {
				t.Fatalf("audit = %+v, want schema 1 start sequence 1", view.Audit)
			}
			if view.Identity.Epoch != test.wantCurrentEpoch {
				t.Fatalf("epoch = %d, want %d", view.Identity.Epoch, test.wantCurrentEpoch)
			}
			if test.wantNodeStatus != "" {
				node := findProjectedNode(t, view, projectionTestFormationID)
				if node.Status != test.wantNodeStatus {
					t.Fatalf("node status = %q, want %q", node.Status, test.wantNodeStatus)
				}
			}
			if test.wantGateStatus != "" {
				gate := findProjectedGate(t, view, projectionTestGateID)
				if gate.Status != test.wantGateStatus {
					t.Fatalf("gate status = %q, want %q", gate.Status, test.wantGateStatus)
				}
			}
			if len(view.Blocks) != test.wantBlocks || len(view.Escalations) != test.wantEscalations {
				t.Fatalf("blocks/escalations = %d/%d, want %d/%d", len(view.Blocks), len(view.Escalations), test.wantBlocks, test.wantEscalations)
			}
		})
	}
}

func TestProjectCanonicalRunStructuralTransitionSubstructuresAndOrdering(t *testing.T) {
	t.Run("start identity nodes and audit", func(t *testing.T) {
		view := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID))))
		assertExactJSONValue(t, "start identity", view.Identity, map[string]any{
			"boardId": projectionTestBoardID, "boardSlug": projectionTestBoardSlug, "boardRev": 7,
			"runRoot":   map[string]any{"kind": "mission", "nodeId": projectionTestMissionID},
			"missionId": projectionTestMissionID, "beadId": "ctx-7i1.1", "epoch": 0, "redact": false,
		})
		assertExactJSONValue(t, "start audit", view.Audit, map[string]any{"eventSchema": 1, "startSeq": 1, "consumedEventCount": 1})
		assertStringOrder(t, "frozen graph node order", projectedNodeIDs(view), []string{projectionTestMissionID, projectionTestFormationID, projectionTestGateID})
	})

	t.Run("schema 2 start and activation audit", func(t *testing.T) {
		for _, activated := range []bool{false, true} {
			input := schema2ProjectionInput(t, activated)
			view := ProjectRunView(mustProjectCanonicalFixture(t, input))
			events := canonicalLedgerEvents(t, input)
			started := events[0]["data"].(map[string]any)
			want := map[string]any{
				"eventSchema": 2, "authoritySchema": 2, "startSeq": 1, "consumedEventCount": len(events),
				"admissionCommandId": started["admissionCommandId"], "commandPayloadSha256": started["commandPayloadSha256"],
				"workspaceAdmissionSeq": 1, "admissionPolicyRev": 2, "admissionPolicySha256": started["admissionPolicySha256"],
				"latestWriterFence": 1, "graphSnapshotSha256": started["graphSnapshotSha256"],
				"bindingProjectionSha256": started["bindingProjectionSha256"],
			}
			if activated {
				want["activationPolicyRev"] = 2
				want["activationPolicySha256"] = started["admissionPolicySha256"]
			}
			assertExactJSONValue(t, fmt.Sprintf("schema 2 activated=%v audit", activated), view.Audit, want)
		}
	})

	t.Run("node waiting", func(t *testing.T) {
		view := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID),
			schema1Event(projectionTestRunID, 2, RunEventNodeWaiting, map[string]any{
				"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"},
			}),
		)))
		assertExactJSONValue(t, "waiting node", findProjectedNode(t, view, projectionTestFormationID), map[string]any{
			"nodeId": projectionTestFormationID, "kind": "formation", "status": "waiting",
			"readiness": map[string]any{"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"}},
			"attempts":  []any{}, "outputs": []any{}, "gates": []any{}, "sessions": []any{},
		})
		assertExactJSONValue(t, "waiting audit", view.Audit, map[string]any{"eventSchema": 1, "startSeq": 1, "consumedEventCount": 2})
	})

	t.Run("node start", func(t *testing.T) {
		view := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID), schema1NodeStartedEvent(projectionTestRunID, 2),
		)))
		inputRef := map[string]any{"edgeId": "edge_root_work", "fromNodeId": projectionTestMissionID, "fromPortId": "out", "toPortId": "port_in", "outputSeq": 1}
		attemptRef := map[string]any{"nodeId": projectionTestFormationID, "attempt": 1}
		assertExactJSONValue(t, "started node", findProjectedNode(t, view, projectionTestFormationID), map[string]any{
			"nodeId": projectionTestFormationID, "kind": "formation", "status": "running", "latestAttempt": 1,
			"readiness": map[string]any{"neededInputs": 1, "readyInputs": 1, "totalInputs": 1, "waitingFor": []string{}},
			"attempts":  []any{attemptRef}, "outputs": []any{}, "gates": []any{}, "sessions": []any{},
		})
		assertExactJSONValue(t, "started attempt", findProjectedAttempt(t, view, projectionTestFormationID, 1), map[string]any{
			"nodeId": projectionTestFormationID, "attempt": 1, "status": "running", "startedSeq": 2,
			"inputRefs": []any{inputRef}, "slots": []any{}, "outputs": []any{},
		})
		assertAttemptOrder(t, view, []string{projectionTestFormationID + "/1"})
	})

	t.Run("node terminal output and dispositions", func(t *testing.T) {
		view := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID), schema1NodeStartedEvent(projectionTestRunID, 2), schema1NodeOutputEvent(projectionTestRunID, 3, "done"),
		)))
		inputRef := map[string]any{"edgeId": "edge_root_work", "fromNodeId": projectionTestMissionID, "fromPortId": "out", "toPortId": "port_in", "outputSeq": 1}
		outputRef := map[string]any{"nodeId": projectionTestFormationID, "attempt": 1, "portId": "port_out"}
		assertExactJSONValue(t, "terminal node", findProjectedNode(t, view, projectionTestFormationID), map[string]any{
			"nodeId": projectionTestFormationID, "kind": "formation", "status": "done", "finalDisposition": "done", "latestAttempt": 1,
			"readiness": map[string]any{"neededInputs": 1, "readyInputs": 1, "totalInputs": 1, "waitingFor": []string{}},
			"attempts":  []any{map[string]any{"nodeId": projectionTestFormationID, "attempt": 1}}, "outputs": []any{outputRef}, "gates": []any{}, "sessions": []any{},
		})
		assertExactJSONValue(t, "terminal attempt", findProjectedAttempt(t, view, projectionTestFormationID, 1), map[string]any{
			"nodeId": projectionTestFormationID, "attempt": 1, "status": "done", "startedSeq": 2, "completedSeq": 3,
			"inputRefs": []any{inputRef}, "slots": []any{}, "outputs": []any{outputRef}, "disposition": "done",
		})
		assertExactJSONValue(t, "terminal output", findProjectedOutput(t, view, projectionTestFormationID, 1, "port_out"), map[string]any{
			"nodeId": projectionTestFormationID, "attempt": 1, "portId": "port_out", "outcomeSeq": 3,
			"payloadProjection": map[string]any{"availability": "available", "exact": true, "payload": map[string]any{"kind": "work", "mediaType": "text/plain", "text": "done"}},
		})
		assertOutputOrder(t, view, []string{projectionTestFormationID + "/1/port_out"})
	})

	t.Run("gate open and verdict", func(t *testing.T) {
		open := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID), schema1GateEvaluatingEvent(projectionTestRunID, 2),
		)))
		assertExactJSONValue(t, "open gate", findProjectedGate(t, open, projectionTestGateID), map[string]any{
			"gateId": projectionTestGateID, "attempt": 1, "status": "evaluating", "evaluatingSeq": 2, "evidence": []any{},
		})
		assertGateOrder(t, open, []string{projectionTestGateID + "/1"})

		verdict := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID), schema1GateEvaluatingEvent(projectionTestRunID, 2), schema1GateVerdictEvent(projectionTestRunID, 3, "pass"),
		)))
		assertExactJSONValue(t, "verdict gate", findProjectedGate(t, verdict, projectionTestGateID), map[string]any{
			"gateId": projectionTestGateID, "attempt": 1, "status": "passed", "evaluatingSeq": 2, "verdictSeq": 3,
			"verdict": "pass", "reason": "reviewed", "evidence": []any{},
		})
	})

	t.Run("escalation block resume and stable order", func(t *testing.T) {
		dispatches := []any{
			map[string]any{"dispatchId": "dispatch-b", "nodeId": projectionTestFormationID, "slotId": "slot_reviewer"},
			map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "dispatchSeq": 0},
		}
		secondBlock := schema1BlockedEvent(projectionTestRunID, 6, true, []any{})
		secondBlock["epoch"] = uint64(1)
		secondBlock["data"].(map[string]any)["nextEpoch"] = uint64(2)
		resumedView := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID),
			schema1EscalationEvent(projectionTestRunID, 2, true),
			schema1BlockedEvent(projectionTestRunID, 3, true, dispatches),
			schema1ResumedEvent(projectionTestRunID, 4, 3, dispatches),
		)))
		if resumedView.Status != "running" || resumedView.Final {
			t.Fatalf("resume overlay status/final = %q/%v, want running/false", resumedView.Status, resumedView.Final)
		}
		assertExactJSONValue(t, "resumed retained escalation", resumedView.Escalations, []any{map[string]any{
			"seq": 2, "nodeId": projectionTestFormationID, "severity": "needs-attention", "reason": "operator review", "source": "agent", "trigger": "sentinel", "blocks": true,
		}})
		assertExactJSONValue(t, "resumed retained block", resumedView.Blocks, []any{map[string]any{
			"seq": 3, "epoch": 0, "scope": "node", "nodeId": projectionTestFormationID, "code": "operator_review", "reason": "blocked",
			"resumeAllowed": true, "resumePolicy": "explicit", "nextEpoch": 1, "openDispatches": dispatches,
		}})
		assertExactJSONValue(t, "resumed audit", resumedView.Audit, map[string]any{"eventSchema": 1, "startSeq": 1, "consumedEventCount": 4})
		view := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID),
			schema1EscalationEvent(projectionTestRunID, 2, true),
			schema1BlockedEvent(projectionTestRunID, 3, true, dispatches),
			schema1ResumedEvent(projectionTestRunID, 4, 3, dispatches),
			schema1Event(projectionTestRunID, 5, RunEventEscalationRaised, map[string]any{
				"trigger": "policy", "severity": "info", "reason": "second", "source": "system", "nodeId": "", "gateId": projectionTestGateID, "blocks": false,
			}),
			secondBlock,
		)))
		assertExactJSONValue(t, "first escalation", view.Escalations[0], map[string]any{
			"seq": 2, "nodeId": projectionTestFormationID, "severity": "needs-attention", "reason": "operator review", "source": "agent", "trigger": "sentinel", "blocks": true,
		})
		assertExactJSONValue(t, "second escalation", view.Escalations[1], map[string]any{
			"seq": 5, "gateId": projectionTestGateID, "severity": "info", "reason": "second", "source": "system", "trigger": "policy", "blocks": false,
		})
		assertExactJSONValue(t, "first block", view.Blocks[0], map[string]any{
			"seq": 3, "epoch": 0, "scope": "node", "nodeId": projectionTestFormationID, "code": "operator_review", "reason": "blocked",
			"resumeAllowed": true, "resumePolicy": "explicit", "nextEpoch": 1, "openDispatches": dispatches,
		})
		assertExactJSONValue(t, "second block", view.Blocks[1], map[string]any{
			"seq": 6, "epoch": 1, "scope": "node", "nodeId": projectionTestFormationID, "code": "operator_review", "reason": "blocked",
			"resumeAllowed": true, "resumePolicy": "explicit", "nextEpoch": 2, "openDispatches": []any{},
		})
		assertUint64Order(t, "block source order", blockSequences(view), []uint64{3, 6})
		assertUint64Order(t, "escalation source order", escalationSequences(view), []uint64{2, 5})
		assertExactJSONValue(t, "resume identity", view.Identity, map[string]any{
			"boardId": projectionTestBoardID, "boardSlug": projectionTestBoardSlug, "boardRev": 7,
			"runRoot":   map[string]any{"kind": "mission", "nodeId": projectionTestMissionID},
			"missionId": projectionTestMissionID, "beadId": "ctx-7i1.1", "epoch": 1, "redact": false,
		})
		assertExactJSONValue(t, "resume/block audit", view.Audit, map[string]any{"eventSchema": 1, "startSeq": 1, "consumedEventCount": 6})
	})

	t.Run("cancel and failure apply exact attempt dispositions", func(t *testing.T) {
		for _, test := range []struct {
			eventType   string
			status      string
			disposition string
			data        map[string]any
		}{
			{RunEventCanceled, "canceled", "canceled", map[string]any{"reason": "operator stop", "requestedBy": "human:test", "softInterruptedSlots": []string{}, "final": true}},
			{RunEventFailed, "failed", "failed", map[string]any{"code": "projection_fixture", "reason": "failed", "boundary": "engine", "recoverable": false, "relatedSeq": 2, "final": true}},
		} {
			view := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
				schema1StartedEvent(projectionTestRunID), schema1NodeStartedEvent(projectionTestRunID, 2), schema1Event(projectionTestRunID, 3, test.eventType, test.data),
			)))
			if view.Status != test.status || !view.Final {
				t.Fatalf("%s status/final = %q/%v", test.eventType, view.Status, view.Final)
			}
			node := findProjectedNode(t, view, projectionTestFormationID)
			assertExactJSONValue(t, test.eventType+" node", node, map[string]any{
				"nodeId": projectionTestFormationID, "kind": "formation", "status": test.status, "finalDisposition": test.disposition, "latestAttempt": 1,
				"readiness": map[string]any{"neededInputs": 1, "readyInputs": 1, "totalInputs": 1, "waitingFor": []string{}},
				"attempts":  []any{map[string]any{"nodeId": projectionTestFormationID, "attempt": 1}}, "outputs": []any{}, "gates": []any{}, "sessions": []any{},
			})
			assertExactJSONValue(t, test.eventType+" attempt", findProjectedAttempt(t, view, projectionTestFormationID, 1), map[string]any{
				"nodeId": projectionTestFormationID, "attempt": 1, "status": test.status, "startedSeq": 2, "completedSeq": 3,
				"inputRefs": []any{map[string]any{"edgeId": "edge_root_work", "fromNodeId": projectionTestMissionID, "fromPortId": "out", "toPortId": "port_in", "outputSeq": 1}},
				"slots":     []any{}, "outputs": []any{}, "disposition": test.disposition,
			})
			assertExactJSONValue(t, test.eventType+" audit", view.Audit, map[string]any{"eventSchema": 1, "startSeq": 1, "consumedEventCount": 3})
		}
	})

	t.Run("success preserves completed dispositions and output refs", func(t *testing.T) {
		view := ProjectRunView(mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID), schema1NodeStartedEvent(projectionTestRunID, 2), schema1NodeOutputEvent(projectionTestRunID, 3, "done"),
			schema1Event(projectionTestRunID, 4, RunEventSucceeded, map[string]any{"final": true}),
		)))
		if view.Status != "succeeded" || !view.Final {
			t.Fatalf("success status/final = %q/%v", view.Status, view.Final)
		}
		node := findProjectedNode(t, view, projectionTestFormationID)
		if node.Status != "done" || len(node.Outputs) != 1 {
			t.Fatalf("success changed completed node projection: %+v", node)
		}
		assertExactJSONValue(t, "success audit", view.Audit, map[string]any{"eventSchema": 1, "startSeq": 1, "consumedEventCount": 4})
	})
}

func TestProjectCanonicalRunUsesFrozenGraphOrderForAllStructuralReferences(t *testing.T) {
	input := schema1OrderingProjectionInput(t)
	view := ProjectRunView(mustProjectCanonicalFixture(t, input))

	assertStringOrder(t, "order-distinguishing node order", projectedNodeIDs(view), []string{
		projectionTestMissionID, "fmn_alpha", "fmn_beta", "gate_alpha", "gate_beta",
	})
	assertAttemptOrder(t, view, []string{"fmn_alpha/1", "fmn_beta/1", "gate_alpha/1", "gate_beta/1"})
	assertGateOrder(t, view, []string{"gate_alpha/1", "gate_beta/1"})
	assertOutputOrder(t, view, []string{"fmn_alpha/1/port_z", "fmn_alpha/1/port_a", "fmn_beta/1/port_b"})

	alphaOutputs := []any{
		map[string]any{"nodeId": "fmn_alpha", "attempt": 1, "portId": "port_z"},
		map[string]any{"nodeId": "fmn_alpha", "attempt": 1, "portId": "port_a"},
	}
	betaOutputs := []any{map[string]any{"nodeId": "fmn_beta", "attempt": 1, "portId": "port_b"}}
	assertExactJSONValue(t, "alpha node attempts", findProjectedNode(t, view, "fmn_alpha").Attempts, []any{map[string]any{"nodeId": "fmn_alpha", "attempt": 1}})
	assertExactJSONValue(t, "alpha node outputs", findProjectedNode(t, view, "fmn_alpha").Outputs, alphaOutputs)
	assertExactJSONValue(t, "alpha attempt outputs", findProjectedAttempt(t, view, "fmn_alpha", 1).Outputs, alphaOutputs)
	assertExactJSONValue(t, "beta node attempts", findProjectedNode(t, view, "fmn_beta").Attempts, []any{map[string]any{"nodeId": "fmn_beta", "attempt": 1}})
	assertExactJSONValue(t, "beta node outputs", findProjectedNode(t, view, "fmn_beta").Outputs, betaOutputs)
	assertExactJSONValue(t, "beta attempt outputs", findProjectedAttempt(t, view, "fmn_beta", 1).Outputs, betaOutputs)

	for _, gateID := range []string{"gate_alpha", "gate_beta"} {
		gateRef := map[string]any{"gateId": gateID, "attempt": 1}
		node := findProjectedNode(t, view, gateID)
		attempt := findProjectedAttempt(t, view, gateID, 1)
		assertExactJSONValue(t, gateID+" node attempts", node.Attempts, []any{map[string]any{"nodeId": gateID, "attempt": 1}})
		assertExactJSONValue(t, gateID+" node gate refs", node.Gates, []any{gateRef})
		assertExactJSONValue(t, gateID+" owning attempt gate ref", attempt.Gate, gateRef)
		assertExactJSONValue(t, gateID+" owning attempt outputs", attempt.Outputs, []any{})
		assertExactJSONValue(t, gateID+" owning attempt sessions", attempt.Slots, []any{})
	}

	sessionInput, _ := schema2OpenDispatchLifecycleInput(t, false)
	sessionView := ProjectRunView(mustProjectCanonicalFixture(t, sessionInput))
	assertSchema2SessionProjection(t, sessionView, "none")
}

func TestProjectCanonicalRunSchema2ActivateAndArtifactTransitions(t *testing.T) {
	queued := mustProjectCanonicalFixture(t, schema2ProjectionInput(t, false))
	queuedView := ProjectRunView(queued)
	if queuedView.Status != "queued" || queuedView.Final {
		t.Fatalf("start-only schema-2 status/final = %q/%v, want queued/false", queuedView.Status, queuedView.Final)
	}

	activatedInput := schema2ProjectionInput(t, true)
	activated := mustProjectCanonicalFixture(t, activatedInput)
	if got := ProjectRunView(activated).Status; got != "running" {
		t.Fatalf("activated status = %q, want running", got)
	}

	available := map[string]any{
		"artifactId":   "art_01KXNP6VY3227H78329V52CKF8",
		"availability": "available",
		"name":         "report",
		"artifact": map[string]any{
			"artifactId": "art_01KXNP6VY3227H78329V52CKF8",
			"rootId":     "root_workspace",
			"ref":        "artifacts/report.json",
			"mediaType":  "application/json",
			"sizeBytes":  2,
			"sha256":     projectionSHA256([]byte("{}")),
		},
	}
	availableProjection := mustProjectCanonicalFixture(t, schema2ProjectionInput(t, true,
		schema2Event(projectionTestRunID, 3, "artifact_attached", map[string]any{
			"artifactProjection": available, "source": map[string]any{"kind": "system", "sourceId": "projection-test"},
		}),
	))
	assertExactJSONValue(t, "available artifact projection", ProjectRunView(availableProjection).Artifacts[0], available)

	secondAvailable := map[string]any{
		"artifactId": "art_01KXNP6VY3227H78329V52CKF9", "availability": "available", "name": "trace",
		"artifact": map[string]any{
			"artifactId": "art_01KXNP6VY3227H78329V52CKF9", "rootId": "root_workspace", "ref": "artifacts/trace.txt",
			"mediaType": "text/plain", "sizeBytes": 5, "sha256": projectionSHA256([]byte("trace")),
		},
	}
	extra := []map[string]any{
		schema2Event(projectionTestRunID, 3, "artifact_attached", map[string]any{
			"artifactProjection": available,
			"source":             map[string]any{"kind": "system", "sourceId": "projection-test"},
		}),
		schema2Event(projectionTestRunID, 4, "artifact_attached", map[string]any{
			"artifactProjection": secondAvailable,
			"source":             map[string]any{"kind": "system", "sourceId": "projection-test"},
		}),
		schema2Event(projectionTestRunID, 5, "artifact_observed", map[string]any{
			"artifactId":   "art_01KXNP6VY3227H78329V52CKF8",
			"availability": "redacted",
			"errorCode":    "policy_redacted",
			"observedAt":   "2026-07-20T10:00:04Z",
			"relatedSeq":   3,
		}),
	}
	projection := mustProjectCanonicalFixture(t, schema2ProjectionInput(t, true, extra...))
	view := ProjectRunView(projection)
	if len(view.Artifacts) != 2 {
		t.Fatalf("artifacts = %#v, want two projections in first-registration order", view.Artifacts)
	}
	assertExactJSONValue(t, "revoked artifact projection", view.Artifacts[0], map[string]any{
		"artifactId": "art_01KXNP6VY3227H78329V52CKF8", "availability": "redacted", "name": "report", "errorCode": "policy_redacted",
	})
	assertExactJSONValue(t, "second available artifact projection", view.Artifacts[1], secondAvailable)
	assertStringOrder(t, "artifact first-registration order", projectedArtifactIDs(view), []string{"art_01KXNP6VY3227H78329V52CKF8", "art_01KXNP6VY3227H78329V52CKF9"})
	page := mustProjectEventPage(t, projection, 0, RunPageMaximumLimit)
	firstArtifact := eventDataMember(t, page.Events[2], "artifactProjection")
	if !jsonBytesEqual(firstArtifact, mustMarshalJSON(t, map[string]any{
		"artifactId": "art_01KXNP6VY3227H78329V52CKF8", "availability": "redacted", "name": "report", "errorCode": "policy_redacted",
	})) {
		t.Fatalf("historical event retained superseded artifact state: %s", firstArtifact)
	}
}

func TestProjectCanonicalRunRejectsInvalidSequencesAndFinality(t *testing.T) {
	started := schema1StartedEvent(projectionTestRunID)
	validTerminal := schema1Event(projectionTestRunID, 2, RunEventSucceeded, map[string]any{"final": true})
	tests := []struct {
		name   string
		events []map[string]any
	}{
		{
			name: "shuffled",
			events: []map[string]any{
				started,
				schema1Event(projectionTestRunID, 3, RunEventNodeWaiting, map[string]any{"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"}}),
				schema1NodeStartedEvent(projectionTestRunID, 2),
			},
		},
		{
			name: "duplicate sequence",
			events: []map[string]any{
				started,
				schema1NodeStartedEvent(projectionTestRunID, 2),
				schema1Event(projectionTestRunID, 2, RunEventError, map[string]any{"code": "duplicate", "message": "duplicate", "boundary": "schema", "recoverable": false, "relatedSeq": 1}),
			},
		},
		{
			name: "sequence zero",
			events: []map[string]any{
				withEventSequence(started, uint64(0)),
			},
		},
		{
			name: "sequence above JSON safe integer",
			events: []map[string]any{
				started,
				withEventSequence(schema1NodeStartedEvent(projectionTestRunID, 2), MaxJSONSafeInteger+1),
			},
		},
		{
			name: "run id mismatch",
			events: []map[string]any{
				started,
				schema1Event(projectionTestOtherRunID, 2, RunEventNodeWaiting, map[string]any{"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"}}),
			},
		},
		{
			name: "post-terminal mutation",
			events: []map[string]any{
				started,
				validTerminal,
				schema1NodeStartedEvent(projectionTestRunID, 3),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ProjectCanonicalRun(schema1ProjectionInput(t, test.events...))
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}
}

func TestProjectCanonicalRunValidatesInputRolesAndOwnsBytes(t *testing.T) {
	schema1 := schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID))
	for _, role := range []CanonicalInputRole{
		CanonicalInputRoleSchema1Ledger,
		CanonicalInputRoleSchema1GraphSnapshot,
		CanonicalInputRoleSchema1BindingsSnapshot,
	} {
		t.Run("schema 1 missing "+string(role), func(t *testing.T) {
			input := cloneCanonicalInput(schema1)
			input.Documents = removeCanonicalRole(input.Documents, role)
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
		t.Run("schema 1 duplicate "+string(role), func(t *testing.T) {
			input := cloneCanonicalInput(schema1)
			input.Documents = append(input.Documents, canonicalDocumentByRole(t, input, role))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	t.Run("schema 1 rejects schema 2 role", func(t *testing.T) {
		input := cloneCanonicalInput(schema1)
		input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema2RunBootstrap, []byte(`{}`)))
		_, err := ProjectCanonicalRun(input)
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	schema2 := schema2ProjectionInput(t, true)
	for _, role := range []CanonicalInputRole{
		CanonicalInputRoleSchema2WorkspaceRegistry,
		CanonicalInputRoleSchema2WorkspaceBootstrap,
		CanonicalInputRoleSchema2WorkspaceAuthority,
		CanonicalInputRoleSchema2RunBootstrap,
		CanonicalInputRoleSchema2GraphSnapshot,
		CanonicalInputRoleSchema2PrivateBindings,
		CanonicalInputRoleSchema2Ledger,
	} {
		t.Run("schema 2 missing "+string(role), func(t *testing.T) {
			input := cloneCanonicalInput(schema2)
			input.Documents = removeCanonicalRole(input.Documents, role)
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
		t.Run("schema 2 duplicate "+string(role), func(t *testing.T) {
			input := cloneCanonicalInput(schema2)
			input.Documents = append(input.Documents, canonicalDocumentByRole(t, input, role))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(*CanonicalRunReadInput)
	}{
		{
			name: "missing admission policy chain",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = removeCanonicalRole(input.Documents, CanonicalInputRoleSchema2AdmissionPolicy)
			},
		},
		{
			name: "duplicate admission policy identity",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalDocumentByRole(t, *input, CanonicalInputRoleSchema2AdmissionPolicy))
			},
		},
		{
			name: "missing referenced command",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = removeCanonicalRole(input.Documents, CanonicalInputRoleSchema2CommandRecord)
			},
		},
		{
			name: "duplicate command identity",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalDocumentByRole(t, *input, CanonicalInputRoleSchema2CommandRecord))
			},
		},
		{
			name: "unreferenced extra command",
			mutate: func(input *CanonicalRunReadInput) {
				extra := schema2CommandRecord(t, projectionTestOtherCmdID, "start", "applied", projectionTestRunID)
				input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema2CommandRecord, extra))
			},
		},
		{
			name: "unreferenced private state",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema2RunPrivateState, canonicalJSON(t, map[string]any{
					"recordSchema": 1, "privateStateId": "state_01KXNP6VY3227H78329V52CKF8",
				})))
			},
		},
		{
			name: "schema 2 rejects schema 1 role",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema1Ledger, []byte("{}\n")))
			},
		},
		{
			name: "unknown role",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRole("projection-test-unknown"), []byte("{}")))
			},
		},
		{
			name: "SHA-256 mismatch",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents[0].SHA256 = strings.Repeat("f", 64)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := cloneCanonicalInput(schema2)
			test.mutate(&input)
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	t.Run("projection owns mutable reader bytes", func(t *testing.T) {
		input := cloneCanonicalInput(schema1)
		projection := mustProjectCanonicalFixture(t, input)
		before := mustMarshalJSON(t, ProjectRunView(projection))
		for index := range input.Documents {
			for byteIndex := range input.Documents[index].Bytes {
				input.Documents[index].Bytes[byteIndex] = 'x'
			}
		}
		after := mustMarshalJSON(t, ProjectRunView(projection))
		if !bytes.Equal(after, before) {
			t.Fatalf("projection retained mutable input aliases\nbefore: %s\nafter:  %s", before, after)
		}
	})
}

func TestProjectCanonicalRunRequiresCompleteAdmissionPolicyChain(t *testing.T) {
	valid := schema2InputWithTwoPolicies(t)
	mustProjectCanonicalFixture(t, valid)
	assertSchema2SelectedPolicyLinkage(t, valid, 2)
	assertSchema2SelectedPolicyLinkage(t, schema2ProjectionInput(t, true), 2)
	assertSchema2AuthorityHistory(t, valid)

	for _, test := range []struct {
		name   string
		mutate func(*CanonicalRunReadInput)
	}{
		{name: "missing prior revision", mutate: func(input *CanonicalRunReadInput) {
			for index, document := range input.Documents {
				if document.Role == CanonicalInputRoleSchema2AdmissionPolicy {
					input.Documents = append(input.Documents[:index], input.Documents[index+1:]...)
					return
				}
			}
		}},
		{name: "duplicate selected revision", mutate: func(input *CanonicalRunReadInput) {
			policies := canonicalDocumentsByRole(*input, CanonicalInputRoleSchema2AdmissionPolicy)
			input.Documents = append(input.Documents, policies[len(policies)-1])
		}},
		{name: "unreferenced extra revision", mutate: func(input *CanonicalRunReadInput) {
			prior := canonicalDocumentsByRole(*input, CanonicalInputRoleSchema2AdmissionPolicy)[1]
			extra := canonicalJSON(t, map[string]any{
				"policySchema": 1, "policyRev": 3, "priorPolicySha256": prior.SHA256, "state": "configured",
				"maxActiveRuns": 3, "maxQueuedRuns": 3,
			})
			input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema2AdmissionPolicy, extra))
		}},
		{name: "broken prior hash", mutate: func(input *CanonicalRunReadInput) {
			for index := range input.Documents {
				if input.Documents[index].Role != CanonicalInputRoleSchema2AdmissionPolicy {
					continue
				}
				var policy map[string]any
				if json.Unmarshal(input.Documents[index].Bytes, &policy) == nil && policy["policyRev"] == float64(2) {
					policy["priorPolicySha256"] = strings.Repeat("f", 64)
					input.Documents[index] = canonicalInputDocument(CanonicalInputRoleSchema2AdmissionPolicy, canonicalJSON(t, policy))
					return
				}
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := cloneCanonicalInput(valid)
			test.mutate(&input)
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}
}

func TestProjectRunViewIsDeterministicDefensiveAndHistoryFree(t *testing.T) {
	projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1Event(projectionTestRunID, 2, RunEventNodeWaiting, map[string]any{
			"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"},
		}),
	))

	first := ProjectRunView(projection)
	second := ProjectRunView(projection)
	firstRaw := mustMarshalJSON(t, first)
	secondRaw := mustMarshalJSON(t, second)
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatalf("ProjectRunView is nondeterministic\nfirst:  %s\nsecond: %s", firstRaw, secondRaw)
	}
	if bytes.Contains(firstRaw, []byte(`"events"`)) {
		t.Fatalf("RunView embeds event history: %s", firstRaw)
	}

	if len(first.Nodes) == 0 {
		t.Fatal("fixture projected no nodes")
	}
	first.Nodes[0].Status = "failed"
	if len(first.Nodes[0].Readiness.WaitingFor) > 0 {
		first.Nodes[0].Readiness.WaitingFor[0] = "mutated"
	}
	first.Nodes = append(first.Nodes, first.Nodes[0])
	third := ProjectRunView(projection)
	thirdRaw := mustMarshalJSON(t, third)
	if !bytes.Equal(thirdRaw, secondRaw) {
		t.Fatalf("ProjectRunView returned aliased state\nbefore mutation: %s\nafter mutation:  %s", secondRaw, thirdRaw)
	}
}

func TestProjectRunViewAndEventPageDefensivelyCopyEveryPopulatedFamily(t *testing.T) {
	events := []map[string]any{
		schema1StartedEvent(projectionTestRunID),
		schema1NodeStartedEvent(projectionTestRunID, 2),
		schema1NodeOutputEvent(projectionTestRunID, 3, "done"),
		schema1GateEvaluatingEvent(projectionTestRunID, 4),
		schema1Event(projectionTestRunID, 5, RunEventHumanInputRequested, map[string]any{
			"gateId": projectionTestGateID, "nodeId": projectionTestFormationID, "choices": []string{"pass", "fail"}, "requestedBy": "agent:test",
			"inputRef":    map[string]any{"edgeId": "edge_work_gate", "fromNodeId": projectionTestFormationID, "fromPortId": "port_out", "toPortId": "in", "outputSeq": 3},
			"codeVerdict": "pass", "codeReason": "checks pass", "codePerKind": map[string]any{"code": "pass"}, "timeoutSeconds": 300,
		}),
		schema1EscalationEvent(projectionTestRunID, 6, true),
		schema1BlockedEvent(projectionTestRunID, 7, true, []any{}),
	}
	projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t, events...))

	wantView := mustMarshalJSON(t, ProjectRunView(projection))
	mutatedView := ProjectRunView(projection)
	if mutations := poisonEveryMutableLeaf(reflect.ValueOf(&mutatedView).Elem()); mutations < 12 {
		t.Fatalf("rich RunView fixture exposed only %d mutable leaves; expected nested identity, readiness, attempts, outputs, gates, blocks, escalations, and actions", mutations)
	}
	if got := mustMarshalJSON(t, ProjectRunView(projection)); !bytes.Equal(got, wantView) {
		t.Fatalf("RunView mutation leaked into projection\nwant: %s\ngot:  %s", wantView, got)
	}

	wantPage := mustMarshalJSON(t, mustProjectEventPage(t, projection, 0, RunPageMaximumLimit))
	mutatedPage := mustProjectEventPage(t, projection, 0, RunPageMaximumLimit)
	if mutations := poisonEveryMutableLeaf(reflect.ValueOf(&mutatedPage).Elem()); mutations < 12 {
		t.Fatalf("rich RunEventPage fixture exposed only %d mutable leaves", mutations)
	}
	if got := mustMarshalJSON(t, mustProjectEventPage(t, projection, 0, RunPageMaximumLimit)); !bytes.Equal(got, wantPage) {
		t.Fatalf("RunEventPage mutation leaked into projection\nwant: %s\ngot:  %s", wantPage, got)
	}
}

func TestProjectRunViewAndEventPageDefensiveCopiesAreFamilyComplete(t *testing.T) {
	dispatches := []any{map[string]any{
		"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "dispatchSeq": 2,
	}}
	structuralProjection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1NodeStartedEvent(projectionTestRunID, 2),
		schema1NodeOutputEvent(projectionTestRunID, 3, "done"),
		schema1GateEvaluatingEvent(projectionTestRunID, 4),
		schema1GateVerdictEvent(projectionTestRunID, 5, "pass"),
		schema1EscalationEvent(projectionTestRunID, 6, true),
		schema1BlockedEvent(projectionTestRunID, 7, true, dispatches),
	))
	wantStructuralView := mustMarshalJSON(t, ProjectRunView(structuralProjection))
	mutated := ProjectRunView(structuralProjection)
	if len(mutated.Nodes) == 0 || len(mutated.Attempts) == 0 || len(mutated.Gates) == 0 || len(mutated.Outputs) == 0 || len(mutated.Blocks) == 0 ||
		len(mutated.Blocks[0].OpenDispatches) == 0 || len(mutated.Escalations) == 0 {
		t.Fatalf("structural defensive fixture did not populate every required Task-1 family: %+v", mutated)
	}
	requireMutateStringPath(t, &mutated, "identity", "Identity", "BoardSlug")
	requireMutateStringPath(t, &mutated, "node", "Nodes", 1, "NodeID")
	requireMutateStringPath(t, &mutated, "node attempt ref", "Nodes", 1, "Attempts", 0, "NodeID")
	requireMutateStringPath(t, &mutated, "attempt", "Attempts", 0, "NodeID")
	requireMutateStringPath(t, &mutated, "attempt input ref", "Attempts", 0, "InputRefs", 0, "FromNodeID")
	requireMutateStringPath(t, &mutated, "attempt output ref", "Attempts", 0, "Outputs", 0, "PortID")
	requireMutateStringPath(t, &mutated, "gate", "Gates", 0, "Reason")
	requireMutateStringPath(t, &mutated, "output ref", "Outputs", 0, "PortID")
	requireMutateFamily(t, &mutated, "payload projection", "Outputs", 0, "PayloadProjection")
	requireMutateStringPath(t, &mutated, "block open dispatch", "Blocks", 0, "OpenDispatches", 0, "DispatchID")
	requireMutateStringPath(t, &mutated, "escalation", "Escalations", 0, "Reason")
	if got := mustMarshalJSON(t, ProjectRunView(structuralProjection)); !bytes.Equal(got, wantStructuralView) {
		t.Fatalf("explicit structural-family mutation leaked into RunView\nwant: %s\ngot:  %s", wantStructuralView, got)
	}

	wantPage := mustMarshalJSON(t, mustProjectEventPage(t, structuralProjection, 0, RunPageMaximumLimit))
	mutatedPage := mustProjectEventPage(t, structuralProjection, 0, RunPageMaximumLimit)
	if len(mutatedPage.Events) != 7 {
		t.Fatalf("event defensive fixture events = %d, want 7", len(mutatedPage.Events))
	}
	requireMutateStringPath(t, &mutatedPage, "event envelope", "Events", 1, "Actor")
	requireMutateFamily(t, &mutatedPage, "event nested data", "Events", 1, "Data")
	if got := mustMarshalJSON(t, mustProjectEventPage(t, structuralProjection, 0, RunPageMaximumLimit)); !bytes.Equal(got, wantPage) {
		t.Fatalf("explicit event-family mutation leaked into RunEventPage\nwant: %s\ngot:  %s", wantPage, got)
	}

	artifact := map[string]any{
		"artifactId": "art_01KXNP6VY3227H78329V52CKF8", "availability": "available", "name": "report",
		"artifact": map[string]any{
			"artifactId": "art_01KXNP6VY3227H78329V52CKF8", "rootId": "root_workspace", "ref": "artifacts/report.json",
			"mediaType": "application/json", "sizeBytes": 2, "sha256": projectionSHA256([]byte("{}")),
		},
	}
	artifactProjection := mustProjectCanonicalFixture(t, schema2ProjectionInput(t, true,
		schema2Event(projectionTestRunID, 3, "artifact_attached", map[string]any{
			"artifactProjection": artifact, "source": map[string]any{"kind": "system", "sourceId": "copy-test"},
		}),
	))
	wantArtifactView := mustMarshalJSON(t, ProjectRunView(artifactProjection))
	artifactView := ProjectRunView(artifactProjection)
	if len(artifactView.Artifacts) != 1 {
		t.Fatalf("artifact defensive fixture did not populate artifacts: %+v", artifactView)
	}
	requireMutateStringPath(t, &artifactView, "artifact metadata", "Artifacts", 0, "Name")
	requireMutateStringPath(t, &artifactView, "artifact safe ref", "Artifacts", 0, "Artifact", "Ref")
	if got := mustMarshalJSON(t, ProjectRunView(artifactProjection)); !bytes.Equal(got, wantArtifactView) {
		t.Fatalf("explicit artifact mutation leaked into RunView\nwant: %s\ngot:  %s", wantArtifactView, got)
	}

	sessionInput, _ := schema2OpenDispatchLifecycleInput(t, false)
	sessionProjection := mustProjectCanonicalFixture(t, sessionInput)
	wantSessionView := mustMarshalJSON(t, ProjectRunView(sessionProjection))
	sessionView := ProjectRunView(sessionProjection)
	if len(sessionView.Sessions) != 2 || len(sessionView.Blocks) != 1 || len(sessionView.Blocks[0].OpenDispatches) != 2 {
		t.Fatalf("session/open-dispatch defensive fixture incomplete: %+v", sessionView)
	}
	requireMutateStringPath(t, &sessionView, "session target", "Sessions", 0, "SessionTargetID")
	requireMutateStringPath(t, &sessionView, "session baseline", "Sessions", 0, "Baseline", "SHA256")
	requireMutateStringPath(t, &sessionView, "session capability", "Sessions", 0, "PeekCapability", "Generation")
	requireMutateStringPath(t, &sessionView, "node session ref", "Nodes", 0, "Sessions", 0, "BindingID")
	requireMutateStringPath(t, &sessionView, "attempt slot ref", "Attempts", 0, "Slots", 0, "BindingID")
	requireMutateStringPath(t, &sessionView, "schema-2 open dispatch", "Blocks", 0, "OpenDispatches", 0, "TargetLeaseID")
	if got := mustMarshalJSON(t, ProjectRunView(sessionProjection)); !bytes.Equal(got, wantSessionView) {
		t.Fatalf("explicit session/open-dispatch mutation leaked into RunView\nwant: %s\ngot:  %s", wantSessionView, got)
	}
}

func TestProjectRunViewGenerationTracksImmutableIncarnation(t *testing.T) {
	baseInput := schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID))
	baseProjection := mustProjectCanonicalFixture(t, baseInput)
	base := ProjectRunView(baseProjection)
	if !lowercaseSHA256Pattern.MatchString(base.Generation) {
		t.Fatalf("generation = %q, want exact lowercase 64-hex SHA-256", base.Generation)
	}

	appended := cloneCanonicalInput(baseInput)
	appended = replaceCanonicalDocument(t, appended, CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t,
		schema1StartedEvent(projectionTestRunID),
		schema1Event(projectionTestRunID, 2, RunEventError, map[string]any{
			"code": "display_only", "message": "tail", "reason": "tail", "boundary": "schema", "nodeId": "", "gateId": "", "slotId": "", "dispatchId": "", "recoverable": true, "relatedSeq": 1,
		}),
	))
	if got := ProjectRunView(mustProjectCanonicalFixture(t, appended)).Generation; got != base.Generation {
		t.Fatalf("generation changed across an appended ledger tail: got %q want %q", got, base.Generation)
	}

	firstChanged := cloneCanonicalInput(baseInput)
	changedStart := schema1StartedEvent(projectionTestRunID)
	changedStart["actor"] = "agent:other"
	firstChanged = replaceCanonicalDocument(t, firstChanged, CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, changedStart))
	snapshotChanged := replaceCanonicalDocument(t, cloneCanonicalInput(baseInput), CanonicalInputRoleSchema1GraphSnapshot, append([]byte(schema1ProjectionSnapshot), '\n'))
	bindingsChanged := replaceCanonicalDocument(t, cloneCanonicalInput(baseInput), CanonicalInputRoleSchema1BindingsSnapshot, append([]byte(schema1ProjectionBindings), '\n'))
	for _, test := range []struct {
		name  string
		input CanonicalRunReadInput
	}{
		{name: "first event", input: firstChanged},
		{name: "snapshot", input: snapshotChanged},
		{name: "bindings", input: bindingsChanged},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := ProjectRunView(mustProjectCanonicalFixture(t, test.input)).Generation
			if got == base.Generation || !lowercaseSHA256Pattern.MatchString(got) {
				t.Fatalf("generation after %s change = %q, base %q", test.name, got, base.Generation)
			}
		})
	}

	schema2Base := schema2ProjectionInput(t, true)
	schema2Generation := ProjectRunView(mustProjectCanonicalFixture(t, schema2Base)).Generation
	if !lowercaseSHA256Pattern.MatchString(schema2Generation) {
		t.Fatalf("schema-2 generation = %q, want lowercase SHA-256", schema2Generation)
	}
	for _, test := range []struct {
		name   string
		change schema2IdentityChange
	}{
		{name: "run authority", change: schema2IdentityChange{runAuthorityID: "auth_01KXNP6VY3227H78329V52CKF9"}},
		{name: "graph snapshot", change: schema2IdentityChange{changeGraph: true}},
		{name: "private bindings", change: schema2IdentityChange{changeBindings: true}},
		{name: "admission command", change: schema2IdentityChange{commandID: projectionTestOtherCmdID}},
	} {
		t.Run("schema 2 tuple changes with "+test.name, func(t *testing.T) {
			projection := mustProjectCanonicalFixture(t, schema2InputWithIdentityChange(t, schema2Base, test.change))
			if got := ProjectRunView(projection).Generation; got == schema2Generation || !lowercaseSHA256Pattern.MatchString(got) {
				t.Fatalf("schema-2 generation after coherent %s change = %q, base %q", test.name, got, schema2Generation)
			}
		})
	}

	tail := cloneCanonicalInput(schema2Base)
	events := canonicalLedgerEvents(t, tail)
	tailError := schema2Event(projectionTestRunID, uint64(len(events)+1), "error", map[string]any{
		"code": "display_only", "message": "tail", "boundary": "schema", "errorScope": "run",
		"recoverable": true, "relatedSeq": 1,
	})
	delete(tailError, "attempt")
	events = append(events, tailError)
	tail = replaceCanonicalDocument(t, tail, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
	if got := ProjectRunView(mustProjectCanonicalFixture(t, tail)).Generation; got != schema2Generation {
		t.Fatalf("schema-2 generation changed across a coherent ordinary ledger tail: got %q want %q", got, schema2Generation)
	}
}

func TestProjectRunEventPageCountsProjectionOnlySlot(t *testing.T) {
	projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1NodeStartedEvent(projectionTestRunID, 2),
	))
	projection.events[1] = projectedEvent{scanSeq: 2, omitted: true}
	projection.events = append(projection.events, projectedEvent{
		scanSeq: 3,
		safe:    cloneSafeEventWithSequence(t, mustSafeProjectedEvent(t, projection, 0), 3),
	})
	projection.latestSeq = 3
	projection.view.Cursor = 3

	before := projectionFingerprint(t, projection)
	page, err := ProjectRunEventPage(projection, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Schema != RunEventPageSchema {
		t.Fatalf("schema = %q, want %q", page.Schema, RunEventPageSchema)
	}
	if page.Generation != ProjectRunView(projection).Generation {
		t.Fatalf("generation = %q, want projection generation", page.Generation)
	}
	if page.Cursor != 2 {
		t.Fatalf("cursor = %d, want 2", page.Cursor)
	}
	if !page.HasMore {
		t.Fatal("hasMore = false, want true")
	}
	if len(page.Events) != 0 {
		t.Fatalf("events = %v, want none", page.Events)
	}
	if after := projectionFingerprint(t, projection); after != before {
		t.Fatalf("event selector mutated projection\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestProjectRunEventPageIdentityCursorAndOrdering(t *testing.T) {
	var selector func(CanonicalRunProjection, uint64, int) (RunEventPage, error) = ProjectRunEventPage
	_ = selector // The fixed selector signature has no adapter-supplied identity fields.

	projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1NodeStartedEvent(projectionTestRunID, 2),
		schema1NodeOutputEvent(projectionTestRunID, 3, "done"),
	))
	view := ProjectRunView(projection)
	page := mustProjectEventPage(t, projection, 0, RunPageMaximumLimit)
	pageRaw := mustMarshalJSON(t, page)
	var decoded struct {
		Schema     string          `json:"schema"`
		RunID      string          `json:"runId"`
		Generation string          `json:"generation"`
		Source     json.RawMessage `json:"source"`
		Cursor     uint64          `json:"cursor"`
		HasMore    bool            `json:"hasMore"`
		Events     []struct {
			Seq uint64 `json:"seq"`
		} `json:"events"`
	}
	if err := json.Unmarshal(pageRaw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != RunEventPageSchema || decoded.RunID != view.RunID || decoded.Generation != view.Generation {
		t.Fatalf("page identity = %+v, want schema/run/generation from view %+v", decoded, view)
	}
	if !lowercaseSHA256Pattern.MatchString(decoded.Generation) {
		t.Fatalf("generation = %q, want lowercase SHA-256", decoded.Generation)
	}
	if !bytes.Equal(decoded.Source, mustMarshalJSON(t, view.Source)) {
		t.Fatalf("page source = %s, want exact view source %s", decoded.Source, mustMarshalJSON(t, view.Source))
	}
	for index := 1; index < len(decoded.Events); index++ {
		if decoded.Events[index-1].Seq >= decoded.Events[index].Seq {
			t.Fatalf("events not strictly ascending: %+v", decoded.Events)
		}
	}

	for _, since := range []uint64{4, 41, MaxJSONSafeInteger} {
		empty := mustProjectEventPage(t, projection, since, 1)
		if empty.Schema != RunEventPageSchema || empty.RunID != view.RunID || empty.Generation != view.Generation {
			t.Fatalf("empty page lost identity at since %d: %+v", since, empty)
		}
		if !reflect.DeepEqual(empty.Source, view.Source) || empty.Cursor != since || empty.HasMore || len(empty.Events) != 0 {
			t.Fatalf("empty page at since %d = %+v, want echoed cursor, no events, hasMore false", since, empty)
		}
	}
}

func TestProjectRunEventPageLimitAndExactByteBoundary(t *testing.T) {
	base := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1Event(projectionTestRunID, 2, RunEventError, map[string]any{
			"code": "page_fixture", "message": "x", "reason": "x", "boundary": "schema", "nodeId": "", "gateId": "", "slotId": "", "dispatchId": "", "recoverable": true, "relatedSeq": 1,
		}),
	))

	for _, invalidLimit := range []int{0, RunPageMaximumLimit + 1} {
		_, err := ProjectRunEventPage(base, 0, invalidLimit)
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	}
	if _, err := ProjectRunEventPage(base, MaxJSONSafeInteger+1, 1); err == nil {
		t.Fatal("since above MaxJSONSafeInteger succeeded")
	} else {
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	}

	many := projectionWithRepeatedSafeEvents(t, base, 201)
	one := mustProjectEventPage(t, many, 0, 1)
	if one.Cursor != 1 || len(one.Events) != 1 || !one.HasMore {
		t.Fatalf("limit 1 page = %+v", one)
	}
	twoHundred := mustProjectEventPage(t, many, 0, RunPageMaximumLimit)
	if twoHundred.Cursor != RunPageMaximumLimit || len(twoHundred.Events) != RunPageMaximumLimit || !twoHundred.HasMore {
		t.Fatalf("limit 200 page cursor/events/hasMore = %d/%d/%v", twoHundred.Cursor, len(twoHundred.Events), twoHundred.HasMore)
	}

	for _, target := range []int{RunPageMaximumBytes - 1, RunPageMaximumBytes} {
		projection := projectionWithCompletePageSize(t, base, target)
		page := mustProjectEventPage(t, projection, 1, 1)
		if got := len(mustMarshalJSON(t, page)); got != target {
			t.Fatalf("encoded page bytes = %d, want %d", got, target)
		}
	}

	oversized := projectionWithCompletePageSize(t, base, RunPageMaximumBytes+1)
	_, err := ProjectRunEventPage(oversized, 1, 1)
	requireProjectionError(t, err, ErrRunProjectionResourceLimit)

	firstSafe := cloneSafeEventWithSequence(t, mustSafeProjectedEvent(t, base, 1), 2)
	secondProjection := projectionWithCompletePageSize(t, base, RunPageMaximumBytes)
	secondSafe := cloneSafeEventWithSequence(t, mustSafeProjectedEvent(t, secondProjection, 1), 3)
	bounded := base
	bounded.events = []projectedEvent{
		{scanSeq: 1, safe: cloneSafeEventWithSequence(t, firstSafe, 1)},
		{scanSeq: 2, safe: firstSafe},
		{scanSeq: 3, safe: secondSafe},
	}
	bounded.latestSeq = 3
	bounded.view.Cursor = 3
	page := mustProjectEventPage(t, bounded, 1, RunPageMaximumLimit)
	if page.Cursor != 2 || len(page.Events) != 1 || !page.HasMore {
		t.Fatalf("byte-capped nonempty page = cursor %d events %d hasMore %v, want 2/1/true", page.Cursor, len(page.Events), page.HasMore)
	}
}

type countingCanonicalRunReader struct {
	input        CanonicalRunReadInput
	readRunCalls int
	readIDs      []string
}

func (r *countingCanonicalRunReader) ReadRun(runID string) (CanonicalRunReadInput, error) {
	r.readRunCalls++
	r.readIDs = append(r.readIDs, runID)
	return cloneCanonicalInput(r.input), nil
}

func (r *countingCanonicalRunReader) ListRunIdentities(RunIdentityPageRequest) (RunIdentityPage, error) {
	panic("ListRunIdentities must not be called by ReadCanonicalRun")
}

func (r *countingCanonicalRunReader) ReadCommand(SubmittedCommandIdentity) (CanonicalCommandReadInput, error) {
	panic("ReadCommand must not be called by ReadCanonicalRun")
}

func TestStoreReadCanonicalRunReadsOnceAndSelectorsDoNotRead(t *testing.T) {
	reader := &countingCanonicalRunReader{
		input: schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID),
			schema1NodeStartedEvent(projectionTestRunID, 2),
		),
	}
	store := NewStore(t.TempDir())
	store.canonicalRunAuthorityReader = reader

	projection, err := store.ReadCanonicalRun(projectionTestRunID)
	if err != nil {
		t.Fatal(err)
	}
	if reader.readRunCalls != 1 || !reflect.DeepEqual(reader.readIDs, []string{projectionTestRunID}) {
		t.Fatalf("ReadRun calls/ids = %d/%v, want one exact read", reader.readRunCalls, reader.readIDs)
	}
	_ = ProjectRunView(projection)
	if _, err := ProjectRunEventPage(projection, 0, RunPageMaximumLimit); err != nil {
		t.Fatal(err)
	}
	if reader.readRunCalls != 1 {
		t.Fatalf("pure selectors caused %d reader calls, want one total", reader.readRunCalls)
	}
}

func TestProjectCommandReceiptPreservesTerminalUnion(t *testing.T) {
	for _, commandKind := range []string{"start", "resume", "cancel", "verdict"} {
		for _, state := range []string{"applied", "rejected"} {
			t.Run(commandKind+"/"+state, func(t *testing.T) {
				history := canonicalCommandHistory(t, projectionTestCommandID, commandKind, state, projectionTestRunID)
				input := canonicalCommandInput(t, projectionTestCommandID, commandKind, state)
				if !bytes.Equal(input.Record, history.terminal) {
					t.Fatalf("canonical command input did not select exact terminal generation\nwant: %s\ngot:  %s", history.terminal, input.Record)
				}
				assertAuthenticatedCommandHistory(t, history)
				receipt, err := ProjectCommandReceipt(input)
				if err != nil {
					t.Fatal(err)
				}
				var got, record map[string]any
				if err := json.Unmarshal(mustMarshalJSON(t, receipt), &got); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(input.Record, &record); err != nil {
					t.Fatal(err)
				}
				want := map[string]any{
					"commandId": record["commandId"], "commandPayloadSha256": record["commandPayloadSha256"],
					"commandKind": record["commandKind"], "outcomeWriterFence": "9", "state": state,
					"decisionAdmissionPolicyRef": record["decisionAdmissionPolicyRef"],
				}
				if state == "applied" {
					want["runId"] = projectionTestRunID
					want["effectSeq"] = float64(7)
				} else {
					want["rejectionCode"] = "fixture_rejected"
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("receipt = %#v, want exact closed %s arm %#v", got, state, want)
				}
			})
		}
	}

	t.Run("recovery repair preserves origin fence", func(t *testing.T) {
		history := schema2CommandHistoryForPayload(t, projectionTestCommandID, "cancel", "applied", canonicalCommandPayload("cancel", projectionTestRunID), schema2CommandOutcome{
			runID: projectionTestRunID, effectSeq: 7, admittedWriterFence: 1, stateWriterFence: 2, outcomeWriterFence: 1,
		})
		assertAuthenticatedCommandHistory(t, history)
		var terminal map[string]any
		if err := json.Unmarshal(history.terminal, &terminal); err != nil {
			t.Fatal(err)
		}
		input := CanonicalCommandReadInput{
			Source: CanonicalRunSourceSchema2,
			Submitted: SubmittedCommandIdentity{
				CommandID: projectionTestCommandID, CommandKind: "cancel", CommandPayloadSHA256: terminal["commandPayloadSha256"].(string),
			},
			Record: history.terminal,
		}
		receipt, err := ProjectCommandReceipt(input)
		if err != nil {
			t.Fatal(err)
		}
		assertExactJSONValue(t, "recovery-repaired receipt", receipt, map[string]any{
			"commandId": projectionTestCommandID, "commandPayloadSha256": terminal["commandPayloadSha256"], "commandKind": "cancel",
			"state": "applied", "runId": projectionTestRunID, "effectSeq": 7, "outcomeWriterFence": "1", "decisionAdmissionPolicyRef": nil,
		})
	})
}

func TestProjectCommandReceiptRejectsPendingMismatchAndSubstitution(t *testing.T) {
	valid := canonicalCommandInput(t, projectionTestCommandID, "start", "applied")
	tests := []struct {
		name   string
		mutate func(*CanonicalCommandReadInput)
	}{
		{name: "wrong submitted id", mutate: func(input *CanonicalCommandReadInput) { input.Submitted.CommandID = projectionTestOtherCmdID }},
		{name: "right id wrong submitted payload hash", mutate: func(input *CanonicalCommandReadInput) { input.Submitted.CommandPayloadSHA256 = strings.Repeat("f", 64) }},
		{name: "cross kind", mutate: func(input *CanonicalCommandReadInput) { input.Submitted.CommandKind = "resume" }},
		{name: "pending", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) {
				record["state"] = "pending"
				delete(record, "runId")
				delete(record, "effectSeq")
				delete(record, "outcomeWriterFence")
				delete(record, "decisionAdmissionPolicyRef")
			})
		}},
		{name: "stale writer state", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["stateWriterFence"] = "1"; record["admittedWriterFence"] = "2" })
		}},
		{name: "substituted record id", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["commandId"] = projectionTestOtherCmdID })
		}},
		{name: "substituted embedded payload", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["commandPayload"] = map[string]any{"kind": "start"} })
		}},
		{name: "substituted record kind", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["commandKind"] = "resume" })
		}},
		{name: "substituted record payload hash", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["commandPayloadSha256"] = strings.Repeat("e", 64) })
		}},
		{name: "terminal missing outcome fence", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { delete(record, "outcomeWriterFence") })
		}},
		{name: "unknown record member", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["projectionUnknown"] = true })
		}},
		{name: "applied missing run id", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { delete(record, "runId") })
		}},
		{name: "applied missing effect sequence", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { delete(record, "effectSeq") })
		}},
		{name: "applied contains rejection field", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["rejectionCode"] = "forbidden" })
		}},
		{name: "start nil decision policy", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["decisionAdmissionPolicyRef"] = nil })
		}},
		{name: "start missing decision policy member", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { delete(record, "decisionAdmissionPolicyRef") })
		}},
	}

	t.Run("duplicate JSON member", func(t *testing.T) {
		input := cloneCanonicalCommandInput(valid)
		input.Record = bytes.Replace(input.Record, []byte(`"commandId":"`+projectionTestCommandID+`"`), []byte(`"commandId":"`+projectionTestCommandID+`","commandId":"`+projectionTestCommandID+`"`), 1)
		if receipt, err := ProjectCommandReceipt(input); err == nil {
			t.Fatalf("duplicate member returned receipt: %#v", receipt)
		} else {
			requireProjectionError(t, err, ErrRunCommandNotTerminal)
		}
	})

	t.Run("pending carrying terminal members", func(t *testing.T) {
		input := cloneCanonicalCommandInput(valid)
		input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["state"] = "pending" })
		if receipt, err := ProjectCommandReceipt(input); err == nil {
			t.Fatalf("pending record carrying terminal members returned receipt: %#v", receipt)
		} else {
			requireProjectionError(t, err, ErrRunCommandNotTerminal)
		}
	})

	t.Run("non-start decision policy must be null", func(t *testing.T) {
		input := canonicalCommandInput(t, projectionTestCommandID, "resume", "applied")
		input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) {
			record["decisionAdmissionPolicyRef"] = map[string]any{"policyRev": 1, "policySha256": strings.Repeat("a", 64)}
		})
		if receipt, err := ProjectCommandReceipt(input); err == nil {
			t.Fatalf("non-start policy ref returned receipt: %#v", receipt)
		} else {
			requireProjectionError(t, err, ErrRunCommandNotTerminal)
		}
	})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := cloneCanonicalCommandInput(valid)
			test.mutate(&input)
			if receipt, err := ProjectCommandReceipt(input); err == nil {
				t.Fatalf("mismatch returned receipt: %#v", receipt)
			} else {
				requireProjectionError(t, err, ErrRunCommandNotTerminal)
			}
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "rejected missing code", mutate: func(record map[string]any) { delete(record, "rejectionCode") }},
		{name: "rejected contains run id", mutate: func(record map[string]any) { record["runId"] = projectionTestRunID }},
		{name: "rejected contains effect sequence", mutate: func(record map[string]any) { record["effectSeq"] = 7 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := canonicalCommandInput(t, projectionTestCommandID, "start", "rejected")
			input.Record = mutateCommandRecord(t, input.Record, test.mutate)
			if receipt, err := ProjectCommandReceipt(input); err == nil {
				t.Fatalf("invalid rejected arm returned receipt: %#v", receipt)
			} else {
				requireProjectionError(t, err, ErrRunCommandNotTerminal)
			}
		})
	}
}

func TestProjectCommandReceiptRejectsClosedRecordMatrixForEveryKindAndState(t *testing.T) {
	commonRequired := []string{
		"commandSchema", "recordRev", "priorGeneration", "commandEncoding", "commandId", "commandKind",
		"commandPayload", "commandPayloadSha256", "admittedWriterFence", "stateWriterFence", "state",
		"outcomeWriterFence", "decisionAdmissionPolicyRef",
	}
	for _, kind := range []string{"start", "resume", "cancel", "verdict"} {
		for _, state := range []string{"applied", "rejected"} {
			t.Run(kind+"/"+state, func(t *testing.T) {
				valid := canonicalCommandInput(t, projectionTestCommandID, kind, state)
				for _, member := range commonRequired {
					t.Run("missing "+member, func(t *testing.T) {
						input := cloneCanonicalCommandInput(valid)
						input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { delete(record, member) })
						requireCommandReceiptMatrixError(t, input)
					})
				}

				for _, test := range []struct {
					name   string
					mutate func(map[string]any)
				}{
					{name: "unknown top-level member", mutate: func(record map[string]any) { record["projectionUnknown"] = true }},
					{name: "outcome fence exceeds publishing fence", mutate: func(record map[string]any) { record["outcomeWriterFence"] = "10" }},
					{name: "state fence precedes admission", mutate: func(record map[string]any) { record["stateWriterFence"] = "0" }},
					{name: "terminal has no pending predecessor", mutate: func(record map[string]any) {
						record["recordRev"] = 1
						record["priorGeneration"] = nil
					}},
					{name: "pending predecessor hash substitution", mutate: func(record map[string]any) {
						record["priorGeneration"].(map[string]any)["sha256"] = strings.Repeat("f", 64)
					}},
					{name: "embedded payload substitution", mutate: func(record map[string]any) { record["commandPayload"].(map[string]any)["actor"] = "human:other" }},
					{name: "record payload hash substitution", mutate: func(record map[string]any) { record["commandPayloadSha256"] = strings.Repeat("e", 64) }},
				} {
					t.Run(test.name, func(t *testing.T) {
						input := cloneCanonicalCommandInput(valid)
						input.Record = mutateCommandRecord(t, input.Record, test.mutate)
						requireCommandReceiptMatrixError(t, input)
					})
				}

				t.Run("duplicate member", func(t *testing.T) {
					input := cloneCanonicalCommandInput(valid)
					needle := []byte(`"commandId":"` + projectionTestCommandID + `"`)
					input.Record = bytes.Replace(input.Record, needle, append(append([]byte{}, needle...), append([]byte(","), needle...)...), 1)
					requireCommandReceiptMatrixError(t, input)
				})

				if state == "applied" {
					for _, member := range []string{"runId", "effectSeq"} {
						t.Run("applied missing "+member, func(t *testing.T) {
							input := cloneCanonicalCommandInput(valid)
							input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { delete(record, member) })
							requireCommandReceiptMatrixError(t, input)
						})
					}
					t.Run("applied forbids rejectionCode", func(t *testing.T) {
						input := cloneCanonicalCommandInput(valid)
						input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["rejectionCode"] = "forbidden" })
						requireCommandReceiptMatrixError(t, input)
					})
				} else {
					t.Run("rejected missing rejectionCode", func(t *testing.T) {
						input := cloneCanonicalCommandInput(valid)
						input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { delete(record, "rejectionCode") })
						requireCommandReceiptMatrixError(t, input)
					})
					for _, member := range []string{"runId", "effectSeq"} {
						t.Run("rejected forbids "+member, func(t *testing.T) {
							input := cloneCanonicalCommandInput(valid)
							input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) {
								if member == "runId" {
									record[member] = projectionTestRunID
								} else {
									record[member] = 7
								}
							})
							requireCommandReceiptMatrixError(t, input)
						})
					}
				}

				if kind == "start" {
					for _, test := range []struct {
						name   string
						mutate func(map[string]any)
					}{
						{name: "null", mutate: func(record map[string]any) { record["decisionAdmissionPolicyRef"] = nil }},
						{name: "missing revision", mutate: func(record map[string]any) {
							delete(record["decisionAdmissionPolicyRef"].(map[string]any), "policyRev")
						}},
						{name: "missing hash", mutate: func(record map[string]any) {
							delete(record["decisionAdmissionPolicyRef"].(map[string]any), "policySha256")
						}},
						{name: "zero revision", mutate: func(record map[string]any) { record["decisionAdmissionPolicyRef"].(map[string]any)["policyRev"] = 0 }},
						{name: "invalid hash", mutate: func(record map[string]any) {
							record["decisionAdmissionPolicyRef"].(map[string]any)["policySha256"] = "not-a-hash"
						}},
						{name: "unknown nested member", mutate: func(record map[string]any) {
							record["decisionAdmissionPolicyRef"].(map[string]any)["projectionUnknown"] = true
						}},
					} {
						t.Run("start policy "+test.name, func(t *testing.T) {
							input := cloneCanonicalCommandInput(valid)
							input.Record = mutateCommandRecord(t, input.Record, test.mutate)
							requireCommandReceiptMatrixError(t, input)
						})
					}
				} else {
					t.Run("non-start policy must be null", func(t *testing.T) {
						input := cloneCanonicalCommandInput(valid)
						input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) {
							record["decisionAdmissionPolicyRef"] = map[string]any{"policyRev": 1, "policySha256": strings.Repeat("a", 64)}
						})
						requireCommandReceiptMatrixError(t, input)
					})
				}
			})
		}
	}
}

func TestCanonicalRunSourceSelectionIsClosedAndNonAuthorizing(t *testing.T) {
	schema1 := mustProjectCanonicalFixture(t, schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID)))
	schema1View := ProjectRunView(schema1)
	var schema1Source map[string]any
	if err := json.Unmarshal(mustMarshalJSON(t, schema1View.Source), &schema1Source); err != nil {
		t.Fatal(err)
	}
	if schema1Source["eventSchema"] != float64(1) || schema1Source["compatibility"] != true {
		t.Fatalf("schema-1 source = %#v", schema1Source)
	}
	if _, exists := schema1Source["authoritySchema"]; exists {
		t.Fatalf("schema-1 source invented authority schema: %#v", schema1Source)
	}

	schema2Input := schema2ProjectionInput(t, true)
	schema2 := mustProjectCanonicalFixture(t, schema2Input)
	schema2View := ProjectRunView(schema2)
	var schema2Source map[string]any
	if err := json.Unmarshal(mustMarshalJSON(t, schema2View.Source), &schema2Source); err != nil {
		t.Fatal(err)
	}
	if schema2Source["eventSchema"] != float64(2) || schema2Source["authoritySchema"] != float64(2) || schema2Source["compatibility"] != false {
		t.Fatalf("schema-2 source = %#v", schema2Source)
	}

	claimedInvalid := cloneCanonicalInput(schema2Input)
	claimedInvalid.Documents = removeCanonicalRole(claimedInvalid.Documents, CanonicalInputRoleSchema2RunBootstrap)
	for _, document := range schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID)).Documents {
		claimedInvalid.Documents = append(claimedInvalid.Documents, document)
	}
	if projection, err := ProjectCanonicalRun(claimedInvalid); err == nil {
		t.Fatalf("invalid claimed schema 2 fell back to schema 1: %+v", ProjectRunView(projection))
	} else {
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	}

	capability := disabledRuntimeAuthorityCapability()
	if capability.SemanticProjection {
		t.Fatalf("projector fixtures self-authorized semantic projection: %+v", capability)
	}
}

func TestProjectCanonicalRunRejectsUnknownEventTypeAndCompatibilityArmsDoNotAuthorize(t *testing.T) {
	unknown := schema1Event(projectionTestRunID, 2, "projection_unknown", map[string]any{})
	_, err := ProjectCanonicalRun(schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID), unknown))
	requireProjectionError(t, err, ErrRunEventUnknown)

	for _, eventType := range []string{RunEventOrchestrationTeam, RunEventPeerPlane, RunEventAdapterSend, RunEventVerificationVerdict} {
		t.Run(eventType, func(t *testing.T) {
			var selected schema1SafeRegistryCase
			for _, candidate := range schema1SafeRegistryCases() {
				if candidate.eventType == eventType {
					selected = candidate
					break
				}
			}
			input, targetSeq := schema1RegistryFixture(t, selected)
			events := canonicalLedgerEvents(t, input)
			baselineInput := replaceCanonicalDocument(t, cloneCanonicalInput(input), CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, events[:len(events)-1]...))
			baseline := ProjectRunView(mustProjectCanonicalFixture(t, baselineInput))
			projection := mustProjectCanonicalFixture(t, input)
			got := ProjectRunView(projection)
			got.Cursor = baseline.Cursor
			got.Audit.ConsumedEventCount = baseline.Audit.ConsumedEventCount
			if !bytes.Equal(mustMarshalJSON(t, got), mustMarshalJSON(t, baseline)) {
				t.Fatalf("compatibility-only %s changed structural or authorizing state\nbefore: %s\nafter:  %s", eventType, mustMarshalJSON(t, baseline), mustMarshalJSON(t, got))
			}
			page := mustProjectEventPage(t, projection, targetSeq-1, 1)
			if len(page.Events) != 1 || !bytes.Contains(mustMarshalJSON(t, page.Events[0]), []byte(`"type":"`+eventType+`"`)) {
				t.Fatalf("compatibility-only %s was not retained as readable audit evidence: %s", eventType, mustMarshalJSON(t, page))
			}
		})
	}
}

type schema1SafeRegistryCase struct {
	eventType   string
	rawData     map[string]any
	publicData  map[string]any
	privateKeys []string
}

func TestProjectCanonicalRunSchema1SafeEventRegistry(t *testing.T) {
	cases := schema1SafeRegistryCases()
	if len(cases) != 21 {
		t.Fatalf("schema-1 registry cases = %d, want all 21 constants", len(cases))
	}
	registered := map[string]bool{}
	for _, eventType := range []string{
		RunEventStarted, RunEventResumed, RunEventNodeWaiting, RunEventNodeStarted,
		RunEventOrchestrationTeam, RunEventPeerPlane, RunEventSlotDispatch,
		RunEventAdapterSend, RunEventSlotResult, RunEventNodeOutput,
		RunEventGateEvaluating, RunEventGateVerdict, RunEventVerificationVerdict,
		RunEventEscalationRaised, RunEventHumanInputRequested,
		RunEventHumanVerdictRecorded, RunEventError, RunEventBlocked,
		RunEventCanceled, RunEventFailed, RunEventSucceeded,
	} {
		registered[eventType] = true
	}

	for _, test := range cases {
		t.Run(test.eventType, func(t *testing.T) {
			delete(registered, test.eventType)
			input, targetSeq := schema1RegistryFixture(t, test)
			projection := mustProjectCanonicalFixture(t, input)
			page := mustProjectEventPage(t, projection, targetSeq-1, 1)
			if len(page.Events) != 1 {
				t.Fatalf("sanitized target event count = %d, want 1", len(page.Events))
			}
			var event struct {
				Type string                     `json:"type"`
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(mustMarshalJSON(t, page.Events[0]), &event); err != nil {
				t.Fatal(err)
			}
			if event.Type != test.eventType {
				t.Fatalf("event type = %q, want %q", event.Type, test.eventType)
			}
			gotData := mustMarshalJSON(t, event.Data)
			wantData := mustMarshalJSON(t, test.publicData)
			if !jsonBytesEqual(gotData, wantData) {
				t.Fatalf("safe data = %s, want complete nested projection %s; event=%s", gotData, wantData, mustMarshalJSON(t, page.Events[0]))
			}
			allKeys := recursiveJSONKeys(t, mustMarshalJSON(t, page.Events[0]))
			for _, key := range test.privateKeys {
				if allKeys[key] {
					t.Fatalf("private key %q survived sanitizer: %s", key, mustMarshalJSON(t, page.Events[0]))
				}
			}

			for _, mutation := range []struct {
				name   string
				mutate func([]map[string]any)
			}{
				{
					name: "unknown envelope key",
					mutate: func(events []map[string]any) {
						events[len(events)-1]["privateRoute"] = "/secret"
					},
				},
				{
					name: "unknown data key",
					mutate: func(events []map[string]any) {
						events[len(events)-1]["data"].(map[string]any)["projectionUnknown"] = true
					},
				},
				{
					name: "wrong safe key type",
					mutate: func(events []map[string]any) {
						data := events[len(events)-1]["data"].(map[string]any)
						keys := sortedAnyKeys(test.publicData)
						for _, key := range keys {
							data[key] = map[string]any{"wrong": true}
							return
						}
					},
				},
			} {
				t.Run(mutation.name, func(t *testing.T) {
					events := canonicalLedgerEvents(t, input)
					mutation.mutate(events)
					mutated := replaceCanonicalDocument(t, cloneCanonicalInput(input), CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, events...))
					_, err := ProjectCanonicalRun(mutated)
					requireProjectionError(t, err, ErrRunEventUnknown)
				})
			}
		})
	}
	if len(registered) != 0 {
		t.Fatalf("schema-1 constants missing parity fixtures: %v", sortedBoolKeys(registered))
	}
}

func TestProjectCanonicalRunSchema1RunStartedWriterParity(t *testing.T) {
	mission := schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID))
	missionView := ProjectRunView(mustProjectCanonicalFixture(t, mission))
	assertRunRootJSON(t, missionView, `{"kind":"mission","nodeId":"`+projectionTestMissionID+`"}`)

	for _, mutation := range []struct {
		name  string
		key   string
		value any
	}{
		{name: "mission rejects mode", key: "mode", value: "formation"},
		{name: "mission rejects formation id", key: "formationId", value: projectionTestFormationID},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			input := cloneCanonicalInput(mission)
			events := canonicalLedgerEvents(t, input)
			events[0]["data"].(map[string]any)[mutation.key] = mutation.value
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, events...))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunEventUnknown)
		})
	}

	formationStart := schema1StartedEvent(projectionTestRunID)
	formationStart["missionId"] = "single_" + projectionTestFormationID
	formationStart["beadId"] = ""
	formationData := formationStart["data"].(map[string]any)
	formationData["missionId"] = "single_" + projectionTestFormationID
	formationData["beadId"] = ""
	formationData["mode"] = "formation"
	formationData["formationId"] = projectionTestFormationID
	formation := schema1ProjectionInput(t, formationStart)
	formationView := ProjectRunView(mustProjectCanonicalFixture(t, formation))
	assertRunRootJSON(t, formationView, `{"kind":"formation","nodeId":"`+projectionTestFormationID+`"}`)
	formationPage := mustProjectEventPage(t, mustProjectCanonicalFixture(t, formation), 0, 1)
	formationRaw := mustMarshalJSON(t, formationPage.Events[0])
	if !bytes.Contains(formationRaw, []byte(`"mode":"formation"`)) || !bytes.Contains(formationRaw, []byte(`"formationId":"`+projectionTestFormationID+`"`)) {
		t.Fatalf("isolated-Formation start lost public discriminants: %s", formationRaw)
	}

	for _, mutation := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing mode", mutate: func(data map[string]any) { delete(data, "mode") }},
		{name: "missing formation id", mutate: func(data map[string]any) { delete(data, "formationId") }},
		{name: "empty formation id", mutate: func(data map[string]any) { data["formationId"] = "" }},
		{name: "invalid formation id", mutate: func(data map[string]any) { data["formationId"] = "../work" }},
		{name: "mismatched formation id", mutate: func(data map[string]any) { data["formationId"] = "fmn_other" }},
		{name: "wrong mode", mutate: func(data map[string]any) { data["mode"] = "mission" }},
		{name: "unknown key", mutate: func(data map[string]any) { data["formationPath"] = "/private" }},
	} {
		t.Run("formation rejects "+mutation.name, func(t *testing.T) {
			input := cloneCanonicalInput(formation)
			events := canonicalLedgerEvents(t, input)
			mutation.mutate(events[0]["data"].(map[string]any))
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, events...))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunEventUnknown)
		})
	}
}

func TestProjectCanonicalRunSchema1OpenDispatchParity(t *testing.T) {
	variants := []struct {
		name       string
		dispatches []any
	}{
		{
			name: "three required ids",
			dispatches: []any{
				map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker"},
				map[string]any{"dispatchId": "dispatch-b", "nodeId": projectionTestFormationID, "slotId": "slot_reviewer"},
			},
		},
		{
			name: "present zero dispatch sequence",
			dispatches: []any{
				map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "dispatchSeq": 0},
				map[string]any{"dispatchId": "dispatch-b", "nodeId": projectionTestFormationID, "slotId": "slot_reviewer"},
			},
		},
	}
	for _, test := range variants {
		t.Run(test.name, func(t *testing.T) {
			projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
				schema1StartedEvent(projectionTestRunID),
				schema1BlockedEvent(projectionTestRunID, 2, true, test.dispatches),
				schema1ResumedEvent(projectionTestRunID, 3, 2, test.dispatches),
			))
			page := mustProjectEventPage(t, projection, 1, 2)
			if len(page.Events) != 2 {
				t.Fatalf("blocked/resumed page events = %d, want 2", len(page.Events))
			}
			blocked := eventDataMember(t, page.Events[0], "openDispatches")
			resumed := eventDataMember(t, page.Events[1], "openDispatches")
			if !bytes.Equal(blocked, resumed) {
				t.Fatalf("resumed carry changed blocked dispatch bytes\nblocked: %s\nresumed: %s", blocked, resumed)
			}
			if !bytes.Equal(blocked, mustMarshalJSON(t, test.dispatches)) {
				t.Fatalf("source order/optional presence changed: got %s want %s", blocked, mustMarshalJSON(t, test.dispatches))
			}
		})
	}

	for _, test := range []struct {
		name       string
		dispatches []any
	}{
		{name: "missing dispatch id", dispatches: []any{map[string]any{"nodeId": projectionTestFormationID, "slotId": "slot_worker"}}},
		{name: "missing node id", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "slotId": "slot_worker"}}},
		{name: "missing slot id", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID}}},
		{name: "invalid dispatch id grammar", dispatches: []any{map[string]any{"dispatchId": "../dispatch", "nodeId": projectionTestFormationID, "slotId": "slot_worker"}}},
		{name: "invalid node id grammar", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "nodeId": "../node", "slotId": "slot_worker"}}},
		{name: "invalid slot id grammar", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "../slot"}}},
		{name: "unsafe dispatch sequence", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "dispatchSeq": MaxJSONSafeInteger + 1}}},
		{name: "unknown nested key", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "targetLeaseId": "lease-private"}}},
		{name: "duplicate dispatch id", dispatches: []any{
			map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker"},
			map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_reviewer"},
		}},
	} {
		t.Run("rejects "+test.name, func(t *testing.T) {
			_, err := ProjectCanonicalRun(schema1ProjectionInput(t,
				schema1StartedEvent(projectionTestRunID),
				schema1BlockedEvent(projectionTestRunID, 2, true, test.dispatches),
			))
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}
}

func TestProjectCanonicalRunSchema2OpenDispatchIsSourceSelected(t *testing.T) {
	for _, test := range []struct {
		name              string
		revokedCapability bool
		blockSeq          uint64
		capabilityState   string
		revokedSeq        string
	}{
		{name: "none capability", blockSeq: 8, capabilityState: "none"},
		{name: "revoked capability", revokedCapability: true, blockSeq: 9, capabilityState: "revoked", revokedSeq: "8"},
	} {
		t.Run(test.name, func(t *testing.T) {
			validInput, wantDispatches := schema2OpenDispatchLifecycleInput(t, test.revokedCapability)
			assertSchema2OpenDispatchOrderingPrecondition(t, validInput, wantDispatches)
			projection := mustProjectCanonicalFixture(t, validInput)
			view := ProjectRunView(projection)
			if view.Status != "running" || view.Identity.Epoch != 1 || len(view.Blocks) != 1 {
				t.Fatalf("reattach lifecycle status/epoch/blocks = %q/%d/%d, want running/1/1", view.Status, view.Identity.Epoch, len(view.Blocks))
			}
			assertExactJSONValue(t, "schema-2 reattach block", view.Blocks[0], map[string]any{
				"seq": test.blockSeq, "epoch": 0, "scope": "node", "nodeId": projectionTestFormationID, "reason": "reattach required",
				"resumeAllowed": true, "resumePolicy": "reattach_only", "nextEpoch": 1, "openDispatches": wantDispatches,
			})
			page := mustProjectEventPage(t, projection, test.blockSeq-1, 2)
			if len(page.Events) != 2 {
				t.Fatalf("schema-2 block/resume page events = %d, want 2", len(page.Events))
			}
			blockedDispatches := eventDataMember(t, page.Events[0], "openDispatches")
			resumedDispatches := eventDataMember(t, page.Events[1], "openDispatches")
			wantDispatchBytes := mustMarshalJSON(t, wantDispatches)
			if !jsonBytesEqual(blockedDispatches, wantDispatchBytes) || !bytes.Equal(blockedDispatches, resumedDispatches) {
				t.Fatalf("schema-2 blocked/resumed dispatches changed\nwant:    %s\nblocked: %s\nresumed: %s", wantDispatchBytes, blockedDispatches, resumedDispatches)
			}
			var gotDispatches []map[string]json.RawMessage
			if err := json.Unmarshal(blockedDispatches, &gotDispatches); err != nil {
				t.Fatal(err)
			}
			if len(gotDispatches) != 2 {
				t.Fatalf("schema-2 dispatches = %d, want the complete two-item unmatched set", len(gotDispatches))
			}
			if string(gotDispatches[0]["dispatchId"]) != `"dsp_01KXNP6VY3227H78329V52CKF9"` || string(gotDispatches[0]["dispatchSeq"]) != `6` ||
				string(gotDispatches[1]["dispatchId"]) != `"dsp_01KXNP6VY3227H78329V52CKF8"` || string(gotDispatches[1]["dispatchSeq"]) != `7` {
				t.Fatalf("schema-2 unmatched set lost stable dispatchSeq order: %s", blockedDispatches)
			}
			gotRevokedSeq, hasRevokedSeq := gotDispatches[1]["peekCapabilityRevokedSeq"]
			if test.revokedCapability {
				if !hasRevokedSeq || string(gotRevokedSeq) != test.revokedSeq {
					t.Fatalf("revoked dispatch lost exact revocation sequence %s: %s", test.revokedSeq, blockedDispatches)
				}
			} else if hasRevokedSeq {
				t.Fatalf("none-state dispatch invented optional revocation sequence: %s", blockedDispatches)
			}
			if _, reviewerRevoked := gotDispatches[0]["peekCapabilityRevokedSeq"]; reviewerRevoked {
				t.Fatalf("unchanged reviewer dispatch invented revocation state: %s", blockedDispatches)
			}
			for index, dispatch := range gotDispatches {
				if string(dispatch["latestCapabilityGeneration"]) != `"0"` || string(dispatch["latestCapabilityIssuedSeq"]) != `0` ||
					string(dispatch["latestSteeringGeneration"]) != `"0"` {
					t.Fatalf("dispatch %d lost required zero-valued lifecycle members: %s", index, blockedDispatches)
				}
			}
			assertSchema2SessionProjection(t, view, test.capabilityState)
		})
	}

	validInput, _ := schema2OpenDispatchLifecycleInput(t, false)

	for _, test := range []struct {
		name   string
		mutate func(blocked, resumed []any)
	}{
		{name: "none state nonzero generation", mutate: func(blocked, resumed []any) {
			blocked[0].(map[string]any)["latestCapabilityGeneration"] = "1"
			resumed[0].(map[string]any)["latestCapabilityGeneration"] = "1"
		}},
		{name: "none state nonzero issued sequence", mutate: func(blocked, resumed []any) {
			blocked[0].(map[string]any)["latestCapabilityIssuedSeq"] = 4
			resumed[0].(map[string]any)["latestCapabilityIssuedSeq"] = 4
		}},
		{name: "none state carries revocation sequence", mutate: func(blocked, resumed []any) {
			blocked[0].(map[string]any)["peekCapabilityRevokedSeq"] = 5
			resumed[0].(map[string]any)["peekCapabilityRevokedSeq"] = 5
		}},
		{name: "interrupt none carries request", mutate: func(blocked, resumed []any) {
			blocked[0].(map[string]any)["interruptRequestedSeq"] = 5
			resumed[0].(map[string]any)["interruptRequestedSeq"] = 5
		}},
		{name: "unknown nested member", mutate: func(blocked, resumed []any) {
			blocked[0].(map[string]any)["targetKey"] = "private"
			resumed[0].(map[string]any)["targetKey"] = "private"
		}},
		{name: "resume carry differs from block", mutate: func(_ []any, resumed []any) {
			resumed[0].(map[string]any)["latestSteeringGeneration"] = "1"
		}},
	} {
		t.Run("rejects "+test.name, func(t *testing.T) {
			input := cloneCanonicalInput(validInput)
			events := canonicalLedgerEvents(t, input)
			blocked, resumed := schema2OpenDispatchCarries(t, events)
			test.mutate(blocked, resumed)
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(blocked, resumed []any)
	}{
		{name: "revoked state missing revocation sequence", mutate: func(blocked, resumed []any) {
			delete(blocked[1].(map[string]any), "peekCapabilityRevokedSeq")
			delete(resumed[1].(map[string]any), "peekCapabilityRevokedSeq")
		}},
		{name: "revocation sequence does not name lifecycle event", mutate: func(blocked, resumed []any) {
			blocked[1].(map[string]any)["peekCapabilityRevokedSeq"] = 7
			resumed[1].(map[string]any)["peekCapabilityRevokedSeq"] = 7
		}},
	} {
		t.Run("rejects "+test.name, func(t *testing.T) {
			input, _ := schema2OpenDispatchLifecycleInput(t, true)
			events := canonicalLedgerEvents(t, input)
			blocked, resumed := schema2OpenDispatchCarries(t, events)
			test.mutate(blocked, resumed)
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	for _, target := range []string{"block", "resume"} {
		for _, test := range []struct {
			name   string
			mutate func([]any) []any
		}{
			{name: "missing dispatch", mutate: func(carry []any) []any { return carry[:1] }},
			{name: "extra dispatch", mutate: func(carry []any) []any {
				extra := cloneStringAnyMap(carry[1].(map[string]any))
				extra["dispatchId"] = "dsp_01KXNP6VY3227H78329V52CKFA"
				extra["targetLeaseId"] = "lease_01KXNP6VY3227H78329V52CKFA"
				extra["dispatchSeq"] = 5
				return append([]any{extra}, carry...)
			}},
			{name: "duplicate dispatch", mutate: func(carry []any) []any { return append(carry, cloneAny(carry[1])) }},
			{name: "reordered dispatches", mutate: func(carry []any) []any { return []any{carry[1], carry[0]} }},
			{name: "mismatched dispatch", mutate: func(carry []any) []any {
				carry[1].(map[string]any)["targetLeaseId"] = "lease_01KXNP6VY3227H78329V52CKFA"
				return carry
			}},
		} {
			t.Run("rejects "+target+" "+test.name, func(t *testing.T) {
				input := cloneCanonicalInput(validInput)
				events := canonicalLedgerEvents(t, input)
				blocked, resumed := schema2OpenDispatchCarries(t, events)
				if target == "block" {
					schema2SetOpenDispatchCarry(t, events, "run_blocked", test.mutate(blocked))
				} else {
					schema2SetOpenDispatchCarry(t, events, "run_resumed", test.mutate(resumed))
				}
				input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
				_, err := ProjectCanonicalRun(input)
				requireProjectionError(t, err, ErrRunProjectionInvalid)
			})
		}
	}

	t.Run("rejects already-matched dispatch in block and resume carry", func(t *testing.T) {
		input := schema2AlreadyMatchedDispatchCarryInput(t)
		_, err := ProjectCanonicalRun(input)
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	schema1 := SafeSchema1OpenDispatch{
		DispatchID: "dispatch-a", NodeID: projectionTestFormationID, SlotID: "slot_worker",
	}
	schema2 := SafeSchema2OpenDispatch{
		DispatchID:                 "dsp_01KXNP6VY3227H78329V52CKF8",
		TargetLeaseID:              "lease_01KXNP6VY3227H78329V52CKF8",
		NodeID:                     projectionTestFormationID,
		Attempt:                    1,
		SlotID:                     "slot_worker",
		AgentID:                    "agent_worker",
		BindingID:                  "binding_worker",
		SessionTargetID:            "target_opaque",
		TargetFingerprint:          strings.Repeat("a", 64),
		DispatchSeq:                3,
		PeekCapabilityState:        "none",
		LatestCapabilityGeneration: "0",
		LatestCapabilityIssuedSeq:  0,
		LatestSteeringGeneration:   "0",
		InterruptState:             "none",
	}
	var schema1Arm SafeOpenDispatch = schema1
	var schema2Arm SafeOpenDispatch = schema2
	schema1Raw := mustMarshalJSON(t, schema1Arm)
	schema2Raw := mustMarshalJSON(t, schema2Arm)
	for _, forbidden := range []string{"targetLeaseId", "attempt", "agentId", "bindingId", "sessionTargetId", "targetFingerprint", "peekCapabilityState", "latestCapabilityGeneration", "latestSteeringGeneration", "interruptState"} {
		if bytes.Contains(schema1Raw, []byte(`"`+forbidden+`"`)) {
			t.Fatalf("schema-1 dispatch acquired schema-2 member %q: %s", forbidden, schema1Raw)
		}
	}
	for _, required := range []string{"targetLeaseId", "attempt", "agentId", "bindingId", "sessionTargetId", "targetFingerprint", "peekCapabilityState", "latestCapabilityGeneration", "latestSteeringGeneration", "interruptState"} {
		if !bytes.Contains(schema2Raw, []byte(`"`+required+`"`)) {
			t.Fatalf("schema-2 dispatch lost member %q: %s", required, schema2Raw)
		}
	}

	wrongForSchema1 := map[string]any{}
	if err := json.Unmarshal(schema2Raw, &wrongForSchema1); err != nil {
		t.Fatal(err)
	}
	_, err := ProjectCanonicalRun(schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1BlockedEvent(projectionTestRunID, 2, true, []any{wrongForSchema1}),
	))
	requireProjectionError(t, err, ErrRunProjectionInvalid)

	wrongForSchema2 := map[string]any{}
	if err := json.Unmarshal(schema1Raw, &wrongForSchema2); err != nil {
		t.Fatal(err)
	}
	schema2Block := schema2Event(projectionTestRunID, 3, "run_blocked", map[string]any{
		"reason": "blocked", "blockScope": "run", "resumeAllowed": true,
		"resumePolicy": "reattach_only", "openDispatches": []any{wrongForSchema2},
		"retryTargets": []any{}, "nextEpoch": 1,
	})
	_, err = ProjectCanonicalRun(schema2ProjectionInput(t, true, schema2Block))
	requireProjectionError(t, err, ErrRunProjectionInvalid)
}

func TestProjectCanonicalRunSafeRunEventHasExactDiscriminants(t *testing.T) {
	type eventTypeCase struct {
		source  int
		literal string
		typeOf  reflect.Type
	}
	tests := append(schema2SafeEventTypes(), schema1SafeEventTypes()...)
	if len(tests) != 58 {
		t.Fatalf("source-specific safe event arms = %d, want 37 schema-2 + 21 schema-1", len(tests))
	}
	byLiteral := map[string][]int{}
	safeInterface := reflect.TypeOf((*SafeRunEvent)(nil)).Elem()
	if safeInterface.Kind() != reflect.Interface || safeInterface.NumMethod() == 0 {
		t.Fatalf("SafeRunEvent = %v, want closed marker interface with no raw map fallback", safeInterface)
	}
	if reflect.TypeOf(map[string]any{}).Implements(safeInterface) {
		t.Fatal("map[string]any implements SafeRunEvent raw fallback")
	}
	for _, test := range tests {
		byLiteral[test.literal] = append(byLiteral[test.literal], test.source)
		dataField, ok := test.typeOf.FieldByName("Data")
		if !ok {
			t.Fatalf("%s has no exported Data field", test.typeOf)
		}
		wantDataName := strings.TrimSuffix(test.typeOf.Name(), "Event") + "Data"
		if test.source == 1 && test.literal == RunEventStarted {
			if dataField.Type.Kind() != reflect.Interface || dataField.Type.NumMethod() == 0 {
				t.Fatalf("%s.Data = %v, want a closed conditional-union interface", test.typeOf, dataField.Type)
			}
			mission := reflect.TypeOf(SafeSchema1RunStartedMissionData{})
			formation := reflect.TypeOf(SafeSchema1RunStartedFormationData{})
			if !mission.Implements(dataField.Type) || !formation.Implements(dataField.Type) || reflect.TypeOf(map[string]any{}).Implements(dataField.Type) {
				t.Fatalf("%s.Data union does not admit exactly the two named closed start data structs", test.typeOf)
			}
		} else if dataField.Type.Kind() != reflect.Struct || dataField.Type.Name() != wantDataName {
			t.Fatalf("%s.Data = %v, want exact named struct %s", test.typeOf, dataField.Type, wantDataName)
		}
		value := reflect.New(test.typeOf)
		field := value.Elem().FieldByName("Type")
		if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.String {
			t.Fatalf("%s has no exported string Type discriminant", test.typeOf)
		}
		field.SetString(test.literal)
		event, ok := value.Interface().(SafeRunEvent)
		if !ok {
			t.Fatalf("%s does not implement SafeRunEvent", test.typeOf)
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(mustMarshalJSON(t, event), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Type != test.literal {
			t.Fatalf("%s JSON type = %q, want %q", test.typeOf, envelope.Type, test.literal)
		}
	}
	if len(byLiteral) != 41 {
		t.Fatalf("safe event discriminants = %d, want exactly 41: %v", len(byLiteral), sortedIntSliceKeys(byLiteral))
	}
	for literal, sources := range byLiteral {
		sort.Ints(sources)
		if isSchema1OnlyEvent(literal) {
			if !reflect.DeepEqual(sources, []int{1}) {
				t.Fatalf("schema-1-only %q sources = %v, want [1]", literal, sources)
			}
			continue
		}
		if isSharedSafeEvent(literal) {
			if !reflect.DeepEqual(sources, []int{1, 2}) {
				t.Fatalf("shared %q sources = %v, want schema-specific [1 2]", literal, sources)
			}
			continue
		}
		if !reflect.DeepEqual(sources, []int{2}) {
			t.Fatalf("schema-2-only %q sources = %v, want [2]", literal, sources)
		}
	}
}

func schema1StartedEvent(runID string) map[string]any {
	return map[string]any{
		"ts":        "2026-07-20T10:00:00Z",
		"runId":     runID,
		"seq":       uint64(1),
		"type":      RunEventStarted,
		"actor":     "agent:test",
		"boardId":   projectionTestBoardID,
		"boardRev":  uint64(7),
		"missionId": projectionTestMissionID,
		"beadId":    "ctx-7i1.1",
		"epoch":     uint64(0),
		"attempt":   uint64(0),
		"data": map[string]any{
			"boardSlug":        projectionTestBoardSlug,
			"boardPath":        ".formations/boards/projection.formation.toml",
			"boardRev":         uint64(7),
			"snapshot":         ".formations/runs/projection/" + runID + ".snapshot.toml",
			"bindingsSnapshot": ".formations/runs/projection/" + runID + ".bindings.toml",
			"missionId":        projectionTestMissionID,
			"beadId":           "ctx-7i1.1",
			"objective":        "Project the run",
			"limits": map[string]any{
				"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false,
			},
		},
	}
}

func schema1Event(runID string, sequence uint64, eventType string, data map[string]any) map[string]any {
	event := map[string]any{
		"ts":        fmt.Sprintf("2026-07-20T10:00:%02dZ", sequence-1),
		"runId":     runID,
		"seq":       sequence,
		"type":      eventType,
		"actor":     "agent:test",
		"boardId":   projectionTestBoardID,
		"boardRev":  uint64(7),
		"missionId": projectionTestMissionID,
		"beadId":    "ctx-7i1.1",
		"epoch":     uint64(0),
		"attempt":   uint64(1),
		"data":      cloneStringAnyMap(data),
	}
	switch eventType {
	case RunEventNodeWaiting, RunEventNodeStarted, RunEventSlotDispatch, RunEventAdapterSend, RunEventSlotResult, RunEventNodeOutput:
		event["nodeId"] = projectionTestFormationID
	case RunEventGateEvaluating, RunEventGateVerdict, RunEventHumanInputRequested, RunEventHumanVerdictRecorded:
		event["nodeId"] = projectionTestFormationID
		event["gateId"] = projectionTestGateID
	case RunEventEscalationRaised, RunEventBlocked:
		event["nodeId"] = projectionTestFormationID
	}
	if eventType == RunEventResumed {
		event["epoch"] = uint64(1)
	}
	return event
}

func schema2Event(runID string, sequence uint64, eventType string, data map[string]any) map[string]any {
	event := schema1Event(runID, sequence, eventType, data)
	event["schema"] = uint64(2)
	event["authoritySchema"] = uint64(2)
	event["writerFence"] = uint64(1)
	return event
}

func schema1NodeStartedEvent(runID string, sequence uint64) map[string]any {
	return schema1Event(runID, sequence, RunEventNodeStarted, map[string]any{
		"nodeKind": "formation",
		"inputRefs": []any{
			map[string]any{"edgeId": "edge_root_work", "fromNodeId": projectionTestMissionID, "fromPortId": "out", "toPortId": "port_in", "outputSeq": 1},
		},
		"reason": "initial",
	})
}

func schema1NodeOutputEvent(runID string, sequence uint64, status string) map[string]any {
	return schema1Event(runID, sequence, RunEventNodeOutput, map[string]any{
		"status": status,
		"text":   "done",
		"outputs": map[string]any{
			"port_out": map[string]any{"text": "done"},
		},
		"reason": "completed",
	})
}

func schema1GateEvaluatingEvent(runID string, sequence uint64) map[string]any {
	return schema1Event(runID, sequence, RunEventGateEvaluating, map[string]any{
		"kinds":     []string{"human"},
		"criterion": "Approve the result",
		"inputRef": map[string]any{
			"edgeId": "edge_work_gate", "fromNodeId": projectionTestFormationID, "fromPortId": "port_out", "toPortId": "in", "outputSeq": 1,
		},
		"judgeChain": []string{},
	})
}

func schema1GateVerdictEvent(runID string, sequence uint64, verdict string) map[string]any {
	return schema1Event(runID, sequence, RunEventGateVerdict, map[string]any{
		"verdict":     verdict,
		"perKind":     map[string]any{"human": verdict},
		"routePort":   verdict,
		"routedEdges": []string{},
		"reason":      "reviewed",
		"inputRef": map[string]any{
			"edgeId": "edge_work_gate", "fromNodeId": projectionTestFormationID, "fromPortId": "port_out", "toPortId": "in", "outputSeq": 1,
		},
	})
}

func schema1EscalationEvent(runID string, sequence uint64, blocks bool) map[string]any {
	return schema1Event(runID, sequence, RunEventEscalationRaised, map[string]any{
		"trigger": "sentinel", "severity": "needs-attention", "reason": "operator review",
		"source": "agent", "nodeId": projectionTestFormationID, "gateId": "", "blocks": blocks,
	})
}

func schema1BlockedEvent(runID string, sequence uint64, resumeAllowed bool, dispatches []any) map[string]any {
	return schema1Event(runID, sequence, RunEventBlocked, map[string]any{
		"reason": "blocked", "code": "operator_review", "boundary": "engine",
		"blockedNodeId": projectionTestFormationID, "blockedGateId": "", "waitingNodes": []string{},
		"recoverable": resumeAllowed, "resumeAllowed": resumeAllowed, "resumePolicy": "explicit",
		"openDispatches": dispatches, "nextEpoch": uint64(1),
	})
}

func schema1ResumedEvent(runID string, sequence, blockedSequence uint64, dispatches []any) map[string]any {
	return schema1Event(runID, sequence, RunEventResumed, map[string]any{
		"resumedFromSeq": blockedSequence, "resumedBy": "human:test", "resumeMode": "reattach",
		"reason": "continue", "openDispatches": dispatches,
	})
}

const schema1ProjectionSnapshot = `schema = 1
id = "brd_projection"
slug = "projection"
title = "Projection fixture"
rev = 7

[[mission]]
id = "mis_root"
title = "Root"
goal = "Project the run"
beadId = "ctx-7i1.1"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[[formation.input]]
id = "port_in"
label = "Input"

[[formation.output]]
id = "port_out"
label = "Output"

[[formation.slot]]
id = "slot_worker"
label = "Worker"
agentId = "worker"
harness = "codex"
controller = true

[[formation.slot]]
id = "slot_reviewer"
label = "Reviewer"
agentId = "reviewer"
harness = "codex"
controller = false

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["human"]
criterion = "Approve the result"

[[connection]]
id = "edge_root_work"
from = "mis_root:out"
to = "fmn_work:port_in"

[[connection]]
id = "edge_work_gate"
from = "fmn_work:port_out"
to = "gate_review:in"
`

const schema1ProjectionBindings = `schema = 1
runId = "run_01KXNP6VY3227H78329V52CKF8"
boardId = "brd_projection"
boardSlug = "projection"
boardRev = 7
missionId = "mis_root"

[[binding]]
nodeId = "fmn_work"
slotId = "slot_worker"
agentId = "worker"
harness = "codex"
sessionStem = "worker"
cardPath = "/private/worker.toml"
cardSha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

[[binding]]
nodeId = "fmn_work"
slotId = "slot_reviewer"
agentId = "reviewer"
harness = "codex"
sessionStem = "reviewer"
cardPath = "/private/reviewer.toml"
cardSha256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
`

const schema1OrderingSnapshot = `schema = 1
id = "brd_projection"
slug = "projection"
title = "Projection ordering fixture"
rev = 7

[[mission]]
id = "mis_root"
title = "Root"
goal = "Project the run"
beadId = "ctx-7i1.1"

[[formation]]
id = "fmn_alpha"
type = "solo"
title = "Alpha"
[[formation.input]]
id = "port_in"
label = "Input"
[[formation.output]]
id = "port_z"
label = "First declared output"
[[formation.output]]
id = "port_a"
label = "Second declared output"
[[formation.slot]]
id = "slot_alpha"
label = "Alpha"
agentId = "alpha"
harness = "codex"
controller = true

[[formation]]
id = "fmn_beta"
type = "solo"
title = "Beta"
[[formation.input]]
id = "port_in"
label = "Input"
[[formation.output]]
id = "port_b"
label = "Output"
[[formation.slot]]
id = "slot_beta"
label = "Beta"
agentId = "beta"
harness = "codex"
controller = true

[[gate]]
id = "gate_alpha"
title = "Alpha review"
kinds = ["human"]
criterion = "Approve alpha"

[[gate]]
id = "gate_beta"
title = "Beta review"
kinds = ["human"]
criterion = "Approve beta"

[[connection]]
id = "edge_root_alpha"
from = "mis_root:out"
to = "fmn_alpha:port_in"
[[connection]]
id = "edge_root_beta"
from = "mis_root:out"
to = "fmn_beta:port_in"
[[connection]]
id = "edge_alpha_gate"
from = "fmn_alpha:port_z"
to = "gate_alpha:in"
[[connection]]
id = "edge_beta_gate"
from = "fmn_beta:port_b"
to = "gate_beta:in"
`

const schema1OrderingBindings = `schema = 1
runId = "run_01KXNP6VY3227H78329V52CKF8"
boardId = "brd_projection"
boardSlug = "projection"
boardRev = 7
missionId = "mis_root"

[[binding]]
nodeId = "fmn_beta"
slotId = "slot_beta"
agentId = "beta"
harness = "codex"
sessionStem = "beta"
cardPath = "/private/beta.toml"
cardSha256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

[[binding]]
nodeId = "fmn_alpha"
slotId = "slot_alpha"
agentId = "alpha"
harness = "codex"
sessionStem = "alpha"
cardPath = "/private/alpha.toml"
cardSha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
`

func schema1OrderingProjectionInput(t *testing.T) CanonicalRunReadInput {
	t.Helper()
	inputRef := func(edgeID, nodeID, portID string, outputSeq uint64) map[string]any {
		return map[string]any{
			"edgeId": edgeID, "fromNodeId": nodeID, "fromPortId": portID, "toPortId": "in", "outputSeq": outputSeq,
		}
	}
	nodeStarted := func(sequence uint64, nodeID, edgeID string) map[string]any {
		event := schema1Event(projectionTestRunID, sequence, RunEventNodeStarted, map[string]any{
			"nodeKind": "formation", "reason": "initial",
			"inputRefs": []any{map[string]any{
				"edgeId": edgeID, "fromNodeId": projectionTestMissionID, "fromPortId": "out", "toPortId": "port_in", "outputSeq": 1,
			}},
		})
		event["nodeId"] = nodeID
		return event
	}
	nodeOutput := func(sequence uint64, nodeID string, outputs map[string]any) map[string]any {
		event := schema1Event(projectionTestRunID, sequence, RunEventNodeOutput, map[string]any{
			"status": "done", "text": nodeID + " done", "outputs": outputs, "reason": "completed",
		})
		event["nodeId"] = nodeID
		return event
	}
	gateEvaluating := func(sequence uint64, gateID string, input map[string]any) map[string]any {
		event := schema1Event(projectionTestRunID, sequence, RunEventGateEvaluating, map[string]any{
			"kinds": []string{"human"}, "criterion": "Approve " + gateID, "inputRef": input, "judgeChain": []string{},
		})
		event["nodeId"] = gateID
		event["gateId"] = gateID
		return event
	}
	gateVerdict := func(sequence uint64, gateID string, input map[string]any) map[string]any {
		event := schema1Event(projectionTestRunID, sequence, RunEventGateVerdict, map[string]any{
			"verdict": "pass", "perKind": map[string]any{"human": "pass"}, "routePort": "pass", "routedEdges": []string{},
			"reason": "reviewed", "inputRef": input,
		})
		event["nodeId"] = gateID
		event["gateId"] = gateID
		return event
	}
	betaInput := inputRef("edge_beta_gate", "fmn_beta", "port_b", 3)
	alphaInput := inputRef("edge_alpha_gate", "fmn_alpha", "port_z", 7)
	input := schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		nodeStarted(2, "fmn_beta", "edge_root_beta"),
		nodeOutput(3, "fmn_beta", map[string]any{"port_b": map[string]any{"text": "beta"}}),
		gateEvaluating(4, "gate_beta", betaInput),
		gateVerdict(5, "gate_beta", betaInput),
		nodeStarted(6, "fmn_alpha", "edge_root_alpha"),
		nodeOutput(7, "fmn_alpha", map[string]any{
			"port_a": map[string]any{"text": "alpha second"}, "port_z": map[string]any{"text": "alpha first"},
		}),
		gateEvaluating(8, "gate_alpha", alphaInput),
		gateVerdict(9, "gate_alpha", alphaInput),
	)
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema1GraphSnapshot, []byte(schema1OrderingSnapshot))
	return replaceCanonicalDocument(t, input, CanonicalInputRoleSchema1BindingsSnapshot, []byte(schema1OrderingBindings))
}

func schema1ProjectionInput(t *testing.T, events ...map[string]any) CanonicalRunReadInput {
	t.Helper()
	return CanonicalRunReadInput{
		RunID:  projectionTestRunID,
		Source: CanonicalRunSourceSchema1,
		Documents: []CanonicalInputDocument{
			canonicalInputDocument(CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, events...)),
			canonicalInputDocument(CanonicalInputRoleSchema1GraphSnapshot, []byte(schema1ProjectionSnapshot)),
			canonicalInputDocument(CanonicalInputRoleSchema1BindingsSnapshot, []byte(schema1ProjectionBindings)),
		},
	}
}

type schema2AuthorityFixture struct {
	policies             []CanonicalInputDocument
	authorityGenerations [][]byte
	selectedPolicyRev    uint64
	selectedPolicySHA256 string
}

func schema2AuthorityHistory(t *testing.T, rootHash string) schema2AuthorityFixture {
	t.Helper()
	disabledPolicy := canonicalInputDocument(CanonicalInputRoleSchema2AdmissionPolicy, canonicalJSON(t, map[string]any{
		"policySchema": 1, "policyRev": 1, "priorPolicySha256": "", "state": "disabled",
	}))
	initialAuthority := canonicalJSON(t, map[string]any{
		"recordRev": 1, "priorGeneration": nil, "authoritySchema": 2,
		"workspaceAuthorityId": projectionTestWorkspaceID,
		"rootIdentityEncoding": "workspace-root-identity-v1", "workspaceRootIdentitySha256": rootHash,
		"nextWriterFence": 1, "nextAdmissionSeq": 1,
		"admissionPolicyRef": map[string]any{"policyRev": 1, "policySha256": disabledPolicy.SHA256},
	})
	writerAllocated := nextMutableRecordGeneration(t, initialAuthority, func(record map[string]any) {
		record["nextWriterFence"] = 2
	})
	configuredPolicy := canonicalInputDocument(CanonicalInputRoleSchema2AdmissionPolicy, canonicalJSON(t, map[string]any{
		"policySchema": 1, "policyRev": 2, "priorPolicySha256": disabledPolicy.SHA256, "state": "configured",
		"maxActiveRuns": 1, "maxQueuedRuns": 1,
	}))
	policyConfigured := nextMutableRecordGeneration(t, writerAllocated, func(record map[string]any) {
		record["admissionPolicyRef"] = map[string]any{"policyRev": 2, "policySha256": configuredPolicy.SHA256}
	})
	admissionAllocated := nextMutableRecordGeneration(t, policyConfigured, func(record map[string]any) {
		record["nextAdmissionSeq"] = 2
	})
	return schema2AuthorityFixture{
		policies:             []CanonicalInputDocument{disabledPolicy, configuredPolicy},
		authorityGenerations: [][]byte{initialAuthority, writerAllocated, policyConfigured, admissionAllocated},
		selectedPolicyRev:    2,
		selectedPolicySHA256: configuredPolicy.SHA256,
	}
}

func nextMutableRecordGeneration(t *testing.T, previous []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal(previous, &record); err != nil {
		t.Fatal(err)
	}
	previousRev := uint64(record["recordRev"].(float64))
	record["recordRev"] = previousRev + 1
	record["priorGeneration"] = map[string]any{"recordRev": previousRev, "sha256": projectionSHA256(previous)}
	mutate(record)
	return canonicalJSON(t, record)
}

func schema2ProjectionInput(t *testing.T, activated bool, extra ...map[string]any) CanonicalRunReadInput {
	t.Helper()
	graph := []byte(`schema = 2
id = "brd_projection"
slug = "projection"
title = "Projection fixture"
rev = 7

[[mission]]
id = "mis_root"
title = "Root"
goal = "Project the run"
beadId = "ctx-7i1.1"

[[authoredConfigManifest]]
classification = "authored_config"
sourceKind = "mission_objective"
nodeId = "mis_root"
encoding = "mission-objective-utf8-v1"
mediaType = "text/markdown"
sha256 = "` + projectionSHA256([]byte("Project the run")) + `"
`)
	bindings := []byte(`schema = 2
runId = "` + projectionTestRunID + `"
boardId = "` + projectionTestBoardID + `"
boardRev = 7
`)
	graphHash := projectionSHA256(graph)
	bindingsHash := projectionSHA256(bindings)
	rootHash := strings.Repeat("1", 64)
	authority := schema2AuthorityHistory(t, rootHash)
	commandRecord := schema2AdmissionCommandRecord(t, projectionTestCommandID, authority.selectedPolicyRev, authority.selectedPolicySHA256)
	var command map[string]any
	if err := json.Unmarshal(commandRecord, &command); err != nil {
		t.Fatal(err)
	}
	commandPayloadHash := command["commandPayloadSha256"].(string)
	runBootstrap := canonicalJSON(t, map[string]any{
		"runBootstrapSchema":      1,
		"workspaceAuthorityId":    projectionTestWorkspaceID,
		"runId":                   projectionTestRunID,
		"runAuthorityId":          projectionTestAuthorityID,
		"graphSnapshotEncoding":   "run-graph-snapshot-toml-v1",
		"graphSnapshotSha256":     graphHash,
		"privateBindingsEncoding": "run-private-bindings-toml-v1",
		"privateBindingsSha256":   bindingsHash,
	})
	started := schema2Event(projectionTestRunID, 1, "run_started", map[string]any{
		"workspaceAuthorityId":    projectionTestWorkspaceID,
		"workspaceAdmissionSeq":   1,
		"admissionPolicyRev":      authority.selectedPolicyRev,
		"admissionPolicySha256":   authority.selectedPolicySHA256,
		"admissionCommandId":      projectionTestCommandID,
		"commandPayloadSha256":    commandPayloadHash,
		"boardSlug":               projectionTestBoardSlug,
		"boardPath":               ".formations/boards/projection.formation.toml",
		"sourceBoardSchema":       2,
		"snapshotSchema":          2,
		"runAuthorityId":          projectionTestAuthorityID,
		"graphSnapshotSha256":     graphHash,
		"privateBindingsSha256":   bindingsHash,
		"bindingProjectionSha256": strings.Repeat("2", 64),
		"runRoot":                 map[string]any{"kind": "mission", "nodeId": projectionTestMissionID},
		"rootInputProjection": map[string]any{
			"classification": "authored_config", "sourceKind": "mission_objective",
			"encoding": "mission-objective-utf8-v1", "mediaType": "text/markdown",
			"sha256": projectionSHA256([]byte("Project the run")), "text": "Project the run",
		},
		"limits": map[string]any{"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false},
	})
	started["actor"] = "human:test"
	events := []map[string]any{started}
	if activated {
		events = append(events, schema2Event(projectionTestRunID, 2, "run_activated", map[string]any{
			"workspaceAdmissionSeq": 1, "admissionPolicyRev": authority.selectedPolicyRev,
			"admissionPolicySha256": authority.selectedPolicySHA256, "reason": "immediate",
		}))
	}
	events = append(events, extra...)
	documents := []CanonicalInputDocument{
		canonicalInputDocument(CanonicalInputRoleSchema2WorkspaceRegistry, canonicalJSON(t, map[string]any{
			"registrySchema": 1, "recordRev": 1, "priorGeneration": nil,
			"entries": []any{map[string]any{
				"workspaceAuthorityId": projectionTestWorkspaceID, "configuredPath": "/workspace",
				"device": "1", "inode": "2", "workspaceRootIdentitySha256": rootHash,
			}},
		})),
		canonicalInputDocument(CanonicalInputRoleSchema2WorkspaceBootstrap, canonicalJSON(t, map[string]any{
			"bootstrapSchema": 1, "workspaceAuthorityId": projectionTestWorkspaceID,
			"rootIdentityEncoding": "workspace-root-identity-v1", "workspaceRootIdentitySha256": rootHash,
		})),
		canonicalInputDocument(CanonicalInputRoleSchema2WorkspaceAuthority, authority.authorityGenerations[len(authority.authorityGenerations)-1]),
	}
	documents = append(documents, authority.policies...)
	documents = append(documents,
		canonicalInputDocument(CanonicalInputRoleSchema2RunBootstrap, runBootstrap),
		canonicalInputDocument(CanonicalInputRoleSchema2GraphSnapshot, graph),
		canonicalInputDocument(CanonicalInputRoleSchema2PrivateBindings, bindings),
		canonicalInputDocument(CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...)),
		canonicalInputDocument(CanonicalInputRoleSchema2CommandRecord, commandRecord),
	)
	return CanonicalRunReadInput{
		RunID:     projectionTestRunID,
		Source:    CanonicalRunSourceSchema2,
		Documents: documents,
	}
}

func schema2OpenDispatchLifecycleInput(t *testing.T, revokedCapability bool) (CanonicalRunReadInput, []any) {
	t.Helper()
	input := schema2ProjectionInput(t, true)
	rootText := `{"goal":"Project the run"}`
	rootHash := projectionSHA256([]byte(rootText))
	graph := []byte(`schema = 2
id = "` + projectionTestBoardID + `"
slug = "` + projectionTestBoardSlug + `"
title = "Projection fixture"
rev = 7

[[formation]]
id = "` + projectionTestFormationID + `"
type = "peer"
title = "Work"

[[formation.input]]
id = "port_in"
label = "Input"

[[formation.output]]
id = "port_out"
label = "Output"

[[formation.slot]]
id = "slot_reviewer"
label = "Reviewer"
agentId = "reviewer"
harness = "codex"
controller = true

[[formation.slot]]
id = "slot_worker"
label = "Worker"
agentId = "worker"
harness = "codex"
controller = false

[[authoredConfigManifest]]
classification = "authored_config"
sourceKind = "formation_brief"
nodeId = "` + projectionTestFormationID + `"
encoding = "formation-brief-jcs-v1"
mediaType = "application/json"
sha256 = "` + rootHash + `"
`)
	bindings := []byte(`schema = 2
runId = "` + projectionTestRunID + `"
boardId = "` + projectionTestBoardID + `"
boardRev = 7

[[binding]]
bindingId = "binding_worker"
nodeId = "` + projectionTestFormationID + `"
slotId = "slot_worker"
agentId = "worker"
harness = "codex"
sessionTargetId = "target_worker"
targetFingerprint = "` + strings.Repeat("a", 64) + `"
sessionLineageSha256 = "` + strings.Repeat("c", 64) + `"

[[binding]]
bindingId = "binding_reviewer"
nodeId = "` + projectionTestFormationID + `"
slotId = "slot_reviewer"
agentId = "reviewer"
harness = "codex"
sessionTargetId = "target_reviewer"
targetFingerprint = "` + strings.Repeat("b", 64) + `"
sessionLineageSha256 = "` + strings.Repeat("d", 64) + `"
`)
	graphHash := projectionSHA256(graph)
	bindingsHash := projectionSHA256(bindings)

	var bootstrap map[string]any
	if err := json.Unmarshal(canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2RunBootstrap).Bytes, &bootstrap); err != nil {
		t.Fatal(err)
	}
	bootstrap["graphSnapshotSha256"] = graphHash
	bootstrap["privateBindingsSha256"] = bindingsHash
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2RunBootstrap, canonicalJSON(t, bootstrap))
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2GraphSnapshot, graph)
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2PrivateBindings, bindings)

	events := canonicalLedgerEvents(t, input)
	started := events[0]
	delete(started, "missionId")
	delete(started, "beadId")
	startedData := started["data"].(map[string]any)
	startedData["graphSnapshotSha256"] = graphHash
	startedData["privateBindingsSha256"] = bindingsHash
	startedData["bindingProjectionSha256"] = projectionSHA256(canonicalJSON(t, []any{
		map[string]any{"bindingId": "binding_reviewer", "nodeId": projectionTestFormationID, "slotId": "slot_reviewer", "sessionTargetId": "target_reviewer"},
		map[string]any{"bindingId": "binding_worker", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "sessionTargetId": "target_worker"},
	}))
	startedData["runRoot"] = map[string]any{"kind": "formation", "nodeId": projectionTestFormationID}
	startedData["rootInputProjection"] = map[string]any{
		"classification": "authored_config", "sourceKind": "formation_brief", "encoding": "formation-brief-jcs-v1",
		"mediaType": "application/json", "sha256": rootHash, "text": rootText,
	}
	delete(events[1], "missionId")
	delete(events[1], "beadId")

	rootPayload := map[string]any{
		"availability": "available", "exact": true, "classification": "authored_config", "sourceKind": "formation_brief",
		"encoding": "formation-brief-jcs-v1", "mediaType": "application/json", "sha256": rootHash,
		"payload": map[string]any{"kind": "work", "mediaType": "application/json", "text": rootText},
	}
	nodeStarted := schema2FormationEvent(projectionTestRunID, 3, "node_started", map[string]any{
		"nodeId": projectionTestFormationID, "nodeKind": "formation", "attempt": 1, "reason": "initial",
		"inputRefs": []any{map[string]any{
			"inputId": "input_run_seed", "sourceKind": "run_seed", "runId": projectionTestRunID, "seedId": "seed_root",
			"seedEncoding": "formation-brief-jcs-v1", "seedMediaType": "application/json", "seedSha256": rootHash,
			"toNodeId": projectionTestFormationID, "toPortId": "port_in", "payloadProjection": rootPayload,
		}},
	})
	bindingWorker := schema2FormationEvent(projectionTestRunID, 4, "slot_binding_observed", map[string]any{
		"bindingId": "binding_worker", "slotId": "slot_worker", "sessionTargetId": "target_worker", "health": "runnable",
		"reason": "resolved", "observedAt": "2026-07-20T10:00:03Z", "relatedSeq": 3,
	})
	bindingWorker["nodeId"] = projectionTestFormationID
	bindingWorker["slotId"] = "slot_worker"
	bindingReviewer := schema2FormationEvent(projectionTestRunID, 5, "slot_binding_observed", map[string]any{
		"bindingId": "binding_reviewer", "slotId": "slot_reviewer", "sessionTargetId": "target_reviewer", "health": "runnable",
		"reason": "resolved", "observedAt": "2026-07-20T10:00:04Z", "relatedSeq": 3,
	})
	bindingReviewer["nodeId"] = projectionTestFormationID
	bindingReviewer["slotId"] = "slot_reviewer"
	dispatchReviewer := schema2SlotDispatchEvent(t, 6, "dsp_01KXNP6VY3227H78329V52CKF9", "lease_01KXNP6VY3227H78329V52CKF9", "slot_reviewer", "reviewer", "binding_reviewer", "target_reviewer", strings.Repeat("b", 64), "peer-turn")
	dispatchWorker := schema2SlotDispatchEvent(t, 7, "dsp_01KXNP6VY3227H78329V52CKF8", "lease_01KXNP6VY3227H78329V52CKF8", "slot_worker", "worker", "binding_worker", "target_worker", strings.Repeat("a", 64), "peer-turn")

	openDispatches := []any{
		map[string]any{
			"dispatchId": "dsp_01KXNP6VY3227H78329V52CKF9", "targetLeaseId": "lease_01KXNP6VY3227H78329V52CKF9",
			"nodeId": projectionTestFormationID, "attempt": 1, "slotId": "slot_reviewer", "agentId": "reviewer", "bindingId": "binding_reviewer",
			"sessionTargetId": "target_reviewer", "targetFingerprint": strings.Repeat("b", 64), "dispatchSeq": 6,
			"peekCapabilityState": "none", "latestCapabilityGeneration": "0", "latestCapabilityIssuedSeq": 0,
			"latestSteeringGeneration": "0", "interruptState": "none",
		},
		map[string]any{
			"dispatchId": "dsp_01KXNP6VY3227H78329V52CKF8", "targetLeaseId": "lease_01KXNP6VY3227H78329V52CKF8",
			"nodeId": projectionTestFormationID, "attempt": 1, "slotId": "slot_worker", "agentId": "worker", "bindingId": "binding_worker",
			"sessionTargetId": "target_worker", "targetFingerprint": strings.Repeat("a", 64), "dispatchSeq": 7,
			"peekCapabilityState": "none", "latestCapabilityGeneration": "0", "latestCapabilityIssuedSeq": 0,
			"latestSteeringGeneration": "0", "interruptState": "none",
		},
	}
	nextSeq := uint64(8)
	if revokedCapability {
		revoked := schema2FormationEvent(projectionTestRunID, nextSeq, "slot_peek_capability_revoked", map[string]any{
			"dispatchId": "dsp_01KXNP6VY3227H78329V52CKF8", "targetLeaseId": "lease_01KXNP6VY3227H78329V52CKF8",
			"bindingId": "binding_worker", "sessionTargetId": "target_worker", "targetFingerprint": strings.Repeat("a", 64),
			"capabilityGeneration": "0", "capabilityIssuedSeq": 0, "steeringGeneration": "0", "reason": "recovered_fence",
			"revokedAt": "2026-07-20T10:00:05Z", "inputClosed": true,
		})
		revoked["nodeId"] = projectionTestFormationID
		revoked["slotId"] = "slot_worker"
		events = append(events, nodeStarted, bindingWorker, bindingReviewer, dispatchReviewer, dispatchWorker, revoked)
		item := openDispatches[1].(map[string]any)
		item["peekCapabilityState"] = "revoked"
		item["peekCapabilityRevokedSeq"] = nextSeq
		nextSeq++
	} else {
		events = append(events, nodeStarted, bindingWorker, bindingReviewer, dispatchReviewer, dispatchWorker)
	}
	blockedSeq := nextSeq
	blocked := schema2FormationEvent(projectionTestRunID, blockedSeq, "run_blocked", map[string]any{
		"reason": "reattach required", "blockScope": "node", "blockedNodeId": projectionTestFormationID,
		"resumeAllowed": true, "resumePolicy": "reattach_only", "openDispatches": cloneAny(openDispatches),
		"retryTargets": []any{}, "nextEpoch": 1,
	})
	blocked["nodeId"] = projectionTestFormationID
	resumePayload := canonicalCommandPayload("resume", projectionTestRunID)
	resumePayload["blockedSeq"] = blockedSeq
	resumePayloadRaw := canonicalJSON(t, resumePayload)
	resumePayloadHash := projectionSHA256(resumePayloadRaw)
	resumeSeq := blockedSeq + 1
	resumed := schema2FormationEvent(projectionTestRunID, resumeSeq, "run_resumed", map[string]any{
		"commandId": projectionTestOtherCmdID, "commandPayloadSha256": resumePayloadHash, "resumedFromSeq": blockedSeq,
		"resumedBy": "human:test", "resumeMode": "reattach", "reason": "continue", "openDispatches": cloneAny(openDispatches), "retryTargets": []any{},
	})
	resumed["actor"] = "human:test"
	resumed["epoch"] = uint64(1)
	events = append(events, blocked, resumed)

	admissionPayload := canonicalCommandPayload("start", projectionTestRunID)
	admissionPayload["runRoot"] = map[string]any{"kind": "formation", "nodeId": projectionTestFormationID}
	policyRev, policyHash := selectedAdmissionPolicy(t, input)
	admissionHistory := schema2CommandHistoryForPayload(t, projectionTestCommandID, "start", "applied", admissionPayload, schema2CommandOutcome{
		runID: projectionTestRunID, effectSeq: 1, admittedWriterFence: 1, stateWriterFence: 1, outcomeWriterFence: 1,
		decisionPolicyRef: map[string]any{"policyRev": policyRev, "policySha256": policyHash},
	})
	var admissionRecord map[string]any
	if err := json.Unmarshal(admissionHistory.terminal, &admissionRecord); err != nil {
		t.Fatal(err)
	}
	startedData["commandPayloadSha256"] = admissionRecord["commandPayloadSha256"]
	resumeHistory := schema2CommandHistoryForPayload(t, projectionTestOtherCmdID, "resume", "applied", resumePayload, schema2CommandOutcome{
		runID: projectionTestRunID, effectSeq: resumeSeq, admittedWriterFence: 1, stateWriterFence: 1, outcomeWriterFence: 1,
	})
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2CommandRecord, admissionHistory.terminal)
	input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema2CommandRecord, resumeHistory.terminal))
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
	return input, openDispatches
}

func schema2AlreadyMatchedDispatchCarryInput(t *testing.T) CanonicalRunReadInput {
	t.Helper()
	input, _ := schema2OpenDispatchLifecycleInput(t, false)
	events := canonicalLedgerEvents(t, input)
	blocked := cloneStringAnyMap(events[len(events)-2])
	resumed := cloneStringAnyMap(events[len(events)-1])
	blocked["seq"] = uint64(10)
	blocked["ts"] = "2026-07-20T10:00:09Z"
	resumed["seq"] = uint64(11)
	resumed["ts"] = "2026-07-20T10:00:10Z"
	resumedData := resumed["data"].(map[string]any)
	resumedData["resumedFromSeq"] = uint64(10)
	resumePayload := canonicalCommandPayload("resume", projectionTestRunID)
	resumePayload["blockedSeq"] = uint64(10)
	resumePayloadHash := projectionSHA256(canonicalJSON(t, resumePayload))
	resumedData["commandPayloadSha256"] = resumePayloadHash
	resumeHistory := schema2CommandHistoryForPayload(t, projectionTestOtherCmdID, "resume", "applied", resumePayload, schema2CommandOutcome{
		runID: projectionTestRunID, effectSeq: 11, admittedWriterFence: 1, stateWriterFence: 1, outcomeWriterFence: 1,
	})

	events = append(events[:len(events)-2], schema2MatchedReviewerEvents(t)...)
	events = append(events, blocked, resumed)
	input = replaceCanonicalCommandRecord(t, input, projectionTestOtherCmdID, resumeHistory.terminal)
	return replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
}

func schema2MatchedReviewerEvents(t *testing.T) []map[string]any {
	t.Helper()
	dispatchID := "dsp_01KXNP6VY3227H78329V52CKF9"
	leaseID := "lease_01KXNP6VY3227H78329V52CKF9"
	fingerprint := strings.Repeat("b", 64)
	revoked := schema2FormationEvent(projectionTestRunID, 8, "slot_peek_capability_revoked", map[string]any{
		"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": "binding_reviewer", "sessionTargetId": "target_reviewer",
		"targetFingerprint": fingerprint, "capabilityGeneration": "0", "capabilityIssuedSeq": 0, "steeringGeneration": "0",
		"reason": "result_closure", "revokedAt": "2026-07-20T10:00:07Z", "inputClosed": true,
	})
	revoked["nodeId"] = projectionTestFormationID
	revoked["slotId"] = "slot_reviewer"

	startCursor := map[string]any{"historyEpoch": "AQIDBAUGBwgJCgsMDQ4PEA", "offset": "0"}
	endCursor := map[string]any{"historyEpoch": "AQIDBAUGBwgJCgsMDQ4PEA", "offset": "24"}
	fingerprintHash := projectionSHA256([]byte(fingerprint))
	closureBarrier := map[string]any{
		"dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprintSha256": fingerprintHash,
		"monitorEvidenceSha256": strings.Repeat("2", 64), "terminalCaptureEnd": endCursor,
	}
	auditProof := map[string]any{
		"proofKind": "tmux_clients_accounted", "dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprint": fingerprint,
		"attachmentAuditRegistrationSha256": strings.Repeat("1", 64), "closureBarrierSha256": projectionSHA256(canonicalJSON(t, closureBarrier)),
		"terminalCaptureEnd": endCursor, "monitorEvidenceSha256": strings.Repeat("2", 64),
	}
	auditHash := projectionSHA256(canonicalJSON(t, auditProof))
	sentinel := "<<<CHROTE-DONE run-id=" + projectionTestRunID + " status=ok artifact=report.md>>>"
	baseline := map[string]any{
		"targetFingerprintSha256": fingerprintHash, "historyEpoch": "AQIDBAUGBwgJCgsMDQ4PEA", "offset": "0", "cols": 120, "rows": 40,
	}
	baselineHash := projectionSHA256(canonicalJSON(t, baseline))
	turnResult := map[string]any{
		"turnKey": "turn_slot_reviewer", "phase": "peer-turn", "status": "done",
		"turnPayload": map[string]any{"availability": "available", "exact": true, "payload": map[string]any{"kind": "work", "mediaType": "text/plain", "text": "review complete"}},
		"outputs":     map[string]any{}, "reportArtifactId": "", "artifactIds": []any{}, "diffArtifactIds": []any{},
	}
	turnClosureProof := map[string]any{
		"proofKind": "harness_turn_closed", "dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprint": fingerprint,
		"paneHistoryBaselineSha256": baselineHash, "peekCapabilityRevokedSeq": 8, "steeringGeneration": "0",
		"sentinelSha256": projectionSHA256([]byte(sentinel)), "terminalCaptureEnd": endCursor,
		"harnessReadyEvidenceSha256": strings.Repeat("4", 64), "clientAttachmentAuditProofSha256": auditHash,
	}
	result := schema2FormationEvent(projectionTestRunID, 9, "slot_result", map[string]any{
		"dispatchId": dispatchID, "targetLeaseId": leaseID, "turnKey": "turn_slot_reviewer", "turnPhase": "peer-turn",
		"nodeId": projectionTestFormationID, "attempt": 1, "slotId": "slot_reviewer", "agentId": "reviewer", "bindingId": "binding_reviewer",
		"sessionTargetId": "target_reviewer", "targetFingerprint": fingerprint,
		"paneHistoryBaselineEncoding": "tmux-pane-history-baseline-v1", "paneHistoryBaselineDispatchSeq": 6, "paneHistoryBaselineSha256": baselineHash,
		"peekCapabilityRevokedSeq": 8, "steeringGeneration": "0", "operatorInfluenced": false, "status": "ok",
		"capturedRange": map[string]any{"sessionTargetId": "target_reviewer", "start": startCursor, "end": endCursor, "startedAt": "2026-07-20T10:00:06Z", "endedAt": "2026-07-20T10:00:08Z"},
		"sentinel":      sentinel, "clientAttachmentAuditProof": auditProof, "clientAttachmentAuditProofSha256": auditHash,
		"turnClosureProof": turnClosureProof, "turnResult": turnResult, "turnResultEncoding": "slot-turn-result-jcs-v1", "turnResultSha256": projectionSHA256(canonicalJSON(t, turnResult)),
	})
	result["nodeId"] = projectionTestFormationID
	result["slotId"] = "slot_reviewer"
	return []map[string]any{revoked, result}
}

func replaceCanonicalCommandRecord(t *testing.T, input CanonicalRunReadInput, commandID string, raw []byte) CanonicalRunReadInput {
	t.Helper()
	for index, document := range input.Documents {
		if document.Role != CanonicalInputRoleSchema2CommandRecord {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(document.Bytes, &record); err == nil && record["commandId"] == commandID {
			input.Documents[index] = canonicalInputDocument(CanonicalInputRoleSchema2CommandRecord, raw)
			return input
		}
	}
	t.Fatalf("canonical input has no command record %s", commandID)
	return input
}

func schema2FormationEvent(runID string, sequence uint64, eventType string, data map[string]any) map[string]any {
	event := schema2Event(runID, sequence, eventType, data)
	delete(event, "missionId")
	delete(event, "beadId")
	return event
}

func schema2OpenDispatchCarries(t *testing.T, events []map[string]any) ([]any, []any) {
	t.Helper()
	var blocked, resumed []any
	for _, event := range events {
		switch event["type"] {
		case "run_blocked":
			blocked = event["data"].(map[string]any)["openDispatches"].([]any)
		case "run_resumed":
			resumed = event["data"].(map[string]any)["openDispatches"].([]any)
		}
	}
	if blocked == nil || resumed == nil {
		t.Fatalf("schema-2 lifecycle missing block/resume carries")
	}
	return blocked, resumed
}

func assertSchema2OpenDispatchOrderingPrecondition(t *testing.T, input CanonicalRunReadInput, dispatches []any) {
	t.Helper()
	graph := canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2GraphSnapshot).Bytes
	bindings := canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2PrivateBindings).Bytes
	if bytes.Index(graph, []byte(`id = "slot_reviewer"`)) >= bytes.Index(graph, []byte(`id = "slot_worker"`)) {
		t.Fatalf("graph slot declaration order no longer distinguishes reviewer before worker")
	}
	if bytes.Index(bindings, []byte(`bindingId = "binding_worker"`)) >= bytes.Index(bindings, []byte(`bindingId = "binding_reviewer"`)) {
		t.Fatalf("binding inventory order no longer distinguishes worker before reviewer")
	}
	if len(dispatches) != 2 || dispatches[0].(map[string]any)["bindingId"] != "binding_reviewer" || dispatches[1].(map[string]any)["bindingId"] != "binding_worker" {
		t.Fatalf("dispatchSeq order no longer differs from binding inventory: %+v", dispatches)
	}
}

func schema2SetOpenDispatchCarry(t *testing.T, events []map[string]any, eventType string, carry []any) {
	t.Helper()
	for _, event := range events {
		if event["type"] == eventType {
			event["data"].(map[string]any)["openDispatches"] = carry
			return
		}
	}
	t.Fatalf("schema-2 lifecycle missing %s", eventType)
}

func schema2SlotDispatchEvent(t *testing.T, sequence uint64, dispatchID, leaseID, slotID, agentID, bindingID, targetID, fingerprint, turnPhase string) map[string]any {
	t.Helper()
	promptHash := strings.Repeat("e", 64)
	fingerprintHash := projectionSHA256([]byte(fingerprint))
	barrier := map[string]any{
		"dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprintSha256": fingerprintHash,
		"attachmentAuditRegistrationSha256": strings.Repeat("1", 64), "promptSha256": promptHash, "monitorEvidenceSha256": strings.Repeat("2", 64),
	}
	ready := map[string]any{
		"targetFingerprintSha256": fingerprintHash, "acquisitionChallengeSha256": strings.Repeat("3", 64),
		"dispatchInputBarrierSha256": projectionSHA256(canonicalJSON(t, barrier)), "harnessReadyEvidenceSha256": strings.Repeat("4", 64),
	}
	baseline := map[string]any{
		"targetFingerprintSha256": fingerprintHash, "historyEpoch": "AQIDBAUGBwgJCgsMDQ4PEA", "offset": "0", "cols": 120, "rows": 40,
	}
	event := schema2FormationEvent(projectionTestRunID, sequence, "slot_dispatch", map[string]any{
		"dispatchId": dispatchID, "targetLeaseId": leaseID, "turnKey": "turn_" + slotID, "turnPhase": turnPhase,
		"turnInputs": map[string]any{"nodeStartedSeq": 3, "priorTurnResults": []any{}},
		"nodeId":     projectionTestFormationID, "attempt": 1, "slotId": slotID, "agentId": agentID, "harness": "codex",
		"bindingId": bindingID, "sessionTargetId": targetID, "targetFingerprint": fingerprint,
		"dispatchInputBarrierEncoding": "target-dispatch-input-barrier-v1", "dispatchInputBarrier": barrier,
		"dispatchInputBarrierSha256": projectionSHA256(canonicalJSON(t, barrier)),
		"targetReadyProofEncoding":   "target-ready-proof-v1", "targetReadyProof": ready, "targetReadyProofSha256": projectionSHA256(canonicalJSON(t, ready)),
		"paneHistoryBaselineEncoding": "tmux-pane-history-baseline-v1", "paneHistoryBaseline": baseline,
		"paneHistoryBaselineSha256": projectionSHA256(canonicalJSON(t, baseline)), "steeringGeneration": "0",
		"promptSha256": promptHash, "nativeAck": false, "recordedBeforeSend": true,
	})
	event["nodeId"] = projectionTestFormationID
	event["slotId"] = slotID
	return event
}

type schema2IdentityChange struct {
	runAuthorityID string
	commandID      string
	changeGraph    bool
	changeBindings bool
}

func schema2InputWithTwoPolicies(t *testing.T) CanonicalRunReadInput {
	t.Helper()
	return schema2ProjectionInput(t, true)
}

func assertSchema2SelectedPolicyLinkage(t *testing.T, input CanonicalRunReadInput, wantRev uint64) {
	t.Helper()
	var authority, command map[string]any
	if err := json.Unmarshal(canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2WorkspaceAuthority).Bytes, &authority); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2CommandRecord).Bytes, &command); err != nil {
		t.Fatal(err)
	}
	want := authority["admissionPolicyRef"]
	if !jsonBytesEqual(mustMarshalJSON(t, command["decisionAdmissionPolicyRef"]), mustMarshalJSON(t, want)) {
		t.Fatalf("command decision policy ref = %s, want exact authority selection %s", mustMarshalJSON(t, command["decisionAdmissionPolicyRef"]), mustMarshalJSON(t, want))
	}
	wantMap := want.(map[string]any)
	if wantMap["policyRev"] != float64(wantRev) {
		t.Fatalf("selected policy revision = %#v, want %d", wantMap["policyRev"], wantRev)
	}
	for _, event := range canonicalLedgerEvents(t, input) {
		if event["type"] != "run_started" && event["type"] != "run_activated" {
			continue
		}
		data := event["data"].(map[string]any)
		got := map[string]any{"policyRev": data["admissionPolicyRev"], "policySha256": data["admissionPolicySha256"]}
		if !jsonBytesEqual(mustMarshalJSON(t, got), mustMarshalJSON(t, want)) {
			t.Fatalf("%s policy ref = %s, want exact authority/command selection %s", event["type"], mustMarshalJSON(t, got), mustMarshalJSON(t, want))
		}
	}
}

func assertSchema2AuthorityHistory(t *testing.T, input CanonicalRunReadInput) {
	t.Helper()
	history := schema2AuthorityHistory(t, strings.Repeat("1", 64))
	current := canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2WorkspaceAuthority)
	wantCurrent := history.authorityGenerations[len(history.authorityGenerations)-1]
	if !bytes.Equal(current.Bytes, wantCurrent) {
		t.Fatalf("selected workspace authority is not the exact authenticated admission-allocation generation\nwant: %s\ngot:  %s", wantCurrent, current.Bytes)
	}
	policies := canonicalDocumentsByRole(input, CanonicalInputRoleSchema2AdmissionPolicy)
	if len(policies) != len(history.policies) {
		t.Fatalf("admission policy generations = %d, want %d", len(policies), len(history.policies))
	}
	for index := range history.policies {
		if !bytes.Equal(policies[index].Bytes, history.policies[index].Bytes) {
			t.Fatalf("admission policy generation %d changed\nwant: %s\ngot:  %s", index+1, history.policies[index].Bytes, policies[index].Bytes)
		}
	}
	for index, raw := range history.authorityGenerations {
		var generation map[string]any
		if err := json.Unmarshal(raw, &generation); err != nil {
			t.Fatal(err)
		}
		wantRev := uint64(index + 1)
		if generation["recordRev"] != float64(wantRev) {
			t.Fatalf("authority generation %d recordRev = %#v", wantRev, generation["recordRev"])
		}
		if index == 0 {
			if generation["priorGeneration"] != nil {
				t.Fatalf("initial authority priorGeneration = %#v, want null", generation["priorGeneration"])
			}
			continue
		}
		assertExactJSONValue(t, fmt.Sprintf("authority generation %d predecessor", wantRev), generation["priorGeneration"], map[string]any{
			"recordRev": wantRev - 1, "sha256": projectionSHA256(history.authorityGenerations[index-1]),
		})
	}
	assertExactJSONValue(t, "initial disabled authority state", decodeCanonicalObject(t, history.authorityGenerations[0]), map[string]any{
		"recordRev": 1, "priorGeneration": nil, "authoritySchema": 2,
		"workspaceAuthorityId": projectionTestWorkspaceID,
		"rootIdentityEncoding": "workspace-root-identity-v1", "workspaceRootIdentitySha256": strings.Repeat("1", 64),
		"nextWriterFence": 1, "nextAdmissionSeq": 1,
		"admissionPolicyRef": map[string]any{"policyRev": 1, "policySha256": history.policies[0].SHA256},
	})
	configured := decodeCanonicalObject(t, history.authorityGenerations[2])
	assertExactJSONValue(t, "configured authority policy ref", configured["admissionPolicyRef"], map[string]any{
		"policyRev": 2, "policySha256": history.policies[1].SHA256,
	})
	allocated := decodeCanonicalObject(t, history.authorityGenerations[3])
	if allocated["nextWriterFence"] != float64(2) || allocated["nextAdmissionSeq"] != float64(2) {
		t.Fatalf("allocated authority counters = writer %#v admission %#v, want 2/2", allocated["nextWriterFence"], allocated["nextAdmissionSeq"])
	}
}

func schema2InputWithIdentityChange(t *testing.T, base CanonicalRunReadInput, change schema2IdentityChange) CanonicalRunReadInput {
	t.Helper()
	input := cloneCanonicalInput(base)
	var bootstrap map[string]any
	if err := json.Unmarshal(canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2RunBootstrap).Bytes, &bootstrap); err != nil {
		t.Fatal(err)
	}
	events := canonicalLedgerEvents(t, input)
	started := events[0]["data"].(map[string]any)

	if change.runAuthorityID != "" {
		bootstrap["runAuthorityId"] = change.runAuthorityID
		started["runAuthorityId"] = change.runAuthorityID
	}
	if change.changeGraph {
		graph := append([]byte(nil), canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2GraphSnapshot).Bytes...)
		graph = append(graph, []byte("\n# coherent generation fixture\n")...)
		graphHash := projectionSHA256(graph)
		input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2GraphSnapshot, graph)
		bootstrap["graphSnapshotSha256"] = graphHash
		started["graphSnapshotSha256"] = graphHash
	}
	if change.changeBindings {
		bindings := append([]byte(nil), canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2PrivateBindings).Bytes...)
		bindings = append(bindings, []byte("\n# coherent generation fixture\n")...)
		bindingsHash := projectionSHA256(bindings)
		input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2PrivateBindings, bindings)
		bootstrap["privateBindingsSha256"] = bindingsHash
		started["privateBindingsSha256"] = bindingsHash
	}
	if change.commandID != "" {
		started["admissionCommandId"] = change.commandID
		policyRev, policyHash := selectedAdmissionPolicy(t, input)
		input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2CommandRecord, schema2AdmissionCommandRecord(t, change.commandID, policyRev, policyHash))
	}
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2RunBootstrap, canonicalJSON(t, bootstrap))
	return replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
}

func schema2AdmissionCommandRecord(t *testing.T, commandID string, policyRev uint64, policyHash string) []byte {
	t.Helper()
	history := schema2CommandHistoryForPayload(t, commandID, "start", "applied", canonicalCommandPayload("start", projectionTestRunID), schema2CommandOutcome{
		runID: projectionTestRunID, effectSeq: 1, admittedWriterFence: 1, stateWriterFence: 1, outcomeWriterFence: 1,
		decisionPolicyRef: map[string]any{"policyRev": policyRev, "policySha256": policyHash},
	})
	return history.terminal
}

func schema2CommandRecord(t *testing.T, commandID, kind, state, runID string) []byte {
	t.Helper()
	return canonicalCommandHistory(t, commandID, kind, state, runID).terminal
}

type schema2CommandHistory struct {
	pending  []byte
	terminal []byte
}

type schema2CommandOutcome struct {
	runID               string
	effectSeq           uint64
	rejectionCode       string
	admittedWriterFence uint64
	stateWriterFence    uint64
	outcomeWriterFence  uint64
	decisionPolicyRef   any
}

func canonicalCommandHistory(t *testing.T, commandID, kind, state, runID string) schema2CommandHistory {
	t.Helper()
	decisionPolicyRef := any(nil)
	if kind == "start" {
		decisionPolicyRef = map[string]any{"policyRev": 2, "policySha256": strings.Repeat("a", 64)}
	}
	return schema2CommandHistoryForPayload(t, commandID, kind, state, canonicalCommandPayload(kind, runID), schema2CommandOutcome{
		runID: runID, effectSeq: 7, rejectionCode: "fixture_rejected",
		admittedWriterFence: 9, stateWriterFence: 9, outcomeWriterFence: 9,
		decisionPolicyRef: decisionPolicyRef,
	})
}

func schema2CommandHistoryForPayload(t *testing.T, commandID, kind, state string, payload map[string]any, outcome schema2CommandOutcome) schema2CommandHistory {
	t.Helper()
	payloadHash := projectionSHA256(canonicalJSON(t, payload))
	admittedWriterFence := strconv.FormatUint(outcome.admittedWriterFence, 10)
	stateWriterFence := strconv.FormatUint(outcome.stateWriterFence, 10)
	outcomeWriterFence := strconv.FormatUint(outcome.outcomeWriterFence, 10)
	pending := canonicalJSON(t, map[string]any{
		"commandSchema": 1, "recordRev": 1, "priorGeneration": nil,
		"commandEncoding": "run-command-jcs-v1", "commandId": commandID, "commandKind": kind,
		"commandPayload": payload, "commandPayloadSha256": payloadHash,
		"admittedWriterFence": admittedWriterFence, "stateWriterFence": admittedWriterFence, "state": "pending",
	})
	terminal := map[string]any{
		"commandSchema": 1, "recordRev": 2,
		"priorGeneration": map[string]any{"recordRev": 1, "sha256": projectionSHA256(pending)},
		"commandEncoding": "run-command-jcs-v1", "commandId": commandID, "commandKind": kind,
		"commandPayload": payload, "commandPayloadSha256": payloadHash,
		"admittedWriterFence": admittedWriterFence, "stateWriterFence": stateWriterFence, "state": state,
		"outcomeWriterFence": outcomeWriterFence, "decisionAdmissionPolicyRef": outcome.decisionPolicyRef,
	}
	if state == "applied" {
		terminal["runId"] = outcome.runID
		terminal["effectSeq"] = outcome.effectSeq
	} else {
		terminal["rejectionCode"] = outcome.rejectionCode
	}
	return schema2CommandHistory{pending: pending, terminal: canonicalJSON(t, terminal)}
}

func assertAuthenticatedCommandHistory(t *testing.T, history schema2CommandHistory) {
	t.Helper()
	pending := decodeCanonicalObject(t, history.pending)
	terminal := decodeCanonicalObject(t, history.terminal)
	assertExactJSONValue(t, "pending command generation identity", map[string]any{
		"recordRev": pending["recordRev"], "priorGeneration": pending["priorGeneration"], "state": pending["state"],
	}, map[string]any{"recordRev": 1, "priorGeneration": nil, "state": "pending"})
	for _, forbidden := range []string{"runId", "effectSeq", "rejectionCode", "outcomeWriterFence", "decisionAdmissionPolicyRef"} {
		if _, exists := pending[forbidden]; exists {
			t.Fatalf("pending command generation contains terminal member %q: %s", forbidden, history.pending)
		}
	}
	assertExactJSONValue(t, "terminal command predecessor", terminal["priorGeneration"], map[string]any{
		"recordRev": 1, "sha256": projectionSHA256(history.pending),
	})
	if terminal["recordRev"] != float64(2) || terminal["state"] == "pending" {
		t.Fatalf("terminal command generation identity/state = %#v/%#v, want rev2 terminal", terminal["recordRev"], terminal["state"])
	}
	for _, member := range []string{"commandEncoding", "commandId", "commandKind", "commandPayload", "commandPayloadSha256", "admittedWriterFence"} {
		if !jsonBytesEqual(mustMarshalJSON(t, terminal[member]), mustMarshalJSON(t, pending[member])) {
			t.Fatalf("terminal command changed immutable member %q\npending:  %s\nterminal: %s", member, history.pending, history.terminal)
		}
	}
}

func decodeCanonicalObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func selectedAdmissionPolicy(t *testing.T, input CanonicalRunReadInput) (uint64, string) {
	t.Helper()
	var authority map[string]any
	if err := json.Unmarshal(canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2WorkspaceAuthority).Bytes, &authority); err != nil {
		t.Fatal(err)
	}
	ref := authority["admissionPolicyRef"].(map[string]any)
	return uint64(ref["policyRev"].(float64)), ref["policySha256"].(string)
}

func canonicalCommandPayload(kind, runID string) map[string]any {
	base := map[string]any{
		"kind": kind, "authoritySchema": 2, "actor": "human:test", "workspaceAuthorityId": projectionTestWorkspaceID,
	}
	switch kind {
	case "start":
		base["boardId"] = projectionTestBoardID
		base["runRoot"] = map[string]any{"kind": "mission", "nodeId": projectionTestMissionID}
		base["expectedBoardRev"] = 7
		base["expectedBoardETag"] = strings.Repeat("b", 64)
		base["limits"] = map[string]any{"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false}
	case "resume":
		base["runId"] = runID
		base["blockedSeq"] = 3
		base["resumeMode"] = "reattach"
		base["reason"] = "continue"
	case "cancel":
		base["runId"] = runID
		base["expectedLastSeq"] = 3
		base["reason"] = "stop"
	case "verdict":
		base["runId"] = runID
		base["gateId"] = projectionTestGateID
		base["requestedSeq"] = 3
		base["verdict"] = "pass"
		base["reason"] = "approved"
	}
	return base
}

func canonicalCommandInput(t *testing.T, commandID, kind, state string) CanonicalCommandReadInput {
	t.Helper()
	record := schema2CommandRecord(t, commandID, kind, state, projectionTestRunID)
	var decoded map[string]any
	if err := json.Unmarshal(record, &decoded); err != nil {
		t.Fatal(err)
	}
	return CanonicalCommandReadInput{
		Source: CanonicalRunSourceSchema2,
		Submitted: SubmittedCommandIdentity{
			CommandID: commandID, CommandKind: kind, CommandPayloadSHA256: decoded["commandPayloadSha256"].(string),
		},
		Record: record,
	}
}

func schema1SafeRegistryCases() []schema1SafeRegistryCase {
	inputRef := map[string]any{
		"edgeId": "edge_root_work", "fromNodeId": projectionTestMissionID, "fromPortId": "out",
		"toPortId": "port_in", "outputSeq": 1,
		"ref": "/private/input", "text": "private input", "reportRef": "/private/report", "artifactRef": "/private/artifact",
	}
	tests := []schema1SafeRegistryCase{
		{
			eventType: RunEventStarted,
			rawData: map[string]any{
				"boardSlug": projectionTestBoardSlug, "boardRev": 7, "missionId": projectionTestMissionID,
				"beadId": "ctx-7i1.1", "limits": map[string]any{"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false},
			},
			privateKeys: []string{"boardPath", "snapshot", "bindingsSnapshot", "objective"},
		},
		{
			eventType: RunEventResumed,
			rawData: map[string]any{
				"resumedFromSeq": 2, "resumedBy": "human:test", "resumeMode": "reattach", "reason": "continue", "openDispatches": []any{},
			},
		},
		{eventType: RunEventNodeWaiting, rawData: map[string]any{"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"}}},
		{
			eventType:   RunEventNodeStarted,
			rawData:     map[string]any{"nodeKind": "formation", "inputRefs": []any{inputRef}, "reason": "initial", "brief": map[string]any{"goal": "private"}},
			privateKeys: []string{"brief", "ref", "text", "reportRef", "artifactRef"},
		},
		{
			eventType: RunEventOrchestrationTeam,
			rawData: map[string]any{
				"mode": "orchestrated", "controllerSlot": "slot_worker",
				"controller": map[string]any{"slotId": "slot_worker", "label": "Worker", "agentId": "worker", "harness": "codex", "sessionStem": "private", "sessionRef": "private"},
				"workers":    []any{map[string]any{"slotId": "slot_reviewer", "label": "Reviewer", "agentId": "reviewer", "harness": "codex", "sessionStem": "private", "sessionRef": "private"}},
				"socket":     "/private/socket", "cwd": "/private/cwd",
			},
			privateKeys: []string{"socket", "cwd", "sessionStem", "sessionRef"},
		},
		{
			eventType: RunEventPeerPlane,
			rawData: map[string]any{
				"mode": "peer", "peers": []any{map[string]any{"slotId": "slot_worker", "label": "Worker", "agentId": "worker", "harness": "codex", "sessionStem": "private", "sessionRef": "private"}},
				"path": "/private", "socket": "/private/socket", "cwd": "/private/cwd",
			},
			privateKeys: []string{"path", "socket", "cwd", "sessionStem", "sessionRef"},
		},
		{
			eventType: RunEventSlotDispatch,
			rawData: map[string]any{
				"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "agentId": "worker", "harness": "codex",
				"phase": "solo", "promptSha256": strings.Repeat("a", 64), "nativeAck": true, "recordedBeforeSend": true,
				"sessionStem": "private", "sessionRef": "private", "promptRef": "private",
			},
			privateKeys: []string{"sessionStem", "sessionRef", "promptRef"},
		},
		{
			eventType: RunEventAdapterSend,
			rawData: map[string]any{
				"adapter": "tmux", "dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "phase": "solo",
				"socketSha256": strings.Repeat("b", 64), "promptSha256": strings.Repeat("a", 64), "sent": true, "sessionRef": "private",
			},
			privateKeys: []string{"sessionRef"},
		},
		{
			eventType: RunEventSlotResult,
			rawData: map[string]any{
				"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "status": "ok",
				"sentinel": map[string]any{"runId": projectionTestRunID, "status": "done", "artifact": "/private/artifact"},
			},
			privateKeys: []string{"artifact"},
		},
		{
			eventType: RunEventNodeOutput,
			rawData: map[string]any{
				"status": "done", "text": "done", "reason": "completed", "reportRef": "/private/report",
				"outputs": map[string]any{"port_out": map[string]any{"text": "done", "ref": "/private", "reportRef": "/private/report", "artifactRef": "/private/artifact"}},
			},
			privateKeys: []string{"reportRef", "ref", "artifactRef"},
		},
		{
			eventType:   RunEventGateEvaluating,
			rawData:     map[string]any{"kinds": []string{"human"}, "criterion": "Approve", "inputRef": inputRef, "judgeChain": []string{}},
			privateKeys: []string{"ref", "text", "reportRef", "artifactRef"},
		},
		{
			eventType: RunEventGateVerdict,
			rawData: map[string]any{
				"verdict": "pass", "perKind": map[string]any{"human": "pass"}, "routePort": "pass", "routedEdges": []string{}, "reason": "approved", "inputRef": inputRef,
			},
			privateKeys: []string{"ref", "text", "reportRef", "artifactRef"},
		},
		{eventType: RunEventVerificationVerdict, rawData: map[string]any{"verificationId": "verification-1", "verdict": "pass"}},
		{eventType: RunEventEscalationRaised, rawData: map[string]any{"trigger": "sentinel", "severity": "needs-attention", "reason": "review", "source": "agent", "nodeId": projectionTestFormationID, "gateId": "", "blocks": false}},
		{
			eventType: RunEventHumanInputRequested,
			rawData: map[string]any{
				"gateId": projectionTestGateID, "nodeId": projectionTestFormationID, "choices": []string{"pass", "fail"}, "requestedBy": "agent:test", "inputRef": inputRef,
				"codeVerdict": "pass", "codeReason": "checks pass", "codePerKind": map[string]any{"code": "pass"}, "timeoutSeconds": 300, "prompt": "private prompt",
			},
			privateKeys: []string{"prompt", "ref", "text", "reportRef", "artifactRef"},
		},
		{eventType: RunEventHumanVerdictRecorded, rawData: map[string]any{"gateId": projectionTestGateID, "nodeId": projectionTestFormationID, "verdict": "pass", "reason": "approved", "requestedSeq": 2, "decidedBy": "human:test"}},
		{eventType: RunEventError, rawData: map[string]any{"code": "fixture", "message": "safe message", "reason": "safe reason", "boundary": "schema", "nodeId": projectionTestFormationID, "gateId": "", "slotId": "", "dispatchId": "", "recoverable": true, "relatedSeq": 1}},
		{eventType: RunEventBlocked, rawData: map[string]any{"reason": "blocked", "code": "fixture", "boundary": "engine", "blockedNodeId": projectionTestFormationID, "blockedGateId": "", "waitingNodes": []string{}, "recoverable": true, "resumeAllowed": true, "resumePolicy": "explicit", "openDispatches": []any{}, "nextEpoch": 1}},
		{eventType: RunEventCanceled, rawData: map[string]any{"reason": "stop", "requestedBy": "human:test", "softInterruptedSlots": []string{}, "final": true}},
		{eventType: RunEventFailed, rawData: map[string]any{"code": "fixture", "reason": "failed", "boundary": "engine", "recoverable": false, "relatedSeq": 1, "final": true}},
		{eventType: RunEventSucceeded, rawData: map[string]any{"final": true, "mode": "mission", "formationId": "", "missionId": projectionTestMissionID, "reason": "done", "summaryRef": "/private/summary", "outputRefs": []string{"/private/output"}, "artifactRefs": []string{"/private/artifact"}}, privateKeys: []string{"summaryRef", "outputRefs", "artifactRefs"}},
	}
	for i := range tests {
		tests[i].publicData = schema1ExpectedPublicData(tests[i])
	}
	return tests
}

func schema1ExpectedPublicData(test schema1SafeRegistryCase) map[string]any {
	inputRef := map[string]any{
		"edgeId": "edge_root_work", "fromNodeId": projectionTestMissionID, "fromPortId": "out",
		"toPortId": "port_in", "outputSeq": 1,
	}
	switch test.eventType {
	case RunEventStarted:
		return map[string]any{
			"boardSlug": projectionTestBoardSlug, "boardRev": 7, "missionId": projectionTestMissionID,
			"beadId": "ctx-7i1.1", "limits": map[string]any{"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false},
		}
	case RunEventNodeStarted:
		return map[string]any{"nodeKind": "formation", "inputRefs": []any{inputRef}, "reason": "initial"}
	case RunEventOrchestrationTeam:
		return map[string]any{
			"mode": "orchestrated", "controllerSlot": "slot_worker",
			"controller": map[string]any{"slotId": "slot_worker", "label": "Worker", "agentId": "worker", "harness": "codex"},
			"workers":    []any{map[string]any{"slotId": "slot_reviewer", "label": "Reviewer", "agentId": "reviewer", "harness": "codex"}},
		}
	case RunEventPeerPlane:
		return map[string]any{"mode": "peer", "peers": []any{map[string]any{"slotId": "slot_worker", "label": "Worker", "agentId": "worker", "harness": "codex"}}}
	case RunEventSlotDispatch:
		return map[string]any{
			"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "agentId": "worker", "harness": "codex",
			"phase": "solo", "promptSha256": strings.Repeat("a", 64), "nativeAck": true, "recordedBeforeSend": true,
		}
	case RunEventAdapterSend:
		return map[string]any{
			"adapter": "tmux", "dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "phase": "solo",
			"socketSha256": strings.Repeat("b", 64), "promptSha256": strings.Repeat("a", 64), "sent": true,
		}
	case RunEventSlotResult:
		return map[string]any{
			"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "status": "ok",
			"sentinel": map[string]any{"runId": projectionTestRunID, "status": "done"},
		}
	case RunEventNodeOutput:
		return map[string]any{
			"status": "done", "text": "done", "reason": "completed",
			"outputs": map[string]any{"port_out": map[string]any{"text": "done"}},
		}
	case RunEventGateEvaluating:
		return map[string]any{"kinds": []string{"human"}, "criterion": "Approve", "inputRef": inputRef, "judgeChain": []string{}}
	case RunEventGateVerdict:
		return map[string]any{"verdict": "pass", "perKind": map[string]any{"human": "pass"}, "routePort": "pass", "routedEdges": []string{}, "reason": "approved", "inputRef": inputRef}
	case RunEventHumanInputRequested:
		return map[string]any{
			"gateId": projectionTestGateID, "nodeId": projectionTestFormationID, "choices": []string{"pass", "fail"}, "requestedBy": "agent:test", "inputRef": inputRef,
			"codeVerdict": "pass", "codeReason": "checks pass", "codePerKind": map[string]any{"code": "pass"}, "timeoutSeconds": 300,
		}
	case RunEventSucceeded:
		return map[string]any{"final": true, "mode": "mission", "formationId": "", "missionId": projectionTestMissionID, "reason": "done"}
	default:
		return cloneAny(test.rawData).(map[string]any)
	}
}

func schema1RegistryFixture(t *testing.T, test schema1SafeRegistryCase) (CanonicalRunReadInput, uint64) {
	t.Helper()
	if test.eventType == RunEventStarted {
		started := schema1StartedEvent(projectionTestRunID)
		data := started["data"].(map[string]any)
		for key, value := range test.rawData {
			data[key] = cloneAny(value)
		}
		return schema1ProjectionInput(t, started), 1
	}
	events := []map[string]any{schema1StartedEvent(projectionTestRunID)}
	appendEvent := func(event map[string]any) {
		event["seq"] = uint64(len(events) + 1)
		events = append(events, event)
	}
	switch test.eventType {
	case RunEventResumed:
		appendEvent(schema1BlockedEvent(projectionTestRunID, 2, true, []any{}))
	case RunEventSlotDispatch:
		appendEvent(schema1NodeStartedEvent(projectionTestRunID, 2))
	case RunEventAdapterSend:
		appendEvent(schema1NodeStartedEvent(projectionTestRunID, 2))
		appendEvent(schema1Event(projectionTestRunID, 3, RunEventSlotDispatch, map[string]any{
			"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "agentId": "worker", "harness": "codex", "phase": "solo",
			"promptSha256": strings.Repeat("a", 64), "nativeAck": true, "recordedBeforeSend": true,
		}))
	case RunEventSlotResult:
		appendEvent(schema1NodeStartedEvent(projectionTestRunID, 2))
		appendEvent(schema1Event(projectionTestRunID, 3, RunEventSlotDispatch, map[string]any{
			"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "agentId": "worker", "harness": "codex", "phase": "solo",
			"promptSha256": strings.Repeat("a", 64), "nativeAck": true, "recordedBeforeSend": true,
		}))
	case RunEventNodeOutput:
		appendEvent(schema1NodeStartedEvent(projectionTestRunID, 2))
	case RunEventGateVerdict:
		appendEvent(schema1GateEvaluatingEvent(projectionTestRunID, 2))
	case RunEventHumanInputRequested:
		appendEvent(schema1GateEvaluatingEvent(projectionTestRunID, 2))
	case RunEventHumanVerdictRecorded:
		appendEvent(schema1GateEvaluatingEvent(projectionTestRunID, 2))
		appendEvent(schema1Event(projectionTestRunID, 3, RunEventHumanInputRequested, map[string]any{
			"gateId": projectionTestGateID, "nodeId": projectionTestFormationID, "choices": []string{"pass", "fail"}, "requestedBy": "agent:test",
			"inputRef":    map[string]any{"edgeId": "edge_work_gate", "fromNodeId": projectionTestFormationID, "fromPortId": "port_out", "toPortId": "in", "outputSeq": 1},
			"codeVerdict": "pass", "codeReason": "checks pass", "codePerKind": map[string]any{"code": "pass"}, "timeoutSeconds": 300,
		}))
	}
	target := schema1Event(projectionTestRunID, uint64(len(events)+1), test.eventType, test.rawData)
	appendEvent(target)
	return schema1ProjectionInput(t, events...), uint64(len(events))
}

func schema2SafeEventTypes() []struct {
	source  int
	literal string
	typeOf  reflect.Type
} {
	return []struct {
		source  int
		literal string
		typeOf  reflect.Type
	}{
		{2, "run_started", reflect.TypeOf(SafeSchema2RunStartedEvent{})},
		{2, "run_activated", reflect.TypeOf(SafeSchema2RunActivatedEvent{})},
		{2, "run_resumed", reflect.TypeOf(SafeSchema2RunResumedEvent{})},
		{2, "node_waiting", reflect.TypeOf(SafeSchema2NodeWaitingEvent{})},
		{2, "node_input_ignored", reflect.TypeOf(SafeSchema2NodeInputIgnoredEvent{})},
		{2, "node_started", reflect.TypeOf(SafeSchema2NodeStartedEvent{})},
		{2, "slot_binding_observed", reflect.TypeOf(SafeSchema2SlotBindingObservedEvent{})},
		{2, "slot_dispatch", reflect.TypeOf(SafeSchema2SlotDispatchEvent{})},
		{2, "slot_peek_capability_issued", reflect.TypeOf(SafeSchema2SlotPeekCapabilityIssuedEvent{})},
		{2, "slot_steering_started", reflect.TypeOf(SafeSchema2SlotSteeringStartedEvent{})},
		{2, "slot_steering_ended", reflect.TypeOf(SafeSchema2SlotSteeringEndedEvent{})},
		{2, "slot_peek_capability_revoked", reflect.TypeOf(SafeSchema2SlotPeekCapabilityRevokedEvent{})},
		{2, "slot_reconciliation_interrupt", reflect.TypeOf(SafeSchema2SlotReconciliationInterruptEvent{})},
		{2, "slot_reconciliation_interrupt_outcome", reflect.TypeOf(SafeSchema2SlotReconciliationInterruptOutcomeEvent{})},
		{2, "slot_result", reflect.TypeOf(SafeSchema2SlotResultEvent{})},
		{2, "formation_result", reflect.TypeOf(SafeSchema2FormationResultEvent{})},
		{2, "tool_dispatch", reflect.TypeOf(SafeSchema2ToolDispatchEvent{})},
		{2, "tool_process_launch", reflect.TypeOf(SafeSchema2ToolProcessLaunchEvent{})},
		{2, "tool_result", reflect.TypeOf(SafeSchema2ToolResultEvent{})},
		{2, "node_output", reflect.TypeOf(SafeSchema2NodeOutputEvent{})},
		{2, "gate_evaluating", reflect.TypeOf(SafeSchema2GateEvaluatingEvent{})},
		{2, "gate_kind_result", reflect.TypeOf(SafeSchema2GateKindResultEvent{})},
		{2, "judge_result", reflect.TypeOf(SafeSchema2JudgeResultEvent{})},
		{2, "judge_attempt_failed", reflect.TypeOf(SafeSchema2JudgeAttemptFailedEvent{})},
		{2, "gate_verdict", reflect.TypeOf(SafeSchema2GateVerdictEvent{})},
		{2, "artifact_attached", reflect.TypeOf(SafeSchema2ArtifactAttachedEvent{})},
		{2, "artifact_observed", reflect.TypeOf(SafeSchema2ArtifactObservedEvent{})},
		{2, "escalation_raised", reflect.TypeOf(SafeSchema2EscalationRaisedEvent{})},
		{2, "human_input_requested", reflect.TypeOf(SafeSchema2HumanInputRequestedEvent{})},
		{2, "human_verdict_recorded", reflect.TypeOf(SafeSchema2HumanVerdictRecordedEvent{})},
		{2, "error", reflect.TypeOf(SafeSchema2ErrorEvent{})},
		{2, "run_blocked", reflect.TypeOf(SafeSchema2RunBlockedEvent{})},
		{2, "run_cancel_requested", reflect.TypeOf(SafeSchema2RunCancelRequestedEvent{})},
		{2, "run_canceled", reflect.TypeOf(SafeSchema2RunCanceledEvent{})},
		{2, "run_failure_reconciliation_started", reflect.TypeOf(SafeSchema2RunFailureReconciliationStartedEvent{})},
		{2, "run_failed", reflect.TypeOf(SafeSchema2RunFailedEvent{})},
		{2, "run_succeeded", reflect.TypeOf(SafeSchema2RunSucceededEvent{})},
	}
}

func schema1SafeEventTypes() []struct {
	source  int
	literal string
	typeOf  reflect.Type
} {
	return []struct {
		source  int
		literal string
		typeOf  reflect.Type
	}{
		{1, "run_started", reflect.TypeOf(SafeSchema1RunStartedEvent{})},
		{1, "run_resumed", reflect.TypeOf(SafeSchema1RunResumedEvent{})},
		{1, "node_waiting", reflect.TypeOf(SafeSchema1NodeWaitingEvent{})},
		{1, "node_started", reflect.TypeOf(SafeSchema1NodeStartedEvent{})},
		{1, "orchestration_team", reflect.TypeOf(SafeSchema1OrchestrationTeamEvent{})},
		{1, "peer_plane", reflect.TypeOf(SafeSchema1PeerPlaneEvent{})},
		{1, "slot_dispatch", reflect.TypeOf(SafeSchema1SlotDispatchEvent{})},
		{1, "adapter_send", reflect.TypeOf(SafeSchema1AdapterSendEvent{})},
		{1, "slot_result", reflect.TypeOf(SafeSchema1SlotResultEvent{})},
		{1, "node_output", reflect.TypeOf(SafeSchema1NodeOutputEvent{})},
		{1, "gate_evaluating", reflect.TypeOf(SafeSchema1GateEvaluatingEvent{})},
		{1, "gate_verdict", reflect.TypeOf(SafeSchema1GateVerdictEvent{})},
		{1, "verification_verdict", reflect.TypeOf(SafeSchema1VerificationVerdictEvent{})},
		{1, "escalation_raised", reflect.TypeOf(SafeSchema1EscalationRaisedEvent{})},
		{1, "human_input_requested", reflect.TypeOf(SafeSchema1HumanInputRequestedEvent{})},
		{1, "human_verdict_recorded", reflect.TypeOf(SafeSchema1HumanVerdictRecordedEvent{})},
		{1, "error", reflect.TypeOf(SafeSchema1ErrorEvent{})},
		{1, "run_blocked", reflect.TypeOf(SafeSchema1RunBlockedEvent{})},
		{1, "run_canceled", reflect.TypeOf(SafeSchema1RunCanceledEvent{})},
		{1, "run_failed", reflect.TypeOf(SafeSchema1RunFailedEvent{})},
		{1, "run_succeeded", reflect.TypeOf(SafeSchema1RunSucceededEvent{})},
	}
}

func isSchema1OnlyEvent(eventType string) bool {
	switch eventType {
	case "orchestration_team", "peer_plane", "adapter_send", "verification_verdict":
		return true
	default:
		return false
	}
}

func isSharedSafeEvent(eventType string) bool {
	switch eventType {
	case "run_started", "run_resumed", "node_waiting", "node_started", "slot_dispatch", "slot_result", "node_output", "gate_evaluating", "gate_verdict", "escalation_raised", "human_input_requested", "human_verdict_recorded", "error", "run_blocked", "run_canceled", "run_failed", "run_succeeded":
		return true
	default:
		return false
	}
}

func mustProjectCanonicalFixture(t *testing.T, input CanonicalRunReadInput) CanonicalRunProjection {
	t.Helper()
	projection, err := ProjectCanonicalRun(input)
	if err != nil {
		t.Fatalf("project canonical fixture: %v", err)
	}
	return projection
}

func mustProjectEventPage(t *testing.T, projection CanonicalRunProjection, since uint64, limit int) RunEventPage {
	t.Helper()
	page, err := ProjectRunEventPage(projection, since, limit)
	if err != nil {
		t.Fatalf("project event page: %v", err)
	}
	return page
}

func findProjectedNode(t *testing.T, view RunView, nodeID string) RunNodeView {
	t.Helper()
	for _, node := range view.Nodes {
		if node.NodeID == nodeID {
			return node
		}
	}
	t.Fatalf("node %q absent from projection: %+v", nodeID, view.Nodes)
	return RunNodeView{}
}

func findProjectedGate(t *testing.T, view RunView, gateID string) RunGateView {
	t.Helper()
	for _, gate := range view.Gates {
		if gate.GateID == gateID {
			return gate
		}
	}
	t.Fatalf("gate %q absent from projection: %+v", gateID, view.Gates)
	return RunGateView{}
}

func findProjectedAttempt(t *testing.T, view RunView, nodeID string, attemptNumber uint64) RunAttemptView {
	t.Helper()
	for _, attempt := range view.Attempts {
		if attempt.NodeID == nodeID && attempt.Attempt == attemptNumber {
			return attempt
		}
	}
	t.Fatalf("attempt %s/%d absent from projection: %+v", nodeID, attemptNumber, view.Attempts)
	return RunAttemptView{}
}

func findProjectedOutput(t *testing.T, view RunView, nodeID string, attemptNumber uint64, portID string) RunOutputView {
	t.Helper()
	for _, output := range view.Outputs {
		if output.NodeID == nodeID && output.Attempt == attemptNumber && output.PortID == portID {
			return output
		}
	}
	t.Fatalf("output %s/%d/%s absent from projection: %+v", nodeID, attemptNumber, portID, view.Outputs)
	return RunOutputView{}
}

func assertExactJSONValue(t *testing.T, label string, got, want any) {
	t.Helper()
	gotRaw := mustMarshalJSON(t, got)
	wantRaw := mustMarshalJSON(t, want)
	if !jsonBytesEqual(gotRaw, wantRaw) {
		t.Fatalf("%s = %s, want exact %s", label, gotRaw, wantRaw)
	}
}

func projectedNodeIDs(view RunView) []string {
	ids := make([]string, len(view.Nodes))
	for index, node := range view.Nodes {
		ids[index] = node.NodeID
	}
	return ids
}

func projectedArtifactIDs(view RunView) []string {
	ids := make([]string, len(view.Artifacts))
	for index, artifact := range view.Artifacts {
		var identity struct {
			ArtifactID string `json:"artifactId"`
		}
		_ = json.Unmarshal(mustMarshalJSONNoTest(artifact), &identity)
		ids[index] = identity.ArtifactID
	}
	return ids
}

func mustMarshalJSONNoTest(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func blockSequences(view RunView) []uint64 {
	sequences := make([]uint64, len(view.Blocks))
	for index, block := range view.Blocks {
		sequences[index] = block.Seq
	}
	return sequences
}

func escalationSequences(view RunView) []uint64 {
	sequences := make([]uint64, len(view.Escalations))
	for index, escalation := range view.Escalations {
		sequences[index] = escalation.Seq
	}
	return sequences
}

func assertStringOrder(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func assertUint64Order(t *testing.T, label string, got, want []uint64) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func assertAttemptOrder(t *testing.T, view RunView, want []string) {
	t.Helper()
	got := make([]string, len(view.Attempts))
	for index, attempt := range view.Attempts {
		got[index] = fmt.Sprintf("%s/%d", attempt.NodeID, attempt.Attempt)
	}
	assertStringOrder(t, "attempt order", got, want)
}

func assertGateOrder(t *testing.T, view RunView, want []string) {
	t.Helper()
	got := make([]string, len(view.Gates))
	for index, gate := range view.Gates {
		got[index] = fmt.Sprintf("%s/%d", gate.GateID, gate.Attempt)
	}
	assertStringOrder(t, "gate order", got, want)
}

func assertOutputOrder(t *testing.T, view RunView, want []string) {
	t.Helper()
	got := make([]string, len(view.Outputs))
	for index, output := range view.Outputs {
		got[index] = fmt.Sprintf("%s/%d/%s", output.NodeID, output.Attempt, output.PortID)
	}
	assertStringOrder(t, "output order", got, want)
}

func assertSchema2SessionProjection(t *testing.T, view RunView, capabilityState string) {
	t.Helper()
	if len(view.Sessions) != 2 {
		t.Fatalf("schema-2 sessions = %d, want the complete two-slot session set", len(view.Sessions))
	}
	expected := []struct {
		bindingID, slotID, dispatchID, leaseID, targetID, fingerprint, lineage, capabilityState string
	}{
		{"binding_reviewer", "slot_reviewer", "dsp_01KXNP6VY3227H78329V52CKF9", "lease_01KXNP6VY3227H78329V52CKF9", "target_reviewer", strings.Repeat("b", 64), strings.Repeat("d", 64), "none"},
		{"binding_worker", "slot_worker", "dsp_01KXNP6VY3227H78329V52CKF8", "lease_01KXNP6VY3227H78329V52CKF8", "target_worker", strings.Repeat("a", 64), strings.Repeat("c", 64), capabilityState},
	}
	for index, want := range expected {
		baseline := map[string]any{
			"targetFingerprintSha256": projectionSHA256([]byte(want.fingerprint)), "historyEpoch": "AQIDBAUGBwgJCgsMDQ4PEA", "offset": "0", "cols": 120, "rows": 40,
		}
		assertExactJSONValue(t, fmt.Sprintf("session[%d]", index), view.Sessions[index], map[string]any{
			"bindingId": want.bindingID, "nodeId": projectionTestFormationID, "attempt": 1, "slotId": want.slotID,
			"dispatchId": want.dispatchID, "targetLeaseId": want.leaseID, "sessionTargetId": want.targetID,
			"bindingHealth": "runnable", "sessionLineageSha256": want.lineage,
			"targetFingerprintSha256": projectionSHA256([]byte(want.fingerprint)),
			"baseline":                map[string]any{"encoding": "tmux-pane-history-baseline-v1", "sha256": projectionSHA256(canonicalJSON(t, baseline)), "state": "valid"},
			"attachment":              map[string]any{"state": "accounted"}, "occupancy": map[string]any{"state": "active"},
			"peekCapability": map[string]any{"state": want.capabilityState, "issuedSeq": 0, "generation": "0"},
			"steering":       map[string]any{"state": "closed", "generation": "0"}, "operatorInfluenced": false,
		})
	}
	refs := []any{
		map[string]any{"bindingId": "binding_reviewer", "nodeId": projectionTestFormationID, "attempt": 1, "slotId": "slot_reviewer"},
		map[string]any{"bindingId": "binding_worker", "nodeId": projectionTestFormationID, "attempt": 1, "slotId": "slot_worker"},
	}
	node := findProjectedNode(t, view, projectionTestFormationID)
	attempt := findProjectedAttempt(t, view, projectionTestFormationID, 1)
	assertExactJSONValue(t, "node session refs", node.Sessions, refs)
	assertExactJSONValue(t, "attempt slot refs", attempt.Slots, refs)
}

func requireCommandReceiptMatrixError(t *testing.T, input CanonicalCommandReadInput) {
	t.Helper()
	if receipt, err := ProjectCommandReceipt(input); err == nil {
		t.Fatalf("invalid command record returned receipt: %#v", receipt)
	} else {
		requireProjectionError(t, err, ErrRunCommandNotTerminal)
	}
}

func requireMutateStringPath(t *testing.T, root any, label string, path ...any) {
	t.Helper()
	value, commit, ok := mutableValueAtPath(reflect.ValueOf(root), path)
	if !ok || value.Kind() != reflect.String || !value.CanSet() {
		t.Fatalf("%s mutation path %v did not resolve to a settable string", label, path)
	}
	value.SetString("projection-test-mutated-" + label)
	commit()
}

func requireMutateFamily(t *testing.T, root any, label string, path ...any) {
	t.Helper()
	value, commit, ok := mutableValueAtPath(reflect.ValueOf(root), path)
	if !ok {
		t.Fatalf("%s mutation path %v did not resolve", label, path)
	}
	if mutations := poisonEveryMutableLeaf(value); mutations == 0 {
		t.Fatalf("%s mutation path %v contained no mutable leaf", label, path)
	}
	commit()
}

func mutableValueAtPath(root reflect.Value, path []any) (reflect.Value, func(), bool) {
	commits := make([]func(), 0)
	current := root
	unwrap := func() bool {
		for current.IsValid() && (current.Kind() == reflect.Pointer || current.Kind() == reflect.Interface) {
			if current.IsNil() {
				return false
			}
			if current.Kind() == reflect.Pointer {
				current = current.Elem()
				continue
			}
			container := current
			copyValue := reflect.New(container.Elem().Type()).Elem()
			copyValue.Set(container.Elem())
			current = copyValue
			commits = append(commits, func() { container.Set(copyValue) })
		}
		return current.IsValid()
	}
	if !unwrap() {
		return reflect.Value{}, func() {}, false
	}
	for _, component := range path {
		if !unwrap() {
			return reflect.Value{}, func() {}, false
		}
		switch component := component.(type) {
		case string:
			if current.Kind() != reflect.Struct {
				return reflect.Value{}, func() {}, false
			}
			current = current.FieldByName(component)
		case int:
			if current.Kind() != reflect.Slice || component < 0 || component >= current.Len() {
				return reflect.Value{}, func() {}, false
			}
			current = current.Index(component)
		default:
			return reflect.Value{}, func() {}, false
		}
	}
	if !unwrap() {
		return reflect.Value{}, func() {}, false
	}
	return current, func() {
		for index := len(commits) - 1; index >= 0; index-- {
			commits[index]()
		}
	}, true
}

func requireProjectionError(t *testing.T, err, target error) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want typed %v", target)
	}
	if !errors.Is(err, target) {
		t.Fatalf("error = %T %v, want errors.Is(..., %v)", err, err, target)
	}
}

func canonicalInputDocument(role CanonicalInputRole, raw []byte) CanonicalInputDocument {
	owned := append([]byte(nil), raw...)
	return CanonicalInputDocument{Role: role, Bytes: owned, SHA256: projectionSHA256(owned)}
}

func projectionSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func marshalProjectionLedger(t *testing.T, events ...map[string]any) []byte {
	t.Helper()
	var buffer bytes.Buffer
	for _, event := range events {
		buffer.Write(canonicalJSON(t, event))
		buffer.WriteByte('\n')
	}
	return buffer.Bytes()
}

func canonicalJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneCanonicalInput(input CanonicalRunReadInput) CanonicalRunReadInput {
	clone := input
	clone.Documents = make([]CanonicalInputDocument, len(input.Documents))
	for index, document := range input.Documents {
		clone.Documents[index] = document
		clone.Documents[index].Bytes = append([]byte(nil), document.Bytes...)
	}
	return clone
}

func cloneCanonicalCommandInput(input CanonicalCommandReadInput) CanonicalCommandReadInput {
	clone := input
	clone.Record = append([]byte(nil), input.Record...)
	return clone
}

func canonicalDocumentByRole(t *testing.T, input CanonicalRunReadInput, role CanonicalInputRole) CanonicalInputDocument {
	t.Helper()
	for _, document := range input.Documents {
		if document.Role == role {
			clone := document
			clone.Bytes = append([]byte(nil), document.Bytes...)
			return clone
		}
	}
	t.Fatalf("canonical input has no %s document", role)
	return CanonicalInputDocument{}
}

func canonicalDocumentsByRole(input CanonicalRunReadInput, role CanonicalInputRole) []CanonicalInputDocument {
	documents := make([]CanonicalInputDocument, 0)
	for _, document := range input.Documents {
		if document.Role == role {
			documents = append(documents, document)
		}
	}
	return documents
}

func removeCanonicalRole(documents []CanonicalInputDocument, role CanonicalInputRole) []CanonicalInputDocument {
	filtered := make([]CanonicalInputDocument, 0, len(documents))
	for _, document := range documents {
		if document.Role != role {
			filtered = append(filtered, document)
		}
	}
	return filtered
}

func replaceCanonicalDocument(t *testing.T, input CanonicalRunReadInput, role CanonicalInputRole, raw []byte) CanonicalRunReadInput {
	t.Helper()
	return replaceCanonicalDocumentObject(input, role, canonicalInputDocument(role, raw))
}

func replaceCanonicalDocumentObject(input CanonicalRunReadInput, role CanonicalInputRole, replacement CanonicalInputDocument) CanonicalRunReadInput {
	for index := range input.Documents {
		if input.Documents[index].Role == role {
			input.Documents[index] = replacement
			return input
		}
	}
	input.Documents = append(input.Documents, replacement)
	return input
}

func canonicalLedgerEvents(t *testing.T, input CanonicalRunReadInput) []map[string]any {
	t.Helper()
	var raw []byte
	for _, document := range input.Documents {
		if document.Role == CanonicalInputRoleSchema1Ledger || document.Role == CanonicalInputRoleSchema2Ledger {
			raw = document.Bytes
			break
		}
	}
	if raw == nil {
		t.Fatal("canonical input has no ledger")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var events []map[string]any
	for decoder.More() {
		var event map[string]any
		if err := decoder.Decode(&event); err != nil {
			t.Fatalf("decode canonical ledger: %v", err)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		t.Fatal("canonical ledger contains no events")
	}
	return events
}

func withEventSequence(event map[string]any, sequence uint64) map[string]any {
	clone := cloneStringAnyMap(event)
	clone["seq"] = sequence
	return clone
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	clone := make(map[string]any, len(input))
	for key, value := range input {
		clone[key] = cloneAny(value)
	}
	return clone
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(typed)
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneAny(item)
		}
		return clone
	case []string:
		return append([]string(nil), typed...)
	case []map[string]any:
		clone := make([]map[string]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneStringAnyMap(item)
		}
		return clone
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return value
	}
}

func removeKeysRecursively(value any, keys []string) {
	private := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		private[key] = struct{}{}
	}
	var walk func(any)
	walk = func(current any) {
		switch current := current.(type) {
		case map[string]any:
			for key, nested := range current {
				if _, omit := private[key]; omit {
					delete(current, key)
					continue
				}
				walk(nested)
			}
		case []any:
			for _, nested := range current {
				walk(nested)
			}
		}
	}
	walk(value)
}

func jsonBytesEqual(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func poisonEveryMutableLeaf(value reflect.Value) int {
	if !value.IsValid() {
		return 0
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return 0
		}
		copyValue := reflect.New(value.Elem().Type()).Elem()
		copyValue.Set(value.Elem())
		mutations := poisonEveryMutableLeaf(copyValue)
		if mutations > 0 && value.CanSet() {
			value.Set(copyValue)
		}
		return mutations
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0
		}
		return poisonEveryMutableLeaf(value.Elem())
	}
	switch value.Kind() {
	case reflect.Struct:
		mutations := 0
		for index := 0; index < value.NumField(); index++ {
			if value.Field(index).CanSet() {
				mutations += poisonEveryMutableLeaf(value.Field(index))
			}
		}
		return mutations
	case reflect.Slice:
		mutations := 0
		for index := 0; index < value.Len(); index++ {
			mutations += poisonEveryMutableLeaf(value.Index(index))
		}
		if value.CanSet() && value.Len() > 0 {
			value.Set(reflect.Append(value, reflect.Zero(value.Type().Elem())))
			mutations++
		}
		return mutations
	case reflect.Map:
		if value.IsNil() || value.Len() == 0 {
			return 0
		}
		keys := value.MapKeys()
		value.SetMapIndex(keys[0], reflect.Value{})
		return 1
	case reflect.String:
		if value.CanSet() {
			value.SetString("projection-test-mutated")
			return 1
		}
	case reflect.Bool:
		if value.CanSet() {
			value.SetBool(!value.Bool())
			return 1
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if value.CanSet() {
			value.SetUint(value.Uint() + 1)
			return 1
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value.CanSet() {
			value.SetInt(value.Int() + 1)
			return 1
		}
	}
	return 0
}

func mutateCommandRecord(t *testing.T, raw []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var record map[string]any
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	mutate(record)
	return canonicalJSON(t, record)
}

func assertRunRootJSON(t *testing.T, view RunView, want string) {
	t.Helper()
	got := mustMarshalJSON(t, view.Identity.RunRoot)
	if string(got) != want {
		t.Fatalf("runRoot = %s, want %s", got, want)
	}
}

func eventDataMember(t *testing.T, event SafeRunEvent, member string) json.RawMessage {
	t.Helper()
	var decoded struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(mustMarshalJSON(t, event), &decoded); err != nil {
		t.Fatal(err)
	}
	value, ok := decoded.Data[member]
	if !ok {
		t.Fatalf("event data has no %q: %s", member, mustMarshalJSON(t, event))
	}
	return value
}

func mustSafeProjectedEvent(t *testing.T, projection CanonicalRunProjection, index int) SafeRunEvent {
	t.Helper()
	if index < 0 || index >= len(projection.events) || projection.events[index].omitted {
		t.Fatalf("projected safe event index %d invalid", index)
	}
	return projection.events[index].safe
}

func cloneSafeEventWithSequence(t *testing.T, event SafeRunEvent, sequence uint64) SafeRunEvent {
	t.Helper()
	value := reflect.ValueOf(event)
	pointer := value.Kind() == reflect.Pointer
	if pointer {
		value = value.Elem()
	}
	clone := reflect.New(value.Type()).Elem()
	clone.Set(value)
	sequenceField := clone.FieldByName("Seq")
	if !sequenceField.IsValid() || !sequenceField.CanSet() {
		t.Fatalf("safe event %T has no settable Seq field", event)
	}
	switch sequenceField.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		sequenceField.SetUint(sequence)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		sequenceField.SetInt(int64(sequence))
	default:
		t.Fatalf("safe event %T Seq kind = %s", event, sequenceField.Kind())
	}
	var result any
	if pointer {
		copyPointer := reflect.New(clone.Type())
		copyPointer.Elem().Set(clone)
		result = copyPointer.Interface()
	} else {
		result = clone.Interface()
	}
	safe, ok := result.(SafeRunEvent)
	if !ok {
		t.Fatalf("cloned %T does not implement SafeRunEvent", result)
	}
	return safe
}

func cloneSafeEventWithMessage(t *testing.T, event SafeRunEvent, message string) SafeRunEvent {
	t.Helper()
	value := reflect.ValueOf(event)
	pointer := value.Kind() == reflect.Pointer
	if pointer {
		value = value.Elem()
	}
	clone := reflect.New(value.Type()).Elem()
	clone.Set(value)
	data := clone.FieldByName("Data")
	if !data.IsValid() {
		t.Fatalf("safe error event %T has no Data field", event)
	}
	if data.Kind() == reflect.Pointer {
		dataClone := reflect.New(data.Elem().Type())
		dataClone.Elem().Set(data.Elem())
		data.Set(dataClone)
		data = data.Elem()
	}
	messageField := data.FieldByName("Message")
	if !messageField.IsValid() || !messageField.CanSet() || messageField.Kind() != reflect.String {
		t.Fatalf("safe error event %T has no settable Data.Message", event)
	}
	messageField.SetString(message)
	var result any
	if pointer {
		copyPointer := reflect.New(clone.Type())
		copyPointer.Elem().Set(clone)
		result = copyPointer.Interface()
	} else {
		result = clone.Interface()
	}
	safe, ok := result.(SafeRunEvent)
	if !ok {
		t.Fatalf("cloned %T does not implement SafeRunEvent", result)
	}
	return safe
}

func projectionWithRepeatedSafeEvents(t *testing.T, base CanonicalRunProjection, count int) CanonicalRunProjection {
	t.Helper()
	safe := mustSafeProjectedEvent(t, base, 0)
	projection := base
	projection.events = make([]projectedEvent, count)
	for index := range projection.events {
		sequence := uint64(index + 1)
		projection.events[index] = projectedEvent{scanSeq: sequence, safe: cloneSafeEventWithSequence(t, safe, sequence)}
	}
	projection.latestSeq = uint64(count)
	projection.view.Cursor = uint64(count)
	return projection
}

func projectionWithCompletePageSize(t *testing.T, base CanonicalRunProjection, target int) CanonicalRunProjection {
	t.Helper()
	projection := base
	safe := cloneSafeEventWithSequence(t, mustSafeProjectedEvent(t, base, 1), 2)
	view := ProjectRunView(base)
	candidate := RunEventPage{
		Schema: RunEventPageSchema, RunID: view.RunID, Generation: view.Generation, Source: view.Source,
		Cursor: 2, HasMore: false, Events: []SafeRunEvent{safe},
	}
	baseSize := len(mustMarshalJSON(t, candidate))
	if target < baseSize {
		t.Fatalf("target page size %d smaller than fixed envelope %d", target, baseSize)
	}
	safe = cloneSafeEventWithMessage(t, safe, strings.Repeat("x", target-baseSize+1))
	candidate.Events = []SafeRunEvent{safe}
	actual := len(mustMarshalJSON(t, candidate))
	difference := actual - target
	if difference < 0 || difference > target-baseSize+1 {
		t.Fatalf("unable to size complete page: target=%d actual=%d", target, actual)
	}
	safe = cloneSafeEventWithMessage(t, safe, strings.Repeat("x", target-baseSize+1-difference))
	candidate.Events = []SafeRunEvent{safe}
	if got := len(mustMarshalJSON(t, candidate)); got != target {
		t.Fatalf("complete candidate bytes = %d, want %d", got, target)
	}
	projection.events = append([]projectedEvent(nil), base.events...)
	projection.events[1] = projectedEvent{scanSeq: 2, safe: safe}
	projection.latestSeq = 2
	projection.view.Cursor = 2
	return projection
}

func projectionFingerprint(t *testing.T, projection CanonicalRunProjection) string {
	t.Helper()
	var buffer strings.Builder
	buffer.Write(mustMarshalJSON(t, projection.view))
	fmt.Fprintf(&buffer, "|latest=%d", projection.latestSeq)
	for _, event := range projection.events {
		fmt.Fprintf(&buffer, "|seq=%d|omitted=%v|", event.scanSeq, event.omitted)
		if !event.omitted {
			buffer.Write(mustMarshalJSON(t, event.safe))
		}
	}
	return buffer.String()
}

func recursiveJSONKeys(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				keys[key] = true
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return keys
}

func sortedRawKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedAnyKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedIntSliceKeys(values map[string][]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
