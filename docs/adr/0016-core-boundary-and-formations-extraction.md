# ADR-0016: Keep CHROTE's Core Small and Extract Agent Formations

- **Status:** Accepted
- **Date:** 2026-08-30
- **Owners:** CHROTE maintainers

## Context

CHROTE is a private browser cockpit for host-owned work. Experimental agent
orchestration had grown into a second product inside the repository, while rare
recovery and repair paths repeatedly expanded request-path code. Extra
application access policy also competed with CHROTE's purpose: showing the
trusted operator what the configured service can already reach.

## Decision

1. CHROTE's core product jobs are Terminal, Files, Beads, Scheduled, Server, and
   Settings. Optional local adapters may remain isolated and degradable.
2. Formations, Archon, the Agents view, and their active specifications and
   history belong in
   [chrote-agent-formations](https://github.com/Perttulands/chrote-agent-formations),
   where they remain experimental and outside CHROTE's release promise.
3. Recovery, restore, cleanup, migration, and one-off repair are agent skills or
   operator procedures, not CHROTE request-path features. CHROTE must not kill
   external tmux work during its own restart, but it does not recreate ordinary
   work after process death or host reboot.
4. File access is `CHROTE_ROOTS`, canonical-path containment, and the service
   identity's Unix permissions. CHROTE does not add a second sensitive-path or
   cross-user policy. Explicit grants may add access but never remove owner
   access.

## Consequences

- The repository has one product and one current product specification.
- Experimental orchestration can evolve without compatibility pressure from the
  CHROTE server and dashboard.
- Rare operations retain human judgment and explicit host context instead of
  becoming always-on code.
- A CHROTE endpoint remains terminal-grade access inside a trusted network
  boundary; configured roots and Unix permissions must be chosen accordingly.
- ADR-0015 remains the detailed access and non-interference decision. This ADR
  replaces the removed Formations recovery ADR as the record of why that system
  left the core repository.
