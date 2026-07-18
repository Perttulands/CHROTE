package formations

import (
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	FormationTypeSolo         = "solo"
	FormationTypePeer         = "peer"
	FormationTypeFlow         = "flow"
	FormationTypeOrchestrated = "orchestrated"
	FormationPortInput        = "input"
	FormationPortOutput       = "output"
)

type FormationCreateRequest struct {
	Type      string
	Title     string
	X         int
	Y         int
	UpdatedBy string
}

type FormationCreateResult struct {
	Board     *BoardDocument  `json:"board"`
	Layout    *LayoutDocument `json:"layout"`
	Formation FormationNode   `json:"formation"`
}

type GateCreateResult struct {
	Board  *BoardDocument  `json:"board"`
	Layout *LayoutDocument `json:"layout"`
	Gate   GateNode        `json:"gate"`
}

type MissionCreateResult struct {
	Board   *BoardDocument  `json:"board"`
	Layout  *LayoutDocument `json:"layout"`
	Mission MissionNode     `json:"mission"`
}

type FormationDeleteRequest struct {
	ID        string
	UpdatedBy string
}

type FormationDeleteResult struct {
	Board       *BoardDocument  `json:"board"`
	Layout      *LayoutDocument `json:"layout"`
	FormationID string          `json:"formationId"`
}

type GateDeleteRequest struct {
	ID        string
	UpdatedBy string
}

type GateDeleteResult struct {
	Board  *BoardDocument  `json:"board"`
	Layout *LayoutDocument `json:"layout"`
	GateID string          `json:"gateId"`
}

type MissionDeleteRequest struct {
	ID        string
	UpdatedBy string
}

type MissionDeleteResult struct {
	Board     *BoardDocument  `json:"board"`
	Layout    *LayoutDocument `json:"layout"`
	MissionID string          `json:"missionId"`
}

type FormationSlotAssignmentRequest struct {
	FormationID string
	SlotID      string
	AgentID     string
	Harness     string
	UpdatedBy   string
}

type FormationControllerRequest struct {
	FormationID string
	SlotID      string
	UpdatedBy   string
}

type FormationBriefRequest struct {
	FormationID string
	Goal        string
	BeadID      string
	Files       []string
	Links       []string
	UpdatedBy   string
}

type FormationBriefClearRequest struct {
	FormationID string
	UpdatedBy   string
}

type FormationVerificationRequest struct {
	FormationID string
	Kinds       []string
	Criterion   string
	OnFail      string
	UpdatedBy   string
}

type FormationVerificationRemovalRequest struct {
	FormationID       string
	ReplacementGateID string
	UpdatedBy         string
}

type FormationPortRequest struct {
	FormationID string
	Direction   string
	Label       string
	UpdatedBy   string
}

type FormationPortRemovalRequest struct {
	FormationID string
	PortID      string
	UpdatedBy   string
}

type FormationWireRequest struct {
	From      string
	To        string
	UpdatedBy string
}

type FormationRewireRequest struct {
	From       string
	PreviousTo string
	To         string
	UpdatedBy  string
}

type GateCreateRequest struct {
	Title                      string
	Kinds                      []string
	Criterion                  string
	Command                    string // legacy inspection-only compatibility input
	CommandArgv                []string
	CommandCWD                 string
	CommandShell               string
	LegacyCommandFieldsPresent bool
	X                          int
	Y                          int
	UpdatedBy                  string
}

type GateUpdateRequest struct {
	GateID                     string
	Title                      string
	Kinds                      []string
	Criterion                  string
	Command                    string // legacy inspection-only compatibility input
	CommandArgv                []string
	CommandCWD                 string
	CommandShell               string
	LegacyCommandFieldsPresent bool
	UpdatedBy                  string
}

type GateJudgeRequest struct {
	GateID    string
	Chain     []string
	UpdatedBy string
}

type MissionCreateRequest struct {
	Title     string
	Goal      string
	BeadID    string
	X         int
	Y         int
	UpdatedBy string
}

type FormationNode struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Title        string                 `json:"title"`
	Brief        *FormationBrief        `json:"brief,omitempty"`
	Inputs       []FormationPort        `json:"inputs"`
	Outputs      []FormationPort        `json:"outputs"`
	Slots        []FormationSlot        `json:"slots"`
	Verification *FormationVerification `json:"verification,omitempty"`
}

type FormationPort struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type FormationSlot struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	AgentID    string `json:"agentId,omitempty"`
	Harness    string `json:"harness,omitempty"`
	Controller bool   `json:"controller"`
}

type FormationBrief struct {
	Goal   string   `json:"goal,omitempty"`
	BeadID string   `json:"beadId,omitempty"`
	Files  []string `json:"files,omitempty"`
	Links  []string `json:"links,omitempty"`
}

type FormationVerification struct {
	ID        string   `json:"id"`
	Kinds     []string `json:"kinds"`
	Criterion string   `json:"criterion"`
	OnFail    string   `json:"onFail"`
}

type BoardConnection struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type GateNode struct {
	ID                    string                               `json:"id"`
	Title                 string                               `json:"title"`
	Kinds                 []string                             `json:"kinds"`
	Criterion             string                               `json:"criterion"`
	Command               string                               `json:"command,omitempty"` // legacy inspection-only metadata
	CommandArgv           []string                             `json:"commandArgv,omitempty"`
	CommandCWD            string                               `json:"commandCwd,omitempty"`
	CommandShell          string                               `json:"commandShell,omitempty"`
	LegacyScriptMigration *LegacyScriptGateMigrationInspection `json:"legacyScriptMigration,omitempty"`
	legacyCommandFields   map[string]int
}

type MissionNode struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Goal   string `json:"goal"`
	BeadID string `json:"beadId"`
}

type LayoutNode struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
}

type LayoutEdge struct {
	ID   string `json:"id"`
	Lane string `json:"lane"`
}

func (s *Store) ResolveBoardSelector(selector string) (string, error) {
	if err := validateSlug(selector); err != nil {
		return "", err
	}
	boards, err := s.ListBoards()
	if err != nil {
		return "", err
	}
	matches := []BoardSummary{}
	for _, board := range boards {
		if board.ID == selector || board.Slug == selector {
			matches = append(matches, board)
		}
	}
	if len(matches) == 1 {
		return matches[0].Slug, nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%w: board %q matched %d boards", ErrAmbiguousSelector, selector, len(matches))
	}
	if _, err := os.Stat(s.BoardPath(selector)); err == nil {
		return selector, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return "", ErrNotFound
}

func (s *Store) CreateFormation(slug string, req FormationCreateRequest, opts WriteOptions) (*FormationCreateResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if opts.ExpectedETag == "" || opts.ExpectedRev == 0 {
		return nil, ErrPreconditionRequired
	}
	formation, err := newFormationNode(req)
	if err != nil {
		return nil, err
	}

	path := s.BoardPath(slug)
	var result *FormationCreateResult
	err = withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		current, err := parseBoard(raw)
		if err != nil {
			return err
		}
		if opts.ExpectedETag != current.ETag || opts.ExpectedRev != current.Rev {
			return ErrConflict
		}

		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		if req.UpdatedBy != "" {
			doc.setScalar("updatedBy", renderString(req.UpdatedBy))
		}
		nextRev := current.Rev + 1
		doc.setScalar("rev", renderInt(nextRev))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))

		nextRaw := appendFormationBlock(doc.bytes(), formation)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		board, err := parseBoard(nextRaw)
		if err != nil {
			return err
		}
		layout, err := s.upsertLayoutNode(slug, board.ID, board.Rev, LayoutNode{ID: formation.ID, X: req.X, Y: req.Y})
		if err != nil {
			return err
		}
		result = &FormationCreateResult{
			Board:     board,
			Layout:    layout,
			Formation: formation,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) DeleteFormation(slug string, req FormationDeleteRequest, opts WriteOptions) (*FormationDeleteResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, ErrNotFound
	}
	if opts.ExpectedETag == "" || opts.ExpectedRev == 0 {
		return nil, ErrPreconditionRequired
	}
	path := s.BoardPath(slug)
	deletedIDs := map[string]bool{req.ID: true}
	var result *FormationDeleteResult
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		current, err := parseBoard(raw)
		if err != nil {
			return err
		}
		if opts.ExpectedETag != current.ETag || opts.ExpectedRev != current.Rev {
			return ErrConflict
		}

		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		if req.UpdatedBy != "" {
			doc.setScalar("updatedBy", renderString(req.UpdatedBy))
		}
		nextRev := current.Rev + 1
		doc.setScalar("rev", renderInt(nextRev))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))

		nextRaw, deleted := deleteFormationBlock(doc.bytes(), req.ID)
		if !deleted {
			return ErrNotFound
		}
		nextRaw = deleteConnectionsTouchingNodes(nextRaw, deletedIDs)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		board, err := parseBoard(nextRaw)
		if err != nil {
			return err
		}
		layout, err := s.deleteLayoutNodes(slug, board.ID, board.Rev, deletedIDs)
		if err != nil {
			return err
		}
		result = &FormationDeleteResult{
			Board:       board,
			Layout:      layout,
			FormationID: req.ID,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) DeleteGate(slug string, req GateDeleteRequest, opts WriteOptions) (*GateDeleteResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, ErrNotFound
	}
	if opts.ExpectedETag == "" || opts.ExpectedRev == 0 {
		return nil, ErrPreconditionRequired
	}
	path := s.BoardPath(slug)
	deletedIDs := map[string]bool{req.ID: true}
	var result *GateDeleteResult
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		current, err := parseBoard(raw)
		if err != nil {
			return err
		}
		if opts.ExpectedETag != current.ETag || opts.ExpectedRev != current.Rev {
			return ErrConflict
		}

		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		if req.UpdatedBy != "" {
			doc.setScalar("updatedBy", renderString(req.UpdatedBy))
		}
		nextRev := current.Rev + 1
		doc.setScalar("rev", renderInt(nextRev))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))

		nextRaw, deleted := deleteGateBlock(doc.bytes(), req.ID)
		if !deleted {
			return ErrNotFound
		}
		nextRaw = deleteConnectionsTouchingNodes(nextRaw, deletedIDs)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		board, err := parseBoard(nextRaw)
		if err != nil {
			return err
		}
		layout, err := s.deleteLayoutNodes(slug, board.ID, board.Rev, deletedIDs)
		if err != nil {
			return err
		}
		result = &GateDeleteResult{
			Board:  board,
			Layout: layout,
			GateID: req.ID,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) DeleteMission(slug string, req MissionDeleteRequest, opts WriteOptions) (*MissionDeleteResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, ErrNotFound
	}
	if opts.ExpectedETag == "" || opts.ExpectedRev == 0 {
		return nil, ErrPreconditionRequired
	}
	path := s.BoardPath(slug)
	deletedIDs := map[string]bool{req.ID: true}
	var result *MissionDeleteResult
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		current, err := parseBoard(raw)
		if err != nil {
			return err
		}
		if opts.ExpectedETag != current.ETag || opts.ExpectedRev != current.Rev {
			return ErrConflict
		}

		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		if req.UpdatedBy != "" {
			doc.setScalar("updatedBy", renderString(req.UpdatedBy))
		}
		nextRev := current.Rev + 1
		doc.setScalar("rev", renderInt(nextRev))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))

		nextRaw, deleted := deleteMissionBlock(doc.bytes(), req.ID)
		if !deleted {
			return ErrNotFound
		}
		nextRaw = deleteConnectionsTouchingNodes(nextRaw, deletedIDs)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		board, err := parseBoard(nextRaw)
		if err != nil {
			return err
		}
		layout, err := s.deleteLayoutNodes(slug, board.ID, board.Rev, deletedIDs)
		if err != nil {
			return err
		}
		result = &MissionDeleteResult{
			Board:     board,
			Layout:    layout,
			MissionID: req.ID,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) AssignFormationSlot(slug string, req FormationSlotAssignmentRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.FormationID == "" || req.SlotID == "" {
		return nil, ErrNotFound
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		lines := splitLines(raw)
		formationStart, formationEnd, ok := findFormationBlockByID(lines, req.FormationID)
		if !ok {
			return nil, ErrNotFound
		}
		slotStart, slotEnd, ok := findFormationSlotBlock(lines, formationStart, formationEnd, req.SlotID)
		if !ok {
			return nil, ErrNotFound
		}
		if req.AgentID == "" {
			lines = removeScalarInLineRange(lines, slotStart+1, slotEnd, "agentId")
			lines = removeScalarInLineRange(lines, slotStart+1, slotEnd, "harness")
			return renderTOMLLines(lines), nil
		}
		lines = setScalarInLineRange(lines, slotStart+1, slotEnd, "agentId", renderString(req.AgentID))
		if req.Harness != "" {
			lines = setScalarInLineRange(lines, slotStart+1, slotEnd, "harness", renderString(req.Harness))
		} else {
			lines = removeScalarInLineRange(lines, slotStart+1, slotEnd, "harness")
		}
		return renderTOMLLines(lines), nil
	})
}

func (s *Store) SetFormationController(slug string, req FormationControllerRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.FormationID == "" || req.SlotID == "" {
		return nil, ErrNotFound
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		lines := splitLines(raw)
		formationStart, formationEnd, ok := findFormationBlockByID(lines, req.FormationID)
		if !ok {
			return nil, ErrNotFound
		}
		if formationHeaderScalar(lines, formationStart, formationEnd, "type") != FormationTypeOrchestrated {
			return nil, fmt.Errorf("%w: controller role is only valid for orchestrated formations", ErrInvalidSlug)
		}
		found := false
		for i := formationStart + 1; i < formationEnd; i++ {
			section, ok := tomlSectionName(lines[i].body)
			if !ok || section != "formation.slot" {
				continue
			}
			end := tomlBlockEnd(lines, i)
			slotID := scalarInBlock(lines, i+1, end, "id")
			if slotID == req.SlotID {
				found = true
			}
			lines = setScalarInLineRange(lines, i+1, end, "controller", strconv.FormatBool(slotID == req.SlotID))
			i = end - 1
		}
		if !found {
			return nil, ErrNotFound
		}
		return renderTOMLLines(lines), nil
	})
}

func (s *Store) SetFormationBrief(slug string, req FormationBriefRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.FormationID == "" {
		return nil, ErrNotFound
	}
	if req.BeadID != "" && !isSafeBeadsIssueID(req.BeadID) {
		return nil, fmt.Errorf("%w: beadId must be a safe Beads issue id", ErrInvalidSlug)
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		lines := splitLines(raw)
		formationStart, formationEnd, ok := findFormationBlockByID(lines, req.FormationID)
		if !ok {
			return nil, ErrNotFound
		}
		briefStart, briefEnd, ok := findFormationSection(lines, formationStart, formationEnd, "formation.brief")
		if !ok {
			insertAt := formationHeaderEnd(lines, formationStart, formationEnd)
			lines = insertTomLLines(lines, insertAt, renderBriefSection(req))
			return renderTOMLLines(lines), nil
		}
		lines = setScalarInLineRange(lines, briefStart+1, briefEnd, "goal", renderString(req.Goal))
		lines = setScalarInLineRange(lines, briefStart+1, briefEnd, "beadId", renderString(req.BeadID))
		lines = setScalarInLineRange(lines, briefStart+1, briefEnd, "files", renderStringArray(req.Files))
		lines = setScalarInLineRange(lines, briefStart+1, briefEnd, "links", renderStringArray(req.Links))
		return renderTOMLLines(lines), nil
	})
}

func (s *Store) ClearFormationBrief(slug string, req FormationBriefClearRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.FormationID == "" {
		return nil, ErrNotFound
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		lines := splitLines(raw)
		formationStart, formationEnd, ok := findFormationBlockByID(lines, req.FormationID)
		if !ok {
			return nil, ErrNotFound
		}
		briefStart, briefEnd, ok := findFormationSection(lines, formationStart, formationEnd, "formation.brief")
		if !ok {
			return nil, ErrNotFound
		}
		lines = append(lines[:briefStart], lines[briefEnd:]...)
		return renderTOMLLines(lines), nil
	})
}

func (s *Store) SetFormationVerification(slug string, req FormationVerificationRequest, opts WriteOptions) (*BoardDocument, error) {
	return nil, fmt.Errorf(
		"%w: formation %q cannot author retired inline verification; create and wire an explicit Gate instead",
		ErrLegacyInlineVerificationRequiresMigration,
		req.FormationID,
	)
}

func (s *Store) RemoveFormationVerification(slug string, req FormationVerificationRemovalRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.FormationID == "" {
		return nil, ErrNotFound
	}
	if req.ReplacementGateID == "" {
		return nil, fmt.Errorf("%w: replacement Gate is required before removing inline verification", ErrLegacyInlineVerificationRequiresMigration)
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, board *BoardDocument) ([]byte, error) {
		formation, formationOK := findFormation(board.Formations, req.FormationID)
		_, gateOK := findGate(board.Gates, req.ReplacementGateID)
		if !formationOK || !gateOK || !formationOutputWiresToGate(board.Connections, formation, req.ReplacementGateID) {
			return nil, fmt.Errorf(
				"%w: replacement Gate %q must exist and be wired from formation %q before removing inline verification",
				ErrLegacyInlineVerificationRequiresMigration,
				req.ReplacementGateID,
				req.FormationID,
			)
		}
		lines := splitLines(raw)
		formationStart, formationEnd, ok := findFormationBlockByID(lines, req.FormationID)
		if !ok {
			return nil, ErrNotFound
		}
		verificationStart, verificationEnd, ok := findFormationSection(lines, formationStart, formationEnd, "formation.verification")
		if !ok {
			return nil, ErrNotFound
		}
		verificationSections := 0
		for i := formationStart + 1; i < formationEnd; i++ {
			section, sectionOK := tomlSectionName(lines[i].body)
			if sectionOK && section == "formation.verification" {
				verificationSections++
			}
		}
		if verificationSections != 1 {
			return nil, fmt.Errorf(
				"%w: formation %q has %d inline verification sections; repair the source before migration",
				ErrLegacyInlineVerificationRequiresMigration,
				req.FormationID,
				verificationSections,
			)
		}
		lines = append(lines[:verificationStart], lines[verificationEnd:]...)
		nextRaw := renderTOMLLines(lines)
		nextBoard, err := parseBoard(nextRaw)
		if err != nil {
			return nil, err
		}
		nextFormation, nextFormationOK := findFormation(nextBoard.Formations, req.FormationID)
		if !nextFormationOK || nextFormation.Verification != nil {
			return nil, fmt.Errorf(
				"%w: formation %q still contains inline verification after removal",
				ErrLegacyInlineVerificationRequiresMigration,
				req.FormationID,
			)
		}
		return nextRaw, nil
	})
}

func formationOutputWiresToGate(connections []BoardConnection, formation FormationNode, gateID string) bool {
	for _, output := range formation.Outputs {
		from := formation.ID + ":" + output.ID
		to := gateID + ":in"
		for _, connection := range connections {
			if connection.From == from && connection.To == to {
				return true
			}
		}
	}
	return false
}

func (s *Store) CreateGate(slug string, req GateCreateRequest, opts WriteOptions) (*GateCreateResult, error) {
	if err := rejectLegacyScriptGateWrite(req.LegacyCommandFieldsPresent, req.Command, req.CommandArgv, req.CommandCWD, req.CommandShell); err != nil {
		return nil, err
	}
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if opts.ExpectedETag == "" || opts.ExpectedRev == 0 {
		return nil, ErrPreconditionRequired
	}
	kinds := req.Kinds
	if len(kinds) == 0 {
		kinds = []string{"code"}
	}
	title := req.Title
	if title == "" {
		title = "Review gate"
	}
	gate := GateNode{
		ID:        newPrefixedID("gate"),
		Title:     title,
		Kinds:     kinds,
		Criterion: req.Criterion,
	}

	path := s.BoardPath(slug)
	var result *GateCreateResult
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		current, err := parseBoard(raw)
		if err != nil {
			return err
		}
		if opts.ExpectedETag != current.ETag || opts.ExpectedRev != current.Rev {
			return ErrConflict
		}

		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		if req.UpdatedBy != "" {
			doc.setScalar("updatedBy", renderString(req.UpdatedBy))
		}
		doc.setScalar("rev", renderInt(current.Rev+1))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))

		nextRaw := appendGateBlock(doc.bytes(), gate)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		board, err := parseBoard(nextRaw)
		if err != nil {
			return err
		}
		layout, err := s.upsertLayoutNode(slug, board.ID, board.Rev, LayoutNode{ID: gate.ID, X: req.X, Y: req.Y})
		if err != nil {
			return err
		}
		result = &GateCreateResult{
			Board:  board,
			Layout: layout,
			Gate:   gate,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) UpdateGate(slug string, req GateUpdateRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.GateID == "" {
		return nil, ErrNotFound
	}
	if err := rejectLegacyScriptGateWrite(req.LegacyCommandFieldsPresent, req.Command, req.CommandArgv, req.CommandCWD, req.CommandShell); err != nil {
		return nil, err
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		lines := splitLines(raw)
		gateStart, gateEnd, ok := findGateBlockByID(lines, req.GateID)
		if !ok {
			return nil, ErrNotFound
		}
		if req.Title != "" {
			lines = setScalarInLineRange(lines, gateStart+1, gateEnd, "title", renderString(req.Title))
		}
		if len(req.Kinds) > 0 {
			lines = setScalarInLineRange(lines, gateStart+1, gateEnd, "kinds", renderStringArray(req.Kinds))
		}
		if req.Criterion != "" {
			lines = setScalarInLineRange(lines, gateStart+1, gateEnd, "criterion", renderString(req.Criterion))
		}
		return renderTOMLLines(lines), nil
	})
}

func (s *Store) SetGateJudgeChain(slug string, req GateJudgeRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.GateID == "" || len(req.Chain) == 0 {
		return nil, ErrNotFound
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		lines := splitLines(raw)
		gateStart, gateEnd, ok := findGateBlockByID(lines, req.GateID)
		if !ok {
			return nil, ErrNotFound
		}
		lines = setGateFormationKind(lines, gateStart, gateEnd, true)
		raw = renderTOMLLines(lines)
		raw = deleteGateJudgeConnections(raw, req.GateID)
		connections, err := judgeChainConnections(raw, req)
		if err != nil {
			return nil, err
		}
		for _, connection := range connections {
			raw = appendConnectionBlock(raw, connection)
		}
		return raw, nil
	})
}

func (s *Store) DetachGateJudge(slug string, req GateJudgeRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.GateID == "" {
		return nil, ErrNotFound
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		lines := splitLines(raw)
		gateStart, gateEnd, ok := findGateBlockByID(lines, req.GateID)
		if !ok {
			return nil, ErrNotFound
		}
		lines = setGateFormationKind(lines, gateStart, gateEnd, false)
		raw = renderTOMLLines(lines)
		return deleteGateJudgeConnections(raw, req.GateID), nil
	})
}

func (s *Store) CreateMission(slug string, req MissionCreateRequest, opts WriteOptions) (*MissionCreateResult, error) {
	if !isSafeBeadsIssueID(req.BeadID) {
		return nil, fmt.Errorf("%w: mission beadId must be a safe Beads issue id", ErrInvalidSlug)
	}
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if opts.ExpectedETag == "" || opts.ExpectedRev == 0 {
		return nil, ErrPreconditionRequired
	}
	title := req.Title
	if title == "" {
		title = "Mission"
	}
	mission := MissionNode{
		ID:     newPrefixedID("mis"),
		Title:  title,
		Goal:   req.Goal,
		BeadID: req.BeadID,
	}

	path := s.BoardPath(slug)
	var result *MissionCreateResult
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		current, err := parseBoard(raw)
		if err != nil {
			return err
		}
		if opts.ExpectedETag != current.ETag || opts.ExpectedRev != current.Rev {
			return ErrConflict
		}

		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		if req.UpdatedBy != "" {
			doc.setScalar("updatedBy", renderString(req.UpdatedBy))
		}
		doc.setScalar("rev", renderInt(current.Rev+1))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))

		nextRaw := appendMissionBlock(doc.bytes(), mission)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		board, err := parseBoard(nextRaw)
		if err != nil {
			return err
		}
		layout, err := s.upsertLayoutNode(slug, board.ID, board.Rev, LayoutNode{ID: mission.ID, X: req.X, Y: req.Y})
		if err != nil {
			return err
		}
		result = &MissionCreateResult{
			Board:   board,
			Layout:  layout,
			Mission: mission,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) AddFormationPort(slug string, req FormationPortRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.FormationID == "" {
		return nil, ErrNotFound
	}
	if req.Direction != FormationPortInput && req.Direction != FormationPortOutput {
		return nil, fmt.Errorf("%w: formation port direction must be input or output", ErrInvalidSlug)
	}
	label := req.Label
	if label == "" {
		if req.Direction == FormationPortInput {
			label = "Input"
		} else {
			label = "Output"
		}
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		lines := splitLines(raw)
		formationStart, formationEnd, ok := findFormationBlockByID(lines, req.FormationID)
		if !ok {
			return nil, ErrNotFound
		}
		section := "formation.input"
		if req.Direction == FormationPortOutput {
			section = "formation.output"
		}
		lines = insertTomLLines(lines, formationEnd, []tomlLine{
			{body: "[[" + section + "]]", newline: "\n"},
			{body: "id = " + renderString(newPrefixedID("port")), newline: "\n"},
			{body: "label = " + renderString(label), newline: "\n"},
		})
		_ = formationStart
		return renderTOMLLines(lines), nil
	})
}

func (s *Store) RemoveFormationPort(slug string, req FormationPortRemovalRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.FormationID == "" || req.PortID == "" {
		return nil, ErrNotFound
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		lines := splitLines(raw)
		formationStart, formationEnd, ok := findFormationBlockByID(lines, req.FormationID)
		if !ok {
			return nil, ErrNotFound
		}
		portStart, portEnd, ok := findFormationPortBlock(lines, formationStart, formationEnd, req.PortID)
		if !ok {
			return nil, ErrNotFound
		}
		lines = append(lines[:portStart], lines[portEnd:]...)
		nextRaw := renderTOMLLines(lines)
		return deleteConnectionsTouchingEndpoints(nextRaw, map[string]bool{req.FormationID + ":" + req.PortID: true}), nil
	})
}

func (s *Store) WireFormationPorts(slug string, req FormationWireRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.From == "" || req.To == "" {
		return nil, ErrNotFound
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, current *BoardDocument) ([]byte, error) {
		fromNode, ok := endpointAllowsDirection(raw, req.From, FormationPortOutput)
		if !ok {
			return nil, ErrNotFound
		}
		toNode, ok := endpointAllowsDirection(raw, req.To, FormationPortInput)
		if !ok {
			return nil, ErrNotFound
		}
		if fromNode == toNode {
			return nil, ErrConflict
		}
		for _, connection := range current.Connections {
			if connection.From == req.From && connection.To == req.To {
				return nil, ErrConflict
			}
			if connection.To == req.To {
				return nil, ErrConflict
			}
		}
		return appendConnectionBlock(raw, BoardConnection{
			ID:   newPrefixedID("edge"),
			From: req.From,
			To:   req.To,
		}), nil
	})
}

func (s *Store) UnwireFormationPorts(slug string, req FormationWireRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.From == "" || req.To == "" {
		return nil, ErrNotFound
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, _ *BoardDocument) ([]byte, error) {
		nextRaw, deleted := deleteConnectionByEndpoints(raw, req.From, req.To)
		if !deleted {
			return nil, ErrNotFound
		}
		return nextRaw, nil
	})
}

func (s *Store) RewireFormationTarget(slug string, req FormationRewireRequest, opts WriteOptions) (*BoardDocument, error) {
	if req.From == "" || req.PreviousTo == "" || req.To == "" {
		return nil, ErrNotFound
	}
	return s.updateBoardDefinition(slug, req.UpdatedBy, opts, func(raw []byte, current *BoardDocument) ([]byte, error) {
		fromNode, ok := endpointAllowsDirection(raw, req.From, FormationPortOutput)
		if !ok {
			return nil, ErrNotFound
		}
		toNode, ok := endpointAllowsDirection(raw, req.To, FormationPortInput)
		if !ok {
			return nil, ErrNotFound
		}
		if fromNode == toNode {
			return nil, ErrConflict
		}
		hasOriginal := false
		for _, connection := range current.Connections {
			if connection.From == req.From && connection.To == req.PreviousTo {
				hasOriginal = true
				continue
			}
			if connection.From == req.From && connection.To == req.To {
				return nil, ErrConflict
			}
			if connection.To == req.To {
				return nil, ErrConflict
			}
		}
		if !hasOriginal {
			return nil, ErrNotFound
		}
		nextRaw, deleted := deleteConnectionByEndpoints(raw, req.From, req.PreviousTo)
		if !deleted {
			return nil, ErrNotFound
		}
		return appendConnectionBlock(nextRaw, BoardConnection{
			ID:   newPrefixedID("edge"),
			From: req.From,
			To:   req.To,
		}), nil
	})
}

func (s *Store) UpdateLayoutNodes(slug string, nodes []LayoutNode, opts WriteOptions) (*LayoutDocument, error) {
	return s.updateLayoutNodes(slug, nodes, nil, opts)
}

func (s *Store) updateLayoutNodes(slug string, nodes []LayoutNode, board *BoardDocument, opts WriteOptions) (*LayoutDocument, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if opts.ExpectedETag == "" {
		return nil, ErrPreconditionRequired
	}
	path := s.LayoutPath(slug)
	var layout *LayoutDocument
	err := withFileLock(path, func() error {
		recreatingMissing := false
		raw, err := os.ReadFile(path)
		switch {
		case err == nil:
		case os.IsNotExist(err) && opts.ExpectedETag == "*":
			if board == nil {
				board, err = s.ReadBoard(slug)
				if err != nil {
					return err
				}
			}
			raw = []byte("schema = 1\nboardId = " + renderString(board.ID) + "\nboardRev = " + renderInt(board.Rev) + "\nupdatedAt = " + renderString(s.now().Format(time.RFC3339)) + "\n")
			recreatingMissing = true
		case os.IsNotExist(err):
			return ErrNotFound
		default:
			return err
		}
		current, err := parseLayout(raw)
		if err != nil {
			return err
		}
		if !recreatingMissing && opts.ExpectedETag != current.ETag {
			return ErrConflict
		}
		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		if board != nil {
			doc.setScalar("boardId", renderString(board.ID))
			doc.setScalar("boardRev", renderInt(board.Rev))
		}
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))
		nextRaw := patchLayoutNodeBlocks(doc.bytes(), nodes)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		layout, err = parseLayout(nextRaw)
		return err
	})
	if err != nil {
		return nil, err
	}
	return layout, nil
}

func (s *Store) UpdateLayoutEdges(slug string, edges []LayoutEdge, opts WriteOptions) (*LayoutDocument, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if opts.ExpectedETag == "" {
		return nil, ErrPreconditionRequired
	}
	path := s.LayoutPath(slug)
	var layout *LayoutDocument
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		current, err := parseLayout(raw)
		if err != nil {
			return err
		}
		if opts.ExpectedETag != current.ETag {
			return ErrConflict
		}
		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))
		nextRaw := patchLayoutEdgeBlocks(doc.bytes(), edges)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		layout, err = parseLayout(nextRaw)
		return err
	})
	if err != nil {
		return nil, err
	}
	return layout, nil
}

func (s *Store) updateBoardDefinition(slug, updatedBy string, opts WriteOptions, mutate func([]byte, *BoardDocument) ([]byte, error)) (*BoardDocument, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if opts.ExpectedETag == "" || opts.ExpectedRev == 0 {
		return nil, ErrPreconditionRequired
	}
	path := s.BoardPath(slug)
	var next *BoardDocument
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		current, err := parseBoard(raw)
		if err != nil {
			return err
		}
		if opts.ExpectedETag != current.ETag || opts.ExpectedRev != current.Rev {
			return ErrConflict
		}

		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		if updatedBy != "" {
			doc.setScalar("updatedBy", renderString(updatedBy))
		}
		doc.setScalar("rev", renderInt(current.Rev+1))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))
		nextRaw, err := mutate(doc.bytes(), current)
		if err != nil {
			return err
		}
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		next, err = parseBoard(nextRaw)
		return err
	})
	if err != nil {
		return nil, err
	}
	return next, nil
}

func newFormationNode(req FormationCreateRequest) (FormationNode, error) {
	if err := validateFormationType(req.Type); err != nil {
		return FormationNode{}, err
	}
	title := req.Title
	if title == "" {
		title = defaultFormationTitle(req.Type)
	}
	return FormationNode{
		ID:    newPrefixedID("fmn"),
		Type:  req.Type,
		Title: title,
		Inputs: []FormationPort{{
			ID:    newPrefixedID("port"),
			Label: "Input",
		}},
		Outputs: []FormationPort{{
			ID:    newPrefixedID("port"),
			Label: "Output",
		}},
		Slots: defaultFormationSlots(req.Type),
	}, nil
}

func validateFormationType(formationType string) error {
	switch formationType {
	case FormationTypeSolo, FormationTypePeer, FormationTypeFlow, FormationTypeOrchestrated:
		return nil
	default:
		return fmt.Errorf("%w: unsupported formation type %q", ErrInvalidSlug, formationType)
	}
}

func defaultFormationTitle(formationType string) string {
	switch formationType {
	case FormationTypeSolo:
		return "Solo task"
	case FormationTypePeer:
		return "Peer huddle"
	case FormationTypeFlow:
		return "New flow"
	case FormationTypeOrchestrated:
		return "Orchestration"
	default:
		return "Formation"
	}
}

func defaultFormationSlots(formationType string) []FormationSlot {
	switch formationType {
	case FormationTypeSolo:
		return []FormationSlot{newFormationSlot("Agent", false)}
	case FormationTypePeer:
		return []FormationSlot{
			newFormationSlot("Peer", false),
			newFormationSlot("Peer", false),
		}
	case FormationTypeFlow:
		return []FormationSlot{
			newFormationSlot("Plan", false),
			newFormationSlot("Execute", false),
			newFormationSlot("Push", false),
		}
	case FormationTypeOrchestrated:
		return []FormationSlot{
			newFormationSlot("Orchestrator", true),
			newFormationSlot("Agent", false),
			newFormationSlot("Agent", false),
		}
	default:
		return nil
	}
}

func newFormationSlot(label string, controller bool) FormationSlot {
	return FormationSlot{
		ID:         newPrefixedID("slot"),
		Label:      label,
		Controller: controller,
	}
}

func appendFormationBlock(raw []byte, formation FormationNode) []byte {
	var b strings.Builder
	b.Write(raw)
	text := string(raw)
	if text != "" && !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	if text != "" {
		b.WriteByte('\n')
	}
	b.WriteString("[[formation]]\n")
	b.WriteString("id = " + renderString(formation.ID) + "\n")
	b.WriteString("type = " + renderString(formation.Type) + "\n")
	b.WriteString("title = " + renderString(formation.Title) + "\n\n")
	for _, input := range formation.Inputs {
		b.WriteString("[[formation.input]]\n")
		b.WriteString("id = " + renderString(input.ID) + "\n")
		b.WriteString("label = " + renderString(input.Label) + "\n\n")
	}
	for _, output := range formation.Outputs {
		b.WriteString("[[formation.output]]\n")
		b.WriteString("id = " + renderString(output.ID) + "\n")
		b.WriteString("label = " + renderString(output.Label) + "\n\n")
	}
	for i, slot := range formation.Slots {
		b.WriteString("[[formation.slot]]\n")
		b.WriteString("id = " + renderString(slot.ID) + "\n")
		b.WriteString("label = " + renderString(slot.Label) + "\n")
		b.WriteString("controller = " + strconv.FormatBool(slot.Controller) + "\n")
		if slot.AgentID != "" {
			b.WriteString("agentId = " + renderString(slot.AgentID) + "\n")
		}
		if slot.Harness != "" {
			b.WriteString("harness = " + renderString(slot.Harness) + "\n")
		}
		if i < len(formation.Slots)-1 {
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

func deleteFormationBlock(raw []byte, formationID string) ([]byte, bool) {
	lines := splitLines(raw)
	for i := 0; i < len(lines); i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "formation" {
			continue
		}
		end := formationBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, end, "id") != formationID {
			continue
		}
		lines = append(lines[:i], lines[end:]...)
		return renderTOMLLines(lines), true
	}
	return raw, false
}

func deleteGateBlock(raw []byte, gateID string) ([]byte, bool) {
	return deleteTopLevelBlockByID(raw, "gate", gateID)
}

func deleteMissionBlock(raw []byte, missionID string) ([]byte, bool) {
	return deleteTopLevelBlockByID(raw, "mission", missionID)
}

func deleteTopLevelBlockByID(raw []byte, sectionName, id string) ([]byte, bool) {
	lines := splitLines(raw)
	for i := 0; i < len(lines); i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != sectionName {
			continue
		}
		end := tomlBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, end, "id") != id {
			continue
		}
		lines = append(lines[:i], lines[end:]...)
		return renderTOMLLines(lines), true
	}
	return raw, false
}

func formationBlockEnd(lines []tomlLine, start int) int {
	for j := start + 1; j < len(lines); j++ {
		section, ok := tomlSectionName(lines[j].body)
		if !ok {
			if isTOMLHeaderLine(lines[j].body) {
				return j
			}
			continue
		}
		if section == "formation" || !strings.HasPrefix(section, "formation.") {
			return j
		}
	}
	return len(lines)
}

func deleteConnectionsTouchingNodes(raw []byte, nodeIDs map[string]bool) []byte {
	lines := splitLines(raw)
	for i := 0; i < len(lines); {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "connection" {
			i++
			continue
		}
		end := tomlBlockEnd(lines, i)
		from := scalarInBlock(lines, i+1, end, "from")
		to := scalarInBlock(lines, i+1, end, "to")
		if nodeIDs[endpointNodeID(from)] || nodeIDs[endpointNodeID(to)] {
			lines = append(lines[:i], lines[end:]...)
			continue
		}
		i = end
	}
	return renderTOMLLines(lines)
}

func deleteConnectionsTouchingEndpoints(raw []byte, endpoints map[string]bool) []byte {
	lines := splitLines(raw)
	for i := 0; i < len(lines); {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "connection" {
			i++
			continue
		}
		end := tomlBlockEnd(lines, i)
		from := scalarInBlock(lines, i+1, end, "from")
		to := scalarInBlock(lines, i+1, end, "to")
		if endpoints[from] || endpoints[to] {
			lines = append(lines[:i], lines[end:]...)
			continue
		}
		i = end
	}
	return renderTOMLLines(lines)
}

func deleteConnectionByEndpoints(raw []byte, from, to string) ([]byte, bool) {
	lines := splitLines(raw)
	for i := 0; i < len(lines); i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "connection" {
			continue
		}
		end := tomlBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, end, "from") == from && scalarInBlock(lines, i+1, end, "to") == to {
			lines = append(lines[:i], lines[end:]...)
			return renderTOMLLines(lines), true
		}
	}
	return raw, false
}

func appendConnectionBlock(raw []byte, connection BoardConnection) []byte {
	var b strings.Builder
	b.Write(raw)
	text := string(raw)
	if text != "" && !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	if text != "" {
		b.WriteByte('\n')
	}
	b.WriteString("[[connection]]\n")
	b.WriteString("id = " + renderString(connection.ID) + "\n")
	b.WriteString("from = " + renderString(connection.From) + "\n")
	b.WriteString("to = " + renderString(connection.To) + "\n")
	return []byte(b.String())
}

func appendGateBlock(raw []byte, gate GateNode) []byte {
	var b strings.Builder
	b.Write(raw)
	text := string(raw)
	if text != "" && !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	if text != "" {
		b.WriteByte('\n')
	}
	b.WriteString("[[gate]]\n")
	b.WriteString("id = " + renderString(gate.ID) + "\n")
	b.WriteString("title = " + renderString(gate.Title) + "\n")
	b.WriteString("kinds = " + renderStringArray(gate.Kinds) + "\n")
	b.WriteString("criterion = " + renderString(gate.Criterion) + "\n")
	return []byte(b.String())
}

func appendMissionBlock(raw []byte, mission MissionNode) []byte {
	var b strings.Builder
	b.Write(raw)
	text := string(raw)
	if text != "" && !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	if text != "" {
		b.WriteByte('\n')
	}
	b.WriteString("[[mission]]\n")
	b.WriteString("id = " + renderString(mission.ID) + "\n")
	b.WriteString("title = " + renderString(mission.Title) + "\n")
	b.WriteString("goal = " + renderString(mission.Goal) + "\n")
	b.WriteString("beadId = " + renderString(mission.BeadID) + "\n")
	return []byte(b.String())
}

func deleteGateJudgeConnections(raw []byte, gateID string) []byte {
	endpoint := gateID + ":judge"
	lines := splitLines(raw)
	for i := 0; i < len(lines); {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "connection" {
			i++
			continue
		}
		end := tomlBlockEnd(lines, i)
		from := scalarInBlock(lines, i+1, end, "from")
		to := scalarInBlock(lines, i+1, end, "to")
		if from == endpoint || to == endpoint {
			lines = append(lines[:i], lines[end:]...)
			continue
		}
		i = end
	}
	return renderTOMLLines(lines)
}

func judgeChainConnections(raw []byte, req GateJudgeRequest) ([]BoardConnection, error) {
	board, err := parseBoard(raw)
	if err != nil {
		return nil, err
	}
	existing := append([]BoardConnection(nil), board.Connections...)
	firstInput, err := firstFormationPortEndpoint(raw, req.Chain[0], FormationPortInput)
	if err != nil {
		return nil, err
	}
	connections := []BoardConnection{}
	addConnection := func(connection BoardConnection) error {
		exists, err := validateConnectionCandidate(existing, connection)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		existing = append(existing, connection)
		connections = append(connections, connection)
		return nil
	}
	if err := addConnection(BoardConnection{
		ID:   newPrefixedID("edge"),
		From: req.GateID + ":judge",
		To:   firstInput,
	}); err != nil {
		return nil, err
	}
	for i := 0; i < len(req.Chain)-1; i++ {
		from, err := firstFormationPortEndpoint(raw, req.Chain[i], FormationPortOutput)
		if err != nil {
			return nil, err
		}
		to, err := firstFormationPortEndpoint(raw, req.Chain[i+1], FormationPortInput)
		if err != nil {
			return nil, err
		}
		if err := addConnection(BoardConnection{ID: newPrefixedID("edge"), From: from, To: to}); err != nil {
			return nil, err
		}
	}
	lastOutput, err := firstFormationPortEndpoint(raw, req.Chain[len(req.Chain)-1], FormationPortOutput)
	if err != nil {
		return nil, err
	}
	if err := addConnection(BoardConnection{
		ID:   newPrefixedID("edge"),
		From: lastOutput,
		To:   req.GateID + ":judge",
	}); err != nil {
		return nil, err
	}
	return connections, nil
}

func validateConnectionCandidate(existing []BoardConnection, candidate BoardConnection) (bool, error) {
	if endpointNodeID(candidate.From) == endpointNodeID(candidate.To) {
		return false, ErrConflict
	}
	for _, connection := range existing {
		if connection.From == candidate.From && connection.To == candidate.To {
			return true, nil
		}
		if connection.To == candidate.To {
			return false, ErrConflict
		}
	}
	return false, nil
}

func firstFormationPortEndpoint(raw []byte, formationID, direction string) (string, error) {
	lines := splitLines(raw)
	formationStart, formationEnd, ok := findFormationBlockByID(lines, formationID)
	if !ok {
		return "", ErrNotFound
	}
	sectionName := "formation.input"
	if direction == FormationPortOutput {
		sectionName = "formation.output"
	}
	for i := formationStart + 1; i < formationEnd; i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != sectionName {
			continue
		}
		end := tomlBlockEnd(lines, i)
		portID := scalarInBlock(lines, i+1, end, "id")
		if portID != "" {
			return formationID + ":" + portID, nil
		}
	}
	return "", ErrNotFound
}

func setGateFormationKind(lines []tomlLine, gateStart, gateEnd int, present bool) []tomlLine {
	kinds := parseStringArray(scalarInBlock(lines, gateStart+1, gateEnd, "kinds"))
	hasFormation := false
	filtered := make([]string, 0, len(kinds)+1)
	for _, kind := range kinds {
		if kind == "formation" {
			hasFormation = true
			if !present {
				continue
			}
		}
		filtered = append(filtered, kind)
	}
	if present && !hasFormation {
		filtered = append(filtered, "formation")
	}
	if len(filtered) == 0 {
		filtered = []string{"code"}
	}
	return setScalarInLineRange(lines, gateStart+1, gateEnd, "kinds", renderStringArray(filtered))
}

func (s *Store) upsertLayoutNode(slug, boardID string, boardRev int, node LayoutNode) (*LayoutDocument, error) {
	path := s.LayoutPath(slug)
	var layout *LayoutDocument
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		switch {
		case err == nil:
		case os.IsNotExist(err):
			raw = []byte("schema = 1\nboardId = " + renderString(boardID) + "\nboardRev = " + renderInt(boardRev) + "\nupdatedAt = " + renderString(s.now().Format(time.RFC3339)) + "\n")
		default:
			return err
		}
		current, err := parseLayout(raw)
		if err != nil {
			return err
		}
		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		doc.setScalar("boardId", renderString(boardID))
		doc.setScalar("boardRev", renderInt(boardRev))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))

		nextRaw := appendLayoutNodeBlock(doc.bytes(), node)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		layout, err = parseLayout(nextRaw)
		return err
	})
	if err != nil {
		return nil, err
	}
	return layout, nil
}

func (s *Store) deleteLayoutNodes(slug, boardID string, boardRev int, nodeIDs map[string]bool) (*LayoutDocument, error) {
	path := s.LayoutPath(slug)
	var layout *LayoutDocument
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		switch {
		case err == nil:
		case os.IsNotExist(err):
			layout = &LayoutDocument{
				Schema:   CurrentSchema,
				BoardID:  boardID,
				BoardRev: boardRev,
				Nodes:    []LayoutNode{},
			}
			return nil
		default:
			return err
		}
		current, err := parseLayout(raw)
		if err != nil {
			return err
		}
		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		doc.setScalar("boardId", renderString(boardID))
		doc.setScalar("boardRev", renderInt(boardRev))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))

		nextRaw := deleteLayoutNodeBlocks(doc.bytes(), nodeIDs)
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		layout, err = parseLayout(nextRaw)
		return err
	})
	if err != nil {
		return nil, err
	}
	return layout, nil
}

func appendLayoutNodeBlock(raw []byte, node LayoutNode) []byte {
	var b strings.Builder
	b.Write(raw)
	text := string(raw)
	if text != "" && !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	if text != "" {
		b.WriteByte('\n')
	}
	b.WriteString("[[node]]\n")
	b.WriteString("id = " + renderString(node.ID) + "\n")
	b.WriteString("x = " + renderInt(node.X) + "\n")
	b.WriteString("y = " + renderInt(node.Y) + "\n")
	return []byte(b.String())
}

func patchLayoutNodeBlocks(raw []byte, patches []LayoutNode) []byte {
	lines := splitLines(raw)
	patched := make(map[string]bool, len(patches))
	byID := make(map[string]LayoutNode, len(patches))
	for _, patch := range patches {
		if patch.ID != "" {
			byID[patch.ID] = patch
		}
	}
	for i := 0; i < len(lines); i++ {
		section, isSection := tomlSectionName(lines[i].body)
		if !isSection || !strings.HasPrefix(strings.TrimSpace(lines[i].body), "[[") || section != "node" {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j].body), "[") {
				end = j
				break
			}
		}
		nodeID := ""
		for j := i + 1; j < end; j++ {
			key, value, ok := tomlKeyValue(lines[j].body)
			if ok && key == "id" {
				nodeID = value
				break
			}
		}
		patch, ok := byID[nodeID]
		if !ok {
			continue
		}
		lines = setScalarInLineRange(lines, i+1, end, "x", renderInt(patch.X))
		lines = setScalarInLineRange(lines, i+1, end, "y", renderInt(patch.Y))
		patched[nodeID] = true
	}
	nextRaw := renderTOMLLines(lines)
	for _, patch := range patches {
		if patch.ID == "" || patched[patch.ID] {
			continue
		}
		nextRaw = appendLayoutNodeBlock(nextRaw, patch)
	}
	return nextRaw
}

func patchLayoutEdgeBlocks(raw []byte, patches []LayoutEdge) []byte {
	lines := splitLines(raw)
	patched := make(map[string]bool, len(patches))
	byID := make(map[string]LayoutEdge, len(patches))
	for _, patch := range patches {
		if patch.ID != "" {
			byID[patch.ID] = patch
		}
	}
	for i := 0; i < len(lines); i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "edge" {
			continue
		}
		end := tomlBlockEnd(lines, i)
		edgeID := scalarInBlock(lines, i+1, end, "id")
		patch, ok := byID[edgeID]
		if !ok {
			continue
		}
		lines = setScalarInLineRange(lines, i+1, end, "lane", renderString(patch.Lane))
		patched[edgeID] = true
	}
	nextRaw := renderTOMLLines(lines)
	for _, patch := range patches {
		if patch.ID == "" || patched[patch.ID] {
			continue
		}
		nextRaw = appendLayoutEdgeBlock(nextRaw, patch)
	}
	return nextRaw
}

func appendLayoutEdgeBlock(raw []byte, edge LayoutEdge) []byte {
	var b strings.Builder
	b.Write(raw)
	text := string(raw)
	if text != "" && !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	if text != "" {
		b.WriteByte('\n')
	}
	b.WriteString("[[edge]]\n")
	b.WriteString("id = " + renderString(edge.ID) + "\n")
	b.WriteString("lane = " + renderString(edge.Lane) + "\n")
	return []byte(b.String())
}

func deleteLayoutNodeBlocks(raw []byte, nodeIDs map[string]bool) []byte {
	lines := splitLines(raw)
	for i := 0; i < len(lines); {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "node" {
			i++
			continue
		}
		end := tomlBlockEnd(lines, i)
		nodeID := scalarInBlock(lines, i+1, end, "id")
		if nodeIDs[nodeID] {
			lines = append(lines[:i], lines[end:]...)
			continue
		}
		i = end
	}
	return renderTOMLLines(lines)
}

func tomlBlockEnd(lines []tomlLine, start int) int {
	for j := start + 1; j < len(lines); j++ {
		if _, ok := tomlSectionName(lines[j].body); ok || isTOMLHeaderLine(lines[j].body) {
			return j
		}
	}
	return len(lines)
}

func scalarInBlock(lines []tomlLine, start, end int, key string) string {
	for i := start; i < end && i < len(lines); i++ {
		if _, ok := tomlSectionName(lines[i].body); ok || isTOMLHeaderLine(lines[i].body) {
			break
		}
		fieldKey, value, ok := tomlKeyValue(lines[i].body)
		if ok && fieldKey == key {
			return value
		}
	}
	return ""
}

func tomlSectionName(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	openWidth := 1
	closeWidth := 1
	if strings.HasPrefix(trimmed, "[[") {
		openWidth = 2
		closeWidth = 2
	} else if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	end := tomlHeaderEnd(trimmed, openWidth, closeWidth)
	if end < 0 {
		return "", false
	}
	tail := strings.TrimSpace(trimmed[end+closeWidth:])
	if tail != "" && !strings.HasPrefix(tail, "#") {
		return "", false
	}
	path, ok := parseTOMLKeyPath(trimmed[openWidth:end])
	if !ok {
		return "", false
	}
	return canonicalTOMLKeyPath(path), true
}

func tomlHeaderEnd(line string, openWidth, closeWidth int) int {
	inBasicString := false
	inLiteralString := false
	escaped := false
	for i := openWidth; i < len(line); i++ {
		ch := line[i]
		switch {
		case escaped:
			escaped = false
		case inBasicString && ch == '\\':
			escaped = true
		case !inLiteralString && ch == '"':
			inBasicString = !inBasicString
		case !inBasicString && ch == '\'':
			inLiteralString = !inLiteralString
		case !inBasicString && !inLiteralString && ch == ']':
			if closeWidth == 1 || (i+1 < len(line) && line[i+1] == ']') {
				return i
			}
		}
	}
	return -1
}

func parseTOMLKeyPath(raw string) ([]string, bool) {
	var path []string
	for i := 0; ; {
		for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
			i++
		}
		if i >= len(raw) {
			return nil, false
		}
		var segment string
		switch raw[i] {
		case '"':
			start := i
			i++
			closed := false
			escaped := false
			for i < len(raw) {
				ch := raw[i]
				i++
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					closed = true
					break
				}
			}
			if !closed {
				return nil, false
			}
			unquoted, err := strconv.Unquote(raw[start:i])
			if err != nil {
				return nil, false
			}
			segment = unquoted
		case '\'':
			i++
			start := i
			for i < len(raw) && raw[i] != '\'' {
				i++
			}
			if i >= len(raw) {
				return nil, false
			}
			segment = raw[start:i]
			i++
		default:
			start := i
			for i < len(raw) && isBareTOMLKeyByte(raw[i]) {
				i++
			}
			if start == i {
				return nil, false
			}
			segment = raw[start:i]
		}
		path = append(path, segment)
		for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
			i++
		}
		if i == len(raw) {
			return path, true
		}
		if raw[i] != '.' {
			return nil, false
		}
		i++
	}
}

func canonicalTOMLKeyPath(path []string) string {
	segments := make([]string, len(path))
	for i, segment := range path {
		if segment != "" && isBareTOMLKey(segment) {
			segments[i] = segment
		} else {
			segments[i] = strconv.Quote(segment)
		}
	}
	return strings.Join(segments, ".")
}

func isBareTOMLKey(key string) bool {
	for i := 0; i < len(key); i++ {
		if !isBareTOMLKeyByte(key[i]) {
			return false
		}
	}
	return true
}

func isBareTOMLKeyByte(ch byte) bool {
	return ch >= 'a' && ch <= 'z' ||
		ch >= 'A' && ch <= 'Z' ||
		ch >= '0' && ch <= '9' ||
		ch == '_' ||
		ch == '-'
}

func tomlAssignmentIndex(line string) int {
	inBasicString := false
	inLiteralString := false
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case escaped:
			escaped = false
		case inBasicString && ch == '\\':
			escaped = true
		case !inLiteralString && ch == '"':
			inBasicString = !inBasicString
		case !inBasicString && ch == '\'':
			inLiteralString = !inLiteralString
		case !inBasicString && !inLiteralString && ch == '#':
			return -1
		case !inBasicString && !inLiteralString && ch == '=':
			return i
		}
	}
	return -1
}

func isTOMLHeaderLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "[")
}

func endpointNodeID(endpoint string) string {
	return strings.SplitN(endpoint, ":", 2)[0]
}

func findFormationBlockByID(lines []tomlLine, formationID string) (int, int, bool) {
	for i := 0; i < len(lines); i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "formation" {
			continue
		}
		end := formationBlockEnd(lines, i)
		if formationHeaderScalar(lines, i, end, "id") == formationID {
			return i, end, true
		}
	}
	return 0, 0, false
}

func findGateBlockByID(lines []tomlLine, gateID string) (int, int, bool) {
	for i := 0; i < len(lines); i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "gate" {
			continue
		}
		end := tomlBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, end, "id") == gateID {
			return i, end, true
		}
	}
	return 0, 0, false
}

func findMissionBlockByID(lines []tomlLine, missionID string) (int, int, bool) {
	for i := 0; i < len(lines); i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "mission" {
			continue
		}
		end := tomlBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, end, "id") == missionID {
			return i, end, true
		}
	}
	return 0, 0, false
}

func formationHeaderScalar(lines []tomlLine, start, end int, key string) string {
	for i := start + 1; i < end && i < len(lines); i++ {
		if _, ok := tomlSectionName(lines[i].body); ok || isTOMLHeaderLine(lines[i].body) {
			break
		}
		fieldKey, value, ok := tomlKeyValue(lines[i].body)
		if ok && fieldKey == key {
			return value
		}
	}
	return ""
}

func formationHeaderEnd(lines []tomlLine, start, end int) int {
	for i := start + 1; i < end && i < len(lines); i++ {
		if _, ok := tomlSectionName(lines[i].body); ok || isTOMLHeaderLine(lines[i].body) {
			return i
		}
	}
	return end
}

func findFormationSection(lines []tomlLine, formationStart, formationEnd int, sectionName string) (int, int, bool) {
	for i := formationStart + 1; i < formationEnd; i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != sectionName {
			continue
		}
		end := tomlBlockEnd(lines, i)
		if end > formationEnd {
			end = formationEnd
		}
		return i, end, true
	}
	return 0, 0, false
}

func findFormationSlotBlock(lines []tomlLine, formationStart, formationEnd int, slotID string) (int, int, bool) {
	for i := formationStart + 1; i < formationEnd; i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || section != "formation.slot" {
			continue
		}
		end := tomlBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, end, "id") == slotID {
			return i, end, true
		}
		i = end - 1
	}
	return 0, 0, false
}

func findFormationPortBlock(lines []tomlLine, formationStart, formationEnd int, portID string) (int, int, bool) {
	for i := formationStart + 1; i < formationEnd; i++ {
		section, ok := tomlSectionName(lines[i].body)
		if !ok || (section != "formation.input" && section != "formation.output") {
			continue
		}
		end := tomlBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, end, "id") == portID {
			return i, end, true
		}
		i = end - 1
	}
	return 0, 0, false
}

func endpointAllowsDirection(raw []byte, endpoint, direction string) (string, bool) {
	nodeID, portID, ok := splitEndpoint(endpoint)
	if !ok {
		return "", false
	}
	lines := splitLines(raw)
	if formationStart, formationEnd, ok := findFormationBlockByID(lines, nodeID); ok {
		portStart, _, ok := findFormationPortBlock(lines, formationStart, formationEnd, portID)
		if !ok {
			return "", false
		}
		section, ok := tomlSectionName(lines[portStart].body)
		if !ok {
			return "", false
		}
		if section == "formation.input" && direction == FormationPortInput {
			return nodeID, true
		}
		if section == "formation.output" && direction == FormationPortOutput {
			return nodeID, true
		}
		return "", false
	}
	if _, _, ok := findGateBlockByID(lines, nodeID); ok {
		switch portID {
		case "in":
			return nodeID, direction == FormationPortInput
		case "pass", "fail":
			return nodeID, direction == FormationPortOutput
		case "judge":
			return nodeID, true
		default:
			return "", false
		}
	}
	if _, _, ok := findMissionBlockByID(lines, nodeID); ok {
		return nodeID, portID == "out" && direction == FormationPortOutput
	}
	return "", false
}

func splitEndpoint(endpoint string) (string, string, bool) {
	parts := strings.SplitN(endpoint, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func insertTomLLines(lines []tomlLine, index int, body []tomlLine) []tomlLine {
	if index > len(lines) {
		index = len(lines)
	}
	next := make([]tomlLine, 0, len(lines)+len(body))
	next = append(next, lines[:index]...)
	next = append(next, body...)
	next = append(next, lines[index:]...)
	return next
}

func renderBriefSection(req FormationBriefRequest) []tomlLine {
	lines := []tomlLine{
		{body: "[formation.brief]", newline: "\n"},
		{body: "goal = " + renderString(req.Goal), newline: "\n"},
		{body: "beadId = " + renderString(req.BeadID), newline: "\n"},
		{body: "files = " + renderStringArray(req.Files), newline: "\n"},
		{body: "links = " + renderStringArray(req.Links), newline: "\n"},
	}
	return lines
}

func setScalarInLineRange(lines []tomlLine, start, end int, key, renderedValue string) []tomlLine {
	for i := start; i < end && i < len(lines); i++ {
		fieldKey, _, ok := tomlKeyValue(lines[i].body)
		if ok && fieldKey == key {
			lines[i].body = replaceScalarValue(lines[i].body, renderedValue)
			return lines
		}
	}
	newLine := tomlLine{body: key + " = " + renderedValue, newline: "\n"}
	if end > len(lines) {
		end = len(lines)
	}
	lines = append(lines, tomlLine{})
	copy(lines[end+1:], lines[end:])
	lines[end] = newLine
	return lines
}

func removeScalarInLineRange(lines []tomlLine, start, end int, key string) []tomlLine {
	for i := start; i < end && i < len(lines); i++ {
		fieldKey, _, ok := tomlKeyValue(lines[i].body)
		if ok && fieldKey == key {
			return append(lines[:i], lines[i+1:]...)
		}
	}
	return lines
}

func renderTOMLLines(lines []tomlLine) []byte {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.body)
		b.WriteString(line.newline)
	}
	return []byte(b.String())
}

func parseFormationNodes(raw []byte) []FormationNode {
	var formations []FormationNode
	var current *FormationNode
	var active string
	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		section, isSection := tomlSectionName(line.body)
		isArraySection := strings.HasPrefix(trimmed, "[[")
		switch {
		case isSection && isArraySection && section == "formation":
			formations = append(formations, FormationNode{})
			current = &formations[len(formations)-1]
			active = "formation"
			continue
		case isSection && isArraySection && section == "formation.input":
			if current != nil {
				current.Inputs = append(current.Inputs, FormationPort{})
				active = "input"
			}
			continue
		case isSection && isArraySection && section == "formation.output":
			if current != nil {
				current.Outputs = append(current.Outputs, FormationPort{})
				active = "output"
			}
			continue
		case isSection && isArraySection && section == "formation.slot":
			if current != nil {
				current.Slots = append(current.Slots, FormationSlot{})
				active = "slot"
			}
			continue
		case isSection && !isArraySection && section == "formation.brief":
			if current != nil {
				current.Brief = &FormationBrief{}
				active = "brief"
			}
			continue
		case isSection && !isArraySection && section == "formation.verification":
			if current != nil {
				current.Verification = &FormationVerification{}
				active = "verification"
			}
			continue
		case isSection:
			active = ""
			continue
		case isTOMLHeaderLine(line.body):
			active = ""
			continue
		}
		if current == nil {
			continue
		}
		key, value, ok := tomlKeyValue(line.body)
		if !ok {
			continue
		}
		switch active {
		case "formation":
			switch key {
			case "id":
				current.ID = value
			case "type":
				current.Type = value
			case "title":
				current.Title = value
			}
		case "input":
			port := &current.Inputs[len(current.Inputs)-1]
			switch key {
			case "id":
				port.ID = value
			case "label":
				port.Label = value
			}
		case "output":
			port := &current.Outputs[len(current.Outputs)-1]
			switch key {
			case "id":
				port.ID = value
			case "label":
				port.Label = value
			}
		case "slot":
			slot := &current.Slots[len(current.Slots)-1]
			switch key {
			case "id":
				slot.ID = value
			case "label":
				slot.Label = value
			case "agentId":
				slot.AgentID = value
			case "harness":
				slot.Harness = value
			case "controller":
				slot.Controller, _ = strconv.ParseBool(value)
			}
		case "brief":
			switch key {
			case "goal":
				current.Brief.Goal = value
			case "beadId":
				current.Brief.BeadID = value
			case "files":
				current.Brief.Files = parseStringArray(value)
			case "links":
				current.Brief.Links = parseStringArray(value)
			}
		case "verification":
			switch key {
			case "id":
				current.Verification.ID = value
			case "kinds":
				current.Verification.Kinds = parseStringArray(value)
			case "criterion":
				current.Verification.Criterion = value
			case "onFail":
				current.Verification.OnFail = value
			}
		}
	}
	return formations
}

func parseBoardConnections(raw []byte) []BoardConnection {
	var connections []BoardConnection
	var current *BoardConnection
	active := false
	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		section, isSection := tomlSectionName(line.body)
		isArraySection := strings.HasPrefix(trimmed, "[[")
		switch {
		case isSection && isArraySection && section == "connection":
			connections = append(connections, BoardConnection{})
			current = &connections[len(connections)-1]
			active = true
			continue
		case isTOMLHeaderLine(line.body):
			active = false
			continue
		}
		if !active || current == nil {
			continue
		}
		key, value, ok := tomlKeyValue(line.body)
		if !ok {
			continue
		}
		switch key {
		case "id":
			current.ID = value
		case "from":
			current.From = value
		case "to":
			current.To = value
		}
	}
	return connections
}

func parseGateNodes(raw []byte) []GateNode {
	var gates []GateNode
	var current *GateNode
	active := false
	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		section, isSection := tomlSectionName(line.body)
		isArraySection := strings.HasPrefix(trimmed, "[[")
		switch {
		case isSection && isArraySection && section == "gate":
			gates = append(gates, GateNode{legacyCommandFields: map[string]int{}})
			current = &gates[len(gates)-1]
			active = true
			continue
		case isTOMLHeaderLine(line.body):
			active = false
			continue
		}
		if !active || current == nil {
			continue
		}
		key, value, ok := tomlKeyValue(line.body)
		if !ok {
			continue
		}
		switch key {
		case "id":
			current.ID = value
		case "title":
			current.Title = value
		case "kinds":
			current.Kinds = parseStringArray(value)
		case "criterion":
			current.Criterion = value
		case "command":
			current.legacyCommandFields[key]++
			current.Command = value
		case "commandArgv":
			current.legacyCommandFields[key]++
			current.CommandArgv = parseStringArray(value)
		case "commandCwd":
			current.legacyCommandFields[key]++
			current.CommandCWD = value
		case "commandShell":
			current.legacyCommandFields[key]++
			current.CommandShell = value
		}
	}
	return gates
}

func parseMissionNodes(raw []byte) []MissionNode {
	var missions []MissionNode
	var current *MissionNode
	active := false
	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		section, isSection := tomlSectionName(line.body)
		isArraySection := strings.HasPrefix(trimmed, "[[")
		switch {
		case isSection && isArraySection && section == "mission":
			missions = append(missions, MissionNode{})
			current = &missions[len(missions)-1]
			active = true
			continue
		case isTOMLHeaderLine(line.body):
			active = false
			continue
		}
		if !active || current == nil {
			continue
		}
		key, value, ok := tomlKeyValue(line.body)
		if !ok {
			continue
		}
		switch key {
		case "id":
			current.ID = value
		case "title":
			current.Title = value
		case "goal":
			current.Goal = value
		case "beadId":
			current.BeadID = value
		}
	}
	return missions
}

func parseLayoutNodes(raw []byte) []LayoutNode {
	var nodes []LayoutNode
	var current *LayoutNode
	active := false
	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		section, isSection := tomlSectionName(line.body)
		isArraySection := strings.HasPrefix(trimmed, "[[")
		switch {
		case isSection && isArraySection && section == "node":
			nodes = append(nodes, LayoutNode{})
			current = &nodes[len(nodes)-1]
			active = true
			continue
		case isTOMLHeaderLine(line.body):
			active = false
			continue
		}
		if !active || current == nil {
			continue
		}
		key, value, ok := tomlKeyValue(line.body)
		if !ok {
			continue
		}
		switch key {
		case "id":
			current.ID = value
		case "x":
			current.X, _ = strconv.Atoi(value)
		case "y":
			current.Y, _ = strconv.Atoi(value)
		}
	}
	return nodes
}

func parseLayoutEdges(raw []byte) []LayoutEdge {
	var edges []LayoutEdge
	var current *LayoutEdge
	active := false
	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		section, isSection := tomlSectionName(line.body)
		isArraySection := strings.HasPrefix(trimmed, "[[")
		switch {
		case isSection && isArraySection && section == "edge":
			edges = append(edges, LayoutEdge{})
			current = &edges[len(edges)-1]
			active = true
			continue
		case isTOMLHeaderLine(line.body):
			active = false
			continue
		}
		if !active || current == nil {
			continue
		}
		key, value, ok := tomlKeyValue(line.body)
		if !ok {
			continue
		}
		switch key {
		case "id":
			current.ID = value
		case "lane":
			current.Lane = value
		}
	}
	return edges
}

func tomlKeyValue(line string) (string, string, bool) {
	eq := tomlAssignmentIndex(line)
	if eq < 0 {
		return "", "", false
	}
	path, ok := parseTOMLKeyPath(line[:eq])
	if !ok {
		return "", "", false
	}
	key := canonicalTOMLKeyPath(path)
	valuePart := line[eq+1:]
	if comment := commentIndex(valuePart); comment >= 0 {
		valuePart = valuePart[:comment]
	}
	value := strings.TrimSpace(valuePart)
	if unquoted, err := strconv.Unquote(value); err == nil {
		value = unquoted
	}
	return key, value, true
}

func parseStringArray(value string) []string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if body == "" {
		return []string{}
	}
	var values []string
	for _, part := range splitArrayValues(body) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if unquoted, err := strconv.Unquote(part); err == nil {
			values = append(values, unquoted)
		} else {
			values = append(values, part)
		}
	}
	return values
}

func splitArrayValues(body string) []string {
	var values []string
	start := 0
	inString := false
	escaped := false
	for i, r := range body {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == ',' && !inString:
			values = append(values, body[start:i])
			start = i + 1
		}
	}
	values = append(values, body[start:])
	return values
}

func renderStringArray(values []string) string {
	if values == nil {
		values = []string{}
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, renderString(value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newPrefixedID(prefix string) string {
	return prefix + "_" + encodeCrockford(uint64(time.Now().UTC().UnixMilli()), 10) + randomCrockford(16)
}

func encodeCrockford(value uint64, length int) string {
	out := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		out[i] = crockfordAlphabet[value&31]
		value >>= 5
	}
	return string(out)
}

func randomCrockford(length int) string {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		n := uint64(time.Now().UnixNano())
		for i := range buf {
			buf[i] = byte((n >> ((i % 8) * 8)) & 31)
		}
	}
	for i := range buf {
		buf[i] = crockfordAlphabet[buf[i]&31]
	}
	return string(buf)
}
