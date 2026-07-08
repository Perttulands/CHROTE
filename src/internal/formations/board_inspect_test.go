package formations

import (
	"strings"
	"testing"
)

func cleanValidateBoardFixture() string {
	return `schema = 1
id = "brd_clean"
slug = "clean"
title = "Clean board"
rev = 3
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id = "mis_main"
title = "Main"
goal = "Ship it"
beadId = "home-1.1"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

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

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_in"
label = "Input"

[[formation.output]]
id = "port_ship_out"
label = "Output"

[[formation.slot]]
id = "slot_ship"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[gate]]
id = "gate_review"
title = "Script review"
kinds = ["script"]
criterion = "Run the gate"
commandArgv = ["./gate.sh"]
commandCwd = "."

[[connection]]
id = "edge_mission_work"
from = "mis_main:out"
to = "fmn_work:port_work_in"

[[connection]]
id = "edge_work_gate"
from = "fmn_work:port_work_out"
to = "gate_review:in"

[[connection]]
id = "edge_gate_pass_ship"
from = "gate_review:pass"
to = "fmn_ship:port_ship_in"
`
}

func mustParseValidateBoardFixture(t *testing.T, raw string) *BoardDocument {
	t.Helper()
	board, err := parseBoard([]byte(raw))
	if err != nil {
		t.Fatalf("parse board fixture: %v", err)
	}
	return board
}

func findBoardFindings(findings []BoardFinding, code string) []BoardFinding {
	var out []BoardFinding
	for _, finding := range findings {
		if finding.Code == code {
			out = append(out, finding)
		}
	}
	return out
}

func hasBoardFinding(findings []BoardFinding, nodeID, messageSubstring string) bool {
	for _, finding := range findings {
		if finding.NodeID == nodeID && strings.Contains(finding.Message, messageSubstring) {
			return true
		}
	}
	return false
}

func TestValidateBoardCleanBoardHasNoErrorsOrWarnings(t *testing.T) {
	report := ValidateBoard(mustParseValidateBoardFixture(t, cleanValidateBoardFixture()))
	if len(report.Errors) != 0 {
		t.Fatalf("clean board produced errors: %+v", report.Errors)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("clean board produced warnings: %+v", report.Warnings)
	}
}

func TestValidateBoardReportsDanglingConnectionEndpoints(t *testing.T) {
	raw := cleanValidateBoardFixture() + `
[[connection]]
id = "edge_broken_from"
from = "fmn_work:port_does_not_exist"
to = "fmn_ship:port_ship_in"

[[connection]]
id = "edge_broken_to"
from = "fmn_ship:port_ship_out"
to = "fmn_ghost:port_ghost_in"
`
	report := ValidateBoard(mustParseValidateBoardFixture(t, raw))
	dangling := findBoardFindings(report.Errors, FindingDanglingConnection)
	if len(dangling) != 2 {
		t.Fatalf("dangling connection errors = %d, want 2:\n%+v", len(dangling), report.Errors)
	}
	if !hasBoardFinding(dangling, "edge_broken_from", "fmn_work:port_does_not_exist") {
		t.Fatalf("missing precise dangling-from finding naming edge and endpoint:\n%+v", dangling)
	}
	if !hasBoardFinding(dangling, "edge_broken_to", "fmn_ghost:port_ghost_in") {
		t.Fatalf("missing precise dangling-to finding naming edge and endpoint:\n%+v", dangling)
	}
}

func TestValidateBoardReportsOnlyUnroutableGates(t *testing.T) {
	raw := cleanValidateBoardFixture() + `
[[gate]]
id = "gate_orphan"
title = "Orphan"
kinds = ["code"]
criterion = "Decide somehow"

[[gate]]
id = "gate_human"
title = "Human sanity gate"
kinds = ["human"]
criterion = "An operator confirms the result is real"
`
	report := ValidateBoard(mustParseValidateBoardFixture(t, raw))
	unroutable := findBoardFindings(report.Errors, FindingGateNotRoutable)
	if len(unroutable) != 1 || unroutable[0].NodeID != "gate_orphan" {
		t.Fatalf("unroutable gate findings = %+v, want only gate_orphan", unroutable)
	}
}

func TestValidateBoardRequiresExecutableRouteForMixedHumanScriptGate(t *testing.T) {
	raw := cleanValidateBoardFixture() + `
[[gate]]
id = "gate_mixed"
title = "Code plus human"
kinds = ["code", "human"]
criterion = "Automated checks pass and a human confirms"
`
	report := ValidateBoard(mustParseValidateBoardFixture(t, raw))
	unroutable := findBoardFindings(report.Errors, FindingGateNotRoutable)
	if len(unroutable) != 1 || unroutable[0].NodeID != "gate_mixed" {
		t.Fatalf("mixed human/script gate findings = %+v, want gate_mixed unroutable", unroutable)
	}
}

func TestValidateBoardScriptCommandForms(t *testing.T) {
	shellBoard := strings.Replace(cleanValidateBoardFixture(), `commandArgv = ["./gate.sh"]`, `commandShell = "./gate.sh"`, 1)
	if report := ValidateBoard(mustParseValidateBoardFixture(t, shellBoard)); len(findBoardFindings(report.Errors, FindingGateNotRoutable)) != 0 {
		t.Fatalf("commandShell gate was reported unroutable: %+v", report.Errors)
	}

	legacyOnly := strings.Replace(cleanValidateBoardFixture(), `commandArgv = ["./gate.sh"]`, `command = "./gate.sh"`, 1)
	report := ValidateBoard(mustParseValidateBoardFixture(t, legacyOnly))
	unroutable := findBoardFindings(report.Errors, FindingGateNotRoutable)
	if len(unroutable) != 1 || unroutable[0].NodeID != "gate_review" {
		t.Fatalf("legacy command findings = %+v, want gate_review unroutable", unroutable)
	}
}

func TestValidateBoardReportsInvalidFormationType(t *testing.T) {
	raw := cleanValidateBoardFixture() + `
[[formation]]
id = "fmn_bogus"
type = "wormhole"
title = "Bogus"
`
	report := ValidateBoard(mustParseValidateBoardFixture(t, raw))
	bad := findBoardFindings(report.Errors, FindingInvalidFormationType)
	if len(bad) != 1 || bad[0].NodeID != "fmn_bogus" {
		t.Fatalf("invalid-formation-type errors = %+v, want fmn_bogus", bad)
	}
}

func TestValidateBoardReportsMissionCountAndRunnability(t *testing.T) {
	noMission := strings.Replace(cleanValidateBoardFixture(), `[[mission]]
id = "mis_main"
title = "Main"
goal = "Ship it"
beadId = "home-1.1"

`, "", 1)
	report := ValidateBoard(mustParseValidateBoardFixture(t, noMission))
	if got := findBoardFindings(report.Errors, FindingMissionCount); len(got) != 1 {
		t.Fatalf("mission-count errors = %+v, want one", got)
	}

	unwired := strings.Replace(cleanValidateBoardFixture(), `[[connection]]
id = "edge_mission_work"
from = "mis_main:out"
to = "fmn_work:port_work_in"

`, "", 1)
	report = ValidateBoard(mustParseValidateBoardFixture(t, unwired))
	warnings := findBoardFindings(report.Warnings, FindingMissionNotRunnable)
	if len(warnings) != 1 || warnings[0].NodeID != "mis_main" {
		t.Fatalf("mission-runnable warnings = %+v, want mis_main", warnings)
	}
}
