package formations

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestToolMutationValidationUsesDedicatedSentinel(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Store, string, *BoardDocument) error
	}{
		{
			name: "invalid create tuple",
			run: func(store *Store, slug string, board *BoardDocument) error {
				request := toolAuthoringCreateRequest(ToolPlacement{})
				request.ProfileVersion = "missing"
				_, err := store.CreateTool(slug, request, toolAuthoringAbsentOptions(board))
				return err
			},
		},
		{
			name: "invalid create request",
			run: func(store *Store, slug string, board *BoardDocument) error {
				request := toolAuthoringCreateRequest(ToolPlacement{})
				request.Params = nil
				_, err := store.CreateTool(slug, request, toolAuthoringAbsentOptions(board))
				return err
			},
		},
		{
			name: "invalid update candidate",
			run: func(store *Store, slug string, board *BoardDocument) error {
				params := map[string]any{"mode": "relaxed"}
				_, err := store.UpdateTool(slug, ToolUpdateRequest{ToolID: "tool_target", Params: &params}, toolAuthoringAbsentOptions(board))
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-invalid-identity"
			boardRaw := toolAuthoringBoardFixture(slug, 5, true, toolUpdateTargetBlock())
			writeFixture(t, store.BoardPath(slug), boardRaw)
			board, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("read Tool validation source: %v", err)
			}

			err = test.run(store, slug, board)
			if err == nil || !errors.Is(err, ErrInvalidToolMutation) {
				t.Fatalf("Tool validation error = %v, want errors.Is(ErrInvalidToolMutation)", err)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
		})
	}
}

func TestToolMutationValidationSentinelPreservesNonValidationIdentity(t *testing.T) {
	assertDistinct := func(t *testing.T, err, want error) {
		t.Helper()
		if err == nil || !errors.Is(err, want) {
			t.Fatalf("Tool mutation error = %v, want errors.Is(%v)", err, want)
		}
		if errors.Is(err, ErrInvalidToolMutation) {
			t.Fatalf("Tool mutation error %v was swallowed by ErrInvalidToolMutation", err)
		}
	}

	t.Run("conflict", func(t *testing.T) {
		store := newToolAuthoringStore(t)
		slug := "tool-identity-conflict"
		boardRaw := toolAuthoringBoardFixture(slug, 5, true, "")
		writeFixture(t, store.BoardPath(slug), boardRaw)
		board, err := store.ReadBoard(slug)
		if err != nil {
			t.Fatalf("read conflict source: %v", err)
		}
		opts := toolAuthoringAbsentOptions(board)
		opts.Board.ExpectedETag = strings.Repeat("0", 64)
		_, err = store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), opts)
		assertDistinct(t, err, ErrConflict)
		assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
	})

	t.Run("not found", func(t *testing.T) {
		store := newToolAuthoringStore(t)
		slug := "tool-identity-not-found"
		boardRaw := toolAuthoringBoardFixture(slug, 5, true, toolUpdateTargetBlock())
		writeFixture(t, store.BoardPath(slug), boardRaw)
		board, err := store.ReadBoard(slug)
		if err != nil {
			t.Fatalf("read not-found source: %v", err)
		}
		title := "Missing"
		_, err = store.UpdateTool(slug, ToolUpdateRequest{ToolID: "tool_missing", Title: &title}, toolAuthoringAbsentOptions(board))
		assertDistinct(t, err, ErrNotFound)
		assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
	})

	t.Run("precondition required", func(t *testing.T) {
		store := newToolAuthoringStore(t)
		slug := "tool-identity-precondition"
		boardRaw := toolAuthoringBoardFixture(slug, 5, true, "")
		writeFixture(t, store.BoardPath(slug), boardRaw)
		board, err := store.ReadBoard(slug)
		if err != nil {
			t.Fatalf("read precondition source: %v", err)
		}
		opts := toolAuthoringAbsentOptions(board)
		opts.Layout = nil
		_, err = store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), opts)
		assertDistinct(t, err, ErrPreconditionRequired)
		assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
	})

	t.Run("unsupported schema", func(t *testing.T) {
		store := newToolAuthoringStore(t)
		slug := "tool-identity-unsupported"
		boardRaw := strings.Replace(toolAuthoringBoardFixture(slug, 5, true, ""), "schema = 2", "schema = 3", 1)
		writeFixture(t, store.BoardPath(slug), boardRaw)
		opts := ToolWriteOptions{
			Board:  WriteOptions{ExpectedETag: etag([]byte(boardRaw)), ExpectedRev: 5},
			Layout: &LayoutWriteExpectation{State: LayoutWriteAbsent},
		}
		_, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), opts)
		assertDistinct(t, err, ErrUnsupportedSchema)
		assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
	})

	t.Run("malformed current board is not candidate validation", func(t *testing.T) {
		store := newToolAuthoringStore(t)
		slug := "tool-identity-malformed-board"
		boardRaw := "schema = 2\nid = [\n"
		writeFixture(t, store.BoardPath(slug), boardRaw)
		opts := ToolWriteOptions{
			Board:  WriteOptions{ExpectedETag: etag([]byte(boardRaw)), ExpectedRev: 1},
			Layout: &LayoutWriteExpectation{State: LayoutWriteAbsent},
		}
		_, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), opts)
		if err == nil {
			t.Fatal("malformed current board accepted Tool mutation")
		}
		if errors.Is(err, ErrInvalidToolMutation) {
			t.Fatalf("malformed current board error %v was classified as candidate validation", err)
		}
		assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
	})

	t.Run("malformed current layout is not candidate validation", func(t *testing.T) {
		store := newToolAuthoringStore(t)
		slug := "tool-identity-malformed-layout"
		boardRaw := toolAuthoringBoardFixture(slug, 5, true, "")
		layoutRaw := "schema = 1\nboardId = [\n"
		writeFixture(t, store.BoardPath(slug), boardRaw)
		writeFixture(t, store.LayoutPath(slug), layoutRaw)
		board, err := store.ReadBoard(slug)
		if err != nil {
			t.Fatalf("read malformed-layout board: %v", err)
		}
		opts := ToolWriteOptions{
			Board:  WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev},
			Layout: &LayoutWriteExpectation{State: LayoutWritePresent, ETag: etag([]byte(layoutRaw))},
		}
		_, err = store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), opts)
		if err == nil {
			t.Fatal("malformed current layout accepted Tool mutation")
		}
		if errors.Is(err, ErrInvalidToolMutation) {
			t.Fatalf("malformed current layout error %v was classified as candidate validation", err)
		}
		assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
	})

	t.Run("publication uncertainty remains distinct", func(t *testing.T) {
		err := fmt.Errorf("publication context: %w", ErrDefinitionPublicationUncertain)
		assertDistinct(t, err, ErrDefinitionPublicationUncertain)
	})
}
