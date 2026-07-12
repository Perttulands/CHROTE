package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempRootFilesHandler(t *testing.T) (*FilesHandler, string) {
	t.Helper()

	root := filepath.ToSlash(t.TempDir())
	return &FilesHandler{
		allowedRoots:   []string{root},
		writeRoots:     []string{root},
		deniedRoots:    defaultDeniedFileRoots(),
		maxUploadBytes: defaultMaxUploadBytes,
	}, root
}

func filePathValue(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(path), "/")
}

func TestFilesHandler_RootAllowedRootListsActualFilesystemRoot(t *testing.T) {
	handler := &FilesHandler{allowedRoots: []string{"/"}}
	req := httptest.NewRequest(http.MethodGet, "/api/files/resources/", nil)
	rec := httptest.NewRecorder()

	handler.ListRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListRoot status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response DirectoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode root listing: %v", err)
	}
	if !response.IsDir {
		t.Fatal("root listing should be a directory")
	}
	if len(response.Items) == 1 && response.Items[0].Name == "" {
		t.Fatalf("root listing returned blank virtual-root item; want actual / contents")
	}
	if _, err := os.Stat("/home"); err == nil {
		foundHome := false
		for _, item := range response.Items {
			if item.Name == "home" && item.IsDir {
				foundHome = true
				break
			}
		}
		if !foundHome {
			t.Fatalf("root listing did not include /home directory; items = %+v", response.Items)
		}
	}
}

func TestFilesHandler_ResolveSafePath_RootAllowedRootCoversFilesystemChildren(t *testing.T) {
	handler := &FilesHandler{allowedRoots: []string{"/"}}

	for _, path := range []string{"/home", "/srv"} {
		t.Run(path, func(t *testing.T) {
			result := handler.resolveSafePath(path)
			if result.Error != "" || result.IsRoot {
				t.Fatalf("resolveSafePath(%q) = %+v, want allowed filesystem child", path, result)
			}
			if result.Path != filepath.Clean(path) {
				t.Fatalf("resolved path = %q, want %q", result.Path, filepath.Clean(path))
			}
		})
	}
}

func TestFilesHandler_ResolveSafePath_NormalizesBackslashSeparators(t *testing.T) {
	handler := &FilesHandler{allowedRoots: []string{"/"}}

	result := handler.resolveSafePath(`/home\perttu`)
	if result.Error != "" || result.IsRoot {
		t.Fatalf("resolveSafePath with backslashes = %+v, want allowed absolute path", result)
	}
	if result.Path != "/home/perttu" {
		t.Fatalf("resolved path = %q, want /home/perttu", result.Path)
	}
}

func TestFilesHandler_TempRootResourceRoundTrip(t *testing.T) {
	handler, root := tempRootFilesHandler(t)
	filePath := filepath.Join(root, "nested", "note.txt")
	filePathKey := filePathValue(filePath)

	createReq := httptest.NewRequest(http.MethodPost, "/api/files/resources/"+filePathKey, bytes.NewBufferString("hello chrote"))
	createReq.SetPathValue("path", filePathKey)
	createRec := httptest.NewRecorder()
	handler.CreateResource(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateResource status = %d, want %d: %s", createRec.Code, http.StatusOK, createRec.Body.String())
	}

	if got, err := os.ReadFile(filePath); err != nil || string(got) != "hello chrote" {
		t.Fatalf("created file content = %q, err = %v; want hello chrote", got, err)
	}

	dirPath := filepath.Join(root, "nested")
	dirPathValue := filePathValue(dirPath)
	listReq := httptest.NewRequest(http.MethodGet, "/api/files/resources/"+dirPathValue, nil)
	listReq.SetPathValue("path", dirPathValue)
	listRec := httptest.NewRecorder()
	handler.GetResource(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GetResource directory status = %d, want %d: %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listing DirectoryResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode directory listing: %v", err)
	}
	if !listing.IsDir {
		t.Fatal("expected directory listing")
	}
	if len(listing.Items) != 1 || listing.Items[0].Name != "note.txt" || listing.Items[0].IsDir {
		t.Fatalf("directory items = %+v, want one file named note.txt", listing.Items)
	}

	infoReq := httptest.NewRequest(http.MethodGet, "/api/files/resources/"+filePathKey, nil)
	infoReq.SetPathValue("path", filePathKey)
	infoRec := httptest.NewRecorder()
	handler.GetResource(infoRec, infoReq)
	if infoRec.Code != http.StatusOK {
		t.Fatalf("GetResource file status = %d, want %d: %s", infoRec.Code, http.StatusOK, infoRec.Body.String())
	}

	var info FileInfoResponse
	if err := json.Unmarshal(infoRec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode file info: %v", err)
	}
	if info.IsDir || info.Name != "note.txt" || info.Size != int64(len("hello chrote")) {
		t.Fatalf("file info = %+v, want note.txt size %d", info, len("hello chrote"))
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/files/raw/"+filePathKey, nil)
	downloadReq.SetPathValue("path", filePathKey)
	downloadRec := httptest.NewRecorder()
	handler.DownloadFile(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("DownloadFile status = %d, want %d: %s", downloadRec.Code, http.StatusOK, downloadRec.Body.String())
	}
	if got := downloadRec.Body.String(); got != "hello chrote" {
		t.Fatalf("DownloadFile body = %q, want hello chrote", got)
	}

	renamedPath := filepath.Join(root, "nested", "renamed.txt")
	renameBody, err := json.Marshal(RenameRequest{
		Action:      "rename",
		Destination: filepath.ToSlash(renamedPath),
	})
	if err != nil {
		t.Fatalf("marshal rename body: %v", err)
	}
	renameReq := httptest.NewRequest(http.MethodPatch, "/api/files/resources/"+filePathKey, bytes.NewReader(renameBody))
	renameReq.SetPathValue("path", filePathKey)
	renameRec := httptest.NewRecorder()
	handler.RenameResource(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("RenameResource status = %d, want %d: %s", renameRec.Code, http.StatusOK, renameRec.Body.String())
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("original path still exists after rename, err = %v", err)
	}
	if got, err := os.ReadFile(renamedPath); err != nil || string(got) != "hello chrote" {
		t.Fatalf("renamed file content = %q, err = %v; want hello chrote", got, err)
	}

	renamedPathValue := filePathValue(renamedPath)
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/files/resources/"+renamedPathValue, nil)
	deleteReq.SetPathValue("path", renamedPathValue)
	deleteRec := httptest.NewRecorder()
	handler.DeleteResource(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("DeleteResource status = %d, want %d: %s", deleteRec.Code, http.StatusOK, deleteRec.Body.String())
	}
	if _, err := os.Stat(renamedPath); !os.IsNotExist(err) {
		t.Fatalf("renamed path still exists after delete, err = %v", err)
	}
}

func TestFilesHandler_ResolveSafePath_RejectsCurrentTraversalAndPrefixSiblingCases(t *testing.T) {
	handler, root := tempRootFilesHandler(t)

	tests := []struct {
		name string
		path string
	}{
		{name: "parent traversal", path: filepath.Join(root, "..", "outside.txt")},
		{name: "prefix sibling", path: root + "-sibling/secret.txt"},
		{name: "unconfigured absolute path", path: "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.resolveSafePath(filepath.ToSlash(tt.path))
			if result.Error == "" || result.IsRoot {
				t.Fatalf("resolveSafePath(%q) = %+v, want rejected non-root path", tt.path, result)
			}
		})
	}
}

func TestFilesHandler_ResolveSafePath_AllowsInRootSymlinkToInRootTarget(t *testing.T) {
	handler, root := tempRootFilesHandler(t)

	targetDir := filepath.Join(root, "target")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	targetFile := filepath.Join(targetDir, "safe.txt")
	if err := os.WriteFile(targetFile, []byte("safe"), 0600); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	linkPath := filepath.Join(root, "link.txt")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	result := handler.resolveSafePath(filepath.ToSlash(linkPath))
	if result.Error != "" || result.IsRoot {
		t.Fatalf("resolveSafePath(%q) = %+v, want allowed file path", linkPath, result)
	}
	if got, err := os.ReadFile(result.Path); err != nil || string(got) != "safe" {
		t.Fatalf("symlink read content = %q, err = %v; want safe", got, err)
	}
}

func TestFilesHandler_RejectsOutboundSymlinkToOutOfRootTarget(t *testing.T) {
	handler, root := tempRootFilesHandler(t)

	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside target"), 0600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}

	linkPath := filepath.Join(root, "outbound-link.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Fatalf("create outbound symlink: %v", err)
	}

	linkPathValue := filePathValue(linkPath)
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw/"+linkPathValue, nil)
	req.SetPathValue("path", linkPathValue)
	rec := httptest.NewRecorder()

	handler.DownloadFile(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("DownloadFile outbound symlink status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}
