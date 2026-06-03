# CHROTE Formations — Behavioral Spec (S0)

This `spec/` is the **behavioral source of truth** for the agent-orchestration system (per
[DECISIONS-LOCKED.md](../DECISIONS-LOCKED.md), **D5**). The Gherkin `.feature` files describe **what
the system does**; the `archon` CLI surface, the `/api/formations` + `/api/agents` surfaces, and the
React UI are all **derived from these scenarios**, not designed separately.

Two upstream sources ground every scenario:
- **The vision** — `../perttus_vision_for_agent_teams_and_orchestration.md` (the why).
- **The prototype** — `../03-formations.html` / `../03-formations.js` is the **canonical behavioral
  reference for the canvas** (D7). Its header comment is itself a requirements doc; the interaction
  scenarios here are extracted from its actual code, not imagined.
- **The active contracts** — [`contracts.md`](contracts.md) is the live schema/API/addressing contract.
  Archived design packets are background only.

> Status: design-phase spec. Runner-agnostic `.feature` files. When implementation starts, wire
> them to **godog** (Go engine/CLI) and **cucumber-js / playwright-bdd** (React UI). Relocatable
> into the repo's test tree at that point.

---

## How to read a scenario

Each scenario is tagged with the surface(s) it derives:

| Tag | Meaning |
|---|---|
| `@cli` | An `archon` CLI behavior — read the verbs/flags out of these. |
| `@ui` | A React UI gesture/behavior (the prototype's interaction model). |
| `@file` | A persisted-state guarantee (TOML definition / NDJSON ledger). |
| `@layout` | A presentation-only effect (node x/y, wire lanes) — sidecar, never structure. |
| `@e2e` | Whole-system acceptance. |

A capability typically appears as a UI gesture **and** an equivalent `archon` verb **and** a file
effect — because the UI and CLI are two clients of one writer (D1), and structure must round-trip
files↔CLI↔UI (D7).

---

## The model in one breath

A **mission** (entry point, bead-backed) seeds a goal into a directed graph of **formations**
(coordination units: `solo` / `peer` / `flow` / `orchestrated`, holding **slots** that reference live
agents, a **brief**, an optional in-line **verification**, and N **input** / N **output** ports) and
**gates** (checkpoints with `in` / `pass` / `fail` / `judge`, combining `code` / `human` / `formation`
checks). **Connections** are directed `output → input` edges. A **run** cascades work along the wires,
waits at JOINs, routes at gates, can have a gate judged by **one formation or a chain of them**, and
produces append-only **ledger** events that status is projected from.

---

## Derived `archon` CLI surface

Read out of the scenarios; this is the consolidated list (not a separate design). Global flags:
`--json`, `--workspace`; fail-loud non-zero exits; idempotent where noted. Commands marked
`[projected]` are intentionally not executable acceptance yet; the scenario lands with the owning slice
before implementation.

```
# agents (agents.feature, agent-factory.feature)
archon agent list [--json] [--capable <tag>] [--assignable]
archon agent inspect <id> [--json]
archon agent new <id> --kind <kind> --harness <h> [--capable a,b] [--personality x] [--from <path>]
archon agent edit <id> [--add-capability t|--remove-capability t] [--add-harness h --session-stem s] [--note "…"]
archon agent spawn <id>
archon agent attach <id>
archon agent retire <id> [--force]

# formations & slots (formations-and-slots.feature, connections.feature)
archon formation create <boardId|slug> <type> --title "…"
archon formation list [projected] | inspect <boardId|slug>
archon formation assign <boardId|slug> <formationId|alias> --slot <slotId> --agent <id> [--harness <h>]
archon formation wire   <board> <from>:<port> <to>:<port>
archon formation unwire <board> <from>:<port> <to>:<port>
archon formation add-input <board> <formation> [--label "…"]
archon formation add-output <board> <formation> [--label "…"]
archon formation remove-input <board> <formation> <portId>
archon formation remove-output <board> <formation> <portId>
archon formation rename <board> <formation> "…" [projected]
archon formation set-type <board> <formation> <type> [projected]
archon formation duplicate <board> <formation> [projected]
archon formation rm <board> <formation> [projected]
archon formation set-brief <board> <formation> [--goal "…"] [--bead bd-NNN] [--file p] [--remove-file p]
archon formation run <board> <formation>          # single-node test run

# gates & judges (gates-and-judges.feature, verification.feature, escalation.feature)
archon gate create <board> --kinds code,human --criterion "…"
archon gate judge  <board> <gate> (--chain f1,f2,f3 | --detach)
archon gate judge  <board> <gate> (--attach <formation> | --return <formation>:<port>) [projected]
archon gate duplicate <board> <gate> [projected]
archon gate rm <board> <gate> [projected]
archon gate approve <runId> <gateId> --reason "…"      # human verdict during a run
archon gate reject  <runId> <gateId> --reason "…"
archon verification add <board> <formation> --kinds … --criterion "…" [--onfail block|pushback]
archon verification config <board> <formation> [projected]
archon verification rm <board> <formation> [projected]

# missions & runs (mission.feature, run-execution.feature, run-recovery.feature, visibility.feature)
archon mission create <board> --title "…" --goal "…"          # bead-backed
archon mission run <board>
archon mission inspect <board> [projected]
archon mission list [projected]
archon run status <runId|unique-selector>
archon run logs   <runId|unique-selector> [--node <id>] [--follow]
archon run resume <runId>
archon run abort  <runId>
archon run list [projected]
```

Agent tag flags target `[card].tags`: `--capable`, `--add-capability`, and
`--remove-capability` operate on bare capability tags, while `--personality x`
stores `personality:x` alongside reserved `focus:` and `taste:` facets.

Wiring (`wire`/`unwire`) is the single verb for all edges — mission→formation, formation→gate,
gate `pass`/`fail`→next, and formation↔gate `judge`. Node positions and hand-routed wire lanes are
**UI/layout** only and have no CLI verbs in stage 1.

---

## Addressing, ids, and ports

- **Connection address:** `<nodeId>:<portId>` → `<nodeId>:<portId>` (always output → input).
- **Stable ids are real addresses:** board, node, slot, port, edge, and run mutations use the prefixed
  ids in [`contracts.md`](contracts.md). Slugs, titles, and aliases may resolve to ids before mutation,
  but persisted references use ids.
- **Fixed ports:** gate = `in` / `pass` / `fail` / `judge`; mission = `out`. Formation input/output
  ports are dynamic with stable ids.
- **Port aliases:** `in[N]` / `out[N]` in feature examples are documented shorthand for "the
  input/output port at creation-order N." The writer resolves the alias once to the stable `port_…` id
  and holds that id through reloads and port removals.
- **ID prefixes (ULID-based, self-describing):** `brd_` board · `mis_` mission · `fmn_` formation ·
  `slot_` · `gate_` · `ver_` verification · `port_` · `edge_` · `run_`. Ids round-trip
  files↔CLI↔UI; edits are diffs against existing ids, never full rewrites.

## Ledger event vocabulary (NDJSON, append-only; status is projected)

```
run_started · node_waiting · node_started · slot_dispatch · slot_result · node_output ·
gate_evaluating · gate_verdict · verification_verdict · artifact_attached ·
escalation_raised · human_input_requested · human_verdict_recorded ·
error · run_blocked · run_canceled · run_failed · run_succeeded
```

Every event uses the envelope in [`contracts.md`](contracts.md). `run_started` includes run id, board id,
board revision or snapshot, mission id, monotonic sequence, actor, and initial attempt/epoch. Terminal
events are exactly `run_succeeded`, `run_failed`, `run_blocked`, and `run_canceled`.

## Sentinels (completion + escalation over tmux, no native ACK)

```
<<<CHROTE-DONE     run-id="…" status="ok|error" artifact="…">>>
<<<CHROTE-ESCALATE run-id="…" reason="…">>>
```
The run-id must match the active run (stray/fake markers are ignored). Captured pane text is recorded
as **data**, never executed.

## File layout & flags

```
~/agents/<id>.toml                                  # central persona cards (cross-project)
<workspace>/.formations/boards/<board>.formation.toml   # definition (structure) — TOML
<workspace>/.formations/layout/<board>.layout.toml      # presentation (x/y, lanes) — sidecar
<workspace>/.formations/runs/<board>/<run-id>.ndjson    # append-only run ledger (+ latest.json)
# .formations/board.ndjson (notice board) — DEFERRED
```
Kill switches: `chrote-formations` (UI localStorage flag, default off) · `CHROTE_FORMATIONS` (server
env, default off). `rm -rf .formations/` is a clean rollback. The shared formations package is the
**single writer** of definition files.

---

## Feature index

| File | Covers |
|---|---|
| `contracts.md` | active schema/API/addressing/ledger contract |
| `agents.feature` | discovery (progressive disclosure), inspection, liveness via Oracle |
| `agent-factory.feature` | create / introspect / evolve / spawn / attach / retire agents (D3) |
| `formations-and-slots.feature` | the four types, slots, controller, staffing, briefs |
| `connections.feature` | connect-anything output→input, reconnect, hand-route, JOIN, fan-out, dynamic ports |
| `gates-and-judges.feature` | gate kinds, pass/fail/block/pushback, **single + chained judges** |
| `verification.feature` | in-formation check, kinds, block/pushback |
| `mission.feature` | entry point, chain, bead-backed, start |
| `run-execution.feature` | cascade, JOIN readiness, gate routing, judge execution, outputs, cancel, limits |
| `run-recovery.feature` | resume/replay, fail-loud binding/sentinel/limit failures |
| `briefs-and-io.feature` | input editor (goal/bead/files), output report (artifacts/diffs), bead resolution |
| `canvas.feature` | pan/zoom/fit, arrange (layout), undo, on-board terminals |
| `context-menus.feature` | right-click everything — the per-element menus |
| `terminals.feature` | live agent terminals on the canvas |
| `escalation.feature` | escalation sentinel → ledger → Archon; human verdicts |
| `visibility.feature` | explainable-on-request; the optional read-only tab |
| `reversibility-persistence.feature` | kill switches, atomic writes, single writer, schema versioning |
| `career-web-experience.feature` | the whole-system end-to-end acceptance |

---

## Next step

With this spec and [`contracts.md`](contracts.md) as the contract, S1/S2 can start behind
`chrote-formations` and `CHROTE_FORMATIONS`. Beads planning is a separate step outside this S0 cleanup.
Nothing is committed to CHROTE runtime until a slice ships behind its flags.
