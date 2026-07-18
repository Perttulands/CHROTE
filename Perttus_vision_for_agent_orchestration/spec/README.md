# CHROTE Formations — Behavioral Spec (S0)

This `spec/` is the supporting **S0 behavioral baseline** for the agent-orchestration system. It
preserves the Gherkin-first intent recorded in [DECISIONS-LOCKED.md](../DECISIONS-LOCKED.md), but it
does not override current root specs, current code, the
[source-truth index](../../docs/source-truth-index.md), or later accepted ADRs. Scenarios that
conflict with ADR-0005, ADR-0006, or ADR-0007 must be updated before they can serve as implementation
acceptance.

The Gherkin `.feature` files describe intended cross-surface behavior. The `archon` CLI,
`/api/formations` + `/api/agents`, and React UI should converge on that behavior only after its
status is reconciled with the current/accepted-target labels in the newer sources.

Three historical inputs inform this packet:
- **The vision** — `../perttus_vision_for_agent_teams_and_orchestration.md` (the why).
- **The prototype** — `../03-formations.html` / `../03-formations.js` is a visual
  and interaction reference for the canvas. It does not override current graph,
  persistence, runtime, or availability contracts.
- **The supporting contracts** — [`contracts.md`](contracts.md) preserves the S0
  schema/API/addressing baseline and accepted-target amendments. Current root
  specs, code, the source-truth index, and later ADRs win on conflict.

> Status: supporting historical/target packet. Current Formations implementation
> already exists. These runner-agnostic `.feature` files become executable
> acceptance only when an owning slice reconciles and wires them to the current
> Go and dashboard test stacks.

---

## How to read a scenario

Each scenario is tagged with the surface(s) it derives:

| Tag | Meaning |
|---|---|
| `@cli` | An `archon` CLI behavior — read the verbs/flags out of these. |
| `@api` | A coordinator HTTP/API behavior shared by CLI and UI clients. |
| `@ui` | A React UI gesture/behavior (the prototype's interaction model). |
| `@file` | A persisted-state guarantee (TOML definition / NDJSON ledger). |
| `@security` | A fail-closed authority, redaction, or exposure boundary. |
| `@layout` | A presentation-only effect (node x/y, wire lanes) — sidecar, never structure. |
| `@e2e` | Whole-system acceptance. |

A capability typically appears as a UI gesture **and** an equivalent `archon` verb **and** a file
effect. UI and Archon definition edits use the one shared definition serializer (D1), and structure
must round-trip files↔CLI↔UI (D7). Schema-2 runtime commands instead go through ADR-0007's sole
fenced workspace coordinator.

---

## The model in one breath

A **mission** emits one validated work payload from fixed `out` into a directed graph of
**formations** (agent execution in `solo` / `peer` / `flow` / `orchestrated` coordination),
accepted-target **Tools** (pure bounded transformations through frozen host-owned profiles), and
**gates** (human/code/formation evaluation and routing). **Connections** bind one output producer to
one exact input port; JOINs use multiple distinct required ports. A Gate pass preserves the evaluated
work and provenance, while fail creates one stable typed `gate_feedback` object whose traversals may
perform correction or exact-attempt pushback; formation-judge output is verdict metadata only. A
**run** records immutable attempt inputs and append-only **ledger** events from which status is
projected.

---

## Historical/projected `archon` CLI surface

Read out of the scenarios; this is not an inventory of the current binary. See
the current command list in [`../../ARCHON.md`](../../ARCHON.md). Global flags:
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

# boards & layout (canvas.feature)
archon board arrange <boardId|slug> [projected]       # explicit full-layout mutation

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

# gates & judges (gates-and-judges.feature, escalation.feature)
archon gate create <board> --kinds code,human --criterion "…"
archon gate judge  <board> <gate> (--chain f1,f2,f3 | --detach)
archon gate judge  <board> <gate> (--attach <formation> | --return <formation>:<port>) [projected]
archon gate duplicate <board> <gate> [projected]
archon gate rm <board> <gate> [projected]
archon gate approve <runId> <gateId> --reason "…"      # human verdict during a run
archon gate reject  <runId> <gateId> --reason "…"

# missions & runs (mission.feature, run-execution.feature, run-recovery.feature, visibility.feature)
archon mission create <board> --title "…" --goal "…"          # bead-backed
archon mission run <board>
archon mission inspect <board> [projected]
archon mission list [projected]
archon run status <runId|unique-selector>
archon run logs   <runId|unique-selector> [--node <id>] [--follow]
archon run resume <runId>
archon run cancel <runId> [projected]              # canonical target command
archon run abort  <runId>                          # current spelling; target alias normalizes to cancel
archon run list [projected]
```

Workflow-definition authoring, validation, and inspection remain available
offline. Run start/resume/cancel/verdict and run list/status/log/follow require
the coordinator's sanitized API projection; Archon does not read private run
authority or fall back to a local engine.

Agent tag flags target `[card].tags`: `--capable`, `--add-capability`, and
`--remove-capability` operate on bare capability tags, while `--personality x`
stores `personality:x` alongside reserved `focus:` and `taste:` facets.

Wiring (`wire`/`unwire`) is the single verb for compatible workflow payload
edges—Mission/Formation/Tool/Gate routing—and for the Gate's reserved `judge`
evaluation-control relationship. The `judge` socket permits one send and one
return and never carries downstream work. Node positions and hand-routed wire
lanes are **layout**, never structure. Creation may place only the new element
through the shared heuristic. Full-board layout changes require the explicit UI
Arrange action or accepted-target `archon board arrange`; no other verb mutates
existing user arrangement.

---

## Addressing, ids, and ports

- **Connection address:** `<nodeId>:<portId>` → `<nodeId>:<portId>`. Workflow
  payload edges are always compatible output → declared input; `gate:judge` is
  the named evaluation-control exception described above.
- **Stable ids are real addresses:** board, node, slot, port, edge, and run mutations use the prefixed
  ids in [`contracts.md`](contracts.md). Slugs, titles, and aliases may resolve to ids before mutation,
  but persisted references use ids.
- **Fixed addresses:** Gate = `in` / `pass` / `fail` plus reserved control socket
  `judge`; Mission = `out`. Formation and accepted-target Tool input/output
  ports are directional, typed, and stable.
- **Port aliases:** `in[N]` / `out[N]` in feature examples are documented shorthand for "the
  input/output port at creation-order N." The writer resolves the alias once to the stable `port_…` id
  and holds that id through reloads and port removals.
- **ID prefixes (ULID-based, self-describing):** `brd_` board · `mis_` mission · `fmn_` formation ·
  accepted-target `tool_` · `slot_` · `gate_` · legacy schema-1 `ver_` verification · `port_` · `edge_` · `run_` · private `wsa_` workspace authority · client `cmd_` runtime command. `wsa_` and `cmd_` use canonical uppercase ULID grammar `^(wsa|cmd)_[0-7][0-9A-HJKMNP-TV-Z]{25}$`; command ids are validated before path construction. Ids round-trip
  files↔CLI↔UI; edits are diffs against existing ids, never full rewrites.

Schema-1 inline verification is read-only compatibility/migration input. The
schema-2 CLI and UI do not author, configure, or remove it; validation fails
`legacy_inline_verification_requires_migration` until `ctx-ug7.17` defines or
retires the feature.

## Ledger event vocabulary (NDJSON, append-only; status is projected)

```
run_started · run_activated · node_waiting · node_input_ignored · node_started · slot_binding_observed · slot_dispatch ·
slot_peek_capability_issued · slot_steering_started · slot_steering_ended · slot_peek_capability_revoked ·
slot_reconciliation_interrupt · slot_reconciliation_interrupt_outcome · slot_result ·
formation_result · tool_dispatch · tool_process_launch · tool_result · node_output ·
gate_evaluating · gate_kind_result · judge_result · judge_attempt_failed · gate_verdict · artifact_attached · artifact_observed ·
escalation_raised · human_input_requested · human_verdict_recorded ·
error · run_blocked · run_resumed · run_cancel_requested · run_canceled ·
run_failure_reconciliation_started · run_failed · run_succeeded
```

Every event uses the envelope in [`contracts.md`](contracts.md). `run_started`
includes run id, board id/revision, opaque workspace/run authority ids,
admission command id/hash, workspace admission sequence, current writer fence,
exact admission-policy revision/hash, graph/private-binding/safe-projection hashes, conditional Mission id,
monotonic event sequence, actor, and initial attempt/epoch; it exposes no
private snapshot path or bytes. `formation_result` durably closes ordinary
Formation aggregation before `node_output`, so recovery never reparses mutable
capture or redispatches completed work.
An admitted run with only `run_started` projects `queued`; its unique fenced
`run_activated` binds the exact immutable activation-policy revision/hash,
projects `running`, and precedes graph/dispatch events.
Every activated non-final run retains its `maxActiveRuns` slot through blocked,
human-waiting, canceling, and failing states until execution finality in this first
contract.
Every failure first fsyncs `run_failure_reconciliation_started`, which projects
`failing` and freezes exact cause/resource snapshots; `run_failed` must name
that start and byte-match its failure header while disposing those snapshots.
Execution-final events are exactly `run_succeeded`, `run_failed`, and
`run_canceled`. Non-authorizing binding/artifact observation events may follow for inspection, but
cannot reopen an epoch, change outcome, or authorize dispatch. `run_blocked`
stops the current epoch; only a block with `resumeAllowed=true` can be resumed
explicitly with `run_resumed`.

## Sentinels (completion + escalation over tmux, no native ACK)

```
<<<CHROTE-DONE     run-id="…" dispatch-id="…" target-lease-id="…" status="ok|error|needs-review" artifact="…">>>
<<<CHROTE-ESCALATE run-id="…" reason="…">>>
```
A completion sentinel must exact-match the active run, dispatch, and host target
lease; run id alone is insufficient. An escalation must match the active run.
Stray/fake markers are ignored. Captured pane text is recorded as **data**,
never executed.

## File layout and current availability

```
~/agents/<id>.toml                                  # central persona cards (cross-project)
<workspace>/.formations/boards/<board>.formation.toml   # definition (structure) — TOML
<workspace>/.formations/layout/<board>.layout.toml      # presentation (x/y, lanes) — sidecar
<workspace>/.formations/artifacts/<run-id>/...          # registered sanitized files only
<chrote-data>/formations/workspaces/registry.lock
<chrote-data>/formations/workspaces/registry.private.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/workspace.bootstrap.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/workspace.private.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/owner.lock
<chrote-data>/formations/workspaces/<workspace-authority-id>/owner.private.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/admission-policies/<policy-rev>.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/commands/  # private command journal
<chrote-data>/formations/workspaces/<workspace-authority-id>/runs/<run-id>/events.ndjson
<chrote-data>/formations/workspaces/<workspace-authority-id>/runs/<run-id>/graph.snapshot.toml
<chrote-data>/formations/workspaces/<workspace-authority-id>/runs/<run-id>/bindings.private.toml
<chrote-data>/formations/workspaces/<workspace-authority-id>/runs/<run-id>/refs/
<chrote-data>/formations/workspaces/<workspace-authority-id>/quarantine/
# .formations/board.ndjson (notice board) — DEFERRED
```
Canonical run authority is outside every generic Files root. Run APIs expose
sanitized hash-linked projections; File Peek receives only currently authorized
registered artifacts, never raw ledger/binding/ref paths.
Formations is now always-on; the historical `chrote-formations` and
`CHROTE_FORMATIONS` default-off flags are not current availability contracts.
Executor-specific environment guards remain a safety ladder, not a product
feature switch. Rollback preserves `.formations/` evidence and reverts code; it
does not delete the canonical workspace data. The shared formations package is
the **single serializer** of definition files; the current fenced server
coordinator is the sole semantic writer of schema-2 runtime authority.

---

## Feature index

| File | Covers |
|---|---|
| `contracts.md` | supporting S0 schema/API/addressing/ledger baseline plus accepted-target amendments |
| `agents.feature` | discovery (progressive disclosure), inspection, liveness via Oracle |
| `agent-factory.feature` | create / introspect / evolve / spawn / attach / retire agents (D3) |
| `formations-and-slots.feature` | the four types, slots, controller, staffing, briefs |
| `connections.feature` | compatible payload wiring, reconnect, hand-route, JOIN, fan-out, dynamic ports |
| `gates-and-judges.feature` | gate kinds, pass/fail/block/pushback, **single + chained judges** |
| `verification.feature` | schema-1 inline-verification inspection and schema-2 fail-closed boundary |
| `mission.feature` | entry point, chain, bead-backed, start |
| `run-execution.feature` | cascade, JOIN readiness, gate routing, judge execution, outputs, cancel, limits |
| `run-recovery.feature` | resume/replay, fail-loud binding/sentinel/limit failures |
| `briefs-and-io.feature` | input editor (goal/bead/files), output report (artifacts/diffs), bead resolution |
| `canvas.feature` | pan/zoom/fit, arrange (layout), undo, on-board terminals |
| `context-menus.feature` | right-click everything — the per-element menus |
| `terminals.feature` | live agent terminals on the canvas |
| `escalation.feature` | escalation sentinel → ledger → Archon; human verdicts |
| `visibility.feature` | historical explainability baseline; current Formations UI/root specs win on availability |
| `reversibility-persistence.feature` | historical flag scenarios plus current atomic-write, single-writer, and schema-versioning baseline |
| `career-web-experience.feature` | the whole-system end-to-end acceptance |

---

## Current continuation

The historical S1/S2 sequencing has been superseded: current main already has a
real Formations foundation. ADR-0005, ADR-0006, and ADR-0007 constrain the stabilization and
mixed-workflow slices tracked under Beads epic `ctx-ug7`. Each target becomes a
current claim only after its owning implementation and certification gates pass.
