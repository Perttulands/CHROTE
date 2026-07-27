# ADR-0010: Configured Agent-User Socket Ownership for Formations Execution

## Status

Accepted 2026-07-26 — records owner rulings already deployed and proven on
2026-07-24. Supersedes the socket-topology rejection in ADR-0009 ("A
Formations-only server or socket") and the FORMATIONS.md execution-environment
rules it anchored: the fixed-`/tmp` dogfood restriction and the blanket
`session_target_attachment_audit_unavailable` refusal of every non-temporary or
configured socket.

## Context

A Formations run's agent process runs as the owner of the tmux server the
executor drives: the pane inherits the server owner's Unix identity, not the
identity of the process issuing the tmux command. Agents must authenticate as
the operator, whose credentials live in the operator's home. In a split install
(isolated service user, separate operator — the `/srv` lane), the service user
cannot authenticate as the operator, so the executor must drive a server owned
by the operator. Lazy-starting a server on a configured socket would start one
owned by the service user, silently reverting agents to the wrong identity
(chrote-fjy).

ADR-0009 rejected "A Formations-only server or socket" as violating the
same-pool contract, and FORMATIONS.md forbade the tmux executor from touching
any socket outside fixed `/tmp` (dogfood) or any non-temporary/configured
socket (production). Two owner rulings changed that contract:

1. CHROTE supports ANY user; nothing may hardcode a username. The agent-user is
   configuration (`docs/superpowers/specs/2026-07-24-formations-config-agent-user.md`).
2. The executor shares the cockpit tmux socket by owner ruling, and by owner
   ruling also supports ANY configured socket — including a dedicated
   Formations socket (recorded in `src/internal/formations/tmux_executor.go`,
   validate/ensureServer path).

The implementation shipped (commit 4b8ade3 and successors) and a real
claude-code agent run succeeded on 2026-07-24 on a dedicated operator-owned
socket: run `run_01KYASXZ28GSTGT4ZCP3JV0E3E` in
`archon-real-agent-smoke-20260629t0644z`, `run_succeeded final:true`. The root
spec was never updated, so FORMATIONS.md asserted a contract the deployed
system deliberately violates. This ADR closes that gap (bead chrote-jkk).

## Decision

1. The tmux executor runs on ANY configured socket whose backing tmux server is
   owned by the configured agent-user. `CHROTE_FORMATIONS_AGENT_USER` names
   that user; empty defaults to the service user, so single-user installs need
   zero configuration. No username is ever hardcoded.
2. Ownership is verified against the pinned socket identity before execution.
   A server owned by anyone else fails loud (`agent_user_owner_mismatch`); an
   absent server for a non-self agent-user fails loud rather than lazy-starting
   one under the wrong identity. Only in self mode may the executor lazy-start
   a server. Provisioning a correctly-owned server for a split install happens
   out of band with explicit owner sign-off.
3. A dedicated Formations socket is permitted. ADR-0009's rejection of "A
   Formations-only server or socket" is superseded on this point by owner
   ruling; the accumulated-context goal is served by configuration (pointing
   Formations at the cockpit socket) rather than by prohibition.
4. The fixed-`/tmp` dogfood restriction is removed. Dogfood uses the same
   agent-user-verified executor path on a disposable socket; socket-identity
   pinning (real, stable, non-symlink path, revalidated per operation) still
   applies everywhere.

## Unchanged

- The golden rule: the executor only creates and tears down its own
  uniquely-named sessions and never issues `kill-server`.
- ADR-0009's core finding stands: stock tmux cannot make an owner-accessible
  socket an enforcement boundary, and the same-UID input fence remains
  infeasible without an operation-time kernel boundary (`ctx-ug7.37` path).
  This ADR supersedes only the socket-topology rejection, not that analysis.
- Attachment of Formations slots to existing cockpit Terminal sessions (the
  session-target ceremony in FORMATIONS.md) remains unavailable and
  uncertified. This ADR governs the executor's own created sessions only.

## Consequences

- FORMATIONS.md "Execution environments" items 2 and 4 are rewritten to match
  this contract (same change as this ADR).
- Certification language elsewhere in FORMATIONS.md that cites the fixed-`/tmp`
  boundary or the blanket configured-socket refusal must route here instead.
- A wrong or stale `CHROTE_FORMATIONS_AGENT_USER` fails every run loud at
  validate time; that is intended and must not be papered over with a fallback.
