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
	"time"

	"github.com/chrote/server/internal/core"
)

// BeadsHandler handles beads-related API endpoints
type BeadsHandler struct {
	bdCommand   string
	execTimeout time.Duration

	storeSummaryMu      sync.Mutex
	storeSummaries      map[string]storeSummaryCacheEntry
	storeSummaryRefresh map[string]*storeSummaryRefresh
	storeSummarySlots   chan struct{}
}

// NewBeadsHandler creates a new BeadsHandler
func NewBeadsHandler() *BeadsHandler {
	bdCommand := os.Getenv("CHROTE_BD_COMMAND")
	if bdCommand == "" {
		bdCommand = "bd"
	}

	return &BeadsHandler{
		bdCommand:           bdCommand,
		execTimeout:         60 * time.Second,
		storeSummaries:      make(map[string]storeSummaryCacheEntry),
		storeSummaryRefresh: make(map[string]*storeSummaryRefresh),
		storeSummarySlots:   make(chan struct{}, workspaceProbeFanOut),
	}
}

// RegisterRoutes registers the beads routes on the given mux
func (h *BeadsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/beads/health", h.Health)
	mux.HandleFunc("GET /api/beads/projects", h.ListProjects)
	mux.HandleFunc("GET /api/beads/work", h.Work)
	mux.HandleFunc("GET /api/beads/issue", h.IssueDetail)
}

// getBdVersion returns the bd version or error.
func (h *BeadsHandler) getBdVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.bdCommand, "version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// checkBdInstalled checks if bd is available.
func (h *BeadsHandler) checkBdInstalled() bool {
	_, err := h.getBdVersion()
	return err == nil
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

func effectiveUsername() string {
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	return fmt.Sprintf("uid %d", os.Geteuid())
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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

// execBdJSON runs a bd command with --json and returns parsed JSON.
func (h *BeadsHandler) execBdJSON(projectPath string, args ...string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), h.execTimeout)
	defer cancel()

	cmdArgs := append([]string{"--json"}, args...)
	cmd := exec.CommandContext(ctx, h.bdCommand, cmdArgs...)
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("bd %s timed out after %v", strings.Join(args, " "), h.execTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("bd %s failed: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("bd %s failed: %v", strings.Join(args, " "), err)
	}

	var result interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("bd %s returned invalid JSON: %v. Output: %s", strings.Join(args, " "), err, string(output)[:min(200, len(output))])
	}

	return result, nil
}

// execBdIssues runs a bd command that returns a JSON array of issue objects.
func (h *BeadsHandler) execBdIssues(projectPath string, args ...string) ([]map[string]interface{}, error) {
	result, err := h.execBdJSON(projectPath, args...)
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

// transformIssue converts raw JSONL issue to frontend-expected format

// Health handles GET /api/beads/health
func (h *BeadsHandler) Health(w http.ResponseWriter, r *http.Request) {
	version, err := h.getBdVersion()
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

	h.addProjectPrefixes(projects)

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

type storeSummaryCacheEntry struct {
	manifestHash string
	summary      storeSummary
}

type storeSummaryRefresh struct {
	done    chan struct{}
	summary storeSummary
	err     error
}

// beadCard is the Bead the card shows: its own text, and every neighbour it
// links to.
type beadCard struct {
	beadBrief
	Updated     string      `json:"updated,omitempty"`
	Created     string      `json:"created,omitempty"`
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
var beadIDPattern = regexp.MustCompile(`^(.+)-[a-z0-9]{3,6}(\.[0-9]+)*$`)

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

// projectPrefix asks bd for one Bead of the project and reads its prefix. An
// empty project has no prefix to report and no ids in anyone's terminal either.
func (h *BeadsHandler) projectPrefix(projectPath string) string {
	issues, err := h.execBdIssues(projectPath, "list", "--status", "all", "--limit", "1")
	if err != nil || len(issues) == 0 {
		return ""
	}
	return beadPrefix(beadString(issues[0], "id"))
}

// storeManifestHash is the cache key for the counts projection: the content of
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

// readStoreSummary computes the one complete projection used by the workspace
// route and the Beads rail. The status counts are exclusive so their sum is the
// store's total, while the legacy open count can be derived by omitting closed.
func (h *BeadsHandler) readStoreSummary(projectPath string) (storeSummary, error) {
	issues, err := h.execBdIssues(projectPath, "list", "--status", "all", "--limit", "0")
	if err != nil {
		return storeSummary{}, err
	}
	byID := make(map[string]map[string]interface{}, len(issues))
	for _, issue := range issues {
		if id := beadString(issue, "id"); id != "" {
			byID[id] = issue
		}
	}

	now := time.Now()
	summary := storeSummary{}
	for _, issue := range issues {
		if summary.Prefix == "" {
			summary.Prefix = beadPrefix(beadString(issue, "id"))
		}
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
		case status == "blocked" || hasActiveBlocker(issue, byID):
			summary.Counts.Status.Blocked++
		case status == "deferred" || isFutureDefer(issue, now):
			summary.Counts.Status.Deferred++
		default:
			summary.Counts.Status.Open++
		}
	}
	return summary, nil
}

func (h *BeadsHandler) ensureStoreSummaryStateLocked() {
	if h.storeSummaries == nil {
		h.storeSummaries = make(map[string]storeSummaryCacheEntry)
	}
	if h.storeSummaryRefresh == nil {
		h.storeSummaryRefresh = make(map[string]*storeSummaryRefresh)
	}
	if h.storeSummarySlots == nil {
		h.storeSummarySlots = make(chan struct{}, workspaceProbeFanOut)
	}
}

func (h *BeadsHandler) refreshStoreSummary(projectPath string, refresh *storeSummaryRefresh) {
	h.storeSummarySlots <- struct{}{}
	summary, err := h.readStoreSummary(projectPath)
	// The key is read after bd has run, because bd rewrites the manifest as it
	// reads. Hashing before would file the entry under bytes that no longer
	// exist, which is the miss this handler used to take on every request.
	var hash string
	if err == nil {
		hash, err = storeManifestHash(projectPath)
	}
	<-h.storeSummarySlots

	h.storeSummaryMu.Lock()
	refresh.summary = summary
	refresh.err = err
	if err == nil {
		h.storeSummaries[projectPath] = storeSummaryCacheEntry{
			manifestHash: hash,
			summary:      summary,
		}
	}
	if h.storeSummaryRefresh[projectPath] == refresh {
		delete(h.storeSummaryRefresh, projectPath)
	}
	close(refresh.done)
	h.storeSummaryMu.Unlock()
}

// cachedStoreSummary returns a matching cached projection at once. A miss
// starts one background refresh. A follow-up request may wait for that exact
// refresh after the browser has already painted the store rail.
func (h *BeadsHandler) cachedStoreSummary(projectPath string, wait bool) (storeSummary, bool, bool, error) {
	hash, err := storeManifestHash(projectPath)
	if err != nil {
		return storeSummary{}, false, false, err
	}

	h.storeSummaryMu.Lock()
	h.ensureStoreSummaryStateLocked()
	if cached, ok := h.storeSummaries[projectPath]; ok && cached.manifestHash == hash {
		h.storeSummaryMu.Unlock()
		return cached.summary, true, false, nil
	}
	refresh := h.storeSummaryRefresh[projectPath]
	if refresh == nil {
		refresh = &storeSummaryRefresh{done: make(chan struct{})}
		h.storeSummaryRefresh[projectPath] = refresh
		go h.refreshStoreSummary(projectPath, refresh)
	}
	done := refresh.done
	h.storeSummaryMu.Unlock()

	if !wait {
		return storeSummary{}, false, true, nil
	}
	<-done
	if refresh.err != nil {
		return storeSummary{}, false, false, refresh.err
	}
	// A write during the refresh changes the next request's cache key. This
	// response can still use the complete snapshot the refresh just produced.
	return refresh.summary, true, false, nil
}

// addProjectPrefixes gives every discovered project the prefix its Bead ids
// carry, because that is what the terminal's link provider matches on. One bd
// call per project, all of them at once: the list is short and the operator is
// waiting for it.
func (h *BeadsHandler) addProjectPrefixes(projects []map[string]interface{}) {
	var wait sync.WaitGroup
	prefixes := make([]string, len(projects))
	for index, project := range projects {
		path, _ := project["path"].(string)
		if path == "" {
			continue
		}
		wait.Add(1)
		go func(index int, path string) {
			defer wait.Done()
			prefixes[index] = h.projectPrefix(path)
		}(index, path)
	}
	wait.Wait()
	for index, prefix := range prefixes {
		if prefix != "" {
			projects[index]["prefix"] = prefix
		}
	}
}

// Work handles GET /api/beads/work: the open work of one project, with the
// finished children of its open epics, which is what the map, the ready lists
// and the stale list are all views of.
//
// One `bd list --status all` answers all of it: the statuses of the blockers
// decide what is blocked, and the parents decide what hangs under which epic.
// Nothing is kept between requests.
func (h *BeadsHandler) Work(w http.ResponseWriter, r *http.Request) {
	projectPath, code, msg := validateBeadsProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			core.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	issues, err := h.execBdIssues(projectPath, "list", "--status", "all", "--limit", "0")
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	byID := make(map[string]map[string]interface{}, len(issues))
	for _, issue := range issues {
		if id := beadString(issue, "id"); id != "" {
			byID[id] = issue
		}
	}

	// An open epic is a root of the map, so its finished children come too:
	// reviewing an epic means seeing what is already done under it.
	openEpics := make(map[string]bool)
	for _, issue := range issues {
		if !isClosedBead(issue) && isEpic(issue) {
			openEpics[beadString(issue, "id")] = true
		}
	}

	beads := make([]beadRow, 0, len(issues))
	prefix := ""
	for _, issue := range issues {
		id := beadString(issue, "id")
		if id == "" {
			continue
		}
		if prefix == "" {
			prefix = beadPrefix(id)
		}
		parent := beadString(issue, "parent")
		closed := isClosedBead(issue)
		if closed && !openEpics[parent] {
			continue
		}
		row := beadRow{
			beadBrief:  beadBriefOf(issue),
			Updated:    firstString(issue["updated_at"], issue["updated"]),
			DeferUntil: deferredUntil(issue),
			Parent:     parent,
		}
		if !closed {
			for _, blockerID := range beadDependencyIDs(issue, blocksDependency) {
				if blocker, known := byID[blockerID]; known && isClosedBead(blocker) {
					continue
				}
				row.BlockedBy = append(row.BlockedBy, blockerID)
			}
			row.Blocked = len(row.BlockedBy) > 0
		}
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
// One `bd list --status all` answers all of it, the same call the map makes:
// the Bead's own text is in its record, its parents are the records the parent
// field names, and its children and dependents are the records that name it.
// bd is spawned once per card, whatever the depth of the chain.
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
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	issues, err := h.execBdIssues(projectPath, "list", "--status", "all", "--limit", "0")
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	byID := make(map[string]map[string]interface{}, len(issues))
	for _, raw := range issues {
		if id := beadString(raw, "id"); id != "" {
			byID[id] = raw
		}
	}
	issue, known := byID[issueID]
	if !known {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("No Bead %s in %s", issueID, projectPath))
		return
	}

	card := beadCard{
		beadBrief:   beadBriefOf(issue),
		Updated:     firstString(issue["updated_at"], issue["updated"]),
		Created:     firstString(issue["created_at"], issue["created"]),
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
