# CHROTE Formations - Supporting S0 Contracts

This file preserves the S0 implementation baseline. It is supporting material,
not the current source of truth. Current root specs, current code, the
[source-truth index](../../docs/source-truth-index.md), and later accepted ADRs
win when this packet conflicts with them. Accepted-target additions below
constrain future slices; they do not claim the current binary implements them.

Within the historical packet, these contracts still supersede archived `fm`,
`.chrote/orchestration`, dot-separated event names, `judgeFormationId`, mock
verdicts, and prototype counter ids.

`03-formations.js` remains a behavior and visual reference for the canvas. Its
`s1`/`f1`/`g1` counters, in-memory connection shape, stored gate verdicts, and
canned terminals are not persistence or run contracts.

## Files

```text
~/agents/<id>.toml
<workspace>/.formations/boards/<slug>.formation.toml
<workspace>/.formations/layout/<slug>.layout.toml
<workspace>/.formations/artifacts/<run-id>/...  # registered sanitized files only
<chrote-data>/formations/runs/<run-id>/events.ndjson
<chrote-data>/formations/runs/<run-id>/graph.snapshot.toml
<chrote-data>/formations/runs/<run-id>/bindings.private.toml
<chrote-data>/formations/runs/<run-id>/refs/       # writer-only/private
```

The shared formations package is the only writer for persona cards, board, and
layout files. The UI, `archon`, and agents all call that writer. Canonical run
authority lives under the configured CHROTE data root outside every configured
Files read/write root: append-only ledger, immutable graph snapshot, private
binding authority, sealed Tool inputs, and pending raw-redaction roots. The
generic Files API cannot list, read, write, rename, or delete this tree. Run APIs
return sanitized projections; only registered, currently authorized artifacts
may enter the existing File Peek surface. A same-named workspace file or
mutated inspection export is never execution or replay authority.

## Identity and Addressing

Stable ids are real addresses:

```text
brd_...  board
mis_...  mission
fmn_...  formation
tool_... tool (accepted target)
slot_... slot
gate_... gate
ver_...  legacy schema-1 verification
port_... dynamic Formation/Tool workflow payload port
edge_... connection
run_...  run
```

Board slugs, node titles, and `in[N]` / `out[N]` are aliases for examples,
tests, and human display. A writer resolves aliases once to stable ids before a
mutation. Persisted references use `<nodeId>:<portId>` and never `in[0]` or
`out[0]`.

Every graph input port accepts at most one incoming producer. JOIN is represented
by adding distinct required input ports and wiring one upstream edge to each. A
second edge to any occupied input is invalid on read, write, and run preflight;
it is never an implicit multi-edge JOIN. Output ports may fan out.

Slot identity is:

```text
slot.agentId       optional persona card id while authoring; required to run
slot.harness       optional harness variant id
```

The slot never stores a tmux session name. Assignment is staffing intent, not
proof that a slot is runnable. At run start, `agentId` plus the optional harness
resolves through the persona card. Preflight state is `unresolved`, `runnable`,
`ambiguous`, or `unavailable`, with a stable reason code such as `agent_missing`
when unresolved. Only a runnable resolution becomes the immutable exact binding
described below. After start, binding health is projected separately as
`runnable`, `unavailable`, or `stale`. Dispatch consumes the frozen binding
rather than resolving `session_stem` again. Implicit Oracle prefix stripping
such as deriving `susie` from `claude-susie` without a card-declared variant is
deferred until Perttu explicitly chooses that rule.

Persona ids are human slugs. The filename stem in `~/agents/<id>.toml` must
equal `[card].id`; mismatches are invalid. The default harness variant's
`session_stem` defaults to `card.id` when omitted, and the run ledger persona key
is `card.id`. No eval or future metadata field may override that identity spine.
Creating a persona with an existing id fails loud and leaves the existing card
byte-for-byte unchanged.

## Persona Card Schema

TOML, one file per durable persona.

```toml
schema = 1

[card]
id           = "susie"
display_name = "Susie"
kind         = "specialist"
summary      = "UI and web-experience designer."
tags         = ["design", "react", "taste:visual"]
status       = "active" # active | dormant | retired

[harness]
default = "claude-code"

[[harness.variant]]
id           = "claude-code"
session_stem = "susie" # default is card.id when omitted on the default variant
launch       = "claude --session susie"
source       = "/home/perttu/chrote/CLAUDE.md"

[[harness.variant]]
id           = "openai-codex"
session_stem = "codex-susie"
launch       = "codex --session codex-susie"
source       = "/home/perttu/.codex/config.toml"

[identity]
soul   = "~/.hermes/profiles/susie/SOUL.md"
skills = ["humanizer", "prototype-it"]
voice  = "Warm, direct, visual-quality focused."

[standards]
holds       = "visual-voice"
review_lens = "Legibility, hierarchy, and CHROTE fit."

[[note]]
ts    = "2026-06-03T16:30:00Z"
actor = "agent:archon"
text  = "React quality improved over sprint 3."
```

Required for S1: `schema`, `[card].id`, `[card].kind`, `[harness].default`, and
at least one `[[harness.variant]]`. Roster reads load only `[card]` plus the
harness summary. Deep inspection may follow `source`, `soul`, or skill pointers,
but list output must not inline source files, prompts, skill bodies, or secrets.

`[card].tags` is the only tag list. Bare entries are capabilities. The reserved
facet prefixes are `personality:`, `focus:`, and `taste:`.
`archon agent list --capable <tag>`, `archon agent new --capable a,b`,
`archon agent edit --add-capability t`, and `--remove-capability t` operate on
bare capability tags. `archon agent new --personality x` writes `personality:x`
into the same list.

`archon agent edit --note "..."` appends an optional `[[note]]` item with `ts`,
`actor`, and `text`. Notes are not evaluation schema. Future `[eval]` or other
metadata is unknown-preserved but not modeled in S1, and cannot override
`card.id` for slot references, default ledger persona keys, or session-stem
defaults.

Unknown keys and comments survive edits byte-for-byte unless the edit targets
that exact key. API responses may expose camelCase fields such as `displayName`;
TOML stays as written above.

## Board Definition Schema

TOML, one canonical graph per board.

```toml
schema   = 2
id       = "brd_01J9_sesssearch"
slug     = "session-search"
title    = "Improve session search"
rev      = 7
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id     = "mis_01J9_improve"
title  = "Improve session search"
goal   = "Make session search fuzzy and keyboard-first"
beadId = "bd-204"

[[formation]]
id    = "fmn_01J9_research"
type  = "peer" # solo | peer | flow | orchestrated
title = "Research huddle"

[formation.brief]
goal   = "Compare options."
beadId = "bd-204"
files  = ["src/SessionPanel.tsx"]

[[formation.input]]
id        = "port_01J9_research_in"
label     = "Input"
direction = "input"
kind      = "work"
acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]
required  = true
role      = "data"

[[formation.output]]
id        = "port_01J9_research_out"
label     = "Output"
direction = "output"
kind      = "work"
acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]

[[formation.slot]]
id         = "slot_01J9_peer_a"
label      = "Peer A"
agentId    = "susie"
harness    = "claude-code"
controller = false

[[gate]]
id        = "gate_01J9_review"
title     = "Review gate"
kinds     = ["human", "code"]
criterion = "Research is sound and safe to build."

[gate.code]
profileId      = "report.required-sections"
profileVersion = "1"
sections       = ["Findings", "Recommendation"]

[[connection]]
id   = "edge_01J9_start"
channel = "workflow"
from = "mis_01J9_improve:out"
to   = "fmn_01J9_research:port_01J9_research_in"

[[connection]]
id   = "edge_01J9_gate"
channel = "workflow"
from = "fmn_01J9_research:port_01J9_research_out"
to   = "gate_01J9_review:in"
```

The `direction`, `kind`, `acceptedMediaTypes`, `required`, and `role` port fields
shown above are board schema 2, the ADR-0006 accepted target. A schema-1
Formation input normalizes in memory to `direction=input`, `kind=work`,
`acceptedMediaTypes` equal to the stable full initial set, `required=true`,
`role=data`; an output normalizes to `direction=output`, `kind=work` with the
  same media set. Fixed Mission `out` accepts only `text/markdown` in this phase;
  Gate `in`/`pass` work ports use the full set;
Gate `fail` is `gate_feedback` and has no media set. Read/inspect does not rewrite. The
first schema-2 structural mutation performs one atomic content-preserving
migration and writes these defaults explicitly.

A schema-1 Gate fail edge into a work input loads in degraded inspection state
with `legacy_fail_route_requires_migration`. Structural read succeeds, but board
validation and run preflight reject execution until the edge is explicitly
rewired to a typed feedback data port or the evaluated source's `retry_control`
port. The writer never infers legacy annotated-work pushback.

Schema-1 edges touching `gate:judge` normalize to `channel=judge` only when they
form one unambiguous linear Formation-only send/return chain with no endpoint
cross-use. Ambiguous legacy judge paths load degraded with
`legacy_judge_channel_requires_migration` and cannot validate or run until
repaired.

A safely normalizable schema-1 board may start without canonical mutation.
Preflight writes a normalized schema-2 immutable run snapshot, records
`sourceBoardSchema=1` and `snapshotSchema=2`, and executes only that snapshot.
Unsafe normalization rejects before `run_started`.

Schema-1 inline `formation.verification` remains inspectable compatibility
state, but it is not safely normalizable because its verdict lacks exact
attempt/output identity and replay-safe block/revision finality. A board that
contains it fails schema-2 validation and run preflight with
`legacy_inline_verification_requires_migration`. The schema-2 writer does not
emit `verification_verdict`; definition or retirement is owned by
`ctx-ug7.17`.

A schema-2 `code` Gate references a host-owned, versioned, certified
deterministic in-process evaluator profile plus modeled non-secret parameters.
It cannot store or launch argv/shell commands, access network/secrets, read
undeclared host state, or write host state. Its closed observable-input policy
allows only the declared evaluated input and frozen bundle/parameters/policy;
locale/timezone are normalized and clock/entropy are frozen or denied. Profile
admission also requires a total evaluator under deterministic host-metered fuel.
The frozen policy caps input bytes, result bytes, and operations, and the host
contains panics. Exhaustion or panic becomes Gate-scoped
`error(code=gate_evaluator_error)` with no kind result/verdict/route; it cannot
wedge run wall-clock finality. Admission runs repeat vectors with expected
verdict/reason/evidence hashes. Run
preflight freezes a `RunGateBinding` including content-addressed evaluator
implementation-bundle and determinism-policy hashes. A completed durable kind
result is reused on replay. A crash before that event may repeat only when the
exact evaluator bundle and policy remain available and exact; upgrade, missing
bundle, or mismatch fails loud without evaluation or routing. Before every
initial or recovery evaluation, the engine also resolves and revalidates the
exact authoritative input bytes against `inputSha256`; a marker, summary, hash,
or drifted artifact is never substituted. Lost Redact=true input appends
terminal `run_failed(code=redacted_input_unavailable)`; non-redacted integrity
drift appends Gate-scoped `error(code=gate_input_integrity_failed)` followed by
terminal `run_failed(code=gate_input_integrity_failed)` whose
`failureCause={kind=error,errorSeq}` names that error and fails the exact Gate
attempt. Redacted loss first records bounded Gate/attempt/input context, but its
terminal failure keeps the source input sequence as provenance and
`failureCause={kind=none}` as required by ADR-0005. Both exactly dispose every
open attempt, slot dispatch, and Tool lease. Neither path emits a kind result,
verdict, or route. A profile that needs an OS process is rejected until a
separate ledger-before-spawn process-fence or retirement decision in
`ctx-ug7.16` lands. Current schema-1
`commandArgv`/explicit `commandShell` Script Gates remain compatibility behavior,
but a board containing them is not safely normalizable to schema 2 and fails
with `legacy_script_gate_requires_fenced_migration` rather than inheriting Tool
replay semantics.

The accepted target adds a bounded Tool node. The exact profile registry and
packaging belong to `ctx-ug7.8`; board-authored commands are not Tool profiles.

```toml
[[tool]]
id             = "tool_01J9_normalize"
title          = "Normalize report"
profileId      = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id       = "port_01J9_normalize_in"
label    = "Report"
direction = "input"
kind     = "work"
acceptedMediaTypes = ["application/json"]
required = true
role     = "data"

[[tool.output]]
id    = "port_01J9_normalize_out"
label = "Normalized report"
direction = "output"
kind  = "work"
acceptedMediaTypes = ["application/json"]
```

Tool definitions store only profile identity/version constraint plus modeled
non-secret parameters. Run start freezes the resolved profile version/content
hash, parameters, effective policy hash, and content-addressed execution-bundle
hash. The bundle covers executable/script/toolchain identity, argv template, cwd
contract, normalized non-secret allowlisted environment values,
supervisor/fence policy, and limits. The first profile class is certified pure
and deterministic: network, secrets, undeclared environment/filesystem reads,
and external writes are denied; locale/timezone are normalized; clock/entropy
are frozen or denied; inputs are sealed read-only; and outputs are confined to
one private run root. Profile admission requires repeat vectors with expected
output hashes. Executable, argv, isolated cwd, environment, limits, and
process supervision/fencing and replay enforcement remain host-owned. Preflight
rejects the Tool before `run_started` if the frozen supervisor/fence policy
cannot be provided. A completed Tool lease is never rerun; an
unresolved lease may rerun only after its latest process scope is proven
quiescent and with identical input/profile/parameter/policy/determinism-policy/execution-bundle
hashes. Effectful profiles are
deferred. Mission, Formation, and Tool workflow
ports declare stable id, label, direction, accepted payload kind, and input
required/role semantics. Every `work` port also declares a non-empty
`acceptedMediaTypes` subset of `text/plain`, `text/markdown`, and
`application/json`; incompatible media rejects before attempt start. Full JSON
Schema remains deferred.

Missions retain fixed port `out` as the only run-start output in this phase. The
authored objective is encoded as `mission-objective-utf8-v1`: bounded UTF-8 with
BOM rejected, CRLF/CR normalized to LF, and no other whitespace or newline
added/removed. Its media type is `text/markdown`,
`rootInputProjection.sha256` hashes those exact bytes, and every first
destination must accept that media or preflight
fails `mission_objective_media_incompatible` before `run_started`. Run start
rejects unknown port keys. Gates retain
fixed addresses `in`, `pass`, `fail`, and the reserved `judge`
evaluation-control socket. `pass` forwards the evaluated work unchanged and
`fail` carries typed feedback. `judge` is not a typed workflow payload port: it
permits exactly one judge send and one judge return or neither on an executable
board, and carries evaluation control/evidence only. Schema-2 judge connections
persist `channel=judge`; ordinary connections default to `channel=workflow`.
Structural validation rejects generic judge wiring, extra producers/consumers,
and workflow cross-use; executable preflight also rejects an unpaired or
non-linear judge path.
Gate `in` accepts `work`, `pass` routes that same `work`, and `fail` routes
`gate_feedback`. Formation and Tool ports are explicit arrays with stable
`port_...` ids. `retry_control` is allowed only on an optional Formation input;
Tool inputs use `data`.

Mission, Formation, and Tool outputs produce only `work`, and Tool inputs accept
only `work`. Formation inputs may declare `work` or `gate_feedback`, including
ordinary `data` feedback inputs and the optional `retry_control` shape, but only
the fixed Gate `fail` port produces `gate_feedback`. Shared read, mutation, and
preflight validation reject every other feedback producer.

Shared structural validation runs on read and mutation. It rejects unknown
endpoints, direction/kind mismatches, a second producer for an input, and every
cycle in the `channel=workflow` graph after validated `retry_control` edges are
removed. The remaining data graph must be acyclic; only the narrow direct-source
retry-control loop below is legal. Draft
boards may persist an unwired required port or one half of a judge relationship
while being authored. Explicit board validation and run preflight reject every
reachable required input without a producer and every incomplete judge pair
before dispatch.

A port's declared kind governs successful routable delivery. `unavailable` and
`error` may terminate any declared output as non-delivered system outcome
envelopes; recording either under that port is not a kind mismatch.

A node attempt freezes every required ref and every optional ref already present
at `node_started`. Later deliveries never mutate it or silently combine a new
value with stale JOIN inputs. A late optional `data` delivery records
`node_input_ignored` with `reason=late_optional`; a late required delivery is an
invalid execution path and fails loud except for the direct-source revision cycle
below. In the first mixed-workflow contract, only `gate_feedback` on an optional
Formation input whose persisted role is `retry_control` can trigger an in-graph
bounded next attempt. Its evaluated source node and attempt must match the
receiving node's exact completed attempt; the new attempt reuses that attempt's
frozen work refs. Duplicate, stale, or mismatched feedback never dispatches.
Explicit operator resume is the separate blocked-run trigger. Attempt, dispatch,
and wall-clock limits apply.

If the graph quiesces before a node starts and at least one required input was
delivered while another can no longer be produced, the coordinator does not
leave `node_waiting` active or attempt success. It appends
`error(code=unsatisfied_required_input,errorScope=node)` naming the node and
waiting sequence, then a non-resumable `run_blocked` with `blockScope=node`,
that `blockedNodeId`, `reason=unsatisfied_required_input`, empty
`openDispatches`/`retryTargets`, `resumeAllowed=false`,
`resumePolicy=new_run_required`, and no `nextEpoch`. The node projects `blocked`;
its readiness counts remain evidence. A corrected graph starts a new run.

Gate definitions do not store run verdicts, default verdicts, or gate-level
`onFail`. Fail behavior is determined by wiring: an unwired `fail` port blocks;
each failed evaluation creates one stable typed `gate_feedback` object, and all
fail-edge traversals reference it. An unwired fail therefore has zero deliveries
and records one aggregate
`gate_verdict(verdict=fail,routePort=fail,routedEdges=[])`. After in-flight and
independent work settles, a non-resumable
`run_blocked(reason=unwired_gate_fail)` uses `blockScope=gate`, the exact
`blockedGateId`, no `blockedNodeId`, empty `openDispatches`/`retryTargets`,
`resumeAllowed=false`, `resumePolicy=new_run_required`, and no `nextEpoch`. The
Gate projects blocked while retaining its FAIL verdict; its completed upstream
Formation remains completed. Fan-out does not duplicate feedback identity. `pushback` is a
deliberately narrow route action, not another verdict. It is valid only when the
direct evaluated source is the receiving Formation, that Formation's entire
connected workflow-output frontier is the single edge into this Gate, and the
Gate fail frontier is the single edge back to its `retry_control`. The first Gate
evaluation allocates one stable run-local revision-cycle id.
Matching feedback starts source attempt N+1 only while the frozen authoritative
inputs remain live or durably exact; otherwise redacted replay fails terminally.
Its output opens Gate attempt N+1
linked by revision-cycle id, feedback id, prior Gate sequence, and new source
attempt. This is the sole permitted late-required delivery. Mixed fail fan-out, side-output
delivery, downstream replay, and non-source pushback are invalid. In-formation
verification retains its recorded schema-1 `onFail` value for inspection only;
schema-2 never evaluates or routes it.

Gate pass records a new edge traversal while preserving the durable evaluated
ref/projection and its original source node, source port, output sequence, and
attempt. During fresh redacted execution it also passes the same exact live
authoritative payload; it never substitutes durable evidence. Gate
fail does not forward or rewrite that work. Formation-judge output contributes
verdict evidence only; it never becomes downstream work implicitly. The judge
exit must return exactly one bounded `judgeResult` with `verdict`, safe `reason`,
and typed `evidence`. Missing, malformed, or multiple returns append
`judge_attempt_failed` with `code=invalid_judge_result`, complete that judge
attempt as failed, block the Gate, and route neither branch.

An executable Gate's persisted `kinds` array must be a non-empty,
duplicate-free subset of `code`, `formation`, and `human`. Board validation and
run preflight reject an empty array, duplicate entry, or unknown value before
`run_started`; file order never supplies multiplicity or precedence. Gate kinds
use one deterministic all-of policy. The engine canonicalizes the declared set
into `code`, then `formation`, then `human`, regardless of file/UI order. Each declared kind must pass for the aggregate verdict to pass. A strict
fail appends its durable result and short-circuits later kinds; the aggregate
verdict is fail and later declared kinds are `not_run`. A code evaluator
boundary error is not a fail verdict: it appends a Gate-scoped
`error(code=gate_evaluator_error)`, including for deterministic fuel/input/result
exhaustion or a contained panic. After independent work settles, absent an
execution-final event, it appends `run_blocked` with `blockScope=gate`, only the
Gate id, empty open dispatches/retry targets, `resumeAllowed=false`,
`resumePolicy=new_run_required`, and no `nextEpoch`; it records no route. An
invalid Formation judge follows the
`judge_attempt_failed` block below.

Each completed `code` or `formation` check appends/fsyncs one unique
`gate_kind_result` keyed by Gate attempt and kind before the next kind starts.
The code result also exact-matches the frozen `RunGateBinding`, input hash,
profile/evaluator-bundle/parameter/policy/determinism-policy hashes, and pure
evaluator result hash. It also proves the exact authoritative input bytes were
available and revalidated for `inputSha256`. A crash before this event repeats only with that exact
certified bundle/policy; a runtime or profile upgrade cannot substitute.
The formation result references the exit judge result; replay never reruns a
completed kind. Human is last. After prior kinds pass, one
`human_input_requested` records their result sequences and projects the Gate/run
as `waiting_human`; no `gate_verdict` or route exists yet. One matching
`human_verdict_recorded` completes that kind and continues aggregation in the
same epoch. A stale, duplicate, wrong-Gate, or wrong-attempt decision is rejected.
The request has no independent `timeoutSeconds`. It remains outstanding until
one matching decision, cancellation, or the immutable run wall-clock limit;
limit exhaustion follows the terminal run-limit rule rather than inventing a
human default verdict.

Exactly one `gate_verdict` is accepted after the first durable fail or after all
declared kinds pass. Its `perKind` map contains every declared kind with
`pass`, `fail`, or `not_run`. On fail, `reason` names the first failing kind's
bounded safe reason; on pass it is the stable aggregate reason
`all_declared_kinds_passed`. Invalid or waiting kinds append no aggregate
verdict and route neither branch.

Judge chains use the reserved evaluation-control socket:

```toml
[[connection]]
id = "edge_01J9_judge_send"
channel = "judge"
from = "gate_01J9_review:judge"
to = "fmn_01J9_judge_a:port_01J9_judge_a_in"

[[connection]]
id = "edge_01J9_judge_mid"
channel = "judge"
from = "fmn_01J9_judge_a:port_01J9_judge_a_out"
to = "fmn_01J9_judge_b:port_01J9_judge_b_in"

[[connection]]
id = "edge_01J9_judge_return"
channel = "judge"
from = "fmn_01J9_judge_b:port_01J9_judge_b_out"
to = "gate_01J9_review:judge"
```

Judge connections form one linear Formation-only entry-to-exit chain. They
order bounded `judgeContext`/`judgeResult` control and never deliver
`PortPayload`. Branch, JOIN, repeated node, Tool participation, side entry/exit,
or workflow use of chain endpoints is invalid. A complete judge channel is
required if and only if `gate.kinds` contains `formation`; draft writes may
temporarily retain an incomplete/mismatched relationship, but board validation
and run preflight reject it. Before the next member dispatches, one strict
`judge_result` is appended and fsynced for the current member, keyed by
Gate/judge attempts and chain index with context/result hashes and prior-result
sequences. Invalid output instead appends and fsyncs `judge_attempt_failed` with
the same judge key, context hash, prior-result sequences, stable code/reason, and
related capture sequence. Exactly one of those completion events is accepted for
each judge key; the failed form blocks the Gate and dispatches no next member.
After that failed completion is durable, the coordinator appends `run_blocked`
only after it prevents new Gate/dependent dispatch and already in-flight and
independent branches settle and record evidence. The block has
`blockScope=gate`, `blockedNodeId=judgeNodeId`, `blockedGateId=gateId`, empty `openDispatches` and
`retryTargets`, `reason=invalid_judge_result`, `resumeAllowed=false`,
`resumePolicy=new_run_required`, and no `nextEpoch`. This contract has no
same-run judge retry; corrected configuration or staffing starts a new run. If
quiescence produces an execution-final event, finality takes precedence and the
coordinator does not append a later block.
Replay rebuilds durable prior-result state from accepted `judge_result` events
and never reruns or reparses completed judge capture. Dispatch of the next exact
`judgeContext` additionally requires the evaluated authoritative input to remain
live or durably exact; otherwise recovery appends terminal
`run_failed(code=redacted_input_unavailable)`. Conflicting completion events for one judge key are
a loud ledger error.

## Layout Sidecar Schema

Layout is presentation only. Deleting it never changes the graph.

```toml
schema   = 1
boardId  = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "fmn_01J9_research"
x = 420
y = 160

[[edge]]
id = "edge_01J9_gate"
lane = "mid-2"
```

`boardRev` lets the UI detect stale layout against a newer board. Missing layout
entries auto-place. Extra layout entries for deleted nodes or edges are ignored
and cleaned by the writer on the next layout save.

Selected run/node, inspector state, and terminal Peek open/focus/tile/hide state
and geometry are user-local dashboard presentation. They are not board or layout
structure, run-ledger truth, durable tmux target identity, or tmux lifecycle
authority. Each Peek resolves one exact slot dispatch's opaque
`sessionTargetId` plus `dispatchId`, `targetLeaseId`, binding, and frozen target
fingerprint; an attempt may expose several. Live run Peek is authorized only
while that exact dispatch owns the matching active/unmatched occupancy record,
or its exact non-authorizing terminal hold still occupies the unchanged pane.
A quarantine is ambiguous and never authorizes live run Peek. Once occupancy is
replaced by a result/final release receipt, the target may be reused: the old run
shows captured historical evidence or explicit `pane_moved_on`, never the
current live bytes as if they belonged to that attempt. The UI may separately
offer **Open current session**, labeled outside run evidence. Peek must surface
unavailable/stale rather than look up another session by name or trust the
stable opaque handle alone.

## Run Ledger Envelope

Each run is append-only NDJSON. Every line is one JSON object with this minimum
envelope:

```json
{
  "schema": 2,
  "ts": "2026-06-03T16:05:00Z",
  "runId": "run_01J9...",
  "seq": 1,
  "type": "run_started",
  "actor": "agent:archon",
  "boardId": "brd_01J9_sesssearch",
  "boardRev": 7,
  "missionId": "mis_01J9_improve",
  "beadId": "bd-204",
  "epoch": 0,
  "attempt": 0,
  "data": {
    "boardSlug": "session-search",
    "boardPath": ".formations/boards/session-search.formation.toml",
    "sourceBoardSchema": 1,
    "snapshotSchema": 2,
    "runAuthorityId": "auth_01J9...",
    "graphSnapshotSha256": "sha256:...",
    "privateBindingsSha256": "sha256:...",
    "bindingProjectionSha256": "sha256:...",
    "runRoot": {"kind": "mission", "nodeId": "mis_01J9_improve"},
    "rootInputProjection": {
      "classification": "authored_config",
      "sourceKind": "mission_objective",
      "encoding": "mission-objective-utf8-v1",
      "mediaType": "text/markdown",
      "sha256": "sha256:...",
      "text": "Make session search fuzzy and keyboard-first"
    },
    "limits": {"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false}
  }
}
```

All schema-2 events include `schema=2`, `ts`, `runId`, monotonic `seq`, `type`,
and `actor`. Events
that touch graph objects include the relevant `boardId`, `boardRev`, `missionId`,
`nodeId`, `slotId`, `gateId`, `edgeId`, `attempt`, or `epoch`. `run_started`
is the first event and must include top-level board id/revision plus opaque
authority id, exact graph/private-binding/projection hashes, `runRoot`, typed
authored-config root-input projection, and run id. `missionId` and optional `beadId` are top-level only
for a Mission root, and `runRoot.nodeId` must equal that `missionId`; an isolated
Formation root omits both.

A ledger without `schema` is schema 1 and projects with its recorded legacy
semantics. Schema-1 events are never reinterpreted as exact Gate pass-through,
typed feedback, or schema-2 input refs. One ledger cannot mix event schemas.
Schema-1 runs remain inspectable but are not resumable by the schema-2 engine;
resume fails with `legacy_run_requires_new_run`.

Run state is epoch-aware:

```text
run_started seq=1, epoch=0
run_blocked closes an epoch and may be resumed explicitly
run_resumed opens the next epoch after an operator resume
human_input_requested enters waiting_human until its exact decision or cancel
run_cancel_requested enters canceling and forbids dispatch, replay, or resume
run_succeeded, run_failed, and run_canceled are final for execution
```

`seq` is monotonic for the whole run and never resets. `run_started` appears
exactly once at `seq=1`. `epoch` starts at `0` and increments only when an
operator explicitly resumes a blocked run. A `run_blocked` event stops dispatch
for that epoch. `archon run resume <runId>` or the equivalent API action is
accepted only when the latest projection is blocked and `resumeAllowed=true`;
it appends `run_resumed` as the first event in `epoch + 1` with
`data.resumedFromSeq` set to the `run_blocked` sequence. The engine must not
append another execution event after `run_succeeded`, `run_failed`, or
`run_canceled`. The reconciler may still append non-authorizing
`slot_binding_observed` or `artifact_observed`; those events cannot change the
run/node outcome, reopen an epoch, or authorize dispatch.
For `resumeMode=retry-failed-producer`, `run_resumed.retryTargets` must exactly
match the blocked event, and each subsequent `node_started(reason=resume)` uses
the frozen failed-attempt inputs and attempt N+1. This phase permits exactly one
target. No target may have delivered an edge in the prior attempt.

Ledger replay is idempotent. A `slot_dispatch` is recorded before the prompt is
sent. If the recovery reconciler sees a dispatch without a `slot_result`, it
re-attaches capture or records a loud `error`; it never blindly re-delivers the
prompt. See `docs/adr/0001-formations-run-recovery-contract.md`.

## Run Snapshot And Revision Rules

In the accepted target, run start reads the current board definition and
validates `runRoot`. A Mission root selects its reachable graph including judge
chains. An isolated Formation root is valid only when it has a non-empty brief
and exactly one required `data` input accepting `work` and `application/json`;
incoming board edges are outside the root. Preflight encodes the parsed brief as
`formation-brief-jcs-v1`: RFC 8785 canonical UTF-8 JSON over exactly `{goal,
beadId, files, links}`, with missing scalars/arrays normalized to `""`/`[]`, array
order preserved, and no trailing newline. Those exact bytes are the synthetic
`run_seed` work payload, use `mediaType=application/json`, and determine
`seedSha256`. Optional inputs remain absent, `retry_control` is
not seeded, downstream edges are not traversed, and every other required-input
shape rejects before `run_started`. Preflight resolves every declared slot, Tool
profile, and schema-2 code-Gate evaluator profile in that executable subgraph.
Before appending `run_started`, the single writer creates and fsyncs one
writer-only authority directory with write-once snapshots under the host-private CHROTE data root. It
contains the canonical ledger, normalized graph snapshot, private bindings, and
run-private refs; none is beneath a configured Files root. It computes SHA-256
over the graph snapshot, private binding authority, and safe binding projection.
Every dispatch/recovery read exact-verifies those hashes. Before admission,
missing/altered authority rejects with `run_authority_integrity_failed` and no
event. After `run_started`, it appends a scoped error with that code and terminal
`run_failed(code=run_authority_integrity_failed)` with exact dispositions; it
sends/evaluates nothing.

The normalized graph snapshot embeds a closed private
`authoredConfigManifest`, covered by `graphSnapshotSha256`. Each stable
`(sourceKind,nodeId)` entry is exactly `{classification="authored_config",
sourceKind,nodeId,encoding,mediaType,sha256}` and classifies one exact snapshot
field: Mission objective, whole Formation brief, or Gate criterion. Mission and
brief entries use the root encodings above; Gate criteria use
`sourceKind=gate_criterion`, `gate-criterion-utf8-v1`, and `text/plain`. Entries
sort by `(sourceKind,nodeId)`. The writer rejects a copied value without its
matching entry, a hash/role/encoding/media mismatch, an extra entry, and every
other source kind. Root-input, root-derived payload, and criterion projections
must exact-match their corresponding manifest entry as well as the snapshot
value. This is the explicit ADR-0005 exception for configuration already durable
in the canonical board; it may outlive a later board edit/deletion. No
runtime/external input, capture, output, evidence, secret, or composed prompt
enters that exception or snapshot.
Human prompt and PASS/FAIL labels use only the closed fixed-system templates;
dynamic human-request text is invalid as a durable event field.

If startup finds a writer-private authority directory without a valid seq-1
`run_started`, it is not a run and sends nothing. Recovery first cleans and
fsyncs every pending-redaction obligation/raw target, then deletes the orphan
tree idempotently and fsyncs its parent. Unprovable cleanup/identity quarantines
the tree as non-authorizing with no public bytes or replay handle; it is never
adopted as a run.

Run preflight returns one `SlotResolution` per selected declared slot; unassigned
is `unresolved/agent_unassigned`. It starts only when every resolution is
runnable. A successful run
then writes one host-private `RunSlotBinding` per slot with binding id,
slot/agent/harness identity, card path/hash, and exact runtime target: adapter kind, tmux server/socket
identity, session id/name, window id, pane id, resolved cwd/root, resolution
time, target fingerprint, private canonical `targetKey`, and server-issued opaque
`sessionTargetId`. `targetKey` identifies one pane incarnation from canonical
tmux server identity, pane id, and pane-birth fingerprint; it is not derived from
a display name or raw socket-path spelling. The durable resolver registry
enforces a one-to-one mapping between `targetKey` and `sessionTargetId`: every
independent resolution of that exact pane incarnation returns the same opaque
handle, while a replacement pane receives a new key and handle. The target
fingerprint commits to adapter/harness identity, foreground-process start
identity, cwd/root, and pane-incarnation evidence. Raw routing and `targetKey`
remain private. The API exposes only a hash-linked `RunSlotBindingProjection`
with opaque target id and safe display/root metadata. Each slot dispatch
references its own binding, target, and fingerprint, so a multi-slot attempt may
expose several sessions. Runtime health is projected from appended evidence and
never mutates the frozen identity.
Preflight rejects two selected slots that resolve to the same private
`targetKey` (and therefore the same `sessionTargetId`) with
`duplicate_session_target` before `run_started`. Across
runs, runnable resolution is not dispatch ownership: the host-wide target lease
below is the atomic authority immediately before send.

The same private binding authority stores one `RunToolBinding` per reachable Tool:
`toolBindingId`, node/profile ids, exact profile version/content SHA-256,
normalized parameters and their SHA-256, effective-policy and
determinism-policy SHA-256 values, and immutable execution-bundle SHA-256. The content-addressed bundle covers executable,
script/toolchain identity, argv template, cwd contract, normalized non-secret
allowlisted environment values, supervisor/fence policy, and limits; a mutable
host path is not execution identity.
The board snapshot keeps the authored constraint; the resolved binding is
execution authority. Preflight rejects a reachable Tool before `run_started` if
the frozen supervisor/fence policy is unavailable.

The private authority also stores one `RunGateBinding` per reachable schema-2
code Gate: `gateBindingId`, Gate/profile ids, exact profile version/content
SHA-256, content-addressed evaluator-bundle SHA-256, normalized parameters and
SHA-256, effective-policy SHA-256, determinism-policy SHA-256, and positive
`maxInputBytes`, `maxResultBytes`, deterministic `maxOperations`, and
`resultEncoding=decision-result-jcs-v1`. Only the
allowlisted in-process certified deterministic evaluator class is accepted in
this phase.

Dispatch and recovery consume the frozen per-slot binding and never re-resolve a
mutable persona card or fall back to a same-named session. After acquiring the
target lease and immediately before send, the adapter rechecks the frozen
fingerprint and persists it in `slot_dispatch`; changed card authority, harness,
foreground process, cwd/root, or pane incarnation appends a stale binding
observation and sends nothing. Recovery may reattach only the same qualified
pane/fingerprint; rebinding to a replacement target requires a new run.
Non-runnable resolution and unhealthy bindings fail loud. The current
`sessionStem`/`sessionRef` fields remain compatibility evidence until this target
lands; they are not sufficient exact addressing. The snapshot never inlines
source files, prompts, skill bodies, or secrets.

`run_started` records the source board path, board id, board rev, opaque
`runAuthorityId`, graph/binding/projection hashes, and run root; it exposes no
host-private path. If a caller supplies an expected
board rev or ETag and it does not match the current board, run start fails with
the same conflict semantics as a board write and appends no event. Resume
exact-verifies and replays the private graph/binding authority and never revalidates or rewrites current
board/layout/persona files. Board, layout, and persona edits after `run_started`
apply only to future runs. Generic Files operations against a private authority
id/path or same-named workspace substitute are denied or ignored as authority.
Runs never write persona cards, board definitions, or layout definitions.

## Event Payload Schemas

Each event uses the envelope above. Fields listed here are inside `data` unless
the envelope already names the field. Unknown `data` keys are allowed for
forward-compatible private readers only when the writer extension is
schema-registered and redaction-classified; Redact=true rejects an unregistered
field before append. Public run-detail, bounded-event, SSE, CLI, and UI projection
uses an event-type safe-field allowlist and drops/rejects every unknown or private
key rather than passing its value through. The required keys below must be
present.

| Event type | Required payload |
|---|---|
| `run_started` | `boardSlug`, `boardPath`, `sourceBoardSchema`, `snapshotSchema`, opaque `runAuthorityId`, `graphSnapshotSha256`, `privateBindingsSha256`, `bindingProjectionSha256`, `runRoot`, exact closed `rootInputProjection` classified `authored_config` with source kind, versioned encoding/media/SHA-256, and canonical `text`, `limits` (`maxDispatch`, `maxAttempts`, `wallClockSeconds`, `redact`); private paths never appear, and board/revision plus conditional Mission ids stay in the envelope |
| `run_resumed` | `resumedFromSeq`, `resumedBy`, `resumeMode` (`reattach`, `retry-failed-producer`), bounded redaction-safe `reason`, `openDispatches`, exact `retryTargets`; `reattach` preserves the blocked event's exact unmatched-dispatch set and never creates a dispatch, while failed-producer retry requires `openDispatches=[]` and exactly one target |
| `node_waiting` | `nodeId`, `neededInputs`, `readyInputs`, `totalInputs`, `waitingFor` (`edgeId` or `portId` list) |
| `node_input_ignored` | `nodeId`, `toPortId`, `inputRef`, `reason` (`late_optional`, `duplicate_feedback`, `stale_feedback`, `mismatched_feedback`), `relatedAttempt` |
| `node_started` | `nodeId`, `nodeKind` (`mission`, `formation`, `tool`, `gate`), `attempt`, `reason` (`initial`, `resume`, `pushback`, `revision-cycle`, `judge`); ordinary attempts require immutable durable `inputRefs`, judge attempts instead require `contextEncoding=judge-context-jcs-v1`, immutable `judgeContextSha256`/`priorResultSeqs`; optional `triggerFeedbackId`/`priorGateSeq` |
| `slot_binding_observed` | `bindingId`, `slotId`, `sessionTargetId`, `health` (`runnable`, `unavailable`, `stale`), stable `reason`, `observedAt`, `relatedSeq` |
| `slot_dispatch` | `dispatchId`, `targetLeaseId`, `nodeId`, `attempt`, `slotId`, `agentId`, `harness`, `bindingId`, `sessionTargetId`, `targetFingerprint`, `promptSha256`, boolean `nativeAck`, `recordedBeforeSend=true`; `nativeAck` is never adapter text, no prompt bytes/ref/path/authority id is durable, and the fingerprint is revalidated under the acquired lease immediately before the same already-hashed in-memory byte slice is sent at most once |
| `slot_result` | `dispatchId`, `targetLeaseId`, `nodeId`, `attempt`, `slotId`, `agentId`, `bindingId`, `sessionTargetId`, `targetFingerprint`, `status` (`ok`, `error`, `needs-review`), terminal `capturedRange`, artifact ids, exact `sentinel`, closed `turnClosureProof`; timeout or a sentinel followed by unclosed output is never a result |
| `tool_dispatch` | `toolLeaseId`, `nodeId`, `attempt`, `toolBindingId`, `inputManifestSha256`, `inputHashes`, `profileSha256`, `parametersSha256`, `policySha256`, `determinismPolicySha256`, `executionBundleSha256`, private lease-root authority or redacted `redactionObligationId`, `recordedBeforeExecute=true` |
| `tool_process_launch` | `toolLeaseId`, unique `launchId`, `nodeId`, `attempt`, monotonically increasing `generation` starting at 1, unique opaque non-PID `processScopeId`, opaque `deadlineAuthorityId`, `recordedBeforeSpawn=true`; private supervisor scope and immutable deadline-authority records are durable first, exact-match the open lease/launch tuple, and each launch consumes dispatch and wall-clock limits |
| `tool_result` | `toolLeaseId`, `launchId`, `generation`, `nodeId`, `attempt`, `status` (`ok`, `error`, `timeout`), full durable canonical output-projection map with explicit exactness, `outputHashes`, authoritative new `artifactRegistrations`, `artifacts`, optional bounded non-routable `displayEvidence`, `timing`; one fsynced event atomically closes the lease/latest launch and registers its new artifact ids |
| `node_output` | `nodeId`, `status` (`done`, `needs-review`, `blocked`, `failed`), durable `PayloadProjection` values keyed by stable output port id, optional `reportArtifactId`, stable-order `artifactIds` and `diffArtifactIds`, `producedBy`, `timing`, `deliveredEdges`; every top-level artifact id is already durably registered, and Mission `out` requires the exact classified `rootDerivedPayloadProjection` matching `run_started.rootInputProjection` |
| `gate_evaluating` | `gateId`, `gateAttempt`, `nodeId`, `kinds`, typed `criterionProjection` with `classification=authored_config`, `sourceKind=gate_criterion`, `encoding=gate-criterion-utf8-v1`, and `mediaType=text/plain`, `inputRef`, `judgeChain`, optional `revisionCycleId`/`triggerFeedbackId`/`priorGateSeq`; no dynamic value is interpolated into the durable criterion |
| `gate_kind_result` | `gateId`, `gateAttempt`, `kind` (`code`, `formation`), strict `verdict` (`pass`, `fail`), bounded safe `reason`, typed `evidence`, `evaluatedInputRef`, `resultEncoding=decision-result-jcs-v1`, `resultSha256`, `relatedSeqs`; code additionally requires `gateBindingId`, `inputSha256`, `profileSha256`, `evaluatorBundleSha256`, `parametersSha256`, `policySha256`, `determinismPolicySha256`; unique per Gate attempt/kind and fsynced before the next kind |
| `judge_result` | `gateId`, `gateAttempt`, `judgeNodeId`, `judgeAttempt`, `chainIndex`, `contextEncoding=judge-context-jcs-v1`, `contextSha256`, `priorResultSeqs`, strict `result`, `resultEncoding=decision-result-jcs-v1`, `resultSha256`; completes that judge Formation attempt and is fsynced before the next member dispatch |
| `judge_attempt_failed` | `gateId`, `gateAttempt`, `judgeNodeId`, `judgeAttempt`, `chainIndex`, `contextSha256`, `priorResultSeqs`, `code=invalid_judge_result`, bounded safe `reason`, `relatedSeq`; completes that judge Formation attempt as failed and blocks the Gate |
| `gate_verdict` | `gateId`, `gateAttempt`, `verdict` (`pass`, `fail`), exact all-declared-kind `perKind`, `kindResultSeqs`, `evaluatedInputRef`, `routePort` (`pass`, `fail`), `routedEdges`, bounded redaction-safe `reason`; PASS preserves the exact authorized live value and durable ref/projection, while FAIL creates exactly one stable typed `feedbackPayload` referenced by zero or more fail-edge traversals (zero blocks); invalid/waiting evaluation emits no aggregate verdict |
| `artifact_attached` | `artifactProjection` with new stable `artifactId` (available safe ref or unavailable/redacted/expired metadata), discriminated non-Tool `source` (`slot`, `gate`, or `system`); appended/fsynced before any later reference |
| `artifact_observed` | existing `artifactId`, `availability` (`available`, `unavailable`, `redacted`, `expired`), `artifact` required only for `available` with matching id and exact first-established descriptor, stable `errorCode` required for all other states, `observedAt`, `relatedSeq` |
| `escalation_raised` | `trigger`, `severity` (`info`, `needs-attention`, `stop`), bounded redaction-safe `reason`, `source` (`system`, `agent`, `human`), `nodeId`, `gateId`, `blocks` |
| `human_input_requested` | `gateId`, `gateAttempt`, `nodeId`, bounded closed fixed-system `promptProjection` using template `gate-human-verdict-v1`, exact closed fixed-system `choiceProjections` object keyed by `pass` and `fail`, `requestedBy`, `evaluatedInputRef`, exact `completedKindResultSeqs`; fields never interpolate runtime input/output/evidence/secrets, the request is unique for that Gate attempt and projects `waiting_human`, and it has no independent timeout or default verdict |
| `human_verdict_recorded` | `gateId`, `gateAttempt`, `nodeId`, `verdict` (`pass`, `fail`), bounded safe `reason`, `requestedSeq`, `decidedBy`; exactly once for the matching outstanding request |
| `error` | `code`, bounded redaction-safe template `message` with no raw adapter/error text, `boundary` (`engine`, `writer`, `adapter`, `tmux`, `schema`, `operator`, `evaluator`), `errorScope` (`run`, `node`, `gate`, `slot`, `tool`), conditional graph identity, `recoverable`, `relatedSeq` |
| `run_blocked` | stable bounded redaction-safe `reason`, `blockScope` (`run`, `node`, `gate`), conditional `blockedNodeId`/`blockedGateId`, `resumeAllowed`, `resumePolicy` (`retry_failed_producer`, `reattach_only`, `new_run_required`), `openDispatches`, `retryTargets`, conditional `nextEpoch`; exact policy invariants below are writer-enforced |
| `run_cancel_requested` | bounded redaction-safe `reason`, `requestedBy`, exact `openNodeAttempts`, exact `openSlotDispatches`, exact `openToolLeases`; unique per run, appended/fsynced before cancellation work, and makes the writer reject new dispatches, launches, results, outputs/routing, ordinary replay/rerun, and resume |
| `run_canceled` | `cancelRequestSeq`, bounded redaction-safe `reason`, `requestedBy`, exact `nodeAttemptDispositions`, exact `slotDispatchDispositions`, exact `reconciledToolLeases`, `final=true`; all three arrays exactly cover the named request snapshots |
| `run_failed` | `code`, bounded redaction-safe `reason`, `unrecoverable`, `relatedSeq`, typed `failureCause`, exact `nodeAttemptDispositions` covering every still-open node attempt (possibly empty), exact `slotDispatchDispositions` covering every still-open slot dispatch (possibly empty), exact `toolLeaseDispositions` covering every still-open Tool lease (possibly empty), `final=true` |
| `run_succeeded` | optional `summaryArtifactId`, stable-order `outputArtifactIds`, `final=true`; every id is already durably registered, and the event is invalid while any node attempt, slot dispatch, Tool lease, or host target lease for this run remains open, or any never-started node has an input delivery on a taken path |

Every allowlisted schema-2 free-form metadata string outside a typed
payload/configuration/artifact projection is either a closed identifier/template
or is explicitly bounded and redaction-safe. The writer rejects raw adapter
text, captures, secrets, and unclassified runtime/evidence strings from those
metadata fields before append; typed projections still follow their declared
Redact=false or ADR-0005 semantics. Public projection does not perform late
best-effort scrubbing.

Bare `promptRef`, `textRef`, `reportRef`, `summaryRef`, `outputRefs`,
`artifactRef`, and diff refs may occur only in schema-1 ledgers or writer-private
adapter ingress. They are invalid in schema-2 events and every API projection.
Writer-private ingress validates and registers sanitized file evidence before a
schema-2 event records its stable `artifactId`; schema-1 read projection consumes
compatibility refs privately and never returns them directly. The projector
cannot mint artifact identity from an embedded ref.

If `redact` is true, a persistent compatibility target is valid only after it is
sanitized or replaced inside an authorized root. A sanitized replacement is
evidence-only unless it is the exact policy-permitted value. A ref to an
unsanitized prompt, reply, report, or artifact content is invalid even when the
ledger omits inline text. The run-private pending-redaction registry is the only
temporary exception: its cleanup locator is written and fsynced before raw bytes
reach the target, is never an event field/output/graph input, and lives with every
pending raw target under host-private writer authority outside all generic Files
roots.

Schema-2 run, event, SSE, and UI projections resolve every recorded artifact id
through its latest `ArtifactProjection`: either an available `SafeArtifactRef` or
metadata-only unavailable/redacted/expired state. No bare ref is sent to Files or
File Peek.

Structured payload fields use these shapes:

- `authoredConfigManifest`: private stable `(sourceKind,nodeId)`-ordered array of
  exact `{classification="authored_config", sourceKind, nodeId, encoding,
  mediaType, sha256}` entries embedded in and hashed with the normalized graph
  snapshot. The closed source kinds are `mission_objective`, `formation_brief`,
  and `gate_criterion`; each combination uses its fixed encoding/media contract
  and hashes the exact corresponding snapshot value. It contains no prompt,
  runtime value, or public path.
- `authoredConfigTextProjection`: exact closed
  `{classification="authored_config", sourceKind, encoding, mediaType, sha256,
  text}` over one canonical Gate criterion. It requires
  `sourceKind=gate_criterion`, `encoding=gate-criterion-utf8-v1`, and
  `mediaType=text/plain`; rejects a BOM; normalizes CRLF/CR to LF; and otherwise
  preserves bounded UTF-8 bytes exactly. Other source kinds, encodings, media
  types, unknown keys, or mismatch with the same Gate/node's private manifest
  entry are invalid. A fixed-system human prompt is instead
  the exact closed
  `{classification="fixed_system", sourceKind="human_prompt",
  templateId="gate-human-verdict-v1"}` and never contains or interpolates run
  data. Its `choiceProjections` is exactly the closed object
  `{pass={classification="fixed_system",sourceKind="human_choice",
  templateId="gate-human-pass-v1"},
  fail={classification="fixed_system",sourceKind="human_choice",
  templateId="gate-human-fail-v1"}}`; the key binds each label to its verdict,
  and no board-authored choice label exists in schema 2.
  Fixed-system template ids are immutable registry identities: changing rendered
  text requires a new versioned id, and an unknown id is rejected before append
  or projection.
- `rootInputProjection`: exact closed `{classification="authored_config",
  sourceKind, encoding, mediaType, sha256, text}`. A Mission requires
  `sourceKind=mission_objective`, `encoding=mission-objective-utf8-v1`, and
  `mediaType=text/markdown`; an isolated Formation requires
  `sourceKind=formation_brief`, `encoding=formation-brief-jcs-v1`, and
  `mediaType=application/json`. `text` is the canonical UTF-8 value whose exact
  bytes produce `sha256`; for a Formation it is the canonical JSON text. Unknown
  keys and every other field combination are invalid. This projection is not a
  `PayloadProjection` and contains no runtime/external value. The only permitted
  durable payload copy of these bytes is the classified root-derived variant
  below, exact-matched back to this projection. Both must also exact-match the
  selected root's private manifest entry.
- `inputRefs`: immutable durable discriminated array. Both variants require
  stable `inputId` and `payloadProjection`. `sourceKind="edge"` additionally
  requires `{runId, originEdgeId, deliveryEdgeId, sourceNodeId, sourcePortId,
  sourceOutputSeq, sourceAttempt, toNodeId, toPortId}`.
  `sourceKind="run_seed"` instead requires `{runId, seedId,
  seedEncoding="formation-brief-jcs-v1", seedMediaType="application/json",
  seedSha256, toNodeId, toPortId}` and the exact classified
  `rootDerivedPayloadProjection` matching `run_started.rootInputProjection`; it
  never invents an edge or producer node. An edge delivery originating at the
  Mission fixed `out` likewise carries that classified projection unchanged.
  `gate_verdict.evaluatedInputRef` retains the exact evaluated ref.
  A downstream pass ref preserves its origin/source fields and exact durable
  `payloadProjection`, then records the pass edge and destination only as the new
  delivery traversal. A fail ref instead sets source node/port to Gate/`fail`,
  source output seq to the `gate_verdict` seq, and source attempt to
  `gateAttempt`; its feedback payload retains only the identity pointer below,
  never the evaluated ref's `payloadProjection`.
- `payload`: a discriminated union: `work` requires an allowlisted `mediaType`
  and exactly one of bounded `text` or safe `artifact`; `gate_feedback` requires
  `feedback`; `unavailable` and `error` each require stable `code`, safe
  `message`, and engine-derived `retryable`. Work media are limited initially to
  bounded UTF-8 `text/plain`, `text/markdown`, and `application/json`. The one
  selected representation uses that declared media type; a mixed text+artifact
  payload is invalid rather than an ambiguous two-component value.
- `payloadProjection`: exactly one of generic available, root-derived available,
  or redacted. Generic available is the closed
  `{availability="available", exact=true, payload}` shape and forbids
  classification/root-derived keys. `rootDerivedPayloadProjection` is the closed
  `{availability="available", exact=true, classification="authored_config",
  sourceKind, encoding, mediaType, sha256, payload}` shape. Its `sourceKind`,
  encoding, media type, hash, and `payload={kind="work",mediaType,text}` must
  byte-for-byte match the same run's `rootInputProjection` and private manifest
  entry; Mission and isolated Formation variants use their respective closed
  combinations above. It is
  allowed only for Mission fixed `out`, its unchanged edge/Gate-pass deliveries,
  and the isolated Formation `run_seed`. Every unclassified or mismatched exact
  copy of root-authored bytes and every other producer of this variant is
  invalid. Redacted is
  `{availability="redacted", exact=false,
  code="redacted_payload_unavailable", payloadSha256}`. The first form is
  durably routable only when its payload kind is compatible `work` or
  `gate_feedback`; the root-derived form is durably routable only under its
  restricted provenance; exact `unavailable`/`error` outcomes remain
  non-delivered. The redacted form is evidence, never an input.
- `executionInputRef`: ephemeral `{durableInputRef, authoritativePayload}` used
  only during fresh execution; it is never written to the ledger or API.
- `executionOutput`: ephemeral `{durableOutput, authoritativePayload}` under the
  same port/attempt identity when a redacted durable output projection cannot
  hold the authoritative value.
- `artifact`: `{artifactId, rootId, ref, mediaType, sizeBytes, sha256}` where
  `ref` is relative to the identified authorized root and resolves to a regular,
  non-symlink-escaped, size-bounded file. The run-scoped resolver uses a
  root-relative no-follow open (openat-style), validates regular-file identity,
  size, media, and SHA-256 on that opened handle, and supplies only those same
  verified bytes/handle to Files/File Peek; it never reopens the path after
  validation. This descriptor proves those
  checks passed when attached; its shape alone does not prove current mutable-file
  availability.
- `artifactProjection`: exactly `{artifactId, availability="available", name,
  artifact}` or `{artifactId, availability, name, errorCode}` where metadata-only
  `availability` is `unavailable`, `redacted`, or `expired` and no readable ref
  is present. For the available form, both `artifactId` values must match.
- `artifactSource`: exactly one of
  `{kind="slot", dispatchId, nodeId, slotId}`,
  `{kind="tool", toolLeaseId, nodeId}`,
  `{kind="gate", gateId, gateAttempt}`, or
  `{kind="system", sourceId}`. `artifact_attached` uses the non-Tool forms;
  `tool_result` derives the Tool form from its own lease/node fields.
- `artifactRegistrations`: the Tool result's possibly empty array of new
  `artifactProjection` values. Every id is unique and unregistered before that
  event; all are sourced to the result's exact Tool lease/node.
- `evaluatedInputPointer`: exactly `{inputId, gateInputSeq}` where
  `gateInputSeq` names the matching `gate_evaluating` event whose `inputRef` has
  that `inputId` for the same Gate attempt. It is identity/provenance lookup
  only and never embeds a `RunInputRef`, `payloadProjection`, payload, artifact,
  text, or execution authority.
- `feedback`: `{feedbackId, revisionCycleId?, gateId, verdict="fail",
  evaluatedInput, reason, evidence, gateSeq, gateAttempt}`, where
  `evaluatedInput` is the identity-only pointer above. It is redaction-safe,
  bounded, and created exactly once per failed gate-evaluation sequence;
  fail-edge fan-out references the same object.
- `evidence`: array whose items are exactly one of
  `{kind="artifact", artifactId}`, `{kind="ledger", seq}`, or bounded
  redaction-safe `{kind="text", text}`. Evidence never embeds a readable ref;
  API projection resolves the id through its latest authorized state.
- `displayEvidence`: a bounded inspection-only array whose items are exactly
  `{kind="text", text}` with redaction-safe text or
  `{kind="artifact", artifactProjection}`. Artifact items obey the same durable
  registration and latest-projection rules, including same-event Tool
  registration. Display evidence is never a port output, execution input, or
  routing authority.
- `judgeResult`: exactly `{verdict, reason, evidence}` where `verdict` is `pass`
  or `fail`; it is bounded Gate metadata, not a workflow payload.
- `gateKindResult`: exactly `{kind, verdict, reason, evidence,
  evaluatedInputRef, resultSha256, relatedSeqs}`. For `kind=code`,
  `relatedSeqs=[]` and the event also exact-matches its Gate binding/input/profile/
  parameter/policy hashes; for `kind=formation`, it is the unique ascending chain-order
  array of that kind's accepted `judge_result` sequences and includes the exit
  result. It never names another Gate attempt.
- `perKind`: exact object keyed by every declared Gate kind, independent of the
  board's stored order, with values `pass`, `fail`, or `not_run`.
- `completedKindResultSeqs`: exact object for the declared kinds preceding
  `human` in canonical order. It includes each completed/pass kind exactly once:
  `code`/`formation` values point to their `gate_kind_result` sequences. An
  undeclared prior kind is absent; a fail cannot produce a human request.
- `kindResultSeqs`: exact object keyed by every declared Gate kind. `code` and
  `formation` values point to their `gate_kind_result`; `human` points to its
  `human_verdict_recorded`; a `not_run` kind has JSON null. The writer rejects a
  sequence from another Gate/attempt/kind or a map inconsistent with `perKind`.
- `judgeContext`: exactly `{gateId, gateAttempt, criterion, kinds,
  evaluatedInput, durableEvaluatedInput, priorResults}` where `evaluatedInput`
  is the ephemeral execution ref and `durableEvaluatedInput` is its ledger-safe
  `RunInputRef`; it is bounded execution control for one linear judge chain and
  never a workflow payload. `judge-context-jcs-v1` applies RFC 8785 to exactly
  those fields with no unknown keys/trailing newline, fixed Gate-kind order,
  judge-chain `priorResults` order, and preserved nested evidence order;
  `judgeContextSha256` hashes those exact UTF-8 bytes.
- `decision-result-jcs-v1`: RFC 8785 canonical UTF-8 JSON over exactly
  `{verdict,reason,evidence}` with no unknown keys/trailing newline. Evidence
  order is preserved and every member uses its closed `EvidenceRef` shape.
  `resultSha256` for both code-Gate and judge results hashes those exact bytes;
  encoding ids are frozen in run authority and replay cannot substitute another
  serializer.
- `capturedRange`: `{sessionTargetId, start, end, startedAt, endedAt}` where
  `start` and `end` are adapter-defined capture cursors, not executable text.
- `reportArtifactId`/`summaryArtifactId`: optional stable ids whose durable
  registration precedes the referencing event.
- `artifactIds`, `diffArtifactIds`, and `outputArtifactIds`: stable-order arrays
  of already-registered ids. Raw schema-2 events store ids only; run detail,
  event, SSE, CLI, and UI responses hydrate each through the latest
  `ArtifactProjection`, so an earlier readable descriptor cannot bypass a later
  unavailable/redacted/expired observation.
- `artifacts` is used only where an event schema expressly names an artifact
  projection or registration field. Each id must already have one durable
  registration or appear in the current Tool result's authoritative
  `artifactRegistrations`; an embedded projection is a non-authoritative cache
  and must equal the registered/observed state at its event sequence.
- `timing`: `{startedAt, finishedAt, durationMs}`.
- `deliveredEdges`: array of `{originEdgeId, deliveryEdgeId, toNodeId, toPortId,
  sourceNodeId, sourcePortId, sourceOutputSeq, sourceAttempt}`.
- `errorScope`: `run` omits graph ids; `node` requires `nodeId` plus either
  `attempt` for an opened attempt or `waitingSeq` for a pre-attempt readiness
  failure;
  `gate` requires `{nodeId, attempt, gateId, gateAttempt}`; `slot` requires
  `{nodeId, attempt, slotId, bindingId, sessionTargetId}` and includes
  `dispatchId` when one exists; `tool` requires
  `{nodeId, attempt, toolLeaseId}`. The writer rejects unrelated or invented ids. A code
  Gate evaluator boundary failure uses `boundary=evaluator,errorScope=gate`.
- `nodeAttemptSnapshot`: exactly `{nodeId, nodeKind, attempt, startSeq, phase,
  phaseSeq}` copied from an unmatched `node_started` and its latest durable
  phase. `phase` is `started`, `gate_evaluating`, or `waiting_human`;
  `phaseSeq` is the sequence that established that phase. A matching
  `gate_evaluating` requires the same open Gate attempt. Its unique outstanding
  `human_input_requested` changes that attempt to `waiting_human`; the matching
  `human_verdict_recorded` returns it to `gate_evaluating` without opening a new
  attempt.
- `openNodeAttempts`: stable `startSeq`-order array of `nodeAttemptSnapshot`
  values that exactly equals all attempts open immediately before
  `run_cancel_requested`. `node_started` opens an attempt. Matching
  `node_output` closes an ordinary Mission, Formation, or Tool attempt;
  `judge_result` or `judge_attempt_failed` closes a judge Formation attempt;
  `gate_verdict` closes a Gate attempt. A matching non-resumable Gate-scoped
  `run_blocked` closes that attempt only when it is still open; after an unwired
  FAIL verdict it instead overlays the already-closed Gate as blocked without
  changing or closing the attempt again. A final disposition is the only other
  closer.
- `nodeAttemptDispositions`: stable `startSeq`-order array preserving every
  `nodeAttemptSnapshot` field and adding `disposition`. `run_canceled` requires
  `canceled_non_authorizing` for exact coverage of the cancel-request snapshot.
  `run_failed` requires `failed_non_authorizing` for the one attempt, if any,
  resolved by `failureCause`, and `abandoned_non_authorizing` for every other
  attempt open immediately before failure. It rejects missing, duplicate,
  extra, or identity-changing entries; a disposed `waiting_human` request
  cannot later accept a decision.
- `failureCause`: exactly one discriminated value: `{kind=slot,
  dispatchId}`, `{kind=tool, toolLeaseId}`, `{kind=error, errorSeq}`, or
  `{kind=none}`. A slot/Tool cause must identify the sole failed matching
  resource disposition and its open parent attempt. An error cause must name a
  prior `error` whose `errorScope` is `run`, `node`, or `gate`; an
  attempt-bearing node- or Gate-scoped error resolves its exact open attempt,
  while a pre-attempt node error or run-scoped error resolves none. A slot- or Tool-scoped error must instead select the matching
  `{kind=slot}` or `{kind=tool}` open resource. `none` also resolves none. That single
  discriminant is the deterministic cause precedence: every other open attempt
  is collateral. `run_failed.relatedSeq` is context/provenance only—ADR-0005
  uses it for the source-value event—and never selects a failed attempt.
- `openDispatches`: array of `{dispatchId, targetLeaseId, nodeId, attempt, slotId, agentId, bindingId,
  sessionTargetId, targetFingerprint, dispatchSeq}`.
- `openSlotDispatches`: stable `dispatchSeq`-order array with the exact
  `openDispatches` item shape that equals all unmatched slot dispatches
  immediately before `run_cancel_requested`.
- `slotDispatchDispositions`: stable `dispatchSeq`-order array preserving every
  open-dispatch field and adding `disposition`, `softInterrupt` (`sent`,
  `unavailable`, or `unsupported`), and `targetLeaseState`
  (`released_quiescent`, `terminal_hold`, or `quarantined`).
  `released_quiescent` requires the
  exact target to be proven quiescent and its occupying host-registry record
  durably replaced by an exact non-occupying release receipt before finality.
  That receipt preserves `{targetKey, targetLeaseId, sessionTargetId, runId,
  dispatchId, targetFingerprint, releaseKind=final_quiescent, proof}` across a crash until the
  final event is fsynced. Its proof is a closed writer-validated union: either
  an exact dispatch/lease/fingerprint-bound cancel acknowledgement followed by
  certified harness-ready evidence and terminal capture boundary, or proof that
  the exact pane incarnation/fingerprint is gone. Sent Ctrl-C, silence, generic
  idle/prompt text, display name, bare PID, or an unknown proof kind is invalid.
  `terminal_hold` requires that exact record to be
  durably marked non-authorizing and busy before finality; later
  non-authorizing reconciliation may release it only after proving quiescence.
  `quarantined` requires exact membership in a durable fail-closed
  `TargetQuarantine` reconstructed from missing/conflicting private state. That
  target-key record contains a deduplicated candidate set in stable
  `(runId, dispatchId, targetLeaseId)` order; every candidate preserves its full
  dispatch identity and optional exact result sequence. Each unmatched candidate
  maps to its own final slot disposition. The quarantine remains busy until
  every candidate dispatch is proven quiescent and removed.
  `run_canceled` requires
  `disposition=canceled_non_authorizing` and exact coverage of the cancel
  request snapshot. `run_failed` requires
  `disposition=failed_non_authorizing` for the dispatch selected by
  `failureCause.kind=slot` and
  `abandoned_non_authorizing` for every other dispatch; when no slot caused the
  failure, all are abandoned. It exactly covers all dispatches still open
  immediately before failure. The writer rejects missing, duplicate,
  extra, or identity-changing entries; no disposition kills a tmux session.
  `softInterrupt=sent` is valid only when the frozen binding/target is proven to
  host that exact unresolved dispatch and attempt. Otherwise the engine records
  `unavailable`/`unsupported` and sends no keystrokes to possibly unrelated work.
  A sent interrupt is an outcome, never release proof; without a certified
  acknowledgement or pane-incarnation-gone proof the record remains held.
  A target lease whose exact `slot_result` is already fsynced is result-closed,
  not an open slot disposition; every execution-final event is rejected until
  its occupying registry record is durably replaced by the exact
  `releaseKind=result_committed` receipt naming the result sequence and carrying
  its certified turn-closure proof.
- `blockScope`: `run` omits both blocked ids; `node` requires
  `blockedNodeId` and omits `blockedGateId`; `gate` requires `blockedGateId` and
  includes `blockedNodeId` only when one exact upstream/judge node caused the
  block. Retryable producer blocks use node scope and exactly one `retryTarget`
  in this phase. Immutable limit exhaustion is terminal under the failure rule
  below rather than represented as a non-final block.
- `resumePolicy`: `retry_failed_producer` requires `resumeAllowed=true`,
  `openDispatches=[]`, exactly one whole-producer `retryTarget` in this phase,
  and `nextEpoch`; `reattach_only` requires `resumeAllowed=true`, a non-empty
  exact unmatched `openDispatches` set, `retryTargets=[]`, and `nextEpoch`;
  `new_run_required` requires `resumeAllowed=false`, `retryTargets=[]`, and no
  `nextEpoch`, with `openDispatches` either empty or the exact unmatched set
  remaining when that block is appended; its late authority is revoked above.
  A post-reattach `new_run_required` set may therefore be a strict subset of
  the preceding `reattach_only` set, but may not add or change an identity. No
  other combination is accepted.
- `toolLeaseSnapshot`: exactly `{toolLeaseId, nodeId, attempt, dispatchSeq,
  latestLaunch?}` where `latestLaunch`, when present, is exactly `{launchId,
  generation, processScopeId, deadlineAuthorityId, launchSeq}` copied from the latest public launch
  for that lease. A lease without `tool_process_launch` omits `latestLaunch`.
- `openToolLeases`: stable `dispatchSeq`-order array of `toolLeaseSnapshot`
  values that exactly equals the open Tool leases immediately before
  `run_cancel_requested`.
- `reconciledToolLeases`: stable `dispatchSeq`-order array preserving every
  field of the request's `openToolLeases` and adding exactly one `disposition`:
  `never_launched_cleaned` requires no `latestLaunch`, while
  `launch_fenced_cleaned` requires it. The writer rejects any missing,
  duplicate, extra, or changed lease/launch identity.
- `toolLeaseDispositions`: stable `dispatchSeq`-order array preserving every
  `toolLeaseSnapshot` field for the leases open immediately before `run_failed`
  and adding `disposition=failed_private_cleanup_owned` for the lease selected
  by `failureCause.kind=tool` or `abandoned_private_cleanup_owned` otherwise.
  When no Tool caused
  the failure, every entry is abandoned. The writer rejects incomplete or
  identity-changing finalization.
- `retryTargets`: stable node-order array of `{nodeId, attempt, outputPortIds,
  outcomeSeqs, deliveredEdges=[]}`. Each entry names one whole failed producer
  attempt and lists all of that attempt's unsuccessful declared outputs in
  stable port order; selective port replay is not implied. Schema 2 permits exactly one
  entry. It is the first unresolved retry failure under the deterministic
  selection rule below, not an arbitrary choice by a coordinator.

Artifact projection remains replay-deterministic. It uses the latest
`artifact_attached`, `tool_result.artifactRegistrations`, or
`artifact_observed` event for `artifactId` and never reads the filesystem. An
`artifact_observed(availability=available)` event supplies the
newly validated `artifact` descriptor, whose id must match the event's existing
`artifactId`. The first available registration or observation establishes
the descriptor; every later available observation for that id must match its
root, ref, media type, size, and SHA-256 exactly. Changed content or location
requires a new artifact id and registration. Other observation states omit
the descriptor and require a stable `errorCode`; an observation for an unknown
artifact id is invalid. Files/File Peek revalidates and renders through one
root-relative no-follow handle/byte buffer without a path reopen, and returns
unavailable immediately on a failed check; the reconciler appends
`artifact_observed` before that observation changes durable run projection.

Exactly one durable registration source establishes each artifact id, initial
projection, and source. Slot, Gate, and system artifacts use
`artifact_attached`, appended and fsynced before any later `slot_result`,
`node_output`, `judge_result`, `gate_verdict`, display evidence, diff, or
`PortPayload` reference. New Tool artifacts use the sole same-event exception:
`tool_result.artifactRegistrations`. Before accepting that event, the writer
validates every new id, source, descriptor, and all same-event references; one
append/fsync then atomically registers them and closes the exact Tool lease.
An open lease has no public registrations, so recovery can remove/rerun its root.
A duplicate registration across either source is a loud ledger error. Other
embedded projections are non-authoritative caches: at their event sequence they
must equal the latest registered/observed projection and cannot establish or
mutate identity. Later observations supersede projection without rewriting
earlier events.

The raw writer-only ledger is never a browser authority surface. Run detail,
bounded event, and SSE responses sanitize every historical artifact occurrence
through the latest authorized `ArtifactProjection`; evidence carries only
`artifactId`. After an unavailable, redacted, or expired observation, those APIs
and File Peek expose no older readable root/ref even when registration history
contained one. Pending raw-redaction roots are outside every Files-authorized
root and cannot be enumerated or fetched during their cleanup window.

`node_output.outputs[portId]` is always the durable projection map. The engine
fsyncs it before downstream dispatch. Fresh redacted execution may pair one entry
with an in-memory `executionOutput` carrying the exact authoritative value under
the same port/attempt identity. That is the same logical output contract, not a
second durable routing path. Recovery never routes a redacted projection or
substitutes its marker, hash, summary, or sanitized evidence.

`unavailable` and `error` are durable unsuccessful attempt outcomes. They do not
satisfy or route through an ordinary successful `work` input, and descendants on
that dependency path do not dispatch. A node attempt finalizes all declared
outputs before delivering any edge. If one output fails, the attempt records
`deliveredEdges=[]`; successful sibling ports from that attempt do not route.
Already in-flight and independent branches may finish and append evidence.
Each unsuccessful output's `retryable` is derived by the engine from a stable
code/profile policy, never from agent prose, and causes no automatic retry. An
unsuccessful producer attempt is retryable only when every unsuccessful declared
output on that attempt is retryable; one non-retryable sibling makes the whole
attempt non-retryable. Its candidate `outputPortIds`/`outcomeSeqs` include every
unsuccessful output in stable port order, and its ordering sequence is the
minimum of those outcome sequences. Successful sibling outputs remain
non-delivered and do not affect that aggregation. After in-flight work settles,
the engine derives `unresolvedOutputFailures`: the latest closed unsuccessful
attempt for each producer with `deliveredEdges=[]` and no later attempt for that
node. It sorts candidates by minimum `outcomeSeq`, then `nodeId`. If any
candidate is non-retryable, the first non-retryable candidate under that order
selects terminal `run_failed(code=declared_output_failed)`, with its minimum
outcome sequence as provenance-only `relatedSeq` and
`failureCause={kind=none}` because the producer attempt is already closed.
Other closed failures remain evidence and no arrival-time coordinator choice is
allowed. Otherwise, when retryable candidates remain, the engine appends
`run_blocked` for only the first candidate with `blockScope=node`, that producer id,
`resumePolicy=retry_failed_producer`, `openDispatches=[]`, and exactly one
whole-producer `retryTarget`. Other retryable candidates remain closed durable
outcomes: they are neither retried nor abandoned by that block, and success is
invalid while any candidate remains.

Explicit operator resume is the second bounded attempt trigger, separate from
in-graph `gate_feedback`. It is valid only when the latest state is retryable
`run_blocked`, every target proves `deliveredEdges=[]`, and every frozen
execution-authoritative input remains available. `run_resumed` opens the next
epoch, then target attempt N+1 reuses the failed attempt's immutable `inputRefs`
and unchanged `RunSlotBinding`/`RunToolBinding` with a new dispatch or Tool lease.
Completed siblings are not rerun and limits still apply. A prior delivery or
discarded redacted input rejects resume with the applicable terminal failure.
After the selected retry settles and the graph again becomes quiescent, the
engine recomputes the same ordered set. A successful later attempt removes its
node's older candidate; a retryable later failure replaces it using the newer
outcome sequence. If another candidate remains, a new `run_blocked` selects the
new first candidate and requires a separate explicit resume. General-purpose
error ports, selective replay, and multi-target resume are deferred.

Quiescence has one closed outcome-selection order; subsystems do not race to
append competing blocks:

1. An existing `run_cancel_requested` permits only its reconciliation/finality.
   Otherwise any valid execution-final condition is resolved before a non-final
   block, and after a final event no block may append.
2. Any unmatched slot dispatch selects the complete-set recovery/reattach path
   below before graph-semantic blockers. This preserves open authority. After
   exact results close/release all of them, the engine recomputes graph
   quiescence; an unresolved reattach subset remains the selected non-resumable
   block.
3. With no unmatched dispatch, an outstanding exact human request keeps the run
   `waiting_human`; no `run_blocked` hides it. After decision/cancel/terminal
   wall-clock outcome, quiescence is recomputed.
4. Non-resumable semantic candidates dominate retryable-output candidates.
   Their closed reason set is `unsatisfied_required_input`,
   `unwired_gate_fail`, `gate_evaluator_error`, and `invalid_judge_result`.
   Each candidate derives a causal sequence respectively from the latest
   waiting event, FAIL `gate_verdict`, scoped `error`, or
   `judge_attempt_failed`. Select exactly one by `(causalSeq, reasonRank,
   blockedGateId, blockedNodeId)`, with `reasonRank` in the order listed above.
   Other candidates remain inspectable durable evidence and create no second
   block. The selected block is non-resumable under its existing scope rule.
5. Only when none of the above exists may the ordered whole-producer retry rule
   select one `retry_failed_producer` block.

The writer validates the selected reason/scope/ids against this order. It
rejects a resumable or later-sorting block that would mask an unmatched lease,
human wait, non-resumable candidate, or earlier candidate.

An exhausted immutable per-run limit is terminal, not a non-final block. The
engine first appends `error` with code `max_dispatch_exhausted`,
`max_attempts_exhausted`, or `wall_clock_exhausted`; max-attempt uses a
pre-attempt node scope when an exact waiting node exists, while the run-wide
limits use run scope. It then appends
`run_failed(code=run_limit_exhausted,unrecoverable=true)` whose
`failureCause={kind=error,errorSeq}` names that error and whose exact
node-attempt, slot-dispatch, and Tool-lease dispositions revoke every open
authority. Slots receive only proven-target soft interrupts; Tool supervisors
retain private fencing/cleanup ownership. Late results/output/routing are
rejected. Continuing requires a new run with a new frozen limit snapshot.

## Pure Tool Lease And Replay

`tool_dispatch.toolLeaseId` is unique within a run and is the one-to-one durable
lease for one exact Tool node attempt. Before appending it, the host validates
every exact input and materializes its bytes into a sealed, content-addressed,
read-only input set under the writer-only run data root. No-follow source reads,
bounded copy, fsync, atomic rename, and final SHA-256 validation produce a
manifest over input id, media type, size, hash, and object identity. For
`Redact=true`, the already-fsynced private obligation owns that materialization
before any raw byte is persisted; it is never under a Files-authorized root.
The coordinator then appends/fsyncs the lease before starting the host profile.
It references the frozen `RunToolBinding`, records the input-manifest plus
canonical input/profile/parameter/policy/determinism-policy/execution-bundle hashes, and owns one
host-private run root.

Each spawn receives only that sealed set and an empty confined output root; it
never reads a mutable source artifact. Recovery rerun exact-verifies and reuses
the same manifest/objects. Missing, changed, or unsealable materialization fails
loud before spawn and never falls back to the source. The certified deterministic
sandbox exposes only those inputs and frozen bundle/parameters/policy; denies
network, secrets, undeclared environment/filesystem reads, and external writes;
normalizes locale/timezone; and freezes or denies clock/entropy. Profile
admission includes repeat vectors with expected output hashes.

Before every actual process spawn, the host supervisor reserves and fsyncs a
private per-generation scope record that identifies the whole process/descendant
scope plus an immutable `ToolLaunchDeadlineAuthority`. The latter binds the same
run/lease/launch/node/attempt/generation/scope identity, records one
`deadlineAuthorityId`, and derives `startedAt`, timeout-policy hash/duration, and
`effectiveDeadlineAt` exactly once before public launch or spawn. The writer
requires globally unique launch, scope, and deadline-authority ids and exact-matches
`{runId, toolLeaseId, launchId, nodeId, attempt, generation, processScopeId,
deadlineAuthorityId}` across the open lease, private records, and launch event. The first public generation is 1;
each later generation is exactly one greater than the prior launch. It
appends/fsyncs the launch with opaque `processScopeId`/`deadlineAuthorityId` and
`recordedBeforeSpawn=true`; only then may the process spawn inside that scope.
The id is never a reusable raw PID; the private record retains native supervisor
identity/start evidence for the exact tree. A private scope record without a
matching launch is one atomic/fsynced reservation with its deadline authority.
Recovery fences the scope and either reuses that exact pair or deletes both and
directory-fsyncs before reserving a replacement; it never retains one without the
other or spawns. Public generation does not advance. Every launch consumes dispatch and wall-clock limits. Its
`attempt` remains the enclosing logical node attempt; only a new node attempt
consumes `maxAttempts`.

Before any normal-completion, startup-recovery, or cancellation path first
terminates, seals, or waits on a recorded launch, writer-private authority fsyncs
one exact `ToolQuiescenceBoundary`. It contains a unique `boundaryId`, matching
`boundaryKind` (`normal`, `recovery`, or `cancel`), the exact
`deadlineAuthorityId`, exact run/lease/launch/node/
attempt/generation/scope identity, and a matching cause: private supervisor exit
id, durable recovery id, or `cancelRequestSeq`. Its `startedAt`, timeout-policy
hash/duration, and `effectiveDeadlineAt` must byte-match the already-fsynced
launch deadline authority. An unresolved boundary for that exact launch is reused after
every crash or restart, including when recovery continues work begun in another
mode; neither time is recomputed from restart time. A crash after launch/spawn but
before boundary creation reconstructs the boundary only from that same authority,
never from callback order, map order, or restart time.

At every such decision boundary, the coordinator freezes the complete set of
launched Tool leases whose quiescence proof missed its persisted
`effectiveDeadlineAt`. Candidates sort by
`(effectiveDeadlineAt, dispatchSeq, toolLeaseId)`. The first candidate alone
selects `run_failed(code=tool_process_not_quiescent).failureCause` and receives
`failed_private_cleanup_owned`; every other candidate receives
`abandoned_private_cleanup_owned`. Callback, process-exit arrival, goroutine,
and map iteration order are never cause authority.

The Tool writes staged output under the lease root. After the process exits, the
supervisor must seal the scope against new members and prove the entire recorded
scope quiescent before the coordinator
fsyncs file bytes, atomically promotes final names, and fsyncs the parent
directory. A failed deadline enters the uniform frozen-candidate selector and
appends one terminal `run_failed(code=tool_process_not_quiescent)` with no
promotion or result. The
supervisor retains private cleanup ownership and may keep terminating, sealing,
and proving the scope after finality. Only after later quiescence may it remove
or quarantine a non-redacted root; for a redacted root it must sanitize/remove
raw targets and fsync that cleanup before deleting the redaction obligation. It
then deletes the scope record, but never promotes output, appends `tool_result`,
reruns the Tool, or appends another execution event. In the normal nonterminal
path, only the initial quiescence proof allows the coordinator to construct and
validate one
`tool_result`, keyed to the latest launch id/generation, whose
`artifactRegistrations` contains every new Tool artifact sourced to that lease.
Appending and fsyncing that single event atomically registers those ids and
closes the lease; it never records a result before final output is durable. The
supervisor then deletes the quiescent private scope record and fsyncs its
directory; `run_succeeded` requires no remaining Tool scope record.
`tool_result.outputs` contains the full
durable canonical port-projection map needed to append/project `node_output`
idempotently after a crash. Each entry is exactly an available, exact
`PayloadProjection` or hash-only redacted `PayloadProjection`; recovery routes
only `availability=available, exact=true` payloads.

For `Redact=true`, the run-private pending-redaction registry writes and fsyncs
an obligation owning the exact root in `pending(generation=1)` before public
`tool_dispatch`. The writer rejects that dispatch unless the matching private
obligation is already durable; the event stores only `redactionObligationId`,
never the cleanup locator. It also rejects a redacted `tool_process_launch`
unless the obligation is pending for that exact launch generation. Process
execution starts only after those durable writes. If a crash leaves an obligation without
a matching dispatch, registry recovery cleans its exact root, deletes the entry,
fsyncs the directory, and never executes the Tool. Finalization
sanitizes/removes raw targets, then fsyncs the
private obligation from `pending(generation=N)` to
`cleaned(generation=N)` while retaining its locator. The writer accepts
`tool_result` only when that cleaned generation matches the latest launch, so
every public registration is policy-safe. After the result is fsynced, it deletes the private
entry and fsyncs the registry directory; `run_succeeded` requires no remaining
entry. Sanitized non-exact text, refs, and
summaries live only in separate bounded display or artifact evidence fields such
as `tool_result.artifacts`; they never occupy `tool_result.outputs`, become an
`ExecutionInputRef`, or authorize routing. Fresh ephemeral routing remains
allowed by ADR-0005 under an `ExecutionOutput`. If recovery needs discarded
authoritative input or output,
append terminal `run_failed(code=redacted_input_unavailable)`; hashes and Tool leases are never
substitute inputs or rerun authority.

Replay never executes a lease with a matching `tool_result`. A valid matching
result also supplies the only Tool artifact registrations, so a lease is either
fully committed or has no public artifact ids. A matching result with leftover
current or prior sealed/quiescent scope records or a cleaned-redaction record
removes those records without rerunning.

For an open lease with no `tool_process_launch`, recovery idempotently cleans its
root, fences and deletes any orphan private scope record without spawning, then
may reserve and fsync generation 1 after the normal hash, input, and limit
checks. A redacted lease retains and reuses its `pending(generation=1)`
obligation. The absence of a scope is not an error in this
ledger-before-spawn window because no process could have started.

Once a launch is recorded, recovery first requires its matching private scope
and reuses any unresolved quiescence boundary for that launch, creating and
fsyncing a recovery boundary only when none exists. It then asks the supervisor
to terminate the scope, seal it against new members, and prove every process and
descendant quiescent by the persisted deadline. It does not
remove, promote, or reuse any root and does not append a new launch until that
proof exists. If a recorded launch's scope is missing or ambiguous, or remains
live at its deadline, include that lease in the uniform startup-recovery
candidate set and append the one selected terminal
`run_failed(code=tool_process_not_quiescent)`; perform no cleanup or rerun for
any failed candidate;
the supervisor retains the post-final cleanup ownership defined above. After quiescence, ordinary recovery
removes the entire recorded root, including staged files and
promoted-but-uncommitted final names; redacted recovery uses the retained private
obligation for generation N to clean the exact targets and, if still pending,
fsyncs it to `cleaned(generation=N)`. The generation-N scope record remains as a
durable sealed/quiescent tombstone. A redacted rerun additionally fsyncs
`cleaned(generation=N)` to `pending(generation=N+1)`, resetting cleanup state
while retaining the locator, before it records or spawns generation N+1. A
crash in that prelaunch window is safe: `pending(latestGeneration+1)` with no
matching public launch is a prepared generation. Recovery idempotently cleans
the root, fences any orphan scope for that generation without spawning, retains
the pending obligation, and validates before reserving or reusing that same next
generation.
Recovery may rerun the pure profile under that newly fsynced launch generation only when
input-manifest, input, profile, parameter, effective-policy,
determinism-policy, and execution-bundle hashes equal
the recorded lease and every execution-authoritative input is still available.
The prior sealed/quiescent tombstone remains until either the next launch or a
terminal non-rerun event is fsynced; only then is it deleted and its parent
directory fsynced. Thus a crash on either side of the decision always leaves an
authoritative scope for the latest recorded launch. Any mismatch or discarded
redacted input appends the corresponding terminal error before private records
are retired. Recovery never re-resolves the board's version constraint or
substitutes a newer profile.

Cancellation starts with an appended and fsynced `run_cancel_requested`. That
intent snapshots every open node attempt, slot dispatch, and Tool lease, stops new dispatch, and forbids ordinary
replay/rerun even if the coordinator crashes before cancellation completes. The
writer also rejects new `tool_process_launch`, `tool_result`, `node_output`, edge
routing, or other execution authority except cancellation reconciliation and its
final event.
Only the first accepted request appends this event; a repeated abort is
idempotent against the same canceling/final state and cannot replace any
snapshot. `run_canceled.cancelRequestSeq` names that unique event. Every
snapshotted attempt, including a Gate waiting for human input or evaluating
without an open slot/Tool, receives a canceled/non-authorizing disposition; a
later human decision is rejected.
Active slot work is soft-interrupted without killing tmux sessions only when the
frozen target is proven to host that exact unresolved dispatch/attempt; otherwise
no keystroke is sent. Each dispatch receives a non-authorizing cancellation
disposition with the interrupt outcome. A
never-launched Tool lease is cleaned without execution. For each recorded Tool
launch, the supervisor terminates and seals the matching scope, proves every
descendant quiescent by the frozen deadline, and the coordinator cleans the root
without promotion or `tool_result`. For a redacted lease it also
sanitizes/removes raw targets and fsyncs the matching obligation to `cleaned`.
Only then may `run_canceled` append with exact attempt and slot dispositions and
an exact, discriminated Tool entry covering all three request snapshots;
quiescent private
records and cleaned obligations may be deleted and directory-fsynced after that
final event. If one or more launched Tool scopes miss the persisted cancellation
deadline, the uniform complete-set selector above chooses the sole cause by
`(effectiveDeadlineAt, dispatchSeq, toolLeaseId)` and all candidate supervisors
retain private cleanup ownership. The post-final cleanup rule applies to every
candidate.

Every execution-final event revokes authority for every still-open node attempt,
slot dispatch, and Tool lease. Before accepting one, the writer enumerates every
occupying host target-registry record and non-occupying release receipt for the
run. Each result-closed dispatch—one whose exact `slot_result` is already
fsynced—must have its exact durable `result_committed` receipt with certified
turn-closure proof and cannot be
omitted or represented at finality as a terminal hold, quarantine, or open slot
disposition. Missing/conflicting private state may create the temporary
fail-closed quarantine below, but it must be proven quiescent and replaced by
that receipt before finality. Every unmatched dispatch must correspond one-to-
one to an exact open-dispatch disposition and either its durable
`final_quiescent` receipt, its still-occupying terminal hold, or its exact
candidate in a still-occupying quarantine. The
writer rejects finality while any dispatch, occupying record, or receipt is
unaccounted for. `run_succeeded` is invalid while any public authority or
occupying registry record remains; exact release receipts are permitted only as
crash proof and may be removed and directory-fsynced after the final event.
`run_canceled` carries the complete reconciled snapshots above and projects
every open attempt, including `waiting_human` Gates, canceled.
`run_failed.failureCause` is the sole cause selector: a matching slot or Tool
disposition takes its parent attempt, an exact node/Gate-scoped error takes its
named attempt, and a run-scoped error or `none` takes no attempt. `relatedSeq`
never participates. The resolved attempt is failed and every other open attempt
is abandoned, so no final run can still project evaluating, waiting, or active.
Final projection also covers selected-root nodes that never opened an attempt.
For each node in the frozen run root, the latest attempt's normal completion or
final disposition wins. If no attempt ever opened, `run_canceled` projects the
node `canceled` and `run_failed` projects it `abandoned`. A valid
`run_succeeded` projects such a node `not_run` only when the ledger proves it was
unreached (no input delivery for an activation) or lies solely on an untaken
branch. Success is invalid if a never-started node has a delivered input on a
taken path or otherwise remains partially activated. Prior `node_waiting`
readiness counts remain inspection evidence, never an active post-final status.
`run_failed.slotDispatchDispositions` marks every open
causative dispatch failed/non-authorizing and every collateral dispatch
abandoned/non-authorizing. Projection applies that state to each slot and marks
a parent attempt failed when it owns a causative slot, otherwise abandoned. The
engine makes a best-effort soft interrupt without killing the tmux session.
Each slot disposition also proves the target registry record was durably
released after quiescence, durably converted to a non-authorizing terminal hold,
or durably quarantined after missing/conflicting private identity. Holds and
quarantines keep the target busy.
`run_failed.toolLeaseDispositions` exactly covers all leases still
open at failure: the causative lease is failed and every other lease is
abandoned, all non-authorizing and private-cleanup-owned. Their supervisors
terminate, seal, prove, and clean under the post-final rule; no result, output,
routing, replay, or rerun can follow.

## Dispatcher Lease And Idempotency

Tmux prompt/capture is serialized by a host-wide exclusive target registry keyed
by private canonical `targetKey`; the one-to-one opaque `sessionTargetId` is its
API handle, not an independently minted per-run key. Run ledgers remain
independent but do not own a pane concurrently. Before `slot_dispatch`, the engine allocates `dispatchId`
and unique `targetLeaseId`, atomically acquires and fsyncs exactly one private
record `{targetKey, targetLeaseId, sessionTargetId, runId, dispatchId, bindingId, nodeId,
attempt, slotId, targetFingerprint}`. Under that acquired lease and immediately
before public dispatch/send, it exact-rechecks the frozen persona-card hash,
harness/foreground-process start identity, cwd/root, and pane incarnation. Any
drift appends a stale binding observation, releases the unsent orphan lease, and
sends nothing. It also materializes every artifact-backed ordinary or judge
Formation input through one authorized-root-relative no-follow open, validates
regular identity, media type, size, and SHA-256 on that handle, and reads the
prompt bytes from that same handle without reopening. It never gives a mutable
path as input authority. Non-redacted mismatch appends node-scoped
`error(code=formation_input_integrity_failed)` and terminal failure with that
error as `failureCause`; unavailable Redact=true bytes fail terminally with
`redacted_input_unavailable`. Both send nothing and append no `slot_dispatch`.
Only an exact match appends/fsyncs `slot_dispatch` with the same
fingerprint before sending. A
crash after private acquisition but before public dispatch is safe because no
prompt could have been sent; recovery releases that exact orphan. A release
never deletes its only proof before public finality. It atomically replaces the
occupying record with a durable, non-authorizing, non-occupying release receipt
under the same lease/dispatch identity. Receipts do not block a new acquisition
on the key, but remain until this run's final event is fsynced. Any public
dispatch with neither its required occupying record nor its exact release
receipt, or with conflicting private identity, enters fail-closed reconciliation. The arbiter uses the frozen binding's `targetKey` to
atomically create or promote a durable non-authorizing `TargetQuarantine`. It
deduplicates the expected identity/result and every conflicting occupant into
separate candidates sorted by `(runId, dispatchId, targetLeaseId)`. It records
`target_lease_missing_or_mismatched`, never
resends or reconstructs active authority, and rejects every acquisition on that
key until reconciliation proves all recorded candidate dispatches quiescent.
Each unmatched candidate may report `targetLeaseState=quarantined` in its own
final slot disposition. A result-closed candidate has no such disposition, so its
quarantine must be proven quiescent and durably replaced by its exact
`result_committed` release receipt with certified turn-closure proof before any execution-final event. If the
frozen canonical key itself is missing/corrupt, the arbiter creates a separate
durable host-wide target-dispatch quarantine before reporting the ledger
invalid. Its stable candidates preserve every available
`{targetKeyEvidence?, targetLeaseId, sessionTargetId, runId, dispatchId,
bindingId}` identity, and its safety latch denies all target acquisition until
operator repair plus non-authorizing reconciliation proves/removes every
candidate. No prompt may be sent while it is active.

Every dispatched prompt requires its completion sentinel to exact-match
`runId`, `dispatchId`, and `targetLeaseId`; run id alone is insufficient. A
sentinel is not sufficient while later bytes from the old turn remain possible.
The matching `slot_result` repeats those identities/fingerprint and is accepted
only when the sentinel is terminal in the bounded capture and a certified
harness adapter proves return to a closed/ready turn for that fingerprint. The
closed proof records terminal capture boundary and harness-ready evidence;
silence or generic prompt text is insufficient. After that result is
fsynced, the registry lease may be atomically replaced by a
`releaseKind=result_committed` receipt naming the exact result sequence and
carrying that same turn-closure proof; success
is forbidden until that transition is durable. If cancellation or failure is
requested between result fsync and receipt creation, recovery completes that
exact transition; if private identity is missing/conflicting, it first uses the
result-closed quarantine path above and then replaces the proven-quiescent
quarantine with the receipt. The result-closed dispatch is not open and receives
no final slot disposition. Cancellation/failure may release an unmatched exact
active record only after the old dispatch is proven quiescent. A correct-target
interrupt alone is not proof. The closed admissible proof is either an exact
dispatch/lease/fingerprint-bound cancel acknowledgement followed by certified
harness-ready evidence at a terminal capture boundary, or private proof that
the exact pane incarnation/fingerprint is gone. A replacement pane gets a new
key/fingerprint and is never interrupted or released as the old target. Before
finality, the engine either atomically replaces
that record with a `releaseKind=final_quiescent` receipt and reports
`targetLeaseState=released_quiescent`, or durably marks it as a
non-authorizing terminal hold and reports
`targetLeaseState=terminal_hold`; a missing/conflicting record instead reports
the already-durable `targetLeaseState=quarantined`. Holds and quarantines remain
busy until later non-authorizing reconciliation proves quiescence. A crash after
receipt creation reuses that receipt to finish the same final disposition and
never quarantines or re-interrupts the already released dispatch. No registry
state or receipt expires by
time, PID, display name, or process restart and never authorizes a prompt.

An idle deadline without an exact sentinel plus certified closed-turn proof appends
`error(code=dispatch_idle_timeout,errorScope=slot)` and leaves both the public
dispatch and target lease unmatched. It never appends `slot_result` and never
permits ordinary lease release. The dispatch enters the same bounded reattach
state machine below; only a later exact closed-turn result or an execution-final
slot disposition with a certified `final_quiescent` receipt, terminal hold, or quarantine can retire
its authority.

If another run loses the atomic acquisition race, no `slot_dispatch` or prompt
is produced. Before `run_started`, preflight rejects `session_target_busy`.
After start, the engine appends a node-scoped
`error(code=session_target_busy)` for the exact attempt and terminal
`run_failed` with that error as `failureCause`, empty slot dispositions for the
unsent work, and exact dispositions for every other open authority. A later run
may use the target only after the prior registry lease/hold/quarantine is safely
released.

`slot_dispatch.dispatchId` is the durable lease for one attempted delivery to
one slot. It is unique within a run and includes the run id, epoch, node id, slot
id, and attempt in its generated value; the payload also records node id and
attempt explicitly. The engine must append
`slot_dispatch` and fsync the ledger before calling the adapter boundary that can
send text to tmux. For tmux adapters, `nativeAck` is always `false`; a future
adapter may set a native acknowledgement token, but it never replaces
ledger-before-send ordering.

Replay handles open dispatch leases as follows:

1. Enumerate every unmatched `slot_dispatch` for the run in stable
   `dispatchSeq` order. A dispatch with a matching `slot_result` is complete,
   never redispatched, and follows the result-closed release rule above.
2. For each unmatched dispatch, require its recorded `targetLeaseId`,
   `bindingId`, and opaque `sessionTargetId` to exact-match the host registry
   and still resolve to the same live pane. The reconciler must prove that tuple
   identifies the same unresolved dispatch and attempt; it never falls back to
   `sessionStem`, display name, or another same-named session.
3. In one bounded automatic pass, reattach capture to every provable target in
   stable order and wait only for each matching run/dispatch/target-lease
   sentinel plus certified closed-turn proof. Only that exact captured closure
   appends/fsyncs `slot_result` and then releases that target lease. Another idle deadline appends the timeout error
   above and leaves that dispatch unmatched. No branch sends a prompt.
4. After that pass reaches graph/recovery quiescence, append a loud `error` for
   each session that is missing, dead, ambiguous, or still unprovable, then
   determine whether continuation needs a discarded execution-authoritative
   value.
5. For an ordinary recoverable boundary, freeze the exact set `U` of every
   dispatch still unmatched in stable `dispatchSeq` order and append one
   `run_blocked` with `openDispatches=U`, empty `retryTargets`,
   `resumeAllowed=true`, `resumePolicy=reattach_only`, and `nextEpoch`.
   `blockScope=node` with that node id is valid only when every member of `U`
   belongs to the same node; otherwise it uses `blockScope=run` and omits both
   blocked ids. While that block is current, the writer rejects late
   `slot_result`, node output, and routing for `U`; only its exact reattach
   resume reopens result acceptance. One explicit operator resume appends
   `run_resumed(resumeMode=reattach)` with exactly `U` and makes one more bounded
   capture-reattach pass against those exact targets. It appends no new
   `slot_dispatch` and sends no prompt. Exact results accepted during that epoch
   close and release their original leases. At the next quiescence, if none
   remain, ordinary graph execution continues. Otherwise freeze the exact
   remaining subset `U'` and append `run_blocked` with `openDispatches=U'`,
   empty `retryTargets`, `resumeAllowed=false`,
   `resumePolicy=new_run_required`, and no `nextEpoch`; derive node/run scope
   from `U'` by the same rule. The writer rejects added, changed, or already
   proven dispatch identities. That block revokes result, output, and routing
   authority only for `U'`; it accepts only abort/finality and non-authorizing
   inspection observations. The old run remains blocked until aborted. A
   separately started run is independent and neither closes nor supersedes any
   old lease; replacement-target rebinding and unresolved-lease redispatch are
   never same-run operations.
6. For `Redact=true`, if continuation requires a raw value that was
   intentionally not persisted, append `run_failed` with
   `code=redacted_input_unavailable`,
   `reason=redacted_input_unavailable`, `unrecoverable=true`, `relatedSeq` set to
   the exact source event whose authoritative value was required,
   `failureCause={kind=none}` because that provenance sequence is not attempt
   causation, `nodeAttemptDispositions` exactly covering every still-open node
   attempt (possibly empty),
   `slotDispatchDispositions` exactly covering every still-open slot dispatch
   (possibly empty),
   `toolLeaseDispositions` exactly covering every still-open Tool lease (possibly
   empty), and `final=true`. Do not append `run_blocked`, open another epoch, or dispatch a
   marker, hash, summary, pending cleanup target, capture, report, or artifact
   as input.
7. Never send the original prompt a second time in the same run. A new dispatch
   is allowed only for a new logical attempt after the prior attempt closed with
   a retryable failure, never to supersede an unmatched dispatch lease.

Automatic replay or process reconnect does not advance the epoch. It may append
`slot_result` for any proven completed dispatch. When recovery cannot be proven,
it appends errors followed by one bounded `reattach_only` block/resume path for
the exact stable set of every remaining unmatched dispatch, or the terminal
redacted-input failure above. Reattach resume advances the epoch but preserves
that set and sends nothing. Exact results may reduce it. A second unproven
reattach blocks non-resumably with only the remaining subset and requires
cancellation plus a new run; a final redacted-input failure rejects resume. The
non-resumable open-dispatch block rejects late slot results, node outputs, and
routing for that subset; an independent run has no effect on it.

Adapter output is data only. Captured text, sentinels, artifact paths, and
escalation reasons are recorded and parsed; they are never executed.

## Projection Mapping

Status is projected by replaying events in ascending `seq`. Missing or duplicate
sequence numbers make the ledger invalid and surface as `error` on read. A
projector must be deterministic and must not inspect live tmux state or current
board/layout/persona files after `run_started`; it uses only the ledger and the
snapshots named there. Runtime recovery is a separate reconciler that appends
events; projection changes only after those events exist.

Run projection:

- `run_started` initializes status `running`.
- `run_blocked` projects status `blocked` for its epoch and stops automatic
  dispatch. Explicit resume may append a higher-epoch event only when
  `resumeAllowed=true`, in which case `nextEpoch` is required;
  `resumePolicy=new_run_required` omits `nextEpoch`, remains blocked, and rejects
  resume. When such a block retains non-empty `openDispatches`, it also revokes
  those leases' result/output/routing authority; only abort/finality and
  non-authorizing inspection observations remain acceptable.
- `run_resumed` projects status `running` for the new epoch and records the
  blocked sequence being resumed.
- `human_input_requested` projects `waiting_human`; only its matching
  `human_verdict_recorded` returns that same epoch to Gate evaluation. Restart
  reuses completed kind results and the outstanding request without rerunning
  code or judge work.
- `run_cancel_requested` projects status `canceling`. It rejects new dispatch,
  launch, result commit, node output/routing, ordinary recovery/rerun, and resume;
  replay may only continue cancellation reconciliation toward `run_canceled` or
  terminal `run_failed`.
- `run_succeeded`, `run_failed`, and `run_canceled` project final statuses and
  reject further execution events. Later binding/artifact observations may update
  inspection health only; they cannot change outcome or authorize work.
  Private Tool fencing and cleanup may continue after a failed fence without
  ledger execution events, but cannot promote, commit, or rerun work.
  The final event also closes every open node attempt, slot dispatch, and Tool
  lease: failure projects exact failed/abandoned dispositions, cancellation uses
  its exact reconciled snapshots, and success is invalid if any remain open.
  This includes a Gate in `gate_evaluating` or `waiting_human`; its final
  disposition rejects any later human decision.
  Projection then closes every frozen selected-root node that never opened an
  attempt: canceled for `run_canceled`, abandoned for `run_failed`, and
  `not_run` for a valid success only when the ledger proves it unreached or on
  an untaken branch. A partially activated never-started node makes success
  invalid and, after graph quiescence, takes the non-resumable
  `unsatisfied_required_input` block above. Earlier `node_waiting` counts remain
  inspectable but never keep an active waiting status after finality/blocking.
- The private host target registry is the dispatch occupancy fence, not an
  additional run-status source. The writer accepts `run_succeeded` only after
  every occupying target record for the run is durably absent. Before any final
  event, each result-closed dispatch has an exact non-occupying release receipt
  and each unmatched dispatch exact-matches a final slot disposition with an
  already-durable release receipt, terminal hold, or quarantine. Receipts are
  deleted only after final-event fsync. Cancellation/failure slot projection exposes the recorded
  `targetLeaseState`; a later private release of a terminal hold or quarantine
  does not rewrite that historical final disposition.
- An unsuccessful declared output prevents `run_succeeded`. After in-flight and
  independent branches settle, retryable-only failures are selected one at a
  time by `(minimum outcomeSeq, nodeId)` and project through resumable
  `run_blocked`; unselected candidates remain durable closed failures. Any
  non-retryable declared-output failure uses the same stable selection and
  appends/projects terminal `run_failed(code=declared_output_failed)` with no
  closed attempt misrepresented as an open failure cause.
- Before finality, an open `slot_dispatch` without a result projects `running` with open
  dispatches from the ledger. If recovery cannot reattach or prove completion,
  the reconciler appends `error` and then either the bounded `reattach_only`
  block/resume path for every unmatched dispatch, ending with only the still
  unmatched subset in non-resumable `new_run_required`, or terminal
  `run_failed` with `code=redacted_input_unavailable` when a redacted run cannot
  recover required raw input. No path supersedes or redispatches an unmatched
  lease.

Node projection:

- `node_waiting` projects waiting readiness counts.
- `node_input_ignored` preserves late/duplicate/stale input evidence without
  changing the frozen attempt or dispatching another one.
- `node_started` projects `running` and the current `attempt`; a judge attempt
  uses its context hash/prior-result refs instead of workflow `inputRefs`.
- `slot_binding_observed` updates inspection health for the frozen binding and
  never changes its target identity.
- `slot_dispatch` marks a slot in flight.
- `slot_result` marks that slot complete, failed, or needs review.
- `tool_dispatch` marks one pure Tool lease in flight.
- `tool_process_launch` records one bounded, supervisor-fenced process generation
  before spawn; it does not close the lease.
- `tool_result` closes the
  exact lease and supplies the durable canonical output map from which a missing
  `node_output` can be appended/projected without rerun. Its validated
  `artifactRegistrations` atomically establish new lease-sourced artifacts.
- `run_failed` closes every open node attempt, slot dispatch, and Tool lease. Its
  typed cause resolves at most one attempt as failed; all collateral attempts
  project abandoned, including Gate evaluation or human-wait phases. A causative slot
  and parent attempt project failed; collateral slots/attempts project abandoned. The
  related Tool projects failed/private-cleanup-owned and other open Tools project
  abandoned/private-cleanup-owned. `run_canceled` projects every snapshotted
  attempt and open slot canceled and every Tool reconciled. Restart
  never accepts a late result or takes an open-lease rerun branch.
- After applying the latest attempt completion/disposition, a final event
  overlays every selected-root node that never started: canceled on
  `run_canceled`, abandoned on `run_failed`, or `not_run` on valid success when
  it was provably unreached/untaken. A delivered input to a never-started node
  makes `run_succeeded` invalid. Its prior readiness counts remain evidence only.
- `node_output` projects `done`, `needs-review`, `blocked`, or `failed`; only
  successful compatible work outputs deliver the listed output edges.
- `judge_result` completes its judge Formation attempt and supplies the only
  replay-authoritative result for the next judge; it does not project workflow
  output. The next dispatch still requires the exact evaluated input to remain
  live or durably exact.
- `judge_attempt_failed` completes its judge Formation attempt as failed and
  blocks the Gate; it does not project workflow output. Exactly one
  `judge_result` or `judge_attempt_failed` may complete a judge key. The required
  following non-resumable `run_blocked` projects the run itself as blocked.
- `artifact_attached` registers one new non-Tool artifact identity, source, and
  initial projection; later events may only mirror that registered projection.
- `artifact_observed` updates only an existing identified artifact's inspection
  availability; an available observation supplies its validated descriptor.
- `gate_evaluating`, `gate_verdict`, `gate_kind_result`,
  `human_input_requested`, and `human_verdict_recorded`
  project the canonical all-of Gate state for that attempt. A waiting request
  routes nothing; one aggregate verdict closes evaluation and provides the only
  route. A schema-1 `verification_verdict` remains legacy inspection evidence
  only and is never accepted into a schema-2 ledger.

## API Surface

The API is the shared UI/CLI contract. Board and layout write endpoints use
`If-Match` with the relevant ETag/revision. Persona-card edits use the shared
writer's stale-read conflict detection. Write conflicts return `409`.

| Route | Purpose |
|---|---|
| `GET /api/agents` | Persona roster left-joined with live Oracle sessions. |
| `GET /api/agents/{agentId}` | Deep persona inspection with pointers, not inlined sources. |
| `POST /api/agents` | Create a persona card through the shared writer. |
| `PATCH /api/agents/{agentId}` | Edit modeled card fields while preserving unknown fields. |
| `GET /api/formations/boards` | List boards by id, slug, title, and rev. |
| `GET /api/formations/boards/{boardIdOrSlug}` | Read board definition plus ETag. |
| `PATCH /api/formations/boards/{boardId}` | Field/id-addressed board mutations. |
| `GET /api/formations/boards/{boardId}/layout` | Read layout sidecar plus ETag. |
| `PATCH /api/formations/boards/{boardId}/layout` | Layout-only mutations by stable ids. |
| `POST /api/formations/runs` | Start from a Mission/bead root or isolated Formation root; returns `runId`. |
| `GET /api/formations/runs/{runId}` | Project current status plus hash-linked safe binding/artifact projections from private authority; no raw route or private path. |
| `GET /api/formations/runs/{runId}/events` | Bounded sanitized event projection; never raw ledger bytes/private paths, and every artifact id is hydrated through latest authorization. |
| `GET /api/formations/runs/{runId}/stream?since=<seq>` | Sanitized SSE projection with `Last-Event-ID`; revoked artifact refs and private authority never replay. |
| `POST /api/formations/runs/{runId}/abort` | Append/fsync cancellation intent, stop dispatch, soft-interrupt exact slots, replace target occupancy only with certified `final_quiescent` receipts or retain exact terminal holds/quarantines, and fence/clean Tool leases; finish with `run_canceled` or fail closed with `run_failed(code=tool_process_not_quiescent)`; never kills tmux sessions. |
| `POST /api/formations/runs/{runId}/resume` | Resume an exact `retry_failed_producer` block or make the one bounded `reattach_only` attempt; human Gate decisions use their verdict endpoint in the same epoch, and unmatched leases are never redispatched. |
| `POST /api/formations/runs/{runId}/gates/{gateId}/verdict` | Human verdict for one waiting gate instance. |

Run operations target `runId` once a run exists. Mission, Formation, or bead
selectors are allowed only when they resolve to exactly one active run;
otherwise the command or API call fails loud.

## Write Rules

These rules apply to persona-card, board, and layout writes unless a rule names
a narrower target.

1. Load, mutate by stable id, and serialize with deterministic key ordering.
2. Preserve unknown keys and comments unless editing that key.
3. Use temp file plus rename for atomic writes.
4. Create persona cards with no-overwrite semantics; duplicate ids fail loud.
5. Reject persona-card writes where the filename stem does not equal `card.id`.
6. Increment `rev` and update `updatedAt` on structural board writes.
7. Use optimistic revision/ETag conflict handling; never clobber silently.
   Persona-card edits must detect stale reads or concurrent file changes and
   fail loud rather than overwriting.
8. Refuse newer schema versions; up-migrate older versions only with content
   preservation tests.
9. Runs never write persona cards, board definitions, or layout definitions.

## Deferred From S0

These are real design topics but not required to unblock S1/S2:

- Implicit Oracle prefix-stripping for legacy sessions not declared in cards.
- Gate-level `--onfail`; S0 used fail wiring and kept `onFail` only on legacy
  inline verification. Schema-2 now rejects that shape pending `ctx-ug7.17`.
- General-purpose error-routing ports; the first mixed-workflow contract stops
  `unavailable` and `error` outcomes loudly.
- A second reusable formation-definition registry; embedded nodes and explicit
  copying remain the first contract.
- Interactive keystroke forwarding inside canvas terminal popups.
- Broad undo matrices for every S3/S4 mutation beyond the structural cases in
  `canvas.feature`.
