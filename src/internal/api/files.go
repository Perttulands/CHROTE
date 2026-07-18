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
	writeRoots     []string
	deniedRoots    []string
	deniedRootIDs  map[fileIdentity]struct{}
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

type fileIdentity struct {
	device uint64
	inode  uint64
}

type confinedParent struct {
	directory *os.File
	name      string
}

var errFilesPathChanged = errors.New("files path changed during validation")

// Linux openat/unlinkat flags used by CHROTE's Linux service lane.
const fileAtRemoveDirectory = 0x200
const fileOpenPath = 0x200000

// NewFilesHandler creates a new file API handler
func NewFilesHandler() *FilesHandler {
	return NewFilesHandlerWithFormationsDataRoot(strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_DATA_ROOT")))
}

// NewFilesHandlerWithFormationsDataRoot creates a file API handler that keeps
// the supplied host-private Formations root outside the generic Files surface.
func NewFilesHandlerWithFormationsDataRoot(formationsDataRoot string) *FilesHandler {
	allowedRoots := core.GetAllowedRoots()
	deniedRoots := append(defaultDeniedFileRoots(), configuredFileRoots("CHROTE_FILE_DENY_PATHS", nil)...)
	deniedRoots = appendUniqueFileRoots(deniedRoots, canonicalFileRootAliases(formationsDataRoot)...)
	return &FilesHandler{
		allowedRoots:   allowedRoots,
		writeRoots:     configuredFileRoots("CHROTE_WRITE_ROOTS", allowedRoots),
		deniedRoots:    deniedRoots,
		deniedRootIDs:  fileRootIdentities(deniedRoots),
		maxUploadBytes: configuredMaxUploadBytes(),
	}
}

func fileRootIdentities(roots []string) map[fileIdentity]struct{} {
	identities := make(map[fileIdentity]struct{})
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		if identity, ok := identityFromFileInfo(info); ok {
			identities[identity] = struct{}{}
		}
	}
	return identities
}

func identityFromFileInfo(info os.FileInfo) (fileIdentity, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}, false
	}
	return fileIdentity{device: uint64(stat.Dev), inode: stat.Ino}, true
}

func (h *FilesHandler) deniedIdentitySnapshot() map[fileIdentity]struct{} {
	identities := make(map[fileIdentity]struct{}, len(h.deniedRootIDs)+len(h.deniedRoots))
	for identity := range h.deniedRootIDs {
		identities[identity] = struct{}{}
	}
	for identity := range fileRootIdentities(h.deniedRoots) {
		identities[identity] = struct{}{}
	}
	return identities
}

func canonicalFileRootAliases(root string) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	absolute = filepath.Clean(absolute)
	aliases := []string{absolute}
	canonical, err := canonicalPathAllowMissing(absolute)
	if err == nil && canonical != absolute {
		aliases = append(aliases, canonical)
	}
	return aliases
}

func appendUniqueFileRoots(roots []string, additions ...string) []string {
	seen := make(map[string]bool, len(roots)+len(additions))
	for _, root := range roots {
		seen[root] = true
	}
	for _, root := range additions {
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	return roots
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
	matchedRoot := ""
	for _, root := range roots {
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absoluteRoot = filepath.Clean(absoluteRoot)
		if core.IsPathUnderRoot(path, absoluteRoot) && len(absoluteRoot) > len(matchedRoot) {
			matchedRoot = absoluteRoot
		}
	}
	return matchedRoot, matchedRoot != ""
}

func (h *FilesHandler) isDeniedPath(path string) bool {
	if isSensitiveCredentialPath(path) {
		return true
	}
	_, denied := isPathUnderAnyRoot(path, h.deniedRoots)
	return denied
}

func (h *FilesHandler) isDeniedMutationPath(path string) bool {
	if h.isDeniedPath(path) {
		return true
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	absolute = filepath.Clean(absolute)
	for _, deniedRoot := range h.deniedRoots {
		deniedAbsolute, err := filepath.Abs(deniedRoot)
		if err != nil {
			continue
		}
		if core.IsPathUnderRoot(filepath.Clean(deniedAbsolute), absolute) {
			return true
		}
	}
	return false
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
	if h.isDeniedMutationPath(normalized) {
		return PathResult{Error: "Sensitive path not available in CHROTE Files"}
	}

	resolvedParent, err := canonicalPathAllowMissing(filepath.Dir(normalized))
	if err != nil {
		return PathResult{Error: "Invalid path"}
	}
	if h.isDeniedPath(resolvedParent) {
		return PathResult{Error: "Sensitive path not available in CHROTE Files"}
	}
	operationPath := filepath.Join(resolvedParent, filepath.Base(normalized))
	canonicalOperation, err := canonicalPathAllowMissing(operationPath)
	if err != nil {
		return PathResult{Error: "Invalid path"}
	}
	if h.isDeniedMutationPath(operationPath) || h.isDeniedMutationPath(canonicalOperation) {
		return PathResult{Error: "Sensitive path not available in CHROTE Files"}
	}
	matchedRoot, allowed := isPathUnderAnyRoot(operationPath, h.allowedRoots)
	if !allowed {
		return PathResult{Error: "Path not allowed"}
	}
	writeRoot, writable := isPathUnderAnyRoot(operationPath, h.writeRoots)
	return PathResult{
		Path:        operationPath,
		Root:        matchedRoot,
		Writable:    writable,
		IsWriteRoot: writable && filepath.Clean(operationPath) == filepath.Clean(writeRoot),
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

func (h *FilesHandler) validateConfinedFile(file *os.File, logicalPath string, denied map[fileIdentity]struct{}) error {
	if file == nil || h.isDeniedPath(logicalPath) {
		return errFilesPathChanged
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if identity, ok := identityFromFileInfo(info); ok {
		if _, denied := denied[identity]; denied {
			return errFilesPathChanged
		}
	}
	return nil
}

func (h *FilesHandler) openConfinedRoot(root string, denied map[fileIdentity]struct{}) (*os.File, string, error) {
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
	if err := h.validateConfinedFile(current, string(os.PathSeparator), denied); err != nil {
		_ = current.Close()
		return nil, "", err
	}
	if canonicalRoot == string(os.PathSeparator) {
		return current, canonicalRoot, nil
	}

	openedPath := string(os.PathSeparator)
	for _, component := range strings.Split(strings.TrimPrefix(canonicalRoot, string(os.PathSeparator)), string(os.PathSeparator)) {
		next, err := openDirectoryAt(current, component)
		_ = current.Close()
		if err != nil {
			return nil, "", err
		}
		openedPath = filepath.Join(openedPath, component)
		if err := h.validateConfinedFile(next, openedPath, denied); err != nil {
			_ = next.Close()
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
	denied := h.deniedIdentitySnapshot()
	current, canonicalRoot, err := h.openConfinedRoot(result.Root, denied)
	if err != nil {
		return nil, err
	}
	components, err := confinedPathComponents(canonicalRoot, result.Path)
	if err != nil {
		_ = current.Close()
		return nil, err
	}
	openedPath := canonicalRoot
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
		openedPath = filepath.Join(openedPath, component)
		if err := h.validateConfinedFile(next, openedPath, denied); err != nil {
			_ = next.Close()
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
	denied := h.deniedIdentitySnapshot()
	current, canonicalRoot, err := h.openConfinedRoot(result.Root, denied)
	if err != nil {
		return nil, err
	}
	components, err := confinedPathComponents(canonicalRoot, result.Path)
	if err != nil {
		_ = current.Close()
		return nil, err
	}
	openedPath := canonicalRoot
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
		openedPath = filepath.Join(openedPath, component)
		if err := h.validateConfinedFile(next, openedPath, denied); err != nil {
			_ = next.Close()
			return nil, err
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

func (h *FilesHandler) writeConfinedFileAt(parent *confinedParent, logicalPath string, body []byte) error {
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
	if err := h.validateConfinedFile(file, logicalPath, h.deniedIdentitySnapshot()); err != nil {
		return err
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

func (h *FilesHandler) validateMutationEntry(parent *confinedParent, logicalPath string, allowMissing bool) error {
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
	defer file.Close()
	return h.validateConfinedFile(file, logicalPath, h.deniedIdentitySnapshot())
}

func (h *FilesHandler) removeConfinedAt(parent *os.File, name, logicalPath string, denied map[fileIdentity]struct{}) error {
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
	identity, hasIdentity := identityFromFileInfo(info)
	if err := h.validateConfinedFile(directory, logicalPath, denied); err != nil {
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
			if err := h.removeConfinedAt(directory, child, filepath.Join(logicalPath, child), denied); err != nil {
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
	confirmationIdentity, confirmationHasIdentity := identityFromFileInfo(confirmationInfo)
	if hasIdentity != confirmationHasIdentity || hasIdentity && identity != confirmationIdentity {
		return errFilesPathChanged
	}
	return unlinkDirectoryAt(parent, name)
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
	allowedRoots := h.visibleAllowedRoots()
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

func (h *FilesHandler) visibleAllowedRoots() []string {
	visible := make([]string, 0, len(h.allowedRoots))
	for _, root := range h.allowedRoots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if h.isDeniedPath(absolute) {
			continue
		}
		canonical, err := canonicalPathAllowMissing(absolute)
		if err != nil || h.isDeniedPath(canonical) {
			continue
		}
		visible = append(visible, root)
	}
	return visible
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
	denied := h.deniedIdentitySnapshot()
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
			if openErr = h.validateConfinedFile(entryFile, fullPath, denied); openErr == nil {
				info, openErr = entryFile.Stat()
			}
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

	if result.Error != "" || result.IsRoot || !result.Writable || result.IsWriteRoot {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "Path is outside configured write roots"
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

	if err := h.writeConfinedFileAt(parent, result.Path, body); err != nil {
		writeFilesUseError(w, err, false)
		return
	}

	core.WriteJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// RenameResource handles PATCH /api/files/resources/* - rename/move
func (h *FilesHandler) RenameResource(w http.ResponseWriter, r *http.Request) {
	requestPath := "/" + r.PathValue("path")
	result := h.resolveMutationPath(requestPath)

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

	destResult := h.resolveMutationPath(req.Destination)
	if destResult.Error != "" || destResult.IsRoot || !destResult.Writable || destResult.IsWriteRoot {
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
	if err := h.validateMutationEntry(sourceParent, result.Path, false); err != nil {
		writeFilesUseError(w, err, false)
		return
	}
	if err := h.validateMutationEntry(destinationParent, destResult.Path, true); err != nil {
		writeFilesUseError(w, err, false)
		return
	}

	if err := syscall.Renameat(int(sourceParent.directory.Fd()), sourceParent.name, int(destinationParent.directory.Fd()), destinationParent.name); err != nil {
		writeFilesUseError(w, err, false)
		return
	}

	core.WriteJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// DeleteResource handles DELETE /api/files/resources/* - delete file/folder
func (h *FilesHandler) DeleteResource(w http.ResponseWriter, r *http.Request) {
	requestPath := "/" + r.PathValue("path")
	result := h.resolveMutationPath(requestPath)

	if result.Error != "" || result.IsRoot || !result.Writable || result.IsWriteRoot {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "Path is outside configured write roots"
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
	if err := h.validateMutationEntry(parent, result.Path, false); err != nil {
		writeFilesUseError(w, err, true)
		return
	}

	if err := h.removeConfinedAt(parent.directory, parent.name, result.Path, h.deniedIdentitySnapshot()); err != nil {
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
