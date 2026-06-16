package formations

import (
	"errors"
	"strings"
	"testing"
)

// hasFindingMatching reports whether any finding has the given node id and a
// message that mentions the given substring (used to assert the endpoint that is
// named in a dangling-connection message).
func hasFindingMatching(findings []BoardFinding, nodeID, messageSubstring string) bool {
	for _, finding := range findings {
		if finding.NodeID == nodeID && strings.Contains(finding.Message, messageSubstring) {
			return true
		}
	}
	return false
}

func errorMentions(err error, substring string) bool {
	return err != nil && strings.Contains(err.Error(), substring)
}

// cleanInspectBoardFixture is a structurally sound board: a mission wired to a
// formation, a script gate with a routing config, and valid endpoints. It must
// produce zero validation errors and zero warnings.
func cleanInspectBoardFixture() string {
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

[[gate]]
id = "gate_review"
title = "Script review"
kinds = ["script"]
criterion = "Run the gate"
scriptRoot = "/srv/repo"
scriptCwd = "/srv/repo"
scriptCommand = ["./gate.sh"]
scriptTimeoutSeconds = 30
scriptOutputLimitBytes = 65536

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

func mustParseBoardFixture(t *testing.T, raw string) *BoardDocument {
	t.Helper()
	board, err := parseBoard([]byte(raw))
	if err != nil {
		t.Fatalf("parse board fixture: %v", err)
	}
	return board
}

func findFindings(findings []BoardFinding, code string) []BoardFinding {
	var out []BoardFinding
	for _, finding := range findings {
		if finding.Code == code {
			out = append(out, finding)
		}
	}
	return out
}

func TestValidateBoardCleanBoardHasNoErrorsOrWarnings(t *testing.T) {
	report := ValidateBoard(mustParseBoardFixture(t, cleanInspectBoardFixture()))
	if len(report.Errors) != 0 {
		t.Fatalf("clean board produced errors: %+v", report.Errors)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("clean board produced warnings: %+v", report.Warnings)
	}
}

func TestValidateBoardReportsDanglingConnectionEndpoints(t *testing.T) {
	// edge_broken_from references a port that does not exist on fmn_work;
	// edge_broken_to references a node that does not exist at all.
	raw := cleanInspectBoardFixture() + `
[[connection]]
id = "edge_broken_from"
from = "fmn_work:port_does_not_exist"
to = "fmn_ship:port_ship_in"

[[connection]]
id = "edge_broken_to"
from = "fmn_ship:port_ship_out"
to = "fmn_ghost:port_ghost_in"
`
	report := ValidateBoard(mustParseBoardFixture(t, raw))

	dangling := findFindings(report.Errors, FindingDanglingConnection)
	if len(dangling) != 2 {
		t.Fatalf("dangling connection errors = %d, want 2:\n%+v", len(dangling), report.Errors)
	}

	// The bad-from finding must name the offending edge and its from endpoint.
	if !hasFindingMatching(dangling, "edge_broken_from", "fmn_work:port_does_not_exist") {
		t.Fatalf("missing precise dangling-from finding naming edge and endpoint:\n%+v", dangling)
	}
	// The bad-to finding must name the offending edge and its to endpoint.
	if !hasFindingMatching(dangling, "edge_broken_to", "fmn_ghost:port_ghost_in") {
		t.Fatalf("missing precise dangling-to finding naming edge and endpoint:\n%+v", dangling)
	}
}

func TestValidateBoardReportsGateWithNeitherScriptNorJudge(t *testing.T) {
	// gate_orphan has no script config and no judge chain, so it can never route.
	raw := cleanInspectBoardFixture() + `
[[gate]]
id = "gate_orphan"
title = "Orphan"
kinds = ["code"]
criterion = "Decide somehow"
`
	report := ValidateBoard(mustParseBoardFixture(t, raw))

	unroutable := findFindings(report.Errors, FindingGateNotRoutable)
	if len(unroutable) != 1 {
		t.Fatalf("gate-not-routable errors = %d, want 1:\n%+v", len(unroutable), report.Errors)
	}
	if unroutable[0].NodeID != "gate_orphan" {
		t.Fatalf("gate-not-routable NodeID = %q, want gate_orphan", unroutable[0].NodeID)
	}
	// The script gate in the clean fixture must NOT be flagged.
	for _, finding := range unroutable {
		if finding.NodeID == "gate_review" {
			t.Fatalf("script gate gate_review was wrongly flagged as not routable")
		}
	}
}

func TestValidateBoardJudgeGateIsRoutable(t *testing.T) {
	// gate_review_judge routes via a judge chain (judge -> formation -> judge),
	// not a script config, and must NOT be flagged as unroutable.
	raw := cleanInspectBoardFixture() + `
[[gate]]
id = "gate_review_judge"
title = "Judge review"
kinds = ["code"]
criterion = "Judge it"

[[formation]]
id = "fmn_judge"
type = "solo"
title = "Judge"

[[formation.input]]
id = "port_judge_in"
label = "Input"

[[formation.output]]
id = "port_judge_out"
label = "Output"

[[connection]]
id = "edge_judge_enter"
from = "gate_review_judge:judge"
to = "fmn_judge:port_judge_in"

[[connection]]
id = "edge_judge_return"
from = "fmn_judge:port_judge_out"
to = "gate_review_judge:judge"
`
	report := ValidateBoard(mustParseBoardFixture(t, raw))
	for _, finding := range findFindings(report.Errors, FindingGateNotRoutable) {
		if finding.NodeID == "gate_review_judge" {
			t.Fatalf("judge-chain gate was wrongly flagged as not routable:\n%+v", report.Errors)
		}
	}
}

func TestValidateBoardHumanGateIsRoutable(t *testing.T) {
	// A "human"-kind gate routes via an operator verdict (RecordHumanGateVerdict);
	// it has neither a script config nor a judge chain by design, and must NOT be
	// flagged as unroutable. This guards the regression where every human gate on
	// real boards (e.g. "Human sanity gate") was wrongly reported as an error.
	raw := cleanInspectBoardFixture() + `
[[gate]]
id = "gate_human"
title = "Human sanity gate"
kinds = ["human"]
criterion = "An operator confirms the result is real"
`
	report := ValidateBoard(mustParseBoardFixture(t, raw))
	for _, finding := range findFindings(report.Errors, FindingGateNotRoutable) {
		if finding.NodeID == "gate_human" {
			t.Fatalf("human gate was wrongly flagged as not routable:\n%+v", report.Errors)
		}
	}
}

func TestValidateBoardReportsInvalidFormationType(t *testing.T) {
	raw := cleanInspectBoardFixture() + `
[[formation]]
id = "fmn_bogus"
type = "wormhole"
title = "Bogus"
`
	report := ValidateBoard(mustParseBoardFixture(t, raw))
	bad := findFindings(report.Errors, FindingInvalidFormationType)
	if len(bad) != 1 {
		t.Fatalf("invalid-formation-type errors = %d, want 1:\n%+v", len(bad), report.Errors)
	}
	if bad[0].NodeID != "fmn_bogus" {
		t.Fatalf("invalid-formation-type NodeID = %q, want fmn_bogus", bad[0].NodeID)
	}
}

func TestValidateBoardWarnsOnMissionWithoutOutgoingConnection(t *testing.T) {
	// mis_orphan has no outgoing connection, so RunMission can never start it.
	raw := cleanInspectBoardFixture() + `
[[mission]]
id = "mis_orphan"
title = "Orphan mission"
goal = "Never runs"
beadId = "home-2.2"
`
	report := ValidateBoard(mustParseBoardFixture(t, raw))

	warnings := findFindings(report.Warnings, FindingMissionNotRunnable)
	if len(warnings) != 1 {
		t.Fatalf("mission-not-runnable warnings = %d, want 1:\n%+v", len(warnings), report.Warnings)
	}
	if warnings[0].NodeID != "mis_orphan" {
		t.Fatalf("mission-not-runnable NodeID = %q, want mis_orphan", warnings[0].NodeID)
	}
	// The wired mission mis_main must NOT be flagged.
	for _, finding := range warnings {
		if finding.NodeID == "mis_main" {
			t.Fatalf("wired mission mis_main was wrongly warned as not runnable")
		}
	}
}

// TestValidateBoardReportsMissionCountNotExactlyOne locks the Mission Board
// identity invariant in the validator: a board must have EXACTLY one mission.
// Zero missions means the board has no identity to run; more than one means the
// runner would need a picker, which a Mission Board deliberately does not have.
// Both are hard errors with a precise count in the message.
func TestValidateBoardReportsMissionCountNotExactlyOne(t *testing.T) {
	// Zero missions: strip the only mission from the clean fixture.
	noMission := strings.Replace(cleanInspectBoardFixture(), `[[mission]]
id = "mis_main"
title = "Main"
goal = "Ship it"
beadId = "home-1.1"

`, "", 1)
	zeroReport := ValidateBoard(mustParseBoardFixture(t, noMission))
	zero := findFindings(zeroReport.Errors, FindingMissionCount)
	if len(zero) != 1 {
		t.Fatalf("zero-mission errors = %d, want exactly 1:\n%+v", len(zero), zeroReport.Errors)
	}
	if !strings.Contains(zero[0].Message, "exactly one mission") || !strings.Contains(zero[0].Message, "found none") {
		t.Fatalf("zero-mission message = %q, want it to state exactly-one and found-none", zero[0].Message)
	}

	// Two missions: append a second mission to the clean fixture.
	twoMissions := cleanInspectBoardFixture() + `
[[mission]]
id = "mis_extra"
title = "Extra"
goal = "One too many"
beadId = "home-2.2"
`
	twoReport := ValidateBoard(mustParseBoardFixture(t, twoMissions))
	two := findFindings(twoReport.Errors, FindingMissionCount)
	if len(two) != 1 {
		t.Fatalf("two-mission errors = %d, want exactly 1:\n%+v", len(two), twoReport.Errors)
	}
	if !strings.Contains(two[0].Message, "exactly one mission") || !strings.Contains(two[0].Message, "found 2") {
		t.Fatalf("two-mission message = %q, want it to state exactly-one and found 2", two[0].Message)
	}

	// Exactly one mission (the clean fixture) must NOT produce a mission_count error.
	oneReport := ValidateBoard(mustParseBoardFixture(t, cleanInspectBoardFixture()))
	if one := findFindings(oneReport.Errors, FindingMissionCount); len(one) != 0 {
		t.Fatalf("one-mission board wrongly produced mission_count errors: %+v", one)
	}
}

func TestValidateBoardFindingsAreDeterministicallyOrdered(t *testing.T) {
	raw := cleanInspectBoardFixture() + `
[[formation]]
id = "fmn_zzz_bad"
type = "wormhole"
title = "Z bad"

[[formation]]
id = "fmn_aaa_bad"
type = "blackhole"
title = "A bad"
`
	first := ValidateBoard(mustParseBoardFixture(t, raw))
	second := ValidateBoard(mustParseBoardFixture(t, raw))
	if len(first.Errors) != len(second.Errors) {
		t.Fatalf("nondeterministic error count: %d vs %d", len(first.Errors), len(second.Errors))
	}
	for i := range first.Errors {
		if first.Errors[i] != second.Errors[i] {
			t.Fatalf("nondeterministic error ordering at %d: %+v vs %+v", i, first.Errors[i], second.Errors[i])
		}
	}
	// Verify the type errors are sorted by node id (aaa before zzz).
	typeErrors := findFindings(first.Errors, FindingInvalidFormationType)
	if len(typeErrors) != 2 {
		t.Fatalf("invalid-formation-type errors = %d, want 2", len(typeErrors))
	}
	if typeErrors[0].NodeID != "fmn_aaa_bad" || typeErrors[1].NodeID != "fmn_zzz_bad" {
		t.Fatalf("type errors not sorted by node id: %+v", typeErrors)
	}
}

func TestExportBoardCombinesBoardAndLayout(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("clean"), cleanInspectBoardFixture())
	writeFixture(t, store.LayoutPath("clean"), `schema = 1
boardId = "brd_clean"
boardRev = 3
updatedAt = "2026-06-03T16:00:00Z"

[[node]]
id = "fmn_work"
x = 100
y = 200
`)

	export, err := store.ExportBoard("clean")
	if err != nil {
		t.Fatalf("export board: %v", err)
	}
	if export.Board == nil || export.Board.ID != "brd_clean" {
		t.Fatalf("export board missing or wrong id: %+v", export.Board)
	}
	if export.Layout == nil || export.Layout.BoardID != "brd_clean" {
		t.Fatalf("export layout missing or wrong board id: %+v", export.Layout)
	}
	if len(export.Layout.Nodes) != 1 || export.Layout.Nodes[0].X != 100 {
		t.Fatalf("export layout did not round-trip node positions: %+v", export.Layout)
	}
}

func TestExportBoardToleratesMissingLayout(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	// A freshly created board has no layout file. Export must still succeed,
	// with a nil layout, because a missing layout is a normal degraded state.
	writeFixture(t, store.BoardPath("clean"), cleanInspectBoardFixture())

	export, err := store.ExportBoard("clean")
	if err != nil {
		t.Fatalf("export board without layout: %v", err)
	}
	if export.Board == nil {
		t.Fatalf("export board missing")
	}
	if export.Layout != nil {
		t.Fatalf("export layout should be nil when no layout file exists: %+v", export.Layout)
	}
}

func TestExportBoardErrorsOnMissingBoard(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	if _, err := store.ExportBoard("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("export missing board err = %v, want ErrNotFound", err)
	}
}

func TestScanAgentReferencesFindsAssignedAgentAcrossBoards(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	// Board alpha assigns "scout" to slot_work.
	writeFixture(t, store.BoardPath("alpha"), cleanInspectBoardFixture())
	// Board beta assigns "scout" to a different slot and "other" to another.
	writeFixture(t, store.BoardPath("beta"), `schema = 1
id = "brd_beta"
slug = "beta"
title = "Beta"
rev = 1
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_beta"
type = "peer"
title = "Beta work"

[[formation.slot]]
id = "slot_beta_a"
label = "A"
agentId = "scout"
controller = true

[[formation.slot]]
id = "slot_beta_b"
label = "B"
agentId = "other"
controller = false
`)

	refs, err := store.ScanAgentReferences("scout")
	if err != nil {
		t.Fatalf("scan agent references: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("references for scout = %d, want 2:\n%+v", len(refs), refs)
	}
	// Deterministic ordering: alpha before beta.
	if refs[0].BoardSlug != "alpha" || refs[0].FormationID != "fmn_work" || refs[0].SlotID != "slot_work" {
		t.Fatalf("first reference wrong: %+v", refs[0])
	}
	if refs[1].BoardSlug != "beta" || refs[1].FormationID != "fmn_beta" || refs[1].SlotID != "slot_beta_a" {
		t.Fatalf("second reference wrong: %+v", refs[1])
	}
	if refs[0].BoardID != "brd_clean" || refs[1].BoardID != "brd_beta" {
		t.Fatalf("references must carry board ids: %+v", refs)
	}
}

func TestScanAgentReferencesReturnsEmptyForUnassignedAgent(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("alpha"), cleanInspectBoardFixture())

	refs, err := store.ScanAgentReferences("nobody")
	if err != nil {
		t.Fatalf("scan agent references: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("references for unassigned agent = %d, want 0:\n%+v", len(refs), refs)
	}
}

func TestScanAgentReferencesFailsLoudOnUnreadableBoard(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("alpha"), cleanInspectBoardFixture())
	// A board file that exists but cannot be parsed (unsupported schema). Scan
	// must NOT silently skip it: skipping would hide references and make retire
	// unsafe.
	writeFixture(t, store.BoardPath("broken"), `schema = 999
id = "brd_broken"
slug = "broken"
title = "Broken"
rev = 1
`)

	if _, err := store.ScanAgentReferences("scout"); err == nil {
		t.Fatalf("scan with unreadable board returned nil error, want failure naming the board")
	} else if !errorMentions(err, "broken") {
		t.Fatalf("scan error must name the unreadable board, got: %v", err)
	}
}
