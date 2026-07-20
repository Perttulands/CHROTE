# Formations canonical run view design

**Status:** Approved architecture design for `ctx-7i1`

**Base:** `884deeec2c4d4ec2e220b7450dccdd6a10238ef5`

**Date:** 2026-07-20

## Purpose and authority

This design replaces overlapping run-state reduction in the API, Archon, Comms,
Agents, and Cockpit with one canonical, read-only projection. It is an
architecture checkpoint, not the later file-by-file implementation plan.

ADR-0001, ADR-0005, ADR-0006, ADR-0007, `FORMATIONS.md`, `DATA-MODEL.md`, and
`Perttus_vision_for_agent_orchestration/spec/contracts.md` remain authoritative.
This design narrows their implementation seam; it does not add or reinterpret an
event, command, Tool, Gate, target, execution, or redaction contract.

Success means:

- one source is selected for a run and read into one immutable projection input;
- one pure reducer produces the run view used by every consumer;
- a separate pure reducer produces terminal command receipts from the same
  canonical authority provider;
- public projections fail closed and contain only bounded, redaction-safe data;
- the dashboard has one data controller while visible UX remains unchanged and
  explicitly owner-designed later.

## Architecture

The dependency direction is:

```text
canonical authority provider
        |
        v
CanonicalRunAuthorityReader (physical and integrity boundary)
        |
        +-- immutable CanonicalRunReadInput --> ProjectRunView --> RunView
        |
        `-- immutable CanonicalCommandReadInput --> ProjectCommandReceipt
                                                        |
                                                        v
                             API / Archon / Comms / dashboard controller
```

### Stable authority-reader seam

`CanonicalRunAuthorityReader` is the one stable, read-only abstraction supplying
projection inputs. Its logical operations are `ReadRun`, `ListRuns`, and
`ReadCommand`. Implementations may stream while constructing an input, but each
successful operation returns one immutable snapshot. Projection never reopens a
path or consults mutable state after that boundary.

The reader owns no persistence, materialized cache, publication path, or
independent lifecycle. It is not another run store or source of truth.

The reader owns physical and integrity checks only:

- race-safe root and file opening, type/mode/link/resource limits, complete
  reads, and immutable snapshot construction;
- workspace/run bootstrap selection and exact encoding/hash linkage;
- graph, binding, ledger, command, policy, manifest, and registered private-state
  byte integrity;
- strict source classification and the rule that one run has exactly one source.

The reader does not reduce run status, interpret node or Gate behavior, sanitize
events, derive actions, hydrate artifacts, or decide recovery state. Those are
projector responsibilities.

For each run, selection is closed and precedence is absolute:

1. A guarded private schema-2 authority claim selects schema 2.
2. Once such a claim exists, missing, malformed, unsupported,
   capability-disabled, hash-invalid, or semantically invalid schema-2 input
   rejects the read. It never falls back to schema 1.
3. Schema-1 compatibility is eligible only when no canonical schema-2 claim
   exists, or when the caller explicitly uses the offline compatibility store.
4. Compatibility input remains schema 1. It cannot invent schema-2 command,
   binding, target, artifact, fence, or authority fields.

The production schema-2 provider is the foundation owned by `ctx-ug7.6.1`. That
work remains independent and later binds its writer/publication implementation
to this interface. This branch defines and certifies the reader/projector seam;
it does not stack on the moving writer branch.

The existing code-owned capability representation keeps
`SemanticProjection=false` until both the complete guarded foundation provider
and this exact projector implementation are registered. The guard alone, the
projector alone, matching schema numbers, or a persisted workspace value cannot
turn it on. No new capability id is introduced. Consequently this branch cannot
self-authorize production schema-2 reads before `ctx-ug7.6.1` lands.

### Pure projectors

`ProjectRunView(CanonicalRunReadInput) -> RunView` is the sole semantic reducer.
It performs schema-specific decoding, complete semantic validation, status and
finality reduction, result/linkage parity validation, recovery-state derivation,
sanitization, latest-artifact hydration, and action derivation. It has no
filesystem, clock, tmux, network, local-storage, or mutation dependency.

`ProjectCommandReceipt(CanonicalCommandReadInput) -> RunCommandReceipt` is a
separate pure projection because a rejected start has no run. It accepts only a
terminal `applied` or `rejected` record. `pending` is not a receipt and returns a
typed not-terminal result. This is a command audit projection over the same
provider, not a second source of run truth.

Raw engine or reconciler reads may remain private execution inputs. No public
adapter may call them directly or perform its own semantic scan.

Projection is atomic per run. Any schema, sequence, parity, hash, manifest,
source-role, linkage, result, binding, or artifact-authority failure rejects the
entire `RunView`; no partial view, events page, or list response is returned.

## Versioned public data contract

The canonical public shape is `formations.run-view.v1`:

```ts
RunView {
  schema: "formations.run-view.v1"
  runId: string
  source: { eventSchema: 1 | 2, authoritySchema?: JsonSafeInteger, compatibility: boolean }
  cursor: JsonSafeInteger
  status: "queued" | "running" | "blocked" | "waiting_human" |
          "canceling" | "failing" | "succeeded" | "failed" | "canceled"
  final: boolean
  recoveryState: "live" | "interrupted-finalization" | "pending-redaction"
  reconcileCondition: CoordinatorReconcileCondition | null
  identity: RunIdentity
  audit: RunAudit
  events: SafeRunEvent[]
  nodes: RunNodeView[]
  attempts: RunAttemptView[]
  gates: RunGateView[]
  outputs: RunOutputView[]
  artifacts: ArtifactProjection[]
  blocks: RunBlockView[]
  escalations: RunEscalationView[]
  sessions: RunSessionView[]
  actions: RunAction[]
}
```

The collection item fields are also versioned contract, not adapter-specific
bags:

```ts
RunNodeView {
  nodeId, kind, status, finalDisposition?, latestAttempt?, readiness,
  attempts[], outputs[], gates[], sessions[]
}
RunAttemptView {
  nodeId, attempt, status, startedSeq?, completedSeq?, inputRefs[], slots[],
  outputs[], gate?, disposition?
}
RunGateView {
  gateId, attempt, status, evaluatingSeq?, requestSeq?, verdictSeq?, verdict?,
  reason?, evidence[]
}
RunOutputView { nodeId, attempt, portId, outcomeSeq, payloadProjection }
SafeArtifactRef { artifactId, rootId, ref, mediaType, sizeBytes, sha256 }
ArtifactProjection =
  | { artifactId, availability: "available", name, artifact: SafeArtifactRef }
  | { artifactId, availability: "unavailable" | "redacted" | "expired",
      name, errorCode }
RunBlockView {
  seq, epoch, scope, nodeId?, gateId?, code?, reason, resumeAllowed,
  resumePolicy, nextEpoch?, openDispatches[]
}
RunEscalationView { seq, nodeId?, gateId?, severity, reason, source, trigger, blocks }
RunSessionView {
  bindingId, nodeId, attempt, slotId, dispatchId?, targetLeaseId?,
  sessionTargetId, bindingHealth, availabilityReason?, sessionLineageSha256,
  targetFingerprintSha256, baseline, attachment, occupancy, peekCapability,
  steering, operatorInfluenced
}
RunAction =
  | { kind: "cancel", expectedLastSeq }
  | { kind: "resume", blockedSeq, resumeMode }
  | { kind: "verdict", gateId, requestedSeq, allowedVerdicts: ["pass", "fail"] }
  | { kind: "peek", bindingId, nodeId, attempt, slotId, dispatchId,
      targetLeaseId, sessionTargetId }
```

Every unannotated status, kind, disposition, reason, source, policy, state, and
reference above is a closed value or shape already registered by the frozen
contracts. This design does not introduce an open string namespace.

`JsonSafeInteger` is an exact JSON integer in `0..9007199254740991`, narrowed to
the positive ranges required by each frozen contract. Reducers use exact integer
types and JSON encoders; they never decode through a floating-point number or
round a revision, sequence, fence, or admission identity. Contracted uint64
strings, such as a steering generation or history offset, remain canonical
unsigned decimal strings rather than being converted to JSON numbers.

Object member order is not semantic. Collection order is semantic and stable:

- `events`: ascending canonical ledger `seq`;
- `nodes`: frozen selected-root graph order, then `nodeId` only as a defensive
  tie-breaker;
- `attempts`: node order, then ascending `attempt`;
- `gates`: node order, then ascending `attempt`;
- `outputs`: node order, attempt, frozen port declaration order;
- `artifacts`: first registration sequence, then `artifactId`;
- `blocks` and `escalations`: ascending source sequence;
- `sessions`: node order, attempt, frozen slot order, then dispatch sequence;
- `actions`: `cancel`, `resume`, `verdict`, `peek`, then the corresponding graph,
  attempt, slot, and source-sequence order.

Schema-1 compatibility populates only fields its ledger can honestly establish;
missing schema-2-only data uses an absent optional field or an explicit closed
unavailable state, never synthesized authority.

### Identity and audit

`RunIdentity` contains the safe frozen run identity: board id/slug/revision,
run root, optional Mission and Bead ids, current epoch, and `redact`. It never
contains a private root, path, socket, target key, or authority locator.

`RunAudit` contains only registered safe audit facts: event and authority schema,
start sequence, consumed event count, admission command id/payload hash,
workspace admission sequence, admission and activation policy revision/hash,
latest consumed writer fence, graph snapshot hash, and safe binding-projection
hash where the selected schema provides them. Schema-1 cannot fabricate them.

`RunCommandReceipt` preserves exactly the frozen safe receipt union:

```ts
RunCommandReceipt =
  | { commandId, commandPayloadSha256, commandKind, outcomeWriterFence,
      state: "applied", runId, effectSeq, decisionAdmissionPolicyRef }
  | { commandId, commandPayloadSha256, commandKind, outcomeWriterFence,
      state: "rejected", rejectionCode, decisionAdmissionPolicyRef }
```

The command kind is only `start`, `resume`, `cancel`, or `verdict`; aliases were
normalized before hashing. The policy ref is non-null only for a start decision.
The outcome fence is immutable even if a later writer fence repaired publication.

### Run, node, Gate, output, and artifact views

`SafeRunEvent` is a discriminated union, not `map[string]any`. Its common
allowlist is the applicable subset of `seq`, `type`, `ts`, `runId`, `actor`,
graph ids, `epoch`, and `attempt`; `data` uses a closed decoder and a separate
public allowlist for that exact known event type. Artifact occurrences carry an
`artifactId` hydrated to the latest `ArtifactProjection`.

`RunNodeView` contains node id/kind, projected status/finality disposition,
latest attempt, readiness counts, and references to its attempts, outputs,
Gates, and sessions. `RunAttemptView` contains node id, attempt, status,
start/completion sequences, safe input identities, slot references, output
references, and any final disposition. Neither surface contains execution input
bytes.

`RunGateView` contains Gate id, attempt, status, evaluation/request/verdict
sequences, closed verdict, and bounded safe reason/evidence. It never treats
judge output as work or exposes a pending prompt assembled from runtime data.

`RunOutputView` is keyed by node id, attempt, and stable port id. It carries the
outcome sequence and the frozen `PayloadProjection`; redacted or inexact evidence
is never presented as routable work. The fixed `formation_needs_review` and
`invalid_formation_outputs` projections must byte-for-byte match the frozen
constants, including message, retryability, and hashes.

The artifacts collection embeds the frozen `ArtifactProjection` union exactly.
The available variant contains `{artifactId, availability:"available", name,
artifact:SafeArtifactRef}`; `SafeArtifactRef` is exactly `{artifactId, rootId,
ref, mediaType, sizeBytes, sha256}` and its nested `artifactId` must equal the
projection's top-level `artifactId`. The unavailable, redacted, and expired
variants contain exactly `{artifactId, availability, name, errorCode}` and no
readable ref. State derives only from the latest durable registration or
observation.

`RunBlockView` and `RunEscalationView` retain their source sequence, safe graph
identity, closed code/severity/source/trigger fields, bounded safe reason, and
the exact ledgered resume/block attributes. They do not create recovery or
mutation permission.

### Safe run-bound sessions

`RunSessionView` contains only registered safe values:

- binding, node, attempt, slot, dispatch, target-lease, and opaque
  `sessionTargetId` identity;
- binding health plus closed availability/arbitration reason;
- same-session-lineage and target-fingerprint hashes;
- baseline encoding, SHA-256, and closed validation state, never the exact
  baseline token;
- typed attachment and occupancy state;
- Peek capability state and issuance/revocation sequence, never a bearer;
- canonical steering generation/open state and `operatorInfluenced`.

It omits socket/server routes, `targetKey`, private paths, raw session lookup
identity, prompt/capture/pane bytes, exact history epoch/offset/grid token, input
bytes, and capability tokens. Live tmux cannot create or repair this view.

### Permitted actions

`actions` contains only ledger- and authority-derived preconditions:

- `cancel` with the expected last consumed sequence for a cancellable non-final
  run;
- `resume` with the exact blocked sequence and canonical resume mode;
- `verdict` with Gate id, outstanding request sequence, and the closed
  `pass`/`fail` choice set;
- `peek` with exact safe run/binding/dispatch/target-lease/opaque-target identity
  only while current occupancy and capability state permit issuance.

The projection does not carry a command id, issue a Peek bearer, or perform an
effect. Command submission still requires a new client-stable command id and
coordinator revalidation; Peek issuance and input still require the coordinator
and target boundary. No status, recovery state, local-storage value, persona
label, or live tmux observation creates an action.

## Cursor and event transport

`cursor` is the highest canonical ledger sequence consumed, not the last public
event emitted. A registered projection-only event may be omitted from `events`
but still advances the cursor. Event responses always return `{runId, cursor,
events}`, including `events=[]` after filtering.

Replay-only SSE follows the same reduction. Each emitted safe event uses its
canonical sequence as the SSE id. At the end of every replay response, the
adapter emits a sanitized transport-only `cursor` control event whose SSE id and
data cursor both equal the consumed sequence. This advances `Last-Event-ID`
across an omitted extension-only tail, including when no safe events remain.
The control event is not a ledger event and grants no authority. The response
then closes; this design adds no live-follow loop.

All historical event artifact occurrences are hydrated after reduction through
the latest authorization. A formerly readable `ArtifactProjection` never
survives in an older event after the latest state becomes unavailable,
redacted, or expired.

## Single recovery vocabulary

Every backend and frontend consumes the exact `recoveryState` field and values:

1. `pending-redaction` iff the canonical private input contains any unresolved
   run-owned redaction obligation. It wins over every other state.
2. Otherwise `interrupted-finalization` iff replay shows at least one frozen
   incomplete deterministic transition:
   - a deciding `slot_result` without `formation_result`;
   - `formation_result` or `tool_result` without matching `node_output`;
   - `node_output` with incomplete declared delivery, routing, or finality;
   - `run_cancel_requested` without matching terminal cancellation/failure; or
   - `run_failure_reconciliation_started` without matching `run_failed`.
3. Otherwise `live`.

`live` means only that replay evidences no recovery-only gap. It is not an alias
for run status `running`. Status and recovery state remain independent fields.

The two non-live states expose only
`{kind:"coordinator-reconcile", state:<recoveryState>}` in
`reconcileCondition`. They add no action. Resume, cancel, verdict, and Peek remain
exact projections of their existing authority. No client resend, replay, tmux,
cleanup, redaction input, or coordinator authority is implied.

## Sanitization and artifact opening

Schema-2 known events use closed payload decoders and per-event public
allowlists. An unknown authority-bearing type or key rejects the whole view.
Registered projection-only extensions are redaction-classified, omitted, and
cannot alter status, finality, actions, bindings, or artifacts. They also cannot
change the cursor rule: their canonical sequence is consumed even though their
public event is omitted.

Every public string is a closed enum/id/hash or passes its field-specific bound
and redaction-safe normalization. Public output never includes raw adapter error
text, prompt/capture/pane bytes, a pending raw locator, bearer capability,
socket/private route, or exact private baseline token. Human requests resolve
only known immutable fixed-system template ids; unknown ids reject.

Artifact open uses optimistic reauthorization:

1. Project the run at cursor C1 and require the exact latest
   `ArtifactProjection` P1 to be the `available` variant with
   `artifact:SafeArtifactRef` D and equal top-level/nested `artifactId`.
2. Open D once root-relative with no-follow semantics; validate regular-file
   identity, media type, `sizeBytes`, and SHA-256; read the bounded bytes once.
3. Re-project from canonical authority.
4. Require the latest `ArtifactProjection` P2 to be exactly field-equal to P1,
   including an identical D across `artifactId`, `rootId`, `ref`, `mediaType`,
   `sizeBytes`, and `sha256`. The successful second projection is the
   authorization linearization point.
5. Render only the already verified buffer, or the same verified handle when
   buffering is inapplicable. Never reopen the path.

Any failure or projection/ref change discards the bytes and fails the open.
The reconciler may later append an observation, but the read endpoint never
mutates projection truth.

## Consumer migration

Migration precedes deletion. API list/detail/status, mutation receipts,
escalations, events, replay-only SSE, and artifact open consume `RunView` or
`RunCommandReceipt`. Archon status/list/logs/follow/ask and the existing Comms
Formations-run projection consume the same abstractions. Existing response fields
may remain temporarily only as adapters derived from the canonical projection.
Duplicate consumer scans are removed only after fixture parity.

### DATA CONTRACT — engineering review

One dashboard controller owns:

- initial restore and polling from `RunView`;
- cursor replacement/merge, rejecting a same-sequence mismatch;
- the local-storage run-id hint, which selects a fetch candidate but never
  supplies status, authority, session identity, or an action;
- normalized nodes, attempts, Gates, outputs, artifacts, sessions, and actions;
- command receipt handling and fresh-view reconciliation after mutations;
- stale/error state.

The controller atomically replaces its last good view only with a fully valid
new view. On read failure it may retain the last good view as explicitly stale,
but disables mutation/Peek submission until a fresh view re-establishes current
actions. Agents and Cockpit retain presentation state only: selection, focus,
expanded panels, tile geometry, and similar local choices.

Parity tests cover API/Archon/Comms/controller equality; schema-1 supported
ledgers; queued, running, blocked, resumed, waiting, canceling, failing, and final
runs; multiple attempts; failed Gates; exact/redacted outputs; artifact
revocation; session/baseline/Peek states; omitted extension tails and empty event
pages; JSON-safe maximum identities; all recovery predicates and precedence;
and whole-view rejection for every integrity class.

### UX-OWNER: Perttu

The following are explicitly outside this engineering slice and route to Perttu:

- visible recovery-state labels and explanatory copy;
- hierarchy of run, node, attempt, Gate, artifact, and session information;
- action placement;
- artifact-reader presentation;
- interactive Peek entry and presentation;
- multi-session board visualization.

No owner-UX item is designed or implemented here. Existing presentation remains
in place except that it consumes the canonical data controller.

## SDD checkpoints and handoff

The reviewed implementation plan will create executable child Beads. It will
retain four RED-first milestones and use fresh implementer/reviewer cycles:

1. Projector kernel and parity.
2. Authority security, sanitization, and artifact authorization/open.
3. Consumer adapters, split into separately reviewed tasks:
   - API status/list/detail, mutation receipts, and escalations;
   - API events, replay-only SSE, and artifacts;
   - Archon;
   - Comms.
4. Dashboard controller data contract only, with visual UX fenced to Perttu.

Every implementation task starts with a tests-only RED commit, receives an
independent RED review, proceeds to GREEN, receives task review, and records both
implementation and review evidence in the SDD progress ledger and its child
Bead. A broad exact-branch review follows all tasks. This document intentionally
does not assign files, create child Beads, or serve as that implementation plan.

## Non-goals and safety boundary

This design adds no asynchronous coordinator, authority writer implementation,
live SSE follow loop, UI redesign, mission room, terminal iframe/session
creation, Tool execution, or tmux behavior. It does not authorize client resend,
replay, cleanup, redaction input, or lifecycle mutation.

The implementation work must not infer run or target authority from current
board/persona files, local storage, live tmux, raw ledgers, historical artifact
refs, or compatibility fields. The certified Tool worktree/branch remains
untouched. No Context Citadel project-local architecture write is required.

This checkpoint performs no push, PR, merge, deployment, service action, live
runtime action, or tmux action.
