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

## Core nouns

| Noun | Meaning |
| --- | --- |
| Agent | Durable persona card with stable id, display name, capabilities, and harness variants |
| Formation | Typed coordination unit: `solo`, `peer`, `flow`, or `orchestrated` |
| Slot | Role position inside a formation, optionally assigned to an agent id |
| Mission | Concrete goal and entry point for a run, usually linked to work state |
| Board | Persisted graph of missions, formations, gates, connections, and layout metadata |
| Connection | Directed edge from an output port to an input port |
| Gate | Routing checkpoint that passes, fails, blocks, or loops work |
| Verification | Inline check local to a formation |
| Run | Execution instance that binds slots, dispatches work, records events, and projects state |
| Ledger | Append-only event history for a run |

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
7. **Feature flags default off until ready.** UI flag: `chrote-formations`.
   Server env: `CHROTE_FORMATIONS`.
8. **Beads can anchor missions; it is not the graph store.**
9. **No command-execution landmines.** Free-text criteria never become implicit
   shell execution. Executable checks require explicit operator-authored config
   and guardrails.

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
