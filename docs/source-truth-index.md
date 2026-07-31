# CHROTE Documentation Source of Truth

When CHROTE documents disagree, this file says which one wins.

It is a routing table, not another product spec. `scripts/doc-lint.py` enforces
the stable parts of the map.

## Public reader path

A first-time reader should not need architecture archaeology or private operator
notes.

| Need | Start here |
| --- | --- |
| What CHROTE is and whether to use it | [`README.md`](../README.md) |
| Current shipped product and roadmap boundary | [`PRD.md`](../PRD.md) |
| Install or upgrade | [`docs/installation.md`](installation.md) |
| Diagnose a supported install | [`docs/troubleshooting.md`](troubleshooting.md) |
| Security and trust boundary | [`SECURITY.md`](../SECURITY.md) |
| Contribute and reproduce CI | [`CONTRIBUTING.md`](../CONTRIBUTING.md) |
| Release history | [`CHANGELOG.md`](../CHANGELOG.md) |

The README is the landing page, not the authority for deep runtime contracts.
When it conflicts with the PRD or an active source-truth spec, fix the README.

## Active source-truth specs

| Spec | Owns |
| --- | --- |
| [`PRD.md`](../PRD.md) | Current product, shipped views, operator outcomes, non-goals, and roadmap boundary |
| [`FORMATIONS.md`](../FORMATIONS.md) | Unreleased experimental Formation and mission model, ports, gates, runs, and execution environments |
| [`ARCHON.md`](../ARCHON.md) | Unreleased experimental Archon CLI and shared-storage contract |
| [`DATA-MODEL.md`](../DATA-MODEL.md) | Durable data, ids, revisions, ledgers, and browser-state boundary |
| [`DESIGN-SYSTEM.md`](../DESIGN-SYSTEM.md) | Dashboard visual and interaction principles and theme ids |
| [`SECURITY.md`](../SECURITY.md) | Public network, identity, filesystem, terminal, and secret-handling boundary |

The four machine-frontmattered specs are `FORMATIONS.md`, `ARCHON.md`,
`DATA-MODEL.md`, and `DESIGN-SYSTEM.md`. They carry
`authority: source-of-truth` and `enforced_by: scripts/doc-lint.py`.

`PRD.md` is product-level authority and is linted for its stable shipped-view
inventory. If it starts carrying more executable invariants, extend the lint
deliberately rather than relying on prose discipline.

## Supporting active docs

| Document | Role |
| --- | --- |
| [`COMPONENTS.md`](../COMPONENTS.md) | Public component and optional-integration map |
| [`docs/CHROTE_VISION.md`](CHROTE_VISION.md) | Short product thesis |
| [`docs/TEST_STRATEGY.md`](TEST_STRATEGY.md) | Test layers, commands, and CI policy |
| [`docs/private-beads-sidecar.md`](private-beads-sidecar.md) | Host-neutral contract for private Beads transport, revision pairing, and restore |
| [`docs/PRD-terminal-lifecycle.md`](PRD-terminal-lifecycle.md) | Terminal iframe and tmux lifecycle constraints |
| [`docs/adr/`](adr/) | Accepted architectural decisions |
| [`dashboard/README.md`](../dashboard/README.md) | Dashboard contributor map and local commands |

Supporting docs explain or operationalize the source-truth specs. They do not
silently override them.

## Host-local operations are not public product truth

Machine-specific paths, service identities, socket ACLs, ports, restart helpers,
rollback binaries, and migration lanes belong in private operator configuration
or the owning infrastructure repository.

Public docs may document generic environment variables, user services, and
security requirements. They must not require another user to reconstruct one
operator's host layout.

`AGENTS.md` and `CLAUDE.md` are contributor/agent instructions for this checkout,
not end-user installation guides. If they contain local execution context, it
must not be copied into public onboarding prose.

## Plans, explorations, and historical material

The following are useful context but are not current product authority:

- [`docs/plans/`](plans/) — implementation plans;
- [`docs/archive/`](archive/) — retired or historical material;
- [`Perttus_vision_for_agent_orchestration/spec/`](../Perttus_vision_for_agent_orchestration/spec/) — active design reference packet where still cited by Formations specs;
- `Perttus_vision_for_agent_orchestration/archive/` — superseded exploration;
- `docs/gascity-*` — evaluation material, not a hidden runtime dependency.

If historical text conflicts with an active source-truth spec, the active spec
wins. Do not repair history into looking current; label or archive it.

## Intentionally absent documents and tools

These names have appeared in older plans but are not current authority:

- `CHROTE.md`
- `SPEC-CHANGELOG.md`
- `ARCHON_BDD.md`
- `FORMATIONS_BDD.md`
- `scripts/dead-link-check.py`
- `scripts/agent-check`

Do not invent them to satisfy stale prose. Add a new canonical document or tool
only when it has a clear owner and enforcement role.

## Current enforcement boundary

`scripts/doc-lint.py` checks:

1. required source-truth frontmatter;
2. valid `enforced_by` paths;
3. required index entries and local links;
4. theme-id parity between code and specs;
5. stable security/runtime facts;
6. absence of host-local operator lanes from public product docs;
7. the shipped PRD view inventory;
8. placeholder repository links.

It intentionally does not decide whether every roadmap paragraph is wise or
every archived note is still interesting. Those are review problems, not regex
problems.

When stable drift recurs, extend the lint. When a check would merely encode a
transient implementation detail, leave it to tests and review.
