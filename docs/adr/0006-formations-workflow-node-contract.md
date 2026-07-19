# ADR-0006: Formations Has Four Explicit Workflow Node Kinds

## Status
Accepted target; implementation is incomplete; runtime ownership amended by ADR-0007

## Context
The current Formations board model has missions, agent formations, gates, stable
formation ports, file-backed connections, and run ledgers. It can execute useful
agent graphs, but it does not yet define the full mixed-workflow contract needed
for agent-first authoring and inspection.

Several ambiguities are load-bearing:

- there is no deterministic Tool node, so a script gate can be mistaken for a
  general transformation step;
- formation ports are named, but their accepted payload kind is implicit;
- writer operations reject a second edge to one input, while malformed files do
  not yet receive the same whole-board preflight;
- gate pass and fail routes can rewrite provenance or forward ordinary work
  without distinguishing verdict feedback;
- judge prose can be confused with the payload being judged;
- board slot assignments and runtime tmux targets can look equally runnable even
  though only the latter identify a live pane.

A generic plugin-node schema would hide these differences from Archon and the
canvas. The workflow instead needs a small model whose semantics remain legible
in files, the CLI, the engine, the API, and the UI.

## Decision
Formations defines four workflow node kinds.

| Node kind | Responsibility | Routing behavior |
| --- | --- | --- |
| Mission | Run entry and declared input | Emits one initial work payload on fixed `out` in this phase |
| Formation | Agent execution in `solo`, `peer`, `flow`, or `orchestrated` coordination | Consumes declared inputs and emits agent-produced outputs |
| Tool | Bounded deterministic transformation through a host-owned versioned profile | Transforms declared inputs into declared outputs without agent judgment |
| Gate | Human, code, or formation-judge evaluation and routing | Records verdict metadata and chooses pass/fail; it does not transform work |

Mission nodes do not accept incoming workflow payload edges. In this phase the
authored objective becomes one `mission-objective-utf8-v1` payload: bounded UTF-8
with BOM rejected, CRLF/CR normalized to LF, no other whitespace/newline change,
`mediaType=text/markdown`, and SHA-256 over those exact bytes. The fixed `out`
accepts only that media; every first destination must accept it or preflight fails
`mission_objective_media_incompatible` before `run_started`. Run start binds that
validated `work` payload to the existing stable `out` port;
Mission `node_output.outputs[out]` and unchanged downstream deliveries use only
the classified root-derived projection exact-matching the same run's
`rootInputProjection`; a generic unclassified exact copy is invalid.
Multiple Mission outputs and arbitrary port-keyed start maps are deferred.
Formation and Tool nodes may declare multiple input and output ports. Gate nodes
retain stable `in`, `pass`, `fail`,
and the reserved `judge` evaluation-control socket. `judge` is not a typed
workflow payload port: it permits exactly one judge send and one judge return or
neither on an executable board, and carries evaluation control/evidence only.
Schema-2 judge edges persist `channel=judge`; ordinary edges default to
`channel=workflow`. Generic wiring, fan-out, JOIN, and payload-kind rules do not
apply to the judge channel. Structural validation rejects extra judge
producers/consumers or cross-use, while executable preflight rejects an unpaired
or non-linear judge path. `in` accepts `work`, `pass` routes that same `work`, and `fail`
routes the evaluation's `gate_feedback`; `pass` and `fail` are not replacement
work producers. An executable Gate has a complete judge channel if and only if
its `kinds` contains `formation`. Draft editing may temporarily persist one half
or a kind/channel mismatch, but validation and run preflight reject the mismatch.

Mission, Formation, and Tool outputs produce only `work`; Tool inputs accept
only `work`. Formation inputs may accept `work` or `gate_feedback` as declared,
but only the fixed Gate `fail` port can produce `gate_feedback`. Shared read,
write, and preflight validation reject every other feedback producer, so an
ordinary node cannot mint Gate verdict or retry authority.

Every run records a `runRoot`. A Mission root executes its reachable graph,
including reachable Tool nodes and Gate judge chains. An isolated Formation root
is executable only when that Formation has one non-empty brief and exactly one
required `data` input accepting `work` and `application/json`. Incoming board
edges are outside the isolated root. Preflight encodes the parsed brief as
`formation-brief-jcs-v1`: RFC 8785 canonical UTF-8 JSON over exactly `{goal,
beadId, files, links}`, with missing scalar/array fields normalized to `""`/`[]`,
array order preserved, and no trailing newline. Those exact bytes become one
synthetic `run_seed` work input with `mediaType=application/json` and determine
its `seedSha256`. Its durable projection is the classified root-derived variant
exact-matching the same run's root input; a generic unclassified copy is invalid.
Optional inputs remain absent,
`retry_control` is never seeded, and downstream edges are not traversed. Zero,
multiple, or non-work required inputs reject before `run_started`.
Binding/profile preflight is scoped to the selected root's executable subgraph,
never unrelated board nodes.
For either root, `run_started.rootInputProjection` uses the exact closed flat
shape in `spec/contracts.md`; its `sha256` hashes the same canonical Mission
objective or Formation-brief bytes and its `text` contains those same canonical
UTF-8 bytes. It is not a `PayloadProjection`; its only permitted durable payload
copy is the classified root-derived variant above, restricted to Mission
`out`/unchanged deliveries or isolated `run_seed`.

A Formation remains an embedded agent-execution node in the board for this
phase. Templates may create or copy one, but runs do not dereference a second
versioned formation-definition store. Reusable referenced Formations can be
designed later if copying proves insufficient.

Canonical run authority is writer-private. The append-only ledger, normalized
graph snapshot, private bindings, sealed Tool inputs, and pending raw-redaction
state live under the configured CHROTE data root outside every generic Files
read/write root. `run_started` stores an opaque authority id plus exact graph,
private-binding, and safe-projection SHA-256 values; every execution/recovery read
rechecks them. Generic Files cannot list, read, mutate, rename, or delete this
tree. Run/event/SSE APIs expose sanitized projections, and only currently
authorized registered artifacts may use the existing File Peek renderer. A
same-named workspace file, altered export, raw ledger record, or pending cleanup
locator is never authority. ADR-0007 fixes one fenced workspace coordinator,
durable command admission, bounded queueing, and authority-schema compatibility;
this ADR defines the per-run/node/resource boundary, and `ctx-ug7.6` owns their
combined implementation.

The graph snapshot may copy exact Mission objectives, Formation briefs, and Gate
criteria only when its embedded, hash-covered `authoredConfigManifest`
classifies the exact field/node as `authored_config` with a closed source role,
versioned encoding/media type, and SHA-256. Missing, extra, or mismatched entries
are invalid. Mission objective and Formation brief use the
root encodings above. Gate criteria use `sourceKind=gate_criterion`,
`gate-criterion-utf8-v1`, and `text/plain`; that text encoding rejects a BOM,
normalizes CRLF/CR to LF, and otherwise preserves bounded UTF-8 bytes. Durable
human-request prompt and PASS/FAIL choices use closed fixed-system templates,
never arbitrary authored text. Those versioned template ids are immutable; text
changes require a new id and unknown ids fail before append/projection. This
narrow ADR-0005 exception is configuration
already durable in the board and may outlive later edits/deletion.
Runtime/external values and composed prompts are never covered by the exception.

A private authority directory without valid seq-1 `run_started` is not admitted
and sends nothing. A supported current fenced owner validates the historical
origin fence, claims cleanup with its higher state fence, then cleans/fsyncs every pending raw target and
obligation before deleting the orphan tree and fsyncing its parent. Unprovable
cleanup/identity quarantines it as non-authorizing with no public bytes or replay
handle; it is never adopted as a run.

An ordinary Formation parses each bounded closed turn once and fsyncs its
redaction-safe `slot-turn-result-jcs-v1` payload/output envelope and hash inside
`slot_result` before releasing that target. From those immutable turn results
and the immutable graph snapshot's fixed formation-type rule it derives one
fsynced `formation_result`
before `node_output`. That result carries the complete safe canonical `outputs`
map, `outputHashes`, registered artifact identities, contributing `slot_result`
sequence ids, and a hash over the closed `formation-result-jcs-v1` envelope.
Recovery derives a missing result or materializes a missing
`node_output` from that immutable result and never reparses mutable capture or
resends completed work. ADR-0007 fixes the bounded solo/flow/peer/orchestrated
turn schedules, exact ordered prior-result seq/hash inputs for every dispatch,
coordinator-mediated worker dispatches, first-non-ok status mapping, and closed
fixed needs-review/invalid-output projections. With Redact=true, only the safe
projection is durable; a fresh paired raw value may survive safe fsyncs in
process memory until all scheduled internal and taken-edge consumers send or
become non-deliverable.
If recovery later requires discarded raw bytes, the run terminates with
`redacted_input_unavailable`.

### Non-executing Tool authoring foundation

`ctx-ug7.8.1` adds definition, authoring, validation, and projection only. Until
the certified runtime slice lands, `tool_execution_unavailable` is the sole
non-executing Tool runtime code. The API maps it to HTTP 422 and Archon JSON
returns the same lowercase code. There is no isolated Tool-run endpoint.

Run admission parses the board, resolves the requested Mission or isolated
Formation root, and applies existing structural and legacy-migration validation
first. It then scans that root's complete possible graph. A Mission scan includes
both Gate branches and reachable judge chains; an isolated Formation scan does
not traverse unrelated downstream board nodes. If the selected root contains a
Tool, admission returns `tool_execution_unavailable`, and only after that check
may it check global runtime authority. This precedence applies only against the
global authority error; malformed definitions and legacy migration errors retain
their existing precedence. Rejection occurs before snapshot, binding, ledger,
artifact, evaluator, executor, or process mutation.

Tool identity is exact and immutable. `profileId` is case-sensitive ASCII, at
most 128 bytes, and matches
`^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)+$`. `profileVersion` is an opaque,
case-sensitive ASCII token matching
`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`. Registry lookup is exact tuple equality on
`(profileId, profileVersion)` with no ranges, aliases, defaults, fallback, or
latest selection. Port and parameter machine names match
`^[a-z][a-z0-9_]{0,63}$`.

The registry is private, compiled, immutable, and data-only. It exposes only
deterministic lookup/list copies and has no runtime registration API. Its closed
shapes are:

```text
ToolProfileDescriptor {
  profileId: string
  profileVersion: string
  displayName: string
  ports: ToolPortDescriptor[]       // ordered
  parameters: ToolParameterSpec[]   // ordered
}

ToolPortDescriptor {
  name: string
  label: string
  direction: "input" | "output"
  kind: "work"
  acceptedMediaTypes: string[]
  required?: boolean   // present only for input
  role?: "data"        // present only for input
}

ToolParameterSpec {
  name: string
  label: string
  type: "string" | "boolean" | "integer"
  required: boolean
  enum?: string[]
  minBytes?: integer
  maxBytes?: integer
  minimum?: integer
  maximum?: integer
}
```

String specs alone may use `enum` and `minBytes`/`maxBytes`; integer specs alone
require `minimum`/`maximum`; boolean specs carry none of those constraint fields.
Unknown or irrelevant fields reject. A descriptor cannot represent callbacks or
interfaces; executable, argv, shell, cwd, environment, path, bundle, or limits;
secrets; network or effects; runtime capability; defaults; or unmodeled
metadata. Board-authored data therefore never selects process authority.

A board-generated port `id` is instance identity, descriptor `name` is stable
semantic binding identity, and `label` is display-only. Tool board ports
reproduce descriptor order and exact name, direction, kind, ordered duplicate-
free `acceptedMediaTypes`, plus the exact presence and value of input
`required`/`role`. Clients never author Tool ports independently; creation
derives them from the descriptor. This slice's update surface changes only the
title and complete parameter map. The profile tuple and ports are immutable;
replacement is deferred.

A Tool has at most 16 parameters. RFC 8785 canonical JSON for the complete
parameter object is at most 4096 bytes. Values are only valid UTF-8 NUL-free
strings, booleans, or JSON-safe integers in
`[-9007199254740991,9007199254740991]`; TOML floats, datetimes, arrays, tables,
and nested values reject. Every key must be descriptor-declared and satisfy its
enum, byte-length, or range constraint. Registry compilation rejects any
lowercase machine name containing `secret`, `credential`, `token`, `password`,
or `passwd`, and rejects these exact reserved authority names: `api_key`,
`apikey`, `private_key`, `access_key`, `executable`, `exec`, `command`, `cmd`,
`argv`, `shell`, `cwd`, `env`, `environment`, `path`, `bundle`, `limits`,
`network`, and `effects`.

Descriptor binding compares ordered media arrays exactly. Shared routing treats
media arrays as sets and requires the producer's complete possible media set to
be a subset of the consumer set; nonempty intersection is insufficient. Work-
port arrays are nonempty, duplicate-free, and contain only `text/plain`,
`text/markdown`, or `application/json`.

Schema ownership is split exactly as `CurrentBoardSchema=2`,
`CurrentLayoutSchema=1`, and `NewBoardSchema=1`. Pure board and layout reads
never write. Empty and Tool-free new boards remain schema 1, and ordinary
non-Tool mutations preserve the board's existing schema. Under board/layout
locks and revision/ETag CAS, the first successful Tool creation on a schema-1
board first rejects ambiguous legacy fail or judge routes, inline verification,
and every legacy script-Gate shape before staging. It then publishes one
content-preserving board-file replacement that writes typed Formation defaults,
explicit safe `workflow`/`judge` channels, `schema=2`, and the Tool with one
revision increment. Layout remains schema 1; only the new Tool receives heuristic
connection-aware, bounded free-space, grid-snap placement, and existing
coordinates never move. The writer computes, validates, and stages both board
and layout bytes before publication and restores original
bytes for an ordinary returned failure, so invalid, CAS, or write failures leave
both files byte-identical. This contract does not claim cross-file power-loss
atomicity without a future journal. A schema-1 reader rejects board schema 2.

The initial and only descriptor is exactly:

```text
profileId      = "json.normalize"
profileVersion = "1"
displayName    = "Normalize JSON"

input port:  name="input", label="Report", direction="input", kind="work",
             acceptedMediaTypes=["application/json"], required=true, role="data"
output port: name="output", label="Normalized report", direction="output", kind="work",
             acceptedMediaTypes=["application/json"]
parameter:   name="mode", label="Mode", type="string", required=true,
             enum=["strict"], minBytes=6, maxBytes=6
```

`json.normalize@1` freezes authoring and validation shape only; it claims no
algorithm, runner, or output. The linter identifier, ports, parameters, media,
sealed source/result contract, and descriptor are absent and reserved for a
future owner product-direction decision. No placeholder is inferred here.

The later executing Tool contract freezes the exact profile version token,
profile content hash, parameters, effective policy and determinism-policy
hashes, and immutable
execution-bundle hash as one `RunToolBinding` per Tool in private run authority.
The execution bundle is content-addressed over executable,
script/toolchain identity, argv template, isolated-cwd contract, normalized
non-secret allowlisted environment values, supervisor/fence policy, and limits;
a mutable host path alone is not an identity. The first Tool contract is
certified pure and deterministic. Its closed sandbox exposes only a sealed input
set and frozen bundle/parameters/policy; denies network, secrets, undeclared
environment/filesystem reads, and external writes; normalizes locale/timezone;
freezes or denies clock/entropy; confines output to one private root; and passes
repeat vectors with expected output hashes. The host owns executable path, argv
template, isolated cwd,
environment allowlist, resource limits, process supervision/fencing, and replay
enforcement. Preflight rejects the Tool before `run_started` if the frozen
supervisor/fence policy cannot be provided. A completed
Tool lease is never rerun. Each lease id is unique within the run and maps
one-to-one to one Tool node attempt. Before `tool_dispatch`, the host validates
and copies every exact input into a no-follow, fsynced, atomically installed,
content-addressed read-only set under private run authority. Its manifest fixes
input ids, media, sizes, hashes, and object identities; a Redact=true obligation
owns it before raw bytes persist. The coordinator then appends/fsyncs
`tool_dispatch` with the manifest hash plus
input/profile/parameter/policy/determinism-policy/execution-bundle hashes and private lease-root or
redacted-obligation authority. Every spawn/rerun reads only that sealed set and
never reopens mutable source artifacts; a mismatch fails before spawn. For each actual process
start, the host supervisor first reserves and fsyncs a private per-generation
scope record plus an immutable launch-deadline authority. The latter binds the
same identity and derives one start time/effective deadline from the frozen
timeout policy before a public launch or process exists. The coordinator then
appends/fsyncs `tool_process_launch` with opaque scope/deadline-authority ids,
stable launch id, monotonically increasing generation, and
`recordedBeforeSpawn=true`; only then may the process spawn inside that scope.
The first public generation is 1. The writer requires unique launch, scope, and
deadline-authority ids and an exact `{runId, toolLeaseId, launchId, nodeId,
attempt, generation, processScopeId, deadlineAuthorityId}` match between the open
lease, reserved private records, and launch event.
The scope id is never a reusable raw PID; its private record holds the supervisor
identity/start fingerprint needed to fence that exact process tree.
The private scope/deadline reservation is one atomic/fsynced identity set. With no
matching launch, recovery fences then reuses that exact pair or deletes both and
directory-fsyncs before replacement; one half never survives or spawns. Before a normal-completion, startup-recovery, or cancellation path first
terminates, seals, or waits on a recorded launch, writer-private authority fsyncs
one exact Tool quiescence-boundary record. It binds the run, lease, launch, node,
attempt, generation, and scope to the matching boundary cause and exact deadline
authority; its start, policy, duration, and effective deadline must byte-match
that pre-spawn record. An unresolved boundary for that launch is reused across crashes and mode
changes; restart time never resets or recomputes the deadline. The Tool writes
staged output. After process exit, the supervisor must
seal the scope against new members and prove the whole descendant scope quiescent
by that persisted deadline; otherwise the same terminal
`tool_process_not_quiescent` rule forbids promotion and result.
Only after proof does the coordinator fsync output, promote it atomically, and
fsync the parent directory. It then appends/fsyncs one
`tool_result` keyed to the latest launch id/generation that atomically closes the
lease, registers every new Tool artifact in `artifactRegistrations`, and carries
the complete durable canonical port-output projection map needed to project
`node_output` without rerunning.
The supervisor then deletes the quiescent private scope record and fsyncs its
directory; success requires no remaining Tool scope record.
Each port entry states whether
an exact execution-authoritative payload remains durably available or only
hash-only redacted evidence remains; only the former can route during recovery.
Recovery of an open lease with no recorded launch may discard any orphaned
private scope and create generation 1 after the normal hash, input, and limit
checks; no process could have started in that ledger-before-spawn window. Once a
launch is recorded, its matching private scope is mandatory. Recovery removes
the entire staged or promoted-but-uncommitted lease root and may rerun only
after the supervisor terminates and seals that scope and proves the exact launch
generation quiescent by a bounded deadline. After proof and root cleanup, the
private record remains as a durable sealed/quiescent tombstone until either the
next launch or a terminal non-rerun event is fsynced; only then is the prior
record deleted and its parent directory fsynced. A crash before that decision
therefore still has the recorded generation's fence proof, while a crash after a
new launch has the new generation's scope. Cleanup and the next fsynced launch generation occur only
after proof and only when every recorded hash, including the execution bundle,
still matches. A recorded launch whose scope is missing or ambiguous,
or whose quiescence cannot be proven, causes terminal
`run_failed(code=tool_process_not_quiescent)` and never cleans, promotes, or
reruns that root. Each launch consumes dispatch and wall-clock limits while
retaining its enclosing logical node attempt; only a new node attempt consumes
`maxAttempts`.
At normal completion, startup recovery, and cancellation, the coordinator
freezes the complete set of launched Tool leases that miss their persisted
deadline and sorts `(effectiveDeadlineAt, dispatchSeq, toolLeaseId)`. The first
alone selects failure cause; all others are abandoned/private-cleanup-owned.
Callback, process-exit, goroutine, and map order never decide.
Canonical cancel first appends/fsyncs `run_cancel_requested`, whose exact open-attempt,
open-slot, and open-lease snapshots stop new dispatch/replay and make the writer reject new launches,
results, outputs, routing, or other execution authority except cancellation
reconciliation/finality. It soft-interrupts only a frozen target proven to host
that exact unresolved dispatch/attempt and never kills tmux sessions; every open
dispatch receives a canceled/non-authorizing disposition. A public lease with no launch is cleaned without execution. Every open
Tool launch is terminated, sealed, and proven quiescent; its root is cleaned
without promotion or result before `run_canceled` may become final. If proof
fails by the deadline, the outcome is
terminal `run_failed(code=tool_process_not_quiescent)`, not a successful
cancellation. Every snapshotted node attempt, including a Gate in
`gate_evaluating` or `waiting_human` with no open slot/Tool, receives a
canceled/non-authorizing disposition and cannot accept a later human decision.
The supervisor retains private cleanup ownership after that final
failure and may continue fencing. Only after later quiescence may it remove or
quarantine an unredacted root; a redacted root must be sanitized/removed and the
cleanup fsynced before its obligation is deleted. Scope records are then deleted,
with no promotion, result, rerun, or further execution event.
Current `abort`, plus any legacy `stop` spelling, becomes a compatibility alias:
the coordinator normalizes either to canonical cancel before request hashing,
so an alias cannot create a second command or state transition.
Every execution-final event revokes all remaining node-attempt, slot, and Tool
authority. Before accepting finality, the writer enumerates every host target
occupancy record and non-occupying release receipt for the run. A result-closed
dispatch whose exact `slot_result` is already durable must have an exact
`result_committed` receipt carrying its certified turn-closure proof and cannot remain at finality as a slot disposition,
terminal hold, or quarantine. Missing/conflicting private state may use a
temporary fail-closed quarantine, but it must become that receipt first. Every
unmatched dispatch must exact-match one final slot disposition backed by a
`final_quiescent` receipt with an admissible certified proof or an already-durable terminal hold/quarantine; an
unaccounted record or receipt rejects finality. Receipts retain release proof
across a crash, do not occupy the key, and are removed only after final-event
fsync. `node_started` opens an attempt; its normal kind-specific completion
closes it, while Gate evaluation and human-wait events remain phases of that
same attempt. Success is invalid with any open attempt, dispatch, Tool lease, or
host target occupancy record for the run; cancellation exactly reconciles all three public
snapshots and records each unmatched target lease as durably released after
quiescence, retained as a non-authorizing terminal hold, or quarantined when
private identity is missing/conflicting. Failure carries a typed
`failureCause` selecting one exact slot, Tool lease, scoped error, or no cause.
That cause resolves at most one open attempt as failed; every other open attempt,
slot, and Tool is abandoned. `run_failed.relatedSeq` remains context/provenance
only—ADR-0005 uses it for the discarded source-value event—and never selects the
causative attempt. All dispositions are non-authorizing; Tool scopes remain
owned by private post-final cleanup.
Final projection covers nodes that never reached `node_started` as well. For
each node in the frozen selected run root, the latest attempt completion or
disposition wins. A never-started node becomes `canceled` on `run_canceled` and
`abandoned` on `run_failed`. It may become `not_run` on `run_succeeded` only
when the ledger proves it was unreached or solely on an untaken branch; a
delivered input on a taken path makes success invalid. Earlier `node_waiting`
counts remain inspection evidence and cannot leave a final node actively
waiting.
Effectful Tool profiles are
deferred. Arbitrary board-authored Gate commands remain schema-1
inspection/migration-plan input and cannot be executed or relabelled as a Tool
step.

For `Redact=true`, ADR-0005 tightens this order: before any persistent Tool
target or public lease exists, the private pending-redaction registry records and
fsyncs an obligation owning that exact lease root. Only then may public
`tool_dispatch` append/fsync the opaque obligation id; it never exposes the
cleanup locator. The initial obligation is `pending` for generation 1, and the
writer accepts a redacted `tool_process_launch` only when its generation matches
that pending obligation. Process execution starts only after both writes. A crash
after private creation but before ledger dispatch leaves an orphan with no
executable lease; registry recovery cleans its exact root, deletes the entry,
and fsyncs the directory without executing the Tool. Finalization
sanitizes or removes raw targets, then fsyncs the private obligation from
`pending(generation=N)` to `cleaned(generation=N)` while retaining its locator.
The `tool_result` registration commit is accepted only when that cleaned
generation matches the latest launch and may contain only policy-safe
projections. After `tool_result` is fsynced, the registry entry is deleted and
its parent directory fsynced. Recovery therefore has the exact root for any open
redacted lease, including a crash after cleanup but before the result. After it
has fenced generation N and cleaned the root, a rerun first fsyncs
`cleaned(generation=N)` to `pending(generation=N+1)`, resetting the cleanup
state while retaining the locator; only then may it record and spawn generation
N+1. A crash before that launch leaves a safe prepared generation: recovery
cleans idempotently, fences any orphan N+1 scope without spawning, retains the
pending N+1 obligation, and validates before reserving or reusing generation
N+1. A closed lease with a leftover cleaned entry deletes that entry without
rerunning. `run_succeeded` requires no remaining redaction or Tool-scope entry.
Every `tool_result.outputs[portId]` is either an exact available
`PayloadProjection` or hash-only redacted evidence. Sanitized non-exact text,
refs, and summaries belong only in separate bounded display or artifact evidence
fields such as `tool_result.displayEvidence` and `tool_result.artifacts`; they
never occupy the output map, become an `ExecutionInputRef`, or authorize routing.
Fresh execution may still route a live ephemeral value held outside the ledger.
If recovery needs discarded raw input or output, it terminates with
`redacted_input_unavailable`; a hash or completed Tool lease never authorizes
reconstruction or rerun.

### Ports, connections, and joins

Every persisted workflow payload port has a stable id, display label, direction,
accepted payload kind, and—for inputs—a required flag plus `data` or
`retry_control` role. `retry_control` is Formation-input-only; Tool inputs use
`data`. A `retry_control` input is optional, accepts only
`gate_feedback`, and is the only in-graph port role that can trigger another
attempt. Explicit operator resume is the separate bounded trigger defined below.
A connection has a stable edge id, `workflow` or `judge` channel, and one exact
source node/port plus destination node/port. The reserved Gate `judge` channel is
the named exception to payload-port directionality; it cannot carry downstream
work. Its Formation endpoints are reserved from workflow edges for that board.

A port's accepted kind governs successful routable delivery. `unavailable` and
`error` may terminate production for any declared output as system outcome
envelopes; they are recorded under that port but are not delivered and are not
misreported as kind-mismatch errors.

The durable `node_output.outputs[portId]` map stores `PayloadProjection`, not
necessarily the authoritative bytes. An exact available projection contains the
routable `PortPayload` only when its kind is `work` or `gate_feedback` and the
destination accepts it; exact `unavailable`/`error` outcomes remain non-delivered.
A redacted projection contains only safe metadata/hash.
During fresh execution the engine may pair the latter with an in-memory
`ExecutionOutput` under the same port/attempt identity. It fsyncs the durable
output event before downstream dispatch and never persists that live value.
Recovery may route only a compatible exact available `work`/`gate_feedback`
projection; losing an ephemeral value
fails terminally rather than substituting its evidence.

An input port accepts at most one incoming producer. An output may fan out to
multiple consumers. A second edge to the same input is invalid whether it enters
through the UI, Archon, API, or a hand-edited file; all readers and writers run
the same whole-board validation.

A join is a node with multiple distinct required input ports. It starts only
after every required port contains one compatible payload. There is no implicit
merge order and no multi-edge merge into one port. At `node_started`, the engine
freezes all required refs and any optional refs already present as that attempt's
immutable execution input set; absent optional ports remain visible in
projection. The ledger records only their durable refs as defined below.

A later optional `data` delivery records `node_input_ignored` with
`reason=late_optional` and never mutates or retriggers the attempt. A late
required `data` delivery is an invalid execution path and fails loud except for
the validated direct-source revision cycle defined below. In this phase,
`gate_feedback` arriving on `retry_control` is the only in-graph trigger for a
bounded next attempt. Its identity-only `evaluatedInput` pointer must resolve
through the named Gate-input ledger event to that receiving node's exact
completed source attempt. The new attempt reuses that attempt's frozen work
refs and binds the new feedback ref. Duplicate, stale, or mismatched feedback
never dispatches and records a typed ignored/error outcome. Ordinary joins do
not silently rerun with a stale mixture of old and new inputs. Attempt, dispatch,
and wall-clock limits apply.

### Minimal payload and provenance contract

A routed port payload has one of four kinds:

- `work`: exactly one of bounded inline text or one safe artifact ref;
- `gate_feedback`: typed correction metadata from one failed gate evaluation;
- `unavailable`: a stable code, safe explanation, and retryability metadata that
  no declared output value exists;
- `error`: a stable code, safe explanation, and retryability metadata for a
  failed production attempt.

Every work port declares a non-empty `acceptedMediaTypes` subset and each work
payload declares one allowlisted media type from it. The initial contract supports
bounded UTF-8 `text/plain`, `text/markdown`, and `application/json`; arbitrary
binary payloads are deferred. A safe artifact ref includes stable `artifactId`,
an authorized `rootId`, root-relative ref, media type, byte size, and SHA-256. It
must resolve to a regular, non-symlink-escaped, size-bounded file under that exact
root. Resolution uses one root-relative no-follow open; regular identity,
size/media/hash, and authorization are checked on that handle, and File Peek
renders only the same verified bytes/handle without reopening the path. Every artifact projection carries a stable `artifactId`; unavailable states
keep that id while omitting the ref. The descriptor records content that passed
those checks when attached; its shape alone does not assert mutable-file
availability forever. `ArtifactProjection`
shows a readable ref only while the latest durable artifact observation is
`available`. Unavailable, redacted, or expired projections retain typed metadata
and an error code but no readable ref. File hydration always rechecks the host
file; a failed read returns unavailable immediately, and the reconciler appends
`artifact_observed` before durable projection changes. An
`artifact_observed(availability=available)` event must carry a newly validated
`SafeArtifactRef` whose `artifactId` matches the stable event id. The first
available registration or observation establishes the descriptor; every later
available observation for that id must match its root, ref, media type, size, and
SHA-256 exactly. Changed content or location requires a new `artifactId` and
registration.

Exactly one durable registration source establishes each artifact id and its
initial projection. Slot, Gate, and system artifacts use an `artifact_attached`
event appended and fsynced before any later result, output, evidence, diff, or
graph payload reference; its discriminated source identifies the producer. New
Tool artifacts are the sole same-event exception: `tool_result` atomically
registers them in `artifactRegistrations` under that exact lease before its other
fields are projected. An open Tool lease therefore has no public artifact
registration; recovery may remove and rerun its root without colliding with a
durable id. A completed lease is never rerun.

Evidence embeds only `{artifactId}`, never a readable ref. Canonical descriptors
remain in writer-private registration history. Run detail, event, and SSE APIs
hydrate every artifact occurrence through the latest authorized projection; an
unavailable/redacted/expired observation removes old readable authority from
historical responses and File Peek. Full JSON Schema is deferred, but a mixed
text+artifact work payload is invalid rather than an ambiguous representation.

Schema-2 `node_output` uses only optional `reportArtifactId` plus stable-order
`artifactIds`/`diffArtifactIds`; `run_succeeded` uses only optional
`summaryArtifactId` plus `outputArtifactIds`. Every id is registered before the
referencing event. Run detail, event, SSE, CLI, and UI projections hydrate the
latest authorized `ArtifactProjection` rather than exposing event-time refs.

Other embedding fields are non-authoritative caches: at their sequence they must
equal the latest registered/observed projection and cannot establish or mutate
identity. A later observation supersedes projection without rewriting earlier
events. A duplicate registration across either registration source is a loud
ledger error. Other observation states carry a stable error code and no readable
descriptor. Every observation targets an already-known artifact id. Projection
itself never consults the filesystem. Bare refs are allowed only in schema-1
ledgers or writer-private adapter ingress. They must establish a safe registration
before schema-2 append, are invalid in schema-2 events/APIs, and never let the
projector mint identity.
Markdown and text reports open
through CHROTE's existing Files/File Peek boundary rather than a second viewer.
ADR-0005 applies when the run is redacted.

Payload provenance is not mutable prose inside the payload. Durable
`RunInputRef` is a discriminated union with stable `inputId`. An ordinary
`sourceKind=edge` ref identifies `runId`, source node/port, source output
sequence/attempt, origin edge, and current delivery edge/destination. The
isolated-Formation exception is `sourceKind=run_seed`, which records `runId`,
stable seed id, `formation-brief-jcs-v1` encoding, `application/json` media type,
SHA-256 of the exact canonical frozen-brief bytes, and destination node/port, but
no invented edge or producer node. Both forms carry
`payloadProjection`: either an exact durably available payload or a redacted
metadata/hash state with no payload. The latter is evidence only and never a
graph input.

The executor may pair that durable ref with an in-memory `ExecutionInputRef`
containing the authoritative payload during fresh execution. That value is
never written to the ledger/API and is destroyed according to ADR-0005. Recovery
may route only an exact available durable payload; if the live value was
discarded, a marker, hash, summary, or sanitized replacement cannot substitute.
For Gate fail delivery, source node/port are the Gate/`fail`, source output
sequence is the `gate_verdict` sequence, and source attempt is the Gate attempt;
the feedback object retains only an identity pointer to the evaluated work,
never its `RunInputRef` or payload projection.
`unavailable` and `error` are durable, inspectable unsuccessful attempt outcomes.
An output-producing attempt finalizes all declared outputs before it
delivers any edge. If one declared output fails, that attempt records
`deliveredEdges=[]`; no successful sibling output from the same attempt routes.
No descendant dispatch occurs on the affected dependency path, while already
in-flight and independent branches may finish and append evidence. `retryable`
is an engine decision per unsuccessful output derived from a stable code/profile
policy, never an agent hint, and never causes automatic retry. A producer
attempt is retryable only when every unsuccessful declared output is retryable;
one non-retryable sibling makes the whole attempt non-retryable. Its candidate
lists every unsuccessful port/outcome in stable port order and uses the minimum
outcome sequence for ordering; successful siblings remain non-delivered. Once
in-flight work settles, the engine
derives each producer's latest unsuccessful attempt with `deliveredEdges=[]`
and no later attempt, ordered by `(minimum outcomeSeq, nodeId)`. If any is
non-retryable, the first such candidate deterministically selects
`run_failed(code=declared_output_failed)`, provenance-only `relatedSeq`, and
`failureCause={kind=none}` because that attempt is already closed. Otherwise the
first ordered retryable candidate alone appends `run_blocked` with
`blockScope=node`, its producer id,
`resumePolicy=retry_failed_producer`, `openDispatches=[]`, and one whole-producer
retry target. Other candidates remain closed durable failures, are neither
retried nor abandoned, and forbid success.

Explicit operator resume is allowed only for that blocked retryable state. The
one target names one failed `{nodeId, attempt}`, its output port/outcome
sequences, and proves `deliveredEdges=[]`. `run_resumed` opens the next epoch and
starts attempt N+1 for that target from the failed attempt's frozen `inputRefs` and the
same `RunSlotBinding`/`RunToolBinding`, using a new dispatch or Tool lease.
Completed siblings are not rerun, limits still apply, and no retry is automatic.
If an execution-authoritative input is unavailable—especially one discarded by
redaction—resume is rejected with the applicable terminal failure.
At the next graph quiescence the ordered set is recomputed: success removes the
old candidate, a retryable later failure replaces it at its newer outcome
sequence, and another first candidate produces another block requiring a
separate explicit resume. General-purpose error ports, multi-target resume, and
selective replay after any prior delivery are deferred.
If the graph instead quiesces before a JOIN starts after receiving only some
required inputs, the engine appends a node-scoped
`error(code=unsatisfied_required_input)` naming its waiting sequence, followed
by non-resumable `run_blocked` with that node id, empty open dispatches/retry
targets, `resumePolicy=new_run_required`, and no next epoch. The node projects
`blocked`, not indefinitely waiting; corrected wiring requires a new run.
`run_blocked.resumePolicy` is a closed union. `retry_failed_producer` permits
resume only with no open dispatches, one whole-producer retry target, and a next
epoch. `reattach_only` permits one bounded resume with a non-empty unchanged
open-dispatch set, no retry target, and a next epoch; it sends no prompt.
`new_run_required` forbids resume and has no retry target or next epoch; it may
retain only the exact unmatched dispatch set at that block. After reattach this
may be a strict subset of the preceding set, but it cannot add or change an
identity; its late authority is revoked.
At recovery quiescence, `reattach_only` freezes every unmatched dispatch in
stable dispatch-sequence order. It uses node scope only when they all belong to
one node; otherwise it uses run scope. The current block rejects late results,
outputs, and routing until exact resume. During that bounded no-prompt epoch,
exact results may close/release individual leases. The next block contains only
the still-unmatched subset and derives scope again; an empty subset continues
ordinary graph execution.

Quiescence selects one outcome rather than letting node subsystems race. An
existing cancel intent or valid execution-final condition takes precedence over
non-final blocks. Unmatched dispatch recovery comes next and preserves every
open lease before graph semantics. With no unmatched dispatch, an outstanding
human request remains `waiting_human` and is not hidden by a block. Then the
non-resumable reasons `unsatisfied_required_input`, `unwired_gate_fail`,
`gate_evaluator_error`, and `invalid_judge_result` dominate retryable outputs;
select exactly one by `(causalSeq, reasonRank, blockedGateId, blockedNodeId)`
using that listed rank. Other candidates remain evidence. Only when none exists
may the ordered whole-producer retry rule select a resumable block. The writer
validates this closed order and rejects a competing/later block.
Exhausting an immutable max-dispatch, max-attempt, or wall-clock limit appends a
scoped stable limit `error`, then terminal
`run_failed(code=run_limit_exhausted)` with that error as `failureCause` and
exact dispositions for every open attempt, slot dispatch, and Tool lease. Slots
are soft-interrupted only on proven exact targets; Tool scopes retain private
post-final fencing/cleanup ownership. Late result/output/routing is rejected.
Continuing requires a new run with a new frozen limit snapshot.

The current `FormationOutputPayload` fields (`text`, `ref`, `reportRef`, and
`artifactRef`) remain readable only as schema-1 or writer-private compatibility
inputs while the canonical writer is introduced. Bare refs terminate before
schema-2 append; they are not four routing models or public authority.

These execution semantics use board schema 2 and run-event schema 2. The
non-executing authoring boundary above separately fixes board schema 2, layout
schema 1, and new-board schema 1. A schema-1 Formation
input normalizes on read to `kind=work`, the stable full initial
`acceptedMediaTypes` set, `required=true`, and `role=data`; a schema-1 output
normalizes to `kind=work` with that same media set. Fixed Mission `out` accepts
only `text/markdown`; Gate `in`/`pass` work ports use the full set. Gate `fail` is `gate_feedback` and has
no media set. Inspection does not rewrite the file.
The first successful Tool creation is the schema-2 structural mutation defined
above; ordinary non-Tool writes never perform that migration. A legacy Gate fail edge targeting
a work input loads as degraded with
`legacy_fail_route_requires_migration`; inspection remains possible, but board
validation and run preflight reject it until it is rewired to a typed feedback
data port or the evaluated source's `retry_control` port. Legacy annotated-work
pushback is never inferred.

ADR-0008 retires schema-1 inline Formation verification. It remains inspectable
but cannot be normalized into execution because its existing verdict has no
exact attempt/output identity or replay-safe block/revision closer. Validation,
Mission start, isolated Formation start, and resume fail
`legacy_inline_verification_requires_migration` before artifacts or work. New
definitions and `verification_verdict` events are rejected; historical records
remain non-authorizing evidence. Compatibility removal must name an existing
replacement Gate already wired from a named output of the Formation; it neither
creates nor rewires that Gate. Historical cancellation and failure remain
available as non-executing terminal containment.

Run start does not mutate a schema-1 canonical board. If it has no degraded
legacy features, preflight writes a normalized schema-2 immutable run snapshot and
records both source and snapshot schema. Schema-2 events execute only that
normalized snapshot. A board that cannot normalize safely is rejected before
`run_started`.

New ledgers declare event schema 2. A ledger without that declaration projects
as schema 1 with its recorded compatibility semantics; it is never reinterpreted
as exact pass-through or typed feedback. One ledger cannot mix event schemas.
Schema-1 runs remain inspectable but cannot resume under the schema-2 engine;
resume fails with `legacy_run_requires_new_run`.

### Gate payload semantics

A Gate evaluates one exact input reference. `gate_verdict` stores verdict,
per-kind result, safe reason/evidence, evaluated input reference, and chosen
route independently of the work payload.

Schema-2 code kinds select a frozen `RunGateBinding` for a certified
deterministic, host-owned in-process evaluator profile. The binding includes
profile/content, content-addressed evaluator implementation bundle, parameter,
policy, and determinism-policy hashes plus positive input-byte, result-byte, and
deterministic operation limits and
`resultEncoding=decision-result-jcs-v1`. Its closed observable-input policy
permits only declared input plus those frozen values; denies spawn, network,
secrets, undeclared host reads, and writes; normalizes locale/timezone; freezes
or denies clock/entropy; and passes repeat vectors with expected result hashes.
Admission requires a total evaluator under host-metered deterministic fuel, and
the host contains panics. Fuel/input/result exhaustion or panic appends
Gate-scoped `error(code=gate_evaluator_error)` and cannot wedge run wall-clock
finality; it emits no kind result/verdict/route.
An accepted strict code result is RFC 8785 canonical UTF-8 JSON over exactly
`{verdict,reason,evidence}` with no unknown keys or trailing newline. Evidence
array order is preserved and every element uses the closed safe evidence union.
`gate_kind_result.resultSha256` hashes those exact bytes; replay cannot
substitute another serializer or result encoding.
A crash before `gate_kind_result` repeats only with the exact bundle/policy;
upgrade or missing content fails loud without evaluation/routing. Every initial
or recovery evaluation must also resolve/revalidate the exact authoritative
input bytes against the frozen input hash. Hash-only/redacted evidence is never
a substitute: lost Redact=true input records terminal
`run_failed(code=redacted_input_unavailable)`, while non-redacted artifact drift
first appends Gate-scoped `error(code=gate_input_integrity_failed)`, then terminal
`run_failed(code=gate_input_integrity_failed)` with that error as
`failureCause={kind=error,errorSeq}` so the exact Gate attempt fails. Redacted
loss first records bounded Gate/attempt/input context but keeps the source input
sequence as provenance and `failureCause={kind=none}` under ADR-0005. Both exactly
dispose every open attempt/slot/Tool authority; neither emits result/verdict/route.

Gate-owned process evaluation is retired. Authored presence of any schema-1
Gate command field—`command`, `commandArgv`, `commandShell`, or `commandCwd`,
including an explicitly empty value—is inspectable but not safely normalizable.
The writer does not create or update these fields. Whole-board validation reports
`legacy_script_gate_requires_fenced_migration`; an inert legacy string or
cwd-only field receives that same finding because dropping or interpreting it
would be a silent migration. It may separately be `gate_not_routable`.

Mission preflight checks the selected Mission's complete possible executable
subgraph, including both Gate branches and reachable judge chains. A reachable
legacy command Gate fails `legacy_script_gate_requires_fenced_migration` before
snapshot, binding, ledger, `run_started`, evaluator, or process mutation. Resume
checks that same selected root in the immutable run snapshot and returns the same
error before `run_resumed`. An unreachable legacy Gate remains a validation
error but does not block that Mission. An isolated Formation root does not
traverse board edges, so legacy Gates elsewhere on the board do not block it.

Before Tool profiles exist, migration is inspection only. Its closed projection
binds board id/revision/ETag, Gate id, source mode, present field names, affected
edge ids, the stable error, `targetKind=tool_plus_pure_gate`, `ready=false`,
`applySupported=false`, and the unmet profile/mapping/CAS requirements. It
contains no raw command values, resolved executable/cwd/environment, generated
Tool ids/ports, inferred parameters, or suggested profile. Board inspection
remains the authorized exact source-definition view; validation and API
inspection may expose this non-authorizing plan beside it.

`ctx-ug7.8.1` owns non-executing Tool definitions, registry descriptors, and
board authoring. `ctx-ug7.8` owns certified host-private implementation
packaging and runtime execution. `ctx-ug7.30` owns pure code-Gate profiles and
the future explicit apply. Such an apply may preserve the
Gate id/title/criterion, judge relationships,
pass/fail edges, and existing layout while inserting one newly placed Tool only
after explicit Tool and pure-Gate profile selection plus parameter, port, media,
downstream, and compare-and-swap validation. The Gate then evaluates and passes
through the exact Tool output, not the Tool's upstream input. If that payload
mapping cannot be proven, the source definition remains byte-identical and the
migration fails loud.

An executable Gate's persisted `kinds` array is a non-empty, duplicate-free
subset of `code`, `formation`, and `human`; preflight rejects empty, duplicate,
or unknown hand edits before `run_started`. Gate kinds are an all-of set in
fixed `code`, `formation`, `human` order. Each completed code/formation kind appends/fsyncs one unique `gate_kind_result`
before the next starts. A fail short-circuits later kinds as `not_run`; invalid
code evaluation records `error(code=gate_evaluator_error)` and, after independent work
settles, a non-resumable Gate-scoped `new_run_required` block with only the Gate
id and no open dispatches, retry targets, or next epoch; it routes neither
branch. An execution-final event reached during quiescence takes precedence.
Human is requested only after prior
kinds pass and projects `waiting_human`; its matching decision continues the
same attempt. Exactly one aggregate `gate_verdict` lists every declared kind and
alone authorizes routing. Replay reuses completed kind results and never reruns
the code evaluator or judge chain while waiting for a human.
The request has no independent timeout or default verdict. It ends only with
its exact decision, cancellation, or terminal exhaustion of the immutable run
wall-clock limit.

On pass, each pass edge forwards the durable evaluated ref/projection and source
provenance unchanged. During fresh redacted execution it also forwards the same
exact live authoritative payload. The route records the new edge traversal, but
does not replace the source node/port/output sequence with the Gate or substitute
judge text or durable evidence.

On fail, the Gate creates exactly one stable `gate_feedback` object per gate
evaluation sequence. Zero or more fail-edge traversals may reference that same
object: an unwired fail has zero deliveries and blocks, while fan-out does not
duplicate feedback identity. It contains:

- a stable feedback id;
- the Gate id and failed verdict;
- the original evaluated input's stable `inputId` plus the matching
  `gate_evaluating` sequence, with no embedded input ref/projection/payload;
- a bounded, redaction-safe reason;
- bounded evidence values typed as stable artifact ids, ledger event refs, or
  inline redaction-safe text;
- the gate-evaluation sequence and Gate attempt.

With no fail edge, the aggregate strict-fail verdict still records
`routePort=fail` and `routedEdges=[]` and closes the Gate attempt. After other
work settles, a Gate-scoped `run_blocked(reason=unwired_gate_fail)` names only
that Gate, has empty open dispatches/retry targets, is non-resumable
`new_run_required`, and omits `nextEpoch`. The Gate projects blocked with its
FAIL verdict visible; this is a blocked overlay on the Gate attempt already
closed by `gate_verdict`, not a second closer. The completed upstream Formation
is not rewritten.

A separate correction node that needs both original work and feedback declares
distinct required `work` and `gate_feedback` data ports. `pushback` is not a
verdict; it is one deliberately narrow fail-route cycle. It is valid only when
the evaluated input came directly from the receiving Formation, that source
node's entire connected workflow-output frontier is the one edge into this Gate,
and the Gate's fail frontier is exactly one edge to that Formation's
`retry_control`. The first Gate evaluation allocates a stable `revisionCycleId`
for this run-local cycle. Matching feedback starts source attempt N+1 from the
matched attempt's frozen work refs only while their authoritative values remain
live or durably exact; otherwise ADR-0005 fails terminally. Its revised output opens Gate attempt N+1, linked by
stable `revisionCycleId`, triggering `feedbackId`, prior Gate sequence, and new
source attempt. This is the sole permitted late-required delivery. Mixed fail
fan-out, downstream replay, another connected source output, and pushback to an
earlier non-source step are invalid; use an explicit correction path instead.
`retry_control` edges are excluded from the ordinary data DAG, but the bounded
cycle remains subject to attempt, dispatch, and wall-clock limits. After those
validated edges are removed, the `channel=workflow` graph must be acyclic;
shared read/write validation and run preflight reject every other cycle.
Feedback
references the evaluated work; it never overwrites or copies it implicitly.

Formation judges receive evaluation control from `judge` and return verdict
evidence to the same reserved socket. Judge edges form one linear, Formation-only
entry-to-exit chain: no branch, JOIN, side entry/exit, repeated node, workflow
cross-use, or Tool node is allowed. They order evaluation; they do not deliver
ordinary `PortPayload` values. Every judge receives a bounded `JudgeContext`
containing Gate id/attempt, criterion/kinds, the exact in-memory evaluated input,
its durable `RunInputRef`, and prior JudgeResults. Each returns exactly one
bounded `JudgeResult`:
`{verdict: pass|fail, reason, evidence[]}`, where evidence uses the same safe
artifact/ledger/bounded-text union as Gate feedback. The exit result becomes Gate
metadata, never downstream work. Missing, malformed, or multiple returns append
and fsync `judge_attempt_failed` with `code=invalid_judge_result`, complete that
judge Formation attempt as failed, block the Gate, and route neither pass nor
fail. If judge-produced content should continue through the workflow, use a
separate Formation or Tool outside the judge channel.

`judge-context-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{gateId,gateAttempt,criterion,kinds,evaluatedInput,durableEvaluatedInput,
priorResults}` with no unknown keys or trailing newline. Kinds use fixed Gate
order, prior results use judge-chain order, and nested evidence order is
preserved. `judgeContextSha256` hashes those exact bytes. Judge results use the
same `decision-result-jcs-v1` contract as code-Gate results and record
`resultSha256` over those exact result bytes.

Because judge control is not a workflow input, its Formation attempt records
`node_started` with `contextEncoding=judge-context-jcs-v1`, immutable
`judgeContextSha256`, and prior-result sequences instead of `inputRefs`. It emits
no ordinary `node_output`; either an accepted
`judge_result` or `judge_attempt_failed` completes that judge attempt.

Before the next chain member can dispatch, the coordinator appends and fsyncs
one `judge_result` for the current member. It is keyed by Gate id/attempt, judge
node/attempt, and chain index and records the strict result, context/result
encodings and hashes, and prior-result sequences. Invalid output instead appends
`judge_attempt_failed` with the same key, context hash, prior-result sequences,
stable code/reason, and related capture sequence. Exactly one completion event
is accepted for that key: either `judge_result` or `judge_attempt_failed`; a
conflicting duplicate is a loud ledger error. After the failed event is durable,
the coordinator prevents new Gate/dependent dispatch, lets already in-flight and
independent branches settle and record evidence, then appends `run_blocked` with
`blockScope=gate`, both judge and Gate ids, `reason=invalid_judge_result`, empty open dispatches and
retry targets, `resumeAllowed=false`, and
`resumePolicy=new_run_required`. A non-resumable block omits `nextEpoch`. This
occurs unless quiescence produces an execution-final event, whose finality takes
precedence. This phase defines no in-run judge retry; corrected configuration or
staffing starts a new run.

Recovery rebuilds the durable next-context projection and prior results from
accepted `judge_result` events and never reruns or reparses a completed judge's
raw capture. It may construct and dispatch the next exact `JudgeContext` only
while the evaluated authoritative input remains live or durably exact. If a
redacted crash discarded that value, recovery appends terminal
`run_failed(code=redacted_input_unavailable)`; it never reruns an earlier judge or substitutes a
marker, hash, summary, or sanitized evidence. `judge_result` is evaluation
metadata, not `PortPayload`.

### Assignment, runtime binding, and terminal view state

Board definitions store a slot label, stable agent id, and optional harness
constraint. They never store tmux socket, session, window, or pane targets.
`assigned` therefore means only that structural assignment exists; it does not
mean runnable.

In production, Formations calls the same configured Terminal-session resolver
and inventory shown by cockpit Terminal tabs. That inventory is the union of
the explicitly configured user/socket sources; it may contain one or several
tmux servers, but there is no separate Formations-only production source or
pool. Reusing accumulated session context is intentional; the evidence contract
is same-session-lineage, not a clean-room agent. A persona `session_stem` that
matches targets in more than one inventory source is ambiguous and fails loud;
the board never chooses a raw socket path. A disposable source may replace the
inventory only when both Terminal tabs and Formations consume it through that
same resolver in an explicitly isolated test, certification, or dogfood harness.

At run start, the host produces a `SlotResolution` for each declared slot in the
selected `runRoot` executable subgraph. An unassigned slot is `unresolved` with
`reason=agent_unassigned`; assigned states are `unresolved`, `runnable`,
`ambiguous`, or `unavailable` with a stable reason. A run begins only when every
slot is runnable. It then writes one host-private immutable `RunSlotBinding` per
slot plus a hash-linked safe `RunSlotBindingProjection`. Private authority
includes binding/slot/agent/harness identity, persona-card path/hash, and exact
target. The safe projection exposes only opaque `sessionTargetId`, card hash,
and safe display/root metadata. The private target records tmux server/socket
identity, session id/name, window id, pane id, resolved cwd/root, resolution
time, fingerprint, and canonical private
`targetKey` for that exact pane incarnation. A durable one-to-one resolver
mapping makes every independent resolution of that key return the same opaque
handle; a replacement pane gets a new key/handle. Raw routing and the key remain
server-side. The fingerprint commits to adapter/harness, foreground-process
start identity, cwd/root, and pane-incarnation evidence.
Two selected slots resolving to the same `targetKey`/`sessionTargetId` reject before
`run_started`; exact identity does not make interleaved use safe.
Preflight also reports a matching candidate that is leased, attached, or not
proven closed/ready as unavailable and creates no binding. The stable reasons
are `session_target_leased`, `session_target_attached`,
`session_target_harness_busy`, and `session_target_readiness_unknown`;
ambiguity and fingerprint drift remain `session_target_ambiguous` and
`session_target_stale`; inability to arm the complete certified client/input
monitor is `session_target_attachment_audit_unavailable`. A candidate is not idle merely because pane output is
quiet or prompt-like. It is ready only when the certified harness adapter's
non-pane control channel proves a closed/ready turn for the exact current
fingerprint. A certified open turn is `session_target_harness_busy`; missing,
unsupported, stale, or non-unique evidence is
`session_target_readiness_unknown`. Both are unavailable. Visible external
clients and connected CHROTE Terminal clients, including hidden pooled iframes,
count as attachments. Slot binding never detaches or reclaims them. The user may
explicitly disconnect a CHROTE-owned presentation client before retrying; that
separate presentation action cannot detach an external client or another run's
lease. The observation is not a lease: immediately before send, atomic target
acquisition repeats the registry, attachment, fingerprint, and certified-ready
checks under the target critical section. It obtains a fresh one-shot
`target-ready-proof-v1` bound to that acquisition and fails
`session_target_busy` with the corresponding stable reason if any check no
longer passes. It never steals, creates, or selects an alternate session.
An in-process lock is not that monitor/fence. The stock owner-accessible tmux
socket admits independent raw command clients, so it must return
`session_target_attachment_audit_unavailable` until the same-pool enforcement
decision in `ctx-ug7.21`, implementation in `ctx-ug7.22`, and certification in
`ctx-ug7.23` are complete.

For attachment arbitration, Formation ownership begins only after the exact
target-registry occupancy is fsynced; a frozen `RunSlotBinding` alone grants no
exception. After that occupancy and the matching durable ledger dispatch exist,
only a CHROTE-issued run-bound Peek capability exact-matching run, dispatch,
target lease, binding, and fingerprint is an authorized additional attachment.
Only the latest fsynced issuance is valid. A later issuance requires prior
clients/input drained and atomically invalidates every older token/generation;
a superseded route reaching the target boundary is foreign.
Shared Terminal attach paths reject the occupied target unless the request is
converted to that exact Peek capability. Any other client remains foreign. The
writer-private certified attachment-monitor registration is armed and durable
before occupancy is fsynced, then observes every client attach, detach, target
selection, resize/reflow, history, pane-lifecycle/topology/other mutation, and
input-capable command/control connection affecting the pane
through release, including transient clients and raw `send-keys`/paste-style
routes. Its closed authorized-route union is only the one-shot barrier-bound
workflow prompt, metadata attach from a durable Peek-capability issuance,
generation-gated Peek bytes, and one exact ledger-before-send reconciliation
interrupt. A registered Peek client counts as authorized only when its inbound
bytes terminate at the coordinator's steering-generation gate. A foreign event
or lost monitor continuity durably latches
`session_target_foreign_attachment`, `session_target_foreign_input`, or
`session_target_attachment_audit_unavailable`, revokes and drains Peek input,
and makes the dispatch non-authorizing: it can produce neither `slot_result`
nor an ordinary release. Raw foreign input is never retroactively classified as
a steering generation. A foreign-input/attachment or lost-audit latch stays
held/quarantined until exact pane-incarnation-gone proof; current-client absence
cannot reconstruct history. Other closure/cancel-ack holds with continuous
audit may use the exact final-quiescence proof below. No output is promoted. Recovery suspends Peek
input until the capability, recovered occupancy, and attachment-monitor
continuity are revalidated under the current fence.

Across runs, the host owns one durable exclusive target lease per
`targetKey`; `sessionTargetId` is its opaque API handle, not a per-run key.
Before composing any ordinary or judge Formation prompt, the host materializes
each artifact-backed execution input through one authorized-root-relative
no-follow open, validates regular-file identity, media type, size, and SHA-256 on
that handle, and reads the bytes for the prompt from that same handle without
reopening. The prompt receives those exact verified bytes, never a mutable path
as input authority. A non-redacted mismatch appends node-scoped
`error(code=formation_input_integrity_failed)` and terminal failure with that
error as the exact cause; unavailable Redact=true bytes follow ADR-0005's
`redacted_input_unavailable` failure. Both occur before `slot_dispatch` and send
nothing.
Before durable ledger `slot_dispatch`, it allocates dispatch/target
lease ids and atomically acquires/fsyncs a private record binding the target,
run, dispatch, binding, node, attempt, slot, fingerprint, and the exact
pre-occupancy `attachment-audit-registration-v1` hash. The registration and its
first anchored `fence_transition` journal record fsync before occupancy, so no
empty-chain or post-occupancy gap exists. Under that lease and
immediately before send it exact-rechecks card/harness/process/cwd/root/pane;
drift records stale, atomically retires the unsent fence/occupancy, and sends
nothing. The certified boundary drains and linearizes its durable interaction
journal, rejects a foreign/lost-audit latch, and installs one exact
`target-dispatch-input-barrier-v1` permitting only the already-hashed prompt for
this dispatch/lease/fingerprint. It then obtains `target-ready-proof-v1`, RFC
8785 canonical JSON over exactly
`{targetFingerprintSha256,acquisitionChallengeSha256,dispatchInputBarrierSha256,harnessReadyEvidenceSha256}`;
all four values are 64 lowercase hex. The challenge is fresh and the certified
adapter issues the proof only for a closed/ready turn on its non-pane control
channel. Pane bytes, silence, prompt text, or an earlier proof cannot satisfy
it. The host then captures one exact `tmux-pane-history-baseline-v1` and records
the dispatch barrier/proof, ready proof/hash, baseline/hash, and prompt hash in
writer-private `slot_dispatch` before the adapter consumes that one send permit.
The fence synchronously rejects/latches attach, select, resize/reflow, history,
pane-lifecycle/topology/other mutation, and raw input races through send; after send it admits only metadata-authorized Peek
attach, steering-generation input, or one ledger-before-send reconciliation
interrupt. Failure or crash before ledger dispatch atomically retires the exact
unsent fence with occupancy; missing/conflicting fence state quarantines rather
than reopening the pane.
The baseline is RFC 8785 canonical JSON over exactly
`{targetFingerprintSha256,historyEpoch,offset,cols,rows}`: exactly 64 lowercase
hex fingerprint characters hashing the exact UTF-8 bytes of the dispatch's
frozen `targetFingerprint` string with no trailing newline, the canonical
unpadded base64url encoding of exactly 16 capture-continuity epoch bytes, an
unsigned decimal 64-bit absolute byte offset string with
no leading zero except `"0"`, and JSON-integer grid dimensions in `1..65535`.
It contains no pane output or input bytes. The tmux adapter changes
`historyEpoch` whenever it cannot prove continuous cursor identity, including
pane replacement, history clear, or restart without durable cursor continuity.
`capturedRange.start` must exact-match the baseline epoch/offset and its end must
be strictly later. Trim past the boundary, reset, resize/reflow, missing cursor,
or ambiguous comparison fails `capture_baseline_unavailable`; it permits no
`slot_result` or ordinary release and follows the existing hold/quarantine
reconciliation path. Older pane history can inform the agent but cannot be
parsed as this dispatch's sentinel or output. Sanitized projections omit the
exact token and expose only encoding, SHA-256, and validation state; a
`slot_result` exact-references the dispatch sequence/hash rather than supplying
another baseline value. Only after the matching durable ledger
dispatch is fsynced may the adapter receive the same in-memory prompt byte slice
whose SHA-256 the event records. It receives that slice at most once, then the
coordinator discards it. No prompt bytes/ref/path/authority id is durable, and the
hash cannot reconstruct or authorize resend. The prompt contains run, dispatch,
and target-lease ids. The completion sentinel and `slot_result` must exact-match all three;
run id alone is not attribution. A private lease without ledger dispatch is a
safe unsent orphan. Release atomically replaces an occupying record with an
exact durable non-occupying receipt; that receipt permits later acquisition but
survives until this run's final event is fsynced. Any ledger dispatch lacking
both its expected occupancy record and release receipt, or with conflicting private state,
may already have sent: before failure/finality, the arbiter reconstructs or
promotes a durable non-authorizing quarantine at its frozen `targetKey`. Its
stable `(runId, dispatchId, targetLeaseId)` candidate set preserves the expected
identity/result and each conflicting occupant separately. It is
never resent or restored as active authority. An unmatched dispatch may expose
its exact candidate in its final slot disposition. A result-closed candidate has no
open disposition, so its quarantine must be proven quiescent and replaced by an
exact `result_committed` receipt carrying certified turn-closure proof plus a
closed barrier-held or post-result-pane-gone release proof before any final
event. The key remains busy until every candidate dispatch is proven
quiescent. A missing/corrupt frozen key creates a separate durable host-wide
quarantine with every available run/dispatch/lease/binding identity; its safety
latch denies all target acquisition until operator repair and non-authorizing
reconciliation remove every candidate.

Normal completion first atomically stops capability issuance, drains any input
channel, closes every open steering generation, and fsyncs one irreversible
`slot_peek_capability_revoked` event for the dispatch. It then requires an exact
sentinel terminal in bounded capture plus a certified harness-ready/closed-turn
proof for the frozen fingerprint and a certified attachment-audit proof that
continuously accounts for every client/input event since occupancy. Under the
same target critical section the certified monitor drains/linearizes its event
journal, installs a per-target mutation/input fence, and produces the closed interaction
barrier bound into the proof. Pre-barrier foreign input prevents a result;
post-barrier unregistered attach/select/resize/reflow/history/pane-lifecycle/
topology/other mutation/input is rejected at the
adapter boundary. The fence remains in
force while `slot_result` fsyncs and until occupancy is replaced by a
`result_committed` receipt naming the result sequence, proof, and exact
barrier-held release proof. That receipt fsync is the release linearization
point, and the result cannot contribute to Formation result/routing/finality
before it. Trailing old-turn output is not a result. If fence/barrier continuity
is lost after result fsync but before the receipt, the immutable result remains
uncommitted in a result-closed quarantine; it may become the exact receipt only
after proving the original barrier held or the old pane incarnation is gone.
Cancel/failure requested in this window completes the same result-closed
transition first; it cannot convert or omit it. An unmatched
canceled/failed dispatch likewise stops capability issuance, drains input,
closes any generation with `reason=capability_revoked`, and fsyncs capability
revocation before interruption or proof validation. It fsyncs one exact
`slot_reconciliation_interrupt` bound to the durable cancel request or
failure-reconciliation start before the fence permits one `0x03` attempt; a
crash after intent never resends, and a missing outcome is `send_uncertain`.
It replaces occupancy with
a `final_quiescent` receipt
only after either the closed `dispatch_cancel_ack` proof exact-binds that
interrupt request, capability revocation/generation, certified ready boundary,
and continuous attachment-audit proof under the same input fence/barrier
through the release transition, or proof
that the old pane incarnation is gone;
however, a dispatch already latched for foreign attachment/input or lost audit
continuity cannot use the cancel/ready branch. Its sole release proof is exact
old-pane-incarnation-gone because current readiness cannot reconstruct history;
otherwise a non-authorizing terminal
hold retains the input fence and keeps the target busy until later proof. Its final slot disposition records
`targetLeaseState=released_quiescent|terminal_hold|quarantined` only after the
corresponding registry transition is durable. Registry state does not expire by
time/name/PID; sent interrupt, silence, and generic prompt/idle text are not
proof. A replacement pane has a new key/fingerprint and cannot be released or
interrupted as the old target. A receipt is not occupancy, and crash recovery reuses it instead
of quarantining or re-interrupting released work. A concurrent acquisition
failure sends nothing. Preflight returns its specific stable unavailable reason;
after run start, `slot_binding_observed` preserves that reason and the exact
attempt fails with node-scoped `error(code=session_target_busy)`.
An idle deadline without the exact three-part sentinel plus certified closed-turn proof records
`dispatch_idle_timeout` as a slot error but no `slot_result`; the dispatch and
target lease stay unmatched and enter bounded reattach. Timeout alone never
authorizes lease release.

After run start, `bindingHealth` is a projected observation—`runnable`,
`unavailable`, or `stale`—not a mutation of the frozen target identity. The
reconciler appends `slot_binding_observed` with binding id, opaque target id,
health, stable reason, observation time, and related sequence. Projection uses
the latest such event and never consults live tmux on its own.

Binding and artifact observations are non-authorizing inspection evidence. They
may append after an execution-final event, but cannot change run/node outcome,
open an epoch, rebind a target, or authorize dispatch.

Each slot dispatch references its own binding, target, and fingerprint, so one multi-slot
Formation attempt can expose several exact sessions. Dispatch consumes the
frozen binding and never re-resolves a mutable persona card or same-name session.
A run never rebinds a slot to a different target: recovery may reattach only the
same qualified pane, and a replacement pane requires a new run.

Run inspection and terminal Peek use each dispatch binding recorded for the
exact attempt plus its dispatch/target-lease ids and frozen fingerprint, not a
fresh lookup by display label, session name, or opaque handle alone. Live run
Peek input requires that exact active/unmatched occupancy while the run remains
non-final and before cancel/failure reconciliation begins. A terminal hold is
non-authorizing run evidence and permits no run-bound input, even before or
after finality. Quarantine is ambiguous and cannot open a live run view. After
a result/final release receipt, the pane may be reused, so
the old attempt exposes captured history or explicit `pane_moved_on`; a separate
Open current session action must be labeled as non-run context. Whether one or
several terminals are open, focused, tiled, or hidden on the Formations board is
user-local presentation state. It is not board structure, run truth, or tmux
lifecycle authority.

An authorized live Peek is a full interactive user attach and may send literal
terminal input, including control characters, to steer or interrupt the agent
on that exact session. This operator effect is allowed; it is distinct from an
automatic workflow prompt, retry, coordinator interrupt, or lifecycle command
and grants the browser no dispatch authority.

Peek input is accepted only through the exact run-bound capability above and is
serialized with result closure under the target occupancy. Before forwarding
the first input, the coordinator fsyncs `slot_steering_started` with a new
monotonic steering generation and no raw keystrokes. While that generation is
open, result closure is forbidden. After the input channel is drained/released,
the coordinator fsyncs `slot_steering_ended`, recaptures from the dispatch
baseline, and obtains a fresh certified
turn-closure proof bound to that generation. The proof must come from the
certified adapter channel and cannot be satisfied solely by user-writable pane
bytes, echoed sentinel text, or generic prompt/ready text. Concurrent later
input either opens a newer generation and invalidates the proof before
`slot_result`, or is rejected after the capability is revoked. Safe inspection
marks the dispatch operator-influenced without storing input bytes.

Cancel request, failure reconciliation, normal result closure, and every
execution-final event serialize through the same lock. They stop new capability
issuance, revoke the existing capability, drain its channel, close any open
generation with `slot_steering_ended(reason=capability_revoked)`, and fsync
`slot_peek_capability_revoked` before accepting closure or finality. Terminal
failure first fsyncs its unique cause/snapshot-bearing
`run_failure_reconciliation_started`, which projects non-final `failing` and
admits only reconciliation. `run_failed.failureReconciliationSeq` must name that
start, byte-match its exact
`{code,reason,unrecoverable,relatedSeq,failureCause}` header, and dispose all
three frozen snapshots exactly. A direct failure uses
`originCancelRequestSeq=0`. When cancel escalates, that field exact-names the
unique cancel request, the failure snapshot preserves cancel-time resource
identities and prior interrupt request/outcome, and a missing outcome remains
`send_uncertain`; failure reuses it and never sends a second coordinator
reconciliation interrupt. Either
authority may create only the exact no-resend reconciliation-interrupt permit above.
Finality
enumerates capability state as well as target records and rejects an issued
capability, open input channel, open steering generation, or closure proof that
does not bind the latest closed generation and revocation sequence. A held
session may later be opened only through a separately labeled non-run Terminal
action after ordinary target arbitration permits it; run authority never
survives finality.

Run-bound Peek uses the shared deferred/ephemeral iframe lifecycle, but an
active dispatch freezes the pane grid recorded in its history baseline. Moving
or resizing the tile changes only the browser viewport; it must not send a tmux
resize or `SIGWINCH`. Any resize/reflow from another source invalidates the
baseline as above. Closing, moving, focusing, or tiling Peek never creates,
kills, or rebinds a tmux session.

## Rationale
Four explicit kinds are enough to explain the workflow without creating a
generic plugin framework. Typed ports make joins and failures deterministic.
Pass-through and feedback semantics keep Gates honest: users can see the work
that was evaluated, the verdict, and the correction request as separate facts.
Run-bound targets make live inspection precise without putting ephemeral tmux
identity into reusable board files. Pane/history baselines preserve attribution
while deliberately allowing same-session context reuse.

## Alternatives Considered
- **One generic plugin node:** rejected. It hides execution and safety
  differences and gives the canvas no trustworthy visual vocabulary.
- **Use Script Gates as Tool steps:** rejected. Evaluation and transformation
  have different payload, replay, and side-effect semantics.
- **Retain Gate-owned argv/shell execution behind a process fence:** rejected.
  It would duplicate the Tool supervisor/profile authority while keeping
  transformation-shaped process work hidden inside an evaluator.
- **Infer a Tool profile or argv from legacy command text:** rejected. A board
  cannot choose executable authority, and a Tool-to-Gate composition evaluates
  and forwards Tool output rather than the original input.
- **Allow many producers on one input:** rejected. Merge order and provenance
  become implicit; explicit ports make joins inspectable.
- **Route judge output as work by default:** rejected. It silently replaces the
  payload under evaluation.
- **Persist tmux targets in boards:** rejected. Reusable structure would contain
  stale runtime identity.
- **Reference a second reusable-formation registry now:** rejected. Embedded
  nodes and copyable templates avoid another version-resolution boundary until
  reuse requirements are proven.
- **Use sticky latest-value joins:** rejected. A completed attempt keeps an
  immutable input set; only an explicit retry/control rule can bind a new set.
- **Allow effectful Tools in the first contract:** rejected. Network and external
  side effects make deterministic replay and recovery a different design.
- **Rebind a stale slot inside a run:** rejected. It would silently change the
  execution identity captured by the run snapshot; a replacement target starts
  a new run.
- **Add multiple Mission outputs now:** rejected. Fixed `out` keeps run-start
  validation unambiguous until a real port-keyed initialization need appears.
- **Adopt full JSON Schema and arbitrary binaries now:** rejected. The bounded
  text/JSON and safe-ref envelope is sufficient for the first mixed workflow.
- **Resolve a profile range, alias, default, fallback, or latest version:**
  rejected. Exact immutable tuple lookup prevents authoring and replay from
  changing meaning when the registry changes.
- **Bind Tool ports by ordinal or display label:** rejected. Reordering or copy
  changes would silently change semantic wiring; stable machine names bind the
  descriptor while generated ids bind the board instance.
- **Treat nonempty media intersection as compatibility:** rejected. A producer
  could still emit a media type the consumer rejects, so its complete possible
  set must be a subset of the consumer set.
- **Guess the coding-mission linter descriptor:** rejected. Its identifier,
  ports, parameters, media, and sealed source/result contract remain a future
  owner product-direction decision.

## Consequences
Current boards remain readable, but new mixed-workflow behavior needs schema,
validation, and projection work. Tool profiles become a host-owned registry
rather than convenient inline commands and are pure in the first slice.
The first registry is deliberately one non-executing `json.normalize@1`
authoring descriptor. It grants no runner or process authority, and Tool roots
fail `tool_execution_unavailable` until the later runtime slice lands. The
linter surface is not part of this decision.
Legacy command-backed Gates become degraded definitions whose command fields
are read-only until an explicit certified Tool-plus-pure-Gate migration can
prove its payload mapping. Non-command title, kinds, or criterion edits may
preserve those source fields byte-for-byte; some arbitrary commands therefore
have no automatic migration.
Correction loops require explicit port roles and exact-attempt refs. Artifact
refs gain stable artifact and root identity. Exact session inspection requires per-slot binding
authority plus safe projections and must surface when an old target is stale.

This is accepted target behavior, not a claim that the current binary implements
it. Current main has Mission, Formation, and Gate nodes; untyped formation ports;
legacy output-ref fields; partial name-level session bindings; and no Tool node,
typed feedback payload, exact pass-through provenance, or exact run-bound pane
target. API and Archon also construct peer synchronous engines, persist run files
inside the workspace, and lack the workspace command journal, owner fence,
durable queue, and `formation_result` recovery boundary required here and by
ADR-0007.

## Enforcement
- Shared structural validation rejects unknown endpoints, incompatible payload
  kinds/media types, ambiguous text+artifact work, duplicate producers, invalid retry-control ports, workflow-channel
  cycles after valid retry-control edges are removed, and extra judge
  relationships on read and write. Draft boards may retain unwired required
  ports or an incomplete judge pair; board validation and run preflight reject
  those executable gaps before dispatch.
- Gate, pushback, replay, and projection fixtures prove unchanged pass-through,
  one feedback identity across fan-out, exact-attempt retry, late-input handling,
  judge-metadata separation, failure lifecycle, and attempt limits.
- CLI/API parity fixtures round-trip the same ids, ports, payloads, artifacts,
  per-slot binding health, and zero-or-more qualified attempt targets.
- Tool fixtures freeze exact profile/policy/execution-bundle hashes, seal
  immutable input manifests, deny nondeterministic inputs, apply one stable
  multi-failure selector at normal/recovery/cancel boundaries, and prove pure
  replay. Gate fixtures reject evaluator-bundle/policy substitution. Artifact
  fixtures reject missing identity, root escape, symlinks, hash/size drift,
  revoked historical refs, and unsafe evidence refs.
- Non-executing Tool fixtures freeze the exact descriptor shapes and sole
  `json.normalize@1` entry; exact tuple/name/port/media/parameter validation;
  schema-1 read byte identity; board/layout schema separation; content-
  preserving first-Tool migration; byte-identical rejection; heuristic-only
  placement; API/Archon parity; and HTTP 422/lowercase
  `tool_execution_unavailable` before global runtime authority or effects.
  Repository checks prove this slice adds no runtime registration, callback,
  executable/process, tmux, launch, lease, recovery, or artifact authority and
  no speculative linter descriptor.
- `ctx-ug7.8.1` owns non-executing Tool definitions, registry descriptors, and
  board authoring; `ctx-ug7.8` owns certified host-private implementation
  packaging and runtime execution; `ctx-7i1` owns canonical run projection,
  baseline hash/validation state, and safe operator-influence state;
  `ctx-ug7.9` and `ctx-ug7.10` own inspection and the post-occupancy exact Peek
  capability/steering protocol;
  `ctx-ug7.11`, `ctx-ug7.13`, and `ctx-ug7.14` own Archon authoring, packaging,
  and the empty-workspace mixed-workflow E2E.
- `ctx-ug7.6` is the non-claimable coordinator integration epic.
  `ctx-ug7.6.1` owns workspace authority, command records, publication, and
  fencing; `.6.2` owns asynchronous command admission and FIFO policy; `.6.3`
  owns replay, results, Gate-bundle replay, failure/cancel reconciliation,
  finality accounting, and the timeout/complete-set reattach state machine;
  `.6.4` owns the shared Terminal resolver, pooled-client arbitration,
  host-wide per-target occupancy/lease lifecycle, exact baseline cursor, and
  acquisition-time fingerprint recheck.
- `ctx-ug7.4` defines the workspace command, owner-lease/fence,
  `formation_result`, and authority-schema contracts. `.6.1` through `.6.4`
  implement their split portions together with host-private hashed canonical
  run authority, generic Files denial, pending-redaction isolation, binding
  integrity, and coordinator enforcement. `ctx-ug7.10` alone implements the
  post-occupancy Peek capability, steering-generation/result serialization,
  and certified non-pane-forgeable closure protocol. `ctx-7i1` is the sole implementation
  owner of sanitized event/binding/artifact projection and historical/SSE
  revocation; coordinator/API entrypoints must consume it and never bypass it.
  `ctx-ug7.8` implements sealed Tool inputs and the certified deterministic
  runtime sandbox.
- `ctx-ug7.16` enforces the accepted retirement of Gate-owned process execution,
  stable selected-root start/resume fences, and non-mutating migration
  inspection. `ctx-ug7.8.1` owns non-executing Tool definitions, registry
  descriptors, and board authoring; `ctx-ug7.8` owns certified host-private
  implementation packaging and runtime execution; `ctx-ug7.30` owns pure
  code-Gate profiles and the later explicit apply. ADR-0008 and `ctx-ug7.17` retire
  inline Formation verification in favor of explicit Gates; its legacy shape is
  inspection and replacement-Gate-bound removal input only.
- `ctx-8o9`, `ctx-ug7.5`, and `ctx-ug7.15` own CI and exact-candidate
  certification. `scripts/doc-lint.py` remains the active-spec hygiene gate.
