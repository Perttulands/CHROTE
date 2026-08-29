package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

func TestFilesHandlerServesReadablePathUnderConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".ssh", "config")
	writeFileFixture(t, path, "readable fixture")
	t.Setenv("CHROTE_ROOTS", root)

	mux := http.NewServeMux()
	NewFilesHandler().RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw"+filepath.ToSlash(path), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "readable fixture" {
		t.Fatalf("readable under-root file = %d %q, want 200 with file content", rec.Code, rec.Body.String())
	}
}

func TestFilesHandlerUnreadablePathUnderConfiguredRootReturnsPermissionError(t *testing.T) {
	const helperEnv = "CHROTE_FILES_PERMISSION_HELPER"
	if os.Getenv(helperEnv) == "1" {
		mux := http.NewServeMux()
		NewFilesHandler().RegisterRoutes(mux)
		path := os.Getenv("CHROTE_FILES_PERMISSION_PATH")
		req := httptest.NewRequest(http.MethodGet, "/api/files/raw"+filepath.ToSlash(path), nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("unreadable under-root file status = %d, want 403: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "PERMISSION_DENIED") || !strings.Contains(strings.ToLower(rec.Body.String()), "permission denied") {
			t.Fatalf("unreadable under-root response does not preserve the permission cause: %s", rec.Body.String())
		}
		return
	}

	root, err := os.MkdirTemp("", "chrote-files-permission-")
	if err != nil {
		t.Fatalf("create permission fixture root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatalf("make permission fixture root searchable: %v", err)
	}
	path := filepath.Join(root, "unreadable.txt")
	if err := os.WriteFile(path, []byte("must not be served"), 0); err != nil {
		t.Fatalf("create unreadable fixture: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestFilesHandlerUnreadablePathUnderConfiguredRootReturnsPermissionError$")
	cmd.Env = append(os.Environ(),
		helperEnv+"=1",
		"CHROTE_ROOTS="+root,
		"CHROTE_FILES_PERMISSION_PATH="+path,
	)
	if os.Geteuid() == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("unprivileged Files permission probe: %v\n%s", err, output)
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
	handler := &FilesHandler{allowedRoots: []string{root}, maxUploadBytes: defaultMaxUploadBytes}
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
	handler := &FilesHandler{allowedRoots: []string{root}, maxUploadBytes: defaultMaxUploadBytes}
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
	handler := &FilesHandler{allowedRoots: []string{root}, maxUploadBytes: defaultMaxUploadBytes}
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
	handler := &FilesHandler{allowedRoots: []string{root}, maxUploadBytes: defaultMaxUploadBytes}
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

func TestFilesHandlerRejectsOversizedUpload(t *testing.T) {
	root := t.TempDir()
	handler := &FilesHandler{
		allowedRoots:   []string{root},
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

func TestFilesHandlerDownloadRejectsParentReplacedByOutboundSymlink(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "visible")
	outside := t.TempDir()
	writeFileFixture(t, filepath.Join(visible, "secret.txt"), "public")
	writeFileFixture(t, filepath.Join(outside, "secret.txt"), "outside")

	handler := &FilesHandler{
		allowedRoots:   []string{root},
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

func TestFilesHandlerDeleteRecursesThroughPinnedDirectoryHandles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "nested")
	writeFileFixture(t, filepath.Join(target, "one", "two", "note.txt"), "delete")
	handler := &FilesHandler{
		allowedRoots:   []string{root},
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
