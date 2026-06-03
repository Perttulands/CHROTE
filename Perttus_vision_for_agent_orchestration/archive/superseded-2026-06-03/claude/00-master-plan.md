# CHROTE Formations — Master Plan

> Synthesis of the vision interview, the `03-formations` prototype, the existing CHROTE
> architecture, the prior meta-harness/Gas City research, and seven parallel design passes.
> Detail lives in the numbered docs; this is the spine.

---

## 1. What we are building (and what we are not)

**We are building an agent organization around Perttu** — named agents, team leaders, persistent
specialists — that he reaches through a small number of trusted touchpoints (chiefly the **Archon**)
while accessing the capability of many agents and harnesses underneath.

**We are not building** an automation framework, a workflow runner, or an orchestration dashboard.
The prototype *looks* like a node-graph editor, but its own header says the truth: it is an
**inspection + light-tweak surface** over a system that **agents author and run through a CLI, with
files on disk as the source of truth.** Perttu does not hand-build formations. Agents do.

This distinction governs every decision below. Two quotes anchor it:

> "The primary way these formations and missions are created is through agents using very
> agent-native and simple primitives." — Perttu, this engagement
>
> "In the first stage, I don't really expect there to be anything in terms of making this visible.
> If I want to know something, I can just ask the agents." — vision §7

### Core principles (non-negotiable)

1. **Agent-driven & agent-native.** The interface agents use is a small, `bd`-style CLI over plain
   files. The human almost never touches it.
2. **Files are canonical.** Definitions and run history live on disk. Every other component (CLI,
   engine, CHROTE) is a *client* of those files. Lose any client; the work survives.
3. **One touchpoint, many agents.** The Archon (and team leaders) preserve a single coherent
   conversation over a large, multi-harness org. Cross-harness collaboration (Claude Code + Codex as
   peers) is the headline value — not one provider's sub-agents.
4. **Inspection surface, not an authoring tool.** Agents author via the CLI; the cockpit's Formations
   tab makes that legible (built incrementally from slice 1) but never becomes where things are built.
   Run-time "what's happening?" stays answerable by asking the agents (they read the ledger).
5. **Agent judgment over safeguards — but full audit.** No approval gates, policy engines, or
   cost-kill switches in stage 1. But everything is recorded in an append-only ledger. *Audit is not
   a safeguard* (see §7).
6. **Modular and feature-flagged.** Every layer is independently removable. Flag off ⇒ zero impact.
7. **Simplicity first; fail loud.** Minimum structure, no speculative abstraction, no silent
   failures (per `CLAUDE.md` and the Gas City lessons).

---

## 2. Architecture at a glance

Seven thin layers, each a client of the files beneath it. Full detail in
[01-architecture.md](01-architecture.md).

```
   Perttu
     │  (talks to)
     ▼
  ┌─────────┐   the Archon — root persona, single touchpoint
  │ ARCHON  │   frames goals · picks/assembles teams · stays involved · escalates
  └────┬────┘
       │ drives, via the CLI
       ▼
  ┌──────────────────────────────────────────────────────────────────────┐
  │  archon — agent CLI (mirrors `bd`)  +  CHROTE UI  ── two clients, one system │ doc 03
  │  agent · formation · mission · run · gate    (missions keyed by bd-IDs)  │
  └───────┬───────────────────────────────────────────┬───────────────────┘
          │ both clients of                            │ invoke
          ▼                                             ▼
  ┌────────────────────────┐                   ┌───────────────────────────┐
  │  FILES (source of truth)│                   │  RUN ENGINE (hosted)       │  doc 04
  │  .formations/ boards    │   doc 02          │  cascade · JOIN · gates    │
  │  ~/agents/  personas    │   doc 05          │  → adapters → tmux agents  │
  │  runs/ NDJSON ledger    │   doc 04          │  append-only ledger        │
  │  board.ndjson notices   │   doc 06          └───────────┬───────────────┘
  └───────────┬────────────┘                               │ tmux send-keys + capture-pane
              │ read (+ small tweaks)                       ▼
              ▼                                   ┌──────────────────────┐
  ┌────────────────────────────┐                 │ live agent sessions    │
  │  CHROTE cockpit             │  doc 07         │ (Claude Code, Codex,   │
  │  Formations tab (read-only, │◀── SSE ─────────│  Pi, OpenCode, …)      │
  │  flag-gated) · Agents · Beads│  run events    │ = Oracle-detected tmux │
  └────────────────────────────┘                 └──────────────────────┘
```

### The "one id" spine

A single string ties the whole system together. A persona's `id` is **simultaneously**:

- the persona card filename (`~/agents/susie.toml`),
- the tmux **session stem** Oracle binds liveness to (`susie` / `claude-susie`),
- the `agentId` a formation **slot** references,
- a **team** member reference,
- the **ledger** key for that agent's track record (`interactions.jsonl`, `persona=susie`).

Get this one identifier right and registry ⇄ formations ⇄ live session ⇄ teams ⇄ evaluation all
join for free. It is the most important invariant in the design.

---

## 3. The seven decisions (consensus)

Seven independent design passes converged on these. Each has a detail doc. Genuinely open forks are
in [08-open-questions.md](08-open-questions.md).

| # | Area | Decision | Why |
|---|------|----------|-----|
| 1 | **Data format** | TOML, **one file per board** under `<project>/.formations/` (sibling of `.beads/`); x/y in a **layout sidecar**; run history in **append-only NDJSON**. Stable ULID-style ids round-trip files↔CLI↔UI. | Agents already author TOML (Gas Town formulas). Diffs clean. The definition file stays purely logical; run state never pollutes it. [doc 02] |
| 2 | **CLI** | **`archon`** — a `bd`-style CLI (`agent`/`formation`/`mission`/`run`/`gate`; `--json`, fail-loud, idempotent). The **primary** interface; the cockpit UI issues the *same* operations against the *same* system. Missions/runs are keyed by **Beads IDs** (a mission is bead-backed work). | "Formation is only one noun" — the CLI commands the whole org, so it takes the org's name. [doc 03] |
| 3 | **Run engine** | A deterministic **Go scheduler in the hosted formations system** (in the CHROTE server), invoked identically from `archon` and the UI; drives agents through a pruned **harness adapter**; cross-harness work is **tmux `send-keys` + `capture-pane` + a sentinel completion line** (no native ACK in stage 1). Append-only ledger ⇒ recoverable across disconnect/restart. | Execution is "a system accessed the same as anything else" (Perttu); determinism lives in Go, not prompt-space; stays file-backed so it isn't a Gas-City control plane. [doc 04] |
| 4 | **Registry / personas** | A **persona = a small TOML "card"** in `~/agents/` that *points at* the real definition (a Hermes profile, a `CLAUDE.md`, a skill). **Persistent ⇔ a card exists.** Liveness comes from the existing Oracle, **left-joined** by session-stem. Progressive disclosure = read more of the card. | Don't reinvent identity: Hermes profiles already exist (incl. **`archon`**). Reuse them. [doc 05] |
| 5 | **Shared state** | **Beads = work; a file-native NDJSON notice board = communication.** *Not* messages-as-beads — `bd mail` delegates to Gastown, which this install forbids. | Keeps `bd` pristine, keeps comms `cat`-able and dependency-free. [doc 06] |
| 6 | **Escalation** | An escalation is a **board post** (source of truth) → mirrored to a durable **`bd` event/wisp** → optional **TTS interrupt** (the `/srv` gateway). Agents raise it via a `CHROTE-ESCALATE` sentinel; the engine records it; the Archon surfaces it. | One loud, durable, agent-judged flag. No gate. [doc 06] |
| 7 | **CHROTE integration** | A new **Formations tab behind a default-off flag** (`chrote-formations-tab`, same pattern as `uiV2`). **Read-only inspection first.** The vanilla-JS canvas is ported as one self-contained imperative component (not rewritten in React). Run events stream over **SSE** (reuse the `services.go` `SetWriteDeadline` trick). | Reversible, surgical, matches existing conventions. [doc 07] |

> **Decisions 5–6 are deferred** (Perttu: the notice board is "a very different system"). Stage-1
> communication is the brief (inline) + the run ledger + conversational status via the Archon;
> escalation stays minimal until the board is built. The design above stands for when it is.

---

## 4. Where execution lives (decided)

The design agents split here; Perttu resolved it: *"The mission execution is a system that can be
accessed through the CLI and the UI same as anything else."*

**Execution is part of the one formations system, hosted in the CHROTE server (exposing
`/api/formations/*`). Both `archon` (CLI) and the cockpit UI are clients of it.** Running a mission is
just an operation on that system, available identically from either surface. Files + the append-only
NDJSON ledger are the source of truth; the server replays the ledger on restart, so runs survive
browser/device disconnect and server restarts.

Why this is right (and not the Gas City trap): Gas City failed as a *separate, heavyweight runtime with
its own worldview and a second supervisor service*. A CHROTE-native, file-backed, fail-loud formations
package inside the existing server is categorically different. So the guard becomes **"don't grow a
Gas-City worldview"** — stay file-backed, simple, fail-loud, never auto-manage or drain sessions — not
"don't host execution." The JOIN/cascade/recovery bookkeeping is deterministic Go (not prompt-space),
which is what the run-engine analysis correctly required.

**Deferred:** autonomous/scheduled runs with no agent or browser present (e.g. integrity-maintainer
loops). The hosted system already gives a persistent host; adding a scheduler/timer is a later,
additive decision — the ledger format is ready for it. See [01-architecture §3](01-architecture.md).

---

## 5. Phasing — vertical full-stack slices

**Principle (Perttu): slice full-stack, not layer-by-layer.** Each slice delivers one capability
end-to-end — an agent does it via `archon`, it round-trips through the files, and it appears/works in
the cockpit UI — *before* the next capability starts. This surfaces file↔CLI↔UI integration and
identity/round-trip issues early, and keeps every slice independently useful and reversible. The
fragile cross-harness *run* is its own slice, tackled once the structural round-trip is proven
full-stack.

Indicative slice order — each is CLI + files + API + UI together (PRD "Phase 4/5"):

```
 1. SEE A FORMATION    agent runs `archon formation create peer --agents codex,claude`
                       → board file written → renders in the Formations tab (read).   ← first slice
 2. ASSIGN AN AGENT    `archon formation assign …` AND the UI drag gesture → write-back round-trip.
 3. SPAWN & ATTACH     `archon agent spawn codex --name scout` / `attach` → real session, both surfaces.
 4. BUILD A MISSION    `archon mission create/wire` → a chain (mission → formation → gate), + its bead.
 5. RUN IT (X-HARNESS) `archon mission run` → engine delivers briefs to agents in different harnesses
                       via tmux + sentinel, streams events (SSE) to the UI, logs the ledger.  ← fragile part
 6. RECOVER & DECIDE   `archon run resume/abort`, `archon gate approve/reject` — both surfaces.
```

**Deferred entirely for now:** the notice board (a different system), gates' machinery beyond
approve/reject, all four formation types at once, harness adapters beyond tmux, evaluation, an
autonomous daemon, multi-host.

---

## 6. The first slice (concrete)

The first vertical slice is **"see a formation"** (slice 1 above): an agent authors a formation via
`archon`, it persists to a `.formations/` board file, and it renders in a read-only Formations tab —
proving the **data model + `archon` write + `/api/formations` read + UI canvas** round-trip
end-to-end, with stable ids surviving the round-trip. Small, full-stack, flag-gated.

This deliberately tackles the load-bearing round-trip/identity risk first and cheaply, and gets the
prototype canvas rendering *real* agent-authored data early — the point of full-stack slicing.

### Scope IN (slice 1)
- `.formations/` board TOML (one formation: type + slots referencing agent ids) written by `archon`.
- `archon agent list` + `archon formation create | list | inspect`.
- `GET /api/formations/boards/{id}` composing the board into the prototype's JSON shape.
- The Formations tab (behind `chrote-formations-tab`, default off) rendering the board read-only.
- A round-trip test: an agent-authored field the UI doesn't model survives a CLI edit byte-for-byte
  (Rule 7).

### Scope OUT (later slices / deferred)
UI write-back (slice 2) · spawn/attach (3) · mission build (4) · the cross-harness **run**, engine,
adapters, ledger streaming (5) · resume/abort + gate verdicts (6) · the notice board, gates' full
machinery, peer/orchestrated execution, more adapters, evaluation, multi-host (deferred).

### The proof it builds toward (slices 1–6 = the career/web-experience scenario)

By slice 6: Perttu gives one goal to the Archon; it assembles a team and runs a cross-harness mission
(`archon mission run` → bead `bd-…`); a real artifact appears; Perttu asks *"what came back?"* and gets
a plain-language answer from the ledger; the run survives a disconnect (`archon run resume`); killing a
pane fails loud; a gate escalates for his judgment (`archon gate approve/reject`). See §11.

---

## 7. The autonomy-vs-audit tension (resolved)

Perttu wants **no safeguards** and reliance on agent judgment (vision §7, Finding 7). The prior
CHROTE research (correctly) warns: *do not build automatic agent-to-agent routing without an
explicit run model, audit trail, adapter boundary, and recovery story*
(`agent-collaboration-primitives.md`). These look like they collide. They do not:

- **"No safeguards"** is about *not gating work* — no approval prompts, no policy engine, no
  cost-kill that interrupts judgment.
- **The legacy caution** is about *observability and boundedness* — can you tell what happened, is
  there one clean seam between agents, can you recover.

This design **delivers exactly what the legacy docs demanded, while honoring "no safeguards":**

| Legacy requirement | This design | A gate? |
|---|---|---|
| Explicit run model | Formation TOML + ULID run-id + the hosted run engine | No — structure |
| Audit trail | Append-only NDJSON ledger of every dispatch/result/escalate/error | No — it records, never blocks |
| Adapter boundary | One tmux adapter; captured pane text is *data to record*, not commands to execute | No — an interface |
| Recovery story | Ledger-backed `archon run resume` | No — durability |

Autonomy lives in the agents' judgment about *what to do*; the system merely makes that judgment
**auditable and bounded enough to fail loud.** Escalation (`CHROTE-ESCALATE` → ledger → Archon →
Perttu) is the deliberate, agent-judged channel for the "meaningful reasons" in vision §16. The only
"limits" are fail-loud ones (per-run max-dispatch, wall-clock timeout): they *record-and-stop*, they
do not *ask permission*.

---

## 8. Avoiding the Gas City failure modes

Gas City was mechanically capable but failed as a direction. Each named failure maps to a specific
choice here:

| Gas City failure | This design avoids it by |
|---|---|
| Heavyweight worldview (city/rig/pack/supervisor) | No worldview. A formation is a TOML file, a persona a TOML card, a run NDJSON lines — all `cat`-able. `.formations/` is a sibling of `.beads/`, not a parallel universe. |
| Competing control plane | The engine **dispatches + records only**. It never creates/pins/drains/reconciles sessions and has no supervisor. CHROTE stays the cockpit; tmux stays the session authority. |
| Dolt dependency | Zero new datastore. TOML + NDJSON files. Reuse `bd` for work; no Dolt. |
| Silent failures (`mol-review-quorum` exited 1, no output) | Fail-loud is a principle: every dropped sentinel / dead pane / bad file → a visible ledger `error` event and an Archon that *says* it. |
| Auto-drained sessions | The engine never kills/drains/pins sessions. It attaches to existing ones and leaves them running (CHROTE's golden rule). |
| Too broad to vendor | Vendor nothing. Build the minimum (one adapter, linear flow, three personas), CHROTE-native, small enough to read in one sitting. |
| Overloading Beads / exposed supervisor API | `bd` stays work-only. No new network surface — `archon` is a local CLI; the engine is the local CHROTE server. |

---

## 9. Risk register (top items)

Full register in [01-architecture.md](01-architecture.md) and the per-slice docs. The ones that
will actually bite:

| Risk | L/I | Mitigation |
|---|---|---|
| **Cross-harness tmux is fragile — no native ACK.** Keys land mid-prompt; the sentinel is missed; harnesses differ (Codex vs Claude ENTER handling). | H/H | Sentinel line with run-id; verify-after-send (re-capture); timeout + idle-detection → loud `error`; prove same-harness first, then cross-harness, in the same phase. [doc 04 §3] |
| **Scope creep back to canvas-first / Gas City.** | H/H | Canvas is the *last* phase behind its own flag; acceptance is conversational, so "done" can't be claimed by shipping a pretty canvas. |
| **The engine grows a Gas-City worldview / becomes heavyweight.** | M/H | Hard scope cap: dispatch + record only; no session management/draining; stays file-backed + fail-loud — a CHROTE-native package, not a separate runtime with its own worldview. |
| **Human gets pulled back into choreography** (the very pain we're killing). | M/H | First-slice acceptance *requires* Perttu to give one goal to the Archon and never name a socket/session. |
| **Prompt injection / blast radius** (a captured reply contains a fake sentinel or instructions). | M/H | Sentinel carries the run-id (stray markers ignored); captured text is recorded, not executed; localhost-only; the audit trail makes any bad action visible. |
| **Runaway loop / cost.** | M/M | Stage-1 is linear (no cycles). Per-run max-dispatch + wall-clock timeout as fail-loud limits. Cost is itself a named escalation trigger. |

---

## 10. Reversibility — kill switch at every layer

The codebase already has the pattern (`featureFlags.ts`, default-off `uiV2`). We extend it and add
backend/CLI stops:

| Layer | Kill switch | Effect |
|---|---|---|
| Backend (routes) | `CHROTE_FORMATIONS=off` (default) + restart | `/api/formations/*` not registered; no background goroutine starts. CHROTE behaves exactly as today. |
| CLI | `archon` is a separate binary | Don't build it / `rm` it. The hosted engine is gated by `CHROTE_FORMATIONS` (row 1). |
| UI tab | `chrote-formations-tab` localStorage flag, default false | No tab; `FormationsView` never mounts. |
| Data | everything in `<project>/.formations/` + `~/agents/` | `rm -rf .formations/` — workspace, code, sessions, `.beads/` untouched. |

Enabling Formations changes **no existing default**. "Go back" = unset one env var and leave one UI
flag at its default.

---

## 11. Success criteria

**Slice 1 ("see a formation") — all falsifiable:**

1. An agent runs `archon formation create …`; a `.formations/` board file appears with stable ids.
2. `GET /api/formations/boards/{id}` returns the composed board; the Formations tab renders it.
3. Round-trip: an agent-authored field the UI doesn't model survives an `archon` edit byte-for-byte,
   with a minimal diff (Rule 7).
4. Clean rollback: `chrote-formations-tab` off → no tab; `CHROTE_FORMATIONS=off` + restart →
   `go test ./...` green, `/api/health` OK, all existing PRD acceptance criteria still pass;
   `.formations/` is inert.

**The full proof (by slice 6 — the career/web-experience scenario):**

1. One-touchpoint: Perttu gives one goal to the Archon, never names a socket/session.
2. Real cross-harness run: the ledger shows `dispatch → result(sentinel) → end` to a specialist in a
   different harness; a real artifact exists; the UI shows it live.
3. Visibility: the Archon answers "what came back?" from the ledger (conversationally), and the tab
   shows the same truthfully.
4. Recoverable: disconnect mid-run, return, `archon run resume <bead>` (or the Archon) still correct.
5. Fails loud: kill the specialist pane → visible `error` event; the Archon says it went dark.
6. A gate escalates for Perttu's judgment; `archon gate approve/reject` routes it.

**"The broader system is working":** Perttu routinely works through 2–3 trusted touchpoints and
accesses many specialists/harnesses *without holding the coordination context himself*; two leaders
can collaborate on a blueprint; at least one persistent standards-holder demonstrably improves an
output across sessions; enough is captured to later ask "how is this agent doing?"; and it stays
localhost-only, file-canonical, governance-free, and one-flag-removable.

---

## 12. Decisions & next step

The load-bearing decisions are **made** (recorded in [08-open-questions.md](08-open-questions.md)):
execution is one hosted system accessed identically by CLI + UI; phasing is **vertical full-stack
slices**; the CLI is **`archon`** (missions bead-backed); the **notice board is deferred**. Remaining
choices (first cross-harness pair; write-back mechanics; sentinel convention; ledger text storage;
per-run limits) are Tier-2/3 with recorded defaults and don't block starting.

**Next step:** stand up the work as a **Beads epic** with the slice structure in §5 (tracked in `bd`,
per `CLAUDE.md`). Slice 1 — "see a formation" — is the first child. This plan is design-only; nothing
is committed to CHROTE runtime until a slice ships behind its flags.
