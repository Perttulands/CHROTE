# CHROTE Vision

CHROTE is a private cockpit for host-owned work.

The browser is disposable. Host-owned state—not browser state—is authoritative.

## Canonical source

[`../PRD.md`](../PRD.md) is the canonical product source for current behavior and
roadmap boundaries. This document preserves the short version and should not
become a second product spec.

## Operating principle

Everything important stays on the configured Linux or WSL host:

- terminals and tmux sessions;
- AI agents and their native harness state;
- files, dev servers, builds, tests, and logs;
- Beads workspaces;
- schedules and runtime history;
- experimental Formations definitions, missions, gates, and run ledgers when in use;
- explicitly configured local services.

CHROTE gives the human one browser-based control surface for that state. The
browser is glass, not the vault.

## Product direction

CHROTE grows only where it improves the operator's ability to understand and
deliberately coordinate host-owned work:

1. **Host-owned cockpit:** three terminal workspaces, unified Sessions/Files
   sidecars, Files, Beads, Agents, scheduling, and server health.
2. **Local capability surface:** explicit adapters for services that earn a
   first-class operator workflow.
3. **Auditable orchestration experiment:** file-backed Formations, missions,
   ports, gates, run ledgers, and controlled executor promotion, without calling
   the experiment shipped before its release gate passes.
4. **Agent teams:** stronger ownership, reservations, handoffs, and human
   approval once those contracts are proven under real use.

CHROTE should not become a pile of dashboards for things that are already better
in a terminal. It should make host-owned work legible without stealing ownership
from the tools and files that created it.

See [`meta-harness-desired-state.md`](meta-harness-desired-state.md) and
[`agent-collaboration-primitives.md`](agent-collaboration-primitives.md) for
roadmap ideas. They are exploration, not active runtime promises.
