package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestFormationsHandlerReadsAndWritesBoardNotesWithETagFences(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeBoardNotesAPIFixture(t, store.BoardPath("session-search"))
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/formations/boards/session-search/notes", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET notes status = %d, body=%s", get.Code, get.Body.String())
	}
	var empty struct {
		Data struct {
			Notes formations.BoardNotesDocument `json:"notes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode empty notes: %v", err)
	}
	if empty.Data.Notes.ETag != "*" || empty.Data.Notes.Board != "" {
		t.Fatalf("empty notes = %+v", empty.Data.Notes)
	}

	missingFence := httptest.NewRecorder()
	mux.ServeHTTP(missingFence, httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search/notes", bytes.NewBufferString(`{"target":"board","text":"shared"}`)))
	if missingFence.Code != http.StatusPreconditionRequired {
		t.Fatalf("PATCH without If-Match status = %d, body=%s", missingFence.Code, missingFence.Body.String())
	}

	patch := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search/notes", bytes.NewBufferString(`{"target":"board","text":"shared\nplan","updatedBy":"human:perttu"}`))
	patch.Header.Set("If-Match", "*")
	patched := httptest.NewRecorder()
	mux.ServeHTTP(patched, patch)
	if patched.Code != http.StatusOK {
		t.Fatalf("PATCH notes status = %d, body=%s", patched.Code, patched.Body.String())
	}
	var current struct {
		Data struct {
			Notes formations.BoardNotesDocument `json:"notes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(patched.Body.Bytes(), &current); err != nil {
		t.Fatalf("decode patched notes: %v", err)
	}
	if current.Data.Notes.Board != "shared\nplan" || current.Data.Notes.ETag == "*" || current.Data.Notes.UpdatedBy != "human:perttu" {
		t.Fatalf("patched notes = %+v", current.Data.Notes)
	}

	stale := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search/notes", bytes.NewBufferString(`{"target":"board","text":"stale"}`))
	stale.Header.Set("If-Match", "*")
	staleResponse := httptest.NewRecorder()
	mux.ServeHTTP(staleResponse, stale)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale PATCH status = %d, body=%s", staleResponse.Code, staleResponse.Body.String())
	}

	element := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search/notes", bytes.NewBufferString(`{"target":"fmn_frame","text":"keep this narrow"}`))
	element.Header.Set("If-Match", current.Data.Notes.ETag)
	elementResponse := httptest.NewRecorder()
	mux.ServeHTTP(elementResponse, element)
	if elementResponse.Code != http.StatusOK {
		t.Fatalf("element PATCH status = %d, body=%s", elementResponse.Code, elementResponse.Body.String())
	}
}

func TestFormationsHandlerRejectsUnknownBoardNoteTarget(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	writeBoardNotesAPIFixture(t, store.BoardPath("session-search"))
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search/notes", bytes.NewBufferString(`{"target":"fmn_missing","text":"nope"}`))
	request.Header.Set("If-Match", "*")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown note target status = %d, body=%s", response.Code, response.Body.String())
	}
}

func writeBoardNotesAPIFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o770); err != nil {
		t.Fatalf("mkdir board fixture: %v", err)
	}
	const fixture = `schema = 1
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
	if err := os.WriteFile(path, []byte(fixture), 0o660); err != nil {
		t.Fatalf("write board fixture: %v", err)
	}
}
