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
| Why CHROTE exists | [`VISION.md`](../VISION.md) |
| Durable product requirements | [`PRD.md`](../PRD.md) |
| System ownership and runtime shape | [`ARCHITECTURE.md`](../ARCHITECTURE.md) |
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
| [`VISION.md`](../VISION.md) | Product purpose, intended experience, and enduring direction |
| [`PRD.md`](../PRD.md) | Durable product requirements, operator outcomes, and non-goals |
| [`ARCHITECTURE.md`](../ARCHITECTURE.md) | Runtime structure, state ownership, component boundaries, and failure behavior |
| [`DESIGN-SYSTEM.md`](../DESIGN-SYSTEM.md) | Dashboard visual and interaction principles and theme ids |
| [`SECURITY.md`](../SECURITY.md) | Public network, identity, filesystem, terminal, and secret-handling boundary |

`DESIGN-SYSTEM.md` is the machine-frontmattered spec. It carries
`authority: source-of-truth` and `enforced_by: scripts/doc-lint.py`.

`PRD.md` is product-level authority and is linted for its stable view inventory.
If it starts carrying more executable invariants, extend the lint deliberately
rather than relying on prose discipline.

## Supporting active docs

| Document | Role |
| --- | --- |
| [`COMPONENTS.md`](../COMPONENTS.md) | Public component and optional-integration map |
| [`docs/TEST_STRATEGY.md`](TEST_STRATEGY.md) | Test layers, commands, and CI policy |
| [`docs/private-beads-sidecar.md`](private-beads-sidecar.md) | Host-neutral contract for private Beads transport, revision pairing, and restore |
| [`docs/adr/`](adr/) | Accepted decisions; [`ADR-0016`](adr/0016-core-boundary-and-formations-extraction.md) records the core boundary and extraction, [`ADR-0017`](adr/0017-terminal-viewing-model.md) the terminal viewing model, and [`ADR-0018`](adr/0018-terminal-transport-ownership.md) the terminal transport |
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

## Non-current ideas

[`docs/legacy-ideas.md`](legacy-ideas.md) is the only idea graveyard. It is not a
plan, backlog, or product authority. Worthwhile work must become a Bead before
implementation; all current claims remain in the sources above.

## Current enforcement boundary

`scripts/doc-lint.py` checks:

1. required source-truth frontmatter;
2. the shipped PRD view inventory;
3. local Markdown links.

`scripts/host-neutrality.py` separately checks tracked files for operator-local
topology. Review owns product judgment that cannot be reduced to these checks.

When stable drift recurs, extend the lint. When a check would merely encode a
transient implementation detail, leave it to tests and review.
