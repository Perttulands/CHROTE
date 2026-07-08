# ADR-0004: Mission Rooms Are Agent-Team Work Ledgers

## Status
Accepted

## Context
CHROTE previously deleted `ChroteChat`; the cockpit is not a generic chat app. The new requirement is different: humans and multiple agents need a durable place to coordinate real work, leave auditable evidence, and recover context without private tmux side channels.

The architecture proposal in `/tmp/chrote-comms-architecture-proposal.md` framed the right primitive: append-only event streams bound to work objects, projected into a Slack-shaped reading surface. Two standalone dogfoods outside `/srv/chrote` tested that framing:

- `ctx-q8x.10` proved a one-human/four-agent room state model with ordinary posts, passive mentions, human Send without a second confirmation, and blocked automated nudges.
- `ctx-q8x.11` ran five real Codex agents through a file-backed room and produced 72 contiguous events, 7 claims, 17 artifacts, and 18 passive mentions. It proved room posting works, but free-text ownership failed: several agents claimed overlapping UI-shell work.
- The structured dogfood in `/srv/prototypes/chrote-agent-team-structured-dogfood-20260704-010611` added structured claims/reservations/projection and ended with 59 contiguous events, 5 agents, 5 claims, 4 done, 1 supervisor-cancelled stale claim, 28 artifacts, 0 open claims, 0 unaddressed mentions, 0 projection risks, and no tmux injections.

The load-bearing problem is not chat UX. It is agent-team work ownership, provenance, and recovery.

## Decision
Mission rooms are **work-object-bound append-only ledgers for agent-team communication and coordination**. They are not free-floating channels, DMs, reactions, presence, or a standalone Slack clone.

Every room is attached to a durable CHROTE work object:

- formation run rooms;
- bead rooms;
- board/project rooms;
- agent/session rooms;
- artifact, gate, and decision contexts.

CHROTE renders room-shaped projections, but the authoritative write target follows the object:

- finalized formation run ledgers are read-only projections; post-run discussion uses a sibling comms ledger keyed by run id;
- bead discussion text stays authoritative in `bd comments`; CHROTE may add only thin overlay metadata such as mentions, attachment refs, or nudge outcomes;
- board/project/agent rooms use CHROTE-owned NDJSON comms ledgers under the same local-file durability discipline as Formations;
- projections are disposable and rebuilt from ledgers/backing stores.

CHROTE-owned comms events must have a schema/version field, monotonic per-room sequence, append-only writes, `flock`/fsync durability, and replayable projections. Do not introduce SQLite or another runtime persistence dependency for v1.

Before any writer or live agent integration, the room contract must include these first-class concepts:

- claim lifecycle: `claimed`, `narrowed`, `blocked`, `superseded`, `cancelled`, `done`;
- claim fields: owner, category/slice, dependencies, reserved paths/files, expected artifacts, verification command/evidence, artifact event links;
- path reservation conflict detection;
- broad directory/subtree reservation warnings so a single agent cannot silently monopolize a work tree;
- artifact salvage for cancelled/superseded stale claims, with `salvaged_by`, `verified_by`, and original claim-owner provenance, never owner impersonation;
- pinned human boundaries/decisions;
- passive mention triage with addressed/unaddressed state;
- one canonical projection/export reused by UI, CLI, tests, and agents.

Human-authored Send is the authorization; do not add a second confirmation modal for ordinary human sends. Mentions are passive inbox metadata and must never write into tmux or a live session by themselves. Automated, gate-triggered, agent-originated, or ambient nudges remain disabled unless a later explicit policy enables them. No autonomous agent-to-agent routing or auto-reply loops in v1.

## Rationale
Work-bound ledgers preserve CHROTE's cockpit identity while giving agents the coordination primitive they actually need. Append-only NDJSON with file locks matches existing Formations recovery and rollback discipline, keeps server/CLI peer-writer parity possible, and avoids adding a new local database just for v1 projections.

The dogfoods made the priority obvious: ordinary room messages are easy; preventing duplicate agent work and preserving evidence is hard. Structured claims, reservations, pinned boundaries, mention triage, artifact linkage, and salvage provenance are therefore product core, not later polish.

## Alternatives Considered
- **Generic Slack/Chat tab:** rejected. It repeats the removed ChroteChat shape and creates channels detached from work evidence.
- **SQLite-backed room store:** rejected for v1. It adds a second persistence model and new concurrency/rollback concerns when NDJSON + `flock` already matches the repo.
- **Shared markdown peer planes:** rejected as the live protocol. Markdown remains an export; ledgers are the source of truth.
- **Second Beads issue/comment database:** rejected. `bd comments` remains bead discussion authority.
- **Automatic agent-to-agent nudges:** rejected for v1. Passive mentions and pull-based reading are safe; live-session writes require explicit audited activation.
- **Second confirmation for human-authored Send:** rejected. Pressing Send is already intent; extra confirmation belongs only to automated/dangerous live-session actions.

## Consequences
Mission-room implementation must start with read-only projection, then writer commands, then live agent send/nudge integration. A pretty chat pane before the ledger/projection/claim contract would be the wrong order.

The model is more structured than chat and slightly heavier for agents, but it makes ownership, handoff, and recovery inspectable. It also leaves some future work explicit: UI affordances for broad reservation warnings, timeout/stale-worker policy, and CHROTE-facing browser dogfood before production integration.

## Enforcement
- Prototype source: `/srv/prototypes/agent-team-rooms`.
- SDD contract: `/srv/prototypes/agent-team-rooms/SPEC.md`.
- Prototype verification command: `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v`.
- Structured dogfood report: `/srv/prototypes/chrote-agent-team-structured-dogfood-20260704-010611/STRUCTURED_DOGFOOD_REPORT.md`.
- Beads: `ctx-q8x.1`, `ctx-q8x.12`, `ctx-gio`, and later CHROTE-facing browser dogfood `ctx-q8x.13`.

Any CHROTE production implementation must cite this ADR, preserve the backing-store routing above, and prove projection/writer behavior against the structured claim/reservation/salvage contract before adding live nudge delivery.
