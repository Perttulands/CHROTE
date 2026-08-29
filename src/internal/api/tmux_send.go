package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chrote/server/internal/core"
)

type sendPaneTarget struct {
	SessionID      string `json:"sessionId"`
	Session        string `json:"session"`
	PaneID         string `json:"pane"`
	PanePID        string `json:"panePid"`
	ServerPID      string `json:"serverPid"`
	WindowID       string `json:"windowId,omitempty"`
	WindowName     string `json:"windowName,omitempty"`
	CurrentPath    string `json:"currentPath,omitempty"`
	CurrentCommand string `json:"currentCommand,omitempty"`
	Active         bool   `json:"active"`
}

type sendTargetError struct {
	Status  int
	Code    string
	Message string
}

func (e *sendTargetError) Error() string { return e.Message }

func parseSendPaneTargets(output string) []sendPaneTarget {
	targets := []sendPaneTarget{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.Split(line, "	")
		if len(parts) < 5 {
			continue
		}
		target := sendPaneTarget{
			SessionID: strings.TrimSpace(parts[0]),
			Session:   strings.TrimSpace(parts[1]),
			PaneID:    strings.TrimSpace(parts[2]),
			PanePID:   strings.TrimSpace(parts[3]),
			ServerPID: strings.TrimSpace(parts[4]),
		}
		if len(parts) >= 10 {
			target.WindowID = strings.TrimSpace(parts[5])
			target.WindowName = strings.TrimSpace(parts[6])
			target.CurrentPath = strings.TrimSpace(parts[7])
			target.CurrentCommand = strings.TrimSpace(parts[8])
			target.Active = strings.TrimSpace(parts[9]) == "1"
		}
		if !tmuxSessionIDPattern.MatchString(target.SessionID) ||
			!tmuxPaneIDPattern.MatchString(target.PaneID) ||
			!tmuxPIDPattern.MatchString(target.PanePID) ||
			!tmuxPIDPattern.MatchString(target.ServerPID) {
			continue
		}
		targets = append(targets, target)
	}
	return targets
}

func (h *TmuxHandler) listSendPanes(ctx context.Context, target tmuxTarget, sessionName string) ([]sendPaneTarget, error) {
	output, err := h.runTmuxOnSocketContext(ctx, target.socket, "list-panes", "-a", "-F", "#{session_id}	#{session_name}	#{pane_id}	#{pane_pid}	#{pid}	#{window_id}	#{window_name}	#{pane_current_path}	#{pane_current_command}	#{pane_active}")
	if err != nil {
		return nil, err
	}
	panes := []sendPaneTarget{}
	for _, pane := range parseSendPaneTargets(output) {
		if pane.Session == sessionName {
			panes = append(panes, pane)
		}
	}
	if len(panes) == 0 {
		return nil, &sendTargetError{Status: http.StatusNotFound, Code: "SESSION_NOT_FOUND", Message: fmt.Sprintf("tmux session %q was not found exactly", sessionName)}
	}
	return panes, nil
}

func (h *TmuxHandler) resolveSendPane(ctx context.Context, target tmuxTarget, sessionName, requestedPane string) (sendPaneTarget, error) {
	panes, err := h.listSendPanes(ctx, target, sessionName)
	if err != nil {
		return sendPaneTarget{}, err
	}
	requestedPane = strings.TrimSpace(requestedPane)
	if requestedPane == "" {
		if len(panes) != 1 {
			return sendPaneTarget{}, &sendTargetError{Status: http.StatusConflict, Code: "PANE_REQUIRED", Message: fmt.Sprintf("tmux session %q has %d panes; select an exact %%pane", sessionName, len(panes))}
		}
		return panes[0], nil
	}
	if !tmuxPaneIDPattern.MatchString(requestedPane) {
		return sendPaneTarget{}, &sendTargetError{Status: http.StatusBadRequest, Code: "BAD_REQUEST", Message: "pane must be an immutable tmux pane ID such as %7"}
	}
	for _, pane := range panes {
		if pane.PaneID == requestedPane {
			return pane, nil
		}
	}
	return sendPaneTarget{}, &sendTargetError{Status: http.StatusConflict, Code: "PANE_NOT_IN_SESSION", Message: fmt.Sprintf("pane %q does not belong to tmux session %q", requestedPane, sessionName)}
}

func sameSendPaneGeneration(expected, actual sendPaneTarget) bool {
	return expected.SessionID == actual.SessionID &&
		expected.Session == actual.Session &&
		expected.PaneID == actual.PaneID &&
		expected.PanePID == actual.PanePID &&
		expected.ServerPID == actual.ServerPID
}

const (
	atomicSendPastedMarker              = "CHROTE_SEND_PASTED"
	atomicSendSubmitKeyMarker           = "CHROTE_SEND_SUBMIT_KEY_DISPATCHED"
	atomicSendTargetChangedMark         = "CHROTE_SEND_TARGET_CHANGED"
	atomicSendSubmitTargetChangedMarker = "CHROTE_SEND_SUBMIT_TARGET_CHANGED"
	tmuxSendSubmitSettleDelay           = 1200 * time.Millisecond
	tmuxSendSubmitObservationDelay      = 400 * time.Millisecond
	tmuxSendSubmitObservationTimeout    = 2 * time.Second
)

var tmuxSendSleep = func(ctx context.Context, delay time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func atomicSendCondition(pane sendPaneTarget) string {
	return fmt.Sprintf("#{&&:#{==:#{session_id},%s},#{&&:#{==:#{pane_id},%s},#{&&:#{==:#{pane_pid},%s},#{==:#{pid},%s}}}}", pane.SessionID, pane.PaneID, pane.PanePID, pane.ServerPID)
}

func atomicRetrySendCondition(pane sendPaneTarget, harness tmuxSubmitHarness) string {
	return fmt.Sprintf("#{&&:%s,#{==:#{pane_current_command},%s}}", atomicSendCondition(pane), harness.currentCommand())
}

func atomicPasteCommand(bufferName string, pane sendPaneTarget) string {
	command := fmt.Sprintf("paste-buffer -p -d -b %s -t %s", bufferName, pane.PaneID)
	return command + " ; display-message -p " + atomicSendPastedMarker
}

func atomicSubmitCommand(pane sendPaneTarget) string {
	return fmt.Sprintf("send-keys -t %s Enter ; display-message -p %s", pane.PaneID, atomicSendSubmitKeyMarker)
}

type tmuxSubmitHarness int

const (
	tmuxSubmitHarnessUnknown tmuxSubmitHarness = iota
	tmuxSubmitHarnessCodex
	tmuxSubmitHarnessClaude
)

func (harness tmuxSubmitHarness) currentCommand() string {
	switch harness {
	case tmuxSubmitHarnessCodex:
		return "codex"
	case tmuxSubmitHarnessClaude:
		return "claude"
	default:
		return ""
	}
}

func submitHarnessForPane(pane sendPaneTarget) tmuxSubmitHarness {
	switch strings.ToLower(strings.TrimSpace(pane.CurrentCommand)) {
	case "codex":
		return tmuxSubmitHarnessCodex
	case "claude":
		return tmuxSubmitHarnessClaude
	default:
		return tmuxSubmitHarnessUnknown
	}
}

type submitPayloadEvidence struct {
	witness            string
	codexCollapsedTags []string
}

func buildSubmitPayloadEvidence(payload string) (submitPayloadEvidence, bool) {
	for _, line := range strings.Split(strings.ReplaceAll(payload, "\r\n", "\n"), "\n") {
		witness := strings.Join(strings.Fields(line), " ")
		if len([]rune(witness)) < 16 {
			continue
		}
		runes := []rune(witness)
		if len(runes) > 80 {
			witness = string(runes[:80])
		}
		evidence := submitPayloadEvidence{witness: witness}
		for _, size := range []int{len(payload), utf8.RuneCountInString(payload)} {
			tag := fmt.Sprintf("[Pasted Content %d chars]", size)
			if len(evidence.codexCollapsedTags) == 0 || evidence.codexCollapsedTags[len(evidence.codexCollapsedTags)-1] != tag {
				evidence.codexCollapsedTags = append(evidence.codexCollapsedTags, tag)
			}
		}
		return evidence, true
	}
	return submitPayloadEvidence{}, false
}

func activeSubmitComposer(harness tmuxSubmitHarness, capture string) (string, bool) {
	capture = strings.ReplaceAll(capture, "\r", "")
	lower := strings.ToLower(capture)
	if strings.Contains(lower, "esc to interrupt") ||
		strings.Contains(lower, "ctrl+c to interrupt") ||
		strings.Contains(lower, "quick safety check") ||
		strings.Contains(lower, "yes, i trust this folder") {
		return "", false
	}

	hasCodex := strings.Contains(capture, "OpenAI Codex")
	hasClaude := strings.Contains(capture, "Claude Code")
	if hasCodex == hasClaude {
		return "", false
	}
	prefix := ""
	footerMatches := func(string) bool { return false }
	switch harness {
	case tmuxSubmitHarnessCodex:
		if !hasCodex {
			return "", false
		}
		prefix = "›"
		footerMatches = func(line string) bool {
			line = strings.ToLower(strings.TrimSpace(line))
			return strings.Contains(line, "gpt-") || strings.Contains(line, "% left")
		}
	case tmuxSubmitHarnessClaude:
		if !hasClaude {
			return "", false
		}
		prefix = "❯"
		footerMatches = func(line string) bool {
			return strings.Contains(strings.ToLower(line), "? for shortcuts")
		}
	default:
		return "", false
	}

	lines := strings.Split(capture, "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if last < 1 || !footerMatches(lines[last]) {
		return "", false
	}
	composerStart := -1
	for index := last - 1; index >= 0; index-- {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), prefix) {
			composerStart = index
			break
		}
	}
	if composerStart < 0 {
		return "", false
	}
	for _, line := range lines[composerStart+1 : last] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "$ ") || strings.HasPrefix(trimmed, "# ") {
			return "", false
		}
	}
	return strings.Join(strings.Fields(strings.Join(lines[composerStart:last], "\n")), " "), true
}

func composerMatchesPayload(harness tmuxSubmitHarness, composer string, evidence submitPayloadEvidence) bool {
	if evidence.witness != "" && strings.Contains(composer, evidence.witness) {
		return true
	}
	if harness != tmuxSubmitHarnessCodex {
		return false
	}
	for _, tag := range evidence.codexCollapsedTags {
		if strings.Contains(composer, tag) {
			return true
		}
	}
	return false
}

func (h *TmuxHandler) captureSubmitComposer(
	ctx context.Context,
	target tmuxTarget,
	pane sendPaneTarget,
	harness tmuxSubmitHarness,
	evidence submitPayloadEvidence,
	requirePayload bool,
) (string, bool) {
	panes, err := h.listSendPanes(ctx, target, pane.Session)
	if err != nil {
		return "", false
	}
	live := sendPaneTarget{}
	found := false
	for _, candidate := range panes {
		if sameSendPaneGeneration(pane, candidate) {
			live = candidate
			found = true
			break
		}
	}
	if !found || submitHarnessForPane(live) != harness {
		return "", false
	}
	output, err := h.runTmuxOnSocketContext(ctx, target.socket, "capture-pane", "-p", "-J", "-t", pane.PaneID, "-S", "-200")
	if err != nil {
		return "", false
	}
	composer, ok := activeSubmitComposer(harness, output)
	if !ok || (requirePayload && !composerMatchesPayload(harness, composer, evidence)) {
		return "", false
	}
	return composer, true
}

func (h *TmuxHandler) shouldRetrySubmit(
	ctx context.Context,
	target tmuxTarget,
	pane sendPaneTarget,
	harness tmuxSubmitHarness,
	evidence submitPayloadEvidence,
	expectedComposer string,
) bool {
	if expectedComposer == "" {
		return false
	}
	observationCtx, cancel := context.WithTimeout(ctx, tmuxSendSubmitObservationTimeout)
	defer cancel()
	capture := func() (string, bool) {
		if err := tmuxSendSleep(observationCtx, tmuxSendSubmitObservationDelay); err != nil {
			return "", false
		}
		return h.captureSubmitComposer(observationCtx, target, pane, harness, evidence, true)
	}
	first, ok := capture()
	if !ok || first != expectedComposer {
		return false
	}
	second, ok := capture()
	return ok && second == expectedComposer
}

type paneSendKind int

const (
	// paneSendDelivered means tmux confirmed the paste against the pinned pane
	// generation. SubmitKeyDispatched separately records at least one guarded
	// Enter transport receipt; it never claims application acceptance.
	paneSendDelivered paneSendKind = iota
	// paneSendTargetChanged means the pane generation moved before the paste ran,
	// so nothing was delivered.
	paneSendTargetChanged
	// paneSendUnknown means tmux never confirmed the outcome; the payload may or
	// may not have landed and must not be retried blindly.
	paneSendUnknown
)

// paneSendResult reports what the guarded tmux paste did. Kind covers everything
// after the buffer is loaded; a load failure is returned as an error instead
// because nothing can have been delivered yet.
type paneSendResult struct {
	Kind                paneSendKind
	SubmitKeyDispatched bool
	BufferCleaned       bool
	Detail              string
	OperationErr        error
	CleanupErr          error
}

func reservePaneSendCleanup(ctx context.Context, reserve time.Duration) (context.Context, time.Duration, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	deadline, bounded := ctx.Deadline()
	if reserve <= 0 || !bounded {
		return ctx, 0, func() {}
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return ctx, 0, func() {}
	}
	if maximumReserve := remaining / 4; reserve > maximumReserve {
		reserve = maximumReserve
	}
	if reserve <= 0 {
		return ctx, 0, func() {}
	}
	sendCtx, cancel := context.WithDeadline(ctx, deadline.Add(-reserve))
	return sendCtx, reserve, cancel
}

func paneSendCleanupContext(operationCtx context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if budget <= 0 {
		return operationCtx, func() {}
	}
	deadline := time.Now().Add(budget)
	if operationDeadline, bounded := operationCtx.Deadline(); bounded && operationDeadline.Before(deadline) {
		deadline = operationDeadline
	}
	return context.WithDeadline(context.WithoutCancel(operationCtx), deadline)
}

// sendBufferToPane is the single delivery path shared by Send to Session and
// scheduled tasks: load the payload into a private buffer, paste it only while
// the pinned pane generation still matches, optionally dispatch one guarded
// Enter, and leave no buffer behind. Interactive sends may additionally allow
// one evidence-gated guarded retry; unattended scheduled sends do not.
// Interactive sends pass a background operation
// context so request cancellation cannot tear down a half-applied operator action;
// scheduled sends pass their bounded delivery context end to end.
func (h *TmuxHandler) sendBufferToPane(loadCtx, operationCtx context.Context, cleanupReserve time.Duration, target tmuxTarget, pane sendPaneTarget, bufferName, payloadPath string, submit, retryPendingComposer bool, evidence submitPayloadEvidence) (paneSendResult, error) {
	if operationCtx == nil {
		operationCtx = context.Background()
	}
	sendCtx, cleanupBudget, cancelSend := reservePaneSendCleanup(operationCtx, cleanupReserve)
	defer cancelSend()
	if cleanupBudget > 0 {
		loadCtx = sendCtx
	}
	harness := submitHarnessForPane(pane)
	beforePasteComposer := ""
	beforePasteCaptured := false
	if retryPendingComposer && harness != tmuxSubmitHarnessUnknown && evidence.witness != "" {
		beforePasteComposer, beforePasteCaptured = h.captureSubmitComposer(sendCtx, target, pane, harness, evidence, false)
	}
	bufferDeleted := false
	deleteBuffer := func() error {
		if bufferDeleted {
			return nil
		}
		cleanupCtx, cancelCleanup := paneSendCleanupContext(operationCtx, cleanupBudget)
		defer cancelCleanup()
		_, err := h.runTmuxOnSocketContext(cleanupCtx, target.socket, "delete-buffer", "-b", bufferName)
		if err == nil {
			bufferDeleted = true
		}
		return err
	}

	if _, err := h.runTmuxOnSocketContext(loadCtx, target.socket, "load-buffer", "-b", bufferName, payloadPath); err != nil {
		if cleanupErr := deleteBuffer(); cleanupErr != nil {
			err = fmt.Errorf("%w; buffer cleanup failed: %v", err, cleanupErr)
		}
		return paneSendResult{Kind: paneSendUnknown, BufferCleaned: bufferDeleted}, err
	}

	output, err := h.runTmuxOnSocketContext(
		sendCtx,
		target.socket,
		"if-shell", "-F", "-t", pane.PaneID,
		atomicSendCondition(pane),
		atomicPasteCommand(bufferName, pane),
		"display-message -p "+atomicSendTargetChangedMark,
	)
	if err != nil {
		cleanupErr := deleteBuffer()
		return paneSendResult{Kind: paneSendUnknown, BufferCleaned: cleanupErr == nil, Detail: err.Error(), OperationErr: err, CleanupErr: cleanupErr}, nil
	}

	switch marker := strings.TrimSpace(output); marker {
	case atomicSendTargetChangedMark:
		cleanupErr := deleteBuffer()
		return paneSendResult{Kind: paneSendTargetChanged, BufferCleaned: cleanupErr == nil, CleanupErr: cleanupErr}, nil
	case atomicSendPastedMarker:
		// paste-buffer -d consumed the buffer on success.
		bufferDeleted = true
	default:
		cleanupErr := deleteBuffer()
		return paneSendResult{
			Kind:          paneSendUnknown,
			BufferCleaned: cleanupErr == nil,
			Detail:        fmt.Sprintf("unexpected guarded paste result %q", marker),
			CleanupErr:    cleanupErr,
		}, nil
	}

	if !submit {
		return paneSendResult{Kind: paneSendDelivered, BufferCleaned: true}, nil
	}

	// Agent TUIs can swallow a submit key delivered in the same burst as a large
	// bracketed paste. Let the paste settle, then guard the first Enter against the
	// exact pane generation again before dispatching it.
	if err := tmuxSendSleep(sendCtx, tmuxSendSubmitSettleDelay); err != nil {
		return paneSendResult{
			Kind:          paneSendDelivered,
			BufferCleaned: true,
			Detail:        "submit key was not dispatched: " + err.Error(),
			OperationErr:  err,
		}, nil
	}
	pastedComposer := ""
	retryEligible := false
	if beforePasteCaptured {
		var pastedCaptured bool
		pastedComposer, pastedCaptured = h.captureSubmitComposer(sendCtx, target, pane, harness, evidence, true)
		retryEligible = pastedCaptured && pastedComposer != beforePasteComposer
	}
	output, err = h.runTmuxOnSocketContext(
		sendCtx,
		target.socket,
		"if-shell", "-F", "-t", pane.PaneID,
		atomicSendCondition(pane),
		atomicSubmitCommand(pane),
		"display-message -p "+atomicSendSubmitTargetChangedMarker,
	)
	if err != nil {
		return paneSendResult{Kind: paneSendUnknown, BufferCleaned: true, Detail: err.Error(), OperationErr: err}, nil
	}
	switch marker := strings.TrimSpace(output); marker {
	case atomicSendSubmitKeyMarker:
		if !retryEligible {
			return paneSendResult{Kind: paneSendDelivered, SubmitKeyDispatched: true, BufferCleaned: true}, nil
		}
		// A second Enter is eligible only when two bounded captures positively
		// identify the same non-empty pending prompt in a recognized idle composer.
		// The retry itself uses the same immutable server-side generation guard.
		if !h.shouldRetrySubmit(sendCtx, target, pane, harness, evidence, pastedComposer) {
			return paneSendResult{Kind: paneSendDelivered, SubmitKeyDispatched: true, BufferCleaned: true}, nil
		}
		output, err = h.runTmuxOnSocketContext(
			sendCtx,
			target.socket,
			"if-shell", "-F", "-t", pane.PaneID,
			atomicRetrySendCondition(pane, harness),
			atomicSubmitCommand(pane),
			"display-message -p "+atomicSendSubmitTargetChangedMarker,
		)
		if err != nil {
			return paneSendResult{
				Kind:                paneSendDelivered,
				SubmitKeyDispatched: true,
				BufferCleaned:       true,
				Detail:              "optional submit retry was not confirmed: " + err.Error(),
				OperationErr:        err,
			}, nil
		}
		switch retryMarker := strings.TrimSpace(output); retryMarker {
		case atomicSendSubmitKeyMarker:
			return paneSendResult{Kind: paneSendDelivered, SubmitKeyDispatched: true, BufferCleaned: true}, nil
		case atomicSendSubmitTargetChangedMarker:
			return paneSendResult{
				Kind:                paneSendDelivered,
				SubmitKeyDispatched: true,
				BufferCleaned:       true,
				Detail:              "target changed before optional submit retry; retry was suppressed",
			}, nil
		default:
			return paneSendResult{
				Kind:                paneSendDelivered,
				SubmitKeyDispatched: true,
				BufferCleaned:       true,
				Detail:              fmt.Sprintf("unexpected optional submit retry result %q", retryMarker),
			}, nil
		}
	case atomicSendSubmitTargetChangedMarker:
		return paneSendResult{
			Kind:          paneSendDelivered,
			BufferCleaned: true,
			Detail:        "target changed after paste; submit key was not dispatched",
		}, nil
	default:
		return paneSendResult{
			Kind:          paneSendUnknown,
			BufferCleaned: true,
			Detail:        fmt.Sprintf("unexpected guarded submit result %q", marker),
		}, nil
	}
}

// resolveActiveSendPane picks the pane an unattended sender should use: the
// session's only pane, or its active pane. Interactive sends pin an exact pane
// instead; a scheduled task has no operator to disambiguate at fire time.
func (h *TmuxHandler) resolveActiveSendPane(ctx context.Context, target tmuxTarget, sessionName string) (sendPaneTarget, error) {
	panes, err := h.listSendPanes(ctx, target, sessionName)
	if err != nil {
		return sendPaneTarget{}, err
	}
	if len(panes) == 1 {
		return panes[0], nil
	}
	for _, pane := range panes {
		if pane.Active {
			return pane, nil
		}
	}
	return sendPaneTarget{}, &sendTargetError{
		Status:  http.StatusConflict,
		Code:    "PANE_REQUIRED",
		Message: fmt.Sprintf("tmux session %q has %d panes and no active pane", sessionName, len(panes)),
	}
}

// ListSessionPanes handles GET /api/tmux/sessions/{name}/panes and exposes
// immutable pane IDs plus human-readable labels for an explicit safe choice.
func (h *TmuxHandler) ListSessionPanes(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	target, err := sendTargetFromRequest(h, r, "")
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	panes, err := h.listSendPanes(r.Context(), target, sessionName)
	if err != nil {
		var targetErr *sendTargetError
		if errors.As(err, &targetErr) {
			core.WriteError(w, targetErr.Status, targetErr.Code, targetErr.Message)
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"session":  sessionName,
		"unixUser": target.unixUser,
		"panes":    panes,
	})
}

// SendToSession handles POST /api/tmux/sessions/{name}/send. It pins exact
// immutable pane identity and pastes through an atomic guarded tmux queue.
func (h *TmuxHandler) SendToSession(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	if err := parseSessionDropForm(w, r); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	target, targetErr := sendTargetFromRequest(h, r, sessionDropFormValue(r, "unixUser"))
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}

	pane, err := h.resolveSendPane(r.Context(), target, sessionName, sessionDropFormValue(r, "pane"))
	if err != nil {
		var targetFailure *sendTargetError
		if errors.As(err, &targetFailure) {
			core.WriteError(w, targetFailure.Status, targetFailure.Code, targetFailure.Message)
		} else {
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		}
		return
	}
	requestedPane := strings.TrimSpace(sessionDropFormValue(r, "pane"))
	if requestedPane != "" {
		expected := sendPaneTarget{
			SessionID: strings.TrimSpace(sessionDropFormValue(r, "sessionId")),
			Session:   sessionName,
			PaneID:    requestedPane,
			PanePID:   strings.TrimSpace(sessionDropFormValue(r, "panePid")),
			ServerPID: strings.TrimSpace(sessionDropFormValue(r, "serverPid")),
		}
		if !tmuxSessionIDPattern.MatchString(expected.SessionID) ||
			!tmuxPIDPattern.MatchString(expected.PanePID) ||
			!tmuxPIDPattern.MatchString(expected.ServerPID) {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "an explicit pane requires its sessionId, panePid, and serverPid generation tuple")
			return
		}
		if !sameSendPaneGeneration(expected, pane) {
			core.WriteError(w, http.StatusConflict, "TARGET_CHANGED", "the selected tmux pane generation changed; refresh the chooser before retrying")
			return
		}
	} else if strings.TrimSpace(sessionDropFormValue(r, "sessionId")) != "" || strings.TrimSpace(sessionDropFormValue(r, "panePid")) != "" || strings.TrimSpace(sessionDropFormValue(r, "serverPid")) != "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "pane generation fields require an explicit pane")
		return
	}

	if err := h.lockSessionDrops(r.Context()); err != nil {
		core.WriteError(w, http.StatusRequestTimeout, "REQUEST_CANCELLED", "send request was cancelled before persistence")
		return
	}
	defer h.unlockSessionDrops()
	manifest, err := writeSessionDrop(r, sessionName, target, pane)
	if err != nil {
		if errors.Is(err, errEmptySessionDrop) {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "SESSION_DROP_ERROR", err.Error())
		return
	}
	dropPath := filepath.Dir(manifest.Payload)
	retainDrop := false
	defer func() {
		if !retainDrop {
			_ = os.RemoveAll(dropPath)
		}
	}()

	bufferName := "chrote-send-" + manifest.ID
	submissionRequested := submitFormValue(sessionDropFormValue(r, "submit"))
	writeUnknownOutcome := func(detail string, bufferCleaned bool, cleanupErr error) {
		retainDrop = true
		warning := "tmux did not confirm whether delivery occurred; inspect the exact pane before retrying"
		if strings.TrimSpace(detail) != "" {
			warning += ": " + strings.TrimSpace(detail)
		}
		if cleanupErr != nil {
			warning += fmt.Sprintf("; buffer cleanup could not be confirmed: %v", cleanupErr)
		}
		core.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"success":             false,
			"transport":           "unknown",
			"retryable":           false,
			"deliveryConfirmed":   false,
			"submissionRequested": submissionRequested,
			"submitKeyDispatched": false,
			"bufferCleaned":       bufferCleaned,
			"targetVerified":      false,
			"warning":             warning,
			"session":             sessionName,
			"sessionId":           pane.SessionID,
			"pane":                pane.PaneID,
			"panePid":             pane.PanePID,
			"serverPid":           pane.ServerPID,
			"unixUser":            target.unixUser,
			"dropId":              manifest.ID,
			"dropPath":            dropPath,
			"payload":             manifest.Payload,
			"files":               manifest.Files,
			"timestamp":           time.Now().UTC().Format(time.RFC3339),
		})
	}
	result, err := h.sendBufferToPane(r.Context(), context.Background(), 0, target, pane, bufferName, manifest.Payload, submissionRequested, true, manifest.submitEvidence)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}
	switch result.Kind {
	case paneSendUnknown:
		writeUnknownOutcome(result.Detail, result.BufferCleaned, result.CleanupErr)
		return
	case paneSendTargetChanged:
		if result.CleanupErr != nil {
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", fmt.Sprintf("target changed and buffer cleanup failed: %v", result.CleanupErr))
			return
		}
		core.WriteError(w, http.StatusConflict, "TARGET_CHANGED", "tmux session or pane changed while preparing the send; inspect and retry")
		return
	}
	retainDrop = true
	warnings := []string{}
	verifiedPane, verifyErr := h.resolveSendPane(r.Context(), target, sessionName, pane.PaneID)
	targetVerified := verifyErr == nil && sameSendPaneGeneration(pane, verifiedPane)
	if strings.TrimSpace(result.Detail) != "" {
		warnings = append(warnings, result.Detail)
	}
	if !targetVerified {
		warnings = append(warnings, "target changed before post-send verification")
	}
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":             true,
		"transport":           "pasted",
		"submissionRequested": submissionRequested,
		"submitKeyDispatched": result.SubmitKeyDispatched,
		"bufferCleaned":       true,
		"targetVerified":      targetVerified,
		"warning":             strings.Join(warnings, "; "),
		"session":             sessionName,
		"sessionId":           pane.SessionID,
		"pane":                pane.PaneID,
		"panePid":             pane.PanePID,
		"serverPid":           pane.ServerPID,
		"unixUser":            target.unixUser,
		"dropId":              manifest.ID,
		"dropPath":            dropPath,
		"payload":             manifest.Payload,
		"files":               manifest.Files,
		"timestamp":           time.Now().UTC().Format(time.RFC3339),
	})
}
