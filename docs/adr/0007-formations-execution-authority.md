# ADR-0007: Formations Runtime Has One Fenced Workspace Coordinator

## Status
Accepted target; implementation is incomplete

## Context
Current main has one shared Go package and append-only run files, but it does not
have one semantic execution owner. The API constructs a run engine and executes
the request synchronously before replying. Archon constructs a separate local
engine for start and resume, and both API and Archon can append a final
`run_canceled` event directly. Per-file locks serialize bytes; they do not stop
two processes from making conflicting dispatch or recovery decisions.

ADR-0001 makes run replay explicit, ADR-0005 defines the redacted-input and
cleanup boundary, and ADR-0006 fixes the mixed-workflow, target-lease, result,
and finality contracts. None of them chooses which process admits commands,
owns the private run tree, or may act after another process takes over.
Durable asynchronous execution also needs a command identity before the client
can safely retry a response lost across a crash.

## Decision

### Separate definition writes from runtime authority

The shared Formations package remains the only serializer for board, layout,
and persona-definition mutations. UI and Archon may continue to author,
validate, and inspect those workflow definitions without a running coordinator.
New elements may receive the shared connection-aware placement heuristic, but
no create, connect, validate, save, run, replay, or reconnect path may rearrange
existing user coordinates. Full-board layout changes require the explicit UI or
Archon `arrange` action; they are definition-side mutations, never coordinator
recovery or execution side effects.

For runtime state governed by `WorkspaceAuthority.authoritySchema=2`, the CHROTE
server coordinator is the sole semantic
writer for one configured workspace. Start, resume, cancel, and human-verdict
commands are submitted to it. Archon never falls back to a local run engine and
never reads the writer-private ledger directly. Run list, status, logs, follow,
and actions require the coordinator's sanitized projection. If the coordinator
is unavailable or does not own the workspace, runtime commands fail loudly
before mutation or external side effect.

A shared-package file lock remains a byte-integrity mechanism. It is not an
execution lease, a fencing token, or permission for a peer process to dispatch.
ADR-0006's host-wide per-target lease remains a separate resource arbiter: a
current workspace owner still must acquire the exact target lease before send.

Production target resolution uses the same configured Terminal-session resolver
and inventory that powers cockpit Terminal tabs. That inventory is the union of
the explicitly configured user/socket sources; Formations does not own a second
production source or pool and does not require clean-room sessions. Accumulated
agent context is an intended input to selection. A persona stem that matches
more than one inventory source is ambiguous and fails loud rather than choosing
a raw socket path. Reproducibility therefore means the same session lineage, not
an empty session. Each dispatch binds the exact pane incarnation and ADR-0006's
closed pane/history baseline in the writer-private ledger before send. The
baseline distinguishes earlier history from post-baseline pane evidence
associated with this dispatch; that later evidence may be operator-influenced
and is not claimed to have sole-agent causality.

A session that is ambiguous, stale, already leased, attached, or not certified
closed/ready fails loudly at slot binding and final atomic acquisition; a hidden
pooled iframe still counts as attached. Closed/ready means fresh proof for the
exact fingerprint from the certified harness adapter's non-pane control channel,
never quiet output, prompt text, or silence. A certified open turn reports
`session_target_harness_busy`; missing or non-unique readiness evidence reports
`session_target_readiness_unknown`; inability to arm complete client/input
monitoring reports `session_target_attachment_audit_unavailable`. Binding never detaches a client. The user
may explicitly disconnect a CHROTE-owned presentation client and retry, but
cannot reclaim an external attachment or another run's lease. Formation
attachment ownership begins only when the exact target-registry occupancy is
fsynced, not when a slot binding is frozen. After the matching ledger dispatch
is durable, an exact CHROTE-issued run-bound Peek capability is the sole
authorized additional attachment. Shared Terminal attach paths deny an occupied
target unless using that capability. Only the latest fsynced issuance is valid;
a later issuance requires prior clients/input drained, atomically invalidates
every older token/generation, and makes a superseded route reaching the target
boundary foreign. A certified monitor accounts for every
attach/detach/target-selection, resize/reflow, history,
pane-lifecycle/topology/other mutation, and input-capable command/control route,
including transient clients and raw tmux send-key routes; registered Peek bytes
must terminate at the steering-generation gate. A foreign event or lost monitor
continuity revokes Peek, forbids result/ordinary release, and holds
or quarantines the target pending non-authorizing quiescence proof. Such a
lost-history latch releases only on exact old-pane-incarnation-gone proof;
current-client absence or a later ready turn is insufficient.
This is an enforceable adapter boundary, not a CHROTE process mutex. Stock tmux
on an owner-accessible raw socket permits independent `send-keys`, paste,
control, select, attach, and resize commands and therefore cannot certify the
boundary. Until `ctx-ug7.21` selects, `ctx-ug7.22` implements, and `ctx-ug7.23`
certifies a same-session-pool enforcement primitive, that adapter reports
`session_target_attachment_audit_unavailable` and sends nothing. Those gates do
not authorize a Formations-only production socket or pool: Terminal tabs and
Formations must still resolve the same target and lineage through the shared
inventory.

The legacy disposable adapter is trusted dogfood only. It rejected configured,
default, and non-`/tmp` sockets until
[ADR-0010](0010-formations-agent-user-socket-ownership.md) removed that
restriction (2026-07-26); it still rejects symlink-escaped sockets, records the
initial socket identity, and revalidates that identity before each adapter call
so an observed between-call retarget fails before the next list, describe,
capture, or send.
That check is not a same-UID enforcement primitive: a dogfood process can still
race inside a stock tmux command or use an owner-accessible raw socket directly.
Consequently this adapter and its path checks cannot certify production access;
only the `.21` through `.23` capability can do so.

Peek is a full user-initiated interactive attach to the exact qualified live
target. It may send literal input, including control characters, and steer or
interrupt the agent. That human interaction is part of the same live session
lineage, but it is not a second automatic workflow dispatcher: only the current
fenced coordinator may perform workflow prompt sends, retries, coordinator
interrupts, or lifecycle transitions. Input is serialized under the target
occupancy through a monotonic steering generation: a generation is durable
before its first bytes are forwarded, result closure waits until it is closed,
and the certified non-pane-byte closure proof exact-matches the latest generation.
Later input invalidates an earlier proof. Projection marks operator influence
without persisting raw keystrokes. On restart, input stays suspended until the
capability and recovered occupancy exact-match again.
Normal closure, cancel/failure reconciliation, and finality first stop capability
issuance, drain input, close any open generation, and durably record irreversible
capability revocation. The certified boundary admits only the barrier-bound
one-shot workflow prompt, metadata from a durable Peek capability issuance,
generation-gated Peek bytes, and one exact ledger-before-send/no-resend
reconciliation interrupt bound to the cancel request or frozen failure start.
Closure proof binds revocation plus a continuous interaction audit and keeps a
mutation/input fence through receipt fsync. A terminal hold is non-authorizing
run evidence, not interactive run control; no run-bound input capability
survives cancel/failure reconciliation or an execution-final event.

The pane grid captured in the dispatch baseline remains frozen while the target
is active. Peek tile movement/resize is viewport-only and cannot send tmux
resize or `SIGWINCH`; a real resize, history clear/trim past the boundary,
reflow, or otherwise unresolvable baseline fails closed without result or
ordinary target release. Closing or moving Peek never creates, kills, or
rebinds a session.
Disposable workspaces, ports, sockets, and sessions remain mandatory isolation
for certification and dogfood where specified; they do not define production
session topology.

### Bind the workspace to one renewable owner and monotonic fence

Each configured workspace receives one server-issued opaque
`workspaceAuthorityId`, stored under the writer-only Formations host-authority
root and bound to the exact configured workspace-root identity. A caller-supplied
path, a path alias, a changed symlink target, or a same-named workspace cannot
select or replace that authority. Rebinding or moving a workspace is an explicit
migration; an identity mismatch fails closed.
The id matches canonical uppercase ULID grammar
`^wsa_[0-7][0-9A-HJKMNP-TV-Z]{25}$` and is validated before directory construction.

For `WorkspaceAuthority.authoritySchema=2`, the existing explicit
`CHROTE_FORMATIONS_DATA_ROOT` server
configuration seam supplies `<formations-host-authority-root>`. It is one stable,
absolute, once-opened root shared by every CHROTE lane capable of that authority
schema on the host. It is not derived from a lane's service data directory, a workspace, a
Files root, caller input, or ambient working directory, and there is no per-lane
fallback. Independent injected private roots are test fixtures only: two lanes
using different roots are non-production and cannot claim host-wide authority.
The server configuration and provisioning layer supplies this root; `ctx-ug7.15`
owns publication and active/retained inventory. This decision authorizes no live
path migration, service change, deployment, or UID migration.

Writer trust is the dedicated CHROTE service UID. Production provisioning names
that account in the tracked `services/chrote-srv.service` unit as `User=chrote`;
the service manager and kernel resolve the account and launch the coordinator
with its effective UID. Server construction passes that kernel effective UID as
the numeric writer identity to the authority library, which checks the live
effective UID and the `st_uid` of the opened host root, private directories,
files, and lock files. No board, authority file, caller value, environment
variable, or separately configurable numeric UID may self-assert writer trust.
The required modes are exact directory mode `0700`, exact regular-file mode
`0600`, no special mode bits, and link count one for files. Same-UID agent
processes are outside the supported topology because Unix modes cannot isolate
peer processes sharing a UID; agents must not run as the service UID. Tests may
inject an expected UID and disposable root, but production uses only the
provisioned account plus kernel credential. No UID is persisted in authority
JSON, and this ADR authorizes no UID or service-configuration migration.

First registration is serialized by a coordinator-local mutex plus the
process-shared host registry lock at the stable
`<formations-host-authority-root>/workspaces/` parent, before an authority-id directory
or its `owner.lock` can be selected. Under that lock the registrar strict-validates
closed `registry.private.json` schema 1, then opens the root
once, derives the identity below, and enforces uniqueness both for the cleaned
configured spelling and for the opened `(device,inode)` identity. The private
registry maps that identity to one authority id. An alias, changed target, second
mapping, or conflicting orphan requires explicit migration and cannot create a
second owner domain. That shared mapping selects the corresponding `owner.lock`.
Creation fsyncs the new bootstrap/private directory before
publishing the registry mapping and fsyncing its parent. Recovery may complete
one unique exact unregistered creation; a conflict is non-authorizing and fails
loud without choosing either directory.
The registry entries are exactly `{workspaceAuthorityId,configuredPath,device,
inode,workspaceRootIdentitySha256}`, sorted by decoded unsigned numeric `device`,
decoded unsigned numeric `inode`, then valid UTF-8 `configuredPath` bytewise;
unknown schema/key, duplicate identity, or conflicting mapping permits no
selection or mutation.

`workspace-root-identity-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{configuredPath,resolvedPath,device,inode}`. Paths are absolute valid UTF-8 with
NUL rejected, lexical dot segments removed, `/` separators, and no trailing
slash except root. `configuredPath` retains the cleaned configured spelling;
`resolvedPath`, device, and inode come from the same race-safe opened directory
identity after symlink resolution. Device and inode are JSON strings containing
canonical unsigned base-10 `uint64` (`0` or a non-zero digit followed by digits,
with no sign or leading zero), so RFC 8785 never rounds them.
`workspaceRootIdentitySha256` hashes those exact bytes with no trailing newline;
the object remains private.

An immutable `workspace.bootstrap.json` contains only bootstrap schema, authority
id, root-identity encoding, and root-identity hash. A process encodes it as RFC
8785 `workspace-bootstrap-jcs-v1` over exactly
`{bootstrapSchema,workspaceAuthorityId,rootIdentityEncoding,
workspaceRootIdentitySha256}` with no unknown key or trailing newline. The
mutable `workspace.private.json` separately carries the current
`authoritySchema` as a monotonic high-water mark for every command, run, and
private record in that workspace authority domain.

The `authoritySchema=2` Phase-B capability registry is immutable binary-owned code, never a
persisted workspace record and never selected by a board. Its complete required
set is the bytewise-ordered pair
`formations.runtime-authority-read-guard.v1`, then
`formations.workspace-authority.v1`. The complete pair validates before an
`owner.lock` path is selected or taken and before fence allocation. The
read-guard capability remains non-authorizing. The workspace-authority
capability authorizes only workspace registration, private publication, owner
lease and fence, and command-journal foundation work; semantic projection, run
reconciliation, cleanup, quarantine, and execution remain false and
unavailable. A missing, duplicate, unknown, or unsupported required capability
fails before any owner-lock selection or owner/fence mutation.

A process then takes the combined coordinator-local mutex and process-shared
`owner.lock` and strict-validates the closed bootstrap and current workspace
authority record plus its exact hash-matched current immutable admission-policy
generation and complete contiguous prior-hash chain to revision 1 without
mutation. A missing generation, hash/revision discontinuity, or cycle is invalid.
An unsupported, missing, or conflicting bootstrap, workspace authority, or
policy schema causes lock release and remains strictly
read-only: it does not allocate a fence, clean, quarantine, project a run as
valid, or touch tmux. Matching schema numbers are not by themselves a capability
grant. Admission under `authoritySchema=2` stays disabled until the complete safe
projector and coordinator/reconciler are registered and every runnable rollback
binary is certified to honor this bootstrap-and-workspace guard; binaries older
than that guard are not runtime-safe rollback targets.

Only then may a coordinator reserve a strictly increasing `writerFence`, fsync
the advanced counter, and publish the renewable host-local owner lease. A crash
may leave a counter gap but can never reuse a fence. Takeover and renewal use the
same lock. `owner.lock` is held continuously for the owner epoch and is the
exclusion truth; `owner.private.json` is evidence and fencing, not an availability
veto after the kernel lock is released. `acquiredAt`, `renewedAt`, and
`leaseUntil` are canonical UTC `time.RFC3339Nano` renderings using `Z`; parsing
and formatting back to UTC must reproduce the exact string, and
`acquiredAt <= renewedAt < leaseUntil`. A process that still holds the kernel
lock but has reached `leaseUntil` must publish a valid renewal before authority
work. Once that lock is released, including by crash, a successor may allocate
and publish a strictly higher fence regardless of the predecessor's unexpired
wall-clock lease. Time never bypasses a still-held kernel lock. An implementation
that cannot prove these locking semantics may not execute Formations.

The same lock is the authority critical section. It is held from current
lease/fence validation through each command admission, ledger/private-state
write and fsync, or bounded non-idempotent prompt send, Tool spawn, cancel
interrupt, cleanup, or quarantine call. Fence allocation cannot race between a
check and its side effect. A stale owner, or an expired owner that has not first
renewed, fails `stale_workspace_fence` and performs no operation.
Lock order is host registry (registration only), workspace owner, then
host target arbiter; no path acquires them in reverse.

Every ledger event with event schema 2 records its origin `writerFence`. Valid history is a
monotonic non-decreasing sequence of allocated owner epochs: an older prefix
remains authoritative after takeover. A lower fence after a higher event, an
unallocated fence, or a new append/effect not using the current fence is invalid.
Private records likewise preserve immutable `originWriterFence`; a takeover may
reconcile them only through a new transition carrying its current
`stateWriterFence`, never by pretending the origin fence equals the new fence.

Immutable private files are never written in place at their canonical path. Under
the governing lock, the writer emits complete bytes to a unique same-directory
staging file, fsyncs it, atomically installs the canonical name with no-replace
semantics, then fsyncs the parent directory. An existing canonical file is
success only when its strict bytes/hash are the exact requested immutable value;
otherwise it conflicts and is never replaced. A crash before install leaves only
a non-authorizing staging file; a crash after install can expose only complete
bytes, and recovery repeats the parent fsync. Only the governing authority holder
(the registrar under `registry.lock` or current fenced owner under `owner.lock`)
may remove a validated unreferenced staging file. Mutable registry generations
publish under `registry.lock`;
workspace counter/authority, owner-lease, and command-record generations publish
under `owner.lock`. Immutable admission-policy generations publish under
`owner.lock` before the current workspace ref changes. Each mutable record starts
at `recordRev=1`. Every overwrite-style mutable JSON record (`WorkspaceRegistry`,
`WorkspaceAuthority`, `WorkspaceOwnerLease`, and `RunCommandRecord`) carries the
closed field `priorGeneration`, exactly `null` or `{recordRev,sha256}`. Revision 1
requires `null`; revision N greater than 1 requires `recordRev=N-1`. Their sole
accepted target encodings are respectively `workspace-registry-jcs-v1`,
`workspace-authority-jcs-v1`, `workspace-owner-lease-jcs-v1`, and
`run-command-record-jcs-v1`. Each encoding is the complete closed corresponding
JSON record (one exact state variant for a command record), serialized as RFC
8785 canonical UTF-8 with no BOM, insignificant JSON whitespace, or trailing
newline. The closed path-to-encoding map is
`registry.private.json=workspace-registry-jcs-v1`,
`workspace.private.json=workspace-authority-jcs-v1`,
`owner.private.json=workspace-owner-lease-jcs-v1`, and
`commands/<commandId>.json=run-command-record-jcs-v1`.
No persisted encoding discriminator is added. `priorGeneration.sha256` is the
64-lowercase-hex SHA-256 of the exact complete previous-generation bytes in that
same named encoding.

`RunCommandRecord.commandPayload` remains the closed `run-command-jcs-v1` JSON
object. Its payload hash is computed from those canonical payload bytes; the
object is then embedded as JSON and the complete containing command record is
re-encoded as `run-command-record-jcs-v1`, never stored as an opaque string or
pre-encoded byte field. Immutable admission-policy generations retain only their
existing `priorPolicySha256` chain. Every successful replacement is exactly the
last published revision plus one and rejects a stale, skipped, or regressed
predecessor. The caller's expected predecessor must exact-match the closed next
record's `priorGeneration`. Only that authenticated binding permits an exact-next
retry after canonical replacement. Desired-state idempotency with a false or
stale predecessor remains rejected; without record-specific proof the
format-neutral publisher rejects a consumed predecessor and recovery rereads
authoritative current state.

These four encodings are the only accepted target encodings for their mutable
record families. Where a record carries `registrySchema`, `leaseSchema`, or
`commandSchema`, that value is exactly `1`; `WorkspaceAuthority.authoritySchema`
is exactly `2` for this target. Pre-freeze or experimental records that reuse
those schemas or the target workspace-authority shape but omit required
`priorGeneration` are malformed, non-authorizing, and never auto-upgraded. This
decision authorizes no migration of shipped or live records and no deployment.
Before `formations.workspace-authority.v1` may authorize foundation work, the
guard must make every permissively decoded or non-canonical legacy form
non-authorizing, including bytes that do not exact-match the named encoding and
records missing required `priorGeneration`; compatibility parsing may expose
inspection evidence only.
Each replacement uses a
generation-checked same-directory temp file, file fsync, atomic rename, and
parent-directory fsync. Migration holds both locks in the declared parent-then-
workspace order. A torn, missing, stale, or conflicting published record is
non-authorizing and fails loud; a leftover temp file cannot outrank the last
valid published generation. A canonical immutable path is never a partial-file
recovery surface.

Every JSON record/policy revision governed by `authoritySchema=2`, writer-fence field, allocated
event/effect sequence, workspace-admission identity, and next counter is an integer in
`1..9007199254740991`. Allocation that would exceed that range fails closed
before mutation or effect; values are never rounded, wrapped, or reused.
The only zero-valued sequence-reference sentinels are `priorIssuedSeq`,
`capabilityIssuedSeq`, `latestCapabilityIssuedSeq`, and
`originCancelRequestSeq`, plus private-journal `routeSeq`. Each is in `0..9007199254740991`, and `0` never names
or allocates an event.

The versioned foundation reader capability
`formations.runtime-authority-read-guard.v1` has one explicit parser resource
envelope: each closed JSON record and each NDJSON event is at most 1 MiB, and
JSON container nesting is at most 64 levels. Exceeding either ceiling returns a
typed non-authorizing rejection. These ceilings apply to one record or event,
not to history: the guard streams directory enumeration, complete ledgers, and
the complete admission-policy chain without a lower run-count, ledger-size, or
policy-revision cutoff. Changing these parser ceilings requires a versioned
guard capability and rollback certification; a matching authority schema alone
does not widen them. Its successful result reports bounded per-class ledger
counts, not an in-memory run-path inventory; exact sanitized run inventory and
projection remain the projector's responsibility.

### Bind each run to one immutable bootstrap

Each private run directory contains one immutable
`runs/<run-id>/run.bootstrap.json`. It is RFC 8785 canonical UTF-8 JSON encoded
as `run-bootstrap-jcs-v1`, with no unknown keys or trailing newline, over exactly
`{runBootstrapSchema,workspaceAuthorityId,runId,runAuthorityId,
graphSnapshotEncoding,graphSnapshotSha256,privateBindingsEncoding,
privateBindingsSha256}`. `runBootstrapSchema` is `1`;
`graphSnapshotEncoding` is `run-graph-snapshot-toml-v1`; and
`privateBindingsEncoding` is `run-private-bindings-toml-v1`. Hashes are exactly
64 lowercase hexadecimal characters without a prefix. The three ids use the
existing uppercase-Crockford prefixed-id grammar: `wsa_`, `run_`, or `auth_`
followed by 26 characters, the first in `0..7`.

The complete graph and private-binding files publish immutably before the run
bootstrap. Only the bootstrap selects that authoritative pair; stray complete
snapshot or binding files authorize nothing. `ctx-ug7.6.2` later exact-binds the
bootstrap identity and hashes into `run_started`; this accepted target does not
claim that implementation exists yet.

### Journal every runtime command before its effect

The private workspace command journal accepts a closed `start`, `resume`,
`cancel`, or `verdict` command with a client-stable `commandId` and an RFC 8785
canonical `run-command-jcs-v1` payload hash. Command ids are unique within the
workspace. Compatibility spellings such as `abort` or `stop` normalize to
`cancel` before canonicalization; they do not create another command or state.

`commandId` matches canonical uppercase ULID grammar
`^cmd_[0-7][0-9A-HJKMNP-TV-Z]{25}$`, validated before any path construction; the journal file
is exactly that id plus `.json`. Every lookup/create/update is serialized in the
workspace authority critical section and uses create-if-absent semantics, so
concurrent requests have one durable linearization point.

The sole `commands/<commandId>.json` journal file fsyncs the complete closed
canonical payload, its hash, immutable `admittedWriterFence`, current
`stateWriterFence`, and state before the semantic effect. It contains no second
actor authority: actor is read only from the hash-bound payload. On read it
recanonicalizes the stored payload and rejects a hash, kind, or containing-
workspace mismatch. The matching `run_started`,
`run_resumed`, `run_cancel_requested`, or `human_verdict_recorded` event binds
that command id and payload hash; every duplicated event field, including actor,
workspace/run identity, reason, mode/verdict, and precondition sequence,
exact-matches the stored payload. The same record then becomes the durable
receipt: `pending` has no outcome fields; `applied` has exactly `runId`,
`effectSeq`, immutable `outcomeWriterFence`, and
`decisionAdmissionPolicyRef`; `rejected` has exactly `rejectionCode`, that
outcome fence, and the decision policy ref. The ref exact-matches the immutable
generation used for every terminal start decision and is `null` for every
non-start command. `stateWriterFence` names the fence that last published the
record; `outcomeWriterFence` separately names the applied event's origin fence
or the rejection decision fence. A replacement at F2 may therefore repair an F1
effect with `stateWriterFence=F2` and `outcomeWriterFence=F1`.
`RunCommandReceipt` is only the closed API projection of that terminal record,
not another file. Takeover never changes `admittedWriterFence` or a terminal
outcome fence, but records its current fence when it publishes command state.

The canonical payload domains are closed. `authoritySchema` is integer `2`;
`runRoot.kind` is `mission` or `formation` and its stable node id must match;
`resumeMode` is `reattach` or `retry-failed-producer`; `verdict` is `pass` or
`fail`. Revisions and sequences are JSON integers in `1..9007199254740991`.
`limits.maxDispatch`, `maxAttempts`, and `wallClockSeconds` are JSON integers in
`1..2147483647`, and `limits.redact` is boolean. An absent expected ETag is `""`;
otherwise it is exactly 64 lowercase hexadecimal characters. Identifiers use
their registered stable-id validators, and every SHA-256 field is exactly 64
lowercase hexadecimal characters except the explicitly empty revision-1
`priorPolicySha256`. Actor/reason normalization is fixed by
the data contract. Unknown keys, wrong JSON types, non-canonical enums, and
out-of-range values are rejected before journaling.

- The same `commandId` and payload hash returns the original durable receipt.
- The same id with another hash returns `command_id_conflict` and has no effect.
- A different start id is a different request and may create another run.
- Cancel is also semantically unique per run: a later alias or key cannot replace
  the first cancel snapshot or create a second cancellation state.
- Resume exact-matches the blocked sequence it consumes. Verdict exact-matches
  the outstanding human-request sequence. Stale commands are typed rejections.

For start, the current owner performs bounded preflight, fsyncs the pending
command and writer-private immutable authority, appends/fsyncs
`run_started(seq=1)`, appends/fsyncs `run_activated` when immediate capacity was
reserved, and durably links the receipt to that run before returning the run id.
Long-running graph execution is not part of the response. A response lost after
`run_started` is recovered by command id and returns the same run id.

A bounded workspace FIFO uses immutable `workspaceAdmissionSeq` from
`run_started`; no volatile queue is authority. `maxActiveRuns` is a JSON integer
in `1..2147483647`; `maxQueuedRuns` is a JSON integer in `0..2147483647`. There
is no implicit policy default. Workspace creation publishes closed immutable
`state=disabled` revision 1, which rejects new starts as `admission_disabled` and
pauses queued activation without canceling active or queued work; queue wall
clocks continue. Only a closed configured generation admits or activates.

Each policy is RFC 8785 `workspace-admission-policy-jcs-v1` and lives at immutable
`admission-policies/<policyRev>.json`. Revision 1 has
`priorPolicySha256=""`; every later JSON-safe revision is exactly the previous
revision plus one and carries the previous generation's SHA-256. An update binds
its expected current revision/hash and exact canonical next bytes. Under
`owner.lock` and the current fence, it stages/fsyncs and atomically
no-replace-installs the next chained generation before atomically
advancing/fsyncing the `WorkspaceAuthority` revision/hash ref. If the current ref
still names the expected predecessor and that exact next generation is already
installed, retry completes the ref change. If the current ref already names that
exact next generation whose prior hash is the expected ref, retry returns the
original success. Every other stale ref or byte/hash conflict fails before
mutation. Referenced historical generations are retained. Every terminal start
command binds its exact decision policy
revision/hash; `run_started` binds the admission generation, and every immediate
or dequeued `run_activated` binds the configured generation used for activation.

Under one workspace admission critical section, `activeCount` is the number of
non-final ledgers with `run_activated`, and `queuedCount` is the number whose
latest projected status is exactly `queued`. An unactivated `canceling` or
`failing` run is not queued and can never activate. `maxActiveRuns` alone gates activation.
Before a fresh start may activate, the coordinator activates eligible queued
runs by smallest `workspaceAdmissionSeq` while capacity remains. It may then
reserve and fsync a new admission sequence and append `run_started` plus immediate
`run_activated` when capacity still exists, even when `maxQueuedRuns=0`. With no
active slot, `maxQueuedRuns` alone gates the fresh queued admission: append only
`run_started` when `queuedCount < maxQueuedRuns`; otherwise durably reject
`run_queue_full` with the decision policy ref before a run directory or event.
Lowering `maxActiveRuns` blocks only new activation until active count fits;
lowering `maxQueuedRuns` blocks only fresh queued admission at or above the new
limit and never blocks dequeue. Neither change cancels or reorders admitted
work. Counter reservation is fsynced before publication, gaps are allowed, and
sequence reuse is forbidden.

`run_started` alone projects `queued`; the unique fenced `run_activated` projects
`running` and is required before every graph/dispatch event. On capacity release,
the coordinator activates the smallest eligible queued admission sequence under the same
lock before dispatch. Queue wait begins at admission and consumes wall-clock;
an expired queued run may fail without activation. Restart derives both counts
and FIFO order for current state from run ledgers and strict-validates every
retained policy ref. `authoritySchema=2` has no workspace-global admission-decision sequence:
policy refs attribute an individual decision to exact policy bytes but do not
independently prove the historical cross-run interleaving of capacity changes.
Serialization correctness comes from the continuously held authority lock and
the required contention/crash tests. Concurrent starts, `maxQueuedRuns=0`,
cancellation, cleanup, and recovery use the same formula; reconciliation takes
precedence over fresh activation.
Every failure, including an expired queued run, first fsyncs
`run_failure_reconciliation_started`, projects non-final `failing`, and freezes
the exact `{code,reason,unrecoverable,relatedSeq,failureCause}` header plus all
open-resource snapshots. `run_failed.failureReconciliationSeq` names that start,
byte-matches the header, and exactly disposes those snapshots. Direct failure
uses `originCancelRequestSeq=0`; cancel escalation exact-names its unique request,
preserves prior slot-interrupt state, and never resends a durable request.
In this first contract, activation consumes one `maxActiveRuns` slot until the
run reaches an execution-final event, including while blocked, `waiting_human`,
canceling, or failing. This conservative rule makes capacity reconstruction depend
only on the ledger. Releasing and durably requeueing non-final runs would require
a later explicit lifecycle/schema decision.

### Persist a Formation result before projecting its output

For an ordinary workflow Formation, mutable capture and in-memory aggregation
are not replay authority. Before a target lease can become result-closed, the
writer parses its bounded terminal capture once, registers any new artifacts,
and appends/fsyncs `slot_result` with one closed
`slot-turn-result-jcs-v1` envelope and hash. That envelope carries the exact
durable turn payload and declared port projections needed by the frozen
formation-type rule, or hash-only redacted projections when policy forbids them.

After the complete successful schedule or its first non-`ok` result and output
validation, the writer deterministically derives and appends/fsyncs one unique
`formation_result` for the node attempt
from the immutable graph snapshot's fixed formation-type rule plus those turn
envelopes. The required schedule is fixed: `solo` has its sole terminal `solo`
turn; `flow` has one `flow-step` per persisted slot and the last is terminal;
`peer` has one `peer-turn` per persisted slot then the first slot performs
terminal `peer-facilitator`; `orchestrated` has the unique controller perform
`leader-plan`, each non-controller slot perform one `leader-worker` turn in
persisted order, and the controller perform terminal `leader-agentic`. Every
turn is a coordinator-owned slot dispatch/result under the workspace and target
leases. Every dispatch binds a closed `turnInputs` value containing the exact
`nodeStartedSeq` plus an ordered array of prior
`{slotResultSeq,turnResultSha256}` references. `solo`, the first `flow-step`,
each `peer-turn`, and `leader-plan` use no prior turn result. A later `flow-step`
uses only its immediate predecessor. `peer-facilitator` uses every `peer-turn`
in persisted slot order. Each `leader-worker` uses only `leader-plan`, while
`leader-agentic` uses `leader-plan` followed by every `leader-worker` in
persisted worker order. Every phase also consumes the exact frozen node inputs
named by `nodeStartedSeq`; these references and the prompt hash make composition
and audit deterministic. A controller may not mutate, prompt, capture, or
interrupt another Formation-bound tmux target directly; a future dynamic
multi-turn action protocol requires an authority-schema bump.

Only an `ok` terminal turn may carry a non-empty declared-port `outputs` map, and
it must contain all and only the Formation's declared outputs. Every non-terminal
turn and every non-`ok` turn uses `{}` and communicates through `turnPayload`.
An `ok` turn advances the schedule; all required `ok` turns map to
`formation_result.status=done`. The first `error` or `needs-review` result stops
the schedule. `error` maps to `failed` and repeats its exact engine-normalized
non-routable error `turnPayload` for every declared output; `needs-review` maps to
`needs-review` and repeats the fixed non-routable
closed `{availability="available",exact=true,payload={kind="unavailable",
code="formation_needs_review",message="Formation requires review",
retryable=true}}` projection. A bounded closed capture with a valid terminal
sentinel but invalid declared outputs becomes the closed turn payload
`{availability="available",exact=true,payload={kind="error",
code="invalid_formation_outputs",message="Formation outputs do not match the declared ports",
retryable=true}}` with `outputs={}` and no raw parser text. A pre-result resource block
uses `run_blocked` and creates no Formation result; `blocked` is not a
`formation_result` status. The contributing sequences are the exact completed
prefix, making recovery deterministic. These fixed retryable outcomes may enter
only the existing explicit whole-producer retry selection after quiescence; they
never route or retry automatically.

Result report comes from the deciding last contributing turn; artifact and diff
ids are the stable first-seen union across that prefix. These phase/slot
identities are recorded in dispatch and result.
The result contains the complete durable safe canonical `outputs` map, `outputHashes`,
prior artifact ids, contributing slot-result sequences, and an exact canonical
result hash. New non-Tool artifacts are registered before that event.

`node_output` exact-matches and names the Formation result sequence/hash. A
crash after `formation_result` may therefore materialize `node_output` once
without reparsing capture or dispatching again. For `Redact=true`, only the safe
projection is durable. Fresh execution may retain the paired exact value in
process memory across the safe fsyncs needed for `slot_result`,
`formation_result`, `node_output`, Gate pass-through, join readiness, and each
downstream dispatch. Each exact value remains paired to its source input or turn
result until every scheduled intra-Formation consumer and every taken-edge
consumer has either composed/sent its one prompt or durably become
non-deliverable and no retry/Gate path retains it. It is erased immediately after
that closed consumer set drains, and always at cancellation/finality. Process
loss discards it.
Recovery may finish evidence projection, but if further routing then requires
that raw output it records terminal
`redacted_input_unavailable` under ADR-0005. Tool results retain ADR-0006's
same-event result/registration rule; judge results retain their dedicated
contract.

### Recover only under the current fence

A replacement coordinator validates the supported bootstrap and current
workspace authority record plus its current immutable policy generation, then
reserves and fsyncs a newer workspace fence
before reconciling commands, admitted ledgers, target leases, Tool scopes, and
redaction obligations. Private staging and obligations preserve immutable
command/origin-fence identity; recovery validates that historical owner epoch,
then records its action under the current higher state fence.

The crash outcomes are fixed:

1. A durable pending start with no admitted run resumes bounded admission from
   its stored canonical payload or returns one typed rejection. Pending resume,
   cancel, or verdict rechecks its exact named precondition and appends its effect
   once only if still valid; otherwise it records a stable rejection. It never
   guesses fields from the payload hash.
2. Any matching `run_started`, `run_resumed`, `run_cancel_requested`, or
   `human_verdict_recorded` effect durable without a receipt repairs the applied
   receipt from that event and never reapplies the command. The repaired record
   carries the current `stateWriterFence` but preserves that event's origin fence
   as `outcomeWriterFence`. `run_started` remains the same admitted run and is
   scheduled once.
3. `slot_dispatch` durable with uncertain send has one unmatched lease. Reattach
   only; never resend or supersede the prompt.
4. A successful scheduled-terminal or first non-`ok` deciding `slot_result`
   durable with `formation_result` missing derives that result once from
   immutable slot-turn envelopes and never dispatches the next phase. A durable
   `formation_result`/`tool_result` with `node_output` missing materializes the
   exact output once. Neither path reparses mutable capture.
5. `node_output` durable with routing or finality missing replays remaining
   delivery and finalization once under the current fence, subject to redacted
   input availability and all open-resource proofs.
6. `run_cancel_requested` durable resumes cancellation reconciliation only.
   Ordinary recovery and dispatch remain forbidden.
7. `run_failure_reconciliation_started` durable projects non-final `failing`
   and resumes only its exact frozen cause/header and open-resource snapshots.
   It cannot select another cause, return to ordinary execution/canceling, or
   resend a durable slot-interrupt request, including one with no outcome.

A private authority directory without valid seq-1 `run_started` remains a
non-authorizing orphan. Only the current fenced owner may clean it. Recovery
first completes every understood pending raw cleanup and fsyncs the obligation,
then removes the orphan and fsyncs its parent. Missing, conflicting, unsupported,
or unprovable identity/cleanup quarantines the entire tree with no public bytes,
replay handle, send, or adoption.

### Version authority changes; ignore only projection-only extensions

Ledger event schema 2 includes the command identity, workspace authority, writer fence,
Formation result, root-input projection, and authored-configuration manifest in
its required authority semantics before the first event-schema-2 run is admitted.

Admission is gated on the immutable workspace bootstrap, the current mutable
workspace authority-schema high-water mark, and a certified rollback set in
which every runnable binary implements the non-authorizing guard for both. A
pre-guard binary is prohibited from runtime use against that workspace; "prior
version" means a certified guarded rollback version, not arbitrary old
current-main code.

An authority-schema upgrade holds `owner.lock`, validates the current fence, and
atomically advances/fsyncs `workspace.private.json.authoritySchema` before
publishing any command, run, event, or private record with the new semantics.
The high-water mark never decreases. A crash after advancing it but before new
authority publication is safely unavailable to an older reader, not silently
downgraded. A new authority schema starts a new run and does not reinterpret an
older ledger; explicit migration is required to leave that high-water domain.

After that point, any private event, record, or field that can change admission,
identity, dispatch, result acceptance, routing, cleanup, cancellation, or
finality requires a supported authority-schema bump. An older or unsupported
reader may inspect only a separately certified sanitized projection. It never
allocates a fence, adopts authority, cleans, quarantines, dispatches, accepts a
result, mutates execution state, or finalizes. A supported current coordinator
owns cleanup/quarantine after acquiring its fence; unknown identity fails loud.

A registered projection-only extension may remain ignorable only when it is
redaction-classified, excluded from every public safe-field allowlist, and
incapable of changing status, actions, bindings, artifacts, or execution. It
does not carry authority merely because a private reader recognizes it.

## Rationale
The existing server is already the durable host boundary, so a second scheduler
daemon adds lifecycle and deployment complexity without solving a proven need.
A workspace lease plus monotonic fence closes split ownership, while command
receipts close lost-response ambiguity. Durable result authority closes the
remaining result-before-output replay gap. Keeping definition work offline
preserves agent-first authoring without granting a disconnected CLI execution
authority. A single host-authority root makes the workspace lock and fence
domain honest across CHROTE lanes. Binary-owned capabilities prevent stale
workspace bytes from claiming implementation support, while the immutable run
bootstrap and authenticated mutable predecessor make the selected snapshot and
every retried transition independently verifiable.

## Alternatives Considered
- **Keep API and Archon as peer engines:** rejected. File locks serialize bytes
  but cannot choose one dispatch, recovery, or cancellation owner.
- **Let the browser own execution:** rejected. Closing the cockpit must not stop
  admitted work.
- **Add a standalone daemon now:** rejected. The CHROTE server is the narrower
  host-owned coordinator boundary.
- **Use only an in-memory queue:** rejected. Restart would lose admitted work and
  command idempotency.
- **Persist binary capability support in the workspace:** rejected. A workspace
  record can outlive or misrepresent the active and retained binaries.
- **Use a registry and owner lock below each lane's service data root:** rejected.
  Two lanes could authorize the same opened workspace with independent fences.
- **Let an unexpired wall-clock lease veto takeover after kernel-lock release:**
  rejected. It reduces crash availability without strengthening exclusion.
- **Treat content-addressed graph and binding files as self-selecting:** rejected.
  Stray complete files do not choose one authoritative pair for a run.
- **Treat same-UID agents as isolated by private modes:** rejected. Unix modes do
  not isolate processes that share the writer UID.
- **Accept exact desired bytes after a false or consumed predecessor:** rejected.
  That turns a stale claim into successful mutable-transition authentication.
- **Add a workspace-global admission-decision ledger now:** rejected. Current
  recovery needs current counts/FIFO and exact policy attribution; a new
  cross-run forensic authority surface is not required for execution correctness
  and would need its own schema, crash protocol, projection, and retention rules.
- **Return only after execution finishes:** rejected. It couples run lifetime to
  client connectivity and leaves retry outcome ambiguous.
- **Give Formations a separate clean production tmux pool:** rejected. The
  cockpit pool and accumulated session context are intentional product inputs;
  exact lineage/baselines and fail-loud arbitration provide the evidence and
  safety boundary.
- **Keep run Peek watch-only:** rejected. User steering is an explicit product
  capability. Exact-target authorization and coordinator-only automatic
  dispatch preserve the authority boundary without suppressing user input.
- **Treat all registered private fields as forward-compatible:** rejected.
  Ignoring a field that changes authority can make an old reader dispatch or
  finalize incorrectly.
- **Reparse capture after a crash:** rejected. Captures are mutable evidence and,
  for redacted runs, may already have been replaced.

## Consequences
Runtime commands require a reachable coordinator and stable command ids. Queue
capacity can reject starts before admission, and a code rollback may become
inspection-only for newer authority schemas. Pre-bootstrap-guard binaries are
not rollback candidates for a workspace at `authoritySchema=2`. Workspace moves
need explicit identity migration. Every lane capable of that authority schema
must receive the same explicit
host-authority root and run under the dedicated service UID; changing either is
separate operational migration work. A still-held owner kernel lock fails closed
until the owner exits or an operator intervenes, while a released lock permits a
higher-fence takeover without waiting for stale lease time.
Because an activated blocked or human-waiting run retains capacity in this first
contract, operators may need a larger `maxActiveRuns`; releasing that capacity
requires an explicit durable requeue design.
Admission inspection can explain each recorded policy identity and reconstruct
current counts/FIFO, but it cannot present a nonexistent global historical
decision order as proof. Adding that forensic order requires a later explicit
authority-schema decision.

Session reuse means rerunning the same graph against a different lineage is not
the same execution, and user steering can intentionally affect an in-flight
agent. Operators gain continuity and direct control, while certification must
prove exact target/baseline attribution and fail-loud busy or attached binding
instead of claiming clean-room determinism. Disposable-socket tests still prove
isolation, but cannot be cited as the production topology.

In return, browser disconnects and process restarts do not duplicate work;
stale processes cannot append or send; callers can retry safely; and every crash
boundary has one durable recovery answer.

## Enforcement
- `ctx-phz` implements the ADR-0005 pending-redaction and cleanup primitives
  against this private owner boundary.
- `ctx-ug7.18` implements the non-authorizing registry/bootstrap/workspace-
  authority/closed-envelope guard before event-schema-2 projection, fence acquisition,
  recovery, cleanup, or runtime mutation is enabled.
- `ctx-ug7.36` freezes the code-owned capability pair, shared host root,
  dedicated-writer trust, kernel-lock/lease semantics, run bootstrap, and
  authenticated mutable predecessor. `ctx-ug7.6.1` must test each boundary,
  including live effective-UID plus opened-file `st_uid` checks and rejection of
  legacy/non-canonical mutable bytes, before its workspace-authority capability
  can authorize foundation work.
- `ctx-7i1` owns the sole sanitized run/event/binding/artifact projection,
  baseline hash/validation state, steering generation, and operator-influence
  view; exact baseline tokens, capabilities, and input stay private.
- `ctx-ug7.6` is the coordinator integration and exact-candidate gate.
  `ctx-ug7.6.1` owns workspace authority, command records, publication, and
  fencing; `.6.2` owns asynchronous command admission and FIFO policy; `.6.3`
  owns replay, results, failure/cancel reconciliation, and finality; `.6.4`
  owns the shared Terminal resolver, occupancy, and pane/history lineage.
- `ctx-ug7.10` implements the post-occupancy exact Peek capability, interactive
  steering/closure protocol, frozen active grid, and no create/kill/rebind
  behavior while preserving coordinator-only automatic dispatch.
- `ctx-ug7.21` is the bounded same-pool fence decision/prototype; `.22`
  implements the chosen primitive and `.23` adversarially certifies it through
  the shared resolver. `.6.4` consumes only that certified capability, and
  `.10` waits for `.6.4`. Foundation certification `.5` is intentionally not
  blocked on production tmux proof and instead certifies the unavailable,
  send-nothing boundary.
- `ctx-ug7.14` implements new-element placement and the sole explicit full-board
  `arrange` seam; no runtime or recovery path may rearrange user layout.
- `ctx-rul` uses disposable sockets only as dogfood isolation and must state that
  production uses the shared cockpit pool.
- `ctx-ug7.16` must bump authority schema if retained Script Gate behavior adds
  process/evaluator authority. ADR-0008 retires inline Formation verification,
  so `ctx-ug7.17` adds no authority and requires no bump.
- `ctx-ug7.5` certifies the foundation reader guard and stabilization candidate;
  `ctx-ug7.15` owns host-root publication and active/retained capability
  inventory and certifies cross-version rejection, projection-only exclusion,
  shared-resolver topology, baseline loss, steering races, and the complete exact
  candidate.
- Tests include multi-process contention, stale fences, lost responses, command
  conflicts, bounded backpressure, subprocess kill/restart at every crash row,
  result-to-output replay, unsupported-schema readers, and docs parity.
