package formations

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrDispatchDeadPane = errors.New("formations dispatch dead pane")
	ErrDispatchTimeout  = errors.New("formations dispatch timeout")
)

type DispatchAdapter interface {
	SendSlotDispatch(SlotDispatchPayload) error
}

type SlotDispatcher struct {
	store   *Store
	adapter DispatchAdapter
}

type SlotDispatchRequest struct {
	NodeID      string
	SlotID      string
	AgentID     string
	Harness     string
	SessionStem string
	SessionRef  string
	Prompt      string
	Attempt     int
}

type SlotDispatchPayload struct {
	RunID      string
	DispatchID string
	NodeID     string
	SlotID     string
	SessionRef string
	Prompt     string
}

type SlotDispatchLease struct {
	RunID      string
	DispatchID string
	NodeID     string
	SlotID     string
}

type CompletionSentinel struct {
	RunID    string
	Status   string
	Artifact string
}

func NewSlotDispatcher(store *Store, adapter DispatchAdapter) *SlotDispatcher {
	return &SlotDispatcher{store: store, adapter: adapter}
}

func (d *SlotDispatcher) DispatchSlot(runID string, req SlotDispatchRequest) (SlotDispatchLease, error) {
	if d == nil || d.store == nil {
		return SlotDispatchLease{}, fmt.Errorf("%w: dispatch store required", ErrNotFound)
	}
	dispatchID := newDispatchID(runID, req.NodeID, req.SlotID, req.Attempt)
	lease := SlotDispatchLease{
		RunID:      runID,
		DispatchID: dispatchID,
		NodeID:     req.NodeID,
		SlotID:     req.SlotID,
	}
	if err := d.store.AppendRunEvent(runID, RunEvent{
		Type:    RunEventSlotDispatch,
		NodeID:  req.NodeID,
		SlotID:  req.SlotID,
		Attempt: req.Attempt,
		Data: map[string]any{
			"dispatchId":         dispatchID,
			"nodeId":             req.NodeID,
			"slotId":             req.SlotID,
			"agentId":            req.AgentID,
			"harness":            req.Harness,
			"sessionStem":        req.SessionStem,
			"sessionRef":         req.SessionRef,
			"promptSha256":       etag([]byte(req.Prompt)),
			"promptRef":          "",
			"nativeAck":          false,
			"recordedBeforeSend": true,
		},
	}); err != nil {
		return lease, err
	}
	payload := SlotDispatchPayload{
		RunID:      runID,
		DispatchID: dispatchID,
		NodeID:     req.NodeID,
		SlotID:     req.SlotID,
		SessionRef: req.SessionRef,
		Prompt:     req.Prompt,
	}
	if d.adapter == nil {
		return lease, nil
	}
	if err := d.adapter.SendSlotDispatch(payload); err != nil {
		if blockErr := d.appendDispatchErrorAndBlock(runID, dispatchID, req.NodeID, req.SlotID, dispatchErrorCode(err), err.Error(), "tmux"); blockErr != nil {
			return lease, blockErr
		}
		return lease, err
	}
	return lease, nil
}

func (d *SlotDispatcher) CompleteFromCapture(runID, dispatchID, captured string) error {
	sentinel, ok := ParseCompletionSentinel(captured, runID)
	if !ok {
		if err := d.appendDispatchErrorAndBlock(runID, dispatchID, "", "", "completion_sentinel_timeout", "completion sentinel timeout", "adapter"); err != nil {
			return err
		}
		return ErrDispatchTimeout
	}
	dispatch := d.dispatchEvent(runID, dispatchID)
	if dispatch.Type == "" {
		message := fmt.Sprintf("unknown dispatch %q", dispatchID)
		if err := d.appendDispatchErrorAndBlock(runID, dispatchID, "", "", "unknown_dispatch", message, "adapter"); err != nil {
			return err
		}
		return errors.New(message)
	}
	return d.store.AppendRunEvent(runID, RunEvent{
		Type:    RunEventSlotResult,
		NodeID:  dispatch.NodeID,
		SlotID:  dispatch.SlotID,
		Attempt: dispatch.Attempt,
		Data: map[string]any{
			"dispatchId": dispatchID,
			"nodeId":     dispatch.NodeID,
			"slotId":     dispatch.SlotID,
			"status":     sentinel.Status,
			"sentinel": map[string]any{
				"runId":    sentinel.RunID,
				"status":   sentinel.Status,
				"artifact": sentinel.Artifact,
			},
		},
	})
}

func (d *SlotDispatcher) dispatchEvent(runID, dispatchID string) RunEvent {
	if d == nil || d.store == nil || dispatchID == "" {
		return RunEvent{}
	}
	events, err := d.store.ReadRunEvents(runID)
	if err != nil {
		return RunEvent{}
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type != RunEventSlotDispatch || event.Data == nil {
			continue
		}
		if stringFromAny(event.Data["dispatchId"]) == dispatchID {
			return event
		}
	}
	return RunEvent{}
}

func ParseCompletionSentinel(captured, runID string) (CompletionSentinel, bool) {
	remaining := captured
	for {
		start := strings.Index(remaining, "<<<CHROTE-DONE ")
		if start == -1 {
			return CompletionSentinel{}, false
		}
		remaining = remaining[start+len("<<<CHROTE-DONE "):]
		end := strings.Index(remaining, ">>>")
		if end == -1 {
			return CompletionSentinel{}, false
		}
		fields := parseSentinelFields(remaining[:end])
		remaining = remaining[end+len(">>>"):]
		if fields["run-id"] != runID {
			continue
		}
		return CompletionSentinel{
			RunID:    fields["run-id"],
			Status:   fields["status"],
			Artifact: fields["artifact"],
		}, true
	}
}

func parseSentinelFields(raw string) map[string]string {
	fields := map[string]string{}
	for _, part := range strings.Fields(raw) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		fields[key] = strings.Trim(value, `"`)
	}
	return fields
}

func (d *SlotDispatcher) appendDispatchErrorAndBlock(runID, dispatchID, nodeID, slotID, code, message, boundary string) error {
	if err := d.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventError,
		NodeID: nodeID,
		SlotID: slotID,
		Data: map[string]any{
			"code":        code,
			"message":     message,
			"boundary":    boundary,
			"nodeId":      nodeID,
			"slotId":      slotID,
			"recoverable": true,
			"dispatchId":  dispatchID,
		},
	}); err != nil {
		return err
	}
	return d.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventBlocked,
		NodeID: nodeID,
		SlotID: slotID,
		Data: map[string]any{
			"reason":        message,
			"blockedNodeId": nodeID,
			"resumeAllowed": true,
			"resumePolicy":  "explicit",
			"openDispatches": []map[string]any{{
				"dispatchId":  dispatchID,
				"nodeId":      nodeID,
				"slotId":      slotID,
				"dispatchSeq": 0,
			}},
			"nextEpoch": 1,
		},
	})
}

func dispatchErrorCode(err error) string {
	if errors.Is(err, ErrDispatchDeadPane) {
		return "dead_pane"
	}
	return "dispatch_failed"
}

func newDispatchID(runID, nodeID, slotID string, attempt int) string {
	return fmt.Sprintf("dsp_%s_%s_%s_%d_%s", runID, nodeID, slotID, attempt, randomCrockford(8))
}
