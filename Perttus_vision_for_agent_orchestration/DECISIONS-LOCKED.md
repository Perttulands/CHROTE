# Decisions Locked — CHROTE Formations / Agent Orchestration

> **Authoritative.** Recorded 2026-06-03 with Perttu. Where this document conflicts with any
> sibling doc, **this wins**. It resolves the genuine contradictions that had accumulated across
> the design packets (engine location, CLI name, first-slice definition, build order) and records
> a methodology pivot Perttu made this session (UI + Gherkin as the behavioral source of truth).
>
> Superseded framings are listed in §4. The vision interview
> (`perttus_vision_for_agent_teams_and_orchestration.md`) and the conceptual model
> (`archive/superseded-2026-06-03/data-model-comparison.md` primitives,
> `archive/superseded-2026-06-03/claude/01-07`) still stand except where noted.

---

## 1. Locked decisions

### D1 — Run engine: a shared Go library, invoked by both surfaces
The run engine is a **Go package** (e.g. `internal/formations`) callable by **both** the CHROTE
server (`/api/formations/*`) **and** the `archon` CLI. Neither surface is privileged. The
**on-disk append-only NDJSON ledger is canonical**; status is projected from it; runs survive
restart by replaying it.

This supersedes both earlier framings — "engine lives inside `fm`, CHROTE only reads" (synthesis)
and "engine hosted in the server, CLI spectates" (archived `claude/08`). It satisfies Perttu's actual
requirement — *"accessed through the CLI and the UI same as anything else"* — without making either
the authority. Guard remains: **don't grow a Gas-City worldview** (stay file-backed, fail-loud,
never manage/drain sessions).

### D2 — CLI name: `archon`
The CLI is `archon` — the same name as the root persona, because it is how you command the org.
Nouns: `agent · formation · mission · run · gate`. Missions/runs keyed by Beads IDs. This
supersedes `fm` (synthesis) and `chrotectl` (codex).

### D3 — Agent factory is first-class and built early
Persona cards are **not** merely pointers. A **minimal factory ships early**: `archon agent new` /
introspect, plus a UI path, can **create** persona cards (pointing at existing Hermes / `CLAUDE.md`
/ `.codex` configs) and optionally spawn sessions. Agent-building is a first-class capability from
the start, not a deferred Phase 1.5. This supersedes the "card is a pointer, not a factory; defer
the factory" recommendation.

### D4 — Cross-harness handoff is not a special risk
Cross-harness execution is **just another tmux session** receiving a dispatch. It gets **no
dedicated proof slice or milestone** (overruling the fundamentals doc's "headline risk / prove it
first" framing). The engine still handles the one real fragility — **sentinel detection + idle
timeout → loud `error` event** — so a missed completion fails loud instead of hanging. First pair,
when runs land: **Claude Code + Codex**.

### D5 — Methodology: UI + Gherkin are the behavioral source of truth; the CLI is derived
Build order is **inverted** from the master-plan (which had CLI-first and a read-only UI tab last):

1. A **comprehensive Gherkin (`.feature`) spec** is written first as the **source of truth for what
   the system does**. It is the acceptance backbone and reveals the required `archon` CLI surface,
   `/api/formations` surface, and UI interactions.
2. The **UI is built first and is a first-class write + inspect surface** (not read-only).
3. The **`archon` CLI is derived** from the Gherkin scenarios — exactly the verbs the spec needs.

**This does not override "files are canonical / agents author."** TOML definitions + NDJSON ledger
remain canonical for **persistence**; agents remain primary authors via the shared engine library;
the UI is an additional first-class authoring surface that writes through the same package. "UI as
source of truth" means **source of truth for behavior/requirements**, not the data store.

### D6 — UI is built fresh in React
The Formations UI is **built natively in React/TSX** in the existing dashboard, matching dashboard
conventions, behind the `chrote-formations` feature flag. The vanilla-JS prototype
(`03-formations.html/.js`) is a **behavioral + visual reference** (see D7) — its code is **not
embedded and not evolved in place**; it is re-implemented in React. This supersedes the master-plan's
"port the vanilla-JS canvas as a self-contained imperative component."

### D7 — Permissive direct-manipulation interaction model; the prototype is its canonical reference
The governing UX principle is **"whatever the user expects to work, works."** Concretely, captured
faithfully from `03-formations.js` (whose header comment is itself a requirements doc):

- **Right-click any element** for useful commands. The formation card is a catch-all; element-specific
  menus (agent, formation, header, input row, output row, verification band, slot, gate, wire, mission,
  empty board) layer on top via `stopPropagation`.
- **Connect anything to anything, and it just works.** Connections always go **output → input**;
  pressing an input port grabs its existing wire to reconnect; dropping an output on a gate's **judge**
  socket auto-creates the loop; dropping a judge wire on empty canvas **spawns** a judge formation;
  either end of a committed wire is re-draggable; the middle hand-routes the lane; duplicates and
  self-connections are refused.
- **Composable gate judges**: a gate's judge may be **one formation (a loop) or several formations
  wired in sequence** (chain entry → … → exit, exit returns to the gate). Pushback/revise loops are
  just backward wires; the engine tolerates cycles-via-wire while the auto-route graph stays acyclic.
- **Multi-input JOIN** (a node waits for all inputs — `waiting · N/M inputs`), **fan-out**, **dynamic
  add/remove of input/output ports**, **undo on every mutation**, and **live agent terminals on the
  canvas** are all part of the model.
- **Layout vs structure**: hand-routed wire lanes and node x/y are **presentation** (layout sidecar),
  never structure. Edge/port/node identity is stable and round-trips files↔CLI↔UI.

`03-formations.js` is the **canonical behavioral reference** for the canvas. The Gherkin spec must
capture this interaction model in full — it is the substance of the product, not chrome. The UI
remains an **inspection + light-tweak** surface in spirit (agents do the heavy authoring via the CLI),
but every gesture it exposes must "just work" and write through the shared package (D1, D5).

---

## 2. Settled defaults (carried forward; object if any is wrong)

| Area | Default |
|---|---|
| Definition format | **TOML** (`.formations/boards/*.formation.toml`), layout in a sidecar |
| Ledger / run state | **append-only NDJSON** under `.formations/runs/<board>/` |
| File layout | `.formations/` (sibling of `.beads/`) + central `~/agents/` for persona cards |
| IDs | prefixed ULIDs (`brd_`, `mis_`, `fmn_`, `slot_`, `gate_`, `edge_`…); stable across round-trips |
| The "one-id spine" | persona `id` = card filename = default tmux session stem = slot `agentId` = team ref = ledger key; explicit harness variants may declare their own `session_stem` while keeping `agentId` as the durable key |
| Slot → session binding | **static** (`agentId`), resolve to live session at run; unavailable ⇒ **fail loud**, no silent substitution |
| Single writer | the shared formations package is the only writer of definition files (CLI + UI both go through it) |
| Ledger text | full prompt+reply text by default (localhost, single-user) + a per-run `redact` flag |
| Fail-loud limits | per-run max-dispatch + wall-clock timeout that **record-and-stop** (not approval gates) |
| Beads | work tracking only — never the graph store or comms channel |
| Notice board | **deferred** (stage-1 comms = inline brief + ledger + conversational status via the Archon) |
| Reversibility | `chrote-formations` flag (default off) + `CHROTE_FORMATIONS` env; `rm -rf .formations/` is clean |

---

## 3. Reshaped build sequence (vertical slices, UI + Gherkin first)

Each slice = **Gherkin scenarios → React UI → shared Go formations package → `archon` CLI verbs**,
together. The comprehensive Gherkin spec is drafted up front; slices implement against it.

| # | Slice | Delivers |
|---|---|---|
| **S0** | **Behavioral spec** | The comprehensive `.feature` spec across all feature areas. Derives the CLI/API/UI surface. |
| **S1** | **Agents exist & are authored** | Persona cards (`~/agents/*.toml`) + minimal factory (`archon agent new`/introspect + UI); `/api/agents` left-joined onto Oracle liveness; Agents surface lists them. *(D3)* |
| **S2** | **See & author a formation** | Fresh React canvas + `archon formation create/inspect`; slots reference S1 agent ids; persisted to `.formations/`; round-trip test (unknown fields survive). |
| **S3** | **Assign, wire & build a mission** | Assign agents to slots, wire connections, `archon mission create/wire` — UI + CLI; mission backed by a bead. |
| **S4** | **Run it** | Shared-lib engine dispatches via tmux (cross-harness included), sentinel/idle fail-loud, NDJSON ledger, SSE to UI; `archon mission run`. |
| **S5** | **Recover & decide** | `archon run resume/abort`, `archon gate approve/reject` — UI + CLI. |

Whole-system acceptance: the career/web-experience scenario (master-plan §11), expressed as Gherkin
and runnable behind the flags.

---

## 4. Superseded / affected docs

| Doc | Status |
|---|---|
| `archive/superseded-2026-06-03/synthesis-and-decisions.md` | **Superseded** on engine location (recommended CLI-hosted) and CLI name (`fm`). |
| `archive/superseded-2026-06-03/questions-and-decisions.md` | **Superseded** — its Tier-1 "OPEN" items are resolved here; Q6-Q10 answers updated by D3/D5. |
| `archive/superseded-2026-06-03/fundamentals-and-vertical-slices.md` | **Superseded** on build order (headless/CLI-first, UI last) and on cross-harness as "headline risk" (D4, D5). The F1-F4 *engineering concerns* (atomic writes, ledger projection, sentinel handling, binding) still hold as implementation requirements. |
| `archive/superseded-2026-06-03/data-model-comparison.md` | "Decide engine location first; if server-hosted reconsider JSON" — **resolved**: shared-lib engine (D1), **keep TOML** (format args are independent of who hosts the engine). Primitives table still stands. |
| `archive/superseded-2026-06-03/claude/00-master-plan.md`, `archive/superseded-2026-06-03/claude/08-open-questions.md` | **Partially superseded**: engine is shared-lib not server-hosted (D1); UI is first/first-class/React not last/read-only/ported (D5, D6); factory is early not deferred (D3). Architecture, one-id spine, Gas-City guardrails, risk register still stand. |
| `archive/superseded-2026-06-03/codex/*` | **Superseded** where it differs (JSON, `chrotectl`, `.chrote/orchestration/`, notice-board-in-v0). |

---

## 5. Next step

Write the **comprehensive Gherkin spec (S0)** — the behavioral source of truth — then stand up the
work as a **Beads epic** with the §3 slice structure (tracked in `bd`, per `CLAUDE.md`). Nothing is
committed to CHROTE runtime until a slice ships behind its flags.
