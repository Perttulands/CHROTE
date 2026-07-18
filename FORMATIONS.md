---
type: spec
status: active
authority: source-of-truth
workspace: chrote
enforced_by: scripts/doc-lint.py
---

# CHROTE Formations Spec

Status: **Active core source of truth**.

Formations is CHROTE's spatial model for organizing AI agents into missions,
teams, gates, and recoverable runs. It pairs with `ARCHON.md` for the CLI surface
and `DATA-MODEL.md` for persistence and event formats.

## One-sentence definition

**Formations is CHROTE's file-backed command-and-control layer for composing AI
agents into executable teams, with CHROTE as the spatial cockpit and `archon` as
the exact command surface.**

## Product purpose

The problem is not running one agent. The problem is operating a durable personal
AI organization:

- persistent personas with identities, capabilities, harnesses, and standards;
- temporary or reusable teams assembled for specific work;
- work that must be started, observed, redirected, gated, resumed, or aborted;
- human judgment entering at mission/gate/taste moments, not every substep;
- agent-native automation that is scriptable and reproducible.

Formations gives that organization a concrete operating model:

- humans see and steer the work spatially in CHROTE;
- agents and scripts author and execute the same work through `archon`;
- both surfaces round-trip through the same files, shared Go package, and run
  ledger.

## Collaboration model

Formations exist to enable capable agents to collaborate, not to reduce them to
fixed pipeline steps. The system should give high-tier models enough context,
tools, and visibility to use their intelligence inside the team setting while
CHROTE/Archon keeps the work reproducible, bounded, and observable.

- `solo` is one agent working with a clear brief and output contract.
- `flow` is ordered handoff where the sequence itself is the coordination model.
- `peer` is collaborative work without hierarchy: agents share the brief, use a
  shared run plane (for example an append-only chat/blackboard file) to converse,
  inspect or critique each other's work as the available tools allow, and
  converge on a synthesis or set of artifacts. Archon should seed the first turn
  with the task and enough team context to get the group moving; peers then read
  the shared plane, decide what to say or do next, write their contribution, and
  may wait, inspect sibling sessions with scoped tools, or continue work. A
  lightweight facilitator may nudge stuck peers, detect loops, or surface
  problems, but it must not become a hidden hierarchy or fixed choreography.
- `orchestrated` is leader-driven collaboration: one appointed agent owns team
  coordination, but it should steer through practical affordances such as
  prompting worker sessions, inspecting/capturing session state, collecting
  artifacts, using monitors or subagents to surface key worker status, requesting
  revisions, running or requesting checks, and deciding when the formation is
  ready to finish. Those affordances may be native tools the agent already uses
  well, such as `tmux` CLI against scoped session names, or Archon helpers where
  formation context, lookup, provenance, or UI projection adds value.

Do not treat peer or orchestrated formations as a rigid choreography. Archon and
the runtime provide the team roster, scoped session mapping, redaction/output
caps, artifact collection, and ledger evidence; the agents supply the judgment
about how to collaborate. Do not build Archon commands that merely duplicate
standard terminal skills unless they add formation semantics, safety, or durable
evidence.

## Core nouns

| Noun | Meaning |
| --- | --- |
| Agent | Durable persona card with stable id, display name, capabilities, and harness variants |
| Formation | Agent-execution node with `solo`, `peer`, `flow`, or `orchestrated` coordination |
| Slot | Role position inside a formation, optionally assigned to an agent id |
| Mission | Run entry/input node with fixed `out` in this phase, usually linked to work state |
| Tool | Pure bounded deterministic transformation through a frozen host-owned profile |
| Board | Persisted graph of missions, formations, Tools, gates, connections, and layout metadata |
| Connection | Stable `workflow` payload edge or reserved `judge` control edge |
| Port | Stable named input/output address with a declared payload kind |
| Payload | Typed work, gate feedback, unavailable result, or error routed through a port |
| Gate | Human/code/judge evaluator and router; never a transformation step |
| Verification | Schema-1 inline check retained for inspection; schema-2 execution is deferred |
| Run | Execution instance that binds slots, dispatches work, records events, and projects state |
| Ledger | Append-only event history for a run |

## Current implementation and accepted target

Current main implements Mission, Formation, and Gate nodes, stable Formation
ports, fixed Mission/Gate ports, file-backed connections, agent execution, and
run ledgers. It does not implement Tool nodes, typed port kinds, exact
pass-through provenance, typed gate feedback, or exact run-bound pane targets.
Its workspace run files are not yet the accepted security boundary.

ADR-0006 accepts those missing pieces as the mixed-workflow target. A board will
combine Mission entry, Formation agent work, Tool transformation, and Gate
evaluation without reducing them to generic plugins. This section is a contract
for the landing slices, not a claim that current main already supports Tool
authoring or exact terminal Peek.

In the accepted target, canonical ledger, graph snapshot, private bindings,
sealed Tool inputs, and pending raw-redaction roots live under the writer-only
CHROTE data root outside generic Files roots. `run_started` binds their exact
hashes. Run/event APIs expose sanitized projections, and File Peek receives only
currently authorized registered artifacts; a workspace substitute or historical
revoked ref is never replay/read authority. This storage/ownership landing is
the accepted target here; planned ADR-0007 (`ctx-ug7.4`) will own its detailed
storage decision, and `ctx-ug7.6` owns coordinator enforcement.

The private graph snapshot may copy exact Mission objectives, Formation briefs,
and Gate criteria only when its embedded, hash-covered
`authoredConfigManifest` classifies the exact field/node as `authored_config`
with a closed source role, versioned encoding/media type, and SHA-256.
Missing/extra/mismatched entries reject. Gate criteria use
`gate-criterion-utf8-v1`/`text/plain`; human prompt and PASS/FAIL choices use
only closed fixed-system templates. This narrow ADR-0005 exception is
configuration already durable in the board and may outlive later board edits.
Mission output/unchanged deliveries and isolated Formation seed input may copy
objective/brief bytes only through the classified root-derived projection that
exact-matches `run_started.rootInputProjection`; generic unclassified copies are
invalid.
Runtime/external values and every composed prompt remain inside redaction and are
never admitted through that exception.

A private authority directory without a valid seq-1 `run_started` is not a run
and sends nothing. Startup recovery first cleans/fsyncs every pending raw target
and obligation, then deletes the orphan tree and parent-directory-fsyncs; if
cleanup/identity is unprovable it quarantines the tree as non-authorizing and
never exposes or adopts it.

The target is board schema 2 and event schema 2. Schema-1 Formation ports remain
readable with explicit in-memory defaults (`work`, the stable full initial
`acceptedMediaTypes` set of `text/plain`, `text/markdown`, and
`application/json`, plus required `data` inputs) and are written only by an
atomic schema-2 migration. Fixed Mission `out` accepts only `text/markdown`;
Gate `in`/`pass` work ports use the full set. Gate `fail` is `gate_feedback` and has no media set. A
legacy fail edge into a work
input loads as degraded for inspection but cannot validate or run until rewired;
annotated-work pushback is never inferred. Old ledgers project with their
recorded schema-1 semantics and are never reinterpreted as typed feedback. A
safely normalizable schema-1 board may start from an immutable normalized
schema-2 run snapshot without rewriting the canonical board; source/snapshot
schema are recorded. Schema-1 runs are inspect-only and resume returns
`legacy_run_requires_new_run`.
Schema-1 inline Formation verification is also inspection-only for schema-2.
Its existing verdict lacks exact attempt/output and replay-safe revision
identity, so validation and run preflight fail
`legacy_inline_verification_requires_migration` until `ctx-ug7.17` defines or
retires it. Schema-2 emits no `verification_verdict`.

## Required invariants

1. **One model, multiple surfaces.** UI gestures and `archon` verbs must use the
   same files and shared Go package.
2. **Files are canonical for persistence.** Browser state is not durable truth.
3. **Ledger is canonical for run history.** Status is projected from append-only
   events.
4. **Stable ids matter.** Node, port, edge, slot, agent, board, mission, tool,
   gate, and run ids must survive round-trips.
5. **Layout is not structure.** Node positions and wire lanes live in layout
   sidecars, not board definitions.
6. **Execution context fails loud.** Missing or ambiguous sessions, harnesses,
   checks, cwd, or agents cannot silently substitute.
7. **Formations is always-on.** It is a permanent first-class surface, not a
   feature flag. The only Formations env vars are the executor safety ladder
   (`CHROTE_FORMATIONS_LAB_*` / `CHROTE_FORMATIONS_TMUX_*` /
   `CHROTE_FORMATIONS_SCRIPT_GATES` / `CHROTE_FORMATIONS_GATE_*` /
   `CHROTE_FORMATIONS_TMUX_PROD_SMOKE`), which gate execution-environment
   or gate-adapter promotion, never feature availability.
8. **Beads can anchor missions; it is not the graph store.**
9. **No command-execution landmines.** Free-text criteria never become implicit
   shell execution. Executable script gates require explicit operator-authored
   command config and guardrails: `commandArgv` is the default form; CHROTE
   passes argv literally and does not insert a shell. `commandCwd` is a cwd guard
   under the workspace, not a filesystem sandbox. `commandShell` is the explicit
   shell opt-in. Legacy `command` strings are parseable for old boards but are
   not executable by script gates.
10. **Ports carry concrete payloads.** Connections route the payload attached to
    their source output port. Every output-producing Mission, Formation, or Tool
    node emits `node_output.outputs` keyed by stable output port ids; missing,
    unknown, or incompatible output ids block loudly instead of broadcasting a
    blob. Gates use the separate verdict/routing contract below.
11. **One input, one producer.** Every input port accepts at most one incoming
    edge. Joins declare multiple distinct required ports and start only when all
    are satisfied.
12. **Attempts bind immutable inputs.** A completed attempt is never mutated by
    later deliveries. Inputs freeze at `node_started`; late optional data is
    recorded and ignored. Only matching feedback on a persisted `retry_control`
    role creates an in-graph bounded next attempt from the exact evaluated source
    attempt. Explicit operator resume is a separate bounded trigger for a
    retryable blocked producer whose attempt delivered no edges.
13. **Gates evaluate; they do not transform.** Pass preserves the evaluated
    work and its provenance. Fail creates one stable typed feedback object;
    pushback is a fail-route action, not a verdict, and judge output becomes
    verdict metadata only.
14. **Workflow payload ports are directional.** The Gate's reserved `judge`
    socket is an evaluation-control relationship, not a typed payload port. It
    permits one linear Formation-only judge channel with one send and one return
    and never carries downstream work. Gate `in` accepts work, `pass` preserves
    it, and `fail` carries feedback.

## Port payload contract

Connections are not just visual arrows. A wire from `source:summary` to
`writer:input` carries the logical `summary` output under one stable port/attempt
identity. Durable `node_output.outputs[portId]` and
`node_started.inputRefs[]` store `PayloadProjection`, not necessarily raw bytes.
Each `RunInputRef` has stable `inputId`; `sourceKind=edge` records source/output
and traversal provenance, while `sourceKind=run_seed` records run/seed ids,
`formation-brief-jcs-v1` encoding, `application/json` media type, exact
frozen-brief byte hash, and destination without inventing an edge. Its projection
must be the closed `authored_config` root-derived variant exact-matching
`run_started.rootInputProjection`; a generic unclassified exact copy is invalid.
Mission `node_output.outputs[out]` and its unchanged downstream deliveries use
the same classified root-derived rule for the canonical objective.

There is one logical routing contract: `node_output.outputs[portId]`. During
fresh redacted execution an in-memory `ExecutionOutput`/`ExecutionInputRef` may
carry the exact authoritative value under the same identity; neither is written
to the ledger or API. Recovery never routes a redacted projection or substitutes
its hash, marker, summary, or sanitized evidence. Free-form
`node_output.text` is display summary only and never feeds graph edges. Current
Formation executor ingress and schema-1 reads use
`{text, ref, reportRef, artifactRef}` compatibility fields. They terminate at
writer-private normalization and are invalid in schema-2 events/APIs. The writer
registers safe file evidence first; schema-2 `node_output` records only optional
`reportArtifactId` plus stable-order `artifactIds`/`diffArtifactIds`, and
`run_succeeded` records only optional `summaryArtifactId` plus
`outputArtifactIds`. Public surfaces hydrate their latest authorized projection.
The accepted canonical payload adds `kind`, allowlisted `mediaType`,
safe ref size/hash metadata, and typed unavailable/error fields without creating
another routing path.

Payload kinds are `work`, `gate_feedback`, `unavailable`, and `error`. Tool inputs
use `data`; `retry_control` is allowed only on an optional Formation input. The
Mission, Formation, and Tool outputs produce only `work`; Tool inputs accept
only `work`. Formation inputs may declare `work` or `gate_feedback`, but only the
fixed Gate `fail` port produces `gate_feedback`. Shared readers, writers,
validation, and preflight reject every other feedback producer. The
first mixed-workflow contract permits bounded UTF-8 `text/plain`,
`text/markdown`, and `application/json`, plus safe refs to authorized-root
regular files. Every work port declares a non-empty accepted-media subset, and a
work payload contains exactly one of bounded text or one artifact under its one
media type; mixed representations are invalid. Full JSON Schema is deferred.
Each artifact projection has a stable `artifactId`. While available, its safe ref
names the authorized `rootId`, root-relative path, media type, size, and SHA-256.
Unavailable/redacted/expired projections keep the id and error metadata but no
readable ref. A port's declared kind governs successful delivery;
`unavailable` and `error` may instead terminate any declared output as
non-delivered system outcomes without being mislabeled as kind mismatches.
They are inspectable unsuccessful attempt outcomes, not work. A producer
finalizes all declared outputs before routing any of them; one failed output
means that attempt records `deliveredEdges=[]`, including for successful sibling
ports. Descendants on that dependency path do not dispatch, while
independent/in-flight branches may finish. Retryability is an engine policy and
never automatic. A producer is retryable only when every unsuccessful declared
output is retryable; one non-retryable sibling makes the whole attempt terminal,
while successful siblings remain non-delivered. At quiescence, latest unresolved producer failures
are ordered by `(minimum outcomeSeq, nodeId)`. If any is non-retryable, the first
non-retryable candidate selects terminal failure. Otherwise the first retryable
candidate alone blocks using `blockScope=node`,
`resumePolicy=retry_failed_producer`, empty open dispatches, and one
whole-producer retry target. Other candidates remain durable closed failures and
prevent success.
Explicit operator resume starts attempt N+1 from that target's frozen
inputs and unchanged run bindings with a new dispatch/Tool lease. It is rejected
after any prior delivery or when authoritative input is unavailable. At the
next quiescence, success removes the old candidate, a later retryable failure
replaces it, and another ordered candidate requires another explicit resume. A
non-retryable candidate selected by the same stable order ends the run as
`run_failed(code=declared_output_failed)` with provenance-only `relatedSeq` and
`failureCause={kind=none}` because its producer attempt is already closed.
Blocked runs use one closed policy union: `retry_failed_producer` has no open
dispatch and exactly one whole-producer target; `reattach_only` preserves a
non-empty unmatched dispatch set and sends no prompt; `new_run_required` allows
neither retry target nor next epoch. The post-reattach set may contain only the
still-unmatched subset of the preceding set. Only the first two permit the exact
bounded resume their names describe.
At quiescence, cancel/finality wins first; unmatched-dispatch recovery preserves
open authority next; an outstanding human request remains visible; then one
non-resumable semantic blocker is selected by stable causal sequence/reason/id
before any retryable-output block. Other candidates remain inspectable evidence,
not competing `run_blocked` events.
Exhausted immutable max-dispatch, max-attempt, or wall-clock limits append a
scoped stable limit error and terminal
`run_failed(code=run_limit_exhausted)` with exact dispositions for every open
attempt, slot dispatch, and Tool lease. Late
results/output/routing are rejected; Tool scopes remain privately owned for
fencing/cleanup. Continuing requires a new run with a new frozen limit snapshot.

Tmux-backed agents receive the port-output contract as a fenced
`chrote-outputs` JSON block instruction; lab runs synthesize deterministic
payloads for every output port. If an executable node omits a declared output or
emits an unknown/incompatible output id, the run records a typed loud failure.

At `node_started`, every required execution ref and any optional execution ref
already present freeze as the attempt input set; the ledger stores only their
durable projections. A later optional `data` delivery records
`node_input_ignored`; a late required delivery fails loud. A
`gate_feedback` value can trigger another attempt only through a persisted
optional `retry_control` input and only when the traversal names one exact
evaluated source attempt whose node matches the receiver. It is the only
in-graph retry trigger; operator resume is the separate blocked-run trigger.
After validated `retry_control` edges are removed, the workflow-channel graph
must be acyclic. Read, mutation, validation, and preflight reject every other
cycle rather than allowing a required-input deadlock.
If an acyclic graph nevertheless quiesces with a never-started JOIN holding only
some required inputs, the engine records node-scoped
`unsatisfied_required_input` and a non-resumable node block with empty open
dispatches/retry targets. The node projects `blocked`, not indefinitely waiting;
corrected wiring starts a new run.

Short routed outputs may live entirely in the JSON envelope:

```json
{"port_summary":{"text":"short non-secret payload"}}
```

Long routed outputs must not put prose into JSON strings. Current tmux-backed
emitters write the full payload to a text artifact and provide a compatibility
`ref`:

```json
{"port_summary":{"text":"short non-secret summary","ref":".formations/artifacts/<run-id>/summary.md"}}
```

The accepted writer resolves that private compatibility input, durably registers
the safe descriptor, and records its stable `artifactId` in schema-2 fields. The
routable payload may then use that registered descriptor:

```json
{"kind":"work","mediaType":"text/markdown","artifact":{"artifactId":"art_01J9_summary","rootId":"workspace","ref":".formations/artifacts/<run-id>/summary.md","sizeBytes":1234,"sha256":"..."}}
```

For tmux-backed runs, local file refs are resolved only after canonicalizing them
under the executor's configured roots/workspace. Missing, unreadable, out-of-root,
symlink-escaped, non-regular, non-text, or oversized refs block loudly. `ref` is
opened root-relative with no-follow semantics; regular identity, size/media/hash,
and authorization are validated on that handle, and File Peek renders only the
same verified bytes/handle without reopening the path. It is not a general host-file read capability, and these root checks are not an OS
sandbox: tmux agents still run with the host Unix user's permissions. Treat
output text, refs, filenames, and hydrated artifact content as durable non-secret
run evidence. `Redact=true` runs use the stricter boundary below.

The descriptor proves validation when it was attached, not perpetual mutable-file
availability. File access rechecks it and fails unavailable immediately. Durable
projection changes only after the reconciler appends `artifact_observed` for that
existing `artifactId`; projection never inspects the filesystem. An available
observation carries a newly validated descriptor with the same id. The first
available registration or observation establishes its immutable
root/ref/media/size/hash values; every later available observation must match or
use a new artifact id and registration. Slot, Gate, and system artifacts use one
source-discriminated fsynced `artifact_attached` before any later reference. New
Tool artifacts are registered atomically by
`tool_result.artifactRegistrations`; an open Tool lease has no public artifact
registration and cannot collide with cleanup/rerun.
At their event sequence, embedded projections only mirror the latest
registered/observed projection; they cannot register or mutate an id, and later
observations do not rewrite them. Other states omit the descriptor and require a
stable error code; an unknown artifact id or duplicate registration is invalid.
Evidence stores only stable artifact ids. Run detail, event, and SSE APIs hydrate
every occurrence through the latest authorized projection rather than exposing
raw ledger descriptors. After redaction/expiry, historical events and File Peek
cannot recover an older readable ref.

## Tool profile contract

A board stores a Tool profile id/version constraint and modeled non-secret
parameters, never executable text. Run start freezes the resolved profile
version/content hash, normalized parameters/hash, effective policy and
determinism-policy hashes, and
immutable execution-bundle hash in a `RunToolBinding` inside host-private run
authority. The content-addressed bundle covers executable,
script/toolchain identity, argv template, cwd contract, normalized non-secret
allowlisted environment values, supervisor/fence policy, and limits; a mutable
path is not an identity. Preflight rejects a reachable Tool before `run_started`
when the frozen supervisor/fence policy is unavailable.
The first Tool class is certified deterministic: network, secrets, undeclared
host reads, and external writes are denied; locale/timezone are normalized;
clock/entropy are frozen or denied; and repeat vectors fix expected output
hashes. Before dispatch, exact inputs are copied into one sealed,
content-addressed read-only set under the host-private run root; spawn and replay
never reopen the mutable source. Each Tool lease id is unique
within the run and maps one-to-one to a node attempt. Before process execution,
`tool_dispatch` is fsynced with that lease id, input-manifest and
input/profile/parameter/policy/determinism-policy/execution-bundle hashes, and private lease-root or
redacted-obligation authority. Before every actual spawn,
the host reserves/fsyncs a private descendant process scope plus an immutable
deadline authority whose start and effective deadline derive once from the frozen
timeout policy. The ledger fsyncs `tool_process_launch` with stable
launch/scope/deadline-authority ids plus the next
generation.
Generation starts at 1; the writer requires unique launch/scope/deadline-authority
ids and an exact run, lease, launch, node, attempt, generation, scope, and
deadline-authority match across the lease, private records, and launch event.
The opaque scope id is never a reusable raw PID; private identity/start evidence
fences the exact tree. Only then does the process spawn. An orphan scope/deadline
reservation without a launch is one atomic identity set: recovery fences then
reuses the exact pair or deletes both and directory-fsyncs before replacement,
without spawning. Before normal completion, startup
recovery, or cancellation first terminates, seals, or waits on a recorded launch,
writer-private authority fsyncs one exact Tool quiescence-boundary record. It
binds the run/lease/launch/node/attempt/generation/scope, matching boundary cause,
and exact pre-spawn deadline authority. Its start, timeout policy/hash, duration,
and `effectiveDeadlineAt` must byte-match that authority. An unresolved record for that launch survives and is
reused across every restart; recovery never derives a fresh start or deadline
from restart time. A crash after spawn but before boundary creation reconstructs
it only from that same authority. The Tool writes staged output. After exit, the supervisor must
seal the scope against new members and prove the whole descendant scope quiescent
by that persisted deadline; failure records terminal
`run_failed(code=tool_process_not_quiescent)` with no promotion/result. Private cleanup ownership
continues after finality; later quiescence permits cleanup but never promotion,
result, rerun, or another execution event. Only after an on-time proof is output
fsynced, promoted by atomic rename, and directory-fsynced. One appended/fsynced
`tool_result` keyed to the latest launch then atomically closes the lease,
registers every new Tool artifact, and contains the complete
durable canonical port-output projection map with exact availability, so
`node_output` can be recovered without rerun. The quiescent private scope record is then deleted and
directory-fsynced; success requires no Tool scope record. Recovery routes only
exact available payloads. A
completed lease is never replayed. An open lease with no recorded launch may
discard an orphan private scope and create generation 1 after normal validation;
no process could have started before the launch event. Once a launch is recorded,
recovery must require its matching scope, terminate and seal it against new
members, and prove every descendant quiescent by deadline before cleaning the
root. Its sealed/quiescent record remains durable until the next generation's
launch or a terminal non-rerun event is fsynced, then is deleted and
directory-fsynced. A
missing or ambiguous scope for a recorded
launch, or failed quiescence proof, records terminal
`run_failed(code=tool_process_not_quiescent)` and permits neither cleanup nor rerun. Each launch
consumes dispatch and wall-clock limits while retaining its enclosing node
attempt; only a new node attempt consumes `maxAttempts`.
At normal completion, startup recovery, and cancellation, all Tool scopes that
miss their persisted deadline are selected as one complete set ordered by
`(effectiveDeadlineAt, dispatchSeq, toolLeaseId)`. The first alone is failure
cause; callback/map/process-exit order never decides.
Abort first fsyncs `run_cancel_requested`, whose open-attempt, open-slot, and
lease snapshots block new
dispatch/replay and makes the writer reject launches, results, outputs, and
routing except cancellation finality. It soft-interrupts only an exact unresolved
dispatch proven on its frozen target, never kills tmux sessions, records every
slot disposition, cleans never-launched leases without execution, and fences every
launched Tool scope before cleaning its root and appending `run_canceled`. A
scope that cannot be proven quiescent by deadline instead causes terminal
`run_failed(code=tool_process_not_quiescent)`; the supervisor retains private cleanup ownership
after finality. Later proof may remove or quarantine an unredacted root; a
redacted root must be sanitized/removed and cleanup fsynced before deleting its
obligation. Neither path may promote, record a result, or rerun. Cancellation
also disposes every snapshotted node attempt, including a Gate waiting for human
input, and rejects a later decision.
Every execution-final event revokes all open node-attempt, slot, and Tool
authority. Success permits none; cancellation reconciles all three exact
snapshots. Failure's typed `failureCause` selects one exact slot, Tool lease,
scoped error, or no cause; it resolves at most one attempt as failed and marks
every collateral attempt/resource abandoned. `relatedSeq` is context only and
never chooses the cause. All are non-authorizing; Tools remain
private-cleanup-owned.
Final projection also closes nodes in the frozen run root that never started:
cancel makes them `canceled`, failure makes them `abandoned`, and valid success
may mark them `not_run` only when the ledger proves they were unreached or on an
untaken branch. A delivered input on a taken path makes success invalid.
`node_waiting` readiness counts remain inspectable evidence, not an active
post-final state.
Effectful Tool profiles are deferred.

For `Redact=true`, the private pending-redaction registry must own and fsync the
exact lease root before public `tool_dispatch`, which exposes only its opaque id.
It starts as `pending` for generation 1, and every redacted launch must match a
pending obligation generation before process execution. An obligation orphaned
before dispatch is cleaned/deleted without Tool execution. After raw targets are
sanitized/removed, the registry fsyncs the matching generation from `pending` to
`cleaned` while retaining its locator. Only a cleaned generation matching the
latest launch may commit `tool_result` and policy-safe artifact registrations;
the entry is deleted and directory-fsynced after the result. After recovery
cleans generation N, a rerun must first fsync `cleaned(N)` to `pending(N+1)`;
only then may it record and spawn generation N+1. If recovery crashes before the
launch, it treats pending N+1 as a safe prepared generation, cleans idempotently,
fences any orphan scope without spawning, and reuses N+1 after validation.
Recovery therefore keeps an
exact root for every open redacted lease, and success requires no
remaining redaction or Tool-scope entry. Its output map contains only exact available or
hash-only redacted `PayloadProjection` values. Sanitized non-exact text, refs,
and summaries live only in separate bounded `tool_result.displayEvidence` or
`tool_result.artifacts` fields; they never become port outputs or routing
authority. Fresh execution may keep a live `ExecutionOutput` outside durable
state. Lost raw execution values follow ADR-0005's terminal
`redacted_input_unavailable` rule, not Tool rerun.

## Gate routing contract

A Gate evaluates one exact work input. Its verdict, per-kind result, safe
reason/evidence, evaluated input ref, and route are separate ledger metadata.
Schema-2 `code` kinds select a frozen, host-owned, certified deterministic
in-process evaluator profile. Its closed policy denies network, secrets,
undeclared host reads, process spawn, and writes; normalizes locale/timezone;
and freezes/denies clock and entropy. `RunGateBinding` freezes profile/content,
evaluator-bundle, parameter, policy, and determinism-policy hashes plus positive
input-byte, result-byte, and deterministic operation limits plus
`resultEncoding=decision-result-jcs-v1`. Admission requires
a total evaluator under host-metered fuel and panic containment. Exhaustion or
panic records Gate-scoped `error(code=gate_evaluator_error)`; it cannot wedge wall-clock
finality and emits no kind result/verdict/route. Crash-repeat requires the exact
bundle and policy; an upgrade cannot substitute. Every initial or recovery
evaluation also resolves and revalidates exact authoritative input bytes against
`inputSha256`. Lost Redact=true input appends terminal
`run_failed(code=redacted_input_unavailable)` after bounded Gate/attempt/input
context, with the source sequence as provenance and `failureCause={kind=none}`.
Non-redacted drift appends Gate-scoped
`error(code=gate_input_integrity_failed)` then terminal
`run_failed(code=gate_input_integrity_failed)` with that error as
`failureCause={kind=error,errorSeq}`, so the exact Gate attempt fails. Both use
exact dispositions for every open attempt, slot dispatch, and Tool lease; neither
substitutes hash-only evidence or emits result/verdict/route.
An accepted code result is RFC 8785 canonical UTF-8 JSON over exactly
`{verdict,reason,evidence}` with no unknown keys/trailing newline and preserved
evidence order. `gate_kind_result.resultSha256` hashes those exact bytes; replay
cannot substitute another serializer.
Current schema-1 argv/shell Script Gates do not silently inherit this safety:
schema-2 normalization rejects them with
`legacy_script_gate_requires_fenced_migration` until the process-fence or
retirement work in `ctx-ug7.16` lands.

An executable Gate declares a non-empty, duplicate-free subset of `code`,
`formation`, and `human`; preflight rejects empty, duplicate, or unknown values
before `run_started`. Declared kinds are an all-of set evaluated in fixed
`code`, `formation`, `human` order. Each completed code/formation kind fsyncs one unique
`gate_kind_result` before the next starts; replay reuses it. The first fail
short-circuits later kinds as `not_run`. Human is requested only after prior
kinds pass and projects `waiting_human`; its exact decision continues the same
Gate attempt. Exactly one aggregate `gate_verdict` then contains every declared
kind and is the only event allowed to route.
The human request has no independent timeout or default verdict. It ends only
with its exact decision, cancellation, or the run's terminal wall-clock limit.
A code-evaluator boundary failure is not a FAIL verdict. It records Gate-scoped
`error(code=gate_evaluator_error)` and, after independent work settles, a
non-resumable `new_run_required` Gate block with only that Gate id, empty open
dispatches/retry targets, and no next epoch. It routes neither branch; an
execution-final event reached during quiescence takes precedence.

On pass, the downstream input keeps the original source node, source port,
output sequence, attempt, and durable projection unchanged; fresh redacted
execution also forwards the same exact live value. The pass edge is recorded as
a new traversal only, and evidence is never substituted. On fail, the Gate
creates one stable `gate_feedback` object for that evaluation; zero or more fail edges reference it. An unwired fail has
zero deliveries and, after quiescence, records a non-resumable Gate-scoped
`unwired_gate_fail` block with empty open dispatches/retry targets. The Gate
keeps its visible FAIL verdict; the block overlays the attempt already closed by
that verdict and never closes it twice. Its completed upstream Formation stays
completed. Fan-out does not duplicate feedback identity.
Feedback references the original input only by its stable input id and matching
Gate-input sequence, and carries feedback/Gate ids, failed verdict, bounded typed
safe evidence, gate sequence, and Gate attempt. It embeds no work ref, payload
projection, payload, or artifact, so it cannot replace or silently copy work.

A separate correction node declares distinct required work and feedback `data`
ports. `pushback` is a deliberately narrow fail-route action. The evaluated
input must come directly from the receiving Formation; that Formation's entire
connected workflow-output frontier must be the single edge into the Gate, and
the Gate fail frontier must be the single edge back to its `retry_control`.
The first Gate evaluation allocates a stable run-local revision-cycle id.
Matching feedback starts source attempt N+1 only while the frozen authoritative
inputs remain live or durably exact; otherwise ADR-0005 fails terminally. The
revised output opens Gate attempt N+1 linked by revision-cycle id, feedback id, prior Gate seq, and new
source attempt. This is the only late-required delivery allowed. Mixed fail
fan-out, side-output delivery, downstream replay, and non-source pushback are
invalid; use an explicit correction path instead. Formation
judge output returns through the reserved `judge` control socket and determines
verdict metadata only. The exit returns exactly one bounded
`{verdict: pass|fail, reason, evidence[]}` value using the safe evidence union.
Missing, malformed, or multiple returns append `judge_attempt_failed` with
`code=invalid_judge_result`, complete the judge Formation attempt as failed,
block, route neither branch, and emit no ordinary `node_output`. Schema-2 judge
edges use `channel=judge` and form one
linear Formation-only entry-to-exit chain. They order bounded `JudgeContext` /
`JudgeResult` evaluation, not PortPayload delivery; branch, JOIN, repeated node,
Tool participation, workflow cross-use, and side entry/exit are invalid.
`judge-context-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{gateId,gateAttempt,criterion,kinds,evaluatedInput,durableEvaluatedInput,
priorResults}` with fixed Gate-kind order, judge-chain prior-result order,
preserved nested evidence order, no unknown keys, and no trailing newline.
`node_started` records that encoding and the SHA-256 of those exact bytes. Judge
results use `decision-result-jcs-v1`; `judge_result` records both encoding ids and
the exact context/result hashes.
An executable Gate has a complete judge channel if and only if its kinds include
`formation`. Each member's strict result is appended and fsynced as
`judge_result` before the next judge dispatch. Exactly one `judge_result` or
`judge_attempt_failed` completes each judge key; conflicting completion is a
loud ledger error. Replay rebuilds durable prior results from those events and
never reruns or reparses completed judge capture. The next exact JudgeContext
dispatch requires the evaluated input to remain live or durably exact; otherwise
recovery appends terminal `run_failed(code=redacted_input_unavailable)`. A judge Formation freezes
context hash/prior-result sequences at `node_started`; either completion event
closes that attempt without ordinary workflow `node_output`. Invalid output is
then prevented from starting new Gate/dependent work. Already in-flight and
independent branches settle before `run_blocked` uses `blockScope=gate` and names the judge and Gate with no
open dispatches/retry targets, `resumeAllowed=false`,
`resumePolicy=new_run_required`, and no `nextEpoch`; there is no same-run judge
retry in this phase. An execution-final event produced during quiescence takes
precedence, so no later block is appended.
Continuing judge-authored content requires a separate Formation or Tool outside
the judge channel.

## Redacted runs and replay

A `Redact=true` run separates execution-authoritative values from durable
evidence. Fresh execution may route a raw value only from the live attempt's
ephemeral in-memory state. Runtime/external ledger fields, composed prompts,
verifier and per-kind feedback, captures, reports, artifact contents, output refs
and their targets, derived evidence, and errors must not retain that value. Exact
typed `authored_config` copied from the board (Mission objective, Formation brief,
or Gate criterion) is the sole configuration exception. Human prompt and
PASS/FAIL labels use closed fixed-system templates; unclassified or dynamic
strings remain covered. A safe ref may remain only after its target is
sanitized or replaced inside an authorized root.

Redaction markers, hashes, and display summaries explain what was removed; they
are never graph inputs. Raw executor capture may exist only as cleanup-owned
transient material. Before a persistent path can contain raw bytes, a durable
pending-redaction obligation is written and fsynced for that exact target. Its
internal cleanup locator is never exposed as an output ref or graph input.
Cleanup and recovery are idempotent, and no run can become final-successful
while a capture, report, or artifact remains pending redaction.

Recovery may reattach the same unresolved attempt without redispatch when its
qualified session identity is proven. It may finish pending cleanup or continue
other work only when no discarded authoritative value is required. Pending
cleanup targets and other evidence are never sources for reconstructing graph
input. At recovery quiescence, the first `reattach_only` block freezes every
unmatched dispatch in stable order, using node scope only when all belong to one
node and run scope otherwise. It rejects late results/output/routing until exact
explicit resume. That one bounded epoch sends nothing and may close individual
exact results. If some remain, the next non-resumable block contains only that
subset and requires cancellation before the old work can be represented
retired. A separately started run is independent and does not close any old
lease. An unmatched lease is never superseded or redispatched in the same run.
If a future dispatch needs raw input that redaction intentionally
discarded, append terminal `type=run_failed` with
`data.code=redacted_input_unavailable`,
`data.reason=redacted_input_unavailable`, `data.unrecoverable=true`, and
`data.final=true`; `data.relatedSeq` identifies the exact source event whose raw
value was required and is not a cause selector, `data.failureCause={kind=none}`,
and `data.nodeAttemptDispositions`, `data.slotDispatchDispositions`, and
`data.toolLeaseDispositions` exactly close every node attempt, slot dispatch,
and Tool lease still open at failure (each array may be empty). The run cannot resume or open another epoch; retry means a
new run with newly supplied authoritative input. Never dispatch a redaction
marker or summary in its place.

ADR-0005 defines this boundary and its rejected alternatives.

## Run-bound sessions and local terminal view state

A board slot stores staffing intent: label, stable agent id, and optional harness
constraint. It stores no tmux target, and `assigned` does not mean runnable.

Run preflight records a `SlotResolution` (`unresolved`, `runnable`, `ambiguous`,
or `unavailable`) for each declared slot in the selected `runRoot` executable
subgraph; an unassigned slot is `unresolved/agent_unassigned`. It starts only
when all are runnable. The run then writes one
host-private immutable exact `RunSlotBinding` plus a hash-linked safe projection
per slot with an opaque `sessionTargetId`. Its private canonical `targetKey` identifies the
exact tmux server/pane incarnation, and a durable one-to-one mapping guarantees
independent resolutions of that pane return the same opaque handle. The private
fingerprint also freezes persona card, harness/foreground-process start,
cwd/root, and pane evidence; acquisition-time mismatch records stale and sends
nothing. Runtime health (`runnable`, `unavailable`, or
`stale`) is projected from appended evidence without changing the frozen
server/socket/session/window/pane identity. The reconciler records that evidence
as `slot_binding_observed` with binding/target ids, health, stable reason,
observation time, and related seq; projection never queries tmux. A multi-slot
attempt can expose several targets. Clients never reconstruct one from a display label or
same-named session, and a run cannot rebind a slot to a replacement pane.
Two selected slots resolving to one private key/opaque target reject before run
start. Across runs, a host-wide durable exclusive target lease keyed by
`targetKey` binds one target to one exact
run/dispatch/binding/node/attempt/slot before public dispatch. Before prompt
composition, every artifact-backed ordinary or judge Formation input is opened
once relative to its authorized root without following links; regular identity,
media, size, and SHA-256 are checked and the prompt consumes only the bytes from
that same handle, never the mutable path. Non-redacted drift fails
`formation_input_integrity_failed`; unavailable redacted bytes fail
`redacted_input_unavailable`; neither records `slot_dispatch` or sends. Only
after exact input validation may the
coordinator hash one in-memory prompt byte slice, fsync only its hash/dispatch
identity, and send that same slice at most once before discarding it. No prompt
bytes/ref/path/authority id is durable, and the hash never authorizes reconstruction
or resend. The prompt carries run, dispatch, and target-lease ids.
Sentinel/result must match all three. Busy acquisition sends nothing and fails
loud. Completion also requires the sentinel to be terminal and a certified
harness-ready/closed-turn proof for the frozen fingerprint; trailing old output
keeps occupancy. Success waits for the durable `result_committed` receipt
carrying that proof. Release atomically replaces occupancy with exact
non-occupying crash proof retained through final-event fsync. Result-closed
dispatches require `result_committed` receipts and never become slot
dispositions. For each unmatched slot, cancel/failure records a
`final_quiescent` receipt only with an exact dispatch-bound cancel/ready
acknowledgement or old-pane-gone proof; otherwise it retains a non-authorizing
busy hold or fail-closed quarantine. Holds/quarantines remain until exact
old-dispatch quiescence is proven. Receipts do not block later safe acquisition.
A sent interrupt, silence, time/name/PID expiry, or interleaved pane use is not
proof.
If a public dispatch lacks its expected private lease, the arbiter first
reconstructs a non-authorizing quarantine at the frozen key. Its stable
candidate set preserves the expected identity/result and each conflict as a
separate run/dispatch/lease entry. An unmatched candidate may expose that state
in its final disposition; a result-closed candidate must
become an exact release receipt before finality. It never recreates active authority or frees the
pane. A missing key creates a separate durable host-wide quarantine of every
available dispatch identity and denies all target acquisition until repair and
non-authorizing reconciliation clear it.
An idle deadline without the exact sentinel plus certified closed-turn proof records
`dispatch_idle_timeout` but no slot result. The dispatch and target lease remain
unmatched for bounded reattach; timeout alone never releases the target.

Binding and artifact observation events may append after execution finality, but
they are inspection-only. They cannot change run/node outcome, open an epoch, or
authorize another dispatch.

Open/closed Peek surfaces, terminal geometry, focus, and tiling are user-local
dashboard state. They do not mutate the board, run ledger, or tmux lifecycle.
Run-bound live Peek also requires the exact dispatch/target lease/fingerprint to
still own active unmatched occupancy or its terminal hold. After a release
receipt or quarantine, show captured history or `pane_moved_on`; opening the
current reused session is a separate non-run action.
Current main has only partial name-level binding evidence; exact run-bound Peek
remains target work.

## Interaction model

The reference interaction model is permissive direct manipulation:

- right-click meaningful elements for local commands;
- drag agents from roster into slots;
- create missions, formations, gates, and templates from the canvas;
- edit briefs and explicit Gates through local popovers; legacy inline
  verification remains read-only until `ctx-ug7.17` resolves it;
- connect, reconnect, route, and remove wires directly;
- start missions and see work cascade through the graph;
- project run status onto cards, gates, wires, and outputs;
- support undo for structural mutations.

Product principle:

> Whatever the user reasonably expects to work on the canvas should work.

Permissive gestures do not mean invalid persisted state. Gestures normalize into
valid model operations or fail loudly with an understandable reason.

## Prototype references

The local prototype remains the observable UI reference:

- `Perttus_vision_for_agent_orchestration/03-formations.html` is the visual and
  geometry reference.
- `Perttus_vision_for_agent_orchestration/03-formations.js` is the behavioral
  reference for observable canvas interactions.

Mock timings, canned outputs, in-memory ids, and prototype-only code structure
are not persistence or engine contracts.

## Run and recovery model

A correct run model:

1. A run records its root. A Mission root traverses its reachable graph and
   judge chains. An isolated Formation root requires a non-empty brief and
   exactly one required `data` input accepting `work` and `application/json`.
   Preflight encodes exactly `{goal, beadId, files, links}` as RFC 8785 canonical
   UTF-8 JSON (`formation-brief-jcs-v1`), normalizes missing scalar/array fields to
   `""`/`[]`, preserves array order, adds no trailing newline, and freezes those
   bytes as the `application/json` synthetic `run_seed` input and `seedSha256`.
   Incoming board edges, optional inputs,
   `retry_control`, and downstream edges remain outside the isolated root. Any
   other required-input shape fails before `run_started`.
   For either root, `run_started.rootInputProjection` uses the exact closed shape
   in `spec/contracts.md`; its `sha256` hashes the canonical Mission objective or
   Formation-brief bytes and its `text` contains those same canonical UTF-8 bytes.
   The only durable payload copies are the classified root-derived projections
   for Mission `out`/unchanged deliveries or the isolated `run_seed`, with exact
   source-role/encoding/media/hash/text parity; every unclassified copy rejects.
2. The engine snapshots the board and resolves slot bindings to live sessions.
3. Work dispatches to selected agent harnesses or bounded Tool profiles.
4. Events append to the NDJSON ledger.
5. Projected state updates UI, API, and CLI.
6. Join points wait for required inputs.
7. Gates evaluate code, human, or formation judges and route pass/fail/pushback.
8. Timeouts, missing sessions, sentinel failures, ambiguous checks, and blocked
   gates record loud events.
9. Runs can be inspected, followed, aborted, and—when their terminal state and
   evidence contract permit it—resumed from `archon` and reflected in the UI.
   `run_failed(code=redacted_input_unavailable)` is final and cannot resume.

Watching is optional. Recovery must not depend on a browser tab staying open.

## Execution environments

Formations execution promotes through explicit execution environments and gate
adapters. Each step up is an explicit configuration decision, never a silent
fallback.

1. **Lab.** `CHROTE_FORMATIONS_LAB_*` configures a deterministic executor that
   synthesizes outputs and sentinels with no tmux involvement. Full run-engine,
   ledger, gate, and recovery behavior is exercisable here. When lab harnesses
   are configured, lab takes precedence over the tmux executor.
2. **Isolated tmux.** `CHROTE_FORMATIONS_TMUX_*` dispatches to real agent
   sessions, but the executor refuses to run unless the socket, cwd, and all
   roots live under the system temp directory and the socket is not the
   default-resolved tmux socket. Dogfooding happens on a throwaway socket with
   its own sessions; the live cockpit socket is unreachable by construction.
3. **Script gates.** `CHROTE_FORMATIONS_SCRIPT_GATES` enables operator-authored
   script/lint/code gate commands. Script gates execute `commandArgv` literally
   without a CHROTE-inserted shell, inside the board workspace or a `commandCwd`
   that resolves under that workspace. This is a cwd guard, not a filesystem
   sandbox. A shell is allowed only with explicit `commandShell`.
   Legacy `command` text is stored for compatibility but deliberately fails if
   no structured command is present. Output caps, timeouts, redaction, and
   fail-loud gate verdicts apply. Script Gates return verdict/evidence only;
   they are not deterministic Tool transformation steps.
   This is current schema-1 compatibility behavior. Accepted schema-2 execution
   instead requires the pure in-process code-Gate profile contract above;
   command-backed migration fails loud until process fencing is separately
   implemented.
4. **Live socket (prod smoke).** Setting `CHROTE_FORMATIONS_TMUX_PROD_SMOKE`
   is the explicit operator opt-in that lifts the temp-socket and temp-root
   restrictions so the executor may target the live CHROTE tmux socket and real
   workspace roots. Nothing else is relaxed: sessions must already exist with
   the configured prefix, panes must be alive with cwd inside configured roots,
   output caps, timeouts, redaction, and fail-loud ledger events all still
   apply. The executor never creates or kills tmux sessions.

Promotion to the live socket means setting the `CHROTE_FORMATIONS_TMUX_*`
boundary and the prod-smoke opt-in on the CHROTE service itself, with lab
variables unset. `.env.example` documents the full variable surface.

## Build sequence

Build vertical slices:

```text
behavior scenario
→ React UI gesture/projection
→ shared Go formations package
→ archon verb
→ file/ledger round-trip
→ tests and review against root specs
```

Do not build a headless engine the UI cannot explain. Do not build a UI toy the
CLI cannot reproduce. The useful unit is an end-to-end capability.

## Success criteria

Formations is working when:

- a human can understand the agent organization and current mission state by
  looking at the canvas;
- an agent can author and mutate the same graph through `archon`;
- UI changes appear in files and CLI output without structural drift;
- CLI changes appear in the UI without structural drift;
- run state is visible on the graph, not buried only in logs;
- explicit Gates can block, pass, fail, drive wired correction, or delegate
  judgment without granting legacy inline verification schema-2 authority;
- failures leave durable evidence and recovery handles;
- mixed Mission/Formation/Tool/Gate workflows expose named inputs, outputs,
  artifacts, and typed failure/feedback without semantic drift;
- node inspection can identify the exact run attempt and qualified terminal
  target or explain why it is unavailable;
- the system remains reversible and local-first.

## Related root specs

- `ARCHON.md` — command surface and CLI verbs.
- `DATA-MODEL.md` — board, layout, persona, settings, and ledger formats.
- `DESIGN-SYSTEM.md` — cockpit feel, canvas affordances, themes, and UI density.
- `PRD.md` — product requirements and rollout staging.
