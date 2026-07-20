package formations

import (
	"errors"
	"fmt"
)

const ToolExecutionUnavailableCode = "tool_execution_unavailable"

var ErrToolExecutionUnavailable = errors.New(ToolExecutionUnavailableCode)

func preflightMissionDefinition(board *BoardDocument, missionID string) error {
	selected := reachableNodeIDs(board, missionID)
	if err := rejectLegacyInlineVerificationForNodes(board, selected); err != nil {
		return err
	}
	if err := rejectLegacyScriptGateForMission(board, missionID); err != nil {
		return err
	}
	return preflightSelectedTools(board, selected)
}

func preflightIsolatedFormationDefinition(board *BoardDocument, formationID string) error {
	selected := map[string]bool{formationID: true}
	if err := rejectLegacyInlineVerificationForNodes(board, selected); err != nil {
		return err
	}
	return preflightSelectedTools(board, selected)
}

func preflightSelectedTools(board *BoardDocument, selected map[string]bool) error {
	if board == nil {
		return nil
	}
	hasTool := false
	for _, tool := range board.Tools {
		if !selected[tool.ID] {
			continue
		}
		hasTool = true
		if board.Schema != CurrentBoardSchema {
			return fmt.Errorf("%s: Tool %q requires board schema %d", FindingInvalidTool, tool.ID, CurrentBoardSchema)
		}
		descriptor, ok := LookupToolProfileDescriptor(tool.ProfileID, tool.ProfileVersion)
		if !ok {
			return fmt.Errorf("%s: Tool %q uses unknown profile tuple %q@%q", FindingInvalidTool, tool.ID, tool.ProfileID, tool.ProfileVersion)
		}
		if err := validateToolNodeAgainstDescriptor(tool, descriptor); err != nil {
			return fmt.Errorf("%s: %w", FindingInvalidTool, err)
		}
	}
	if hasTool {
		return ErrToolExecutionUnavailable
	}
	return nil
}

func reachableNodeIDs(board *BoardDocument, rootID string) map[string]bool {
	selected := map[string]bool{rootID: true}
	if board == nil {
		return selected
	}
	queue := []string{rootID}
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		for _, connection := range outgoingConnections(board.Connections, nodeID) {
			nextID, _ := endpointParts(connection.To)
			if nextID == "" || selected[nextID] {
				continue
			}
			selected[nextID] = true
			queue = append(queue, nextID)
		}
	}
	return selected
}
