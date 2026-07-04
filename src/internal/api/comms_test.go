package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrote/server/internal/comms"
)

func TestCommsHandlerProjectionUsesSuccessEnvelopeAndCanonicalModel(t *testing.T) {
	workspace := t.TempDir()
	writeCommsAPIFixture(t, workspace, "project", "dogfood", []map[string]any{
		{"seq": 1, "kind": "room_initialized", "actor": "room-system", "text": "Brief", "timestamp": "2026-07-04T00:00:01Z", "visible_to": []string{"Perttu", "Builder", "Reviewer"}, "metadata": map[string]any{}},
		{"seq": 2, "kind": "boundary_pinned", "actor": "Perttu", "text": "Use structured claims.", "timestamp": "2026-07-04T00:00:02Z", "visible_to": []string{"Perttu", "Builder", "Reviewer"}, "metadata": map[string]any{"pinned": true}},
		{"seq": 3, "kind": "task_claimed", "actor": "Builder", "text": "Build shell", "timestamp": "2026-07-04T00:00:03Z", "visible_to": []string{"Perttu", "Builder", "Reviewer"}, "metadata": map[string]any{"claim_status": "claimed", "reservations": []string{"ui/index.html"}}},
	})
	handler := NewCommsHandlerWithStore(comms.NewStore(workspace))
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/comms/rooms/project:dogfood/projection", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Success bool                 `json:"success"`
		Data    comms.RoomProjection `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rec.Body.String())
	}
	if !response.Success {
		t.Fatalf("success = false: %s", rec.Body.String())
	}
	if response.Data.Schema != "mission-room.projection.v1" || response.Data.RoomRef != "project:dogfood" {
		t.Fatalf("projection identity = %+v", response.Data)
	}
	if response.Data.LatestBoundary == nil || response.Data.LatestBoundary.Seq != 2 {
		t.Fatalf("latest boundary = %+v", response.Data.LatestBoundary)
	}
}

func TestCommsHandlerMessagesPaginatesAndDoesNotLeakPrivateInbox(t *testing.T) {
	workspace := t.TempDir()
	writeCommsAPIFixture(t, workspace, "project", "dogfood", []map[string]any{
		{"seq": 1, "kind": "room_initialized", "actor": "room-system", "text": "Brief", "timestamp": "2026-07-04T00:00:01Z", "visible_to": []string{"Perttu", "Reviewer"}, "metadata": map[string]any{}},
		{"seq": 2, "kind": "room_post", "actor": "Perttu", "text": "@Reviewer check this.", "timestamp": "2026-07-04T00:00:02Z", "visible_to": []string{"Perttu", "Reviewer"}, "metadata": map[string]any{}},
		{"seq": 3, "kind": "passive_mention", "actor": "room-system", "text": "Private mention", "timestamp": "2026-07-04T00:00:03Z", "visible_to": []string{"Reviewer"}, "to": []string{"Reviewer"}, "metadata": map[string]any{"mentioned": "Reviewer", "source_seq": 2}},
		{"seq": 4, "kind": "room_post", "actor": "Reviewer", "text": "Public response.", "timestamp": "2026-07-04T00:00:04Z", "visible_to": []string{"Perttu", "Reviewer"}, "metadata": map[string]any{}},
	})
	handler := NewCommsHandlerWithStore(comms.NewStore(workspace))
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	publicRec := httptest.NewRecorder()
	mux.ServeHTTP(publicRec, httptest.NewRequest(http.MethodGet, "/api/comms/rooms/project:dogfood/messages?since=1&limit=5", nil))
	if publicRec.Code != http.StatusOK {
		t.Fatalf("public status = %d: %s", publicRec.Code, publicRec.Body.String())
	}
	var publicResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Messages []comms.RoomMessage `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(publicRec.Body.Bytes(), &publicResponse); err != nil {
		t.Fatalf("decode public response: %v", err)
	}
	if got := messageSeqs(publicResponse.Data.Messages); len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Fatalf("public messages seqs = %v, want [2 4] without private mention", got)
	}

	privateRec := httptest.NewRecorder()
	mux.ServeHTTP(privateRec, httptest.NewRequest(http.MethodGet, "/api/comms/rooms/project:dogfood/messages?since=1&limit=2&includePrivateFor=Reviewer", nil))
	var privateResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Messages  []comms.RoomMessage `json:"messages"`
			NextSince int                 `json:"nextSince"`
		} `json:"data"`
	}
	if err := json.Unmarshal(privateRec.Body.Bytes(), &privateResponse); err != nil {
		t.Fatalf("decode private response: %v", err)
	}
	if got := messageSeqs(privateResponse.Data.Messages); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("private paged seqs = %v, want [2 3]", got)
	}
	if privateResponse.Data.NextSince != 3 {
		t.Fatalf("nextSince = %d, want last visible seq", privateResponse.Data.NextSince)
	}
}

func TestCommsHandlerDoesNotRegisterWriterRoutes(t *testing.T) {
	handler := NewCommsHandlerWithStore(comms.NewStore(t.TempDir()))
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/comms/rooms/project:dogfood/messages", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST messages status = %d, want 405 to keep first slice read-only", rec.Code)
	}
}

func TestCommsHandlerExportReturnsCanonicalReadOnlyEvents(t *testing.T) {
	workspace := t.TempDir()
	writeCommsAPIFixture(t, workspace, "project", "dogfood", []map[string]any{
		{"seq": 1, "kind": "room_initialized", "actor": "room-system", "text": "Brief", "timestamp": "2026-07-04T00:00:01Z", "visible_to": []string{"Perttu", "Reviewer"}, "metadata": map[string]any{}},
		{"seq": 2, "kind": "room_post", "actor": "Perttu", "text": "Ship the read-only projection.", "timestamp": "2026-07-04T00:00:02Z", "visible_to": []string{"Perttu", "Reviewer"}, "metadata": map[string]any{}},
	})
	handler := NewCommsHandlerWithStore(comms.NewStore(workspace))
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/comms/rooms/project:dogfood/export?format=markdown", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Success bool             `json:"success"`
		Data    comms.RoomExport `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode export response: %v", err)
	}
	if response.Data.Format != "markdown" || response.Data.RoomRef != "project:dogfood" {
		t.Fatalf("export identity = %+v", response.Data)
	}
	if !strings.Contains(response.Data.Markdown, "#002 `room_post` **Perttu**") {
		t.Fatalf("markdown export = %q", response.Data.Markdown)
	}
}

func messageSeqs(messages []comms.RoomMessage) []int {
	seqs := make([]int, 0, len(messages))
	for _, message := range messages {
		seqs = append(seqs, message.Seq)
	}
	return seqs
}

func writeCommsAPIFixture(t *testing.T, workspace, kind, id string, events []map[string]any) {
	t.Helper()
	path := filepath.Join(workspace, ".formations", "comms", kind, id+".ndjson")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
}
