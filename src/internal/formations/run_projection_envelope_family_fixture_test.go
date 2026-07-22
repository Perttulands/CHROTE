package formations

import "testing"

// schema2EnvelopeFixtureRule is the tests-only oracle for the frozen schema-2
// public-envelope contract. It is deliberately closed: adding a safe schema-2
// event requires classifying its authority envelope before fixtures can use it.
type schema2EnvelopeFixtureRule struct {
	EventType string
	Class     string
	Rule      string
}

func schema2EnvelopeFixtureRules() []schema2EnvelopeFixtureRule {
	return []schema2EnvelopeFixtureRule{
		{EventType: "run_started", Class: "C", Rule: "root_conditional"},
		{EventType: "run_activated", Class: "B", Rule: "run_scoped"},
		{EventType: "run_resumed", Class: "B", Rule: "multi_resource"},
		{EventType: "node_waiting", Class: "A", Rule: "node_without_attempt"},
		{EventType: "node_input_ignored", Class: "C", Rule: "direct_node_related_attempt"},
		{EventType: "node_started", Class: "A", Rule: "direct_node_attempt"},
		{EventType: "slot_binding_observed", Class: "C", Rule: "binding_node_slot_without_attempt"},
		{EventType: "slot_dispatch", Class: "C", Rule: "direct_node_slot_attempt"},
		{EventType: "slot_peek_capability_issued", Class: "C", Rule: "retained_dispatch_node_slot_attempt"},
		{EventType: "slot_steering_started", Class: "C", Rule: "retained_dispatch_node_slot_attempt"},
		{EventType: "slot_steering_ended", Class: "C", Rule: "retained_dispatch_node_slot_attempt"},
		{EventType: "slot_peek_capability_revoked", Class: "C", Rule: "retained_dispatch_node_slot_attempt"},
		{EventType: "slot_reconciliation_interrupt", Class: "C", Rule: "retained_dispatch_node_slot_attempt"},
		{EventType: "slot_reconciliation_interrupt_outcome", Class: "C", Rule: "retained_dispatch_node_slot_attempt"},
		{EventType: "slot_result", Class: "C", Rule: "direct_node_slot_attempt"},
		{EventType: "formation_result", Class: "C", Rule: "direct_node_attempt"},
		{EventType: "tool_dispatch", Class: "C", Rule: "direct_node_attempt"},
		{EventType: "tool_process_launch", Class: "C", Rule: "direct_node_attempt"},
		{EventType: "tool_result", Class: "C", Rule: "direct_node_attempt"},
		{EventType: "node_output", Class: "C", Rule: "direct_node_open_attempt"},
		{EventType: "gate_evaluating", Class: "C", Rule: "gate_node_gate_attempt"},
		{EventType: "gate_kind_result", Class: "C", Rule: "gate_node_gate_attempt"},
		{EventType: "judge_result", Class: "C", Rule: "judge_node_attempt_parent_gate"},
		{EventType: "judge_attempt_failed", Class: "C", Rule: "judge_node_attempt_parent_gate"},
		{EventType: "gate_verdict", Class: "C", Rule: "gate_node_gate_attempt"},
		{EventType: "artifact_attached", Class: "D", Rule: "task2_source_authority"},
		{EventType: "artifact_observed", Class: "B", Rule: "artifact_resource"},
		{EventType: "escalation_raised", Class: "C", Rule: "optional_node_and_gate_without_attempt"},
		{EventType: "human_input_requested", Class: "C", Rule: "gate_node_gate_attempt"},
		{EventType: "human_verdict_recorded", Class: "C", Rule: "gate_node_gate_attempt"},
		{EventType: "error", Class: "A", Rule: "scope_conditioned"},
		{EventType: "run_blocked", Class: "C", Rule: "scope_conditioned_without_attempt"},
		{EventType: "run_cancel_requested", Class: "B", Rule: "multi_resource"},
		{EventType: "run_canceled", Class: "B", Rule: "multi_resource"},
		{EventType: "run_failure_reconciliation_started", Class: "B", Rule: "multi_resource"},
		{EventType: "run_failed", Class: "B", Rule: "multi_resource"},
		{EventType: "run_succeeded", Class: "B", Rule: "run_scoped"},
	}
}

func TestSchema2EnvelopeFixtureRegistryIsClosed(t *testing.T) {
	t.Helper()
	registered := make(map[string]struct{}, len(schema2SafeEventTypes()))
	for _, event := range schema2SafeEventTypes() {
		registered[event.literal] = struct{}{}
	}

	counts := map[string]int{}
	seen := make(map[string]struct{}, len(registered))
	for _, rule := range schema2EnvelopeFixtureRules() {
		if _, ok := registered[rule.EventType]; !ok {
			t.Fatalf("envelope registry contains non-schema-2 event %q", rule.EventType)
		}
		if _, duplicate := seen[rule.EventType]; duplicate {
			t.Fatalf("envelope registry contains duplicate event %q", rule.EventType)
		}
		if rule.Rule == "" {
			t.Fatalf("envelope registry has empty rule for %q", rule.EventType)
		}
		seen[rule.EventType] = struct{}{}
		counts[rule.Class]++
	}
	if len(seen) != 37 || len(seen) != len(registered) {
		t.Fatalf("envelope registry covers %d events; schema-2 registry covers %d", len(seen), len(registered))
	}
	for eventType := range registered {
		if _, ok := seen[eventType]; !ok {
			t.Fatalf("schema-2 event %q lacks an audited envelope rule", eventType)
		}
	}
	if counts["A"] != 3 || counts["B"] != 8 || counts["C"] != 25 || counts["D"] != 1 || len(counts) != 4 {
		t.Fatalf("envelope audit class counts = %#v; want A=3 B=8 C=25 D=1", counts)
	}
}

func schema2ApplyFixtureEnvelope(event map[string]any, eventType string, data map[string]any) {
	delete(event, "nodeId")
	delete(event, "slotId")
	delete(event, "gateId")
	delete(event, "attempt")

	switch eventType {
	case "run_started":
		root, _ := data["runRoot"].(map[string]any)
		if root["kind"] == "mission" {
			event["missionId"] = root["nodeId"]
		} else if root["kind"] == "formation" {
			delete(event, "missionId")
			delete(event, "beadId")
		}
	case "run_activated", "run_resumed", "artifact_attached", "artifact_observed",
		"run_cancel_requested", "run_canceled", "run_failure_reconciliation_started",
		"run_failed", "run_succeeded":
		// Run-, resource-, and multi-resource events have no singular graph subject.
	case "node_waiting":
		schema2SetFixtureIdentity(event, "nodeId", data["nodeId"])
	case "node_input_ignored":
		schema2SetFixtureIdentity(event, "nodeId", data["nodeId"])
		schema2SetFixtureIdentity(event, "attempt", data["relatedAttempt"])
	case "node_started":
		schema2SetFixtureIdentity(event, "nodeId", data["nodeId"])
		schema2SetFixtureIdentity(event, "attempt", data["attempt"])
	case "slot_binding_observed":
		schema2SetFixtureIdentity(event, "nodeId", projectionTestFormationID)
		schema2SetFixtureIdentity(event, "slotId", data["slotId"])
	case "slot_dispatch", "slot_result":
		schema2SetFixtureIdentity(event, "nodeId", data["nodeId"])
		schema2SetFixtureIdentity(event, "slotId", data["slotId"])
		schema2SetFixtureIdentity(event, "attempt", data["attempt"])
	case "slot_peek_capability_issued", "slot_steering_started", "slot_steering_ended",
		"slot_peek_capability_revoked", "slot_reconciliation_interrupt",
		"slot_reconciliation_interrupt_outcome":
		slotID := "slot_worker"
		if data["bindingId"] == "binding_reviewer" || data["dispatchId"] == "dsp_01KXNP6VY3227H78329V52CKF9" {
			slotID = "slot_reviewer"
		}
		schema2SetFixtureIdentity(event, "nodeId", projectionTestFormationID)
		schema2SetFixtureIdentity(event, "slotId", slotID)
		schema2SetFixtureIdentity(event, "attempt", uint64(1))
	case "formation_result", "tool_dispatch", "tool_process_launch", "tool_result":
		schema2SetFixtureIdentity(event, "nodeId", data["nodeId"])
		schema2SetFixtureIdentity(event, "attempt", data["attempt"])
	case "node_output":
		schema2SetFixtureIdentity(event, "nodeId", data["nodeId"])
		schema2SetFixtureIdentity(event, "attempt", uint64(1))
	case "gate_evaluating", "human_input_requested", "human_verdict_recorded":
		schema2SetFixtureIdentity(event, "nodeId", data["nodeId"])
		schema2SetFixtureIdentity(event, "gateId", data["gateId"])
		schema2SetFixtureIdentity(event, "attempt", data["gateAttempt"])
	case "gate_kind_result", "gate_verdict":
		schema2SetFixtureIdentity(event, "nodeId", data["gateId"])
		schema2SetFixtureIdentity(event, "gateId", data["gateId"])
		schema2SetFixtureIdentity(event, "attempt", data["gateAttempt"])
	case "judge_result", "judge_attempt_failed":
		schema2SetFixtureIdentity(event, "nodeId", data["judgeNodeId"])
		schema2SetFixtureIdentity(event, "gateId", data["gateId"])
		schema2SetFixtureIdentity(event, "attempt", data["judgeAttempt"])
	case "escalation_raised":
		schema2SetFixtureIdentity(event, "nodeId", data["nodeId"])
		schema2SetFixtureIdentity(event, "gateId", data["gateId"])
	case "error":
		for _, member := range []string{"nodeId", "slotId", "gateId", "attempt"} {
			schema2SetFixtureIdentity(event, member, data[member])
		}
	case "run_blocked":
		schema2SetFixtureIdentity(event, "nodeId", data["blockedNodeId"])
		schema2SetFixtureIdentity(event, "gateId", data["blockedGateId"])
	default:
		panic("unclassified schema-2 envelope fixture: " + eventType)
	}
}

func schema2SetFixtureIdentity(event map[string]any, member string, value any) {
	switch typed := value.(type) {
	case string:
		if typed != "" {
			event[member] = typed
		}
	case uint64:
		if typed != 0 {
			event[member] = typed
		}
	case int:
		if typed > 0 {
			event[member] = typed
		}
	case float64:
		if typed > 0 {
			event[member] = typed
		}
	}
}
