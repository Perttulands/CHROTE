package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chrote/server/internal/core"
)

func TestFilesHandler_NewFilesHandler(t *testing.T) {
	handler := NewFilesHandler()

	if handler == nil {
		t.Fatal("NewFilesHandler() returned nil")
	}
	// Check that handler uses the configured roots (default is 2)
	expectedRoots := len(core.GetAllowedRoots())
	if len(handler.allowedRoots) != expectedRoots {
		t.Errorf("Expected %d allowed roots, got %d", expectedRoots, len(handler.allowedRoots))
	}
}

func newFilesHandlerForRoots(t *testing.T, allowedRoots []string, deniedRoot string) *FilesHandler {
	t.Helper()
	core.ResetConfigForTesting()
	t.Cleanup(core.ResetConfigForTesting)
	t.Setenv("CHROTE_ROOTS", strings.Join(allowedRoots, ","))
	t.Setenv("CHROTE_WRITE_ROOTS", strings.Join(allowedRoots, ","))
	t.Setenv("CHROTE_FILE_DENY_PATHS", deniedRoot)
	return NewFilesHandler()
}

func fileItemNames(items []FileItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	sort.Strings(names)
	return names
}

// A configured deny root must not be disclosed through either the virtual root
// or a parent directory listing.
func TestFilesHandlerConfiguredDenyRootIsFilteredFromListings(t *testing.T) {
	parentRoot := t.TempDir()
	deniedRoot := filepath.Join(parentRoot, "protected-private")
	ordinaryRoot := filepath.Join(parentRoot, "ordinary")
	for _, path := range []string{deniedRoot, ordinaryRoot} {
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	handler := newFilesHandlerForRoots(t, []string{ordinaryRoot, deniedRoot}, deniedRoot)
	req := httptest.NewRequest(http.MethodGet, "/api/files/resources/", nil)
	rec := httptest.NewRecorder()
	handler.ListRoot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListRoot status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var rootResponse DirectoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &rootResponse); err != nil {
		t.Fatalf("decode root listing: %v", err)
	}
	if got := fileItemNames(rootResponse.Items); len(got) != 1 || got[0] != strings.TrimPrefix(ordinaryRoot, "/") {
		t.Fatalf("root listing names = %q, want only ordinary root", got)
	}

	parentHandler := newFilesHandlerForRoots(t, []string{parentRoot}, deniedRoot)
	parentReq := httptest.NewRequest(http.MethodGet, "/api/files/resources/ignored", nil)
	parentReq.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(parentRoot), "/"))
	parentRec := httptest.NewRecorder()
	parentHandler.GetResource(parentRec, parentReq)
	if parentRec.Code != http.StatusOK {
		t.Fatalf("parent listing status = %d, want 200: %s", parentRec.Code, parentRec.Body.String())
	}
	var parentResponse DirectoryResponse
	if err := json.Unmarshal(parentRec.Body.Bytes(), &parentResponse); err != nil {
		t.Fatalf("decode parent listing: %v", err)
	}
	if got := fileItemNames(parentResponse.Items); len(got) != 1 || got[0] != "ordinary" {
		t.Fatalf("parent listing names = %q, want only ordinary", got)
	}
}

func TestFilesHandler_ResolveSafePath_Root(t *testing.T) {
	handler := NewFilesHandler()

	tests := []struct {
		name       string
		path       string
		wantIsRoot bool
	}{
		{"root path", "/", true},
		{"empty path", "", true},
		{"dot path", ".", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.resolveSafePath(tt.path)
			if result.IsRoot != tt.wantIsRoot {
				t.Errorf("resolveSafePath(%q) isRoot = %v, want %v", tt.path, result.IsRoot, tt.wantIsRoot)
			}
		})
	}
}

func TestFilesHandler_ResolveSafePath_NotAllowed(t *testing.T) {
	handler := NewFilesHandler()

	tests := []struct {
		name string
		path string
	}{
		{"etc passwd", "/etc/passwd"},
		{"random path", "/foo/bar"},
		{"windows path", "C:/Windows"},
		{"tmp path", "/tmp/test"},
		{"home path", "/home/user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.resolveSafePath(tt.path)
			if result.Error == "" && !result.IsRoot {
				t.Errorf("resolveSafePath(%q) expected error, got none (path: %s)", tt.path, result.Path)
			}
		})
	}
}

func TestFilesHandler_ListRoot(t *testing.T) {
	handler := NewFilesHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/files/resources/", nil)
	rec := httptest.NewRecorder()

	handler.ListRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ListRoot status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response DirectoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if !response.IsDir {
		t.Error("Root should be a directory")
	}

	expectedRoots := len(core.GetAllowedRoots())
	if len(response.Items) != expectedRoots {
		t.Errorf("Expected %d root items, got %d", expectedRoots, len(response.Items))
	}

	// Check that all items are directories
	for _, item := range response.Items {
		if !item.IsDir {
			t.Errorf("Root item %s should be a directory", item.Name)
		}
	}
}

func TestFilesHandler_GetResource_NotAllowed(t *testing.T) {
	handler := NewFilesHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/files/resources/etc/passwd", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("GetResource status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestFilesHandler_GetResource_EmptyPath(t *testing.T) {
	handler := NewFilesHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/files/resources/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GetResource for root status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response DirectoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if !response.IsDir {
		t.Error("Root should be a directory")
	}
}

func TestFilesHandler_CreateResource_AtRoot(t *testing.T) {
	handler := NewFilesHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Try to create file at root (should fail)
	req := httptest.NewRequest(http.MethodPost, "/api/files/resources/testfile.txt", bytes.NewBufferString("test"))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("CreateResource at root status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestFilesHandler_RenameResource_InvalidPath(t *testing.T) {
	handler := NewFilesHandler()

	// Use a path that is never under any allowed root
	req := httptest.NewRequest(http.MethodPatch, "/api/files/resources/etc/shadow", bytes.NewBufferString("{}"))
	req.SetPathValue("path", "/etc/shadow")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.RenameResource(rec, req)

	// Path is forbidden because it's not under an allowed root
	if rec.Code != http.StatusForbidden {
		t.Errorf("RenameResource with invalid path status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestFilesHandler_RenameResource_NotAllowedPath(t *testing.T) {
	handler := NewFilesHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := RenameRequest{
		Action:      "rename",
		Destination: "/tmp/dest.txt",
	}
	bodyBytes, _ := json.Marshal(body)

	// Try to rename a path not under allowed roots
	req := httptest.NewRequest(http.MethodPatch, "/api/files/resources/tmp/test.txt", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("RenameResource with not allowed path status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestFilesHandlerRenameRefusesToClobberExistingDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "occupied.txt")
	writeFileFixture(t, source, "source content")
	writeFileFixture(t, destination, "destination content")
	handler := &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		maxUploadBytes: defaultMaxUploadBytes,
	}
	body, err := json.Marshal(RenameRequest{Action: "rename", Destination: destination})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/files/resources/ignored", bytes.NewReader(body))
	req.SetPathValue("path", source)
	rec := httptest.NewRecorder()

	handler.RenameResource(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for an occupied destination: %s", rec.Code, rec.Body.String())
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "destination content" {
		t.Fatalf("destination was clobbered: content=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "source content" {
		t.Fatalf("source was consumed by a refused rename: content=%q err=%v", got, err)
	}
}

func TestFilesHandlerRenameRefusesToClobberExistingDirectoryDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source-dir")
	destination := filepath.Join(root, "occupied-dir")
	writeFileFixture(t, filepath.Join(source, "child.txt"), "source child")
	writeFileFixture(t, filepath.Join(destination, "keep.txt"), "destination child")
	handler := &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		maxUploadBytes: defaultMaxUploadBytes,
	}
	body, err := json.Marshal(RenameRequest{Action: "rename", Destination: destination})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/files/resources/ignored", bytes.NewReader(body))
	req.SetPathValue("path", source)
	rec := httptest.NewRecorder()

	handler.RenameResource(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for an occupied directory destination: %s", rec.Code, rec.Body.String())
	}
	if got, err := os.ReadFile(filepath.Join(destination, "keep.txt")); err != nil || string(got) != "destination child" {
		t.Fatalf("destination directory was replaced: content=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(source, "child.txt")); err != nil {
		t.Fatalf("source directory was consumed by a refused rename: %v", err)
	}
}

func TestFilesHandlerRenameStillMovesToFreeDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "moved.txt")
	writeFileFixture(t, source, "source content")
	handler := &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		maxUploadBytes: defaultMaxUploadBytes,
	}
	body, err := json.Marshal(RenameRequest{Action: "rename", Destination: destination})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/files/resources/ignored", bytes.NewReader(body))
	req.SetPathValue("path", source)
	rec := httptest.NewRecorder()

	handler.RenameResource(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a free destination: %s", rec.Code, rec.Body.String())
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "source content" {
		t.Fatalf("destination content = %q err=%v, want the moved source", got, err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists after a successful rename: %v", err)
	}
}

func TestFilesHandler_DeleteResource_AtRoot(t *testing.T) {
	handler := NewFilesHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/", nil)
	req.SetPathValue("path", "")
	rec := httptest.NewRecorder()

	handler.DeleteResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("DeleteResource at root status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestFilesHandler_DownloadFile_AtRoot(t *testing.T) {
	handler := NewFilesHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/files/raw/", nil)
	req.SetPathValue("path", "")
	rec := httptest.NewRecorder()

	handler.DownloadFile(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("DownloadFile at root status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestFilesHandler_RegisterRoutes(t *testing.T) {
	handler := NewFilesHandler()
	mux := http.NewServeMux()

	// Should not panic
	handler.RegisterRoutes(mux)
}

func TestFilesHandlerRootReadPolicyBlocksSensitivePaths(t *testing.T) {
	handler := &FilesHandler{
		allowedRoots:   []string{"/"},
		writeRoots:     []string{t.TempDir()},
		deniedRoots:    defaultDeniedFileRoots(),
		maxUploadBytes: defaultMaxUploadBytes,
	}

	for _, path := range []string{
		"/proc/self/environ",
		"/etc/chrote/chrote-srv.env",
		"/home/operator/.config/gh/hosts.yml",
		"/home/operator/.ssh/config",
	} {
		t.Run(path, func(t *testing.T) {
			result := handler.resolveSafePath(path)
			if result.Error == "" {
				t.Fatalf("resolveSafePath(%q) allowed a sensitive path: %#v", path, result)
			}
		})
	}
}

func TestFilesHandlerReadAllKeepsOrdinaryFilesUseful(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("useful"), 0644); err != nil {
		t.Fatal(err)
	}
	handler := &FilesHandler{
		allowedRoots:   []string{"/"},
		writeRoots:     []string{root},
		deniedRoots:    defaultDeniedFileRoots(),
		maxUploadBytes: defaultMaxUploadBytes,
	}

	result := handler.resolveSafePath(path)
	if result.Error != "" || result.Path != path {
		t.Fatalf("ordinary file rejected: %#v", result)
	}
}

func TestFilesHandlerSensitiveAllowPathsExposeOnlyConfiguredOwnerPrivateFiles(t *testing.T) {
	root := t.TempDir()
	ownerRoot := filepath.Join(root, "home", "operator")
	outsidePrivate := filepath.Join(root, "outside", ".ssh")
	for _, path := range []string{
		filepath.Join(ownerRoot, ".hermes"),
		filepath.Join(ownerRoot, ".ssh"),
		outsidePrivate,
	} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	outsideSecret := filepath.Join(outsidePrivate, "config")
	if err := os.WriteFile(outsideSecret, []byte("not read"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(ownerRoot, ".ssh", "outside")
	if err := os.Symlink(outsidePrivate, alias); err != nil {
		t.Fatal(err)
	}

	core.ResetConfigForTesting()
	t.Cleanup(core.ResetConfigForTesting)
	t.Setenv("CHROTE_ROOTS", root)
	t.Setenv("CHROTE_WRITE_ROOTS", root)
	t.Setenv("CHROTE_FILE_ALLOW_SENSITIVE_PATHS", ownerRoot)
	handler := NewFilesHandler()

	for _, path := range []string{
		filepath.Join(ownerRoot, ".hermes"),
		filepath.Join(ownerRoot, ".ssh", "new-key.pub"),
	} {
		if result := handler.resolveSafePath(path); result.Error != "" || !result.Writable {
			t.Fatalf("resolveSafePath(%q) = %#v, want opted-in owner-private writable path", path, result)
		}
		if result := handler.resolveMutationPath(path); result.Error != "" || !result.Writable {
			t.Fatalf("resolveMutationPath(%q) = %#v, want opted-in owner-private writable path", path, result)
		}
	}
	if result := handler.resolveSafePath(outsideSecret); result.Error != "Sensitive path not available in CHROTE Files" {
		t.Fatalf("outside owner-private path resolved as %#v, want sensitive-path rejection", result)
	}
	if result := handler.resolveSafePath(filepath.Join(alias, "config")); result.Error != "Sensitive path not available in CHROTE Files" {
		t.Fatalf("canonical escape through opted-in root resolved as %#v, want sensitive-path rejection", result)
	}
}

func TestFilesHandlerDenyRootsOverrideSensitiveAllowPaths(t *testing.T) {
	root := t.TempDir()
	ownerRoot := filepath.Join(root, "home", "operator")
	deniedRoot := filepath.Join(ownerRoot, ".hermes", "deny-me")
	if err := os.MkdirAll(deniedRoot, 0700); err != nil {
		t.Fatal(err)
	}

	core.ResetConfigForTesting()
	t.Cleanup(core.ResetConfigForTesting)
	t.Setenv("CHROTE_ROOTS", root)
	t.Setenv("CHROTE_WRITE_ROOTS", root)
	t.Setenv("CHROTE_FILE_ALLOW_SENSITIVE_PATHS", ownerRoot)
	t.Setenv("CHROTE_FILE_DENY_PATHS", deniedRoot)
	handler := NewFilesHandler()

	if result := handler.resolveSafePath(deniedRoot); result.Error != "Sensitive path not available in CHROTE Files" {
		t.Fatalf("resolveSafePath(%q) = %#v, want explicit private-root rejection", deniedRoot, result)
	}
	if result := handler.resolveMutationPath(deniedRoot); result.Error != "Sensitive path not available in CHROTE Files" {
		t.Fatalf("resolveMutationPath(%q) = %#v, want explicit private-root rejection", deniedRoot, result)
	}
}

func TestFilesHandlerSensitiveAllowPathsSupportHTTPReadWriteRenameDelete(t *testing.T) {
	root := t.TempDir()
	ownerRoot := filepath.Join(root, "home", "operator")
	privateRoot := filepath.Join(ownerRoot, ".hermes")
	if err := os.MkdirAll(privateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	core.ResetConfigForTesting()
	t.Cleanup(core.ResetConfigForTesting)
	t.Setenv("CHROTE_ROOTS", root)
	t.Setenv("CHROTE_WRITE_ROOTS", root)
	t.Setenv("CHROTE_FILE_ALLOW_SENSITIVE_PATHS", ownerRoot)
	handler := NewFilesHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	original := filepath.Join(privateRoot, "probe.txt")
	renamed := filepath.Join(privateRoot, "renamed.txt")
	request := func(method, path string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/files/resources"+filepath.ToSlash(path), bytes.NewReader(body))
		if method == http.MethodPatch {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := request(http.MethodPost, original, []byte("owner-private")); rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodGet, privateRoot, nil); rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rawReq := httptest.NewRequest(http.MethodGet, "/api/files/raw"+filepath.ToSlash(original), nil)
	rawRec := httptest.NewRecorder()
	mux.ServeHTTP(rawRec, rawReq)
	if rawRec.Code != http.StatusOK || rawRec.Body.String() != "owner-private" {
		t.Fatalf("raw read = %d %q, want 200 and task-owned bytes", rawRec.Code, rawRec.Body.String())
	}
	renameBody, err := json.Marshal(RenameRequest{Action: "rename", Destination: renamed})
	if err != nil {
		t.Fatal(err)
	}
	if rec := request(http.MethodPatch, original, renameBody); rec.Code != http.StatusOK {
		t.Fatalf("rename status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodDelete, renamed, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(renamed); !os.IsNotExist(err) {
		t.Fatalf("renamed private fixture still exists: %v", err)
	}
}

func TestFilesHandlerMutationRequiresConfiguredWriteRoot(t *testing.T) {
	readRoot := t.TempDir()
	writeRoot := t.TempDir()
	readOnlyFile := filepath.Join(readRoot, "read-only.txt")
	if err := os.WriteFile(readOnlyFile, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	handler := &FilesHandler{
		allowedRoots:   []string{readRoot, writeRoot},
		writeRoots:     []string{writeRoot},
		maxUploadBytes: defaultMaxUploadBytes,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/files/resources/ignored", bytes.NewBufferString("replace"))
	req.SetPathValue("path", readOnlyFile)
	rec := httptest.NewRecorder()
	handler.CreateResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	content, err := os.ReadFile(readOnlyFile)
	if err != nil || string(content) != "keep" {
		t.Fatalf("read-only file changed: content=%q err=%v", content, err)
	}
}

func TestFilesHandlerSymlinkCannotEscapeReadRoot(t *testing.T) {
	readRoot := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(readRoot, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	handler := &FilesHandler{
		allowedRoots:   []string{readRoot},
		writeRoots:     []string{readRoot},
		maxUploadBytes: defaultMaxUploadBytes,
	}

	if result := handler.resolveSafePath(filepath.Join(link, "secret.txt")); result.Error == "" {
		t.Fatalf("symlink escape allowed: %#v", result)
	}
}

func TestFilesHandlerDeleteRemovesSymlinkNotTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(kept, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	handler := &FilesHandler{allowedRoots: []string{root}, writeRoots: []string{root}, maxUploadBytes: defaultMaxUploadBytes}
	req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
	req.SetPathValue("path", link)
	rec := httptest.NewRecorder()

	handler.DeleteResource(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("symlink still exists after delete: %v", err)
	}
	if content, err := os.ReadFile(kept); err != nil || string(content) != "keep" {
		t.Fatalf("symlink target was changed: content=%q err=%v", content, err)
	}
}

func TestFilesHandlerRenameMovesSymlinkNotTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	moved := filepath.Join(root, "moved-link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	handler := &FilesHandler{allowedRoots: []string{root}, writeRoots: []string{root}, maxUploadBytes: defaultMaxUploadBytes}
	body, err := json.Marshal(RenameRequest{Action: "rename", Destination: moved})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/files/resources/ignored", bytes.NewReader(body))
	req.SetPathValue("path", link)
	rec := httptest.NewRecorder()

	handler.RenameResource(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if info, err := os.Lstat(moved); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("renamed path is not the symlink: info=%v err=%v", info, err)
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "keep" {
		t.Fatalf("symlink target was changed: content=%q err=%v", content, err)
	}
}

func TestFilesHandlerDeleteCanRemoveOutboundSymlinkWithoutTouchingTarget(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(target, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "outbound-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	handler := &FilesHandler{allowedRoots: []string{root}, writeRoots: []string{root}, maxUploadBytes: defaultMaxUploadBytes}
	req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
	req.SetPathValue("path", link)
	rec := httptest.NewRecorder()

	handler.DeleteResource(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("outbound symlink still exists after delete: %v", err)
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "keep" {
		t.Fatalf("outbound target was changed: content=%q err=%v", content, err)
	}
}

func TestFilesHandlerMutationCannotEscapeThroughParentSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(victim, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(root, "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Fatal(err)
	}
	handler := &FilesHandler{allowedRoots: []string{root}, writeRoots: []string{root}, maxUploadBytes: defaultMaxUploadBytes}
	req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
	req.SetPathValue("path", filepath.Join(escape, "victim.txt"))
	rec := httptest.NewRecorder()

	handler.DeleteResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	if content, err := os.ReadFile(victim); err != nil || string(content) != "keep" {
		t.Fatalf("outbound victim was changed: content=%q err=%v", content, err)
	}
}

func TestFilesHandlerMutationCannotRemoveConfiguredWriteRoot(t *testing.T) {
	root := t.TempDir()
	handler := &FilesHandler{allowedRoots: []string{root}, writeRoots: []string{root}, maxUploadBytes: defaultMaxUploadBytes}
	req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
	req.SetPathValue("path", root)
	rec := httptest.NewRecorder()

	handler.DeleteResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("write root was removed: info=%v err=%v", info, err)
	}
}

func TestFilesHandlerMutationCannotMoveOrRemoveNestedConfiguredWriteRoot(t *testing.T) {
	for _, order := range []struct {
		name       string
		writeRoots func(parent, child string) []string
	}{
		{name: "parent first", writeRoots: func(parent, child string) []string { return []string{parent, child} }},
		{name: "child first", writeRoots: func(parent, child string) []string { return []string{child, parent} }},
	} {
		for _, action := range []string{"delete", "rename"} {
			t.Run(order.name+"/"+action, func(t *testing.T) {
				parent := t.TempDir()
				child := filepath.Join(parent, "nested-write-root")
				if err := os.Mkdir(child, 0755); err != nil {
					t.Fatal(err)
				}
				handler := &FilesHandler{
					allowedRoots:   []string{parent},
					writeRoots:     order.writeRoots(parent, child),
					maxUploadBytes: defaultMaxUploadBytes,
				}

				var req *http.Request
				switch action {
				case "delete":
					req = httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
					req.SetPathValue("path", child)
				case "rename":
					destination := filepath.Join(parent, "moved-write-root")
					body, err := json.Marshal(RenameRequest{Action: "rename", Destination: destination})
					if err != nil {
						t.Fatal(err)
					}
					req = httptest.NewRequest(http.MethodPatch, "/api/files/resources/ignored", bytes.NewReader(body))
					req.SetPathValue("path", child)
				}
				rec := httptest.NewRecorder()

				if action == "delete" {
					handler.DeleteResource(rec, req)
				} else {
					handler.RenameResource(rec, req)
				}

				if rec.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want 403 for configured nested write root: %s", rec.Code, rec.Body.String())
				}
				if info, err := os.Stat(child); err != nil || !info.IsDir() {
					t.Fatalf("nested write root was moved or removed: info=%v err=%v", info, err)
				}
			})
		}
	}
}

func TestFilesHandlerRenameRejectsAncestorOfDeniedRoot(t *testing.T) {
	root := t.TempDir()
	container := filepath.Join(root, "container")
	privateRoot := filepath.Join(container, "protected-private")
	writeFileFixture(t, filepath.Join(privateRoot, "authority.json"), "private")
	handler := &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		deniedRoots:    []string{privateRoot},
		deniedRootIDs:  fileRootIdentities([]string{privateRoot}),
		maxUploadBytes: defaultMaxUploadBytes,
	}
	destination := filepath.Join(root, "moved-container")
	body, err := json.Marshal(RenameRequest{Action: "rename", Destination: destination})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/files/resources/ignored", bytes.NewReader(body))
	req.SetPathValue("path", container)
	rec := httptest.NewRecorder()

	handler.RenameResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for ancestor of denied root: %s", rec.Code, rec.Body.String())
	}
	if got, err := os.ReadFile(filepath.Join(privateRoot, "authority.json")); err != nil || string(got) != "private" {
		t.Fatalf("denied root was moved or changed: content=%q err=%v", got, err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("ancestor rename created destination: %v", err)
	}
}

func TestFilesHandlerDeleteRejectsAncestorOfDeniedFile(t *testing.T) {
	root := t.TempDir()
	container := filepath.Join(root, "container")
	deniedFile := filepath.Join(container, "authority.json")
	writeFileFixture(t, deniedFile, "private")
	handler := &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		deniedRoots:    []string{deniedFile},
		deniedRootIDs:  fileRootIdentities([]string{deniedFile}),
		maxUploadBytes: defaultMaxUploadBytes,
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
	req.SetPathValue("path", container)
	rec := httptest.NewRecorder()

	handler.DeleteResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for ancestor of denied file: %s", rec.Code, rec.Body.String())
	}
	if got, err := os.ReadFile(deniedFile); err != nil || string(got) != "private" {
		t.Fatalf("denied file was removed or changed: content=%q err=%v", got, err)
	}
}

func TestFilesHandlerRejectsOversizedUpload(t *testing.T) {
	root := t.TempDir()
	handler := &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		maxUploadBytes: 4,
	}
	target := filepath.Join(root, "large.txt")
	req := httptest.NewRequest(http.MethodPost, "/api/files/resources/ignored", bytes.NewBufferString("12345"))
	req.SetPathValue("path", target)
	rec := httptest.NewRecorder()

	handler.CreateResource(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("oversized upload created target: %v", err)
	}
}

func TestFilesHandlerDownloadRejectsPrivateRootSwappedAfterPathValidation(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "visible")
	private := filepath.Join(root, "protected-private")
	writeFileFixture(t, filepath.Join(visible, "authority.json"), "public")
	writeFileFixture(t, filepath.Join(private, "authority.json"), "private")

	handler := newFilesHandlerForRoots(t, []string{root}, private)
	handler.operationHook = swapPrivateRootAfterStage(t, "download", visible, private)
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw/ignored", nil)
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(filepath.Join(visible, "authority.json")), "/"))
	rec := httptest.NewRecorder()

	handler.DownloadFile(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 after private-root swap: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "private") {
		t.Fatalf("private content escaped through download: %q", rec.Body.String())
	}
}

func TestFilesHandlerListingRejectsPrivateRootSwappedAfterPathValidation(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "visible")
	private := filepath.Join(root, "protected-private")
	writeFileFixture(t, filepath.Join(visible, "notes.txt"), "public")
	writeFileFixture(t, filepath.Join(private, "authority.json"), "private")

	handler := newFilesHandlerForRoots(t, []string{root}, private)
	handler.operationHook = swapPrivateRootAfterStage(t, "get", visible, private)
	req := httptest.NewRequest(http.MethodGet, "/api/files/resources/ignored", nil)
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(visible), "/"))
	rec := httptest.NewRecorder()

	handler.GetResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 after private-root swap: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "authority.json") {
		t.Fatalf("private listing escaped: %q", rec.Body.String())
	}
}

func TestFilesHandlerCreateRejectsPrivateRootSwappedAfterPathValidation(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "visible")
	private := filepath.Join(root, "protected-private")
	if err := os.MkdirAll(visible, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(private, 0755); err != nil {
		t.Fatal(err)
	}

	handler := newFilesHandlerForRoots(t, []string{root}, private)
	handler.operationHook = swapPrivateRootAfterStage(t, "create", visible, private)
	target := filepath.Join(visible, "injected.txt")
	req := httptest.NewRequest(http.MethodPost, "/api/files/resources/ignored", bytes.NewBufferString("must not enter private root"))
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(target), "/"))
	rec := httptest.NewRecorder()

	handler.CreateResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 after private-root swap: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("create reached swapped private root: %v", err)
	}
}

func TestFilesHandlerDeleteRejectsPrivateRootSwappedAfterPathValidation(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "visible")
	private := filepath.Join(root, "protected-private")
	writeFileFixture(t, filepath.Join(visible, "victim.txt"), "public")
	privateVictim := filepath.Join(private, "victim.txt")
	writeFileFixture(t, privateVictim, "private")

	handler := newFilesHandlerForRoots(t, []string{root}, private)
	handler.operationHook = swapPrivateRootAfterStage(t, "delete", visible, private)
	req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(filepath.Join(visible, "victim.txt")), "/"))
	rec := httptest.NewRecorder()

	handler.DeleteResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 after private-root swap: %s", rec.Code, rec.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(visible, "victim.txt"))
	if err != nil || string(content) != "private" {
		t.Fatalf("delete reached swapped private root: content=%q err=%v", content, err)
	}
}

func TestFilesHandlerRenameRejectsPrivateRootSwappedAfterPathValidation(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "visible")
	private := filepath.Join(root, "protected-private")
	writeFileFixture(t, filepath.Join(visible, "source.txt"), "public")
	privateSource := filepath.Join(private, "source.txt")
	writeFileFixture(t, privateSource, "private")

	handler := newFilesHandlerForRoots(t, []string{root}, private)
	handler.operationHook = swapPrivateRootAfterStage(t, "rename", visible, private)
	destination := filepath.Join(visible, "moved.txt")
	body, err := json.Marshal(RenameRequest{Action: "rename", Destination: destination})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/files/resources/ignored", bytes.NewReader(body))
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(filepath.Join(visible, "source.txt")), "/"))
	rec := httptest.NewRecorder()

	handler.RenameResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 after private-root swap: %s", rec.Code, rec.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(visible, "source.txt"))
	if err != nil || string(content) != "private" {
		t.Fatalf("rename reached swapped private root: content=%q err=%v", content, err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("rename created destination inside private root: %v", err)
	}
}

func TestFilesHandlerDownloadRejectsParentReplacedByOutboundSymlink(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "visible")
	outside := t.TempDir()
	writeFileFixture(t, filepath.Join(visible, "secret.txt"), "public")
	writeFileFixture(t, filepath.Join(outside, "secret.txt"), "outside")

	handler := &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		deniedRoots:    defaultDeniedFileRoots(),
		maxUploadBytes: defaultMaxUploadBytes,
	}
	handler.operationHook = replaceDirectoryWithSymlinkAfterStage(t, "download", visible, outside)
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw/ignored", nil)
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(filepath.Join(visible, "secret.txt")), "/"))
	rec := httptest.NewRecorder()

	handler.DownloadFile(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 after outbound symlink swap: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "outside") {
		t.Fatalf("outbound content escaped through download: %q", rec.Body.String())
	}
}

func TestFilesHandlerMutationsRejectParentReplacedByOutboundSymlink(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		root := t.TempDir()
		visible := filepath.Join(root, "visible")
		outside := t.TempDir()
		if err := os.MkdirAll(visible, 0755); err != nil {
			t.Fatal(err)
		}
		handler := &FilesHandler{
			allowedRoots:   []string{root},
			writeRoots:     []string{root},
			deniedRoots:    defaultDeniedFileRoots(),
			maxUploadBytes: defaultMaxUploadBytes,
		}
		handler.operationHook = replaceDirectoryWithSymlinkAfterStage(t, "create", visible, outside)
		req := httptest.NewRequest(http.MethodPost, "/api/files/resources/ignored", bytes.NewBufferString("outside"))
		req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(filepath.Join(visible, "injected.txt")), "/"))
		rec := httptest.NewRecorder()

		handler.CreateResource(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("create status = %d, want 403 after outbound symlink swap: %s", rec.Code, rec.Body.String())
		}
		if _, err := os.Stat(filepath.Join(outside, "injected.txt")); !os.IsNotExist(err) {
			t.Fatalf("create escaped allowed root: %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		root := t.TempDir()
		visible := filepath.Join(root, "visible")
		outside := t.TempDir()
		writeFileFixture(t, filepath.Join(visible, "victim.txt"), "public")
		outsideVictim := filepath.Join(outside, "victim.txt")
		writeFileFixture(t, outsideVictim, "outside")
		handler := &FilesHandler{
			allowedRoots:   []string{root},
			writeRoots:     []string{root},
			deniedRoots:    defaultDeniedFileRoots(),
			maxUploadBytes: defaultMaxUploadBytes,
		}
		handler.operationHook = replaceDirectoryWithSymlinkAfterStage(t, "delete", visible, outside)
		req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
		req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(filepath.Join(visible, "victim.txt")), "/"))
		rec := httptest.NewRecorder()

		handler.DeleteResource(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("delete status = %d, want 403 after outbound symlink swap: %s", rec.Code, rec.Body.String())
		}
		if got, err := os.ReadFile(outsideVictim); err != nil || string(got) != "outside" {
			t.Fatalf("delete escaped allowed root: content=%q err=%v", got, err)
		}
	})
}

func TestFilesHandlerDownloadKeepsStableInRootSymlinkUseful(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFileFixture(t, target, "safe")
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	handler := &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		deniedRoots:    defaultDeniedFileRoots(),
		maxUploadBytes: defaultMaxUploadBytes,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw/ignored", nil)
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(link), "/"))
	rec := httptest.NewRecorder()

	handler.DownloadFile(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "safe" {
		t.Fatalf("stable in-root symlink download = %d %q, want 200 safe", rec.Code, rec.Body.String())
	}
}

func TestFilesHandlerDownloadRejectsStaticPrivateRootAlias(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "protected-private")
	writeFileFixture(t, filepath.Join(private, "authority.json"), "private")
	alias := filepath.Join(root, "protected-alias")
	if err := os.Symlink(private, alias); err != nil {
		t.Fatal(err)
	}
	handler := newFilesHandlerForRoots(t, []string{root}, private)
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw/ignored", nil)
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(filepath.Join(alias, "authority.json")), "/"))
	rec := httptest.NewRecorder()

	handler.DownloadFile(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("static private alias status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

func TestFilesHandlerDeleteRejectsPrivateRootSwappedIntoFinalDirectory(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "visible")
	private := filepath.Join(root, "protected-private")
	if err := os.MkdirAll(visible, 0755); err != nil {
		t.Fatal(err)
	}
	writeFileFixture(t, filepath.Join(private, "authority.json"), "private")
	handler := newFilesHandlerForRoots(t, []string{root}, private)
	handler.operationHook = swapPrivateRootAfterStage(t, "delete", visible, private)
	req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(visible), "/"))
	rec := httptest.NewRecorder()

	handler.DeleteResource(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 after final-directory swap: %s", rec.Code, rec.Body.String())
	}
	if got, err := os.ReadFile(filepath.Join(visible, "authority.json")); err != nil || string(got) != "private" {
		t.Fatalf("delete traversed swapped private root: content=%q err=%v", got, err)
	}
}

func TestFilesHandlerDeleteRecursesThroughPinnedDirectoryHandles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "nested")
	writeFileFixture(t, filepath.Join(target, "one", "two", "note.txt"), "delete")
	handler := &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		deniedRoots:    defaultDeniedFileRoots(),
		maxUploadBytes: defaultMaxUploadBytes,
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/files/resources/ignored", nil)
	req.SetPathValue("path", strings.TrimPrefix(filepath.ToSlash(target), "/"))
	rec := httptest.NewRecorder()

	handler.DeleteResource(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("recursive descriptor delete status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("recursive descriptor delete left target: %v", err)
	}
}

func writeFileFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func swapPrivateRootAfterStage(t *testing.T, wantStage, visible, private string) func(string) {
	t.Helper()
	swapped := false
	return func(stage string) {
		if stage != wantStage || swapped {
			return
		}
		swapped = true
		if err := os.Rename(visible, visible+"-moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(private, visible); err != nil {
			t.Fatal(err)
		}
	}
}

func replaceDirectoryWithSymlinkAfterStage(t *testing.T, wantStage, visible, outside string) func(string) {
	t.Helper()
	replaced := false
	return func(stage string) {
		if stage != wantStage || replaced {
			return
		}
		replaced = true
		if err := os.Rename(visible, visible+"-moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, visible); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFilesHandler_SuccessResponse(t *testing.T) {
	resp := SuccessResponse{Success: true}
	bytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded SuccessResponse
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if !decoded.Success {
		t.Error("Expected success=true")
	}
}

func TestFilesHandler_DirectoryResponse(t *testing.T) {
	resp := DirectoryResponse{
		IsDir: true,
		Items: []FileItem{
			{Name: "test.txt", Size: 100, IsDir: false, Type: "txt"},
			{Name: "subdir", Size: 0, IsDir: true, Type: ""},
		},
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded DirectoryResponse
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if !decoded.IsDir {
		t.Error("Expected isDir=true")
	}

	if len(decoded.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(decoded.Items))
	}
}

func TestFilesHandler_FileInfoResponse(t *testing.T) {
	resp := FileInfoResponse{
		IsDir:    false,
		Name:     "test.txt",
		Size:     1024,
		Modified: "2026-01-18T00:00:00Z",
		Type:     "txt",
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded FileInfoResponse
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.IsDir {
		t.Error("Expected isDir=false")
	}

	if decoded.Name != "test.txt" {
		t.Errorf("Name = %s, want test.txt", decoded.Name)
	}

	if decoded.Size != 1024 {
		t.Errorf("Size = %d, want 1024", decoded.Size)
	}

	if decoded.Type != "txt" {
		t.Errorf("Type = %s, want txt", decoded.Type)
	}
}
