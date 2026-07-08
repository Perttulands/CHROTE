package formations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicCreatesSharedGroupWritableArtifacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "artifact.toml")

	if err := writeAtomic(path, []byte("schema = 1\n")); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}

	assertSharedArtifactMode(t, path)
}

func TestWithFileLockCreatesSharedGroupWritableLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locks", "artifact.toml")

	if err := withFileLock(path, func() error { return nil }); err != nil {
		t.Fatalf("withFileLock: %v", err)
	}

	assertSharedArtifactMode(t, path+".lock")
}

func TestAppendRunEventLineCreatesSharedGroupWritableLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs", "example.ndjson")

	if err := appendRunEventLine(path, RunEvent{Type: RunEventStarted}); err != nil {
		t.Fatalf("appendRunEventLine: %v", err)
	}

	assertSharedArtifactMode(t, path)
}

func assertSharedArtifactMode(t *testing.T, path string) {
	t.Helper()

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if got, want := fileInfo.Mode().Perm(), os.FileMode(0o660); got != want {
		t.Fatalf("written file mode = %04o, want %04o so service and CLI users in the shared group can both read/write", got, want)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat parent dir: %v", err)
	}
	if got, want := dirInfo.Mode().Perm(), os.FileMode(0o770); got != want {
		t.Fatalf("parent dir mode = %04o, want %04o so shared-group writers can create run artifacts", got, want)
	}
	if dirInfo.Mode()&os.ModeSetgid == 0 {
		t.Fatalf("parent dir mode = %v, want setgid so shared artifacts keep the workspace group", dirInfo.Mode())
	}
}
