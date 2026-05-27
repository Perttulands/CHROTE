# Meta-Harness Desired State

This document separates what Perttu has stated as the desired state from implementation interpretations and recommendations.

## User-Stated Desired State

Perttu wants CHROTE to grow into a meta-harness for AI work.

The access model starts with CHROTE's existing strength: named tmux sessions.
The next shape is named agent identities. A named identity may still be backed
by tmux, but it should also be valid in Gas City so it can receive routed work,
use mail/nudge, participate in molecules, and leave durable workflow evidence.

The meta-harness should be able to include multiple AI harnesses and products in one system, such as:

- Claude Code
- Codex
- Pi
- OpenCode
- other local or remote agent harnesses as needed

These agents and harnesses should become interchangeable parts of Perttu's system. The important property is not that one product wins, but that the workspace can slot different products into the same operating model.

The workspace should support workflows such as:

- giving the user a named agent, for example Claudia, that can be opened from
  CHROTE like any other session;
- telling another named agent, for example Codxia, "Help Claudia get this done"
  and having the collaboration route through Gas City primitives;
- one agent makes a plan
- two other agents review it from different perspectives
- the first agent or a chair role synthesizes the result
- a senate session where several AI agents discuss a topic to form a holistic view
- teams of agents that can prompt each other, not only a fixed sequence of calls

Subagents inside one product are useful, but not enough. The more important capability is spawning or coordinating agents in completely different harnesses.

Previous CHROTE work has already used tmux `send-keys` to make agents prompt each other. That proved the concept, but it is cumbersome. The desired state is a cleaner orchestration and team layer above that kind of mechanism.

Perttu pointed to Gas City because it provides the orchestration plumbing CHROTE
is missing: valid runtime identities, agent mail, nudging, sling/delegation,
molecules, workflow automation, event evidence, and later automation layers.

Perttu pointed to the Pi communication repositories because they express a useful team concept: agents are not only run in sequence; they can form a team and prompt each other. In practice this may still be agents sending prompts without shared context beyond those messages, but the team shape is important.

Recipes are key. Gas City's molecule system is relevant because molecules are prebuilt bundles of Beads that form workflows.

## User-Stated Non-Goals

The desired system should not be limited to:

- one AI product
- one vendor's subagent feature
- a linear chain of agent calls
- manual tmux pane targeting as the normal interface
- a workflow that dies when the client device disconnects
- passive transcript watching, status surfacing, or review summaries as the core
  purpose of Gas City inside CHROTE
- forced migration of current old sessions before the named-identity model can
  move forward

## Acceptance Shape

A successful meta-harness should let Perttu:

- define a reusable workflow such as plan plus two reviews
- choose which harnesses or agent products fill each role
- access named agents from CHROTE as the normal entry point
- delegate from one named agent to another without manually targeting tmux panes
- run the workflow from the host-owned Ubuntu workspace
- watch the workflow progress from CHROTE
- inspect each agent's session, messages, outputs, Beads, and artifacts
- intervene or redirect when needed
- close the Surface or browser
- return later from another device and recover the state

The host remains the source of truth. The client remains disposable.

## Assistant Interpretation

This section is not the user's stated desired state. It is the current working interpretation of what the architecture probably needs.

CHROTE likely needs a layered model:

- cockpit: browser UI for durable host state
- identity/access registry: known names, harnesses, roles, capabilities, launch
  methods, active sessions, and Gas City identity bindings
- adapter layer: one adapter per harness, with Gas City ownership preferred and
  tmux fallback allowed
- message/run ledger: durable record of prompts, replies, role assignments, state transitions, artifacts, and audit events
- recipe engine: reusable workflow definitions, using Gas City formulas and
  molecules where they fit
- team communication layer: explicit messages between roles and sessions, using
  Gas City mail/nudge where possible

The key design pressure is to make orchestration deliberate, inspectable, and recoverable instead of hidden behind fragile terminal automation.

## Evaluation Targets

Current descriptive Gas City context and CHROTE leverage framing lives in `docs/chrote-gascity-framing.md`. Gas City should be treated as:

- the orchestration substrate selected by ADR-0001
- the source of Beads-backed mail, nudge, session, formula, molecule, sling, and
  event primitives
- a component CHROTE must integrate without letting it take over Beads, Context
  Citadel, authentication, or the browser access layer

Second-look assessment: `docs/gascity-meta-harness-evaluation.md` as evidence/history, not as a standing implementation plan.

Pi comms should be evaluated as:

- a team communication concept
- a possible adapter target
- a source of peer registration and message-delivery ideas

tmux should be evaluated as:

- a durable session substrate
- a fallback control plane
- not the desired day-to-day orchestration UI

## Open Questions

- Should CHROTE reach Gas City first through CLI, supervisor API, SDK/library, or
  a narrow adapter?
- What is the minimum adapter interface for Claude Code, Codex, Pi, OpenCode, and generic tmux agents?
- How should recipes be represented to the user when the underlying execution is
  a Gas City formula/molecule?
- What is the right shared-context model for a team: Gas City mail, Beads,
  shared files, Context Citadel, or a richer run database?
- What safety controls are needed before agents can prompt other agents automatically?
