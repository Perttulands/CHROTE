package formations

import (
	"fmt"
	"sort"
	"strings"
)

// Finding codes reported by ValidateBoard. They are stable strings so CLI and
// API consumers can branch on them.
const (
	FindingDanglingConnection   = "dangling_connection"
	FindingGateNotRoutable      = "gate_not_routable"
	FindingInvalidFormationType = "invalid_formation_type"
	FindingMissionCount         = "mission_count"
	FindingMissionNotRunnable   = "mission_not_runnable"
)

// BoardFinding is a single structural problem located on the board. NodeID names
// the offending node, or the edge id for connection problems.
type BoardFinding struct {
	Code    string `json:"code"`
	NodeID  string `json:"nodeId"`
	Message string `json:"message"`
}

// BoardValidationReport separates blocking errors from advisory warnings.
type BoardValidationReport struct {
	Errors   []BoardFinding `json:"errors"`
	Warnings []BoardFinding `json:"warnings"`
}

// ValidateBoard performs a read-only structural integrity check. It reuses the
// same endpoint, formation-type, judge-chain, and graph helpers used by the
// authoring/run paths so findings match runtime behavior.
func ValidateBoard(board *BoardDocument) BoardValidationReport {
	var report BoardValidationReport
	if board == nil {
		return report
	}

	raw := []byte(board.TOML)
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

	for _, gate := range board.Gates {
		hasJudgeChain := len(judgeChainForGate(board, gate.ID)) > 0
		hasScriptCommand := len(gate.CommandArgv) > 0 || strings.TrimSpace(gate.CommandShell) != ""
		nonHumanKinds := withoutGateKind(gate.Kinds, "human")
		isHumanOnlyGate := len(nonHumanKinds) == 0 && hasGateKind(gate.Kinds, "human")
		hasScorecardEvaluator := scorecardGateKinds(nonHumanKinds)
		if hasScorecardEvaluator {
			if _, err := validateScorecardPolicy(GateEvaluation{
				Kinds:             nonHumanKinds,
				ScoreThreshold:    gate.ScoreThreshold,
				RequireNoMustFix:  gate.RequireNoMustFix,
				RequiredReviewers: gate.RequiredReviewers,
				ReviewerWeights:   gate.ReviewerWeights,
			}); err != nil {
				report.Errors = append(report.Errors, BoardFinding{
					Code:    FindingGateNotRoutable,
					NodeID:  gate.ID,
					Message: fmt.Sprintf("gate %q has an invalid scorecard policy: %v", gate.ID, err),
				})
				continue
			}
		}
		if !hasJudgeChain && !hasScriptCommand && !isHumanOnlyGate && !hasScorecardEvaluator {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingGateNotRoutable,
				NodeID:  gate.ID,
				Message: fmt.Sprintf("gate %q has no judge chain, command argv/shell, built-in scorecard policy, or human-only kind, so the board cannot route it", gate.ID),
			})
		}
	}

	for _, formation := range board.Formations {
		if err := validateFormationType(formation.Type); err != nil {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingInvalidFormationType,
				NodeID:  formation.ID,
				Message: fmt.Sprintf("formation %q has invalid type %q; valid types are %q, %q, %q, %q", formation.ID, formation.Type, FormationTypeSolo, FormationTypePeer, FormationTypeFlow, FormationTypeOrchestrated),
			})
		}
	}

	if len(board.Missions) == 0 {
		report.Errors = append(report.Errors, BoardFinding{
			Code:    FindingMissionCount,
			Message: "a board must have at least one mission to run",
		})
	}
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
