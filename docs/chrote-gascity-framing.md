# CHROTE / Gas City Understanding

Date: 2026-05-24

This is a historical descriptive context document for CHROTE and Gas City.

It is not an implementation plan or task checklist. It exists to preserve what
was learned about Gas City, how it works, and what it appeared to afford during
the CHROTE/Gas City investigation.

## Current Decision Note

As of 2026-05-30, ADR-0003
(`docs/adr/0003-rollback-active-gascity-integration.md`) is the active decision:
CHROTE has rolled back the active Gas City integration and keeps only Gas City
docs/plans/research as historical context.

The product framing that led to the rollback is:

- CHROTE is where Perttu accesses named sessions and named agent identities.
- Gas City supplies the plumbing for valid identities, mail, nudging,
  sling/delegation, formulas, molecules, events, and later automation.
- The primary felt win is agent collaboration that improves output quality, not
  a passive transcript/status dashboard.
- Current old tmux sessions do not need migration before the Gas City-backed
  identity model can proceed.

Any remaining "candidate" or "substrate" language below describes historical
exploration or a superseded implementation shape, not current CHROTE direction.

## What Gas City Is

Gas City is composable orchestration infrastructure for multi-agent coding workflows.

The upstream README describes it as an orchestration-builder SDK for multi-agent systems. It extracts reusable infrastructure from Gas Town into a configurable toolkit with runtime providers, work routing, formulas, orders, health patrol, and declarative city configuration.

A Gas City workspace is a "city": a directory with configuration, agents, formulas, runtime state, and supervisor/controller behavior. Role behavior is supplied by configuration and prompt templates rather than hardcoded roles.

## How Gas City Works

Gas City's architecture is organized around five primitive concepts and four derived mechanisms.

### Primitives

1. **Session**
   - Starts, stops, prompts, and observes agent/runtime sessions regardless of provider.
   - Providers include tmux, subprocess, exec scripts, Kubernetes, fake/test providers, and routing layers such as ACP/auto/hybrid.
   - Separates session lifecycle from role behavior.

2. **Task Store / Beads**
   - Stores work units with CRUD, parent/child relationships, dependencies, labels, query, and status.
   - Tasks, messages, molecules, convoys, and other domain records can all be represented as beads.
   - Providers include Dolt-backed `bd`, file-backed store, memory, and exec-backed store.

3. **Event Bus**
   - Append-only event stream for system activity.
   - Supports listing, sequence cursors, and watching/reactive notification.
   - Gives the system an observation/audit substrate.

4. **Config**
   - TOML-based city, agent, rig, provider, formula, and pack configuration.
   - Capability appears by configuration presence rather than a pile of separate feature flags.

5. **Prompt Templates**
   - Markdown / Go-template prompts define role behavior.
   - Gas City infrastructure handles transport, state, and dispatch; user configuration defines what roles actually do.

### Derived Mechanisms

6. **Messaging**
   - Mail is bead-backed durable message state.
   - Nudge is live text delivery through the runtime/session provider.
   - Durable message first, live notification second.

7. **Formulas, Molecules, Wisps, Orders**
   - Formula: reusable TOML workflow definition.
   - Molecule: materialized workflow as a bead tree.
   - Wisp: ephemeral/TTL-style molecule.
   - Order: formula triggered by schedule or event conditions.

8. **Dispatch / Sling**
   - Routes work by composing sessions, config, beads, formulas, nudges, convoys, and events.
   - This is not a separate primitive; it is a higher-level composition.

9. **Health Patrol**
   - Probes sessions, compares config thresholds, emits stall/recovery events, and restarts with backoff.

## What The Local Spike Proved

Local evidence lives in `docs/gascity-sidecar-spike-results.md`.

The sidecar spike showed that, in this environment:

- a local Gas City workspace can exist at `/home/perttu/gascity`;
- `gc doctor --verbose` passed in the spike;
- file-backed Beads worked without Dolt for the spike;
- the supervisor API was available on localhost at `127.0.0.1:8372`;
- tmux-backed mock sessions could be created and observed;
- mail, session nudge/submit, and sling-style routing worked with mock agents;
- no public/Tailscale exposure was added;
- no real paid or credentialed AI harness was started.

Important gaps from the same evidence:

- real harness behavior is not validated by the mock-agent spike;
- Dolt-backed durability and production-like Beads behavior still need validation;
- transcript/log recovery for real sessions remains a separate concern;
- `mol-review-quorum` and similar formula workflows need production-like validation before relying on them;
- safety boundaries for paid/credentialed harnesses are not solved by the spike.

## What Gas City Affords CHROTE

Gas City offers CHROTE-relevant leverage in these areas:

- **Durable work graph:** beads as shared records for tasks, messages, molecules, convoys, and workflow state.
- **Runtime abstraction:** sessions can sit behind providers instead of CHROTE treating tmux as the whole ontology.
- **Message model:** mail gives durable communication; nudge gives live wake-up/delivery.
- **Workflow packaging:** formulas and molecules encode reusable multi-step workflows such as plan-review-synthesis shapes.
- **Dispatch surface:** sling demonstrates routing work to sessions/agents while recording durable state.
- **Observation stream:** event bus and supervisor API provide runtime evidence
  and recovery hooks.
- **Health/reconciliation:** patrol behavior suggests a path toward keeping long-running agent sessions inspectable and recoverable.
- **Config/pack model:** city config, packs, formulas, and provider presets make orchestration behavior portable without hardcoding roles.

These are the plumbing layers CHROTE should avoid rebuilding unless a specific
Gas City boundary proves unusable.

## CHROTE Context

CHROTE's desired direction, captured separately in
`docs/meta-harness-desired-state.md`, is a host-owned AI meta-harness. It
should let Perttu access named sessions and named identities across multiple
harnesses such as Claude Code, Codex, Pi, OpenCode, Hermes, and generic
tmux-backed agents.

The desired experience includes:

- reusable workflows such as plan + two reviews + synthesis;
- interchangeable harnesses filling workflow roles;
- the ability to tell one named agent to help another, for example Codxia
  helping Claudia;
- visibility into sessions, messages, artifacts, and Beads when the human needs
  to inspect or intervene;
- recoverability after browser/client disconnect;
- human ability to intervene or redirect.

Gas City is relevant because it already contains the orchestration primitives
needed below that access layer.

## Sharp Problem Statement

The historical problem was no longer whether Gas City should be considered. At
that point, the superseded ADR-0001 decision was that Gas City would be the
orchestration substrate and CHROTE would be the access/operator layer.

The problem is:

> How should CHROTE expose named Gas City-backed agent identities and workflows
> so Perttu can delegate between agents without manually coordinating tmux, while
> preserving safety, recoverability, and agent comprehensibility?

A sharper version:

> What is the smallest evidence-backed vertical slice where one named agent can
> help another through Gas City mail/nudge/sling/molecule primitives, with
> CHROTE as the operator access surface?

## Open Implementation Questions

The next work should answer these questions without drifting into passive
dashboard work:

1. **Leverage**
   - Which Gas City capabilities remove real CHROTE work now?
   - Which capabilities are useful later but not needed for the first named-agent
     slice?
   - Which are too coupled to Gas City's worldview to reuse directly?

2. **Integration shape**
   - Which bounded Gas City surfaces should CHROTE use first: native CLI,
     supervisor API, event stream, or adapter package?
   - Should CHROTE use Gas City as an SDK/library where public APIs exist?
   - Should CHROTE copy/adapt any tiny stable primitives?
   - Which seams need upstream contribution before CHROTE relies on them?

3. **Workflow fit**
   - Can Gas City formulas/molecules represent the desired plan-review-synthesis and senate workflows?
   - What is missing for real harnesses rather than mock agents?
   - How do mail/nudge/session concepts map to human-visible named-agent
     workflows?

4. **State and recovery**
   - Which system is the durable source for each kind of workflow state?
   - Can CHROTE recover session output, messages, and artifacts reliably through
     Gas City surfaces when the user reopens a named identity?
   - What happens when CHROTE is down, Gas City is down, or the client disconnects?

5. **Safety**
   - What safety boundary is required before any real paid/credentialed harness is driven through Gas City or CHROTE?
   - What actions need explicit human approval?
   - How are cross-system actions audited without leaking tokens or raw private transcripts?

6. **Upstream relationship**
   - If a needed seam is missing, is it better to contribute it upstream to Gas City, build a local adapter, or defer?
   - What would make upstream contribution practical or impractical?

## Historical Candidate Approaches

These were useful candidates during the research phase:

- Treat Gas City as a local sidecar and observe/control it only through public/native surfaces.
- Use Gas City as an SDK/library where stable public APIs exist.
- Use Gas City as a reference design while CHROTE builds its own narrow layer.
- Copy/adapt only tiny stable helper code with license attribution if that proves simpler and safe.
- Contribute missing public seams upstream before building local workarounds.
- Do nothing beyond preserving the current spike until a concrete workflow demands integration.

ADR-0001 selected the Gas City substrate direction, but ADR-0003 superseded that
decision. These options remain useful only as historical implementation-shape
checks if a future reintroduction is explicitly chosen.

## Evaluation Criteria

A good answer should score well on:

- **Leverage:** reuses meaningful existing Gas City capability instead of rebuilding it.
- **Modularity:** avoids creating another everything app.
- **Reversibility:** can be backed out if Gas City fit is poor.
- **Observability:** makes sessions, messages, workflow state, and errors inspectable.
- **Recoverability:** survives client disconnects and process restarts where possible.
- **Safety:** keeps localhost/control surfaces bounded and protects tokens/private transcripts.
- **Agent comprehensibility:** future agents can understand the seam without loading two entire systems into their heads.
- **Upstreamability:** missing seams can plausibly be contributed or discussed upstream.

## Current Non-Decisions

This document does not decide:

- whether CHROTE reaches Gas City first through a sidecar API, SDK/library,
  CLI, or narrow adapter;
- exactly which Gas City controls belong in the Gas City tab;
- which Gas City mutations are safe enough for CHROTE UI controls;
- whether native `gc` commands should be the final operator interface;
- how formulas/molecules are represented to the operator;
- whether any stale/noisy Gastown artifacts should be deleted.

Those require separate evidence and explicit decisions.

## Evidence And History

These documents are evidence or historical exploration, not standing plans:

- `docs/gascity-sidecar-spike-results.md` records the local sidecar spike and localhost supervisor findings.
- `docs/gascity-meta-harness-evaluation.md` records why Gas City looked like a strong meta-harness candidate.
- `docs/chrote-cli-control-plane-exploration.md` records a superseded CHROTE-native control-plane branch of thinking.
- `docs/meta-harness-desired-state.md` records Perttu's desired meta-harness shape separately from assistant recommendations.
- `docs/agent-collaboration-primitives.md` records broader collaboration primitive research.

Retired `/home/perttu/plans/chrote-gascity-*.md` files were plan-shaped exploration artifacts. Their useful descriptive content has been folded into this file or preserved in the evidence docs above.

## Current Reading For Agents

When using this document:

- read it as context and problem framing;
- treat ADR-0003 as the active ownership decision: CHROTE has no active Gas City
  integration, while Gas City research is retained as historical evidence;
- do not turn passive transcript watching or status surfacing into the main
  product value;
- do not require current old tmux sessions to migrate before named Gas
  City-backed identities are built;
- do not delete or rewrite `/home/perttu/gascity` or `/home/perttu/research/upstreams/gascity` as part of doc cleanup;
- do not expose the Gas City supervisor outside localhost without a separate explicit design and approval.
