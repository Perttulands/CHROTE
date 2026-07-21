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
        +-- immutable CanonicalRunReadInput --> ProjectCanonicalRun
        |                                      (one semantic reducer)
        |                                               |
        |                       +-----------------------+------------------+
        |                       |                                          |
        |                       v                                          v
        |              ProjectRunView                            ProjectRunEventPage
        |                       |                                          |
        |                       v                                          v
        |                    RunView                                RunEventPage
        |
        `-- immutable CanonicalCommandReadInput --> ProjectCommandReceipt
                                                        |
                                                        v
                             API / Archon / Comms / dashboard controller
```

### Stable authority-reader seam

`CanonicalRunAuthorityReader` is the one stable, read-only abstraction supplying
projection inputs. Its logical operations are `ReadRun`, `ListRunIdentities`,
and `ReadCommand`. `ListRunIdentities` returns one bounded identity page, never
complete run inputs; the list adapter then calls `ReadRun` and projects one
selected identity at a time. `ReadCommand` takes the complete submitted-command
identity, not merely a lookup string. Implementations may stream while
constructing an input, but each successful operation returns one immutable
snapshot. Projection never reopens a path or consults mutable state after that
boundary.

The reader owns no persistence, materialized cache, publication path, or
independent lifecycle. It is not another run store or source of truth.

The reader owns physical and integrity checks only:

- race-safe root and file opening, type/mode/link/resource limits, complete
  reads, and immutable snapshot construction;
- workspace/run bootstrap selection and exact encoding/hash linkage;
- graph, binding, ledger, command, policy, manifest, and registered private-state
  byte integrity;
- strict source classification and the rule that one run has exactly one source.

The total raw ledger document is bounded for both sources before an immutable
`CanonicalRunReadInput` can contain it and before `CanonicalRunProjection` can
materialize the sanitized stream. The code-owned implementation ceiling is
`RunLedgerReadMaximumBytes = 64 << 20` (64 MiB). It is not a public transfer
contract. For a descriptor/no-follow ledger read, a known stat size above the
ceiling returns `ErrRunProjectionResourceLimit` before allocation. The reader
then streams at most `RunLedgerReadMaximumBytes + 1` bytes from that same opened
handle so an unknown size or post-stat growth is also detected. Observing the
extra byte discards the complete candidate document and returns the same error;
no partial input, projection, view, or page is available. The separate complete
`RunEventPage` transfer ceiling remains 1 MiB.

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

`ProjectCanonicalRun(CanonicalRunReadInput) -> CanonicalRunProjection` is the
sole semantic reducer. It performs schema-specific decoding, complete semantic
validation, status and finality reduction, result/linkage parity validation,
recovery-state derivation, event sanitization, latest-artifact hydration, and
action derivation. It replays the complete immutable canonical input. Its
`CanonicalRunProjection` result is private projector state containing the
graph/run-limit-bounded structural view plus the sanitized canonical event
stream; it is not a public contract, cache, or second store. The reducer has no
filesystem, clock, tmux, network, local-storage, or mutation dependency.

`ProjectRunView(CanonicalRunProjection) -> RunView` selects only the current
structural state and never embeds event history.
`ProjectRunEventPage(CanonicalRunProjection, since, limit) -> RunEventPage`
selects one bounded page from the same private sanitized stream. Both are pure
selectors over the one semantic reduction. Adapters may invoke the reducer per
immutable read, but semantic validation, reduction, sanitization, and artifact
hydration must not be reimplemented in either selector.

`ProjectCommandReceipt(CanonicalCommandReadInput) -> RunCommandReceipt` is a
separate pure projection because a rejected start has no run. The normalized
submission boundary constructs a private `SubmittedCommandIdentity` containing
the client-stable `commandId`, normalized `commandKind`, and SHA-256 of the
canonical command payload. The reader is called with that identity and the
durable terminal record must exact-match all three members before projection.
It accepts only a terminal `applied` or `rejected` record. `pending` is not a
receipt and returns a typed not-terminal result. A rejected start never acquires
a synthetic run id. This is a command audit projection over the same provider,
not a second source of run truth.

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
  generation: string
  source: { eventSchema: 1 | 2, authoritySchema?: JsonSafeInteger, compatibility: boolean }
  cursor: JsonSafeInteger
  status: "queued" | "running" | "blocked" | "waiting_human" |
          "canceling" | "failing" | "succeeded" | "failed" | "canceled"
  final: boolean
  recoveryState: "live" | "interrupted-finalization" | "pending-redaction"
  reconcileCondition: CoordinatorReconcileCondition | null
  identity: RunIdentity
  audit: RunAudit
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

Run listing is a separate bounded public contract:

```ts
RunListPage {
  schema: "formations.run-list.v1"
  runs: RunView[]
  cursor: string
  hasMore: boolean
}
```

Runs are ordered by ascending canonical `runId` byte order. `after` is the
exclusive last-scanned run id; absence means before the first id. The default
and maximum `limit` are both 50 scanned candidate identities. Filtering occurs
after candidate selection, so a filtered candidate consumes one slot and moves
the cursor. A physical enumerator may scan all directory entries but retains
only the lexicographically next 51 safe run ids in a bounded selection
structure. It never retains all ids or all immutable run inputs. Every selected
id still passes the existing unique-run resolver, so a selected id present in
more than one run directory fails closed.

`cursor` equals the last identity actually scanned, or echoes `after` (the empty
string when absent) when none was scanned. `hasMore` is true iff at least one
unconsumed identity exists after that cursor, including an identity deferred by
the encoded-byte cap. An empty filtered page can therefore advance its cursor
and still report more work.

The exact JSON encoding of the complete `RunListPage` is capped at 4 MiB
(`4 << 20` bytes). Before a candidate would overflow the page, selection stops
without consuming that identity, and `hasMore` remains true. A single
`RunView` that cannot fit in an otherwise empty page returns the typed
projection resource-limit error. No adapter silently truncates or rebuilds all
pages in memory.

Candidate projection is fail-loud before filtering or page publication. If any
selected candidate has a guarded schema-2 claim but the post-writer,
pre-activation capability is non-authorizing, the whole `RunListPage` fails
with HTTP 503 and public code `RUNTIME_AUTHORITY_NON_AUTHORIZING`. The adapter
never skips or filters that candidate, falls back to schema 1, or returns a
partial page containing candidates projected before the failure. This
deliberately exposes authority misconfiguration during a mixed-window read.

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

`RunEventPage.events` is ordered by ascending canonical ledger `seq`.
`RunListPage.runs` is ordered by ascending canonical `runId`; that is the only
list order used by the store, API, Archon, Comms, and dashboard.

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

`RunCommandReceipt` preserves exactly the frozen safe receipt union under the
literal upstream heading `Workspace Runtime Authority And Commands`:

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

`SafeRunEvent` is a discriminated union used only in `RunEventPage` and replay
transport, not in `RunView`. Its common
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

`SafeArtifactRef.ref` is the contract-sanctioned root-relative path. It is
resolved only beneath its named `rootId` through the descriptor-relative
no-follow boundary. It is not a generic public member named `path`, and it is
the sole path-bearing exception needed for authorized artifact reads; it does
not weaken the global prohibition on a public JSON member named `path`.

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

`RunView.cursor` is the latest canonical sequence consumed by the complete
structural view. Detail polling never embeds event history in `RunView`.

The separate public page is:

```ts
RunEventPage {
  schema: "formations.run-events.v1"
  runId: string
  generation: string
  source: { eventSchema: 1 | 2, authoritySchema?: JsonSafeInteger, compatibility: boolean }
  cursor: JsonSafeInteger
  hasMore: boolean
  events: SafeRunEvent[]
}
```

`generation` is the same lowercase 64-hex immutable run-incarnation SHA-256 as
the matching `RunView.generation`, and `source` is byte-for-byte the same safe
source classification as `RunView.source`. Both are mandatory even for an empty
page so an events-only consumer cannot combine a replaced run incarnation or
schema-1 compatibility evidence with another structural view.

The request is `since` plus `limit`. `since` is the last-consumed cursor in
`0..9007199254740991`, so eligible canonical events satisfy `seq > since`;
`limit` defaults to `200` and is rejected unless it is in `1..200`. The limit
bounds canonical events scanned, not safe events emitted. A
registered projection-only event consumes one scan slot, is omitted, and
advances the page cursor. `cursor` is the highest canonical sequence consumed by
this page, or the requested `since` if nothing was consumed. `hasMore` is true
when the immutable snapshot has a higher canonical sequence than `cursor`.

The JSON encoding of the complete `RunEventPage`, including its mandatory
generation and source, is capped at `1 MiB` (`1 << 20` bytes).
Before consuming an event that would exceed the cap, the projector ends a
non-empty page without advancing over that event. If one sanitized event cannot
fit in an otherwise empty page, projection fails closed with a typed projection
resource-limit error. This byte cap applies to the page projection; the API
success envelope adds only its fixed bounded wrapper.

Before returning `CanonicalRunProjection`, the sole reducer validates every
emitted safe event in a singleton complete `RunEventPage` with its real run id,
generation, source, and cursor. Therefore an individually oversized safe event rejects the immutable
projection before an adapter writes an events or SSE response; adapters do not
need a second full-replay preflight.

The events endpoint returns exactly one page. Replay-only SSE reads one
immutable snapshot, emits the same sanitized events in bounded internal pages,
and never accumulates the complete replay. Each emitted safe event uses its
canonical sequence as the SSE id. At the end of every replay response, the
adapter emits a sanitized transport-only `cursor` control event whose SSE id and
data cursor both equal the final consumed page cursor. If no canonical sequence
is consumed, including when the requested `since` is greater than the snapshot's
latest sequence, that cursor remains the requested `since`; SSE never regresses
the resume token to a lower snapshot high-water. This also advances
`Last-Event-ID` across an omitted extension-only tail when no safe events remain.
The control event is not a ledger event and grants no authority.
The response then closes; this design adds no live-follow loop.

All historical event artifact occurrences are hydrated after reduction through
the latest authorization. A formerly readable `ArtifactProjection` never
survives in an older event after the latest state becomes unavailable,
redacted, or expired.

## Single recovery vocabulary

Every backend and frontend consumes the exact `recoveryState` field and values:

1. Schema 1 always projects `recoveryState="live"` and
   `reconcileCondition=null`. Its frozen 21-row vocabulary has no
   `formation_result`, schema-2 private redaction obligation, or schema-2
   deterministic reconciliation authority, so absence of those records is
   never a recovery gap.
2. Schema 2 alone evaluates recovery predicates. It projects
   `pending-redaction` iff the canonical private input contains any unresolved
   run-owned redaction obligation. That state wins over every other state.
3. Otherwise schema 2 projects `interrupted-finalization` iff replay shows at
   least one frozen incomplete deterministic transition:
   - a deciding `slot_result` without `formation_result`;
   - `formation_result` or `tool_result` without matching `node_output`;
   - `node_output` with incomplete declared delivery, routing, or finality;
   - `run_cancel_requested` without matching terminal cancellation/failure; or
   - `run_failure_reconciliation_started` without matching `run_failed`.
4. Otherwise schema 2 projects `live`.

For the first schema-2 predicate, a deciding `slot_result` is exactly the first
result in the frozen current Formation schedule whose status is non-`ok`, or the
`ok` result that completes that schedule. After either case,
`formation_result` is deterministically required. An intermediate `ok` result
with a later scheduled turn is not deciding and its absence of
`formation_result` does not create a recovery condition.

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

## Closed implementation contract appendix

This appendix closes the local projection contract that was intentionally only
sketched above. Implementers do not infer fields from fixtures. The stable
upstream anchors are the literal headings `Workspace Runtime Authority And
Commands`, `Run Ledger Envelope`, `Event Payload Schemas`, `Projection Mapping`,
and `API Surface` in
`Perttus_vision_for_agent_orchestration/spec/contracts.md`; `FORMATIONS.md` and
`DATA-MODEL.md` supply the existing identifier grammars and graph ordering.
Those heading anchors, not mutable line numbers, define every referenced frozen
shape. The tables below define the projection-only overlay. When an anchor and a
table disagree, projection fails and the design must be amended; code does not
average the two.

### Exact public Go, JSON, and TypeScript shapes

Go exported field names use the first column, JSON tags use the second, and
TypeScript names are exactly the JSON names. `uint64/jsi` means Go `uint64`, JSON
number, and TypeScript `JsonSafeInteger`, with range checked before encoding.
`uint64/decimal` means Go `uint64` or validated decimal value, JSON string, and
TypeScript canonical unsigned-decimal string. All slices are non-null JSON
arrays and all maps have stable sorted keys. A member shown without an explicit
type is a required `string`; `?` alone makes it optional. For each registry row,
Go defines `SafeSchema<N><PascalEventName>Event` with the common envelope and
`SafeSchema<N><PascalEventName>Data` with exactly the listed JSON tags;
TypeScript defines the corresponding
`{type:"literal",data:SafeSchemaNPascalEventNameData}` arm. For the 17 shared
type literals both schema-specific arms exist, and the enclosing
`RunEventPage.source.eventSchema` selects exactly one decoder/shape. The four
schema-1-only arms have no schema-2 counterpart. This naming rule is part of the
contract, not license for a generic data map.

| Go type | Closed members (`Go: json/TS: type`) |
| --- | --- |
| `CanonicalRunSourceProjection` | `EventSchema:eventSchema: 1\|2`; `AuthoritySchema:authoritySchema?: uint64/jsi`; `Compatibility:compatibility: boolean` |
| `RunIdentity` | `BoardID:boardId:string`; `BoardSlug:boardSlug:string`; `BoardRev:boardRev:uint64/jsi`; `RunRoot:runRoot:{kind:"mission"\|"formation",nodeId:string}`; `MissionID:missionId?:string`; `BeadID:beadId?:string`; `Epoch:epoch:uint64/jsi`; `Redact:redact:boolean` |
| `RunAudit` | `EventSchema:eventSchema:1\|2`; optional schema-2-only `AuthoritySchema:authoritySchema`, `AdmissionCommandID:admissionCommandId`, `CommandPayloadSHA256:commandPayloadSha256`, `WorkspaceAdmissionSeq:workspaceAdmissionSeq`, `AdmissionPolicyRev:admissionPolicyRev`, `AdmissionPolicySHA256:admissionPolicySha256`, `ActivationPolicyRev:activationPolicyRev`, `ActivationPolicySHA256:activationPolicySha256`, `LatestWriterFence:latestWriterFence`, `GraphSnapshotSHA256:graphSnapshotSha256`, `BindingProjectionSHA256:bindingProjectionSha256`; required `StartSeq:startSeq:uint64/jsi`, `ConsumedEventCount:consumedEventCount:uint64/jsi` |
| `RunReadiness` | `NeededInputs:neededInputs:uint64/jsi`; `ReadyInputs:readyInputs:uint64/jsi`; `TotalInputs:totalInputs:uint64/jsi`; `WaitingFor:waitingFor:string[]` |
| `RunNodeView` | `NodeID:nodeId`; `Kind:kind:"mission"\|"formation"\|"tool"\|"gate"`; `Status:status:"not_run"\|"waiting"\|"running"\|"waiting_human"\|"done"\|"needs-review"\|"blocked"\|"failed"\|"canceled"\|"abandoned"`; `FinalDisposition:finalDisposition?:"done"\|"failed"\|"canceled"\|"abandoned"\|"not_run"`; `LatestAttempt:latestAttempt?:uint64/jsi`; `Readiness:readiness:RunReadiness`; `Attempts:attempts:RunAttemptRef[]`; `Outputs:outputs:RunOutputRef[]`; `Gates:gates:RunGateRef[]`; `Sessions:sessions:RunSessionRef[]` |
| `RunAttemptView` | `NodeID:nodeId`; `Attempt:attempt:uint64/jsi`; `Status:status:"waiting"\|"running"\|"waiting_human"\|"done"\|"needs-review"\|"blocked"\|"failed"\|"canceled"\|"abandoned"`; optional `StartedSeq:startedSeq`, `CompletedSeq:completedSeq`; `InputRefs:inputRefs:SafeInputIdentity[]`; `Slots:slots:RunSessionRef[]`; `Outputs:outputs:RunOutputRef[]`; `Gate:gate?:RunGateRef`; `Disposition:disposition?:"done"\|"failed"\|"canceled"\|"abandoned"` |
| `RunGateView` | `GateID:gateId`; `Attempt:attempt:uint64/jsi`; `Status:status:"idle"\|"evaluating"\|"waiting_human"\|"passed"\|"failed"\|"blocked"\|"abandoned"`; optional `EvaluatingSeq:evaluatingSeq`, `RequestSeq:requestSeq`, `VerdictSeq:verdictSeq`; `Verdict:verdict?:"pass"\|"fail"`; `Reason:reason?:string`; `Evidence:evidence:SafeGateEvidence[]` |
| `RunOutputView` | `NodeID:nodeId`; `Attempt:attempt:uint64/jsi`; `PortID:portId`; `OutcomeSeq:outcomeSeq:uint64/jsi`; `PayloadProjection:payloadProjection:PayloadProjection` exactly as anchored under `Event Payload Schemas` |
| `RunBlockView` | `Seq:seq:uint64/jsi`; `Epoch:epoch:uint64/jsi`; `Scope:scope:"run"\|"node"\|"gate"`; optional `NodeID:nodeId`, `GateID:gateId`, `Code:code`; `Reason:reason`; `ResumeAllowed:resumeAllowed`; `ResumePolicy:resumePolicy:"retry_failed_producer"\|"reattach_only"\|"new_run_required"\|"explicit"\|"authoring"`; optional `NextEpoch:nextEpoch`; `OpenDispatches:openDispatches:SafeOpenDispatch[]` |
| `RunEscalationView` | `Seq:seq:uint64/jsi`; optional `NodeID:nodeId`, `GateID:gateId`; `Severity:severity:"info"\|"needs-attention"\|"stop"`; `Reason:reason`; `Source:source:"system"\|"agent"\|"human"`; `Trigger:trigger:string`; `Blocks:blocks:boolean` |
| `RunSessionView` | `BindingID:bindingId`; `NodeID:nodeId`; `Attempt:attempt:uint64/jsi`; `SlotID:slotId`; optional `DispatchID:dispatchId`, `TargetLeaseID:targetLeaseId`; `SessionTargetID:sessionTargetId`; `BindingHealth:bindingHealth:"runnable"\|"unavailable"\|"stale"`; optional `AvailabilityReason:availabilityReason`; `SessionLineageSHA256:sessionLineageSha256`; `TargetFingerprintSHA256:targetFingerprintSha256`; `Baseline:baseline:{encoding:string,sha256:string,state:"valid"\|"unavailable"\|"stale"}`; `Attachment:attachment:{state:"accounted"\|"foreign"\|"audit_lost"}`; `Occupancy:occupancy:{state:"active"\|"released"\|"held"\|"quarantined"}`; `PeekCapability:peekCapability:{state:"none"\|"issued"\|"input_open"\|"revoked",issuedSeq:uint64/jsi,generation:uint64/decimal}`; `Steering:steering:{state:"closed"\|"open",generation:uint64/decimal,startedSeq?:uint64/jsi}`; `OperatorInfluenced:operatorInfluenced:boolean` |
| `CoordinatorReconcileCondition` | `Kind:kind:"coordinator-reconcile"`; `State:state:"interrupted-finalization"\|"pending-redaction"` |
| `RunAction` | exact union: `cancel {kind,expectedLastSeq}`; `resume {kind,blockedSeq,resumeMode:"reattach"\|"retry-failed-producer"}`; `verdict {kind,gateId,requestedSeq,allowedVerdicts:["pass","fail"]}`; `peek {kind,bindingId,nodeId,attempt,slotId,dispatchId,targetLeaseId,sessionTargetId}` |
| `RunView` | exactly the members in the `formations.run-view.v1` block above, with `Generation:generation:string` and `ReconcileCondition:reconcileCondition:CoordinatorReconcileCondition|null`; no `events` member |
| `RunListPage` | `Schema:schema:"formations.run-list.v1"`; `Runs:runs:RunView[]`; `Cursor:cursor:string`; `HasMore:hasMore:boolean` |
| `RunEventPage` | `Schema:schema:"formations.run-events.v1"`; `RunID:runId`; `Generation:generation:string`; `Source:source:CanonicalRunSourceProjection`; `Cursor:cursor:uint64/jsi`; `HasMore:hasMore:boolean`; `Events:events:SafeRunEvent[]` |
| `RunCommandReceipt` | exact two-arm union anchored under `Workspace Runtime Authority And Commands`; schema-2 fences remain `uint64/decimal` when the frozen authority record declares uint64 beyond JSON-safe range; event sequences remain `uint64/jsi` |

`RunAttemptRef` is exactly `{nodeId,attempt}`, `RunOutputRef` is exactly
`{nodeId,attempt,portId}`, `RunGateRef` is exactly `{gateId,attempt}`, and
`RunSessionRef` is exactly `{bindingId,nodeId,attempt,slotId}`. `SafeInputIdentity`
contains only graph/source ids, attempt/output sequence, and a typed
`PayloadProjection`; it never contains a ledger/file ref. `SafeGateEvidence`
is the frozen typed evidence union with every artifact occurrence replaced by
its latest `ArtifactProjection`.

`SafeSchema1OpenDispatch` is exactly
`{dispatchId,nodeId,slotId,dispatchSeq?}`. The three ids are required and use
their stable identifier grammars. `dispatchSeq`, when present, is a JSON-safe
`uint64` and may equal `0`, matching the current dispatch-error producer.
Unknown nested keys reject the whole projection. Schema-1 arrays preserve
source order because that order is semantic; duplicate `dispatchId` rejects the
whole projection even when the other members agree.

`SafeSchema2OpenDispatch` is separately and exactly
`{dispatchId,targetLeaseId,nodeId,attempt,slotId,agentId,bindingId,
sessionTargetId,targetFingerprint,dispatchSeq,peekCapabilityState,
latestCapabilityGeneration,latestCapabilityIssuedSeq,
latestSteeringGeneration,openSteeringStartedSeq?,peekCapabilityRevokedSeq?,
interruptState,interruptRequestedSeq?,interruptOutcomeSeq?}` with the enums,
requiredness, unsigned-decimal generations, JSON-safe sequences, ordering, and
cross-field invariants anchored at `openDispatches` in the frozen schema-2
contract. It omits target keys, routes, tokens, and proof bytes.

`SafeOpenDispatch` is the source-selected union
`SafeSchema1OpenDispatch | SafeSchema2OpenDispatch`. `RunView.source` and the
enclosing safe-event source select exactly one arm for every occurrence in
`RunBlockView`, `run_blocked`, or `run_resumed`; a schema-1 item is never padded,
translated, or coerced into schema-2 capability/lease fields. Go uses distinct
`SafeSchema1OpenDispatch` and `SafeSchema2OpenDispatch` structs, JSON preserves
the selected exact shape, and TypeScript uses the same source-discriminated
union.

For schema 1, `run_blocked.openDispatches` and
`run_resumed.openDispatches` use only `SafeSchema1OpenDispatch[]`. When resume
carries the prior blocked array, its sanitized public JSON bytes must equal the
blocked array bytes exactly, including source order and whether `dispatchSeq`
was absent or present as `0`. Projection never sorts or fills that legacy array.
Schema-2 occurrences use only `SafeSchema2OpenDispatch[]` and retain the frozen
schema-2 ordering rules.

Every public identifier, hash, time, enum, bounded reason/message, payload,
artifact, and evidence member uses the grammar or byte bound of its named
stable anchor. Projection adds no permissive `any`, `interface{}`, raw map, or
`Record<string, unknown>` escape hatch. The globally forbidden public JSON
member names are `path`, `boardPath`, `socket`, `cwd`, `sessionStem`,
`sessionRef`, `targetKey`, `token`, `prompt`, `promptRef`, `capture`, `pane`,
`paneHistoryBaseline`, `targetReadyProof`, `dispatchInputBarrier`,
`clientAttachmentAuditProof`, `leaseRoot`, and `redactionObligationId`.

`RunView.generation` and `RunEventPage.generation` are the same lowercase
64-hex SHA-256 over the canonical immutable run-incarnation identity, excluding
the advancing ledger tail. For schema 2 the
input is the canonical tuple `(runId,runAuthorityId,graphSnapshotSha256,
privateBindingsSha256,admissionCommandId)`; for schema 1 it is `(runId,
sha256(first run_started record),sha256(board snapshot),sha256(bindings
snapshot))`. The projector computes it from already verified input bytes. A
different generation under the same run id is replacement/tamper, never a
continuation.

### Closed safe-event registries

The common public event envelope is exactly `{ts,runId,seq,type,actor,boardId?,
boardRev?,missionId?,beadId?,nodeId?,slotId?,gateId?,edgeId?,epoch?,attempt?,
data}`. `seq`, `boardRev`, `epoch`, and `attempt` are JSON-safe integers; absent
optional identity is omitted. No schema, authority, writer fence, private route,
or unknown top-level member is copied. The public `SafeRunEvent` discriminated
union has exactly 41 event-type literals: all 37 schema-2 types below plus the
four schema-1 compatibility-only types below. The 17 shared literals have the
two source-selected typed payload variants defined above.

For schema 2, requiredness and nested closed types exact-match the `Run Ledger
Envelope` event table. This public sanitizer table is the complete key allowlist;
an upstream required key not listed here is deliberately private and omitted.
An unknown key rejects the whole projection rather than being silently dropped.

| Schema-2 type | Exact public `data` keys |
| --- | --- |
| `run_started` | `workspaceAuthorityId,workspaceAdmissionSeq,admissionPolicyRev,admissionPolicySha256,admissionCommandId,commandPayloadSha256,boardSlug,sourceBoardSchema,snapshotSchema,runAuthorityId,graphSnapshotSha256,privateBindingsSha256,bindingProjectionSha256,runRoot,rootInputProjection,limits` |
| `run_activated` | `workspaceAdmissionSeq,admissionPolicyRev,admissionPolicySha256,reason` |
| `run_resumed` | `commandId,commandPayloadSha256,resumedFromSeq,resumedBy,resumeMode,reason,openDispatches:SafeSchema2OpenDispatch[],retryTargets` |
| `node_waiting` | `nodeId,neededInputs,readyInputs,totalInputs,waitingFor` |
| `node_input_ignored` | `nodeId,toPortId,inputRef,reason,relatedAttempt` |
| `node_started` | `nodeId,nodeKind,attempt,reason,inputRefs,contextEncoding,judgeContextSha256,priorResultSeqs,triggerFeedbackId,priorGateSeq` with the anchored ordinary/judge exclusivity |
| `slot_binding_observed` | `bindingId,slotId,sessionTargetId,health,reason,observedAt,relatedSeq` |
| `slot_dispatch` | `dispatchId,targetLeaseId,turnKey,turnPhase,turnInputs,nodeId,attempt,slotId,agentId,harness,bindingId,sessionTargetId,targetFingerprint,dispatchInputBarrierEncoding,dispatchInputBarrierSha256,targetReadyProofEncoding,targetReadyProofSha256,paneHistoryBaselineEncoding,paneHistoryBaselineSha256,steeringGeneration,promptSha256,nativeAck,recordedBeforeSend` |
| `slot_peek_capability_issued` | `dispatchId,targetLeaseId,bindingId,sessionTargetId,targetFingerprint,capabilityGeneration,priorIssuedSeq,issuedAt` |
| `slot_steering_started` | `dispatchId,targetLeaseId,bindingId,sessionTargetId,targetFingerprint,capabilityIssuedSeq,capabilityGeneration,steeringGeneration,actor,startedAt,recordedBeforeInput` |
| `slot_steering_ended` | `startedSeq,dispatchId,targetLeaseId,targetFingerprint,steeringGeneration,reason,endedAt` |
| `slot_peek_capability_revoked` | `dispatchId,targetLeaseId,bindingId,sessionTargetId,targetFingerprint,capabilityGeneration,capabilityIssuedSeq,steeringGeneration,reason,revokedAt,inputClosed` |
| `slot_reconciliation_interrupt` | `dispatchId,targetLeaseId,bindingId,sessionTargetId,targetFingerprint,authorityKind,authoritySeq,interruptEncoding,interruptSha256,recordedBeforeSend` |
| `slot_reconciliation_interrupt_outcome` | `requestedSeq,dispatchId,targetLeaseId,targetFingerprint,outcome,observedAt` |
| `slot_result` | `dispatchId,targetLeaseId,turnKey,turnPhase,nodeId,attempt,slotId,agentId,bindingId,sessionTargetId,targetFingerprint,paneHistoryBaselineEncoding,paneHistoryBaselineDispatchSeq,paneHistoryBaselineSha256,peekCapabilityRevokedSeq,steeringGeneration,operatorInfluenced,status,turnResult,turnResultEncoding,turnResultSha256,clientAttachmentAuditProofSha256` |
| `formation_result` | `nodeId,attempt,status,outputs,outputHashes,reportArtifactId,artifactIds,diffArtifactIds,contributingSlotResultSeqs,resultEncoding,resultSha256` |
| `tool_dispatch` | `toolLeaseId,nodeId,attempt,toolBindingId,inputManifestSha256,inputHashes,profileSha256,parametersSha256,policySha256,determinismPolicySha256,executionBundleSha256,recordedBeforeExecute` |
| `tool_process_launch` | `toolLeaseId,launchId,nodeId,attempt,generation,recordedBeforeSpawn` |
| `tool_result` | `toolLeaseId,launchId,generation,nodeId,attempt,status,outputs,outputHashes,artifactRegistrations,artifacts,displayEvidence,timing` |
| `node_output` | `nodeId,status,outputs,reportArtifactId,artifactIds,diffArtifactIds,producedBy,timing,deliveredEdges` |
| `gate_evaluating` | `gateId,gateAttempt,nodeId,kinds,criterionProjection,inputRef,judgeChain,revisionCycleId,triggerFeedbackId,priorGateSeq` |
| `gate_kind_result` | `gateId,gateAttempt,kind,verdict,reason,evidence,evaluatedInputRef,resultEncoding,resultSha256,relatedSeqs,gateBindingId,inputSha256,profileSha256,evaluatorBundleSha256,parametersSha256,policySha256,determinismPolicySha256` with the anchored code-only conditional keys |
| `judge_result` | `gateId,gateAttempt,judgeNodeId,judgeAttempt,chainIndex,contextEncoding,contextSha256,priorResultSeqs,result,resultEncoding,resultSha256` |
| `judge_attempt_failed` | `gateId,gateAttempt,judgeNodeId,judgeAttempt,chainIndex,contextSha256,priorResultSeqs,code,reason,relatedSeq` |
| `gate_verdict` | `gateId,gateAttempt,verdict,perKind,kindResultSeqs,evaluatedInputRef,routePort,routedEdges,reason,feedbackPayload` |
| `artifact_attached` | `artifactProjection,source` |
| `artifact_observed` | `artifactId,availability,artifact,errorCode,observedAt,relatedSeq` with the anchored availability discriminant |
| `escalation_raised` | `trigger,severity,reason,source,nodeId,gateId,blocks` |
| `human_input_requested` | `gateId,gateAttempt,nodeId,promptProjection,choiceProjections,requestedBy,evaluatedInputRef,completedKindResultSeqs` |
| `human_verdict_recorded` | `commandId,commandPayloadSha256,gateId,gateAttempt,nodeId,verdict,reason,requestedSeq,decidedBy` |
| `error` | `code,message,boundary,errorScope,nodeId,gateId,slotId,toolLeaseId,recoverable,relatedSeq` with scope-conditioned identity |
| `run_blocked` | `reason,blockScope,blockedNodeId,blockedGateId,resumeAllowed,resumePolicy,openDispatches:SafeSchema2OpenDispatch[],retryTargets,nextEpoch` |
| `run_cancel_requested` | `commandId,commandPayloadSha256,reason,requestedBy,openNodeAttempts,openSlotDispatches,openToolLeases` |
| `run_canceled` | `cancelRequestSeq,reason,requestedBy,nodeAttemptDispositions,slotDispatchDispositions,reconciledToolLeases,final` |
| `run_failure_reconciliation_started` | `originCancelRequestSeq,code,reason,unrecoverable,relatedSeq,failureCause,openNodeAttempts,openSlotDispatches,openToolLeases,recordedBeforeReconciliation` |
| `run_failed` | `failureReconciliationSeq,code,reason,unrecoverable,relatedSeq,failureCause,nodeAttemptDispositions,slotDispatchDispositions,toolLeaseDispositions,final` |
| `run_succeeded` | `summaryArtifactId,outputArtifactIds,final` |

The schema-1 registry is separately closed over the 21 constants in
`src/internal/formations/run_ledger.go`. Its safe nested identities use the same
public types above. Recognized private keys are omitted only where this table
names them; every other unexpected envelope or data key rejects the complete
projection. This preserves historical compatibility without upgrading it to
schema-2 authority.

Schema-1 scope in this slice is the minimum exact frozen 21 rows below. Do not
add another compatibility event, infer newer authority, or perform unrelated
legacy hardening. `ctx-4dr9` owns later deletion of this compatibility surface.

Schema-1 `run_started` has an exact conditional data union. Both arms contain
the existing common members `{boardSlug,boardRev,missionId,beadId,limits}`:

- the Mission-root arm requires both `mode` and `formationId` to be absent and
  derives `RunIdentity.runRoot={kind:"mission",nodeId:missionId}`;
- the isolated-Formation arm requires and publicly preserves
  `mode:"formation"` plus a nonempty `formationId` validated by the stable
  Formation/node-id grammar, and derives
  `RunIdentity.runRoot={kind:"formation",nodeId:formationId}`.

Go decodes these as distinct `SafeSchema1RunStartedMissionData` and
`SafeSchema1RunStartedFormationData` structs; JSON and TypeScript expose the
same closed union with `never` for the absent Mission fields. Any other `mode`,
an empty/invalid `formationId`, a `formationId` without formation mode, or any
unknown key rejects the whole projection. Both variants continue to recognize
only `boardPath`, `snapshot`, `bindingsSnapshot`, and `objective` as private
omissions.

| Schema-1 type | Exact safe `data` allowlist | Exact recognized-private omission set |
| --- | --- | --- |
| `run_started` | conditional union above: Mission root has `boardSlug,boardRev,missionId,beadId,limits`; isolated Formation adds and preserves `mode:"formation",formationId` | `boardPath,snapshot,bindingsSnapshot,objective` |
| `run_resumed` | `resumedFromSeq,resumedBy,resumeMode,reason,openDispatches:SafeSchema1OpenDispatch[]` | none |
| `node_waiting` | `neededInputs,readyInputs,totalInputs,waitingFor` | none |
| `node_started` | `nodeKind,inputRefs,reason` where input refs omit locator/content fields | `brief`; nested `ref,text,reportRef,artifactRef` |
| `orchestration_team` | `mode,controllerSlot,controller,workers`; controller/worker entries are exactly `slotId,label,agentId,harness` | `socket,cwd`; nested `sessionStem,sessionRef` |
| `peer_plane` | `mode,peers`; peer entries are exactly `slotId,label,agentId,harness` | `path,socket,cwd`; nested `sessionStem,sessionRef` |
| `slot_dispatch` | `dispatchId,nodeId,slotId,agentId,harness,phase,promptSha256,nativeAck,recordedBeforeSend` | `sessionStem,sessionRef,promptRef` |
| `adapter_send` | `adapter,dispatchId,nodeId,slotId,phase,socketSha256,promptSha256,sent`; `adapter` is exactly `tmux` | `sessionRef` |
| `slot_result` | `dispatchId,nodeId,slotId,status,sentinel`; sentinel is exactly `runId,status` | nested sentinel `artifact` |
| `node_output` | `status,text,outputs,reason`; output keys are declared port ids and values omit locator fields | `reportRef`; nested `ref,reportRef,artifactRef` |
| `gate_evaluating` | `kinds,criterion,inputRef,judgeChain`; input ref omits locator/content fields | nested `ref,text,reportRef,artifactRef` |
| `gate_verdict` | `verdict,perKind,routePort,routedEdges,reason,inputRef`; input ref omits locator/content fields | nested `ref,text,reportRef,artifactRef` |
| `verification_verdict` | `verificationId,verdict` | none |
| `escalation_raised` | `trigger,severity,reason,source,nodeId,gateId,blocks` | none |
| `human_input_requested` | `gateId,nodeId,choices,requestedBy,inputRef,codeVerdict,codeReason,codePerKind,timeoutSeconds`; input ref omits locator/content fields | `prompt`; nested `ref,text,reportRef,artifactRef` |
| `human_verdict_recorded` | `gateId,nodeId,verdict,reason,requestedSeq,decidedBy` | none |
| `error` | `code,message,reason,boundary,nodeId,gateId,slotId,dispatchId,recoverable,relatedSeq` | none |
| `run_blocked` | `reason,code,boundary,blockedNodeId,blockedGateId,waitingNodes,recoverable,resumeAllowed,resumePolicy,openDispatches:SafeSchema1OpenDispatch[],nextEpoch` | none |
| `run_canceled` | `reason,requestedBy,softInterruptedSlots,final` | none |
| `run_failed` | `code,reason,boundary,recoverable,relatedSeq,final` | none |
| `run_succeeded` | `final,mode,formationId,missionId,reason` | `summaryRef,outputRefs,artifactRefs` |

Schema-1 `verification_verdict` is readable historical display evidence only.
It never creates a `RunGateView`, verdict action, status transition, route,
recovery predicate, receipt, or schema-2 field. The other four
compatibility-only public arms are `orchestration_team`, `peer_plane`,
`adapter_send`, and `verification_verdict`; the first three likewise expose no
session route or execution authority. All 21 constants require parity fixtures,
and all 37 schema-2 types require exact fixtures. Unknown types fail with no raw
fallback.

### Public typed errors

Every API error uses the existing error envelope and a fixed safe message. The
same typed cause maps identically from list, detail, events, SSE-before-headers,
artifact, Comms run-room, and mutation receipt adapters:

| Typed cause | HTTP | Public code |
| --- | ---: | --- |
| invalid/duplicate/empty/overflow run query | 400 | `FORMATIONS_RUN_QUERY_INVALID` |
| run/artifact identity not found or artifact not currently readable | 404 | `NOT_FOUND` |
| command record is pending/stale/substituted or not terminal | 409 | `FORMATIONS_COMMAND_NOT_TERMINAL` |
| artifact authority changed across optimistic revalidation | 409 | `FORMATIONS_ARTIFACT_AUTHORIZATION_CHANGED` |
| unsupported schema or invalid canonical projection/parity | 422 | `FORMATIONS_RUN_PROJECTION_INVALID` |
| unknown authority-bearing event type/key | 422 | `FORMATIONS_RUN_EVENT_UNKNOWN` |
| encoded page, event, input, or verified artifact exceeds its implementation limit | 413 | `FORMATIONS_RUN_RESOURCE_LIMIT` |
| guarded provider/capability unavailable or non-authorizing | 503 | `RUNTIME_AUTHORITY_NON_AUTHORIZING` |
| unexpected internal failure | 500 | `INTERNAL` |

Artifact errors set `Cache-Control: no-store` before this mapping. SSE errors
after headers follow the transport rule above: safe stderr/connection failure,
never a second public stdout/body shape. No error message includes a path,
private key, raw adapter text, record bytes, or authority locator.

## Consumer migration

Migration precedes deletion. API list/detail/status and escalations consume
`RunView`; mutation receipts consume `RunCommandReceipt`; events and replay-only
SSE consume `RunEventPage`; artifact open consumes the optimistic canonical open
flow. Archon status/list/logs/follow/ask and the existing Comms Formations-run
projection consume the same abstractions. Existing response fields may remain
temporarily only as adapters derived from the selected canonical projection.
Duplicate consumer scans are removed only after fixture parity.

API, Archon, and Comms select a structural view and every event page from the
same immutable `CanonicalRunProjection` and fail closed unless run ID,
generation, and source exact-match before exposing page data. The dashboard
receives detail and page separately, so its decoder and atomic refresh apply the
same equality check before merging. No consumer may infer page generation from
the requested run ID or fill a missing generation from a nearby view.

The accepted artifact route is exactly
`GET /api/formations/runs/{runId}/artifacts/{artifactId}`. Every response path,
including an error, sets `Cache-Control: no-store`. Success additionally sets
`X-Content-Type-Options: nosniff`, the verified `Content-Type`, and exact
`Content-Length`; it sends no `Content-Disposition`, ETag, Last-Modified, or
other validator. The handler writes only the already verified buffer. The
64 MiB buffer ceiling is an implementation resource limit for this algorithm,
not a public artifact contract or generic Files-read policy.

### Built-server contract split

Schema-1 compatibility cannot prove an artifact-success response. Its 21-event
vocabulary contains no artifact registration or public artifact identity, and
the projector must not synthesize either from a path, reference, or fixture.
Schema 2 remains non-authorizing in the production binary. Built-server
coverage therefore uses two explicit, non-interchangeable server kinds. The
production kind is invoked twice with isolated roots so a workspace-wide claim
cannot contaminate schema-1 assertions:

1. The supplied, normally built `chrote-server` is the production-wiring lane.
   Before either normal invocation, the contract script creates its separate
   no-fallback tree and a non-server, compiled-test-only guard probe reads that
   exact temporary authority root and configured workspace. The probe calls
   `GuardRuntimeWorkspaceAuthorityV1` directly,
   requires a nil error, exactly zero schema-1 inspection ledgers and one
   schema-2 guarded ledger, and the complete disabled capability including
   `SemanticProjection:false`; before/after snapshots must be identical. Only
   after that affirmative probe succeeds does the first normal invocation use
   only the ordinary temporary schema-1 root and prove bounded
   list/detail/events with exact detail/page generation parity, finite SSE,
   Comms, and embedded-asset behavior. After that process exits, a second
   isolated normal-server invocation uses the already-probed tree containing
   the same `run_01KXNP6VY3227H78329V52CKF8` as both a valid schema-1 fallback
   and a valid guarded schema-2 claim. Both detail and a run-list window that
   selects this candidate must return HTTP 503
   `RUNTIME_AUTHORITY_NON_AUTHORIZING`; list returns no partial page and never
   skips or filters the candidate. Neither route returns the fallback view while
   `RuntimeAuthorityCapability.SemanticProjection` remains false. The two
   normal-server logs are separate. Neither invocation claims artifact-success
   coverage for schema 1.
2. A separately compiled `src/internal/api` test binary is the HTTP artifact
   contract lane. Code in the exact `_test.go` fixture
   `src/internal/api/formations_run_contract_server_test.go` registers the real
   Formations run-artifact handler against injected in-memory implementations
   of the canonical reader and verified-artifact opener. It proves exact
   artifact bytes and the accepted response headers over HTTP. The fixture
   cannot read an authority root, does not alter
   `RuntimeAuthorityCapability`, and is excluded from every production build.
   The contract script gives Playwright this server through the separate
   `CHROTE_FORMATIONS_ARTIFACT_CONTRACT_URL`; it never substitutes that URL for
   the production server URL.

The compiled test binary is launched under `env -i` with only `PATH`, `LANG`,
evidence-rooted `HOME`/`TMPDIR`,
`CHROTE_FORMATIONS_CONTRACT_LISTEN`, and
`CHROTE_FORMATIONS_CONTRACT_SHUTDOWN_NONCE`. The existing `internal/api`
`TestMain` then creates `TMPDIR/chrote-tmux-tests-*/` and sets its sole
additional allowed CHROTE variable,
`CHROTE_SESSION_BANK_PATH=<that-directory>/sessions.json`. The fixture validates
that resolved containment and exact basename without reading the session-bank
path, and rejects every other `CHROTE_*` input, including every authority,
root, provider, or runtime input. It binds the supplied random loopback address
and exposes exactly the nonce-protected test-only route
`POST /api/__formations_contract/shutdown`. The contract script calls that
route in a `finally`/trap-controlled cleanup, waits for graceful
`http.Server.Shutdown`, and requires the test process to exit successfully.
The normal server must return 404 for that exact API path through its existing
API fallback and its built binary must not contain the fixture marker. These
checks prove that the artifact lane is test-only rather than a hidden production
authority path.

The guard probe is exactly
`src/internal/formations/authority_guard_contract_test.go`, compiled only with
the `formations_guard_contract` build tag and executed under `env -i` with the
script-generated no-fallback `formations-data` root and configured workspace as
its only two `CHROTE_*` inputs,
`CHROTE_FORMATIONS_GUARD_CONTRACT_DATA_ROOT` and
`CHROTE_FORMATIONS_GUARD_CONTRACT_WORKSPACE`. It is read-only and emits the
fixed success marker
`FORMATIONS_GUARD_CONTRACT_ACCEPTED schema1Inspection=0 schema2Guarded=1 semanticProjection=false`
to `authority-guard-probe.log`, and exposes no HTTP route or production
capability. A guard rejection cannot satisfy this probe, even though the later
normal-server 503 deliberately does not reveal whether its internal reason was
guard rejection or disabled capability. Both server lanes and the non-server
probe use one `mktemp -d` evidence root and perform no live service, provider,
or tmux action.

Archon JSON logs/follow output is page-NDJSON: every successful stdout line is
one complete `formations.run-events.v1` page with its mandatory immutable
generation/source. `--node` is text-only and combining
it with `--json` is a usage error with exit code 2; JSON never relabels a
filtered subset as a canonical page. After any mid-stream failure, Archon writes
only a safe error to stderr, exits nonzero, and emits no error-shaped stdout
line. This is the deliberate replacement for the old array/raw-event JSON
formats.

Creating a new ADR for this deliberate Archon page-NDJSON break is explicitly
declined. The owner ruling, this approved design, the implementation plan, and
their contract tests are the record and enforcement; no compatibility wrapper
or legacy hardening is added.

Comms keeps a 200-scanned-slot run-room preview and exposes its incompleteness:
`RoomProjection` adds required-for-run `messagesCursor` and `messagesHasMore`,
`RoomMessages` adds required-for-run `hasMore`, and run export adds
required-for-run `eventsCursor` and `eventsHasMore` while retaining its
200-message cap. In Go, all completeness additions, including
`RoomMessages.HasMore`, are pointers to JSON-safe `uint64`/`bool`, so run rooms
encode zero/false through non-nil pointers while non-run rooms omit the members
through nil pointers. The corresponding
sequence, cursor, and count members are JSON-safe `uint64`. For `run:` rooms
only, API `since` and `limit`
are each accepted at most once and parsed as digits-only unsigned decimal;
`since` is `0..MaxJSONSafeInteger`, absent limit is 200, and explicit limit is
`1..200`. Empty, duplicate, signed, negative, decimal, or overflowing values
return the typed 400 query error. Non-run parser/default behavior and valid JSON
shape remain unchanged; zero/false completeness fields are omitted there.
Before mapping run messages or export, Comms exact-matches the selected page's
run ID, generation, and source to the view selected from that same immutable
projection.

### DATA CONTRACT — engineering review

One dashboard controller owns:

- initial restore and polling from `RunView`;
- a separate bounded `RunEventPage` cursor and at most the 400 highest-sequence
  safe events; evicting older display events never rewinds the consumed cursor;
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

One restore or refresh performs at most one event-page read with limit 200 and
at most three `RunView` reads. It never loops to full history. Starting from the
retained cursor, it obtains a candidate view whose run identity and generation
match the retained state and whose cursor does not regress below that retained
cursor, reads one page, and re-reads the view only as needed until a candidate
structural view has `view.cursor >= page.cursor`. It need not chase writes that
occur after the selected page. The controller commits the candidate view, page,
bounded event window, and cursors in one atomic state transition only when:

- `retainedEventCursor <= page.cursor <= committedView.cursor`;
- page run identity, source classification, and run generation exact-match;
- all page ordering, duplicate, same-sequence equality, and source checks pass;
- the run generation did not change across the refresh.

Otherwise the complete previous last-good state remains byte-identical, is
marked stale, and cannot submit mutations or Peek. `eventHasMore` is true when
the selected page says so or when the retained event cursor is below the
committed structural cursor. Lagging bounded history is explicit data state;
later refreshes advance one page, and history never derives status, actions, or
authority. Continuous writer churn therefore has a fixed read budget and cannot
force an unbounded reconciliation loop.

Parity tests cover API/Archon/Comms/controller equality; schema-1 supported
ledgers; queued, running, blocked, resumed, waiting, canceling, failing, and final
runs; multiple attempts; failed Gates; exact/redacted outputs; artifact
revocation; session/baseline/Peek states; omitted extension tails and empty event
pages; scan-limit and encoded-byte event-page bounds; an individually oversized
sanitized event; JSON-safe maximum identities; all recovery predicates and
precedence, including schema-1 finality staying `live` with no reconcile
condition and every schema-2 deciding-`slot_result` case; aggregate ledger bytes
at exactly 64 MiB and one byte over; and whole-view rejection for every integrity
class.

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

1. Projector kernel, bounded event-page contract, and parity.
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

This event/detail refinement is governed by ADR-0006, ADR-0007, and this approved
design. It does not create a new ADR or architectural authority.
