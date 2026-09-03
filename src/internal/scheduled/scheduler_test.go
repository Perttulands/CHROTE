package scheduled

import (
	"context"
	"errors"
	"fmt"
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

func TestConcurrentRunNowCannotDispatchTwiceOrLoseReceipt(t *testing.T) {
	service, runner, taskID := newBlockedMutationService(t)
	releaseOnce := sync.Once{}
	release := func() { releaseOnce.Do(func() { close(runner.release) }) }
	defer release()

	firstDone := make(chan error, 1)
	go func() {
		_, _, err := service.RunNow(context.Background(), taskID, "first")
		firstDone <- err
	}()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("first RunNow did not enter delivery")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, _, err := service.RunNow(context.Background(), taskID, "second")
		secondDone <- err
	}()
	var secondErr error
	duplicateSend := false
	select {
	case secondErr = <-secondDone:
	case <-runner.entered:
		duplicateSend = true
	case <-time.After(time.Second):
		t.Fatal("second RunNow neither failed with a conflict nor entered delivery")
	}
	release()
	if err := <-firstDone; err != nil {
		t.Fatalf("first RunNow returned error: %v", err)
	}
	if duplicateSend {
		if err := <-secondDone; err != nil {
			t.Logf("duplicate RunNow later returned %v", err)
		}
		t.Fatal("two concurrent RunNow calls both dispatched the prompt")
	}
	if secondErr == nil || !strings.Contains(secondErr.Error(), "already being modified") {
		t.Fatalf("second RunNow error = %v, want a fail-loud task mutation conflict", secondErr)
	}
	loaded, err := service.Get(taskID)
	if err != nil {
		t.Fatalf("Get after RunNow: %v", err)
	}
	if len(loaded.RecentRuns) != 1 || runner.sendCount() != 1 {
		t.Fatalf("persisted runs/sends = %d/%d, want exactly one of each", len(loaded.RecentRuns), runner.sendCount())
	}
}

func TestRunNowBlocksConcurrentTaskMutation(t *testing.T) {
	mutations := []struct {
		name string
		run  func(*Service, string) error
	}{
		{name: "patch", run: func(service *Service, id string) error {
			name := "stale patch"
			_, err := service.Patch(context.Background(), id, PatchTaskRequest{Name: &name})
			return err
		}},
		{name: "pause", run: func(service *Service, id string) error {
			_, err := service.Pause(id, "pauser")
			return err
		}},
		{name: "resume", run: func(service *Service, id string) error {
			_, err := service.Resume(id, "resumer")
			return err
		}},
		{name: "delete", run: func(service *Service, id string) error { return service.Delete(id) }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			service, runner, taskID := newBlockedMutationService(t)
			releaseOnce := sync.Once{}
			release := func() { releaseOnce.Do(func() { close(runner.release) }) }
			defer release()
			runDone := make(chan error, 1)
			go func() {
				_, _, err := service.RunNow(context.Background(), taskID, "runner")
				runDone <- err
			}()
			select {
			case <-runner.entered:
			case <-time.After(time.Second):
				t.Fatal("RunNow did not enter delivery")
			}
			mutationErr := mutation.run(service, taskID)
			release()
			if err := <-runDone; err != nil {
				t.Fatalf("RunNow returned error: %v", err)
			}
			if mutationErr == nil || !strings.Contains(mutationErr.Error(), "already being modified") {
				t.Fatalf("concurrent %s error = %v, want a fail-loud task mutation conflict", mutation.name, mutationErr)
			}
			loaded, err := service.Get(taskID)
			if err != nil {
				t.Fatalf("task was lost after rejected %s: %v", mutation.name, err)
			}
			if len(loaded.RecentRuns) != 1 || runner.sendCount() != 1 {
				t.Fatalf("after %s, persisted runs/sends = %d/%d, want one", mutation.name, len(loaded.RecentRuns), runner.sendCount())
			}
		})
	}
}

func TestManualRunClaimPreventsConcurrentScheduledDispatch(t *testing.T) {
	service, runner, taskID := newBlockedMutationService(t)
	releaseOnce := sync.Once{}
	release := func() { releaseOnce.Do(func() { close(runner.release) }) }
	defer release()
	manualDone := make(chan error, 1)
	go func() {
		_, _, err := service.RunNow(context.Background(), taskID, "manual")
		manualDone <- err
	}()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("manual run did not enter delivery")
	}

	tickDone := make(chan struct {
		runs []RunEntry
		err  error
	}, 1)
	go func() {
		runs, err := service.RunDue(context.Background())
		tickDone <- struct {
			runs []RunEntry
			err  error
		}{runs: runs, err: err}
	}()
	var tickResult struct {
		runs []RunEntry
		err  error
	}
	duplicateSend := false
	select {
	case tickResult = <-tickDone:
	case <-runner.entered:
		duplicateSend = true
	case <-time.After(time.Second):
		t.Fatal("scheduler neither skipped claimed task nor entered duplicate delivery")
	}
	release()
	if err := <-manualDone; err != nil {
		t.Fatalf("manual RunNow returned error: %v", err)
	}
	if duplicateSend {
		tickResult = <-tickDone
		t.Fatal("scheduled and manual runs dispatched the same task concurrently")
	}
	if tickResult.err != nil || len(tickResult.runs) != 0 {
		t.Fatalf("RunDue result = %+v/%v, want claimed task skipped", tickResult.runs, tickResult.err)
	}
	loaded, err := service.Get(taskID)
	if err != nil {
		t.Fatalf("Get after manual/scheduled arbitration: %v", err)
	}
	if len(loaded.RecentRuns) != 1 || runner.sendCount() != 1 {
		t.Fatalf("persisted runs/sends = %d/%d, want one manual run", len(loaded.RecentRuns), runner.sendCount())
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

type blockingTaskMutationRunner struct {
	entered chan Target
	release chan struct{}
	mu      sync.Mutex
	sends   int
}

func (r *blockingTaskMutationRunner) ValidateTarget(context.Context, Target) error { return nil }

func (r *blockingTaskMutationRunner) SendPrompt(_ context.Context, target Target, _ string) (Delivery, error) {
	r.mu.Lock()
	r.sends++
	r.mu.Unlock()
	r.entered <- target
	<-r.release
	return Delivery{Pane: "%1", SubmitKeyDispatched: true}, nil
}

func (r *blockingTaskMutationRunner) sendCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sends
}

func newBlockedMutationService(t *testing.T) (*Service, *blockingTaskMutationRunner, string) {
	t.Helper()
	now := time.Date(2026, 7, 31, 20, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir())
	runner := &blockingTaskMutationRunner{entered: make(chan Target, 4), release: make(chan struct{})}
	task := &Task{
		ID:         "tsk_mutation",
		Name:       "serialized task",
		Prompt:     "run once",
		Targets:    []Target{{SessionName: "ops", UnixUser: "alice"}},
		Schedule:   Schedule{Type: "interval", EveryMinutes: 15, Timezone: "UTC"},
		Enabled:    true,
		NextRun:    timePtr(now.Add(-time.Minute)),
		CreatedAt:  now.Add(-time.Hour),
		UpdatedAt:  now.Add(-time.Hour),
		RecentRuns: []RunEntry{},
		Audit:      []AuditEntry{},
	}
	if err := store.Save(task); err != nil {
		t.Fatalf("save mutation test task: %v", err)
	}
	service := NewService(store, runner, ServiceOptions{Now: func() time.Time { return now }})
	return service, runner, task.ID
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
