package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// FilesHandler handles file browser API requests
type FilesHandler struct {
	allowedRoots   []string
	writeRoots     []string
	deniedRoots    []string
	maxUploadBytes int64
}

const defaultMaxUploadBytes int64 = 64 << 20

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
	Path        string
	Root        string
	IsRoot      bool
	Writable    bool
	IsWriteRoot bool
	Error       string
}

// SuccessResponse is a simple success response
type SuccessResponse struct {
	Success bool `json:"success"`
}

// NewFilesHandler creates a new file API handler
func NewFilesHandler() *FilesHandler {
	allowedRoots := core.GetAllowedRoots()
	return &FilesHandler{
		allowedRoots:   allowedRoots,
		writeRoots:     configuredFileRoots("CHROTE_WRITE_ROOTS", allowedRoots),
		deniedRoots:    append(defaultDeniedFileRoots(), configuredFileRoots("CHROTE_FILE_DENY_PATHS", nil)...),
		maxUploadBytes: configuredMaxUploadBytes(),
	}
}

func configuredFileRoots(name string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return append([]string(nil), fallback...)
	}
	roots := make([]string, 0)
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		absolute, err := filepath.Abs(part)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if !seen[absolute] {
			seen[absolute] = true
			roots = append(roots, absolute)
		}
	}
	return roots
}

func configuredMaxUploadBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("CHROTE_MAX_UPLOAD_BYTES"))
	if raw == "" {
		return defaultMaxUploadBytes
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return defaultMaxUploadBytes
	}
	return value
}

func defaultDeniedFileRoots() []string {
	return []string{
		"/proc",
		"/sys",
		"/dev",
		"/run",
		"/root",
		"/etc/chrote",
		"/etc/ssl/private",
	}
}

func isSensitiveCredentialPath(path string) bool {
	parts := strings.Split(strings.Trim(filepath.ToSlash(filepath.Clean(path)), "/"), "/")
	sensitiveSegments := map[string]bool{
		".ssh": true, ".gnupg": true, ".aws": true, ".azure": true,
		".kube": true, ".docker": true, ".password-store": true,
		".hermes": true, ".netrc": true, ".git-credentials": true,
	}
	for index, part := range parts {
		if sensitiveSegments[part] {
			return true
		}
		if part == ".config" && index+1 < len(parts) {
			switch parts[index+1] {
			case "gh", "gcloud", "hermes", "opencode":
				return true
			}
		}
		if part == ".local" && index+2 < len(parts) && parts[index+1] == "share" && parts[index+2] == "keyrings" {
			return true
		}
	}
	return false
}

func canonicalPathAllowMissing(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(absolute)
	suffix := make([]string, 0)
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for _, part := range suffix {
				resolved = filepath.Join(resolved, part)
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
}

func isPathUnderAnyRoot(path string, roots []string) (string, bool) {
	for _, root := range roots {
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absoluteRoot = filepath.Clean(absoluteRoot)
		if core.IsPathUnderRoot(path, absoluteRoot) {
			return absoluteRoot, true
		}
	}
	return "", false
}

func (h *FilesHandler) isDeniedPath(path string) bool {
	if isSensitiveCredentialPath(path) {
		return true
	}
	_, denied := isPathUnderAnyRoot(path, h.deniedRoots)
	return denied
}

func normalizeRequestPath(path string) string {
	normalized := filepath.Clean(path)
	normalized = strings.ReplaceAll(filepath.ToSlash(normalized), "\\", "/")
	return filepath.ToSlash(filepath.Clean(normalized))
}

// resolveSafePath validates and resolves a path - CRITICAL for security
func (h *FilesHandler) resolveSafePath(requestPath string) PathResult {
	decoded := requestPath
	if decoded == "" {
		decoded = "/"
	}
	normalized := normalizeRequestPath(decoded)
	if normalized == "/" || normalized == "." {
		return PathResult{IsRoot: true}
	}
	if !filepath.IsAbs(normalized) {
		return PathResult{Error: "Path not allowed"}
	}
	if h.isDeniedPath(normalized) {
		return PathResult{Error: "Sensitive path not available in CHROTE Files"}
	}

	resolved, err := canonicalPathAllowMissing(normalized)
	if err != nil {
		return PathResult{Error: "Invalid path"}
	}
	if h.isDeniedPath(resolved) {
		return PathResult{Error: "Sensitive path not available in CHROTE Files"}
	}
	matchedRoot, allowed := isPathUnderAnyRoot(resolved, h.allowedRoots)
	if !allowed {
		return PathResult{Error: "Path not allowed"}
	}
	writeRoot, writable := isPathUnderAnyRoot(resolved, h.writeRoots)
	return PathResult{
		Path:        resolved,
		Root:        matchedRoot,
		Writable:    writable,
		IsWriteRoot: writable && filepath.Clean(resolved) == filepath.Clean(writeRoot),
	}
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
	if h.hasOnlyFilesystemRoot() {
		h.writeDirectoryListing(w, string(os.PathSeparator))
		return
	}

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

func (h *FilesHandler) hasOnlyFilesystemRoot() bool {
	if len(h.allowedRoots) != 1 {
		return false
	}
	absRoot, err := filepath.Abs(h.allowedRoots[0])
	if err != nil {
		return false
	}
	return filepath.Clean(absRoot) == string(os.PathSeparator)
}

func (h *FilesHandler) writeDirectoryListing(w http.ResponseWriter, dirPath string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	items := make([]FileItem, 0, len(entries))
	for _, entry := range entries {
		fullPath := filepath.Join(dirPath, entry.Name())
		resolved := h.resolveSafePath(fullPath)
		if resolved.Error != "" {
			continue
		}
		info, err := os.Stat(resolved.Path)
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
			entryResult := h.resolveSafePath(fullPath)
			if entryResult.Error != "" {
				continue
			}
			info, err := os.Stat(entryResult.Path)
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

	if result.Error != "" || result.IsRoot || !result.Writable || result.IsWriteRoot {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "Path is outside configured write roots"
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

	// Read and bound the body before creating parent directories.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			core.WriteError(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "File exceeds the configured upload limit")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	if err := os.MkdirAll(filepath.Dir(result.Path), 0755); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

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

	if result.Error != "" || result.IsRoot || !result.Writable || result.IsWriteRoot {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "Path is outside configured write roots"
		}
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", errMsg)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
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
	if destResult.Error != "" || destResult.IsRoot || !destResult.Writable || destResult.IsWriteRoot {
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

	if result.Error != "" || result.IsRoot || !result.Writable || result.IsWriteRoot {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "Path is outside configured write roots"
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
