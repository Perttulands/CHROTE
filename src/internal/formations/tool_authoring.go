package formations

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	LayoutWriteAbsent  = "absent"
	LayoutWritePresent = "present"
)

type LayoutWriteExpectation struct {
	State string
	ETag  string
}

type ToolWriteOptions struct {
	Board  WriteOptions
	Layout *LayoutWriteExpectation
}

type ToolPlacement struct {
	X                 *int
	Y                 *int
	PredecessorNodeID string
	SuccessorNodeID   string
}

type ToolCreateRequest struct {
	ProfileID      string
	ProfileVersion string
	Title          string
	Params         map[string]any
	Placement      ToolPlacement
	UpdatedBy      string
}

type ToolCreateResult struct {
	Board  *BoardDocument  `json:"board"`
	Layout *LayoutDocument `json:"layout"`
	Tool   ToolNode        `json:"tool"`
}

func (s *Store) CreateTool(slug string, req ToolCreateRequest, opts ToolWriteOptions) (*ToolCreateResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if err := validateToolWriteOptions(opts); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, fmt.Errorf("Tool title is required")
	}
	if req.Params == nil {
		return nil, fmt.Errorf("Tool parameter object is required")
	}
	if err := validateToolUpdatedBy(req.UpdatedBy); err != nil {
		return nil, err
	}
	descriptor, ok := LookupToolProfileDescriptor(req.ProfileID, req.ProfileVersion)
	if !ok {
		return nil, fmt.Errorf("unknown Tool profile tuple %q@%q", req.ProfileID, req.ProfileVersion)
	}
	if err := validateToolPlacementUnion(req.Placement); err != nil {
		return nil, err
	}

	expected := definitionPairStateIdentity{
		board: definitionPairIdentity{present: true, sha256: opts.Board.ExpectedETag},
		layout: definitionPairIdentity{
			present: opts.Layout.State == LayoutWritePresent,
			sha256:  opts.Layout.ETag,
		},
	}
	var candidate definitionPairState
	var created ToolNode
	request := definitionPairPublicationRequest{
		expected: expected,
		build: func(current definitionPairState) (definitionPairState, error) {
			built, tool, err := s.buildToolCreateCandidate(slug, current, req, descriptor)
			if err != nil {
				return definitionPairState{}, err
			}
			candidate = cloneDefinitionPairState(built)
			created = tool
			return built, nil
		},
		cas: func(current definitionPairState) error {
			board, err := parseBoard(current.board)
			if err != nil {
				return err
			}
			if board.Rev != opts.Board.ExpectedRev {
				return ErrConflict
			}
			return nil
		},
	}
	if err := s.publishDefinitionPair(slug, request, nil); err != nil {
		return nil, err
	}
	board, err := parseBoard(candidate.board)
	if err != nil {
		return nil, err
	}
	layout, err := parseLayout(candidate.layout.raw)
	if err != nil {
		return nil, err
	}
	return &ToolCreateResult{Board: board, Layout: layout, Tool: created}, nil
}

func validateToolWriteOptions(opts ToolWriteOptions) error {
	if opts.Board.ExpectedETag == "" || opts.Board.ExpectedRev == 0 || opts.Layout == nil {
		return ErrPreconditionRequired
	}
	switch opts.Layout.State {
	case LayoutWriteAbsent:
		if opts.Layout.ETag != "" {
			return ErrPreconditionRequired
		}
	case LayoutWritePresent:
		if opts.Layout.ETag == "" {
			return ErrPreconditionRequired
		}
	default:
		return ErrPreconditionRequired
	}
	return nil
}

func validateToolPlacementUnion(placement ToolPlacement) error {
	hasX := placement.X != nil
	hasY := placement.Y != nil
	hasHints := placement.PredecessorNodeID != "" || placement.SuccessorNodeID != ""
	if hasX != hasY {
		return fmt.Errorf("Tool exact placement requires both x and y")
	}
	if hasX && hasHints {
		return fmt.Errorf("Tool placement must use exact coordinates or heuristic hints, not both")
	}
	if hasX && (!validToolLayoutCoordinate(int64(*placement.X)) || !validToolLayoutCoordinate(int64(*placement.Y))) {
		return fmt.Errorf("invalid_layout_coordinate: Tool exact placement is outside signed 32-bit bounds")
	}
	if !hasX && placement.PredecessorNodeID != "" && placement.PredecessorNodeID == placement.SuccessorNodeID {
		return fmt.Errorf("Tool placement predecessor and successor must differ")
	}
	return nil
}

func validateToolUpdatedBy(updatedBy string) error {
	if updatedBy == "" {
		return nil
	}
	rendered := renderString(updatedBy)
	parsed, ok := parseTOMLBasicString(rendered)
	if !ok || parsed != updatedBy {
		return fmt.Errorf("invalid_tool_updated_by: UpdatedBy is not a TOML basic string")
	}
	return nil
}

func validToolLayoutCoordinate(value int64) bool {
	return value >= -1<<31 && value <= 1<<31-1
}

func (s *Store) buildToolCreateCandidate(
	slug string,
	current definitionPairState,
	req ToolCreateRequest,
	descriptor ToolProfileDescriptor,
) (definitionPairState, ToolNode, error) {
	inspected, err := parseBoard(current.board)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	if inspected.Schema == CurrentBoardSchema {
		if err := validateToolMutationBoard(inspected, slug); err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
	}
	migratedRaw, err := migrateBoardToToolSchema(current.board)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	board, err := parseBoard(migratedRaw)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	if err := validateToolMutationBoard(board, slug); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}

	var layout *LayoutDocument
	if current.layout.present {
		layout, err = parseLayout(current.layout.raw)
		if err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
		if err := validateToolMutationLayout(current.layout.raw, layout, board.ID); err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
	}
	position, err := toolCreatePosition(req.Placement, board, layout)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	tool := s.newToolNode(req, descriptor)
	if err := validateToolNodeAgainstDescriptor(tool, descriptor); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}

	doc := parseTOMLDocument(migratedRaw)
	updatedAt := s.now().Format(time.RFC3339)
	if req.UpdatedBy != "" {
		doc.setScalar("updatedBy", renderString(req.UpdatedBy))
	}
	doc.setScalar("rev", renderInt(board.Rev+1))
	doc.setScalar("updatedAt", renderString(updatedAt))
	nextBoardRaw := appendToolBlock(doc.bytes(), tool)
	nextBoard, err := parseBoard(nextBoardRaw)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	if err := validateToolMutationBoard(nextBoard, slug); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}

	position.ID = tool.ID
	var nextLayoutRaw []byte
	if current.layout.present {
		nextLayoutRaw, err = updatePresentToolLayout(current.layout.raw, nextBoard, tool.ID, position, updatedAt)
	} else {
		nextLayoutRaw = renderNewToolLayout(nextBoard, position, updatedAt)
	}
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	nextLayout, err := parseLayout(nextLayoutRaw)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	if err := validateToolMutationLayout(nextLayoutRaw, nextLayout, nextBoard.ID); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	return definitionPairState{
		board:  nextBoardRaw,
		layout: definitionPairContent{present: true, raw: nextLayoutRaw},
	}, tool, nil
}

func (s *Store) newToolNode(req ToolCreateRequest, descriptor ToolProfileDescriptor) ToolNode {
	tool := ToolNode{
		ID:             s.nextToolDefinitionID("tool"),
		Title:          req.Title,
		ProfileID:      descriptor.ProfileID,
		ProfileVersion: descriptor.ProfileVersion,
		Params:         cloneToolParameters(req.Params),
	}
	for _, source := range descriptor.Ports {
		port := ToolPort{
			ID:                 s.nextToolDefinitionID("port"),
			Name:               source.Name,
			Label:              source.Label,
			Direction:          source.Direction,
			Kind:               source.Kind,
			AcceptedMediaTypes: append([]string(nil), source.AcceptedMediaTypes...),
			Required:           cloneToolBool(source.Required),
			Role:               cloneToolString(source.Role),
		}
		if port.Direction == FormationPortInput {
			tool.Inputs = append(tool.Inputs, port)
		} else {
			tool.Outputs = append(tool.Outputs, port)
		}
	}
	return tool
}

func (s *Store) nextToolDefinitionID(prefix string) string {
	if s.newToolDefinitionID != nil {
		return s.newToolDefinitionID(prefix)
	}
	return newPrefixedID(prefix)
}

func cloneToolParameters(parameters map[string]any) map[string]any {
	cloned := make(map[string]any, len(parameters))
	for name, value := range parameters {
		cloned[name] = value
	}
	return cloned
}

func appendToolBlock(raw []byte, tool ToolNode) []byte {
	var builder strings.Builder
	builder.Write(raw)
	if len(raw) > 0 && !strings.HasSuffix(string(raw), "\n") {
		builder.WriteByte('\n')
	}
	if len(raw) > 0 {
		builder.WriteByte('\n')
	}
	builder.WriteString("[[tool]]\n")
	builder.WriteString("id = " + renderString(tool.ID) + "\n")
	builder.WriteString("title = " + renderString(tool.Title) + "\n")
	builder.WriteString("profileId = " + renderString(tool.ProfileID) + "\n")
	builder.WriteString("profileVersion = " + renderString(tool.ProfileVersion) + "\n\n")
	builder.WriteString("[tool.params]\n")
	parameterNames := make([]string, 0, len(tool.Params))
	for name := range tool.Params {
		parameterNames = append(parameterNames, name)
	}
	sort.Strings(parameterNames)
	for _, name := range parameterNames {
		builder.WriteString(name + " = " + renderToolParameter(tool.Params[name]) + "\n")
	}
	for _, port := range tool.Inputs {
		builder.WriteByte('\n')
		appendToolPortBlock(&builder, "input", port)
	}
	for _, port := range tool.Outputs {
		builder.WriteByte('\n')
		appendToolPortBlock(&builder, "output", port)
	}
	return []byte(builder.String())
}

func appendToolPortBlock(builder *strings.Builder, section string, port ToolPort) {
	builder.WriteString("[[tool." + section + "]]\n")
	builder.WriteString("id = " + renderString(port.ID) + "\n")
	builder.WriteString("name = " + renderString(port.Name) + "\n")
	builder.WriteString("label = " + renderString(port.Label) + "\n")
	builder.WriteString("direction = " + renderString(port.Direction) + "\n")
	builder.WriteString("kind = " + renderString(port.Kind) + "\n")
	builder.WriteString("acceptedMediaTypes = " + renderStringArray(port.AcceptedMediaTypes) + "\n")
	if port.Required != nil {
		builder.WriteString("required = " + strconv.FormatBool(*port.Required) + "\n")
	}
	if port.Role != nil {
		builder.WriteString("role = " + renderString(*port.Role) + "\n")
	}
}

func renderToolParameter(value any) string {
	switch typed := value.(type) {
	case string:
		return renderString(typed)
	case bool:
		return strconv.FormatBool(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func validateToolMutationBoard(board *BoardDocument, slug string) error {
	if board == nil || board.Schema != CurrentBoardSchema {
		return fmt.Errorf("Tool mutation requires board schema %d", CurrentBoardSchema)
	}
	if !validToolDefinitionID(board.ID) {
		return fmt.Errorf("invalid_board_id: board id %q is invalid", board.ID)
	}
	if board.Slug != slug || board.Rev <= 0 {
		return fmt.Errorf("invalid_board_identity: board slug/revision does not match mutation target")
	}

	nodeIDs := make(map[string]string, len(board.Missions)+len(board.Formations)+len(board.Gates)+len(board.Tools))
	addNode := func(id, kind string) error {
		if !validToolDefinitionID(id) {
			return fmt.Errorf("invalid_node_id: %s node id %q is invalid", kind, id)
		}
		if first, exists := nodeIDs[id]; exists {
			return fmt.Errorf("%s: node id %q duplicates %s", FindingDuplicateNodeID, id, first)
		}
		nodeIDs[id] = kind
		return nil
	}
	for _, mission := range board.Missions {
		if err := addNode(mission.ID, "Mission"); err != nil {
			return err
		}
	}
	portIDs := make(map[string]string)
	addPort := func(id, owner string) error {
		if !validToolDefinitionID(id) {
			return fmt.Errorf("invalid_port_id: port id %q is invalid", id)
		}
		if first, exists := portIDs[id]; exists {
			return fmt.Errorf("duplicate_port_id: port id %q duplicates port owned by %s", id, first)
		}
		portIDs[id] = owner
		return nil
	}
	for _, formation := range board.Formations {
		if err := addNode(formation.ID, "Formation"); err != nil {
			return err
		}
		for _, port := range append(append([]FormationPort(nil), formation.Inputs...), formation.Outputs...) {
			if err := addPort(port.ID, formation.ID); err != nil {
				return err
			}
		}
	}
	for _, gate := range board.Gates {
		if err := addNode(gate.ID, "Gate"); err != nil {
			return err
		}
	}
	for _, tool := range board.Tools {
		if err := addNode(tool.ID, "Tool"); err != nil {
			return err
		}
		if strings.TrimSpace(tool.Title) == "" {
			return fmt.Errorf("%s: Tool %q title is blank", FindingInvalidTool, tool.ID)
		}
		for _, port := range append(append([]ToolPort(nil), tool.Inputs...), tool.Outputs...) {
			if err := addPort(port.ID, tool.ID); err != nil {
				return err
			}
		}
	}
	edgeIDs := make(map[string]bool, len(board.Connections))
	for _, connection := range board.Connections {
		if !validToolDefinitionID(connection.ID) {
			return fmt.Errorf("invalid_edge_id: edge id %q is invalid", connection.ID)
		}
		if edgeIDs[connection.ID] {
			return fmt.Errorf("duplicate_edge_id: edge id %q is duplicated", connection.ID)
		}
		edgeIDs[connection.ID] = true
	}
	for _, finding := range ValidateBoard(board).Errors {
		if finding.Code == FindingMissionCount || finding.Code == FindingGateNotRoutable {
			continue
		}
		return fmt.Errorf("%s: %s", finding.Code, finding.Message)
	}
	return nil
}

func validToolDefinitionID(id string) bool {
	if id == "" || len(id) > 128 || !utf8.ValidString(id) {
		return false
	}
	for index := 0; index < len(id); index++ {
		value := id[index]
		if isASCIIAlphaNumeric(value) || value == '_' || value == '-' || value == '.' {
			continue
		}
		return false
	}
	return true
}

type toolLayoutOwnedBlock struct {
	start int
	end   int
	kind  string
	id    string
}

func validateToolMutationLayout(raw []byte, layout *LayoutDocument, boardID string) error {
	if layout == nil || layout.Schema != CurrentLayoutSchema {
		return fmt.Errorf("invalid_layout_schema: Tool mutation requires layout schema %d", CurrentLayoutSchema)
	}
	if layout.BoardID != boardID {
		return fmt.Errorf("%w: layout board %q does not match %q", ErrConflict, layout.BoardID, boardID)
	}
	_, err := parseToolLayoutOwnedBlocks(raw)
	return err
}

func parseToolLayoutOwnedBlocks(raw []byte) ([]toolLayoutOwnedBlock, error) {
	lines := splitLines(raw)
	seen := map[string]map[string]bool{"node": {}, "edge": {}}
	var blocks []toolLayoutOwnedBlock
	for index := 0; index < len(lines); index++ {
		section, ok := tomlLineSectionName(lines[index])
		if !ok || section != "node" && section != "edge" {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(lines[index].body), "[[") {
			return nil, fmt.Errorf("invalid_layout_id: layout %s must be an array block", section)
		}
		end := tomlBlockEnd(lines, index)
		id := ""
		count := 0
		for field := index + 1; field < end; field++ {
			assignment := tomlAssignmentIndex(lines[field].body)
			if assignment < 0 {
				continue
			}
			path, valid := parseTOMLKeyPath(lines[field].body[:assignment])
			if !valid || len(path) != 1 || path[0] != "id" {
				continue
			}
			_, literal, present, err := parseToolAssignment(lines[field].body)
			if err != nil || !present {
				return nil, fmt.Errorf("invalid_layout_id: malformed %s id", section)
			}
			id, err = parseToolString(literal)
			if err != nil {
				return nil, fmt.Errorf("invalid_layout_id: malformed %s id", section)
			}
			count++
		}
		if count != 1 || !validToolDefinitionID(id) {
			return nil, fmt.Errorf("invalid_layout_id: %s id fields = %d", section, count)
		}
		if seen[section][id] {
			return nil, fmt.Errorf("duplicate_layout_id: %s id %q is duplicated", section, id)
		}
		seen[section][id] = true
		blocks = append(blocks, toolLayoutOwnedBlock{start: index, end: end, kind: section, id: id})
		index = end - 1
	}
	return blocks, nil
}

func toolCreatePosition(placement ToolPlacement, board *BoardDocument, layout *LayoutDocument) (LayoutNode, error) {
	if placement.X != nil {
		return LayoutNode{X: *placement.X, Y: *placement.Y}, nil
	}
	positions := toolBoardPositions(board, layout)
	predecessor, hasPredecessor := positions[placement.PredecessorNodeID]
	successor, hasSuccessor := positions[placement.SuccessorNodeID]
	if placement.PredecessorNodeID != "" && !hasPredecessor {
		return LayoutNode{}, fmt.Errorf("unknown Tool placement predecessor %q", placement.PredecessorNodeID)
	}
	if placement.SuccessorNodeID != "" && !hasSuccessor {
		return LayoutNode{}, fmt.Errorf("unknown Tool placement successor %q", placement.SuccessorNodeID)
	}
	desiredX, desiredY := layoutPlacementMin, layoutPlacementMin
	switch {
	case hasPredecessor && hasSuccessor:
		desiredX = (predecessor.X + successor.X) / 2
		desiredY = (predecessor.Y + successor.Y) / 2
	case hasPredecessor:
		desiredX = predecessor.X + layoutPlacementStep
		desiredY = predecessor.Y
	case hasSuccessor:
		desiredX = successor.X - layoutPlacementStep
		desiredY = successor.Y
	}
	occupied := make([]LayoutNode, 0, len(positions))
	for _, position := range positions {
		occupied = append(occupied, position)
	}
	x := maxInt(layoutPlacementMin, snapLayoutPosition(desiredX))
	y := maxInt(layoutPlacementMin, snapLayoutPosition(desiredY))
	for attempt := 0; attempt < layoutPlacementMaxAttempts; attempt++ {
		if !layoutPositionCollides(x, y, occupied) {
			return LayoutNode{X: x, Y: y}, nil
		}
		x += layoutPlacementStep
		if x > layoutPlacementWrapX {
			x = layoutPlacementMin
			y += layoutPlacementStep
		}
	}
	return LayoutNode{}, fmt.Errorf("%w: no free layout position within bounded search", ErrConflict)
}

func toolBoardPositions(board *BoardDocument, layout *LayoutDocument) map[string]LayoutNode {
	persisted := make(map[string]LayoutNode)
	if layout != nil {
		for _, node := range layout.Nodes {
			persisted[node.ID] = node
		}
	}
	positions := make(map[string]LayoutNode, len(board.Missions)+len(board.Formations)+len(board.Gates)+len(board.Tools))
	index := 0
	add := func(id string) {
		position, ok := persisted[id]
		if !ok {
			position = LayoutNode{ID: id, X: 140 + index*308, Y: 168 + (index%2)*196}
		}
		positions[id] = position
		index++
	}
	for _, mission := range board.Missions {
		add(mission.ID)
	}
	for _, formation := range board.Formations {
		add(formation.ID)
	}
	for _, gate := range board.Gates {
		add(gate.ID)
	}
	for _, tool := range board.Tools {
		add(tool.ID)
	}
	return positions
}

func updatePresentToolLayout(raw []byte, board *BoardDocument, newToolID string, position LayoutNode, updatedAt string) ([]byte, error) {
	blocks, err := parseToolLayoutOwnedBlocks(raw)
	if err != nil {
		return nil, err
	}
	nodeIDs, edgeIDs := toolBoardAuthorityIDs(board)
	filtered := filterToolLayoutBlocks(raw, blocks, nodeIDs, edgeIDs, newToolID)
	doc := parseTOMLDocument(filtered)
	doc.setScalar("boardRev", renderInt(board.Rev))
	doc.setScalar("updatedAt", renderString(updatedAt))
	return appendLayoutNodeBlock(doc.bytes(), position), nil
}

func filterToolLayoutBlocks(raw []byte, blocks []toolLayoutOwnedBlock, nodeIDs, edgeIDs map[string]bool, excludedNodeID string) []byte {
	lines := splitLines(raw)
	byStart := make(map[int]toolLayoutOwnedBlock, len(blocks))
	for _, block := range blocks {
		byStart[block.start] = block
	}
	filtered := make([]tomlLine, 0, len(lines))
	for index := 0; index < len(lines); {
		block, ok := byStart[index]
		if !ok {
			filtered = append(filtered, lines[index])
			index++
			continue
		}
		keep := block.kind == "node" && nodeIDs[block.id] && block.id != excludedNodeID ||
			block.kind == "edge" && edgeIDs[block.id]
		if keep {
			filtered = append(filtered, lines[block.start:block.end]...)
		}
		index = block.end
	}
	return renderTOMLLines(filtered)
}

func toolBoardAuthorityIDs(board *BoardDocument) (map[string]bool, map[string]bool) {
	nodes := make(map[string]bool, len(board.Missions)+len(board.Formations)+len(board.Gates)+len(board.Tools))
	for _, mission := range board.Missions {
		nodes[mission.ID] = true
	}
	for _, formation := range board.Formations {
		nodes[formation.ID] = true
	}
	for _, gate := range board.Gates {
		nodes[gate.ID] = true
	}
	for _, tool := range board.Tools {
		nodes[tool.ID] = true
	}
	edges := make(map[string]bool, len(board.Connections))
	for _, connection := range board.Connections {
		edges[connection.ID] = true
	}
	return nodes, edges
}

func renderNewToolLayout(board *BoardDocument, position LayoutNode, updatedAt string) []byte {
	raw := []byte("schema = " + renderInt(CurrentLayoutSchema) + "\n" +
		"boardId = " + renderString(board.ID) + "\n" +
		"boardRev = " + renderInt(board.Rev) + "\n" +
		"updatedAt = " + renderString(updatedAt) + "\n")
	return appendLayoutNodeBlock(raw, position)
}
