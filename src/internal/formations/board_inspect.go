package formations

import (
	"fmt"
	"sort"
)

// Finding codes reported by ValidateBoard. They are stable strings so CLI and
// API consumers can branch on them.
const (
	FindingDanglingConnection                        = "dangling_connection"
	FindingGateNotRoutable                           = "gate_not_routable"
	FindingInvalidFormationType                      = "invalid_formation_type"
	FindingLegacyScriptGate                          = LegacyScriptGateMigrationCode
	FindingLegacyInlineVerificationRequiresMigration = LegacyInlineVerificationMigrationCode
	FindingMissionCount                              = "mission_count"
	FindingMissionNotRunnable                        = "mission_not_runnable"
	FindingInvalidTool                               = "invalid_tool"
	FindingDuplicateNodeID                           = "duplicate_node_id"
	FindingDuplicateInputProducer                    = "duplicate_input_producer"
	FindingIncompatibleMedia                         = "incompatible_media"
	FindingIncompatiblePayloadKind                   = "incompatible_payload_kind"
	FindingInvalidJudgeRelationship                  = "invalid_judge_relationship"
)

// BoardFinding is a single structural problem located on the board. NodeID names
// the offending node, or the edge id for connection problems.
type BoardFinding struct {
	Code    string                               `json:"code"`
	NodeID  string                               `json:"nodeId"`
	Message string                               `json:"message"`
	Details *LegacyScriptGateMigrationInspection `json:"details,omitempty"`
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
		if finding, incompatible := toolConnectionCompatibilityFinding(board, connection); incompatible {
			report.Errors = append(report.Errors, finding)
		}
	}
	inputProducers := make(map[string]string, len(board.Connections))
	for _, connection := range board.Connections {
		if first, exists := inputProducers[connection.To]; exists {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingDuplicateInputProducer,
				NodeID:  connection.ID,
				Message: fmt.Sprintf("connection %q is a second producer for input %q; it is already produced by connection %q", connection.ID, connection.To, first),
			})
			continue
		}
		inputProducers[connection.To] = connection.ID
	}

	for _, gate := range board.Gates {
		hasJudgeChain := len(judgeChainForGate(board, gate.ID)) > 0
		hasScriptCommand := gateHasLegacyScriptCommand(gate)
		nonHumanKinds := withoutGateKind(gate.Kinds, "human")
		isHumanOnlyGate := len(nonHumanKinds) == 0 && hasGateKind(gate.Kinds, "human")
		if hasScriptCommand {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingLegacyScriptGate,
				NodeID:  gate.ID,
				Message: legacyScriptGateMigrationError(gate.ID).Error(),
				Details: gate.LegacyScriptMigration,
			})
		}
		if !hasJudgeChain && !isHumanOnlyGate {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingGateNotRoutable,
				NodeID:  gate.ID,
				Message: fmt.Sprintf("gate %q has no judge chain or human-only kind, so the current board cannot route it; attach a judge chain or make it a human-only gate", gate.ID),
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
		if formation.Verification != nil {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingLegacyInlineVerificationRequiresMigration,
				NodeID:  formation.ID,
				Message: fmt.Sprintf("formation %q uses retired inline verification; create and wire an explicit Gate, then remove the legacy verification", formation.ID),
			})
		}
	}

	seenNodeIDs := make(map[string]string, len(board.Missions)+len(board.Formations)+len(board.Gates)+len(board.Tools))
	for _, mission := range board.Missions {
		seenNodeIDs[mission.ID] = "Mission"
	}
	for _, formation := range board.Formations {
		seenNodeIDs[formation.ID] = "Formation"
	}
	for _, gate := range board.Gates {
		seenNodeIDs[gate.ID] = "Gate"
	}
	for _, tool := range board.Tools {
		if firstKind, exists := seenNodeIDs[tool.ID]; tool.ID != "" && exists {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingDuplicateNodeID,
				NodeID:  tool.ID,
				Message: fmt.Sprintf("Tool node id %q duplicates an existing %s node id", tool.ID, firstKind),
			})
		} else if tool.ID != "" {
			seenNodeIDs[tool.ID] = "Tool"
		}
		if board.Schema != CurrentBoardSchema {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingInvalidTool,
				NodeID:  tool.ID,
				Message: fmt.Sprintf("Tool %q requires board schema %d", tool.ID, CurrentBoardSchema),
			})
			continue
		}
		descriptor, ok := LookupToolProfileDescriptor(tool.ProfileID, tool.ProfileVersion)
		if !ok {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingInvalidTool,
				NodeID:  tool.ID,
				Message: fmt.Sprintf("Tool %q uses unknown profile tuple %q@%q", tool.ID, tool.ProfileID, tool.ProfileVersion),
			})
			continue
		}
		if err := validateToolNodeAgainstDescriptor(tool, descriptor); err != nil {
			report.Errors = append(report.Errors, BoardFinding{
				Code:    FindingInvalidTool,
				NodeID:  tool.ID,
				Message: err.Error(),
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
