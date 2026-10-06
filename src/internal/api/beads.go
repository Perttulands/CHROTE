// Package api provides HTTP handlers for the API
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chrote/server/internal/core"
)

// BeadsHandler handles beads-related API endpoints
type BeadsHandler struct {
	bdCommand     string
	execTimeout   time.Duration
	execSlots     chan struct{}
	prefixSlots   chan struct{}
	optionalSlots chan struct{}

	stores *beadsStoreReader
}

// NewBeadsHandler creates a new BeadsHandler
func NewBeadsHandler() *BeadsHandler {
	bdCommand := os.Getenv("CHROTE_BD_COMMAND")
	if bdCommand == "" {
		bdCommand = "bd"
	}

	h := &BeadsHandler{
		bdCommand:     bdCommand,
		execTimeout:   60 * time.Second,
		execSlots:     make(chan struct{}, 4),
		prefixSlots:   make(chan struct{}, 2),
		optionalSlots: make(chan struct{}, 2),
	}
	h.stores = newBeadsStoreReader(h)
	return h
}

// RegisterRoutes registers the beads routes on the given mux
func (h *BeadsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/beads/health", h.Health)
	mux.HandleFunc("GET /api/beads/projects", h.ListProjects)
	mux.HandleFunc("GET /api/beads/work", h.Work)
	mux.HandleFunc("GET /api/beads/state", h.State)
	mux.HandleFunc("GET /api/beads/closed", h.ClosedWork)
	mux.HandleFunc("GET /api/beads/issue", h.IssueDetail)
	mux.HandleFunc("POST /api/beads/issues", h.CreateIssue)
	mux.HandleFunc("GET /api/beads/formulas", h.Formulas)
	mux.HandleFunc("GET /api/beads/formula", h.FormulaDetail)
	mux.HandleFunc("GET /api/beads/molecules", h.Molecules)
	mux.HandleFunc("GET /api/beads/molecule", h.MoleculeDetail)
}

// getBdVersion returns the bd version or error.
func (h *BeadsHandler) getBdVersion(ctx context.Context) (string, error) {
	output, err := h.runBd(ctx, "", 5*time.Second, "version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// checkBeadsDirectory verifies the project carries a readable modern bd workspace
func (h *BeadsHandler) checkBeadsDirectory(projectPath string) (string, error) {
	beadsPath := filepath.Join(projectPath, ".beads")
	info, err := os.Stat(beadsPath)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return "", beadsPermissionError(beadsPath, err)
		}
		return "", fmt.Errorf("no .beads directory found in %s. Run 'bd init' to create one", projectPath)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("no .beads directory found in %s. Run 'bd init' to create one", projectPath)
	}
	metadataPath := filepath.Join(beadsPath, "metadata.json")
	doltPath := filepath.Join(beadsPath, "embeddeddolt")
	metaInfo, metaErr := os.Stat(metadataPath)
	doltInfo, doltErr := os.Stat(doltPath)
	// An unreadable workspace must never be reported as a missing one: the data
	// may be intact, and the 'bd init' suggestion below invites a destructive
	// re-init (bd init --force discards the workspace).
	if errors.Is(metaErr, fs.ErrPermission) {
		return "", beadsPermissionError(metadataPath, metaErr)
	}
	if errors.Is(doltErr, fs.ErrPermission) {
		return "", beadsPermissionError(doltPath, doltErr)
	}
	if metaErr != nil || !metaInfo.Mode().IsRegular() || doltErr != nil || !doltInfo.IsDir() {
		return "", fmt.Errorf("%s exists but is not a modern bd workspace. Run 'bd init' in %s", beadsPath, projectPath)
	}
	return beadsPath, nil
}

func beadsPermissionError(beadsPath string, cause error) error {
	return fmt.Errorf("cannot access %s as user %s: %w. The workspace may be intact — fix the directory permissions or ACLs instead of re-initializing", beadsPath, effectiveUsername(), cause)
}

// writeBeadsDirectoryError keeps every route honest about an unreadable store:
// permission failure means forbidden, while an absent or incomplete workspace
// remains not found.
func writeBeadsDirectoryError(w http.ResponseWriter, err error) {
	if errors.Is(err, fs.ErrPermission) {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}
	core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
}

func effectiveUsername() string {
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	return fmt.Sprintf("uid %d", os.Geteuid())
}

func configuredBeadsWorkspaces() []string {
	raw := os.Getenv("CHROTE_BEADS_WORKSPACES")
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	workspaces := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		resolved, err := filepath.Abs(part)
		if err != nil {
			continue
		}
		workspaces = append(workspaces, resolved)
	}
	return workspaces
}

func isPathUnder(path string, roots []string) bool {
	for _, root := range roots {
		if core.IsPathUnderRoot(path, root) {
			return true
		}
	}
	return false
}

func isConfiguredBeadsWorkspace(path string) bool {
	for _, workspace := range configuredBeadsWorkspaces() {
		if path == workspace {
			return true
		}
	}
	return false
}

func validateBeadsProjectPath(inputPath string) (string, string, string) {
	if inputPath == "" {
		return "", "BAD_REQUEST", "Missing required parameter: path"
	}

	resolved, err := filepath.Abs(inputPath)
	if err != nil {
		return "", "BAD_REQUEST", "Invalid path: " + err.Error()
	}

	if !isPathUnder(resolved, core.GetAllowedRoots()) && !isConfiguredBeadsWorkspace(resolved) {
		return "", "FORBIDDEN", "Project path not in allowed roots or configured Beads workspaces: " + resolved
	}

	if _, err := os.Stat(resolved); os.IsNotExist(err) {
		return "", "NOT_FOUND", "Project path does not exist: " + resolved
	}

	return resolved, "", ""
}

func projectName(projectPath string) string {
	name := filepath.Base(projectPath)
	if name == "." || name == string(os.PathSeparator) {
		return projectPath
	}
	return name
}

func (h *BeadsHandler) appendProject(projects *[]map[string]interface{}, seen map[string]bool, projectPath, source string) error {
	resolved, err := filepath.Abs(projectPath)
	if err != nil {
		return err
	}
	if seen[resolved] {
		return nil
	}
	beadsPath, err := h.checkBeadsDirectory(resolved)
	if err != nil {
		return err
	}
	*projects = append(*projects, map[string]interface{}{
		"name":      projectName(resolved),
		"path":      resolved,
		"beadsPath": beadsPath,
		"source":    source,
	})
	seen[resolved] = true
	return nil
}

// runBd owns every Beads subprocess, including the wrapper's descendants.
// Waiting for a slot consumes the same budget as execution; a busy optional
// component must not exhaust the server's process allowance.
func (h *BeadsHandler) runBd(ctx context.Context, projectPath string, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	costs := beadsCostsFromContext(ctx)
	admissionStarted := time.Now()
	admitted := false
	defer func() {
		if costs != nil && !admitted {
			costs.bdAdmission += time.Since(admissionStarted)
		}
	}()

	releaseOptional, err := h.admitOptionalBd(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseOptional()
	select {
	case h.execSlots <- struct{}{}:
		defer func() { <-h.execSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if costs != nil {
		costs.bdAdmission += time.Since(admissionStarted)
	}
	admitted = true

	cmd := exec.CommandContext(ctx, h.bdCommand, args...)
	cmd.Dir = projectPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Give the writer wrapper a catchable exit so its permission cleanup
		// can run. WaitDelay bounds this grace; force all descendants below.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	// A wrapper can exit while its descendants still hold the output pipes.
	cmd.WaitDelay = time.Second
	processStarted := time.Now()
	output, err := cmd.Output()
	if costs != nil {
		costs.bdProcess += time.Since(processStarted)
		if cmd.Process != nil {
			costs.sourceCalls++
		}
	}
	if err != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return output, err
}

// execBdJSON runs a bd command with --json and returns parsed JSON.
func (h *BeadsHandler) execBdJSON(ctx context.Context, projectPath string, args ...string) (interface{}, error) {
	cmdArgs := append([]string{"--json"}, args...)
	output, err := h.runBd(ctx, projectPath, h.execTimeout, cmdArgs...)
	// ErrWaitDelay means a successful exit left descendant pipes open. runBd
	// already bounded and cleaned that group and prioritized context failure;
	// only a complete JSON result may proceed to the caller's shape checks.
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("bd %s timed out: %w", strings.Join(args, " "), err)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("bd %s failed: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("bd %s failed: %v", strings.Join(args, " "), err)
	}

	var result interface{}
	decodeStarted := time.Now()
	err = json.Unmarshal(output, &result)
	if costs := beadsCostsFromContext(ctx); costs != nil {
		costs.jsonDecode += time.Since(decodeStarted)
	}
	if err != nil {
		return nil, fmt.Errorf("bd %s returned invalid JSON: %v. Output: %s", strings.Join(args, " "), err, string(output)[:min(200, len(output))])
	}

	return result, nil
}

// execBdIssues runs a bd command that returns a JSON array of issue objects.
func (h *BeadsHandler) execBdIssues(ctx context.Context, projectPath string, args ...string) ([]map[string]interface{}, error) {
	result, err := h.execBdJSON(ctx, projectPath, args...)
	if err != nil {
		return nil, err
	}

	// bd writes a bare array for some filters and {"issues": [...]} for
	// others; both are the same list.
	if envelope, ok := result.(map[string]interface{}); ok {
		if _, listed := envelope["issues"]; listed {
			result = envelope["issues"]
		}
	}
	items, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("bd %s returned %T, expected JSON array", strings.Join(args, " "), result)
	}

	issues := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if typ, ok := obj["_type"].(string); ok && typ != "issue" {
			continue
		}
		issues = append(issues, obj)
	}

	return issues, nil
}

func requiredIssueID(r *http.Request) (string, string, string) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		return "", "BAD_REQUEST", "Missing required parameter: id"
	}
	return id, "", ""
}

// requiredQueryValue reads a value that becomes a positional argument to bd.
// A leading dash is refused here rather than handed over: exec.Command runs no
// shell, but bd's own flag parser would read "--file=/tmp/x" as an option
// instead of as the name or id it was asked for.
func requiredQueryValue(r *http.Request, key string) (string, string, string) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return "", "BAD_REQUEST", "Missing required parameter: " + key
	}
	if strings.HasPrefix(value, "-") {
		return "", "BAD_REQUEST", key + " must not start with a dash: " + value
	}
	return value, "", ""
}

func (h *BeadsHandler) requestProject(w http.ResponseWriter, r *http.Request) (string, bool) {
	return h.checkedProject(w, r.URL.Query().Get("path"))
}

// checkedProject applies the same path and store checks to query parameters
// on read routes and the named destination in a creation request.
func (h *BeadsHandler) checkedProject(w http.ResponseWriter, path string) (string, bool) {
	projectPath, code, msg := validateBeadsProjectPath(path)
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return "", false
	}
	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		writeBeadsDirectoryError(w, err)
		return "", false
	}
	return projectPath, true
}

// CreateIssue handles POST /api/beads/issues. The store remains authoritative:
// bd creates the issue and chooses its id and default priority.
func (h *BeadsHandler) CreateIssue(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path        string `json:"path"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid issue JSON: "+err.Error())
		return
	}
	projectPath, ok := h.checkedProject(w, request.Path)
	if !ok {
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	if request.Title == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Title must not be empty")
		return
	}
	if request.Type != "bug" && request.Type != "feature" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Type must be bug or feature")
		return
	}

	// Binding each value with '=' keeps leading dashes inside the text rather
	// than letting bd's flag parser interpret them as options.
	result, err := h.execBdJSON(r.Context(), projectPath, "create",
		"--title="+request.Title, "--description="+request.Description, "--type="+request.Type)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}
	issue, ok := result.(map[string]interface{})
	if !ok || beadString(issue, "id") == "" || beadString(issue, "title") == "" {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", "bd create returned no issue id or title")
		return
	}
	core.WriteSuccess(w, map[string]interface{}{
		"id":    issue["id"],
		"title": issue["title"],
	})
}

// transformIssue converts raw JSONL issue to frontend-expected format

// Health handles GET /api/beads/health
func (h *BeadsHandler) Health(w http.ResponseWriter, r *http.Request) {
	version, err := h.getBdVersion(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusServiceUnavailable, "BD_NOT_INSTALLED",
			"bd command not found. Install modern Beads and ensure it is on CHROTE's PATH.")
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"status":               "ok",
		"bdVersion":            version,
		"allowedRoots":         core.GetAllowedRoots(),
		"configuredWorkspaces": configuredBeadsWorkspaces(),
	})
}

// ListProjects handles GET /api/beads/projects: the configured Beads projects
// and the manual paths the request names, each validated as a modern store.
// Discovery under the roots is the workspace list's job.
func (h *BeadsHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	var projects []map[string]interface{}
	var warnings []string
	seen := make(map[string]bool)
	configuredWorkspaces := configuredBeadsWorkspaces()

	for _, workspace := range configuredWorkspaces {
		if err := h.appendProject(&projects, seen, workspace, "configured"); err != nil {
			warnings = append(warnings, "Configured Beads workspace is invalid: "+workspace+": "+err.Error())
		}
	}

	for _, projectPath := range r.URL.Query()["path"] {
		resolved, code, msg := validateBeadsProjectPath(projectPath)
		if code != "" {
			warnings = append(warnings, "Manual Beads workspace rejected: "+msg)
			continue
		}
		if err := h.appendProject(&projects, seen, resolved, "manual"); err != nil {
			warnings = append(warnings, "Manual Beads workspace is invalid: "+resolved+": "+err.Error())
		}
	}

	if len(projects) == 0 && len(warnings) > 0 {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND",
			"No projects found. Errors: "+strings.Join(warnings, "; "))
		return
	}

	h.addProjectPrefixes(r.Context(), projects)

	result := map[string]interface{}{"projects": projects}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}
	core.WriteSuccess(w, result)
}

// beadBrief is a Bead as another Bead's neighbour: enough to draw a row and
// follow the link, and nothing the card would have to scroll past.
type beadBrief struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Type     string `json:"type,omitempty"`
	Priority int    `json:"priority"`
}

// beadRow is one row of open work: what the map, the ready lists and the stale
// list all draw, plus the edges that decide where the row belongs.
type beadRow struct {
	beadBrief
	Updated    string   `json:"updated,omitempty"`
	DeferUntil string   `json:"deferUntil,omitempty"`
	Parent     string   `json:"parent,omitempty"`
	BlockedBy  []string `json:"blockedBy,omitempty"`
	Blocked    bool     `json:"blocked"`
	Linked     bool     `json:"linked"`
	Acceptance string   `json:"acceptance,omitempty"`
}

// BeadsStatusCounts partitions a store's Beads into the states the rail uses.
// The groups are exclusive, in this order: closed, in progress, blocked,
// deferred, then ready open work.
type BeadsStatusCounts struct {
	Open       int `json:"open"`
	InProgress int `json:"inProgress"`
	Blocked    int `json:"blocked"`
	Closed     int `json:"closed"`
	Deferred   int `json:"deferred"`
}

// BeadsTypeCounts carries every type the Beads rail names, including zeros.
type BeadsTypeCounts struct {
	Epic     int `json:"epic"`
	Task     int `json:"task"`
	Bug      int `json:"bug"`
	Feature  int `json:"feature"`
	Decision int `json:"decision"`
	Chore    int `json:"chore"`
}

// BeadsCounts is the one counts projection shared by the workspace list and
// the Beads rail. It is computed from the store's complete issue list.
type BeadsCounts struct {
	Status BeadsStatusCounts `json:"status"`
	Type   BeadsTypeCounts   `json:"type"`
}

type storeSummary struct {
	Prefix        string
	Counts        BeadsCounts
	NewestUpdated string
}

// beadCard is the Bead the card shows: its own text, and every neighbour it
// links to.
type beadCard struct {
	beadBrief
	Updated     string      `json:"updated,omitempty"`
	Created     string      `json:"created,omitempty"`
	DeferUntil  string      `json:"deferUntil,omitempty"`
	Assignee    string      `json:"assignee,omitempty"`
	Description string      `json:"description,omitempty"`
	Design      string      `json:"design,omitempty"`
	Acceptance  string      `json:"acceptance,omitempty"`
	Notes       string      `json:"notes,omitempty"`
	Parents     []beadBrief `json:"parents"`
	Children    []beadBrief `json:"children"`
	BlockedBy   []beadBrief `json:"blockedBy"`
	Blocks      []beadBrief `json:"blocks"`
}

// The dependency bd draws when one Bead has to wait for another. Every other
// kind — parent-child above all — says where a Bead belongs, not whether it can
// start.
const blocksDependency = "blocks"

// How far up a parent chain the card walks. Bead ids nest as prefix-abc.1.2, so
// a chain this long is already longer than any store here has.
const maxParentChainDepth = 4

// A Bead id is its project's prefix and a short random tail, with a dotted
// child number for each level of nesting. The prefix is what a project is
// recognised by, in terminal output and in a card's links alike.
var beadIDPattern = regexp.MustCompile(`^(.+)-[a-z0-9]{3,8}(\.[0-9]+)*$`)

// beadPrefix reads the project prefix out of one of its Bead ids. An id that
// does not have the shape yields nothing rather than a guess.
func beadPrefix(id string) string {
	match := beadIDPattern.FindStringSubmatch(strings.TrimSpace(id))
	if match == nil {
		return ""
	}
	return match[1]
}

func beadString(raw map[string]interface{}, key string) string {
	value, _ := raw[key].(string)
	return value
}

func beadBriefOf(raw map[string]interface{}) beadBrief {
	return beadBrief{
		ID:       beadString(raw, "id"),
		Title:    beadString(raw, "title"),
		Status:   beadString(raw, "status"),
		Type:     firstString(raw["issue_type"], raw["type"]),
		Priority: issuePriority(raw),
	}
}

// beadDependencies reads the edges of one raw Bead as bd writes them. `bd list`
// carries them as records with depends_on_id and type; `bd show` and `bd dep
// list` carry the whole neighbour with a dependency_type. Both are the same
// edge, so both are read here.
func beadDependencies(raw map[string]interface{}, kind string) []map[string]interface{} {
	items, ok := raw["dependencies"].([]interface{})
	if !ok {
		return nil
	}
	matches := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		edge, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if firstString(edge["type"], edge["dependency_type"]) != kind {
			continue
		}
		matches = append(matches, edge)
	}
	return matches
}

func beadDependencyIDs(raw map[string]interface{}, kind string) []string {
	edges := beadDependencies(raw, kind)
	ids := make([]string, 0, len(edges))
	for _, edge := range edges {
		if id := firstString(edge["depends_on_id"], edge["id"]); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func isEpic(raw map[string]interface{}) bool {
	return firstString(raw["issue_type"], raw["type"]) == "epic"
}

func isClosedBead(raw map[string]interface{}) bool {
	switch beadString(raw, "status") {
	case "closed", "wont_fix", "duplicate":
		return true
	default:
		return false
	}
}

func positiveBeadCount(value interface{}) bool {
	switch count := value.(type) {
	case float64:
		return count > 0
	case int:
		return count > 0
	case int64:
		return count > 0
	default:
		return false
	}
}

// beadIsLinked relies on aggregate relation counts from the list record, so a
// relation to finished work remains visible without loading that work's body.
func beadIsLinked(raw map[string]interface{}) bool {
	return beadString(raw, "parent") != "" ||
		positiveBeadCount(raw["dependency_count"]) ||
		positiveBeadCount(raw["dependent_count"])
}

// projectPrefix asks bd for one Bead of the project and reads its prefix. An
// empty project has no prefix to report and no ids in anyone's terminal either.
func (h *BeadsHandler) projectPrefix(ctx context.Context, projectPath string) (string, error) {
	// Reuse a known positive identity without demanding any snapshot or counts.
	if prefix := h.stores.retainedPrefix(projectPath); prefix != "" {
		return prefix, nil
	}
	ctx = context.WithValue(ctx, optionalBdContextKey{}, true)
	// Prefix catalog demand is optional, but it shares runBd with selected
	// projects. Never enqueue the whole host ahead of foreground work.
	ctx, cancel := context.WithTimeout(ctx, h.execTimeout)
	defer cancel()
	select {
	case h.prefixSlots <- struct{}{}:
		defer func() { <-h.prefixSlots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	issues, err := h.execBdIssues(ctx, projectPath, "list", "--status", "all", "--limit", "1")
	if err != nil {
		return "", err
	}
	if len(issues) == 0 {
		return "", nil
	}
	return beadPrefix(beadString(issues[0], "id")), nil
}

// storeManifestHash identifies an authoritative store generation: the content of
// the store's Dolt manifest. The mtime cannot serve, because reading a store
// rewrites the manifest with the same bytes, so a time-keyed entry expired on
// the very read that filled it and no request ever saw a count. Content
// changes only when the store does, so the cache still expires from the write
// itself and needs no timer or sweep.
func storeManifestHash(projectPath string) (string, error) {
	doltRoot := filepath.Join(projectPath, ".beads", "embeddeddolt")
	databases, err := os.ReadDir(doltRoot)
	if err != nil {
		return "", fmt.Errorf("read Dolt databases in %s: %w", doltRoot, err)
	}
	for _, database := range databases {
		if !database.IsDir() {
			continue
		}
		manifest := filepath.Join(doltRoot, database.Name(), ".dolt", "noms", "manifest")
		info, statErr := os.Stat(manifest)
		if statErr == nil && info.Mode().IsRegular() {
			content, readErr := os.ReadFile(manifest)
			if readErr != nil {
				return "", fmt.Errorf("read Dolt manifest %s: %w", manifest, readErr)
			}
			sum := sha256.Sum256(content)
			return hex.EncodeToString(sum[:]), nil
		}
		if errors.Is(statErr, fs.ErrPermission) {
			return "", fmt.Errorf("cannot read Dolt manifest %s: %w", manifest, statErr)
		}
	}
	return "", fmt.Errorf("no Dolt manifest found in %s", doltRoot)
}

func deferredUntil(raw map[string]interface{}) string {
	return firstString(raw["defer_until"], raw["deferUntil"])
}

func isFutureDefer(raw map[string]interface{}, now time.Time) bool {
	value := deferredUntil(raw)
	if value == "" {
		return false
	}
	deferred, err := time.Parse(time.RFC3339, value)
	if err != nil {
		deferred, err = time.Parse("2006-01-02", value)
	}
	return err == nil && deferred.After(now)
}

func hasActiveBlocker(raw map[string]interface{}, byID map[string]map[string]interface{}) bool {
	for _, blockerID := range beadDependencyIDs(raw, blocksDependency) {
		blocker, known := byID[blockerID]
		if !known || !isClosedBead(blocker) {
			return true
		}
	}
	return false
}

// unfinishedBlockers reads dependencies against an unfinished snapshot. A
// missing same-store blocker is finished, because the status-filtered command
// deliberately omitted it. A missing foreign-store blocker remains active.
func unfinishedBlockers(raw map[string]interface{}, byID map[string]map[string]interface{}, prefix string) []string {
	blockers := make([]string, 0)
	for _, blockerID := range beadDependencyIDs(raw, blocksDependency) {
		if blocker, known := byID[blockerID]; known {
			if !isClosedBead(blocker) {
				blockers = append(blockers, blockerID)
			}
			continue
		}
		if prefix == "" || beadPrefix(blockerID) != prefix {
			blockers = append(blockers, blockerID)
		}
	}
	return blockers
}

func addBeadType(counts *BeadsTypeCounts, issueType string) {
	switch issueType {
	case "epic":
		counts.Epic++
	case "task":
		counts.Task++
	case "bug":
		counts.Bug++
	case "feature":
		counts.Feature++
	case "decision":
		counts.Decision++
	case "chore":
		counts.Chore++
	}
}

// snapshotSummary computes time-sensitive classification from immutable records.
// A defer_until expiry changes counts without changing the authoritative store.
func snapshotSummary(snapshot *beadsStoreSnapshot, now time.Time) storeSummary {
	summary := storeSummary{Prefix: snapshot.prefix}
	for _, issue := range snapshot.issues {
		updated := firstString(issue["updated_at"], issue["updated"])
		if updated > summary.NewestUpdated {
			summary.NewestUpdated = updated
		}
		addBeadType(&summary.Counts.Type, firstString(issue["issue_type"], issue["type"]))
		status := beadString(issue, "status")
		switch {
		case isClosedBead(issue):
			summary.Counts.Status.Closed++
		case status == "in_progress":
			summary.Counts.Status.InProgress++
		case status == "blocked" || hasActiveBlocker(issue, snapshot.byID):
			summary.Counts.Status.Blocked++
		case beadDeferred(issue, now):
			summary.Counts.Status.Deferred++
		default:
			summary.Counts.Status.Open++
		}
	}
	return summary
}

// A dated deferral expires even if bd has not yet rewritten the status field.
// An undated explicit deferred status remains deferred until an actual write.
func beadDeferred(issue map[string]interface{}, now time.Time) bool {
	if beadString(issue, "status") != "deferred" {
		return isFutureDefer(issue, now)
	}
	deadline, err := time.Parse(time.RFC3339, deferredUntil(issue))
	if err != nil {
		deadline, err = time.Parse("2006-01-02", deferredUntil(issue))
	}
	// Missing or malformed dates cannot establish that a deferral expired.
	return err != nil || deadline.After(now)
}

// Keep the workspace counts interface while sharing its complete store read
// with work, Closed and cards. Legacy beads=wait callers wait for freshness.
func (h *BeadsHandler) cachedStoreSummary(projectPath string, wait bool) (storeSummary, bool, bool, error) {
	snapshot, state, err := h.stores.get(context.Background(), projectPath, storeReadDemand{foreground: wait, waitFresh: wait, waitCold: wait})
	if snapshot == nil {
		return storeSummary{}, false, state.Pending, err
	}
	return snapshotSummary(snapshot, h.stores.now()), true, state.Pending, err
}

// addProjectPrefixes gives every discovered project the prefix its Bead ids
// carry, because that is what the terminal's link provider matches on. One bd
// call per project, sharing the adapter's bounded command slots.
func (h *BeadsHandler) addProjectPrefixes(ctx context.Context, projects []map[string]interface{}) {
	var wait sync.WaitGroup
	prefixes := make([]string, len(projects))
	prefixErrors := make([]error, len(projects))
	for index, project := range projects {
		path, _ := project["path"].(string)
		if path == "" {
			continue
		}
		wait.Add(1)
		go func(index int, path string) {
			defer wait.Done()
			prefixes[index], prefixErrors[index] = h.projectPrefix(ctx, path)
		}(index, path)
	}
	wait.Wait()
	for index, prefix := range prefixes {
		if prefix != "" {
			projects[index]["prefix"] = prefix
		}
		if prefixErrors[index] != nil {
			projects[index]["prefixError"] = prefixErrors[index].Error()
		}
	}
}

// Work handles GET /api/beads/work: the unfinished work of one project, which
// is what the map, the ready lists and the stale list are all views of.
//
// The complete authoritative store snapshot is shared with counts, Closed and
// cards. Only unfinished rows are projected into the primary browser views.
func (h *BeadsHandler) Work(w http.ResponseWriter, r *http.Request) {
	projectPath, code, msg := validateBeadsProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		writeBeadsDirectoryError(w, err)
		return
	}

	snapshot, state, err := h.stores.get(r.Context(), projectPath, storeReadDemand{foreground: true, waitCold: true})
	if err != nil && snapshot == nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}
	issues, byID := snapshot.issues, snapshot.byID

	beads := make([]beadRow, 0, len(issues))
	prefix := snapshot.prefix
	now := h.stores.now()
	for _, issue := range issues {
		id := beadString(issue, "id")
		if id == "" || isClosedBead(issue) {
			continue
		}
		if prefix == "" {
			prefix = beadPrefix(id)
		}
		parent := beadString(issue, "parent")
		row := beadRow{
			beadBrief:  beadBriefOf(issue),
			Updated:    firstString(issue["updated_at"], issue["updated"]),
			DeferUntil: deferredUntil(issue),
			Parent:     parent,
			Linked:     beadIsLinked(issue),
		}
		if row.Status == "deferred" && !beadDeferred(issue, now) {
			row.Status = "open"
		}
		row.BlockedBy = unfinishedBlockers(issue, byID, prefix)
		row.Blocked = len(row.BlockedBy) > 0
		if isEpic(issue) {
			row.Acceptance = beadString(issue, "acceptance_criteria")
		}
		beads = append(beads, row)
	}

	sort.Slice(beads, func(i, j int) bool {
		if beads[i].Priority != beads[j].Priority {
			return beads[i].Priority < beads[j].Priority
		}
		return beads[i].ID < beads[j].ID
	})

	core.WriteSuccess(w, map[string]interface{}{
		"beads":       beads,
		"prefix":      prefix,
		"projectPath": projectPath,
		"state":       state,
	})
}

// ClosedWork handles GET /api/beads/closed. The dashboard calls this route
// only when the operator opens Closed. Finished records share the reader,
// while this projection and its browser rendering remain lazy.
func (h *BeadsHandler) ClosedWork(w http.ResponseWriter, r *http.Request) {
	projectPath, ok := h.requestProject(w, r)
	if !ok {
		return
	}

	snapshot, state, err := h.stores.get(r.Context(), projectPath, storeReadDemand{foreground: true, waitCold: true})
	if err != nil && snapshot == nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	issues := snapshot.issues
	beads := make([]beadRow, 0)
	prefix := snapshot.prefix
	for _, issue := range issues {
		if !isClosedBead(issue) {
			continue
		}
		id := beadString(issue, "id")
		if id == "" {
			continue
		}
		if prefix == "" {
			prefix = beadPrefix(id)
		}
		row := beadRow{
			beadBrief: beadBriefOf(issue),
			Updated:   firstString(issue["updated_at"], issue["updated"]),
			Parent:    beadString(issue, "parent"),
			Linked:    beadIsLinked(issue),
		}
		if isEpic(issue) {
			row.Acceptance = beadString(issue, "acceptance_criteria")
		}
		beads = append(beads, row)
	}
	sort.Slice(beads, func(i, j int) bool {
		if beads[i].Updated != beads[j].Updated {
			return beads[i].Updated > beads[j].Updated
		}
		return beads[i].ID < beads[j].ID
	})

	core.WriteSuccess(w, map[string]interface{}{
		"beads":       beads,
		"prefix":      prefix,
		"projectPath": projectPath,
		"state":       state,
	})
}

// Formulas handles GET /api/beads/formulas. Formula list records already
// carry the resolved source path, so returning the CLI objects preserves the
// winning file's provenance when search paths shadow a name.
func (h *BeadsHandler) Formulas(w http.ResponseWriter, r *http.Request) {
	projectPath, ok := h.requestProject(w, r)
	if !ok {
		return
	}

	result, err := h.execBdJSON(r.Context(), projectPath, "formula", "list")
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}
	if result == nil {
		formulaDir := filepath.Join(projectPath, ".beads", "formulas")
		if err := checkFormulaRegistry(formulaDir); err != nil {
			if errors.Is(err, fs.ErrPermission) {
				core.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
			} else {
				core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
			}
			return
		}
		result = []interface{}{}
	}
	formulas, ok := result.([]interface{})
	if !ok {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", fmt.Sprintf("bd formula list returned %T, expected JSON array", result))
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"formulas":    formulas,
		"projectPath": projectPath,
	})
}

// checkFormulaRegistry distinguishes an absent project registry from one that
// bd silently skipped because the server user cannot read or search it.
func checkFormulaRegistry(formulaDir string) error {
	info, err := os.Stat(formulaDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot access formula registry %s as user %s: %w", formulaDir, effectiveUsername(), err)
	}
	if !info.IsDir() {
		return fmt.Errorf("formula registry %s is not a directory", formulaDir)
	}
	if _, err := os.ReadDir(formulaDir); err != nil {
		return fmt.Errorf("cannot read formula registry %s as user %s: %w", formulaDir, effectiveUsername(), err)
	}
	if _, err := os.Stat(filepath.Join(formulaDir, ".")); err != nil {
		return fmt.Errorf("cannot search formula registry %s as user %s: %w", formulaDir, effectiveUsername(), err)
	}
	return nil
}

// FormulaDetail handles GET /api/beads/formula: the complete resolved formula,
// including variables, steps, dependencies, composition rules, and source.
func (h *BeadsHandler) FormulaDetail(w http.ResponseWriter, r *http.Request) {
	projectPath, ok := h.requestProject(w, r)
	if !ok {
		return
	}
	name, code, msg := requiredQueryValue(r, "name")
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	formula, err := h.execBdJSON(r.Context(), projectPath, "formula", "show", name)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}
	if _, ok := formula.(map[string]interface{}); !ok {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", fmt.Sprintf("bd formula show %s returned %T, expected JSON object", name, formula))
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"formula":     formula,
		"projectPath": projectPath,
	})
}

// Molecules handles GET /api/beads/molecules: template protos and instantiated
// molecule roots, with their current issue fields left intact.
func (h *BeadsHandler) Molecules(w http.ResponseWriter, r *http.Request) {
	projectPath, ok := h.requestProject(w, r)
	if !ok {
		return
	}

	molecules, err := h.execBdIssues(r.Context(), projectPath, "list", "--type", "molecule", "--all", "--include-templates")
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}
	core.WriteSuccess(w, map[string]interface{}{
		"molecules":   molecules,
		"projectPath": projectPath,
	})
}

// MoleculeDetail handles GET /api/beads/molecule: bd's full graph projection,
// including root, issues, dependencies, variables, and origin metadata.
func (h *BeadsHandler) MoleculeDetail(w http.ResponseWriter, r *http.Request) {
	projectPath, ok := h.requestProject(w, r)
	if !ok {
		return
	}
	id, code, msg := requiredQueryValue(r, "id")
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	molecule, err := h.execBdJSON(r.Context(), projectPath, "mol", "show", id)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}
	if _, ok := molecule.(map[string]interface{}); !ok {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", fmt.Sprintf("bd mol show %s returned %T, expected JSON object", id, molecule))
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"molecule":    molecule,
		"projectPath": projectPath,
	})
}

// parentChain walks up from a Bead through the store's own list, nearest
// parent first. The bound keeps a cycle in the data from walking forever.
func parentChain(byID map[string]map[string]interface{}, parentID string) []beadBrief {
	chain := make([]beadBrief, 0, maxParentChainDepth)
	seen := make(map[string]bool)
	for parentID != "" && len(chain) < maxParentChainDepth && !seen[parentID] {
		seen[parentID] = true
		parent, known := byID[parentID]
		if !known {
			return chain
		}
		chain = append(chain, beadBriefOf(parent))
		parentID = beadString(parent, "parent")
	}
	return chain
}

// briefByID names a neighbour from the list, or by id alone when the edge
// points outside the store.
func briefByID(byID map[string]map[string]interface{}, id string) beadBrief {
	if raw, known := byID[id]; known {
		return beadBriefOf(raw)
	}
	return beadBrief{ID: id, Priority: 3}
}

func sortBriefs(briefs []beadBrief) {
	sort.SliceStable(briefs, func(i, j int) bool {
		if briefs[i].Priority != briefs[j].Priority {
			return briefs[i].Priority < briefs[j].Priority
		}
		return briefs[i].ID < briefs[j].ID
	})
}

// IssueDetail handles GET /api/beads/issue: one Bead as the card reads it.
//
// Its record, parents, children and dependents come from the same immutable
// full-store snapshot used by every Beads projection.
func (h *BeadsHandler) IssueDetail(w http.ResponseWriter, r *http.Request) {
	projectPath, code, msg := validateBeadsProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}
	issueID, code, msg := requiredIssueID(r)
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		writeBeadsDirectoryError(w, err)
		return
	}

	snapshot, state, err := h.stores.get(r.Context(), projectPath, storeReadDemand{foreground: true, waitCold: true})
	if err != nil && snapshot == nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}
	issues, byID := snapshot.issues, snapshot.byID
	issue, known := byID[issueID]
	if !known {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("No Bead %s in %s", issueID, projectPath))
		return
	}

	card := beadCard{
		beadBrief:   beadBriefOf(issue),
		Updated:     firstString(issue["updated_at"], issue["updated"]),
		Created:     firstString(issue["created_at"], issue["created"]),
		DeferUntil:  deferredUntil(issue),
		Assignee:    beadString(issue, "assignee"),
		Description: beadString(issue, "description"),
		Design:      beadString(issue, "design"),
		Acceptance:  beadString(issue, "acceptance_criteria"),
		Notes:       beadString(issue, "notes"),
		Parents:     parentChain(byID, beadString(issue, "parent")),
		Children:    []beadBrief{},
		BlockedBy:   []beadBrief{},
		Blocks:      []beadBrief{},
	}
	if card.Status == "deferred" && !beadDeferred(issue, h.stores.now()) {
		card.Status = "open"
	}
	for _, blockerID := range beadDependencyIDs(issue, blocksDependency) {
		card.BlockedBy = append(card.BlockedBy, briefByID(byID, blockerID))
	}
	for _, raw := range issues {
		if beadString(raw, "parent") == issueID {
			card.Children = append(card.Children, beadBriefOf(raw))
		}
		for _, blockerID := range beadDependencyIDs(raw, blocksDependency) {
			if blockerID == issueID {
				card.Blocks = append(card.Blocks, beadBriefOf(raw))
			}
		}
	}
	sortBriefs(card.Children)
	sortBriefs(card.Blocks)

	core.WriteSuccess(w, map[string]interface{}{
		"bead":        card,
		"projectPath": projectPath,
		"state":       state,
	})
}

func issuePriority(issue map[string]interface{}) int {
	switch v := issue["priority"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 3
	}
}

func firstString(values ...interface{}) string {
	for _, value := range values {
		if s, ok := value.(string); ok && s != "" {
			return s
		}
	}
	return ""
}
