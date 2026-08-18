package formations

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeleteBoardArchivesDefinitionAndLayoutWithPreconditions(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	boardRaw := minimalBoard("poems", 7)
	layoutRaw := `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"
`
	writeFixture(t, store.BoardPath("poems"), boardRaw)
	writeFixture(t, store.LayoutPath("poems"), layoutRaw)
	board, err := store.ReadBoard("poems")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	deleted, err := store.DeleteBoard("poems", WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		t.Fatalf("delete board: %v", err)
	}
	if deleted.Slug != "poems" || deleted.Title != board.Title || deleted.ArchiveID == "" {
		t.Fatalf("deletion = %+v, want archived board identity", deleted)
	}
	if _, err := store.ReadBoard("poems"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read deleted board error = %v, want ErrNotFound", err)
	}
	if _, err := store.ReadLayout("poems"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read deleted layout error = %v, want ErrNotFound", err)
	}

	boardArchive := findArchivedDefinition(t, store.BoardPath("poems"), deleted.ArchiveID)
	if got := readFile(t, boardArchive); got != boardRaw {
		t.Fatalf("archived board changed bytes:\n%s", got)
	}
	layoutArchive := findArchivedDefinition(t, store.LayoutPath("poems"), deleted.ArchiveID)
	if got := readFile(t, layoutArchive); got != layoutRaw {
		t.Fatalf("archived layout changed bytes:\n%s", got)
	}
}

func TestDeleteBoardRejectsStalePreconditionsWithoutMutation(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("poems"), minimalBoard("poems", 7))
	before := readFile(t, store.BoardPath("poems"))

	_, err := store.DeleteBoard("poems", WriteOptions{ExpectedETag: "stale", ExpectedRev: 7})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("delete error = %v, want ErrConflict", err)
	}
	if got := readFile(t, store.BoardPath("poems")); got != before {
		t.Fatalf("stale delete changed board:\n%s", got)
	}
}

func TestDeleteBoardRequiresPreconditions(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("poems"), minimalBoard("poems", 7))

	if _, err := store.DeleteBoard("poems", WriteOptions{}); !errors.Is(err, ErrPreconditionRequired) {
		t.Fatalf("delete error = %v, want ErrPreconditionRequired", err)
	}
}

func TestDeleteBoardHoldsLayoutLockThroughBoardArchive(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("poems"), minimalBoard("poems", 7))
	writeFixture(t, store.LayoutPath("poems"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	board, err := store.ReadBoard("poems")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	archived := make(chan struct{})
	release := make(chan struct{})
	store.deleteBoardAfterLayoutArchiveForTest = func() {
		close(archived)
		<-release
	}
	deleteResult := make(chan error, 1)
	go func() {
		_, err := store.DeleteBoard("poems", WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
		deleteResult <- err
	}()
	<-archived

	updateResult := make(chan error, 1)
	go func() {
		_, err := store.UpdateLayoutNodes("poems", []LayoutNode{{ID: "fmn_frame", X: 10, Y: 20}}, WriteOptions{ExpectedETag: "*"})
		updateResult <- err
	}()
	select {
	case err := <-updateResult:
		t.Fatalf("layout update escaped delete lock before board archive: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-deleteResult; err != nil {
		t.Fatalf("delete board: %v", err)
	}
	if err := <-updateResult; !errors.Is(err, ErrNotFound) {
		t.Fatalf("layout recreation error = %v, want ErrNotFound after board archive", err)
	}
	if _, err := store.ReadBoard("poems"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read deleted board error = %v", err)
	}
	if _, err := store.ReadLayout("poems"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read deleted layout error = %v", err)
	}
}

func TestDeleteBoardReportsLayoutArchiveSyncUncertaintyAndRemainsRetryable(t *testing.T) {
	store := NewStore(t.TempDir())
	boardRaw := minimalBoard("poems", 7)
	layoutRaw := `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:00:00Z"
`
	writeFixture(t, store.BoardPath("poems"), boardRaw)
	writeFixture(t, store.LayoutPath("poems"), layoutRaw)
	board, err := store.ReadBoard("poems")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	store.archiveDirectorySyncForTest = func() error { return errors.New("injected directory sync fault") }
	if _, err := store.DeleteBoard("poems", WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev}); !errors.Is(err, ErrDefinitionPublicationUncertain) {
		t.Fatalf("delete error = %v, want ErrDefinitionPublicationUncertain", err)
	}
	if got := readFile(t, store.BoardPath("poems")); got != boardRaw {
		t.Fatalf("uncertain layout archive changed board:\n%s", got)
	}
	if _, err := store.ReadLayout("poems"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("layout after uncertain archive error = %v, want ErrNotFound", err)
	}
	layoutArchive := findArchivedByPrefix(t, store.LayoutPath("poems"))
	if got := readFile(t, layoutArchive); got != layoutRaw {
		t.Fatalf("archived layout bytes changed:\n%s", got)
	}

	store.archiveDirectorySyncForTest = nil
	if _, err := store.DeleteBoard("poems", WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev}); err != nil {
		t.Fatalf("retry delete after reload: %v", err)
	}
	if _, err := store.ReadBoard("poems"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("board after retry error = %v, want ErrNotFound", err)
	}
}

func TestDeleteBoardReportsBoardArchiveSyncUncertaintyWithoutLosingBytes(t *testing.T) {
	store := NewStore(t.TempDir())
	boardRaw := minimalBoard("poems", 7)
	writeFixture(t, store.BoardPath("poems"), boardRaw)
	board, err := store.ReadBoard("poems")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	store.archiveDirectorySyncForTest = func() error { return errors.New("injected directory sync fault") }
	if _, err := store.DeleteBoard("poems", WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev}); !errors.Is(err, ErrDefinitionPublicationUncertain) {
		t.Fatalf("delete error = %v, want ErrDefinitionPublicationUncertain", err)
	}
	if _, err := store.ReadBoard("poems"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("board after uncertain archive error = %v, want ErrNotFound", err)
	}
	boardArchive := findArchivedByPrefix(t, store.BoardPath("poems"))
	if got := readFile(t, boardArchive); got != boardRaw {
		t.Fatalf("archived board bytes changed:\n%s", got)
	}
}

func findArchivedByPrefix(t *testing.T, livePath string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(livePath))
	if err != nil {
		t.Fatalf("read archive directory: %v", err)
	}
	prefix := filepath.Base(livePath) + ".deleted-"
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			return filepath.Join(filepath.Dir(livePath), entry.Name())
		}
	}
	t.Fatalf("archive prefix %q not found", prefix)
	return ""
}

func findArchivedDefinition(t *testing.T, livePath, archiveID string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(livePath))
	if err != nil {
		t.Fatalf("read archive directory: %v", err)
	}
	want := filepath.Base(livePath) + ".deleted-" + archiveID
	for _, entry := range entries {
		if entry.Name() == want {
			return filepath.Join(filepath.Dir(livePath), entry.Name())
		}
	}
	t.Fatalf("archive %q not found", want)
	return ""
}
