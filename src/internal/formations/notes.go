package formations

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	CurrentBoardNotesSchema = 1
	BoardNoteTarget         = "board"
	MaxBoardNoteBytes       = 64 * 1024
	maxBoardNotesBytes      = 512 * 1024
)

var (
	ErrNoteTargetNotFound = errors.New("formation note target not found")
	ErrNoteTooLarge       = errors.New("formation note is too large")
)

type ElementNote struct {
	NodeID string `json:"nodeId" toml:"nodeId"`
	Text   string `json:"text" toml:"text"`
}

type BoardNotesDocument struct {
	Schema    int           `json:"schema"`
	BoardID   string        `json:"boardId"`
	Rev       int           `json:"rev"`
	UpdatedAt time.Time     `json:"updatedAt"`
	UpdatedBy string        `json:"updatedBy,omitempty"`
	Board     string        `json:"board"`
	Elements  []ElementNote `json:"elements"`
	ETag      string        `json:"etag"`
	TOML      string        `json:"-"`
}

type BoardNotePatch struct {
	Target    string `json:"target"`
	Text      string `json:"text"`
	UpdatedBy string `json:"updatedBy,omitempty"`
}

type NoteWriteOptions struct {
	ExpectedETag string
}

type boardNotesTOMLSource struct {
	Schema    int           `toml:"schema"`
	BoardID   string        `toml:"boardId"`
	Rev       int           `toml:"rev"`
	UpdatedAt time.Time     `toml:"updatedAt"`
	UpdatedBy string        `toml:"updatedBy"`
	Board     string        `toml:"board"`
	Elements  []ElementNote `toml:"element"`
}

func (s *Store) NotesPath(slug string) string {
	return filepath.Join(s.workspaceRoot(), ".formations", notesDefinitionKind.directory, slug+notesDefinitionKind.suffix)
}

func (s *Store) ReadBoardNotes(slug string) (*BoardNotesDocument, error) {
	board, err := s.ReadBoard(slug)
	if err != nil {
		return nil, err
	}
	notesDefinition, err := s.openNotesDefinition(slug, false)
	if errors.Is(err, ErrNotFound) {
		return emptyBoardNotes(board.ID), nil
	}
	if err != nil {
		return nil, err
	}
	defer notesDefinition.close()
	raw, err := notesDefinition.readBytes()
	if errors.Is(err, ErrNotFound) {
		return emptyBoardNotes(board.ID), nil
	}
	if err != nil {
		return nil, err
	}
	notes, err := parseBoardNotes(raw)
	if err != nil {
		return nil, err
	}
	if notes.BoardID != board.ID {
		return emptyBoardNotes(board.ID), nil
	}
	return notes, nil
}

func (s *Store) UpdateBoardNote(slug string, patch BoardNotePatch, opts NoteWriteOptions) (*BoardNotesDocument, error) {
	if strings.TrimSpace(opts.ExpectedETag) == "" {
		return nil, ErrPreconditionRequired
	}
	patch.Target = strings.TrimSpace(patch.Target)
	if patch.Target == "" {
		patch.Target = BoardNoteTarget
	}
	if len([]byte(patch.Text)) > MaxBoardNoteBytes {
		return nil, ErrNoteTooLarge
	}

	var updated *BoardNotesDocument
	err := s.withBoardDefinitionLock(slug, func(boardDefinition *definitionFile) error {
		boardRaw, err := boardDefinition.readBytes()
		if err != nil {
			return err
		}
		board, err := parseBoard(boardRaw)
		if err != nil {
			return err
		}
		if patch.Target != BoardNoteTarget && !boardHasNoteTarget(board, patch.Target) {
			return ErrNoteTargetNotFound
		}

		return s.withNotesDefinitionLock(slug, func(notesDefinition *definitionFile) error {
			current := emptyBoardNotes(board.ID)
			exists, err := notesDefinition.exists()
			if err != nil {
				return err
			}
			if exists {
				raw, err := notesDefinition.readBytes()
				if err != nil {
					return err
				}
				persisted, err := parseBoardNotes(raw)
				if err != nil {
					return err
				}
				if persisted.BoardID != board.ID {
					if _, err := notesDefinition.archive(newPrefixedID("archive")); err != nil {
						return err
					}
					exists = false
				} else {
					current = persisted
				}
			}
			if exists {
				if opts.ExpectedETag == "*" || opts.ExpectedETag != current.ETag {
					return ErrConflict
				}
			} else if opts.ExpectedETag != "*" {
				return ErrConflict
			}

			next := cloneBoardNotes(current)
			next.Schema = CurrentBoardNotesSchema
			next.BoardID = board.ID
			next.Rev = current.Rev + 1
			next.UpdatedAt = s.now()
			next.UpdatedBy = strings.TrimSpace(patch.UpdatedBy)
			if patch.Target == BoardNoteTarget {
				next.Board = patch.Text
			} else {
				next.Elements = updateElementNote(next.Elements, patch.Target, patch.Text)
			}
			if boardNotesSize(next) > maxBoardNotesBytes {
				return ErrNoteTooLarge
			}
			rendered := renderBoardNotes(next)
			if err := notesDefinition.writeAtomic(rendered); err != nil {
				return err
			}
			next.ETag = etag(rendered)
			next.TOML = ""
			updated = next
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func emptyBoardNotes(boardID string) *BoardNotesDocument {
	return &BoardNotesDocument{
		Schema:   CurrentBoardNotesSchema,
		BoardID:  boardID,
		Elements: []ElementNote{},
		ETag:     "*",
	}
}

func parseBoardNotes(raw []byte) (*BoardNotesDocument, error) {
	var source boardNotesTOMLSource
	if err := toml.Unmarshal(raw, &source); err != nil {
		return nil, invalidDefinitionSource(err)
	}
	if source.Schema != CurrentBoardNotesSchema || strings.TrimSpace(source.BoardID) == "" || source.Rev < 1 || source.UpdatedAt.IsZero() {
		return nil, invalidDefinitionSource(errors.New("invalid board notes metadata"))
	}
	seen := map[string]struct{}{}
	for _, note := range source.Elements {
		if strings.TrimSpace(note.NodeID) == "" {
			return nil, invalidDefinitionSource(errors.New("element note is missing nodeId"))
		}
		if _, duplicate := seen[note.NodeID]; duplicate {
			return nil, invalidDefinitionSource(fmt.Errorf("duplicate element note %q", note.NodeID))
		}
		seen[note.NodeID] = struct{}{}
		if len([]byte(note.Text)) > MaxBoardNoteBytes {
			return nil, ErrNoteTooLarge
		}
	}
	notes := &BoardNotesDocument{
		Schema:    source.Schema,
		BoardID:   source.BoardID,
		Rev:       source.Rev,
		UpdatedAt: source.UpdatedAt.UTC(),
		UpdatedBy: source.UpdatedBy,
		Board:     source.Board,
		Elements:  append([]ElementNote(nil), source.Elements...),
		ETag:      etag(raw),
	}
	if notes.Elements == nil {
		notes.Elements = []ElementNote{}
	}
	sort.Slice(notes.Elements, func(i, j int) bool { return notes.Elements[i].NodeID < notes.Elements[j].NodeID })
	if boardNotesSize(notes) > maxBoardNotesBytes {
		return nil, ErrNoteTooLarge
	}
	return notes, nil
}

func renderBoardNotes(notes *BoardNotesDocument) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "schema = %d\n", CurrentBoardNotesSchema)
	fmt.Fprintf(&b, "boardId = %s\n", renderString(notes.BoardID))
	fmt.Fprintf(&b, "rev = %d\n", notes.Rev)
	fmt.Fprintf(&b, "updatedAt = %s\n", renderString(notes.UpdatedAt.UTC().Format(time.RFC3339Nano)))
	fmt.Fprintf(&b, "updatedBy = %s\n", renderString(notes.UpdatedBy))
	fmt.Fprintf(&b, "board = %s\n", renderString(notes.Board))

	elements := append([]ElementNote(nil), notes.Elements...)
	sort.Slice(elements, func(i, j int) bool { return elements[i].NodeID < elements[j].NodeID })
	for _, note := range elements {
		fmt.Fprintf(&b, "\n[[element]]\nnodeId = %s\ntext = %s\n", renderString(note.NodeID), renderString(note.Text))
	}
	return []byte(b.String())
}

func cloneBoardNotes(notes *BoardNotesDocument) *BoardNotesDocument {
	clone := *notes
	clone.Elements = append([]ElementNote(nil), notes.Elements...)
	return &clone
}

func updateElementNote(notes []ElementNote, nodeID, text string) []ElementNote {
	updated := make([]ElementNote, 0, len(notes)+1)
	found := false
	for _, note := range notes {
		if note.NodeID != nodeID {
			updated = append(updated, note)
			continue
		}
		found = true
		if text != "" {
			updated = append(updated, ElementNote{NodeID: nodeID, Text: text})
		}
	}
	if !found && text != "" {
		updated = append(updated, ElementNote{NodeID: nodeID, Text: text})
	}
	sort.Slice(updated, func(i, j int) bool { return updated[i].NodeID < updated[j].NodeID })
	return updated
}

func boardHasNoteTarget(board *BoardDocument, target string) bool {
	for _, mission := range board.Missions {
		if mission.ID == target {
			return true
		}
	}
	for _, formation := range board.Formations {
		if formation.ID == target {
			return true
		}
	}
	for _, gate := range board.Gates {
		if gate.ID == target {
			return true
		}
	}
	for _, tool := range board.Tools {
		if tool.ID == target {
			return true
		}
	}
	return false
}

func boardNotesSize(notes *BoardNotesDocument) int {
	total := len([]byte(notes.Board))
	for _, note := range notes.Elements {
		total += len([]byte(note.NodeID)) + len([]byte(note.Text))
	}
	return total
}
