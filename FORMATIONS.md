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
- work that must be started, observed, redirected, gated, resumed, or canceled;
- human judgment entering at mission/gate/taste moments, not every substep;
- agent-native automation that is scriptable and reproducible.

Formations gives that organization a concrete operating model:

- humans see and steer the work spatially in CHROTE;
- agents and scripts author the same definitions through `archon` and submit
  runtime commands to the CHROTE coordinator;
- both surfaces round-trip definitions through the same files/shared Go package
  and inspect the same sanitized canonical run projection.

## Collaboration model

Formations exist to enable capable agents to collaborate, not to reduce them to
fixed pipeline steps. The system should give high-tier models enough context,
tools, and visibility to use their intelligence inside the team setting while
CHROTE/Archon keeps the work reproducible, bounded, and observable.

- `solo` is one agent working with a clear brief and output contract.
- `flow` is ordered handoff where the sequence itself is the coordination model.
- Schema-2 `peer` v1 is one bounded deterministic collaborative round: each slot
  contributes in persisted order, then the first persisted peer performs the
  facilitator synthesis over those contributions. It has no untracked shared
  chat file and no direct sibling-session mutation or capture.
- Schema-2 `orchestrated` v1 is one bounded leader round: the unique controller
  produces a plan, each non-controller slot performs one coordinator-dispatched
  worker turn in persisted order, then the controller performs the terminal
  synthesis. Every worker prompt, result, artifact, and target use ordinary
  fenced coordinator/target-lease authority; the controller never drives another
  bound tmux pane directly.

Richer agent-native peer conversation, shared planes, dynamic worker requests,
revision loops, and scoped sibling inspection remain product direction, not this
authority contract. They require an explicit versioned action/termination,
redaction, and replay design before use. The bounded v1 still leaves the content
and judgment of each turn to the agents while making scheduling auditable.

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
| Verification | Retired schema-1 inline check retained only for compatibility inspection and replacement-Gate-bound removal |
| Run | Execution instance that binds slots, dispatches work, records events, and projects state |
| Ledger | Append-only event history for a run |

## Current implementation and accepted target

Current implementation provides Mission, Formation, Gate, and non-executing Tool
definitions, schema-2 typed ports and connections, file-backed graph/layout
state, agent execution, and run ledgers. The closed initial Tool catalog is the
data-only `json.normalize@1` descriptor. Store, API, and Archon provide Tool
definition CRUD/projection; shared validation, wiring, and explicit Arrange
include Tool nodes; and the dashboard renders Tool cards, ports, connections,
parameters, and unavailable state. No Tool runner or result authority exists. A
selected Mission graph containing a Tool fails
`tool_execution_unavailable` before snapshot, binding, ledger, dispatch,
process, tmux, or artifact mutation. There is no isolated Tool-run endpoint;
isolated Formation runs do not traverse downstream Tools. Exact Tool
pass-through provenance, typed gate feedback at runtime, and exact run-bound
pane targets remain target work.
Legacy Gate command fields remain readable for source inspection and migration
planning, but Gate-owned argv/shell execution and new command-field authoring are
retired.
Its API and Archon each construct a synchronous local run engine, its `abort`
paths append final cancellation directly, and its workspace run files are not
yet the accepted ownership/security boundary.

ADR-0006 accepts those missing pieces as the mixed-workflow target. A board will
combine Mission entry, Formation agent work, Tool transformation, and Gate
evaluation without reducing them to generic plugins. This section is a contract
for the landing slices: current Tool authoring is deliberately non-executing,
and exact terminal Peek remains unavailable.

In the accepted ADR-0007 target, one fenced CHROTE server coordinator is the sole
semantic runtime writer for each configured workspace. Canonical command journal,
ledger, graph snapshot, private bindings/results, sealed Tool inputs, and pending
raw-redaction roots live below one writer-only, lane-independent
`<formations-host-authority-root>/workspaces/` outside generic Files roots. The
existing explicit `CHROTE_FORMATIONS_DATA_ROOT` server configuration supplies
that stable absolute root to every lane capable of `authoritySchema=2`; it is never derived
from a lane's service data directory and has no per-lane production fallback.
Each private run has one immutable `run.bootstrap.json` that exact-selects the
graph snapshot and private bindings by encoding and SHA-256 before `run_started`
can bind their identity. Run/event APIs expose sanitized projections, and File
Peek receives only currently authorized registered artifacts; a workspace
substitute, stray snapshot, or historical revoked ref is never replay/read
authority. `ctx-ug7.6` owns coordinator enforcement.

`authoritySchema=2` foundation support is an immutable code-owned capability registry, not
workspace data or a board choice. Its complete bytewise-ordered pair is
`formations.runtime-authority-read-guard.v1`, then
`formations.workspace-authority.v1`, validated before owner-lock selection or
fence allocation. The read guard is non-authorizing. The workspace-authority
capability authorizes only registration, private publication, owner lease/fence,
and command-journal foundation work; projection, reconciliation, cleanup,
quarantine, and execution remain unavailable until their later gates close.

Production writer trust comes from the dedicated service account provisioned by
the tracked `services/chrote-srv.service` unit as `User=chrote`. The service
manager and kernel resolve that account; server construction passes and checks
the process's live kernel effective UID against the `st_uid` owner identity of
private roots, files, and locks. No board, file, caller value, environment
variable, or separately configurable numeric UID can self-assert writer trust. Same-UID agent
processes are unsupported because private file modes cannot isolate them. Tests
may inject an expected UID and disposable root; this target authorizes no live
path, deployment, service-configuration, or UID migration.

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
and sends nothing. A supported current fenced owner first validates its historical
origin fence, then records the cleanup claim with its higher state fence and
cleans/fsyncs every pending raw target
and obligation, then deletes the orphan tree and parent-directory-fsyncs; if
cleanup/identity is unprovable it quarantines the tree as non-authorizing and
never exposes or adopts it.

Start, resume, cancel, and human-verdict requests carry a workspace-scoped stable
`commandId` plus canonical payload hash. The same id/hash returns the original
durable receipt; another hash conflicts without an effect. One
`commands/<commandId>.json` record holds the canonical request and becomes the
closed applied/rejected receipt; its immutable outcome fence may differ from the
current record-publication fence after takeover repair, and there is no second
actor or receipt file. Start returns its run
id only after private authority and `run_started(seq=1)` are fsynced, not after
long-running execution. Admission uses one explicitly configured, persisted,
immutable hash-linked workspace policy generation; its closed initial state is
disabled, there is no implicit active/queue default, and each start decision,
`run_started`, and `run_activated` binds the exact generation used. Disabled
rejects fresh starts and pauses queued activation without canceling admitted
work. In one admission critical section, active count is
non-final ledgers with `run_activated`; queued count is ledgers whose latest
projected status is exactly `queued`. Unactivated `canceling` or `failing` runs
are not queued and can never activate. Configured `maxActiveRuns` alone gates activation; older
eligible queued runs drain first, then remaining capacity may immediately append
`run_activated` for a fresh start even when `maxQueuedRuns=0`.
`maxQueuedRuns` alone gates a fresh queued admission. Both are closed JSON
integer ranges. A full queue records stable `run_queue_full` with the policy ref
before a run directory or event exists. Admission counters are fsynced before
publication and never reused. Restart reconstructs current counts/FIFO from run
ledgers. Policy refs explain the exact governing generation; `authoritySchema=2` does not
invent a workspace-global historical decision order that it does not persist.
Queue wait counts against the immutable run wall clock. Browser or Archon
disconnect cannot stop admitted work. `run_started` alone projects queued;
`run_activated` projects running and precedes graph/dispatch events.
Every activated non-final ledger counts against `maxActiveRuns`, including while
blocked, `waiting_human`, canceling, or failing, and releases that slot only at an
execution-final event in this first contract.

The coordinator holds one renewable workspace-owner lease with a strictly
increasing `writerFence`. One host registry lock under the shared authority root
prevents two authority ids for one configured or opened root. The private owner
lock first validates
the immutable bootstrap plus mutable workspace authority-schema high-water mark;
unsupported readers remain strictly read-only and do
not fence, clean, or quarantine. Supported owners reserve/fsync counters before
publishing them, and hold that lock from current fence validation through every
authority write or bounded non-idempotent send/spawn/interrupt/cleanup call, so
takeover cannot race the effect. Historical fences form a monotonic prefix;
recovery preserves origin fence and records its higher state fence. ADR-0006
target leases remain separate host-resource ownership.
Registry generations publish only under the host registry lock; workspace,
lease, and command generations publish only under the owner lock. The mutable
records are complete closed RFC 8785 JSON in their sole target encodings:
`workspace-registry-jcs-v1`, `workspace-authority-jcs-v1`,
`workspace-owner-lease-jcs-v1`, and `run-command-record-jcs-v1`. Each carries
required `priorGeneration`; revision 1 uses `null`, and every later revision
exact-binds the complete previous bytes in the same named encoding by revision
and SHA-256. Only that authenticated expected predecessor permits exact-next
retry. The closed map is `registry.private.json=workspace-registry-jcs-v1`,
`workspace.private.json=workspace-authority-jcs-v1`,
`owner.private.json=workspace-owner-lease-jcs-v1`, and
`commands/<commandId>.json=run-command-record-jcs-v1`. A pre-freeze/experimental
record that reuses the target schema while omitting `priorGeneration`, or any
permissively decoded legacy bytes that do not exact-match the named encoding, is
malformed, non-authorizing, and not auto-upgraded. Compatibility parsing may
expose inspection evidence only; it cannot enable
`formations.workspace-authority.v1`.
This contract authorizes no shipped/live record migration. Revisions, fences,
and admission sequences are JSON-safe positive integers; exhaustion fails closed
rather than rounding, wrapping, or reusing identity.

The target is `CurrentBoardSchema=2`, `CurrentLayoutSchema=1`,
`NewBoardSchema=1`, and ledger event schema 2. Schema-1 Formation ports remain readable
with explicit in-memory defaults (`work`, the stable full initial
`acceptedMediaTypes` set of `text/plain`, `text/markdown`, and
`application/json`, plus required `data` inputs). Pure reads never write;
Tool-free new boards remain schema 1; ordinary non-Tool writes preserve schema;
and only the first successful Tool creation writes the canonical board-schema-2
migration. Board schema 2 is monotonic: deleting the final Tool never downgrades
it, and Tool update/delete remain board schema 2. Fixed Mission `out` accepts only
`text/markdown`;
Gate `in`/`pass` work ports use the full set. Gate `fail` is `gate_feedback` and has no media set. A
legacy fail edge into a work
input loads as degraded for inspection but cannot validate or run until rewired;
annotated-work pushback is never inferred. Old ledgers project with their
recorded schema-1 semantics and are never reinterpreted as typed feedback. A
safely normalizable schema-1 board may start from an immutable normalized
board-schema-2 run snapshot without rewriting the canonical board; source/snapshot
schema are recorded. Schema-1 runs are inspect-only and resume returns
`legacy_run_requires_new_run`.
Schema-1 inline Formation verification is retired by ADR-0008. Its existing
verdict lacks exact attempt/output and replay-safe revision identity, so
validation, Mission start, isolated Formation start, and resume fail
`legacy_inline_verification_requires_migration` before run artifacts or work.
New definitions and `verification_verdict` events are rejected; historical
definitions and verdicts remain inspection evidence only. Authors create a
replacement Gate, wire a named Formation output to it, then name that Gate in
the explicit removal request. Cancellation and failure may still close a
historical run without resuming, routing or dispatching legacy verification.

Ledger event schema 2 includes ADR-0007 command identity, workspace/fence authority, the
hash-bound Formation result, root projections, and authored-config manifest
before first admission. Schema numbers alone grant no capability: admission
waits for the complete projector/coordinator and a certified rollback set whose
binaries all honor the bootstrap/workspace-authority guard; pre-guard binaries are prohibited
runtime rollback targets. A later authority-changing extension requires a schema
bump; an older reader may inspect only a certified safe projection and never
fences, cleans, quarantines, executes, or finalizes. Registered projection-only private extensions remain non-authorizing
and absent from all public projections.
The mutable workspace authority schema is a monotonic high-water mark. An upgrade
advances/fsyncs it under the current fence before new-schema authority is written;
it never decreases, and a new schema starts a new run without reinterpreting old
ledgers.

## Required invariants

1. **One model, multiple surfaces.** UI gestures and `archon` definition verbs
   use the same files/shared package; runtime verbs submit to one coordinator.
2. **Definitions and runtime have distinct authority.** Workspace files are
   canonical for definitions; fenced private coordinator state is canonical for
   event-schema-2 execution. Browser state is never durable truth.
3. **Ledger is canonical for run history.** Status is projected from append-only
   events written under the current workspace fence.
4. **Stable ids matter.** Node, port, edge, slot, agent, board, mission, tool,
   gate, and run ids must survive round-trips.
5. **Layout is not structure or an automatic side effect.** Node positions and
   wire lanes live in layout sidecars, not board definitions. Only new elements
   receive the shared connection-aware/free-space/grid-snap placement heuristic;
   existing user arrangement changes only through direct manipulation or the
   explicit UI/Archon `arrange` action.
6. **Execution context fails loud.** Production Formations uses the exact
   Terminal-session resolver and configured inventory used by Terminal tabs,
   including its explicitly configured user/socket sources. Missing, ambiguous,
   busy, attached, stale, or already leased sessions, harnesses, checks, cwd, or
   agents cannot silently substitute or fall back to a Formations-only source.
7. **Formations is always-on.** It is a permanent first-class surface, not a
   feature flag. The executor safety ladder uses
   `CHROTE_FORMATIONS_LAB_*` / `CHROTE_FORMATIONS_TMUX_*` for disposable
   execution environments, never feature availability or production cockpit
   access. Historical Script-Gate environment variables authorize nothing.
8. **Beads can anchor missions; it is not the graph store.**
9. **No Gate-owned command execution.** Free-text criteria never become shell
   execution. Authored presence of legacy `command`, `commandArgv`,
   `commandShell`, or `commandCwd`—including empty values—is read-only
   inspection/migration-plan input and fails
   `legacy_script_gate_requires_fenced_migration` before authoring or execution.
   CHROTE does not resolve, normalize, or run it.
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
the safe descriptor, and records its stable `artifactId` in ledger-event-schema-2 fields. The
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

A board stores one exact immutable `(profileId, profileVersion)` tuple and
modeled non-secret parameters, never executable text. Lookup is exact tuple
equality with no ranges, aliases, defaults, fallback, or latest selection. The
current closed registry contains only the data-only `json.normalize@1`
descriptor: declared ports, bounded parameter schema, profile metadata, and
projection labels, with no executable path, argv, shell, cwd, environment,
secret, callback, or process constructor. Run start later freezes that exact
tuple plus the matching profile content hash,
normalized parameters/hash, effective policy and
determinism-policy hashes, and
immutable execution-bundle hash in a `RunToolBinding` inside host-private run
authority. The content-addressed bundle covers executable,
script/toolchain identity, argv template, cwd contract, normalized non-secret
allowlisted environment values, supervisor/fence policy, and limits; a mutable
path is not an identity. Until that runtime lands, Mission preflight first
validates every selected reachable Tool against the exact current descriptor
and then returns `tool_execution_unavailable`. The authoritative Mission Store
evaluates revision/ETag CAS before this semantic fence. Disconnected Tools do
not broaden a selected Mission. There is no isolated Tool-run endpoint, and
isolated Formation execution keeps its singleton root without traversing
downstream Tools.

Non-executing Tool authoring keeps layout schema 1. The first successful Tool
creation is the only board schema-1-to-2 authoring migration; update changes only
title and the complete parameter map. Delete removes the Tool, every incident
board connection, its layout node, and every incident layout routing entry while
keeping board schema 2. Create and delete hold board then layout locks, validate
revision/ETag CAS, and stage and fsync every present member of exact old/old and
new/new identities before publication. Each identity member is
explicitly absent or SHA-256 over exact bytes; absent is not an empty file.
Restoring absent layout means unlink/no-file plus layout-parent fsync. Any
earlier validation, migration, serialization,
staging, or fsync failure leaves canonical board and layout bytes unchanged.
Publication renames/installs layout (or establishes its no-file state), fsyncs
the present canonical layout file or confirms absence and fsyncs its parent, and
only then renames/installs board and fsyncs the canonical board file and parent.
Layout is non-authorizing and the board is graph authority.

After the first rename, an I/O error is synchronously reconciled under both
locks. Exact hashes alone are insufficient. Old/old returns ordinary failure
only after every present canonical file and both parent directories fsync, using
the absent-layout rule above. New/new reports success only after both canonical
files and both parent directories fsync in layout-before-board order. Rollback
after board publication restores/fsyncs the old board before the old layout. Any
mixed pair or failed sync returns `definition_publication_uncertain`, never
ordinary failure or success. Without a
journal this is not a durable mutation block, but automatic retry is forbidden.
The next explicit Tool mutation reopens and validates the canonical pair and
fsyncs every present member and both parents under both locks before CAS.

Cross-file crash/power-loss atomicity is not claimed without a future journal.
The durable crash states are old/old, layout-new/board-old, or new/new;
board-new/layout-old cannot arise from publication or reverse-order rollback.
Layout-new/board-old projects the old board. UI/graph projection joins positions
and lanes only for ids in that board; missing entries receive only the normal
non-authorizing placement heuristic, and the next successful Tool mutation
filters inert extras. Ordinary stale layout remains non-authorizing presentation
state.
Layout raw entries grant no node or Tool authority.

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

Ordinary workflow Formations use the same result-before-output discipline without
reparsing mutable capture. Each `slot_result` first fsyncs a closed, hashed
`slot-turn-result-jcs-v1` payload/output envelope sufficient for the frozen
formation-type rule. The schedule is sole terminal (`solo`), persisted-order
handoff with last terminal (`flow`), persisted-order peer round plus first-peer
facilitator (`peer`), or controller plan, persisted-order worker turns, and
controller synthesis (`orchestrated`). Every turn is coordinator-dispatched.
Each dispatch binds the same attempt's `nodeStartedSeq` and an exact ordered
prior-result sequence/hash array: none for `solo`, first flow, peer contributions,
or leader plan; the immediate predecessor for later flow; all peer contributions
for facilitation; the plan for each worker; and plan then all workers for leader
synthesis. Every phase also consumes the frozen node inputs named by that start
event.

Only an `ok` terminal turn may carry all and only declared outputs. Non-terminal
and non-`ok` turns have `outputs={}`. All required `ok` turns map to `done`; first
`error` stops and maps to `failed` with its normalized non-routable error at every
declared port; first `needs-review` stops and maps to `needs-review` with fixed
non-routable projection `{availability="available",exact=true,
payload={kind="unavailable",code="formation_needs_review",
message="Formation requires review",retryable=true}}`. Closed invalid output
schema becomes `{availability="available",exact=true,payload={kind="error",
code="invalid_formation_outputs",
message="Formation outputs do not match the declared ports",retryable=true}}`.
Timeout/unclosed output has no result or ordinary
release. Pre-result resource blocks create no Formation result. The complete
successful schedule or first non-`ok` prefix yields one result; its deciding turn
supplies the report and artifact/diff ids are the stable first-seen union over
that prefix. The result carries the complete
durable safe `outputs` map, `outputHashes`,
already-registered artifact ids, stable contributing slot-result sequences, and
`formation-result-jcs-v1` result hash.
`node_output` exact-matches that result sequence/hash. Recovery may append the
missing output once from it. Fresh Redact=true execution may keep the paired raw
value in process memory across safe result/output/Gate/join/dispatch fsyncs until
all scheduled internal turn consumers and taken-edge consumers send once or
become durably non-deliverable; it is then
erased and is always lost at cancellation/finality/process loss. Recovery fails
terminally when a required value was discarded.

Every terminal failure uses one two-event lifecycle. The writer first fsyncs
`run_failure_reconciliation_started`, freezing the exact code/reason/cause and
the remaining `unrecoverable`/`relatedSeq` fields of the closed failure header,
plus all open node-attempt, slot-dispatch, and Tool-lease snapshots; the run projects
non-final `failing` and admits only reconciliation. It then appends
`run_failed.failureReconciliationSeq` naming that start, with byte-equal
cause/header and exact dispositions for all three snapshots. Recovery resumes
those same snapshots, never selects a new cause, and never resends a durable
slot-interrupt request.

Canonical cancel first fsyncs `run_cancel_requested`, whose open-attempt, open-slot, and
lease snapshots block new
dispatch/replay and makes the writer reject launches, results, outputs, and
routing except cancellation finality. It soft-interrupts only an exact unresolved
dispatch proven on its frozen target, never kills tmux sessions, records every
slot disposition, cleans never-launched leases without execution, and fences every
launched Tool scope before cleaning its root and appending `run_canceled`. A
scope that cannot be proven quiescent by deadline instead causes terminal
`run_failed(code=tool_process_not_quiescent)`. Its failure start names the
original cancel request, preserves every cancel-time slot identity and prior
interrupt request/outcome (missing outcome is `send_uncertain`), and never sends
a second coordinator reconciliation interrupt; only a still-open slot with no prior request may receive one
failure-authorized request. The supervisor retains private cleanup ownership
after finality. Later proof may remove or quarantine an unredacted root; a
redacted root must be sanitized/removed and cleanup fsynced before deleting its
obligation. Neither path may promote, record a result, or rerun. Cancellation
also disposes every snapshotted node attempt, including a Gate waiting for human
input, and rejects a later decision.
Current main exposes `abort`; the accepted target adds canonical `cancel` and
normalizes `abort`/`stop` aliases before command hashing. An alias never creates
a second request snapshot, event, or lifecycle state.
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
Gate-owned argv/shell process evaluation is retired. Authored presence of any
legacy Gate command field (`command`, `commandArgv`, `commandShell`, or
`commandCwd`), even an empty value, remains inspectable but is not safely
normalizable. Board validation reports
`legacy_script_gate_requires_fenced_migration`. Mission preflight and resume
report the same stable error before `run_started` or `run_resumed` only when the
frozen selected Mission root can reach that Gate. Unreachable legacy Gates and
all Gates outside an isolated Formation root remain board-validation errors but
do not block that selected run.

The inspection projection is non-mutating and non-authorizing. It records board
identity/revision/ETag, Gate id, source mode and field names, affected edge ids,
the stable error, `targetKind=tool_plus_pure_gate`, `ready=false`,
`applySupported=false`, and closed unmet requirements. It never copies raw
command values, resolves executable/cwd/environment, generates Tool ids or
ports, or suggests a profile. A future explicit apply depends on non-executing
Tool definitions and registry descriptors owned by `ctx-ug7.8.1`, certified
host-private Tool implementations and runtime execution owned by `ctx-ug7.8`,
and pure code-Gate profiles plus the migration write surface owned by
`ctx-ug7.30`. Until all three land, no apply surface exists.

The future composition is Tool → pure code Gate. The Gate evaluates and forwards
the exact Tool output, not the Tool's upstream input, so the eventual apply must
explicitly validate profile/parameter/port/media/downstream compatibility while
preserving Gate identity, criterion, judge relationships, pass/fail edges, and
existing layout. If that mapping cannot be proven, the source board remains
unchanged and migration fails loud.

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
discarded, first append `type=run_failure_reconciliation_started` with
`data.originCancelRequestSeq=0`, the exact failure header/cause, and complete
`data.openNodeAttempts`, `data.openSlotDispatches`, and `data.openToolLeases`.
Then append terminal `type=run_failed` with
`data.failureReconciliationSeq` naming that start,
`data.code=redacted_input_unavailable`,
`data.reason=redacted_input_unavailable`, `data.unrecoverable=true`, and
`data.final=true`; `data.relatedSeq` identifies the exact source event whose raw
value was required and is not a cause selector, `data.failureCause={kind=none}`,
and `data.nodeAttemptDispositions`, `data.slotDispatchDispositions`, and
`data.toolLeaseDispositions` exactly dispose those three frozen snapshots (each
array may be empty). The run cannot resume or open another epoch; retry means a
new run with newly supplied authoritative input. Never dispatch a redaction
marker or summary in its place.

ADR-0005 defines this boundary and its rejected alternatives.

## Run-bound sessions and local terminal view state

A board slot stores staffing intent: label, stable agent id, and optional harness
constraint. It stores no tmux target, and `assigned` does not mean runnable.

Production resolution uses the same configured Terminal-session resolver and
inventory as cockpit Terminal tabs. The inventory is the union of explicitly
configured user/socket sources. A persona stem matching more than one source is
ambiguous and fails loud; board files never choose raw sockets. Accumulated
session context is intentionally reusable; the evidence contract is
same-session-lineage rather than a clean session. A disposable inventory is
test/certification/dogfood isolation only, and proves the production seam only
when both Terminal tabs and Formations consume it through the same resolver.

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
run/dispatch/binding/node/attempt/slot before ledger dispatch. Before prompt
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
loud. A candidate is runnable only when unleased, unattached, and the certified
harness adapter's non-pane channel proves a closed/ready turn for the exact
fingerprint. Certified active work is `session_target_harness_busy`; missing or
non-unique proof is `session_target_readiness_unknown`; quiet output or prompt
text is never proof. Incomplete client/input monitoring is
`session_target_attachment_audit_unavailable`. All fail at slot binding and final atomic acquisition with
a stable unavailable reason; a connected hidden CHROTE Terminal iframe is
attached. Binding never detaches it. A user may explicitly disconnect a
CHROTE-owned presentation client before retrying, but cannot reclaim an external
client or another run's lease. Formation attachment ownership begins only after
the exact target-registry occupancy is fsynced; the final atomic acquisition
repeats the check and never steals, creates, or selects an alternate target.
Stock tmux on an owner-accessible raw socket is not certified merely by a
CHROTE mutex; `ctx-ug7.21` must select, `ctx-ug7.22` must implement, and
`ctx-ug7.23` must certify a same-pool enforcement primitive before that adapter
can dispatch. After exact acquisition, the certified boundary
drains its durable interaction journal and installs the one-send
`target-dispatch-input-barrier-v1`. The coordinator records a fresh
`target-ready-proof-v1` bound to it, then the closed
`tmux-pane-history-baseline-v1` token and SHA-256 in writer-private
`slot_dispatch` before send. The token binds target fingerprint, capture epoch,
absolute byte cursor, and frozen terminal grid without storing pane bytes.
Result capture starts at that exact boundary. History trim/reset, pane
replacement, resize/reflow, restart without proven cursor continuity, or an
ambiguous boundary fails `capture_baseline_unavailable`, produces no
`slot_result`, and does not ordinarily release the target. Sanitized projection
exposes only encoding/hash/validation state. Prior pane history remains agent
context but cannot satisfy this dispatch's sentinel or output. Completion also requires the
sentinel to be terminal and a certified
harness-ready/closed-turn proof for the frozen fingerprint; trailing old output
keeps occupancy. Success waits for the durable `result_committed` receipt
carrying that proof. Release atomically replaces occupancy with exact
non-occupying crash proof retained through final-event fsync. Result-closed
dispatches require `result_committed` receipts and never become slot
dispositions. For each unmatched slot, cancel/failure records a
`final_quiescent` receipt only with an exact dispatch-bound cancel/ready
acknowledgement or old-pane-gone proof; otherwise it retains a non-authorizing
busy hold or fail-closed quarantine. Holds/quarantines remain until exact
old-dispatch quiescence is proven. A foreign-attachment/input or lost-audit
latch cannot use the cancel/ready branch because historical continuity is gone;
only exact old-pane-incarnation-gone proof releases it. Receipts do not block later safe acquisition.
A sent interrupt, silence, time/name/PID expiry, or interleaved pane use is not
proof.
If a ledger dispatch lacks its expected private lease, the arbiter first
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
still own active unmatched occupancy while the run is non-final and before
cancel/failure reconciliation. A terminal hold is non-authorizing evidence and
permits no run-bound input. After a release
receipt or quarantine, show captured history or `pane_moved_on`; opening the
current reused session is a separate non-run action.
An authorized live Peek is a full interactive user attach and may send literal
input, including control characters, to steer or interrupt the exact agent. It
is not automatic workflow dispatch: workflow prompts, retries, coordinator
interrupts, and lifecycle transitions remain coordinator-only. Only a
CHROTE-issued capability exact-matching the durable run/dispatch/target
occupancy may send. Only the latest fsynced issuance is valid; a newer issuance
requires prior clients/input drained, invalidates every older token/generation,
and makes a superseded route reaching the target boundary foreign. Before its first forwarded bytes, a steering generation is
fsynced without raw keystrokes; result closure waits for that generation to
close and requires a fresh certified proof that terminal bytes alone cannot
forge. Later input invalidates earlier proof, and inspection visibly marks the
turn operator-influenced. Restart suspends input until recovered occupancy is
revalidated.

Shared Terminal attach paths reject ordinary attachment after occupancy. A
certified private monitor accounts for every attach/detach/target-selection,
resize/reflow, history, pane-lifecycle/topology/other mutation, and input-capable
tmux command/control route affecting the pane; authorized Peek
metadata requires durable capability issuance and bytes traverse the
steering-generation gate. The only other routes are the one-shot workflow
prompt and one durable/no-resend reconciliation interrupt. A foreign event or lost continuity revokes/drains Peek, forbids result/ordinary
release, and holds or quarantines the target pending non-authorizing quiescence
proof. The stable `foreign_input` class includes any unregistered pane mutation
or input, including history and lifecycle/topology changes. Normal closure, cancel/failure reconciliation, and finality stop
capability issuance, drain input, close open steering, and fsync irreversible
capability revocation. Cancel/failure Ctrl-C requires its own exact
ledger-before-send permit and missing outcome is never retried. Closure binds
revocation and continuous interaction audit under a mutation/input fence through
receipt fsync; no run-bound capability survives finality.

Peek movement and tile resize are browser-viewport changes only while a dispatch
is active: the terminal grid recorded in the baseline stays frozen and no tmux
resize/`SIGWINCH` is sent. A real resize invalidates the baseline. Closing Peek
never creates, kills, or rebinds the session.
Current main has only partial name-level binding evidence; exact run-bound Peek
remains target work.

## Interaction model

The reference interaction model is permissive direct manipulation:

- right-click meaningful elements for local commands;
- drag agents from roster into slots;
- create missions, formations, gates, and templates from the canvas;
- edit briefs and explicit Gates through local popovers; legacy inline
  verification opens a read-only migration view whose removal action requires
  an already-wired replacement Gate;
- connect, reconnect, route, and remove wires directly;
- place only newly created elements heuristically and run full layout only from
  the explicit Arrange action; never auto-arrange existing user work;
- start missions and see work cascade through the graph;
- project run status onto cards, gates, wires, and outputs;
- support undo for structural mutations.

Current `displayLayoutFor` renders persisted coordinates verbatim, including
intentional overlaps. Missing positions receive only a non-authorizing display
fallback. Creation-time placement writes only the new element; render, open,
save, connect, validate, run, replay, and reconnect never move existing user
arrangement. Full-board reflow occurs only through explicit Arrange.

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
9. Runs can be inspected, followed, canceled, and—when their terminal state and
   evidence contract permit it—resumed from `archon` and reflected in the UI.
   `run_failed(code=redacted_input_unavailable)` is final and cannot resume.

Watching is optional. Recovery must not depend on a browser tab staying open.

## Execution environments

Formations execution promotes through explicit execution environments. Each
step up is an explicit configuration decision, never a silent fallback.

1. **Lab.** `CHROTE_FORMATIONS_LAB_*` configures a deterministic executor that
   synthesizes outputs and sentinels with no tmux involvement. Full run-engine,
   ledger, gate, and recovery behavior is exercisable here. When lab harnesses
   are configured, lab takes precedence over the tmux executor.
2. **Isolated tmux dogfood.** `CHROTE_FORMATIONS_TMUX_*` dispatches to real agent
   sessions, but the executor refuses to run unless the socket, cwd, and all
   roots live under fixed `/tmp` and the socket is not the
   default-resolved or configured Terminal socket. No legacy environment flag
   lifts this restriction. Dogfooding happens on a throwaway socket and
   temporary workspace with its own sessions. Known live socket identities and
   observed between-call retargets fail closed, but same-UID dogfood remains a
   trusted test boundary rather than construction-level isolation. It must not
   be treated as the production session-pool design or certification.
3. **Retired Script-gate process path.** No environment flag enables Gate-owned
   argv/shell execution. Legacy Gate command fields are inspectable only and
   produce the stable migration error at validation, selected-root preflight,
   and selected-root resume. Process-backed deterministic work belongs to a
   future execution implementation under `ctx-ug7.8`; current host-profile Tool
   definitions are non-executing, and code Gates remain certified pure
   in-process evaluators.
4. **Shared cockpit execution (accepted contract, currently unavailable).**
   Production Formations must consume the same Terminal inventory resolver and
   session pool as Terminal tabs. The stock tmux adapter cannot yet arbitrate
   connected/busy clients or prove the complete attachment, mutation, input,
   pane-history, and closure journal required by ADR-0007. A non-temporary or
   configured cockpit target therefore fails before any tmux client call with
   `session_target_attachment_audit_unavailable`; it cannot list, capture, send,
   detach, create, or kill through the Formations executor. No legacy
   `PROD_SMOKE` or `DEDICATED` value authorizes this path. `ctx-ug7.21` selects,
   `ctx-ug7.22` implements, and `ctx-ug7.23` certifies the missing same-pool
   input fence. Only that certified implementation may replace the unavailable
   result; it must not reintroduce a Formations-only production socket. The
   disposable adapter records its initial socket identity and revalidates it
   before every adapter operation, so an observed between-call path retarget
   blocks the next list, describe, capture, reattach, or send. This is
   defense-in-depth for trusted dogfood, not a same-UID stock-tmux fence: racing
   within a command or independently using an owner-accessible raw socket
   remains possible and cannot be cited as production certification.

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
  judgment; retired inline verification has no execution or routing authority;
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
