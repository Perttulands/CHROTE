package formations

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCareerWebAcceptance(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createCareerPersona(t, personas, "design-lead", "claude-code", "design")
	createCareerPersona(t, personas, "frontend-codex", "openai-codex", "react")
	writeFixture(t, store.BoardPath("career-web"), careerWebBoardFixture())
	board, err := store.ReadBoard("career-web")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	executor := &careerDispatchExecutor{
		store:      store,
		dispatcher: NewSlotDispatcher(store, &fakeDispatchAdapter{}),
	}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"pass"}})

	status, err := engine.RunMission("career-web", RunStartRequest{
		MissionID:         "mis_showcase_site",
		Actor:             "agent:archon",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
		Limits:            RunLimits{MaxDispatch: 1, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked || !status.ResumeAllowed {
		t.Fatalf("initial status = %+v, want resumable blocked after first dispatch slice", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_design"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("initial executor nodes = %v, want %v", got, want)
	}

	status, err = engine.ResumeRun(status.RunID, RunResumeRequest{
		Actor:  "agent:archon",
		Mode:   "reattach",
		Reason: "operator returned after disconnect",
	})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if status.Status != RunStatusRunning || status.Final {
		t.Fatalf("resumed status = %+v, want running while waiting for human gate", status)
	}
	if got := executor.callCount("fmn_design"); got != 1 {
		t.Fatalf("design node call count = %d, want completed node not re-run after resume", got)
	}
	if got := executor.callCount("fmn_frontend"); got != 1 {
		t.Fatalf("frontend node call count = %d, want resumed incomplete node run once", got)
	}
	artifactPath := filepath.Join(store.Workspace, "index.html")
	if raw, err := os.ReadFile(artifactPath); err != nil || len(raw) == 0 {
		t.Fatalf("artifact %s read err=%v len=%d, want real workspace artifact", artifactPath, err, len(raw))
	}

	recorded, err := store.RecordEscalationFromCapture(status.RunID, "fmn_frontend", "note\n<<<CHROTE-ESCALATE run-id="+status.RunID+" severity=needs-attention reason='frontend wants human taste'>>>")
	if err != nil {
		t.Fatalf("record escalation: %v", err)
	}
	if !recorded {
		t.Fatal("matching escalation sentinel was not recorded")
	}
	open, err := store.ProjectOpenEscalations(status.RunID)
	if err != nil {
		t.Fatalf("project open escalations: %v", err)
	}
	if len(open) != 1 || open[0].NodeID != "fmn_frontend" || open[0].Reason != "frontend wants human taste" {
		t.Fatalf("open escalations = %+v, want frontend human-taste escalation", open)
	}

	status, err = engine.RecordHumanGateVerdict(status.RunID, HumanGateVerdictRequest{
		GateID:  "gate_taste",
		Verdict: "pass",
		Reason:  "artifact fits the job-search story",
		Actor:   "human:perttu",
	})
	if err != nil {
		t.Fatalf("record human verdict: %v", err)
	}
	if status.Status != RunStatusBlocked || status.Final || !status.ResumeAllowed {
		t.Fatalf("post-verdict status = %+v, want resumable block before route dispatch", status)
	}
	status, err = engine.ResumeRun(status.RunID, RunResumeRequest{
		Actor:  "agent:archon",
		Mode:   "reattach",
		Reason: "human gate approved",
	})
	if err != nil {
		t.Fatalf("resume after human verdict: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("final status = %+v, want succeeded after resume routes pass wire", status)
	}

	report, err := store.ProjectRunNodeReport(status.RunID, "fmn_frontend")
	if err != nil {
		t.Fatalf("project frontend report: %v", err)
	}
	if report.ReportRef != "index.html" || report.Text == "" {
		t.Fatalf("frontend report = %+v, want ledger-derived index.html report", report)
	}

	events := readRunEvents(t, filepath.Join(store.Workspace, statusLedgerPath(t, store, status.RunID)))
	if !eventTypesContainSubsequence(events, []string{
		RunEventStarted,
		RunEventNodeStarted,
		RunEventSlotDispatch,
		RunEventSlotResult,
		RunEventNodeOutput,
		RunEventBlocked,
		RunEventResumed,
		RunEventHumanInputRequested,
		RunEventEscalationRaised,
		RunEventHumanVerdictRecorded,
		RunEventGateVerdict,
		RunEventBlocked,
		RunEventResumed,
		RunEventSucceeded,
	}) {
		t.Fatalf("event sequence = %v, want career-web acceptance cascade/recovery/human-gate subsequence", eventTypes(events))
	}
	if !slotDispatchHarnessesInclude(events, "claude-code", "openai-codex") {
		t.Fatalf("slot dispatch harnesses missing cross-harness proof in events: %#v", events)
	}

	deadStarted, err := store.StartRun("career-web", RunStartRequest{
		MissionID:         "mis_showcase_site",
		Actor:             "agent:archon",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start dead-pane run: %v", err)
	}
	deadDispatcher := NewSlotDispatcher(store, &fakeDispatchAdapter{err: ErrDispatchDeadPane})
	if _, err := deadDispatcher.DispatchSlot(deadStarted.RunID, SlotDispatchRequest{
		NodeID:      "fmn_frontend",
		SlotID:      "slot_frontend",
		AgentID:     "frontend-codex",
		Harness:     "openai-codex",
		SessionStem: "frontend-codex",
		SessionRef:  "tmux:frontend-codex",
		Prompt:      "Build index.html",
		Attempt:     1,
	}); err == nil {
		t.Fatal("dead pane dispatch returned nil error, want loud fake-adapter error")
	}
	deadEvents := readRunEvents(t, filepath.Join(store.Workspace, deadStarted.LedgerPath))
	if eventOfType(t, deadEvents, RunEventError).Data["code"] != "dead_pane" {
		t.Fatalf("dead-pane events = %#v, want dead_pane error", deadEvents)
	}
}

type careerDispatchExecutor struct {
	store      *Store
	dispatcher *SlotDispatcher
	calls      []string
}

func (e *careerDispatchExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	e.calls = append(e.calls, req.NodeID)
	slot := req.Formation.Slots[0]
	artifact := "reports/" + req.NodeID + ".md"
	text := "output from " + req.NodeID
	if req.NodeID == "fmn_frontend" {
		artifact = "index.html"
		text = "Built index.html for Perttu's agentic-engineering job search story."
		if err := os.WriteFile(filepath.Join(e.store.Workspace, artifact), []byte("<!doctype html><title>Agentic Engineering</title>"), 0o644); err != nil {
			return FormationExecutionResult{}, err
		}
	}
	lease, err := e.dispatcher.DispatchSlot(req.RunID, SlotDispatchRequest{
		NodeID:      req.NodeID,
		SlotID:      slot.ID,
		AgentID:     slot.AgentID,
		Harness:     slot.Harness,
		SessionStem: slot.AgentID,
		SessionRef:  "tmux:" + slot.AgentID,
		Prompt:      fmt.Sprintf("%s: %s", req.Title, req.Brief.Goal),
		Attempt:     req.Attempt,
	})
	if err != nil {
		return FormationExecutionResult{}, err
	}
	if err := e.dispatcher.CompleteFromCapture(req.RunID, lease.DispatchID, fmt.Sprintf("<<<CHROTE-DONE run-id=%s status=ok artifact=%s>>>", req.RunID, artifact)); err != nil {
		return FormationExecutionResult{}, err
	}
	return FormationExecutionResult{Status: "done", ReportRef: artifact, Text: text, Outputs: payloadsForFormationOutputs(req.Formation, text, artifact)}, nil
}

func (e *careerDispatchExecutor) nodeIDs() []string {
	return append([]string(nil), e.calls...)
}

func (e *careerDispatchExecutor) callCount(nodeID string) int {
	count := 0
	for _, call := range e.calls {
		if call == nodeID {
			count++
		}
	}
	return count
}

func createCareerPersona(t *testing.T, personas *PersonaStore, id, harness, capability string) {
	t.Helper()
	if _, err := personas.CreatePersona(CreatePersonaRequest{
		ID:           id,
		Kind:         "specialist",
		Capabilities: []string{capability},
		Harness:      harness,
	}); err != nil {
		t.Fatalf("create persona %s: %v", id, err)
	}
}

func statusLedgerPath(t *testing.T, store *Store, runID string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(store.Workspace, ".formations", "runs", "career-web", runID+".ndjson"))
	if err != nil {
		t.Fatalf("glob run ledger: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("ledger matches for %s = %v, want one", runID, matches)
	}
	rel, err := filepath.Rel(store.Workspace, matches[0])
	if err != nil {
		t.Fatalf("relative ledger path: %v", err)
	}
	return rel
}

func eventTypes(events []RunEvent) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func eventTypesContainSubsequence(events []RunEvent, want []string) bool {
	types := eventTypes(events)
	index := 0
	for _, typ := range types {
		if index < len(want) && typ == want[index] {
			index++
		}
	}
	return index == len(want)
}

func slotDispatchHarnessesInclude(events []RunEvent, want ...string) bool {
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type == RunEventSlotDispatch {
			if harness, ok := event.Data["harness"].(string); ok {
				seen[harness] = true
			}
		}
	}
	for _, harness := range want {
		if !seen[harness] {
			return false
		}
	}
	return true
}

func careerWebBoardFixture() string {
	return `schema = 1
id = "brd_career_web"
slug = "career-web"
title = "Career web experience"
rev = 7
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id = "mis_showcase_site"
title = "showcase-site"
goal = "I want a web experience that showcases my agentic-engineering work for an AI-company job search"
beadId = "home-7kc4.9"

[[formation]]
id = "fmn_design"
type = "solo"
title = "Design lead"

[formation.brief]
goal = "Frame the narrative and hand off the visual direction."
beadId = "home-7kc4.9"

[[formation.input]]
id = "port_design_in"
label = "Input"

[[formation.output]]
id = "port_design_out"
label = "Output"

[[formation.slot]]
id = "slot_design"
label = "Design lead"
agentId = "design-lead"
harness = "claude-code"
controller = true

[[formation]]
id = "fmn_frontend"
type = "solo"
title = "Frontend specialist"

[formation.brief]
goal = "Build the static index.html artifact from the design direction."
beadId = "home-7kc4.9"

[[formation.input]]
id = "port_frontend_in"
label = "Input"

[[formation.output]]
id = "port_frontend_out"
label = "Output"

[[formation.slot]]
id = "slot_frontend"
label = "Frontend"
agentId = "frontend-codex"
harness = "openai-codex"
controller = true

[[gate]]
id = "gate_taste"
title = "Taste review"
kinds = ["code", "human"]
criterion = "Good enough to ship for an AI-company job search"

[[formation]]
id = "fmn_publish"
type = "solo"
title = "Publish"

[[formation.input]]
id = "port_publish_in"
label = "Input"

[[formation.output]]
id = "port_publish_out"
label = "Output"

[[formation.slot]]
id = "slot_publish"
label = "Publisher"
agentId = "frontend-codex"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_design"
from = "mis_showcase_site:out"
to = "fmn_design:port_design_in"

[[connection]]
id = "edge_design_frontend"
from = "fmn_design:port_design_out"
to = "fmn_frontend:port_frontend_in"

[[connection]]
id = "edge_frontend_gate"
from = "fmn_frontend:port_frontend_out"
to = "gate_taste:in"

[[connection]]
id = "edge_gate_publish"
from = "gate_taste:pass"
to = "fmn_publish:port_publish_in"
`
}
