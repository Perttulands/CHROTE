# Fundamentals and Vertical Slices

> The prior design packets assumed horizontal layers: data model → CLI → registry → engine → adapter → UI. That is wrong. The right approach is to identify the hard fundamental parts, prove each one in isolation with simple code, then build end-to-end vertical slices on top of proven fundamentals. Each slice must be small, working, test-covered, and documented.

---

## The five hard fundamentals

These are the physics of the system. Everything else (CLI, UI, gates, canvas, evaluation) is built on top. If any of these is wrong, the system fails silently or falls apart at scale.

### F1. File contract — atomic writes, round-trip, unknown fields

**What it means:** The canonical files on disk must be the source of truth. Writing must be atomic (temp + rename). Reading must preserve fields the current client doesn't understand. A human edit and an agent edit must round-trip without clobbering each other.

**What "proven" looks like:**
- A test that writes a file with an extra field (`agentReasoning = "..."`), reads it back, issues a rename operation via the API, and asserts the extra field survived.
- A test that simulates two concurrent writers and verifies no corruption (one wins, the other gets a conflict error).
- A test that detects external edit (mtime/fsnotify) and triggers a reload event.
- The diff of any structural change is ≤3 lines.

**Why it's hard:** Go's JSON encoder reorders keys. TOML re-serialization changes table layout. Concurrent writes without locking corrupt files. Losing unknown fields means agents can't add metadata that humans later edit.

**Decision:** TOML for definitions, NDJSON for ledger. Atomic write via temp+rename. Single writer (`fm` or server, not both). External-edit detection via mtime polling (fsnotify where available). Schema version at top of file; refuse newer, up-migrate older.

---

### F2. Run engine — append-only ledger, status projection, resume

**What it means:** A run produces an append-only stream of events. Status is projected from the event log, not stored separately. A run can be resumed after disconnect by replaying the ledger.

**What "proven" looks like:**
- A test that appends 10 events, kills the writer mid-stream, restarts, replays from event 6, and produces the correct final status.
- A test that projects status from a 1000-event log in <10ms.
- A test that cancels a run and verifies the cancellation event is the last one (no further events accepted).
- A test that two runs of the same mission produce independent ledgers with no shared mutable state.

**Why it's hard:** Appending to a file while reading it is racy. Projection logic must be deterministic and fast. Resume requires the mission snapshot to be immutable. Cancel requires a coordination channel that doesn't block the appender.

**Decision:** Each run gets its own directory: `.formations/runs/<board>/<run-id>/`. Events written to `events.ndjson`. Mission snapshot copied to `mission.snapshot.toml` at run start. Status projected on read. Cancel writes a `cancel` file; the engine polls for it.

---

### F3. Adapter boundary — tmux send-keys, capture-pane, sentinel

**What it means:** The engine delivers work to a live agent session via tmux `send-keys`, reads the result via `capture-pane`, and detects completion via a sentinel line in the output. No native ACK. No RPC. Just text over a terminal.

**What "proven" looks like:**
- A test that sends a command to a tmux pane, captures output, and extracts the sentinel line (`<<<CHROTE-DONE run-id="..." status="ok" artifact="..." >>>`).
- A test where the sentinel is missed (pane closed mid-run) and the engine records a timeout error in the ledger after N seconds of idle.
- A test where captured output contains a fake sentinel (prompt injection) and the engine ignores it because the run-id doesn't match.
- A test that proves same-harness handoff (Claude Code → Claude Code) and cross-harness handoff (Claude Code → Codex) both work.

**Why it's hard:** Keys can land mid-prompt. The pane can be closed or frozen. Sentinels can be faked or missed. Different harnesses have different ENTER handling (Codex vs Claude). Idle detection is heuristic. Prompt injection through captured output is a real attack vector.

**Decision:** Sentinel carries the run-id (stray markers ignored). Verify-after-send: send, wait, capture, look for sentinel. If not found, re-capture. Timeout + idle detection → loud `error` event. Same-harness first, then cross-harness. Never execute captured text; only record it.

---

### F4. Persona → session binding

**What it means:** A formation slot says `agentId = "susie"`. The engine must resolve that to a live tmux session named `susie` (or `claude-susie`, or `codex-susie`) and verify it is alive before sending work.

**What "proven" looks like:**
- A test that resolves `"susie"` to a live tmux session when `susie` exists, `claude-susie` exists, and both exist (disambiguation).
- A test that resolves `"susie"` when no session exists and records a clear error (not a silent hang).
- A test that resolves `"susie"` when the session exists but the pane is dead (process exited) and records a dead-session error.
- A test that two different boards reference the same agent and both resolve to the same session without conflict.

**Why it's hard:** Session names are not guaranteed unique. tmux session existence ≠ pane liveness. An agent can have multiple sessions (one per harness). The binding must be fast enough to not block run dispatch.

**Decision:** Human-meaningful primary ID (`susie`) with harness suffix disambiguation (`susie-codex`, `susie-hermes`). Resolution order: exact match → suffix match → error. Liveness check: tmux session exists + pane process running. No caching of liveness (check every dispatch).

---

### F5. Cross-harness handoff

**What it means:** An agent in one harness (e.g., Claude Code) delivers a brief to an agent in another harness (e.g., Codex) via tmux, and receives a result. This is the headline value of the system.

**What "proven" looks like:**
- An end-to-end test: Archon (Hermes) creates a formation with a design-lead slot (Claude Code) and a specialist slot (Codex). The Archon dispatches to the design lead. The design lead works and emits a sentinel. The engine captures the result. A real artifact exists in the workspace.
- The test runs without human intervention and takes <5 minutes.
- The ledger shows: `dispatch → node_started → slot.dispatch → slot.result(sentinel) → node_output → run.end`.
- Kill the specialist pane mid-run → visible `error` event in the ledger.

**Why it's hard:** This is F1–F4 combined in a real scenario. If any fundamental is weak, the handoff fails. Cross-harness means different prompt formats, different ENTER behavior, different sentinel expectations.

**Decision:** This is Slice 2 (see below). Slice 1 proves same-harness. Slice 2 adds cross-harness. If Slice 1 is solid, Slice 2 is mostly adapter configuration.

---

## What is NOT a hard fundamental

These are important but not physics. They can be added, removed, or changed without destabilizing the system:

- **CLI ergonomics** (`fm` vs `chrotectl`, flags, help text) — surface. Change anytime.
- **Canvas / Formations tab** — pure UI. Reads the same files. Can be built in a weekend once the file contract is stable.
- **Gates and verification** — logic on top of the run engine. A gate is just a node that runs a command and emits a `gate_verdict` event.
- **Notice board** — another file format. Independent of the run engine.
- **Evaluation / track record** — read-only projection from the ledger. No write model needed.
- **Dynamic team assembly** — a query over the persona catalogue. Does not change the file contract.
- **Always-on daemon** — a process that calls `fm run` on a schedule. The engine doesn't care who invokes it.

---

## Vertical slice sequence

Each slice is a complete, end-to-end system for a narrow use case. It includes file contract, engine, adapter, and a minimal UI (even if that UI is just `cat` and `grep`). No slice depends on a future slice. Each slice must pass its own tests and be deployable independently.

### Slice 0: File contract spike (no engine, no adapter)

**Goal:** Prove F1 in isolation.

**What it is:** A Go package (`internal/formations/store`) that reads and writes TOML board files with atomic writes, round-trip preservation, and conflict detection.

**Tests:**
- Write a board with extra fields. Rename a node. Assert extra fields survive.
- Two goroutines write the same file. One wins, one gets conflict.
- External process modifies file. Detected via mtime change.

**Deliverable:** `internal/formations/store` package with 100% coverage. No CLI, no API, no tmux.

**Why this is a slice:** It is a working system for the narrow use case of "edit a formation file safely." A human can use it via a test script. It proves the hardest part of the data model before anything else is built.

---

### Slice 1: Same-harness handoff (one formation, one agent)

**Goal:** Prove F2 + F3 + F4 in a real scenario.

**What it is:** A minimal `fm` binary with exactly four verbs: `init`, `run`, `status`, `log`. It reads a board file (one formation, one slot), resolves the slot to a tmux session, sends a brief via `send-keys`, captures output, detects sentinel, and appends to the ledger.

**The scenario:**
1. Perttu tells the Archon: "Research how to improve session search."
2. The Archon (or a test script) runs `fm init session-search` to create a board.
3. The Archon edits the board to set one slot: `agentId = "scout"` (a Hermes session named `scout`).
4. The Archon runs `fm run session-search`.
5. The engine dispatches to `scout` via tmux. Scout works and emits `<<<CHROTE-DONE run-id="..." status="ok" artifact="session-search-notes.md" >>>`.
6. The engine captures the result, appends to the ledger.
7. Perttu runs `fm status session-search` and sees: completed, artifact at `session-search-notes.md`.

**Tests:**
- End-to-end test with a real tmux session running a stub agent (a shell script that emits the sentinel).
- Kill the session mid-run → ledger shows `error: session died`.
- Missed sentinel → ledger shows `error: timeout` after N seconds.
- Fake sentinel (wrong run-id) → ignored, then timeout.
- Resume: `fm run --resume` replays ledger and continues.

**Deliverable:** `fm` binary + `internal/formations/{store,engine,adapter}` packages. One integration test that runs the full scenario. No cross-harness, no gates, no canvas.

**Why this is a slice:** It is a complete system for "assign one task to one agent and get a result." A human can use it today. It proves the core loop works before adding complexity.

---

### Slice 2: Cross-harness handoff (two agents, different harnesses)

**Goal:** Prove F5. The headline value.

**What it is:** Extend Slice 1 to support formations with two slots referencing agents in different harnesses. The engine delivers the brief from slot A to slot B, captures the result, and appends to the ledger.

**The scenario:**
1. Archon creates a board with two slots: `design-lead` (Claude Code session `claude-design`) and `frontend-dev` (Codex session `codex-frontend`).
2. Archon runs `fm run`.
3. Engine dispatches to `claude-design`. Claude Code works, emits sentinel with a brief for the frontend dev.
4. Engine captures the result, dispatches to `codex-frontend`.
5. Codex works, emits sentinel with a prototype `index.html`.
6. Engine captures the result. Ledger shows the full cascade.
7. Perttu asks Archon "what happened?" Archon reads ledger and answers.

**Tests:**
- End-to-end with two real harness sessions (or stub sessions that emulate different ENTER handling).
- Same tests as Slice 1, but for the second slot.
- Abort mid-cascade → ledger shows partial completion, resumable.

**Deliverable:** Extended `fm` + adapter. One integration test with two sessions. No gates, no verification, no canvas.

---

### Slice 3: Gate and verdict

**Goal:** Add a checkpoint between formations.

**What it is:** A gate is a node that runs a command and emits a `pass` or `fail` verdict. If `pass`, the run continues. If `fail`, the run blocks.

**The scenario:**
1. Formation A (design) → Gate (tests) → Formation B (implementation).
2. Gate runs `go test ./...`.
3. Exit 0 → `gate_verdict: pass` → continue to Formation B.
4. Exit 1 → `gate_verdict: fail` → `run_blocked`.
5. Perttu fixes the test, runs `fm run --resume`. Gate re-evaluates.

**Tests:**
- Gate passes → ledger shows `gate_verdict: pass`, run continues.
- Gate fails → ledger shows `gate_verdict: fail`, run blocked.
- Resume after fail → gate re-runs, passes, run continues.
- Human verdict gate → Perttu runs `fm verdict <run-id> <gate-id> --pass`.

**Deliverable:** Gate node type in board schema. Gate evaluation in engine. `fm verdict` verb. Tests for all three gate kinds (code, human, formation-judge deferred).

---

### Slice 4: Persona cards and catalogue

**Goal:** Formalize agent identity and discovery.

**What it is:** `~/agents/*.toml` cards. `fm who` lists cards and live sessions. `fm init` can reference a persona card to auto-populate a slot.

**Tests:**
- Create a persona card. `fm who` lists it.
- Start a tmux session matching the card's `matchStem`. `fm who` shows it as live.
- `fm init` with `--slot scout` auto-populates the slot from the card.
- Two cards, same capability tag. `fm who --capable typescript` lists both.

**Deliverable:** Persona card schema. `fm who` verb. Catalogue query (minimal: tag match + liveness). No track record yet.

---

### Slice 5: Formations tab (read-only)

**Goal:** Make the system visible in CHROTE.

**What it is:** A new CHROTE dashboard tab, default-off via `chrote-formations-tab` flag. Reads board files and run ledgers over SSE. Renders mission list, node graph (read-only), run timeline.

**Tests:**
- Tab hidden when flag is off.
- Tab shows mission list when flag is on.
- Click mission → shows node graph from board file.
- Run in progress → SSE streams events to timeline.
- No write-back. All edits via `fm`.

**Deliverable:** Dashboard tab component. Go SSE endpoint for run events. No mutation API.

---

### Slice 6: Notice board

**Goal:** Add team communication.

**What it is:** `.formations/board.ndjson`. `fm post` and `fm read` verbs. Notices are JSONL lines with sender, audience, kind, body.

**Tests:**
- Post a notice. Read returns it.
- Notice persists across runs.
- Escalation is a notice with `kind = "escalation"`.

**Deliverable:** Notice schema. `fm post`/`fm read`. Escalation detection in engine.

---

### Slice 7: Evaluation and track record

**Goal:** Answer "how is this agent doing?"

**What it is:** Summarize the ledger per agent. `fm eval <agent-id>` shows success rate, average cycle time, recent errors.

**Tests:**
- 10 runs, 8 success, 2 fail. `fm eval` reports 80% success.
- Agent with no runs. `fm eval` reports "no data."

**Deliverable:** Ledger summarizer. `fm eval` verb. No ranking algorithm yet.

---

## What each slice explicitly does NOT include

| Slice | Out of scope |
|---|---|
| 0 | CLI, API, tmux, engine, UI |
| 1 | Cross-harness, gates, canvas, persona cards, notice board |
| 2 | Gates, verification, canvas, persona cards |
| 3 | Human verdict (Slice 3 adds code gate; human verdict is Slice 3 too), canvas |
| 4 | Track record, dynamic binding, evaluation |
| 5 | Write-back, graph editing, run controls (start/cancel deferred) |
| 6 | Beads integration (notices are separate), Gastown delegation |
| 7 | Ranking algorithm, cost tracking, multi-host |

---

## Testing strategy

Every slice must have:

1. **Unit tests for the file contract** (store package): 100% line coverage.
2. **Unit tests for the engine** (event projection, status calculation): 100% line coverage.
3. **Unit tests for the adapter** (sentinel detection, timeout logic): 100% line coverage.
4. **Integration tests for the slice scenario**: One test that runs the full end-to-end flow. Uses real tmux sessions where possible, stub sessions where necessary.
5. **Documentation**: `README.md` in the slice directory explaining what it does, how to run it, and what "done" means.

No slice is "done" until `go test ./...` passes and the integration test demonstrates the scenario without human intervention.

---

## Documentation strategy

Each slice delivers:

- **Code comments:** Every exported type and function has a doc comment.
- **README:** What the slice does, how to run the tests, what the acceptance scenario is.
- **Decision record:** Any decision made in the slice (format choice, ID scheme, timeout value) is recorded in `docs/decisions/YYYY-MM-DD-slug.md`. Not ADRs — just a sentence of context and the rationale.
- **No separate design doc:** The code and tests are the design. If it can't be expressed in code, it's not ready.

---

## Kill switches per slice

| Slice | Kill switch | Effect |
|---|---|---|
| 0 | Remove `internal/formations/store` | Nothing else depends on it yet. |
| 1 | `CHROTE_FORMATIONS=off` (if API added) or just don't run `fm` | CHROTE behaves as today. |
| 2–4 | Same as Slice 1 | |
| 5 | `chrote-formations-tab` localStorage flag | No tab. No API routes registered if server flag is off. |
| 6 | Don't run `fm post` | No notices. No runtime impact. |
| 7 | Don't run `fm eval` | No evaluation. No runtime impact. |

---

## Summary

- **Prove 5 fundamentals first** (F1–F5), each in isolation, with tests.
- **Build 7 vertical slices** (0–6), each end-to-end, each independently useful.
- **No horizontal layers.** No "build the CLI, then the API, then the UI." Each slice has what it needs.
- **Small, tested, documented.** Every slice must pass `go test` and demonstrate its scenario.
- **Slice 1 is the real milestone.** Same-harness handoff is the first proof the system works. Cross-harness (Slice 2) is the headline value but depends on Slice 1 being solid.
