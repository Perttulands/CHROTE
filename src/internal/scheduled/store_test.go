package scheduled

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		Targets:   []Target{{SessionName: "ops", UnixUser: "alice"}},
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

// A task lock keeps a second runner off a task that is already firing, and it
// expires, because a runner killed mid-run would otherwise block its task for
// good and the operator would see a schedule that silently stopped.
func TestStoreTryLockHoldsAFreshLockAndReclaimsAStaleOne(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scheduled-tasks")
	store := NewStore(dir)

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

	staleRelease, claimed, err := store.TryLock("tsk_stale")
	if err != nil {
		t.Fatalf("TryLock returned error: %v", err)
	}
	if !claimed {
		t.Fatal("TryLock claimed = false for stale lock, want true")
	}
	staleRelease()
}

// The persisted shapes below are what survives a restart, so they are pinned
// here beside the writer rather than beside the scheduler that happens to
// produce them. A document written by an older CHROTE has to keep loading, and
// one written today has to carry the current schema and nothing beside it.
func TestTargetRunJSONMigratesLegacySubmissionClaim(t *testing.T) {
	var target TargetRun
	if err := json.Unmarshal([]byte(`{
		"sessionName":"worker-1",
		"status":"success",
		"pane":"%1",
		"submitted":true,
		"message":"pasted and submitted"
	}`), &target); err != nil {
		t.Fatalf("unmarshal legacy target run: %v", err)
	}
	if !target.SubmitKeyDispatched {
		t.Fatalf("legacy target run = %+v, want truthful submit-key receipt", target)
	}
	if target.Message != SubmitKeyDispatchedDetail {
		t.Fatalf("legacy target message = %q, want %q", target.Message, SubmitKeyDispatchedDetail)
	}

	raw, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("marshal migrated target run: %v", err)
	}
	if strings.Contains(string(raw), `"submitted"`) || strings.Contains(string(raw), `pasted and submitted`) {
		t.Fatalf("migrated target run still exposes legacy false ACK: %s", raw)
	}
	if !strings.Contains(string(raw), `"submitKeyDispatched":true`) {
		t.Fatalf("migrated target run JSON = %s, want submit-key receipt", raw)
	}
}

func TestStoreReadsLegacySingleTargetDocument(t *testing.T) {
	dir := t.TempDir()
	legacy := `{
  "id": "tsk_legacy",
  "name": "legacy task",
  "prompt": "still works",
  "target": {"sessionName": "ops", "unixUser": "alice"},
  "schedule": {"type": "interval", "everyMinutes": 10, "timezone": "UTC"},
  "enabled": true,
  "createdAt": "2026-06-27T13:00:00Z",
  "updatedAt": "2026-06-27T13:00:00Z"
}`
	if err := os.WriteFile(filepath.Join(dir, "tsk_legacy.json"), []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy task: %v", err)
	}

	loaded, err := NewStore(dir).Get("tsk_legacy")
	if err != nil {
		t.Fatalf("get legacy task: %v", err)
	}
	if len(loaded.Targets) != 1 || loaded.Targets[0].SessionName != "ops" || loaded.Targets[0].UnixUser != "alice" {
		t.Fatalf("legacy targets = %+v, want the single documented target migrated", loaded.Targets)
	}
}

func TestTaskJSONWritesCurrentTargetsSchema(t *testing.T) {
	raw, err := json.Marshal(Task{
		ID:      "tsk_mirror",
		Targets: []Target{{SessionName: "worker-1"}, {SessionName: "worker-2"}},
	})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	wantKeys := map[string]bool{
		"id": true, "name": true, "prompt": true, "targets": true,
		"schedule": true, "enabled": true, "paused": true,
		"createdAt": true, "updatedAt": true,
	}
	if len(document) != len(wantKeys) {
		t.Fatalf("task schema keys = %v, want current schema keys %v", document, wantKeys)
	}
	for key := range wantKeys {
		if _, ok := document[key]; !ok {
			t.Fatalf("task schema missing current field %q: %s", key, raw)
		}
	}
	var targets []Target
	if err := json.Unmarshal(document["targets"], &targets); err != nil {
		t.Fatalf("decode task targets: %v", err)
	}
	if len(targets) != 2 || targets[0].SessionName != "worker-1" || targets[1].SessionName != "worker-2" {
		t.Fatalf("targets = %+v, want both current targets in order", targets)
	}
}
