# ADR-0003: Roll Back Active Gas City Integration

## Status

Accepted

## Context

CHROTE's attempted Gas City integration drifted toward surfaces that did not
match the operator's desired system. In particular, the work overemphasized a
Gas City dashboard/tab and prototype plumbing before proving that the core
operator experience would improve.

The desired CHROTE baseline remains simple: easy access to named tmux sessions,
with future orchestration only when it clearly supports agent collaboration.
Gas City may still be useful later, but the current active integration is not
going in a direction worth continuing.

## Decision

Roll CHROTE back to the pre-Gas-City active runtime path.

CHROTE keeps only Gas City documents, ADRs, plans, and research notes as
historical context. Active CHROTE code and service configuration must not expose
Gas City APIs, a Gas City tab, Gas City session creation, `gc:` terminal attach
handling, or a `CHROTE_GASCITY_CITY_DIR` runtime dependency.

The full pre-rollback state remains recoverable through the safety branch and
rollback snapshot. `/home/perttu/gascity` is not deleted by this rollback.

2026-05-30 follow-up: the local runnable Gas City runtime was later removed in a
separate cleanup to reduce confusion. The thinking, docs, and conversation
artifacts were retained. Removal manifest:
`/home/perttu/rollback-snapshots/gascity-runtime-removal-20260530-215854/manifest.txt`.

## Consequences

What gets better:

- CHROTE returns to the earlier working tmux access model.
- The product avoids accumulating prototype Gas City wiring as hidden tech debt.
- Future Gas City work must start from a fresh explicit decision and vertical
  slice rather than continuing a confusing partial integration.
- Existing research is still available for later evaluation.

What gets harder:

- Any future Gas City adoption must reintroduce the integration deliberately.
- Previously created prototype code cannot be treated as the current
  implementation path.
- Agents must read these ADR statuses before assuming CHROTE 3.0 is still
  Gas-City-backed.

What remains risky:

- Historical docs may still describe candidate Gas City designs. They are
  evidence, not standing implementation instructions.
- A future reintroduction still needs proof of real agent collaboration value,
  not just dashboard visibility or mock-agent plumbing.
