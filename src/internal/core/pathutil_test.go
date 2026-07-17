package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestValidateProjectPath(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "pathutil_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Reset config and set test roots via env var
	defer func() {
		os.Unsetenv("CHROTE_ROOTS")
		ResetConfigForTesting()
	}()
	os.Setenv("CHROTE_ROOTS", tempDir)
	ResetConfigForTesting()

	// Create a subdirectory
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	tests := []struct {
		name       string
		inputPath  string
		expectPath bool
		expectCode string
	}{
		{"empty path", "", false, "BAD_REQUEST"},
		{"valid root", tempDir, true, ""},
		{"valid subdir", subDir, true, ""},
		{"nonexistent path", filepath.Join(tempDir, "nonexistent"), false, "NOT_FOUND"},
		{"forbidden path", "/etc/passwd", false, "FORBIDDEN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, code, msg := ValidateProjectPath(tt.inputPath)

			if tt.expectPath && resolved == "" {
				t.Errorf("Expected valid path, got empty string. Code: %s, Msg: %s", code, msg)
			}
			if !tt.expectPath && code != tt.expectCode {
				t.Errorf("Expected error code %q, got %q. Msg: %s", tt.expectCode, code, msg)
			}
			if tt.expectPath && code != "" {
				t.Errorf("Expected no error, got code %q, msg: %s", code, msg)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	// Create a temp file
	tempFile, err := os.CreateTemp("", "fileexists_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	if !FileExists(tempPath) {
		t.Errorf("FileExists(%q) = false, expected true", tempPath)
	}

	if FileExists("/nonexistent/path/file.txt") {
		t.Error("FileExists for nonexistent file = true, expected false")
	}
}

func TestValidateProjectPath_ClearsErrorsCorrectly(t *testing.T) {
	// Test that successful validation returns empty error fields
	tempDir, err := os.MkdirTemp("", "pathutil_test2")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	defer func() {
		os.Unsetenv("CHROTE_ROOTS")
		ResetConfigForTesting()
	}()
	os.Setenv("CHROTE_ROOTS", tempDir)
	ResetConfigForTesting()

	resolved, code, msg := ValidateProjectPath(tempDir)

	if resolved == "" {
		t.Error("Expected resolved path, got empty")
	}
	if code != "" {
		t.Errorf("Expected empty code, got %q", code)
	}
	if msg != "" {
		t.Errorf("Expected empty msg, got %q", msg)
	}
}

func TestGetAllowedRoots_NormalizesEnvRoots(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatalf("create workspace root: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(parent); err != nil {
		t.Fatalf("chdir to temp parent: %v", err)
	}

	defer func() {
		_ = os.Chdir(cwd)
		os.Unsetenv("CHROTE_ROOTS")
		ResetConfigForTesting()
	}()
	os.Setenv("CHROTE_ROOTS", " , workspace/. , "+root+" , ")
	ResetConfigForTesting()

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	want := []string{filepath.Clean(absRoot)}
	if got := GetAllowedRoots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAllowedRoots() = %#v, want %#v", got, want)
	}
}

func TestValidateProjectPath_RootAllowedRootCoversFilesystemChildren(t *testing.T) {
	defer func() {
		os.Unsetenv("CHROTE_ROOTS")
		ResetConfigForTesting()
	}()
	os.Setenv("CHROTE_ROOTS", "/")
	ResetConfigForTesting()

	for _, path := range []string{"/home", "/srv"} {
		t.Run(path, func(t *testing.T) {
			_, code, msg := ValidateProjectPath(path)
			if code == "FORBIDDEN" {
				t.Fatalf("ValidateProjectPath(%q) code = FORBIDDEN, want allowed by CHROTE_ROOTS=/ before existence check; msg: %s", path, msg)
			}
		})
	}
}

func TestGetAllowedRoots_RootDominatesOtherRoots(t *testing.T) {
	defer func() {
		os.Unsetenv("CHROTE_ROOTS")
		ResetConfigForTesting()
	}()
	os.Setenv("CHROTE_ROOTS", "/, /srv, /home/operator")
	ResetConfigForTesting()

	got := GetAllowedRoots()
	want := []string{string(os.PathSeparator)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAllowedRoots() = %#v, want %#v", got, want)
	}
}

func TestIsPathUnderRootRequiresAbsolutePath(t *testing.T) {
	if IsPathUnderRoot("relative/path", string(os.PathSeparator)) {
		t.Fatal("relative path reported under filesystem root; want false")
	}
}
