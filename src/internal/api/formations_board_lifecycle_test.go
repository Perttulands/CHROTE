package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestFormationsHandlerCreatesNamedBlankBoardWithDerivedSlug(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/formations/boards", bytes.NewBufferString(`{"title":"  Release Plan  "}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Board formations.BoardDocument `json:"board"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rec.Body.String())
	}
	board := response.Data.Board
	if !response.Success || board.Slug != "release-plan" || board.Title != "Release Plan" || board.Rev != 1 {
		t.Fatalf("created board = %+v, want named blank release-plan", board)
	}
	if board.ETag == "" || rec.Header().Get("ETag") != board.ETag {
		t.Fatalf("ETag header/body = %q/%q, want matching durable ETag", rec.Header().Get("ETag"), board.ETag)
	}
	if board.TOML != "" || len(board.Formations) != 0 || len(board.Missions) != 0 || len(board.Gates) != 0 {
		t.Fatalf("created board leaked source or was not blank: %+v", board)
	}
}

func TestFormationsHandlerRejectsDuplicateDerivedBoardSlug(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	if _, err := store.CreateBoard(formations.BoardCreateRequest{Slug: "release-plan", Title: "Existing"}); err != nil {
		t.Fatalf("seed board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/formations/boards", bytes.NewBufferString(`{"title":"Release plan"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assertFormationsAPIToolError(t, rec, http.StatusConflict, "BOARD_EXISTS")
}

func TestFormationsHandlerDeletesBoardIntoArchive(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	board, err := store.CreateBoard(formations.BoardCreateRequest{Slug: "release-plan", Title: "Release Plan"})
	if err != nil {
		t.Fatalf("seed board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	blindReq := httptest.NewRequest(http.MethodDelete, "/api/formations/boards/release-plan", bytes.NewBufferString(`{"expectedRev":1}`))
	blindRec := httptest.NewRecorder()
	mux.ServeHTTP(blindRec, blindReq)
	if blindRec.Code != http.StatusPreconditionRequired {
		t.Fatalf("blind status = %d, want 428: %s", blindRec.Code, blindRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/formations/boards/release-plan", bytes.NewBufferString(`{"expectedRev":1}`))
	req.Header.Set("If-Match", board.ETag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Deletion formations.BoardDeletion `json:"deletion"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rec.Body.String())
	}
	if !response.Success || response.Data.Deletion.Slug != "release-plan" || response.Data.Deletion.ArchiveID == "" {
		t.Fatalf("response = %+v, want archived deletion", response)
	}
	boards, err := store.ListBoards()
	if err != nil || len(boards) != 0 {
		t.Fatalf("live boards after delete = %+v, err=%v", boards, err)
	}
}
