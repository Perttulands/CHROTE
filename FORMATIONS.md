---
type: spec
status: active
authority: source-of-truth
workspace: chrote
enforced_by: scripts/doc-lint.py
---

# CHROTE Formations Spec

Status: **Active core source of truth**.

Formations is CHROTE's spatial model for organizing AI agents into missions,
teams, gates, and recoverable runs. It pairs with `ARCHON.md` for the CLI surface
and `DATA-MODEL.md` for persistence and event formats.

## One-sentence definition

**Formations is CHROTE's file-backed command-and-control layer for composing AI
agents into executable teams, with CHROTE as the spatial cockpit and `archon` as
the exact command surface.**

## Product purpose

The problem is not running one agent. The problem is operating a durable personal
AI organization:

- persistent personas with identities, capabilities, harnesses, and standards;
- temporary or reusable teams assembled for specific work;
- work that must be started, observed, redirected, gated, resumed, or aborted;
- human judgment entering at mission/gate/taste moments, not every substep;
- agent-native automation that is scriptable and reproducible.

Formations gives that organization a concrete operating model:

- humans see and steer the work spatially in CHROTE;
- agents and scripts author and execute the same work through `archon`;
- both surfaces round-trip through the same files, shared Go package, and run
  ledger.

## Collaboration model

Formations exist to enable capable agents to collaborate, not to reduce them to
fixed pipeline steps. The system should give high-tier models enough context,
tools, and visibility to use their intelligence inside the team setting while
CHROTE/Archon keeps the work reproducible, bounded, and observable.

- `solo` is one agent working with a clear brief and output contract.
- `flow` is ordered handoff where the sequence itself is the coordination model.
- `peer` is collaborative work without hierarchy: agents share the brief, use a
  shared run plane (for example an append-only chat/blackboard file) to converse,
  inspect or critique each other's work as the available tools allow, and
  converge on a synthesis or set of artifacts. Archon should seed the first turn
  with the task and enough team context to get the group moving; peers then read
  the shared plane, decide what to say or do next, write their contribution, and
  may wait, inspect sibling sessions with scoped tools, or continue work. A
  lightweight facilitator may nudge stuck peers, detect loops, or surface
  problems, but it must not become a hidden hierarchy or fixed choreography.
- `orchestrated` is leader-driven collaboration: one appointed agent owns team
  coordination, but it should steer through practical affordances such as
  prompting worker sessions, inspecting/capturing session state, collecting
  artifacts, using monitors or subagents to surface key worker status, requesting
  revisions, running or requesting checks, and deciding when the formation is
  ready to finish. Those affordances may be native tools the agent already uses
  well, such as `tmux` CLI against scoped session names, or Archon helpers where
  formation context, lookup, provenance, or UI projection adds value.

Do not treat peer or orchestrated formations as a rigid choreography. Archon and
the runtime provide the team roster, scoped session mapping, redaction/output
caps, artifact collection, and ledger evidence; the agents supply the judgment
about how to collaborate. Do not build Archon commands that merely duplicate
standard terminal skills unless they add formation semantics, safety, or durable
evidence.

## Core nouns

| Noun | Meaning |
| --- | --- |
| Agent | Durable persona card with stable id, display name, capabilities, and harness variants |
| Formation | Typed coordination unit: `solo`, `peer`, `flow`, or `orchestrated` |
| Slot | Role position inside a formation, optionally assigned to an agent id |
| Mission | Concrete goal and entry point for a run, usually linked to work state |
| Board | A **Mission Board**: exactly one mission (its identity) plus the formations, gates, connections, and layout that carry that mission out |
| Connection | Directed edge from an output port to an input port |
| Gate | Routing checkpoint that passes, fails, blocks, or loops work |
| Verification | Inline check local to a formation |
| Run | Execution instance that binds slots, dispatches work, records events, and projects state |
| Ledger | Append-only event history for a run |

## Mission Boards

A board is a **Mission Board**: it has exactly one mission, and that mission is
the board's identity. There is no generic multi-mission board; you do not create
an empty board and then add missions to it. A board and its single mission are
born together in one atomic write, so there is never a window where an empty
board exists on disk.

- Creating a board creates its mission: `POST /api/formations/boards
  {slug, title, mission:{goal, ...}}` and `archon board new <slug> --title <t>
  --goal <g> [--bead <id>]` both create the board and its mission atomically.
- The one-mission invariant is enforced, not assumed: adding a second mission is
  refused, and `ValidateBoard` records a `mission_count` error when a board has
  anything other than exactly one mission (`board validate` exits non-zero and
  `doctor files` flags such a board as a hard problem). Edit the existing mission
  with `mission set-goal`; do not add another.
- Running a board runs its mission. `POST /api/formations/runs {board}` with no
  explicit mission runs the board's single mission.
- Board summaries carry their mission and latest run, so a gallery can render
  goal plus status without a follow-up read per board.

This makes the board the unit of work a human points at: one board, one mission,
one goal, observable from a gallery.

## Primitive graph model

Formations is built from a small set of loose primitives:

- **Input port.** A named incoming stream on a mission, formation, or gate.
  By default input ports are required: if a node has multiple incoming required
  inputs, it is a join point and cannot run until every connected input has
  delivered. The ledger records that as `node_waiting` with `readyInputs` and
  `totalInputs`.
- **Output port.** A named outgoing stream produced by a run. One output may feed
  one downstream node or fan out to many. A node may also produce multiple
  distinct outputs, each with its own payload.
- **Formation.** A coordination card with a brief, slots, assigned agents, and
  one or more input/output ports. Arriving input payloads plus the brief are the
  task; produced output payloads continue the graph.
- **Gate.** A routing checkpoint using the same port graph. Basic script gates
  run explicit operator-authored checks such as tests, lint, typecheck, or smoke
  commands. Judge gates run a judge formation and interpret its verdict. The
  important contract is the routing result: pass/fail output ports carry the
  verdict payload into whatever nodes are connected downstream. Human gates are
  a simple stop-for-inspection mechanism and can stay minimal until the script
  and judge paths are solid.

The UI mirrors this model on every formation card:

```text
[input ports / incoming streams]
[formation card: brief, type, slots, assigned agents, local state]
[output ports / outgoing streams]
```

This matches the reference prototype: create primitives, connect output ports to
input ports, and let work cascade as inputs become ready.

## Required invariants

1. **One model, multiple surfaces.** UI gestures and `archon` verbs must use the
   same files and shared Go package.
2. **Files are canonical for persistence.** Browser state is not durable truth.
3. **Ledger is canonical for run history.** Status is projected from append-only
   events.
4. **Stable ids matter.** Node, port, edge, slot, agent, board, mission, gate,
   and run ids must survive round-trips.
5. **Layout is not structure.** Node positions and wire lanes live in layout
   sidecars, not board definitions.
6. **Execution context fails loud.** Missing or ambiguous sessions, harnesses,
   checks, cwd, or agents cannot silently substitute.
7. **Formations is always-on.** It is a permanent first-class surface, not a
   feature flag. The only Formations env vars are the executor safety ladder
   (`CHROTE_FORMATIONS_LAB_*` / `CHROTE_FORMATIONS_TMUX_*`, including
   `CHROTE_FORMATIONS_TMUX_DEDICATED` for the dedicated formations socket), which
   gate execution-environment promotion, never feature availability.
8. **Beads can anchor missions; it is not the graph store.**
9. **No command-execution landmines.** Free-text criteria never become implicit
   shell execution. Executable checks require explicit operator-authored config
   and guardrails.
10. **Ports carry concrete payloads.** Connections route the payload attached to
   their source output port. Every formation emits `node_output.outputs` keyed by
   stable output port ids; missing or unknown output ids block loudly instead of
   silently broadcasting a blob.

## Port payload contract

Connections are not just visual arrows. A wire from `source:summary` to
`writer:input` carries the `summary` payload from `source`'s `node_output.outputs`
map into `writer`'s `node_started.inputRefs[]`. That input ref records the
`edgeId`, `fromNodeId`, `fromPortId`, `toPortId`, payload text, and optional
artifact/report refs.

Fan-out is just multiple connections from the same output port: each downstream
input receives that output port's payload.

Fan-in/join is modeled precisely. A single input endpoint is single-source:
exactly one connection may terminate on a given input port. `WireFormationPorts`
rejects a second connection into the same input endpoint rather than silently
merging or overwriting payloads. A join is therefore not "many wires into one
port"; it is modeled as MULTIPLE distinct required input ports on the same node,
one connection per port. Such a node records `node_waiting` with `readyInputs`
and `totalInputs` and does not dispatch until every required input port has
delivered its payload. This keeps every delivered payload attributable to a
specific input port and makes the join's readiness condition explicit.

There is one routing contract: `node_output.outputs[portId]`. Free-form
`node_output.text` is display summary only and never feeds graph edges.
Tmux-backed agents receive this contract as a fenced `chrote-outputs` JSON block
instruction; lab runs synthesize deterministic payloads for every output port.
If a formation omits a declared output or emits an unknown output id, the run
blocks and records `missing_output_payload` or `invalid_output_payload`.

## Strict verdict contract

Gate routing and inline verification are fail-closed by design.

A gate judge verdict and an inline verification verdict must each be exactly the
token `pass` or exactly the token `fail`. A judge formation's verdict is read
from its display output text and must BE exactly that token; the engine does not
fuzzy-match, infer intent from prose, or treat "looks like a pass" as a pass.

Any ambiguous or unrecognized verdict BLOCKS the run loudly and routes neither
pass nor fail:

- an unrecognized gate verdict records a non-resumable `run_blocked` with code
  `ambiguous_gate_verdict`;
- an unrecognized inline verification verdict records a non-resumable
  `run_blocked` with code `ambiguous_verification_verdict`.

The block is non-resumable on purpose: resume must not reinterpret the offending
text. The judge formation, gate evaluator, or verification must be fixed and a
new run started. This is fail-closed: an ambiguous verdict never silently picks a
branch the verdict never authorized.

## Interaction model

The reference interaction model is permissive direct manipulation:

- right-click meaningful elements for local commands;
- drag agents from roster into slots;
- create missions, formations, gates, and templates from the canvas;
- edit briefs and verification through local popovers;
- connect, reconnect, route, and remove wires directly;
- start missions and see work cascade through the graph;
- project run status onto cards, gates, wires, and outputs;
- support undo for structural mutations.

Product principle:

> Whatever the user reasonably expects to work on the canvas should work.

Permissive gestures do not mean invalid persisted state. Gestures normalize into
valid model operations or fail loudly with an understandable reason.

In CHROTE this surface is the **Missions** tab: a Mission Board gallery showing
each board's mission, goal, and latest run, opening into the board canvas. A
session side-panel bound to the dedicated formations socket
(`/api/formations/tmux/*`, `/terminal-formations/`) lets an operator watch and
attach the mission agents' sessions without touching the cockpit socket.

## Prototype references

The local prototype remains the observable UI reference:

- `Perttus_vision_for_agent_orchestration/03-formations.html` is the visual and
  geometry reference.
- `Perttus_vision_for_agent_orchestration/03-formations.js` is the behavioral
  reference for observable canvas interactions.

Mock timings, canned outputs, in-memory ids, and prototype-only code structure
are not persistence or engine contracts.

## Run and recovery model

A correct run model:

1. A mission or formation starts a run.
2. The engine snapshots the board and resolves slot bindings to live sessions.
3. Work dispatches to selected agent harnesses.
4. Events append to the NDJSON ledger.
5. Projected state updates UI, API, and CLI.
6. Join points wait for required inputs.
7. Gates evaluate code, human, or formation judges and route pass/fail/pushback.
8. Timeouts, missing sessions, sentinel failures, ambiguous checks, and blocked
   gates record loud events.
9. Runs can be inspected, followed, resumed, or aborted from `archon` and
   reflected in the UI.

Watching is optional. Recovery must not depend on a browser tab staying open.

## Execution environments

Formations execution uses explicit environment selection. Each step is a visible
configuration decision, never a silent fallback. Mission agents never run on the
cockpit tmux socket: the executor always refuses it.

1. **Lab.** `CHROTE_FORMATIONS_LAB_*` configures a deterministic executor that
   synthesizes outputs and sentinels with no tmux involvement. Full run-engine,
   ledger, gate, and recovery behavior is exercisable here. When lab harnesses
   are configured, lab takes precedence over the tmux executor. Lab success is
   source/runtime mechanics evidence, not deployed real-agent proof.
2. **Dedicated formations socket (runtime).** Setting
   `CHROTE_FORMATIONS_TMUX_DEDICATED=1` is required for tmux-backed mission
   execution. It runs mission agents on a dedicated formations tmux socket
   (`CHROTE_FORMATIONS_TMUX_SOCKET`, sessions named with the configured prefix),
   separated from the cockpit socket. Nothing else is relaxed: the executor
   still refuses the cockpit socket, sessions must already exist with the
   configured prefix, panes must be alive with cwd inside configured roots, and
   output caps, timeouts, redaction, and fail-loud ledger events all still apply.
   The executor never creates or kills tmux sessions.

Promotion to live mission execution means setting the `CHROTE_FORMATIONS_TMUX_*`
boundary, a dedicated `CHROTE_FORMATIONS_TMUX_SOCKET`, and
`CHROTE_FORMATIONS_TMUX_DEDICATED=1` on the CHROTE service itself, with lab
variables unset. `.env.example` documents the full variable surface. There is no
prod-smoke-against-the-cockpit mode: the cockpit socket is never an execution
target.

## Build sequence

Build vertical slices:

```text
behavior scenario
→ React UI gesture/projection
→ shared Go formations package
→ archon verb
→ file/ledger round-trip
→ tests and review against root specs
```

Do not build a headless engine the UI cannot explain. Do not build a UI toy the
CLI cannot reproduce. The useful unit is an end-to-end capability.

## Success criteria

Formations is working when:

- a human can understand the agent organization and current mission state by
  looking at the canvas;
- an agent can author and mutate the same graph through `archon`;
- UI changes appear in files and CLI output without structural drift;
- CLI changes appear in the UI without structural drift;
- run state is visible on the graph, not buried only in logs;
- gates and verification can block, pass, fail, push back, or delegate judgment;
- failures leave durable evidence and recovery handles;
- the system remains reversible and local-first.

## Related root specs

- `ARCHON.md` — command surface and CLI verbs.
- `DATA-MODEL.md` — board, layout, persona, settings, and ledger formats.
- `DESIGN-SYSTEM.md` — cockpit feel, canvas affordances, themes, and UI density.
- `PRD.md` — product requirements and rollout staging.
