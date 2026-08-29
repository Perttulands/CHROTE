package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/chrote/server/internal/core"
)

// FilesHandler handles file browser API requests
type FilesHandler struct {
	allowedRoots   []string
	maxUploadBytes int64
	operationHook  func(string)
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
	Path   string
	Root   string
	IsRoot bool
	Error  string
}

// SuccessResponse is a simple success response
type SuccessResponse struct {
	Success bool `json:"success"`
}

type confinedParent struct {
	directory *os.File
	name      string
}

var errFilesPathChanged = errors.New("files path changed during validation")
var errFilesDestinationExists = errors.New("files destination already exists")

// Linux openat/unlinkat flags used by CHROTE's Linux service lane.
const fileAtRemoveDirectory = 0x200
const fileOpenPath = 0x200000

// NewFilesHandler creates a new file API handler
func NewFilesHandler() *FilesHandler {
	return &FilesHandler{
		allowedRoots:   core.GetAllowedRoots(),
		maxUploadBytes: configuredMaxUploadBytes(),
	}
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
	matchedRoot := ""
	for _, root := range roots {
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absoluteRoot = filepath.Clean(absoluteRoot)
		if canonicalRoot, err := canonicalPathAllowMissing(absoluteRoot); err == nil {
			absoluteRoot = canonicalRoot
		}
		if core.IsPathUnderRoot(path, absoluteRoot) && len(absoluteRoot) > len(matchedRoot) {
			matchedRoot = absoluteRoot
		}
	}
	return matchedRoot, matchedRoot != ""
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

	resolved, err := canonicalPathAllowMissing(normalized)
	if err != nil {
		return PathResult{Error: "Invalid path"}
	}
	matchedRoot, allowed := isPathUnderAnyRoot(resolved, h.allowedRoots)
	if !allowed {
		return PathResult{Error: "Path not allowed"}
	}
	return PathResult{
		Path: resolved,
		Root: matchedRoot,
	}
}

// resolveMutationPath canonicalizes the parent while deliberately preserving
// the final path component. Delete and rename must act on a symlink itself,
// never on the target selected by that symlink.
func (h *FilesHandler) resolveMutationPath(requestPath string) PathResult {
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

	resolvedParent, err := canonicalPathAllowMissing(filepath.Dir(normalized))
	if err != nil {
		return PathResult{Error: "Invalid path"}
	}
	operationPath := filepath.Join(resolvedParent, filepath.Base(normalized))
	matchedRoot, allowed := isPathUnderAnyRoot(operationPath, h.allowedRoots)
	if !allowed {
		return PathResult{Error: "Path not allowed"}
	}
	return PathResult{
		Path: operationPath,
		Root: matchedRoot,
	}
}

func validFilePathComponent(component string) bool {
	return component != "" && component != "." && component != ".." && filepath.Base(component) == component && !strings.ContainsRune(component, 0)
}

func openFileAt(parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
	if parent == nil || !validFilePathComponent(name) {
		return nil, errFilesPathChanged
	}
	fd, err := syscall.Openat(int(parent.Fd()), name, flags|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, mode)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.ENOTDIR) {
			return nil, errFilesPathChanged
		}
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not open confined file")
	}
	return file, nil
}

func openDirectoryAt(parent *os.File, name string) (*os.File, error) {
	return openFileAt(parent, name, syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
}

func openConfinedRoot(root string) (*os.File, string, error) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, "", err
	}
	canonicalRoot, err = filepath.Abs(canonicalRoot)
	if err != nil {
		return nil, "", err
	}
	canonicalRoot = filepath.Clean(canonicalRoot)
	if !filepath.IsAbs(canonicalRoot) {
		return nil, "", errFilesPathChanged
	}

	fd, err := syscall.Open(string(os.PathSeparator), syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, "", err
	}
	current := os.NewFile(uintptr(fd), string(os.PathSeparator))
	if current == nil {
		_ = syscall.Close(fd)
		return nil, "", errors.New("could not open filesystem root")
	}
	if canonicalRoot == string(os.PathSeparator) {
		return current, canonicalRoot, nil
	}

	for _, component := range strings.Split(strings.TrimPrefix(canonicalRoot, string(os.PathSeparator)), string(os.PathSeparator)) {
		next, err := openDirectoryAt(current, component)
		_ = current.Close()
		if err != nil {
			return nil, "", err
		}
		current = next
	}
	return current, canonicalRoot, nil
}

func confinedPathComponents(root, target string) ([]string, error) {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return nil, errFilesPathChanged
	}
	if relative == "." {
		return nil, nil
	}
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return nil, errFilesPathChanged
	}
	components := strings.Split(relative, string(os.PathSeparator))
	for _, component := range components {
		if !validFilePathComponent(component) {
			return nil, errFilesPathChanged
		}
	}
	return components, nil
}

func (h *FilesHandler) openConfinedExisting(result PathResult) (*os.File, error) {
	current, canonicalRoot, err := openConfinedRoot(result.Root)
	if err != nil {
		return nil, err
	}
	components, err := confinedPathComponents(canonicalRoot, result.Path)
	if err != nil {
		_ = current.Close()
		return nil, err
	}
	for index, component := range components {
		flags := syscall.O_RDONLY
		if index < len(components)-1 {
			flags |= syscall.O_DIRECTORY
		}
		next, err := openFileAt(current, component, flags, 0)
		_ = current.Close()
		if err != nil {
			return nil, err
		}
		if index < len(components)-1 {
			info, err := next.Stat()
			if err != nil {
				_ = next.Close()
				return nil, err
			}
			if !info.IsDir() {
				_ = next.Close()
				return nil, errFilesPathChanged
			}
		}
		current = next
	}
	return current, nil
}

func (h *FilesHandler) openConfinedDirectory(result PathResult, create bool) (*os.File, error) {
	current, canonicalRoot, err := openConfinedRoot(result.Root)
	if err != nil {
		return nil, err
	}
	components, err := confinedPathComponents(canonicalRoot, result.Path)
	if err != nil {
		_ = current.Close()
		return nil, err
	}
	for _, component := range components {
		next, openErr := openDirectoryAt(current, component)
		if errors.Is(openErr, os.ErrNotExist) && create {
			if mkdirErr := syscall.Mkdirat(int(current.Fd()), component, 0755); mkdirErr != nil && !errors.Is(mkdirErr, syscall.EEXIST) {
				_ = current.Close()
				return nil, mkdirErr
			}
			next, openErr = openDirectoryAt(current, component)
		}
		_ = current.Close()
		if openErr != nil {
			return nil, openErr
		}
		current = next
	}
	return current, nil
}

func (h *FilesHandler) openConfinedParent(result PathResult, create bool) (*confinedParent, error) {
	if result.Path == "" || result.IsRoot {
		return nil, errFilesPathChanged
	}
	name := filepath.Base(result.Path)
	if !validFilePathComponent(name) {
		return nil, errFilesPathChanged
	}
	parentResult := result
	parentResult.Path = filepath.Dir(result.Path)
	directory, err := h.openConfinedDirectory(parentResult, create)
	if err != nil {
		return nil, err
	}
	return &confinedParent{directory: directory, name: name}, nil
}

func (h *FilesHandler) writeConfinedFileAt(parent *confinedParent, body []byte) error {
	if parent == nil || parent.directory == nil {
		return errFilesPathChanged
	}
	file, err := openFileAt(parent.directory, parent.name, syscall.O_WRONLY, 0)
	created := false
	if errors.Is(err, os.ErrNotExist) {
		file, err = openFileAt(parent.directory, parent.name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL, 0644)
		created = err == nil
	}
	if err != nil {
		return err
	}
	defer func() {
		if file != nil {
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("file target is not a regular file")
	}
	if !created {
		if err := file.Truncate(0); err != nil {
			return err
		}
	}
	if len(body) > 0 {
		written, err := file.Write(body)
		if err != nil {
			return err
		}
		if written != len(body) {
			return io.ErrShortWrite
		}
	}
	err = file.Close()
	file = nil
	return err
}

func (h *FilesHandler) validateMutationEntry(parent *confinedParent, allowMissing bool) error {
	if parent == nil || parent.directory == nil || !validFilePathComponent(parent.name) {
		return errFilesPathChanged
	}
	fd, err := syscall.Openat(
		int(parent.directory.Fd()),
		parent.name,
		fileOpenPath|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		if allowMissing && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	file := os.NewFile(uintptr(fd), parent.name)
	if file == nil {
		_ = syscall.Close(fd)
		return errors.New("could not open mutation target")
	}
	return file.Close()
}

func (h *FilesHandler) removeConfinedAt(parent *os.File, name string) error {
	if parent == nil || !validFilePathComponent(name) {
		return errFilesPathChanged
	}
	directory, err := openDirectoryAt(parent, name)
	if err != nil {
		unlinkErr := syscall.Unlinkat(int(parent.Fd()), name)
		if unlinkErr == nil || errors.Is(unlinkErr, os.ErrNotExist) {
			return nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return err
		}
		return unlinkErr
	}
	info, err := directory.Stat()
	if err != nil {
		_ = directory.Close()
		return err
	}
	for {
		names, readErr := directory.Readdirnames(128)
		for _, child := range names {
			if !validFilePathComponent(child) {
				_ = directory.Close()
				return errFilesPathChanged
			}
			if err := h.removeConfinedAt(directory, child); err != nil {
				_ = directory.Close()
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = directory.Close()
			return readErr
		}
	}
	if err := directory.Close(); err != nil {
		return err
	}

	confirmation, err := openDirectoryAt(parent, name)
	if err != nil {
		return errFilesPathChanged
	}
	confirmationInfo, statErr := confirmation.Stat()
	_ = confirmation.Close()
	if statErr != nil {
		return statErr
	}
	if !os.SameFile(info, confirmationInfo) {
		return errFilesPathChanged
	}
	return unlinkDirectoryAt(parent, name)
}

// renameFileAtNoReplace moves oldName under oldParent to newName under
// newParent and refuses to replace whatever is already sitting at the
// destination. A plain renameat silently overwrites the destination, which
// destroys a file the caller never named.
//
// The occupancy check is descriptor-relative and does not follow symlinks, so a
// symlink at the destination counts as occupied and is left alone.
//
// Known limitation: the check and the rename are two syscalls, so a destination
// created in between is still overwritten. Closing that window needs
// RENAME_NOREPLACE via renameat2, which Go's frozen syscall package does not
// expose on this platform — it needs golang.org/x/sys or per-architecture
// syscall numbers, a dependency call for the repo owner rather than a drive-by
// here. Tracked separately; the racing writer must already be a local process
// under the service account.
func renameFileAtNoReplace(oldParent *os.File, oldName string, newParent *os.File, newName string) error {
	if oldParent == nil || newParent == nil || !validFilePathComponent(oldName) || !validFilePathComponent(newName) {
		return errFilesPathChanged
	}
	fd, err := syscall.Openat(
		int(newParent.Fd()),
		newName,
		fileOpenPath|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err == nil {
		_ = syscall.Close(fd)
		return errFilesDestinationExists
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syscall.Renameat(int(oldParent.Fd()), oldName, int(newParent.Fd()), newName)
}

func unlinkDirectoryAt(parent *os.File, name string) error {
	if parent == nil || !validFilePathComponent(name) {
		return errFilesPathChanged
	}
	namePointer, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, callErr := syscall.Syscall(
		syscall.SYS_UNLINKAT,
		parent.Fd(),
		uintptr(unsafe.Pointer(namePointer)),
		uintptr(fileAtRemoveDirectory),
	)
	runtime.KeepAlive(namePointer)
	if callErr != 0 {
		return callErr
	}
	return nil
}

func writeFilesUseError(w http.ResponseWriter, err error, notFound bool) {
	if errors.Is(err, errFilesPathChanged) {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Path changed during validation")
		return
	}
	if errors.Is(err, os.ErrPermission) {
		core.WriteError(w, http.StatusForbidden, "PERMISSION_DENIED", err.Error())
		return
	}
	if notFound && errors.Is(err, os.ErrNotExist) {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Not found")
		return
	}
	core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
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
	allowedRoots := h.allowedRoots
	if hasOnlyFilesystemRoot(allowedRoots) {
		h.writeDirectoryListing(w, string(os.PathSeparator))
		return
	}

	items := make([]FileItem, len(allowedRoots))
	now := time.Now().Format(time.RFC3339)

	for i, root := range allowedRoots {
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

func hasOnlyFilesystemRoot(roots []string) bool {
	if len(roots) != 1 {
		return false
	}
	absRoot, err := filepath.Abs(roots[0])
	if err != nil {
		return false
	}
	return filepath.Clean(absRoot) == string(os.PathSeparator)
}

func (h *FilesHandler) writeDirectoryListing(w http.ResponseWriter, dirPath string) {
	result := h.resolveSafePath(dirPath)
	if dirPath == string(os.PathSeparator) {
		result = PathResult{Path: dirPath, Root: dirPath}
	}
	if result.Error != "" || (result.IsRoot && result.Path == "") {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Path not allowed")
		return
	}
	directory, err := h.openConfinedExisting(result)
	if err != nil {
		writeFilesUseError(w, err, false)
		return
	}
	defer directory.Close()

	items, err := h.confinedDirectoryItems(directory, result)
	if err != nil {
		writeFilesUseError(w, err, false)
		return
	}

	core.WriteJSON(w, http.StatusOK, DirectoryResponse{
		IsDir: true,
		Items: items,
	})
}

func (h *FilesHandler) confinedDirectoryItems(directory *os.File, directoryResult PathResult) ([]FileItem, error) {
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	items := make([]FileItem, 0, len(entries))
	for _, entry := range entries {
		if !validFilePathComponent(entry.Name()) {
			continue
		}
		fullPath := filepath.Join(directoryResult.Path, entry.Name())
		resolved := h.resolveSafePath(fullPath)
		if resolved.Error != "" {
			continue
		}

		var info os.FileInfo
		if entry.Type()&os.ModeSymlink != 0 {
			target, openErr := h.openConfinedExisting(resolved)
			if openErr != nil {
				continue
			}
			info, openErr = target.Stat()
			_ = target.Close()
			if openErr != nil {
				continue
			}
		} else {
			entryFile, openErr := openFileAt(directory, entry.Name(), syscall.O_RDONLY, 0)
			if openErr != nil {
				continue
			}
			info, openErr = entryFile.Stat()
			_ = entryFile.Close()
			if openErr != nil {
				continue
			}
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
	return items, nil
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
	if h.operationHook != nil {
		h.operationHook("get")
	}

	resource, err := h.openConfinedExisting(result)
	if err != nil {
		writeFilesUseError(w, err, true)
		return
	}
	defer resource.Close()
	stat, err := resource.Stat()
	if err != nil {
		writeFilesUseError(w, err, false)
		return
	}

	if stat.IsDir() {
		items, err := h.confinedDirectoryItems(resource, result)
		if err != nil {
			writeFilesUseError(w, err, false)
			return
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

	if result.Error != "" || result.IsRoot || filepath.Clean(result.Path) == filepath.Clean(result.Root) {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "Cannot create a configured root"
		}
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", errMsg)
		return
	}
	if h.operationHook != nil {
		h.operationHook("create")
	}

	// If path ends with /, create directory
	if strings.HasSuffix(requestPath, "/") {
		directory, err := h.openConfinedDirectory(result, true)
		if err != nil {
			writeFilesUseError(w, err, false)
			return
		}
		_ = directory.Close()
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

	parent, err := h.openConfinedParent(result, true)
	if err != nil {
		writeFilesUseError(w, err, false)
		return
	}
	defer parent.directory.Close()

	if err := h.writeConfinedFileAt(parent, body); err != nil {
		writeFilesUseError(w, err, false)
		return
	}

	core.WriteJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// RenameResource handles PATCH /api/files/resources/* - rename/move
func (h *FilesHandler) RenameResource(w http.ResponseWriter, r *http.Request) {
	requestPath := "/" + r.PathValue("path")
	result := h.resolveMutationPath(requestPath)

	if result.Error != "" || result.IsRoot || filepath.Clean(result.Path) == filepath.Clean(result.Root) {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "Cannot rename a configured root"
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

	destResult := h.resolveMutationPath(req.Destination)
	if destResult.Error != "" || destResult.IsRoot || filepath.Clean(destResult.Path) == filepath.Clean(destResult.Root) {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Invalid destination")
		return
	}
	if h.operationHook != nil {
		h.operationHook("rename")
	}

	sourceParent, err := h.openConfinedParent(result, false)
	if err != nil {
		writeFilesUseError(w, err, false)
		return
	}
	defer sourceParent.directory.Close()
	destinationParent, err := h.openConfinedParent(destResult, false)
	if err != nil {
		writeFilesUseError(w, err, false)
		return
	}
	defer destinationParent.directory.Close()
	if err := h.validateMutationEntry(sourceParent, false); err != nil {
		writeFilesUseError(w, err, false)
		return
	}
	if err := h.validateMutationEntry(destinationParent, true); err != nil {
		writeFilesUseError(w, err, false)
		return
	}

	if err := renameFileAtNoReplace(sourceParent.directory, sourceParent.name, destinationParent.directory, destinationParent.name); err != nil {
		if errors.Is(err, errFilesDestinationExists) {
			core.WriteError(w, http.StatusConflict, "DESTINATION_EXISTS", "A file or folder already exists at the destination")
			return
		}
		writeFilesUseError(w, err, false)
		return
	}

	core.WriteJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// DeleteResource handles DELETE /api/files/resources/* - delete file/folder
func (h *FilesHandler) DeleteResource(w http.ResponseWriter, r *http.Request) {
	requestPath := "/" + r.PathValue("path")
	result := h.resolveMutationPath(requestPath)

	if result.Error != "" || result.IsRoot || filepath.Clean(result.Path) == filepath.Clean(result.Root) {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "Cannot delete a configured root"
		}
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", errMsg)
		return
	}
	if h.operationHook != nil {
		h.operationHook("delete")
	}

	parent, err := h.openConfinedParent(result, false)
	if err != nil {
		writeFilesUseError(w, err, true)
		return
	}
	defer parent.directory.Close()
	if err := h.validateMutationEntry(parent, false); err != nil {
		writeFilesUseError(w, err, true)
		return
	}

	if err := h.removeConfinedAt(parent.directory, parent.name); err != nil {
		writeFilesUseError(w, err, true)
		return
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
	if h.operationHook != nil {
		h.operationHook("download")
	}

	file, err := h.openConfinedExisting(result)
	if err != nil {
		writeFilesUseError(w, err, true)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeFilesUseError(w, err, false)
		return
	}

	if stat.IsDir() {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Cannot download directory")
		return
	}

	// Set download headers
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(result.Path)+"\"")
	http.ServeContent(w, r, filepath.Base(result.Path), stat.ModTime(), file)
}
