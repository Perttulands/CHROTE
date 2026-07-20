package formations

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
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

type ToolUpdateRequest struct {
	ToolID    string
	Title     *string
	Params    *map[string]any
	UpdatedBy string
}

type ToolUpdateResult struct {
	Board  *BoardDocument  `json:"board"`
	Layout *LayoutDocument `json:"layout"`
	Tool   ToolNode        `json:"tool"`
}

type ToolDeleteRequest struct {
	ID        string
	UpdatedBy string
}

type ToolDeleteResult struct {
	Board  *BoardDocument  `json:"board"`
	Layout *LayoutDocument `json:"layout"`
	ToolID string          `json:"toolId"`
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

func (s *Store) UpdateTool(slug string, req ToolUpdateRequest, opts ToolWriteOptions) (*ToolUpdateResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if err := validateToolWriteOptions(opts); err != nil {
		return nil, err
	}
	if req.ToolID == "" {
		return nil, ErrNotFound
	}
	if req.Title == nil && req.Params == nil {
		return nil, fmt.Errorf("Tool update requires title and/or parameters")
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		return nil, fmt.Errorf("Tool title is required")
	}
	if req.Params != nil && *req.Params == nil {
		return nil, fmt.Errorf("Tool parameter object is required")
	}
	if err := validateToolUpdatedBy(req.UpdatedBy); err != nil {
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
	var updated ToolNode
	request := definitionPairPublicationRequest{
		expected: expected,
		build: func(current definitionPairState) (definitionPairState, error) {
			built, tool, err := s.buildToolUpdateCandidate(slug, current, req)
			if err != nil {
				return definitionPairState{}, err
			}
			candidate = cloneDefinitionPairState(built)
			updated = tool
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
	var layout *LayoutDocument
	if candidate.layout.present {
		layout, err = parseLayout(candidate.layout.raw)
		if err != nil {
			return nil, err
		}
	}
	return &ToolUpdateResult{Board: board, Layout: layout, Tool: updated}, nil
}

func (s *Store) DeleteTool(slug string, req ToolDeleteRequest, opts ToolWriteOptions) (*ToolDeleteResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if err := validateToolWriteOptions(opts); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, ErrNotFound
	}
	if err := validateToolUpdatedBy(req.UpdatedBy); err != nil {
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
	request := definitionPairPublicationRequest{
		expected: expected,
		build: func(current definitionPairState) (definitionPairState, error) {
			built, err := s.buildToolDeleteCandidate(slug, current, req)
			if err != nil {
				return definitionPairState{}, err
			}
			candidate = cloneDefinitionPairState(built)
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
	var layout *LayoutDocument
	if candidate.layout.present {
		layout, err = parseLayout(candidate.layout.raw)
		if err != nil {
			return nil, err
		}
	}
	return &ToolDeleteResult{Board: board, Layout: layout, ToolID: req.ID}, nil
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
	if err := validateToolMutationBoardSource(current.board); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
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
	var layoutBlocks []toolLayoutOwnedBlock
	if current.layout.present {
		layout, err = parseLayout(current.layout.raw)
		if err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
		layoutBlocks, err = validateToolMutationLayout(current.layout.raw, layout, board.ID)
		if err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
	}
	position, err := toolCreatePosition(req.Placement, board, layoutBlocks)
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
	if err := validateToolMutationBoardSource(nextBoardRaw); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	nextBoardRaw, err = validateToolSchemaTwoMigrationAuthority(nextBoardRaw)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
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
	if _, err := validateToolMutationLayout(nextLayoutRaw, nextLayout, nextBoard.ID); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	return definitionPairState{
		board:  nextBoardRaw,
		layout: definitionPairContent{present: true, raw: nextLayoutRaw},
	}, tool, nil
}

func (s *Store) buildToolUpdateCandidate(
	slug string,
	current definitionPairState,
	req ToolUpdateRequest,
) (definitionPairState, ToolNode, error) {
	if err := validateToolMutationBoardSource(current.board); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	board, err := parseBoard(current.board)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	if err := validateToolMutationBoard(board, slug); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	validatedBoardRaw, err := validateToolSchemaTwoMigrationAuthority(current.board)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}

	tool, found := toolNodeByID(board, req.ToolID)
	if !found {
		return definitionPairState{}, ToolNode{}, ErrNotFound
	}
	descriptor, ok := LookupToolProfileDescriptor(tool.ProfileID, tool.ProfileVersion)
	if !ok {
		return definitionPairState{}, ToolNode{}, fmt.Errorf("unknown Tool profile tuple %q@%q", tool.ProfileID, tool.ProfileVersion)
	}
	updated := tool
	if req.Title != nil {
		updated.Title = *req.Title
	}
	if req.Params != nil {
		updated.Params = cloneToolParameters(*req.Params)
	}
	if err := validateToolNodeAgainstDescriptor(updated, descriptor); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}

	if current.layout.present {
		layout, err := parseLayout(current.layout.raw)
		if err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
		if _, err := validateToolMutationLayout(current.layout.raw, layout, board.ID); err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
	}

	nextBoardRaw, err := patchToolUpdate(validatedBoardRaw, tool, updated, req)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	doc := parseTOMLDocument(nextBoardRaw)
	updatedAt := s.now().Format(time.RFC3339)
	if req.UpdatedBy != "" {
		doc.setScalar("updatedBy", renderString(req.UpdatedBy))
	}
	doc.setScalar("rev", renderInt(board.Rev+1))
	doc.setScalar("updatedAt", renderString(updatedAt))
	nextBoardRaw = doc.bytes()
	if err := validateToolMutationBoardSource(nextBoardRaw); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	nextBoardRaw, err = validateToolSchemaTwoMigrationAuthority(nextBoardRaw)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	nextBoard, err := parseBoard(nextBoardRaw)
	if err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	if err := validateToolMutationBoard(nextBoard, slug); err != nil {
		return definitionPairState{}, ToolNode{}, err
	}
	updated, found = toolNodeByID(nextBoard, req.ToolID)
	if !found {
		return definitionPairState{}, ToolNode{}, ErrNotFound
	}

	next := definitionPairState{board: nextBoardRaw}
	if current.layout.present {
		nextLayoutRaw, err := updatePresentToolLayoutAuthority(current.layout.raw, nextBoard, "", updatedAt)
		if err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
		nextLayout, err := parseLayout(nextLayoutRaw)
		if err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
		if _, err := validateToolMutationLayout(nextLayoutRaw, nextLayout, nextBoard.ID); err != nil {
			return definitionPairState{}, ToolNode{}, err
		}
		next.layout = definitionPairContent{present: true, raw: nextLayoutRaw}
	}
	return next, updated, nil
}

func (s *Store) buildToolDeleteCandidate(
	slug string,
	current definitionPairState,
	req ToolDeleteRequest,
) (definitionPairState, error) {
	if err := validateToolMutationBoardSource(current.board); err != nil {
		return definitionPairState{}, err
	}
	board, err := parseBoard(current.board)
	if err != nil {
		return definitionPairState{}, err
	}
	if err := validateToolMutationBoard(board, slug); err != nil {
		return definitionPairState{}, err
	}
	validatedBoardRaw, err := validateToolSchemaTwoMigrationAuthority(current.board)
	if err != nil {
		return definitionPairState{}, err
	}
	if _, found := toolNodeByID(board, req.ID); !found {
		return definitionPairState{}, ErrNotFound
	}

	if current.layout.present {
		layout, err := parseLayout(current.layout.raw)
		if err != nil {
			return definitionPairState{}, err
		}
		if _, err := validateToolMutationLayout(current.layout.raw, layout, board.ID); err != nil {
			return definitionPairState{}, err
		}
	}

	nextBoardRaw, deleted := deleteToolBlock(validatedBoardRaw, req.ID)
	if !deleted {
		return definitionPairState{}, ErrNotFound
	}
	nextBoardRaw = deleteConnectionsTouchingNodes(nextBoardRaw, map[string]bool{req.ID: true})
	doc := parseTOMLDocument(nextBoardRaw)
	updatedAt := s.now().Format(time.RFC3339)
	if req.UpdatedBy != "" {
		doc.setScalar("updatedBy", renderString(req.UpdatedBy))
	}
	doc.setScalar("rev", renderInt(board.Rev+1))
	doc.setScalar("updatedAt", renderString(updatedAt))
	nextBoardRaw = doc.bytes()
	if err := validateToolMutationBoardSource(nextBoardRaw); err != nil {
		return definitionPairState{}, err
	}
	nextBoardRaw, err = validateToolSchemaTwoMigrationAuthority(nextBoardRaw)
	if err != nil {
		return definitionPairState{}, err
	}
	nextBoard, err := parseBoard(nextBoardRaw)
	if err != nil {
		return definitionPairState{}, err
	}
	if err := validateToolMutationBoard(nextBoard, slug); err != nil {
		return definitionPairState{}, err
	}

	next := definitionPairState{board: nextBoardRaw}
	if current.layout.present {
		nextLayoutRaw, err := updatePresentToolLayoutAuthority(current.layout.raw, nextBoard, "", updatedAt)
		if err != nil {
			return definitionPairState{}, err
		}
		nextLayout, err := parseLayout(nextLayoutRaw)
		if err != nil {
			return definitionPairState{}, err
		}
		if _, err := validateToolMutationLayout(nextLayoutRaw, nextLayout, nextBoard.ID); err != nil {
			return definitionPairState{}, err
		}
		next.layout = definitionPairContent{present: true, raw: nextLayoutRaw}
	}
	return next, nil
}

func toolNodeByID(board *BoardDocument, id string) (ToolNode, bool) {
	for _, tool := range board.Tools {
		if tool.ID == id {
			return tool, true
		}
	}
	return ToolNode{}, false
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

func patchToolUpdate(raw []byte, before, after ToolNode, req ToolUpdateRequest) ([]byte, error) {
	lines := splitLines(raw)
	start, end, ok := findToolBlockByID(lines, before.ID)
	if !ok {
		return nil, ErrNotFound
	}
	if req.Title != nil && before.Title != after.Title {
		lines = setScalarInLineRange(lines, start+1, end, "title", renderString(after.Title))
	}
	if req.Params != nil && !reflect.DeepEqual(before.Params, after.Params) {
		var err error
		lines, err = replaceToolParameterSection(lines, start, end, after.Params)
		if err != nil {
			return nil, err
		}
	}
	return renderTOMLLines(lines), nil
}

func validateToolMutationBoardSource(raw []byte) error {
	var document map[string]any
	if err := toml.Unmarshal(raw, &document); err != nil {
		return fmt.Errorf("invalid_board_source: malformed board TOML: %w", err)
	}
	return nil
}

func validateToolSchemaTwoMigrationAuthority(raw []byte) ([]byte, error) {
	validated, err := migrateBoardToToolSchema(raw)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(validated, raw) {
		return nil, fmt.Errorf("Tool mutation requires board schema %d", CurrentBoardSchema)
	}
	return validated, nil
}

func findToolBlockByID(lines []tomlLine, toolID string) (int, int, bool) {
	for index := 0; index < len(lines); index++ {
		section, ok := tomlLineSectionName(lines[index])
		if !ok || section != "tool" || !strings.HasPrefix(strings.TrimSpace(lines[index].body), "[[") {
			continue
		}
		end := toolBlockEnd(lines, index)
		if toolStringScalarInBlock(lines, index+1, end, "id") == toolID {
			return index, end, true
		}
	}
	return 0, 0, false
}

func deleteToolBlock(raw []byte, toolID string) ([]byte, bool) {
	lines := splitLines(raw)
	start, end, found := findToolBlockByID(lines, toolID)
	if !found {
		return raw, false
	}
	lines = append(lines[:start], lines[end:]...)
	return renderTOMLLines(lines), true
}

func toolStringScalarInBlock(lines []tomlLine, start, end int, key string) string {
	for index := start; index < end && index < len(lines); index++ {
		if _, ok := tomlLineSectionName(lines[index]); ok || isTOMLHeader(lines[index]) {
			break
		}
		field, literal, present, err := parseToolAssignment(lines[index].body)
		if err != nil || !present || field != key {
			continue
		}
		value, err := parseToolString(literal)
		if err == nil {
			return value
		}
		return ""
	}
	return ""
}

func toolBlockEnd(lines []tomlLine, start int) int {
	for index := start + 1; index < len(lines); index++ {
		section, ok := tomlLineSectionName(lines[index])
		if !ok {
			if isTOMLHeader(lines[index]) {
				return index
			}
			continue
		}
		if section == "tool" || !tomlSectionIsOrDescendsFrom(section, "tool") {
			return index
		}
	}
	return len(lines)
}

func replaceToolParameterSection(lines []tomlLine, toolStart, toolEnd int, parameters map[string]any) ([]tomlLine, error) {
	sectionStart := -1
	sectionEnd := -1
	for index := toolStart + 1; index < toolEnd; index++ {
		section, ok := tomlLineSectionName(lines[index])
		if !ok || section != "tool.params" || strings.HasPrefix(strings.TrimSpace(lines[index].body), "[[") {
			continue
		}
		sectionStart = index
		sectionEnd = tomlBlockEnd(lines, index)
		if sectionEnd > toolEnd {
			sectionEnd = toolEnd
		}
		break
	}
	if sectionStart < 0 {
		return nil, fmt.Errorf("invalid Tool parameter section")
	}

	seen := make(map[string]bool, len(parameters))
	body := make([]tomlLine, 0, sectionEnd-sectionStart-1+len(parameters))
	for _, line := range lines[sectionStart+1 : sectionEnd] {
		key, _, present, err := parseToolAssignment(line.body)
		if err != nil {
			return nil, err
		}
		if !present {
			body = append(body, line)
			continue
		}
		value, keep := parameters[key]
		if !keep {
			continue
		}
		line.body = replaceScalarValue(line.body, renderToolParameter(value))
		body = append(body, line)
		seen[key] = true
	}
	names := make([]string, 0, len(parameters))
	for name := range parameters {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body = append(body, tomlLine{body: name + " = " + renderToolParameter(parameters[name]), newline: "\n"})
	}

	next := make([]tomlLine, 0, len(lines)-(sectionEnd-sectionStart-1)+len(body))
	next = append(next, lines[:sectionStart+1]...)
	next = append(next, body...)
	next = append(next, lines[sectionEnd:]...)
	return next, nil
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
	start   int
	end     int
	kind    string
	id      string
	idCount int
	x       int64
	y       int64
	hasX    bool
	hasY    bool
}

func validateToolMutationLayout(raw []byte, layout *LayoutDocument, boardID string) ([]toolLayoutOwnedBlock, error) {
	if layout == nil {
		return nil, fmt.Errorf("invalid_layout_schema: Tool mutation requires layout schema %d", CurrentLayoutSchema)
	}
	blocks, err := parseToolLayoutOwnedBlocks(raw)
	if err != nil {
		return nil, err
	}
	if layout.Schema != CurrentLayoutSchema {
		return nil, fmt.Errorf("invalid_layout_schema: Tool mutation requires layout schema %d", CurrentLayoutSchema)
	}
	if layout.BoardID != boardID {
		return nil, fmt.Errorf("%w: layout board %q does not match %q", ErrConflict, layout.BoardID, boardID)
	}
	return blocks, nil
}

func parseToolLayoutOwnedBlocks(raw []byte) ([]toolLayoutOwnedBlock, error) {
	lines := splitLines(raw)
	seen := map[string]map[string]bool{"node": {}, "edge": {}}
	var blocks []toolLayoutOwnedBlock
	reservedCounts := map[string]int{"schema": 0, "boardId": 0, "boardRev": 0, "updatedAt": 0}
	active := -1
	rootFields := false
	topLevel := true
	finishActive := func(end int) error {
		if active < 0 {
			return nil
		}
		block := &blocks[active]
		block.end = end
		if block.idCount != 1 || !validToolDefinitionID(block.id) {
			return fmt.Errorf("invalid_layout_id: %s id fields = %d", block.kind, block.idCount)
		}
		if seen[block.kind][block.id] {
			return fmt.Errorf("duplicate_layout_id: %s id %q is duplicated", block.kind, block.id)
		}
		seen[block.kind][block.id] = true
		active = -1
		rootFields = false
		return nil
	}

	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if line.valueContinuation {
			continue
		}
		trimmed := strings.TrimSpace(line.body)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			section, ok := tomlLineSectionName(line)
			if !ok {
				return nil, fmt.Errorf("invalid_layout_owned_source: malformed layout header")
			}
			topLevel = false
			kind := toolLayoutOwnedSectionKind(section)
			if kind == "" {
				if err := finishActive(index); err != nil {
					return nil, err
				}
				continue
			}
			if section == kind {
				if !strings.HasPrefix(trimmed, "[[") {
					return nil, fmt.Errorf("invalid_layout_owned_source: layout %s root must be an array block", kind)
				}
				if err := finishActive(index); err != nil {
					return nil, err
				}
				blocks = append(blocks, toolLayoutOwnedBlock{start: index, end: len(lines), kind: kind})
				active = len(blocks) - 1
				rootFields = true
				continue
			}
			if active < 0 || blocks[active].kind != kind {
				return nil, fmt.Errorf("invalid_layout_owned_source: orphan or competing %s descendant", kind)
			}
			rootFields = false
			continue
		}

		assignment := tomlAssignmentIndex(line.body)
		if assignment < 0 {
			return nil, fmt.Errorf("invalid_layout_owned_source: invalid nonassignment layout source")
		}
		path, valid := parseTOMLKeyPath(line.body[:assignment])
		if !valid {
			return nil, fmt.Errorf("invalid_layout_owned_source: malformed layout assignment key")
		}
		if topLevel && len(path) > 0 {
			if _, reserved := reservedCounts[path[0]]; reserved {
				if len(path) != 1 {
					return nil, fmt.Errorf("invalid_layout_identity: dotted %s root competes with layout identity", path[0])
				}
				reservedCounts[path[0]]++
				if reservedCounts[path[0]] != 1 {
					return nil, fmt.Errorf("invalid_layout_identity: duplicate %s field", path[0])
				}
				if err := validateToolLayoutReservedField(path[0], line.body); err != nil {
					return nil, err
				}
			}
			if path[0] == "node" || path[0] == "edge" {
				return nil, fmt.Errorf("invalid_layout_owned_source: top-level %s assignment competes with layout authority", path[0])
			}
		}
		if active < 0 || !rootFields || len(path) != 1 {
			continue
		}
		block := &blocks[active]
		switch path[0] {
		case "id":
			_, literal, present, err := parseToolAssignment(line.body)
			if err != nil || !present {
				return nil, fmt.Errorf("invalid_layout_id: malformed %s id", block.kind)
			}
			block.id, err = parseToolString(literal)
			if err != nil {
				return nil, fmt.Errorf("invalid_layout_id: malformed %s id", block.kind)
			}
			block.idCount++
		case "x", "y":
			if block.kind != "node" {
				continue
			}
			if err := parseToolLayoutCoordinateField(block, path[0], line.body); err != nil {
				return nil, err
			}
		}
	}
	if err := finishActive(len(lines)); err != nil {
		return nil, err
	}
	if reservedCounts["schema"] != 1 || reservedCounts["boardId"] != 1 {
		return nil, fmt.Errorf(
			"invalid_layout_identity: schema fields = %d, boardId fields = %d; want exactly one each",
			reservedCounts["schema"],
			reservedCounts["boardId"],
		)
	}
	var document map[string]any
	if err := toml.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("invalid_layout_owned_source: malformed layout TOML: %w", err)
	}
	return blocks, nil
}

func toolLayoutOwnedSectionKind(section string) string {
	for _, kind := range []string{"node", "edge"} {
		if tomlSectionIsOrDescendsFrom(section, kind) {
			return kind
		}
	}
	return ""
}

func validateToolLayoutReservedField(field, raw string) error {
	key, literal, present, err := parseToolAssignment(raw)
	if err != nil || !present || key != field {
		return fmt.Errorf("invalid_layout_identity: malformed %s field", field)
	}
	switch field {
	case "schema", "boardRev":
		value, err := strconv.ParseInt(literal, 10, 64)
		if err != nil || strconv.FormatInt(value, 10) != literal {
			return fmt.Errorf("invalid_layout_identity: malformed %s integer", field)
		}
	case "boardId", "updatedAt":
		value, ok := parseTOMLBasicString(literal)
		if !ok || !validToolString(value) {
			return fmt.Errorf("invalid_layout_identity: malformed %s string", field)
		}
	}
	return nil
}

func parseToolLayoutCoordinateField(block *toolLayoutOwnedBlock, field, raw string) error {
	_, literal, present, err := parseToolAssignment(raw)
	if err != nil || !present {
		return fmt.Errorf("invalid_layout_coordinate: malformed %s coordinate", field)
	}
	value, err := parseTOMLInteger(literal)
	if err != nil || !validToolLayoutCoordinate(value) {
		return fmt.Errorf("invalid_layout_coordinate: %s coordinate is outside signed 32-bit bounds", field)
	}
	switch field {
	case "x":
		if block.hasX {
			return fmt.Errorf("invalid_layout_coordinate: duplicate x coordinate")
		}
		block.x, block.hasX = value, true
	case "y":
		if block.hasY {
			return fmt.Errorf("invalid_layout_coordinate: duplicate y coordinate")
		}
		block.y, block.hasY = value, true
	}
	return nil
}

type toolLayoutPosition struct {
	x int64
	y int64
}

func toolCreatePosition(placement ToolPlacement, board *BoardDocument, blocks []toolLayoutOwnedBlock) (LayoutNode, error) {
	if placement.X != nil {
		return LayoutNode{X: *placement.X, Y: *placement.Y}, nil
	}
	positions, err := toolBoardPositions(board, blocks)
	if err != nil {
		return LayoutNode{}, err
	}
	predecessor, hasPredecessor := positions[placement.PredecessorNodeID]
	successor, hasSuccessor := positions[placement.SuccessorNodeID]
	if placement.PredecessorNodeID != "" && !hasPredecessor {
		return LayoutNode{}, fmt.Errorf("unknown Tool placement predecessor %q", placement.PredecessorNodeID)
	}
	if placement.SuccessorNodeID != "" && !hasSuccessor {
		return LayoutNode{}, fmt.Errorf("unknown Tool placement successor %q", placement.SuccessorNodeID)
	}
	desiredX, desiredY := int64(layoutPlacementMin), int64(layoutPlacementMin)
	switch {
	case hasPredecessor && hasSuccessor:
		desiredX, err = checkedToolLayoutMidpoint(predecessor.x, successor.x)
		if err != nil {
			return LayoutNode{}, err
		}
		desiredY, err = checkedToolLayoutMidpoint(predecessor.y, successor.y)
		if err != nil {
			return LayoutNode{}, err
		}
	case hasPredecessor:
		desiredX, err = checkedToolLayoutAdd(predecessor.x, int64(layoutPlacementStep))
		if err != nil {
			return LayoutNode{}, err
		}
		desiredY = predecessor.y
	case hasSuccessor:
		desiredX, err = checkedToolLayoutAdd(successor.x, -int64(layoutPlacementStep))
		if err != nil {
			return LayoutNode{}, err
		}
		desiredY = successor.y
	}
	occupied := make([]toolLayoutPosition, 0, len(positions))
	for _, position := range positions {
		occupied = append(occupied, position)
	}
	x, err := snapToolLayoutPosition(desiredX)
	if err != nil {
		return LayoutNode{}, err
	}
	y, err := snapToolLayoutPosition(desiredY)
	if err != nil {
		return LayoutNode{}, err
	}
	x = maxToolLayoutPosition(int64(layoutPlacementMin), x)
	y = maxToolLayoutPosition(int64(layoutPlacementMin), y)
	for attempt := 0; attempt < layoutPlacementMaxAttempts; attempt++ {
		collides, err := toolLayoutPositionCollides(x, y, occupied)
		if err != nil {
			return LayoutNode{}, err
		}
		if !collides {
			if !validToolLayoutCoordinate(x) || !validToolLayoutCoordinate(y) {
				return LayoutNode{}, invalidToolLayoutCoordinateError()
			}
			return LayoutNode{X: int(x), Y: int(y)}, nil
		}
		x, err = checkedToolLayoutAdd(x, int64(layoutPlacementStep))
		if err != nil {
			return LayoutNode{}, err
		}
		if x > int64(layoutPlacementWrapX) {
			x = int64(layoutPlacementMin)
			y, err = checkedToolLayoutAdd(y, int64(layoutPlacementStep))
			if err != nil {
				return LayoutNode{}, err
			}
		}
	}
	return LayoutNode{}, fmt.Errorf("%w: no free layout position within bounded search", ErrConflict)
}

func toolBoardPositions(board *BoardDocument, blocks []toolLayoutOwnedBlock) (map[string]toolLayoutPosition, error) {
	persisted := make(map[string]toolLayoutPosition)
	for _, block := range blocks {
		if block.kind == "node" && block.hasX && block.hasY {
			persisted[block.id] = toolLayoutPosition{x: block.x, y: block.y}
		}
	}
	positions := make(map[string]toolLayoutPosition, len(board.Missions)+len(board.Formations)+len(board.Gates)+len(board.Tools))
	index := int64(0)
	add := func(id string) error {
		position, ok := persisted[id]
		if !ok {
			xOffset, err := checkedToolLayoutMultiply(index, 308)
			if err != nil {
				return err
			}
			x, err := checkedToolLayoutAdd(140, xOffset)
			if err != nil {
				return err
			}
			yOffset, err := checkedToolLayoutMultiply(index%2, 196)
			if err != nil {
				return err
			}
			y, err := checkedToolLayoutAdd(168, yOffset)
			if err != nil {
				return err
			}
			if !validToolLayoutCoordinate(x) || !validToolLayoutCoordinate(y) {
				return invalidToolLayoutCoordinateError()
			}
			position = toolLayoutPosition{x: x, y: y}
		}
		positions[id] = position
		var err error
		index, err = checkedToolLayoutAdd(index, 1)
		return err
	}
	for _, mission := range board.Missions {
		if err := add(mission.ID); err != nil {
			return nil, err
		}
	}
	for _, formation := range board.Formations {
		if err := add(formation.ID); err != nil {
			return nil, err
		}
	}
	for _, gate := range board.Gates {
		if err := add(gate.ID); err != nil {
			return nil, err
		}
	}
	for _, tool := range board.Tools {
		if err := add(tool.ID); err != nil {
			return nil, err
		}
	}
	return positions, nil
}

func checkedToolLayoutMidpoint(left, right int64) (int64, error) {
	sum, err := checkedToolLayoutAdd(left, right)
	if err != nil {
		return 0, err
	}
	return sum / 2, nil
}

func checkedToolLayoutAdd(left, right int64) (int64, error) {
	const maxInt64 = int64(^uint64(0) >> 1)
	const minInt64 = -maxInt64 - 1
	if right > 0 && left > maxInt64-right || right < 0 && left < minInt64-right {
		return 0, invalidToolLayoutCoordinateError()
	}
	return left + right, nil
}

func checkedToolLayoutMultiply(left, right int64) (int64, error) {
	if left == 0 || right == 0 {
		return 0, nil
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	const minInt64 = -maxInt64 - 1
	if left == -1 && right == minInt64 || right == -1 && left == minInt64 {
		return 0, invalidToolLayoutCoordinateError()
	}
	result := left * right
	if result/right != left {
		return 0, invalidToolLayoutCoordinateError()
	}
	return result, nil
}

func snapToolLayoutPosition(value int64) (int64, error) {
	grid := int64(layoutPlacementGrid)
	quotient := value / grid
	remainder := value % grid
	var err error
	switch {
	case remainder >= grid/2:
		quotient, err = checkedToolLayoutAdd(quotient, 1)
	case remainder <= -grid/2:
		quotient, err = checkedToolLayoutAdd(quotient, -1)
	}
	if err != nil {
		return 0, err
	}
	return checkedToolLayoutMultiply(quotient, grid)
}

func toolLayoutPositionCollides(x, y int64, occupied []toolLayoutPosition) (bool, error) {
	for _, position := range occupied {
		distanceX, err := toolLayoutAbsoluteDistance(position.x, x)
		if err != nil {
			return false, err
		}
		distanceY, err := toolLayoutAbsoluteDistance(position.y, y)
		if err != nil {
			return false, err
		}
		if distanceX < int64(layoutPlacementWidth) && distanceY < int64(layoutPlacementHeight) {
			return true, nil
		}
	}
	return false, nil
}

func toolLayoutAbsoluteDistance(left, right int64) (int64, error) {
	const minInt64 = -int64(^uint64(0)>>1) - 1
	if right == minInt64 {
		return 0, invalidToolLayoutCoordinateError()
	}
	difference, err := checkedToolLayoutAdd(left, -right)
	if err != nil {
		return 0, err
	}
	if difference == minInt64 {
		return 0, invalidToolLayoutCoordinateError()
	}
	if difference < 0 {
		difference = -difference
	}
	return difference, nil
}

func maxToolLayoutPosition(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func invalidToolLayoutCoordinateError() error {
	return fmt.Errorf("invalid_layout_coordinate: layout coordinate arithmetic is unsafe")
}

func updatePresentToolLayout(raw []byte, board *BoardDocument, newToolID string, position LayoutNode, updatedAt string) ([]byte, error) {
	filtered, err := updatePresentToolLayoutAuthority(raw, board, newToolID, updatedAt)
	if err != nil {
		return nil, err
	}
	return appendLayoutNodeBlock(filtered, position), nil
}

func updatePresentToolLayoutAuthority(raw []byte, board *BoardDocument, excludedNodeID, updatedAt string) ([]byte, error) {
	blocks, err := parseToolLayoutOwnedBlocks(raw)
	if err != nil {
		return nil, err
	}
	nodeIDs, edgeIDs := toolBoardAuthorityIDs(board)
	filtered := filterToolLayoutBlocks(raw, blocks, nodeIDs, edgeIDs, excludedNodeID)
	filtered = setToolLayoutScalarPreservingLeadingTrivia(filtered, "boardRev", renderInt(board.Rev))
	filtered = setToolLayoutScalarPreservingLeadingTrivia(filtered, "updatedAt", renderString(updatedAt))
	return filtered, nil
}

func setToolLayoutScalarPreservingLeadingTrivia(raw []byte, key, renderedValue string) []byte {
	doc := parseTOMLDocument(raw)
	if _, exists := doc.fields[key]; exists {
		doc.setScalar(key, renderedValue)
		return doc.bytes()
	}
	insertAt := doc.firstSection
	for insertAt > 0 && toolLayoutTrivia(doc.lines[insertAt-1]) {
		insertAt--
	}
	line := tomlLine{body: key + " = " + renderedValue, newline: "\n"}
	doc.lines = append(doc.lines, tomlLine{})
	copy(doc.lines[insertAt+1:], doc.lines[insertAt:])
	doc.lines[insertAt] = line
	return doc.bytes()
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
		} else {
			for _, line := range lines[block.start:block.end] {
				if toolLayoutTrivia(line) {
					filtered = append(filtered, line)
				}
			}
		}
		index = block.end
	}
	return renderTOMLLines(filtered)
}

func toolLayoutTrivia(line tomlLine) bool {
	trimmed := strings.TrimSpace(line.body)
	return !line.valueContinuation && (trimmed == "" || strings.HasPrefix(trimmed, "#"))
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
