# CHROTE Formations — design dossier

This folder is the synthesized design for the agent-orchestration capability Perttu
described in [`../perttus_vision_for_agent_teams_and_orchestration.md`](../perttus_vision_for_agent_teams_and_orchestration.md)
and prototyped in [`../03-formations.html`](../03-formations.html) / `../03-formations.js`.

It was produced by an architect pass over the vision + prototype + the existing CHROTE
codebase + the prior meta-harness / Gas City research, then fanned out to seven parallel
design agents (one per slice) and synthesized back into this plan.

## Read in this order

| # | Doc | What it answers |
|---|---|---|
| — | **[00-master-plan.md](00-master-plan.md)** | **Start here.** The whole thing in one document: goal, architecture, the seven decisions, phasing, the first slice, risks, Gas-City-avoidance, success criteria. |
| 1 | [01-architecture.md](01-architecture.md) | The layered model, the "one id" spine, and the load-bearing engine-location decision. |
| 2 | [02-data-model.md](02-data-model.md) | The canonical on-disk file format (TOML boards + NDJSON ledger + layout sidecar), IDs, round-trip rules. |
| 3 | [03-cli-surface.md](03-cli-surface.md) | `archon` — the agent CLI that is the **primary** interface. Verbs, transcripts, the Archon/leader patterns as commands. |
| 4 | [04-run-engine-and-adapters.md](04-run-engine-and-adapters.md) | Execution semantics, the harness-neutral adapter, cross-harness reality (tmux + sentinel), the run ledger, escalation. |
| 5 | [05-registry-and-personas.md](05-registry-and-personas.md) | Persona cards, progressive-disclosure registry, persona↔session binding, the Archon, teams, evaluation seam. |
| 6 | [06-shared-state-and-escalation.md](06-shared-state-and-escalation.md) | Beads vs notice board boundary, the file-native board, escalation, conversational visibility. **(Notice board deferred per Perttu — kept as design.)** |
| 7 | [07-chrote-integration.md](07-chrote-integration.md) | The Formations tab, the Go API, write-back, feature flags, build/deploy, reversibility. |
| 8 | [08-open-questions.md](08-open-questions.md) | The decisions to nail down, each with a recommendation. **Answer these to unlock implementation beads.** |

## One-sentence summary

Agents — not Perttu — author and run *formations* (small teams wired into missions) through a
simple `bd`-style CLI over plain files on disk; a single touchpoint persona (the **Archon**) gives
Perttu access to the whole organization conversationally; and CHROTE's existing cockpit grows a
read-only **Formations** tab (behind a default-off flag) that makes those agent-authored flows
legible — never the place they're built.

## Status

Design only. No code yet. Nothing here is committed to CHROTE runtime. The first implementation
slice (see [08](08-open-questions.md)) is gated behind decisions Perttu still needs to make.
