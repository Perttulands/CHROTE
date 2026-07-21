package formations

import (
	"bytes"
	"encoding/json"
	"errors"
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
		name      string
		prepare   func(*projectionState)
		eventType string
		data      map[string]any
		envelope  func(map[string]any)
	}{
		{
			name: "formation result closes its attempt and materializes outputs", prepare: startSchema2RepairAttempt(projectionTestFormationID, "formation"),
			eventType: "formation_result", data: schema2RepairFormationResultData(),
		},
		{
			name: "tool dispatch opens tool attempt state", prepare: startSchema2RepairAttempt("tool_normalize", "tool"),
			eventType: "tool_dispatch", data: schema2RepairToolDispatchData(),
		},
		{
			name: "tool process launch advances tool state", prepare: startSchema2RepairAttempt("tool_normalize", "tool"),
			eventType: "tool_process_launch", data: schema2RepairToolProcessLaunchData(),
		},
		{
			name: "tool result closes tool attempt and materializes outputs", prepare: startSchema2RepairAttempt("tool_normalize", "tool"),
			eventType: "tool_result", data: schema2RepairToolResultData(),
		},
		{
			name: "node output closes the named attempt and materializes outputs", prepare: startSchema2RepairAttempt(projectionTestMissionID, "mission"),
			eventType: "node_output", data: schema2RepairNodeOutputData(projectionTestMissionID),
		},
		{
			name: "gate evaluating opens gate state", prepare: startSchema2RepairAttempt(projectionTestGateID, "gate"),
			eventType: "gate_evaluating", data: schema2RepairGateEvaluatingData(),
		},
		{
			name: "gate kind result retains typed evidence", prepare: prepareSchema2RepairGate,
			eventType: "gate_kind_result", data: schema2RepairGateKindResultData(),
		},
		{
			name: "judge result advances gate evidence", prepare: prepareSchema2RepairGate,
			eventType: "judge_result", data: schema2RepairJudgeResultData(),
		},
		{
			name: "judge attempt failure advances gate evidence", prepare: prepareSchema2RepairGate,
			eventType: "judge_attempt_failed", data: schema2RepairJudgeAttemptFailedData(),
		},
		{
			name: "gate verdict closes the gate attempt", prepare: prepareSchema2RepairGate,
			eventType: "gate_verdict", data: schema2RepairGateVerdictData(),
		},
		{
			name: "human request marks gate and attempt waiting", prepare: prepareSchema2RepairGate,
			eventType: "human_input_requested", data: schema2RepairHumanRequestData(),
		},
		{
			name: "human verdict returns the gate to evaluation", prepare: prepareSchema2RepairHumanGate,
			eventType: "human_verdict_recorded", data: schema2RepairHumanVerdictData(),
		},
		{
			name: "escalation is retained structurally", prepare: startSchema2RepairAttempt(projectionTestFormationID, "formation"),
			eventType: "escalation_raised", data: map[string]any{
				"trigger": "operator_review", "severity": "needs-attention", "reason": "review", "source": "agent",
				"nodeId": projectionTestFormationID, "gateId": "", "blocks": true,
			},
		},
		{
			name: "node input ignored changes readiness evidence", prepare: startSchema2RepairAttempt(projectionTestFormationID, "formation"),
			eventType: "node_input_ignored", data: map[string]any{
				"nodeId": projectionTestFormationID, "toPortId": "port_in", "inputRef": schema2RepairInputRef(),
				"reason": "closed_turn", "relatedAttempt": 1,
			},
		},
		{
			name: "scoped error changes the affected attempt", prepare: startSchema2RepairAttempt(projectionTestFormationID, "formation"),
			eventType: "error", data: map[string]any{
				"code": "dispatch_failed", "message": "dispatch failed", "boundary": "dispatcher", "errorScope": "node",
				"nodeId": projectionTestFormationID, "recoverable": true, "relatedSeq": 2,
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
			before := schema2RepairStructuralFingerprint(t, state.view)
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
			after := schema2RepairStructuralFingerprint(t, state.view)
			if bytes.Equal(before, after) {
				t.Fatalf("%s was consumed as audit-only; structural view did not change: %s", test.eventType, after)
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

func TestProjectCanonicalRunSchema2SessionAuthorityArmsAreNeverAuditOnly(t *testing.T) {
	dispatchID := "dsp_01KXNP6VY3227H78329V52CKF8"
	leaseID := "lease_01KXNP6VY3227H78329V52CKF8"
	fingerprint := strings.Repeat("a", 64)
	tests := []struct {
		name      string
		eventType string
		data      map[string]any
		prepare   func(*RunSessionView)
	}{
		{name: "peek capability issuance updates session capability", eventType: "slot_peek_capability_issued", data: map[string]any{
			"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": "binding_worker", "sessionTargetId": "target_worker",
			"targetFingerprint": fingerprint, "capabilityGeneration": "1", "priorIssuedSeq": 0, "issuedAt": "2026-07-20T10:00:19Z",
		}},
		{name: "steering start opens session steering", eventType: "slot_steering_started", data: map[string]any{
			"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": "binding_worker", "sessionTargetId": "target_worker",
			"targetFingerprint": fingerprint, "capabilityIssuedSeq": 18, "capabilityGeneration": "1", "steeringGeneration": "1",
			"actor": "human:test", "startedAt": "2026-07-20T10:00:19Z", "recordedBeforeInput": true,
		}},
		{name: "steering end closes session steering", eventType: "slot_steering_ended", prepare: func(session *RunSessionView) {
			started := uint64(18)
			session.Steering = RunSessionSteering{State: "open", Generation: "1", StartedSeq: &started}
		}, data: map[string]any{
			"startedSeq": 18, "dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprint": fingerprint,
			"steeringGeneration": "1", "reason": "complete", "endedAt": "2026-07-20T10:00:19Z",
		}},
		{name: "reconciliation interrupt updates session occupancy", eventType: "slot_reconciliation_interrupt", data: map[string]any{
			"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": "binding_worker", "sessionTargetId": "target_worker",
			"targetFingerprint": fingerprint, "authorityKind": "failure", "authoritySeq": 18,
			"interruptEncoding": "slot-reconciliation-interrupt-v1", "interruptSha256": strings.Repeat("b", 64), "recordedBeforeSend": true,
		}},
		{name: "reconciliation outcome updates session occupancy", eventType: "slot_reconciliation_interrupt_outcome", prepare: func(session *RunSessionView) {
			session.Occupancy.State = "held"
		}, data: map[string]any{
			"requestedSeq": 18, "dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprint": fingerprint,
			"outcome": "interrupted", "observedAt": "2026-07-20T10:00:19Z",
		}},
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
			before := schema2RepairStructuralFingerprint(t, state.view)
			event := schema2Event(projectionTestRunID, 20, test.eventType, cloneAny(test.data).(map[string]any))
			delete(event, "missionId")
			delete(event, "beadId")
			raw, safe := schema2RepairDecodeSafeEvent(t, event)
			if err := reduceSchema2Event(&state, raw, safe, nil); err != nil {
				t.Fatalf("reduce admitted %s: %v", test.eventType, err)
			}
			after := schema2RepairStructuralFingerprint(t, state.view)
			if bytes.Equal(before, after) {
				t.Fatalf("%s was consumed as audit-only; session did not change", test.eventType)
			}
		})
	}
}

func TestSafeRunEventPublicTypesContainNoRawJSON(t *testing.T) {
	rawMessageType := reflect.TypeOf(json.RawMessage{})
	for _, event := range append(schema2SafeEventTypes(), schema1SafeEventTypes()...) {
		t.Run(strconv.Itoa(event.source)+"/"+event.literal, func(t *testing.T) {
			paths := schema2RepairTypePaths(event.typeOf, rawMessageType, nil, map[reflect.Type]bool{})
			if len(paths) != 0 {
				t.Fatalf("public SafeRunEvent contains json.RawMessage at %s; use closed named projections", strings.Join(paths, ", "))
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
		{name: "tool input hashes reject invalid hash grammar", eventType: "tool_dispatch", valid: schema2RepairToolDispatchData(), mutate: func(data map[string]any) {
			data["inputHashes"].(map[string]any)["port_tool_in"] = "not-a-hash"
		}},
		{name: "tool result outputs reject unknown payload member", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			data["outputs"].(map[string]any)["port_tool_out"].(map[string]any)["path"] = "/private"
		}},
		{name: "tool result output hashes must match output keys", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			data["outputHashes"] = map[string]any{"other": strings.Repeat("a", 64)}
		}},
		{name: "tool result artifact registration ids must agree", eventType: "tool_result", valid: schema2RepairToolResultData(), mutate: func(data map[string]any) {
			registration := schema2RepairAvailableArtifactProjection()
			registration["artifact"].(map[string]any)["artifactId"] = "artifact_other"
			data["artifactRegistrations"] = []any{registration}
		}},
		{name: "node outputs reject invalid status", eventType: "node_output", valid: schema2RepairNodeOutputData(projectionTestMissionID), mutate: func(data map[string]any) {
			data["status"] = "maybe"
		}},
		{name: "node outputs reject unclosed payload", eventType: "node_output", valid: schema2RepairNodeOutputData(projectionTestMissionID), mutate: func(data map[string]any) {
			data["outputs"].(map[string]any)["out"].(map[string]any)["token"] = "forbidden"
		}},
		{name: "gate criterion requires authored config classification", eventType: "gate_evaluating", valid: schema2RepairGateEvaluatingData(), mutate: func(data map[string]any) {
			data["criterionProjection"].(map[string]any)["classification"] = "runtime"
		}},
		{name: "gate criterion rejects unknown member", eventType: "gate_evaluating", valid: schema2RepairGateEvaluatingData(), mutate: func(data map[string]any) {
			data["criterionProjection"].(map[string]any)["prompt"] = "forbidden"
		}},
		{name: "gate result rejects unknown evidence kind", eventType: "gate_kind_result", valid: schema2RepairGateKindResultData(), mutate: func(data map[string]any) {
			data["evidence"] = []any{map[string]any{"kind": "socket", "text": "forbidden"}}
		}},
		{name: "judge result rejects invalid verdict", eventType: "judge_result", valid: schema2RepairJudgeResultData(), mutate: func(data map[string]any) {
			data["result"].(map[string]any)["verdict"] = "unknown"
		}},
		{name: "gate feedback evaluated input is identity only", eventType: "gate_verdict", valid: schema2RepairGateVerdictData(), mutate: func(data map[string]any) {
			data["feedbackPayload"].(map[string]any)["evaluatedInput"].(map[string]any)["payloadProjection"] = schema2RepairWorkProjection("forbidden")
		}},
		{name: "artifact source rejects unknown kind", eventType: "artifact_attached", valid: schema2RepairArtifactAttachedData(), mutate: func(data map[string]any) {
			data["source"].(map[string]any)["kind"] = "path"
		}},
		{name: "fixed system prompt rejects wrong template", eventType: "human_input_requested", valid: schema2RepairHumanRequestData(), mutate: func(data map[string]any) {
			data["promptProjection"].(map[string]any)["templateId"] = "gate-human-verdict-v2"
		}},
		{name: "choice projections require exact pass and fail", eventType: "human_input_requested", valid: schema2RepairHumanRequestData(), mutate: func(data map[string]any) {
			delete(data["choiceProjections"].(map[string]any), "fail")
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
		{name: "bootstrap authority mismatch", role: CanonicalInputRoleSchema2WorkspaceBootstrap, mutate: func(value map[string]any) { value["workspaceAuthorityId"] = "wsa_other" }},
		{name: "bootstrap root hash mismatch", role: CanonicalInputRoleSchema2WorkspaceBootstrap, mutate: func(value map[string]any) { value["workspaceRootIdentitySha256"] = strings.Repeat("f", 64) }},
		{name: "current authority unknown member", role: CanonicalInputRoleSchema2WorkspaceAuthority, mutate: func(value map[string]any) { value["projectionUnknown"] = true }},
		{name: "current authority id mismatch", role: CanonicalInputRoleSchema2WorkspaceAuthority, mutate: func(value map[string]any) { value["workspaceAuthorityId"] = "wsa_other" }},
		{name: "run bootstrap unknown member", role: CanonicalInputRoleSchema2RunBootstrap, mutate: func(value map[string]any) { value["projectionUnknown"] = true }},
		{name: "run bootstrap run mismatch", role: CanonicalInputRoleSchema2RunBootstrap, mutate: func(value map[string]any) { value["runId"] = projectionTestOtherRunID }},
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
	view := ProjectRunView(mustProjectCanonicalFixture(t, input))
	for _, forbidden := range []string{"fmn_disconnected", "gate_disconnected"} {
		if raw := mustMarshalJSON(t, view); bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("disconnected board element %q leaked into a structural collection: %s", forbidden, raw)
		}
	}
	if len(view.Nodes) != 3 || len(view.Attempts) != 0 || len(view.Gates) != 0 || len(view.Outputs) != 0 || len(view.Sessions) != 0 {
		t.Fatalf("selected-root structural cardinality inflated: nodes=%d attempts=%d gates=%d outputs=%d sessions=%d", len(view.Nodes), len(view.Attempts), len(view.Gates), len(view.Outputs), len(view.Sessions))
	}
}

func TestProjectCommandReceiptPreservesFullUint64WriterFences(t *testing.T) {
	for _, state := range []string{"applied", "rejected"} {
		t.Run(state, func(t *testing.T) {
			fence := uint64(MaxJSONSafeInteger + 17)
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

func startSchema2RepairAttempt(nodeID, kind string) func(*projectionState) {
	return func(state *projectionState) {
		state.startAttempt(nodeID, 1, 2, []SafeInputIdentity{})
		if node := state.node(nodeID); node != nil {
			node.Kind = kind
		}
	}
}

func prepareSchema2RepairGate(state *projectionState) {
	startSchema2RepairAttempt(projectionTestGateID, "gate")(state)
	state.startGate(projectionTestGateID, 1, 3, SafeInputIdentity{})
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

func schema2RepairTypePaths(current, target reflect.Type, prefix []string, seen map[reflect.Type]bool) []string {
	if current == target {
		return []string{strings.Join(prefix, ".")}
	}
	for current.Kind() == reflect.Pointer || current.Kind() == reflect.Slice || current.Kind() == reflect.Array {
		current = current.Elem()
		if current == target {
			return []string{strings.Join(prefix, ".")}
		}
	}
	if current.Kind() != reflect.Struct || seen[current] {
		return nil
	}
	seen[current] = true
	defer delete(seen, current)
	var paths []string
	for index := 0; index < current.NumField(); index++ {
		field := current.Field(index)
		paths = append(paths, schema2RepairTypePaths(field.Type, target, append(prefix, field.Name), seen)...)
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
		data["kindResultSeqs"] = []any{18, 19}
	case "human_input_requested":
		data["evaluatedInputRef"] = reducerRef
		data["completedKindResultSeqs"] = []any{18}
	case "node_input_ignored":
		data["inputRef"] = reducerRef
	}
	return data
}

func schema2RepairFormationResultData() map[string]any {
	output := schema2RepairWorkProjection("formation output")
	return map[string]any{
		"nodeId": projectionTestFormationID, "attempt": 1, "status": "done", "outputs": map[string]any{"port_out": output},
		"outputHashes": map[string]any{"port_out": projectionSHA256(mustMarshalJSONNoTest(output))}, "reportArtifactId": "",
		"artifactIds": []any{}, "diffArtifactIds": []any{}, "contributingSlotResultSeqs": []any{10},
		"resultEncoding": "formation-result-jcs-v1", "resultSha256": strings.Repeat("a", 64),
	}
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
		"reportArtifactId": "", "artifactIds": []any{}, "diffArtifactIds": []any{},
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
		"inputRef": schema2RepairInputRef(), "judgeChain": []any{}, "revisionCycleId": "", "triggerFeedbackId": "", "priorGateSeq": 0,
	}
}

func schema2RepairGateKindResultData() map[string]any {
	return map[string]any{
		"gateId": projectionTestGateID, "gateAttempt": 1, "kind": "code", "verdict": "pass", "reason": "lint passed",
		"evidence": []any{map[string]any{"kind": "text", "text": "clean"}}, "evaluatedInputRef": schema2RepairInputRef(),
		"resultEncoding": "gate-kind-result-jcs-v1", "resultSha256": strings.Repeat("a", 64), "relatedSeqs": []any{},
		"gateBindingId": "gatebinding_lint", "inputSha256": strings.Repeat("b", 64), "profileSha256": strings.Repeat("c", 64),
		"evaluatorBundleSha256": strings.Repeat("d", 64), "parametersSha256": strings.Repeat("e", 64),
		"policySha256": strings.Repeat("f", 64), "determinismPolicySha256": strings.Repeat("1", 64),
	}
}

func schema2RepairJudgeResultData() map[string]any {
	result := map[string]any{"verdict": "pass", "reason": "approved", "evidence": []any{}}
	return map[string]any{
		"gateId": projectionTestGateID, "gateAttempt": 1, "judgeNodeId": projectionTestFormationID, "judgeAttempt": 1,
		"chainIndex": 0, "contextEncoding": "judge-context-jcs-v1", "contextSha256": strings.Repeat("a", 64),
		"priorResultSeqs": []any{}, "result": result, "resultEncoding": "judge-result-jcs-v1", "resultSha256": projectionSHA256(mustMarshalJSONNoTest(result)),
	}
}

func schema2RepairJudgeAttemptFailedData() map[string]any {
	return map[string]any{
		"gateId": projectionTestGateID, "gateAttempt": 1, "judgeNodeId": projectionTestFormationID, "judgeAttempt": 1,
		"chainIndex": 0, "contextSha256": strings.Repeat("a", 64), "priorResultSeqs": []any{},
		"code": "judge_failed", "reason": "invalid result", "relatedSeq": 19,
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

func schema2RepairWideFenceCommandInput(t *testing.T, state string, fence uint64, payload map[string]any) CanonicalCommandReadInput {
	t.Helper()
	fenceString := strconv.FormatUint(fence, 10)
	payloadHash := projectionSHA256(canonicalJSON(t, payload))
	pending := canonicalJSON(t, map[string]any{
		"commandSchema": 1, "recordRev": 1, "priorGeneration": nil, "commandEncoding": "run-command-jcs-v1",
		"commandId": projectionTestCommandID, "commandKind": "cancel", "commandPayload": payload, "commandPayloadSha256": payloadHash,
		"admittedWriterFence": fenceString, "stateWriterFence": fenceString, "state": "pending",
	})
	terminal := map[string]any{
		"commandSchema": 1, "recordRev": 2, "priorGeneration": map[string]any{"recordRev": 1, "sha256": projectionSHA256(pending)},
		"commandEncoding": "run-command-jcs-v1", "commandId": projectionTestCommandID, "commandKind": "cancel",
		"commandPayload": payload, "commandPayloadSha256": payloadHash, "admittedWriterFence": fenceString,
		"stateWriterFence": fenceString, "state": state, "outcomeWriterFence": fenceString, "decisionAdmissionPolicyRef": nil,
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
