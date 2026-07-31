package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
	"github.com/chrote/server/internal/scheduled"
)

// ScheduledHandler exposes persisted tmux prompt schedules.
type ScheduledHandler struct {
	service   *scheduled.Service
	scheduler *scheduled.Scheduler
}

// ScheduledTmuxRunner delivers scheduled prompts through CHROTE's tmux config.
type ScheduledTmuxRunner struct {
	tmux *TmuxHandler
}

const (
	scheduledMutationIntentHeader = "X-Chrote-Intent"
	scheduledMutationIntentValue  = "scheduled-task"
)

// NewScheduledTmuxRunner creates a safe argv-only tmux prompt runner.
func NewScheduledTmuxRunner(tmux *TmuxHandler) *ScheduledTmuxRunner {
	if tmux == nil {
		tmux = NewTmuxHandler()
	}
	return &ScheduledTmuxRunner{tmux: tmux}
}

// ValidateTarget verifies the configured tmux socket/user can see the target session.
func (r *ScheduledTmuxRunner) ValidateTarget(ctx context.Context, target scheduled.Target) error {
	resolved, err := r.tmux.targetForUnixUser(target.UnixUser)
	if err != nil {
		return err
	}
	if strings.TrimSpace(target.SessionName) == "" {
		return fmt.Errorf("%w: target sessionName is required", scheduled.ErrTargetNotFound)
	}
	_, err = r.runTmux(ctx, resolved.socket, "has-session", "-t", target.SessionName)
	if err != nil {
		return fmt.Errorf("%w: %s", scheduled.ErrTargetNotFound, err.Error())
	}
	return nil
}

// SendPrompt delivers the prompt through the same guarded paste path as Send to
// Session: the prompt is loaded into a private tmux buffer and pasted only while
// the resolved pane generation still matches, then one guarded submit key is
// dispatched after the paste settles. Prompt text is never shell-interpolated.
func (r *ScheduledTmuxRunner) SendPrompt(ctx context.Context, target scheduled.Target, prompt string) (scheduled.Delivery, error) {
	resolved, err := r.tmux.targetForUnixUser(target.UnixUser)
	if err != nil {
		return scheduled.Delivery{}, err
	}
	if strings.TrimSpace(target.SessionName) == "" {
		return scheduled.Delivery{}, fmt.Errorf("%w: target sessionName is required", scheduled.ErrTargetNotFound)
	}
	pane, err := r.tmux.resolveActiveSendPane(ctx, resolved, target.SessionName)
	if err != nil {
		return scheduled.Delivery{}, fmt.Errorf("%w: %s", scheduled.ErrTargetNotFound, err.Error())
	}

	payloadPath, cleanup, err := writeScheduledPromptPayload(prompt)
	if err != nil {
		return scheduled.Delivery{}, err
	}
	defer cleanup()

	bufferName := "chrote-scheduled-" + strings.TrimPrefix(pane.PaneID, "%")
	result, err := r.tmux.sendBufferToPane(ctx, resolved, pane, bufferName, payloadPath, true)
	if err != nil {
		return scheduled.Delivery{}, err
	}
	switch result.Kind {
	case paneSendTargetChanged:
		return scheduled.Delivery{}, fmt.Errorf("%w: pane %s changed before the prompt was pasted", scheduled.ErrTargetNotFound, pane.PaneID)
	case paneSendUnknown:
		detail := strings.TrimSpace(result.Detail)
		if detail == "" {
			detail = "tmux did not confirm delivery"
		}
		return scheduled.Delivery{}, fmt.Errorf("delivery to pane %s is unconfirmed: %s", pane.PaneID, detail)
	}
	if !result.SubmitKeyDispatched {
		return scheduled.Delivery{}, fmt.Errorf("%w: prompt was pasted but the submit key was not dispatched", scheduled.ErrTargetNotFound)
	}
	return scheduled.Delivery{
		Pane:      pane.PaneID,
		Submitted: true,
		Detail:    "pasted; submit key dispatched (application acceptance unconfirmed)",
	}, nil
}

// writeScheduledPromptPayload stages the prompt for tmux load-buffer. The file is
// private to the CHROTE service and removed after the paste; unlike Send to
// Session there are no attachments to retain, so the prompt reaches the pane
// with nothing appended to it.
func writeScheduledPromptPayload(prompt string) (string, func(), error) {
	file, err := os.CreateTemp("", "chrote-scheduled-prompt-*.txt")
	if err != nil {
		return "", func() {}, fmt.Errorf("stage scheduled prompt: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("stage scheduled prompt: %w", err)
	}
	if _, err := file.WriteString(prompt); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("stage scheduled prompt: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("stage scheduled prompt: %w", err)
	}
	return path, cleanup, nil
}

func (r *ScheduledTmuxRunner) runTmux(ctx context.Context, socket string, args ...string) (string, error) {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", context.DeadlineExceeded
		}
	}
	return r.tmux.runTmuxOnSocketContext(ctx, socket, args...)
}

// NewScheduledHandler creates the production scheduled-task handler.
func NewScheduledHandler(tmux *TmuxHandler) *ScheduledHandler {
	store := scheduled.NewStore("")
	service := scheduled.NewService(store, NewScheduledTmuxRunner(tmux), scheduled.ServiceOptions{ValidateTargets: true})
	return NewScheduledHandlerWithService(service)
}

// NewScheduledHandlerWithService creates a handler around an explicit service for tests.
func NewScheduledHandlerWithService(service *scheduled.Service) *ScheduledHandler {
	return &ScheduledHandler{
		service:   service,
		scheduler: scheduled.NewScheduler(service, 30*time.Second),
	}
}

// RegisterRoutes registers scheduled-task routes.
func (h *ScheduledHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/scheduled-tasks", h.ListTasks)
	mux.HandleFunc("POST /api/scheduled-tasks", h.CreateTask)
	mux.HandleFunc("GET /api/scheduled-tasks/{id}", h.GetTask)
	mux.HandleFunc("PATCH /api/scheduled-tasks/{id}", h.PatchTask)
	mux.HandleFunc("DELETE /api/scheduled-tasks/{id}", h.DeleteTask)
	mux.HandleFunc("POST /api/scheduled-tasks/{id}/pause", h.PauseTask)
	mux.HandleFunc("POST /api/scheduled-tasks/{id}/resume", h.ResumeTask)
	mux.HandleFunc("POST /api/scheduled-tasks/{id}/run-now", h.RunTaskNow)
}

// StartScheduler starts the background schedule loop.
func (h *ScheduledHandler) StartScheduler() error {
	if h == nil || h.scheduler == nil {
		return nil
	}
	return h.scheduler.Start()
}

// StopScheduler stops the background schedule loop.
func (h *ScheduledHandler) StopScheduler() {
	if h == nil || h.scheduler == nil {
		return
	}
	h.scheduler.Stop()
}

func (h *ScheduledHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.service.List()
	if err != nil {
		writeScheduledError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]any{"tasks": tasks})
}

func (h *ScheduledHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	if !requireScheduledMutation(w, r, true) {
		return
	}
	var request scheduledTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid scheduled task JSON: "+err.Error())
		return
	}
	if !rejectScheduledSockets(w, request.Target, request.Targets) {
		return
	}
	task, err := h.service.Create(r.Context(), scheduled.CreateTaskRequest{
		Name:      request.Name,
		Prompt:    request.Prompt,
		Targets:   scheduledTargetList(request.Target, request.Targets),
		Schedule:  request.Schedule,
		Enabled:   request.Enabled,
		Paused:    request.Paused,
		CreatedBy: request.CreatedBy,
		UpdatedBy: request.UpdatedBy,
	})
	if err != nil {
		writeScheduledError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]any{"task": task})
}

func (h *ScheduledHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	task, err := h.service.Get(r.PathValue("id"))
	if err != nil {
		writeScheduledError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]any{"task": task})
}

func (h *ScheduledHandler) PatchTask(w http.ResponseWriter, r *http.Request) {
	if !requireScheduledMutation(w, r, true) {
		return
	}
	var request scheduledTaskPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid scheduled task JSON: "+err.Error())
		return
	}
	requestedTargets := []scheduledAPITarget{}
	if request.Targets != nil {
		requestedTargets = *request.Targets
	}
	if !rejectScheduledSockets(w, request.Target, requestedTargets) {
		return
	}
	patch := scheduled.PatchTaskRequest{
		Name:      request.Name,
		Prompt:    request.Prompt,
		Schedule:  request.Schedule,
		Enabled:   request.Enabled,
		Paused:    request.Paused,
		UpdatedBy: request.UpdatedBy,
	}
	if request.Target != nil || request.Targets != nil {
		targets := scheduledTargetList(request.Target, requestedTargets)
		patch.Targets = &targets
	}
	task, err := h.service.Patch(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		writeScheduledError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]any{"task": task})
}

func (h *ScheduledHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	if !requireScheduledMutation(w, r, false) {
		return
	}
	id := r.PathValue("id")
	if err := h.service.Delete(id); err != nil {
		writeScheduledError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]any{"deleted": id})
}

func (h *ScheduledHandler) PauseTask(w http.ResponseWriter, r *http.Request) {
	if !requireScheduledMutation(w, r, false) {
		return
	}
	actor := actorFromRequest(r)
	task, err := h.service.Pause(r.PathValue("id"), actor)
	if err != nil {
		writeScheduledError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]any{"task": task})
}

func (h *ScheduledHandler) ResumeTask(w http.ResponseWriter, r *http.Request) {
	if !requireScheduledMutation(w, r, false) {
		return
	}
	actor := actorFromRequest(r)
	task, err := h.service.Resume(r.PathValue("id"), actor)
	if err != nil {
		writeScheduledError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]any{"task": task})
}

func (h *ScheduledHandler) RunTaskNow(w http.ResponseWriter, r *http.Request) {
	if !requireScheduledMutation(w, r, false) {
		return
	}
	actor := actorFromRequest(r)
	task, run, err := h.service.RunNow(r.Context(), r.PathValue("id"), actor)
	if err != nil {
		writeScheduledError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]any{"task": task, "run": run})
}

func requireScheduledMutation(w http.ResponseWriter, r *http.Request, requireJSON bool) bool {
	if r.Header.Get(scheduledMutationIntentHeader) != scheduledMutationIntentValue {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", "scheduled task mutations require X-Chrote-Intent: scheduled-task")
		return false
	}
	if !requireJSON {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		core.WriteError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "scheduled task mutations require Content-Type: application/json")
		return false
	}
	return true
}

type scheduledTaskRequest struct {
	Name      string               `json:"name"`
	Prompt    string               `json:"prompt"`
	Target    *scheduledAPITarget  `json:"target,omitempty"`
	Targets   []scheduledAPITarget `json:"targets,omitempty"`
	Schedule  scheduled.Schedule   `json:"schedule"`
	Enabled   *bool                `json:"enabled,omitempty"`
	Paused    bool                 `json:"paused,omitempty"`
	CreatedBy string               `json:"createdBy,omitempty"`
	UpdatedBy string               `json:"updatedBy,omitempty"`
}

type scheduledTaskPatchRequest struct {
	Name      *string               `json:"name,omitempty"`
	Prompt    *string               `json:"prompt,omitempty"`
	Target    *scheduledAPITarget   `json:"target,omitempty"`
	Targets   *[]scheduledAPITarget `json:"targets,omitempty"`
	Schedule  *scheduled.Schedule   `json:"schedule,omitempty"`
	Enabled   *bool                 `json:"enabled,omitempty"`
	Paused    *bool                 `json:"paused,omitempty"`
	UpdatedBy string                `json:"updatedBy,omitempty"`
}

type scheduledAPITarget struct {
	SessionName string `json:"sessionName"`
	UnixUser    string `json:"unixUser,omitempty"`
	Socket      string `json:"socket,omitempty"`
}

func (t scheduledAPITarget) toScheduled() scheduled.Target {
	return scheduled.Target{SessionName: t.SessionName, UnixUser: t.UnixUser}
}

// scheduledTargetList folds the documented single `target` object and the
// multi-target `targets` array into one ordered list. Callers may send either.
func scheduledTargetList(single *scheduledAPITarget, many []scheduledAPITarget) []scheduled.Target {
	targets := make([]scheduled.Target, 0, len(many)+1)
	for _, target := range many {
		targets = append(targets, target.toScheduled())
	}
	if single != nil {
		targets = append(targets, single.toScheduled())
	}
	return targets
}

// rejectScheduledSockets fails closed on client-supplied socket paths; CHROTE
// resolves tmux sockets server-side from its terminal configuration.
func rejectScheduledSockets(w http.ResponseWriter, single *scheduledAPITarget, many []scheduledAPITarget) bool {
	if single != nil && single.Socket != "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "target socket is not accepted; CHROTE resolves sockets server-side")
		return false
	}
	for _, target := range many {
		if target.Socket != "" {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "target socket is not accepted; CHROTE resolves sockets server-side")
			return false
		}
	}
	return true
}

func actorFromRequest(r *http.Request) string {
	var body struct {
		UpdatedBy string `json:"updatedBy"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	return strings.TrimSpace(body.UpdatedBy)
}

func writeScheduledError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, scheduled.ErrNotFound):
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, scheduled.ErrInvalid):
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, scheduled.ErrTargetNotFound):
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	default:
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
	}
}
