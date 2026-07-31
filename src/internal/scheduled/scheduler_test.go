package scheduled

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestFireTaskBoundsConcurrentFanOutAndPreservesTargetOrder(t *testing.T) {
	runner := &blockingScheduledRunner{
		entered: make(chan Target, 3),
		release: make(chan struct{}),
	}
	service := NewService(NewStore(t.TempDir()), runner, ServiceOptions{MaxConcurrentDeliveries: 2})
	task := &Task{
		Prompt: "continue",
		Targets: []Target{
			{SessionName: "worker-1"},
			{SessionName: "worker-2"},
			{SessionName: "worker-3"},
		},
	}
	done := make(chan RunEntry, 1)
	go func() { done <- service.fireTask(context.Background(), task, "manual") }()

	for range 2 {
		select {
		case <-runner.entered:
		case <-time.After(time.Second):
			t.Fatal("fan-out did not start two deliveries concurrently")
		}
	}
	select {
	case target := <-runner.entered:
		t.Fatalf("fan-out exceeded the configured concurrency before release: %+v", target)
	case <-time.After(50 * time.Millisecond):
	}
	close(runner.release)

	var run RunEntry
	select {
	case run = <-done:
	case <-time.After(time.Second):
		t.Fatal("fan-out did not finish after releasing deliveries")
	}
	if len(run.Targets) != 3 {
		t.Fatalf("target results = %+v, want all three", run.Targets)
	}
	for index, want := range []string{"worker-1", "worker-2", "worker-3"} {
		if run.Targets[index].SessionName != want {
			t.Fatalf("target result order = %+v, want task order", run.Targets)
		}
	}
}

func TestCreateBoundsConcurrentTargetValidation(t *testing.T) {
	runner := &blockingValidationRunner{
		entered: make(chan Target, 16),
		release: make(chan struct{}),
	}
	service := NewService(NewStore(t.TempDir()), runner, ServiceOptions{
		ValidateTargets:         true,
		TargetValidationTimeout: time.Second,
		MaxConcurrentDeliveries: 4,
	})
	targets := make([]Target, 16)
	for index := range targets {
		targets[index] = Target{SessionName: fmt.Sprintf("worker-%02d", index)}
	}

	done := make(chan error, 1)
	go func() {
		_, err := service.Create(context.Background(), CreateTaskRequest{
			Name:     "bounded validation",
			Prompt:   "inspect",
			Targets:  targets,
			Schedule: Schedule{Type: "interval", EveryMinutes: 60, Timezone: "UTC"},
		})
		done <- err
	}()

	observed := make([]Target, 0, 4)
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	for len(observed) < 4 {
		select {
		case target := <-runner.entered:
			observed = append(observed, target)
		case <-timer.C:
			close(runner.release)
			<-done
			t.Fatalf("validation started %d targets, want concurrency 4", len(observed))
		}
	}
	select {
	case target := <-runner.entered:
		close(runner.release)
		<-done
		t.Fatalf("validation exceeded concurrency bound with fifth target %+v", target)
	case <-time.After(50 * time.Millisecond):
	}
	close(runner.release)
	if err := <-done; err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if runner.maxConcurrency() != 4 {
		t.Fatalf("max validation concurrency = %d, want 4", runner.maxConcurrency())
	}
}

func TestDeliveryRunTimeoutIncludesValidation(t *testing.T) {
	runner := &slowValidationRunner{validationDelay: 30 * time.Millisecond}
	service := NewService(NewStore(t.TempDir()), runner, ServiceOptions{
		ValidateTargets:         true,
		RunTimeout:              10 * time.Millisecond,
		TargetValidationTimeout: time.Second,
	})
	task := &Task{
		ID:      "tsk_delivery_budget",
		Prompt:  "stay inside one delivery budget",
		Targets: []Target{{SessionName: "worker-1"}},
	}

	run := service.fireTask(context.Background(), task, "manual")
	if runner.sendCalled {
		t.Fatal("SendPrompt received a fresh timeout after validation exhausted the delivery budget")
	}
	if run.Status != RunStatusError || len(run.Targets) != 1 || !strings.Contains(run.Targets[0].Message, context.DeadlineExceeded.Error()) {
		t.Fatalf("run = %+v, want a deadline error without attempting send", run)
	}
}

func TestTargetRunJSONReportsSubmitKeyDispatchWithoutSubmittedClaim(t *testing.T) {
	raw, err := json.Marshal(TargetRun{
		SessionName:         "worker-1",
		Status:              RunStatusSuccess,
		SubmitKeyDispatched: true,
	})
	if err != nil {
		t.Fatalf("marshal target run: %v", err)
	}
	if !strings.Contains(string(raw), `"submitKeyDispatched":true`) {
		t.Fatalf("target run JSON = %s, want submitKeyDispatched receipt", raw)
	}
	if strings.Contains(string(raw), `"submitted"`) {
		t.Fatalf("target run JSON = %s, must not claim application submission", raw)
	}
}

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

type blockingValidationRunner struct {
	entered chan Target
	release chan struct{}
	mu      sync.Mutex
	active  int
	max     int
}

func (r *blockingValidationRunner) ValidateTarget(_ context.Context, target Target) error {
	r.mu.Lock()
	r.active++
	if r.active > r.max {
		r.max = r.active
	}
	r.mu.Unlock()
	r.entered <- target
	<-r.release
	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	return nil
}

func (r *blockingValidationRunner) SendPrompt(context.Context, Target, string) (Delivery, error) {
	return Delivery{}, errors.New("SendPrompt should not run during task validation")
}

func (r *blockingValidationRunner) maxConcurrency() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.max
}

type slowValidationRunner struct {
	validationDelay time.Duration
	sendCalled      bool
}

func (r *slowValidationRunner) ValidateTarget(context.Context, Target) error {
	time.Sleep(r.validationDelay)
	return nil
}

func (r *slowValidationRunner) SendPrompt(context.Context, Target, string) (Delivery, error) {
	r.sendCalled = true
	return Delivery{}, errors.New("send should not receive a fresh timeout")
}

type blockingScheduledRunner struct {
	entered chan Target
	release chan struct{}
}

func (r *blockingScheduledRunner) ValidateTarget(context.Context, Target) error { return nil }

func (r *blockingScheduledRunner) SendPrompt(ctx context.Context, target Target, _ string) (Delivery, error) {
	select {
	case r.entered <- target:
	case <-ctx.Done():
		return Delivery{}, ctx.Err()
	}
	select {
	case <-r.release:
		return Delivery{Pane: "%1", SubmitKeyDispatched: true, Detail: "submit key dispatched"}, nil
	case <-ctx.Done():
		return Delivery{}, ctx.Err()
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
	return Delivery{Pane: "%1", SubmitKeyDispatched: true, Detail: "submit key dispatched"}, nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}
