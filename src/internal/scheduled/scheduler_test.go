package scheduled

import (
	"context"
	"errors"
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
		Target:    Target{SessionName: "ops", UnixUser: "perttu"},
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
		Target:    Target{SessionName: "ops", UnixUser: "perttu"},
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
		Target:    Target{SessionName: "ops", UnixUser: "perttu"},
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

type schedulerTestRunner struct {
	sent   []schedulerTestSend
	onSend func()
}

type schedulerTestSend struct {
	target Target
	prompt string
}

func (r *schedulerTestRunner) ValidateTarget(context.Context, Target) error {
	return nil
}

func (r *schedulerTestRunner) SendPrompt(_ context.Context, target Target, prompt string) error {
	r.sent = append(r.sent, schedulerTestSend{target: target, prompt: prompt})
	if r.onSend != nil {
		r.onSend()
	}
	return nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}
