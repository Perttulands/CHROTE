// Package scheduled owns persisted CHROTE scheduled tmux tasks.
package scheduled

import (
	"context"
	"errors"
	"time"
)

const (
	// RunStatusSuccess records a prompt delivery that completed without runner error.
	RunStatusSuccess = "success"
	// RunStatusError records a prompt delivery that failed at validation or send time.
	RunStatusError = "error"
)

var (
	// ErrNotFound indicates that a scheduled task does not exist.
	ErrNotFound = errors.New("scheduled task not found")
	// ErrInvalid indicates that a scheduled task request is invalid.
	ErrInvalid = errors.New("invalid scheduled task")
	// ErrTargetNotFound indicates that the selected tmux target is unavailable.
	ErrTargetNotFound = errors.New("scheduled task target not found")
)

// Runner validates scheduled task targets and sends prompts to them.
type Runner interface {
	ValidateTarget(context.Context, Target) error
	SendPrompt(context.Context, Target, string) error
}

// Target identifies the selected tmux destination. Socket paths are deliberately
// not part of this public schema; they are resolved service-side from CHROTE's
// tmux user/socket configuration.
type Target struct {
	SessionName string `json:"sessionName"`
	UnixUser    string `json:"unixUser,omitempty"`
}

// Schedule describes either an interval or cron trigger.
type Schedule struct {
	Type         string `json:"type"`
	Expression   string `json:"expression,omitempty"`
	Timezone     string `json:"timezone"`
	EveryMinutes int    `json:"everyMinutes,omitempty"`
	Duration     string `json:"duration,omitempty"`
}

// RunEntry is a bounded audit record for recent task executions.
type RunEntry struct {
	ID         string    `json:"id"`
	Trigger    string    `json:"trigger"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Status     string    `json:"status"`
	Message    string    `json:"message,omitempty"`
}

// AuditEntry is a bounded operator/agent-visible change log.
type AuditEntry struct {
	At      time.Time `json:"at"`
	Actor   string    `json:"actor,omitempty"`
	Action  string    `json:"action"`
	Message string    `json:"message,omitempty"`
}

// Task is the durable scheduled-task document persisted by Store.
type Task struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Prompt     string       `json:"prompt"`
	Target     Target       `json:"target"`
	Schedule   Schedule     `json:"schedule"`
	Enabled    bool         `json:"enabled"`
	Paused     bool         `json:"paused"`
	NextRun    *time.Time   `json:"nextRun,omitempty"`
	LastRun    *time.Time   `json:"lastRun,omitempty"`
	LastStatus string       `json:"lastStatus,omitempty"`
	CreatedBy  string       `json:"createdBy,omitempty"`
	UpdatedBy  string       `json:"updatedBy,omitempty"`
	CreatedAt  time.Time    `json:"createdAt"`
	UpdatedAt  time.Time    `json:"updatedAt"`
	RecentRuns []RunEntry   `json:"recentRuns,omitempty"`
	Audit      []AuditEntry `json:"audit,omitempty"`
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	clone := *task
	if task.NextRun != nil {
		next := *task.NextRun
		clone.NextRun = &next
	}
	if task.LastRun != nil {
		last := *task.LastRun
		clone.LastRun = &last
	}
	clone.RecentRuns = append([]RunEntry(nil), task.RecentRuns...)
	clone.Audit = append([]AuditEntry(nil), task.Audit...)
	return &clone
}
