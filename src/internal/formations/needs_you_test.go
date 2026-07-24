package formations

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeNeedsYouNotifier records every notification it is asked to send and can be
// primed to fail, so tests can assert dedup/resolve behaviour without any network.
type fakeNeedsYouNotifier struct {
	mu   sync.Mutex
	sent []NeedsYouNotification
	fail error
}

func (f *fakeNeedsYouNotifier) NotifyNeedsYou(_ context.Context, n NeedsYouNotification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, n)
	return nil
}

func (f *fakeNeedsYouNotifier) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

func humanRequestEvent(seq int, gateID, prompt string) RunEvent {
	return RunEvent{
		Seq:    seq,
		Type:   RunEventHumanInputRequested,
		GateID: gateID,
		NodeID: gateID,
		Data:   map[string]any{"prompt": prompt, "gateId": gateID, "nodeId": gateID},
	}
}

func escalationEvent(seq int, nodeID, reason, severity string, blocks bool) RunEvent {
	return RunEvent{
		Seq:    seq,
		Type:   RunEventEscalationRaised,
		NodeID: nodeID,
		Data:   map[string]any{"reason": reason, "severity": severity, "blocks": blocks},
	}
}

func TestNeedsYouProjectionOpensAndResolvesHumanGate(t *testing.T) {
	open := projectOpenNeedsYouAsks([]RunEvent{
		{Seq: 1, Type: RunEventStarted},
		humanRequestEvent(2, "gate_review", "Good enough to ship"),
	})
	if len(open) != 1 {
		t.Fatalf("open asks = %#v, want one human gate ask", open)
	}
	ask := open[0]
	if ask.Kind != "human_gate" || ask.Seq != 2 || ask.GateID != "gate_review" || ask.Ask != "Good enough to ship" {
		t.Fatalf("human gate ask = %+v, want gate_review verdict ask at seq 2", ask)
	}

	resolved := projectOpenNeedsYouAsks([]RunEvent{
		{Seq: 1, Type: RunEventStarted},
		humanRequestEvent(2, "gate_review", "Good enough to ship"),
		{Seq: 3, Type: RunEventHumanVerdictRecorded, GateID: "gate_review", NodeID: "gate_review"},
	})
	if len(resolved) != 0 {
		t.Fatalf("open asks after verdict = %#v, want none", resolved)
	}
}

func TestNeedsYouProjectionOpensAndResolvesBlockingEscalation(t *testing.T) {
	open := projectOpenNeedsYouAsks([]RunEvent{
		{Seq: 1, Type: RunEventStarted},
		escalationEvent(2, "fmn_work", "cannot proceed without a call", "stop", true),
		{Seq: 3, Type: RunEventBlocked, NodeID: "fmn_work"},
	})
	if len(open) != 1 {
		t.Fatalf("open asks = %#v, want one escalation ask", open)
	}
	ask := open[0]
	if ask.Kind != "escalation" || ask.Seq != 2 || ask.NodeID != "fmn_work" || !ask.Blocks || ask.Ask != "cannot proceed without a call" {
		t.Fatalf("escalation ask = %+v, want blocking escalation at seq 2", ask)
	}

	resumed := projectOpenNeedsYouAsks([]RunEvent{
		{Seq: 1, Type: RunEventStarted},
		escalationEvent(2, "fmn_work", "cannot proceed without a call", "stop", true),
		{Seq: 3, Type: RunEventBlocked, NodeID: "fmn_work"},
		{Seq: 4, Type: RunEventResumed},
	})
	if len(resumed) != 0 {
		t.Fatalf("open asks after resume = %#v, want none", resumed)
	}
}

func TestNeedsYouProjectionIgnoresNonBlockingEscalation(t *testing.T) {
	open := projectOpenNeedsYouAsks([]RunEvent{
		{Seq: 1, Type: RunEventStarted},
		escalationEvent(2, "fmn_work", "found a better direction", "needs-attention", false),
	})
	if len(open) != 0 {
		t.Fatalf("open asks = %#v, want none for a non-blocking escalation", open)
	}
}

func TestNeedsYouProjectionTerminalRunHasNoAsks(t *testing.T) {
	open := projectOpenNeedsYouAsks([]RunEvent{
		{Seq: 1, Type: RunEventStarted},
		humanRequestEvent(2, "gate_review", "Good enough to ship"),
		{Seq: 3, Type: RunEventFailed},
	})
	if len(open) != 0 {
		t.Fatalf("open asks for terminal run = %#v, want none", open)
	}
}

func TestNeedsYouReconcileNotifiesOnceAndDedups(t *testing.T) {
	store, started := startS4DispatchRun(t)
	if _, err := store.RecordEscalationFromCapture(started.RunID, "fmn_work",
		"<<<CHROTE-ESCALATE run-id="+started.RunID+" severity=stop reason='need a decision'>>>"); err != nil {
		t.Fatalf("record blocking escalation: %v", err)
	}

	notifier := &fakeNeedsYouNotifier{}
	engine := NewRunEngine(store, nil, NewUnavailableFormationExecutor("test"))
	engine.SetNeedsYouNotifier(notifier, "https://board.example")

	engine.reconcileNeedsYou(started.RunID)
	if notifier.count() != 1 {
		t.Fatalf("notifications after first reconcile = %d, want 1", notifier.count())
	}
	got := notifier.sent[0]
	if got.RunID != started.RunID || got.Kind != "escalation" || !got.Blocks || got.Ask != "need a decision" {
		t.Fatalf("notification = %+v, want blocking escalation ask for the run", got)
	}
	if got.BoardURL == "" {
		t.Fatalf("notification board url empty, want a pointer back to the board")
	}

	// Same engine, second pass: no re-notify.
	engine.reconcileNeedsYou(started.RunID)
	if notifier.count() != 1 {
		t.Fatalf("notifications after second reconcile = %d, want 1 (dedup)", notifier.count())
	}

	// Fresh engine over the same store: durable dedup must survive.
	fresh := &fakeNeedsYouNotifier{}
	engine2 := NewRunEngine(store, nil, NewUnavailableFormationExecutor("test"))
	engine2.SetNeedsYouNotifier(fresh, "https://board.example")
	engine2.reconcileNeedsYou(started.RunID)
	if fresh.count() != 0 {
		t.Fatalf("fresh-engine notifications = %d, want 0 (durable dedup)", fresh.count())
	}
}

func TestNeedsYouReconcileNilNotifierIsNoOp(t *testing.T) {
	store, started := startS4DispatchRun(t)
	if _, err := store.RecordEscalationFromCapture(started.RunID, "fmn_work",
		"<<<CHROTE-ESCALATE run-id="+started.RunID+" severity=stop reason='need a decision'>>>"); err != nil {
		t.Fatalf("record blocking escalation: %v", err)
	}

	// No notifier set: reconcile must do nothing and must not mark the ask notified.
	engine := NewRunEngine(store, nil, NewUnavailableFormationExecutor("test"))
	engine.reconcileNeedsYou(started.RunID)

	// Now opt in: the still-open ask must fire exactly once.
	notifier := &fakeNeedsYouNotifier{}
	engine.SetNeedsYouNotifier(notifier, "")
	engine.reconcileNeedsYou(started.RunID)
	if notifier.count() != 1 {
		t.Fatalf("notifications after opting in = %d, want 1", notifier.count())
	}
}

func TestNeedsYouReconcileRetriesAfterSendFailure(t *testing.T) {
	store, started := startS4DispatchRun(t)
	if _, err := store.RecordEscalationFromCapture(started.RunID, "fmn_work",
		"<<<CHROTE-ESCALATE run-id="+started.RunID+" severity=stop reason='need a decision'>>>"); err != nil {
		t.Fatalf("record blocking escalation: %v", err)
	}

	notifier := &fakeNeedsYouNotifier{fail: errors.New("channel down")}
	engine := NewRunEngine(store, nil, NewUnavailableFormationExecutor("test"))
	engine.SetNeedsYouNotifier(notifier, "")

	engine.reconcileNeedsYou(started.RunID) // fails, must not record the seq
	notifier.mu.Lock()
	notifier.fail = nil
	notifier.mu.Unlock()

	engine.reconcileNeedsYou(started.RunID) // retry now succeeds
	if notifier.count() != 1 {
		t.Fatalf("notifications after retry = %d, want 1", notifier.count())
	}
}

func TestNeedsYouRunMissionAutoNotifiesHumanGate(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s5HumanGateBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	notifier := &fakeNeedsYouNotifier{}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"pass"}})
	engine.SetNeedsYouNotifier(notifier, "")

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Final {
		t.Fatalf("status = %+v, want non-final while awaiting a human verdict", status)
	}
	if notifier.count() != 1 {
		t.Fatalf("notifications = %d, want 1 auto-push for the waiting human gate", notifier.count())
	}
	if got := notifier.sent[0]; got.Kind != "human_gate" || got.GateID != "gate_review" {
		t.Fatalf("notification = %+v, want a human_gate ask for gate_review", got)
	}
}

func TestWebhookNeedsYouNotifierPostsJSON(t *testing.T) {
	type captured struct {
		method      string
		contentType string
		body        NeedsYouNotification
	}
	got := make(chan captured, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body NeedsYouNotification
		_ = json.Unmarshal(raw, &body)
		got <- captured{method: r.Method, contentType: r.Header.Get("Content-Type"), body: body}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewWebhookNeedsYouNotifier(server.URL)
	err := notifier.NotifyNeedsYou(context.Background(), NeedsYouNotification{
		RunID: "run_1", Kind: "escalation", Ask: "need a decision", Blocks: true,
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	c := <-got
	if c.method != http.MethodPost || c.contentType != "application/json" {
		t.Fatalf("request = %s %s, want POST application/json", c.method, c.contentType)
	}
	if c.body.RunID != "run_1" || c.body.Ask != "need a decision" {
		t.Fatalf("posted body = %+v, want the notification fields", c.body)
	}
}

func TestWebhookNeedsYouNotifierNon2xxIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	notifier := NewWebhookNeedsYouNotifier(server.URL)
	if err := notifier.NotifyNeedsYou(context.Background(), NeedsYouNotification{RunID: "run_1"}); err == nil {
		t.Fatal("notify returned nil error for a 500 response, want an error")
	}
}
