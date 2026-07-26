package scheduled

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSchedulerTickReloadsTasksComputesNextRunAndFiresDueTasks(t *testing.T) {
	fixedNow := time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)
	now := fixedNow
	store := NewStore(t.TempDir())
	runner := &schedulerTestRunner{}
	service := NewService(store, runner, ServiceOptions{Now: func() time.Time { return now }})
	scheduler := NewScheduler(service, time.Hour)

	manualTask := Task{
		ID:        "tsk_manual",
		Name:      "manual persisted task",
		Prompt:    "hello from scheduler",
		Targets:   []Target{{SessionName: "ops", UnixUser: "alice"}},
		Schedule:  Schedule{Type: "interval", EveryMinutes: 10, Timezone: "UTC"},
		Enabled:   true,
		CreatedAt: fixedNow.Add(-time.Hour),
		UpdatedAt: fixedNow.Add(-time.Hour),
	}
	if err := store.Save(&manualTask); err != nil {
		t.Fatalf("save manual task: %v", err)
	}

	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("initial Tick returned error: %v", err)
	}
	loaded, err := store.Get(manualTask.ID)
	if err != nil {
		t.Fatalf("get loaded task: %v", err)
	}
	wantNext := fixedNow.Add(10 * time.Minute)
	if loaded.NextRun == nil || !loaded.NextRun.Equal(wantNext) {
		t.Fatalf("nextRun after reload = %v, want %s", loaded.NextRun, wantNext.Format(time.RFC3339))
	}
	if len(runner.sent) != 0 {
		t.Fatalf("runner sent on future task = %+v, want none", runner.sent)
	}

	now = fixedNow.Add(5 * time.Minute)
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("early Tick returned error: %v", err)
	}
	loaded, err = store.Get(manualTask.ID)
	if err != nil {
		t.Fatalf("get early task: %v", err)
	}
	if loaded.NextRun == nil || !loaded.NextRun.Equal(wantNext) {
		t.Fatalf("nextRun after early tick drifted to %v, want it unchanged at %s", loaded.NextRun, wantNext.Format(time.RFC3339))
	}
	if len(runner.sent) != 0 {
		t.Fatalf("runner sent before due time = %+v, want none", runner.sent)
	}

	now = wantNext
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("due Tick returned error: %v", err)
	}
	loaded, err = store.Get(manualTask.ID)
	if err != nil {
		t.Fatalf("get fired task: %v", err)
	}
	if len(runner.sent) != 1 || runner.sent[0].prompt != "hello from scheduler" || runner.sent[0].target.SessionName != "ops" {
		t.Fatalf("runner sent = %+v, want due task prompt", runner.sent)
	}
	if loaded.LastStatus != RunStatusSuccess || loaded.LastRun == nil || len(loaded.RecentRuns) != 1 {
		t.Fatalf("fired task = %+v, want successful run persisted", loaded)
	}
	wantAfterFire := wantNext.Add(10 * time.Minute)
	if loaded.NextRun == nil || !loaded.NextRun.Equal(wantAfterFire) {
		t.Fatalf("nextRun after fire = %v, want %s", loaded.NextRun, wantAfterFire.Format(time.RFC3339))
	}
}

func TestSchedulerDoesNotResurrectTaskDeletedDuringFire(t *testing.T) {
	fixedNow := time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir())
	runner := &schedulerTestRunner{}
	service := NewService(store, runner, ServiceOptions{Now: func() time.Time { return fixedNow }})
	scheduler := NewScheduler(service, time.Hour)

	dueTask := Task{
		ID:        "tsk_deleted_during_fire",
		Name:      "deleted during fire",
		Prompt:    "do not resurrect",
		Targets:   []Target{{SessionName: "ops", UnixUser: "alice"}},
		Schedule:  Schedule{Type: "interval", EveryMinutes: 10, Timezone: "UTC"},
		Enabled:   true,
		NextRun:   timePtr(fixedNow.Add(-time.Minute)),
		CreatedAt: fixedNow.Add(-time.Hour),
		UpdatedAt: fixedNow.Add(-time.Hour),
	}
	if err := store.Save(&dueTask); err != nil {
		t.Fatalf("save due task: %v", err)
	}
	runner.onSend = func() {
		if err := store.Delete(dueTask.ID); err != nil {
			t.Fatalf("delete task during send: %v", err)
		}
	}

	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if _, err := store.Get(dueTask.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted task lookup err = %v, want ErrNotFound and no resurrection", err)
	}
}

func TestSchedulerSkipsDueTaskAlreadyClaimedByAnotherRunner(t *testing.T) {
	fixedNow := time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir())
	runner := &schedulerTestRunner{}
	service := NewService(store, runner, ServiceOptions{Now: func() time.Time { return fixedNow }})
	scheduler := NewScheduler(service, time.Hour)

	dueTask := Task{
		ID:        "tsk_claimed_elsewhere",
		Name:      "claimed elsewhere",
		Prompt:    "must send once only",
		Targets:   []Target{{SessionName: "ops", UnixUser: "alice"}},
		Schedule:  Schedule{Type: "interval", EveryMinutes: 10, Timezone: "UTC"},
		Enabled:   true,
		NextRun:   timePtr(fixedNow.Add(-time.Minute)),
		CreatedAt: fixedNow.Add(-time.Hour),
		UpdatedAt: fixedNow.Add(-time.Hour),
	}
	if err := store.Save(&dueTask); err != nil {
		t.Fatalf("save due task: %v", err)
	}
	release, claimed, err := store.TryLock(dueTask.ID)
	if err != nil || !claimed {
		t.Fatalf("TryLock claimed=%v err=%v, want claimed lock", claimed, err)
	}
	defer release()

	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if len(runner.sent) != 0 {
		t.Fatalf("runner sent while task was locked = %+v, want no duplicate send", runner.sent)
	}
}

func TestFireTaskFansOutToEveryTargetAndSurvivesOneDeadSession(t *testing.T) {
	fixedNow := time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir())
	runner := &schedulerTestRunner{sendErr: func(target Target) error {
		if target.SessionName == "worker-2" {
			return errors.New("session is gone")
		}
		return nil
	}}
	service := NewService(store, runner, ServiceOptions{Now: func() time.Time { return fixedNow }})

	task := Task{
		ID:     "tsk_fan_out",
		Name:   "continue work",
		Prompt: "Continue if work is clear",
		Targets: []Target{
			{SessionName: "worker-1", UnixUser: "alice"},
			{SessionName: "worker-2", UnixUser: "alice"},
			{SessionName: "worker-3", UnixUser: "alice"},
		},
		Schedule:  Schedule{Type: "interval", EveryMinutes: 10, Timezone: "UTC"},
		Enabled:   true,
		NextRun:   timePtr(fixedNow.Add(-time.Minute)),
		CreatedAt: fixedNow.Add(-time.Hour),
		UpdatedAt: fixedNow.Add(-time.Hour),
	}
	if err := store.Save(&task); err != nil {
		t.Fatalf("save task: %v", err)
	}

	if _, err := service.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue returned error: %v", err)
	}

	if len(runner.sent) != 3 {
		t.Fatalf("sent to %d targets, want all 3 attempted despite the dead one: %+v", len(runner.sent), runner.sent)
	}
	loaded, err := store.Get(task.ID)
	if err != nil {
		t.Fatalf("get fired task: %v", err)
	}
	if loaded.LastStatus != RunStatusPartial {
		t.Fatalf("lastStatus = %q, want %q", loaded.LastStatus, RunStatusPartial)
	}
	if len(loaded.RecentRuns) != 1 || len(loaded.RecentRuns[0].Targets) != 3 {
		t.Fatalf("recent run target results = %+v, want one run recording all 3 targets", loaded.RecentRuns)
	}
	statuses := map[string]string{}
	for _, result := range loaded.RecentRuns[0].Targets {
		statuses[result.SessionName] = result.Status
	}
	if statuses["worker-1"] != RunStatusSuccess || statuses["worker-3"] != RunStatusSuccess {
		t.Fatalf("healthy targets = %+v, want success for worker-1 and worker-3", statuses)
	}
	if statuses["worker-2"] != RunStatusError {
		t.Fatalf("dead target status = %q, want %q", statuses["worker-2"], RunStatusError)
	}
	if !strings.Contains(loaded.RecentRuns[0].Message, "worker-2") {
		t.Fatalf("run message = %q, want it to name the failing target", loaded.RecentRuns[0].Message)
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

func TestTaskJSONMirrorsFirstTargetForOlderReaders(t *testing.T) {
	raw, err := json.Marshal(Task{
		ID:      "tsk_mirror",
		Targets: []Target{{SessionName: "worker-1"}, {SessionName: "worker-2"}},
	})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	var document struct {
		Target  *Target  `json:"target"`
		Targets []Target `json:"targets"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	if document.Target == nil || document.Target.SessionName != "worker-1" {
		t.Fatalf("legacy mirror = %+v, want the first target", document.Target)
	}
	if len(document.Targets) != 2 {
		t.Fatalf("targets = %+v, want both persisted", document.Targets)
	}
}

type schedulerTestRunner struct {
	sent    []schedulerTestSend
	onSend  func()
	sendErr func(Target) error
}

type schedulerTestSend struct {
	target Target
	prompt string
}

func (r *schedulerTestRunner) ValidateTarget(context.Context, Target) error {
	return nil
}

func (r *schedulerTestRunner) SendPrompt(_ context.Context, target Target, prompt string) (Delivery, error) {
	r.sent = append(r.sent, schedulerTestSend{target: target, prompt: prompt})
	if r.onSend != nil {
		r.onSend()
	}
	if r.sendErr != nil {
		if err := r.sendErr(target); err != nil {
			return Delivery{}, err
		}
	}
	return Delivery{Pane: "%1", Submitted: true, Detail: "pasted and submitted"}, nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}
