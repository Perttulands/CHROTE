# 04 · Run Engine, Adapters & the Ledger

How a mission actually executes, how agents in different harnesses collaborate, and how it all stays
recoverable and auditable.

> **Decisions applied (Perttu — see [08](08-open-questions.md)):** the CLI is **`archon`** (read
> `archon` wherever this doc shows `fm`); the engine is **hosted in the CHROTE server's formations
> system**, invoked identically by `archon` and the UI; missions/runs are keyed by **Beads IDs**.

---

## 1. Where it lives (recap)

A deterministic Go scheduler **hosted in the CHROTE server's formations system** (not a separate
process). `archon run <mission>` and the UI "Run" button are two clients of the same system: it builds
an immutable graph snapshot, schedules nodes, drives agents through adapters, and appends to the
on-disk ledger. Because the server is a managed service, runs survive browser/device disconnect; on
restart the server replays the ledger; `archon run resume <bead>` re-attaches. Full rationale in
[01 §3](01-architecture.md); deferred autonomous/scheduled runs in [08 Q1](08-open-questions.md).

The engine is a **conductor, not a brain**: it owns the two things agents are bad at — durable
cross-step bookkeeping (JOIN/cascade) and surviving their own context loss (recovery) — and delegates
all *thinking* to agents.

---

## 2. Execution semantics

The prototype's `flowFrom`/`followBranch`/`runFormation`/`inputsReady`/`upstreamFormation` are
`setTimeout` theatre but encode the exact semantics. Formalized:

### In-memory run state (all replayable from the ledger)

```
Run { id, missionId, graph (immutable snapshot), nodeState{}, edgeDelivered{}, status, epoch }
NodeRun { status: idle|running|needs-review|done|blocked, output*, attempt, slots[] }
```
The graph is **snapshotted at launch** so concurrent definition edits don't corrupt an in-flight run
(they apply to the next run).

### Runnable-node algorithm (the heart)

A node is **runnable** iff its status is `idle` **and every in-edge has delivered** (the JOIN rule —
the prototype's `inputsReady`). The cascade is an **event-driven readiness sweep**, not a topological
pre-sort (because fail-wires create cycles a topo order can't express):

```
sweep(run):
  for each idle node n:
     if not allInEdgesDelivered(n): emit node.waiting (k/m inputs); continue
     start n (formation → goroutine; gate → evaluate)
  if nothing running or runnable: finalize run (done | blocked)
# called at launch and after every node/gate completion.
```
Each formation/gate runs in its own goroutine; on completion it (a) appends its result to the ledger,
(b) marks out-edges delivered per the routing rule, (c) re-sweeps under the run mutex. Slow tmux I/O
happens *outside* the lock (the Oracle discipline). Fan-out is free; multiple downstream nodes can
start in one sweep.

### Gates → routing

On reaching a gate, evaluate each kind: **Code** = run the agent-wired command (exit 0 contributes
pass); **Human** = emit `gate.awaiting_human` + escalation, block until a verdict arrives; **Formation**
= run the judge formation as a sub-run, its output status decides. Combine: all kinds must pass (strict
AND — [08 Q on verdicts]). Then route: **PASS** → pass-port edges; **FAIL + fail wire** → down the fail
wire (may point backward = revise); **FAIL + unwired fail** → **block** (mark upstream `blocked`,
escalate).

### Verification (in-formation)

Runs at the end of a formation's work. pass → finalize `done`; fail+block → `blocked` + escalate;
fail+pushback → re-deliver the brief + feedback to the formation's own slots (`attempt++`) and re-run.

### Pushback / cycle tolerance

The forward graph is acyclic; **fail-wires are the only backward edges.** A backward fail-wire resets
the target to runnable, bumps `epoch`, augments its brief with the gate's criterion + failing output.
**Cycle tolerance = bounded re-entry:** `maxAttempts` (default 3) per node; the 4th entry → **block +
escalate** ("revise loop exhausted"). Never silently spins. This deserves the heaviest tests.

### Per-type execution style (`type` is a hint)

| Type | Behavior |
|---|---|
| **solo** | Deliver brief to the one slot; its reply is the output. |
| **peer** | Deliver to all slots concurrently; concatenate replies into the output (status `needs-review` if a slot flags disagreement — the engine surfaces, it does not judge). The proven Claude+Codex pattern. |
| **flow** | Sequential: slot 1's reply feeds slot 2's brief, …; last reply is the output. |
| **orchestrated** | Deliver the brief to the **controller slot only**; hand it a sub-roster of worker session refs and let *that agent* drive the workers. The engine does not route inside an orchestrated formation. This is the explicit agent-as-runner escape hatch. |

Stage 1 ships solo/peer/flow (deterministic); orchestrated comes once the "here are your workers"
contract is proven.

### Run identity & recovery

Run id = ULID. Resumable because every transition is a ledger append. On engine start (or
`archon run resume <bead>`): replay `events.ndjson` → rebuild state → re-sweep. Nodes that were `running`
at crash are **re-checked, not blindly re-run** — the adapter's `Capture`/`IsBusy` tells us whether
the agent is still working (resume the capture loop) or finished while we were down (capture the reply,
finalize); indeterminate → `needs-review` + escalate. **Never blind re-deliver a prompt.** `cancel`
marks the run cancelled and stops capture loops but **does not kill agent sessions** (golden rule); it
sends a soft `Interrupt` only to slots mid-delivery for this run.

---

## 3. The harness adapter & cross-harness reality

### Interface (pruned to stage-1 minimum — 6 verbs)

```go
type Adapter interface {
    Deliver(ctx, session, prompt) (TurnID, error)   // send a prompt; tmux: C-u + text + uppercase ENTER
    Capture(ctx, session, lines) (string, error)    // capture-pane -p -S -N
    IsBusy(ctx, session) (bool, error)              // working vs idle-at-prompt (heuristic)
    Interrupt(ctx, session) error                    // Esc/C-c, used on cancel only
    Describe(ctx, session) (AgentInfo, error)        // reuse Oracle's enrichAgent fields
    Capabilities() Caps                              // honest self-report: nativeAck=false for tmux
}
```
Pruned away vs Gas City's 13-verb provider: `start/stop` (agents are *discovered live*, not started by
the engine), `nudge` (= `Deliver`), `peek` (= `Capture`), meta ops, `list-running` (= Oracle). A
per-harness `HarnessProfile` (config, not code) carries the reality: prompt markers, busy markers,
submit/clear keys.

```
claude-code: { prompt_markers:["❯"], busy_markers:["esc to interrupt","✶"], submit:"ENTER", clear:"C-u" }
codex:       { prompt_markers:["›"],  busy_markers:["working","tokens used"], submit:"ENTER", clear:"C-u" }
```

### How two agents in different harnesses collaborate (concrete)

1. Resolve each slot → a `SessionRef` (the CHROTE tmux socket + session name).
2. `Capture` both → confirm each is idle at its prompt marker; if busy, wait/escalate (never clobber).
3. `Deliver` the brief to both, embedding a **sentinel instruction**: *"When fully done, print on its
   own line: `<<<CHROTE-DONE turn=<run-id>>>>`. If you need Perttu's judgment, print
   `<<<CHROTE-ESCALATE turn=<run-id> reason=…>>>`."*
4. Start a per-slot `Capture` loop (~3–5 s).
5. Completion = the sentinel line appears (authoritative) **or** busy clears + prompt marker returns +
   output stable for a debounce window. The text between the prompt echo and the sentinel is the reply.
6. JOIN: when all slots complete, assemble the output and run verification.

The agents share nothing beyond the brief (and any bead/board ref passed in it). The engine just
delivers, captures, and detects done.

### The tmux reality — failure modes & fail-loud handling

| Mode | Reality | Handling |
|---|---|---|
| No real ACK | send-keys succeeds even if the TUI ignored the key | sentinel is primary; busy-clear + marker-return + debounce is secondary |
| ENTER didn't submit | some harnesses need uppercase ENTER; prompt sits in the input | always `C-u` + uppercase `ENTER`; re-capture after ~2 s; if still unsent, retry ENTER **once**, then `node.blocked` "delivery unconfirmed" + escalate. Never spam ENTER. |
| TUI redraws / spinners | capture returns animation frames | busy = profile markers + line-stability, not raw byte-diff; strip ANSI (reuse the tmux handler's `colorRegex`) |
| Completion ambiguity | agent prints something prompt-like mid-thought | sentinel authoritative; if it never arrives but the agent goes idle for the full debounce → finalize `needs-review` ("completed without sentinel"), loud, never clean `done` |
| Reply scrolled off / huge | capture `-p` sees only the visible pane | capture with generous `-S -N`; **prefer agents writing artifacts to files and naming the path in the sentinel** (`artifact=/path/report.md`) over scraping the pane |

The tmux adapter reports `nativeAck=false`, so the ledger and UI truthfully show "completion inferred."
When native adapters land later (`nativeAck=true`), the same engine gets real ACKs for free.

---

## 4. Run ledger (durable, append-only, files-as-truth)

`runs/<board>/run-<ulid>.ndjson` — one JSON object per line, append-only + a small atomically-rewritten
`latest.json` snapshot (a regenerable cache for fast UI loads). **Not** Beads-backed (keep work and run
mechanics separate; no Dolt). The ledger references bead ids but stores run mechanics as plain NDJSON —
greppable, replayable, survives `bd` being down.

Event envelope: `{ ts, runId, seq, epoch, type, nodeId?, slotId?, data, actor }`. Types:

- **Run:** `run.started` (mission, graph ref, objective seed) · `run.finished` (done/blocked) · `run.cancelled`
- **Node:** `node.waiting` (k/m) · `node.started` · `node.finalized` (output: status/report/artifacts/diffs/agents/timing) · `node.blocked` (reason) · `node.revised` (attempt, fromGate, feedback)
- **Slot/adapter:** `slot.assigned` · `slot.delivered` (turnId, prompt — redactable) · `slot.reply` (text or artifact path) · `slot.delivery_unconfirmed`
- **Routing:** `edge.delivered` · `gate.evaluating` · `gate.verdict` (perKind results) · `gate.awaiting_human` · `verify.verdict`
- **Escalation:** `escalation.raised` (trigger, severity, message, nodeId)

This guarantees: **(a)** "ask the agents what's happening" is answerable (an agent reads the ledger and
explains — the vision's stage-1 model); **(b)** the UI renders truthfully (every status maps to a real
event, no theatre); **(c)** recovery (replay). `node.finalized` is appended *before* downstream
delivery, so replay never double-delivers (idempotency). The one non-idempotent action — `send-keys` —
is guarded: `slot.delivered` is logged before the capture loop; on replay with no `slot.reply`, the
engine re-attaches the capture loop rather than re-delivering.

---

## 5. Escalation emission

The engine's job is narrow: emit a structured `escalation.raised` event when a trigger fires, and stop
the affected branch when the trigger demands it. **Where it lands and how Perttu sees it is owned by
[doc 06](06-shared-state-and-escalation.md).** Triggers (vision §16), split by source:

| Trigger | Source | Mechanism |
|---|---|---|
| blocked work | system | any `node.blocked` auto-emits `escalation.raised{trigger:blocked}` |
| decision needs taste | agent | a Human gate `gate.awaiting_human` emits it |
| disagreement / drift / cost-risk / opportunity / "found something better" / "stop, this is wrong" | agent | the agent prints `<<<CHROTE-ESCALATE …>>>`; the engine parses it → `escalation.raised` |

So escalation is **both** system-level (the event + the sentinel parser — tiny, always present) and
prompt-level (the brief tells agents *when* to raise it, drawn from §16). Judgment lives in the agent;
the channel lives in the engine. `severity: info|needs-attention|stop`; `stop` additionally pauses the
affected branch. No approval gate, no policy engine — a loud, durable, agent-judged flag.

---

## 6. Streaming, recovery, concurrency

- **SSE to CHROTE**: the ledger event *is* the SSE payload (one schema, two transports). Reuse the
  Oracle handler shape; **clear the write deadline per connection** with
  `http.NewResponseController(w).SetWriteDeadline(time.Time{})` (already proven in `services.go`) to
  defeat the server's 30 s `WriteTimeout`; keep a 30 s heartbeat. Clients pass `?since=<seq>` (or
  `Last-Event-ID`) → the handler replays ledger events `seq > since` then goes live → a reconnecting
  device sees no gap (the "close the laptop, return from another device" requirement).
- **Recovery**: on startup, scan `runs/*/`, replay any non-terminal run, re-sweep.
- **Concurrency**: one writer goroutine per run owns its NDJSON (no cross-run contention); in-memory
  `Run` guarded by a per-run mutex; a top-level `map[runId]*Run` under an RWMutex (the Oracle shape).

---

## 7. Minimal first slice

The smallest engine that runs one real cross-harness collaboration with a truthful, recoverable
ledger:

- Graph: `Mission → peer formation` with slot A = `claude-code`, slot B = `codex` → one in-formation
  `code` verification. (Linear; no standalone gates, no fail-wires, no orchestrated.)
- Engine: graph snapshot, `sweep`, the peer path, JOIN-of-two, NDJSON ledger + replay, finalize,
  `--resume`.
- Adapter: `TmuxAdapter` only, `claude-code` + `codex` profiles; `Deliver` (C-u + ENTER + sentinel),
  `Capture` loop, `IsBusy`, sentinel detection.
- API: `POST /api/runs`, `GET /api/runs/{id}`, `GET /api/runs/{id}/stream` (SSE + `?since`),
  `POST /api/runs/{id}/cancel`. The engine is provable headless (curl + reading the NDJSON); the run
  slice (slice 5) wires this to the cockpit's SSE in the same slice.

**The single most important acceptance test:** kill the CHROTE server / runner mid-run, restart, and
the run **resumes from the ledger** — the recovery property neither pure-agent-driven nor
browser-as-truth can offer. Build standalone gates + fail-wire loops *second* (the subtle-correctness
part, §2, deserves its own focused tests).
