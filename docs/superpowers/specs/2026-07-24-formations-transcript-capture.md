# Formations: native-transcript completion capture (chrote-e4y)

Status: RED → GREEN (SDD)
Owner bead: chrote-e4y
Depends-on: chrote-jkk (executor-as-agent-user cutover, done)

## Problem

The tmux Formations executor drives an interactive coding-agent TUI
(`claude-code`) inside a detached tmux session. It submits the prompt with
`send-keys`, then detects completion by **scraping the live pane**
(`capture-pane -p -J -S -2000`) for a `<<<CHROTE-DONE run-id=... status=ok ...>>>`
sentinel and a ```chrote-outputs``` fenced JSON block.

Pane-scraping is the wrong source. The TUI constantly redraws (spinners,
soft-wrap, box glyphs, "Working (Ns)" status), so the captured text is noisy and
the sentinel/JSON is frequently mangled or exceeds the output cap even when the
agent finished perfectly.

Proof: for the smoke run `run_01KYACKJ...`, the agent's **native transcript**
contains the exact, clean completion — the ```chrote-outputs``` block and
`<<<CHROTE-DONE run-id=run_01KYACKJ... status=ok artifact=none>>>` with the
matching run-id. The agent did the task correctly; the executor just read the
wrong source.

## Goal

Detect completion and extract the agent's output by reading the harness's
**native transcript file**, not the pane. The live tmux session stays fully
interactive, steerable, and peekable — this is NOT `-p`/print mode. The
transcript is a read-only side-channel; the session is never touched by the
capture path.

Non-goals: changing the submit path's fundamentals (it already pastes via
`load-buffer`/`paste-buffer` then sends `ENTER`+`C-m`), changing the codex
harness path, or perfect concurrency for many same-run same-cwd dispatches
(documented follow-up).

## Ground truth (verified 2026-07-24)

- `claude-code` writes one JSONL transcript per session at
  `<agent-home>/.claude/projects/<encoded-cwd>/<session-uuid>.jsonl`.
- The transcript **filename is the session uuid** (globally unique).
- Each transcript record carries a `cwd` field (the agent's working directory)
  and a `sessionId` field — so a candidate file can be *verified* by reading its
  `cwd` rather than trusting a path-encoding guess.
- Assistant turns appear as records with `type: "assistant"`; the emitted text
  is in `message.content` (a string, or an array of blocks each with `text`).
  The final assistant turn's text contains the ```chrote-outputs``` block and
  the `<<<CHROTE-DONE ...>>>` sentinel.
- `claude --session-id <uuid>` is supported (deterministic correlation is
  possible as a future hardening; see Follow-ups). This spec does NOT require
  launch-command changes.

## Design

Introduce a per-harness **capture strategy**. For `claude-code`, capture reads
the transcript; for every other harness, capture keeps the existing pane scrape
(no codex regression). The strategy produces a plain-text "captured" string that
flows unchanged through the existing sentinel pipeline
(`countCompletionSentinels`, `hasNewCompletionSentinel`,
`ParseCompletionSentinel`, `extractCapturedSlotText`,
`dispatcher.CompleteFromCapture`). Only the SOURCE of `captured` changes.

### Transcript reader (claude-code)

Inputs: agent-home dir, the executor's configured cwd, the run id, and a
`dispatchStart` timestamp (recorded just before `SendPrompt`).

1. Resolve the transcript directory. Default: `<agent-home>/.claude/projects`.
   `<agent-home>` is derived from the configured agent user
   (`os/user.Lookup(AgentUser).HomeDir`); when `AgentUser` is empty it is the
   service user's own home (`os/user.Current`). An optional override env
   `CHROTE_FORMATIONS_TRANSCRIPT_ROOT` wins when set. Never hardcode a user or
   home path (Perttu: "Cant hardcode a user").
2. Enumerate candidate `*.jsonl` files under the transcript root's project
   subdirectories. A candidate qualifies when: its first parseable record's
   `cwd` equals the configured cwd, AND its mtime is `>= dispatchStart` (minus a
   small skew tolerance, e.g. 2s). Pick the newest qualifying file.
3. Read the file, decode each JSONL record, keep `type == "assistant"` records,
   flatten `message.content` to text (string or array-of-blocks), and
   concatenate the assistant texts in order into the "captured" string.
4. Return that string. Downstream counting/parsing is unchanged — the run-id in
   the sentinel disambiguates, and `previousSentinels` handles reused sessions
   (a reused claude session keeps the same transcript file, which grows).

Triple-guard against reading a stale/foreign transcript: (a) `cwd` field match,
(b) mtime `>= dispatchStart`, (c) run-id sentinel match downstream.

### Wiring

- Thread the harness id (`variant.ID`) and `dispatchStart` from `executeSlot`
  into `countExistingCompletionSentinels` and `waitForCompletion` so they choose
  the capture strategy.
- `waitForCompletion` polls the strategy (same deadline/backoff loop as today).
  When the transcript file does not exist yet, treat it as "no sentinel yet" and
  keep polling until the deadline (do NOT silently fall back to the pane — that
  would reintroduce the bug). On timeout, keep the existing
  `completion_sentinel_timeout` error.
- The output-cap check still applies to the transcript-derived text; the cap is
  already raised via config, and transcript text is far smaller than pane noise.

### Submit path (minor hardening)

Keep the existing paste-then-`ENTER`+`C-m` submit with the pending-pasted-input
retry. Per Perttu, the "send to session" pattern (send prompt, then Enter, then
a few more Enters) is what works — ensure the loop sends enough Enters to defeat
a flaky first submit. This is a small tweak-if-needed change, not a rewrite.

## Interface sketch (illustrative, not binding)

```go
// harnessCapture returns the "captured" text for a dispatch, from the source
// appropriate to the harness. claude-code reads the native transcript; other
// harnesses scrape the pane as before.
type harnessCapture interface {
    Capture(ctx context.Context, sessionName, runID string, dispatchStart time.Time) (string, error)
}
```

Prefer a small, testable pure function for transcript parsing:

```go
// assistantTextFromTranscript decodes claude-code JSONL bytes and returns the
// concatenated assistant-message text (in order). Pure; unit-tested with
// fixtures.
func assistantTextFromTranscript(jsonl []byte) (string, error)

// newestMatchingTranscript picks the newest *.jsonl under root whose first
// record's cwd == wantCwd and mtime >= since. Returns "" if none. Pure over an
// injected fs listing so it is unit-testable.
```

## RED tests (write first, must fail before implementation)

Put fixtures under `src/internal/formations/testdata/transcripts/`.

1. `assistantTextFromTranscript` extracts the concatenated assistant text from a
   real-shaped fixture (mix of `attachment`, `user`, `system`, `assistant`
   records; assistant `content` as BOTH a bare string and an array-of-blocks).
   Assert the ```chrote-outputs``` block and `<<<CHROTE-DONE run-id=... >>>`
   sentinel survive intact.
2. Fed through the existing `ParseCompletionSentinel(captured, runID)` and
   `countCompletionSentinels(captured, runID)`, the transcript-derived text
   yields the correct sentinel (status, artifact) and a count of 1.
3. `newestMatchingTranscript` picks the correct file: given three fixture files
   (wrong cwd; right cwd but mtime before dispatchStart; right cwd and newer),
   it returns the third. Returns "" when none qualify.
4. Reused-session semantics: a transcript with TWO assistant turns each emitting
   a run-id sentinel yields `countCompletionSentinels == 2`, so
   `hasNewCompletionSentinel(..., previousSentinels=1)` is true and `=2` is
   false.
5. Harness routing: the claude-code strategy reads a transcript; a non-claude
   harness id routes to the pane scrape (assert via a fake client that records
   which source was consulted).
6. Malformed/partial JSONL lines are skipped, not fatal (agents write
   incrementally; a half-written trailing line must not error the poll).

## GREEN

Implement to pass the RED tests. Keep the change surface minimal: new transcript
reader + capture-strategy routing + threading `variant.ID`/`dispatchStart`.
Do not alter the sentinel grammar, the output routing, or the codex path.

## Verify

- `go test ./...` green in `src/`.
- `go vet ./...` clean.
- No new host-local tokens in committed docs (scripts/doc-lint.py): avoid
  literal service/data paths and service unit names in this file and any new
  doc — use `<agent-home>`, `<workspace-cwd>`, `<encoded-cwd>` placeholders.

Do NOT push, deploy, or restart any service. Hand back for review; the operator
lands, deploys, and re-runs the live smoke test.

## Follow-ups (out of scope here)

- Deterministic correlation via `claude --session-id <uuid>` injected at launch
  (removes the mtime/cwd heuristic; enables safe high-concurrency same-cwd runs).
- Codex `openai-codex` rollout-file reader (mirror this design for its native
  transcript so codex also stops depending on the pane).
