package formations

import (
	"fmt"

	"github.com/pelletier/go-toml/v2"
)

type boardTOMLSource struct {
	Schema      int
	ID          string
	Slug        string
	Title       string
	Rev         int
	UpdatedBy   string
	UpdatedAt   string
	Missions    []MissionNode
	Formations  []FormationNode
	Gates       []GateNode
	Connections []BoardConnection
}

type layoutTOMLSource struct {
	Schema    int
	BoardID   string
	BoardRev  int
	UpdatedAt string
	Nodes     []LayoutNode
	Edges     []LayoutEdge
}

func decodeBoardTOML(raw []byte) (boardTOMLSource, error) {
	document, err := decodeTOMLMap(raw)
	if err != nil {
		return boardTOMLSource{}, err
	}
	var source boardTOMLSource
	if source.Schema, err = tomlInt(document, "schema"); err != nil {
		return boardTOMLSource{}, err
	}
	if source.ID, err = tomlString(document, "id"); err != nil {
		return boardTOMLSource{}, err
	}
	if source.Slug, err = tomlString(document, "slug"); err != nil {
		return boardTOMLSource{}, err
	}
	if source.Title, err = tomlString(document, "title"); err != nil {
		return boardTOMLSource{}, err
	}
	if source.Rev, err = tomlInt(document, "rev"); err != nil {
		return boardTOMLSource{}, err
	}
	if source.UpdatedBy, err = tomlString(document, "updatedBy"); err != nil {
		return boardTOMLSource{}, err
	}
	if source.UpdatedAt, err = tomlString(document, "updatedAt"); err != nil {
		return boardTOMLSource{}, err
	}
	if source.Missions, err = decodeMissionNodes(document); err != nil {
		return boardTOMLSource{}, err
	}
	if source.Formations, err = decodeFormationNodes(document); err != nil {
		return boardTOMLSource{}, err
	}
	if source.Gates, err = decodeGateNodes(document); err != nil {
		return boardTOMLSource{}, err
	}
	if source.Connections, err = decodeBoardConnections(document); err != nil {
		return boardTOMLSource{}, err
	}
	return source, nil
}

func decodeLayoutTOML(raw []byte) (layoutTOMLSource, error) {
	document, err := decodeTOMLMap(raw)
	if err != nil {
		return layoutTOMLSource{}, err
	}
	var source layoutTOMLSource
	if source.Schema, err = tomlInt(document, "schema"); err != nil {
		return layoutTOMLSource{}, err
	}
	if source.BoardID, err = tomlString(document, "boardId"); err != nil {
		return layoutTOMLSource{}, err
	}
	if source.BoardRev, err = tomlInt(document, "boardRev"); err != nil {
		return layoutTOMLSource{}, err
	}
	if source.UpdatedAt, err = tomlString(document, "updatedAt"); err != nil {
		return layoutTOMLSource{}, err
	}
	if source.Nodes, err = decodeLayoutNodes(document); err != nil {
		return layoutTOMLSource{}, err
	}
	if source.Edges, err = decodeLayoutEdges(document); err != nil {
		return layoutTOMLSource{}, err
	}
	return source, nil
}

func decodeTOMLMap(raw []byte) (map[string]any, error) {
	var document map[string]any
	if err := toml.Unmarshal(raw, &document); err != nil {
		return nil, invalidDefinitionSource(err)
	}
	return document, nil
}

func invalidDefinitionSource(err error) error {
	return fmt.Errorf("%w: %v", ErrInvalidDefinitionSource, err)
}

func unsupportedTOMLField(key string, value any) error {
	return invalidDefinitionSource(fmt.Errorf("field %q has unsupported TOML value type %T", key, value))
}

func tomlString(table map[string]any, key string) (string, error) {
	value, present := table[key]
	if !present {
		return "", nil
	}
	decoded, ok := value.(string)
	if !ok {
		return "", unsupportedTOMLField(key, value)
	}
	return decoded, nil
}

func tomlInt(table map[string]any, key string) (int, error) {
	value, present := table[key]
	if !present {
		return 0, nil
	}
	decoded, ok := value.(int64)
	if !ok {
		return 0, unsupportedTOMLField(key, value)
	}
	projected := int(decoded)
	if int64(projected) != decoded {
		return 0, unsupportedTOMLField(key, value)
	}
	return projected, nil
}

func tomlBool(table map[string]any, key string) (bool, error) {
	value, present := table[key]
	if !present {
		return false, nil
	}
	decoded, ok := value.(bool)
	if !ok {
		return false, unsupportedTOMLField(key, value)
	}
	return decoded, nil
}

func tomlStringArray(table map[string]any, key string) ([]string, error) {
	value, present := table[key]
	if !present {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, unsupportedTOMLField(key, value)
	}
	decoded := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, unsupportedTOMLField(key, value)
		}
		decoded = append(decoded, text)
	}
	return decoded, nil
}

func tomlTable(table map[string]any, key string) (map[string]any, bool, error) {
	value, present := table[key]
	if !present {
		return nil, false, nil
	}
	decoded, ok := value.(map[string]any)
	if ok {
		return decoded, true, nil
	}
	items, err := tomlTableArray(table, key)
	if err != nil || len(items) != 1 {
		return nil, false, unsupportedTOMLField(key, value)
	}
	return items[0], true, nil
}

func tomlTableArray(table map[string]any, key string) ([]map[string]any, error) {
	value, present := table[key]
	if !present {
		return nil, nil
	}
	items, ok := value.([]map[string]any)
	if ok {
		if items == nil {
			items = make([]map[string]any, 0)
		}
		return items, nil
	}
	generic, ok := value.([]any)
	if !ok {
		return nil, unsupportedTOMLField(key, value)
	}
	items = make([]map[string]any, 0, len(generic))
	for _, item := range generic {
		tableItem, ok := item.(map[string]any)
		if !ok {
			return nil, unsupportedTOMLField(key, value)
		}
		items = append(items, tableItem)
	}
	return items, nil
}

func decodeMissionNodes(document map[string]any) ([]MissionNode, error) {
	tables, err := tomlTableArray(document, "mission")
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return nil, nil
	}
	nodes := make([]MissionNode, 0, len(tables))
	for _, table := range tables {
		var node MissionNode
		if node.ID, err = tomlString(table, "id"); err != nil {
			return nil, err
		}
		if node.Title, err = tomlString(table, "title"); err != nil {
			return nil, err
		}
		if node.Goal, err = tomlString(table, "goal"); err != nil {
			return nil, err
		}
		if node.BeadID, err = tomlString(table, "beadId"); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func decodeFormationNodes(document map[string]any) ([]FormationNode, error) {
	tables, err := tomlTableArray(document, "formation")
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return nil, nil
	}
	nodes := make([]FormationNode, 0, len(tables))
	for _, table := range tables {
		var node FormationNode
		if node.ID, err = tomlString(table, "id"); err != nil {
			return nil, err
		}
		if node.Type, err = tomlString(table, "type"); err != nil {
			return nil, err
		}
		if node.Title, err = tomlString(table, "title"); err != nil {
			return nil, err
		}
		if node.Inputs, err = decodeFormationPorts(table, "input"); err != nil {
			return nil, err
		}
		if node.Outputs, err = decodeFormationPorts(table, "output"); err != nil {
			return nil, err
		}
		if node.Slots, err = decodeFormationSlots(table); err != nil {
			return nil, err
		}
		brief, present, err := tomlTable(table, "brief")
		if err != nil {
			return nil, err
		}
		if present {
			node.Brief, err = decodeFormationBrief(brief)
			if err != nil {
				return nil, err
			}
		}
		verification, present, err := tomlTable(table, "verification")
		if err != nil {
			return nil, err
		}
		if present {
			node.Verification, err = decodeFormationVerification(verification)
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func decodeFormationPorts(table map[string]any, key string) ([]FormationPort, error) {
	tables, err := tomlTableArray(table, key)
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return nil, nil
	}
	ports := make([]FormationPort, 0, len(tables))
	for _, table := range tables {
		var port FormationPort
		if port.ID, err = tomlString(table, "id"); err != nil {
			return nil, err
		}
		if port.Label, err = tomlString(table, "label"); err != nil {
			return nil, err
		}
		ports = append(ports, port)
	}
	return ports, nil
}

func decodeFormationSlots(table map[string]any) ([]FormationSlot, error) {
	tables, err := tomlTableArray(table, "slot")
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return nil, nil
	}
	slots := make([]FormationSlot, 0, len(tables))
	for _, table := range tables {
		var slot FormationSlot
		if slot.ID, err = tomlString(table, "id"); err != nil {
			return nil, err
		}
		if slot.Label, err = tomlString(table, "label"); err != nil {
			return nil, err
		}
		if slot.AgentID, err = tomlString(table, "agentId"); err != nil {
			return nil, err
		}
		if slot.Harness, err = tomlString(table, "harness"); err != nil {
			return nil, err
		}
		if slot.Controller, err = tomlBool(table, "controller"); err != nil {
			return nil, err
		}
		slots = append(slots, slot)
	}
	return slots, nil
}

func decodeFormationBrief(table map[string]any) (*FormationBrief, error) {
	var brief FormationBrief
	var err error
	if brief.Goal, err = tomlString(table, "goal"); err != nil {
		return nil, err
	}
	if brief.BeadID, err = tomlString(table, "beadId"); err != nil {
		return nil, err
	}
	if brief.Files, err = tomlStringArray(table, "files"); err != nil {
		return nil, err
	}
	if brief.Links, err = tomlStringArray(table, "links"); err != nil {
		return nil, err
	}
	return &brief, nil
}

func decodeFormationVerification(table map[string]any) (*FormationVerification, error) {
	var verification FormationVerification
	var err error
	if verification.ID, err = tomlString(table, "id"); err != nil {
		return nil, err
	}
	if verification.Kinds, err = tomlStringArray(table, "kinds"); err != nil {
		return nil, err
	}
	if verification.Criterion, err = tomlString(table, "criterion"); err != nil {
		return nil, err
	}
	if verification.OnFail, err = tomlString(table, "onFail"); err != nil {
		return nil, err
	}
	return &verification, nil
}

func decodeGateNodes(document map[string]any) ([]GateNode, error) {
	tables, err := tomlTableArray(document, "gate")
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return nil, nil
	}
	nodes := make([]GateNode, 0, len(tables))
	for _, table := range tables {
		node := GateNode{legacyCommandFields: make(map[string]int)}
		if node.ID, err = tomlString(table, "id"); err != nil {
			return nil, err
		}
		if node.Title, err = tomlString(table, "title"); err != nil {
			return nil, err
		}
		if node.Kinds, err = tomlStringArray(table, "kinds"); err != nil {
			return nil, err
		}
		if node.Criterion, err = tomlString(table, "criterion"); err != nil {
			return nil, err
		}
		if node.Check, err = tomlString(table, "check"); err != nil {
			return nil, err
		}
		if node.CheckVersion, err = tomlString(table, "checkVersion"); err != nil {
			return nil, err
		}
		if node.CheckValue, err = tomlString(table, "checkValue"); err != nil {
			return nil, err
		}
		if node.Command, err = tomlString(table, "command"); err != nil {
			return nil, err
		}
		if node.CommandArgv, err = tomlStringArray(table, "commandArgv"); err != nil {
			return nil, err
		}
		if node.CommandCWD, err = tomlString(table, "commandCwd"); err != nil {
			return nil, err
		}
		if node.CommandShell, err = tomlString(table, "commandShell"); err != nil {
			return nil, err
		}
		for _, key := range []string{"command", "commandArgv", "commandCwd", "commandShell"} {
			if _, present := table[key]; present {
				node.legacyCommandFields[key] = 1
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func decodeBoardConnections(document map[string]any) ([]BoardConnection, error) {
	tables, err := tomlTableArray(document, "connection")
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return nil, nil
	}
	connections := make([]BoardConnection, 0, len(tables))
	for _, table := range tables {
		var connection BoardConnection
		if connection.ID, err = tomlString(table, "id"); err != nil {
			return nil, err
		}
		if connection.From, err = tomlString(table, "from"); err != nil {
			return nil, err
		}
		if connection.To, err = tomlString(table, "to"); err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	return connections, nil
}

func decodeLayoutNodes(document map[string]any) ([]LayoutNode, error) {
	tables, err := tomlTableArray(document, "node")
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return nil, nil
	}
	nodes := make([]LayoutNode, 0, len(tables))
	for _, table := range tables {
		var node LayoutNode
		if node.ID, err = tomlString(table, "id"); err != nil {
			return nil, err
		}
		if node.X, err = tomlInt(table, "x"); err != nil {
			return nil, err
		}
		if node.Y, err = tomlInt(table, "y"); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func decodeLayoutEdges(document map[string]any) ([]LayoutEdge, error) {
	tables, err := tomlTableArray(document, "edge")
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return nil, nil
	}
	edges := make([]LayoutEdge, 0, len(tables))
	for _, table := range tables {
		var edge LayoutEdge
		if edge.ID, err = tomlString(table, "id"); err != nil {
			return nil, err
		}
		if edge.Lane, err = tomlString(table, "lane"); err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

func decodedStringInLineRange(lines []tomlLine, start, end int, key string) (string, bool) {
	values, ok := decodedValuesInLineRange(lines, start, end)
	if !ok {
		return "", false
	}
	value, ok := values[key].(string)
	return value, ok
}

func decodedStringArrayInLineRange(lines []tomlLine, start, end int, key string) ([]string, bool) {
	values, ok := decodedValuesInLineRange(lines, start, end)
	if !ok {
		return nil, false
	}
	value, err := tomlStringArray(values, key)
	return value, err == nil && value != nil
}

func decodedValuesInLineRange(lines []tomlLine, start, end int) (map[string]any, bool) {
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	for index := start; index < end; index++ {
		if isTOMLHeader(lines[index]) {
			end = index
			break
		}
	}
	if start >= end {
		return nil, false
	}
	var values map[string]any
	if err := toml.Unmarshal(renderTOMLLines(lines[start:end]), &values); err != nil {
		return nil, false
	}
	return values, true
}
