package core

import (
	"os"
	"path/filepath"
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
		name        string
		inputPath   string
		expectPath  bool
		expectCode  string
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
