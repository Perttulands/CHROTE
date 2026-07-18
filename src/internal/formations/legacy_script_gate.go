package formations

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const LegacyScriptGateMigrationCode = "legacy_script_gate_requires_fenced_migration"

var ErrLegacyScriptGateRequiresFencedMigration = errors.New(LegacyScriptGateMigrationCode)

type LegacyScriptGateMigrationInspection struct {
	Schema          int      `json:"schema"`
	BoardID         string   `json:"boardId"`
	BoardRev        int      `json:"boardRev"`
	BoardETag       string   `json:"boardETag"`
	GateID          string   `json:"gateId"`
	SourceMode      string   `json:"sourceMode"`
	SourceFields    []string `json:"sourceFields"`
	Code            string   `json:"code"`
	TargetKind      string   `json:"targetKind"`
	Ready           bool     `json:"ready"`
	ApplySupported  bool     `json:"applySupported"`
	Requirements    []string `json:"requirements"`
	IncomingEdgeIDs []string `json:"incomingEdgeIds"`
	OutgoingEdgeIDs []string `json:"outgoingEdgeIds"`
}

func gateHasLegacyScriptCommand(gate GateNode) bool {
	return len(gate.legacyCommandFields) > 0 ||
		strings.TrimSpace(gate.Command) != "" ||
		len(gate.CommandArgv) > 0 ||
		strings.TrimSpace(gate.CommandCWD) != "" ||
		strings.TrimSpace(gate.CommandShell) != ""
}

func rejectLegacyScriptGateForMission(board *BoardDocument, missionID string) error {
	if board == nil {
		return nil
	}
	gates := make(map[string]GateNode, len(board.Gates))
	for _, gate := range board.Gates {
		gates[gate.ID] = gate
	}
	visited := map[string]bool{missionID: true}
	queue := []string{missionID}
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		if gate, ok := gates[nodeID]; ok && gateHasLegacyScriptCommand(gate) {
			return legacyScriptGateMigrationError(gate.ID)
		}
		for _, connection := range outgoingConnections(board.Connections, nodeID) {
			nextID, _ := endpointParts(connection.To)
			if nextID == "" || visited[nextID] {
				continue
			}
			visited[nextID] = true
			queue = append(queue, nextID)
		}
	}
	return nil
}

func rejectLegacyScriptGateWrite(fieldsPresent bool, command string, commandArgv []string, commandCWD, commandShell string) error {
	if !fieldsPresent && command == "" && commandArgv == nil && commandCWD == "" && commandShell == "" {
		return nil
	}
	return legacyScriptGateMigrationError("")
}

func populateLegacyScriptGateMigrationInspections(board *BoardDocument) {
	if board == nil {
		return
	}
	for gateIndex := range board.Gates {
		gate := &board.Gates[gateIndex]
		if !gateHasLegacyScriptCommand(*gate) {
			continue
		}
		fields := legacyScriptSourceFields(*gate)
		incoming, outgoing := legacyScriptGateEdgeIDs(board.Connections, gate.ID)
		gate.LegacyScriptMigration = &LegacyScriptGateMigrationInspection{
			Schema:         1,
			BoardID:        board.ID,
			BoardRev:       board.Rev,
			BoardETag:      board.ETag,
			GateID:         gate.ID,
			SourceMode:     legacyScriptSourceMode(*gate),
			SourceFields:   fields,
			Code:           LegacyScriptGateMigrationCode,
			TargetKind:     "tool_plus_pure_gate",
			Ready:          false,
			ApplySupported: false,
			Requirements: []string{
				"host_owned_tool_profile",
				"pure_gate_evaluator_profile",
				"explicit_parameter_mapping",
				"port_media_compatibility",
				"atomic_cas_rewire",
			},
			IncomingEdgeIDs: incoming,
			OutgoingEdgeIDs: outgoing,
		}
	}
}

func legacyScriptSourceFields(gate GateNode) []string {
	fields := make([]string, 0, 4)
	for _, field := range []string{"command", "commandArgv", "commandCwd", "commandShell"} {
		if gate.legacyCommandFields[field] > 0 {
			fields = append(fields, field)
		}
	}
	return fields
}

func legacyScriptSourceMode(gate GateNode) string {
	for _, count := range gate.legacyCommandFields {
		if count > 1 {
			return "conflict"
		}
	}
	modes := 0
	if gate.legacyCommandFields["command"] > 0 {
		modes++
	}
	if gate.legacyCommandFields["commandArgv"] > 0 {
		modes++
	}
	if gate.legacyCommandFields["commandShell"] > 0 {
		modes++
	}
	if modes > 1 {
		return "conflict"
	}
	if gate.legacyCommandFields["command"] > 0 {
		if strings.TrimSpace(gate.Command) == "" {
			return "empty_present"
		}
		return "legacy_string"
	}
	if gate.legacyCommandFields["commandArgv"] > 0 {
		if len(gate.CommandArgv) == 0 {
			return "empty_present"
		}
		return "argv"
	}
	if gate.legacyCommandFields["commandShell"] > 0 {
		if strings.TrimSpace(gate.CommandShell) == "" {
			return "empty_present"
		}
		return "shell"
	}
	if strings.TrimSpace(gate.CommandCWD) == "" {
		return "empty_present"
	}
	return "cwd_only"
}

func legacyScriptGateEdgeIDs(connections []BoardConnection, gateID string) ([]string, []string) {
	var incoming []string
	var outgoing []string
	for _, connection := range connections {
		fromNode, _ := endpointParts(connection.From)
		toNode, _ := endpointParts(connection.To)
		if toNode == gateID {
			incoming = append(incoming, connection.ID)
		}
		if fromNode == gateID {
			outgoing = append(outgoing, connection.ID)
		}
	}
	sort.Strings(incoming)
	sort.Strings(outgoing)
	return incoming, outgoing
}

func rejectLegacyScriptGateForRun(board *BoardDocument, started RunEvent, event *RunEvent) error {
	if board == nil {
		return nil
	}
	if event != nil {
		for _, gate := range board.Gates {
			if gate.ID != event.GateID && gate.ID != event.NodeID {
				continue
			}
			if gateHasLegacyScriptCommand(gate) {
				return legacyScriptGateMigrationError(gate.ID)
			}
		}
	}
	switch mode := stringFromEventData(started, "mode"); mode {
	case "formation":
		formationID := stringFromEventData(started, "formationId")
		if _, ok := findFormation(board.Formations, formationID); !ok || started.MissionID != "single_"+formationID {
			return ErrRunLedgerInvalid
		}
		return nil
	case "":
		// Mission runs predate the explicit root-mode field.
	default:
		return ErrRunLedgerInvalid
	}
	if _, ok := findMission(board, started.MissionID); !ok {
		return ErrRunLedgerInvalid
	}
	return rejectLegacyScriptGateForMission(board, started.MissionID)
}

func legacyScriptGateMigrationError(gateID string) error {
	if gateID == "" {
		return fmt.Errorf(
			"%w: legacy Gate command fields are inspection-only; use a host-approved Tool profile and wire its sealed result into a pure Gate",
			ErrLegacyScriptGateRequiresFencedMigration,
		)
	}
	return fmt.Errorf(
		"%w: gate %q has legacy command fields that cannot enter execution; migrate them to a host-approved Tool profile wired into a pure Gate",
		ErrLegacyScriptGateRequiresFencedMigration,
		gateID,
	)
}
