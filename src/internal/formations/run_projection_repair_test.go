package formations

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Task 1 owns structural reduction only. Recovery-state predicates, actions,
// and full artifact authorization/revocation semantics remain Task 2.
func TestProjectCanonicalRunSchema2StructuralArmsAreNeverAuditOnly(t *testing.T) {
	board := &BoardDocument{
		Missions:   []MissionNode{{ID: projectionTestMissionID}},
		Formations: []FormationNode{{ID: projectionTestFormationID}},
		Tools:      []ToolNode{{ID: "tool_normalize"}},
		Gates:      []GateNode{{ID: projectionTestGateID}},
	}

	tests := []struct {
		name        string
		prepare     func(*projectionState)
		eventType   string
		data        map[string]any
		envelope    func(map[string]any)
		want        func(*projectionState)
		historyOnly bool
	}{
		{
			name: "formation result closes its attempt and materializes outputs", prepare: startSchema2RepairAttempt(projectionTestFormationID, "formation"),
			eventType: "formation_result", data: schema2RepairFormationResultData(),
			want: func(state *projectionState) {
				schema2RepairCompleteAttempt(state, projectionTestFormationID, 1, "done", 20)
				schema2RepairAppendOutput(state, projectionTestFormationID, 1, "port_out", 20, schema2RepairPayload("formation output"))
			},
		},
		{
			name:      "tool dispatch opens tool attempt state",
			eventType: "tool_dispatch", data: schema2RepairToolDispatchData(),
			want: func(state *projectionState) { schema2RepairStartAttempt(state, "tool_normalize", "tool", 1, 20) },
		},
		{
			name: "tool process launch is deliberately non-structural typed history", prepare: startSchema2RepairAttempt("tool_normalize", "tool"),
			eventType: "tool_process_launch", data: schema2RepairToolProcessLaunchData(),
			historyOnly: true,
		},
		{
			name: "tool result closes tool attempt and materializes outputs", prepare: startSchema2RepairAttempt("tool_normalize", "tool"),
			eventType: "tool_result", data: schema2RepairToolResultData(),
			want: func(state *projectionState) {
				schema2RepairCompleteAttempt(state, "tool_normalize", 1, "done", 20)
				schema2RepairAppendOutput(state, "tool_normalize", 1, "port_tool_out", 20, schema2RepairPayload(`{"normalized":true}`))
			},
		},
		{
			name: "node output closes the named attempt and materializes outputs", prepare: startSchema2RepairAttempt(projectionTestMissionID, "mission"),
			eventType: "node_output", data: schema2RepairNodeOutputData(projectionTestMissionID),
			want: func(state *projectionState) {
				schema2RepairCompleteAttempt(state, projectionTestMissionID, 1, "done", 20)
				schema2RepairAppendOutput(state, projectionTestMissionID, 1, "out", 20, schema2RepairPayload("done"))
			},
		},
		{
			name:      "gate evaluating opens gate state",
			eventType: "gate_evaluating", data: schema2RepairGateEvaluatingData(),
			want: func(state *projectionState) { schema2RepairStartGate(state, projectionTestGateID, 1, 20) },
		},
		{
			name: "gate kind result retains typed evidence", prepare: prepareSchema2RepairGate,
			eventType: "gate_kind_result", data: schema2RepairGateKindResultData(),
			want: func(state *projectionState) {
				state.ensureGate(projectionTestGateID, 1).Evidence = append(state.ensureGate(projectionTestGateID, 1).Evidence, SafeGateEvidence{Kind: "text", Text: "clean"})
			},
		},
		{
			name: "judge result closes only the judge attempt and advances gate evidence", prepare: prepareSchema2RepairJudgeGate,
			eventType: "judge_result", data: schema2RepairJudgeResultData(),
			want: func(state *projectionState) {
				schema2RepairCompleteAttempt(state, projectionTestFormationID, 1, "done", 20)
				state.ensureGate(projectionTestGateID, 1).Evidence = append(state.ensureGate(projectionTestGateID, 1).Evidence, SafeGateEvidence{Kind: "ledger", Seq: 18})
			},
		},
		{
			name: "judge attempt failure fails the judge and blocks the gate", prepare: prepareSchema2RepairJudgeGate,
			eventType: "judge_attempt_failed", data: schema2RepairJudgeAttemptFailedData(),
			want: func(state *projectionState) {
				schema2RepairCompleteAttempt(state, projectionTestFormationID, 1, "failed", 20)
				gate := state.ensureGate(projectionTestGateID, 1)
				gate.Status, gate.Reason = "blocked", "invalid result"
				state.node(projectionTestGateID).Status = "blocked"
			},
		},
		{
			name: "gate verdict closes the gate attempt", prepare: prepareSchema2RepairGate,
			eventType: "gate_verdict", data: schema2RepairGateVerdictData(),
			want: func(state *projectionState) { state.finishGate(projectionTestGateID, 1, 20, "fail", "needs revision") },
		},
		{
			name: "human request marks gate and attempt waiting", prepare: prepareSchema2RepairGate,
			eventType: "human_input_requested", data: schema2RepairHumanRequestData(),
			want: func(state *projectionState) {
				state.view.Status = "waiting_human"
				gate := state.ensureGate(projectionTestGateID, 1)
				gate.Status, gate.RequestSeq = "waiting_human", 20
				state.node(projectionTestGateID).Status = "waiting_human"
				schema2RepairAttempt(state, projectionTestGateID, 1).Status = "waiting_human"
			},
		},
		{
			name: "human verdict returns the gate to evaluation", prepare: prepareSchema2RepairHumanGate,
			eventType: "human_verdict_recorded", data: schema2RepairHumanVerdictData(),
			want: func(state *projectionState) {
				state.view.Status = "running"
				state.ensureGate(projectionTestGateID, 1).Status = "evaluating"
				state.node(projectionTestGateID).Status = "running"
				schema2RepairAttempt(state, projectionTestGateID, 1).Status = "running"
			},
		},
		{
			name: "escalation is retained structurally", prepare: startSchema2RepairAttempt(projectionTestFormationID, "formation"),
			eventType: "escalation_raised", data: map[string]any{
				"trigger": "operator_review", "severity": "needs-attention", "reason": "review", "source": "agent",
				"nodeId": projectionTestFormationID, "gateId": "", "blocks": true,
			},
			want: func(state *projectionState) {
				state.view.Escalations = append(state.view.Escalations, RunEscalationView{Seq: 20, NodeID: projectionTestFormationID, Severity: "needs-attention", Reason: "review", Source: "agent", Trigger: "operator_review", Blocks: true})
			},
		},
		{
			name: "node input ignored is deliberately non-structural typed history", prepare: startSchema2RepairAttempt(projectionTestFormationID, "formation"),
			eventType: "node_input_ignored", data: map[string]any{
				"nodeId": projectionTestFormationID, "toPortId": "port_in", "inputRef": schema2RepairInputRef(),
				"reason": "late_optional", "relatedAttempt": 1,
			},
			historyOnly: true,
		},
		{
			name: "scoped error changes the affected attempt", prepare: startSchema2RepairAttempt(projectionTestFormationID, "formation"),
			eventType: "error", data: map[string]any{
				"code": "dispatch_failed", "message": "dispatch failed", "boundary": "engine", "errorScope": "node",
				"nodeId": projectionTestFormationID, "recoverable": true, "relatedSeq": 2,
			},
			want: func(state *projectionState) {
				state.node(projectionTestFormationID).Status = "blocked"
				schema2RepairAttempt(state, projectionTestFormationID, 1).Status = "blocked"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, board)
			state.view.Status = "running"
			state.view.Identity.RunRoot = RunRoot{Kind: "mission", NodeID: projectionTestMissionID}
			if test.prepare != nil {
				test.prepare(&state)
			}
			before := schema2RepairCloneView(t, state.view)
			want := schema2RepairCloneState(t, state)
			if test.want != nil {
				test.want(&want)
			}
			data := schema2RepairReducerData(test.eventType, cloneAny(test.data).(map[string]any))
			event := schema2Event(projectionTestRunID, 20, test.eventType, data)
			delete(event, "missionId")
			delete(event, "beadId")
			if test.envelope != nil {
				test.envelope(event)
			}
			raw, safe := schema2RepairDecodeSafeEvent(t, event)
			if err := reduceSchema2Event(&state, raw, safe, map[string]RunCommandReceipt{}); err != nil {
				t.Fatalf("reduce admitted %s: %v", test.eventType, err)
			}
			schema2RepairAssertTypedHistory(t, safe, test.eventType, data)
			gotFingerprint := schema2RepairStructuralFingerprint(t, state.view)
			if test.historyOnly {
				if wantFingerprint := schema2RepairStructuralFingerprint(t, before); !bytes.Equal(gotFingerprint, wantFingerprint) {
					t.Fatalf("%s must preserve the frozen structural view\ngot:  %s\nwant: %s", test.eventType, gotFingerprint, wantFingerprint)
				}
				return
			}
			wantFingerprint := schema2RepairStructuralFingerprint(t, want.view)
			if !bytes.Equal(gotFingerprint, wantFingerprint) {
				t.Fatalf("%s structural postcondition mismatch\ngot:  %s\nwant: %s", test.eventType, gotFingerprint, wantFingerprint)
			}
		})
	}
}

func TestProjectCanonicalRunSchema2CompletionStatusAndDispositionMatrix(t *testing.T) {
	board := &BoardDocument{
		Missions:   []MissionNode{{ID: projectionTestMissionID}},
		Formations: []FormationNode{{ID: projectionTestFormationID}},
		Gates:      []GateNode{{ID: projectionTestGateID}},
	}

	for _, eventType := range []string{"formation_result", "node_output"} {
		for _, test := range []struct {
			status          string
			wantStatus      string
			wantDisposition string
		}{
			{status: "done", wantStatus: "done", wantDisposition: "done"},
			{status: "failed", wantStatus: "failed", wantDisposition: "failed"},
			{status: "needs-review", wantStatus: "needs-review", wantDisposition: ""},
			{status: "blocked", wantStatus: "blocked", wantDisposition: ""},
		} {
			if eventType == "formation_result" && test.status == "blocked" {
				continue
			}
			t.Run(eventType+"/"+test.status, func(t *testing.T) {
				state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, board)
				state.view.Status = "running"
				schema2RepairStartAttempt(&state, projectionTestFormationID, "formation", 1, 2)
				raw := rawProjectionEvent{envelope: safeEventEnvelope{RunID: projectionTestRunID, Seq: 20}}
				var safe SafeRunEvent
				switch eventType {
				case "formation_result":
					safe = SafeSchema2FormationResultEvent{Type: eventType, Data: SafeSchema2FormationResultData{
						NodeID: projectionTestFormationID, Attempt: 1, Status: test.status, Outputs: SafePayloadProjections{},
					}}
				case "node_output":
					safe = SafeSchema2NodeOutputEvent{Type: eventType, Data: SafeSchema2NodeOutputData{
						NodeID: projectionTestFormationID, Status: test.status, Outputs: SafePayloadProjections{},
					}}
				}
				if err := reduceSchema2Event(&state, raw, safe, nil); err != nil {
					t.Fatalf("reduce admitted %s(%s): %v", eventType, test.status, err)
				}
				node := state.node(projectionTestFormationID)
				attempt := state.existingAttempt(projectionTestFormationID, 1)
				if node == nil || attempt == nil {
					t.Fatalf("completion lost node/attempt: node=%#v attempt=%#v", node, attempt)
				}
				if node.Status != test.wantStatus || attempt.Status != test.wantStatus {
					t.Fatalf("statuses = node:%q attempt:%q, want %q", node.Status, attempt.Status, test.wantStatus)
				}
				if node.FinalDisposition != test.wantDisposition || attempt.Disposition != test.wantDisposition {
					t.Fatalf("dispositions = node:%q attempt:%q, want %q", node.FinalDisposition, attempt.Disposition, test.wantDisposition)
				}
			})
		}
	}

	for _, test := range []struct {
		verdict         string
		wantGateStatus  string
		wantNodeStatus  string
		wantDisposition string
	}{
		{verdict: "pass", wantGateStatus: "passed", wantNodeStatus: "done", wantDisposition: "done"},
		{verdict: "fail", wantGateStatus: "failed", wantNodeStatus: "failed", wantDisposition: "failed"},
	} {
		t.Run("gate_verdict/"+test.verdict, func(t *testing.T) {
			state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, board)
			state.view.Status = "running"
			prepareSchema2RepairGate(&state)
			raw := rawProjectionEvent{envelope: safeEventEnvelope{RunID: projectionTestRunID, Seq: 20}}
			safe := SafeSchema2GateVerdictEvent{Type: "gate_verdict", Data: SafeSchema2GateVerdictData{
				GateID: projectionTestGateID, GateAttempt: 1, Verdict: test.verdict, Reason: "decision",
			}}
			if err := reduceSchema2Event(&state, raw, safe, nil); err != nil {
				t.Fatalf("reduce admitted gate verdict %s: %v", test.verdict, err)
			}
			gate := state.existingGate(projectionTestGateID, 1)
			node := state.node(projectionTestGateID)
			attempt := state.existingAttempt(projectionTestGateID, 1)
			if gate == nil || node == nil || attempt == nil {
				t.Fatalf("verdict lost gate/node/attempt: gate=%#v node=%#v attempt=%#v", gate, node, attempt)
			}
			if gate.Status != test.wantGateStatus || node.Status != test.wantNodeStatus || attempt.Status != test.wantNodeStatus {
				t.Fatalf("statuses = gate:%q node:%q attempt:%q, want %q/%q/%q", gate.Status, node.Status, attempt.Status, test.wantGateStatus, test.wantNodeStatus, test.wantNodeStatus)
			}
			if node.FinalDisposition != test.wantDisposition || attempt.Disposition != test.wantDisposition {
				t.Fatalf("dispositions = node:%q attempt:%q, want %q", node.FinalDisposition, attempt.Disposition, test.wantDisposition)
			}
		})
	}
}

func TestProjectCanonicalRunSchema2PreservesEveryAdmittedGateEvidenceKind(t *testing.T) {
	for _, test := range []struct {
		name     string
		evidence map[string]any
		want     SafeGateEvidence
	}{
		{name: "artifact", evidence: map[string]any{"kind": "artifact", "artifactId": "artifact_report"}, want: SafeGateEvidence{Kind: "artifact", ArtifactID: "artifact_report"}},
		{name: "ledger", evidence: map[string]any{"kind": "ledger", "seq": 18}, want: SafeGateEvidence{Kind: "ledger", Seq: 18}},
		{name: "text", evidence: map[string]any{"kind": "text", "text": "clean"}, want: SafeGateEvidence{Kind: "text", Text: "clean"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, &BoardDocument{Gates: []GateNode{{ID: projectionTestGateID}}})
			state.view.Status = "running"
			prepareSchema2RepairGate(&state)
			data := schema2RepairWithEvidence(schema2RepairGateKindResultData(), test.evidence)
			event := schema2Event(projectionTestRunID, 20, "gate_kind_result", data)
			delete(event, "missionId")
			delete(event, "beadId")
			raw, safe := schema2RepairDecodeSafeEvent(t, event)
			if err := reduceSchema2Event(&state, raw, safe, nil); err != nil {
				t.Fatalf("reduce admitted %s evidence: %v", test.name, err)
			}
			gate := state.existingGate(projectionTestGateID, 1)
			if gate == nil || len(gate.Evidence) != 1 || gate.Evidence[0] != test.want {
				t.Fatalf("preserved %s evidence = %#v, want %#v", test.name, gate, test.want)
			}
		})
	}
}

func TestProjectCanonicalRunRejectsInvalidSchema2StartAttempt(t *testing.T) {
	for _, test := range []struct {
		name    string
		nodeID  string
		attempt uint64
	}{
		{name: "unknown node", nodeID: "fmn_missing", attempt: 1},
		{name: "zero attempt", nodeID: projectionTestFormationID, attempt: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, &BoardDocument{Formations: []FormationNode{{ID: projectionTestFormationID}}})
			data := map[string]any{
				"nodeId": test.nodeID, "nodeKind": "formation", "attempt": test.attempt, "reason": "initial", "inputRefs": []any{},
			}
			event := schema2Event(projectionTestRunID, 2, "node_started", data)
			delete(event, "missionId")
			delete(event, "beadId")
			raw, safe := schema2RepairDecodeSafeEvent(t, event)
			if err := reduceSchema2Event(&state, raw, safe, nil); err == nil {
				t.Fatalf("invalid startAttempt node=%q attempt=%d was silently consumed", test.nodeID, test.attempt)
			} else {
				requireProjectionError(t, err, ErrRunProjectionInvalid)
			}
		})
	}
}

func TestProjectCanonicalRunRejectsInvalidSchema1StartAttempt(t *testing.T) {
	for _, test := range []struct {
		name    string
		nodeID  string
		attempt uint64
	}{
		{name: "unknown node", nodeID: "fmn_missing", attempt: 1},
		{name: "zero attempt", nodeID: projectionTestFormationID, attempt: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema1, &BoardDocument{Formations: []FormationNode{{ID: projectionTestFormationID}}})
			event := schema1Event(projectionTestRunID, 2, "node_started", map[string]any{"inputRefs": []any{}})
			event["nodeId"], event["attempt"] = test.nodeID, test.attempt
			raw, err := decodeProjectionEvent(canonicalJSON(t, event), CanonicalRunSourceSchema1, projectionTestRunID)
			if err != nil {
				t.Fatalf("decode invalid-start fixture: %v", err)
			}
			safe, err := sanitizeSchema1Event(raw)
			if err != nil {
				t.Fatalf("sanitize invalid-start fixture: %v", err)
			}
			if err := reduceSchema1Event(&state, raw, safe); err == nil {
				t.Fatalf("schema-1 invalid startAttempt node=%q attempt=%d was silently consumed", test.nodeID, test.attempt)
			} else {
				requireProjectionError(t, err, ErrRunProjectionInvalid)
			}
		})
	}
}

func TestProjectCanonicalRunSchema2SessionAuthorityArmsAreNeverAuditOnly(t *testing.T) {
	dispatchID := "dsp_01KXNP6VY3227H78329V52CKF8"
	leaseID := "lease_01KXNP6VY3227H78329V52CKF8"
	fingerprint := strings.Repeat("a", 64)
	tests := []struct {
		name      string
		eventType string
		data      map[string]any
		prepare   func(*RunSessionView)
		want      func(*RunSessionView)
	}{
		{name: "peek capability issuance updates session capability", eventType: "slot_peek_capability_issued", data: map[string]any{
			"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": "binding_worker", "sessionTargetId": "target_worker",
			"targetFingerprint": fingerprint, "capabilityGeneration": "1", "priorIssuedSeq": 0, "issuedAt": "2026-07-20T10:00:19Z",
		}, want: func(session *RunSessionView) {
			session.PeekCapability = RunSessionPeekCapability{State: "issued", IssuedSeq: 20, Generation: "1"}
		}},
		{name: "steering start opens only the named session steering generation", eventType: "slot_steering_started", prepare: func(session *RunSessionView) {
			session.PeekCapability = RunSessionPeekCapability{State: "issued", IssuedSeq: 18, Generation: "1"}
		}, data: map[string]any{
			"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": "binding_worker", "sessionTargetId": "target_worker",
			"targetFingerprint": fingerprint, "capabilityIssuedSeq": 18, "capabilityGeneration": "1", "steeringGeneration": "1",
			"actor": "human:test", "startedAt": "2026-07-20T10:00:19Z", "recordedBeforeInput": true,
		}, want: func(session *RunSessionView) {
			started := uint64(20)
			session.PeekCapability.State = "input_open"
			session.Steering = RunSessionSteering{State: "open", Generation: "1", StartedSeq: &started}
		}},
		{name: "steering end closes session steering", eventType: "slot_steering_ended", prepare: func(session *RunSessionView) {
			started := uint64(18)
			session.PeekCapability = RunSessionPeekCapability{State: "input_open", IssuedSeq: 17, Generation: "1"}
			session.Steering = RunSessionSteering{State: "open", Generation: "1", StartedSeq: &started}
		}, data: map[string]any{
			"startedSeq": 18, "dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprint": fingerprint,
			"steeringGeneration": "1", "reason": "released", "endedAt": "2026-07-20T10:00:19Z",
		}, want: func(session *RunSessionView) {
			session.PeekCapability.State = "issued"
			session.Steering = RunSessionSteering{State: "closed", Generation: "1"}
		}},
		{name: "reconciliation interrupt updates session occupancy", eventType: "slot_reconciliation_interrupt", data: map[string]any{
			"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": "binding_worker", "sessionTargetId": "target_worker",
			"targetFingerprint": fingerprint, "authorityKind": "failure", "authoritySeq": 18,
			"interruptEncoding": "terminal-etx-v1", "interruptSha256": strings.Repeat("b", 64), "recordedBeforeSend": true,
		}, want: func(session *RunSessionView) { session.Occupancy.State = "held" }},
		{name: "reconciliation outcome updates session occupancy", eventType: "slot_reconciliation_interrupt_outcome", prepare: func(session *RunSessionView) {
			session.Occupancy.State = "held"
		}, data: map[string]any{
			"requestedSeq": 18, "dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprint": fingerprint,
			"outcome": "unavailable", "observedAt": "2026-07-20T10:00:19Z",
		}, want: func(session *RunSessionView) { session.Occupancy.State = "quarantined" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newProjectionState(projectionTestRunID, CanonicalRunSourceSchema2, &BoardDocument{Formations: []FormationNode{{ID: projectionTestFormationID}}})
			state.dispatches[dispatchID] = SafeSchema2SlotDispatchData{
				DispatchID: dispatchID, TargetLeaseID: leaseID, NodeID: projectionTestFormationID, Attempt: 1,
				SlotID: "slot_worker", AgentID: "worker", BindingID: "binding_worker", SessionTargetID: "target_worker",
				TargetFingerprint: fingerprint, SteeringGeneration: "0",
			}
			state.dispatchSeq[dispatchID] = 4
			state.view.Sessions = []RunSessionView{{
				BindingID: "binding_worker", NodeID: projectionTestFormationID, Attempt: 1, SlotID: "slot_worker",
				DispatchID: dispatchID, TargetLeaseID: leaseID, SessionTargetID: "target_worker",
				BindingHealth: "runnable", Baseline: RunSessionBaseline{State: "valid"}, Attachment: RunSessionAttachment{State: "accounted"},
				Occupancy: RunSessionOccupancy{State: "active"}, PeekCapability: RunSessionPeekCapability{State: "none", Generation: "0"},
				Steering: RunSessionSteering{State: "closed", Generation: "0"},
			}}
			if test.prepare != nil {
				test.prepare(&state.view.Sessions[0])
			}
			want := schema2RepairCloneState(t, state)
			test.want(&want.view.Sessions[0])
			event := schema2Event(projectionTestRunID, 20, test.eventType, cloneAny(test.data).(map[string]any))
			delete(event, "missionId")
			delete(event, "beadId")
			raw, safe := schema2RepairDecodeSafeEvent(t, event)
			if err := reduceSchema2Event(&state, raw, safe, nil); err != nil {
				t.Fatalf("reduce admitted %s: %v", test.eventType, err)
			}
			schema2RepairAssertTypedHistory(t, safe, test.eventType, test.data)
			gotFingerprint := schema2RepairStructuralFingerprint(t, state.view)
			wantFingerprint := schema2RepairStructuralFingerprint(t, want.view)
			if !bytes.Equal(gotFingerprint, wantFingerprint) {
				t.Fatalf("%s session postcondition mismatch\ngot:  %s\nwant: %s", test.eventType, gotFingerprint, wantFingerprint)
			}
		})
	}
}

func TestSafeRunEventPublicTypesContainOnlyClosedNamedProjections(t *testing.T) {
	for _, event := range append(schema2SafeEventTypes(), schema1SafeEventTypes()...) {
		t.Run(strconv.Itoa(event.source)+"/"+event.literal, func(t *testing.T) {
			paths := schema2RepairUnsafePublicTypePaths(event.typeOf, nil, map[reflect.Type]bool{})
			if len(paths) != 0 {
				t.Fatalf("public SafeRunEvent contains permissive or anonymous nested types at %s; use closed named projections", strings.Join(paths, ", "))
			}
		})
	}
}

func TestSchema2NestedPublicProjectionFamiliesAreClosed(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		valid     map[string]any
		mutate    func(map[string]any)
	}{
		{name: "turn inputs reject unknown member", eventType: "slot_dispatch", valid: schema2RepairSlotDispatchData(t), mutate: func(data map[string]any) {
			data["turnInputs"].(map[string]any)["prompt"] = "forbidden"
		}},
		{name: "turn inputs require node started sequence", eventType: "slot_dispatch", valid: schema2RepairSlotDispatchData(t), mutate: func(data map[string]any) {
			delete(data["turnInputs"].(map[string]any), "nodeStartedSeq")
		}},
		{name: "turn inputs reject unsafe result sequence", eventType: "slot_dispatch", valid: schema2RepairSlotDispatchData(t), mutate: func(data map[string]any) {
			data["turnInputs"].(map[string]any)["priorTurnResults"] = []any{map[string]any{"slotResultSeq": MaxJSONSafeInteger + 1, "turnResultSha256": strings.Repeat("a", 64)}}
		}},
		{name: "turn inputs reject invalid result hash", eventType: "slot_dispatch", valid: schema2RepairSlotDispatchData(t), mutate: func(data map[string]any) {
			data["turnInputs"].(map[string]any)["priorTurnResults"] = []any{map[string]any{"slotResultSeq": 1, "turnResultSha256": "not-a-hash"}}
		}},
		{name: "turn inputs require prior results", eventType: "slot_dispatch", valid: schema2RepairSlotDispatchData(t), mutate: func(data map[string]any) {
			delete(data["turnInputs"].(map[string]any), "priorTurnResults")
		}},
		{name: "turn inputs reject duplicate result sequence", eventType: "slot_dispatch", valid: schema2RepairSlotDispatchData(t), mutate: func(data map[string]any) {
			item := map[string]any{"slotResultSeq": 1, "turnResultSha256": strings.Repeat("a", 64)}
			data["turnInputs"].(map[string]any)["priorTurnResults"] = []any{item, cloneAny(item)}
		}},
		{name: "turn inputs reject reordered result sequence", eventType: "slot_dispatch", valid: schema2RepairSlotDispatchData(t), mutate: func(data map[string]any) {
			data["turnInputs"].(map[string]any)["priorTurnResults"] = []any{
				map[string]any{"slotResultSeq": 9, "turnResultSha256": strings.Repeat("a", 64)},
				map[string]any{"slotResultSeq": 8, "turnResultSha256": strings.Repeat("b", 64)},
			}
		}},
		{name: "turn result identity rejects unknown member", eventType: "slot_dispatch", valid: schema2RepairSlotDispatchData(t), mutate: func(data map[string]any) {
			data["turnInputs"].(map[string]any)["priorTurnResults"] = []any{map[string]any{"slotResultSeq": 1, "turnResultSha256": strings.Repeat("a", 64), "result": "forbidden"}}
		}},
		{name: "tool inputs require manifest hash", eventType: "tool_dispatch", valid: schema2RepairToolDispatchData(), mutate: func(data map[string]any) {
			delete(data, "inputManifestSha256")
		}},
		{name: "tool input hashes reject invalid hash grammar", eventType: "tool_dispatch", valid: schema2RepairToolDispatchData(), mutate: func(data map[string]any) {
			data["inputHashes"].(map[string]any)["port_tool_in"] = "not-a-hash"
		}},
		{name: "tool input hash keys require identifier grammar", eventType: "tool_dispatch", valid: schema2RepairToolDispatchData(), mutate: func(data map[string]any) {
			data["inputHashes"] = map[string]any{"../private": strings.Repeat("a", 64)}
		}},
		{name: "tool inputs reject unknown member", eventType: "tool_dispatch", valid: schema2RepairToolDispatchData(), mutate: func(data map[string]any) {
			data["executable"] = "/private/tool"
		}},
		{name: "tool dispatch must be recorded before execution", eventType: "tool_dispatch", valid: schema2RepairToolDispatchData(), mutate: func(data map[string]any) {
			data["recordedBeforeExecute"] = false
		}},
		{name: "tool result requires timing", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			delete(data, "timing")
		}},
		{name: "tool result outputs reject unknown payload member", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			data["outputs"].(map[string]any)["port_tool_out"].(map[string]any)["path"] = "/private"
		}},
		{name: "tool result output hashes must match output keys", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			data["outputHashes"] = map[string]any{"other": strings.Repeat("a", 64)}
		}},
		{name: "tool result output hash must cover canonical projection", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			data["outputHashes"].(map[string]any)["port_tool_out"] = strings.Repeat("a", 64)
		}},
		{name: "tool result artifact registration ids must agree", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			registration := schema2RepairAvailableArtifactProjection()
			registration["artifact"].(map[string]any)["artifactId"] = "artifact_other"
			data["artifactRegistrations"] = []any{registration}
		}},
		{name: "tool result rejects duplicate artifact registrations", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			registration := schema2RepairAvailableArtifactProjection()
			data["artifactRegistrations"] = []any{registration, cloneAny(registration)}
		}},
		{name: "tool result display evidence rejects unknown member", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			data["displayEvidence"] = []any{map[string]any{"kind": "text", "text": "ok", "ref": "forbidden"}}
		}},
		{name: "tool result timing rejects negative duration", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			data["timing"].(map[string]any)["durationMs"] = -1
		}},
		{name: "tool result timing rejects unknown member", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			data["timing"].(map[string]any)["deadline"] = "private"
		}},
		{name: "node outputs require status", eventType: "node_output", valid: schema2RepairNodeOutputData(projectionTestMissionID), mutate: func(data map[string]any) {
			delete(data, "status")
		}},
		{name: "node outputs reject invalid status", eventType: "node_output", valid: schema2RepairNodeOutputData(projectionTestMissionID), mutate: func(data map[string]any) {
			data["status"] = "maybe"
		}},
		{name: "node outputs reject unclosed payload", eventType: "node_output", valid: schema2RepairNodeOutputData(projectionTestMissionID), mutate: func(data map[string]any) {
			data["outputs"].(map[string]any)["out"].(map[string]any)["token"] = "forbidden"
		}},
		{name: "node outputs reject unknown producer kind", eventType: "node_output", valid: schema2RepairNodeOutputData(projectionTestMissionID), mutate: func(data map[string]any) {
			data["producedBy"].(map[string]any)["kind"] = "socket"
		}},
		{name: "node outputs require producer outcome sequence", eventType: "node_output", valid: schema2RepairNodeOutputData(projectionTestMissionID), mutate: func(data map[string]any) {
			delete(data["producedBy"].(map[string]any), "outcomeSeq")
		}},
		{name: "node outputs reject malformed delivered edge identity", eventType: "node_output", valid: schema2RepairNodeOutputData(projectionTestMissionID), mutate: func(data map[string]any) {
			data["deliveredEdges"] = []any{map[string]any{
				"originEdgeId": "../private", "deliveryEdgeId": "edge_root_work", "toNodeId": projectionTestFormationID, "toPortId": "port_in",
				"sourceNodeId": projectionTestMissionID, "sourcePortId": "out", "sourceOutputSeq": 1, "sourceAttempt": 1,
			}}
		}},
		{name: "gate criterion requires text", eventType: "gate_evaluating", valid: schema2RepairGateEvaluatingData(), mutate: func(data map[string]any) {
			delete(data["criterionProjection"].(map[string]any), "text")
		}},
		{name: "gate criterion requires authored config classification", eventType: "gate_evaluating", valid: schema2RepairGateEvaluatingData(), mutate: func(data map[string]any) {
			data["criterionProjection"].(map[string]any)["classification"] = "runtime"
		}},
		{name: "gate criterion rejects unknown member", eventType: "gate_evaluating", valid: schema2RepairGateEvaluatingData(), mutate: func(data map[string]any) {
			data["criterionProjection"].(map[string]any)["prompt"] = "forbidden"
		}},
		{name: "gate criterion hash must match text", eventType: "gate_evaluating", valid: schema2RepairGateEvaluatingData(), mutate: func(data map[string]any) {
			data["criterionProjection"].(map[string]any)["sha256"] = strings.Repeat("a", 64)
		}},
		{name: "gate criterion encoding is fixed", eventType: "gate_evaluating", valid: schema2RepairGateEvaluatingData(), mutate: func(data map[string]any) {
			data["criterionProjection"].(map[string]any)["encoding"] = "utf8"
		}},
		{name: "gate criterion text is bounded", eventType: "gate_evaluating", valid: schema2RepairGateEvaluatingData(), mutate: func(data map[string]any) {
			text := strings.Repeat("x", 1<<20)
			criterion := data["criterionProjection"].(map[string]any)
			criterion["text"], criterion["sha256"] = text, projectionSHA256([]byte(text))
		}},
		{name: "gate result rejects unknown evidence kind", eventType: "gate_kind_result", valid: schema2RepairGateKindResultData(), mutate: func(data map[string]any) {
			data["evidence"] = []any{map[string]any{"kind": "socket", "text": "forbidden"}}
		}},
		{name: "gate result evidence requires kind payload", eventType: "gate_kind_result", valid: schema2RepairGateKindResultData(), mutate: func(data map[string]any) {
			data["evidence"] = []any{map[string]any{"kind": "artifact"}}
		}},
		{name: "gate result hash must match exact result", eventType: "gate_kind_result", valid: schema2RepairGateKindResultData(), mutate: func(data map[string]any) {
			data["reason"] = "changed after hash"
		}},
		{name: "code gate result requires every binding hash", eventType: "gate_kind_result", valid: schema2RepairGateKindResultData(), mutate: func(data map[string]any) {
			delete(data, "policySha256")
		}},
		{name: "formation gate result forbids code binding hashes", eventType: "gate_kind_result", valid: schema2RepairFormationGateKindResultData(), mutate: func(data map[string]any) {
			data["gateBindingId"] = "gatebinding_forbidden"
		}},
		{name: "judge result rejects invalid verdict", eventType: "judge_result", valid: schema2RepairJudgeResultData(), mutate: func(data map[string]any) {
			data["result"].(map[string]any)["verdict"] = "unknown"
		}},
		{name: "judge result requires reason", eventType: "judge_result", valid: schema2RepairJudgeResultData(), mutate: func(data map[string]any) {
			delete(data["result"].(map[string]any), "reason")
		}},
		{name: "judge result hash must match closed result", eventType: "judge_result", valid: schema2RepairJudgeResultData(), mutate: func(data map[string]any) {
			data["result"].(map[string]any)["reason"] = "changed after hash"
		}},
		{name: "gate feedback evaluated input is identity only", eventType: "gate_verdict", valid: schema2RepairGateVerdictData(), mutate: func(data map[string]any) {
			data["feedbackPayload"].(map[string]any)["evaluatedInput"].(map[string]any)["payloadProjection"] = schema2RepairWorkProjection("forbidden")
		}},
		{name: "gate feedback verdict is fail", eventType: "gate_verdict", valid: schema2RepairGateVerdictData(), mutate: func(data map[string]any) {
			data["feedbackPayload"].(map[string]any)["verdict"] = "pass"
		}},
		{name: "gate feedback requires input sequence", eventType: "gate_verdict", valid: schema2RepairGateVerdictData(), mutate: func(data map[string]any) {
			delete(data["feedbackPayload"].(map[string]any)["evaluatedInput"].(map[string]any), "gateInputSeq")
		}},
		{name: "gate feedback reason is bounded", eventType: "gate_verdict", valid: schema2RepairGateVerdictData(), mutate: func(data map[string]any) {
			data["feedbackPayload"].(map[string]any)["reason"] = strings.Repeat("x", 1<<20)
		}},
		{name: "artifact source rejects unknown kind", eventType: "artifact_attached", valid: schema2RepairArtifactAttachedData(), mutate: func(data map[string]any) {
			data["source"].(map[string]any)["kind"] = "path"
		}},
		{name: "artifact source requires gate identity", eventType: "artifact_attached", valid: schema2RepairArtifactAttachedData(), mutate: func(data map[string]any) {
			delete(data["source"].(map[string]any), "gateId")
		}},
		{name: "artifact attached forbids tool source", eventType: "artifact_attached", valid: schema2RepairArtifactAttachedData(), mutate: func(data map[string]any) {
			data["source"] = map[string]any{"kind": "tool", "toolLeaseId": "toollease_01KXNP6VY3227H78329V52CKF8", "nodeId": "tool_normalize"}
		}},
		{name: "fixed system prompt rejects wrong template", eventType: "human_input_requested", valid: schema2RepairHumanRequestData(), mutate: func(data map[string]any) {
			data["promptProjection"].(map[string]any)["templateId"] = "gate-human-verdict-v2"
		}},
		{name: "fixed system prompt rejects authored classification", eventType: "human_input_requested", valid: schema2RepairHumanRequestData(), mutate: func(data map[string]any) {
			data["promptProjection"].(map[string]any)["classification"] = "authored_config"
		}},
		{name: "choice projections require exact pass and fail", eventType: "human_input_requested", valid: schema2RepairHumanRequestData(), mutate: func(data map[string]any) {
			delete(data["choiceProjections"].(map[string]any), "fail")
		}},
		{name: "choice label binds exact template", eventType: "human_input_requested", valid: schema2RepairHumanRequestData(), mutate: func(data map[string]any) {
			data["choiceProjections"].(map[string]any)["pass"].(map[string]any)["templateId"] = "gate-human-fail-v1"
		}},
		{name: "choice projections reject unknown choice", eventType: "human_input_requested", valid: schema2RepairHumanRequestData(), mutate: func(data map[string]any) {
			data["choiceProjections"].(map[string]any)["later"] = map[string]any{"classification": "fixed_system", "sourceKind": "human_choice", "templateId": "gate-human-later-v1"}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			valid := schema2Event(projectionTestRunID, 2, test.eventType, cloneAny(test.valid).(map[string]any))
			schema2RepairDecodeSafeEvent(t, valid)
			invalid := cloneAny(valid).(map[string]any)
			test.mutate(invalid["data"].(map[string]any))
			raw, err := decodeProjectionEvent(canonicalJSON(t, invalid), CanonicalRunSourceSchema2, projectionTestRunID)
			if err != nil {
				t.Fatalf("invalid nested fixture failed before nested sanitizer: %v", err)
			}
			if safe, err := sanitizeSchema2Event(raw); err == nil {
				t.Fatalf("invalid nested %s projection was exposed: %#v", test.eventType, safe)
			}
		})
	}
}

func TestSchema2NestedProjectionClosedEnumsAcceptEveryFrozenLiteral(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		variants  []map[string]any
	}{
		{name: "tool result status", eventType: "tool_result", variants: schema2RepairVariants(schema2RepairToolResultData(), "status", "ok", "error", "timeout")},
		{name: "node output status", eventType: "node_output", variants: schema2RepairVariants(schema2RepairNodeOutputData(projectionTestMissionID), "status", "done", "needs-review", "blocked", "failed")},
		{name: "code gate verdict", eventType: "gate_kind_result", variants: []map[string]any{schema2RepairGateKindResultVerdictData("pass"), schema2RepairGateKindResultVerdictData("fail")}},
		{name: "judge verdict", eventType: "judge_result", variants: []map[string]any{schema2RepairJudgeResultVerdictData("pass"), schema2RepairJudgeResultVerdictData("fail")}},
		{name: "gate evidence kind", eventType: "gate_kind_result", variants: []map[string]any{
			schema2RepairWithEvidence(schema2RepairGateKindResultData(), map[string]any{"kind": "artifact", "artifactId": "artifact_report"}),
			schema2RepairWithEvidence(schema2RepairGateKindResultData(), map[string]any{"kind": "ledger", "seq": 1}),
			schema2RepairWithEvidence(schema2RepairGateKindResultData(), map[string]any{"kind": "text", "text": "clean"}),
		}},
		{name: "non-tool artifact source kind", eventType: "artifact_attached", variants: []map[string]any{
			schema2RepairWithArtifactSource(map[string]any{"kind": "slot", "dispatchId": "dsp_01KXNP6VY3227H78329V52CKF8", "nodeId": projectionTestFormationID, "slotId": "slot_worker"}),
			schema2RepairWithArtifactSource(map[string]any{"kind": "gate", "gateId": projectionTestGateID, "gateAttempt": 1}),
			schema2RepairWithArtifactSource(map[string]any{"kind": "system", "sourceId": "system_report"}),
		}},
		{name: "artifact availability", eventType: "artifact_attached", variants: []map[string]any{
			schema2RepairArtifactAttachedData(),
			schema2RepairWithArtifactProjection(schema2RepairUnavailableArtifactProjection("unavailable")),
			schema2RepairWithArtifactProjection(schema2RepairUnavailableArtifactProjection("redacted")),
			schema2RepairWithArtifactProjection(schema2RepairUnavailableArtifactProjection("expired")),
		}},
		{name: "payload kind", eventType: "node_output", variants: []map[string]any{
			schema2RepairNodeOutputData(projectionTestMissionID),
			schema2RepairNodeOutputWithPayload(map[string]any{"kind": "unavailable", "code": "formation_needs_review", "message": "Formation requires review", "retryable": true}),
			schema2RepairNodeOutputWithPayload(map[string]any{"kind": "error", "code": "invalid_formation_outputs", "message": "Formation outputs do not match the declared ports", "retryable": true}),
			schema2RepairNodeOutputWithPayload(map[string]any{"kind": "gate_feedback", "feedback": cloneAny(schema2RepairGateVerdictData()["feedbackPayload"])}),
		}},
		{name: "gate result kind", eventType: "gate_kind_result", variants: []map[string]any{schema2RepairGateKindResultData(), schema2RepairFormationGateKindResultData()}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for index, data := range test.variants {
				t.Run(strconv.Itoa(index), func(t *testing.T) {
					event := schema2Event(projectionTestRunID, 20, test.eventType, cloneAny(data).(map[string]any))
					if _, safe := schema2RepairDecodeSafeEvent(t, event); safe == nil {
						t.Fatalf("accepted %s enum variant produced nil event", test.eventType)
					}
				})
			}
		})
	}
}

func TestSchema2RemainingNestedProjectionFamiliesAreClosed(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		valid     map[string]any
		mutate    func(map[string]any)
	}{
		{name: "turn result requires turn key", eventType: "slot_result", valid: schema2RepairSlotResultData(t), mutate: func(data map[string]any) {
			delete(data["turnResult"].(map[string]any), "turnKey")
		}},
		{name: "turn result rejects unknown member", eventType: "slot_result", valid: schema2RepairSlotResultData(t), mutate: func(data map[string]any) {
			data["turnResult"].(map[string]any)["capture"] = "private"
		}},
		{name: "turn result phase must match dispatch", eventType: "slot_result", valid: schema2RepairSlotResultData(t), mutate: func(data map[string]any) {
			data["turnResult"].(map[string]any)["phase"] = "solo"
		}},
		{name: "turn result status is closed", eventType: "slot_result", valid: schema2RepairSlotResultData(t), mutate: func(data map[string]any) {
			data["turnResult"].(map[string]any)["status"] = "maybe"
		}},
		{name: "turn result hash matches canonical nested value", eventType: "slot_result", valid: schema2RepairSlotResultData(t), mutate: func(data map[string]any) {
			data["turnResult"].(map[string]any)["reportArtifactId"] = "artifact_changed"
		}},
		{name: "turn result output payload is closed", eventType: "slot_result", valid: schema2RepairSlotResultData(t), mutate: func(data map[string]any) {
			data["turnResult"].(map[string]any)["outputs"] = map[string]any{"port_out": map[string]any{"availability": "available", "exact": true, "payload": map[string]any{"kind": "work", "mediaType": "text/plain", "text": "ok", "token": "private"}}}
		}},
		{name: "formation result requires outputs", eventType: "formation_result", valid: schema2RepairFormationResultData(), mutate: func(data map[string]any) {
			delete(data, "outputs")
		}},
		{name: "formation result status is closed", eventType: "formation_result", valid: schema2RepairFormationResultData(), mutate: func(data map[string]any) {
			data["status"] = "maybe"
		}},
		{name: "formation output hash keys match outputs", eventType: "formation_result", valid: schema2RepairFormationResultData(), mutate: func(data map[string]any) {
			data["outputHashes"] = map[string]any{"port_other": strings.Repeat("a", 64)}
		}},
		{name: "formation output hash matches canonical projection", eventType: "formation_result", valid: schema2RepairFormationResultData(), mutate: func(data map[string]any) {
			data["outputHashes"].(map[string]any)["port_out"] = strings.Repeat("a", 64)
		}},
		{name: "formation contributing result sequences are unique ascending", eventType: "formation_result", valid: schema2RepairFormationResultData(), mutate: func(data map[string]any) {
			data["contributingSlotResultSeqs"] = []any{10, 10}
		}},
		{name: "formation result hash matches canonical result", eventType: "formation_result", valid: schema2RepairFormationResultData(), mutate: func(data map[string]any) {
			data["reportArtifactId"] = "artifact_changed"
		}},
		{name: "retry target requires node id", eventType: "run_blocked", valid: schema2RepairRetryBlockData(), mutate: func(data map[string]any) {
			delete(data["retryTargets"].([]any)[0].(map[string]any), "nodeId")
		}},
		{name: "retry target rejects unknown member", eventType: "run_blocked", valid: schema2RepairRetryBlockData(), mutate: func(data map[string]any) {
			data["retryTargets"].([]any)[0].(map[string]any)["selectivePortReplay"] = true
		}},
		{name: "retry target requires stable unique output ports", eventType: "run_blocked", valid: schema2RepairRetryBlockData(), mutate: func(data map[string]any) {
			data["retryTargets"].([]any)[0].(map[string]any)["outputPortIds"] = []any{"port_out", "port_out"}
		}},
		{name: "retry target forbids delivered edges", eventType: "run_resumed", valid: schema2RepairRetryResumeData(), mutate: func(data map[string]any) {
			data["retryTargets"].([]any)[0].(map[string]any)["deliveredEdges"] = []any{"edge_forbidden"}
		}},
		{name: "gate per-kind map rejects undeclared values", eventType: "gate_verdict", valid: schema2RepairGateVerdictData(), mutate: func(data map[string]any) {
			data["perKind"].(map[string]any)["code"] = "maybe"
		}},
		{name: "gate result sequence map requires declared key set", eventType: "gate_verdict", valid: schema2RepairGateVerdictData(), mutate: func(data map[string]any) {
			delete(data["kindResultSeqs"].(map[string]any), "human")
		}},
		{name: "cancel node snapshot requires exact phase sequence", eventType: "run_cancel_requested", valid: schema2RepairCancelRequestedData(t), mutate: func(data map[string]any) {
			delete(data["openNodeAttempts"].([]any)[0].(map[string]any), "phaseSeq")
		}},
		{name: "cancel node snapshot phase is closed", eventType: "run_cancel_requested", valid: schema2RepairCancelRequestedData(t), mutate: func(data map[string]any) {
			data["openNodeAttempts"].([]any)[0].(map[string]any)["phase"] = "unknown"
		}},
		{name: "cancel slot snapshot is closed", eventType: "run_cancel_requested", valid: schema2RepairCancelRequestedData(t), mutate: func(data map[string]any) {
			data["openSlotDispatches"].([]any)[0].(map[string]any)["targetKey"] = "private"
		}},
		{name: "cancel tool snapshot requires dispatch sequence", eventType: "run_cancel_requested", valid: schema2RepairCancelRequestedData(t), mutate: func(data map[string]any) {
			delete(data["openToolLeases"].([]any)[0].(map[string]any), "dispatchSeq")
		}},
		{name: "tool latest launch is closed", eventType: "run_failure_reconciliation_started", valid: schema2RepairFailureStartedData(t, map[string]any{"kind": "none"}), mutate: func(data map[string]any) {
			data["openToolLeases"].([]any)[0].(map[string]any)["latestLaunch"].(map[string]any)["pid"] = 123
		}},
		{name: "failure cause requires its discriminant identity", eventType: "run_failure_reconciliation_started", valid: schema2RepairFailureStartedData(t, map[string]any{"kind": "slot", "dispatchId": "dsp_01KXNP6VY3227H78329V52CKF8"}), mutate: func(data map[string]any) {
			delete(data["failureCause"].(map[string]any), "dispatchId")
		}},
		{name: "failure cause rejects mixed identities", eventType: "run_failure_reconciliation_started", valid: schema2RepairFailureStartedData(t, map[string]any{"kind": "none"}), mutate: func(data map[string]any) {
			data["failureCause"].(map[string]any)["toolLeaseId"] = "toollease_01KXNP6VY3227H78329V52CKF8"
		}},
		{name: "canceled node disposition is exact", eventType: "run_canceled", valid: schema2RepairRunCanceledData(t), mutate: func(data map[string]any) {
			data["nodeAttemptDispositions"].([]any)[0].(map[string]any)["disposition"] = "failed_non_authorizing"
		}},
		{name: "canceled slot disposition is closed", eventType: "run_canceled", valid: schema2RepairRunCanceledData(t), mutate: func(data map[string]any) {
			data["slotDispatchDispositions"].([]any)[0].(map[string]any)["releaseProof"] = "private"
		}},
		{name: "canceled reconciled tool disposition is exact", eventType: "run_canceled", valid: schema2RepairRunCanceledData(t), mutate: func(data map[string]any) {
			data["reconciledToolLeases"].([]any)[0].(map[string]any)["disposition"] = "launch_fenced_cleaned"
		}},
		{name: "failed tool disposition is closed", eventType: "run_failed", valid: schema2RepairRunFailedData(t), mutate: func(data map[string]any) {
			data["toolLeaseDispositions"].([]any)[0].(map[string]any)["disposition"] = "cleaned"
		}},
		{name: "failed event must be final", eventType: "run_failed", valid: schema2RepairRunFailedData(t), mutate: func(data map[string]any) {
			data["final"] = false
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			valid := schema2Event(projectionTestRunID, 20, test.eventType, cloneAny(test.valid).(map[string]any))
			schema2RepairDecodeSafeEvent(t, valid)
			invalid := cloneAny(valid).(map[string]any)
			test.mutate(invalid["data"].(map[string]any))
			raw, err := decodeProjectionEvent(canonicalJSON(t, invalid), CanonicalRunSourceSchema2, projectionTestRunID)
			if err != nil {
				t.Fatalf("invalid %s nested fixture failed before sanitizer: %v", test.eventType, err)
			}
			if safe, err := sanitizeSchema2Event(raw); err == nil {
				t.Fatalf("invalid %s nested projection was exposed: %#v", test.eventType, safe)
			}
		})
	}
}

func TestSchema2FailureCauseAcceptsEveryFrozenDiscriminant(t *testing.T) {
	causes := []map[string]any{
		{"kind": "slot", "dispatchId": "dsp_01KXNP6VY3227H78329V52CKF8"},
		{"kind": "tool", "toolLeaseId": "toollease_01KXNP6VY3227H78329V52CKF8"},
		{"kind": "error", "errorSeq": 18},
		{"kind": "none"},
	}
	for _, cause := range causes {
		t.Run(cause["kind"].(string), func(t *testing.T) {
			event := schema2Event(projectionTestRunID, 20, "run_failure_reconciliation_started", schema2RepairFailureStartedData(t, cause))
			schema2RepairDecodeSafeEvent(t, event)
		})
	}
}

func TestProjectCanonicalRunValidatesEverySchema2AuthorityDocument(t *testing.T) {
	base := schema2ProjectionInput(t, true)
	tests := []struct {
		name   string
		role   CanonicalInputRole
		mutate func(map[string]any)
	}{
		{name: "registry unknown member", role: CanonicalInputRoleSchema2WorkspaceRegistry, mutate: func(value map[string]any) { value["projectionUnknown"] = true }},
		{name: "registry duplicate workspace entry", role: CanonicalInputRoleSchema2WorkspaceRegistry, mutate: func(value map[string]any) {
			value["entries"] = append(value["entries"].([]any), cloneAny(value["entries"].([]any)[0]))
		}},
		{name: "registry schema is exact", role: CanonicalInputRoleSchema2WorkspaceRegistry, mutate: func(value map[string]any) { value["registrySchema"] = 2 }},
		{name: "registry record revision is positive", role: CanonicalInputRoleSchema2WorkspaceRegistry, mutate: func(value map[string]any) { value["recordRev"] = 0 }},
		{name: "registry prior generation is closed", role: CanonicalInputRoleSchema2WorkspaceRegistry, mutate: func(value map[string]any) {
			value["priorGeneration"] = map[string]any{"recordRev": 1, "sha256": strings.Repeat("a", 64), "path": "/private"}
		}},
		{name: "registry device decimal is canonical", role: CanonicalInputRoleSchema2WorkspaceRegistry, mutate: func(value map[string]any) {
			value["entries"].([]any)[0].(map[string]any)["device"] = "01"
		}},
		{name: "registry root hash has exact grammar", role: CanonicalInputRoleSchema2WorkspaceRegistry, mutate: func(value map[string]any) {
			value["entries"].([]any)[0].(map[string]any)["workspaceRootIdentitySha256"] = "not-a-hash"
		}},
		{name: "bootstrap authority mismatch", role: CanonicalInputRoleSchema2WorkspaceBootstrap, mutate: func(value map[string]any) { value["workspaceAuthorityId"] = "wsa_other" }},
		{name: "bootstrap root hash mismatch", role: CanonicalInputRoleSchema2WorkspaceBootstrap, mutate: func(value map[string]any) { value["workspaceRootIdentitySha256"] = strings.Repeat("f", 64) }},
		{name: "bootstrap schema is exact", role: CanonicalInputRoleSchema2WorkspaceBootstrap, mutate: func(value map[string]any) { value["bootstrapSchema"] = 2 }},
		{name: "bootstrap root encoding is exact", role: CanonicalInputRoleSchema2WorkspaceBootstrap, mutate: func(value map[string]any) { value["rootIdentityEncoding"] = "workspace-root-identity-v2" }},
		{name: "current authority unknown member", role: CanonicalInputRoleSchema2WorkspaceAuthority, mutate: func(value map[string]any) { value["projectionUnknown"] = true }},
		{name: "current authority id mismatch", role: CanonicalInputRoleSchema2WorkspaceAuthority, mutate: func(value map[string]any) { value["workspaceAuthorityId"] = "wsa_other" }},
		{name: "current authority schema is exact", role: CanonicalInputRoleSchema2WorkspaceAuthority, mutate: func(value map[string]any) { value["authoritySchema"] = 3 }},
		{name: "current authority revision is positive", role: CanonicalInputRoleSchema2WorkspaceAuthority, mutate: func(value map[string]any) { value["recordRev"] = 0 }},
		{name: "current authority predecessor hash has exact grammar", role: CanonicalInputRoleSchema2WorkspaceAuthority, mutate: func(value map[string]any) {
			value["priorGeneration"].(map[string]any)["sha256"] = "not-a-hash"
		}},
		{name: "current authority writer fence is positive", role: CanonicalInputRoleSchema2WorkspaceAuthority, mutate: func(value map[string]any) { value["nextWriterFence"] = 0 }},
		{name: "current authority policy ref is closed", role: CanonicalInputRoleSchema2WorkspaceAuthority, mutate: func(value map[string]any) {
			value["admissionPolicyRef"].(map[string]any)["configuredPath"] = "/private"
		}},
		{name: "run bootstrap unknown member", role: CanonicalInputRoleSchema2RunBootstrap, mutate: func(value map[string]any) { value["projectionUnknown"] = true }},
		{name: "run bootstrap run mismatch", role: CanonicalInputRoleSchema2RunBootstrap, mutate: func(value map[string]any) { value["runId"] = projectionTestOtherRunID }},
		{name: "run bootstrap schema is exact", role: CanonicalInputRoleSchema2RunBootstrap, mutate: func(value map[string]any) { value["runBootstrapSchema"] = 2 }},
		{name: "run bootstrap graph encoding is exact", role: CanonicalInputRoleSchema2RunBootstrap, mutate: func(value map[string]any) { value["graphSnapshotEncoding"] = "json" }},
		{name: "run bootstrap bindings hash has exact grammar", role: CanonicalInputRoleSchema2RunBootstrap, mutate: func(value map[string]any) { value["privateBindingsSha256"] = "not-a-hash" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := cloneCanonicalInput(base)
			document := decodeCanonicalObject(t, canonicalDocumentByRole(t, input, test.role).Bytes)
			test.mutate(document)
			input = replaceCanonicalDocument(t, input, test.role, canonicalJSON(t, document))
			if projection, err := ProjectCanonicalRun(input); err == nil {
				t.Fatalf("noncanonical %s document projected: %#v", test.role, ProjectRunView(projection))
			} else {
				requireProjectionError(t, err, ErrRunProjectionInvalid)
			}
		})
	}
}

func TestProjectCanonicalRunRejectsDuplicateAuthorityJSONMembers(t *testing.T) {
	for _, test := range []struct {
		name      string
		duplicate func(*testing.T, CanonicalRunReadInput) CanonicalRunReadInput
	}{
		{name: "registry", duplicate: func(t *testing.T, input CanonicalRunReadInput) CanonicalRunReadInput {
			return schema2RepairDuplicateRoleJSONMember(t, input, CanonicalInputRoleSchema2WorkspaceRegistry, 0, "registrySchema")
		}},
		{name: "bootstrap", duplicate: func(t *testing.T, input CanonicalRunReadInput) CanonicalRunReadInput {
			return schema2RepairDuplicateRoleJSONMember(t, input, CanonicalInputRoleSchema2WorkspaceBootstrap, 0, "bootstrapSchema")
		}},
		{name: "authority", duplicate: func(t *testing.T, input CanonicalRunReadInput) CanonicalRunReadInput {
			return schema2RepairDuplicateRoleJSONMember(t, input, CanonicalInputRoleSchema2WorkspaceAuthority, 0, "authoritySchema")
		}},
		{name: "admission policy", duplicate: func(t *testing.T, input CanonicalRunReadInput) CanonicalRunReadInput {
			index := schema2RepairRoleDocumentIndex(input, CanonicalInputRoleSchema2AdmissionPolicy, 1)
			if index < 0 {
				t.Fatal("configured admission policy fixture missing")
			}
			policy := schema2RepairDuplicateTopLevelJSONMember(t, input.Documents[index].Bytes, "policySchema")
			return schema2RepairReplaceConfiguredPolicyAndReferences(t, input, policy)
		}},
		{name: "run bootstrap", duplicate: func(t *testing.T, input CanonicalRunReadInput) CanonicalRunReadInput {
			return schema2RepairDuplicateRoleJSONMember(t, input, CanonicalInputRoleSchema2RunBootstrap, 0, "runBootstrapSchema")
		}},
		{name: "ledger", duplicate: func(t *testing.T, input CanonicalRunReadInput) CanonicalRunReadInput {
			index := schema2RepairRoleDocumentIndex(input, CanonicalInputRoleSchema2Ledger, 0)
			if index < 0 {
				t.Fatal("schema-2 ledger fixture missing")
			}
			ledger := input.Documents[index].Bytes
			lineEnd := bytes.IndexByte(ledger, '\n')
			if lineEnd < 0 {
				t.Fatalf("schema-2 ledger fixture has no complete row: %q", ledger)
			}
			first := schema2RepairDuplicateTopLevelJSONMember(t, ledger[:lineEnd], "schema")
			duplicate := append(append([]byte(nil), first...), ledger[lineEnd:]...)
			input.Documents[index] = canonicalInputDocument(CanonicalInputRoleSchema2Ledger, duplicate)
			return input
		}},
		{name: "command", duplicate: func(t *testing.T, input CanonicalRunReadInput) CanonicalRunReadInput {
			return schema2RepairDuplicateRoleJSONMember(t, input, CanonicalInputRoleSchema2CommandRecord, 0, "commandSchema")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := schema2ProjectionInput(t, true)
			input = test.duplicate(t, input)
			schema2RepairRequireProjectionInvalid(t, input, "duplicate "+test.name+" JSON member")
		})
	}
}

func schema2RepairDuplicateRoleJSONMember(t *testing.T, input CanonicalRunReadInput, role CanonicalInputRole, occurrence int, key string) CanonicalRunReadInput {
	t.Helper()
	index := schema2RepairRoleDocumentIndex(input, role, occurrence)
	if index < 0 {
		t.Fatalf("fixture lacks %s occurrence %d", role, occurrence)
	}
	duplicate := schema2RepairDuplicateTopLevelJSONMember(t, input.Documents[index].Bytes, key)
	input.Documents[index] = canonicalInputDocument(role, duplicate)
	return input
}

func schema2RepairDuplicateTopLevelJSONMember(t *testing.T, raw []byte, key string) []byte {
	t.Helper()
	object := decodeCanonicalObject(t, raw)
	value, ok := object[key]
	if !ok {
		t.Fatalf("fixture lacks duplicate target %q: %s", key, raw)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		t.Fatalf("duplicate target %q is not a complete JSON object: %s", key, raw)
	}
	keyJSON := canonicalJSON(t, key)
	valueJSON := canonicalJSON(t, value)
	duplicate := append([]byte(nil), trimmed[:len(trimmed)-1]...)
	duplicate = append(duplicate, ',')
	duplicate = append(duplicate, keyJSON...)
	duplicate = append(duplicate, ':')
	duplicate = append(duplicate, valueJSON...)
	duplicate = append(duplicate, '}')
	if !json.Valid(duplicate) {
		t.Fatalf("duplicate target %q produced invalid JSON: %s", key, duplicate)
	}
	return duplicate
}

func TestProjectCanonicalRunValidatesAdmissionPolicyClosedSemantics(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "unknown member", mutate: func(policy map[string]any) { policy["projectionUnknown"] = true }},
		{name: "schema is exact", mutate: func(policy map[string]any) { policy["policySchema"] = 2 }},
		{name: "revision is positive", mutate: func(policy map[string]any) { policy["policyRev"] = 0 }},
		{name: "state is closed", mutate: func(policy map[string]any) { policy["state"] = "permissive" }},
		{name: "configured requires active limit", mutate: func(policy map[string]any) { delete(policy, "maxActiveRuns") }},
		{name: "configured requires positive active limit", mutate: func(policy map[string]any) { policy["maxActiveRuns"] = 0 }},
		{name: "configured requires queued limit", mutate: func(policy map[string]any) { delete(policy, "maxQueuedRuns") }},
		{name: "prior policy hash has exact grammar", mutate: func(policy map[string]any) { policy["priorPolicySha256"] = "not-a-hash" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := schema2ProjectionInput(t, true)
			policy := schema2RepairConfiguredPolicy(t, input)
			test.mutate(policy)
			input = schema2RepairReplaceConfiguredPolicyAndReferences(t, input, canonicalJSON(t, policy))
			schema2RepairRequireProjectionInvalid(t, input, "invalid configured admission policy")
		})
	}

	t.Run("disabled policy forbids configured limits", func(t *testing.T) {
		input := schema2ProjectionInput(t, true)
		index := schema2RepairRoleDocumentIndex(input, CanonicalInputRoleSchema2AdmissionPolicy, 0)
		policy := decodeCanonicalObject(t, input.Documents[index].Bytes)
		policy["maxActiveRuns"], policy["maxQueuedRuns"] = 1, 1
		input.Documents[index] = canonicalInputDocument(CanonicalInputRoleSchema2AdmissionPolicy, canonicalJSON(t, policy))
		schema2RepairRequireProjectionInvalid(t, input, "disabled policy with configured limits")
	})
}

func TestProjectCanonicalRunValidatesGraphBindingsAndCommandLinkage(t *testing.T) {
	t.Run("graph board identity must bind the run start", func(t *testing.T) {
		input := schema2ProjectionInput(t, true)
		graph := canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2GraphSnapshot).Bytes
		graph = bytes.Replace(graph, []byte(`id = "`+projectionTestBoardID+`"`), []byte(`id = "brd_other"`), 1)
		input = schema2RepairReplaceGraphAndHashes(t, input, graph)
		if projection, err := ProjectCanonicalRun(input); err == nil {
			t.Fatalf("mismatched graph board projected: %#v", ProjectRunView(projection).Identity)
		}
	})

	t.Run("private bindings run identity must match", func(t *testing.T) {
		input, _ := schema2OpenDispatchLifecycleInput(t, false)
		bindings := canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2PrivateBindings).Bytes
		bindings = bytes.Replace(bindings, []byte(`runId = "`+projectionTestRunID+`"`), []byte(`runId = "`+projectionTestOtherRunID+`"`), 1)
		input = schema2RepairReplaceBindingsAndHashes(t, input, bindings)
		if projection, err := ProjectCanonicalRun(input); err == nil {
			t.Fatalf("mismatched private bindings projected: %#v", ProjectRunView(projection).Sessions)
		}
	})

	for _, kind := range []string{"cancel", "verdict"} {
		t.Run(kind+" effect sequence must name its event", func(t *testing.T) {
			input := schema2ProjectionInput(t, true)
			sequence := uint64(3)
			payload := canonicalCommandPayload(kind, projectionTestRunID)
			if kind == "cancel" {
				payload["expectedLastSeq"] = sequence - 1
			} else {
				payload["requestedSeq"] = sequence - 1
			}
			history := schema2CommandHistoryForPayload(t, projectionTestOtherCmdID, kind, "applied", payload, schema2CommandOutcome{
				runID: projectionTestRunID, effectSeq: sequence + 10, admittedWriterFence: 1, stateWriterFence: 1, outcomeWriterFence: 1,
			})
			terminal := decodeCanonicalObject(t, history.terminal)
			var event map[string]any
			if kind == "cancel" {
				event = schema2Event(projectionTestRunID, sequence, "run_cancel_requested", map[string]any{
					"commandId": projectionTestOtherCmdID, "commandPayloadSha256": terminal["commandPayloadSha256"], "reason": "stop", "requestedBy": "human:test",
					"openNodeAttempts": []any{}, "openSlotDispatches": []any{}, "openToolLeases": []any{},
				})
			} else {
				event = schema2Event(projectionTestRunID, sequence, "human_verdict_recorded", map[string]any{
					"commandId": projectionTestOtherCmdID, "commandPayloadSha256": terminal["commandPayloadSha256"], "gateId": projectionTestGateID,
					"gateAttempt": 1, "nodeId": projectionTestGateID, "verdict": "pass", "reason": "approved", "requestedSeq": sequence - 1, "decidedBy": "human:test",
				})
			}
			input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema2CommandRecord, history.terminal))
			events := canonicalLedgerEvents(t, input)
			events = append(events, event)
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
			if projection, err := ProjectCanonicalRun(input); err == nil {
				t.Fatalf("%s command with wrong effectSeq projected: %#v", kind, ProjectRunView(projection))
			}
		})
	}
}

func TestProjectCanonicalRunRejectsIncompleteGraphAndBindingsSemantics(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{name: "graph slug differs from run start", mutate: func(graph []byte) []byte {
			return bytes.Replace(graph, []byte(`slug = "`+projectionTestBoardSlug+`"`), []byte(`slug = "other"`), 1)
		}},
		{name: "graph revision differs from run start", mutate: func(graph []byte) []byte {
			return bytes.Replace(graph, []byte("rev = 7"), []byte("rev = 8"), 1)
		}},
		{name: "selected root is absent", mutate: func(graph []byte) []byte {
			return bytes.Replace(graph, []byte(`id = "`+projectionTestMissionID+`"`), []byte(`id = "mis_other"`), 1)
		}},
		{name: "manifest hash differs from authored bytes", mutate: func(graph []byte) []byte {
			return bytes.Replace(graph, []byte(`sha256 = "`+projectionSHA256([]byte("Project the run"))+`"`), []byte(`sha256 = "`+strings.Repeat("a", 64)+`"`), 1)
		}},
		{name: "unknown graph key", mutate: func(graph []byte) []byte {
			return append(graph, []byte("\nprojectionUnknown = true\n")...)
		}},
		{name: "duplicate graph scalar", mutate: func(graph []byte) []byte {
			return bytes.Replace(graph, []byte("schema = 2\n"), []byte("schema = 2\nschema = 2\n"), 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := schema2ProjectionInput(t, true)
			graph := append([]byte(nil), canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2GraphSnapshot).Bytes...)
			input = schema2RepairReplaceGraphAndHashes(t, input, test.mutate(graph))
			schema2RepairRequireProjectionInvalid(t, input, "invalid graph snapshot")
		})
	}

	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{name: "unknown top-level bindings key", mutate: func(bindings []byte) []byte {
			return bytes.Replace(bindings, []byte("boardRev = 7\n"), []byte("boardRev = 7\nprojectionUnknown = true\n"), 1)
		}},
		{name: "unknown bindings section", mutate: func(bindings []byte) []byte {
			return append(bindings, []byte("\n[[privateRoute]]\ntargetKey = \"private\"\n")...)
		}},
		{name: "unrelated malformed bindings line", mutate: func(bindings []byte) []byte {
			return append(bindings, []byte("\nthis is not toml\n")...)
		}},
		{name: "duplicate binding member", mutate: func(bindings []byte) []byte {
			return bytes.Replace(bindings, []byte(`bindingId = "binding_worker"`), []byte("bindingId = \"binding_worker\"\nbindingId = \"binding_worker\""), 1)
		}},
		{name: "duplicate binding identity", mutate: func(bindings []byte) []byte {
			return bytes.Replace(bindings, []byte(`bindingId = "binding_reviewer"`), []byte(`bindingId = "binding_worker"`), 1)
		}},
		{name: "binding node is absent from graph", mutate: func(bindings []byte) []byte {
			return bytes.Replace(bindings, []byte(`nodeId = "`+projectionTestFormationID+`"`), []byte(`nodeId = "fmn_missing"`), 1)
		}},
		{name: "binding slot is absent from node", mutate: func(bindings []byte) []byte {
			return bytes.Replace(bindings, []byte(`slotId = "slot_worker"`), []byte(`slotId = "slot_missing"`), 1)
		}},
		{name: "binding fingerprint has invalid grammar", mutate: func(bindings []byte) []byte {
			return bytes.Replace(bindings, []byte(`targetFingerprint = "`+strings.Repeat("a", 64)+`"`), []byte(`targetFingerprint = "not-a-hash"`), 1)
		}},
		{name: "binding run identity differs", mutate: func(bindings []byte) []byte {
			return bytes.Replace(bindings, []byte(`runId = "`+projectionTestRunID+`"`), []byte(`runId = "`+projectionTestOtherRunID+`"`), 1)
		}},
		{name: "binding board revision differs", mutate: func(bindings []byte) []byte {
			return bytes.Replace(bindings, []byte("boardRev = 7"), []byte("boardRev = 8"), 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, _ := schema2OpenDispatchLifecycleInput(t, false)
			bindings := append([]byte(nil), canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2PrivateBindings).Bytes...)
			input = schema2RepairReplaceBindingsAndHashes(t, input, test.mutate(bindings))
			schema2RepairRequireProjectionInvalid(t, input, "invalid private bindings")
		})
	}
}

func TestProjectCanonicalRunRejectsLedgerAndCommandAuthorityDivergence(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]map[string]any)
	}{
		{name: "ledger event schema differs", mutate: func(events []map[string]any) { events[0]["schema"] = 1 }},
		{name: "ledger authority schema differs", mutate: func(events []map[string]any) { events[0]["authoritySchema"] = 3 }},
		{name: "ledger run identity differs", mutate: func(events []map[string]any) { events[0]["runId"] = projectionTestOtherRunID }},
		{name: "ledger writer fence is zero", mutate: func(events []map[string]any) { events[0]["writerFence"] = 0 }},
		{name: "ledger sequence has a gap", mutate: func(events []map[string]any) { events[len(events)-1]["seq"] = 20 }},
		{name: "ledger start graph hash differs", mutate: func(events []map[string]any) {
			events[0]["data"].(map[string]any)["graphSnapshotSha256"] = strings.Repeat("a", 64)
		}},
		{name: "ledger start bindings hash differs", mutate: func(events []map[string]any) {
			events[0]["data"].(map[string]any)["privateBindingsSha256"] = strings.Repeat("a", 64)
		}},
		{name: "ledger start command identity differs", mutate: func(events []map[string]any) {
			events[0]["data"].(map[string]any)["admissionCommandId"] = projectionTestOtherCmdID
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := schema2ProjectionInput(t, true)
			events := canonicalLedgerEvents(t, input)
			test.mutate(events)
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
			schema2RepairRequireProjectionInvalid(t, input, "divergent schema-2 ledger")
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "command unknown member", mutate: func(record map[string]any) { record["targetKey"] = "private" }},
		{name: "command encoding differs", mutate: func(record map[string]any) { record["commandEncoding"] = "run-command-json-v1" }},
		{name: "command id differs", mutate: func(record map[string]any) { record["commandId"] = projectionTestOtherCmdID }},
		{name: "command payload hash differs", mutate: func(record map[string]any) { record["commandPayloadSha256"] = strings.Repeat("a", 64) }},
		{name: "command predecessor hash differs", mutate: func(record map[string]any) {
			record["priorGeneration"].(map[string]any)["sha256"] = strings.Repeat("a", 64)
		}},
		{name: "command applied run differs", mutate: func(record map[string]any) { record["runId"] = projectionTestOtherRunID }},
		{name: "command decision policy differs", mutate: func(record map[string]any) {
			record["decisionAdmissionPolicyRef"].(map[string]any)["policySha256"] = strings.Repeat("a", 64)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := schema2ProjectionInput(t, true)
			record := decodeCanonicalObject(t, canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2CommandRecord).Bytes)
			test.mutate(record)
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2CommandRecord, canonicalJSON(t, record))
			schema2RepairRequireProjectionInvalid(t, input, "divergent schema-2 command")
		})
	}
}

func TestStoreReadCanonicalRunRejectsReturnedRunSubstitution(t *testing.T) {
	other := schema1ProjectionInput(t, schema1StartedEvent(projectionTestOtherRunID))
	other.RunID = projectionTestOtherRunID
	store := NewStore(t.TempDir())
	store.canonicalRunAuthorityReader = &countingCanonicalRunReader{input: other}
	if projection, err := store.ReadCanonicalRun(projectionTestRunID); err == nil {
		t.Fatalf("requested run %s returned substituted run %s: %#v", projectionTestRunID, projectionTestOtherRunID, ProjectRunView(projection))
	} else {
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	}
}

func TestRuntimeStoreSchema2ClaimNeverFallsBackToExistingSchema1Run(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	bindRuntimeAuthorityFixtureToOpenedWorkspace(t, &fixture, fixture.workspace)
	input := schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID))
	runDir := filepath.Join(fixture.workspace, ".formations", "runs", projectionTestBoardSlug)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(runDir, projectionTestRunID+".ndjson"), string(canonicalDocumentByRole(t, input, CanonicalInputRoleSchema1Ledger).Bytes))
	writeFixture(t, filepath.Join(runDir, projectionTestRunID+".snapshot.toml"), string(canonicalDocumentByRole(t, input, CanonicalInputRoleSchema1GraphSnapshot).Bytes))
	writeFixture(t, filepath.Join(runDir, projectionTestRunID+".bindings.toml"), string(canonicalDocumentByRole(t, input, CanonicalInputRoleSchema1BindingsSnapshot).Bytes))

	store := NewRuntimeStore(fixture.workspace, filepath.Dir(fixture.root))
	if projection, err := store.ReadCanonicalRun(projectionTestRunID); !errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
		t.Fatalf("guarded schema-2 claim with schema-1 ledger = projection:%#v err:%v, want typed no-fallback rejection", ProjectRunView(projection), err)
	}
}

func TestProjectCanonicalRunSelectsOnlyRootReachableGraph(t *testing.T) {
	t.Run("schema 1", func(t *testing.T) {
		input := schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID))
		snapshot := canonicalDocumentByRole(t, input, CanonicalInputRoleSchema1GraphSnapshot).Bytes
		snapshot = append(snapshot, []byte(`

[[formation]]
id = "fmn_disconnected"
type = "solo"
title = "Disconnected"
[[formation.input]]
id = "in"
label = "Input"
[[formation.output]]
id = "out"
label = "Output"
[[formation.slot]]
id = "slot_disconnected"
label = "Disconnected"
agentId = "disconnected"
harness = "codex"
controller = true

[[gate]]
id = "gate_disconnected"
title = "Disconnected gate"
kinds = ["human"]
criterion = "Never reached"
`)...)
		input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema1GraphSnapshot, snapshot)
		schema2RepairAssertSelectedRootProjection(t, ProjectRunView(mustProjectCanonicalFixture(t, input)), 3, "fmn_disconnected", "gate_disconnected")
	})

	t.Run("schema 2", func(t *testing.T) {
		input := schema2ProjectionInput(t, true)
		graph := append([]byte(nil), canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2GraphSnapshot).Bytes...)
		graph = append(graph, []byte(`

[[formation]]
id = "fmn_disconnected"
type = "solo"
title = "Disconnected"
[[formation.input]]
id = "in"
label = "Input"
[[formation.output]]
id = "out"
label = "Output"
[[formation.slot]]
id = "slot_disconnected"
label = "Disconnected"
agentId = "disconnected"
harness = "codex"
controller = true

[[gate]]
id = "gate_disconnected"
title = "Disconnected gate"
kinds = ["human"]
criterion = ""
`)...)
		input = schema2RepairReplaceGraphAndHashes(t, input, graph)
		schema2RepairAssertSelectedRootProjection(t, ProjectRunView(mustProjectCanonicalFixture(t, input)), 1, "fmn_disconnected", "gate_disconnected")
	})
}

func TestProjectCommandReceiptPreservesFullUint64WriterFences(t *testing.T) {
	for _, state := range []string{"applied", "rejected"} {
		t.Run(state, func(t *testing.T) {
			fence := uint64(math.MaxUint64)
			payload := canonicalCommandPayload("cancel", projectionTestRunID)
			input := schema2RepairWideFenceCommandInput(t, state, fence, payload)
			receipt, err := ProjectCommandReceipt(input)
			if err != nil {
				t.Fatalf("project full-uint64 writer fence: %v", err)
			}
			var encoded map[string]any
			if err := json.Unmarshal(mustMarshalJSON(t, receipt), &encoded); err != nil {
				t.Fatal(err)
			}
			if got := encoded["outcomeWriterFence"]; got != strconv.FormatUint(fence, 10) {
				t.Fatalf("outcomeWriterFence = %#v, want exact decimal %d", got, fence)
			}
		})
	}
}

func TestProjectCommandReceiptRejectsNoncanonicalUint64WriterFences(t *testing.T) {
	badValues := []string{"", "0", "+1", "01", " 1", "1 ", "1.0", "-1", "18446744073709551616"}
	for _, state := range []string{"applied", "rejected"} {
		for _, field := range []string{"admittedWriterFence", "stateWriterFence", "outcomeWriterFence"} {
			for _, bad := range badValues {
				name := state + "/" + field + "/" + strconv.Quote(bad)
				t.Run(name, func(t *testing.T) {
					values := map[string]string{"admittedWriterFence": "1", "stateWriterFence": "1", "outcomeWriterFence": "1"}
					values[field] = bad
					input := schema2RepairFenceTextCommandInput(t, state, values["admittedWriterFence"], values["stateWriterFence"], values["outcomeWriterFence"], canonicalCommandPayload("cancel", projectionTestRunID))
					if receipt, err := ProjectCommandReceipt(input); err == nil {
						t.Fatalf("noncanonical %s=%q projected: %#v", field, bad, receipt)
					} else {
						requireProjectionError(t, err, ErrRunCommandNotTerminal)
					}
				})
			}
		}
	}
}

func TestProjectCommandReceiptRejectsWriterFenceTypeAndOrderingViolations(t *testing.T) {
	for _, test := range []struct {
		name     string
		admitted string
		state    string
		outcome  string
	}{
		{name: "state fence precedes admission", admitted: "2", state: "1", outcome: "1"},
		{name: "outcome fence follows state", admitted: "1", state: "1", outcome: "2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := schema2RepairFenceTextCommandInput(t, "applied", test.admitted, test.state, test.outcome, canonicalCommandPayload("cancel", projectionTestRunID))
			if receipt, err := ProjectCommandReceipt(input); err == nil {
				t.Fatalf("unordered writer fences projected: %#v", receipt)
			} else {
				requireProjectionError(t, err, ErrRunCommandNotTerminal)
			}
		})
	}

	for _, field := range []string{"admittedWriterFence", "stateWriterFence", "outcomeWriterFence"} {
		t.Run("numeric JSON "+field+" is not a decimal string", func(t *testing.T) {
			values := map[string]any{"admittedWriterFence": "1", "stateWriterFence": "1", "outcomeWriterFence": "1"}
			values[field] = 1
			input := schema2RepairFenceCommandInput(t, "applied", values["admittedWriterFence"], values["stateWriterFence"], values["outcomeWriterFence"], canonicalCommandPayload("cancel", projectionTestRunID))
			if receipt, err := ProjectCommandReceipt(input); err == nil {
				t.Fatalf("numeric %s projected: %#v", field, receipt)
			} else {
				requireProjectionError(t, err, ErrRunCommandNotTerminal)
			}
		})
	}
}

func startSchema2RepairAttempt(nodeID, kind string) func(*projectionState) {
	return func(state *projectionState) {
		schema2RepairStartAttempt(state, nodeID, kind, 1, 2)
	}
}

func prepareSchema2RepairGate(state *projectionState) {
	schema2RepairStartAttempt(state, projectionTestGateID, "gate", 1, 2)
	schema2RepairStartGate(state, projectionTestGateID, 1, 3)
}

func prepareSchema2RepairJudgeGate(state *projectionState) {
	prepareSchema2RepairGate(state)
	schema2RepairStartAttempt(state, projectionTestFormationID, "formation", 1, 4)
}

func prepareSchema2RepairHumanGate(state *projectionState) {
	prepareSchema2RepairGate(state)
	gate := state.ensureGate(projectionTestGateID, 1)
	gate.Status = "waiting_human"
	gate.RequestSeq = 4
}

func schema2RepairStructuralFingerprint(t *testing.T, view RunView) []byte {
	t.Helper()
	view.Cursor = 0
	view.Audit = RunAudit{}
	view.Identity.Epoch = 0
	return mustMarshalJSON(t, view)
}

func schema2RepairAssertSelectedRootProjection(t *testing.T, view RunView, wantNodes int, forbidden ...string) {
	t.Helper()
	raw := mustMarshalJSON(t, view)
	for _, identity := range forbidden {
		if bytes.Contains(raw, []byte(identity)) {
			t.Fatalf("disconnected board element %q leaked into a structural collection: %s", identity, raw)
		}
	}
	if len(view.Nodes) != wantNodes || len(view.Attempts) != 0 || len(view.Gates) != 0 || len(view.Outputs) != 0 || len(view.Artifacts) != 0 || len(view.Blocks) != 0 || len(view.Escalations) != 0 || len(view.Sessions) != 0 {
		t.Fatalf("selected-root structural cardinality inflated: nodes=%d attempts=%d gates=%d outputs=%d artifacts=%d blocks=%d escalations=%d sessions=%d", len(view.Nodes), len(view.Attempts), len(view.Gates), len(view.Outputs), len(view.Artifacts), len(view.Blocks), len(view.Escalations), len(view.Sessions))
	}
}

func schema2RepairCloneView(t *testing.T, view RunView) RunView {
	t.Helper()
	var result RunView
	if err := json.Unmarshal(mustMarshalJSON(t, view), &result); err != nil {
		t.Fatalf("clone run view: %v", err)
	}
	return result
}

func schema2RepairCloneState(t *testing.T, state projectionState) projectionState {
	t.Helper()
	result := state
	result.view = schema2RepairCloneView(t, state.view)
	result.nodeIndex = schema2RepairCloneIndex(state.nodeIndex)
	result.attemptIndex = schema2RepairCloneIndex(state.attemptIndex)
	result.gateIndex = schema2RepairCloneIndex(state.gateIndex)
	return result
}

func schema2RepairCloneIndex(source map[string]int) map[string]int {
	result := make(map[string]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func schema2RepairStartAttempt(state *projectionState, nodeID, kind string, attempt, sequence uint64) {
	view := state.startAttempt(nodeID, attempt, sequence, []SafeInputIdentity{})
	if view == nil {
		return
	}
	node := state.node(nodeID)
	node.Kind = kind
	ref := RunAttemptRef{NodeID: nodeID, Attempt: attempt}
	for _, existing := range node.Attempts {
		if existing == ref {
			return
		}
	}
	node.Attempts = append(node.Attempts, ref)
}

func schema2RepairStartGate(state *projectionState, gateID string, attempt, sequence uint64) {
	schema2RepairStartAttempt(state, gateID, "gate", attempt, sequence)
	state.startGate(gateID, attempt, sequence, SafeInputIdentity{})
	ref := RunGateRef{GateID: gateID, Attempt: attempt}
	node := state.node(gateID)
	if node != nil {
		node.Gates = []RunGateRef{ref}
	}
	if attemptView := schema2RepairAttempt(state, gateID, attempt); attemptView != nil {
		attemptView.Gate = &ref
	}
}

func schema2RepairCompleteAttempt(state *projectionState, nodeID string, attempt uint64, status string, sequence uint64) {
	node := state.node(nodeID)
	node.Status, node.FinalDisposition = status, status
	view := schema2RepairAttempt(state, nodeID, attempt)
	view.Status, view.Disposition, view.CompletedSeq = status, status, sequence
}

func schema2RepairAttempt(state *projectionState, nodeID string, attempt uint64) *RunAttemptView {
	index, ok := state.attemptIndex[projectionAttemptKey(nodeID, attempt)]
	if !ok {
		return nil
	}
	return &state.view.Attempts[index]
}

func schema2RepairAppendOutput(state *projectionState, nodeID string, attempt uint64, portID string, sequence uint64, payload PayloadProjection) {
	ref := RunOutputRef{NodeID: nodeID, Attempt: attempt, PortID: portID}
	state.view.Outputs = append(state.view.Outputs, RunOutputView{NodeID: nodeID, Attempt: attempt, PortID: portID, OutcomeSeq: sequence, PayloadProjection: payload})
	state.node(nodeID).Outputs = append(state.node(nodeID).Outputs, ref)
	schema2RepairAttempt(state, nodeID, attempt).Outputs = append(schema2RepairAttempt(state, nodeID, attempt).Outputs, ref)
}

func schema2RepairPayload(text string) PayloadProjection {
	return PayloadProjection{Availability: "available", Exact: true, Payload: PayloadValue{Kind: "work", MediaType: "text/plain", Text: text}}
}

func schema2RepairAssertTypedHistory(t *testing.T, safe SafeRunEvent, eventType string, wantData map[string]any) {
	t.Helper()
	typeOf := reflect.TypeOf(safe)
	if typeOf.Kind() != reflect.Struct || !strings.HasPrefix(typeOf.Name(), "SafeSchema2") || !strings.HasSuffix(typeOf.Name(), "Event") {
		t.Fatalf("%s did not produce a concrete typed schema-2 event: %T", eventType, safe)
	}
	var exposed map[string]any
	if err := json.Unmarshal(mustMarshalJSON(t, safe), &exposed); err != nil {
		t.Fatalf("decode exposed %s event: %v", eventType, err)
	}
	if exposed["type"] != eventType {
		t.Fatalf("typed event literal = %#v, want %q", exposed["type"], eventType)
	}
	if got, want := canonicalJSON(t, exposed["data"]), canonicalJSON(t, wantData); !bytes.Equal(got, want) {
		t.Fatalf("typed %s public payload mismatch\ngot:  %s\nwant: %s", eventType, got, want)
	}
}

func schema2RepairDecodeSafeEvent(t *testing.T, event map[string]any) (rawProjectionEvent, SafeRunEvent) {
	t.Helper()
	raw, err := decodeProjectionEvent(canonicalJSON(t, event), CanonicalRunSourceSchema2, projectionTestRunID)
	if err != nil {
		t.Fatalf("decode schema-2 repair fixture: %v\n%s", err, canonicalJSON(t, event))
	}
	safe, err := sanitizeSchema2Event(raw)
	if err != nil {
		t.Fatalf("sanitize schema-2 repair fixture: %v\n%s", err, canonicalJSON(t, event))
	}
	return raw, safe
}

func schema2RepairUnsafePublicTypePaths(current reflect.Type, prefix []string, seen map[reflect.Type]bool) []string {
	path := strings.Join(prefix, ".")
	if path == "" {
		path = current.String()
	}
	if current == reflect.TypeOf((*ArtifactProjection)(nil)).Elem() || current == reflect.TypeOf((*SafeOpenDispatch)(nil)).Elem() || current == reflect.TypeOf((*SafeSchema1RunStartedData)(nil)).Elem() {
		return nil
	}
	if current.Kind() == reflect.Interface {
		return []string{path + " (interface/any)"}
	}
	if current.Kind() == reflect.Pointer {
		return schema2RepairUnsafePublicTypePaths(current.Elem(), prefix, seen)
	}
	if current.Kind() == reflect.Slice || current.Kind() == reflect.Array {
		if current.Elem().Kind() == reflect.Uint8 {
			return []string{path + " (raw bytes)"}
		}
		return schema2RepairUnsafePublicTypePaths(current.Elem(), append(prefix, "[]"), seen)
	}
	if current.Kind() == reflect.Map {
		if current.Name() == "" {
			return []string{path + " (unnamed raw map)"}
		}
		if current.Key().Kind() != reflect.String {
			return []string{path + " (non-string map key)"}
		}
		return schema2RepairUnsafePublicTypePaths(current.Elem(), append(prefix, "{}"), seen)
	}
	if current.Kind() != reflect.Struct {
		return nil
	}
	if len(prefix) != 0 && current.Name() == "" {
		return []string{path + " (anonymous struct)"}
	}
	if seen[current] {
		return nil
	}
	seen[current] = true
	defer delete(seen, current)
	var paths []string
	for index := 0; index < current.NumField(); index++ {
		field := current.Field(index)
		paths = append(paths, schema2RepairUnsafePublicTypePaths(field.Type, append(prefix, field.Name), seen)...)
	}
	return paths
}

func schema2RepairWorkProjection(text string) map[string]any {
	return map[string]any{
		"availability": "available", "exact": true,
		"payload": map[string]any{"kind": "work", "mediaType": "text/plain", "text": text},
	}
}

func schema2RepairInputRef() map[string]any {
	return map[string]any{
		"inputId": "input_root", "sourceKind": "edge", "runId": projectionTestRunID,
		"originEdgeId": "edge_root_work", "deliveryEdgeId": "edge_root_work", "sourceNodeId": projectionTestMissionID,
		"sourcePortId": "out", "sourceOutputSeq": 1, "sourceAttempt": 1, "toNodeId": projectionTestFormationID,
		"toPortId": "port_in", "payloadProjection": schema2RepairWorkProjection("input"),
	}
}

func schema2RepairSlotDispatchData(t *testing.T) map[string]any {
	t.Helper()
	data := schema2SlotDispatchEvent(t, 4, "dsp_01KXNP6VY3227H78329V52CKF8", "lease_01KXNP6VY3227H78329V52CKF8", "slot_worker", "worker", "binding_worker", "target_worker", strings.Repeat("a", 64), "solo")["data"].(map[string]any)
	return cloneAny(data).(map[string]any)
}

func schema2RepairReducerData(eventType string, data map[string]any) map[string]any {
	// C1 isolates reducer behavior. C2 separately requires the richer frozen
	// schema-2 input identity and every other nested projection to become closed.
	reducerRef := map[string]any{
		"inputId": "input_root", "sourceKind": "edge", "runId": projectionTestRunID,
		"toNodeId": projectionTestFormationID, "toPortId": "port_in", "payloadProjection": schema2RepairWorkProjection("input"),
	}
	switch eventType {
	case "gate_evaluating":
		data["inputRef"] = reducerRef
	case "gate_kind_result":
		data["evaluatedInputRef"] = reducerRef
	case "gate_verdict":
		data["evaluatedInputRef"] = reducerRef
	case "human_input_requested":
		data["evaluatedInputRef"] = reducerRef
	case "node_input_ignored":
		data["inputRef"] = reducerRef
	}
	return data
}

func schema2RepairFormationResultData() map[string]any {
	output := schema2RepairWorkProjection("formation output")
	data := map[string]any{
		"nodeId": projectionTestFormationID, "attempt": 1, "status": "done", "outputs": map[string]any{"port_out": output},
		"outputHashes": map[string]any{"port_out": projectionSHA256(mustMarshalJSONNoTest(output))}, "reportArtifactId": "",
		"artifactIds": []any{}, "diffArtifactIds": []any{}, "contributingSlotResultSeqs": []any{10},
		"resultEncoding": "formation-result-jcs-v1",
	}
	schema2SecondRepairNormalizeFormationResult(data)
	return data
}

func schema2RepairToolDispatchData() map[string]any {
	return map[string]any{
		"toolLeaseId": "toollease_01KXNP6VY3227H78329V52CKF8", "nodeId": "tool_normalize", "attempt": 1,
		"toolBindingId": "toolbinding_normalize", "inputManifestSha256": strings.Repeat("a", 64),
		"inputHashes": map[string]any{"port_tool_in": strings.Repeat("b", 64)}, "profileSha256": strings.Repeat("c", 64),
		"parametersSha256": strings.Repeat("d", 64), "policySha256": strings.Repeat("e", 64),
		"determinismPolicySha256": strings.Repeat("f", 64), "executionBundleSha256": strings.Repeat("1", 64),
		"recordedBeforeExecute": true,
	}
}

func schema2RepairToolProcessLaunchData() map[string]any {
	return map[string]any{
		"toolLeaseId": "toollease_01KXNP6VY3227H78329V52CKF8", "launchId": "launch_01KXNP6VY3227H78329V52CKF8",
		"nodeId": "tool_normalize", "attempt": 1, "generation": "1", "recordedBeforeSpawn": true,
	}
}

func schema2RepairToolResultData() map[string]any {
	output := schema2RepairWorkProjection(`{"normalized":true}`)
	return map[string]any{
		"toolLeaseId": "toollease_01KXNP6VY3227H78329V52CKF8", "launchId": "launch_01KXNP6VY3227H78329V52CKF8",
		"generation": "1", "nodeId": "tool_normalize", "attempt": 1, "status": "ok",
		"outputs": map[string]any{"port_tool_out": output}, "outputHashes": map[string]any{"port_tool_out": projectionSHA256(mustMarshalJSONNoTest(output))},
		"artifactRegistrations": []any{}, "artifacts": []any{}, "displayEvidence": []any{},
		"timing": map[string]any{"startedAt": "2026-07-20T10:00:00Z", "finishedAt": "2026-07-20T10:00:01Z", "durationMs": 1000},
	}
}

func schema2RepairNodeOutputData(nodeID string) map[string]any {
	return map[string]any{
		"nodeId": nodeID, "status": "done", "outputs": map[string]any{"out": schema2RepairWorkProjection("done")},
		"reportArtifactId": "artifact_report", "artifactIds": []any{}, "diffArtifactIds": []any{},
		"producedBy":     map[string]any{"kind": "mission", "outcomeSeq": 19},
		"timing":         map[string]any{"startedAt": "2026-07-20T10:00:00Z", "finishedAt": "2026-07-20T10:00:01Z", "durationMs": 1000},
		"deliveredEdges": []any{},
	}
}

func schema2RepairGateEvaluatingData() map[string]any {
	criterion := "Approve the result"
	return map[string]any{
		"gateId": projectionTestGateID, "gateAttempt": 1, "nodeId": projectionTestGateID, "kinds": []any{"code", "human"},
		"criterionProjection": map[string]any{
			"classification": "authored_config", "sourceKind": "gate_criterion", "encoding": "gate-criterion-utf8-v1",
			"mediaType": "text/plain", "sha256": projectionSHA256([]byte(criterion)), "text": criterion,
		},
		"inputRef": schema2RepairInputRef(), "judgeChain": []any{},
		"revisionCycleId": "revision_01KXNP6VY3227H78329V52CKF8", "triggerFeedbackId": "feedback_01KXNP6VY3227H78329V52CKF8", "priorGateSeq": 18,
	}
}

func schema2RepairGateKindResultData() map[string]any {
	return schema2RepairGateKindResultVerdictData("pass")
}

func schema2RepairGateKindResultVerdictData(verdict string) map[string]any {
	evidence := []any{map[string]any{"kind": "text", "text": "clean"}}
	result := map[string]any{"verdict": verdict, "reason": "lint passed", "evidence": evidence}
	return map[string]any{
		"gateId": projectionTestGateID, "gateAttempt": 1, "kind": "code", "verdict": verdict, "reason": "lint passed",
		"evidence": evidence, "evaluatedInputRef": schema2RepairInputRef(),
		"resultEncoding": "decision-result-jcs-v1", "resultSha256": projectionSHA256(mustMarshalJSONNoTest(result)), "relatedSeqs": []any{},
		"gateBindingId": "gatebinding_lint", "inputSha256": strings.Repeat("b", 64), "profileSha256": strings.Repeat("c", 64),
		"evaluatorBundleSha256": strings.Repeat("d", 64), "parametersSha256": strings.Repeat("e", 64),
		"policySha256": strings.Repeat("f", 64), "determinismPolicySha256": strings.Repeat("1", 64),
	}
}

func schema2RepairFormationGateKindResultData() map[string]any {
	data := schema2RepairGateKindResultData()
	data["kind"] = "formation"
	data["relatedSeqs"] = []any{18}
	for _, key := range []string{"gateBindingId", "inputSha256", "profileSha256", "evaluatorBundleSha256", "parametersSha256", "policySha256", "determinismPolicySha256"} {
		delete(data, key)
	}
	return data
}

func schema2RepairVariants(base map[string]any, key string, values ...string) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		variant := cloneAny(base).(map[string]any)
		variant[key] = value
		result = append(result, variant)
	}
	return result
}

func schema2RepairNestedVariants(base map[string]any, objectKey, key string, values ...string) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		variant := cloneAny(base).(map[string]any)
		variant[objectKey].(map[string]any)[key] = value
		result = append(result, variant)
	}
	return result
}

func schema2RepairWithEvidence(base map[string]any, evidence map[string]any) map[string]any {
	result := cloneAny(base).(map[string]any)
	result["evidence"] = []any{evidence}
	result["resultSha256"] = projectionSHA256(mustMarshalJSONNoTest(map[string]any{"verdict": result["verdict"], "reason": result["reason"], "evidence": result["evidence"]}))
	return result
}

func schema2RepairWithArtifactSource(source map[string]any) map[string]any {
	result := schema2RepairArtifactAttachedData()
	result["source"] = source
	return result
}

func schema2RepairUnavailableArtifactProjection(availability string) map[string]any {
	return map[string]any{"artifactId": "artifact_report", "availability": availability, "name": "report.md", "errorCode": availability + "_artifact"}
}

func schema2RepairWithArtifactProjection(projection map[string]any) map[string]any {
	result := schema2RepairArtifactAttachedData()
	result["artifactProjection"] = projection
	return result
}

func schema2RepairNodeOutputWithPayload(payload map[string]any) map[string]any {
	result := schema2RepairNodeOutputData(projectionTestMissionID)
	result["outputs"].(map[string]any)["out"] = map[string]any{"availability": "available", "exact": true, "payload": payload}
	return result
}

func schema2RepairJudgeResultData() map[string]any {
	return schema2RepairJudgeResultVerdictData("pass")
}

func schema2RepairJudgeResultVerdictData(verdict string) map[string]any {
	result := map[string]any{"verdict": verdict, "reason": "approved", "evidence": []any{map[string]any{"kind": "ledger", "seq": 18}}}
	return map[string]any{
		"gateId": projectionTestGateID, "gateAttempt": 1, "judgeNodeId": projectionTestFormationID, "judgeAttempt": 1,
		"chainIndex": 0, "contextEncoding": "judge-context-jcs-v1", "contextSha256": strings.Repeat("a", 64),
		"priorResultSeqs": []any{}, "result": result, "resultEncoding": "decision-result-jcs-v1", "resultSha256": projectionSHA256(mustMarshalJSONNoTest(result)),
	}
}

func schema2RepairJudgeAttemptFailedData() map[string]any {
	return map[string]any{
		"gateId": projectionTestGateID, "gateAttempt": 1, "judgeNodeId": projectionTestFormationID, "judgeAttempt": 1,
		"chainIndex": 0, "contextSha256": strings.Repeat("a", 64), "priorResultSeqs": []any{},
		"code": "invalid_judge_result", "reason": "invalid result", "relatedSeq": 19,
	}
}

func schema2RepairGateVerdictData() map[string]any {
	return map[string]any{
		"gateId": projectionTestGateID, "gateAttempt": 1, "verdict": "fail", "perKind": map[string]any{"code": "pass", "human": "fail"},
		"kindResultSeqs": map[string]any{"code": 18, "human": 19}, "evaluatedInputRef": schema2RepairInputRef(),
		"routePort": "fail", "routedEdges": []any{}, "reason": "needs revision",
		"feedbackPayload": map[string]any{
			"feedbackId": "feedback_01KXNP6VY3227H78329V52CKF8", "gateId": projectionTestGateID, "verdict": "fail",
			"evaluatedInput": map[string]any{"inputId": "input_root", "gateInputSeq": 3}, "reason": "needs revision",
			"evidence": []any{}, "gateSeq": 20, "gateAttempt": 1,
		},
	}
}

func schema2RepairHumanRequestData() map[string]any {
	return map[string]any{
		"gateId": projectionTestGateID, "gateAttempt": 1, "nodeId": projectionTestGateID,
		"promptProjection": map[string]any{"classification": "fixed_system", "sourceKind": "human_prompt", "templateId": "gate-human-verdict-v1"},
		"choiceProjections": map[string]any{
			"pass": map[string]any{"classification": "fixed_system", "sourceKind": "human_choice", "templateId": "gate-human-pass-v1"},
			"fail": map[string]any{"classification": "fixed_system", "sourceKind": "human_choice", "templateId": "gate-human-fail-v1"},
		},
		"requestedBy": "agent:test", "evaluatedInputRef": schema2RepairInputRef(), "completedKindResultSeqs": map[string]any{"code": 18},
	}
}

func schema2RepairHumanVerdictData() map[string]any {
	return map[string]any{
		"commandId": projectionTestOtherCmdID, "commandPayloadSha256": strings.Repeat("a", 64),
		"gateId": projectionTestGateID, "gateAttempt": 1, "nodeId": projectionTestGateID,
		"verdict": "pass", "reason": "approved", "requestedSeq": 4, "decidedBy": "human:test",
	}
}

func schema2RepairAvailableArtifactProjection() map[string]any {
	return map[string]any{
		"artifactId": "artifact_report", "availability": "available", "name": "report.md",
		"artifact": map[string]any{
			"artifactId": "artifact_report", "rootId": "run-artifacts", "ref": "reports/report.md", "mediaType": "text/markdown",
			"sizeBytes": 6, "sha256": strings.Repeat("a", 64),
		},
	}
}

func schema2RepairArtifactAttachedData() map[string]any {
	return map[string]any{
		"artifactProjection": schema2RepairAvailableArtifactProjection(),
		"source":             map[string]any{"kind": "gate", "gateId": projectionTestGateID, "gateAttempt": 1},
	}
}

func schema2RepairSlotResultData(t *testing.T) map[string]any {
	t.Helper()
	events := schema2MatchedReviewerEvents(t)
	return cloneAny(events[len(events)-1]["data"]).(map[string]any)
}

func schema2RepairRetryTarget() map[string]any {
	return map[string]any{
		"nodeId": projectionTestFormationID, "attempt": 1, "outputPortIds": []any{"port_out"},
		"outcomeSeqs": []any{19}, "deliveredEdges": []any{},
	}
}

func schema2RepairRetryBlockData() map[string]any {
	return map[string]any{
		"reason": "retry producer", "blockScope": "node", "blockedNodeId": projectionTestFormationID,
		"resumeAllowed": true, "resumePolicy": "retry_failed_producer", "openDispatches": []any{},
		"retryTargets": []any{schema2RepairRetryTarget()}, "nextEpoch": 1,
	}
}

func schema2RepairRetryResumeData() map[string]any {
	return map[string]any{
		"commandId": projectionTestOtherCmdID, "commandPayloadSha256": strings.Repeat("a", 64), "resumedFromSeq": 19,
		"resumedBy": "human:test", "resumeMode": "retry-failed-producer", "reason": "retry producer",
		"openDispatches": []any{}, "retryTargets": []any{schema2RepairRetryTarget()},
	}
}

func schema2RepairNodeAttemptSnapshot() map[string]any {
	return map[string]any{
		"nodeId": projectionTestFormationID, "nodeKind": "formation", "attempt": 1,
		"startSeq": 3, "phase": "started", "phaseSeq": 3,
	}
}

func schema2RepairOpenDispatchSnapshot(t *testing.T) map[string]any {
	t.Helper()
	_, dispatches := schema2OpenDispatchLifecycleInput(t, false)
	return cloneAny(dispatches[0]).(map[string]any)
}

func schema2RepairToolLeaseSnapshot(launched bool) map[string]any {
	result := map[string]any{
		"toolLeaseId": "toollease_01KXNP6VY3227H78329V52CKF8", "nodeId": "tool_normalize", "attempt": 1, "dispatchSeq": 12,
	}
	if launched {
		result["latestLaunch"] = map[string]any{
			"launchId": "launch_01KXNP6VY3227H78329V52CKF8", "generation": "1", "processScopeId": "process_scope_1",
			"deadlineAuthorityId": "deadline_authority_1", "launchSeq": 13,
		}
	}
	return result
}

func schema2RepairCancelRequestedData(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"commandId": projectionTestOtherCmdID, "commandPayloadSha256": strings.Repeat("a", 64), "reason": "stop", "requestedBy": "human:test",
		"openNodeAttempts":   []any{schema2RepairNodeAttemptSnapshot()},
		"openSlotDispatches": []any{schema2RepairOpenDispatchSnapshot(t)},
		"openToolLeases":     []any{schema2RepairToolLeaseSnapshot(false)},
	}
}

func schema2RepairFailureStartedData(t *testing.T, cause map[string]any) map[string]any {
	t.Helper()
	return map[string]any{
		"originCancelRequestSeq": 0, "code": "engine_failed", "reason": "engine failed", "unrecoverable": true, "relatedSeq": 18,
		"failureCause": cloneAny(cause), "openNodeAttempts": []any{schema2RepairNodeAttemptSnapshot()},
		"openSlotDispatches": []any{schema2RepairOpenDispatchSnapshot(t)}, "openToolLeases": []any{schema2RepairToolLeaseSnapshot(true)},
		"recordedBeforeReconciliation": true,
	}
}

func schema2RepairSlotDisposition(t *testing.T, disposition string) map[string]any {
	t.Helper()
	result := schema2RepairOpenDispatchSnapshot(t)
	result["disposition"] = disposition
	result["softInterrupt"] = "unavailable"
	result["softInterruptRequestedSeq"] = 16
	result["softInterruptOutcomeSeq"] = 17
	result["targetLeaseState"] = "quarantined"
	result["finalPeekCapabilityState"] = "revoked"
	result["finalCapabilityGeneration"] = "0"
	result["finalCapabilityIssuedSeq"] = 0
	result["finalSteeringGeneration"] = "0"
	result["finalPeekCapabilityRevokedSeq"] = 18
	return result
}

func schema2RepairRunCanceledData(t *testing.T) map[string]any {
	t.Helper()
	node := schema2RepairNodeAttemptSnapshot()
	node["disposition"] = "canceled_non_authorizing"
	tool := schema2RepairToolLeaseSnapshot(false)
	tool["disposition"] = "never_launched_cleaned"
	return map[string]any{
		"cancelRequestSeq": 15, "reason": "stop", "requestedBy": "human:test",
		"nodeAttemptDispositions": []any{node}, "slotDispatchDispositions": []any{schema2RepairSlotDisposition(t, "canceled_non_authorizing")},
		"reconciledToolLeases": []any{tool}, "final": true,
	}
}

func schema2RepairRunFailedData(t *testing.T) map[string]any {
	t.Helper()
	node := schema2RepairNodeAttemptSnapshot()
	node["disposition"] = "abandoned_non_authorizing"
	tool := schema2RepairToolLeaseSnapshot(true)
	tool["disposition"] = "abandoned_private_cleanup_owned"
	return map[string]any{
		"failureReconciliationSeq": 15, "code": "engine_failed", "reason": "engine failed", "unrecoverable": true, "relatedSeq": 18,
		"failureCause": map[string]any{"kind": "none"}, "nodeAttemptDispositions": []any{node},
		"slotDispatchDispositions": []any{schema2RepairSlotDisposition(t, "abandoned_non_authorizing")},
		"toolLeaseDispositions":    []any{tool}, "final": true,
	}
}

func schema2RepairReplaceGraphAndHashes(t *testing.T, input CanonicalRunReadInput, graph []byte) CanonicalRunReadInput {
	t.Helper()
	hash := projectionSHA256(graph)
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2GraphSnapshot, graph)
	bootstrap := decodeCanonicalObject(t, canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2RunBootstrap).Bytes)
	bootstrap["graphSnapshotSha256"] = hash
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2RunBootstrap, canonicalJSON(t, bootstrap))
	events := canonicalLedgerEvents(t, input)
	events[0]["data"].(map[string]any)["graphSnapshotSha256"] = hash
	return replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
}

func schema2RepairReplaceBindingsAndHashes(t *testing.T, input CanonicalRunReadInput, bindings []byte) CanonicalRunReadInput {
	t.Helper()
	hash := projectionSHA256(bindings)
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2PrivateBindings, bindings)
	bootstrap := decodeCanonicalObject(t, canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2RunBootstrap).Bytes)
	bootstrap["privateBindingsSha256"] = hash
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2RunBootstrap, canonicalJSON(t, bootstrap))
	events := canonicalLedgerEvents(t, input)
	events[0]["data"].(map[string]any)["privateBindingsSha256"] = hash
	return replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
}

func schema2RepairRoleDocumentIndex(input CanonicalRunReadInput, role CanonicalInputRole, occurrence int) int {
	seen := 0
	for index, document := range input.Documents {
		if document.Role != role {
			continue
		}
		if seen == occurrence {
			return index
		}
		seen++
	}
	return -1
}

func schema2RepairConfiguredPolicy(t *testing.T, input CanonicalRunReadInput) map[string]any {
	t.Helper()
	index := schema2RepairRoleDocumentIndex(input, CanonicalInputRoleSchema2AdmissionPolicy, 1)
	if index < 0 {
		t.Fatal("configured admission policy fixture missing")
	}
	return decodeCanonicalObject(t, input.Documents[index].Bytes)
}

func schema2RepairReplaceConfiguredPolicyAndReferences(t *testing.T, input CanonicalRunReadInput, raw []byte) CanonicalRunReadInput {
	t.Helper()
	index := schema2RepairRoleDocumentIndex(input, CanonicalInputRoleSchema2AdmissionPolicy, 1)
	if index < 0 {
		t.Fatal("configured admission policy fixture missing")
	}
	input.Documents[index] = canonicalInputDocument(CanonicalInputRoleSchema2AdmissionPolicy, raw)
	policy := decodeCanonicalObject(t, raw)
	policyRev := uint64(2)
	if value, ok := policy["policyRev"].(float64); ok && value >= 0 {
		policyRev = uint64(value)
	}
	policyHash := projectionSHA256(raw)

	authority := decodeCanonicalObject(t, canonicalDocumentByRole(t, input, CanonicalInputRoleSchema2WorkspaceAuthority).Bytes)
	authority["admissionPolicyRef"] = map[string]any{"policyRev": policyRev, "policySha256": policyHash}
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2WorkspaceAuthority, canonicalJSON(t, authority))

	events := canonicalLedgerEvents(t, input)
	for _, event := range events {
		data := event["data"].(map[string]any)
		switch event["type"] {
		case "run_started", "run_activated":
			data["admissionPolicyRev"], data["admissionPolicySha256"] = policyRev, policyHash
		}
	}
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2CommandRecord, schema2AdmissionCommandRecord(t, projectionTestCommandID, policyRev, policyHash))
	return input
}

func schema2RepairRequireProjectionInvalid(t *testing.T, input CanonicalRunReadInput, context string) {
	t.Helper()
	if projection, err := ProjectCanonicalRun(input); err == nil {
		t.Fatalf("%s projected: %#v", context, ProjectRunView(projection))
	} else {
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	}
}

func schema2RepairWideFenceCommandInput(t *testing.T, state string, fence uint64, payload map[string]any) CanonicalCommandReadInput {
	t.Helper()
	fenceString := strconv.FormatUint(fence, 10)
	return schema2RepairFenceTextCommandInput(t, state, fenceString, fenceString, fenceString, payload)
}

func schema2RepairFenceTextCommandInput(t *testing.T, state, admittedFence, stateFence, outcomeFence string, payload map[string]any) CanonicalCommandReadInput {
	t.Helper()
	return schema2RepairFenceCommandInput(t, state, admittedFence, stateFence, outcomeFence, payload)
}

func schema2RepairFenceCommandInput(t *testing.T, state string, admittedFence, stateFence, outcomeFence any, payload map[string]any) CanonicalCommandReadInput {
	t.Helper()
	payloadHash := projectionSHA256(canonicalJSON(t, payload))
	pending := canonicalJSON(t, map[string]any{
		"commandSchema": 1, "recordRev": 1, "priorGeneration": nil, "commandEncoding": "run-command-jcs-v1",
		"commandId": projectionTestCommandID, "commandKind": "cancel", "commandPayload": payload, "commandPayloadSha256": payloadHash,
		"admittedWriterFence": admittedFence, "stateWriterFence": admittedFence, "state": "pending",
	})
	terminal := map[string]any{
		"commandSchema": 1, "recordRev": 2, "priorGeneration": map[string]any{"recordRev": 1, "sha256": projectionSHA256(pending)},
		"commandEncoding": "run-command-jcs-v1", "commandId": projectionTestCommandID, "commandKind": "cancel",
		"commandPayload": payload, "commandPayloadSha256": payloadHash, "admittedWriterFence": admittedFence,
		"stateWriterFence": stateFence, "state": state, "outcomeWriterFence": outcomeFence, "decisionAdmissionPolicyRef": nil,
	}
	if state == "applied" {
		terminal["runId"] = projectionTestRunID
		terminal["effectSeq"] = 7
	} else {
		terminal["rejectionCode"] = "fixture_rejected"
	}
	return CanonicalCommandReadInput{
		Source:    CanonicalRunSourceSchema2,
		Submitted: SubmittedCommandIdentity{CommandID: projectionTestCommandID, CommandKind: "cancel", CommandPayloadSHA256: payloadHash},
		Record:    canonicalJSON(t, terminal),
	}
}
