package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrote/server/internal/formations"
)

type fakeAgentLiveness struct {
	live []formations.LiveAgentSession
	err  error
}

func (f fakeAgentLiveness) LiveAgentSessions() ([]formations.LiveAgentSession, error) {
	return f.live, f.err
}

func TestAgentsHandlerListsPersonaRosterJoinedWithLiveSessions(t *testing.T) {
	agentsDir := t.TempDir()
	writeAgentFixture(t, agentsDir, "susie", `schema = 1

[card]
id = "susie"
display_name = "Susie"
kind = "specialist"
tags = ["design", "react", "taste:visual"]

[harness]
default = "claude-code"

[[harness.variant]]
id = "claude-code"
session_stem = "susie"
source = "/tmp/CLAUDE.md"
`)

	handler := NewAgentsHandler(agentsDir, fakeAgentLiveness{live: []formations.LiveAgentSession{
		{Name: "susie", Status: "idle", Attached: true},
		{Name: "scratch", Status: "working"},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()

	handler.ListAgents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Agents []formations.AgentProjection `json:"agents"`
			Count  int                          `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Success || body.Data.Count != 2 {
		t.Fatalf("response = %#v", body)
	}
	if body.Data.Agents[0].ID != "susie" || body.Data.Agents[0].Liveness != formations.AgentLivenessLive {
		t.Fatalf("first agent = %#v, want live susie", body.Data.Agents[0])
	}
	if body.Data.Agents[1].ID != "scratch" || !body.Data.Agents[1].Unbound || body.Data.Agents[1].Assignable {
		t.Fatalf("second agent = %#v, want unbound scratch", body.Data.Agents[1])
	}
	if strings.Contains(rec.Body.String(), "CLAUDE.md contents") ||
		strings.Contains(rec.Body.String(), "/tmp/CLAUDE.md") ||
		strings.Contains(rec.Body.String(), "harnessVariants") {
		t.Fatalf("roster response leaked source or harness internals: %s", rec.Body.String())
	}
}

func TestAgentsHandlerFiltersAssignableAndCapability(t *testing.T) {
	agentsDir := t.TempDir()
	writeAgentFixture(t, agentsDir, "susie", minimalAgentFixture("susie", "specialist", []string{"react", "taste:visual"}))
	writeAgentFixture(t, agentsDir, "codex", minimalAgentFixture("codex", "specialist", []string{"go"}))

	handler := NewAgentsHandler(agentsDir, fakeAgentLiveness{live: []formations.LiveAgentSession{{Name: "scratch", Status: "working"}}})
	req := httptest.NewRequest(http.MethodGet, "/api/agents?capable=react&assignable=1", nil)
	rec := httptest.NewRecorder()

	handler.ListAgents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "codex") || strings.Contains(rec.Body.String(), "scratch") {
		t.Fatalf("filtered roster included non-matching/unbound agents: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "susie") {
		t.Fatalf("filtered roster missing susie: %s", rec.Body.String())
	}
}

func TestAgentsHandlerInspectReturnsPointersWithoutInliningSource(t *testing.T) {
	agentsDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "CLAUDE.md")
	writeTextFile(t, sourcePath, "CLAUDE.md contents must stay out of API responses")
	writeAgentFixture(t, agentsDir, "susie", `schema = 1

[card]
id = "susie"
kind = "specialist"
tags = ["react"]

[harness]
default = "claude-code"

[[harness.variant]]
id = "claude-code"
session_stem = "susie"
source = "`+sourcePath+`"
`)

	handler := NewAgentsHandler(agentsDir, fakeAgentLiveness{})
	req := httptest.NewRequest(http.MethodGet, "/api/agents/susie", nil)
	req.SetPathValue("agentId", "susie")
	rec := httptest.NewRecorder()

	handler.GetAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, sourcePath) {
		t.Fatalf("inspect response missing source pointer: %s", body)
	}
	if strings.Contains(body, "CLAUDE.md contents") || strings.Contains(body, "toml") {
		t.Fatalf("inspect response leaked source contents or raw TOML: %s", body)
	}
}

func TestAgentsHandlerCreatesAndEditsThroughSharedWriter(t *testing.T) {
	agentsDir := t.TempDir()
	handler := NewAgentsHandler(agentsDir, fakeAgentLiveness{})

	createReq := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewBufferString(`{"id":"writer","kind":"specialist","harness":"claude-code","capabilities":["writing","voice"]}`))
	createRec := httptest.NewRecorder()
	handler.CreateAgent(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", createRec.Code, createRec.Body.String())
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/agents/writer", bytes.NewBufferString(`{"addCapability":"react","note":"ready for UI copy"}`))
	patchReq.SetPathValue("agentId", "writer")
	patchReq.Header.Set("If-Match", createRec.Header().Get("ETag"))
	patchRec := httptest.NewRecorder()
	handler.UpdateAgent(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", patchRec.Code, patchRec.Body.String())
	}
	raw := readAgentFixture(t, agentsDir, "writer")
	if !strings.Contains(raw, `"react"`) || !strings.Contains(raw, `text = "ready for UI copy"`) {
		t.Fatalf("shared writer did not persist edit:\n%s", raw)
	}
}

func TestAgentsHandlerRejectsStaleIfMatchWithoutClobber(t *testing.T) {
	agentsDir := t.TempDir()
	writeAgentFixture(t, agentsDir, "writer", minimalAgentFixture("writer", "specialist", []string{"writing"}))
	store := formations.NewPersonaStore(agentsDir)
	stale, err := store.ReadPersona("writer")
	if err != nil {
		t.Fatalf("read stale persona: %v", err)
	}
	handler := NewAgentsHandler(agentsDir, fakeAgentLiveness{})

	firstReq := httptest.NewRequest(http.MethodPatch, "/api/agents/writer", bytes.NewBufferString(`{"addCapability":"react"}`))
	firstReq.SetPathValue("agentId", "writer")
	firstReq.Header.Set("If-Match", stale.ETag)
	firstRec := httptest.NewRecorder()
	handler.UpdateAgent(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first patch status = %d, want 200: %s", firstRec.Code, firstRec.Body.String())
	}
	afterFirst, err := store.ReadPersona("writer")
	if err != nil {
		t.Fatalf("read after first edit: %v", err)
	}
	if !containsString(afterFirst.Tags, "react") {
		t.Fatalf("first edit tags = %#v, want react", afterFirst.Tags)
	}

	staleReq := httptest.NewRequest(http.MethodPatch, "/api/agents/writer", bytes.NewBufferString(`{"removeCapability":"react","addCapability":"go"}`))
	staleReq.SetPathValue("agentId", "writer")
	staleReq.Header.Set("If-Match", stale.ETag)
	staleRec := httptest.NewRecorder()
	handler.UpdateAgent(staleRec, staleReq)
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("stale patch status = %d, want 409: %s", staleRec.Code, staleRec.Body.String())
	}
	afterStale, err := store.ReadPersona("writer")
	if err != nil {
		t.Fatalf("read after stale edit: %v", err)
	}
	if !containsString(afterStale.Tags, "react") || containsString(afterStale.Tags, "go") {
		t.Fatalf("stale edit changed card tags = %#v, want react retained and go absent", afterStale.Tags)
	}
	if afterStale.ETag != afterFirst.ETag {
		t.Fatalf("stale edit changed ETag from %s to %s", afterFirst.ETag, afterStale.ETag)
	}
}

func TestAgentsHandlerDuplicateCreateFailsLoud(t *testing.T) {
	agentsDir := t.TempDir()
	writeAgentFixture(t, agentsDir, "writer", minimalAgentFixture("writer", "specialist", nil))
	handler := NewAgentsHandler(agentsDir, fakeAgentLiveness{})

	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewBufferString(`{"id":"writer","kind":"specialist","harness":"claude-code"}`))
	rec := httptest.NewRecorder()
	handler.CreateAgent(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentsHandlerRejectsOversizedJSONBody(t *testing.T) {
	agentsDir := t.TempDir()
	handler := NewAgentsHandler(agentsDir, fakeAgentLiveness{})
	body := `{"id":"writer","kind":"specialist","summary":"` + strings.Repeat("x", 2*1024*1024) + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.CreateAgent(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", rec.Code, rec.Body.String())
	}
}

func writeAgentFixture(t *testing.T, agentsDir, id, raw string) {
	t.Helper()
	writeTextFile(t, filepath.Join(agentsDir, id+".toml"), raw)
}

func readAgentFixture(t *testing.T, agentsDir, id string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(agentsDir, id+".toml"))
	if err != nil {
		t.Fatalf("read agent fixture: %v", err)
	}
	return string(b)
}

func writeTextFile(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func minimalAgentFixture(id, kind string, tags []string) string {
	return `schema = 1

[card]
id = "` + id + `"
kind = "` + kind + `"
tags = [` + formationsTestRenderStrings(tags) + `]

[harness]
default = "claude-code"

[[harness.variant]]
id = "claude-code"
session_stem = "` + id + `"
`
}

func formationsTestRenderStrings(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, `"`+value+`"`)
	}
	return strings.Join(parts, ", ")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
