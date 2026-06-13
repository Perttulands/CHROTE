# 01 · Architecture

How the pieces fit, where execution lives, and the risk register.

> **Decisions applied (Perttu — see [08](08-open-questions.md)):** the CLI is **`archon`** (read
> `archon` wherever this doc shows `fm`); execution is **one hosted formations system** in the CHROTE
> server, accessed identically by the CLI **and** the UI (not an `fm`-only library CHROTE merely
> observes); the **notice board is deferred**; phasing is **vertical full-stack slices**.

---

## 1. The layered model

Seven layers. The rule that makes the whole thing safe and reversible: **every layer above the files
is a _client_ of the files.** State lives on disk; clients are disposable.

| Layer | What it is | Owns | Doc |
|---|---|---|---|
| **Touchpoints** | The Archon (root persona) + team leaders | The relationship with Perttu; framing; assembly; escalation | 05 |
| **CLI (`archon`)** | The agent command surface; a client of the hosted system | The *operations* (create/assign/run/inspect) — the same ones the UI issues | 03 |
| **Formations system** | A deterministic Go package hosted in the CHROTE server | Cascade, JOIN, gate eval, the ledger; the single writer of definition files; drives adapters | 04 |
| **Adapters** | One per harness; tmux is the stage-1 fallback-and-default | Delivering a prompt to a live session and capturing the reply | 04 |
| **Files** | TOML boards + persona cards + NDJSON ledger + notice board | **The source of truth** | 02, 05, 06 |
| **Live sessions** | tmux sessions running Claude Code / Codex / Pi / … | The actual work | (Oracle) |
| **Cockpit** | CHROTE's existing dashboard + a read-only Formations tab | Inspection; relaying escalations to TTS | 07 |

### Data flow (a mission run)

```
archon run <mission>  /  UI "Run" button          (CLI or UI — same operation on the hosted system)
  └─ system reads board TOML + persona cards      (files → in-memory graph snapshot)
     └─ run engine: sweep for runnable nodes      (deterministic Go, in the CHROTE server)
        └─ for each ready slot: adapter.Deliver()  → tmux send-keys into the live session
           └─ adapter.Capture() loop              ← capture-pane, watch for sentinel
              └─ append events to runs/<id>.ndjson (the ledger = recovery + audit + UI source)
        └─ JOIN: node runs only when all in-edges delivered
        └─ gate: evaluate → route pass/fail
  archon run logs <bead>  ·  CHROTE tab ◀ SSE       (both read the same ledger; neither fakes state)
```

### The "one id" spine

One identifier unifies everything (see master plan §2). A persona `id` (`susie`) is the card
filename, the tmux session stem, the slot `agentId`, the team member ref, and the ledger key. This
is why the registry, formations, live sessions, teams, and evaluation all join without glue code.
**Protecting this invariant is the single most important implementation discipline.**

---

## 2. How it sits inside CHROTE

CHROTE today (verified): a Go server (`New*Handler` + `RegisterRoutes(mux)`, CORS+bearer-auth+logging
middleware, `:8094` localhost), an embedded React dashboard (`//go:embed dist/*`), an **Oracle** that
detects agent-like tmux sessions by prefix and streams diffs over SSE, a **Beads** API that shells
`bd --json`, a **ttyd** terminal proxy, and a tmux socket at `/run/user/1000/chrote-tmux`.

Formations adds:

- **The formations system** — a new API handler (`src/internal/api/formations.go`) + the run-engine
  package, registered only when `CHROTE_FORMATIONS` is enabled. Owns the files, the engine, and the
  ledger; exposes `/api/formations/*` (read, write the small human-tweak set, run, stream). The single
  writer of definition files.
- **`archon`** — a new CLI binary (`src/cmd/archon`), built into `bin/` beside `bv-refresh`. A thin
  **client** of `/api/formations/*` + `/api/agents` (shares the `core` package for allowed-roots). CLI
  and UI therefore issue the same operations against the same system.
- **`/api/agents`** — a thin registry surface that left-joins persona cards onto Oracle's live
  sessions. Oracle itself is untouched and stays orchestrator-neutral.
- **A Formations tab** — behind `chrote-formations-tab` (default off).

Nothing existing changes. The run engine reuses Oracle's proven patterns (tmux exec via `core`'s env,
SSE broadcaster, capture-pane) and lives in the formations system inside the server, which both the
`archon` CLI and the UI call.

---

## 3. Where execution lives (decided)

The design agents split here; **Perttu resolved it**: *"The mission execution is a system that can be
accessed through the CLI and the UI same as anything else."*

### The options the agents weighed
- **(a) Pure agent-driven.** No engine; an orchestrator *agent* drives the others in prompt-space.
  Rejected: JOIN readiness, cycle tolerance, idempotent re-delivery, and crash-recovery can't live
  reliably in an LLM's head — they need deterministic code.
- **(b) Hosted Go scheduler.** A deterministic executor in the server. *The run-engine pass favored
  this.*
- **(c) Standalone daemon.** A separate long-lived process — the literal Gas City shape. Rejected.

The integration pass worried (b) makes the cockpit a "competing control plane" (the Gas City failure).
Perttu's reframing dissolves that worry.

### The decision
**Execution is part of the one formations system, hosted in the CHROTE server, exposing
`/api/formations/*`. Both `archon` (CLI) and the cockpit UI are clients of it.** Running a mission is
an operation on that system, identical from either surface. Files + the append-only NDJSON ledger are
the source of truth; the server replays the ledger on restart.

| Pressure | How this satisfies it |
|---|---|
| Agent-driven | An agent invokes `archon run`; the system executes it. |
| Deterministic JOIN/cascade/recovery | It's Go in the server, not prompt-space. |
| Files-as-truth | The ledger is the only thing the engine writes. |
| Recoverable across disconnect/restart | The server is a managed service; it replays the ledger on start; `archon run resume <bead>` re-attaches. |
| CLI and UI are the *same* system | Both call `/api/formations/*` — they can't diverge. |
| Not the Gas City trap | A CHROTE-native, file-backed package in the existing server — not a separate runtime with its own worldview + supervisor. |

The "don't become a control plane" worry was about a *separate heavyweight runtime*. The real guard is
**"don't grow a Gas-City worldview"**: stay file-backed, simple, fail-loud, never auto-manage or drain
sessions. Hosting deterministic execution in the one server is fine.

**Deferred:** autonomous/scheduled runs with no agent or browser present (e.g. integrity-maintainer
loops on a timer). The hosted system already provides a persistent host; adding a scheduler/timer is a
later, additive decision — the ledger is the contract. → [doc 08, Q1](08-open-questions.md).

---

## 4. Risk register

L/I = likelihood / impact (L/M/H).

### Technical

| Risk | L/I | Mitigation |
|---|---|---|
| **Cross-harness tmux fragility — no native ACK.** send-keys lands mid-prompt; capture misses the sentinel; ENTER/submit differs per harness. | H/H | Sentinel line carrying the run-id; verify-after-send (re-capture, retry ENTER once, then loud `error`); busy-detection via per-harness markers + output-stability debounce, not raw byte-diff; prefer agents writing artifacts to files and naming the path in the sentinel over scraping the pane. [04 §3] |
| **The engine grows a Gas-City worldview / becomes heavyweight.** | M/H | Hard scope cap: dispatch + record only; no session create/pin/drain/reconcile; stays a file-backed, fail-loud, CHROTE-native package — not a separate runtime with its own worldview/supervisor. |
| **Sentinel collides with normal output.** | L/M | Distinctive token + ULID run-id; a marker without the matching id is ignored. |
| **Lost run state on crash/disconnect.** | M/M | Run state is the append-only ledger, separate from definitions; `archon run resume` replays; completed slots are not re-dispatched; in-flight slots resume by re-capturing the pane (never blind re-deliver). |
| **Concurrent `archon` + engine + UI edits corrupt a file.** | L/M | Ledger is append-only (no edit conflicts). Definition edits are field-level by id with `If-Match` etag + atomic temp-rename. The engine never writes definitions. |
| **30s server WriteTimeout kills long SSE.** | L/M | `http.NewResponseController(w).SetWriteDeadline(time.Time{})` per stream — already proven in `services.go`. Keep a heartbeat. [07] |

### Product / strategic

| Risk | L/I | Mitigation |
|---|---|---|
| **Scope creep back to canvas-first / Gas City.** | H/H | Canvas is the last phase behind its own flag; acceptance is conversational so "done" can't be claimed by a pretty canvas. |
| **Human pulled back into choreography** (the pain we're killing). | M/H | First-slice acceptance requires Perttu to give one goal to the Archon and never name a socket/session. |
| **Prompt injection / blast radius.** A captured reply contains a fake sentinel or instructions. | M/H | Sentinel carries run-id; captured text is recorded, not executed; the engine never feeds raw reply text to another agent without it passing the Archon's judgment; localhost-only; the audit trail makes any bad action visible after the fact. |
| **Runaway loop / cost.** | M/M | Stage-1 is linear (no cycles). Per-run max-dispatch + wall-clock timeout as fail-loud limits (record-and-stop, not approve). Cost is a named escalation trigger. |
| **"Working" is never verifiable → endless polish.** | M/M | Falsifiable acceptance criteria (master plan §11). |

### The autonomy-vs-audit tension

Fully resolved in [master plan §7](00-master-plan.md): build the run-model + ledger + adapter +
recovery the legacy docs require (observability/boundedness), skip the approval gates / policy engine
Perttu doesn't want (permission). Audit ≠ safeguard.

---

## 5. What this design deliberately is *not*

- Not a workflow engine you configure (it's an org you talk to).
- Not a dashboard you watch (it's a system you ask).
- Not a new control plane (CHROTE stays the cockpit; tmux stays the session authority).
- Not a vendored runtime (no Gas City, no Dolt, no supervisor service).
- Not governed (no gates/policy in stage 1 — judgment + audit instead).
