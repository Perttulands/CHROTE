---
type: spec
status: active
authority: source-of-truth
workspace: chrote
enforced_by: scripts/doc-lint.py
---

# CHROTE Data Model Spec

Status: **Active core source of truth**.

This document defines the durable state CHROTE owns or projects. It covers the
cockpit, Services, Beads, and Formations. Implementation details may live in code
or narrower docs, but they should not contradict this file.

## State ownership

| State | Owner | Notes |
| --- | --- | --- |
| tmux sessions | tmux socket | CHROTE observes and attaches; tmux owns lifecycle |
| terminal panes/layout | dashboard local state + CHROTE APIs | Browser layout is preference, not work truth |
| workspace files | configured host roots | Server enforces root restrictions |
| Beads issues | `bd` workspaces | CHROTE renders JSON output and performs bounded writes |
| service credentials | host runtime config | Never sent to browser |
| service data | upstream local services | CHROTE proxies and displays selected operations |
| formation boards | `.formations/boards/` | TOML structural definitions |
| formation layout | `.formations/layout/` | TOML sidecars for visual placement/routing |
| agent personas | configured persona roots | TOML cards with stable ids and harness variants |
| canonical run authority (accepted target) | fenced CHROTE coordinator under the configured data root outside every Files root | Workspace command journal, writer-only ledger, immutable graph snapshot, private binding/result authority, raw pending-redaction state, and run-private Tool materializations |
| run inspection | canonical projection APIs plus authorized `.formations/artifacts/` files | Sanitized events, opaque binding projection, and registered safe artifacts; never replay authority |
| target occupancy, quarantine, and release receipts (accepted target) | host runtime state | Durable exclusive occupancy plus crash-safe release proof by canonical private target across runs; never browser authority |
| Formations terminal/inspector view | dashboard local state | Selection, Peek visibility, geometry, focus, and tiling; never workflow truth |

## Dashboard settings

User settings are browser-local preferences unless explicitly backed by server
state.

Current theme ids:

```text
matrix
dark
gastown
```

Host tmux appearance presets may share these ids but apply to tmux status/pane
colors on the host. Treat that as a host-side effect, not cosmetic browser-only
state.

## Session model

```ts
Session {
  name: string
  windows: number
  attached: boolean
  group: string
}
```

Grouping is derived from configurable naming rules. Group labels are display
helpers only; they are not proof that any particular harness or orchestrator is
installed.

### Run-bound slot binding (accepted target)

Generic cockpit sessions remain name/user projections. Formations execution
needs a stricter server-owned binding over the same Terminal-session resolver
and configured inventory. That inventory is the union of explicitly configured
user/socket sources. There is no second production Formations source or pool.
Disposable inventories are test, certification, and dogfood fixtures only; a
topology test registers one through the shared Terminal resolver rather than a
Formations-only path.

```ts
SlotResolution {
  slotId: string
  agentId?: string
  harness?: string
  state: "unresolved" | "runnable" | "ambiguous" | "unavailable"
  reason?: string
}

RunSlotBindingProjection {
  bindingId: string
  slotId: string
  agentId: string
  harness: string
  agentCardSha256: string
  sessionTargetId: string // opaque client/API handle
  adapterKind: "tmux"
  sessionName: string     // safe display metadata, never lookup authority
  resolvedRootId: string  // safe configured-root identity, never a raw path
  resolvedAt: string
}

RunSlotBinding {          // host-private immutable authority
  bindingId: string
  slotId: string
  agentId: string
  harness: string
  agentCardPath: string
  agentCardSha256: string
  projectionSha256: string
  targetKey: string       // private canonical pane-incarnation key
  sessionTargetId: string // opaque client/API handle
  adapterKind: "tmux"
  tmuxServerId: string
  socketIdentity: string
  sessionId: string
  sessionName: string     // display metadata, never lookup authority
  windowId: string
  paneId: string
  resolvedCwd: string
  resolvedRoot: string
  resolvedAt: string
  targetFingerprint: string
}

TargetDispatchIdentity {
  targetKey: string       // private canonical pane-incarnation key
  targetLeaseId: string
  sessionTargetId: string
  runId: string
  dispatchId: string
  bindingId: string
  nodeId: string
  attempt: number
  slotId: string
  targetFingerprint: string
  attachmentAuditRegistrationSha256: string // monitor armed before occupancy fsync
}

TargetDispatchRegistryRecord =
  | {
      state: "active"
      identity: TargetDispatchIdentity
      interactionLatch: "none"
      monitorEvidenceSha256: string
    }
  | {
      state: "latched"
      identity: TargetDispatchIdentity
      interactionLatch: "foreign_attachment" | "foreign_input" | "audit_lost"
      // foreign_input is the closed class for any unregistered pane mutation or input
      monitorEvidenceSha256: string
    }
  | {
      state: "terminal_hold"
      identity: TargetDispatchIdentity
      reason: "turn_closure_unproven" | "cancel_ack_unproven" |
        "foreign_attachment" | "foreign_input" | "attachment_audit_unavailable"
      // foreign_input includes unregistered history/lifecycle/topology mutations
      monitorEvidenceSha256: string
    }

TargetQuarantineCandidate {
  identity: TargetDispatchIdentity
  resultSeq?: number
}

TargetQuarantine {
  targetKey: string
  state: "quarantined"
  candidates: TargetQuarantineCandidate[] // stable runId/dispatchId/leaseId order
}

HostTargetDispatchQuarantine {
  state: "host_wide_quarantined"
  reason: "missing_or_corrupt_target_key"
  candidates: {
    targetKeyEvidence?: string
    targetLeaseId: string
    sessionTargetId: string
    runId: string
    dispatchId: string
    bindingId: string
    targetFingerprint?: string
  }[] // stable runId/dispatchId/leaseId order
}

TargetReleaseIdentity {
  targetKey: string
  targetLeaseId: string
  sessionTargetId: string
  runId: string
  dispatchId: string
  targetFingerprint: string
}

TargetTurnClosureProof =
  {
      proofKind: "harness_turn_closed"
      dispatchId: string
      targetLeaseId: string
      targetFingerprint: string
      paneHistoryBaselineSha256: string
      peekCapabilityRevokedSeq: number
      steeringGeneration: string // unsigned decimal uint64
      sentinelSha256: string
      terminalCaptureEnd: CaptureCursorV1
      harnessReadyEvidenceSha256: string // certified adapter channel, never pane bytes alone
      clientAttachmentAuditProofSha256: string
  }

ClientAttachmentAuditProof {
  proofKind: "tmux_clients_accounted"
  dispatchId: string
  targetLeaseId: string
  targetFingerprint: string
  attachmentAuditRegistrationSha256: string
  closureBarrierSha256: string
  terminalCaptureEnd: CaptureCursorV1
  monitorEvidenceSha256: string // commits to private ordered client-event journal
}

TargetInteractionClosureBarrierV1 {
  dispatchId: string
  targetLeaseId: string
  targetFingerprintSha256: string
  monitorEvidenceSha256: string
  terminalCaptureEnd: CaptureCursorV1
}

AttachmentAuditRegistrationV1 {
  monitorVersion: "formations-tmux-input-fence-v1"
  workspaceAuthorityId: string
  writerFence: number // positive JSON-safe integer
  targetKeySha256: string
  targetFingerprintSha256: string
  sourceIdentitySha256: string
  eventEpoch: string
  startOffset: string // first post-registration journal record offset
  inputGateSha256: string
}

TargetInteractionJournalRecordV1 {
  eventEpoch: string
  offset: string
  priorRecordSha256: string // registration hash for first record, prior record thereafter
  eventKind: "attach" | "detach" | "select" | "resize" | "reflow" |
    "history_clear" | "pane_respawn" | "pane_kill" | "pane_move" |
    "pane_join" | "pane_break" | "pane_swap" | "target_mutation" |
    "input" | "dispatch_send" | "peek_input" |
    "reconciliation_interrupt" | "fence_transition"
  eventIdentitySha256: string
  routeKind: "foreign" | "dispatch_prompt" | "peek_attach" | "peek_steering" |
    "reconciliation_interrupt" | "monitor"
  routeSeq: number // JSON-safe event sequence, or 0 for foreign/monitor
  targetStateSha256: string
}

TargetInteractionStateV1 {
  targetFingerprintSha256: string
  interactionLatch: "none" | "foreign_attachment" | "foreign_input" | "audit_lost"
  fenceState: "registration" | "dispatch_permit" | "peek" |
    "reconciliation_permit" | "closure" | "latched" | "released"
}

TargetInteractionAuditEvidenceV1 {
  registrationSha256: string // exact named registration
  eventEpoch: string // exact registration epoch
  startOffset: string // exact registration start, never empty
  endOffset: string // >= start after contiguous validation
  terminalRecordSha256: string // exact record at endOffset
  foreignState: "none" | "foreign_attachment" | "foreign_input" | "audit_lost"
}

TargetResultCommitReleaseProof =
  | {
      proofKind: "closure_barrier_held"
      closureBarrierSha256: string
    }
  | {
      proofKind: "post_result_pane_incarnation_gone"
      targetKey: string
      sessionTargetId: string
      targetFingerprint: string
      observedAt: string
    }

CaptureCursorV1 {
  historyEpoch: string // canonical unpadded base64url of exactly 16 bytes
  offset: string // unsigned decimal uint64 absolute byte offset
}

PaneHistoryBaselineV1 {
  targetFingerprintSha256: string // SHA-256 of exact UTF-8 targetFingerprint, no newline
  historyEpoch: string // canonical unpadded base64url of exactly 16 bytes
  offset: string // unsigned decimal uint64 absolute byte offset
  cols: number // integer 1..65535
  rows: number // integer 1..65535
}

TargetReadyProofV1 {
  targetFingerprintSha256: string // exact hash rule above
  acquisitionChallengeSha256: string // fresh one-shot challenge
  dispatchInputBarrierSha256: string
  harnessReadyEvidenceSha256: string // certified non-pane closed/ready evidence
}

TargetDispatchInputBarrierV1 {
  dispatchId: string
  targetLeaseId: string
  targetFingerprintSha256: string
  attachmentAuditRegistrationSha256: string
  promptSha256: string
  monitorEvidenceSha256: string
}

PeekSteeringEpoch {
  runId: string
  dispatchId: string
  targetLeaseId: string
  bindingId: string
  targetFingerprint: string
  steeringGeneration: string // monotonic unsigned decimal uint64
  state: "open" | "closed"
}

PeekCapabilityLifecycle {
  runId: string
  dispatchId: string
  targetLeaseId: string
  state: "none" | "issued" | "input_open" | "revoked"
  latestCapabilityGeneration: string
  latestCapabilityIssuedSeq: number // 0 only when none
  latestSteeringGeneration: string
  openSteeringStartedSeq?: number
  peekCapabilityRevokedSeq?: number // unique and irreversible for this dispatch
}

ReconciliationInterruptPermit {
  requestedSeq: number
  dispatchId: string
  targetLeaseId: string
  targetFingerprint: string
  authorityKind: "cancel" | "failure"
  authoritySeq: number
  // cancel iff seq names this run's unique cancel request; failure iff it names
  // this run's unique failure start; the dispatch is in that event's snapshot.
  // The permit is unique per dispatch. Cancel-origin failure reuses any prior
  // request; only a slot with none may create failure authority naming the start.
  interruptEncoding: "terminal-etx-v1"
  interruptSha256: string // SHA-256 of exactly byte 0x03
  outcome?: "sent" | "unavailable" | "unsupported" // absent means send_uncertain
}

FailureCause =
  | { kind: "slot", dispatchId: string }
  | { kind: "tool", toolLeaseId: string }
  | { kind: "error", errorSeq: number }
  | { kind: "none" }

FailureHeader {
  code: string
  reason: string
  unrecoverable: boolean
  relatedSeq: number
  failureCause: FailureCause
}

TargetFinalQuiescenceProof =
  | {
      proofKind: "dispatch_cancel_ack"
      dispatchId: string
      targetLeaseId: string
      targetFingerprint: string
      steeringGeneration: string // latest closed generation after input capability revocation
      peekCapabilityRevokedSeq: number
      interruptRequestedSeq: number
      cancelAckSha256: string
      terminalCaptureEnd: CaptureCursorV1
      harnessReadyEvidenceSha256: string // certified adapter channel, never pane bytes alone
      clientAttachmentAuditProof: ClientAttachmentAuditProof
      clientAttachmentAuditProofSha256: string
    }
  | {
      proofKind: "pane_incarnation_gone"
      targetKey: string
      sessionTargetId: string
      targetFingerprint: string
      steeringGeneration: string
      peekCapabilityRevokedSeq: number
      observedAt: string
    }

TargetReleaseReceipt =
  | {
      releaseKind: "result_committed"
      identity: TargetReleaseIdentity
      resultSeq: number
      proof: TargetTurnClosureProof
      clientAttachmentAuditProof: ClientAttachmentAuditProof
      releaseProof: TargetResultCommitReleaseProof
    }
  | {
      releaseKind: "final_quiescent"
      identity: TargetReleaseIdentity
      proof: TargetFinalQuiescenceProof
    }

RunToolBinding {
  toolBindingId: string
  nodeId: string
  profileId: string
  profileVersion: string
  profileSha256: string
  executionBundleSha256: string
  parameters: object
  parametersSha256: string
  policySha256: string
  determinismPolicySha256: string
}

ToolQuiescenceBoundaryCause =
  | { kind: "normal_completion", supervisorExitId: string }
  | { kind: "startup_recovery", recoveryId: string }
  | { kind: "cancellation", cancelRequestSeq: number }

ToolLaunchDeadlineAuthority { // host-private, fsynced before public launch/spawn
  deadlineAuthorityId: string
  runId: string
  toolLeaseId: string
  launchId: string
  nodeId: string
  attempt: number
  generation: number
  processScopeId: string
  startedAt: string
  timeoutPolicySha256: string
  timeoutMillis: number
  effectiveDeadlineAt: string // derived once as startedAt + timeoutMillis
}

ToolQuiescenceBoundary {   // host-private immutable deadline authority
  boundaryId: string
  deadlineAuthorityId: string
  boundaryKind: "normal" | "recovery" | "cancel"
  runId: string
  toolLeaseId: string
  launchId: string
  nodeId: string
  attempt: number
  generation: number
  processScopeId: string
  boundaryCause: ToolQuiescenceBoundaryCause
  startedAt: string
  timeoutPolicySha256: string
  timeoutMillis: number
  effectiveDeadlineAt: string // derived once as startedAt + timeoutMillis
}

RunGateBinding {
  gateBindingId: string
  gateId: string
  profileId: string
  profileVersion: string
  profileSha256: string
  evaluatorBundleSha256: string
  parameters: object
  parametersSha256: string
  policySha256: string
  determinismPolicySha256: string
  maxInputBytes: number
  maxResultBytes: number
  maxOperations: number
  resultEncoding: "decision-result-jcs-v1"
}

BindingHealth = "runnable" | "unavailable" | "stale" // projected separately
```

Run preflight returns one resolution per declared slot in the selected `runRoot`
executable subgraph; unassigned is `unresolved/agent_unassigned`. It starts only
when every resolution is runnable. A successful run then stores one exact
host-private binding and one hash-linked safe projection per slot. The private
binding records persona-card path/hash, tmux server/socket identity,
session/window/pane ids, verified cwd/root, resolution time, and canonical
`targetKey` for that exact pane incarnation. Its `targetFingerprint` commits to
adapter/harness identity, foreground-process start identity, cwd/root, and the
pane-incarnation evidence used by resolution. A
durable one-to-one resolver mapping guarantees independent resolutions of the
same key return the same opaque `sessionTargetId`; a replacement incarnation
gets a new key/handle. Raw host routing details remain server-side. `bindingHealth` is
projected from the latest `slot_binding_observed` event and never mutates the
frozen target. That event records binding/target ids, health, stable reason,
observation time, and related sequence; projection never queries tmux directly.
Accumulated context in that exact session is intentional. A matching candidate
is runnable only when unleased, unattached, and certified closed/ready for the
exact fingerprint through the harness adapter's non-pane control channel.
Stable unavailable reasons are `session_target_leased`,
`session_target_attached`, `session_target_harness_busy`, and
`session_target_readiness_unknown`; quiet pane output or prompt text is not
readiness, while `session_target_attachment_audit_unavailable` means complete
client/input monitoring cannot be armed. A process-local CHROTE mutex is not
complete monitoring: stock tmux on an owner-accessible raw socket admits
independent attach, select, resize, control, paste, and `send-keys` routes. Until
`ctx-ug7.21` selects, `ctx-ug7.22` implements, and `ctx-ug7.23` certifies an
enforceable same-pool input boundary, that adapter returns
`session_target_attachment_audit_unavailable` and sends nothing; this does not
authorize a separate Formations production pool. Unknown fails unavailable
without a binding. Connected hidden
CHROTE Terminal iframes count as attached; binding never detaches them.
The user may explicitly disconnect a CHROTE-owned presentation client and retry,
but cannot reclaim an external client or another run's lease. Formation
attachment ownership begins only when the exact target-registry occupancy is
fsynced, not when `RunSlotBinding` is written. Immediately before send, atomic
target acquisition repeats registry, attachment, fingerprint, and certified
readiness checks under the target lock. After exact input validation it fsyncs
`AttachmentAuditRegistrationV1`, arms its enforceable route gate, and only then
fsyncs its anchored first `fence_transition` journal record and target
occupancy, leaving no empty-chain or post-occupancy audit gap. Under that same
critical section it drains the chained interaction journal, installs
`TargetDispatchInputBarrierV1`, records a fresh one-shot `TargetReadyProofV1`
bound to that barrier, captures the exact history baseline, and fsyncs
`slot_dispatch` before the barrier permits the one already-hashed prompt send.
It never steals, creates, or selects an alternate target.
Each slot dispatch references its own binding, opaque target, and frozen
fingerprint, so one attempt may expose several sessions. After atomic target
acquisition and immediately before send, the host revalidates that exact
fingerprint and records exact `PaneHistoryBaselineV1` canonical bytes plus their
SHA-256 in the writer-private `slot_dispatch`. A later capture starts at the
exact matching `CaptureCursorV1`; trim/reset, resize/reflow, missing continuity
after restart, or ambiguity fails `capture_baseline_unavailable` with no result
or ordinary release. Sanitized consumers receive encoding, hash, and validation
state only. Changed cwd/root, harness, foreground process, or pane incarnation
records the binding stale and sends nothing. Dispatch, recovery, and terminal
Peek must not fall back to a same-named session. Recovery may reattach the same
exact pane only while the fingerprint and capture continuity match; replacing a
target requires a new run.
Two selected slots resolving to the same private target key/opaque handle reject
before `run_started`. Across runs, a host-wide durable exclusive target lease
keyed by `targetKey` binds one
target to one exact run/dispatch/binding/node/attempt/slot before the durable ledger
dispatch is fsynced and the prompt is sent. Before composing an ordinary or
judge Formation prompt, every artifact-backed input is opened once relative to
its authorized root without following links; regular identity, media, size, and
SHA-256 are validated and prompt bytes come from that same handle, never the
mutable path. Non-redacted drift fails
`formation_input_integrity_failed`; unavailable Redact=true bytes fail
`redacted_input_unavailable`; both send nothing. The coordinator composes one prompt
byte slice in memory, hashes it, fsyncs only `promptSha256`/dispatch identity, and
sends that same slice at most once before discarding it. No prompt bytes, ref,
path, or authority id is durable; the hash cannot reconstruct or authorize a
resend. Prompt, completion sentinel, and
result carry matching run, dispatch, and target-lease ids. Busy acquisition
sends nothing and fails loud with the stable unavailable reason. Shared Terminal
attach paths deny ordinary attachment after occupancy; a certified private
monitor accounts for every later client event. A foreign attach or lost monitor
continuity revokes Peek and latches the dispatch non-authorizing, with no result
or ordinary release until exact quiescence reconciliation. A completion result
closes only after the exact
sentinel is terminal in its bounded capture and the certified harness adapter
proves the same fingerprint returned to a closed/ready turn; trailing output is
not closed. `capturedRange` starts at the dispatch's exact
`paneHistoryBaseline`; prior accumulated history may inform the agent but cannot
be parsed as this dispatch's sentinel or output. Result fsync precedes normal
release. The closure proof exact-matches the baseline hash and latest closed
steering generation, and its certified adapter evidence cannot be produced from
user-writable pane bytes; a typed/echoed sentinel alone never closes the turn.
Result acceptance and release serialize against later Peek input. Closure first
stops capability issuance, drains input, closes every generation, and fsyncs the
unique irreversible capability revocation. Its proof binds that revocation and
the exact `ClientAttachmentAuditProof`; a foreign attach-then-detach race cannot
be accepted or relabeled as steering. Under the same target critical section,
the certified boundary installs `TargetInteractionClosureBarrierV1`, drains the
interaction journal through the terminal cursor, and holds the mutation/input
fence while the exact audit proof and `slot_result` fsync. That result remains
unconsumable by aggregation, routing, or finality until a durable non-occupying
`result_committed` receipt carries the same proof plus either continuous
`closure_barrier_held` release proof or exact
`post_result_pane_incarnation_gone` proof. Release atomically replaces the
occupying record; receipt fsync is its linearization point, after which the
barrier may be removed. The receipt survives crashes through final-event fsync
but does not block safe later acquisition. A post-result barrier fault enters a
result-closed quarantine and cannot promote output while that exact release
proof is absent. A result-closed
dispatch never becomes a slot disposition. Final failure/cancel records a
`final_quiescent` receipt with a closed certified proof, non-authorizing busy hold, or
fail-closed quarantine for each unmatched slot disposition; holds/quarantines
remain until exact quiescence is proven. Valid final proof is either an exact
dispatch-bound cancel/ready acknowledgement for the frozen fingerprint or proof
that that pane incarnation is gone. A foreign-attachment/input or lost-audit
latch cannot use the cancel/ready branch; only exact old-pane-incarnation-gone
proof releases it. A sent interrupt, silence, prompt text,
display name, bare PID, or unknown proof kind is insufficient. Registry state
never expires by time, name, or PID.
If a ledger dispatch lacks its expected private lease, the arbiter first
reconstructs a non-authorizing `TargetQuarantine` at the frozen `targetKey`.
Its stable candidate set preserves the expected identity/result and every
conflicting occupant as separate exact run/dispatch/lease entries. Each
unmatched candidate may expose `quarantined` in its own final disposition; a
result-closed candidate must become its exact release receipt before finality.
That key stays busy until every candidate dispatch is proven quiescent and
removed. A missing/corrupt canonical key creates the separate durable
`HostTargetDispatchQuarantine`, whose stable candidates preserve all identities
available from ledger dispatch/binding evidence and whose safety latch denies
every target acquisition host-wide until operator repair and non-authorizing
quiescence remove every candidate.
Failure never recreates active authority or frees the pane.
An idle deadline without the exact sentinel plus certified closed-turn proof records slot-scoped
`dispatch_idle_timeout` but no `slot_result`; the dispatch and target lease stay
unmatched for bounded reattach, and timeout alone never authorizes release.

## Beads model

CHROTE consumes `bd --json` and keeps Beads as the task source of truth.
Dashboard writes must be bounded operations such as comments or state transitions
that `bd` supports. CHROTE must not fork a second issue database.

## Service adapter model

```text
Browser -> CHROTE route -> localhost upstream service
```

The browser never receives upstream bearer tokens. Missing tokens or unavailable
upstreams produce degraded states.

## Formations files

Recommended workspace layout:

```text
.formations/
  boards/
    <slug>.formation.toml
  layout/
    <slug>.layout.toml
  artifacts/
    <run-id>/
      <registered sanitized output files>
agents/ or CHROTE_AGENTS_DIR/
  <agent-id>.toml
```

Canonical accepted-target run state instead lives under the configured CHROTE
data root, outside every configured Files read/write root:

```text
<chrote-data>/formations/workspaces/
  registry.lock
  registry.private.json
  <workspace-authority-id>/
    workspace.bootstrap.json
    workspace.private.json
    owner.lock
    owner.private.json
    admission-policies/<policy-rev>.json
    commands/<command-id>.json
    runs/<run-id>/
      events.ndjson
      graph.snapshot.toml
      bindings.private.toml
      refs/                # writer-only results, materializations, pending raw state
    quarantine/
```

The generic Files API cannot list, read, write, rename, or delete that tree.
Run/event APIs return sanitized projections; registered safe artifacts may open
through the existing File Peek renderer only after capability-scoped resolution
and current projection revalidation. A same-named workspace file or mutated
inspection export is never execution or replay authority.

The immutable graph snapshot may copy exact Mission objectives, Formation briefs,
and Gate criteria only when its embedded, hash-covered
`RunAuthoredConfigManifest` classifies the exact field/node with a closed source
role, versioned encoding/media type, and SHA-256. Missing/extra/mismatched entries
are invalid. Gate criteria use `gate-criterion-utf8-v1`/`text/plain`; human
prompt and PASS/FAIL labels use only
closed fixed-system templates. This configuration was already durable in the
board and may intentionally outlive later board edits/deletion. No
runtime/external value or composed prompt is part of that exception. Mission
output/unchanged deliveries and isolated Formation seed input may copy the
objective/brief only through the classified root-derived projection that
exact-matches `run_started.rootInputProjection`; generic unclassified copies are
invalid.

A private authority directory without valid seq-1 `run_started` is not an
admitted run and sends nothing. Startup recovery cleans/fsyncs every pending raw
target and obligation, then deletes the orphan tree and parent-directory-fsyncs.
Unprovable cleanup or identity quarantines it as non-authorizing with no public
bytes/replay handle; recovery never adopts it as a run.

### Workspace runtime authority and command journal

Current main still lets the API and Archon construct separate synchronous run
engines and writes run files under the workspace. The following ADR-0007 shapes
are accepted schema-2 target state, not a claim about the current binary.

```ts
WorkspaceRegistry {
  registrySchema: 1
  recordRev: uint64
  entries: WorkspaceRegistryEntry[]
}

WorkspaceRegistryEntry {
  workspaceAuthorityId: string
  configuredPath: string
  device: string
  inode: string
  workspaceRootIdentitySha256: string
}

WorkspaceBootstrap {
  bootstrapSchema: 1
  workspaceAuthorityId: string
  rootIdentityEncoding: "workspace-root-identity-v1"
  workspaceRootIdentitySha256: string
}

WorkspaceAuthority {
  recordRev: uint64
  authoritySchema: uint64             // 2 for this target; monotonic high-water
  workspaceAuthorityId: string       // opaque, server-issued, caller cannot choose
  rootIdentityEncoding: "workspace-root-identity-v1"
  workspaceRootIdentitySha256: string // exact configured/opened root identity
  nextWriterFence: uint64             // strictly increasing, never reused
  nextAdmissionSeq: uint64            // strictly increasing FIFO identity
  admissionPolicyRef: WorkspaceAdmissionPolicyRef
}

WorkspaceAdmissionPolicyRef {
  policyRev: uint64
  policySha256: string
}

WorkspaceAdmissionPolicy =
  | { policySchema: 1, policyRev: uint64, priorPolicySha256: string,
      state: "disabled" }
  | { policySchema: 1, policyRev: uint64, priorPolicySha256: string,
      state: "configured",
      maxActiveRuns: integer, maxQueuedRuns: integer }

WorkspaceOwnerLease {
  leaseSchema: 1
  recordRev: uint64
  workspaceAuthorityId: string
  ownerInstanceId: string             // private process/boot identity
  writerFence: uint64
  acquiredAt: string
  renewedAt: string
  leaseUntil: string
}

RunCommandRecordBase {
  commandSchema: 1
  recordRev: uint64
  commandEncoding: "run-command-jcs-v1"
  commandId: string                   // ^cmd_[0-7][0-9A-HJKMNP-TV-Z]{25}$
  commandKind: "start" | "resume" | "cancel" | "verdict"
  commandPayload: object              // exact closed variant below
  commandPayloadSha256: string
  admittedWriterFence: uint64
  stateWriterFence: uint64
}

RunCommandRecord =
  | RunCommandRecordBase & { state: "pending" }
  | RunCommandRecordBase & { state: "applied", runId: string, effectSeq: number,
      outcomeWriterFence: uint64,
      decisionAdmissionPolicyRef: WorkspaceAdmissionPolicyRef | null }
  | RunCommandRecordBase & { state: "rejected", rejectionCode: string,
      outcomeWriterFence: uint64,
      decisionAdmissionPolicyRef: WorkspaceAdmissionPolicyRef | null }

RunCommandReceipt =
  | {
      commandId: string
      commandPayloadSha256: string
      commandKind: "start" | "resume" | "cancel" | "verdict"
      outcomeWriterFence: uint64
      state: "applied"
      runId: string
      effectSeq: number
      decisionAdmissionPolicyRef: WorkspaceAdmissionPolicyRef | null
    }
  | {
      commandId: string
      commandPayloadSha256: string
      commandKind: "start" | "resume" | "cancel" | "verdict"
      outcomeWriterFence: uint64
      state: "rejected"
      rejectionCode: string
      decisionAdmissionPolicyRef: WorkspaceAdmissionPolicyRef | null
    }
```

`registry.private.json` is a closed schema-1 object. Its entries have exactly the
fields above and sort by decoded unsigned numeric `device`, decoded unsigned
numeric `inode`, then valid UTF-8 `configuredPath` bytewise. They use the same
canonical path/device/inode rules as `workspace-root-identity-v1`. A reader strict-validates
the registry under `registry.lock` before selecting or creating an authority id;
unknown schema/key, duplicate configured/opened identity, or conflicting mapping
authorizes no mutation. Registry generations use the atomic mutable-file rule
below.

`workspace-bootstrap-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{bootstrapSchema,workspaceAuthorityId,rootIdentityEncoding,
workspaceRootIdentitySha256}` with no unknown keys or trailing newline. The
published bootstrap is immutable. Current authority compatibility lives in the
separate mutable `workspace.private.json.authoritySchema` high-water mark.
The schema-2 target requires that value to be exactly `2`; a future supported
authority schema changes the supported value without rewriting the immutable
bootstrap, while an older reader rejects that higher value before mutation.

`workspace-admission-policy-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly
one closed `WorkspaceAdmissionPolicy` variant with no unknown key or trailing
newline. Revision 1 has `priorPolicySha256=""`; every later revision is exactly
the previous revision plus one and names the previous generation's
64-lowercase-hex SHA-256. Its immutable file is
`admission-policies/<policyRev>.json`, where the
decimal revision has no sign or leading zero. `WorkspaceAuthority.admissionPolicyRef`
exact-matches the current file's revision and SHA-256. Historical generations
referenced by a command or ledger event are retained and remain hash-verifiable.
First registration creates/fsyncs disabled revision 1 before publishing the
initial WorkspaceAuthority ref or parent registry mapping.

`workspace-root-identity-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{configuredPath,resolvedPath,device,inode}` with no unknown keys or trailing
newline. Paths are absolute valid UTF-8 with NUL rejected, lexical dot segments
removed, `/` separators, and no trailing slash except root. `configuredPath`
retains the cleaned configured spelling; `resolvedPath`, device, and inode come
from the same race-safe opened directory identity after symlink resolution.
Device and inode are JSON strings matching `0|[1-9][0-9]*` and must decode as
`uint64`; they are not JSON numbers. The object remains private and
`workspaceRootIdentitySha256` hashes those exact bytes.

First registration holds a coordinator-local mutex plus `registry.lock` before selecting an authority-id
directory. `registry.private.json` enforces one mapping for both the cleaned
configured spelling and the opened `(device,inode)` identity, so aliases cannot
create separate owner locks. The new bootstrap/private directory is fsynced
before its mapping and parent are fsynced. One unique exact unregistered creation
may be completed after a crash; duplicate/conflicting identity fails loud and
authorizes nothing.
`workspaceAuthorityId` is server-issued and matches canonical uppercase ULID
grammar `^wsa_[0-7][0-9A-HJKMNP-TV-Z]{25}$`, validated before directory
construction.

`run-command-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly one of these
closed objects, with no unknown keys or trailing newline:

```text
start   = {kind,authoritySchema,actor,workspaceAuthorityId,boardId,runRoot,
           expectedBoardRev,expectedBoardETag,limits}
resume  = {kind,authoritySchema,actor,workspaceAuthorityId,runId,blockedSeq,
           resumeMode,reason}
cancel  = {kind,authoritySchema,actor,workspaceAuthorityId,runId,
           expectedLastSeq,reason}
verdict = {kind,authoritySchema,actor,workspaceAuthorityId,runId,gateId,
           requestedSeq,verdict,reason}
```

`authoritySchema` is integer `2`. `runRoot` is exactly `{kind,nodeId}`, where
kind is `mission` or `formation` and the stable id matches. `resumeMode` is
`reattach` or `retry-failed-producer`; verdict is `pass` or `fail`. Revisions and
sequences are JSON integers in `1..9007199254740991`. `limits` always contains
JSON-integer `maxDispatch`, `maxAttempts`, and `wallClockSeconds` in
`1..2147483647` plus boolean `redact`. An absent ETag/reason normalizes to `""`;
a present ETag is exactly 64 lowercase hexadecimal characters, and no other
field is optional. Identifiers are resolved/validated before the request becomes
journalable. Every SHA-256 field in these authority/command shapes is exactly 64
lowercase hexadecimal characters except the explicitly empty revision-1
`priorPolicySha256`. Actor is non-empty
valid UTF-8 with BOM/NUL rejected, leading/trailing ASCII space and tab removed,
and case otherwise preserved. Reason is valid
UTF-8, rejects BOM/NUL,
normalizes CRLF/CR to LF, trims leading/trailing ASCII space, tab, and LF, and
is then bounded/redaction-checked. `commandId` and transport timing are outside
the bytes. `commandId` matches canonical uppercase ULID grammar
`^cmd_[0-7][0-9A-HJKMNP-TV-Z]{25}$` and is validated before building the exact
`<commandId>.json` path.
`abort` and `stop` normalize to `kind=cancel` before this shape is hashed.
Unknown keys, wrong JSON types, non-canonical enums, out-of-range values, and
invalid transport/schema input are never journalable; every rejection after
valid canonicalization is durable.

The complete canonical `commandPayload` is stored with its hash and
recanonicalized on read. Its hash-bound `actor`, `kind`, workspace, run, reason,
mode/verdict, and precondition are the sole command authority; duplicate event
fields exact-match them. The record contains no second actor. Journal
lookup/create/update is serialized under the workspace authority lock with
create-if-absent semantics. A pending record starts with equal
`admittedWriterFence`/`stateWriterFence` and has no `runId`, `effectSeq`, or
`rejectionCode`. Takeover may advance only the state fence. The same
`commands/<commandId>.json` record becomes the receipt: `applied` requires exactly
`runId`, `effectSeq`, immutable `outcomeWriterFence`, and
`decisionAdmissionPolicyRef`; `rejected` requires exactly `rejectionCode`, the
immutable outcome fence, and the decision policy ref. That ref is the exact
generation used for every terminal start decision and is `null` for every
non-start command.
`RunCommandReceipt` is the closed API projection of that terminal record, not a
second file. `stateWriterFence` names the fence that last published the command
record; `outcomeWriterFence` separately and immutably names the applied effect
event's origin fence or the rejection decision fence. A replacement at fence F2
may therefore repair an F1 effect by publishing the record with
`stateWriterFence=F2` and `outcomeWriterFence=F1`. The same id/hash returns the same receipt; another hash fails
`command_id_conflict` before effect.

Before fence allocation, a process holds a coordinator-local mutex plus
process-shared `owner.lock` and strict-validates the immutable closed bootstrap
plus the mutable current `WorkspaceAuthority` and its hash-matched immutable
admission-policy generation plus the complete contiguous prior-hash chain to
revision 1. Missing generations, discontinuities, or cycles are invalid.
Unsupported, missing, or conflicting bootstrap,
workspace authority, or policy schema is strictly read-only: no fence,
projection-as-valid, cleanup, quarantine, or tmux action. Matching schema numbers alone do not enable schema-2
semantics. Only a registered complete projector/coordinator may reserve a fence,
fsync the advanced counter, and then publish its lease; counter gaps are allowed
and reuse is forbidden. Elapsed lease time cannot bypass a still-held kernel
owner lock.

The owner lock remains held from current lease/fence validation through each
authority write+fsync or bounded non-idempotent send, spawn, interrupt, cleanup,
or quarantine call. Takeover cannot interleave between check and effect. Valid
lock order is parent registry (registration only), workspace authority, then
host target arbiter; reverse acquisition is invalid. Valid
historical event fences are monotonic non-decreasing allocated owner epochs; an
older prefix survives takeover. Fence regression, an unallocated fence, or a new
append/effect not using the current fence is invalid. Private records retain
`originWriterFence`; current recovery writes a separate `stateWriterFence`.

Immutable authority files never receive in-place writes at their canonical path.
Under the governing lock, the writer writes complete bytes to a unique
same-directory staging file, fsyncs it, atomically installs the canonical name
with no-replace semantics, and fsyncs the parent. An existing canonical file is
idempotent success only when its strict bytes/hash equal the requested immutable
value; mismatch conflicts and is never replaced. A pre-install crash leaves only
a non-authorizing staging file; after install, only complete bytes can exist and
recovery repeats the parent fsync. Only the governing authority holder (registrar
under `registry.lock` or current fenced owner under `owner.lock`) may remove a
validated unreferenced staging file. Mutable registry generations publish under
`registry.lock`; workspace authority/counter,
owner-lease, and command-record generations publish under `owner.lock`. Admission
policy generations are immutable and publish under `owner.lock` before the
current workspace reference changes. Each mutable record has
a `recordRev` starting at 1; every successful replacement is exactly the last
published revision plus one and rejects a stale, skipped, or regressed
predecessor. Each uses a generation-checked same-directory
temp, file fsync, atomic rename, and parent fsync. Migration holds both locks in
parent-registry then workspace order. Torn/stale/conflicting published records
fail loud; a temp file never outranks the last valid record, and a canonical
immutable path is never a partial-file recovery surface.
A shared-package file lock protects bytes only and grants no runtime authority.

Every schema-2 JSON record/policy revision, writer-fence field, allocated
event/effect sequence, workspace-admission identity, and next counter is an integer in
`1..9007199254740991`. Allocation past that bound fails closed before mutation;
rounding, wrap, and reuse are invalid.
The only zero-valued sequence-reference sentinels are `priorIssuedSeq`,
`capabilityIssuedSeq`, `latestCapabilityIssuedSeq`, and
`originCancelRequestSeq`, plus private-journal `routeSeq`. Each is a JSON-safe integer in
`0..9007199254740991`, and `0` never names or allocates an event.

For start, the current owner fsyncs the pending command, immutable private run
authority, and `run_started(seq=1)`; when immediate capacity is reserved it also
fsyncs `run_activated` before linking the applied receipt and returning the run
id. `run_started` records `workspaceAuthorityId`, monotonic
`workspaceAdmissionSeq`, exact `admissionPolicyRev`/`admissionPolicySha256`,
`admissionCommandId`, `commandPayloadSha256`,
with `authoritySchema` and `writerFence` in the event envelope. `run_resumed`, `run_cancel_requested`, and
`human_verdict_recorded` likewise bind their command id/hash. A crash after the
effect but before receipt repair returns the same run/effect on retry.
Recovery of a pending start resumes admission from the stored canonical payload.
A pending resume/cancel/verdict rechecks its exact blocked/cancel/human-request
precondition and appends once only when still valid, otherwise records one stable
rejection. If any matching command effect is already durable, recovery repairs
the applied receipt from that event and never reapplies it; the repaired record
uses the current state fence but preserves the effect event's origin fence as its
outcome fence.

`maxActiveRuns` is a JSON integer in `1..2147483647`; `maxQueuedRuns` is a JSON
integer in `0..2147483647`. Under the workspace admission lock, the writer
strict-validates the immutable policy generation named by
`WorkspaceAuthority.admissionPolicyRef`; there is no implicit default, and
schema-2 authority starts with the closed `state=disabled` revision 1. Disabled
rejects new starts as `admission_disabled` and pauses queued activation without
canceling active or queued work; queue wall clocks continue. Admission and
activation resume only after an operator publishes a configured generation.

A policy update binds its expected current revision/hash and exact canonical next
bytes. Under `owner.lock`, it stages/fsyncs and atomically no-replace-installs the
next immutable chained generation before advancing the workspace policy ref and
workspace record revision. If the current ref remains the expected predecessor,
an exact already-installed next generation is completed; if the ref already
names that exact next generation whose prior hash is the expected ref, retry
returns the original success. Every other stale ref or byte/hash conflict fails
before mutation. Every terminal
start command stores the exact decision policy ref. `run_started` stores that
admission revision/hash, and every immediate or dequeued `run_activated` stores
the configured revision/hash used for activation.

`activeCount` counts non-final ledgers with `run_activated`, while `queuedCount`
counts ledgers whose latest projected status is exactly `queued`. Unactivated
`canceling` or `failing` runs are not queued and can never activate. Under a configured policy,
`maxActiveRuns` alone gates activation. Before a fresh start may activate, the
writer activates eligible queued runs by smallest `workspaceAdmissionSeq` while
capacity remains. It may then reserve and fsync the next admission counter and
append `run_started` plus immediate `run_activated` when capacity still exists,
even when `maxQueuedRuns=0`. If no active slot remains, `maxQueuedRuns` alone
gates the new queued admission: append only `run_started` when
`queuedCount < maxQueuedRuns`; otherwise record `run_queue_full` in the terminal
start command with the decision policy ref before a run directory or event.
Lowering `maxActiveRuns` does not cancel active runs but blocks only new
activation until `activeCount < maxActiveRuns`; lowering `maxQueuedRuns` does not
cancel queued runs or block dequeue, but blocks only fresh queued admission while
`queuedCount >= maxQueuedRuns`. Admission counter gaps are allowed and reuse is
forbidden.

`run_started` alone projects queued; the unique `run_activated` projects running
and is required before graph/dispatch events. Restart recomputes exact counts/order
for current state from run ledgers and strict-validates every retained policy
ref. Schema 2 has no workspace-global admission-decision sequence: a policy ref
attributes exact policy bytes but does not independently prove historical
cross-run capacity interleaving. Concurrent starts still serialize under the one
authority critical section and are certified by contention/crash tests. Queue
wait
consumes wall clock, and an expired queued run may fail without activation.
Cancellation, cleanup, and recovery precede fresh activation.
Every activated non-final ledger counts against `maxActiveRuns`, including while
blocked, `waiting_human`, canceling, or failing, and releases that slot only at an
execution-final event. This conservative first policy is reconstructible from
the ledger without an unmodeled requeue transition.

### Board definition

A board definition contains structural state:

- board id/name/version;
- missions with fixed `out` as the single run-start payload address in this
  phase;
- formations;
- accepted-target Tool steps that reference host-owned versioned profiles (not
  implemented on current main);
- gates, including `kinds` and `criterion`; schema-1 compatibility definitions
  may contain `command`, `commandArgv`, `commandShell`, or `commandCwd` only as
  read-only inspection/migration-plan input, while schema-2 code Gates reference
  pure host-owned evaluator profiles and never own a process;
- slots;
- accepted-target stable named ports with direction, accepted payload kind,
  allowlisted `acceptedMediaTypes` for `work`, required flag, and `data` or
  `retry_control` role;
- accepted-target connections with explicit schema-2 `workflow` or reserved
  `judge` channel;
- schema-1 verification/check specs, retained for inspection but rejected from
  schema-2 execution until `ctx-ug7.17` defines or retires them.

All workflow payload ports are directional. A Gate's reserved `judge` socket is
an evaluation-control relationship rather than a typed payload port; it permits
one judge send and one return and never routes downstream work.
`retry_control` is accepted only on an optional Formation input; Tool inputs use
`data`.
After validated `retry_control` edges are removed, the `workflow` channel must
be a DAG. Shared read/write validation and run preflight reject every other
cycle; distinct required ports do not make a cyclic dependency executable.
If an acyclic run quiesces after delivering only some required JOIN inputs, it
records node-scoped `unsatisfied_required_input` and a non-resumable node block
with empty open dispatches/retry targets. That node projects `blocked`; readiness
counts remain evidence rather than a live waiting state.
In the accepted target, executable preflight requires a complete judge channel
if and only if `gate.kinds` contains `formation`; drafts may temporarily retain a
half-edge or kind/channel mismatch while being authored.

An accepted-target Tool board entry stores a profile id/version constraint and
modeled non-secret parameters. Run start freezes the resolved profile
version/hash, parameters, effective policy hash, and content-addressed execution
bundle hash. The first Tool profile class is pure and certified deterministic.
Its closed sandbox permits only the sealed input set, frozen bundle/parameters/
policy, and one empty run-private output root. Network, secrets, undeclared
environment or filesystem reads, and external writes are denied; locale and
timezone are normalized; clock and entropy are denied or supplied by a frozen
deterministic provider. Profile admission includes repeat-run vectors with
expected output hashes. Effectful or uncertified profiles are rejected rather
than hidden behind an ambiguous replay flag.

An accepted-target schema-2 code Gate stores a pure in-process evaluator profile
id/version constraint and modeled non-secret parameters. Run start freezes one
`RunGateBinding` with Gate/profile ids, exact profile version/content hash,
content-addressed evaluator implementation bundle, normalized parameters/hash,
effective policy hash, and determinism-policy hash. The evaluator is admitted
only under the same closed observable-input rule: declared input, frozen bundle/
parameters/policy, normalized locale/timezone, and frozen-or-denied clock and
entropy; it cannot spawn, use network/secrets, read undeclared host state, or
write host state. Its frozen policy also caps input bytes, result bytes, and
deterministic host-metered operations. Admission requires a total evaluator under
that fuel model; the host contains panics. Fuel/input/result exhaustion or panic
records Gate-scoped `error(code=gate_evaluator_error)`, never a kind
result/verdict/route, so
an evaluator cannot wedge run wall-clock finality. Certification includes repeat
vectors with expected `decision-result-jcs-v1` verdict/reason/evidence hashes.
Gate-owned argv/shell execution is retired. Authored presence of any schema-1
Gate command field—including `command`, `commandArgv`, `commandShell`,
`commandCwd`, and explicitly empty values—is unsafe to normalize and yields the
stable validation finding `legacy_script_gate_requires_fenced_migration`.
Presence is determined from the source definition, not reconstructed from
non-empty decoded values. An inert legacy string or cwd-only entry receives the
same finding because silently dropping or interpreting it would be a migration.
It may also be `gate_not_routable` when it has no human or formation-judge route.

Mission preflight applies this fence only to the selected Mission's complete
possible executable subgraph, including every possible pass/fail branch and the
judge chain of each reachable Gate. Resume checks the same selected root in the
frozen run snapshot before appending `run_resumed`. A reachable legacy command
Gate returns `legacy_script_gate_requires_fenced_migration` before snapshot,
binding, ledger, event, evaluator, or process mutation. An unreachable legacy
Gate remains a whole-board validation error but does not block that Mission.
An isolated Formation root contains only the Formation and its canonical brief
seed; it does not traverse downstream board edges, so a legacy Gate elsewhere
does not block the isolated run.

The pre-Tool migration surface is one non-authorizing projection:

```text
LegacyScriptGateMigrationInspection {
  schema: 1
  boardId: string
  boardRev: integer
  boardETag: string
  gateId: string
  sourceMode: legacy_string | argv | shell | cwd_only | conflict | empty_present
  sourceFields: [command | commandArgv | commandShell | commandCwd] // sorted, presence only
  incomingEdgeIds: string[] // stable order
  outgoingEdgeIds: string[] // stable order
  code: "legacy_script_gate_requires_fenced_migration"
  targetKind: "tool_plus_pure_gate"
  ready: false
  applySupported: false
  requirements: [
    "host_owned_tool_profile",
    "pure_gate_evaluator_profile",
    "explicit_parameter_mapping",
    "port_media_compatibility",
    "atomic_cas_rewire"
  ]
}
```

This projection never contains raw command values, a resolved executable/cwd or
environment, generated Tool ids/ports, inferred parameters, or a suggested
profile. The board ETag/revision is the later compare-and-swap boundary, not
authorization to apply. API board inspection may expose this projection beside
the source definition, and `archon board validate --json` exposes the same typed
details while `archon board inspect --json` remains the authorized source view.

`ctx-ug7.8.1` owns non-executing Tool definitions, registry descriptors, and
board validation; `ctx-ug7.8` owns their certified host-private implementations
and runtime execution. `ctx-ug7.30` owns pure code-Gate profiles and the future
explicit apply. That apply may insert a Tool before the existing Gate only
after the caller selects certified Tool and pure-Gate profiles and the
writer validates parameters, port media, and
downstream payload compatibility atomically. It preserves the Gate id, title,
criterion, judge relationships, pass/fail outgoing edges, and existing layout;
only the new Tool receives placement. Because a Gate passes through the exact
Tool output it evaluates rather than the pre-Tool payload, arbitrary legacy
commands cannot be automatically relabelled or claimed equivalent. An
unprovable mapping fails without changing source bytes.

Likewise, schema-1 inline Formation verification is not safely normalizable: its
verdict lacks exact attempt/output identity and replay-safe revision finality.
Schema-2 validation and run preflight fail
`legacy_inline_verification_requires_migration`; schema-2 emits no
`verification_verdict` until `ctx-ug7.17` defines or retires the feature.

ADR-0006 graph typing is board schema 2. Schema-1 Formation inputs normalize in
memory to `kind=work`, `acceptedMediaTypes=["text/plain", "text/markdown",
"application/json"]`, `required=true`, `role=data`; outputs normalize to
`kind=work` with that same stable full initial media set. Fixed Mission `out`
accepts only `text/markdown`; Gate `in`/`pass` work ports use the full set. Gate `fail` is
`gate_feedback` and has no media set. Read/inspect does not rewrite. The first
structural schema-2 write migrates atomically and writes those defaults. A
legacy Gate fail edge into a
work input remains inspectable in degraded state but cannot validate or run
until explicitly rewired; annotated-work pushback is not preserved implicitly.
A safely normalizable schema-1 board may start without canonical mutation:
preflight writes a normalized schema-2 run snapshot and records source/snapshot
schema. Unsafe normalization rejects before `run_started`.

Schema-1 edges touching `gate:judge` normalize to `channel=judge` only when they
form one unambiguous linear Formation-only send/return chain. Ambiguous or
cross-used legacy judge paths load degraded and cannot validate or run until
explicitly repaired.

It does not contain pixel positions, viewport state, or run event history.

### Layout sidecar

A layout sidecar contains visual state:

- node positions;
- collapsed/expanded local UI flags that are safe to persist;
- hand-routed wire lanes;
- canvas grouping hints.

It does not define graph semantics.

Missing coordinates for a newly created element may be filled by the shared
connection-aware, bounded free-space, grid-snap placement heuristic. That
heuristic may write only the new element. Existing user coordinates and
hand-routed lanes are never changed by create, connect, validate, open, save,
run, replay, or reconnect. Full arrangement is a separate explicit UI/Archon
`arrange` mutation.

It also does not store selected run/node, inspector state, terminal targets,
open/focused Peek surfaces, Peek geometry, or terminal tiling. Those are
run-bound observability or user-local presentation state.

### Persona card

A persona card contains stable assignment identity:

- agent id;
- display name;
- tags/capabilities;
- harness variants;
- default cwd/root constraints;
- optional safety notes.

### Run binding authority and safe projection

Board slots store only staffing intent (`agentId` and optional `harness`). At
run preflight, each declared slot in the selected `runRoot` executable subgraph
gets a `SlotResolution`; only a wholly runnable preflight starts a run and writes
one host-private immutable `RunSlotBinding` plus one hash-linked safe projection
per slot, with persona/card path/hash and the exact runtime target described
above. Assignment,
resolution, frozen identity, and projected health are separate. Dispatch must
consume the frozen per-slot binding and must not re-resolve a mutable persona
card into a different target.

The private binding authority writes one `RunToolBinding` per reachable Tool with
its resolved exact profile version/content hash, normalized parameters/hash, and
effective-policy, determinism-policy, and execution-bundle hashes. The content-addressed bundle covers
executable/script/toolchain identity, argv template, cwd contract, normalized
non-secret allowlisted environment values, supervisor/fence policy, and limits;
a host path alone is insufficient. Run preflight rejects a reachable Tool before
`run_started` when the frozen supervisor/fence policy is unavailable.
Before `tool_dispatch`, the host validates every exact input and copies its
bytes into a writer-only content-addressed input set under the run-private data
root using no-follow reads, fsync, atomic rename, and final SHA-256 verification.
The resulting manifest fixes every input id, media type, size, hash, and sealed
object. For `Redact=true`, that input set is owned by the already-fsynced private
redaction obligation and never enters a Files root. The ledger then fsyncs a
`tool_dispatch` lease id that is unique within the run and one-to-one with the
Tool node attempt, with the manifest hash, those execution hashes, and either a
non-redacted private lease root or a redacted obligation id. Every spawn and
recovery rerun reopens only that sealed input set, revalidates its manifest, and
exposes it read-only; it never reopens the mutable source artifact. Missing or
mismatched materialization fails loud before spawn and never reruns. Each actual spawn first reserves and
fsyncs a private host-supervised descendant scope plus one exact
`ToolLaunchDeadlineAuthority`. That record derives `startedAt` and
`effectiveDeadlineAt` once from the frozen timeout policy before any public launch
or process exists. The writer then appends/fsyncs `tool_process_launch` with
stable launch/scope/deadline-authority ids and a monotonically increasing
generation before spawning. Generation starts at 1; launch/scope/deadline-authority
ids are unique, and the writer exact-matches run, lease, launch, node, attempt,
generation, scope, and deadline authority across the open lease, private records,
and launch event. The opaque
scope id is never a reusable raw PID; the private record holds the exact supervisor identity/start fingerprint. A private
scope/deadline reservation orphaned before its launch event is one atomic identity
set: recovery fences then reuses the exact pair or deletes both and
directory-fsyncs before replacement, without spawning.
Before any normal-completion, startup-recovery, or cancellation path first
terminates, seals, or waits on a recorded launch, the writer-private authority
fsyncs one exact `ToolQuiescenceBoundary`. Its cause matches the boundary kind;
its times and timeout policy exact-match the launch's already-durable deadline
authority. An unresolved boundary for that exact launch is
reused after every crash or restart, even when recovery continues work begun by
another boundary kind; neither start time nor deadline is recomputed from restart
time. If a crash occurs after public launch or spawn but before the boundary, recovery
reconstructs it only from that same deadline authority. At each decision boundary, the coordinator freezes the complete set of
launched Tool leases whose proof missed the persisted deadline. It sorts
candidates by `(effectiveDeadlineAt, dispatchSeq, toolLeaseId)`; the first alone selects
`run_failed(code=tool_process_not_quiescent).failureCause`, while every other
candidate is abandoned/private-cleanup-owned. Callback, map iteration, and
process-exit arrival order are never cause authority.
The Tool writes staged output; only after
process exit, sealing against new members, and proof that the whole descendant
scope is quiescent does the coordinator fsync it, promote atomically, and
directory-fsync. Failure records
terminal `run_failed(code=tool_process_not_quiescent)` with no promotion/result. Private cleanup
ownership continues after finality; later quiescence permits cleanup but no
promotion, result, rerun, or execution event. New Tool artifacts
are then registered inside the same appended/fsynced `tool_result` that closes
the lease/latest launch and carries the complete durable canonical port-output
projection map. The quiescent private scope record is then deleted and
directory-fsynced; success requires no remaining Tool scope record.
Each entry says whether an exact authoritative payload remains available or only
redacted evidence remains. Recovery can therefore project `node_output` without
rerun, but may route only an exact available payload. An open lease with no
recorded launch may discard an orphan scope and create generation 1 after normal
validation. Once a launch is recorded, its matching scope is mandatory: the
supervisor terminates and seals it against new members, then proves all
descendants quiescent by deadline before root cleanup. Its durable
sealed/quiescent tombstone remains until the next generation's fsynced launch or
a terminal non-rerun event, then is deleted and directory-fsynced. A missing
or ambiguous scope for a recorded launch, or failed quiescence
proof, records terminal `run_failed(code=tool_process_not_quiescent)`; cleanup/rerun is forbidden.
A successful rerun uses a new fsynced launch generation and consumes dispatch
and wall-clock limits while retaining its node attempt; `maxAttempts` counts only
new logical node attempts.
For each ordinary workflow Formation attempt, the writer parses a bounded closed
turn once and stores its exact durable payload/declared-output projections in a
hashed `slot-turn-result-jcs-v1` envelope inside `slot_result` before releasing
the target. Redact=true may instead retain hash-only projections plus an
ephemeral fresh-execution value. A closed capture with an exact terminal sentinel
but invalid output schema becomes the exact turn payload
`{availability="available",exact=true,payload={kind="error",
code="invalid_formation_outputs",message="Formation outputs do not match the declared ports",
retryable=true}}`, with `outputs={}` and no raw parser text. Timeout or
an unclosed output block has no result and retains unmatched/terminal-hold
authority. After the complete successful schedule or first non-`ok` turn and
output validation, the writer deterministically derives and appends/fsyncs one
`formation_result` from those turn envelopes and the immutable graph snapshot's
fixed formation-type rule. Solo has one terminal `solo` turn. Flow runs one
`flow-step` per persisted slot, last terminal. Peer runs one persisted-order
`peer-turn` per slot then first-slot terminal `peer-facilitator`. Orchestrated
runs controller `leader-plan`, one persisted-order `leader-worker` per
non-controller slot, then controller terminal `leader-agentic`. Every turn is a
coordinator-owned leased dispatch carrying closed `turnInputs={nodeStartedSeq,
priorTurnResults}`. `priorTurnResults` is an ordered array of exact
`{slotResultSeq,turnResultSha256}` values: empty for `solo`, first `flow-step`,
every `peer-turn`, and `leader-plan`; the immediate predecessor only for later
`flow-step`; every peer turn in persisted slot order for `peer-facilitator`;
`leader-plan` only for each worker; and plan followed by every worker in
persisted order for `leader-agentic`. Every phase also consumes the frozen node
inputs named by `nodeStartedSeq`. No agent directly mutates another bound tmux
target. Dynamic peer/leader scheduling requires a later authority-schema bump.

Only an `ok` terminal turn may contain all and only declared outputs; every
non-terminal or non-`ok` turn has `outputs={}`. All required `ok` turns map to
result `done`. The first `error` stops the schedule and maps to `failed`, copying
its exact engine-normalized non-routable error `turnPayload` to every declared
port. The first `needs-review` stops and maps to `needs-review`, copying exact
`{availability="available",exact=true,payload={kind="unavailable",
code="formation_needs_review",message="Formation requires review",retryable=true}}`
to every port.
`blocked` is not a Formation-result status; pre-result resource failure uses
`run_blocked` without a result. The deciding last contributing turn supplies the
report; artifact and diff ids are the stable first-seen union over the completed
prefix. The
result contains the complete durable safe canonical `outputs` map, `outputHashes`,
already-registered artifact ids, stable contributing `slot_result` sequences,
and `resultEncoding=formation-result-jcs-v1` plus its exact `resultSha256`.
`node_output` names that result sequence/hash and exact-matches its safe outputs.
Recovery can therefore materialize a missing `node_output` once without
reparsing mutable capture. For Redact=true, discarded raw output is not in this
result. Fresh execution keeps a paired exact value in process memory across safe
result/output/Gate/join/dispatch fsyncs until every scheduled intra-Formation
turn consumer and every taken-edge consumer has sent once or become durably
non-deliverable and no Gate/retry path retains it; it is then erased, and
cancellation/finality/process loss always discards it. Recovery
that later needs the value fails terminally under ADR-0005. Judge Formations retain `judge_result`;
Tools retain the atomic `tool_result` contract above.

The same private authority writes one `RunGateBinding` per reachable schema-2 code Gate.
Each pure evaluation exact-matches that binding and input/profile/parameter/
policy, evaluator-bundle, and determinism-policy hashes, then fsyncs one unique
`gate_kind_result` carrying those hashes before another kind starts; a completed
result is replay authority and is not recomputed. A crash before that result may
repeat only if the exact evaluator bundle and closed deterministic policy remain
available; an implementation/profile upgrade or missing bundle fails loud
without evaluation or routing. Every evaluation also resolves/revalidates exact
authoritative input bytes against `inputSha256`. Lost redacted input records
one bounded Gate-scoped contextual `error` naming gate/attempt/input, then terminal
`run_failed(code=redacted_input_unavailable)` with the source input sequence as
provenance and `failureCause={kind=none}` as required by ADR-0005. Non-redacted
artifact drift appends Gate-scoped `error(code=gate_input_integrity_failed)`, then
terminal `run_failed(code=gate_input_integrity_failed)` with
`failureCause={kind=error,errorSeq}` naming it. Both exactly dispose every open
attempt/slot/Tool authority. Neither substitutes evidence or emits
result/verdict/route.
Canonical cancel first fsyncs `run_cancel_requested`, whose exact open-attempt, open-slot,
and open-lease snapshots block new
dispatch/replay and makes the writer reject launches, results, outputs, and
routing except cancellation finality. It soft-interrupts only a frozen target
proven to host the exact unresolved dispatch/attempt and never kills tmux
sessions. Attempt snapshot/disposition items preserve node, kind, attempt,
start sequence, and latest durable phase/sequence, including
`gate_evaluating`/`waiting_human`. Slot snapshot/disposition items preserve dispatch, node, attempt, slot, binding,
target lease, target, sequence identity, exact Peek capability state, latest
steering generation, optional open-generation sequence, and any irreversible
revocation sequence. Cancellation stops new capability issuance/input, drains
the channel, closes an open generation, and durably revokes the capability as
reconciliation. Before sending any coordinator interrupt it fsyncs the unique
`slot_reconciliation_interrupt` exact-bound to the cancel request and one
`terminal-etx-v1` byte. The certified fence consumes that permit at most once;
a crash with no outcome is `send_uncertain` and never authorizes resend. Every
final slot disposition also records whether the host
target lease was durably released after quiescence,
retained as a non-authorizing terminal hold, or quarantined after
missing/conflicting private identity. Each Tool snapshot item preserves lease id, node id, attempt,
dispatch sequence, and optional latest launch id/generation/scope/sequence;
reconciliation and failure entries preserve that identity and add one typed
disposition. It cleans never-launched leases without execution, and
terminates, seals, and proves every launched Tool scope quiescent before root
cleanup and `run_canceled`. A failed proof instead records terminal
`run_failed(code=tool_process_not_quiescent)` through the failure lifecycle
below. After such final failure, the supervisor retains
private cleanup ownership. Later proof may remove or quarantine an unredacted
root; a redacted root must be sanitized/removed and cleanup fsynced before its
obligation is deleted. No path may promote, record a result, rerun, or append
another execution event. A later human decision for a disposed Gate is rejected.
Current main's `abort` route/verb remains a schema-1 command spelling. In the
accepted target, `cancel` is canonical and `abort` or `stop` is normalized at
the command boundary before hashing; aliases never produce another event or
lifecycle state.
Every terminal failure first fsyncs one
`run_failure_reconciliation_started`, freezes its cause/header and exact open
attempt/slot/Tool snapshots, and projects non-final `failing`. It stops ordinary
execution and Peek authority; reconciliation may only drain/revoke Peek, reuse
an earlier cancel-authorized interrupt, or create one exact failure-authorized
interrupt for a still-open slot that has no prior request. Missing interrupt
outcome remains `send_uncertain` and is never resent. If failure escalates from
cancel, `originCancelRequestSeq` names the original request and the frozen slot
snapshot preserves its interrupt state. `run_failed.failureReconciliationSeq`
must name that start event, byte-match its cause/header, and exactly dispose all
three snapshots. Recovery resumes the same reconciliation and never chooses a
new cause or returns the run to `canceling`.
Every execution-final event revokes all open node-attempt, slot, Tool, and Peek
authority. It first enumerates every run-owned occupying registry record,
non-occupying release receipt, issued/revoked Peek capability, input channel,
and steering generation. Finality rejects any issued capability, undrained
channel, open generation, or closure proof not bound to the latest generation
and unique revocation. Every result-closed dispatch requires an exact
`result_committed` receipt with certified turn-closure proof; every unmatched dispatch exact-matches a final slot
disposition backed by a certified `final_quiescent` receipt, terminal hold, or quarantine.
Missing/conflicting result-closed state may use a temporary quarantine only
until continuous closure-barrier proof or exact post-result pane-incarnation-
gone proof permits its exact receipt. Success permits no occupying record; receipts are
crash proof deleted only after final-event fsync. Cancellation reconciles all three snapshots;
`run_failed.failureCause` selects one exact slot, Tool lease, scoped error, or
no cause and resolves at most one open attempt as failed. Every other open
attempt/resource is abandoned. `relatedSeq` remains context/provenance only.
All are non-authorizing; Tools remain private-cleanup-owned.
Final projection then covers every frozen selected-root node that never opened
an attempt: `canceled` on cancellation, `abandoned` on failure, or `not_run` on
valid success only when the ledger proves it unreached/untaken. A delivered input
on a taken path makes success invalid. Earlier `node_waiting` readiness counts
remain inspection evidence and never leave a final node actively waiting.

For `Redact=true`, a private pending-redaction obligation owns and is fsynced for
the exact lease root before public `tool_dispatch`, which stores only its opaque
id. The initial state is `pending(generation=1)`, and the writer accepts a
redacted launch only when its generation matches the pending obligation. A private obligation
orphaned before dispatch is cleaned and deleted without Tool execution. Raw
targets are sanitized/removed before the private
obligation is fsynced from `pending(N)` to `cleaned(N)` with its locator retained.
Only a cleaned generation matching the latest launch may commit the redaction-safe `tool_result` and its policy-safe
registrations. The entry is deleted and directory-fsynced after the result;
after recovery cleans generation N, it must fsync `cleaned(N)` to
`pending(N+1)` before recording or spawning the rerun. A crash before that
launch is a safe prepared generation: recovery cleans idempotently, fences any
orphan next-generation scope without spawning, retains the pending obligation,
and reuses N+1 only after validation. Recovery retains an exact
locator for every open lease, and success requires no
remaining redaction or Tool-scope entry. Its `outputs` map contains only
exact available or hash-only redacted `PayloadProjection` values. Sanitized
non-exact text, refs, and summaries remain solely in separate bounded display or
artifact evidence fields such as `tool_result.displayEvidence` and
`tool_result.artifacts`; they never become port output or routing authority.
Discarded authoritative values cannot be reconstructed or rerun from hashes.

### Run ledger

Run ledgers are append-only NDJSON. Event payloads are versioned and should be
sufficient to reconstruct projected state, immutable attempt inputs, and
recovery handles. Every schema-2 ledger event carries its origin `writerFence`.
Valid event fences are monotonic non-decreasing allocated owner epochs; prior
prefixes remain valid after takeover. A lower fence after a higher event, an
unallocated fence, or a new append/effect not using the current fence is invalid.
The four client-command effects also carry their `commandId` and
`commandPayloadSha256`, with start using `admissionCommandId` in `run_started`.
`run_started` and `run_activated` also bind the retained immutable policy
revision/hash used for their distinct admission and activation decisions.
Every duplicated command-derived event field, including envelope actor, run or
workspace id, reason, mode/verdict, and precondition sequence, exact-matches the
stored hash-bound payload.
`node_output` records display text plus durable
`outputs[portId]` `PayloadProjection` values for Mission, Formation, and
accepted-target Tool nodes. Its optional `reportArtifactId` and stable-order
`artifactIds`/`diffArtifactIds` contain only already-registered ids. Schema-2
Formation output additionally exact-matches one prior `formation_result` by
`formationResultSeq` and `resultSha256`; Tool output similarly derives from its
closed Tool result. Schema-2
`run_succeeded` likewise carries only optional `summaryArtifactId` plus
stable-order `outputArtifactIds`. Public projections hydrate all of them through
the latest `ArtifactProjection`. Gate verdicts and routes remain separate events.

`runRoot` is either a Mission or an isolated Formation. A Mission root includes
its reachable graph and judge chains. Its bounded objective uses
`mission-objective-utf8-v1`: UTF-8 with BOM rejected, CRLF/CR normalized to LF,
and no other whitespace or newline added/removed. It is one
`mediaType=text/markdown` work payload; every first destination must accept that
media or preflight fails `mission_objective_media_incompatible` before
`run_started`. Mission `node_output.outputs[out]` and unchanged downstream
deliveries use the classified `RootDerivedPayloadProjection` matching the same
run's root input. An isolated Formation must have a non-empty
brief and exactly one required `data` input accepting `work` and
`application/json`. Preflight freezes `formation-brief-jcs-v1`: RFC 8785
canonical UTF-8 JSON over exactly `{goal, beadId, files, links}`, normalizing
missing scalar/array fields to `""`/`[]`, preserving array order, and adding no
trailing newline. Those exact bytes form the synthetic `run_seed` work payload,
use `mediaType=application/json`, determine `seedSha256`, and use the matching
classified root-derived projection. Incoming board edges,
optional inputs, `retry_control`, and downstream edges are outside the isolated
root. Any other required-input shape rejects before `run_started`. Preflight
never binds unrelated board nodes.

In the accepted target, new runs use event schema 2. A ledger without a schema
declaration is projected as schema 1 with its original compatibility semantics;
schemas never mix within one ledger and old events are never reinterpreted as
typed feedback/pass-through. Schema-1 runs are inspect-only under the schema-2
engine; resume returns `legacy_run_requires_new_run`.
Schema 2 includes the ADR-0007 command, workspace, fence, Formation-result,
root-projection, and authored-config-manifest semantics before its first
admission. Schema-number recognition alone cannot enable it: admission waits for
the complete safe projector/coordinator and a certified rollback set in which
every runnable binary honors the immutable bootstrap plus mutable workspace-
authority guard. Pre-guard binaries are prohibited runtime rollback targets. A later field, record, or event that changes admission, identity,
dispatch, result acceptance, routing, cleanup, cancellation, or finality requires
an authority-schema bump. An older or unsupported reader is inspection-only and
does not allocate a fence, clean, quarantine, or otherwise mutate. A registered projection-only private extension may be ignored
only when it is redaction-classified, excluded from every public projection, and
cannot affect status, actions, bindings, artifacts, or execution.
The mutable `WorkspaceAuthority.authoritySchema` is a monotonic high-water mark.
An upgrade holds `owner.lock`, validates the current fence, then atomically
advances/fsyncs that mark before any new-schema command, run, event, or private
record. It never decreases; a crash after advancement is unavailable to an older
reader, not downgraded. New-schema work starts a new run and older ledgers retain
their original semantics.

Current executor ingress and schema-1 `FormationOutputPayload` values may carry
inline `text`, `ref`, `reportRef`, and `artifactRef`. Those compatibility fields
terminate at writer-private normalization: file-backed evidence is validated and
registered before schema-2 append, and no schema-2 ledger/API field carries the
bare ref. The accepted canonical port payload is:

```ts
RootInputProjection =
  | {
      classification: "authored_config"
      sourceKind: "mission_objective"
      encoding: "mission-objective-utf8-v1"
      mediaType: "text/markdown"
      sha256: string
      text: string
    }
  | {
      classification: "authored_config"
      sourceKind: "formation_brief"
      encoding: "formation-brief-jcs-v1"
      mediaType: "application/json"
      sha256: string
      text: string
    }

RunAuthoredConfigManifestEntry =
  | {
      classification: "authored_config"
      sourceKind: "mission_objective"
      nodeId: string
      encoding: "mission-objective-utf8-v1"
      mediaType: "text/markdown"
      sha256: string
    }
  | {
      classification: "authored_config"
      sourceKind: "formation_brief"
      nodeId: string
      encoding: "formation-brief-jcs-v1"
      mediaType: "application/json"
      sha256: string
    }
  | {
      classification: "authored_config"
      sourceKind: "gate_criterion"
      nodeId: string
      encoding: "gate-criterion-utf8-v1"
      mediaType: "text/plain"
      sha256: string
    }

RunAuthoredConfigManifest = RunAuthoredConfigManifestEntry[]
// Embedded in the graph snapshot in stable (sourceKind,nodeId) order.

RootDerivedPayloadProjection =
  | {
      availability: "available"
      exact: true
      classification: "authored_config"
      sourceKind: "mission_objective"
      encoding: "mission-objective-utf8-v1"
      mediaType: "text/markdown"
      sha256: string
      payload: { kind: "work", mediaType: "text/markdown", text: string }
    }
  | {
      availability: "available"
      exact: true
      classification: "authored_config"
      sourceKind: "formation_brief"
      encoding: "formation-brief-jcs-v1"
      mediaType: "application/json"
      sha256: string
      payload: { kind: "work", mediaType: "application/json", text: string }
    }

GateCriterionProjection = {
  classification: "authored_config"
  sourceKind: "gate_criterion"
  encoding: "gate-criterion-utf8-v1"
  mediaType: "text/plain"
  sha256: string
  text: string
}

HumanPromptProjection = {
  classification: "fixed_system"
  sourceKind: "human_prompt"
  templateId: "gate-human-verdict-v1"
}

HumanChoiceProjections = {
  pass: {
    classification: "fixed_system"
    sourceKind: "human_choice"
    templateId: "gate-human-pass-v1"
  }
  fail: {
    classification: "fixed_system"
    sourceKind: "human_choice"
    templateId: "gate-human-fail-v1"
  }
}

PortPayload =
  | {
      kind: "work"
      mediaType: "text/plain" | "text/markdown" | "application/json"
      text?: string
      artifact?: SafeArtifactRef
    }
  | { kind: "gate_feedback", feedback: GateFeedback }
  | { kind: "unavailable", code: string, message: string, retryable: boolean }
  | { kind: "error", code: string, message: string, retryable: boolean }

RunInputRef =
  | {
      inputId: string
      sourceKind: "edge"
      runId: string
      originEdgeId: string
      deliveryEdgeId: string
      sourceNodeId: string
      sourcePortId: string
      sourceOutputSeq: number
      sourceAttempt: number
      toNodeId: string
      toPortId: string
      payloadProjection: PayloadProjection
    }
  | {
      inputId: string
      sourceKind: "run_seed"
      runId: string
      seedId: string
      seedEncoding: "formation-brief-jcs-v1"
      seedMediaType: "application/json"
      seedSha256: string
      toNodeId: string
      toPortId: string
      payloadProjection: PayloadProjection
    }

PayloadProjection =
  | { availability: "available", exact: true, payload: PortPayload }
  | RootDerivedPayloadProjection
  | {
      availability: "redacted"
      exact: false
      code: "redacted_payload_unavailable"
      payloadSha256: string
    }

ExecutionInputRef {
  durableInputRef: RunInputRef
  authoritativePayload: PortPayload // in-memory only; never ledger/API
}

ExecutionOutput {
  durableOutput: PayloadProjection
  authoritativePayload: PortPayload // in-memory only when projection is redacted
}

GateFeedback {
  feedbackId: string
  revisionCycleId?: string
  gateId: string
  verdict: "fail"
  evaluatedInput: EvaluatedInputPointer
  reason: string
  evidence: EvidenceRef[]
  gateSeq: number
  gateAttempt: number
}

EvaluatedInputPointer {
  inputId: string
  gateInputSeq: number // matching gate_evaluating.inputRef; no payload projection
}

SafeArtifactRef {
  artifactId: string
  rootId: string
  ref: string // relative to the authorized root
  mediaType: string
  sizeBytes: number
  sha256: string
}

ArtifactProjection =
  | {
      artifactId: string
      availability: "available"
      name: string
      artifact: SafeArtifactRef
    }
  | {
      artifactId: string
      availability: "unavailable" | "redacted" | "expired"
      name: string
      errorCode: string
    }

ArtifactSource =
  | { kind: "slot", dispatchId: string, nodeId: string, slotId: string }
  | { kind: "tool", toolLeaseId: string, nodeId: string } // derived from tool_result only
  | { kind: "gate", gateId: string, gateAttempt: number }
  | { kind: "system", sourceId: string }

EvidenceRef =
  | { kind: "artifact", artifactId: string }
  | { kind: "ledger", seq: number }
  | { kind: "text", text: string } // bounded and redaction-safe

JudgeResult {
  verdict: "pass" | "fail"
  reason: string
  evidence: EvidenceRef[]
}

JudgeContext {
  gateId: string
  gateAttempt: number
  criterion: string
  kinds: string[]
  evaluatedInput: ExecutionInputRef
  durableEvaluatedInput: RunInputRef
  priorResults: JudgeResult[]
}
```

The three fixed-system human template ids above are immutable registry
identities. Any text change requires a new versioned id, and unknown ids fail
before append/projection.

The two synthesized ordinary-Formation projections are closed constants.
`formationNeedsReviewProjection` is exactly
`{availability="available",exact=true,payload={kind="unavailable",
code="formation_needs_review",message="Formation requires review",
retryable=true}}`. `invalidFormationOutputsProjection` is exactly
`{availability="available",exact=true,payload={kind="error",
code="invalid_formation_outputs",
message="Formation outputs do not match the declared ports",retryable=true}}`.
No parser/adapter text, locale, or alternate message enters their hashes. Their
`retryable=true` permits only the existing explicit whole-producer retry
selection after quiescence; neither routes an edge or retries automatically.

The normalized private graph snapshot embeds `RunAuthoredConfigManifest`, and
`graphSnapshotSha256` covers both. Each entry classifies and hashes exactly one
Mission objective, whole Formation brief, or Gate criterion in that snapshot.
Missing/extra entries, field/hash/role/encoding/media mismatch, and every other
source kind are invalid. Root-input, root-derived payload, and criterion
projections also exact-match their corresponding manifest entry.

`run_started.rootInputProjection` is this exact closed shape with no unknown
keys. Its `text` contains the same canonical UTF-8 Mission-objective or
Formation-brief bytes whose hash is recorded in `sha256`; it is not a
`PayloadProjection` and never contains a runtime/external value. Its only
permitted durable payload copy is `RootDerivedPayloadProjection`, whose source
role, encoding, media type, hash, and payload text must exact-match it. The
generic available `PayloadProjection` forbids classification/root-derived keys;
an unclassified root-byte copy is invalid. Root-derived projection is allowed
only for Mission fixed `out`, unchanged deliveries/Gate passes from that output,
and isolated Formation `run_seed`.

For an available projection, top-level `artifactId` must equal
`artifact.artifactId`.

Formation dispatch `turnInputs` is exactly
`{nodeStartedSeq,priorTurnResults}` with no unknown keys. `nodeStartedSeq` names
the same attempt's unique `node_started` and frozen ordered input refs.
`priorTurnResults` contains exact ordered `{slotResultSeq,turnResultSha256}`
entries selected by the fixed phase rule; missing, extra, duplicate, reordered,
or hash-mismatched entries reject before dispatch.

`slot-turn-result-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{turnKey,phase,status,turnPayload,outputs,reportArtifactId,artifactIds,
diffArtifactIds}` with no unknown keys or trailing newline. `turnKey`, `phase`,
`status`, and `turnPayload` are required; phase exact-matches the dispatch.
Missing optional `reportArtifactId` normalizes to `""`, missing `outputs` to `{}`,
and missing artifact arrays to `[]`; `turnPayload` and every output use durable safe `PayloadProjection`,
output keys are stable declared port ids, and arrays preserve stable order. The
`slot_result` carries this envelope plus
`turnResultEncoding=slot-turn-result-jcs-v1`; `turnResultSha256` hashes those
exact canonical bytes. The fixed formation-type schedule consumes only these
ordered immutable envelopes after a crash;
if a required projection is redacted, recovery fails
`redacted_input_unavailable` rather than reopening capture.

`formation-result-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{status, outputs, outputHashes, reportArtifactId, artifactIds, diffArtifactIds,
contributingSlotResultSeqs}` with no unknown keys or trailing newline. Missing
optional id/array values normalize to `""`/`[]`; output object keys are the
stable declared port ids, `outputHashes` has exactly those same keys, arrays
preserve their declared stable order, and each output is a durable safe
`PayloadProjection`. Status is exactly `done`, `needs-review`, or `failed` under
the schedule mapping above. Each `outputHashes[portId]` is the SHA-256 of RFC 8785
canonical UTF-8 JSON for that exact closed `outputs[portId]` projection, with no
trailing newline. `resultSha256` hashes those exact
bytes. One unique `formation_result` per ordinary Formation attempt carries the
encoding/hash; its `node_output` must exact-match it.

`decision-result-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{verdict, reason, evidence}` with no unknown keys or trailing newline. Evidence
array order is preserved and each member uses the closed `EvidenceRef` variant
shape. `resultSha256` hashes those exact bytes for both code-Gate and judge
results. `judge-context-jcs-v1` is the same canonicalization over exactly
`{gateId, gateAttempt, criterion, kinds, evaluatedInput,
durableEvaluatedInput, priorResults}`; kinds use fixed Gate evaluation order,
prior results use judge-chain order, and nested evidence order is preserved.
`judgeContextSha256` hashes those exact bytes. The encoding ids are frozen in run
authority and recorded beside, not inside, the canonical objects so builds cannot
substitute map serialization.

A `work` payload requires exactly one of `text` or `artifact`; both use the one
declared `mediaType`. A `work` port declares a non-empty subset of the supported
media types, and delivery outside that set fails before attempt start. Full JSON
Schema remains deferred. The port's declared kind and media set govern
successful routable delivery. `unavailable`
and `error` may terminate any declared output as non-delivered system outcomes;
they do not count as an incompatible successful payload.

Mission, Formation, and Tool outputs produce only `work`, and Tool inputs accept
only `work`. Formation inputs may accept `work` or `gate_feedback` as declared,
but only the fixed Gate `fail` port produces `gate_feedback`. Shared structural
read/write validation and run preflight reject every other feedback producer.

`SafeArtifactRef` records a descriptor that was validated when attached; its
shape cannot prove a mutable file is still present. `ArtifactProjection` exposes
it only while the latest durable artifact observation is `available`;
unavailable/redacted/expired projections never carry a readable ref. File access
uses one root-relative no-follow open, validates regular identity/size/media/hash
on that handle, renders only the same verified bytes/handle without reopening the
path, and fails unavailable immediately. A reconciler
appends `artifact_observed` before durable projection changes; the projector
never consults the filesystem. Current bare `ref`, `reportRef`, `summaryRef`,
`outputRefs`, `artifactRef`, and diff fields are schema-1 or writer-private
compatibility inputs only. They must normalize into a prior durable registration
before any schema-2 append and are invalid in schema-2 ledger/API fields; the
projector never invents an artifact id from a bare ref.

Every projection variant keeps one stable `artifactId`, so
`artifact_observed` can update availability without changing identity. An
available observation must include a newly validated `SafeArtifactRef` with the
same id. The first available registration or observation establishes that id's
immutable root/ref/media type/size/hash descriptor; later available observations
must match it exactly. Changed content or location requires a new id and
registration.

Exactly one durable source registers each id and its initial projection. Slot,
Gate, and system artifacts use a source-discriminated fsynced
`artifact_attached` before any later reference. New Tool artifacts instead use
`tool_result.artifactRegistrations`, which registers them atomically with the
lease-closing result. An open Tool lease has no public registrations, so cleanup
and identical-hash rerun cannot collide with an artifact id. Other embedded
projections are non-authoritative and must match the latest
registered/observed projection at their event sequence; later observations do
not rewrite history. A duplicate registration across either source is a ledger
error.
Unavailable/redacted/expired observations omit the descriptor and require a
stable error code. An observation for an unknown artifact id is invalid. Binding
and artifact observation events may append after execution finality, but they are
inspection-only: they cannot change run/node outcomes, open an epoch, or
authorize dispatch.

Evidence stores only `{kind="artifact", artifactId}`. Canonical registration
descriptors remain in the writer-only ledger; no later result, Gate evidence, or
display field embeds a readable ref as authority. Run/event/stream APIs are
sanitized projections rather than raw ledger reads: every artifact occurrence
is hydrated through the latest `ArtifactProjection`. Once an observation marks
an artifact unavailable, redacted, or expired, historical events and File Peek
expose only its id and metadata-only projection; an older descriptor cannot be
used to recover access.

Run-bound live Peek exact-matches binding, dispatch, target lease, fingerprint,
and active unmatched occupancy while the run is non-final and cancel/failure
reconciliation has not begun. A terminal hold is non-authorizing run evidence,
not interactive control. Quarantine is ambiguous.
After a release receipt, the stable opaque handle may already serve newer work,
so the old attempt shows captured history or `pane_moved_on`; Open current
session is a separately labeled non-run view.

While that exact active occupancy remains valid, Peek is a full
interactive user attach and may send literal input, including control
characters, to steer or interrupt the agent. A CHROTE-issued capability must
exact-match run, dispatch, target lease, binding, and fingerprint. It must also
exact-match the latest fsynced issued sequence and generation. A later issuance
requires prior clients/input drained and atomically invalidates every older
token/generation; a superseded route reaching the target boundary is foreign.
Before the
first bytes are forwarded, `slot_steering_started` fsyncs the next generation
without raw input; result closure is forbidden until `slot_steering_ended` closes
it. `TargetTurnClosureProof` binds the latest closed generation and comes from a
certified adapter channel that user-writable pane bytes cannot forge. Later
input invalidates earlier proof. Safe projection exposes only generation/open
state and `operatorInfluenced`, never keystrokes. Workflow sends, retries,
coordinator interrupts, and lifecycle transitions remain coordinator-only.

Shared Terminal attach paths reject ordinary attachment to an occupied target.
A certified writer-private monitor is armed durably before occupancy fsync and
accounts for every later attach, detach, target-selection, resize/reflow,
history, pane-lifecycle/topology/other mutation, and input-capable tmux
command/control route affecting the pane. Registered Peek
input qualifies only through the steering-generation gate. Foreign client/input
or lost monitor
continuity durably latches the dispatch non-authorizing, revokes/drains Peek,
and forbids result/ordinary release until exact non-authorizing quiescence
reconciliation; raw foreign input is never relabeled as a steering generation.
Because lost history cannot be reconstructed from current-client absence, a
foreign-input/attachment or lost-audit latch releases only after exact proof
that the pane incarnation is gone.
Result closure, cancel/failure reconciliation, and finality stop capability
issuance, drain input, close any open generation, and fsync unique irreversible
capability revocation. Closure binds that sequence and
`ClientAttachmentAuditProof`; no run-bound capability survives finality.

The active dispatch freezes the terminal grid stored in
`PaneHistoryBaselineV1`. Peek tile movement/resize changes the browser viewport
only and sends no tmux resize/`SIGWINCH`; actual reflow invalidates the baseline.
Closing Peek never creates, kills, or rebinds tmux state. After coordinator
restart, input remains suspended until the capability exact-matches recovered
occupancy under the current fence.

Durable `RunInputRef` supplies canonical provenance. `sourceKind=edge` records
source node/port, output sequence/attempt, origin edge, and current delivery
edge/destination. `sourceKind=run_seed` records the isolated Formation's stable
seed id, canonical encoding/media type, and exact frozen-brief byte hash without
inventing an edge or producer. Its
`payloadProjection` must be the classified root-derived variant exact-matching
`run_started.rootInputProjection`; a generic unclassified exact copy is invalid.
A redacted projection carries no payload and cannot be used as an
input; hashes and summaries are evidence only. Fresh execution may pair the ref
with an in-memory `ExecutionInputRef`, which is never persisted. A Gate pass
preserves the source identity and durable projection while recording the pass
edge as the new delivery traversal; fresh redacted execution also forwards the
same exact live authoritative payload. A fail edge carries
`gate_feedback` separately: its input ref uses Gate/`fail` as source node/port,
the `gate_verdict` seq as source output seq, and the Gate attempt as source
attempt, while the feedback object retains only `{inputId, gateInputSeq}` for
the evaluated work. That pointer never embeds its `RunInputRef`, payload
projection, payload, or artifact. Judge output
becomes verdict evidence only. One failed Gate evaluation creates one stable
feedback object; zero or more fail-edge traversals reference it. Pushback is the
narrow exception: the direct evaluated source must be a Formation whose entire
connected workflow-output frontier is the single edge into that Gate, and the
Gate fail frontier must be the single edge back to that Formation's
`retry_control`. Matching feedback starts source attempt N+1 only while its
frozen authoritative inputs remain live or durably exact; otherwise redacted
replay fails terminally. Its revised output
opens Gate attempt N+1 linked by `revisionCycleId`, feedback id, prior Gate seq,
and new source attempt. No fail fan-out, side-output delivery, downstream replay,
or non-source pushback is permitted.

The reserved judge return accepts exactly one bounded `JudgeResult`. Missing,
malformed, or multiple returns append `judge_attempt_failed` with
`code=invalid_judge_result`, complete the judge Formation attempt as failed,
block the Gate, and route neither pass nor fail.

Judge-channel edges order one linear Formation-only chain and carry bounded
`JudgeContext`/`JudgeResult` control, never `PortPayload`. Chain endpoints cannot
also serve workflow edges; branches, JOINs, side entry/exit, repeated nodes, and
Tool participation are invalid. A complete judge channel is required if and only
if `gate.kinds` contains `formation`. Each accepted member result is fsynced as
one `judge_result` before the next judge dispatch, keyed by Gate/judge attempts
and chain index with context/result hashes and prior-result sequences. Exactly
one `judge_result` or `judge_attempt_failed` completes each judge key; a
conflicting duplicate is a loud ledger error. Replay rebuilds durable prior
results from accepted events and never reruns or reparses a completed judge
capture. The next exact JudgeContext dispatch additionally requires the
evaluated input to remain live or durably exact; if redaction discarded it,
recovery appends terminal `run_failed(code=redacted_input_unavailable)`. A judge Formation's
`node_started` freezes context hash/prior-result sequences in place of workflow
input refs, and either completion event closes the attempt without ordinary
`node_output`. A failed completion is followed by `run_blocked` for the judge and
Gate only after new Gate/dependent dispatch is stopped and already in-flight or
independent work settles. It records `reason=invalid_judge_result`, no open
dispatches/retry targets, `blockScope=gate`, both judge and Gate ids,
`resumeAllowed=false`,
`resumePolicy=new_run_required`, and no `nextEpoch`; this contract does not retry
a judge inside the same run. If quiescence produces an execution-final event,
finality takes precedence and no later block is appended.

An executable Gate's `kinds` array is a non-empty, duplicate-free subset of
`code`, `formation`, and `human`; empty, duplicate, and unknown values reject
before `run_started`. Gate kinds are an all-of set in canonical `code`,
`formation`, `human` order. The first durable fail short-circuits later kinds as `not_run`. Human is
requested only after prior results pass; the request projects `waiting_human`
and records their exact result sequences. Its matching decision continues the
same attempt. One aggregate `gate_verdict` records all declared per-kind states
and alone authorizes pass/fail routing.
The request has no independent timeout or default verdict; only its exact
decision, cancellation, or terminal run wall-clock exhaustion ends the wait.
A code-evaluator boundary failure instead records Gate-scoped
`error(code=gate_evaluator_error)` and, after independent work settles, a non-resumable
`new_run_required` Gate block with only that Gate id, empty open
dispatches/retry targets, and no next epoch. It produces no aggregate verdict or
route; execution finality reached during quiescence takes precedence.
An unwired aggregate FAIL records `routePort=fail`, `routedEdges=[]`, then after
quiescence records a non-resumable Gate-scoped `unwired_gate_fail` block with no
blocked node, open dispatches, retry targets, or next epoch. The Gate retains
the FAIL verdict; the block overlays the attempt already closed by that verdict
without closing it twice, while its completed upstream producer remains
completed.

At `node_started`, required execution refs and already-present optional execution
refs freeze; the ledger stores only their durable `RunInputRef` projections. Late
optional data records `node_input_ignored`; only matching feedback on a persisted
`retry_control` role and matching exact evaluated source attempt may create an
in-graph bounded next attempt. `unavailable` and `error` are unsuccessful attempt
outcomes and never auto-retry or route through ordinary work inputs. An attempt
finalizes all declared outputs before delivery; if one fails, it records no
delivered edges, including from successful sibling ports. Independent/in-flight
branches may finish. At quiescence, latest unresolved producer
failures are aggregated per attempt: every unsuccessful output must be
retryable, one non-retryable sibling makes the attempt terminal, and successful
siblings remain non-delivered. Candidates are ordered by
`(minimum outcomeSeq, nodeId)`. If any is non-retryable, the first such candidate
selects terminal failure; otherwise the first retryable candidate alone blocks
with `blockScope=node`, its producer id,
`resumePolicy=retry_failed_producer`, empty open dispatches, and one
whole-producer retry target; other candidates remain closed durable failures and
prevent success.
Explicit operator resume opens a new epoch and starts attempt N+1 from
that target's frozen input refs and unchanged slot/Tool bindings with a new
dispatch/lease. Prior completed siblings are not rerun. A target with prior
delivery or unavailable authoritative input is not resumable. At the next
quiescence the set is recomputed; another candidate requires another explicit
resume. The first non-retryable candidate by the same order ends the run as
`run_failed(code=declared_output_failed)` with provenance-only `relatedSeq` and
`failureCause={kind=none}` because its attempt is already closed. This phase
does not infer general error ports, multi-target resume, or selective replay.
`run_blocked.resumePolicy` is a closed union. `retry_failed_producer` has
`openDispatches=[]`, one whole-producer retry target, and a next epoch;
`reattach_only` has a non-empty unchanged open-dispatch set, no retry target, and
a next epoch but sends no prompt; `new_run_required` has no retry target or next
epoch and forbids resume. Its open-dispatch set is either empty or the frozen
unmatched set at that block. After reattach it may be only a strict remaining
subset of the preceding set; its late authority has been revoked.
Quiescence uses one closed precedence: cancel/finality, complete-set unmatched-
dispatch recovery, outstanding human wait, one non-resumable semantic blocker
selected by stable causal sequence/reason/id, then one retryable producer. Other
candidates remain durable evidence and cannot race another block into the
ledger.
Exhausted immutable max-dispatch, max-attempt, or wall-clock limits record a
scoped stable limit error followed by terminal
`run_failed(code=run_limit_exhausted)` with that error as `failureCause` and
exact attempt/slot/Tool dispositions. Late
results/output/routing are rejected; Tool scopes remain private-cleanup-owned.
Continuing requires a new run with a new frozen limit snapshot.

Inline text is bounded. Artifact refs resolve through a run-scoped, read-only
resolver that maps `rootId` plus relative `ref` and rechecks the exact authorized
root, regular-file/no-symlink rules, media allowlist, size/hash, availability, and
ADR-0005 redaction on one root-relative no-follow handle before the same verified
bytes/handle reaches
the existing Files/File Peek viewer. `tmux://`, `ledger://`, redacted, expired,
or unavailable evidence is never treated as a browser-authoritative host path.

Representative event kinds:

```text
run_started
run_activated
run_resumed
run_cancel_requested
run_failure_reconciliation_started
node_waiting
node_input_ignored
node_started
slot_binding_observed
slot_dispatch
slot_peek_capability_issued
slot_steering_started
slot_steering_ended
slot_peek_capability_revoked
slot_reconciliation_interrupt
slot_reconciliation_interrupt_outcome
slot_result
formation_result
tool_dispatch
tool_process_launch
tool_result
node_output
gate_evaluating
gate_kind_result
judge_result
judge_attempt_failed
gate_verdict
artifact_attached
artifact_observed
escalation_raised
human_input_requested
human_verdict_recorded
error
run_blocked
run_canceled
run_failed
run_succeeded
```

Schema-1 `verification_verdict` remains legacy inspection evidence only and is
not accepted in a schema-2 ledger.

### Redacted-run evidence and recovery

`Redact=true` distinguishes an execution-authoritative value from durable run
evidence. A fresh attempt may hold and route the authoritative raw value only in
live ephemeral memory; the value is not persisted for replay. Redaction markers,
hashes, display summaries, and safe ref metadata are evidence and never valid
substitutes for a graph input.

The durable redaction boundary covers runtime/external ledger fields, composed
prompts, verifier and per-kind feedback, captures, reports, artifact contents,
output refs and their targets, derived evidence, and errors. Exact typed
`authored_config` copied from the board (Mission objective, Formation brief, or
Gate criterion) is the sole configuration exception. Human prompt and PASS/FAIL
labels use closed fixed-system templates; unclassified/dynamic strings are
covered. Ref metadata may persist only when its target has
already been sanitized or replaced and resolves inside an authorized root. Raw
executor capture is cleanup-owned transient material, not accepted evidence.
Before any persistent path can contain raw bytes, a durable pending-redaction
record is written and fsynced with ownership of that exact target. Its internal
cleanup locator is the only metadata allowed to point temporarily at an
unsanitized target and is never exposed as an output ref or graph input.
Recovery replaces each target idempotently and preserves valid redacted evidence
byte-for-byte. `run_succeeded` is invalid while any run-owned target remains
pending.

Recovery may reattach exact unresolved attempts when their qualified session
targets are proven and no prompt is resent. It may also finish pending cleanup
or reconcile work that needs no discarded raw value. At quiescence, the first
`reattach_only` block freezes every unmatched dispatch in stable order, using
node scope only for one node and run scope otherwise. It rejects results/output/
routing until exact resume. That one bounded no-prompt epoch may close exact
individual results; a second block contains only the still-unmatched subset,
blocks non-resumably, and remains so until cancel. A separately started run is
independent and does not resume, supersede, or close any old lease. No same-run
action supersedes or redispatches an unmatched lease. A pending cleanup target or
other durable evidence cannot reconstruct graph input. If a new dispatch
requires an authoritative value that redaction discarded, failure records this
two-event lifecycle (the start sequence is shown symbolically):

```text
type: run_failure_reconciliation_started
data.originCancelRequestSeq: 0
data.code: redacted_input_unavailable
data.reason: redacted_input_unavailable
data.unrecoverable: true
data.relatedSeq: <source event sequence whose raw value was required>
data.failureCause: {kind: none}
data.openNodeAttempts: <exact frozen open node attempts, possibly []>
data.openSlotDispatches: <exact frozen open slot dispatches, possibly []>
data.openToolLeases: <exact frozen open Tool leases, possibly []>
data.recordedBeforeReconciliation: true

type: run_failed
data.failureReconciliationSeq: <sequence of the start event above>
data.code: redacted_input_unavailable
data.reason: redacted_input_unavailable
data.unrecoverable: true
data.relatedSeq: <source event sequence whose raw value was required>
data.failureCause: {kind: none}
data.nodeAttemptDispositions: <exact dispositions for the frozen open node attempts>
data.slotDispatchDispositions: <exact dispositions for the frozen open slot dispatches>
data.toolLeaseDispositions: <exact dispositions for the frozen open Tool leases>
data.final: true
```

That run cannot resume and cannot increment its epoch. A retry creates a new run
with newly supplied authoritative input. See ADR-0005 for the decision and
enforcement boundary.

## Revision and concurrency

- APIs should expose revision/etag values for mutable resources.
- Clients must send the revision they edited from.
- Conflicting writes fail loudly and require reload/merge.
- The shared writer owns normalization and validation.

## Id rules

- Ids are stable, lowercase, and safe for filenames or explicit encoded paths.
- Accepted target ids include a distinct `tool_...` namespace; Tool ids never
  masquerade as Gates or Formations.
- Display names are not ids.
- Generated ids should include enough entropy to avoid collisions without hiding
  their noun type.
- Existing ids should not change during rename operations.
