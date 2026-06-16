// Package formations owns CHROTE Formations definition persistence.
package formations

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const CurrentSchema = 1

var (
	ErrConflict             = errors.New("formations conflict")
	ErrAmbiguousSelector    = errors.New("ambiguous formations selector")
	ErrInvalidSlug          = errors.New("invalid formations slug")
	ErrInvalidBeadID        = errors.New("invalid Beads issue id")
	ErrNotFound             = errors.New("formations file not found")
	ErrPreconditionRequired = errors.New("formations write precondition required")
	ErrUnsupportedSchema    = errors.New("unsupported formations schema")
)

type Store struct {
	Workspace string
	Now       func() time.Time
}

type BoardDocument struct {
	Schema      int               `json:"schema"`
	ID          string            `json:"id"`
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Rev         int               `json:"rev"`
	UpdatedBy   string            `json:"updatedBy,omitempty"`
	UpdatedAt   string            `json:"updatedAt,omitempty"`
	Missions    []MissionNode     `json:"missions,omitempty"`
	Formations  []FormationNode   `json:"formations,omitempty"`
	Gates       []GateNode        `json:"gates,omitempty"`
	Connections []BoardConnection `json:"connections,omitempty"`
	ETag        string            `json:"etag"`
	TOML        string            `json:"toml,omitempty"`
}

type BoardSummary struct {
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Rev   int    `json:"rev"`
	ETag  string `json:"etag"`
	// Mission is the board's single mission (its identity). It is nil only for a
	// malformed board that has no mission; a well-formed Mission Board always has
	// exactly one. Carried in the summary so a gallery can render the goal without
	// reading each board.
	Mission *MissionNode `json:"mission"`
	// LatestRun is the board's most-recent run, or nil when the board has never
	// run. Carried in the summary so a gallery can render run status without a
	// follow-up request per board.
	LatestRun *RunSummary `json:"latestRun"`
}

// RunSummary is the gallery-facing projection of a board's most-recent run: just
// enough to render a status pill without fetching the full run.
type RunSummary struct {
	RunID  string `json:"runId"`
	Status string `json:"status"`
	Final  bool   `json:"final"`
	Epoch  int    `json:"epoch"`
}

type LayoutDocument struct {
	Schema    int          `json:"schema"`
	BoardID   string       `json:"boardId"`
	BoardRev  int          `json:"boardRev"`
	UpdatedAt string       `json:"updatedAt,omitempty"`
	Nodes     []LayoutNode `json:"nodes,omitempty"`
	Edges     []LayoutEdge `json:"edges,omitempty"`
	ETag      string       `json:"etag"`
	TOML      string       `json:"toml,omitempty"`
}

type BoardChangeSignal struct {
	Board      string `json:"board"`
	Changed    bool   `json:"changed"`
	Signal     string `json:"signal,omitempty"`
	ETag       string `json:"etag"`
	ModifiedAt string `json:"modifiedAt"`
}

type BoardMetadataPatch struct {
	Title     *string
	UpdatedBy string
}

type LayoutMetadataPatch struct {
	UpdatedAt time.Time
}

type WriteOptions struct {
	ExpectedETag string
	ExpectedRev  int
}

func NewStore(workspace string) *Store {
	return &Store{
		Workspace: workspace,
		Now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Store) BoardPath(slug string) string {
	return filepath.Join(s.Workspace, ".formations", "boards", slug+".formation.toml")
}

func (s *Store) LayoutPath(slug string) string {
	return filepath.Join(s.Workspace, ".formations", "layout", slug+".layout.toml")
}

// ListBoards returns one gallery-ready summary per board: its identity, its
// single mission, and its most-recent run. Populating the latest run walks the
// run ledgers, so this is the gallery path, not the selector hot path —
// ResolveBoardSelector uses listBoardIdentities, which skips the run walk.
func (s *Store) ListBoards() ([]BoardSummary, error) {
	boards, err := s.listBoardIdentities()
	if err != nil {
		return nil, err
	}
	for i := range boards {
		// The most-recent run is the last entry from ListRuns, which returns runs
		// sorted ascending by run id (run ids are time-ordered). Fail loud if the
		// run ledgers cannot be read rather than hiding a corrupt ledger.
		runs, err := s.ListRuns(RunListFilter{BoardSlug: boards[i].Slug})
		if err != nil {
			return nil, err
		}
		if len(runs) > 0 {
			latest := runs[len(runs)-1]
			boards[i].LatestRun = &RunSummary{
				RunID:  latest.RunID,
				Status: latest.Status,
				Final:  latest.Final,
				Epoch:  latest.Epoch,
			}
		}
	}
	return boards, nil
}

// listBoardIdentities returns the cheap per-board summary: identity plus the
// board's single mission, parsed straight from the board file with no run-ledger
// walk. It is the shared basis for both ResolveBoardSelector (which needs ids and
// slugs) and ListBoards (which enriches each summary with the latest run).
func (s *Store) listBoardIdentities() ([]BoardSummary, error) {
	dir := filepath.Join(s.Workspace, ".formations", "boards")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BoardSummary{}, nil
		}
		return nil, err
	}

	boards := make([]BoardSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".formation.toml") {
			continue
		}
		slug := strings.TrimSuffix(entry.Name(), ".formation.toml")
		board, err := s.ReadBoard(slug)
		if err != nil {
			return nil, err
		}
		summary := BoardSummary{
			ID:    board.ID,
			Slug:  board.Slug,
			Title: board.Title,
			Rev:   board.Rev,
			ETag:  board.ETag,
		}
		// A Mission Board's mission is its identity; carry it so a gallery does not
		// have to read each board. Take the first when present (the one-mission
		// invariant guarantees there is at most one on a well-formed board).
		if len(board.Missions) > 0 {
			mission := board.Missions[0]
			summary.Mission = &mission
		}
		boards = append(boards, summary)
	}
	sort.Slice(boards, func(i, j int) bool {
		return boards[i].Slug < boards[j].Slug
	})
	return boards, nil
}

func (s *Store) ReadBoard(slug string) (*BoardDocument, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	return s.readBoardPath(s.BoardPath(slug))
}

// CreateMissionBoard creates a Mission Board: a board and its single mission in
// one atomic write. A Mission Board's identity IS its mission, so the two are
// born together — there is never a window where an empty board persists on disk.
// The board definition (with the mission block) is rendered and written in a
// single writeAtomic under the board file lock; only the mission's layout
// coordinates land afterward in the non-authoritative layout sidecar (a missing
// layout is a normal degraded state). A duplicate slug, an invalid slug, an empty
// title, or a malformed beadId is rejected with its precise typed error before
// the board is written. opts is accepted for signature symmetry with the other
// write paths; create is unconditional (the slug must not already exist), so its
// preconditions are not consulted.
func (s *Store) CreateMissionBoard(slug, title string, mission MissionCreateRequest, opts WriteOptions) (*BoardDocument, error) {
	_ = opts
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return nil, fmt.Errorf("%w: board title is required", ErrInvalidSlug)
	}
	if err := validateOptionalBeadID(mission.BeadID); err != nil {
		return nil, err
	}
	missionTitle := mission.Title
	if missionTitle == "" {
		missionTitle = "Mission"
	}
	node := MissionNode{
		ID:     newPrefixedID("mis"),
		Title:  missionTitle,
		Goal:   mission.Goal,
		BeadID: mission.BeadID,
	}
	path := s.BoardPath(slug)
	var created *BoardDocument
	err := withFileLock(path, func() error {
		if _, err := os.Stat(path); err == nil {
			return ErrAlreadyExists
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		raw := appendMissionBlock(renderBoard(slug, cleanTitle, strings.TrimSpace(mission.UpdatedBy), s.now()), node)
		if err := writeAtomic(path, raw); err != nil {
			return err
		}
		board, err := parseBoard(raw)
		if err != nil {
			return err
		}
		created = board
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.upsertLayoutNode(slug, created.ID, created.Rev, LayoutNode{ID: node.ID, X: mission.X, Y: mission.Y}); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Store) BoardChangeSince(slug, previousETag string) (*BoardChangeSignal, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	path := s.BoardPath(slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if _, err := parseBoard(raw); err != nil {
		return nil, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	currentETag := etag(raw)
	changed := previousETag != "" && previousETag != currentETag
	signal := ""
	if changed {
		signal = "board.changed"
	}
	return &BoardChangeSignal{
		Board:      slug,
		Changed:    changed,
		Signal:     signal,
		ETag:       currentETag,
		ModifiedAt: stat.ModTime().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Store) UpdateBoardMetadata(slug string, patch BoardMetadataPatch, opts WriteOptions) (*BoardDocument, error) {
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
		if opts.ExpectedETag != current.ETag {
			return ErrConflict
		}
		if opts.ExpectedRev != current.Rev {
			return ErrConflict
		}

		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		if patch.Title != nil {
			doc.setScalar("title", renderString(*patch.Title))
		}
		if patch.UpdatedBy != "" {
			doc.setScalar("updatedBy", renderString(patch.UpdatedBy))
		}
		doc.setScalar("rev", renderInt(current.Rev+1))
		doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))

		nextRaw := doc.bytes()
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

func (s *Store) ReadLayout(slug string) (*LayoutDocument, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	return s.readLayoutPath(s.LayoutPath(slug))
}

func (s *Store) UpdateLayoutMetadata(slug string, patch LayoutMetadataPatch, opts WriteOptions) (*LayoutDocument, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if opts.ExpectedETag == "" {
		return nil, ErrPreconditionRequired
	}
	path := s.LayoutPath(slug)
	var next *LayoutDocument
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

		when := patch.UpdatedAt
		if when.IsZero() {
			when = s.now()
		}
		doc := parseTOMLDocument(raw)
		if current.Schema < CurrentSchema {
			doc.setScalar("schema", renderInt(CurrentSchema))
		}
		doc.setScalar("updatedAt", renderString(when.UTC().Format(time.RFC3339)))

		nextRaw := doc.bytes()
		if err := writeAtomic(path, nextRaw); err != nil {
			return err
		}
		next, err = parseLayout(nextRaw)
		return err
	})
	if err != nil {
		return nil, err
	}
	return next, nil
}

func (s *Store) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s *Store) readBoardPath(path string) (*BoardDocument, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	current, err := parseBoard(raw)
	if err != nil {
		return nil, err
	}
	if current.Schema >= CurrentSchema {
		return current, nil
	}

	var migrated *BoardDocument
	err = withFileLock(path, func() error {
		lockedRaw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		lockedCurrent, err := parseBoard(lockedRaw)
		if err != nil {
			return err
		}
		if lockedCurrent.Schema >= CurrentSchema {
			migrated = lockedCurrent
			return nil
		}
		doc := parseTOMLDocument(lockedRaw)
		doc.setScalar("schema", renderInt(CurrentSchema))
		migratedRaw := doc.bytes()
		if err := writeAtomic(path, migratedRaw); err != nil {
			return err
		}
		migrated, err = parseBoard(migratedRaw)
		return err
	})
	if err != nil {
		return nil, err
	}
	return migrated, nil
}

func (s *Store) readLayoutPath(path string) (*LayoutDocument, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	current, err := parseLayout(raw)
	if err != nil {
		return nil, err
	}
	if current.Schema >= CurrentSchema {
		return current, nil
	}

	var migrated *LayoutDocument
	err = withFileLock(path, func() error {
		lockedRaw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		lockedCurrent, err := parseLayout(lockedRaw)
		if err != nil {
			return err
		}
		if lockedCurrent.Schema >= CurrentSchema {
			migrated = lockedCurrent
			return nil
		}
		doc := parseTOMLDocument(lockedRaw)
		doc.setScalar("schema", renderInt(CurrentSchema))
		migratedRaw := doc.bytes()
		if err := writeAtomic(path, migratedRaw); err != nil {
			return err
		}
		migrated, err = parseLayout(migratedRaw)
		return err
	})
	if err != nil {
		return nil, err
	}
	return migrated, nil
}

func parseBoard(raw []byte) (*BoardDocument, error) {
	doc := parseTOMLDocument(raw)
	schema := doc.intValue("schema")
	if schema > CurrentSchema {
		return nil, fmt.Errorf("%w: schema %d", ErrUnsupportedSchema, schema)
	}
	return &BoardDocument{
		Schema:      schema,
		ID:          doc.stringValue("id"),
		Slug:        doc.stringValue("slug"),
		Title:       doc.stringValue("title"),
		Rev:         doc.intValue("rev"),
		UpdatedBy:   doc.stringValue("updatedBy"),
		UpdatedAt:   doc.stringValue("updatedAt"),
		Missions:    parseMissionNodes(raw),
		Formations:  parseFormationNodes(raw),
		Gates:       parseGateNodes(raw),
		Connections: parseBoardConnections(raw),
		ETag:        etag(raw),
		TOML:        string(raw),
	}, nil
}

func parseLayout(raw []byte) (*LayoutDocument, error) {
	doc := parseTOMLDocument(raw)
	schema := doc.intValue("schema")
	if schema > CurrentSchema {
		return nil, fmt.Errorf("%w: schema %d", ErrUnsupportedSchema, schema)
	}
	return &LayoutDocument{
		Schema:    schema,
		BoardID:   doc.stringValue("boardId"),
		BoardRev:  doc.intValue("boardRev"),
		UpdatedAt: doc.stringValue("updatedAt"),
		Nodes:     parseLayoutNodes(raw),
		Edges:     parseLayoutEdges(raw),
		ETag:      etag(raw),
		TOML:      string(raw),
	}, nil
}

func validateSlug(slug string) error {
	if slug == "" || slug == "." || slug == ".." {
		return ErrInvalidSlug
	}
	if strings.ContainsAny(slug, `/\`) || strings.Contains(slug, "..") {
		return ErrInvalidSlug
	}
	return nil
}

func etag(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func renderString(v string) string {
	return strconv.Quote(v)
}

func renderInt(v int) string {
	return strconv.Itoa(v)
}

func renderBoard(slug, title, updatedBy string, updatedAt time.Time) []byte {
	var b strings.Builder
	b.WriteString("schema = " + renderInt(CurrentSchema) + "\n")
	b.WriteString("id = " + renderString(newPrefixedID("brd")) + "\n")
	b.WriteString("slug = " + renderString(slug) + "\n")
	b.WriteString("title = " + renderString(title) + "\n")
	b.WriteString("rev = 1\n")
	if updatedBy != "" {
		b.WriteString("updatedBy = " + renderString(updatedBy) + "\n")
	}
	b.WriteString("updatedAt = " + renderString(updatedAt.UTC().Format(time.RFC3339)) + "\n")
	return []byte(b.String())
}
