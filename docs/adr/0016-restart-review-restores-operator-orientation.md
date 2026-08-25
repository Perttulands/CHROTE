# ADR-0016: Recovery Is Skill-First; CHROTE Exposes Evidence and Exact Launch

## Status

Accepted 2026-08-25 for `chrote-zjr.2.3`.

This record replaces the proposed 2026-08-18 version of ADR-0016. It supersedes
ADR-0001's generic command and pane-topology reconstruction path while keeping
its explicit-input and unresolved-not-guessed rules. It does not supersede
[ADR-0015](0015-access-first-non-interference.md): CHROTE still preserves access
and does not own ordinary workload durability.

## Context

An abrupt host restart destroys tmux state without identifying which native
Claude or Codex conversation, worktree, Bead, bridge, authority mode, or model
the operator means to resume. That identity is open-world: evidence lives in
provider stores, Git, Beads, tmux, service managers, bridge state, and the
operator's request.

The current recovery surface divides that problem across:

- Session Bank observation and persistence in `src/internal/api/tmux.go`;
- generic agent, command, topology, managed, and unresolved descriptors in
  `src/internal/api/recovery_descriptor.go`;
- a second capability and validation implementation in
  `dashboard/src/sessionBankRecovery.ts`;
- Settings-first recovery controls in `SessionBankSection.tsx`;
- separate snapshot, restore, verification, manifest, and disposable-smoke tools
  under `scripts/tmux-recovery/`.

This is too much machinery and still cannot determine operator intent reliably.
The prior ADR-0016 proposal added another checkpoint, incident ledger, capability
model, and workflow. That would make CHROTE a second recovery supervisor rather
than solve the identity problem.

The external proofs `home-nhn6` and `home-tj89` establish a smaller working
model: a shared skill can correlate bounded native evidence, stop on ambiguity,
preview an exact action, and verify continuity; deterministic code remains
responsible for trustworthy facts and narrow mutations.

## Decision

### 1. Responsibilities are split at four explicit seams

| Owner | Responsibility | Must not do |
| --- | --- | --- |
| CHROTE | Publish qualified live/offline evidence, preserve non-interference and placement hints, and provide one collision-safe exact-launch primitive | Rank provider sessions, inspect transcript bodies, infer intent, supervise arbitrary workloads, or retry through a recovery state machine |
| Shared `agent-session-recovery` skill | Discover and rank native candidates, diagnose the failed boundary, preview the exact provider action, sequence recovery, and verify continuity | Own credentials, mutate CHROTE persistence directly, bypass deterministic ownership/path checks, or guess through ambiguity |
| External service manager or bridge | Own declared durable workload lifecycle, leases, Telegram deduplication, and its own atomic state | Let CHROTE become a second writer or restart owner |
| Operator | Supply intent and authorize ambiguous, destructive, permission-escalating, or external actions | Be represented by a heuristic such as newest mtime, `--last`, or ambient `--continue` |

The skill is one flat top-level package in the shared skills repository. CHROTE
references it by name but does not vendor, mirror, profile-copy, or restate its
provider recipes.

### 2. The four disruption contracts remain distinct

| Disruption | Surviving authority and evidence | Allowed CHROTE mutation | Fail-loud outcome |
| --- | --- | --- | --- |
| Browser refresh or reconnect | Current authoritative tmux inventory plus browser presentation state | Reattach to the exact live target; restore presentation only | A partial source or attach error is shown as an error, never as disappearance |
| CHROTE service restart | External tmux servers and sessions remain authoritative; persisted Session Bank evidence is orientation only | Recreate browser attach processes; do not create workload sessions | If inventory is incomplete, preserve stale evidence and report the failed source |
| Tmux-server or session loss | A complete inventory proves the target absent; Session Bank, native provider stores, Git, Beads, bridges, and service managers remain separate evidence sources | None until an operator-approved exact launch; then create only a fresh owned target | Access, protocol, ownership, native-ID, cwd, or candidate ambiguity stops recovery without changing live state |
| Host reboot | External managers own their declared durable lanes; native provider stores and project state may survive; ordinary tmux work does not | Same exact-launch contract after fresh inventory; never bulk-recreate observed history | Missing native identity or workspace evidence is reported as lost/unknown, not converted into a shell or guessed session |

A socket permission failure, configured-user error, protocol failure, or partial
multi-user response is not proof of absence. CHROTE marks the affected source
incomplete and keeps last-known evidence explicitly stale.

### 3. CHROTE exposes a minimum evidence interface

For each configured terminal source, CHROTE returns:

- qualified source and Unix-user identity;
- complete, partial, or failed inventory status with a bounded error code;
- observation timestamp and source/server generation when available;
- exact live tmux session and pane identities, PID, cwd, and ownership status;
- matching Session Bank evidence: last seen, last cwd, provider/native ID when
  already recorded, and last placement hint when available;
- external-manager or bridge status as a separately attributed observation.

Every field retains its source and freshness. Offline means that a complete
current inventory does not contain the previously observed qualified target.
Stale means current truth could not be established. CHROTE never collapses
stale, partial, inaccessible, and absent into one empty list.

This interface contains no transcript text, prompt, response, environment,
credential, inferred command, or candidate ranking. Missing fields stay missing.

### 4. CHROTE exposes one exact-launch interface

The mutation leaf `chrote-zjr.2.1` may add one deep module that accepts only:

- an exact configured terminal source and Unix user;
- a validated absolute cwd inside an approved root;
- a configured or validated executable plus structured argv, never a shell
  command string or caller-supplied environment;
- a fresh idempotency key and requested visible label.

Immediately before mutation, CHROTE revalidates source/user binding, cwd,
executable, target absence, and collisions. It derives the socket and fresh tmux
identity, writes a creation marker, launches once, and returns immutable
session/pane/process identities plus the observed cwd. Partial creation cleans
up only the exact newly owned target or fails loud with that target retained for
inspection.

The interface never accepts a provider candidate list, `latest`, `--last`,
`--continue`, a transcript path, topology reconstruction, retry policy, or a
request to adopt an existing pane. CHROTE does not add permission-bypass flags.
A skill may preview a bypass flag only when prior authority proves it; granting a
new bypass is a separate operator-authorized boundary change.

### 5. Current components have explicit dispositions

| Current component | Disposition | Migration rule |
| --- | --- | --- |
| Qualified multi-user tmux inventory, partial-failure reporting, and non-interference in `tmux.go` | **Keep** | These are evidence and safety primitives; strengthen qualification where `chrote-zjr.2.1` proves a gap |
| Ordinary `POST /api/tmux/sessions` fresh-shell creation | **Keep** | It remains an explicit new-shell action and is never presented as native-session recovery |
| Session Bank observation, last-seen/cwd/native metadata, remove, and read paths | **Simplify** | Keep bounded orientation and export evidence; stop letting mere observation authorize reconstruction |
| `recovery_descriptor.go` codec and strict validation | **Simplify** | Keep read-only legacy decoding and diagnostics during migration; remove active canonical command selection after replacement proof |
| `UpdateBankedRecovery`, `RecoverBankedSession`, topology recreation, agent canonical argv, and the Python HTTP command special case | **Replace**, then **Delete** | `chrote-zjr.2.1` supplies evidence plus exact launch; `chrote-zjr.2.2` removes the generic executable path only after proof and rollback/export readback |
| `managed_recovery_status.go` | **Keep** | Continue exposing external-manager status as attributed read-only evidence; CHROTE never restarts it |
| `dashboard/src/sessionBankRecovery.ts` browser capability/validator copy | **Delete** | The browser renders backend evidence and explicit action previews; it does not decide recovery safety |
| Settings-first `SessionBankSection` recovery workflow and success-only toast | **Replace** | Sessions presents global offline orientation and action results; Settings may retain storage maintenance only |
| Legacy `resumeCommand`, recovery-plan fields, manifests, and stored descriptor rows | **Unknown** | Preserve readable/exportable state until `chrote-zjr.2.2` proves disposition; Unknown data never authorizes mutation |
| `scripts/tmux-recovery/` snapshot/restore/verify/schema/smoke suite | **Unknown** | Keep untouched while mining conformance and rollback evidence; decide archive, shrink, or deletion only in `chrote-zjr.2.2` |
| ADR-0001 and historical recovery records | **Keep as history** | Mark superseded authority accurately; do not rewrite history to look current |

Unknown means preserve and exclude from active authorization. It does not mean
keep indefinitely.

### 6. Migration is proof-gated and one-way at the active interface

1. `home-nhn6` and `home-tj89` prove the canonical skill and bridge branch outside
   the CHROTE repository.
2. `chrote-zjr.2.1` adds only missing evidence fields and the exact-launch module,
   with disposable tests and no candidate selection.
3. The shared skill uses those interfaces in an end-to-end disposable CHROTE
   recovery proof.
4. `chrote-zjr.2.2` removes or archives generic reconstruction and duplicate
   browser policy only after replacement readback exists.
5. Legacy data remains exportable until its disposition is verified. No migration
   step touches unrelated live tmux sessions or external-manager state.

There is no new checkpoint store, incident ledger, recovery daemon, transcript
index, topology database, or hidden task database.

## Rejected alternatives

### Expand the generic recovery-plan engine

Rejected. More descriptor modes and state transitions still cannot decide which
native conversation or operator intent is correct, and they duplicate external
supervision.

### Put every recovery invariant in the skill

Rejected. Skills are good at contextual judgment, not atomic persistence,
ownership checks, collision safety, redaction, leases, Telegram deduplication,
or service supervision.

### Automatically restart every observed workload

Rejected by ADR-0015. Observation is not intent; ordinary work is transient and
rare durable lanes already have explicit host owners.

### Keep only read-only history and make agents use raw tmux

Rejected. Evidence without a collision-safe exact mutation forces every agent to
reimplement ownership, cwd, socket, and partial-create safety.

### Add the prior Restart Review incident model

Rejected. A checkpoint, active incident, capability projection, progress ledger,
and migration workflow are another recovery state machine. Session Bank evidence,
the shared skill, Beads, and existing service managers already own those facts.

## Consequences

- CHROTE's recovery promise gets smaller and more honest: evidence, placement,
  non-interference, and one exact launch.
- Open-world candidate selection stays adaptable without weakening deterministic
  host boundaries.
- Some legacy entries become inspection-only or lost. That is preferable to a
  plausible-looking empty shell.
- The active product path shrinks after proof; legacy parsers may temporarily
  remain larger than the final design.
- Provider flags and bridge procedures evolve in the shared skill, not in a
  CHROTE release.

## Enforcement

- `home-nhn6` and `home-tj89` own the external skill and bridge proofs.
- `chrote-zjr.2.1` owns the evidence and exact-launch implementation.
- `chrote-zjr.2.2` owns retirement and legacy rollback/export disposition.
- ADR-0015 continues to own non-interference and no-universal-supervisor policy.
- Tests must prove complete-versus-partial source semantics, multi-user
  qualification, collision refusal, structured argv, fresh ownership identities,
  exact cleanup, and zero mutation of unrelated tmux state.
- Documentation and code must contain no CHROTE-local copy of the shared skill or
  provider-specific resume recipes.
