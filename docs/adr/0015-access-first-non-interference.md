# ADR-0015: CHROTE Preserves Access but Does Not Own Workload Durability

## Status

Accepted 2026-08-09 — product and architecture decision.

Supersedes [ADR-0014](0014-persistent-agents-supervised-by-systemd.md)
entirely. It also supersedes the former CHROTE workload-recovery and continuous
supervision rules.

## Context

CHROTE is a private cockpit for one owner operating host-owned work. Its useful
lifecycle property is non-interference: closing a browser or restarting CHROTE
must not deliberately kill a tmux session that has an independent host
lifetime.

That property was repeatedly expanded into a different promise: that a cockpit
lock should recreate an agent after process death and host reboot. Implementing
that promise requires CHROTE to coordinate process ownership, cross-user
privilege, systemd user managers, state-directory ownership, receipts,
transitions, startup ordering, and reboot acceptance. The resulting mechanisms
have not demonstrated recurring operator value. Permission and ownership
changes made in pursuit of tighter boundaries have instead periodically blocked
the owner from their own work.

The design had conflated two independent properties:

- **Non-interference:** CHROTE does not kill external work merely because its
  browser or server lifecycle changes.
- **Recovery:** some other component recreates work after that work or the host
  exits.

CHROTE requires the first. It does not universally provide the second.

## Decision

### 1. Owner access is the primary local invariant

CHROTE's configured Unix accounts are trusted operator identities used for
process ownership, harness separation, and tmux routing. They are not hostile
tenants.

CHROTE does not tighten or replace workspace, session, or socket ownership,
modes, or ACLs to manufacture local isolation or durability. Explicit
operator-configured additive grants may be applied or refreshed, and normal
configured-root/path checks remain, but those grants must never reduce owner
access. Without such configuration, a request path reports missing access
rather than reshaping the owner's permission topology.

### 2. CHROTE promises non-interference, not universal recovery

A browser disconnect or CHROTE restart must not cause CHROTE to terminate an
external tmux server, session, or workload. Browser terminal attaches may be
replaced as recorded by
[ADR-0013](0013-ttyd-restart-lifecycle-and-orphan-reaping.md); the underlying
external work is not CHROTE's to reap.

Non-interference does not make every CHROTE child durable. CHROTE retains the
existing `KillMode=process` boundary because ordinary session creation can
start a tmux server as a CHROTE child when no server exists. Changing that
lifecycle while retiring Persistence v2 would risk killing sessions on a server
restart. This preserves current behavior; it is not a promise to recreate work
after its process or the host exits.

Ordinary sessions and agent processes are otherwise best-effort. They may exit,
be stopped by their owner, or disappear with the host. CHROTE does not promise
to recreate them or resume their native transcript after process death or host
reboot.

### 3. CHROTE has no per-session permanence capability

CHROTE does not expose a "make permanent" lock, own continuous agent desired
state, install per-agent supervision units, publish liveness receipts, or
control host units for ordinary sessions.

### 4. Rare durable workloads stay explicit and host-owned

When a workload has a demonstrated need to survive process death or reboot, the
operator may configure it directly in the host's init system. That configuration
is outside CHROTE's product lifecycle and permission-management path. CHROTE may
observe an externally managed workload, but it does not become its recovery
owner.

Adding a future durability capability requires a new product decision backed by
recurring operator value. It is not justified merely because more ownership,
privilege, or reboot hardening is technically possible.

## Rejected alternatives

- **Complete and harden Persistence v2.** Rejected because the privilege helper,
  unit lifecycle, state transition journal, receipt protocol, preflight, and
  reboot matrix are disproportionate to demonstrated value and increase the
  chance that the cockpit blocks owner access.
- **Keep the lock but describe it as best-effort.** Rejected because
  "permanent" or "locked" implies a durability promise the product does not
  keep.
- **Automatically recover every observed session.** Rejected because topology
  and process evidence do not establish operator intent, and universal recovery
  would recreate transient work that was allowed to end.
- **Tighten local account separation until automatic recovery is safe.**
  Rejected because configured accounts are trusted operational identities, not
  tenant boundaries, and preserving owner access is the higher-value invariant.
- **Change the service cgroup boundary while retiring persistence.** Rejected
  because ordinary session creation can still start a tmux server as a CHROTE
  child. That lifecycle needs its own evidence and decision; this retirement
  must not alter which existing sessions survive a CHROTE restart.

## Consequences

- Removing the lock and its host-control machinery reduces product surface,
  installation burden, partial-state recovery, and permission failure modes.
- A CHROTE restart may replace browser attach processes, but must leave external
  tmux work alone.
- A host reboot may end ordinary sessions and agents. That is an explicit
  product boundary, not a degraded persistence state.
- Operators with a real durability requirement manage that small set of
  workloads explicitly at the host layer.
- CHROTE continues to fail loud when it cannot access configured work. It does
  not silently reinterpret an inaccessible pool as empty or mutate permissions
  to make the error disappear.

## Enforcement

- `PRD.md` owns the non-interference and no-universal-recovery product contract.
- `SECURITY.md` owns the trusted-local-account and access-first permission
  boundary.
- Product code and installation artifacts contain no per-session permanence
  action, Persistent Agents control path, CHROTE-owned agent unit template, or
  privilege helper for agent supervision.
- The CHROTE service lifecycle remains unchanged; retiring persistence does not
  alter ordinary tmux survival behavior.
- Reintroducing any of those surfaces requires an ADR that supersedes this one
  and identifies the demonstrated operator need.
