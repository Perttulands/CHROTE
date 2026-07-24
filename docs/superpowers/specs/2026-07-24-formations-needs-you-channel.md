# Spec: Deliver needs-you asks to the owner's channel

- Date: 2026-07-24
- Bead: chrote-4m1 (epic chrote-z0q — "Make Formations a usable solo agent-workflow cockpit")
- Status: accepted (drives this branch)

## Problem

A Formations run can reach a point where it cannot make progress without a human
decision: a `human` gate is waiting for a pass/fail verdict, or an agent raised a
**blocking** escalation. Today that state is recorded durably in the run ledger and
surfaced on the board (the "Needs you" banner + node highlights). That is enough only
if the operator happens to be looking at the board. CHROTE's whole point for a solo
operator is unattended operation — the run should be able to *reach out* when it needs
a decision, not wait to be noticed.

This spec adds the **outbound push**: when a run enters a needs-you state, CHROTE sends
one notification to the owner's configured channel, pointing back to the board.

## Goal

When a run needs a human decision, push exactly one notification per ask to the owner's
channel, sourced from durable run state, resolve-aware, and off by default so a solo
operator who never configures a channel sees zero behaviour change.

## Non-goals

- No new inbound control surface (you still answer the ask on the board / via the
  existing verdict + resume APIs). The push is a pointer, not a remote control.
- No provider-specific integration baked into the server (see channel mechanism).
- No change to the on-board banner (already shipped) beyond reusing its state source.
- Non-blocking (`needs-attention`) escalations stay **board-only** by design — pushing
  every informational note would be the "stream" the requirements forbid. Only asks that
  actually block a decision are pushed. This scope is adjustable later without rework.

## Trigger conditions (which run states fire a push)

The set of open "needs-you asks" is a **pure projection of the run ledger** (never
optimism, never live in-memory guesses). An ask is one of:

1. **Human gate — a verdict needed.** A `human_input_requested` event for a gate whose
   latest human event is still `human_input_requested` (not yet cleared by a
   `human_verdict_recorded` for that gate). Ask text = the gate prompt. Mirrors the
   engine's existing `latestHumanRequest` open/close logic.
2. **Blocking escalation.** An `escalation_raised` event with `blocks == true` that has
   not been superseded by a later `run_resumed` (seq greater than the escalation's seq).
   Ask text = the escalation reason.

A run that has reached a terminal event (`run_succeeded` / `run_failed` /
`run_canceled`) has **no** open asks — finalization resolves everything.

Each ask carries the ledger event `Seq`, which is unique and monotonic per run and is
used as the dedup key.

## Channel mechanism + config

**Decision: a server-side generic outbound webhook, configured by an env var, off by
default.** The notifier is an interface; the shipped real implementation POSTs the
notification as JSON to a single owner-provided URL.

Config (follows the established `CHROTE_*_URL` convention already used for Context
Citadel and TTS, read in `src/cmd/server/main.go`):

- `CHROTE_NEEDS_YOU_WEBHOOK_URL` — outbound webhook URL. **Empty ⇒ feature off** (a nil
  notifier; reconcile is a no-op). The URL may embed a secret token; it is read from the
  environment, never hardcoded, and never logged.
- `CHROTE_NEEDS_YOU_BOARD_URL` — optional base URL used to build the "open the board"
  pointer (e.g. the Tailscale-served dashboard origin). Empty ⇒ the notification still
  carries the board slug + run id, just no absolute link.

Why a generic webhook and not a Telegram-native integration:

- **Simplest primitive that fits the trust model.** One URL env var + one HTTP POST with
  a small JSON body. No provider SDK, no chat-id bookkeeping, no provider-specific error
  handling in the server.
- **Matches CHROTE's existing config surface.** Owner-provided external endpoints are
  already env-configured URLs (`CHROTE_CONTEXT_API_URL`, `CHROTE_TTS_URL`); this is the
  same shape and the same `http.Client{Timeout}` outbound pattern (`services.go`).
- **Composes with the perimeter.** The owner can point it at Telegram (via a bot relay
  or ntfy's Telegram bridge), a Slack/Discord incoming webhook, ntfy.sh, or an endpoint
  inside their own Tailscale perimeter — without CHROTE choosing for them.
- **Pluggable later.** Because the sender is an interface, a Telegram-native sender is a
  drop-in addition, not a rewrite. Rejected *for now*: a Telegram-native sender (more
  code, provider-locked, an extra secret to manage) and OS/desktop notifications (do not
  reach a phone; awkward from a systemd service on WSL).

## Dedup & resolve semantics

- **Dedup key = `(runID, seq)`.** Each ask is the ledger event that opened it, so its seq
  is a stable, unique key. An ask is notified **at most once**.
- **Durable dedup, not in-memory.** A crash across a restart must not re-announce open
  asks. The set of already-notified seqs is persisted as a run-scoped sidecar artifact
  `<runID>.needs-you.json` in the run directory, written with the same crash-safe,
  authority-guarded run-artifact machinery the ledger already uses
  (`writeRunArtifactAtomicAt` / `readRunArtifactAt`). It is **not** a ledger event,
  because `AppendRunEvent` refuses to append while a run is blocked or final — exactly the
  states we notify in.
- **Resolve-aware.** Reconcile only notifies asks that are **open right now** (per the
  projection above). An ask that was answered (`human_verdict_recorded`), resumed past
  (`run_resumed`), or finalized is no longer projected as open, so it is never notified.
  The pushed message points back to the board, where current state is authoritative — we
  do not send a retraction.
- **Best-effort ordering.** Reconcile checks "already notified?", then sends, then records
  the seq **only on success**. A send failure leaves the seq unrecorded so the next
  genuine state transition retries it. Reconcile fires only at discrete state-change
  points (below), never on a timer, so a transient failure cannot produce a stream.

## Message contract

The notifier receives a `NeedsYouNotification`:

| field       | meaning                                                        |
|-------------|----------------------------------------------------------------|
| `runId`     | the run that needs a decision                                  |
| `boardSlug` | which board (from the run's first ledger event)                |
| `seq`       | dedup key / ledger event seq                                   |
| `kind`      | `human_gate` \| `escalation`                                   |
| `nodeId`    | the node/gate awaiting the decision                            |
| `gateId`    | gate id when kind is `human_gate`                              |
| `ask`       | the human-readable ask text (gate prompt or escalation reason) |
| `severity`  | escalation severity, or `verdict` for a human gate             |
| `blocks`    | always true for pushed asks                                    |
| `boardUrl`  | pointer back to the board (base URL + slug), when configured   |
| `text`      | a one-line human-readable summary for chat channels            |

The real webhook sender serializes this to JSON and POSTs it; a 2xx is success, any
other status (or transport error) is a send failure (retried on the next transition).

## Wiring

- `RunEngine` gains an optional `NeedsYouNotifier` (+ board base URL), set via
  `SetNeedsYouNotifier`, mirroring `SetGateEvaluator`. Nil ⇒ no-op.
- After each entrypoint that can leave a run in a needs-you state returns —
  `RunMission`, `RunFormation`, `ResumeRun`, `RecordHumanGateVerdict` — the engine calls
  `reconcileNeedsYou(runID)`. Reconcile is idempotent, so covering several call sites is
  safe. Notification failures are swallowed and never affect run outcome.
- `FormationsHandler` holds the notifier + board URL and injects them in `newRunEngine`.
- `main.go` reads the two env vars; if the webhook URL is set it builds a
  `WebhookNeedsYouNotifier` and hands it to the handler, otherwise leaves it nil.

## Unconfigured no-op behaviour

With `CHROTE_NEEDS_YOU_WEBHOOK_URL` unset (the default), the handler injects a nil
notifier, `reconcileNeedsYou` returns immediately, no sidecar file is written, and no
outbound request is ever made. Zero friction, zero new I/O for a solo operator who has
not opted in.

## Test plan (RED first)

Pure projection (`[]RunEvent` literals, no store):
- open human gate ⇒ one `human_gate` ask; after `human_verdict_recorded` ⇒ none.
- blocking escalation ⇒ one `escalation` ask; after `run_resumed` ⇒ none.
- non-blocking escalation ⇒ no ask.
- terminal run (succeeded/failed/canceled) ⇒ no asks even with an earlier open request.

Reconcile + dedup (real store + run ledger + fake notifier):
- one open ask ⇒ exactly one `NotifyNeedsYou`; a second reconcile ⇒ no further call.
- dedup survives via the sidecar (a fresh engine over the same store does not re-notify).
- resolved ask ⇒ no notification.
- nil notifier ⇒ no call, no sidecar written.
- send failure ⇒ seq not recorded ⇒ next reconcile retries.

Webhook sender (`httptest` loopback only — never a real external channel):
- posts JSON with the expected fields; non-2xx response ⇒ error.
```
