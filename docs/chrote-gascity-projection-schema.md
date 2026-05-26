# CHROTE Gas City Projection Schema

Date: 2026-05-27

This document defines the first CHROTE 3.0 projection contract between
canonical `/home/perttu` Beads work and Gas City runtime orchestration records.

## Source Of Truth

- `/home/perttu` Beads is canonical for work: issue identity, title, priority,
  acceptance criteria, owner, dependencies, lifecycle, and close reason.
- Gas City is canonical for orchestration/runtime facts: city id, session id,
  mail id, molecule/convoy/wisp id, event sequence, nudge delivery, and provider
  state.
- Context Citadel is canonical for durable context. Gas City mail can carry
  context evidence, but it is not durable memory by itself.

## Link Fields

Use explicit ids, not title matching.

| Direction | Field | Meaning |
| --- | --- | --- |
| Beads -> Gas City | `gc_city` | Gas City city name or path label that owns the runtime record |
| Beads -> Gas City | `gc_workflow_id` | Primary Gas City molecule, convoy, sling, or workflow bead id |
| Beads -> Gas City | `gc_session_id` | Valid Gas City session identity assigned to execute the work |
| Beads -> Gas City | `gc_mail_thread_id` | Gas City mail thread or message id carrying the request/result |
| Beads -> Gas City | `gc_event_seq` | Last observed Gas City event sequence relevant to the bead |
| Gas City -> Beads | `home_bead_id` | Canonical `home-*` Beads id |
| Gas City -> Beads | `home_parent_id` | Parent epic/task id when the runtime record is a child step |
| Gas City -> Beads | `projection_kind` | `session`, `mail`, `workflow`, `event`, `artifact`, or `unknown` |

Preferred Beads storage is metadata when available. If a command path cannot
write metadata safely, append a short bead note using the same key names.

Example note:

```text
Gas City projection:
- gc_city: gascity
- gc_workflow_id: gc-12345
- gc_session_id: codex-smoke
- gc_mail_thread_id: gc-12346
- gc_event_seq: 812
```

## Lifecycle Projection

Beads status wins for work lifecycle.

| Beads status | Gas City display rule |
| --- | --- |
| `open` | Show matching Gas City runtime as active, queued, or missing projection. |
| `in_progress` | Show current Gas City session/mail/event state as execution detail. |
| `closed` | Show Gas City runtime as historical evidence; do not reopen Beads from Gas City alone. |

Gas City status wins only for runtime interpretation:

- session is awake/asleep/stopped;
- mail was sent/read/archived;
- nudge delivery succeeded/failed;
- molecule/convoy has pending/running/done child records;
- event stream reports recovery or restart.

Gas City must not silently close, reopen, reprioritize, or reassign a canonical
Beads issue without an explicit `bd` operation.

## Unlinked Runtime Artifacts

When CHROTE observes a Gas City record without `home_bead_id`:

1. Display it as `Unlinked Gas City runtime`.
2. Show city, kind, id, status, and recent event context.
3. Do not infer a canonical bead by matching title alone.
4. Offer a future explicit link action only after the control-surface safety
   boundary exists.
5. If the artifact changes work state, create or link a Beads follow-up through
   normal Beads workflow.

## Reconciliation Rules

- If Beads and Gas City disagree about work lifecycle, Beads wins.
- If Gas City has useful result mail but no Beads link, preserve the mail id and
  create/link a Beads issue before treating the result as durable work state.
- If Gas City points at a closed bead, keep the runtime record as historical
  evidence unless Perttu explicitly reopens the bead.
- If the Gas City supervisor is down, rebuild the projection from Beads plus
  saved `.gc` evidence after recovery.
- If Beads is unavailable, freeze CHROTE/Gas City mutations that would create,
  close, or claim canonical work.

## Non-Goals

- Gas City does not become the work tracker of record.
- CHROTE does not mirror the entire `gc` command tree.
- The browser never receives raw supervisor mutation access.
- Title-only matching is never canonical.
