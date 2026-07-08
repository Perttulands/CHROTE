package scheduled

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreWritesPrivateGroupSharedModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scheduled-tasks")
	store := NewStore(dir)
	task := &Task{
		ID:        "tsk_modes",
		Name:      "sensitive prompt",
		Prompt:    "do not leak this operator instruction",
		Target:    Target{SessionName: "ops", UnixUser: "perttu"},
		Schedule:  Schedule{Type: "interval", EveryMinutes: 15, Timezone: "UTC"},
		Enabled:   true,
		CreatedAt: time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC),
	}

	if err := store.Save(task); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat task dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o770 {
		t.Fatalf("task dir mode = %o, want 0770 permissions", got)
	}

	fileInfo, err := os.Stat(filepath.Join(dir, "tsk_modes.json"))
	if err != nil {
		t.Fatalf("stat task file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o660 {
		t.Fatalf("task file mode = %o, want 0660 permissions", got)
	}
}

func TestStoreTryLockKeepsFreshLock(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "scheduled-tasks"))
	release, claimed, err := store.TryLock("tsk_fresh")
	if err != nil {
		t.Fatalf("TryLock returned error: %v", err)
	}
	if !claimed {
		t.Fatal("first TryLock claimed = false, want true")
	}
	defer release()

	_, claimed, err = store.TryLock("tsk_fresh")
	if err != nil {
		t.Fatalf("second TryLock returned error: %v", err)
	}
	if claimed {
		t.Fatal("second TryLock claimed fresh lock, want false")
	}
}

func TestStoreTryLockReclaimsStaleLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scheduled-tasks")
	store := NewStore(dir)
	if err := os.MkdirAll(dir, 0o770); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	lockPath := filepath.Join(dir, ".tsk_stale.lock")
	if err := os.WriteFile(lockPath, []byte("999999 2026-01-01T00:00:00Z\n"), 0o660); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}
	stale := time.Now().Add(-(staleTaskLockAfter + time.Minute))
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatalf("age stale lock: %v", err)
	}

	release, claimed, err := store.TryLock("tsk_stale")
	if err != nil {
		t.Fatalf("TryLock returned error: %v", err)
	}
	if !claimed {
		t.Fatal("TryLock claimed = false for stale lock, want true")
	}
	release()
}
