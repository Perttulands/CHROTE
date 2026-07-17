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
7. **Formations is always-on.** It is a permanent first-class surface, not a
   feature flag. The only Formations env vars are the executor safety ladder
   (`CHROTE_FORMATIONS_LAB_*` / `CHROTE_FORMATIONS_TMUX_*` /
   `CHROTE_FORMATIONS_SCRIPT_GATES` / `CHROTE_FORMATIONS_GATE_*` /
   `CHROTE_FORMATIONS_TMUX_PROD_SMOKE`), which gate execution-environment
   or gate-adapter promotion, never feature availability.
8. **Beads can anchor missions; it is not the graph store.**
9. **No command-execution landmines.** Free-text criteria never become implicit
   shell execution. Executable script gates require explicit operator-authored
   command config and guardrails: `commandArgv` is the default form; CHROTE
   passes argv literally and does not insert a shell. `commandCwd` is a cwd guard
   under the workspace, not a filesystem sandbox. `commandShell` is the explicit
   shell opt-in. Legacy `command` strings are parseable for old boards but are
   not executable by script gates.
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

There is one routing contract: `node_output.outputs[portId]`. Free-form
`node_output.text` is display summary only and never feeds graph edges.
Tmux-backed agents receive this contract as a fenced `chrote-outputs` JSON block
instruction; lab runs synthesize deterministic payloads for every output port.
If a formation omits a declared output or emits an unknown output id, the run
blocks and records `missing_output_payload` or `invalid_output_payload`.

Short routed outputs may live entirely in the JSON envelope:

```json
{"port_summary":{"text":"short non-secret payload"}}
```

Long routed outputs must not put prose into JSON strings. Tmux-backed agents
write the full payload to a text artifact and use `ref` as the artifact pointer:

```json
{"port_summary":{"text":"short non-secret summary","ref":".formations/artifacts/<run-id>/summary.md"}}
```

For tmux-backed runs, local file refs are resolved only after canonicalizing them
under the executor's configured roots/workspace. Missing, unreadable, out-of-root,
symlink-escaped, non-regular, non-text, or oversized refs block loudly. `ref` is
not a general host-file read capability, and these root checks are not an OS
sandbox: tmux agents still run with the host Unix user's permissions. Treat
output text, refs, filenames, and hydrated artifact content as durable non-secret
run evidence. `Redact=true` runs use the stricter boundary below.

## Redacted runs and replay

A `Redact=true` run separates execution-authoritative values from durable
evidence. Fresh execution may route a raw value only from the live attempt's
ephemeral in-memory state. The ledger, verifier and per-kind feedback, captures,
reports, artifact contents, output refs and their targets, derived evidence, and
errors must not retain that value. A safe ref may remain only after its target is
sanitized or replaced inside an authorized root.

Redaction markers, hashes, and display summaries explain what was removed; they
are never graph inputs. Raw executor capture may exist only as cleanup-owned
transient material. Before a persistent path can contain raw bytes, a durable
pending-redaction obligation is written and fsynced for that exact target. Its
internal cleanup locator is never exposed as an output ref or graph input.
Cleanup and recovery are idempotent, and no run can become final-successful
while a capture, report, or artifact remains pending redaction.

Recovery may reattach the same unresolved attempt without redispatch when its
qualified session identity is proven. It may finish pending cleanup or continue
other work only when no discarded authoritative value is required. Pending
cleanup targets and other evidence are never sources for reconstructing graph
input. If a future dispatch needs raw input that redaction intentionally
discarded, append terminal `type=run_failed` with
`data.code=redacted_input_unavailable`,
`data.reason=redacted_input_unavailable`, `data.unrecoverable=true`, and
`data.final=true`; `data.relatedSeq` identifies the exact source event whose raw
value was required. The run cannot resume or open another epoch; retry means a
new run with newly supplied authoritative input. Never dispatch a redaction
marker or summary in its place.

ADR-0005 defines this boundary and its rejected alternatives.

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
9. Runs can be inspected, followed, aborted, and—when their terminal state and
   evidence contract permit it—resumed from `archon` and reflected in the UI.
   `run_failed(redacted_input_unavailable)` is final and cannot resume.

Watching is optional. Recovery must not depend on a browser tab staying open.

## Execution environments

Formations execution promotes through explicit execution environments and gate
adapters. Each step up is an explicit configuration decision, never a silent
fallback.

1. **Lab.** `CHROTE_FORMATIONS_LAB_*` configures a deterministic executor that
   synthesizes outputs and sentinels with no tmux involvement. Full run-engine,
   ledger, gate, and recovery behavior is exercisable here. When lab harnesses
   are configured, lab takes precedence over the tmux executor.
2. **Isolated tmux.** `CHROTE_FORMATIONS_TMUX_*` dispatches to real agent
   sessions, but the executor refuses to run unless the socket, cwd, and all
   roots live under the system temp directory and the socket is not the
   default-resolved tmux socket. Dogfooding happens on a throwaway socket with
   its own sessions; the live cockpit socket is unreachable by construction.
3. **Script gates.** `CHROTE_FORMATIONS_SCRIPT_GATES` enables operator-authored
   script/lint/code gate commands. Script gates execute `commandArgv` literally
   without a CHROTE-inserted shell, inside the board workspace or a `commandCwd`
   that resolves under that workspace. This is a cwd guard, not a filesystem
   sandbox. A shell is allowed only with explicit `commandShell`.
   Legacy `command` text is stored for compatibility but deliberately fails if
   no structured command is present. Output caps, timeouts, redaction, and
   fail-loud gate verdicts apply.
4. **Live socket (prod smoke).** Setting `CHROTE_FORMATIONS_TMUX_PROD_SMOKE`
   is the explicit operator opt-in that lifts the temp-socket and temp-root
   restrictions so the executor may target the live CHROTE tmux socket and real
   workspace roots. Nothing else is relaxed: sessions must already exist with
   the configured prefix, panes must be alive with cwd inside configured roots,
   output caps, timeouts, redaction, and fail-loud ledger events all still
   apply. The executor never creates or kills tmux sessions.

Promotion to the live socket means setting the `CHROTE_FORMATIONS_TMUX_*`
boundary and the prod-smoke opt-in on the CHROTE service itself, with lab
variables unset. `.env.example` documents the full variable surface.

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
