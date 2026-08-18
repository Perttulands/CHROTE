package formations

import (
	"errors"
	"strings"
	"testing"
)

func TestBoardNotesRoundTripWithoutChangingExecutableBoard(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), notesBoardFixture())
	boardBefore := readFile(t, store.BoardPath("session-search"))

	empty, err := store.ReadBoardNotes("session-search")
	if err != nil {
		t.Fatalf("read empty notes: %v", err)
	}
	if empty.ETag != "*" || empty.Rev != 0 || empty.BoardID != "brd_notes" || empty.Board != "" || len(empty.Elements) != 0 {
		t.Fatalf("empty notes = %+v, want synthetic empty document", empty)
	}

	boardText := "Shared plan\n\n- preserve the API\n- ship tests"
	afterBoard, err := store.UpdateBoardNote("session-search", BoardNotePatch{
		Target:    BoardNoteTarget,
		Text:      boardText,
		UpdatedBy: "human:operator",
	}, NoteWriteOptions{ExpectedETag: empty.ETag})
	if err != nil {
		t.Fatalf("write board note: %v", err)
	}
	if afterBoard.Rev != 1 || afterBoard.Board != boardText || afterBoard.ETag == "" || afterBoard.ETag == "*" {
		t.Fatalf("board notes = %+v, want persisted multiline board note", afterBoard)
	}

	elementText := "Builder: keep this formation narrow."
	afterElement, err := store.UpdateBoardNote("session-search", BoardNotePatch{
		Target:    "fmn_frame",
		Text:      elementText,
		UpdatedBy: "agent:archon",
	}, NoteWriteOptions{ExpectedETag: afterBoard.ETag})
	if err != nil {
		t.Fatalf("write element note: %v", err)
	}
	if afterElement.Rev != 2 || len(afterElement.Elements) != 1 || afterElement.Elements[0].NodeID != "fmn_frame" || afterElement.Elements[0].Text != elementText {
		t.Fatalf("element notes = %+v, want exact fmn_frame note", afterElement)
	}
	if got := readFile(t, store.BoardPath("session-search")); got != boardBefore {
		t.Fatalf("notes changed executable board bytes:\n%s", got)
	}

	reread, err := store.ReadBoardNotes("session-search")
	if err != nil {
		t.Fatalf("reread notes: %v", err)
	}
	if reread.Board != boardText || reread.Elements[0].Text != elementText || reread.TOML != "" {
		t.Fatalf("reread notes = %+v, want exact text without raw TOML", reread)
	}
}

func TestBoardNotesAreBoundToStableBoardIDAcrossSlugReuse(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	original, err := store.CreateBoard(BoardCreateRequest{Slug: "reused", Title: "Original", UpdatedBy: "human"})
	if err != nil {
		t.Fatalf("create original board: %v", err)
	}
	written, err := store.UpdateBoardNote("reused", BoardNotePatch{Target: BoardNoteTarget, Text: "original secret"}, NoteWriteOptions{ExpectedETag: "*"})
	if err != nil {
		t.Fatalf("write original note: %v", err)
	}
	if _, err := store.DeleteBoard("reused", WriteOptions{ExpectedETag: original.ETag, ExpectedRev: original.Rev}); err != nil {
		t.Fatalf("archive original board: %v", err)
	}
	recreated, err := store.CreateBoard(BoardCreateRequest{Slug: "reused", Title: "Replacement", UpdatedBy: "human"})
	if err != nil {
		t.Fatalf("create replacement board: %v", err)
	}
	if recreated.ID == original.ID {
		t.Fatalf("replacement board reused stable id %q", recreated.ID)
	}
	empty, err := store.ReadBoardNotes("reused")
	if err != nil {
		t.Fatalf("read replacement notes: %v", err)
	}
	if empty.Board != "" || len(empty.Elements) != 0 || empty.ETag != "*" || empty.BoardID != recreated.ID {
		t.Fatalf("replacement leaked archived notes = %+v (old notes %+v)", empty, written)
	}
	updated, err := store.UpdateBoardNote("reused", BoardNotePatch{Target: BoardNoteTarget, Text: "replacement"}, NoteWriteOptions{ExpectedETag: "*"})
	if err != nil {
		t.Fatalf("write replacement note: %v", err)
	}
	if updated.BoardID != recreated.ID || updated.Board != "replacement" {
		t.Fatalf("replacement notes = %+v", updated)
	}
}

func TestBoardNotesRejectUnknownElementAndStaleETagWithoutClobber(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("session-search"), notesBoardFixture())
	empty, err := store.ReadBoardNotes("session-search")
	if err != nil {
		t.Fatalf("read empty notes: %v", err)
	}

	if _, err := store.UpdateBoardNote("session-search", BoardNotePatch{Target: "fmn_missing", Text: "nope"}, NoteWriteOptions{ExpectedETag: empty.ETag}); !errors.Is(err, ErrNoteTargetNotFound) {
		t.Fatalf("unknown target error = %v, want ErrNoteTargetNotFound", err)
	}
	current, err := store.UpdateBoardNote("session-search", BoardNotePatch{Target: BoardNoteTarget, Text: "current"}, NoteWriteOptions{ExpectedETag: empty.ETag})
	if err != nil {
		t.Fatalf("write current note: %v", err)
	}
	if _, err := store.UpdateBoardNote("session-search", BoardNotePatch{Target: BoardNoteTarget, Text: "stale"}, NoteWriteOptions{ExpectedETag: empty.ETag}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale note error = %v, want ErrConflict", err)
	}
	after, err := store.ReadBoardNotes("session-search")
	if err != nil {
		t.Fatalf("read after stale write: %v", err)
	}
	if after.Board != "current" || after.ETag != current.ETag {
		t.Fatalf("stale write clobbered notes = %+v", after)
	}
}

func TestBoardNotesRejectOversizedText(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("session-search"), notesBoardFixture())

	_, err := store.UpdateBoardNote("session-search", BoardNotePatch{
		Target: BoardNoteTarget,
		Text:   strings.Repeat("x", MaxBoardNoteBytes+1),
	}, NoteWriteOptions{ExpectedETag: "*"})
	if !errors.Is(err, ErrNoteTooLarge) {
		t.Fatalf("oversized note error = %v, want ErrNoteTooLarge", err)
	}
}

func notesBoardFixture() string {
	return `schema = 1
id = "brd_notes"
slug = "session-search"
title = "Session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

[[formation.slot]]
id = "slot_frame"
label = "Builder"
`
}
