package scheduled

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxRecentRuns   = 20
	defaultMaxAuditEntries = 50
	defaultRunTimeout      = 5 * time.Second
	defaultValidateTimeout = 5 * time.Second
	maxTargetsPerTask      = 32
)

var (
	safeSessionName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,50}$`)
	safeUnixUser    = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// ServiceOptions configures Service behavior.
type ServiceOptions struct {
	Now                     func() time.Time
	ValidateTargets         bool
	MaxRecentRuns           int
	MaxAuditEntries         int
	RunTimeout              time.Duration
	TargetValidationTimeout time.Duration
	MaxConcurrentDeliveries int
}

// CreateTaskRequest is the normalized API-independent create request.
type CreateTaskRequest struct {
	Name      string
	Prompt    string
	Targets   []Target
	Schedule  Schedule
	Enabled   *bool
	Paused    bool
	CreatedBy string
	UpdatedBy string
}

// PatchTaskRequest is the API-independent partial update request.
type PatchTaskRequest struct {
	Name      *string
	Prompt    *string
	Targets   *[]Target
	Schedule  *Schedule
	Enabled   *bool
	Paused    *bool
	UpdatedBy string
}

// Service owns task validation, persistence, and firing semantics.
type Service struct {
	store                   *Store
	runner                  Runner
	now                     func() time.Time
	validateTargets         bool
	maxRecentRuns           int
	maxAuditEntries         int
	runTimeout              time.Duration
	targetValidationTimeout time.Duration
	maxConcurrentDeliveries int
}

// NewService creates a task service around a Store and Runner.
func NewService(store *Store, runner Runner, options ServiceOptions) *Service {
	if store == nil {
		store = NewStore("")
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	maxRecentRuns := options.MaxRecentRuns
	if maxRecentRuns <= 0 {
		maxRecentRuns = defaultMaxRecentRuns
	}
	maxAuditEntries := options.MaxAuditEntries
	if maxAuditEntries <= 0 {
		maxAuditEntries = defaultMaxAuditEntries
	}
	runTimeout := options.RunTimeout
	if runTimeout <= 0 {
		runTimeout = defaultRunTimeout
	}
	validationTimeout := options.TargetValidationTimeout
	if validationTimeout <= 0 {
		validationTimeout = defaultValidateTimeout
	}
	maxConcurrentDeliveries := options.MaxConcurrentDeliveries
	if maxConcurrentDeliveries <= 0 {
		maxConcurrentDeliveries = 1
	}
	if maxConcurrentDeliveries > maxTargetsPerTask {
		maxConcurrentDeliveries = maxTargetsPerTask
	}
	return &Service{
		store:                   store,
		runner:                  runner,
		now:                     now,
		validateTargets:         options.ValidateTargets,
		maxRecentRuns:           maxRecentRuns,
		maxAuditEntries:         maxAuditEntries,
		runTimeout:              runTimeout,
		targetValidationTimeout: validationTimeout,
		maxConcurrentDeliveries: maxConcurrentDeliveries,
	}
}

// Store returns the underlying durable store.
func (s *Service) Store() *Store {
	return s.store
}

// List returns all scheduled tasks.
func (s *Service) List() ([]Task, error) {
	return s.store.List()
}

// Get returns one scheduled task by ID.
func (s *Service) Get(id string) (*Task, error) {
	return s.store.Get(id)
}

// Create validates, computes nextRun, and persists a new task.
func (s *Service) Create(ctx context.Context, request CreateTaskRequest) (*Task, error) {
	now := s.now().UTC()
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	updatedBy := strings.TrimSpace(request.UpdatedBy)
	createdBy := strings.TrimSpace(request.CreatedBy)
	if updatedBy == "" {
		updatedBy = createdBy
	}
	task := &Task{
		ID:         newTaskID(now),
		Name:       strings.TrimSpace(request.Name),
		Prompt:     request.Prompt,
		Targets:    normalizeTargets(request.Targets, nil),
		Schedule:   request.Schedule,
		Enabled:    enabled,
		Paused:     request.Paused,
		CreatedBy:  createdBy,
		UpdatedBy:  updatedBy,
		CreatedAt:  now,
		UpdatedAt:  now,
		RecentRuns: []RunEntry{},
		Audit:      []AuditEntry{},
	}
	if err := s.validateAndPrepare(ctx, task, true, now); err != nil {
		return nil, err
	}
	task.Audit = boundedAudit(append(task.Audit, AuditEntry{At: now, Actor: updatedBy, Action: "created"}), s.maxAuditEntries)
	if err := s.store.Save(task); err != nil {
		return nil, err
	}
	return cloneTask(task), nil
}

// Patch applies partial changes to an existing task.
func (s *Service) Patch(ctx context.Context, id string, request PatchTaskRequest) (*Task, error) {
	task, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	reschedule := false
	if request.Name != nil {
		task.Name = strings.TrimSpace(*request.Name)
	}
	if request.Prompt != nil {
		task.Prompt = *request.Prompt
	}
	if request.Targets != nil {
		task.Targets = normalizeTargets(*request.Targets, nil)
		reschedule = true
	}
	if request.Schedule != nil {
		task.Schedule = *request.Schedule
		reschedule = true
	}
	if request.Enabled != nil {
		task.Enabled = *request.Enabled
		reschedule = true
	}
	if request.Paused != nil {
		task.Paused = *request.Paused
		reschedule = true
	}
	if request.UpdatedBy != "" {
		task.UpdatedBy = strings.TrimSpace(request.UpdatedBy)
	}
	task.UpdatedAt = now
	if err := s.validateAndPrepare(ctx, task, request.Targets != nil, now); err != nil {
		return nil, err
	}
	if reschedule || (task.Enabled && !task.Paused && task.NextRun == nil) {
		if err := s.recomputeNextRun(task, now); err != nil {
			return nil, err
		}
	}
	task.Audit = boundedAudit(append(task.Audit, AuditEntry{At: now, Actor: task.UpdatedBy, Action: "updated"}), s.maxAuditEntries)
	if err := s.store.Save(task); err != nil {
		return nil, err
	}
	return cloneTask(task), nil
}

// Delete removes an existing task.
func (s *Service) Delete(id string) error {
	return s.store.Delete(id)
}

// Pause marks a task paused and clears nextRun without disabling it.
func (s *Service) Pause(id, actor string) (*Task, error) {
	task, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	task.Paused = true
	task.NextRun = nil
	task.UpdatedAt = now
	if actor = strings.TrimSpace(actor); actor != "" {
		task.UpdatedBy = actor
	}
	task.Audit = boundedAudit(append(task.Audit, AuditEntry{At: now, Actor: task.UpdatedBy, Action: "paused"}), s.maxAuditEntries)
	if err := s.store.Save(task); err != nil {
		return nil, err
	}
	return cloneTask(task), nil
}

// Resume unpauses a task and recomputes its nextRun if it is enabled.
func (s *Service) Resume(id, actor string) (*Task, error) {
	task, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	task.Paused = false
	task.UpdatedAt = now
	if actor = strings.TrimSpace(actor); actor != "" {
		task.UpdatedBy = actor
	}
	if err := s.validateAndPrepare(context.Background(), task, false, now); err != nil {
		return nil, err
	}
	task.Audit = boundedAudit(append(task.Audit, AuditEntry{At: now, Actor: task.UpdatedBy, Action: "resumed"}), s.maxAuditEntries)
	if err := s.store.Save(task); err != nil {
		return nil, err
	}
	return cloneTask(task), nil
}

// RunNow sends the task prompt immediately and persists the run result.
func (s *Service) RunNow(ctx context.Context, id, actor string) (*Task, RunEntry, error) {
	task, err := s.store.Get(id)
	if err != nil {
		return nil, RunEntry{}, err
	}
	run := s.fireTask(ctx, task, "manual")
	now := s.now().UTC()
	if actor = strings.TrimSpace(actor); actor != "" {
		task.UpdatedBy = actor
	}
	task.UpdatedAt = now
	task.Audit = boundedAudit(append(task.Audit, AuditEntry{At: now, Actor: task.UpdatedBy, Action: "run-now", Message: run.Status}), s.maxAuditEntries)
	if err := s.store.Save(task); err != nil {
		return nil, RunEntry{}, err
	}
	return cloneTask(task), run, nil
}

// RunDue reloads persisted tasks, computes missing nextRun values, and fires due tasks once.
func (s *Service) RunDue(ctx context.Context) ([]RunEntry, error) {
	tasks, err := s.store.List()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	runs := []RunEntry{}
	for i := range tasks {
		task := tasks[i]
		originalUpdatedAt := task.UpdatedAt
		changed := false
		if task.Enabled && !task.Paused && task.NextRun != nil && !task.NextRun.After(now) {
			release, claimed, err := s.store.TryLock(task.ID)
			if err != nil {
				return runs, err
			}
			if !claimed {
				continue
			}

			latest, err := s.store.Get(task.ID)
			if errors.Is(err, ErrNotFound) {
				release()
				continue
			}
			if err != nil {
				release()
				return runs, err
			}
			if !latest.UpdatedAt.Equal(originalUpdatedAt) || !latest.Enabled || latest.Paused || latest.NextRun == nil || latest.NextRun.After(now) {
				release()
				continue
			}
			task = *latest

			if err := s.validateAndPrepare(ctx, &task, false, now); err != nil {
				task.LastStatus = RunStatusError
				task.UpdatedAt = now
				task.Audit = boundedAudit(append(task.Audit, AuditEntry{At: now, Action: "scheduler-error", Message: err.Error()}), s.maxAuditEntries)
			} else {
				run := s.fireTask(ctx, &task, "scheduled")
				task.UpdatedAt = now
				task.Audit = boundedAudit(append(task.Audit, AuditEntry{At: now, Action: "scheduled-run", Message: run.Status}), s.maxAuditEntries)
				runs = append(runs, run)
			}
			if err := s.saveSchedulerChange(&task, originalUpdatedAt); err != nil {
				release()
				return runs, err
			}
			release()
			continue
		} else if task.Enabled && !task.Paused && task.NextRun == nil {
			if err := s.validateAndPrepare(ctx, &task, false, now); err != nil {
				task.LastStatus = RunStatusError
				task.UpdatedAt = now
				task.Audit = boundedAudit(append(task.Audit, AuditEntry{At: now, Action: "scheduler-error", Message: err.Error()}), s.maxAuditEntries)
			} else if task.NextRun == nil {
				if err := s.recomputeNextRun(&task, now); err != nil {
					return runs, err
				}
			}
			changed = true
		} else if err := s.validateAndPrepare(ctx, &task, false, now); err != nil {
			task.LastStatus = RunStatusError
			task.UpdatedAt = now
			task.Audit = boundedAudit(append(task.Audit, AuditEntry{At: now, Action: "scheduler-error", Message: err.Error()}), s.maxAuditEntries)
			changed = true
		}
		if changed {
			if err := s.saveSchedulerChange(&task, originalUpdatedAt); err != nil {
				return runs, err
			}
		}
	}
	return runs, nil
}

func (s *Service) saveSchedulerChange(task *Task, originalUpdatedAt time.Time) error {
	latest, err := s.store.Get(task.ID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !latest.UpdatedAt.Equal(originalUpdatedAt) {
		return nil
	}
	return s.store.Save(task)
}

func (s *Service) validateAndPrepare(ctx context.Context, task *Task, validateTarget bool, now time.Time) error {
	if task.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if strings.TrimSpace(task.Prompt) == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalid)
	}
	if strings.ContainsRune(task.Prompt, '\x00') {
		return fmt.Errorf("%w: prompt must not contain NUL bytes", ErrInvalid)
	}
	if len(task.Prompt) > 64*1024 {
		return fmt.Errorf("%w: prompt is too large", ErrInvalid)
	}
	task.Targets = normalizeTargets(task.Targets, nil)
	if len(task.Targets) == 0 {
		return fmt.Errorf("%w: at least one target session is required", ErrInvalid)
	}
	if len(task.Targets) > maxTargetsPerTask {
		return fmt.Errorf("%w: a task may target at most %d sessions", ErrInvalid, maxTargetsPerTask)
	}
	for _, target := range task.Targets {
		if !safeSessionName.MatchString(target.SessionName) {
			return fmt.Errorf("%w: target sessionName is required and must contain only letters, numbers, dashes, and underscores", ErrInvalid)
		}
		if target.UnixUser != "" && !safeUnixUser.MatchString(target.UnixUser) {
			return fmt.Errorf("%w: target unixUser contains invalid characters", ErrInvalid)
		}
	}
	schedule, err := NormalizeSchedule(task.Schedule)
	if err != nil {
		return err
	}
	task.Schedule = schedule
	if err := s.recomputeNextRun(task, now); err != nil {
		return err
	}
	if validateTarget && s.validateTargets {
		if err := s.validateTargetSet(ctx, task.Targets); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recomputeNextRun(task *Task, now time.Time) error {
	if !task.Enabled || task.Paused {
		task.NextRun = nil
		return nil
	}
	next, err := NextRun(task.Schedule, now)
	if err != nil {
		return err
	}
	task.NextRun = &next
	return nil
}

// fireTask delivers the prompt to every target. Targets are independent: one
// missing session is recorded against that target only and never cancels
// delivery to the healthy ones.
func (s *Service) fireTask(ctx context.Context, task *Task, trigger string) RunEntry {
	started := s.now().UTC()
	run := RunEntry{ID: newRunID(started), Trigger: trigger, StartedAt: started}

	targets := normalizeTargets(task.Targets, nil)
	if len(targets) == 0 {
		run.Status = RunStatusError
		run.Message = "task has no targets"
	} else {
		failures := []string{}
		succeeded := 0
		run.Targets = s.deliverToTargets(ctx, targets, task.Prompt)
		for index, result := range run.Targets {
			if result.Status == RunStatusSuccess {
				succeeded++
				continue
			}
			failures = append(failures, fmt.Sprintf("%s: %s", targetLabel(targets[index]), result.Message))
		}
		switch {
		case succeeded == len(targets):
			run.Status = RunStatusSuccess
		case succeeded == 0:
			run.Status = RunStatusError
		default:
			run.Status = RunStatusPartial
		}
		run.Message = strings.Join(failures, "; ")
	}

	finished := s.now().UTC()
	run.FinishedAt = finished
	task.LastRun = &finished
	task.LastStatus = run.Status
	task.RecentRuns = boundedRuns(append([]RunEntry{run}, task.RecentRuns...), s.maxRecentRuns)
	_ = s.recomputeNextRun(task, finished)
	return run
}

func (s *Service) deliverToTargets(ctx context.Context, targets []Target, prompt string) []TargetRun {
	results := make([]TargetRun, len(targets))
	if s.maxConcurrentDeliveries == 1 || len(targets) == 1 {
		for index, target := range targets {
			results[index] = s.deliverToTarget(ctx, target, prompt)
		}
		return results
	}

	semaphore := make(chan struct{}, s.maxConcurrentDeliveries)
	var wait sync.WaitGroup
	for index, target := range targets {
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results[index] = TargetRun{
					SessionName: target.SessionName,
					UnixUser:    target.UnixUser,
					Status:      RunStatusError,
					Message:     ctx.Err().Error(),
				}
				return
			}
			results[index] = s.deliverToTarget(ctx, target, prompt)
		}()
	}
	wait.Wait()
	return results
}

func (s *Service) deliverToTarget(ctx context.Context, target Target, prompt string) TargetRun {
	result := TargetRun{SessionName: target.SessionName, UnixUser: target.UnixUser, Status: RunStatusSuccess}
	deliveryCtx, cancel := context.WithTimeout(ctx, s.runTimeout)
	defer cancel()
	if s.validateTargets {
		if err := s.validateTarget(deliveryCtx, target); err != nil {
			result.Status = RunStatusError
			result.Message = err.Error()
			return result
		}
	}
	delivery, err := s.runner.SendPrompt(deliveryCtx, target, prompt)
	if err != nil {
		result.Status = RunStatusError
		result.Message = err.Error()
		return result
	}
	result.Pane = delivery.Pane
	result.SubmitKeyDispatched = delivery.SubmitKeyDispatched
	result.Message = delivery.Detail
	return result
}

func targetLabel(target Target) string {
	if target.UnixUser == "" {
		return target.SessionName
	}
	return target.UnixUser + "/" + target.SessionName
}

func (s *Service) validateTargetSet(ctx context.Context, targets []Target) error {
	if s.maxConcurrentDeliveries == 1 || len(targets) == 1 {
		for _, target := range targets {
			if err := s.validateTarget(ctx, target); err != nil {
				return err
			}
		}
		return nil
	}

	errorsByTarget := make([]error, len(targets))
	semaphore := make(chan struct{}, s.maxConcurrentDeliveries)
	var wait sync.WaitGroup
	for index, target := range targets {
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				errorsByTarget[index] = fmt.Errorf("%w: %v", ErrTargetNotFound, ctx.Err())
				return
			}
			errorsByTarget[index] = s.validateTarget(ctx, target)
		}()
	}
	wait.Wait()
	for _, err := range errorsByTarget {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) validateTarget(ctx context.Context, target Target) error {
	if s.runner == nil {
		return fmt.Errorf("%w: runner is not configured", ErrTargetNotFound)
	}
	validateCtx, cancel := context.WithTimeout(ctx, s.targetValidationTimeout)
	defer cancel()
	if err := s.runner.ValidateTarget(validateCtx, target); err != nil {
		return fmt.Errorf("%w: %v", ErrTargetNotFound, err)
	}
	if err := validateCtx.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrTargetNotFound, err)
	}
	return nil
}

func normalizeTarget(target Target) Target {
	return Target{SessionName: strings.TrimSpace(target.SessionName), UnixUser: strings.TrimSpace(target.UnixUser)}
}

func boundedRuns(runs []RunEntry, limit int) []RunEntry {
	if limit <= 0 {
		return []RunEntry{}
	}
	if len(runs) > limit {
		runs = runs[:limit]
	}
	return runs
}

func boundedAudit(entries []AuditEntry, limit int) []AuditEntry {
	if limit <= 0 {
		return []AuditEntry{}
	}
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries
}
