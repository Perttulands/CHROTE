package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chrote/server/internal/core"
)

func makeValidBeadsWorkspace(t *testing.T, projectPath string) {
	t.Helper()
	beadsPath := filepath.Join(projectPath, ".beads")
	if err := os.MkdirAll(filepath.Join(beadsPath, "embeddeddolt"), 0700); err != nil {
		t.Fatalf("create embeddeddolt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsPath, "metadata.json"), []byte(`{"prefix":"test"}`), 0600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
}

func makePartialBeadsDirectory(t *testing.T, projectPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectPath, ".beads", "hooks"), 0700); err != nil {
		t.Fatalf("create partial .beads: %v", err)
	}
}

func resetBeadsTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CHROTE_ROOTS", "")
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	core.ResetConfigForTesting()
}

func TestBeadsHandler_CheckBeadsDirectoryRequiresModernWorkspace(t *testing.T) {
	resetBeadsTestEnv(t)
	handler := NewBeadsHandler()
	tempDir := t.TempDir()

	partialProject := filepath.Join(tempDir, "partial")
	makePartialBeadsDirectory(t, partialProject)
	if _, err := handler.checkBeadsDirectory(partialProject); err == nil {
		t.Fatal("partial .beads directory was accepted as a workspace")
	}

	validProject := filepath.Join(tempDir, "valid")
	makeValidBeadsWorkspace(t, validProject)
	if _, err := handler.checkBeadsDirectory(validProject); err != nil {
		t.Fatalf("valid Beads workspace was rejected: %v", err)
	}
}

func TestBeadsHandler_ListProjectsSkipsPartialBeadsDirectories(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("CHROTE_ROOTS", tempDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	core.ResetConfigForTesting()

	validProject := filepath.Join(tempDir, "valid")
	makeValidBeadsWorkspace(t, validProject)
	partialProject := filepath.Join(tempDir, "partial")
	makePartialBeadsDirectory(t, partialProject)

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/projects", nil)
	rec := httptest.NewRecorder()
	handler.ListProjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListProjects status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Projects []struct {
				Path string `json:"path"`
			} `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success {
		t.Fatal("expected success response")
	}
	if len(response.Data.Projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(response.Data.Projects))
	}
	if response.Data.Projects[0].Path != validProject {
		t.Fatalf("project path = %q, want %q", response.Data.Projects[0].Path, validProject)
	}
}

func TestBeadsHandler_ListProjectsAllowsConfiguredWorkspaceOutsideRoots(t *testing.T) {
	rootDir := t.TempDir()
	serviceWorkspace := filepath.Join(t.TempDir(), "srv")
	makeValidBeadsWorkspace(t, serviceWorkspace)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", serviceWorkspace)
	core.ResetConfigForTesting()

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/projects", nil)
	rec := httptest.NewRecorder()
	handler.ListProjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListProjects status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response struct {
		Data struct {
			Projects []struct {
				Path   string `json:"path"`
				Source string `json:"source"`
			} `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(response.Data.Projects))
	}
	if response.Data.Projects[0].Path != serviceWorkspace {
		t.Fatalf("project path = %q, want %q", response.Data.Projects[0].Path, serviceWorkspace)
	}
	if response.Data.Projects[0].Source != "configured" {
		t.Fatalf("project source = %q, want configured", response.Data.Projects[0].Source)
	}
}

func TestBeadsHandler_ListProjectsValidatesManualNestedWorkspace(t *testing.T) {
	rootDir := t.TempDir()
	nestedWorkspace := filepath.Join(rootDir, "research", "upstreams", "beads_viewer")
	makeValidBeadsWorkspace(t, nestedWorkspace)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	core.ResetConfigForTesting()

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/projects?path="+nestedWorkspace, nil)
	rec := httptest.NewRecorder()
	handler.ListProjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListProjects status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response struct {
		Data struct {
			Projects []struct {
				Path   string `json:"path"`
				Source string `json:"source"`
			} `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(response.Data.Projects))
	}
	if response.Data.Projects[0].Path != nestedWorkspace {
		t.Fatalf("project path = %q, want %q", response.Data.Projects[0].Path, nestedWorkspace)
	}
	if response.Data.Projects[0].Source != "manual" {
		t.Fatalf("project source = %q, want manual", response.Data.Projects[0].Source)
	}
}

func TestBeadsHandler_IssuesRejectsInvalidWorkspaceBeforeRunningBd(t *testing.T) {
	rootDir := t.TempDir()
	partialProject := filepath.Join(rootDir, "partial")
	makePartialBeadsDirectory(t, partialProject)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	core.ResetConfigForTesting()

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/issues?path="+partialProject, nil)
	rec := httptest.NewRecorder()
	handler.Issues(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Issues status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
