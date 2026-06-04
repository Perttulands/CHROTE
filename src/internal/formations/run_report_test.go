package formations

import (
	"os"
	"testing"
)

func TestS4BriefRefsAreIncludedInExecutionInput(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Ship a showcase"
beadId = "home-7kc4.7"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[formation.brief]
goal = "Ship the change"
beadId = "home-7kc4.7"
files = ["src/SessionPanel.tsx"]
links = ["https://example.com/spec"]

[[formation.input]]
id = "port_work_in"
label = "Input"

[[formation.output]]
id = "port_work_out"
label = "Output"

[[formation.slot]]
id = "slot_work"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_work"
from = "mis_showcase:out"
to = "fmn_work:port_work_in"
`)
	executor := &capturingBriefExecutor{}
	engine := NewRunEngine(store, personas, executor)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	if _, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
		Limits:            RunLimits{MaxDispatch: 4},
	}); err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if executor.brief.Goal != "Ship the change" || executor.brief.BeadID != "home-7kc4.7" {
		t.Fatalf("execution brief = %+v, want goal and home bead", executor.brief)
	}
	if len(executor.brief.Files) != 1 || executor.brief.Files[0] != "src/SessionPanel.tsx" {
		t.Fatalf("execution brief files = %+v", executor.brief.Files)
	}
	if len(executor.brief.Links) != 1 || executor.brief.Links[0] != "https://example.com/spec" {
		t.Fatalf("execution brief links = %+v", executor.brief.Links)
	}
}

func TestS4RunNodeReportProjectionFromLedgerOnly(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	engine := NewRunEngine(store, personas, staticReportExecutor{
		text:      "Report body from ledger",
		reportRef: "reports/fmn_research.md",
	})
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
		Limits:            RunLimits{MaxDispatch: 4},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if _, err := store.ReadBoard("session-search"); err != nil {
		t.Fatalf("board should be readable but not needed for projection: %v", err)
	}
	removeFile(t, store.BoardPath("session-search"))

	report, err := store.ProjectRunNodeReport(status.RunID, "fmn_research")
	if err != nil {
		t.Fatalf("project node report: %v", err)
	}
	if report.Text != "Report body from ledger" || report.ReportRef != "reports/fmn_research.md" || report.Status != "done" {
		t.Fatalf("node report = %+v, want ledger output fields", report)
	}
}

type capturingBriefExecutor struct {
	brief FormationBrief
}

func (e *capturingBriefExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	e.brief = req.Brief
	return FormationExecutionResult{Status: "done", Text: "done"}, nil
}

type staticReportExecutor struct {
	text      string
	reportRef string
}

func (e staticReportExecutor) ExecuteFormation(FormationExecution) (FormationExecutionResult, error) {
	return FormationExecutionResult{Status: "done", Text: e.text, ReportRef: e.reportRef}, nil
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
}
