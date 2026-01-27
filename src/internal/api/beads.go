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
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// BeadsHandler handles beads-related API endpoints
type BeadsHandler struct {
	bvCommand   string
	execTimeout time.Duration
}

// NewBeadsHandler creates a new BeadsHandler
func NewBeadsHandler() *BeadsHandler {
	return &BeadsHandler{
		bvCommand:   "bv",
		execTimeout: 60 * time.Second,
	}
}

// RegisterRoutes registers the beads routes on the given mux
func (h *BeadsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/beads/health", h.Health)
	mux.HandleFunc("GET /api/beads/projects", h.ListProjects)
	mux.HandleFunc("GET /api/beads/issues", h.Issues)
	mux.HandleFunc("GET /api/beads/triage", h.Triage)
	mux.HandleFunc("GET /api/beads/insights", h.Insights)
	mux.HandleFunc("GET /api/beads/graph", h.Graph)
}

// getBvVersion returns the bv version or error
func (h *BeadsHandler) getBvVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.bvCommand, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// checkBvInstalled checks if bv is available
func (h *BeadsHandler) checkBvInstalled() bool {
	_, err := h.getBvVersion()
	return err == nil
}

// checkBeadsDirectory verifies .beads directory exists
func (h *BeadsHandler) checkBeadsDirectory(projectPath string) (string, error) {
	beadsPath := filepath.Join(projectPath, ".beads")
	if !core.FileExists(beadsPath) {
		return "", fmt.Errorf("no .beads directory found in %s. Run 'bv init' to create one", projectPath)
	}
	return beadsPath, nil
}

// execBvCommand runs a bv command and returns parsed JSON
func (h *BeadsHandler) execBvCommand(flag, projectPath string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), h.execTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.bvCommand, flag, projectPath)
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("bv %s timed out after %v", flag, h.execTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("bv %s failed: %s", flag, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("bv %s failed: %v", flag, err)
	}

	var result interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("bv %s returned invalid JSON: %v. Output: %s", flag, err, string(output)[:min(200, len(output))])
	}

	return result, nil
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

// Health handles GET /api/beads/health
func (h *BeadsHandler) Health(w http.ResponseWriter, r *http.Request) {
	version, err := h.getBvVersion()
	if err != nil {
		core.WriteError(w, http.StatusServiceUnavailable, "BV_NOT_INSTALLED",
			"bv command not found. Install beads_viewer: go install github.com/Dicklesworthstone/beads_viewer@latest")
		return
	}

	core.WriteSuccess(w, map[string]interface{}{
		"status":       "ok",
		"bvVersion":    version,
		"allowedRoots": core.AllowedRoots,
	})
}

// ListProjects handles GET /api/beads/projects
// Scans up to 5 levels deep for .beads folders
func (h *BeadsHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	var projects []map[string]interface{}
	var warnings []string
	const maxDepth = 5

	for _, root := range core.AllowedRoots {
		if !core.FileExists(root) {
			warnings = append(warnings, "Allowed root does not exist: "+root)
			continue
		}

		rootDepth := strings.Count(filepath.ToSlash(root), "/")

		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // Skip directories we can't read
			}

			// Calculate current depth relative to root
			currentDepth := strings.Count(filepath.ToSlash(path), "/") - rootDepth

			// Skip if too deep
			if currentDepth > maxDepth {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip hidden directories (except .beads itself which we're looking for)
			if d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != ".beads" {
				return filepath.SkipDir
			}

			// Skip common non-project directories
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "vendor", "__pycache__", ".git", "dist", "build":
					return filepath.SkipDir
				}
			}

			// Check if this directory contains a .beads folder
			if d.IsDir() && d.Name() != ".beads" {
				beadsPath := filepath.Join(path, ".beads")
				if core.FileExists(beadsPath) {
					projects = append(projects, map[string]interface{}{
						"name":      d.Name(),
						"path":      path,
						"beadsPath": beadsPath,
					})
				}
			}

			return nil
		})

		if err != nil {
			warnings = append(warnings, "Error walking "+root+": "+err.Error())
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
	projectPath, code, msg := core.ValidateProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	beadsPath, err := h.checkBeadsDirectory(projectPath)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	issuesFile := filepath.Join(beadsPath, "issues.jsonl")
	if !core.FileExists(issuesFile) {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND",
			fmt.Sprintf("No issues.jsonl file found in %s. Create issues with 'bv add'.", beadsPath))
		return
	}

	issues, err := h.parseJsonlFile(issuesFile)
	if err != nil {
		core.WriteError(w, http.StatusUnprocessableEntity, "INVALID_JSONL", err.Error())
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

// Triage handles GET /api/beads/triage
func (h *BeadsHandler) Triage(w http.ResponseWriter, r *http.Request) {
	if !h.checkBvInstalled() {
		core.WriteError(w, http.StatusServiceUnavailable, "BV_NOT_INSTALLED",
			"bv command not found. Install beads_viewer.")
		return
	}

	projectPath, code, msg := core.ValidateProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	result, err := h.execBvCommand("--robot-triage", projectPath)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BV_ERROR", err.Error())
		return
	}

	core.WriteSuccess(w, result)
}

// Insights handles GET /api/beads/insights
func (h *BeadsHandler) Insights(w http.ResponseWriter, r *http.Request) {
	if !h.checkBvInstalled() {
		core.WriteError(w, http.StatusServiceUnavailable, "BV_NOT_INSTALLED",
			"bv command not found. Install beads_viewer.")
		return
	}

	projectPath, code, msg := core.ValidateProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	result, err := h.execBvCommand("--robot-insights", projectPath)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BV_ERROR", err.Error())
		return
	}

	core.WriteSuccess(w, result)
}

// Graph handles GET /api/beads/graph
func (h *BeadsHandler) Graph(w http.ResponseWriter, r *http.Request) {
	if !h.checkBvInstalled() {
		core.WriteError(w, http.StatusServiceUnavailable, "BV_NOT_INSTALLED",
			"bv command not found. Install beads_viewer.")
		return
	}

	projectPath, code, msg := core.ValidateProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, msg)
		return
	}

	if _, err := h.checkBeadsDirectory(projectPath); err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	result, err := h.execBvCommand("--robot-graph", projectPath)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "BV_ERROR", err.Error())
		return
	}

	core.WriteSuccess(w, result)
}
