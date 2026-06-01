// Package api provides HTTP handlers for the API
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// BeadsHandler handles beads-related API endpoints
type BeadsHandler struct {
	bdCommand   string
	execTimeout time.Duration
}

// NewBeadsHandler creates a new BeadsHandler
func NewBeadsHandler() *BeadsHandler {
	bdCommand := os.Getenv("CHROTE_BD_COMMAND")
	if bdCommand == "" {
		bdCommand = "bd"
	}

	return &BeadsHandler{
		bdCommand:   bdCommand,
		execTimeout: 60 * time.Second,
	}
}

// RegisterRoutes registers the beads routes on the given mux
func (h *BeadsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/beads/health", h.Health)
	mux.HandleFunc("GET /api/beads/projects", h.ListProjects)
	mux.HandleFunc("GET /api/beads/issues", h.Issues)
	mux.HandleFunc("GET /api/beads/issue", h.IssueDetail)
	mux.HandleFunc("GET /api/beads/comments", h.Comments)
	mux.HandleFunc("POST /api/beads/comments", h.AddComment)
	mux.HandleFunc("GET /api/beads/triage", h.Triage)
	mux.HandleFunc("GET /api/beads/insights", h.Insights)
	mux.HandleFunc("GET /api/beads/graph", h.Graph)
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

// checkBeadsDirectory verifies .beads directory exists
func (h *BeadsHandler) checkBeadsDirectory(projectPath string) (string, error) {
	beadsPath := filepath.Join(projectPath, ".beads")
	info, err := os.Stat(beadsPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("no .beads directory found in %s. Run 'bd init' to create one", projectPath)
	}
	if !isRegularFile(filepath.Join(beadsPath, "metadata.json")) || !isDirectory(filepath.Join(beadsPath, "embeddeddolt")) {
		return "", fmt.Errorf("%s exists but is not a modern bd workspace. Run 'bd init' in %s", beadsPath, projectPath)
	}
	return beadsPath, nil
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
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
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if path == absRoot || strings.HasPrefix(path, absRoot+string(os.PathSeparator)) {
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

func (h *BeadsHandler) execBdIssue(projectPath string, args ...string) (map[string]interface{}, error) {
	result, err := h.execBdJSON(projectPath, args...)
	if err != nil {
		return nil, err
	}

	if items, ok := result.([]interface{}); ok {
		if len(items) == 0 {
			return nil, fmt.Errorf("bd %s returned an empty array, expected issue", strings.Join(args, " "))
		}
		result = items[0]
	}

	issue, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("bd %s returned %T, expected JSON object", strings.Join(args, " "), result)
	}
	if typ, ok := issue["_type"].(string); ok && typ != "issue" {
		return nil, fmt.Errorf("bd %s returned %q, expected issue", strings.Join(args, " "), typ)
	}
	return issue, nil
}

func (h *BeadsHandler) execBdComments(projectPath string, args ...string) ([]map[string]interface{}, error) {
	result, err := h.execBdJSON(projectPath, args...)
	if err != nil {
		return nil, err
	}

	items, ok := result.([]interface{})
	if !ok {
		if obj, ok := result.(map[string]interface{}); ok {
			return []map[string]interface{}{obj}, nil
		}
		return nil, fmt.Errorf("bd %s returned %T, expected JSON array", strings.Join(args, " "), result)
	}

	comments := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		comments = append(comments, obj)
	}
	return comments, nil
}

func bdListArgsFromRequest(r *http.Request) ([]string, error) {
	args := []string{"list"}
	query := r.URL.Query()

	if status := strings.TrimSpace(query.Get("status")); status != "" {
		if !isAllowedBeadsStatus(status) {
			return nil, fmt.Errorf("invalid status %q", status)
		}
		args = append(args, "--status", status)
	}

	if limitRaw := strings.TrimSpace(query.Get("limit")); limitRaw != "" {
		limit, err := strconv.Atoi(limitRaw)
		if err != nil || limit < 0 {
			return nil, fmt.Errorf("invalid limit %q", limitRaw)
		}
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	return args, nil
}

func isAllowedBeadsStatus(status string) bool {
	switch status {
	case "all", "open", "ready", "in_progress", "hooked", "blocked", "closed", "wont_fix", "duplicate", "deferred":
		return true
	default:
		return false
	}
}

func requiredIssueID(r *http.Request) (string, string, string) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		return "", "BAD_REQUEST", "Missing required parameter: id"
	}
	return id, "", ""
}

// parseJsonlFile reads and parses a JSONL file
func (h *BeadsHandler) parseJsonlFile(filePath string) ([]map[string]interface{}, error) {
	if !core.FileExists(filePath) {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var items []map[string]interface{}
	var errors []string
	lineNum := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var item map[string]interface{}
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			errors = append(errors, fmt.Sprintf("Line %d: %v", lineNum, err))
		} else {
			items = append(items, item)
		}
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("JSONL parse errors in %s:\n%s", filePath, strings.Join(errors, "\n"))
	}

	return items, nil
}

// transformIssue converts raw JSONL issue to frontend-expected format
func transformIssue(raw map[string]interface{}) map[string]interface{} {
	issue := make(map[string]interface{})

	// Copy direct fields
	for _, key := range []string{"id", "title", "status", "priority", "assignee", "labels", "description"} {
		if v, ok := raw[key]; ok {
			issue[key] = v
		}
	}

	// Map issue_type -> type
	if v, ok := raw["issue_type"]; ok {
		issue["type"] = v
	}

	// Map created_at -> created, updated_at -> updated
	if v, ok := raw["created_at"]; ok {
		issue["created"] = v
	}
	if v, ok := raw["updated_at"]; ok {
		issue["updated"] = v
	}

	// Transform dependencies: extract depends_on_id from each object
	if deps, ok := raw["dependencies"].([]interface{}); ok && len(deps) > 0 {
		depIds := make([]string, 0, len(deps))
		for _, d := range deps {
			if depObj, ok := d.(map[string]interface{}); ok {
				if depId, ok := depObj["depends_on_id"].(string); ok {
					depIds = append(depIds, depId)
				}
			}
		}
		if len(depIds) > 0 {
			issue["dependencies"] = depIds
		}
	}

	return issue
}

func transformIssueDetail(raw map[string]interface{}) map[string]interface{} {
	detail := make(map[string]interface{}, len(raw)+8)
	for key, value := range raw {
		detail[key] = value
	}
	for key, value := range transformIssue(raw) {
		detail[key] = value
	}
	if v, ok := raw["acceptance_criteria"]; ok {
		detail["acceptance"] = v
	}
	return detail
}

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

// ListProjects handles GET /api/beads/projects
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

	for _, root := range core.GetAllowedRoots() {
		if !core.FileExists(root) {
			warnings = append(warnings, "Allowed root does not exist: "+root)
			continue
		}

		entries, err := os.ReadDir(root)
		if err != nil {
			warnings = append(warnings, "Cannot read directory "+root+": "+err.Error())
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				projectPath := filepath.Join(root, entry.Name())
				if err := h.appendProject(&projects, seen, projectPath, "auto"); err != nil {
					beadsPath := filepath.Join(projectPath, ".beads")
					if isDirectory(beadsPath) {
						warnings = append(warnings, "Ignoring invalid Beads workspace: "+projectPath+": "+err.Error())
					}
				}
			}
		}

		// Check root itself
		if err := h.appendProject(&projects, seen, root, "auto-root"); err != nil {
			beadsPath := filepath.Join(root, ".beads")
			if isDirectory(beadsPath) {
				warnings = append(warnings, "Ignoring invalid Beads workspace: "+root+": "+err.Error())
			}
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

	result := map[string]interface{}{"projects": projects}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}
	core.WriteSuccess(w, result)
}

// Issues handles GET /api/beads/issues
func (h *BeadsHandler) Issues(w http.ResponseWriter, r *http.Request) {
	projectPath, code, msg := validateBeadsProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	beadsPath, err := h.checkBeadsDirectory(projectPath)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	_ = beadsPath
	args, err := bdListArgsFromRequest(r)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	issues, err := h.execBdIssues(projectPath, args...)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	// Transform issues to match frontend interface
	transformed := make([]map[string]interface{}, len(issues))
	for i, issue := range issues {
		transformed[i] = transformIssue(issue)
	}

	core.WriteSuccess(w, map[string]interface{}{
		"issues":      transformed,
		"totalCount":  len(transformed),
		"projectPath": projectPath,
	})
}

// IssueDetail handles GET /api/beads/issue
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

	issue, err := h.execBdIssue(projectPath, "show", issueID)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"issue":       transformIssueDetail(issue),
		"projectPath": projectPath,
	})
}

// Comments handles GET /api/beads/comments
func (h *BeadsHandler) Comments(w http.ResponseWriter, r *http.Request) {
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

	comments, err := h.execBdComments(projectPath, "comments", issueID)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"comments":    comments,
		"projectPath": projectPath,
		"issueId":     issueID,
	})
}

type addCommentRequest struct {
	Path    string `json:"path"`
	ID      string `json:"id"`
	Comment string `json:"comment"`
}

// AddComment handles POST /api/beads/comments
func (h *BeadsHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	var req addCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body: "+err.Error())
		return
	}

	projectPath, code, msg := validateBeadsProjectPath(req.Path)
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}
	issueID := strings.TrimSpace(req.ID)
	if issueID == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing required field: id")
		return
	}
	comment := strings.TrimSpace(req.Comment)
	if comment == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing required field: comment")
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	result, err := h.execBdJSON(projectPath, "comments", "add", issueID, comment)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"comment":     result,
		"projectPath": projectPath,
		"issueId":     issueID,
	})
}

// Triage handles GET /api/beads/triage
func (h *BeadsHandler) Triage(w http.ResponseWriter, r *http.Request) {
	if !h.checkBdInstalled() {
		core.WriteError(w, http.StatusServiceUnavailable, "BD_NOT_INSTALLED",
			"bd command not found.")
		return
	}

	projectPath, code, msg := validateBeadsProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	ready, err := h.execBdIssues(projectPath, "ready")
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	allIssues, err := h.execBdIssues(projectPath, "list")
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	sortIssuesByPriority(ready)

	recommendations := make([]map[string]interface{}, 0, min(5, len(ready)))
	quickWins := make([]string, 0)
	for i, issue := range ready {
		id, _ := issue["id"].(string)
		if id == "" {
			continue
		}
		priority := issuePriority(issue)
		impact := "medium"
		if priority <= 1 {
			impact = "high"
		} else if priority >= 3 {
			impact = "low"
		}
		if len(recommendations) < 5 {
			recommendations = append(recommendations, map[string]interface{}{
				"issueId":         id,
				"rank":            i + 1,
				"reasoning":       "Ready according to bd: open work with no active blockers.",
				"estimatedImpact": impact,
			})
		}
		if dependencyCount(issue) == 0 && len(quickWins) < 8 {
			quickWins = append(quickWins, id)
		}
	}

	blockers := make([]map[string]interface{}, 0)
	for _, issue := range allIssues {
		if dependentCount(issue) > 0 && issue["status"] != "closed" {
			blockers = append(blockers, issue)
		}
	}
	sort.Slice(blockers, func(i, j int) bool {
		return dependentCount(blockers[i]) > dependentCount(blockers[j])
	})
	blockerIDs := make([]string, 0, min(8, len(blockers)))
	for _, issue := range blockers {
		if id, _ := issue["id"].(string); id != "" {
			blockerIDs = append(blockerIDs, id)
		}
		if len(blockerIDs) >= 8 {
			break
		}
	}

	core.WriteSuccess(w, map[string]interface{}{
		"recommendations": recommendations,
		"quickWins":       quickWins,
		"blockers":        blockerIDs,
	})
}

// Insights handles GET /api/beads/insights
func (h *BeadsHandler) Insights(w http.ResponseWriter, r *http.Request) {
	if !h.checkBdInstalled() {
		core.WriteError(w, http.StatusServiceUnavailable, "BD_NOT_INSTALLED",
			"bd command not found.")
		return
	}

	projectPath, code, msg := validateBeadsProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	args, err := bdListArgsFromRequest(r)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	issues, err := h.execBdIssues(projectPath, args...)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	byStatus := map[string]int{}
	byType := map[string]int{}
	openCount := 0
	blockedCount := 0
	closedCount := 0
	for _, issue := range issues {
		status, _ := issue["status"].(string)
		if status == "" {
			status = "unknown"
		}
		byStatus[status]++
		switch status {
		case "closed":
			closedCount++
		case "blocked":
			blockedCount++
			openCount++
		default:
			openCount++
		}
		typ := firstString(issue["issue_type"], issue["type"])
		if typ == "" {
			typ = "unknown"
		}
		byType[typ]++
	}

	warnings := make([]string, 0)
	if blockedCount > 0 {
		warnings = append(warnings, fmt.Sprintf("%d blocked issues need attention", blockedCount))
	}
	if openCount > 50 {
		warnings = append(warnings, "Large open backlog; use bd ready or priorities to focus next work")
	}
	score := 100 - blockedCount*5
	if openCount > 50 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}

	core.WriteSuccess(w, map[string]interface{}{
		"issueCount":   len(issues),
		"openCount":    openCount,
		"blockedCount": blockedCount,
		"closedCount":  closedCount,
		"byStatus":     byStatus,
		"byType":       byType,
		"health": map[string]interface{}{
			"score":    score,
			"risks":    []string{},
			"warnings": warnings,
		},
	})
}

// Graph handles GET /api/beads/graph
func (h *BeadsHandler) Graph(w http.ResponseWriter, r *http.Request) {
	if !h.checkBdInstalled() {
		core.WriteError(w, http.StatusServiceUnavailable, "BD_NOT_INSTALLED",
			"bd command not found.")
		return
	}

	projectPath, code, msg := validateBeadsProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	issues, err := h.execBdIssues(projectPath, "export")
	if err != nil {
		issues, err = h.execBdIssues(projectPath, "list")
	}
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BD_ERROR", err.Error())
		return
	}

	nodes := make([]map[string]interface{}, 0, len(issues))
	edges := make([]map[string]interface{}, 0)
	for _, issue := range issues {
		id, _ := issue["id"].(string)
		if id == "" {
			continue
		}
		nodes = append(nodes, map[string]interface{}{
			"id":       id,
			"title":    issue["title"],
			"status":   issue["status"],
			"priority": issue["priority"],
			"type":     firstString(issue["issue_type"], issue["type"]),
		})
		if deps, ok := issue["dependencies"].([]interface{}); ok {
			for _, dep := range deps {
				depObj, ok := dep.(map[string]interface{})
				if !ok {
					continue
				}
				target, _ := depObj["depends_on_id"].(string)
				if target == "" {
					continue
				}
				edges = append(edges, map[string]interface{}{"source": id, "target": target})
			}
		}
	}

	core.WriteSuccess(w, map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
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

func dependencyCount(issue map[string]interface{}) int {
	switch v := issue["dependency_count"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func dependentCount(issue map[string]interface{}) int {
	switch v := issue["dependent_count"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func sortIssuesByPriority(issues []map[string]interface{}) {
	sort.SliceStable(issues, func(i, j int) bool {
		pi := issuePriority(issues[i])
		pj := issuePriority(issues[j])
		if pi != pj {
			return pi < pj
		}
		return fmt.Sprint(issues[i]["updated_at"]) > fmt.Sprint(issues[j]["updated_at"])
	})
}

func firstString(values ...interface{}) string {
	for _, value := range values {
		if s, ok := value.(string); ok && s != "" {
			return s
		}
	}
	return ""
}
