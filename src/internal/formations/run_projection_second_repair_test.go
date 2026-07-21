package formations

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

type schema2SecondRepairCase struct {
	name      string
	eventType string
	data      map[string]any
	required  []string
	forbidden []string
}

func TestProjectCanonicalRunSchema2PublicDataRequiredness(t *testing.T) {
	cases := schema2SecondRepairRequirednessCases(t)
	allowlist := schema2SecondRepairPublicAllowedKeys()
	seen := map[string]bool{}
	for _, test := range cases {
		seen[test.eventType] = true
		t.Run(test.name, func(t *testing.T) {
			allowed, ok := allowlist[test.eventType]
			if !ok {
				t.Fatalf("schema-2 public allowlist missing %s", test.eventType)
			}
			safe, err := schema2SecondRepairSanitize(test.eventType, test.data)
			if err != nil {
				t.Fatalf("valid %s fixture rejected before requiredness checks: %v", test.name, err)
			}
			schema2SecondRepairRequireAllowlistedPublicData(t, test.eventType, test.data, safe, allowed)
			for _, key := range test.required {
				t.Run("missing_"+key, func(t *testing.T) {
					mutated := cloneAny(test.data).(map[string]any)
					delete(mutated, key)
					schema2SecondRepairRequirePublicRejection(t, test.eventType, mutated)
				})
			}
			for _, key := range test.forbidden {
				t.Run("forbidden_"+key, func(t *testing.T) {
					mutated := cloneAny(test.data).(map[string]any)
					mutated[key] = schema2SecondRepairForbiddenValue(key)
					schema2SecondRepairRequirePublicRejection(t, test.eventType, mutated)
				})
			}
			t.Run("unknown_key", func(t *testing.T) {
				mutated := cloneAny(test.data).(map[string]any)
				mutated["projectionUnknown"] = true
				schema2SecondRepairRequirePublicRejection(t, test.eventType, mutated)
			})
		})
	}

	want := schema2SecondRepairPublicRequiredKeys()
	if len(want) != 37 {
		t.Fatalf("frozen schema-2 requiredness table has %d rows, want 37", len(want))
	}
	if len(allowlist) != 37 {
		t.Fatalf("frozen schema-2 public allowlist has %d rows, want 37", len(allowlist))
	}
	for eventType := range want {
		if !seen[eventType] {
			t.Errorf("schema-2 requiredness fixture missing %s", eventType)
		}
	}
}

func schema2SecondRepairRequireAllowlistedPublicData(t *testing.T, eventType string, rawData map[string]any, safe SafeRunEvent, allowed []string) {
	t.Helper()
	var encoded struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(mustMarshalJSON(t, safe), &encoded); err != nil {
		t.Fatalf("encode sanitized %s event: %v", eventType, err)
	}
	for key := range encoded.Data {
		if !schema2SecondRepairContainsString(allowed, key) {
			t.Fatalf("sanitized %s exposed non-allowlisted data key %s", eventType, key)
		}
	}
	privateInputKeys := make([]string, 0)
	for key := range rawData {
		if !schema2SecondRepairContainsString(allowed, key) {
			privateInputKeys = append(privateInputKeys, key)
		}
	}
	if len(privateInputKeys) != 0 {
		sort.Strings(privateInputKeys)
		t.Logf("sanitized %s omitted private canonical input keys: %s", eventType, strings.Join(privateInputKeys, ","))
	}
}

func TestProjectCanonicalRunSchema2OptionalPublicDataMayBeAbsent(t *testing.T) {
	for _, test := range []struct {
		eventType string
		key       string
	}{
		{eventType: "formation_result", key: "reportArtifactId"},
		{eventType: "tool_result", key: "displayEvidence"},
		{eventType: "node_output", key: "reportArtifactId"},
		{eventType: "gate_evaluating", key: "revisionCycleId"},
		{eventType: "gate_evaluating", key: "triggerFeedbackId"},
		{eventType: "gate_evaluating", key: "priorGateSeq"},
		{eventType: "run_succeeded", key: "summaryArtifactId"},
	} {
		t.Run(test.eventType+"/absent_"+test.key, func(t *testing.T) {
			data := schema2SecondRepairFixture(t, test.eventType)
			delete(data, test.key)
			_, err := schema2SecondRepairSanitize(test.eventType, data)
			if err != nil {
				t.Fatalf("optional %s.%s omission rejected: %v", test.eventType, test.key, err)
			}
		})
	}
}

func TestProjectCanonicalRunSchema2NodeWaitingUsesDataIdentity(t *testing.T) {
	valid := map[string]any{
		"nodeId": projectionTestMissionID, "neededInputs": 1, "readyInputs": 0, "totalInputs": 1,
		"waitingFor": []any{"edge_root_work"},
	}

	for _, test := range []struct {
		name       string
		data       map[string]any
		envelopeID string
		wantValid  bool
		wantError  error
	}{
		{name: "exact identity is preserved", data: valid, envelopeID: projectionTestMissionID, wantValid: true},
		{name: "missing data identity", data: withoutSecondRepairKey(valid, "nodeId"), envelopeID: projectionTestMissionID, wantError: ErrRunEventUnknown},
		{name: "unknown identity", data: withSecondRepairValue(valid, "nodeId", "mis_missing"), envelopeID: "mis_missing", wantError: ErrRunProjectionInvalid},
		{name: "envelope and data identity differ", data: valid, envelopeID: "mis_missing", wantError: ErrRunProjectionInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := schema2Event(projectionTestRunID, 3, "node_waiting", test.data)
			event["nodeId"] = test.envelopeID
			input := schema2ProjectionInput(t, true, event)
			projection, err := ProjectCanonicalRun(input)
			if !test.wantValid {
				requireProjectionError(t, err, test.wantError)
				return
			}
			if err != nil {
				t.Fatalf("project exact node_waiting: %v", err)
			}
			page := mustProjectEventPage(t, projection, 2, 1)
			if len(page.Events) != 1 {
				t.Fatalf("node_waiting event count = %d, want 1", len(page.Events))
			}
			var encoded struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(mustMarshalJSON(t, page.Events[0]), &encoded); err != nil {
				t.Fatal(err)
			}
			var nodeID string
			if err := json.Unmarshal(encoded.Data["nodeId"], &nodeID); err != nil || nodeID != projectionTestMissionID {
				t.Fatalf("projected nodeId = %q (%v), want %q; event=%s", nodeID, err, projectionTestMissionID, mustMarshalJSON(t, page.Events[0]))
			}
		})
	}
}

func TestProjectCanonicalRunSchema2SimpleEventSemantics(t *testing.T) {
	for _, test := range schema2SecondRepairSemanticNegatives(t) {
		t.Run(test.eventType+"/"+test.name, func(t *testing.T) {
			data := schema2SecondRepairFixture(t, test.eventType)
			if _, err := schema2SecondRepairSanitize(test.eventType, data); err != nil {
				t.Fatalf("valid %s fixture rejected before semantic mutation: %v", test.eventType, err)
			}
			test.mutate(data)
			schema2SecondRepairRequirePublicRejection(t, test.eventType, data)
		})
	}
}

func TestProjectCanonicalRunSchema2RunSucceededRequiresValidFinalEvent(t *testing.T) {
	validData := schema2SecondRepairFixture(t, "run_succeeded")
	validEvent := schema2Event(projectionTestRunID, 3, "run_succeeded", validData)
	valid := ProjectRunView(mustProjectCanonicalFixture(t, schema2ProjectionInput(t, true, validEvent)))
	if valid.Status != "succeeded" || !valid.Final {
		t.Fatalf("valid run_succeeded status/final = %q/%v, want succeeded/true", valid.Status, valid.Final)
	}

	invalidData := cloneAny(validData).(map[string]any)
	invalidData["final"] = false
	invalidEvent := schema2Event(projectionTestRunID, 3, "run_succeeded", invalidData)
	projection, err := ProjectCanonicalRun(schema2ProjectionInput(t, true, invalidEvent))
	if err == nil {
		view := ProjectRunView(projection)
		t.Fatalf("run_succeeded(final=false) produced status/final %q/%v", view.Status, view.Final)
	}
	schema2SecondRepairRequireTypedRejection(t, err)
}

func TestProjectCanonicalRunSchema2ActivationMatchesAdmissionIdentity(t *testing.T) {
	input := schema2ProjectionInput(t, true)
	events := canonicalLedgerEvents(t, input)
	events[1]["data"].(map[string]any)["workspaceAdmissionSeq"] = uint64(2)
	input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...))
	_, err := ProjectCanonicalRun(input)
	requireProjectionError(t, err, ErrRunProjectionInvalid)
}

func TestProjectCanonicalRunSchema1NodeOutputRequiresReason(t *testing.T) {
	event := schema1NodeOutputEvent(projectionTestRunID, 3, "done")
	delete(event["data"].(map[string]any), "reason")
	raw, err := decodeProjectionEvent(canonicalJSON(t, event), CanonicalRunSourceSchema1, projectionTestRunID)
	if err != nil {
		t.Fatalf("decode schema-1 node_output fixture: %v", err)
	}
	_, err = sanitizeSchema1Event(raw)
	requireProjectionError(t, err, ErrRunEventUnknown)
}

type schema2SecondRepairSemanticMutation struct {
	eventType string
	name      string
	mutate    func(map[string]any)
}

func schema2SecondRepairRequirednessCases(t *testing.T) []schema2SecondRepairCase {
	t.Helper()
	required := schema2SecondRepairPublicRequiredKeys()
	conditional := map[string]bool{
		"node_started": true, "gate_kind_result": true, "artifact_observed": true, "error": true, "run_blocked": true,
	}
	result := make([]schema2SecondRepairCase, 0, 46)
	eventTypes := make([]string, 0, len(required))
	for eventType := range required {
		eventTypes = append(eventTypes, eventType)
	}
	sort.Strings(eventTypes)
	for _, eventType := range eventTypes {
		if conditional[eventType] {
			continue
		}
		result = append(result, schema2SecondRepairCase{name: eventType, eventType: eventType, data: schema2SecondRepairFixture(t, eventType), required: required[eventType]})
	}

	ordinaryStart := schema2SecondRepairFixture(t, "node_started")
	result = append(result, schema2SecondRepairCase{
		name: "node_started/ordinary", eventType: "node_started", data: ordinaryStart,
		required:  []string{"nodeId", "nodeKind", "attempt", "reason", "inputRefs"},
		forbidden: []string{"contextEncoding", "judgeContextSha256", "priorResultSeqs"},
	})
	judgeStart := map[string]any{
		"nodeId": projectionTestFormationID, "nodeKind": "formation", "attempt": 1, "reason": "judge",
		"contextEncoding": "judge-context-jcs-v1", "judgeContextSha256": strings.Repeat("a", 64), "priorResultSeqs": []any{18},
	}
	result = append(result, schema2SecondRepairCase{
		name: "node_started/judge", eventType: "node_started", data: judgeStart,
		required:  []string{"nodeId", "nodeKind", "attempt", "reason", "contextEncoding", "judgeContextSha256", "priorResultSeqs"},
		forbidden: []string{"inputRefs"},
	})

	codeGate := schema2RepairGateKindResultData()
	result = append(result, schema2SecondRepairCase{
		name: "gate_kind_result/code", eventType: "gate_kind_result", data: codeGate,
		required: required["gate_kind_result"],
	})
	formationGate := schema2RepairFormationGateKindResultData()
	result = append(result, schema2SecondRepairCase{
		name: "gate_kind_result/formation", eventType: "gate_kind_result", data: formationGate,
		required:  []string{"gateId", "gateAttempt", "kind", "verdict", "reason", "evidence", "evaluatedInputRef", "resultEncoding", "resultSha256", "relatedSeqs"},
		forbidden: []string{"gateBindingId", "inputSha256", "profileSha256", "evaluatorBundleSha256", "parametersSha256", "policySha256", "determinismPolicySha256"},
	})

	availableObserved := schema2SecondRepairFixture(t, "artifact_observed")
	result = append(result, schema2SecondRepairCase{
		name: "artifact_observed/available", eventType: "artifact_observed", data: availableObserved,
		required: []string{"artifactId", "availability", "artifact", "observedAt", "relatedSeq"}, forbidden: []string{"errorCode"},
	})
	unavailableObserved := map[string]any{
		"artifactId": "artifact_report", "availability": "unavailable", "errorCode": "artifact_unavailable",
		"observedAt": "2026-07-20T10:00:19Z", "relatedSeq": 18,
	}
	result = append(result, schema2SecondRepairCase{
		name: "artifact_observed/unavailable", eventType: "artifact_observed", data: unavailableObserved,
		required: []string{"artifactId", "availability", "errorCode", "observedAt", "relatedSeq"}, forbidden: []string{"artifact"},
	})

	errorCommon := map[string]any{
		"code": "dispatch_failed", "message": "dispatch failed", "boundary": "engine", "errorScope": "run",
		"recoverable": true, "relatedSeq": 18,
	}
	for _, scope := range []string{"run", "node", "gate", "slot", "tool"} {
		data := cloneAny(errorCommon).(map[string]any)
		data["errorScope"] = scope
		requiredKeys := []string{"code", "message", "boundary", "errorScope", "recoverable", "relatedSeq"}
		forbiddenKeys := []string{"nodeId", "gateId", "slotId", "toolLeaseId"}
		if scope != "run" {
			identityKey := map[string]string{"node": "nodeId", "gate": "gateId", "slot": "slotId", "tool": "toolLeaseId"}[scope]
			data[identityKey] = map[string]string{"nodeId": projectionTestMissionID, "gateId": projectionTestGateID, "slotId": "slot_worker", "toolLeaseId": "toollease_01KXNP6VY3227H78329V52CKF8"}[identityKey]
			requiredKeys = append(requiredKeys, identityKey)
			forbiddenKeys = withoutSecondRepairStrings(forbiddenKeys, identityKey)
		}
		result = append(result, schema2SecondRepairCase{
			name: "error/" + scope, eventType: "error", data: data, required: requiredKeys, forbidden: forbiddenKeys,
		})
	}

	runBlock := map[string]any{
		"reason": "new run required", "blockScope": "run", "resumeAllowed": false, "resumePolicy": "new_run_required",
		"openDispatches": []any{}, "retryTargets": []any{},
	}
	result = append(result, schema2SecondRepairCase{
		name: "run_blocked/run_new_run_required", eventType: "run_blocked", data: runBlock,
		required:  []string{"reason", "blockScope", "resumeAllowed", "resumePolicy", "openDispatches", "retryTargets"},
		forbidden: []string{"blockedNodeId", "blockedGateId", "nextEpoch"},
	})
	nodeRetry := schema2RepairRetryBlockData()
	result = append(result, schema2SecondRepairCase{
		name: "run_blocked/node_retry", eventType: "run_blocked", data: nodeRetry,
		required:  []string{"reason", "blockScope", "blockedNodeId", "resumeAllowed", "resumePolicy", "openDispatches", "retryTargets", "nextEpoch"},
		forbidden: []string{"blockedGateId"},
	})
	gateBlock := cloneAny(runBlock).(map[string]any)
	gateBlock["blockScope"], gateBlock["blockedGateId"] = "gate", projectionTestGateID
	result = append(result, schema2SecondRepairCase{
		name: "run_blocked/gate_new_run_required", eventType: "run_blocked", data: gateBlock,
		required:  []string{"reason", "blockScope", "blockedGateId", "resumeAllowed", "resumePolicy", "openDispatches", "retryTargets"},
		forbidden: []string{"nextEpoch"},
	})

	return result
}

func schema2SecondRepairFixture(t *testing.T, eventType string) map[string]any {
	t.Helper()
	dispatchID := "dsp_01KXNP6VY3227H78329V52CKF8"
	leaseID := "lease_01KXNP6VY3227H78329V52CKF8"
	bindingID := "binding_worker"
	targetID := "target_worker"
	fingerprint := strings.Repeat("a", 64)
	timestamp := "2026-07-20T10:00:19Z"

	switch eventType {
	case "run_started", "run_activated":
		events := canonicalLedgerEvents(t, schema2ProjectionInput(t, true))
		index := 0
		if eventType == "run_activated" {
			index = 1
		}
		return cloneAny(events[index]["data"]).(map[string]any)
	case "run_resumed":
		return schema2RepairRetryResumeData()
	case "node_waiting":
		return map[string]any{"nodeId": projectionTestMissionID, "neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []any{"edge_root_work"}}
	case "node_input_ignored":
		return map[string]any{"nodeId": projectionTestFormationID, "toPortId": "port_in", "inputRef": schema2RepairInputRef(), "reason": "late_optional", "relatedAttempt": 1}
	case "node_started":
		return map[string]any{"nodeId": projectionTestFormationID, "nodeKind": "formation", "attempt": 1, "reason": "initial", "inputRefs": []any{schema2RepairInputRef()}}
	case "slot_binding_observed":
		return map[string]any{"bindingId": bindingID, "slotId": "slot_worker", "sessionTargetId": targetID, "health": "runnable", "reason": "ready", "observedAt": timestamp, "relatedSeq": 2}
	case "slot_dispatch":
		return schema2RepairSlotDispatchData(t)
	case "slot_peek_capability_issued":
		return map[string]any{"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": bindingID, "sessionTargetId": targetID, "targetFingerprint": fingerprint, "capabilityGeneration": "1", "priorIssuedSeq": 0, "issuedAt": timestamp}
	case "slot_steering_started":
		return map[string]any{"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": bindingID, "sessionTargetId": targetID, "targetFingerprint": fingerprint, "capabilityIssuedSeq": 18, "capabilityGeneration": "1", "steeringGeneration": "1", "actor": "human:test", "startedAt": timestamp, "recordedBeforeInput": true}
	case "slot_steering_ended":
		return map[string]any{"startedSeq": 18, "dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprint": fingerprint, "steeringGeneration": "1", "reason": "released", "endedAt": timestamp}
	case "slot_peek_capability_revoked":
		return map[string]any{"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": bindingID, "sessionTargetId": targetID, "targetFingerprint": fingerprint, "capabilityGeneration": "1", "capabilityIssuedSeq": 18, "steeringGeneration": "1", "reason": "result_closure", "revokedAt": timestamp, "inputClosed": true}
	case "slot_reconciliation_interrupt":
		return map[string]any{"dispatchId": dispatchID, "targetLeaseId": leaseID, "bindingId": bindingID, "sessionTargetId": targetID, "targetFingerprint": fingerprint, "authorityKind": "cancel", "authoritySeq": 18, "interruptEncoding": "terminal-etx-v1", "interruptSha256": strings.Repeat("b", 64), "recordedBeforeSend": true}
	case "slot_reconciliation_interrupt_outcome":
		return map[string]any{"requestedSeq": 18, "dispatchId": dispatchID, "targetLeaseId": leaseID, "targetFingerprint": fingerprint, "outcome": "sent", "observedAt": timestamp}
	case "slot_result":
		return schema2RepairSlotResultData(t)
	case "formation_result":
		return schema2RepairFormationResultData()
	case "tool_dispatch":
		return schema2RepairToolDispatchData()
	case "tool_process_launch":
		return schema2RepairToolProcessLaunchData()
	case "tool_result":
		return schema2RepairToolResultData()
	case "node_output":
		return schema2RepairNodeOutputData(projectionTestMissionID)
	case "gate_evaluating":
		return schema2RepairGateEvaluatingData()
	case "gate_kind_result":
		return schema2RepairGateKindResultData()
	case "judge_result":
		return schema2RepairJudgeResultData()
	case "judge_attempt_failed":
		return schema2RepairJudgeAttemptFailedData()
	case "gate_verdict":
		return schema2RepairGateVerdictData()
	case "artifact_attached":
		return schema2RepairArtifactAttachedData()
	case "artifact_observed":
		projection := schema2RepairAvailableArtifactProjection()
		return map[string]any{"artifactId": "artifact_report", "availability": "available", "artifact": cloneAny(projection["artifact"]), "observedAt": timestamp, "relatedSeq": 18}
	case "escalation_raised":
		return map[string]any{"trigger": "operator_review", "severity": "needs-attention", "reason": "review", "source": "agent", "nodeId": projectionTestMissionID, "gateId": "", "blocks": true}
	case "human_input_requested":
		return schema2RepairHumanRequestData()
	case "human_verdict_recorded":
		return schema2RepairHumanVerdictData()
	case "error":
		return map[string]any{"code": "engine_failed", "message": "engine failed", "boundary": "engine", "errorScope": "run", "recoverable": true, "relatedSeq": 18}
	case "run_blocked":
		return map[string]any{"reason": "new run required", "blockScope": "run", "resumeAllowed": false, "resumePolicy": "new_run_required", "openDispatches": []any{}, "retryTargets": []any{}}
	case "run_cancel_requested":
		return schema2RepairCancelRequestedData(t)
	case "run_canceled":
		return schema2RepairRunCanceledData(t)
	case "run_failure_reconciliation_started":
		return schema2RepairFailureStartedData(t, map[string]any{"kind": "none"})
	case "run_failed":
		return schema2RepairRunFailedData(t)
	case "run_succeeded":
		return map[string]any{"summaryArtifactId": "", "outputArtifactIds": []any{}, "final": true}
	default:
		t.Fatalf("missing schema-2 fixture for %s", eventType)
		return nil
	}
}

func schema2SecondRepairPublicAllowedKeys() map[string][]string {
	return map[string][]string{
		"run_started":                           {"workspaceAuthorityId", "workspaceAdmissionSeq", "admissionPolicyRev", "admissionPolicySha256", "admissionCommandId", "commandPayloadSha256", "boardSlug", "sourceBoardSchema", "snapshotSchema", "runAuthorityId", "graphSnapshotSha256", "privateBindingsSha256", "bindingProjectionSha256", "runRoot", "rootInputProjection", "limits"},
		"run_activated":                         {"workspaceAdmissionSeq", "admissionPolicyRev", "admissionPolicySha256", "reason"},
		"run_resumed":                           {"commandId", "commandPayloadSha256", "resumedFromSeq", "resumedBy", "resumeMode", "reason", "openDispatches", "retryTargets"},
		"node_waiting":                          {"nodeId", "neededInputs", "readyInputs", "totalInputs", "waitingFor"},
		"node_input_ignored":                    {"nodeId", "toPortId", "inputRef", "reason", "relatedAttempt"},
		"node_started":                          {"nodeId", "nodeKind", "attempt", "reason", "inputRefs", "contextEncoding", "judgeContextSha256", "priorResultSeqs", "triggerFeedbackId", "priorGateSeq"},
		"slot_binding_observed":                 {"bindingId", "slotId", "sessionTargetId", "health", "reason", "observedAt", "relatedSeq"},
		"slot_dispatch":                         {"dispatchId", "targetLeaseId", "turnKey", "turnPhase", "turnInputs", "nodeId", "attempt", "slotId", "agentId", "harness", "bindingId", "sessionTargetId", "targetFingerprint", "dispatchInputBarrierEncoding", "dispatchInputBarrierSha256", "targetReadyProofEncoding", "targetReadyProofSha256", "paneHistoryBaselineEncoding", "paneHistoryBaselineSha256", "steeringGeneration", "promptSha256", "nativeAck", "recordedBeforeSend"},
		"slot_peek_capability_issued":           {"dispatchId", "targetLeaseId", "bindingId", "sessionTargetId", "targetFingerprint", "capabilityGeneration", "priorIssuedSeq", "issuedAt"},
		"slot_steering_started":                 {"dispatchId", "targetLeaseId", "bindingId", "sessionTargetId", "targetFingerprint", "capabilityIssuedSeq", "capabilityGeneration", "steeringGeneration", "actor", "startedAt", "recordedBeforeInput"},
		"slot_steering_ended":                   {"startedSeq", "dispatchId", "targetLeaseId", "targetFingerprint", "steeringGeneration", "reason", "endedAt"},
		"slot_peek_capability_revoked":          {"dispatchId", "targetLeaseId", "bindingId", "sessionTargetId", "targetFingerprint", "capabilityGeneration", "capabilityIssuedSeq", "steeringGeneration", "reason", "revokedAt", "inputClosed"},
		"slot_reconciliation_interrupt":         {"dispatchId", "targetLeaseId", "bindingId", "sessionTargetId", "targetFingerprint", "authorityKind", "authoritySeq", "interruptEncoding", "interruptSha256", "recordedBeforeSend"},
		"slot_reconciliation_interrupt_outcome": {"requestedSeq", "dispatchId", "targetLeaseId", "targetFingerprint", "outcome", "observedAt"},
		"slot_result":                           {"dispatchId", "targetLeaseId", "turnKey", "turnPhase", "nodeId", "attempt", "slotId", "agentId", "bindingId", "sessionTargetId", "targetFingerprint", "paneHistoryBaselineEncoding", "paneHistoryBaselineDispatchSeq", "paneHistoryBaselineSha256", "peekCapabilityRevokedSeq", "steeringGeneration", "operatorInfluenced", "status", "turnResult", "turnResultEncoding", "turnResultSha256", "clientAttachmentAuditProofSha256"},
		"formation_result":                      {"nodeId", "attempt", "status", "outputs", "outputHashes", "reportArtifactId", "artifactIds", "diffArtifactIds", "contributingSlotResultSeqs", "resultEncoding", "resultSha256"},
		"tool_dispatch":                         {"toolLeaseId", "nodeId", "attempt", "toolBindingId", "inputManifestSha256", "inputHashes", "profileSha256", "parametersSha256", "policySha256", "determinismPolicySha256", "executionBundleSha256", "recordedBeforeExecute"},
		"tool_process_launch":                   {"toolLeaseId", "launchId", "nodeId", "attempt", "generation", "recordedBeforeSpawn"},
		"tool_result":                           {"toolLeaseId", "launchId", "generation", "nodeId", "attempt", "status", "outputs", "outputHashes", "artifactRegistrations", "artifacts", "displayEvidence", "timing"},
		"node_output":                           {"nodeId", "status", "outputs", "reportArtifactId", "artifactIds", "diffArtifactIds", "producedBy", "timing", "deliveredEdges"},
		"gate_evaluating":                       {"gateId", "gateAttempt", "nodeId", "kinds", "criterionProjection", "inputRef", "judgeChain", "revisionCycleId", "triggerFeedbackId", "priorGateSeq"},
		"gate_kind_result":                      {"gateId", "gateAttempt", "kind", "verdict", "reason", "evidence", "evaluatedInputRef", "resultEncoding", "resultSha256", "relatedSeqs", "gateBindingId", "inputSha256", "profileSha256", "evaluatorBundleSha256", "parametersSha256", "policySha256", "determinismPolicySha256"},
		"judge_result":                          {"gateId", "gateAttempt", "judgeNodeId", "judgeAttempt", "chainIndex", "contextEncoding", "contextSha256", "priorResultSeqs", "result", "resultEncoding", "resultSha256"},
		"judge_attempt_failed":                  {"gateId", "gateAttempt", "judgeNodeId", "judgeAttempt", "chainIndex", "contextSha256", "priorResultSeqs", "code", "reason", "relatedSeq"},
		"gate_verdict":                          {"gateId", "gateAttempt", "verdict", "perKind", "kindResultSeqs", "evaluatedInputRef", "routePort", "routedEdges", "reason", "feedbackPayload"},
		"artifact_attached":                     {"artifactProjection", "source"},
		"artifact_observed":                     {"artifactId", "availability", "artifact", "errorCode", "observedAt", "relatedSeq"},
		"escalation_raised":                     {"trigger", "severity", "reason", "source", "nodeId", "gateId", "blocks"},
		"human_input_requested":                 {"gateId", "gateAttempt", "nodeId", "promptProjection", "choiceProjections", "requestedBy", "evaluatedInputRef", "completedKindResultSeqs"},
		"human_verdict_recorded":                {"commandId", "commandPayloadSha256", "gateId", "gateAttempt", "nodeId", "verdict", "reason", "requestedSeq", "decidedBy"},
		"error":                                 {"code", "message", "boundary", "errorScope", "nodeId", "gateId", "slotId", "toolLeaseId", "recoverable", "relatedSeq"},
		"run_blocked":                           {"reason", "blockScope", "blockedNodeId", "blockedGateId", "resumeAllowed", "resumePolicy", "openDispatches", "retryTargets", "nextEpoch"},
		"run_cancel_requested":                  {"commandId", "commandPayloadSha256", "reason", "requestedBy", "openNodeAttempts", "openSlotDispatches", "openToolLeases"},
		"run_canceled":                          {"cancelRequestSeq", "reason", "requestedBy", "nodeAttemptDispositions", "slotDispatchDispositions", "reconciledToolLeases", "final"},
		"run_failure_reconciliation_started":    {"originCancelRequestSeq", "code", "reason", "unrecoverable", "relatedSeq", "failureCause", "openNodeAttempts", "openSlotDispatches", "openToolLeases", "recordedBeforeReconciliation"},
		"run_failed":                            {"failureReconciliationSeq", "code", "reason", "unrecoverable", "relatedSeq", "failureCause", "nodeAttemptDispositions", "slotDispatchDispositions", "toolLeaseDispositions", "final"},
		"run_succeeded":                         {"summaryArtifactId", "outputArtifactIds", "final"},
	}
}

func schema2SecondRepairPublicRequiredKeys() map[string][]string {
	result := make(map[string][]string, 37)
	for eventType, allowed := range schema2SecondRepairPublicAllowedKeys() {
		result[eventType] = append([]string(nil), allowed...)
	}
	for eventType, optional := range map[string][]string{
		"node_started":     {"triggerFeedbackId", "priorGateSeq"},
		"formation_result": {"reportArtifactId"},
		"tool_result":      {"displayEvidence"},
		"node_output":      {"reportArtifactId"},
		"gate_evaluating":  {"revisionCycleId", "triggerFeedbackId", "priorGateSeq"},
		"run_succeeded":    {"summaryArtifactId"},
	} {
		for _, key := range optional {
			result[eventType] = withoutSecondRepairStrings(result[eventType], key)
		}
	}
	return result
}

func schema2SecondRepairSemanticNegatives(t *testing.T) []schema2SecondRepairSemanticMutation {
	t.Helper()
	tooLargeJSI := uint64(MaxJSONSafeInteger) + 1
	tooLong := strings.Repeat("x", (64<<10)+1)
	invalidID := "not an id"
	invalidHash := strings.Repeat("A", 64)
	set := func(key string, value any) func(map[string]any) {
		return func(data map[string]any) { data[key] = value }
	}
	remove := func(key string) func(map[string]any) {
		return func(data map[string]any) { delete(data, key) }
	}

	return []schema2SecondRepairSemanticMutation{
		{eventType: "run_activated", name: "zero admission sequence", mutate: set("workspaceAdmissionSeq", uint64(0))},
		{eventType: "run_activated", name: "nonpositive policy revision", mutate: set("admissionPolicyRev", uint64(0))},
		{eventType: "run_activated", name: "invalid policy hash", mutate: set("admissionPolicySha256", invalidHash)},
		{eventType: "run_activated", name: "unknown reason", mutate: set("reason", "later")},

		{eventType: "node_waiting", name: "invalid node id", mutate: set("nodeId", invalidID)},
		{eventType: "node_waiting", name: "count above JSON safe maximum", mutate: set("neededInputs", tooLargeJSI)},
		{eventType: "node_waiting", name: "ready count exceeds total", mutate: set("readyInputs", uint64(2))},
		{eventType: "node_waiting", name: "waiting identity is invalid", mutate: set("waitingFor", []any{invalidID})},
		{eventType: "node_waiting", name: "waiting count contradicts readiness", mutate: set("neededInputs", uint64(0))},

		{eventType: "node_started", name: "unknown node kind", mutate: set("nodeKind", "script")},
		{eventType: "node_started", name: "zero attempt", mutate: set("attempt", uint64(0))},
		{eventType: "node_started", name: "attempt above JSON safe maximum", mutate: set("attempt", tooLargeJSI)},
		{eventType: "node_started", name: "unknown reason", mutate: set("reason", "automatic")},
		{eventType: "node_started", name: "ordinary attempt carries judge context", mutate: set("contextEncoding", "judge-context-jcs-v1")},

		{eventType: "slot_binding_observed", name: "invalid binding id", mutate: set("bindingId", invalidID)},
		{eventType: "slot_binding_observed", name: "unknown health", mutate: set("health", "healthy")},
		{eventType: "slot_binding_observed", name: "invalid timestamp", mutate: set("observedAt", "not-a-time")},
		{eventType: "slot_binding_observed", name: "related sequence above JSON safe maximum", mutate: set("relatedSeq", tooLargeJSI)},
		{eventType: "slot_binding_observed", name: "reason exceeds bound", mutate: set("reason", tooLong)},

		{eventType: "slot_peek_capability_issued", name: "invalid dispatch id", mutate: set("dispatchId", invalidID)},
		{eventType: "slot_peek_capability_issued", name: "invalid target fingerprint", mutate: set("targetFingerprint", invalidHash)},
		{eventType: "slot_peek_capability_issued", name: "noncanonical capability generation", mutate: set("capabilityGeneration", "01")},
		{eventType: "slot_peek_capability_issued", name: "prior sequence above JSON safe maximum", mutate: set("priorIssuedSeq", tooLargeJSI)},
		{eventType: "slot_peek_capability_issued", name: "invalid issued timestamp", mutate: set("issuedAt", "not-a-time")},
		{eventType: "slot_peek_capability_issued", name: "first issuance generation mismatch", mutate: set("capabilityGeneration", "2")},

		{eventType: "slot_steering_started", name: "zero capability sequence", mutate: set("capabilityIssuedSeq", uint64(0))},
		{eventType: "slot_steering_started", name: "noncanonical capability generation", mutate: set("capabilityGeneration", "01")},
		{eventType: "slot_steering_started", name: "noncanonical steering generation", mutate: set("steeringGeneration", "01")},
		{eventType: "slot_steering_started", name: "steering generation contradicts capability", mutate: set("steeringGeneration", "0")},
		{eventType: "slot_steering_started", name: "actor exceeds bound", mutate: set("actor", tooLong)},
		{eventType: "slot_steering_started", name: "invalid started timestamp", mutate: set("startedAt", "not-a-time")},
		{eventType: "slot_steering_started", name: "recorded before input must be true", mutate: set("recordedBeforeInput", false)},

		{eventType: "slot_steering_ended", name: "zero started sequence", mutate: set("startedSeq", uint64(0))},
		{eventType: "slot_steering_ended", name: "noncanonical steering generation", mutate: set("steeringGeneration", "01")},
		{eventType: "slot_steering_ended", name: "unknown reason", mutate: set("reason", "complete")},
		{eventType: "slot_steering_ended", name: "invalid ended timestamp", mutate: set("endedAt", "not-a-time")},

		{eventType: "slot_peek_capability_revoked", name: "noncanonical capability generation", mutate: set("capabilityGeneration", "01")},
		{eventType: "slot_peek_capability_revoked", name: "capability sequence above JSON safe maximum", mutate: set("capabilityIssuedSeq", tooLargeJSI)},
		{eventType: "slot_peek_capability_revoked", name: "noncanonical steering generation", mutate: set("steeringGeneration", "01")},
		{eventType: "slot_peek_capability_revoked", name: "unknown reason", mutate: set("reason", "done")},
		{eventType: "slot_peek_capability_revoked", name: "invalid revoked timestamp", mutate: set("revokedAt", "not-a-time")},
		{eventType: "slot_peek_capability_revoked", name: "input closed must be true", mutate: set("inputClosed", false)},

		{eventType: "slot_reconciliation_interrupt", name: "unknown authority kind", mutate: set("authorityKind", "operator")},
		{eventType: "slot_reconciliation_interrupt", name: "zero authority sequence", mutate: set("authoritySeq", uint64(0))},
		{eventType: "slot_reconciliation_interrupt", name: "unknown interrupt encoding", mutate: set("interruptEncoding", "raw-etx")},
		{eventType: "slot_reconciliation_interrupt", name: "invalid interrupt hash", mutate: set("interruptSha256", invalidHash)},
		{eventType: "slot_reconciliation_interrupt", name: "recorded before send must be true", mutate: set("recordedBeforeSend", false)},

		{eventType: "slot_reconciliation_interrupt_outcome", name: "zero requested sequence", mutate: set("requestedSeq", uint64(0))},
		{eventType: "slot_reconciliation_interrupt_outcome", name: "unknown outcome", mutate: set("outcome", "failed")},
		{eventType: "slot_reconciliation_interrupt_outcome", name: "invalid observed timestamp", mutate: set("observedAt", "not-a-time")},

		{eventType: "tool_process_launch", name: "invalid tool lease id", mutate: set("toolLeaseId", invalidID)},
		{eventType: "tool_process_launch", name: "zero attempt", mutate: set("attempt", uint64(0))},
		{eventType: "tool_process_launch", name: "generation starts at one", mutate: set("generation", "0")},
		{eventType: "tool_process_launch", name: "noncanonical generation", mutate: set("generation", "01")},
		{eventType: "tool_process_launch", name: "recorded before spawn must be true", mutate: set("recordedBeforeSpawn", false)},

		{eventType: "judge_attempt_failed", name: "invalid gate id", mutate: set("gateId", invalidID)},
		{eventType: "judge_attempt_failed", name: "zero gate attempt", mutate: set("gateAttempt", uint64(0))},
		{eventType: "judge_attempt_failed", name: "invalid judge node id", mutate: set("judgeNodeId", invalidID)},
		{eventType: "judge_attempt_failed", name: "zero judge attempt", mutate: set("judgeAttempt", uint64(0))},
		{eventType: "judge_attempt_failed", name: "invalid context hash", mutate: set("contextSha256", invalidHash)},
		{eventType: "judge_attempt_failed", name: "prior result sequence is zero", mutate: set("priorResultSeqs", []any{0})},
		{eventType: "judge_attempt_failed", name: "wrong failure code", mutate: set("code", "judge_failed")},
		{eventType: "judge_attempt_failed", name: "reason exceeds bound", mutate: set("reason", tooLong)},
		{eventType: "judge_attempt_failed", name: "zero related sequence", mutate: set("relatedSeq", uint64(0))},

		{eventType: "artifact_observed", name: "unknown availability", mutate: set("availability", "missing")},
		{eventType: "artifact_observed", name: "available observation missing artifact", mutate: remove("artifact")},
		{eventType: "artifact_observed", name: "available descriptor id mismatch", mutate: func(data map[string]any) { data["artifact"].(map[string]any)["artifactId"] = "artifact_other" }},
		{eventType: "artifact_observed", name: "available observation carries error code", mutate: set("errorCode", "artifact_unavailable")},
		{eventType: "artifact_observed", name: "invalid observed timestamp", mutate: set("observedAt", "not-a-time")},
		{eventType: "artifact_observed", name: "related sequence above JSON safe maximum", mutate: set("relatedSeq", tooLargeJSI)},

		{eventType: "escalation_raised", name: "invalid trigger", mutate: set("trigger", invalidID)},
		{eventType: "escalation_raised", name: "unknown severity", mutate: set("severity", "warning")},
		{eventType: "escalation_raised", name: "unknown source", mutate: set("source", "operator")},
		{eventType: "escalation_raised", name: "reason exceeds bound", mutate: set("reason", tooLong)},
		{eventType: "escalation_raised", name: "invalid node id", mutate: set("nodeId", invalidID)},

		{eventType: "human_verdict_recorded", name: "invalid command id", mutate: set("commandId", invalidID)},
		{eventType: "human_verdict_recorded", name: "invalid command hash", mutate: set("commandPayloadSha256", invalidHash)},
		{eventType: "human_verdict_recorded", name: "invalid gate id", mutate: set("gateId", invalidID)},
		{eventType: "human_verdict_recorded", name: "zero gate attempt", mutate: set("gateAttempt", uint64(0))},
		{eventType: "human_verdict_recorded", name: "unknown verdict", mutate: set("verdict", "approve")},
		{eventType: "human_verdict_recorded", name: "reason exceeds bound", mutate: set("reason", tooLong)},
		{eventType: "human_verdict_recorded", name: "zero requested sequence", mutate: set("requestedSeq", uint64(0))},
		{eventType: "human_verdict_recorded", name: "empty decider", mutate: set("decidedBy", "")},

		{eventType: "error", name: "invalid code", mutate: set("code", invalidID)},
		{eventType: "error", name: "message exceeds bound", mutate: set("message", tooLong)},
		{eventType: "error", name: "unknown boundary", mutate: set("boundary", "network")},
		{eventType: "error", name: "unknown scope", mutate: set("errorScope", "attempt")},
		{eventType: "error", name: "related sequence above JSON safe maximum", mutate: set("relatedSeq", tooLargeJSI)},

		{eventType: "run_succeeded", name: "final must be true", mutate: set("final", false)},
		{eventType: "run_succeeded", name: "invalid summary artifact id", mutate: set("summaryArtifactId", invalidID)},
		{eventType: "run_succeeded", name: "invalid output artifact id", mutate: set("outputArtifactIds", []any{invalidID})},
		{eventType: "run_succeeded", name: "duplicate output artifact ids", mutate: set("outputArtifactIds", []any{"artifact_report", "artifact_report"})},
	}
}

func schema2SecondRepairSanitize(eventType string, data map[string]any) (SafeRunEvent, error) {
	event := schema2Event(projectionTestRunID, 20, eventType, data)
	if nodeID, ok := data["nodeId"].(string); ok && nodeID != "" {
		event["nodeId"] = nodeID
	}
	if gateID, ok := data["gateId"].(string); ok && gateID != "" {
		event["gateId"] = gateID
	}
	if slotID, ok := data["slotId"].(string); ok && slotID != "" {
		event["slotId"] = slotID
	}
	raw, err := decodeProjectionEvent(mustMarshalJSONNoTest(event), CanonicalRunSourceSchema2, projectionTestRunID)
	if err != nil {
		return nil, err
	}
	return sanitizeSchema2Event(raw)
}

func schema2SecondRepairRequirePublicRejection(t *testing.T, eventType string, data map[string]any) {
	t.Helper()
	_, err := schema2SecondRepairSanitize(eventType, data)
	if err == nil {
		raw := mustMarshalJSON(t, data)
		if len(raw) > 512 {
			raw = append(raw[:512], []byte("...<truncated>")...)
		}
		t.Fatalf("invalid %s data was admitted and exposed: %s", eventType, raw)
	}
	schema2SecondRepairRequireTypedRejection(t, err)
}

func schema2SecondRepairRequireTypedRejection(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrRunEventUnknown) && !errors.Is(err, ErrRunProjectionInvalid) {
		t.Fatalf("error = %T %v, want typed event or projection rejection", err, err)
	}
}

func withoutSecondRepairKey(input map[string]any, key string) map[string]any {
	result := cloneAny(input).(map[string]any)
	delete(result, key)
	return result
}

func withSecondRepairValue(input map[string]any, key string, value any) map[string]any {
	result := cloneAny(input).(map[string]any)
	result[key] = value
	return result
}

func withoutSecondRepairStrings(input []string, excluded string) []string {
	result := make([]string, 0, len(input)-1)
	for _, value := range input {
		if value != excluded {
			result = append(result, value)
		}
	}
	return result
}

func schema2SecondRepairContainsString(input []string, target string) bool {
	for _, value := range input {
		if value == target {
			return true
		}
	}
	return false
}

func schema2SecondRepairForbiddenValue(key string) any {
	switch key {
	case "contextEncoding":
		return "judge-context-jcs-v1"
	case "judgeContextSha256", "inputSha256", "profileSha256", "evaluatorBundleSha256", "parametersSha256", "policySha256", "determinismPolicySha256":
		return strings.Repeat("a", 64)
	case "priorResultSeqs":
		return []any{18}
	case "inputRefs":
		return []any{schema2RepairInputRef()}
	case "gateBindingId":
		return "gatebinding_lint"
	case "artifact":
		return cloneAny(schema2RepairAvailableArtifactProjection()["artifact"])
	case "errorCode":
		return "artifact_unavailable"
	case "nodeId":
		return projectionTestMissionID
	case "gateId", "blockedGateId":
		return projectionTestGateID
	case "slotId":
		return "slot_worker"
	case "toolLeaseId":
		return "toollease_01KXNP6VY3227H78329V52CKF8"
	case "blockedNodeId":
		return projectionTestMissionID
	case "nextEpoch":
		return uint64(1)
	default:
		return true
	}
}
