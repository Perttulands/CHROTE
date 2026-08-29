package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
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

const beadsPermissionHelperEnv = "CHROTE_BEADS_PERMISSION_HELPER"

func makeBeadsPermissionProject(t *testing.T) string {
	t.Helper()
	project, err := os.MkdirTemp("", "chrote-beads-permission-")
	if err != nil {
		t.Fatalf("create permission fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(project) })
	if err := os.Chmod(project, 0o755); err != nil {
		t.Fatalf("make permission fixture searchable: %v", err)
	}
	makeValidBeadsWorkspace(t, project)
	return project
}

func runBeadsPermissionSubprocess(t *testing.T, project string) bool {
	t.Helper()
	if os.Getenv(beadsPermissionHelperEnv) == "1" {
		return true
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = append(os.Environ(),
		beadsPermissionHelperEnv+"=1",
		"CHROTE_BEADS_PERMISSION_PROJECT="+project,
		"CHROTE_ROOTS="+project,
		"CHROTE_BEADS_WORKSPACES=",
		"CHROTE_BEADS_AUTO_DISCOVER=0",
	)
	if os.Geteuid() == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("unprivileged Beads permission probe: %v\n%s", err, output)
	}
	return false
}

func resetBeadsTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CHROTE_ROOTS", "")
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	t.Setenv("CHROTE_BEADS_AUTO_DISCOVER", "")
	t.Setenv("CHROTE_BD_COMMAND", "")
}

func makeFakeBdCommand(t *testing.T, output string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	scriptPath := filepath.Join(dir, "bd")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$BD_ARGS_FILE\"\n" +
		"printf '%s' \"$BD_OUTPUT\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake bd command: %v", err)
	}
	t.Setenv("BD_ARGS_FILE", argsPath)
	t.Setenv("BD_OUTPUT", output)
	t.Setenv("CHROTE_BD_COMMAND", scriptPath)
	return scriptPath, argsPath
}

func readFakeBdArgs(t *testing.T, argsPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake bd args: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
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

func TestBeadsHandler_ListProjectsIncludesConfiguredAndAllowedRootWorkspaces(t *testing.T) {
	rootDir := t.TempDir()
	rootWorkspace := filepath.Join(rootDir, "home-project")
	makeValidBeadsWorkspace(t, rootWorkspace)
	serviceWorkspace := filepath.Join(t.TempDir(), "srv")
	makeValidBeadsWorkspace(t, serviceWorkspace)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", serviceWorkspace)

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

	projectsByPath := map[string]string{}
	for _, project := range response.Data.Projects {
		projectsByPath[project.Path] = project.Source
	}
	if projectsByPath[serviceWorkspace] != "configured" {
		t.Fatalf("configured workspace source = %q, want configured", projectsByPath[serviceWorkspace])
	}
	if projectsByPath[rootWorkspace] != "auto" {
		t.Fatalf("allowed root workspace source = %q, want auto", projectsByPath[rootWorkspace])
	}
}

func TestBeadsHandler_ListProjectsCanDisableAutoDiscovery(t *testing.T) {
	for _, flag := range []string{"0", "false"} {
		flag := flag
		t.Run(flag, func(t *testing.T) {
			rootDir := t.TempDir()
			autoWorkspace := filepath.Join(rootDir, "auto")
			makeValidBeadsWorkspace(t, autoWorkspace)
			manualWorkspace := filepath.Join(rootDir, "manual")
			makeValidBeadsWorkspace(t, manualWorkspace)
			configuredWorkspace := filepath.Join(t.TempDir(), "configured")
			makeValidBeadsWorkspace(t, configuredWorkspace)

			t.Setenv("CHROTE_ROOTS", rootDir)
			t.Setenv("CHROTE_BEADS_WORKSPACES", configuredWorkspace)
			t.Setenv("CHROTE_BEADS_AUTO_DISCOVER", flag)

			handler := NewBeadsHandler()
			req := httptest.NewRequest(http.MethodGet, "/api/beads/projects?path="+manualWorkspace, nil)
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

			projectsByPath := map[string]string{}
			for _, project := range response.Data.Projects {
				projectsByPath[project.Path] = project.Source
			}
			if len(projectsByPath) != 2 {
				t.Fatalf("projects = %#v, want only configured and manual workspaces", projectsByPath)
			}
			if projectsByPath[configuredWorkspace] != "configured" {
				t.Fatalf("configured source = %q, want configured", projectsByPath[configuredWorkspace])
			}
			if projectsByPath[manualWorkspace] != "manual" {
				t.Fatalf("manual source = %q, want manual", projectsByPath[manualWorkspace])
			}
			if _, ok := projectsByPath[autoWorkspace]; ok {
				t.Fatalf("auto-discovered workspace %q was included with CHROTE_BEADS_AUTO_DISCOVER=%s", autoWorkspace, flag)
			}
		})
	}
}

func TestBeadsHandler_ListProjectsValidatesManualNestedWorkspace(t *testing.T) {
	rootDir := t.TempDir()
	nestedWorkspace := filepath.Join(rootDir, "research", "upstreams", "beads_viewer")
	makeValidBeadsWorkspace(t, nestedWorkspace)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")

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

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/issues?path="+partialProject, nil)
	rec := httptest.NewRecorder()
	handler.Issues(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Issues status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestBeadsHandler_IssuesSupportsExplicitStatusAndLimit(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	_, argsPath := makeFakeBdCommand(t, `[{"_type":"issue","id":"test-1","title":"Closed issue","status":"closed"}]`)

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/issues?path="+projectPath+"&status=all&limit=0", nil)
	rec := httptest.NewRecorder()
	handler.Issues(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Issues status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantArgs := []string{"--json", "list", "--status", "all", "--limit", "0"}
	if gotArgs := readFakeBdArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("bd args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestBeadsHandler_InsightsSupportsExplicitStatusAndLimit(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	_, argsPath := makeFakeBdCommand(t, `[{"_type":"issue","id":"test-1","title":"Closed issue","status":"closed"}]`)

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/insights?path="+projectPath+"&status=all&limit=0", nil)
	rec := httptest.NewRecorder()
	handler.Insights(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Insights status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantArgs := []string{"--json", "list", "--status", "all", "--limit", "0"}
	if gotArgs := readFakeBdArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("bd args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestBeadsHandler_IssueDetailRejectsInvalidWorkspaceBeforeRunningBd(t *testing.T) {
	rootDir := t.TempDir()
	partialProject := filepath.Join(rootDir, "partial")
	makePartialBeadsDirectory(t, partialProject)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/issue?path="+partialProject+"&id=test-1", nil)
	rec := httptest.NewRecorder()
	handler.IssueDetail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("IssueDetail status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestBeadsHandler_IssueDetailAcceptsBdShowArrayResponse(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	_, argsPath := makeFakeBdCommand(t, `[{"_type":"issue","id":"test-1","title":"Shown issue","status":"open"}]`)

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/issue?path="+projectPath+"&id=test-1", nil)
	rec := httptest.NewRecorder()
	handler.IssueDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("IssueDetail status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantArgs := []string{"--json", "show", "test-1"}
	if gotArgs := readFakeBdArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("bd args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestBeadsHandler_AddCommentPassesTextAsSingleArgument(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	_, argsPath := makeFakeBdCommand(t, `{"id":"comment-1","body":"ok"}`)

	handler := NewBeadsHandler()
	body := strings.NewReader(`{"path":"` + projectPath + `","id":"test-1","comment":"review this; $(touch /tmp/nope)"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/beads/comments", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.AddComment(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("AddComment status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantArgs := []string{"--json", "comments", "add", "test-1", "review this; $(touch /tmp/nope)"}
	if gotArgs := readFakeBdArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("bd args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestBeadsHandler_CheckBeadsDirectoryUnreadableReportsPermissionError(t *testing.T) {
	project := os.Getenv("CHROTE_BEADS_PERMISSION_PROJECT")
	if project == "" {
		project = makeBeadsPermissionProject(t)
		beadsPath := filepath.Join(project, ".beads")
		if err := os.Chmod(beadsPath, 0); err != nil {
			t.Fatalf("chmod .beads: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(beadsPath, 0o700) })
	}
	beadsPath := filepath.Join(project, ".beads")
	if !runBeadsPermissionSubprocess(t, project) {
		return
	}

	_, err := NewBeadsHandler().checkBeadsDirectory(project)
	if err == nil {
		t.Fatal("unreadable .beads was accepted as a workspace")
	}
	msg := err.Error()
	if !strings.Contains(msg, beadsPath) {
		t.Errorf("error does not name the workspace path %q: %q", beadsPath, msg)
	}
	if !strings.Contains(msg, effectiveUsername()) {
		t.Errorf("error does not name the effective user %q: %q", effectiveUsername(), msg)
	}
	if !strings.Contains(msg, "permission denied") {
		t.Errorf("error does not state the permission cause: %q", msg)
	}
	if strings.Contains(msg, "bd init") {
		t.Errorf("error suggests destructive re-init for possibly intact data: %q", msg)
	}
}

func TestBeadsHandler_CheckBeadsDirectoryUnsearchableParentReportsPermissionError(t *testing.T) {
	project := os.Getenv("CHROTE_BEADS_PERMISSION_PROJECT")
	if project == "" {
		project = makeBeadsPermissionProject(t)
		if err := os.Chmod(project, 0); err != nil {
			t.Fatalf("chmod project: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(project, 0o755) })
	}
	if !runBeadsPermissionSubprocess(t, project) {
		return
	}

	_, err := NewBeadsHandler().checkBeadsDirectory(project)
	if err == nil {
		t.Fatal("unsearchable project directory was accepted as a workspace")
	}
	msg := err.Error()
	if !strings.Contains(msg, filepath.Join(project, ".beads")) {
		t.Errorf("error does not name the workspace path: %q", msg)
	}
	if !strings.Contains(msg, effectiveUsername()) {
		t.Errorf("error does not name the effective user %q: %q", effectiveUsername(), msg)
	}
	if strings.Contains(msg, "bd init") {
		t.Errorf("error suggests destructive re-init for possibly intact data: %q", msg)
	}
}

func TestBeadsHandler_CheckBeadsDirectoryAbsentStillSuggestsBdInit(t *testing.T) {
	resetBeadsTestEnv(t)
	handler := NewBeadsHandler()

	_, err := handler.checkBeadsDirectory(t.TempDir())
	if err == nil {
		t.Fatal("missing .beads was accepted as a workspace")
	}
	if !strings.Contains(err.Error(), "bd init") {
		t.Errorf("missing workspace should still suggest 'bd init': %q", err.Error())
	}
}

func TestBeadsHandler_ListProjectsReportsPermissionErrorForUnreadableWorkspace(t *testing.T) {
	project := os.Getenv("CHROTE_BEADS_PERMISSION_PROJECT")
	if project == "" {
		project = makeBeadsPermissionProject(t)
		beadsPath := filepath.Join(project, ".beads")
		if err := os.Chmod(beadsPath, 0); err != nil {
			t.Fatalf("chmod .beads: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(beadsPath, 0o700) })
	}
	if !runBeadsPermissionSubprocess(t, project) {
		return
	}

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/projects?path="+project, nil)
	rec := httptest.NewRecorder()
	handler.ListProjects(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("ListProjects status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "permission denied") {
		t.Errorf("response does not state the permission cause: %s", body)
	}
	if !strings.Contains(body, effectiveUsername()) {
		t.Errorf("response does not name the effective user %q: %s", effectiveUsername(), body)
	}
	if strings.Contains(body, "bd init") {
		t.Errorf("response suggests destructive re-init for possibly intact data: %s", body)
	}
}
