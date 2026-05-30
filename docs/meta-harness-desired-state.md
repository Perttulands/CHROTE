# Meta-Harness Desired State

> Current status: this is a desired-state research document, not current CHROTE
> implementation guidance. The active Gas City integration was rolled back on
> 2026-05-30. Gas City remains archived research only unless explicitly
> reintroduced through a new decision.

This document separates what Perttu has stated as the desired state from implementation interpretations and recommendations.

## User-Stated Desired State

Perttu wants CHROTE to grow into a meta-harness for AI work.

The meta-harness should be able to include multiple AI harnesses and products in one system, such as:

- Claude Code
- Codex
- Pi
- OpenCode
- other local or remote agent harnesses as needed

These agents and harnesses should become interchangeable parts of Perttu's system. The important property is not that one product wins, but that the workspace can slot different products into the same operating model.

The workspace should support workflows such as:

- one agent makes a plan
- two other agents review it from different perspectives
- the first agent or a chair role synthesizes the result
- a senate session where several AI agents discuss a topic to form a holistic view
- teams of agents that can prompt each other, not only a fixed sequence of calls

Subagents inside one product are useful, but not enough. The more important capability is spawning or coordinating agents in completely different harnesses.

Previous CHROTE work has already used tmux `send-keys` to make agents prompt each other. That proved the concept, but it is cumbersome. The desired state is a cleaner orchestration and team layer above that kind of mechanism.

During the rolled-back exploration, Perttu pointed to Gas City because it looked usable as-is or as a useful source of concepts. The retained lesson is that orchestration may benefit from SDK-like primitives, but Gas City is not an active dependency or current implementation target.

Perttu pointed to the Pi communication repositories because they express a useful team concept: agents are not only run in sequence; they can form a team and prompt each other. In practice this may still be agents sending prompts without shared context beyond those messages, but the team shape is important.

Recipes are key. The archived Gas City molecule research remains useful background because it explored prebuilt bundles of Beads that form workflows.

## User-Stated Non-Goals

The desired system should not be limited to:

- one AI product
- one vendor's subagent feature
- a linear chain of agent calls
- manual tmux pane targeting as the normal interface
- a workflow that dies when the client device disconnects

## Acceptance Shape

A successful meta-harness should let Perttu:

- define a reusable workflow such as plan plus two reviews
- choose which harnesses or agent products fill each role
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
- agent registry: known harnesses, roles, capabilities, launch methods, and active sessions
- adapter layer: one adapter per harness, with native APIs preferred and tmux fallback allowed
- message/run ledger: durable record of prompts, replies, role assignments, state transitions, artifacts, and audit events
- recipe engine: reusable workflow definitions, informed by archived molecule research where useful
- team communication layer: explicit messages between roles and sessions, informed by Pi comms and archived mail/nudge research where useful

The key design pressure is to make orchestration deliberate, inspectable, and recoverable instead of hidden behind fragile terminal automation.

## Archived Research Inputs

Gas City should not be evaluated as a live dependency while the rollback decision stands. The archived research can still inform future design as:

- a studied example of orchestrator SDK shape;
- a source of Beads-backed mail, nudge, session, and molecule concepts;
- a record of why direct reuse was rolled back before it became product foundation.

Archived second-look assessment: `gas-city-research/evaluations/meta-harness-evaluation.md`.

Pi comms should be evaluated as:

- a team communication concept
- a possible adapter target
- a source of peer registration and message-delivery ideas

tmux should be evaluated as:

- a durable session substrate
- a fallback control plane
- not the desired day-to-day orchestration UI

## Open Questions

- What is the minimum adapter interface for Claude Code, Codex, Pi, OpenCode, and generic tmux agents?
- Should recipes be represented directly as Beads, CHROTE config, or a bridge across those?
- What is the right shared-context model for a team: transcript-only, Beads-backed mailbox, shared files, or a richer run database?
- What safety controls are needed before agents can prompt other agents automatically?
