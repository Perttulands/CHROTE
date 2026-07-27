package core

import (
	"path/filepath"
	"testing"
)

func TestFsyncDirSucceedsOnExistingDirectory(t *testing.T) {
	if err := FsyncDir(t.TempDir()); err != nil {
		t.Fatalf("FsyncDir on an existing directory: %v", err)
	}
}

func TestFsyncDirFailsLoudOnMissingDirectory(t *testing.T) {
	if err := FsyncDir(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("FsyncDir on a missing directory should error")
	}
}
