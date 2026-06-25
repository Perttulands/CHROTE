# CHROTE Source-Truth Index

Status: **Active supporting index**.

This file answers one question: when CHROTE docs disagree, which one wins?

It is not a replacement for the specs below. It is the routing table for them, and `scripts/doc-lint.py` enforces the minimum hygiene that keeps this table honest.

## Active source-truth specs

These files define current CHROTE behavior and should not contradict each other.

| File | Authority |
| --- | --- |
| [`PRD.md`](../PRD.md) | Product scope, roadmap posture, and current surface framing |
| [`FORMATIONS.md`](../FORMATIONS.md) | Formations product/model/runtime invariants |
| [`ARCHON.md`](../ARCHON.md) | Archon CLI purpose and command-surface direction |
| [`DATA-MODEL.md`](../DATA-MODEL.md) | Durable state ownership, files, ledgers, API/persistence model |
| [`DESIGN-SYSTEM.md`](../DESIGN-SYSTEM.md) | Dashboard visual/interaction principles and theme ids |

The four machine-linted specs are `FORMATIONS.md`, `ARCHON.md`, `DATA-MODEL.md`, and `DESIGN-SYSTEM.md`. They carry YAML frontmatter with `authority: source-of-truth` and `enforced_by: scripts/doc-lint.py`.

`PRD.md` is intentionally product-level, not machine-linted frontmatter yet. If it starts carrying executable invariants, add frontmatter and extend the lint deliberately.

## Active supporting docs

These docs are useful and current enough to consult, but they do not override the active source-truth specs.

| File | Role |
| --- | --- |
| [`README.md`](../README.md) | Operator-facing overview and discoverability |
| [`COMPONENTS.md`](../COMPONENTS.md) | Implementation/component map |
| [`SECURITY.md`](../SECURITY.md) | Security posture and deployment cautions |
| [`CHANGELOG.md`](../CHANGELOG.md) | Release notes |
| [`AGENTS.md`](../AGENTS.md) | Agent/project work rules |
| [`CLAUDE.md`](../CLAUDE.md) | Claude/agent-specific work rules |
| [`docs/TEST_STRATEGY.md`](TEST_STRATEGY.md) | Test command reference |
| [`docs/installation.md`](installation.md) | Install/rebuild notes |
| [`docs/troubleshooting.md`](troubleshooting.md) | Known operator fixes |
| [`docs/adr/`](adr/) | Accepted architectural decisions; narrower than the active specs |

## Formations historical/reference packet

The `Perttus_vision_for_agent_orchestration/` tree is valuable, but it is not a single current source of truth.

| Path | Status |
| --- | --- |
| [`Perttus_vision_for_agent_orchestration/DECISIONS-LOCKED.md`](../Perttus_vision_for_agent_orchestration/DECISIONS-LOCKED.md) | Historical decision packet. Consult for why earlier pivots happened; current root specs win when behavior changed. |
| [`Perttus_vision_for_agent_orchestration/spec/`](../Perttus_vision_for_agent_orchestration/spec/) | Supporting S0/BDD packet. Use as acceptance/reference material, not as a replacement for current root specs. |
| [`Perttus_vision_for_agent_orchestration/03-formations.html`](../Perttus_vision_for_agent_orchestration/03-formations.html) and [`03-formations.js`](../Perttus_vision_for_agent_orchestration/03-formations.js) | Visual/interaction reference for the cockpit feel. Root specs and current code decide current feature availability/runtime semantics. |
| [`Perttus_vision_for_agent_orchestration/archive/`](../Perttus_vision_for_agent_orchestration/archive/) | Archive/superseded design material. Background only. |

## Archive-only docs

These are not operational instructions unless a current active spec or Bead explicitly revives a slice.

| Path | Status |
| --- | --- |
| [`docs/archive/`](archive/) | Archive-only. May contain stale ports, commands, service names, and dangerous cleanup advice. |
| [`docs/plans/`](plans/) | Planning artifacts. Useful context, not standing instructions. |
| [`docs/legacy-ideas.md`](legacy-ideas.md) | Demoted idea capture. |

## Intentionally absent docs

Do not create stub docs just to satisfy old references.

| Missing path | Current replacement |
| --- | --- |
| `CHROTE.md` | Use [`README.md`](../README.md) for overview and this index for source-truth routing. |
| `SPEC-CHANGELOG.md` | Use [`CHANGELOG.md`](../CHANGELOG.md) for release notes and Beads/ADRs for spec decisions. |
| `ARCHON_BDD.md` / `FORMATIONS_BDD.md` | Use the current root specs plus the supporting S0 packet under [`Perttus_vision_for_agent_orchestration/spec/`](../Perttus_vision_for_agent_orchestration/spec/). |
| `scripts/dead-link-check.py` / `scripts/agent-check` | Not current repo gates. Add them only with passing implementations and Bead ownership. |

## Current enforcement boundary

`scripts/doc-lint.py` deliberately checks only what is stable enough to enforce today:

1. active source-truth frontmatter exists on the four machine-linted specs;
2. every `enforced_by:` path in tracked Markdown points to an existing file;
3. this index references real files or intentionally absent docs;
4. dashboard theme ids in docs match the TypeScript settings type;
5. `SECURITY.md` names the current bind/port/auth environment variables.

It deliberately does **not** yet enforce full `ARCHON.md` versus `src/cmd/archon/main.go` parity. The current branch contains dirty Archon/Formations work; strict CLI parity belongs in the Archon landing lane, not in this source-truth cleanup slice.
