# ADR-0016: Restart Review Restores Operator Orientation, Not Arbitrary Workloads

## Status

Proposed 2026-08-18 for `chrote-zjr.2`. This is an operator-facing product
decision and remains proposed until accepted by the owner.

If accepted, this ADR supersedes ADR-0001's generic Session Bank reconstruction
path while preserving its explicit-owner, typed-input, and
unresolved-rather-than-guessed safety rules. It narrows, but does not reverse,
ADR-0015: CHROTE still promises non-interference rather than continuous
supervision or universal reboot recovery.

## Context

Session Bank currently combines several different ideas under one recovery
label:

- an append-only history of tmux sessions observed during normal polling;
- optional agent resume metadata;
- a generic pane topology and typed-command descriptor model;
- separate operator snapshot, restore, and verification tools;
- browser presentation and a second copy of descriptor policy;
- read-only observation of externally managed workloads.

These parts do not form a dependable operator workflow. Normal CHROTE use
records a session row, but it does not produce a typed workload recovery plan.
The operator tools must be installed, configured, and run separately before a
failure. The primary dashboard hides offline work under Settings. When an
authoritative session poll remains empty, the dashboard removes missing
session-to-window bindings after a second poll even when the same response
contains a banked entry for that session. A successful recovery request produces
a toast and refresh, but does not restore the prior cockpit placement or retain
a durable result the operator can inspect.

The result is a system that can preserve a large graveyard of names while
discarding the user's place, and can describe entries as recoverable even though
the deployed workflow never produced the required plans.

The useful operator need is smaller and clearer:

1. A browser refresh or CHROTE service restart should reconnect to work that is
   still alive.
2. After a tmux-server loss or host reboot, CHROTE should show what was present,
   where it belonged in the cockpit, what is still alive, what can be resumed
   safely, and what is gone.
3. CHROTE should resume only workloads with an explicit, validated native resume
   receipt. It should never imply that an empty shell or reconstructed pane grid
   is the original work.
4. The operator should get one bounded review with persistent progress and
   evidence, not an unbounded history list plus transient toasts.

## Decision

### 1. Recovery means orientation plus evidence

CHROTE recovery exists to restore operator orientation after a continuity
boundary and to perform narrow, explicit resume actions where a real workload
contract exists.

Recovery does not mean recreating arbitrary shell state, inferring commands from
process names, supervising every session, or promising that an ordinary agent
survives process death or reboot.

The operator-facing term is **Restart Review**. `Session Bank` becomes a legacy
storage name, not the primary product concept.

### 2. The four continuity boundaries have different behavior

| Boundary | Detection | CHROTE behavior |
| --- | --- | --- |
| Browser refresh or reconnect | Same host boot and tmux-server generation; live sessions remain | Reload preferences, reattach live sessions, and show no recovery incident |
| CHROTE service restart | New CHROTE process; same host boot and tmux-server generation | Replace browser attach processes as required, preserve placement, reattach live sessions, and show a brief reconnect state rather than Restart Review |
| Tmux-server loss | Same host boot; a previously observed tmux-server generation is gone or replaced | Freeze the last authoritative checkpoint and open Restart Review for only the affected configured source |
| Host reboot | Host boot identity changed | Freeze the last authoritative checkpoint and open one Restart Review across affected configured sources after current inventory becomes authoritative |

A socket access error, configured-user partial response, Session Bank read error,
or other non-authoritative observation does not prove loss. CHROTE retains the
last-known-good inventory and reports the error without opening or advancing a
Restart Review.

### 3. Keep one bounded checkpoint and one active incident

After every full authoritative inventory, CHROTE atomically replaces a small
last-known-good checkpoint. The checkpoint contains only what is needed to
explain and act after a boundary:

- an opaque host boot identity and tmux-server generation per configured source;
- qualified session identity, last observation time, and last observed working
  directory;
- a stable CHROTE recovery target id when CHROTE created or explicitly adopted
  the target;
- a minimal cockpit placement hint: terminal workspace and slot, not terminal
  contents or cosmetic panel geometry;
- an optional validated workload resume receipt or external-manager reference.

The checkpoint is not a transcript, shell snapshot, process environment, or raw
command store. Cosmetic layout remains browser preference. The narrow placement
hint is host-owned recovery context so a replaceable browser can still show the
operator where prior work belonged.

When a continuity boundary is proven, CHROTE freezes the prior checkpoint into
one active Restart Review. Per target, the review records current classification,
the operator's selected action, progress, verification evidence, and final
disposition. A CHROTE restart resumes presentation of that review; it does not
silently repeat an action.

Only one active incident exists for a boundary generation. Completed incident
summaries are retained for 30 days and then garbage-collected. The live
checkpoint is replaced, not appended. This removes the unbounded per-session
graveyard.

### 4. Use a small capability model owned by the backend

Each prior target has exactly one backend-derived state:

- `live`: the exact prior target is still present and can be reattached;
- `resumable`: a validated native workload receipt permits an explicit resume;
- `managed`: an external manager owns lifecycle; CHROTE can recheck and attach
  when the target becomes live but cannot start or restart it;
- `new_shell_only`: CHROTE knows a safe working directory but has no workload
  continuity contract;
- `lost`: no safe action can recreate the work;
- `blocked`: evidence is ambiguous, conflicting, unsafe, or unavailable.

The dashboard decodes and presents these backend capabilities. It does not
reimplement descriptor validation, owner arbitration, topology consistency, or
command policy.

### 5. Narrow workload recovery to native resume receipts

A resumable workload has a small `ResumeReceipt` created before loss through an
explicit harness adapter or an operator-confirmed discovery flow. It contains:

- allowlisted harness kind;
- native session id;
- qualified Unix user and recovery target id;
- validated working directory;
- capture time and evidence source.

It does not contain a rendered shell command, arbitrary argv, environment,
transcript contents, or tmux pane topology. At action time the backend derives
canonical argv from the typed receipt and revalidates ownership, target absence,
user, path, and harness support immediately before mutation.

Codex, Claude, and Hermes resume receipts may remain supported through narrow
adapters. Generic command recovery, including the special-case Python HTTP
server descriptor, is retired from the cockpit. A workload needing automatic
process or reboot lifecycle belongs to an explicit external manager, consistent
with ADR-0015.

Existing validated agent metadata can migrate into native resume receipts.
Existing generic recovery plans remain readable for export and diagnosis but do
not authorize new recovery actions after migration.

### 6. Put Restart Review where the loss is visible

Restart Review is part of the Sessions and terminal workspace experience, not a
collapsed Settings section.

After a proven boundary:

- missing targets remain as ghost cards in their prior slots;
- one banner summarizes counts by capability and opens the full review;
- the review groups placed targets by terminal workspace and lists unplaced
  targets separately;
- every target explains what CHROTE knows, what it does not know, and why an
  action is or is not available;
- labels say `Resume agent`, `Open new shell here`, `Check manager`, or
  `Mark as lost`; an empty shell is never called recovered;
- safe bulk resume starts with no targets selected, previews exact actions, and
  skips no target silently;
- progress and failures remain in the review after navigation or CHROTE restart;
- retry is per failed target, and dismissal is an explicit operator action.

Live targets reattach without confirmation because reattachment does not change
workload lifecycle. Resume, shell creation, and any external-manager operation
require explicit operator intent.

### 7. Success requires post-action evidence

An HTTP 200 or tmux session-name match is not recovery proof.

For a resumed target, success requires:

1. a pre-action record showing the prior target absent;
2. a fresh tmux-server/session/process identity attributable to the action;
3. the allowlisted harness adapter confirming the native receipt it resumed;
4. readiness followed by a separate stability observation;
5. a durable action receipt linked to the active Restart Review.

If the adapter cannot confirm native continuity, CHROTE reports `launched,
continuity unverified`, not `recovered`. A new shell is successful when the new
target exists at the validated directory, but it remains `new shell`; it never
inherits workload-success language.

Disposable proof must terminate the exact test-owned tmux server process, prove
that it died, and compare operating-system process identities before and after.
Blocked teardown, unchanged processes, skipped readiness, or failed cleanup are
fatal. Tests must not use pane ids or unchanged session counts as proof of
recreation.

### 8. Migrate without resurrecting stale work

Migration is one-way for the new model but keeps a rollback artifact:

1. Atomically copy the legacy Session Bank file to a timestamped, mode-preserving
   migration backup.
2. Stop adding new observation-only rows to the legacy store.
3. Import only currently live targets and valid native agent metadata into the
   new checkpoint/receipt model.
4. Put all other legacy rows in a read-only `Legacy history` export view. They
   never participate in bulk actions or automatic restore.
5. Require one explicit operator choice to export/keep the legacy backup or
   acknowledge deletion. Until that choice, the backup remains outside active
   recovery. After acknowledgement, delete it; do not retain a permanent hidden
   graveyard.

The existing Session Bank per-entry remove path remains available during the
migration window. No migration action touches live tmux state.

## Implementation sequence

1. **Make current behavior honest.** Preserve banked bindings as missing/ghost
   placement, stop success-only toasts from claiming recovery, and expose current
   backend capability in Sessions.
2. **Introduce boundary-aware inventory.** Add authoritative source state, host
   boot identity, tmux-server generation, atomic last-known-good checkpoint, and
   active Restart Review without launching workloads.
3. **Move presentation to one backend capability projection.** Delete the
   duplicate browser recovery-policy implementation and Settings-first recovery
   workflow.
4. **Add narrow resume receipts and evidence.** Migrate validated agent metadata,
   implement explicit adapter confirmation, durable action receipts, and
   readiness/stability proof.
5. **Retire generic recovery plans and operator-tool coupling.** Remove generic
   topology/command authorization from the product path and keep only bounded
   compatibility/export parsing.
6. **Migrate and garbage-collect legacy state.** Produce the rollback artifact,
   import valid current data, require operator disposition, and stop unbounded
   growth.

Each step is independently releasable and must leave live tmux sessions
untouched during upgrade.

## Rejected alternatives

### Keep patching Session Bank and install the operator tools

Rejected. It preserves the split-brain workflow: normal dashboard use still does
not produce recovery plans, installation does not make pre-failure capture
automatic, placement still disappears, and browser/backend policy remains
duplicated.

### Automatically restart every observed session

Rejected. Observation is not operator intent, a session name is not workload
identity, and bulk resurrection would revive intentionally transient or long-dead
work.

### Restore pane topology as the primary recovery outcome

Rejected. An empty pane grid resembles recovered work without containing that
work. A user who wants a working directory can choose `Open new shell here`.

### Bring back per-session permanence or CHROTE-owned systemd units

Rejected by ADR-0015. It turns a cockpit into a workload supervisor and reopens
privilege, ownership, boot ordering, and owner-access failure modes without a
demonstrated recurring need.

### Remove all recovery behavior

Rejected. The failed implementation does not erase the real operator need to
understand a restart, retain placement, resume harnesses with genuine native
continuity, and explicitly close what cannot be restored.

### Persist arbitrary commands, terminal contents, or process environments

Rejected. They are unsafe, drift-prone, likely to contain secrets, and cannot
prove that replay matches the operator's intent.

## Consequences

- CHROTE makes a smaller but defensible promise: it restores orientation and
  resumes only explicitly supported native workloads.
- Some prior sessions will be marked lost. That is more useful than a false
  recovery claim.
- The generic descriptor, operator CLI, and browser-policy surface can shrink
  substantially.
- A small server-owned checkpoint, placement hint, active incident, and action
  receipt store are added. They are bounded and do not own workload lifetime.
- Browser cosmetic preferences remain local. Only recovery placement hints move
  to host-owned state.
- Externally managed workloads remain read-only observations.
- Product documentation must stop listing generic typed workload reconstruction
  as shipped once this ADR is accepted and implementation begins.

## Enforcement

Before acceptance, the design is reviewed against `PRD.md`, `SECURITY.md`,
`DATA-MODEL.md`, ADR-0001, ADR-0013, and ADR-0015.

Implementation is complete only when tests and a disposable end-to-end harness
prove:

- service restart reattaches live work without opening Restart Review;
- tmux-server death and host-boot change open exactly one incident from the last
  authoritative checkpoint;
- partial and failed inventory never manufacture loss;
- missing targets keep their placement and classification across browser and
  CHROTE restarts;
- each action is explicit, idempotent, durably receipted, and verified after a
  real process replacement;
- legacy rows cannot become executable recovery merely because they exist;
- incident and migration retention are bounded and cleanup failures are fatal;
- no test or runtime path kills unrelated tmux sessions.
