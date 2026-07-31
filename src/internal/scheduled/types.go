// Package scheduled owns persisted CHROTE scheduled tmux tasks.
package scheduled

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	// RunStatusSuccess records a prompt delivery that completed without runner error.
	RunStatusSuccess = "success"
	// RunStatusError records a prompt delivery that failed at validation or send time.
	RunStatusError = "error"
	// RunStatusPartial records a fan-out where some targets took the prompt and
	// others failed. A dead session must never block healthy ones.
	RunStatusPartial = "partial"
	// SubmitKeyDispatchedDetail is the truthful transport receipt stored for a
	// guarded submit. It deliberately does not claim application acceptance.
	SubmitKeyDispatchedDetail = "pasted; submit key dispatched (application acceptance unconfirmed)"
	legacySubmittedDetail     = "pasted and submitted"
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
	SendPrompt(context.Context, Target, string) (Delivery, error)
}

// Delivery describes the tmux transport receipt for one target. A dispatched
// submit key does not prove that the application accepted the prompt.
type Delivery struct {
	Pane                string
	SubmitKeyDispatched bool
	Detail              string
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

// TargetRun records the delivery outcome for a single target inside one run.
type TargetRun struct {
	SessionName         string `json:"sessionName"`
	UnixUser            string `json:"unixUser,omitempty"`
	Status              string `json:"status"`
	Pane                string `json:"pane,omitempty"`
	SubmitKeyDispatched bool   `json:"submitKeyDispatched,omitempty"`
	Message             string `json:"message,omitempty"`
}

// UnmarshalJSON conservatively migrates the legacy submission claim emitted by
// older CHROTE builds into a truthful tmux transport receipt. MarshalJSON then
// emits only the current schema, so API reads cannot repeat the false ACK.
func (t *TargetRun) UnmarshalJSON(raw []byte) error {
	type targetRunAlias TargetRun
	var document struct {
		targetRunAlias
		LegacySubmitted bool `json:"submitted"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	*t = TargetRun(document.targetRunAlias)
	if document.LegacySubmitted {
		t.SubmitKeyDispatched = true
	}
	if t.Message == legacySubmittedDetail {
		t.Message = SubmitKeyDispatchedDetail
	}
	return nil
}

// RunEntry is a bounded audit record for recent task executions.
type RunEntry struct {
	ID         string      `json:"id"`
	Trigger    string      `json:"trigger"`
	StartedAt  time.Time   `json:"startedAt"`
	FinishedAt time.Time   `json:"finishedAt"`
	Status     string      `json:"status"`
	Message    string      `json:"message,omitempty"`
	Targets    []TargetRun `json:"targets,omitempty"`
}

// AuditEntry is a bounded operator/agent-visible change log.
type AuditEntry struct {
	At      time.Time `json:"at"`
	Actor   string    `json:"actor,omitempty"`
	Action  string    `json:"action"`
	Message string    `json:"message,omitempty"`
}

// Task is the durable scheduled-task document persisted by Store. One task may
// fan the same prompt out to several tmux sessions.
type Task struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Prompt     string       `json:"prompt"`
	Targets    []Target     `json:"targets"`
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

// UnmarshalJSON accepts both the multi-target schema and the legacy single
// `target` object written by earlier CHROTE builds and still documented for
// agent callers.
func (t *Task) UnmarshalJSON(raw []byte) error {
	type taskAlias Task
	var document struct {
		taskAlias
		LegacyTarget *Target `json:"target"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	*t = Task(document.taskAlias)
	t.Targets = normalizeTargets(t.Targets, document.LegacyTarget)
	return nil
}

// MarshalJSON writes the multi-target schema and mirrors the first target into
// the legacy `target` field so an older build (or an older API client) reading
// the same document still sees a usable single target.
func (t Task) MarshalJSON() ([]byte, error) {
	type taskAlias Task
	document := struct {
		taskAlias
		LegacyTarget *Target `json:"target,omitempty"`
	}{taskAlias: taskAlias(t)}
	if len(t.Targets) > 0 {
		first := t.Targets[0]
		document.LegacyTarget = &first
	}
	return json.Marshal(document)
}

// normalizeTargets folds an optional legacy single target into the target list
// and drops empty entries without reordering the caller's selection.
func normalizeTargets(targets []Target, legacy *Target) []Target {
	normalized := make([]Target, 0, len(targets)+1)
	seen := map[Target]bool{}
	appendTarget := func(target Target) {
		target = normalizeTarget(target)
		if target.SessionName == "" || seen[target] {
			return
		}
		seen[target] = true
		normalized = append(normalized, target)
	}
	for _, target := range targets {
		appendTarget(target)
	}
	if legacy != nil {
		appendTarget(*legacy)
	}
	return normalized
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	clone := *task
	clone.Targets = append([]Target(nil), task.Targets...)
	if task.NextRun != nil {
		next := *task.NextRun
		clone.NextRun = &next
	}
	if task.LastRun != nil {
		last := *task.LastRun
		clone.LastRun = &last
	}
	clone.RecentRuns = append([]RunEntry(nil), task.RecentRuns...)
	for index := range clone.RecentRuns {
		clone.RecentRuns[index].Targets = append([]TargetRun(nil), clone.RecentRuns[index].Targets...)
	}
	clone.Audit = append([]AuditEntry(nil), task.Audit...)
	return &clone
}
