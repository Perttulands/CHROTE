package formations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// This file implements the outbound "needs-you" push: when a Formations run
// reaches a state that needs a human decision, CHROTE notifies the owner's
// configured channel exactly once per ask, sourced from the durable run ledger.
//
// The board banner already surfaces these states to an operator who is looking;
// this reaches an operator who is not. See
// docs/superpowers/specs/2026-07-24-formations-needs-you-channel.md.

const (
	needsYouKindHumanGate  = "human_gate"
	needsYouKindEscalation = "escalation"

	needsYouNotifiedArtifactSuffix = ".needs-you.json"
)

// NeedsYouAsk is a single open decision the run cannot make without the owner,
// derived purely from the run ledger. Seq is the ledger event that opened the
// ask and is the stable dedup key.
type NeedsYouAsk struct {
	RunID    string
	Seq      int
	Kind     string // needsYouKindHumanGate | needsYouKindEscalation
	NodeID   string
	GateID   string
	Ask      string // gate prompt or escalation reason
	Severity string // escalation severity, or "verdict" for a human gate
	Blocks   bool
}

// NeedsYouNotification is the message contract handed to a notifier.
type NeedsYouNotification struct {
	RunID     string `json:"runId"`
	BoardSlug string `json:"boardSlug,omitempty"`
	Seq       int    `json:"seq"`
	Kind      string `json:"kind"`
	NodeID    string `json:"nodeId,omitempty"`
	GateID    string `json:"gateId,omitempty"`
	Ask       string `json:"ask"`
	Severity  string `json:"severity,omitempty"`
	Blocks    bool   `json:"blocks"`
	BoardURL  string `json:"boardUrl,omitempty"`
	Text      string `json:"text"`
}

// NeedsYouNotifier delivers a needs-you ask to the owner's channel. The real
// implementation is config-gated; tests inject a fake. A nil notifier means the
// feature is off.
type NeedsYouNotifier interface {
	NotifyNeedsYou(ctx context.Context, n NeedsYouNotification) error
}

// projectOpenNeedsYouAsks derives the set of asks that currently need the owner
// from the run ledger. It never guesses from live state: an ask is open only
// while the ledger says so.
//
//   - A human gate (a verdict needed) is open from human_input_requested until a
//     human_verdict_recorded for that gate clears it.
//   - A blocking escalation is open until a later run_resumed supersedes it.
//   - A run that reached a terminal event has no open asks.
func projectOpenNeedsYouAsks(events []RunEvent) []NeedsYouAsk {
	for _, event := range events {
		switch event.Type {
		case RunEventSucceeded, RunEventFailed, RunEventCanceled:
			return nil
		}
	}

	lastResumeSeq := 0
	for _, event := range events {
		if event.Type == RunEventResumed && event.Seq > lastResumeSeq {
			lastResumeSeq = event.Seq
		}
	}

	asks := make([]NeedsYouAsk, 0)

	// Human gates: latest request per gate, cleared by a recorded verdict.
	openHuman := map[string]NeedsYouAsk{}
	for _, event := range events {
		switch event.Type {
		case RunEventHumanInputRequested:
			openHuman[event.GateID] = NeedsYouAsk{
				RunID:    event.RunID,
				Seq:      event.Seq,
				Kind:     needsYouKindHumanGate,
				GateID:   event.GateID,
				NodeID:   needsYouNodeID(event),
				Ask:      stringFromEventData(event, "prompt"),
				Severity: "verdict",
				Blocks:   true,
			}
		case RunEventHumanVerdictRecorded:
			delete(openHuman, event.GateID)
		}
	}
	for _, ask := range openHuman {
		asks = append(asks, ask)
	}

	// Blocking escalations not yet superseded by a resume.
	for _, event := range events {
		if event.Type != RunEventEscalationRaised || !boolFromEventData(event, "blocks") {
			continue
		}
		if event.Seq <= lastResumeSeq {
			continue
		}
		asks = append(asks, NeedsYouAsk{
			RunID:    event.RunID,
			Seq:      event.Seq,
			Kind:     needsYouKindEscalation,
			GateID:   event.GateID,
			NodeID:   needsYouNodeID(event),
			Ask:      stringFromEventData(event, "reason"),
			Severity: stringFromEventData(event, "severity"),
			Blocks:   true,
		})
	}

	sort.Slice(asks, func(i, j int) bool { return asks[i].Seq < asks[j].Seq })
	return asks
}

func needsYouNodeID(event RunEvent) string {
	if event.NodeID != "" {
		return event.NodeID
	}
	return event.GateID
}

func buildNeedsYouNotification(ask NeedsYouAsk, boardSlug, boardBaseURL string) NeedsYouNotification {
	n := NeedsYouNotification{
		RunID:     ask.RunID,
		BoardSlug: boardSlug,
		Seq:       ask.Seq,
		Kind:      ask.Kind,
		NodeID:    ask.NodeID,
		GateID:    ask.GateID,
		Ask:       ask.Ask,
		Severity:  ask.Severity,
		Blocks:    ask.Blocks,
		BoardURL:  needsYouBoardPointer(boardBaseURL, boardSlug),
	}
	n.Text = needsYouText(n)
	return n
}

func needsYouBoardPointer(base, slug string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	base = strings.TrimRight(base, "/")
	if slug == "" {
		return base
	}
	return base + "/?board=" + url.QueryEscape(slug)
}

func needsYouText(n NeedsYouNotification) string {
	where := n.GateID
	if where == "" {
		where = n.NodeID
	}
	if where == "" {
		where = "run"
	}
	ask := strings.TrimSpace(n.Ask)
	if ask == "" {
		ask = "a decision is needed"
	}
	var b strings.Builder
	b.WriteString("Formations needs you: ")
	b.WriteString(ask)
	b.WriteString(" — run ")
	b.WriteString(n.RunID)
	if n.BoardSlug != "" {
		b.WriteString(" · board ")
		b.WriteString(n.BoardSlug)
	}
	b.WriteString(" · ")
	b.WriteString(where)
	if n.BoardURL != "" {
		b.WriteString(" — ")
		b.WriteString(n.BoardURL)
	}
	return b.String()
}

// WebhookNeedsYouNotifier POSTs the notification as JSON to a single
// owner-provided URL. The URL may embed a secret; it is never logged.
type WebhookNeedsYouNotifier struct {
	url    string
	client *http.Client
}

// NewWebhookNeedsYouNotifier builds a webhook notifier for the given URL.
func NewWebhookNeedsYouNotifier(webhookURL string) *WebhookNeedsYouNotifier {
	return &WebhookNeedsYouNotifier{
		url:    webhookURL,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *WebhookNeedsYouNotifier) NotifyNeedsYou(ctx context.Context, n NeedsYouNotification) error {
	if w == nil || strings.TrimSpace(w.url) == "" {
		return nil
	}
	body, err := json.Marshal(n)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		// Do not surface the URL (may embed a secret).
		return errors.New("needs-you webhook request failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("needs-you webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// needsYouNotifiedRecord is the durable per-run dedup ledger: the seqs already
// pushed to the owner's channel.
type needsYouNotifiedRecord struct {
	Notified []int `json:"notified"`
}

func needsYouNotifiedArtifactName(runID string) string {
	return runID + needsYouNotifiedArtifactSuffix
}

// NeedsYouNotifiedSeqs returns the set of ask seqs already delivered for a run.
func (s *Store) NeedsYouNotifiedSeqs(runID string) (map[int]bool, error) {
	handle, err := s.openRunLedger(runID, false)
	if err != nil {
		return nil, err
	}
	defer handle.close()
	return readNeedsYouNotified(handle.directory, runID)
}

func readNeedsYouNotified(directory *runArtifactDirectory, runID string) (map[int]bool, error) {
	raw, err := readRunArtifactAt(directory, needsYouNotifiedArtifactName(runID), runtimeAuthorityMaxRecordBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[int]bool{}, nil
		}
		return nil, err
	}
	var record needsYouNotifiedRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	seqs := make(map[int]bool, len(record.Notified))
	for _, seq := range record.Notified {
		seqs[seq] = true
	}
	return seqs, nil
}

// MarkNeedsYouNotified durably records that the ask at seq has been delivered,
// so a restart never re-announces it. It is idempotent.
func (s *Store) MarkNeedsYouNotified(runID string, seq int) error {
	if err := s.RequireRuntimeAuthority(); err != nil {
		return err
	}
	handle, err := s.openRunLedger(runID, false)
	if err != nil {
		return err
	}
	defer handle.close()

	existing, err := readNeedsYouNotified(handle.directory, runID)
	if err != nil {
		return err
	}
	if existing[seq] {
		return nil
	}
	existing[seq] = true

	seqs := make([]int, 0, len(existing))
	for value := range existing {
		seqs = append(seqs, value)
	}
	sort.Ints(seqs)
	out, err := json.Marshal(needsYouNotifiedRecord{Notified: seqs})
	if err != nil {
		return err
	}
	return writeRunArtifactAtomicAt(handle.directory, needsYouNotifiedArtifactName(runID), out, int(runtimeAuthorityMaxRecordBytes))
}
