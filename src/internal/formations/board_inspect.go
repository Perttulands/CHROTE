package formations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Finding codes reported by ValidateBoard. They are stable strings so CLI output
// and tooling can branch on them.
const (
	FindingDanglingConnection   = "dangling_connection"
	FindingGateNotRoutable      = "gate_not_routable"
	FindingInvalidFormationType = "invalid_formation_type"
	FindingMissionCount         = "mission_count"
	FindingMissionNotRunnable   = "mission_not_runnable"
)

// BoardFinding is a single structural problem located on the board. NodeID names
// the offending node (or edge id, for connection problems) so messages are
// actionable.
type BoardFinding struct {
	Code    string `json:"code"`
	NodeID  string `json:"nodeId"`
	Message string `json:"message"`
}

// BoardValidationReport separates blocking Errors from advisory Warnings. Both
// slices are sorted deterministically so CLI output and tests are stable.
type BoardValidationReport struct {
	Errors   []BoardFinding `json:"errors"`
	Warnings []BoardFinding `json:"warnings"`
}

// BoardExport is the canonical, portable representation of a board: its
// definition plus its layout sidecar. Layout is nil when no layout file exists
// (a freshly created board has no layout until nodes are placed).
type BoardExport struct {
	Board  *BoardDocument  `json:"board"`
	Layout *LayoutDocument `json:"layout,omitempty"`
}

// AgentReference identifies one slot currently assigned to an agent.
type AgentReference struct {
	BoardSlug   string `json:"boardSlug"`
	BoardID     string `json:"boardId"`
	FormationID string `json:"formationId"`
	SlotID      string `json:"slotId"`
}

// ValidateBoard performs a read-only structural integrity check and returns the
// problems found. It never mutates or writes. It reuses the same predicates the
// authoring and run paths rely on (endpointAllowsDirection, validateFormationType,
// judgeChainForGate, outgoingConnections) so a board that validates clean here
// matches what those paths accept.
func ValidateBoard(board *BoardDocument) BoardValidationReport {
	var report BoardValidationReport
	if board == nil {
		return report
	}

	raw := []byte(board.TOML)

	// Dangling connections: every endpoint must reference an existing node and an
	// existing port/endpoint allowing the required direction (outputs feed From,
	// inputs feed To), matching how WireFormationPorts validates wires.
	for _, connection := range board.Connections {
		if _, ok := endpointAllowsDirection(raw, connection.From, FormationPortOutput); !ok {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingDanglingConnection,
				NodeID:  connection.ID,
				Message: fmt.Sprintf("connection %q has a broken 'from' endpoint %q: it does not reference an existing node output", connection.ID, connection.From),
			})
		}
		if _, ok := endpointAllowsDirection(raw, connection.To, FormationPortInput); !ok {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingDanglingConnection,
				NodeID:  connection.ID,
				Message: fmt.Sprintf("connection %q has a broken 'to' endpoint %q: it does not reference an existing node input", connection.ID, connection.To),
			})
		}
	}

	// Gate routability: a gate routes via a judge chain, a script config, or a
	// human verdict (a "human"-kind gate stops for an operator decision via
	// RecordHumanGateVerdict). A gate with none of these can only route through a
	// runtime-injected evaluator, which is not part of the board, so the board
	// cannot run it on its own.
	for _, gate := range board.Gates {
		hasJudgeChain := len(judgeChainForGate(board, gate.ID)) > 0
		hasScript := gate.Script != nil
		isHumanGate := hasGateKind(gate.Kinds, "human")
		if !hasJudgeChain && !hasScript && !isHumanGate {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingGateNotRoutable,
				NodeID:  gate.ID,
				Message: fmt.Sprintf("gate %q has no judge chain, script-gate config, or human kind, so the board cannot route it; attach a judge chain, set a script gate, or make it a human gate", gate.ID),
			})
		}
	}

	// Formation type validity: reuse the single source of truth.
	for _, formation := range board.Formations {
		if err := validateFormationType(formation.Type); err != nil {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingInvalidFormationType,
				NodeID:  formation.ID,
				Message: fmt.Sprintf("formation %q has invalid type %q; valid types are %q, %q, %q, %q", formation.ID, formation.Type, FormationTypeSolo, FormationTypePeer, FormationTypeFlow, FormationTypeOrchestrated),
			})
		}
	}

	// Mission count: a Mission Board's identity is its single mission. Zero
	// missions means there is nothing to run; more than one would force the runner
	// to pick, which a Mission Board deliberately avoids. Both are hard errors.
	// This is a board-level problem, so NodeID is empty.
	switch len(board.Missions) {
	case 1:
		// Exactly one mission is the only valid shape; no finding.
	case 0:
		report.Errors = append(report.Errors, BoardFinding{
			Code:    FindingMissionCount,
			Message: "a Mission Board must have exactly one mission (found none)",
		})
	default:
		report.Errors = append(report.Errors, BoardFinding{
			Code:    FindingMissionCount,
			Message: fmt.Sprintf("a Mission Board must have exactly one mission (found %d)", len(board.Missions)),
		})
	}

	// Mission runnability (warning): RunMission refuses a mission with zero
	// outgoing connections because it has no first step to start.
	for _, mission := range board.Missions {
		if len(outgoingConnections(board.Connections, mission.ID)) == 0 {
			report.Warnings = append(report.Warnings, BoardFinding{
				Code:    FindingMissionNotRunnable,
				NodeID:  mission.ID,
				Message: fmt.Sprintf("mission %q has no outgoing connection, so it cannot start a run; wire it to a first step", mission.ID),
			})
		}
	}

	sortFindings(report.Errors)
	sortFindings(report.Warnings)
	return report
}

// sortFindings orders findings deterministically by code, then node id, then
// message, so CLI output and tests are stable regardless of board iteration order.
func sortFindings(findings []BoardFinding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		if findings[i].NodeID != findings[j].NodeID {
			return findings[i].NodeID < findings[j].NodeID
		}
		return findings[i].Message < findings[j].Message
	})
}

// ExportBoard produces the canonical, portable export of a board: its definition
// plus its layout sidecar. It fails loud with ErrNotFound if the board does not
// exist. A missing layout is a normal degraded state (a board has no layout until
// nodes are placed), so Export returns a nil Layout in that case rather than
// failing.
func (s *Store) ExportBoard(slug string) (*BoardExport, error) {
	board, err := s.ReadBoard(slug)
	if err != nil {
		return nil, err
	}
	layout, err := s.ReadLayout(slug)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &BoardExport{Board: board, Layout: nil}, nil
		}
		return nil, fmt.Errorf("export board %q: read layout: %w", slug, err)
	}
	return &BoardExport{Board: board, Layout: layout}, nil
}

// ScanAgentReferences finds every slot across all boards currently assigned to
// agentID. It is the safety check for `agent retire`: it must surface every
// reference. It scans all boards and fails loud naming any board it cannot read,
// because silently skipping a board would hide references and make retire unsafe.
// Results are ordered deterministically (by board slug, then formation, then slot).
func (s *Store) ScanAgentReferences(agentID string) ([]AgentReference, error) {
	slugs, err := s.listBoardSlugs()
	if err != nil {
		return nil, fmt.Errorf("scan agent references: list boards: %w", err)
	}
	var refs []AgentReference
	for _, slug := range slugs {
		board, err := s.ReadBoard(slug)
		if err != nil {
			return nil, fmt.Errorf("scan agent references: cannot read board %q: %w", slug, err)
		}
		for _, formation := range board.Formations {
			for _, slot := range formation.Slots {
				if slot.AgentID == agentID {
					refs = append(refs, AgentReference{
						BoardSlug:   slug,
						BoardID:     board.ID,
						FormationID: formation.ID,
						SlotID:      slot.ID,
					})
				}
			}
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].BoardSlug != refs[j].BoardSlug {
			return refs[i].BoardSlug < refs[j].BoardSlug
		}
		if refs[i].FormationID != refs[j].FormationID {
			return refs[i].FormationID < refs[j].FormationID
		}
		return refs[i].SlotID < refs[j].SlotID
	})
	return refs, nil
}

// listBoardSlugs enumerates board slugs from the boards directory without parsing
// each board. Unlike ListBoards it does not eagerly read every board, so a board
// that fails to parse surfaces at the per-board read site where the slug can be
// named, rather than being silently swallowed or reported without context.
func (s *Store) listBoardSlugs() ([]string, error) {
	dir := filepath.Join(s.Workspace, ".formations", "boards")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	slugs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".formation.toml") {
			continue
		}
		slugs = append(slugs, strings.TrimSuffix(entry.Name(), ".formation.toml"))
	}
	sort.Strings(slugs)
	return slugs, nil
}
