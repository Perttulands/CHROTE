# CHROTE / Gas City Understanding

Date: 2026-05-24

This is the canonical descriptive context document for CHROTE and Gas City.

It is not an implementation plan, task checklist, ADR, ownership map, or recommendation. It exists to help agents understand what Gas City is, how it works, what it appears to afford, and the sharp problem CHROTE needs to investigate.

## Current Decision Note

As of 2026-05-26, this document is historical framing and descriptive context.
ADR-0001 (`docs/adr/0001-chrote-3-gas-city-substrate.md`) records the current
CHROTE 3.0 direction: Gas City is the orchestration substrate, while CHROTE is
the authenticated access and operator layer. Any "candidate" or "non-decision"
language below should be read as pre-ADR framing, not the current architecture
decision.

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

## What Gas City Appears To Afford

Gas City appears to offer CHROTE-relevant leverage in these areas:

- **Durable work graph:** beads as shared records for tasks, messages, molecules, convoys, and workflow state.
- **Runtime abstraction:** sessions can sit behind providers instead of CHROTE treating tmux as the whole ontology.
- **Message model:** mail gives durable communication; nudge gives live wake-up/delivery.
- **Workflow packaging:** formulas and molecules encode reusable multi-step workflows such as plan-review-synthesis shapes.
- **Dispatch surface:** sling demonstrates routing work to sessions/agents while recording durable state.
- **Observation stream:** event bus and supervisor API provide read models and recovery hooks.
- **Health/reconciliation:** patrol behavior suggests a path toward keeping long-running agent sessions inspectable and recoverable.
- **Config/pack model:** city config, packs, formulas, and provider presets make orchestration behavior portable without hardcoding roles.

These are affordances to evaluate, not implementation decisions.

## CHROTE Context

CHROTE's desired direction, captured separately in `docs/meta-harness-desired-state.md`, is a host-owned AI meta-harness. It should eventually let Perttu run and observe workflows involving multiple harnesses such as Claude Code, Codex, Pi, OpenCode, and generic tmux-backed agents.

The desired experience includes:

- reusable workflows such as plan + two reviews + synthesis;
- interchangeable harnesses filling workflow roles;
- visibility into sessions, messages, artifacts, and Beads;
- recoverability after browser/client disconnect;
- human ability to intervene or redirect.

Gas City is relevant because it already contains many primitives that look adjacent to that desired state.

## Sharp Problem Statement

The problem is not to decide, today, which system owns which future responsibility.

The problem is:

> Given what Gas City already is and affords, how can CHROTE best leverage it to reach the desired meta-harness capability while preserving modularity, safety, recoverability, and agent comprehensibility?

A sharper version:

> What is the smallest evidence-backed CHROTE/Gas City integration or reuse strategy that gives real leverage, without prematurely committing to sidecar, library, fork, wrapper, rewrite, or copy/adapt architecture?

## Questions To Investigate

The next work should answer questions, not smuggle in decisions:

1. **Leverage**
   - Which Gas City capabilities remove real CHROTE work?
   - Which capabilities are only conceptually useful?
   - Which are too coupled to Gas City's worldview to reuse directly?

2. **Integration shape**
   - Should CHROTE observe a running Gas City sidecar?
   - Should CHROTE call public `gc` / supervisor / event surfaces?
   - Should CHROTE use Gas City as an SDK/library where public APIs exist?
   - Should CHROTE copy/adapt any tiny stable primitives?
   - Should CHROTE only learn from Gas City concepts and build separately?

3. **Workflow fit**
   - Can Gas City formulas/molecules represent the desired plan-review-synthesis and senate workflows?
   - What is missing for real harnesses rather than mock agents?
   - How do mail/nudge/session concepts map to human-visible CHROTE workflows?

4. **State and recovery**
   - Which system is the durable source for workflow state in each candidate shape?
   - Can CHROTE recover session output, messages, and artifacts reliably through Gas City surfaces?
   - What happens when CHROTE is down, Gas City is down, or the client disconnects?

5. **Safety**
   - What safety boundary is required before any real paid/credentialed harness is driven through Gas City or CHROTE?
   - What actions need explicit human approval?
   - How are cross-system actions audited without leaking tokens or raw private transcripts?

6. **Upstream relationship**
   - If a needed seam is missing, is it better to contribute it upstream to Gas City, build a local adapter, or defer?
   - What would make upstream contribution practical or impractical?

## Candidate Approaches To Compare

These are candidates, not decisions:

- Treat Gas City as a local sidecar and observe/control it only through public/native surfaces.
- Use Gas City as an SDK/library where stable public APIs exist.
- Use Gas City as a reference design while CHROTE builds its own narrow layer.
- Copy/adapt only tiny stable helper code with license attribution if that proves simpler and safe.
- Contribute missing public seams upstream before building local workarounds.
- Do nothing beyond preserving the current spike until a concrete workflow demands integration.

Each candidate should be judged by evidence, not architectural vibes.

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

## Non-Decisions

This document does not decide:

- whether Gas City is a sidecar, SDK, dependency, reference, or source of copied code;
- whether CHROTE should expose any Gas City UI;
- whether CHROTE should issue any Gas City mutations;
- whether native `gc` commands should be the final operator interface;
- whether formulas/molecules become CHROTE concepts;
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
- do not treat it as an implementation decision;
- do not infer ownership boundaries from it;
- do not turn candidate approaches into standing decisions;
- do not delete or rewrite `/home/perttu/gascity` or `/home/perttu/research/upstreams/gascity` as part of doc cleanup;
- do not expose the Gas City supervisor outside localhost without a separate explicit design and approval.
