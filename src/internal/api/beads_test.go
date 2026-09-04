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
	manifestDir := filepath.Join(beadsPath, "embeddeddolt", "test", ".dolt", "noms")
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		t.Fatalf("create embeddeddolt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsPath, "metadata.json"), []byte(`{"prefix":"test"}`), 0600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest"), []byte("manifest\n"), 0600); err != nil {
		t.Fatalf("write Dolt manifest: %v", err)
	}
}

func makePartialBeadsDirectory(t *testing.T, projectPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectPath, ".beads", "hooks"), 0700); err != nil {
		t.Fatalf("create partial .beads: %v", err)
	}
}

const beadsPermissionHelperEnv = "CHROTE_BEADS_PERMISSION_HELPER"

// raceExitPromptly cancels the race runtime's exit delay in a re-exec'd child.
// Every permission probe re-execs this binary, and the child inherits the race
// instrumentation, whose default one-second drain dominated the whole Go suite.
const raceExitPromptly = "GORACE=atexit_sleep_ms=0"

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

// requireUnprivileged fails the child of a permission probe that is still root.
// A `chmod 0` fixture denies nothing to root, so a privileged child would prove
// nothing about the permission reporting it is here to pin. Without this the
// failure mode is silent, and these probes are the only place the effective-user
// wording is exercised at all.
func requireUnprivileged(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Fatal("permission probe is running as root; the unprivileged re-exec did not drop privileges and the fixture would deny nothing")
	}
}

func runBeadsPermissionSubprocess(t *testing.T, project string) bool {
	t.Helper()
	if os.Getenv(beadsPermissionHelperEnv) == "1" {
		requireUnprivileged(t)
		return true
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = append(os.Environ(),
		// The child is race-instrumented, and the race runtime sleeps a full
		// second before exiting so late reports are not lost. This probe reads
		// the child's exit status, not a trailing report, so that second is
		// pure waiting.
		raceExitPromptly,
		beadsPermissionHelperEnv+"=1",
		"CHROTE_BEADS_PERMISSION_PROJECT="+project,
		"CHROTE_ROOTS="+project,
		"CHROTE_BEADS_WORKSPACES=",
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
	req := httptest.NewRequest(http.MethodGet, "/api/beads/projects?path="+validProject+"&path="+partialProject, nil)
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
			Warnings []string `json:"warnings"`
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
	if len(response.Data.Warnings) != 1 || !strings.Contains(response.Data.Warnings[0], partialProject) {
		t.Fatalf("warnings = %v, want the partial workspace named", response.Data.Warnings)
	}
}

// A configured workspace is listed even though it sits outside every allowed
// root, and a manual one is listed as manual. The source is part of the
// answer, because the operator can remove one and not the other.
func TestBeadsHandler_ListProjectsListsConfiguredAndManualWorkspacesOnly(t *testing.T) {
	rootDir := t.TempDir()
	rootWorkspace := filepath.Join(rootDir, "home-project")
	makeValidBeadsWorkspace(t, rootWorkspace)
	manualWorkspace := filepath.Join(rootDir, "manual")
	makeValidBeadsWorkspace(t, manualWorkspace)
	serviceWorkspace := filepath.Join(t.TempDir(), "srv")
	makeValidBeadsWorkspace(t, serviceWorkspace)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", serviceWorkspace)

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
	if projectsByPath[serviceWorkspace] != "configured" {
		t.Fatalf("configured workspace source = %q, want configured", projectsByPath[serviceWorkspace])
	}
	if projectsByPath[manualWorkspace] != "manual" {
		t.Fatalf("manual workspace source = %q, want manual", projectsByPath[manualWorkspace])
	}
	// A store under a root is the workspace list's to find, not this route's.
	if len(response.Data.Projects) != 2 {
		t.Fatalf("projects = %#v, want exactly the configured and the manual workspace", projectsByPath)
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

// makeSequencedBdCommand fakes a bd that answers a series of calls: how many
// times a handler asks bd, and what it asks, is part of what it promises.
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
		"chrote-5grx":     "chrote",
		"chrote-5grx.15":  "chrote",
		"chrote-df247db5": "chrote",
		"ctx-t4ak":        "ctx",
		"ctx-t4ak.1.2":    "ctx",
		"nothing":         "",
		"chrote-":         "",
	}
	for id, want := range cases {
		if got := beadPrefix(id); got != want {
			t.Errorf("beadPrefix(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestBeadsHandler_WorkTreatsAnAbsentEightCharacterSameStoreBlockerAsFinished(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)
	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	work := `[{"_type":"issue","id":"chrote-5grx","title":"Ready after legacy blocker closed","status":"open","issue_type":"task","priority":1,"dependencies":[{"depends_on_id":"chrote-df247db5","type":"blocks"}]}]`
	_, argsPath := makeSequencedBdCommand(t, work)

	rec := httptest.NewRecorder()
	NewBeadsHandler().Work(rec, httptest.NewRequest(http.MethodGet, "/api/beads/work?path="+projectPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("Work status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if calls := readSequencedBdCalls(t, argsPath); !reflect.DeepEqual(calls, []string{"--json list --status open,in_progress,blocked,deferred --all"}) {
		t.Fatalf("bd calls = %#v, want the unchanged unfinished command", calls)
	}
	rows := decodeBeadsData(t, rec)["beads"].([]interface{})
	row := rows[0].(map[string]interface{})
	if row["blocked"] != false {
		t.Errorf("blocked = %v, want false for absent same-store blocker chrote-df247db5: %s", row["blocked"], rec.Body.String())
	}
	if blockers, present := row["blockedBy"]; present {
		t.Errorf("blockedBy = %v, want no active blockers: %s", blockers, rec.Body.String())
	}
}

// No read route runs bd against a directory that is not a workspace. No
// fake bd is installed on purpose: if either route reached for a command, it
// would find the operator's real bd and read a store the request never named.
func TestBeadsHandler_ReadRoutesRejectAnInvalidWorkspaceBeforeRunningBd(t *testing.T) {
	rootDir := t.TempDir()
	partialProject := filepath.Join(rootDir, "partial")
	makePartialBeadsDirectory(t, partialProject)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	t.Setenv("CHROTE_BD_COMMAND", filepath.Join(rootDir, "bd-that-does-not-exist"))

	handler := NewBeadsHandler()
	for _, testCase := range []struct {
		name string
		path string
		call func(http.ResponseWriter, *http.Request)
	}{
		{name: "work", path: "/api/beads/work?path=" + partialProject, call: handler.Work},
		{name: "closed", path: "/api/beads/closed?path=" + partialProject, call: handler.ClosedWork},
		{name: "issue detail", path: "/api/beads/issue?path=" + partialProject + "&id=test-1", call: handler.IssueDetail},
		{name: "formulas", path: "/api/beads/formulas?path=" + partialProject, call: handler.Formulas},
		{name: "formula detail", path: "/api/beads/formula?path=" + partialProject + "&name=test", call: handler.FormulaDetail},
		{name: "molecules", path: "/api/beads/molecules?path=" + partialProject, call: handler.Molecules},
		{name: "molecule detail", path: "/api/beads/molecule?path=" + partialProject + "&id=test-1", call: handler.MoleculeDetail},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			testCase.call(rec, httptest.NewRequest(http.MethodGet, testCase.path, nil))

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
			}
		})
	}
}

func TestBeadsHandler_WorkReportsMissingWorkspaceAsNotFound(t *testing.T) {
	rootDir := t.TempDir()
	project := filepath.Join(rootDir, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("create project: %v", err)
	}
	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	t.Setenv("CHROTE_BD_COMMAND", filepath.Join(rootDir, "bd-that-does-not-exist"))

	rec := httptest.NewRecorder()
	NewBeadsHandler().Work(rec, httptest.NewRequest(http.MethodGet, "/api/beads/work?path="+project, nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Work status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no .beads directory") {
		t.Fatalf("Work response does not describe the missing workspace: %s", rec.Body.String())
	}
}

func TestBeadsHandler_WorkLoadsOnlyUnfinishedWorkAndResolvesMissingSameStoreBlockers(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	work := `[
		{"_type":"issue","id":"test-ep1","title":"Epic","status":"open","issue_type":"epic","priority":1,
		 "acceptance_criteria":"Everything under it is done","updated_at":"2026-09-01T00:00:00Z","dependent_count":1},
		{"_type":"issue","id":"test-ep1.1","title":"Blocked child","status":"open","issue_type":"task","priority":1,
		 "parent":"test-ep1","updated_at":"2026-09-02T00:00:00Z",
		 "dependencies":[{"depends_on_id":"test-ep1","type":"parent-child"},
		                 {"depends_on_id":"test-ep1.2","type":"blocks"},
		                 {"depends_on_id":"test-done","type":"blocks"},
		                 {"depends_on_id":"other-far","type":"blocks"}]},
		{"_type":"issue","id":"test-ep1.2","title":"Open blocker","status":"in_progress","issue_type":"task","priority":2,
		 "parent":"test-ep1","updated_at":"2026-09-03T00:00:00Z","defer_until":"2099-01-01T00:00:00Z","dependency_count":1},
		{"_type":"issue","id":"test-orphan","title":"Orphan","status":"open","issue_type":"task","priority":3},
		{"_type":"issue","id":"test-ep1.3","title":"Closed child","status":"closed","issue_type":"task","priority":2,
		 "parent":"test-ep1","updated_at":"2026-09-01T00:00:00Z"}
	]`
	_, argsPath := makeSequencedBdCommand(t, work)

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/work?path="+projectPath, nil)
	rec := httptest.NewRecorder()
	handler.Work(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Work status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if calls := readSequencedBdCalls(t, argsPath); len(calls) != 1 || calls[0] != "--json list --status open,in_progress,blocked,deferred --all" {
		t.Fatalf("bd calls = %#v, want one complete unfinished list", calls)
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
	if len(byID) != 4 {
		t.Errorf("work rows = %d, want only the four unfinished rows: %s", len(byID), rec.Body.String())
	}
	if got := byID["test-ep1"]["acceptance"]; got != "Everything under it is done" {
		t.Errorf("epic acceptance = %v, want its criteria", got)
	}
	if _, present := byID["test-ep1.2"]["acceptance"]; present {
		t.Errorf("a task carries acceptance criteria the map never draws: %s", rec.Body.String())
	}
	if got := byID["test-ep1.2"]["deferUntil"]; got != "2099-01-01T00:00:00Z" {
		t.Errorf("deferUntil = %v, want the bd defer_until timestamp", got)
	}
	blocked := byID["test-ep1.1"]
	if blocked["blocked"] != true {
		t.Errorf("blocked = %v, want true for a Bead waiting on open work", blocked["blocked"])
	}
	if got := blocked["blockedBy"]; !reflect.DeepEqual(got, []interface{}{"test-ep1.2", "other-far"}) {
		t.Errorf("blockedBy = %#v, want the present unfinished blocker and absent foreign-store blocker", got)
	}
	if got := blocked["parent"]; got != "test-ep1" {
		t.Errorf("parent = %v, want test-ep1", got)
	}
	if got := blocked["updated"]; got != "2026-09-02T00:00:00Z" {
		t.Errorf("updated = %v, want the bd timestamp", got)
	}
	if byID["test-orphan"]["linked"] != false {
		t.Errorf("orphan linked = %v, want false", byID["test-orphan"]["linked"])
	}
	for _, id := range []string{"test-ep1", "test-ep1.1", "test-ep1.2"} {
		if byID[id]["linked"] != true {
			t.Errorf("%s linked = %v, want true", id, byID[id]["linked"])
		}
	}
}

func TestBeadsHandler_ClosedWorkLoadsLazilyAndFiltersFinishedStatuses(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)
	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	all := `[
		{"_type":"issue","id":"test-open","title":"Still open","status":"open","issue_type":"task","priority":1},
		{"_type":"issue","id":"test-closed","title":"Closed orphan","status":"closed","issue_type":"task","priority":2,"updated_at":"2026-09-03T00:00:00Z"},
		{"_type":"issue","id":"test-wont","title":"Won't fix","status":"wont_fix","issue_type":"bug","priority":3,"updated_at":"2026-09-04T00:00:00Z","dependency_count":1},
		{"_type":"issue","id":"test-dup","title":"Duplicate","status":"duplicate","issue_type":"epic","priority":1,"acceptance_criteria":"Historical context","updated_at":"2026-09-02T00:00:00Z","dependent_count":1},
		{"_type":"issue","id":"test-child","title":"Closed child","status":"closed","issue_type":"task","priority":2,"parent":"test-dup","updated_at":"2026-09-01T00:00:00Z"}
	]`
	_, argsPath := makeSequencedBdCommand(t, all)

	rec := httptest.NewRecorder()
	NewBeadsHandler().ClosedWork(rec, httptest.NewRequest(http.MethodGet, "/api/beads/closed?path="+projectPath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("ClosedWork status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if calls := readSequencedBdCalls(t, argsPath); !reflect.DeepEqual(calls, []string{"--json list --status all --all"}) {
		t.Fatalf("bd calls = %#v, want the lazy finished-work command", calls)
	}
	data := decodeBeadsData(t, rec)
	rows := data["beads"].([]interface{})
	if len(rows) != 4 {
		t.Fatalf("closed rows = %d, want 4 finished rows: %s", len(rows), rec.Body.String())
	}
	gotIDs := make([]string, 0, len(rows))
	for _, item := range rows {
		gotIDs = append(gotIDs, item.(map[string]interface{})["id"].(string))
	}
	if want := []string{"test-wont", "test-closed", "test-dup", "test-child"}; !reflect.DeepEqual(gotIDs, want) {
		t.Errorf("closed order = %v, want newest first %v", gotIDs, want)
	}
	if got := rows[2].(map[string]interface{})["acceptance"]; got != "Historical context" {
		t.Errorf("closed epic acceptance = %v, want Historical context", got)
	}
	linkedByID := make(map[string]bool, len(rows))
	for _, item := range rows {
		row := item.(map[string]interface{})
		linkedByID[row["id"].(string)] = row["linked"].(bool)
	}
	if linkedByID["test-closed"] {
		t.Error("closed orphan is linked, want false")
	}
	for _, id := range []string{"test-wont", "test-dup", "test-child"} {
		if !linkedByID[id] {
			t.Errorf("%s is not linked, want true", id)
		}
	}
}

func TestBeadsHandler_FormulaAndMoleculeRoutesPreserveCLIDataAndExactCommands(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)
	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	formulaSource := filepath.Join(projectPath, ".beads", "formulas", "release.formula.toml")
	formulaList := fmt.Sprintf(`[{"name":"release","type":"workflow","description":"Ship it","source":%q,"steps":2,"vars":1}]`, formulaSource)
	formulaDetail := fmt.Sprintf(`{"formula":"release","type":"workflow","description":"Ship it","source":%q,"vars":{"target":{"required":true}},"steps":[{"id":"build","title":"Build"},{"id":"ship","title":"Ship","depends_on":["build"]}]}`, formulaSource)
	moleculeList := `[{"_type":"issue","id":"test-proto","title":"Release template","status":"open","issue_type":"molecule","metadata":{"template":true}},{"_type":"issue","id":"test-mol","title":"Release run","status":"in_progress","issue_type":"molecule","source_formula":"release"}]`
	moleculeDetail := `{"root":{"id":"test-mol","title":"Release run","status":"in_progress","source_formula":"release"},"issues":[{"id":"test-mol.1","title":"Build","status":"closed"},{"id":"test-mol.2","title":"Ship","status":"open"}],"dependencies":[{"issue_id":"test-mol.2","depends_on_id":"test-mol.1","type":"blocks"}],"variables":{"target":"prod"}}`
	_, argsPath := makeSequencedBdCommand(t, formulaList, formulaDetail, moleculeList, moleculeDetail)
	handler := NewBeadsHandler()

	cases := []struct {
		path string
		call func(http.ResponseWriter, *http.Request)
		key  string
	}{
		{path: "/api/beads/formulas?path=" + projectPath, call: handler.Formulas, key: "formulas"},
		{path: "/api/beads/formula?path=" + projectPath + "&name=release", call: handler.FormulaDetail, key: "formula"},
		{path: "/api/beads/molecules?path=" + projectPath, call: handler.Molecules, key: "molecules"},
		{path: "/api/beads/molecule?path=" + projectPath + "&id=test-mol", call: handler.MoleculeDetail, key: "molecule"},
	}
	var data []map[string]interface{}
	for _, testCase := range cases {
		rec := httptest.NewRecorder()
		testCase.call(rec, httptest.NewRequest(http.MethodGet, testCase.path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d: %s", testCase.path, rec.Code, http.StatusOK, rec.Body.String())
		}
		decoded := decodeBeadsData(t, rec)
		if decoded[testCase.key] == nil {
			t.Fatalf("%s has no %s: %s", testCase.path, testCase.key, rec.Body.String())
		}
		data = append(data, decoded)
	}

	wantCalls := []string{
		"--json formula list",
		"--json formula show release",
		"--json list --type molecule --all --include-templates",
		"--json mol show test-mol",
	}
	if calls := readSequencedBdCalls(t, argsPath); !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("bd calls = %#v, want %#v", calls, wantCalls)
	}
	listedFormula := data[0]["formulas"].([]interface{})[0].(map[string]interface{})
	if listedFormula["source"] != formulaSource {
		t.Errorf("formula list source = %v, want %s", listedFormula["source"], formulaSource)
	}
	shownFormula := data[1]["formula"].(map[string]interface{})
	if shownFormula["source"] != formulaSource || len(shownFormula["steps"].([]interface{})) != 2 {
		t.Errorf("formula detail lost provenance or steps: %#v", shownFormula)
	}
	shownMolecule := data[3]["molecule"].(map[string]interface{})
	if shownMolecule["variables"].(map[string]interface{})["target"] != "prod" || len(shownMolecule["dependencies"].([]interface{})) != 1 {
		t.Errorf("molecule detail lost state or dependencies: %#v", shownMolecule)
	}
}

func TestBeadsHandler_FormulasTurnsNullIntoAnEmptyList(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)
	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	makeSequencedBdCommand(t, "null")

	rec := httptest.NewRecorder()
	NewBeadsHandler().Formulas(rec, httptest.NewRequest(http.MethodGet, "/api/beads/formulas?path="+projectPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("Formulas status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if formulas := decodeBeadsData(t, rec)["formulas"].([]interface{}); len(formulas) != 0 {
		t.Fatalf("formulas = %#v, want empty list", formulas)
	}
}

func TestBeadsHandler_FormulasReportsAnUnreadableProjectRegistryInsteadOfEmpty(t *testing.T) {
	projectPath := os.Getenv("CHROTE_BEADS_PERMISSION_PROJECT")
	if projectPath == "" {
		projectPath = makeBeadsPermissionProject(t)
		formulaDir := filepath.Join(projectPath, ".beads", "formulas")
		if err := os.MkdirAll(formulaDir, 0o700); err != nil {
			t.Fatalf("create formula registry: %v", err)
		}
		fakeBd := filepath.Join(projectPath, "bd-null")
		if err := os.WriteFile(fakeBd, []byte("#!/bin/sh\nprintf 'null'\n"), 0o755); err != nil {
			t.Fatalf("write fake bd: %v", err)
		}
		t.Setenv("CHROTE_BD_COMMAND", fakeBd)
		if err := os.Chmod(formulaDir, 0); err != nil {
			t.Fatalf("make formula registry unreadable: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(formulaDir, 0o700) })
	}
	if !runBeadsPermissionSubprocess(t, projectPath) {
		return
	}

	rec := httptest.NewRecorder()
	NewBeadsHandler().Formulas(rec, httptest.NewRequest(http.MethodGet, "/api/beads/formulas?path="+projectPath, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("Formulas status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	formulaDir := filepath.Join(projectPath, ".beads", "formulas")
	if body := rec.Body.String(); !strings.Contains(body, formulaDir) || !strings.Contains(body, "permission denied") || !strings.Contains(body, effectiveUsername()) {
		t.Errorf("Formulas did not identify the unreadable registry, user, and cause: %s", body)
	}
}

func TestBeadsHandler_ListProjectsCarriesTheBeadPrefix(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
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

func TestBeadsHandler_IssueDetailReadsTheWholeCardFromOneList(t *testing.T) {
	rootDir := t.TempDir()
	projectPath := filepath.Join(rootDir, "project")
	makeValidBeadsWorkspace(t, projectPath)

	t.Setenv("CHROTE_ROOTS", rootDir)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	list := `[
		{"_type":"issue","id":"test-root","title":"Root","status":"open","issue_type":"epic","priority":0},
		{"_type":"issue","id":"test-ep1","title":"Epic","status":"open","issue_type":"epic","priority":1,"parent":"test-root",
		 "dependencies":[{"depends_on_id":"test-root","type":"parent-child"}]},
		{"_type":"issue","id":"test-ep1.1","title":"Shown","status":"open","issue_type":"task","priority":1,
		 "parent":"test-ep1","description":"What it is","design":"How","acceptance_criteria":"Done when","notes":"Later",
		 "updated_at":"2026-09-02T00:00:00Z",
		 "dependencies":[{"depends_on_id":"test-ep1","type":"parent-child"},
		                 {"depends_on_id":"test-blk","type":"blocks"},
		                 {"depends_on_id":"other-far","type":"blocks"}]},
		{"_type":"issue","id":"test-ep1.1.2","title":"Later child","status":"open","issue_type":"task","priority":2,"parent":"test-ep1.1"},
		{"_type":"issue","id":"test-ep1.1.1","title":"Child","status":"open","issue_type":"task","priority":2,"parent":"test-ep1.1"},
		{"_type":"issue","id":"test-blk","title":"Blocker","status":"open","issue_type":"task","priority":1},
		{"_type":"issue","id":"test-waits","title":"Waiting","status":"open","issue_type":"task","priority":1,
		 "dependencies":[{"depends_on_id":"test-ep1.1","type":"blocks"}]}
	]`
	_, argsPath := makeSequencedBdCommand(t, list, list)

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/issue?path="+projectPath+"&id=test-ep1.1", nil)
	rec := httptest.NewRecorder()
	handler.IssueDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("IssueDetail status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	// The whole card, whatever the depth of its chain, is one bd process.
	wantCalls := []string{"--json list --status all --limit 0"}
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
	links := map[string][]string{
		"parents":   {"test-ep1", "test-root"},
		"children":  {"test-ep1.1.1", "test-ep1.1.2"},
		"blockedBy": {"test-blk", "other-far"},
		"blocks":    {"test-waits"},
	}
	for field, wantIDs := range links {
		items, ok := bead[field].([]interface{})
		if !ok {
			t.Fatalf("%s = %v, want links: %s", field, bead[field], rec.Body.String())
		}
		gotIDs := make([]string, 0, len(items))
		for _, item := range items {
			gotIDs = append(gotIDs, item.(map[string]interface{})["id"].(string))
		}
		if !reflect.DeepEqual(gotIDs, wantIDs) {
			t.Errorf("%s links to %v, want %v", field, gotIDs, wantIDs)
		}
	}
	if got := bead["parents"].([]interface{})[0].(map[string]interface{})["title"]; got != "Epic" {
		t.Errorf("the nearest parent is named %v, want its title from the list", got)
	}

	// An id the store does not hold is a plain not-found, not a bd failure.
	rec = httptest.NewRecorder()
	handler.IssueDetail(rec, httptest.NewRequest(http.MethodGet, "/api/beads/issue?path="+projectPath+"&id=test-none", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("IssueDetail status for a missing id = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
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

// An unreadable workspace is reported as a permission problem, in the check and
// on the wire, and never as a missing one: telling an operator to run `bd init`
// over a store whose data is intact is how the data gets destroyed. The check
// and the route share one fixture and one unprivileged child, because a `chmod
// 0` directory denies root nothing and the re-exec is what makes it deny.
func TestBeadsHandler_UnreadableWorkspaceReportsPermissionRatherThanAbsence(t *testing.T) {
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
	metadataPath := filepath.Join(beadsPath, "metadata.json")
	if !runBeadsPermissionSubprocess(t, project) {
		return
	}

	handler := NewBeadsHandler()

	_, err := handler.checkBeadsDirectory(project)
	if err == nil {
		t.Fatal("unreadable .beads was accepted as a workspace")
	}
	msg := err.Error()
	if !strings.Contains(msg, metadataPath) {
		t.Errorf("error does not name the unreadable file %q: %q", metadataPath, msg)
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

	readRoutes := []struct {
		name string
		path string
		call func(http.ResponseWriter, *http.Request)
	}{
		{name: "work", path: "/api/beads/work?path=" + project, call: handler.Work},
		{name: "closed", path: "/api/beads/closed?path=" + project, call: handler.ClosedWork},
		{name: "issue detail", path: "/api/beads/issue?path=" + project + "&id=test-1", call: handler.IssueDetail},
		{name: "formulas", path: "/api/beads/formulas?path=" + project, call: handler.Formulas},
		{name: "formula detail", path: "/api/beads/formula?path=" + project + "&name=test", call: handler.FormulaDetail},
		{name: "molecules", path: "/api/beads/molecules?path=" + project, call: handler.Molecules},
		{name: "molecule detail", path: "/api/beads/molecule?path=" + project + "&id=test-1", call: handler.MoleculeDetail},
	}
	for _, route := range readRoutes {
		t.Run(route.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route.path, nil)
			rec := httptest.NewRecorder()
			route.call(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, metadataPath) {
				t.Errorf("response does not name the unreadable file %q: %s", metadataPath, body)
			}
			if !strings.Contains(body, "permission denied") {
				t.Errorf("response does not state the permission cause: %s", body)
			}
		})
	}

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

// The Beads routes use the success/data envelope, unlike the flat health and
// tmux endpoints. The dashboard unwraps `data` for these and does not for those,
// so which shape an endpoint serves is a contract, not a detail.
func TestBeadsHandler_HealthUsesTheSuccessDataEnvelope(t *testing.T) {
	resetBeadsTestEnv(t)
	makeSequencedBdCommand(t, "bd version 1.2.3\n")

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	response := decodeJSONMap(t, rec)
	assertTopLevelKeys(t, response, []string{"data", "success", "timestamp"})
	if response["success"] != true {
		t.Fatalf("success = %v, want true", response["success"])
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data = %T, want object", response["data"])
	}
	assertTopLevelKeys(t, data, []string{"allowedRoots", "bdVersion", "configuredWorkspaces", "status"})
}
