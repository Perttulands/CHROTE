package api

import (
	"encoding/json"
	"fmt"
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

	// The projects route asks bd for each project's prefix; no test reaches
	// for the real store to answer that.
	makeSequencedBdCommand(t, "[]")
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

	// The projects route asks bd for each project's prefix; no test reaches
	// for the real store to answer that.
	makeSequencedBdCommand(t, "[]")
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

	// The projects route asks bd for each project's prefix; no test reaches
	// for the real store to answer that.
	makeSequencedBdCommand(t, "[]")
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

			// The projects route asks bd for each project's prefix; no test reaches
			// for the real store to answer that.
			makeSequencedBdCommand(t, "[]")
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

	// The projects route asks bd for each project's prefix; no test reaches
	// for the real store to answer that.
	makeSequencedBdCommand(t, "[]")
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

// makeSequencedBdCommand fakes a bd that answers a series of calls: the card
// asks bd more than once, and the order of those questions is part of what the
// handler promises.
func makeSequencedBdCommand(t *testing.T, outputs ...string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	callPath := filepath.Join(dir, "calls.txt")
	scriptPath := filepath.Join(dir, "bd")
	for index, output := range outputs {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.json", index+1)), []byte(output), 0600); err != nil {
			t.Fatalf("write fake bd output: %v", err)
		}
	}
	script := "#!/bin/sh\n" +
		"printf '%s ' \"$@\" >> \"$BD_ARGS_FILE\"\n" +
		"printf '\\n' >> \"$BD_ARGS_FILE\"\n" +
		"n=$(cat \"$BD_CALL_FILE\" 2>/dev/null || echo 0)\n" +
		"n=$((n+1))\n" +
		"printf '%s' \"$n\" > \"$BD_CALL_FILE\"\n" +
		"cat \"$BD_OUTPUT_DIR/$n.json\" 2>/dev/null || printf '[]'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake bd command: %v", err)
	}
	t.Setenv("BD_ARGS_FILE", argsPath)
	t.Setenv("BD_CALL_FILE", callPath)
	t.Setenv("BD_OUTPUT_DIR", dir)
	t.Setenv("CHROTE_BD_COMMAND", scriptPath)
	return scriptPath, argsPath
}

func readSequencedBdCalls(t *testing.T, argsPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake bd args: %v", err)
	}
	calls := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			calls = append(calls, trimmed)
		}
	}
	return calls
}

func decodeBeadsData(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var envelope struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v: %s", err, rec.Body.String())
	}
	if !envelope.Success {
		t.Fatalf("response is not a success: %s", rec.Body.String())
	}
	return envelope.Data
}

func TestBeadPrefixReadsTheProjectOutOfAnID(t *testing.T) {
	cases := map[string]string{
		"chrote-5grx":    "chrote",
		"chrote-5grx.15": "chrote",
		"ctx-t4ak":       "ctx",
		"ctx-t4ak.1.2":   "ctx",
		"nothing":        "",
		"chrote-":        "",
	}
	for id, want := range cases {
		if got := beadPrefix(id); got != want {
			t.Errorf("beadPrefix(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestBeadsHandler_WorkRejectsInvalidWorkspaceBeforeRunningBd(t *testing.T) {
	rootDir := t.TempDir()
	partialProject := filepath.Join(rootDir, "partial")
	makePartialBeadsDirectory(t, partialProject)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/work?path="+partialProject, nil)
	rec := httptest.NewRecorder()
	handler.Work(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Work status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestBeadsHandler_WorkKeepsOpenWorkAndTheFinishedChildrenOfOpenEpics(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	work := `[
		{"_type":"issue","id":"test-ep1","title":"Epic","status":"open","issue_type":"epic","priority":1,
		 "acceptance_criteria":"Everything under it is done","updated_at":"2026-09-01T00:00:00Z"},
		{"_type":"issue","id":"test-ep1.1","title":"Blocked child","status":"open","issue_type":"task","priority":1,
		 "parent":"test-ep1","updated_at":"2026-09-02T00:00:00Z",
		 "dependencies":[{"depends_on_id":"test-ep1","type":"parent-child"},
		                 {"depends_on_id":"test-ep1.2","type":"blocks"},
		                 {"depends_on_id":"test-done","type":"blocks"}]},
		{"_type":"issue","id":"test-ep1.2","title":"Open blocker","status":"in_progress","issue_type":"task","priority":2,
		 "parent":"test-ep1","updated_at":"2026-09-03T00:00:00Z"},
		{"_type":"issue","id":"test-ep1.3","title":"Finished child","status":"closed","issue_type":"task","priority":2,
		 "parent":"test-ep1","updated_at":"2026-08-01T00:00:00Z"},
		{"_type":"issue","id":"test-done","title":"Finished elsewhere","status":"closed","issue_type":"task","priority":2}
	]`
	_, argsPath := makeSequencedBdCommand(t, work)

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/work?path="+projectPath, nil)
	rec := httptest.NewRecorder()
	handler.Work(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Work status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if calls := readSequencedBdCalls(t, argsPath); len(calls) != 1 || calls[0] != "--json list --status all --limit 0" {
		t.Fatalf("bd calls = %#v, want one full list", calls)
	}

	data := decodeBeadsData(t, rec)
	if data["prefix"] != "test" {
		t.Errorf("prefix = %v, want test", data["prefix"])
	}
	rows, ok := data["beads"].([]interface{})
	if !ok {
		t.Fatalf("beads is %T, want an array: %s", data["beads"], rec.Body.String())
	}
	byID := make(map[string]map[string]interface{}, len(rows))
	for _, row := range rows {
		bead, ok := row.(map[string]interface{})
		if !ok {
			t.Fatalf("row is %T, want an object", row)
		}
		byID[bead["id"].(string)] = bead
	}
	if _, unwanted := byID["test-done"]; unwanted {
		t.Errorf("a closed Bead outside an open epic is in the map: %s", rec.Body.String())
	}
	if _, wanted := byID["test-ep1.3"]; !wanted {
		t.Errorf("the finished child of an open epic is missing: %s", rec.Body.String())
	}
	if got := byID["test-ep1"]["acceptance"]; got != "Everything under it is done" {
		t.Errorf("epic acceptance = %v, want its criteria", got)
	}
	if _, present := byID["test-ep1.2"]["acceptance"]; present {
		t.Errorf("a task carries acceptance criteria the map never draws: %s", rec.Body.String())
	}
	blocked := byID["test-ep1.1"]
	if blocked["blocked"] != true {
		t.Errorf("blocked = %v, want true for a Bead waiting on open work", blocked["blocked"])
	}
	if got := blocked["blockedBy"]; !reflect.DeepEqual(got, []interface{}{"test-ep1.2"}) {
		t.Errorf("blockedBy = %#v, want only the blocker that is still open", got)
	}
	if got := blocked["parent"]; got != "test-ep1" {
		t.Errorf("parent = %v, want test-ep1", got)
	}
	if got := blocked["updated"]; got != "2026-09-02T00:00:00Z" {
		t.Errorf("updated = %v, want the bd timestamp", got)
	}
}

func TestBeadsHandler_ListProjectsCarriesTheBeadPrefix(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	t.Setenv("CHROTE_BEADS_AUTO_DISCOVER", "0")
	makeSequencedBdCommand(t, `[{"_type":"issue","id":"test-4ab","title":"Any Bead","status":"open"}]`)

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/projects?path="+projectPath, nil)
	rec := httptest.NewRecorder()
	handler.ListProjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListProjects status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	data := decodeBeadsData(t, rec)
	projects, ok := data["projects"].([]interface{})
	if !ok || len(projects) != 1 {
		t.Fatalf("projects = %v, want exactly one: %s", data["projects"], rec.Body.String())
	}
	project := projects[0].(map[string]interface{})
	if project["prefix"] != "test" {
		t.Errorf("prefix = %v, want test: %s", project["prefix"], rec.Body.String())
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

func TestBeadsHandler_IssueDetailReadsBothDirectionsAndTheParentChain(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	shown := `[{"_type":"issue","id":"test-ep1.1","title":"Shown","status":"open","issue_type":"task","priority":1,
		"parent":"test-ep1","description":"What it is","design":"How","acceptance_criteria":"Done when","notes":"Later",
		"updated_at":"2026-09-02T00:00:00Z",
		"dependencies":[{"id":"test-ep1","title":"Epic","status":"open","dependency_type":"parent-child"},
		                {"id":"test-blk","title":"Blocker","status":"open","dependency_type":"blocks"}]}]`
	dependents := `[{"id":"test-ep1.1.1","title":"Child","status":"open","issue_type":"task","priority":2,"dependency_type":"parent-child"},
		{"id":"test-waits","title":"Waiting","status":"open","issue_type":"task","priority":1,"dependency_type":"blocks"}]`
	parent := `[{"_type":"issue","id":"test-ep1","title":"Epic","status":"open","issue_type":"epic","priority":1}]`
	_, argsPath := makeSequencedBdCommand(t, shown, parent, dependents)

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/issue?path="+projectPath+"&id=test-ep1.1", nil)
	rec := httptest.NewRecorder()
	handler.IssueDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("IssueDetail status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantCalls := []string{
		"--json show test-ep1.1",
		"--json show test-ep1",
		"--json dep list test-ep1.1 --direction up",
	}
	if calls := readSequencedBdCalls(t, argsPath); !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("bd calls = %#v, want %#v", calls, wantCalls)
	}

	data := decodeBeadsData(t, rec)
	bead, ok := data["bead"].(map[string]interface{})
	if !ok {
		t.Fatalf("bead is %T, want an object: %s", data["bead"], rec.Body.String())
	}
	if bead["description"] != "What it is" || bead["design"] != "How" ||
		bead["acceptance"] != "Done when" || bead["notes"] != "Later" {
		t.Errorf("the card is missing the Bead's own text: %s", rec.Body.String())
	}
	links := map[string]string{"parents": "test-ep1", "children": "test-ep1.1.1", "blockedBy": "test-blk", "blocks": "test-waits"}
	for field, wantID := range links {
		items, ok := bead[field].([]interface{})
		if !ok || len(items) != 1 {
			t.Fatalf("%s = %v, want exactly one link: %s", field, bead[field], rec.Body.String())
		}
		if got := items[0].(map[string]interface{})["id"]; got != wantID {
			t.Errorf("%s links to %v, want %s", field, got, wantID)
		}
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
