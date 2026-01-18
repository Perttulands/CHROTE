package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// FilesHandler handles file browser API requests
type FilesHandler struct {
	allowedRoots []string
}

// FileItem represents a file or directory in listings
type FileItem struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	IsDir    bool   `json:"isDir"`
	Type     string `json:"type"`
}

// DirectoryResponse represents a directory listing
type DirectoryResponse struct {
	IsDir bool       `json:"isDir"`
	Items []FileItem `json:"items"`
}

// FileInfoResponse represents file info
type FileInfoResponse struct {
	IsDir    bool   `json:"isDir"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	Type     string `json:"type"`
}

// RenameRequest represents a rename/move request
type RenameRequest struct {
	Destination string `json:"destination"`
	Action      string `json:"action"`
}

// PathResult represents path resolution result
type PathResult struct {
	Path   string
	Root   string
	IsRoot bool
	Error  string
}

// SuccessResponse is a simple success response
type SuccessResponse struct {
	Success bool `json:"success"`
}

// NewFilesHandler creates a new file API handler
func NewFilesHandler() *FilesHandler {
	return &FilesHandler{
		allowedRoots: []string{"/code", "/vault"},
	}
}

// resolveSafePath validates and resolves a path - CRITICAL for security
func (h *FilesHandler) resolveSafePath(requestPath string) PathResult {
	// Decode and normalize
	decoded := requestPath
	if decoded == "" {
		decoded = "/"
	}

	// Clean the path to prevent traversal
	normalized := filepath.Clean(decoded)

	// Convert to forward slashes for consistency
	normalized = strings.ReplaceAll(normalized, "\\", "/")

	// Must start with allowed root
	var matchedRoot string
	for _, root := range h.allowedRoots {
		if normalized == root || strings.HasPrefix(normalized, root+"/") {
			matchedRoot = root
			break
		}
	}

	if matchedRoot == "" {
		// Root listing
		if normalized == "/" || normalized == "." {
			return PathResult{IsRoot: true}
		}
		return PathResult{Error: "Path not allowed"}
	}

	// Resolve to absolute and verify it's still under allowed root
	resolved, err := filepath.Abs(normalized)
	if err != nil {
		return PathResult{Error: "Invalid path"}
	}

	// Normalize again after Abs
	resolved = strings.ReplaceAll(resolved, "\\", "/")

	if !strings.HasPrefix(resolved, matchedRoot) {
		return PathResult{Error: "Path traversal detected"}
	}

	return PathResult{Path: resolved, Root: matchedRoot}
}

// RegisterRoutes registers all file API routes
func (h *FilesHandler) RegisterRoutes(mux *http.ServeMux) {
	// All file routes - {path...} handles both empty and non-empty paths
	mux.HandleFunc("GET /api/files/resources/{path...}", h.GetResource)
	mux.HandleFunc("POST /api/files/resources/{path...}", h.CreateResource)
	mux.HandleFunc("PATCH /api/files/resources/{path...}", h.RenameResource)
	mux.HandleFunc("DELETE /api/files/resources/{path...}", h.DeleteResource)
	mux.HandleFunc("GET /api/files/raw/{path...}", h.DownloadFile)
}

// ListRoot handles GET /api/files/resources/ - root listing
func (h *FilesHandler) ListRoot(w http.ResponseWriter, r *http.Request) {
	items := make([]FileItem, len(h.allowedRoots))
	now := time.Now().Format(time.RFC3339)

	for i, root := range h.allowedRoots {
		items[i] = FileItem{
			Name:     strings.TrimPrefix(root, "/"),
			Size:     0,
			Modified: now,
			IsDir:    true,
			Type:     "",
		}
	}

	core.WriteJSON(w, http.StatusOK, DirectoryResponse{
		IsDir: true,
		Items: items,
	})
}

// GetResource handles GET /api/files/resources/* - list directory or get file info
func (h *FilesHandler) GetResource(w http.ResponseWriter, r *http.Request) {
	pathVal := r.PathValue("path")
	if pathVal == "" {
		// Root listing
		h.ListRoot(w, r)
		return
	}
	requestPath := "/" + pathVal
	result := h.resolveSafePath(requestPath)

	if result.Error != "" {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", result.Error)
		return
	}

	if result.IsRoot {
		// Return virtual root listing
		h.ListRoot(w, r)
		return
	}

	stat, err := os.Stat(result.Path)
	if err != nil {
		if os.IsNotExist(err) {
			core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Not found")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	if stat.IsDir() {
		entries, err := os.ReadDir(result.Path)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		items := make([]FileItem, 0, len(entries))
		for _, entry := range entries {
			fullPath := filepath.Join(result.Path, entry.Name())
			info, err := os.Stat(fullPath)
			if err != nil {
				continue // Skip inaccessible files
			}

			ext := ""
			if !entry.IsDir() {
				ext = strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
			}

			items = append(items, FileItem{
				Name:     entry.Name(),
				Size:     info.Size(),
				Modified: info.ModTime().Format(time.RFC3339),
				IsDir:    entry.IsDir(),
				Type:     ext,
			})
		}

		core.WriteJSON(w, http.StatusOK, DirectoryResponse{
			IsDir: true,
			Items: items,
		})
	} else {
		ext := strings.TrimPrefix(filepath.Ext(result.Path), ".")
		core.WriteJSON(w, http.StatusOK, FileInfoResponse{
			IsDir:    false,
			Name:     filepath.Base(result.Path),
			Size:     stat.Size(),
			Modified: stat.ModTime().Format(time.RFC3339),
			Type:     ext,
		})
	}
}

// CreateResource handles POST /api/files/resources/* - create folder or upload file
func (h *FilesHandler) CreateResource(w http.ResponseWriter, r *http.Request) {
	requestPath := "/" + r.PathValue("path")
	result := h.resolveSafePath(requestPath)

	if result.Error != "" || result.IsRoot {
		errMsg := result.Error
		if result.IsRoot {
			errMsg = "Cannot create at root"
		}
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", errMsg)
		return
	}

	// If path ends with /, create directory
	if strings.HasSuffix(requestPath, "/") {
		if err := os.MkdirAll(result.Path, 0755); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		core.WriteJSON(w, http.StatusOK, SuccessResponse{Success: true})
		return
	}

	// Otherwise, upload file
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(result.Path), 0755); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	// Read the body and write to file
	body, err := io.ReadAll(r.Body)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer r.Body.Close()

	if err := os.WriteFile(result.Path, body, 0644); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	core.WriteJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// RenameResource handles PATCH /api/files/resources/* - rename/move
func (h *FilesHandler) RenameResource(w http.ResponseWriter, r *http.Request) {
	requestPath := "/" + r.PathValue("path")
	result := h.resolveSafePath(requestPath)

	if result.Error != "" || result.IsRoot {
		errMsg := result.Error
		if result.IsRoot {
			errMsg = "Cannot rename root"
		}
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", errMsg)
		return
	}

	var req RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON")
		return
	}

	if req.Action != "rename" || req.Destination == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid request")
		return
	}

	destResult := h.resolveSafePath(req.Destination)
	if destResult.Error != "" || destResult.IsRoot {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Invalid destination")
		return
	}

	if err := os.Rename(result.Path, destResult.Path); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	core.WriteJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// DeleteResource handles DELETE /api/files/resources/* - delete file/folder
func (h *FilesHandler) DeleteResource(w http.ResponseWriter, r *http.Request) {
	requestPath := "/" + r.PathValue("path")
	result := h.resolveSafePath(requestPath)

	if result.Error != "" || result.IsRoot {
		errMsg := result.Error
		if result.IsRoot {
			errMsg = "Cannot delete root"
		}
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", errMsg)
		return
	}

	stat, err := os.Stat(result.Path)
	if err != nil {
		if os.IsNotExist(err) {
			core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Not found")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	if stat.IsDir() {
		if err := os.RemoveAll(result.Path); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
	} else {
		if err := os.Remove(result.Path); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
	}

	core.WriteJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// DownloadFile handles GET /api/files/raw/* - download file
func (h *FilesHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	requestPath := "/" + r.PathValue("path")
	result := h.resolveSafePath(requestPath)

	if result.Error != "" || result.IsRoot {
		errMsg := result.Error
		if result.IsRoot {
			errMsg = "Cannot download root"
		}
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", errMsg)
		return
	}

	stat, err := os.Stat(result.Path)
	if err != nil {
		if os.IsNotExist(err) {
			core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Not found")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	if stat.IsDir() {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Cannot download directory")
		return
	}

	// Set download headers
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(result.Path)+"\"")
	http.ServeFile(w, r, result.Path)
}
