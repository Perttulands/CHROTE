package formations

import "testing"

func TestS5EscalationSentinelRecordsLedgerAndProjection(t *testing.T) {
	store, started := startS4DispatchRun(t)

	recorded, err := store.RecordEscalationFromCapture(started.RunID, "fmn_research", "noise\n<<<CHROTE-ESCALATE run-id="+started.RunID+" severity=needs-attention reason='found a better direction'>>>\nmore")
	if err != nil {
		t.Fatalf("record escalation: %v", err)
	}
	if !recorded {
		t.Fatal("matching escalation sentinel was not recorded")
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	escalation := eventOfType(t, events, RunEventEscalationRaised)
	if escalation.NodeID != "fmn_research" {
		t.Fatalf("escalation node = %q, want fmn_research", escalation.NodeID)
	}
	if escalation.Data["trigger"] != "sentinel" || escalation.Data["source"] != "agent" || escalation.Data["severity"] != "needs-attention" || escalation.Data["reason"] != "found a better direction" || escalation.Data["blocks"] != false {
		t.Fatalf("escalation data = %#v, want sentinel agent reason with non-blocking severity", escalation.Data)
	}

	open, err := store.ProjectOpenEscalations(started.RunID)
	if err != nil {
		t.Fatalf("project open escalations: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("open escalations = %#v, want one", open)
	}
	if open[0].RunID != started.RunID || open[0].NodeID != "fmn_research" || open[0].Reason != "found a better direction" || open[0].Severity != "needs-attention" {
		t.Fatalf("open escalation projection = %+v, want ledger-derived reason/severity", open[0])
	}
}

func TestS5EscalationSentinelIgnoresOtherRunIDs(t *testing.T) {
	store, started := startS4DispatchRun(t)

	recorded, err := store.RecordEscalationFromCapture(started.RunID, "fmn_research", "<<<CHROTE-ESCALATE run-id=run_other severity=needs-attention reason='wrong run'>>>")
	if err != nil {
		t.Fatalf("record wrong-run escalation: %v", err)
	}
	if recorded {
		t.Fatal("wrong-run escalation sentinel was recorded")
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	for _, event := range events {
		if event.Type == RunEventEscalationRaised {
			t.Fatalf("wrong-run escalation event recorded: %+v", event)
		}
	}
}
