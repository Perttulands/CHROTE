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

// The synthetic root lists exactly the configured roots as directories, whether
// it is reached by its own handler or by asking for the empty path through the
// mux. The browser opens both ways and must see one answer.
func TestFilesHandler_ListRoot(t *testing.T) {
	handler := NewFilesHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	for _, testCase := range []struct {
		name  string
		serve func(http.ResponseWriter, *http.Request)
	}{
		{name: "through the root handler", serve: handler.ListRoot},
		{name: "through the mux on the empty path", serve: mux.ServeHTTP},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/files/resources/", nil)
			rec := httptest.NewRecorder()

			testCase.serve(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
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

			for _, item := range response.Items {
				if !item.IsDir {
					t.Errorf("Root item %s should be a directory", item.Name)
				}
			}
		})
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

// A rename is refused whenever either end of it lies outside the configured
// roots, whether the request names the source directly or arrives through the
// mux with a destination pointing out of them.
func TestFilesHandler_RenameResourceRefusesPathsOutsideTheConfiguredRoots(t *testing.T) {
	handler := NewFilesHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	renameBody, err := json.Marshal(RenameRequest{Action: "rename", Destination: "/tmp/dest.txt"})
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		name     string
		path     string
		body     []byte
		pathName string
		serve    func(http.ResponseWriter, *http.Request)
	}{
		{
			name:     "a source that is under no root",
			path:     "/api/files/resources/etc/shadow",
			body:     []byte("{}"),
			pathName: "/etc/shadow",
			serve:    handler.RenameResource,
		},
		{
			name:  "a destination that is under no root",
			path:  "/api/files/resources/tmp/test.txt",
			body:  renameBody,
			serve: mux.ServeHTTP,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, testCase.path, bytes.NewReader(testCase.body))
			req.Header.Set("Content-Type", "application/json")
			if testCase.pathName != "" {
				req.SetPathValue("path", testCase.pathName)
			}
			rec := httptest.NewRecorder()

			testCase.serve(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
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

// The root is synthetic: it is the list of configured roots, not a directory
// anyone can write in, delete or download. Every operation that would treat it
// as one is refused.
func TestFilesHandler_RefusesOperationsOnTheSyntheticRoot(t *testing.T) {
	handler := NewFilesHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	for _, testCase := range []struct {
		name     string
		method   string
		path     string
		body     string
		pathName string
		serve    func(http.ResponseWriter, *http.Request)
	}{
		{
			name:   "creating a file at the root",
			method: http.MethodPost,
			path:   "/api/files/resources/testfile.txt",
			body:   "test",
			serve:  mux.ServeHTTP,
		},
		{
			name:     "deleting the root",
			method:   http.MethodDelete,
			path:     "/api/files/resources/",
			pathName: "",
			serve:    handler.DeleteResource,
		},
		{
			name:     "downloading the root",
			method:   http.MethodGet,
			path:     "/api/files/raw/",
			pathName: "",
			serve:    handler.DownloadFile,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			req := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			req.SetPathValue("path", testCase.pathName)
			rec := httptest.NewRecorder()

			testCase.serve(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
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
		requireUnprivileged(t)
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
		raceExitPromptly,
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
