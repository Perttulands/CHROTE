package formations

import (
	"fmt"
	"strings"
)

const (
	legacyFailRouteMigrationCode    = "legacy_fail_route_requires_migration"
	legacyJudgeChannelMigrationCode = "legacy_judge_channel_requires_migration"
)

type toolSchemaMigrationPort struct {
	end       int
	portID    string
	direction string
	fields    map[string][]string
}

type toolSchemaMigrationConnection struct {
	end    int
	id     string
	from   string
	to     string
	fields map[string][]string
}

type toolSchemaMigrationGate struct {
	id           string
	kindLiterals []string
}

type toolSchemaMigrationScan struct {
	ports            []toolSchemaMigrationPort
	connections      []toolSchemaMigrationConnection
	gates            []toolSchemaMigrationGate
	formationIDs     map[string]bool
	gateIDs          map[string]bool
	inputOwners      map[string]string
	outputOwners     map[string]string
	ambiguousInputs  map[string]bool
	ambiguousOutputs map[string]bool
}

type toolSchemaMigrationFieldDefault struct {
	key      string
	rendered string
	want     string
}

func migrateBoardToToolSchema(raw []byte) ([]byte, error) {
	lines := splitLines(raw)
	schema, schemaLine, err := toolSchemaMigrationReadSchema(lines)
	if err != nil {
		return nil, err
	}

	board, err := parseBoard(raw)
	if err != nil {
		return nil, err
	}
	if schema == NewBoardSchema && len(board.Tools) != 0 {
		return nil, fmt.Errorf("board schema %d cannot contain Tool definitions", schema)
	}
	for _, gate := range board.Gates {
		if gateHasLegacyScriptCommand(gate) {
			return nil, legacyScriptGateMigrationError(gate.ID)
		}
	}
	if err := rejectLegacyInlineVerification(board); err != nil {
		return nil, err
	}

	scan, err := toolSchemaMigrationScanBoard(lines)
	if err != nil {
		return nil, err
	}
	if err := toolSchemaMigrationRejectLegacyFailRoutes(scan); err != nil {
		return nil, err
	}
	judgeConnections, err := toolSchemaMigrationJudgeConnections(scan)
	if err != nil {
		return nil, err
	}

	insertions := make(map[int][]tomlLine)
	for _, port := range scan.ports {
		if err := toolSchemaMigrationValidatePort(lines, schema, port, insertions); err != nil {
			return nil, err
		}
	}
	for connectionIndex, connection := range scan.connections {
		channel := "workflow"
		if judgeConnections[connectionIndex] {
			channel = "judge"
		}
		missing, err := toolSchemaMigrationValidateOwnedField(connection.fields, "channel", channel)
		if err != nil {
			return nil, fmt.Errorf("connection %q: %w", connection.id, err)
		}
		if missing {
			if schema == CurrentBoardSchema {
				return nil, fmt.Errorf("connection %q is missing migration-owned field %q", connection.id, "channel")
			}
			toolSchemaMigrationAddInsertion(lines, insertions, connection.end, "channel = "+renderString(channel))
		}
	}

	if schema == CurrentBoardSchema {
		return append([]byte(nil), raw...), nil
	}

	next := append([]tomlLine(nil), lines...)
	next[schemaLine].body = replaceScalarValue(next[schemaLine].body, renderInt(CurrentBoardSchema))
	return renderTOMLLines(toolSchemaMigrationApplyInsertions(next, insertions)), nil
}

func toolSchemaMigrationReadSchema(lines []tomlLine) (int, int, error) {
	schema := int64(0)
	schemaLine := -1
	count := 0
	for index, line := range lines {
		if _, ok := tomlLineSectionName(line); ok || isTOMLHeader(line) {
			break
		}
		if line.valueContinuation {
			continue
		}
		key, _, ok := tomlKeyValue(line.body)
		if !ok || key != "schema" {
			continue
		}
		count++
		parsedKey, literal, present, err := parseToolAssignment(line.body)
		if err != nil || !present || parsedKey != "schema" {
			return 0, -1, fmt.Errorf("invalid formations schema")
		}
		if strings.HasPrefix(literal, "0x") || strings.HasPrefix(literal, "0o") || strings.HasPrefix(literal, "0b") {
			return 0, -1, fmt.Errorf("invalid formations schema")
		}
		schema, err = parseToolInteger(literal)
		if err != nil {
			return 0, -1, fmt.Errorf("invalid formations schema")
		}
		schemaLine = index
	}
	if count != 1 {
		return 0, -1, fmt.Errorf("formations schema fields = %d, want exactly one", count)
	}
	if schema > int64(CurrentBoardSchema) {
		return 0, -1, fmt.Errorf("%w: schema %d", ErrUnsupportedSchema, schema)
	}
	if schema != int64(NewBoardSchema) && schema != int64(CurrentBoardSchema) {
		return 0, -1, fmt.Errorf("unsupported formations migration source schema %d", schema)
	}
	return int(schema), schemaLine, nil
}

func toolSchemaMigrationScanBoard(lines []tomlLine) (*toolSchemaMigrationScan, error) {
	scan := &toolSchemaMigrationScan{
		formationIDs:     make(map[string]bool),
		gateIDs:          make(map[string]bool),
		inputOwners:      make(map[string]string),
		outputOwners:     make(map[string]string),
		ambiguousInputs:  make(map[string]bool),
		ambiguousOutputs: make(map[string]bool),
	}
	ownedPortFields := map[string]bool{
		"direction":          true,
		"kind":               true,
		"acceptedMediaTypes": true,
		"required":           true,
		"role":               true,
	}
	ownedConnectionFields := map[string]bool{"channel": true}
	if err := toolSchemaMigrationRejectOwnedDescendantHeaders(lines, ownedPortFields, ownedConnectionFields); err != nil {
		return nil, err
	}

	for index := 0; index < len(lines); index++ {
		section, ok := tomlLineSectionName(lines[index])
		if !ok || section != "formation" || !toolSchemaMigrationArraySection(lines[index]) {
			continue
		}
		formationEnd := formationBlockEnd(lines, index)
		formationID := formationHeaderScalar(lines, index, formationEnd, "id")
		if formationID != "" {
			scan.formationIDs[formationID] = true
		}
		for portIndex := index + 1; portIndex < formationEnd; portIndex++ {
			portSection, ok := tomlLineSectionName(lines[portIndex])
			if !ok || !toolSchemaMigrationArraySection(lines[portIndex]) ||
				(portSection != "formation.input" && portSection != "formation.output") {
				continue
			}
			portEnd := tomlBlockEnd(lines, portIndex)
			if portEnd > formationEnd {
				portEnd = formationEnd
			}
			fields, err := toolSchemaMigrationCollectFields(lines, portIndex+1, portEnd, ownedPortFields)
			if err != nil {
				return nil, err
			}
			direction := FormationPortInput
			if portSection == "formation.output" {
				direction = FormationPortOutput
			}
			portID := scalarInBlock(lines, portIndex+1, portEnd, "id")
			scan.ports = append(scan.ports, toolSchemaMigrationPort{
				end:       portEnd,
				portID:    portID,
				direction: direction,
				fields:    fields,
			})
			if formationID != "" && portID != "" {
				endpoint := formationID + ":" + portID
				if direction == FormationPortInput {
					toolSchemaMigrationRecordOwner(scan.inputOwners, scan.ambiguousInputs, endpoint, formationID)
				} else {
					toolSchemaMigrationRecordOwner(scan.outputOwners, scan.ambiguousOutputs, endpoint, formationID)
				}
			}
			portIndex = portEnd - 1
		}
		index = formationEnd - 1
	}

	for index := 0; index < len(lines); index++ {
		section, ok := tomlLineSectionName(lines[index])
		if !ok || section != "gate" || !toolSchemaMigrationArraySection(lines[index]) {
			continue
		}
		end := tomlBlockEnd(lines, index)
		fields, err := toolSchemaMigrationCollectFields(lines, index+1, end, map[string]bool{"kinds": true})
		if err != nil {
			return nil, err
		}
		gateID := scalarInBlock(lines, index+1, end, "id")
		scan.gates = append(scan.gates, toolSchemaMigrationGate{id: gateID, kindLiterals: fields["kinds"]})
		if gateID != "" {
			scan.gateIDs[gateID] = true
		}
		index = end - 1
	}

	for index := 0; index < len(lines); index++ {
		section, ok := tomlLineSectionName(lines[index])
		if !ok || section != "connection" || !toolSchemaMigrationArraySection(lines[index]) {
			continue
		}
		end := tomlBlockEnd(lines, index)
		fields, err := toolSchemaMigrationCollectFields(lines, index+1, end, ownedConnectionFields)
		if err != nil {
			return nil, err
		}
		scan.connections = append(scan.connections, toolSchemaMigrationConnection{
			end:    end,
			id:     scalarInBlock(lines, index+1, end, "id"),
			from:   scalarInBlock(lines, index+1, end, "from"),
			to:     scalarInBlock(lines, index+1, end, "to"),
			fields: fields,
		})
		index = end - 1
	}
	return scan, nil
}

func toolSchemaMigrationRejectOwnedDescendantHeaders(lines []tomlLine, ownedPortFields, ownedConnectionFields map[string]bool) error {
	for _, line := range lines {
		section, ok := tomlLineSectionName(line)
		if !ok {
			continue
		}
		path, ok := parseTOMLKeyPath(section)
		if !ok {
			continue
		}
		if len(path) > 2 && path[0] == "formation" &&
			(path[1] == "input" || path[1] == "output") && ownedPortFields[path[2]] {
			return fmt.Errorf("nested migration-owned field %q", path[2])
		}
		if len(path) > 1 && path[0] == "connection" && ownedConnectionFields[path[1]] {
			return fmt.Errorf("nested migration-owned field %q", path[1])
		}
	}
	return nil
}

func toolSchemaMigrationArraySection(line tomlLine) bool {
	return strings.HasPrefix(strings.TrimSpace(line.body), "[[")
}

func toolSchemaMigrationCollectFields(lines []tomlLine, start, end int, owned map[string]bool) (map[string][]string, error) {
	fields := make(map[string][]string)
	for index := start; index < end && index < len(lines); index++ {
		if lines[index].valueContinuation {
			continue
		}
		assignment := tomlAssignmentIndex(lines[index].body)
		if assignment < 0 {
			continue
		}
		path, ok := parseTOMLKeyPath(lines[index].body[:assignment])
		if !ok || len(path) == 0 || !owned[path[0]] {
			continue
		}
		if len(path) != 1 {
			return nil, fmt.Errorf("nested migration-owned field %q", path[0])
		}
		key := path[0]
		parsedKey, literal, present, err := parseToolAssignment(lines[index].body)
		if err != nil || !present || parsedKey != key {
			return nil, fmt.Errorf("invalid migration-owned field %q", key)
		}
		fields[key] = append(fields[key], literal)
	}
	return fields, nil
}

func toolSchemaMigrationRecordOwner(owners map[string]string, ambiguous map[string]bool, endpoint, formationID string) {
	if _, exists := owners[endpoint]; exists {
		ambiguous[endpoint] = true
		return
	}
	owners[endpoint] = formationID
}

func toolSchemaMigrationRejectLegacyFailRoutes(scan *toolSchemaMigrationScan) error {
	for _, connection := range scan.connections {
		fromNode, fromPort, ok := splitEndpoint(connection.from)
		if !ok || fromPort != "fail" || !scan.gateIDs[fromNode] {
			continue
		}
		if _, isFormationInput := scan.inputOwners[connection.to]; !isFormationInput {
			continue
		}
		return fmt.Errorf(
			"%s: connection %q routes Gate fail into legacy Formation work input %q",
			legacyFailRouteMigrationCode,
			connection.id,
			connection.to,
		)
	}
	return nil
}

func toolSchemaMigrationJudgeConnections(scan *toolSchemaMigrationScan) (map[int]bool, error) {
	judgeConnections := make(map[int]bool)
	incomingByFormation := make(map[string][]int)
	outgoingByFormation := make(map[string][]int)
	for index, connection := range scan.connections {
		fromNode, _, _ := splitEndpoint(connection.from)
		toNode, _, _ := splitEndpoint(connection.to)
		if scan.formationIDs[fromNode] {
			outgoingByFormation[fromNode] = append(outgoingByFormation[fromNode], index)
		}
		if scan.formationIDs[toNode] {
			incomingByFormation[toNode] = append(incomingByFormation[toNode], index)
		}
	}

	for _, gate := range scan.gates {
		gateEndpoint := gate.id + ":judge"
		var sends []int
		var returns []int
		for index, connection := range scan.connections {
			if connection.from == gateEndpoint {
				sends = append(sends, index)
			}
			if connection.to == gateEndpoint {
				returns = append(returns, index)
			}
		}

		kinds, potentialFormation, kindsValid := toolSchemaMigrationGateKinds(gate.kindLiterals)
		hasFormationKind := containsToolString(kinds, "formation")
		hasJudgeEdges := len(sends) > 0 || len(returns) > 0
		if !kindsValid && (potentialFormation || hasJudgeEdges) {
			return nil, toolSchemaMigrationJudgeError(gate.id, "Gate kinds do not provide an exact formation discriminator")
		}
		if !hasFormationKind && !hasJudgeEdges {
			continue
		}
		if !hasFormationKind || len(sends) != 1 || len(returns) != 1 {
			return nil, toolSchemaMigrationJudgeError(gate.id, "formation kind requires exactly one judge send and one judge return")
		}

		sendIndex := sends[0]
		returnIndex := returns[0]
		send := scan.connections[sendIndex]
		judgeReturn := scan.connections[returnIndex]
		currentFormation, inputOK := scan.inputOwners[send.to]
		returnFormation, outputOK := scan.outputOwners[judgeReturn.from]
		if !inputOK || scan.ambiguousInputs[send.to] || !outputOK || scan.ambiguousOutputs[judgeReturn.from] {
			return nil, toolSchemaMigrationJudgeError(gate.id, "judge endpoints must bind unambiguous Formation input and output ports")
		}
		if judgeConnections[sendIndex] {
			return nil, toolSchemaMigrationJudgeError(gate.id, "judge send is shared by another chain")
		}
		judgeConnections[sendIndex] = true

		expectedIncoming := sendIndex
		visited := make(map[string]bool)
		for {
			if currentFormation == "" || visited[currentFormation] {
				return nil, toolSchemaMigrationJudgeError(gate.id, "judge chain is cyclic or leaves Formation nodes")
			}
			visited[currentFormation] = true
			incoming := incomingByFormation[currentFormation]
			outgoing := outgoingByFormation[currentFormation]
			if len(incoming) != 1 || incoming[0] != expectedIncoming || len(outgoing) != 1 {
				return nil, toolSchemaMigrationJudgeError(gate.id, "judge chain has a side entry, side exit, or disconnected hop")
			}

			nextIndex := outgoing[0]
			next := scan.connections[nextIndex]
			fromOwner, fromOK := scan.outputOwners[next.from]
			if !fromOK || scan.ambiguousOutputs[next.from] || fromOwner != currentFormation {
				return nil, toolSchemaMigrationJudgeError(gate.id, "judge chain must leave each Formation through an output port")
			}
			if nextIndex == returnIndex {
				if next.to != gateEndpoint || currentFormation != returnFormation {
					return nil, toolSchemaMigrationJudgeError(gate.id, "judge return does not close the selected linear chain")
				}
				if judgeConnections[nextIndex] {
					return nil, toolSchemaMigrationJudgeError(gate.id, "judge return is shared by another chain")
				}
				judgeConnections[nextIndex] = true
				break
			}

			nextFormation, nextOK := scan.inputOwners[next.to]
			if !nextOK || scan.ambiguousInputs[next.to] || judgeConnections[nextIndex] {
				return nil, toolSchemaMigrationJudgeError(gate.id, "judge chain contains an ambiguous non-Formation hop")
			}
			judgeConnections[nextIndex] = true
			expectedIncoming = nextIndex
			currentFormation = nextFormation
		}
	}

	for index, connection := range scan.connections {
		fromNode, fromPort, _ := splitEndpoint(connection.from)
		toNode, toPort, _ := splitEndpoint(connection.to)
		touchesJudge := fromPort == "judge" && scan.gateIDs[fromNode] || toPort == "judge" && scan.gateIDs[toNode]
		if touchesJudge && !judgeConnections[index] {
			return nil, toolSchemaMigrationJudgeError("", "Gate judge endpoint is outside one complete Formation chain")
		}
	}
	return judgeConnections, nil
}

func toolSchemaMigrationGateKinds(literals []string) ([]string, bool, bool) {
	potentialFormation := false
	for _, literal := range literals {
		if kinds, err := parseToolStringArray(literal); err == nil && containsToolString(kinds, "formation") {
			potentialFormation = true
			continue
		}
		if kind, err := parseToolString(literal); err == nil && kind == "formation" {
			potentialFormation = true
		}
	}
	if len(literals) != 1 {
		return nil, potentialFormation, false
	}
	kinds, err := parseToolStringArray(literals[0])
	if err != nil || len(kinds) == 0 {
		return nil, potentialFormation, false
	}
	seen := make(map[string]bool)
	for _, kind := range kinds {
		if seen[kind] || kind != "code" && kind != "formation" && kind != "human" {
			return nil, potentialFormation, false
		}
		seen[kind] = true
	}
	return kinds, potentialFormation, true
}

func toolSchemaMigrationJudgeError(gateID, detail string) error {
	if gateID == "" {
		return fmt.Errorf("%s: %s", legacyJudgeChannelMigrationCode, detail)
	}
	return fmt.Errorf("%s: gate %q: %s", legacyJudgeChannelMigrationCode, gateID, detail)
}

func toolSchemaMigrationValidatePort(lines []tomlLine, schema int, port toolSchemaMigrationPort, insertions map[int][]tomlLine) error {
	defaults := []toolSchemaMigrationFieldDefault{
		{key: "direction", rendered: renderString(port.direction), want: port.direction},
		{key: "kind", rendered: renderString("work"), want: "work"},
		{key: "acceptedMediaTypes", rendered: renderStringArray(allToolWorkMediaTypes())},
	}
	if port.direction == FormationPortInput {
		defaults = append(defaults,
			toolSchemaMigrationFieldDefault{key: "required", rendered: "true", want: "true"},
			toolSchemaMigrationFieldDefault{key: "role", rendered: renderString("data"), want: "data"},
		)
	} else if len(port.fields["required"]) != 0 || len(port.fields["role"]) != 0 {
		return fmt.Errorf("Formation output %q contains input-only migration-owned fields", port.portID)
	}

	for _, field := range defaults {
		missing, err := toolSchemaMigrationValidateOwnedField(port.fields, field.key, field.want)
		if err != nil {
			return fmt.Errorf("Formation %s port %q: %w", port.direction, port.portID, err)
		}
		if !missing {
			continue
		}
		if schema == CurrentBoardSchema {
			return fmt.Errorf("Formation %s port %q is missing migration-owned field %q", port.direction, port.portID, field.key)
		}
		toolSchemaMigrationAddInsertion(lines, insertions, port.end, field.key+" = "+field.rendered)
	}
	return nil
}

func toolSchemaMigrationValidateOwnedField(fields map[string][]string, key, want string) (bool, error) {
	values := fields[key]
	if len(values) == 0 {
		return true, nil
	}
	if len(values) != 1 {
		return false, fmt.Errorf("migration-owned field %q occurs %d times", key, len(values))
	}

	var matches bool
	switch key {
	case "acceptedMediaTypes":
		value, err := parseToolStringArray(values[0])
		matches = err == nil && equalToolStrings(value, allToolWorkMediaTypes())
	case "required":
		value, err := parseToolBoolean(values[0])
		matches = err == nil && value
	default:
		value, err := parseToolString(values[0])
		matches = err == nil && value == want
	}
	if !matches {
		return false, fmt.Errorf("migration-owned field %q conflicts with required value", key)
	}
	return false, nil
}

func toolSchemaMigrationAddInsertion(lines []tomlLine, insertions map[int][]tomlLine, index int, body string) {
	if index == len(lines) && index > 0 && lines[index-1].newline == "" {
		index--
		for index > 0 && lines[index].valueContinuation {
			index--
		}
	}
	newline := "\n"
	for previous := index - 1; previous >= 0; previous-- {
		if lines[previous].newline != "" {
			newline = lines[previous].newline
			break
		}
	}
	insertions[index] = append(insertions[index], tomlLine{body: body, newline: newline})
}

func toolSchemaMigrationApplyInsertions(lines []tomlLine, insertions map[int][]tomlLine) []tomlLine {
	count := len(lines)
	for _, inserted := range insertions {
		count += len(inserted)
	}
	next := make([]tomlLine, 0, count)
	for index := 0; index <= len(lines); index++ {
		next = append(next, insertions[index]...)
		if index < len(lines) {
			next = append(next, lines[index])
		}
	}
	return next
}
