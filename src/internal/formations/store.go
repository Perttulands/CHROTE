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
	ErrNotFound             = errors.New("formations file not found")
	ErrPreconditionRequired = errors.New("formations write precondition required")
	ErrUnsupportedSchema    = errors.New("unsupported formations schema")
)

type Store struct {
	Workspace string
	Now       func() time.Time

	runtimeAuthority *runtimeAuthorityBoundary
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

type BoardCreateRequest struct {
	Slug      string
	Title     string
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

func (s *Store) workspaceRoot() string {
	if s == nil {
		return ""
	}
	if s.runtimeAuthority != nil {
		return s.runtimeAuthority.configuredWorkspace
	}
	return s.Workspace
}

func (s *Store) BoardPath(slug string) string {
	return filepath.Join(s.workspaceRoot(), ".formations", "boards", slug+".formation.toml")
}

func (s *Store) LayoutPath(slug string) string {
	return filepath.Join(s.workspaceRoot(), ".formations", "layout", slug+".layout.toml")
}

func (s *Store) ListBoards() ([]BoardSummary, error) {
	dir := filepath.Join(s.workspaceRoot(), ".formations", "boards")
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
		boards = append(boards, BoardSummary{
			ID:    board.ID,
			Slug:  board.Slug,
			Title: board.Title,
			Rev:   board.Rev,
			ETag:  board.ETag,
		})
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

func (s *Store) CreateBoard(req BoardCreateRequest) (*BoardDocument, error) {
	if err := validateSlug(req.Slug); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: board title is required", ErrInvalidSlug)
	}
	path := s.BoardPath(req.Slug)
	var created *BoardDocument
	err := withFileLock(path, func() error {
		if _, err := os.Stat(path); err == nil {
			return ErrAlreadyExists
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		raw := renderBoard(req.Slug, title, strings.TrimSpace(req.UpdatedBy), s.now())
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
	board := &BoardDocument{
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
	}
	populateLegacyScriptGateMigrationInspections(board)
	return board, nil
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
