package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	// Test that invalid paths (not under allowed roots) return 403
	// Note: On Windows test env, /code doesn't exist, so the test verifies
	// that non-allowed paths are rejected
	req := httptest.NewRequest(http.MethodPatch, "/api/files/resources/code/test.txt", bytes.NewBufferString("{}"))
	req.SetPathValue("path", "code/test.txt")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.RenameResource(rec, req)

	// Path is forbidden because it's not under an allowed root on this system
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
