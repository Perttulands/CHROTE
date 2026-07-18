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
<chrote-data>/formations/workspaces/registry.lock
<chrote-data>/formations/workspaces/registry.private.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/workspace.bootstrap.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/workspace.private.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/owner.lock
<chrote-data>/formations/workspaces/<workspace-authority-id>/owner.private.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/admission-policies/<policy-rev>.json
<chrote-data>/formations/workspaces/<workspace-authority-id>/commands/
<chrote-data>/formations/workspaces/<workspace-authority-id>/runs/<run-id>/events.ndjson
<chrote-data>/formations/workspaces/<workspace-authority-id>/runs/<run-id>/graph.snapshot.toml
<chrote-data>/formations/workspaces/<workspace-authority-id>/runs/<run-id>/bindings.private.toml
<chrote-data>/formations/workspaces/<workspace-authority-id>/runs/<run-id>/refs/
<chrote-data>/formations/workspaces/<workspace-authority-id>/quarantine/
```

The shared formations package is the only serializer for persona cards, board,
and layout files. The UI, `archon`, and agents all call that serializer. The
fenced CHROTE server coordinator is the sole semantic writer for schema-2
runtime authority; shared file locks protect bytes but grant no execution
lease. Canonical run
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

ADR-0008 retires schema-1 inline `formation.verification`. It remains inspectable
compatibility state, but it is not safely normalizable because its verdict lacks
exact attempt/output identity and replay-safe block/revision finality. A board
that contains it fails validation, Mission start, isolated Formation start, and
resume with `legacy_inline_verification_requires_migration` before artifacts or
work. New definitions and `verification_verdict` appends are rejected;
historical records remain non-authorizing evidence. Migration means explicitly
creating a replacement Gate, wiring a named Formation output to its input and
naming that Gate as `replacementGateId` when explicitly removing the legacy
block. A historical run may still be canceled or failed without evaluating,
routing, resuming or dispatching the retired check.

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
verdict, or route. A code-Gate profile that needs an OS process is rejected.
Gate-owned process evaluation is retired.

Authored presence of any schema-1 Gate command field—`command`, `commandArgv`,
`commandShell`, or `commandCwd`, including explicitly empty values—is preserved
for source inspection and migration planning only. Presence comes from the
source field, not a reconstructed non-empty value. Whole-board validation emits
`legacy_script_gate_requires_fenced_migration`; legacy-string-only and cwd-only
definitions receive the same finding because the writer cannot silently drop or
interpret them. They may also be `gate_not_routable` when no human or
formation-judge route exists.

Selected-root preflight is narrower than whole-board validation. A Mission start
walks its complete possible executable subgraph, including both Gate branches
and judge chains of reachable Gates, and returns
`legacy_script_gate_requires_fenced_migration` before any snapshot, binding,
ledger, `run_started`, evaluator, or process mutation when that root contains a
legacy command Gate. Resume checks the same root in the frozen snapshot before
`run_resumed`. Unreachable command Gates remain validation errors but do not
block that Mission. An isolated Formation root contains only that Formation and
its canonical brief seed and never traverses downstream board edges, so a Gate
elsewhere does not block it.

Before Tool profiles exist, the only migration surface is this closed,
non-authorizing inspection projection:

```text
LegacyScriptGateMigrationInspection {
  schema: 1
  boardId: string
  boardRev: integer
  boardETag: string
  gateId: string
  sourceMode: legacy_string | argv | shell | cwd_only | conflict | empty_present
  sourceFields: [command | commandArgv | commandShell | commandCwd] // sorted field names
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

The projection never contains raw command values, resolved executable/cwd or
environment, generated Tool ids/ports, inferred parameters, or a suggested
profile. Board ETag/revision is a future compare-and-swap precondition, never
apply authority. API definition inspection may return this projection beside
the source board. `archon board inspect --json` remains the authorized source
view, and `archon board validate --json` returns the same typed migration details.
No migration mutation verb or endpoint exists before the Tool foundation and
runtime authority.

The accepted target adds a bounded Tool node. Non-executing Tool definitions,
registry descriptors, and board authoring belong to `ctx-ug7.8.1`; certified
host-private implementation packaging and runtime execution belong to
`ctx-ug7.8`. Board-authored commands are not Tool profiles.

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

After `ctx-ug7.8.1` provides non-executing Tool definitions and registry
descriptors, and `ctx-ug7.8` provides certified host-private implementations and
runtime execution, `ctx-ug7.30` provides pure code-Gate profiles and the future
explicit migration apply. That apply may insert one Tool before the existing
Gate. The caller must
select the Tool and pure-Gate profiles and provide modeled parameters and port
mapping; the writer atomically verifies board ETag/revision, profile identity,
media/downstream compatibility, and every affected edge. It preserves the Gate
id/title/criterion, judge relationships, pass/fail outgoing edges, and existing
layout; only the new Tool receives placement. The Gate evaluates and forwards
the exact Tool output, not the pre-Tool payload, so arbitrary legacy command
semantics are never inferred. Unprovable mapping leaves the source bytes
unchanged and fails loud.

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

`boardRev` lets the UI detect stale layout against a newer board. A missing entry
for a new element receives only the shared connection-aware, bounded free-space,
grid-snap placement heuristic. Existing coordinates and hand-routed lanes are
never moved by load, create, connect, validate, save, run, replay, or reconnect;
full-board movement occurs only through the explicit UI or Archon `arrange`
operation. Extra layout entries for deleted nodes or edges are ignored and
cleaned by the writer on the next layout save.

Selected run/node, inspector state, and terminal Peek open/focus/tile/hide state
and geometry are user-local dashboard presentation. They are not board or layout
structure, run-ledger truth, durable tmux target identity, or tmux lifecycle
authority. Each Peek resolves one exact slot dispatch's opaque
`sessionTargetId` plus `dispatchId`, `targetLeaseId`, binding, and frozen target
fingerprint; an attempt may expose several. Live run Peek is authorized only
while that exact dispatch owns the matching active/unmatched occupancy record,
the run is non-final, and cancel/failure reconciliation has not begun. A
terminal hold is non-authorizing run evidence and permits no run-bound input.
A quarantine is ambiguous and never authorizes live run Peek. Once occupancy is
replaced by a result/final release receipt, the target may be reused: the old run
shows captured historical evidence or explicit `pane_moved_on`, never the
current live bytes as if they belonged to that attempt. The UI may separately
offer **Open current session**, labeled outside run evidence. Peek must surface
unavailable/stale rather than look up another session by name or trust the
stable opaque handle alone.
While that exact active occupancy remains valid, Peek is a full
interactive user attach and may send literal input, including control
characters, to steer or interrupt that agent. Formation attachment ownership
begins only after the exact target-registry occupancy is fsynced. After the
matching dispatch is durable, only a CHROTE-issued capability exact-matching
run, dispatch, target lease, binding, and fingerprint may send Peek input.
Only the latest fsynced issuance is valid. A later issuance requires prior
clients/input drained and atomically invalidates every older token/generation;
stale attach/input reaching the target boundary is foreign.
Recovery suspends input until that capability exact-matches recovered occupancy
under the current fence. Shared Terminal attach paths reject an occupied target
unless they use that exact capability. The writer-private certified attachment
monitor registration is armed and durable before occupancy is fsynced and then
covers every attach/detach/target-selection, resize/reflow, history,
pane-lifecycle/topology/other mutation, and input-capable tmux command/control
route affecting the pane through release. A registered Peek
connection is authorized only when all inbound bytes traverse the steering
generation gate. A foreign event or lost monitor continuity durably records
`session_target_foreign_attachment`, `session_target_foreign_input`, or
`session_target_attachment_audit_unavailable`, revokes/drains Peek, and forbids
`slot_result` and ordinary release. Foreign input is never reconstructed as a
steering generation; only non-authorizing quiescence reconciliation may later
release the hold/quarantine without promoting output. For a foreign or
audit-lost latch, the sole release proof is exact pane-incarnation-gone; current
client absence or a newly ready prompt cannot reconstruct the missing history.
`session_target_foreign_input` and the `foreign_input` latch are the stable
closed labels for any unregistered pane mutation or input, including history or
pane lifecycle/topology changes.
The private registry
atomically replaces `state=active,interactionLatch=none` with
`state=latched,interactionLatch=foreign_attachment|foreign_input|audit_lost`
and the new journal-evidence hash before the rejected/observed effect can be
forgotten. Recovery rebuilds that latch from the durable chain; an `error` event
alone is never latch authority.

Before the first input bytes are forwarded, the coordinator fsyncs
`slot_steering_started` for the next monotonic generation without storing raw
keystrokes. Result closure is forbidden until the channel is drained/released
and `slot_steering_ended` closes that generation. A fresh certified turn-closure
proof binds the latest generation and cannot be produced solely by user-writable
pane bytes, an echoed sentinel, or generic prompt/ready text. Later accepted
input opens a newer generation and invalidates earlier proof before result or is
rejected after capability revocation. Safe inspection marks
`operatorInfluenced` and generation/open state without input bytes. Literal user
control input is steering; automatic workflow sends, retries, coordinator
interrupts, and lifecycle transitions remain coordinator-only.

Normal result closure, cancel/failure reconciliation, and finality serialize
under the target occupancy. They first stop capability issuance, drain input,
close an open generation with `reason=capability_revoked`, and fsync the unique
irreversible `slot_peek_capability_revoked` event. The final closure proof binds
that event, the latest closed generation, and a continuous attachment-audit
proof. Every execution-final event rejects any issued capability, open input
channel, open steering generation, or unmatched revocation state. Run-bound
control never survives finality; a held session can be opened later only as a
separately labeled non-run Terminal action after ordinary arbitration permits
it.

An active dispatch freezes the terminal grid recorded in its history baseline.
Peek tile movement and resize change only the browser viewport and send no tmux
resize/`SIGWINCH`; actual resize/reflow invalidates the baseline. Closing,
moving, focusing, or tiling Peek never creates, kills, or rebinds tmux state.

## Workspace Runtime Authority And Commands

Current main still constructs separate synchronous run engines in the API and
Archon and writes run files below the workspace. The following ADR-0007 shapes
are accepted schema-2 target state, not current-binary claims.

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
  authoritySchema: uint64 // 2 for this target; monotonic high-water
  workspaceAuthorityId: string
  rootIdentityEncoding: "workspace-root-identity-v1"
  workspaceRootIdentitySha256: string
  nextWriterFence: uint64
  nextAdmissionSeq: uint64
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
      state: "configured", maxActiveRuns: integer, maxQueuedRuns: integer }

WorkspaceOwnerLease {
  leaseSchema: 1
  recordRev: uint64
  workspaceAuthorityId: string
  ownerInstanceId: string
  writerFence: uint64
  acquiredAt: string
  renewedAt: string
  leaseUntil: string
}

RunCommandRecordBase {
  commandSchema: 1
  recordRev: uint64
  commandEncoding: "run-command-jcs-v1"
  commandId: string
  commandKind: "start" | "resume" | "cancel" | "verdict"
  commandPayload: object
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
  | { commandId: string, commandPayloadSha256: string,
      commandKind: "start" | "resume" | "cancel" | "verdict",
      outcomeWriterFence: uint64, state: "applied", runId: string,
      effectSeq: number,
      decisionAdmissionPolicyRef: WorkspaceAdmissionPolicyRef | null }
  | { commandId: string, commandPayloadSha256: string,
      commandKind: "start" | "resume" | "cancel" | "verdict",
      outcomeWriterFence: uint64, state: "rejected", rejectionCode: string,
      decisionAdmissionPolicyRef: WorkspaceAdmissionPolicyRef | null }
```

`registry.private.json` is a closed schema-1 object. Its entries have exactly the
fields above and sort by decoded unsigned numeric `device`, decoded unsigned
numeric `inode`, then valid UTF-8 `configuredPath` bytewise. They use the same
canonical path/device/inode rules as `workspace-root-identity-v1`. It is strict-validated
under `registry.lock` before authority-id selection/creation; unknown schema/key,
duplicate configured/opened identity, or conflicting mapping authorizes no
mutation. Registry generations use the atomic mutable-file rule below.

`workspace-bootstrap-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{bootstrapSchema,workspaceAuthorityId,rootIdentityEncoding,
workspaceRootIdentitySha256}` with no unknown keys or trailing newline. The
published bootstrap is immutable. The separate mutable workspace authority
record carries the current authority-schema high-water mark.
The schema-2 target requires that value to be exactly `2`; a future supported
authority schema changes the supported value without rewriting the immutable
bootstrap, while an older reader rejects that higher value before mutation.

`workspace-admission-policy-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly
one closed `WorkspaceAdmissionPolicy` variant with no unknown key or trailing
newline. Revision 1 has `priorPolicySha256=""`; each later revision is exactly
the previous revision plus one and names the previous generation's
64-lowercase-hex SHA-256. Its immutable file is
`admission-policies/<policyRev>.json`, using
unsigned decimal with no leading zero. The current `WorkspaceAuthority` ref
exact-matches that file; every referenced historical generation is retained and
hash-verifiable.
First registration creates/fsyncs disabled revision 1 before publishing the
initial WorkspaceAuthority ref or parent registry mapping.

`workspace-root-identity-v1` is RFC 8785 canonical UTF-8 JSON over exactly
`{configuredPath,resolvedPath,device,inode}` with no unknown keys or trailing
newline. Paths are absolute valid UTF-8 with NUL rejected, lexical dot segments
removed, `/` separators, and no trailing slash except root. `configuredPath`
retains the cleaned configured spelling; `resolvedPath`, device, and inode come
from the same race-safe opened directory identity after symlink resolution.
Device and inode are JSON strings matching `0|[1-9][0-9]*` and decode as
`uint64`; they are never JSON numbers. The object stays private and
`workspaceRootIdentitySha256` hashes those exact bytes.

The opaque `workspaceAuthorityId` binds one exact configured/opened root
identity and matches canonical uppercase ULID grammar
`^wsa_[0-7][0-9A-HJKMNP-TV-Z]{25}$`, validated before directory construction. First registration holds a
coordinator-local mutex plus the process-shared parent `registry.lock`
before selecting an authority-id directory. The private registry enforces one
mapping for both cleaned configured spelling and opened `(device,inode)`; aliases,
changed targets, or conflicts require explicit migration. The new
bootstrap/private directory is fsynced before its mapping and parent. Recovery
may complete one unique exact creation, but never chooses between conflicts.

Under a coordinator-local mutex plus private process-shared `owner.lock`, a
process strict-validates the immutable closed bootstrap and mutable current
workspace authority record plus its hash-matched immutable admission-policy
generation and complete contiguous prior-hash chain to revision 1 before
mutation. Missing generations, discontinuities, or cycles are invalid.
Unsupported, missing, or conflicting bootstrap,
workspace authority, or policy schema is strictly read-only: no fence, cleanup,
quarantine, valid-run projection, or tmux action. Matching schema numbers alone do not enable schema-2;
admission waits for the complete registered safe projector/coordinator and a
certified guarded rollback set.

A supported coordinator reserves the next `writerFence`, fsyncs the advanced
counter, then publishes its renewable lease. Counter gaps are allowed and reuse
is forbidden; elapsed lease time cannot bypass a still-held kernel lock. The
same lock stays held from current lease/fence validation
through every authority write+fsync and bounded non-idempotent send, Tool spawn,
interrupt, cleanup, or quarantine call; takeover cannot race between check and
effect. Lock order is parent registry (registration only), workspace authority,
then host target arbiter; reverse acquisition is invalid. ADR-0006's per-target
lease remains separately required.

Historical event fences are monotonic non-decreasing allocated owner epochs; an
older prefix remains valid after takeover. Fence regression, an unallocated
fence, or a new append/effect not using the current fence is invalid. Private
records preserve `originWriterFence`; a higher current owner reconciles through
a new `stateWriterFence`. Immutable authority files are never written in place at
their canonical paths. Under the governing lock, complete bytes go to a unique
same-directory staging file, the file is fsynced, the canonical name is installed
atomically with no-replace semantics, and the parent is fsynced. Exact existing
bytes/hash make retry idempotent; mismatch conflicts and is never replaced. A
pre-install crash leaves only a non-authorizing staging file; post-install
recovery sees only complete bytes and repeats the parent fsync. Only the governing
authority holder (registrar under `registry.lock` or current fenced owner under
`owner.lock`) may remove a validated unreferenced staging file. Mutable registry
generations publish under `registry.lock`;
workspace authority/counter, owner-lease, and command-record generations publish
under `owner.lock`. Immutable admission-policy generations publish under
`owner.lock` before the current workspace ref changes. Each mutable record's
`recordRev` starts at 1; every successful replacement is
exactly the last published revision plus one and rejects a stale, skipped, or
regressed predecessor. Each uses a
generation-checked same-directory temp, file fsync, atomic rename, and parent
fsync. Migration holds both locks in parent-registry then workspace order.
Torn/stale/conflicting published records authorize nothing, and a canonical
immutable path is never a partial-file recovery surface.

Every schema-2 JSON record/policy revision, writer-fence field, allocated
event/effect sequence, workspace-admission identity, and next counter is an integer in
`1..9007199254740991`. Allocation past that bound fails closed before mutation;
rounding, wrap, and reuse are invalid.
Closed reference fields that explicitly use absence sentinel `0` are the sole
exception: `priorIssuedSeq`, `capabilityIssuedSeq`,
`latestCapabilityIssuedSeq`, `originCancelRequestSeq`, and private-journal
`routeSeq`. They are JSON-safe integers in
`0..9007199254740991`; `0` never names or allocates an event.

`run-command-jcs-v1` is RFC 8785 canonical UTF-8 JSON over exactly one of these
closed objects with no unknown keys or trailing newline:

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
`reattach` or `retry-failed-producer`, mapping respectively from block policies
`reattach_only` and `retry_failed_producer`; verdict is `pass` or `fail`.
Revisions/sequences are JSON integers in `1..9007199254740991`. `limits` always
contains JSON-integer `maxDispatch`, `maxAttempts`, and `wallClockSeconds` in
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
`^cmd_[0-7][0-9A-HJKMNP-TV-Z]{25}$`, validated before constructing the exact
`<commandId>.json` path. `abort` and `stop` normalize to `kind=cancel` before
hashing. Unknown keys, wrong types, non-canonical enums, out-of-range values, and
invalid transport/schema input are never journalable.

The private journal stores and fsyncs the complete canonical `commandPayload`
plus hash before effect and recanonicalizes it on read. Its payload actor, kind,
workspace/run, reason, mode/verdict, and precondition are the sole command
authority; the record contains no duplicate actor. Lookup/create/update is
serialized in the workspace authority critical section with create-if-absent
semantics. A pending record starts with equal admitted/state fences and has no
outcome fields; takeover may advance only `stateWriterFence`. The same
`commands/<commandId>.json` record becomes the receipt: `applied` requires exactly
`runId`, `effectSeq`, immutable `outcomeWriterFence`, and
`decisionAdmissionPolicyRef`; `rejected` requires exactly `rejectionCode`, that
outcome fence, and the decision policy ref. The ref names the exact generation
used by every terminal start decision and is `null` for non-start commands.
`RunCommandReceipt` is the closed API projection of that terminal record, not a
second file. `stateWriterFence` names the fence that last published the command
record; `outcomeWriterFence` separately and immutably names the effect event's
origin fence or rejection decision fence. Fence F2 may repair an F1 effect with
`stateWriterFence=F2` and `outcomeWriterFence=F1`. The same id/hash returns its durable
receipt; the same id with another hash returns `command_id_conflict` without an
effect. A different start id is a distinct request and may create another run.
Each effect event exact-matches the journaled id/hash and every duplicated
command-derived field, including envelope actor, workspace/run, reason,
mode/verdict, and precondition. Resume consumes the
named blocked sequence, verdict consumes the named outstanding human-request
sequence, and only the first cancel command can establish the cancel snapshot;
stale or duplicate semantic commands return stable typed receipts without a new
event or side effect.
Recovery resumes a pending start from its stored payload. A pending
resume/cancel/verdict rechecks the exact named precondition and applies once only
when still valid, otherwise records one stable rejection. A matching durable
effect for any command kind repairs the applied receipt and is never reapplied;
the repair uses the current state fence while preserving the event's origin
fence as the outcome fence.

For start, the owner fsyncs the pending command, immutable private run authority,
and `run_started(seq=1)`; if immediate capacity is reserved it also fsyncs
`run_activated` before linking the applied receipt and returning `runId`. Graph
execution is asynchronous from that response.

`maxActiveRuns` is a JSON integer in `1..2147483647`; `maxQueuedRuns` is a JSON
integer in `0..2147483647`. In one admission critical section, the writer
strict-validates the immutable generation named by
`WorkspaceAuthority.admissionPolicyRef`. There is no implicit default: schema-2
starts with closed `state=disabled` revision 1. Disabled rejects new starts as
`admission_disabled` and pauses queued activation without canceling active or
queued work; queue wall clocks continue. Only a configured generation admits or
activates.

An update binds its expected current revision/hash and exact canonical next
bytes. Under `owner.lock`, it stages/fsyncs and atomically no-replace-installs the
next immutable chained generation before advancing the workspace policy ref and
workspace record revision. If the current ref remains the expected predecessor,
an exact already-installed next generation is completed; if the ref already
names that exact next generation whose prior hash is the expected ref, retry
returns the original success. Every other stale ref or byte/hash conflict fails
before mutation. Every terminal start command records the exact
decision policy ref. `run_started` records the admission revision/hash, and every
immediate or dequeued `run_activated` records the configured revision/hash used
for activation.

`activeCount` counts non-final ledgers with `run_activated`; `queuedCount` counts
ledgers whose latest projected status is exactly `queued` (and therefore have no
`run_activated`). Unactivated `canceling` or `failing` ledgers are not queued and
can never activate. `maxActiveRuns` alone gates activation. Before a fresh start may activate, eligible queued runs activate by
smallest `workspaceAdmissionSeq` while capacity remains. A new start may then
append `run_started` plus immediate `run_activated` if capacity still exists,
even when `maxQueuedRuns=0`. With no active slot, `maxQueuedRuns` alone gates the
new queued admission: append only `run_started` when
`queuedCount < maxQueuedRuns`; otherwise record `run_queue_full` with the
decision policy ref before a run directory/event. Lowering `maxActiveRuns`
blocks only new activation until active count fits; lowering `maxQueuedRuns`
blocks only fresh queued admission while queue count is at or above the new
limit and never blocks dequeue. Counter gaps are allowed and reuse is forbidden.
`run_started` alone projects queued; unique `run_activated` projects running and
is required before every graph/dispatch event. Restart recomputes exact counts/
FIFO for current state from run ledgers and strict-validates every retained
policy ref. Schema 2 has no workspace-global admission-decision sequence: refs
attribute exact policy bytes but do not independently prove historical cross-run
capacity interleaving. Concurrent starts still serialize under the authority
critical section and are certified by contention/crash tests. Queue time
consumes wall clock,
and an expired queued run can fail without activation. Cleanup, cancellation,
and recovery precede fresh activation.
Every activated non-final ledger counts against `maxActiveRuns`, including while
blocked, `waiting_human`, canceling, or failing, and releases that slot only at an
execution-final event. This conservative first policy is reconstructible from
the ledger without an unmodeled requeue transition.

Archon may author, validate, and inspect workflow definitions offline. All run
commands and run list/status/log/follow require the coordinator's sanitized
projection. Archon never reads private ledgers or falls back to a local engine.

## Run Ledger Envelope

Each run is append-only NDJSON. Every line is one JSON object with this minimum
envelope:

```json
{
  "schema": 2,
  "authoritySchema": 2,
  "writerFence": 42,
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
    "workspaceAuthorityId": "wsa_01J9...",
    "workspaceAdmissionSeq": 17,
    "admissionPolicyRev": 3,
    "admissionPolicySha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "admissionCommandId": "cmd_01J9...",
    "commandPayloadSha256": "sha256:...",
    "boardSlug": "session-search",
    "boardPath": ".formations/boards/session-search.formation.toml",
    "sourceBoardSchema": 2,
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

All schema-2 ledger events include `schema=2`, supported `authoritySchema`,
their origin `writerFence`, `ts`, `runId`, monotonic `seq`, `type`, and `actor`.
Fences are monotonic non-decreasing allocated owner epochs; historical lower
prefixes remain valid after takeover, while a regression/unallocated fence or a
new append not using the current fence is invalid.
Events
that touch graph objects include the relevant `boardId`, `boardRev`, `missionId`,
`nodeId`, `slotId`, `gateId`, `edgeId`, `attempt`, or `epoch`. `run_started`
is the first event and must include top-level board id/revision plus opaque
workspace/run authority ids, admission command id/hash, durable admission
sequence and admission-policy revision/hash, exact graph/private-binding/projection hashes, `runRoot`, typed
authored-config root-input projection, and run id. `missionId` and optional `beadId` are top-level only
for a Mission root, and `runRoot.nodeId` must equal that `missionId`; an isolated
Formation root omits both.
Each command-effect event exact-matches every duplicated command field in its
stored hash-bound payload, including envelope actor, workspace/run identity,
reason, mode/verdict, and precondition sequence.

A ledger without `schema` is schema 1 and projects with its recorded legacy
semantics. Schema-1 events are never reinterpreted as exact Gate pass-through,
typed feedback, or schema-2 input refs. One ledger cannot mix event schemas.
One schema-2 ledger also has one supported `authoritySchema`; an authority bump
starts a new run and is never mixed into an existing ledger.
Schema-1 runs remain inspectable but are not resumable by the schema-2 engine;
resume fails with `legacy_run_requires_new_run`.
Recognizing those numbers is not semantic support. Schema-2 admission remains
disabled until the complete safe projector/coordinator capability set is
registered and every runnable rollback binary honors the immutable bootstrap
plus mutable workspace-authority guard; pre-guard binaries are prohibited
runtime rollback targets.

`WorkspaceAuthority.authoritySchema` is the monotonic high-water mark for the
whole authority domain. An upgrade holds `owner.lock`, validates the current
fence, and atomically advances/fsyncs that mark before any command, run, event, or
private record with new semantics. It never decreases. A crash after advancement
is safely unavailable to an older reader, not downgraded; a new schema starts a
new run and older ledgers retain their original semantics.

Run state is epoch-aware:

```text
run_started seq=1, epoch=0, projects queued
run_activated appears at most once and projects running before graph/dispatch events
run_blocked closes an epoch and may be resumed explicitly
run_resumed opens the next epoch after an operator resume
human_input_requested enters waiting_human until its exact decision or cancel
run_cancel_requested enters canceling and forbids dispatch, replay, or resume
run_failure_reconciliation_started enters failing and admits only reconciliation
run_succeeded, run_failed, and run_canceled are final for execution
```

Throughout this contract, wording that a condition "selects", "records", or
"appends" `run_failed(...)` names the terminal outcome, not permission for a
direct append. The writer must first fsync the unique
`run_failure_reconciliation_started` and may append `run_failed` only through
that frozen lifecycle.

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

Only a coordinator holding a newly fsynced current fence may recover. The crash
matrix is closed: a pending start resumes bounded admission from its stored
payload; pending resume/cancel/verdict recheck their exact precondition and apply
once or reject durably; any durable matching command effect with a missing
receipt repairs that applied receipt without reapplication; an uncertain
`slot_dispatch` reattaches only; a successful scheduled-terminal or first
non-`ok` deciding `slot_result` with missing `formation_result` derives that
result once from immutable slot-turn envelopes and never dispatches the next
phase; a durable `formation_result`/`tool_result`
with missing `node_output` materializes exact output from immutable result
authority; and a durable
`node_output` replays only missing routing/finality. A durable
`run_cancel_requested` permits cancellation reconciliation only. A durable
`run_failure_reconciliation_started` projects `failing` and resumes only its
exact frozen cause/snapshots; it cannot choose a new cause, resume ordinary
execution, or resend any durable slot-interrupt request. None of these
paths reparses mutable capture, redispatches completed work, or lets the prior
fence append or act. Historical origin-fence records remain valid inputs; every
recovery mutation records the new current state fence while a repaired command
receipt preserves the matched effect event's origin fence as its immutable
outcome fence.

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
Before appending `run_started`, the current fenced coordinator creates and fsyncs one
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
`run_started`, it is not a run and sends nothing. Only the current owner, after
acquiring/fsyncing a newer fence and validating that the orphan's immutable
workspace, command, and `originWriterFence` belonged to the historical owner
epoch, may claim cleanup with its current `stateWriterFence`. Recovery first cleans and fsyncs
every understood pending-redaction obligation/raw target, then deletes the
orphan tree idempotently and fsyncs its parent. Unsupported, conflicting, or
unprovable cleanup/identity fails loud; only a supported current fenced owner may
quarantine the tree as non-authorizing with no
public bytes or replay handle; it is never adopted as a run.

Run preflight returns one `SlotResolution` per selected declared slot; unassigned
is `unresolved/agent_unassigned`. It starts only when every resolution is
runnable. Production resolution calls the same Terminal-session resolver and
configured inventory used by cockpit Terminal tabs. That inventory is the union
of explicitly configured user/socket sources; no separate Formations production
source or pool exists. Accumulated session context is intentionally reused. A
persona stem matching more than one inventory source is ambiguous and fails
loud; board files never choose a raw socket path. A matching candidate is
runnable only when it is unleased, unattached, and the certified harness
adapter's non-pane control channel proves a closed/ready turn for the exact
fingerprint. Pane silence, prompt text, and quiet output are never readiness
evidence. Stable unavailable reasons are `session_target_leased`,
`session_target_attached`, `session_target_harness_busy` when the adapter
certifies an open turn, and `session_target_readiness_unknown` when exact
supported readiness evidence is missing, stale, unsupported, or non-unique;
`session_target_attachment_audit_unavailable` means the complete certified
client/input monitor cannot be armed.
Unknown fails unavailable and creates no binding. Connected hidden CHROTE
Terminal iframes count as attached, and slot binding never detaches them.
The user may explicitly disconnect a CHROTE-owned presentation client before
retrying, but cannot reclaim an external attachment or another run's lease. The
final atomic acquisition rechecks registry state, attachments, fingerprint, and
certified readiness under the target critical section and records a fresh
one-shot `target-ready-proof-v1`; any failure sends nothing and uses the matching
stable reason under `session_target_busy`. It never steals, creates, or selects
an alternate target. Disposable inventories are isolated test,
certification, and dogfood fixtures only; topology evidence registers one through
the shared Terminal resolver for both consumers. A successful run
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
below is the atomic authority immediately before send. A frozen binding grants
no attachment exception; Formation ownership begins only when the exact
target-registry occupancy is fsynced. Any client already attached at that
transition is competing. After the matching ledger dispatch is durable, only an
exact run-bound Peek capability may attach as the run-owned exception. Shared
Terminal attach paths reject ordinary attachment while occupancy exists. A
certified writer-private monitor whose registration was armed before occupancy
fsync accounts for every subsequent client event;
foreign attachment/input or lost audit continuity latches the dispatch
non-authorizing, revokes Peek, and forbids result/ordinary release pending exact
quiescence reconciliation.

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
the envelope already names the field. Any event, private record, or field that
can change admission, identity, dispatch, result acceptance, routing, cleanup,
cancellation, or finality requires a supported authority-schema bump. An older
or unsupported reader may consume only a separately certified sanitized
projection; it never allocates a fence, adopts, cleans, quarantines, dispatches,
accepts results, mutates execution state, or finalizes. Matching schema numbers
alone do not grant support. A projection-only extension may remain ignorable only
when it is schema-registered, redaction-classified, excluded from every public
safe-field allowlist, and unable to change status, actions, bindings, artifacts,
or execution. Redact=true rejects an unregistered field before append. Public
run-detail, bounded-event, SSE, CLI, and UI projection
uses an event-type safe-field allowlist and drops/rejects every unknown or private
key rather than passing its value through. The required keys below must be
present.

| Event type | Required payload |
|---|---|
| `run_started` | opaque `workspaceAuthorityId`, monotonic `workspaceAdmissionSeq`, exact `admissionPolicyRev`/`admissionPolicySha256`, `admissionCommandId`, `commandPayloadSha256`, `boardSlug`, `boardPath`, `sourceBoardSchema`, `snapshotSchema`, opaque `runAuthorityId`, `graphSnapshotSha256`, `privateBindingsSha256`, `bindingProjectionSha256`, `runRoot`, exact closed `rootInputProjection` classified `authored_config` with source kind, versioned encoding/media/SHA-256, and canonical `text`, `limits` (`maxDispatch`, `maxAttempts`, `wallClockSeconds`, `redact`); private paths never appear, and board/revision plus conditional Mission ids stay in the envelope |
| `run_activated` | `workspaceAdmissionSeq`, exact configured `admissionPolicyRev`/`admissionPolicySha256`, `reason` (`immediate`, `dequeued`); unique per run, requires latest projected status exactly `queued`, consumes one active slot, and is fsynced before every graph/dispatch event; unactivated `canceling`/`failing` runs are ineligible |
| `run_resumed` | `commandId`, `commandPayloadSha256`, `resumedFromSeq`, `resumedBy`, `resumeMode` (`reattach`, `retry-failed-producer`), bounded redaction-safe `reason`, `openDispatches`, exact `retryTargets`; `reattach` preserves the blocked event's exact unmatched-dispatch set and never creates a dispatch, while failed-producer retry requires `openDispatches=[]` and exactly one target |
| `node_waiting` | `nodeId`, `neededInputs`, `readyInputs`, `totalInputs`, `waitingFor` (`edgeId` or `portId` list) |
| `node_input_ignored` | `nodeId`, `toPortId`, `inputRef`, `reason` (`late_optional`, `duplicate_feedback`, `stale_feedback`, `mismatched_feedback`), `relatedAttempt` |
| `node_started` | `nodeId`, `nodeKind` (`mission`, `formation`, `tool`, `gate`), `attempt`, `reason` (`initial`, `resume`, `pushback`, `revision-cycle`, `judge`); ordinary attempts require immutable durable `inputRefs`, judge attempts instead require `contextEncoding=judge-context-jcs-v1`, immutable `judgeContextSha256`/`priorResultSeqs`; optional `triggerFeedbackId`/`priorGateSeq` |
| `slot_binding_observed` | `bindingId`, `slotId`, `sessionTargetId`, `health` (`runnable`, `unavailable`, `stale`), stable `reason`, `observedAt`, `relatedSeq` |
| `slot_dispatch` | `dispatchId`, `targetLeaseId`, unique attempt-scoped `turnKey`, `turnPhase` (`solo`, `flow-step`, `peer-turn`, `peer-facilitator`, `leader-plan`, `leader-worker`, `leader-agentic`, or judge-specific), closed `turnInputs={nodeStartedSeq,priorTurnResults}` with exact ordered `{slotResultSeq,turnResultSha256}` entries, `nodeId`, `attempt`, `slotId`, `agentId`, `harness`, `bindingId`, `sessionTargetId`, `targetFingerprint`, `dispatchInputBarrierEncoding=target-dispatch-input-barrier-v1`, host-private exact `dispatchInputBarrier`, `dispatchInputBarrierSha256`, `targetReadyProofEncoding=target-ready-proof-v1`, host-private exact `targetReadyProof`, `targetReadyProofSha256`, `paneHistoryBaselineEncoding=tmux-pane-history-baseline-v1`, host-private exact `paneHistoryBaseline`, `paneHistoryBaselineSha256`, `steeringGeneration="0"`, `promptSha256`, boolean `nativeAck`, `recordedBeforeSend=true`; `nativeAck` is never adapter text, no prompt or pane bytes/ref/path/authority id is durable, and the certified monitor drains its journal and installs the one-send input fence before the fresh ready proof/fingerprint/baseline are captured under the authority/target critical section; after event fsync only the same already-hashed in-memory prompt slice may consume that fence exactly once |
| `slot_peek_capability_issued` | exact `dispatchId`, `targetLeaseId`, `bindingId`, `sessionTargetId`, `targetFingerprint`, monotonic unsigned-decimal-string `capabilityGeneration`, JSON-safe integer `priorIssuedSeq` (`0` iff first, otherwise the exact latest issuance), `issuedAt`; before a later issuance all clients/input for the prior issuance are closed/drained; event fsync atomically invalidates every earlier token/generation before the new token may be exposed, contains no token or routing secret, is valid only for active authorizing occupancy before cancel/failure/revocation, and only this latest issuance authorizes attach/detach/select metadata for its generation but no input bytes |
| `slot_steering_started` | exact `dispatchId`, `targetLeaseId`, `bindingId`, `sessionTargetId`, `targetFingerprint`, exact latest `capabilityIssuedSeq`/`capabilityGeneration`, monotonic unsigned-decimal-string `steeringGeneration`, bounded safe `actor`, `startedAt`, `recordedBeforeInput=true`; unique open generation, fsynced before the first input bytes are forwarded, contains no input bytes/hash/text, and is valid only for the current exact target occupancy and latest issued run-bound Peek capability before any cancel/failure reconciliation or `slot_peek_capability_revoked` |
| `slot_steering_ended` | exact `startedSeq`, `dispatchId`, `targetLeaseId`, `targetFingerprint`, matching `steeringGeneration`, `reason` (`released`, `disconnect`, `capability_revoked`, `recovered_revoked`), `endedAt`; appended only after the input channel is closed/drained, closes exactly one open generation, and does not itself prove turn closure |
| `slot_peek_capability_revoked` | exact `dispatchId`, `targetLeaseId`, `bindingId`, `sessionTargetId`, `targetFingerprint`, latest `capabilityGeneration` (unsigned-decimal string, `"0"` when none), JSON-safe integer `capabilityIssuedSeq` (`0` when none), latest closed `steeringGeneration`, `reason` (`result_closure`, `cancel`, `failure`, `foreign_attachment`, `foreign_input`, `attachment_audit_lost`, `recovered_fence`), `revokedAt`, `inputClosed=true`; `foreign_input` is the closed compatibility class for any unregistered pane mutation or input; unique and irreversible for the dispatch, required before every result or final slot proof even when no capability was issued, fsynced only after new capability issuance is stopped and every client/input channel is closed/drained, requires any open generation to have a preceding matching `slot_steering_ended`, contains no capability token or input bytes, and forbids all later run-bound attach/input |
| `slot_reconciliation_interrupt` | exact `dispatchId`, `targetLeaseId`, `bindingId`, `sessionTargetId`, `targetFingerprint`, `authorityKind` (`cancel` or `failure`), `authoritySeq`, `interruptEncoding=terminal-etx-v1`, `interruptSha256`, `recordedBeforeSend=true`; unique per dispatch and fsynced after Peek revocation but before any interrupt bytes; `authorityKind=cancel` iff `authoritySeq` names the same run's unique `run_cancel_requested`, while `failure` iff it names the same run's unique `run_failure_reconciliation_started`, and the exact dispatch must occur in that authority event's snapshot; after a cancel-origin failure start, every cancel-authorized request predates the start and is reused, while a still-open slot with no prior request may create only a failure-authorized request naming that start; it creates one exact coordinator-only fence permit and never authorizes resend |
| `slot_reconciliation_interrupt_outcome` | exact `requestedSeq`, `dispatchId`, `targetLeaseId`, `targetFingerprint`, `outcome` (`sent`, `unavailable`, `unsupported`), `observedAt`; unique, appended after the one permitted adapter attempt, contains no adapter text, and is optional only when a crash leaves the already-attempted effect uncertain |
| `slot_result` | `dispatchId`, `targetLeaseId`, matching `turnKey`/`turnPhase`, `nodeId`, `attempt`, `slotId`, `agentId`, `bindingId`, `sessionTargetId`, `targetFingerprint`, `paneHistoryBaselineEncoding`, `paneHistoryBaselineDispatchSeq`, matching `paneHistoryBaselineSha256`, exact `peekCapabilityRevokedSeq`, latest closed `steeringGeneration`, boolean `operatorInfluenced`, `status` (`ok`, `error`, `needs-review`), terminal `capturedRange`, exact `sentinel`, host-private exact `clientAttachmentAuditProof`, matching `clientAttachmentAuditProofSha256`, closed `turnClosureProof`, closed `turnResult`, `turnResultEncoding=slot-turn-result-jcs-v1`, `turnResultSha256`; `operatorInfluenced` is true iff this dispatch has at least one valid `slot_steering_started` (equivalently latest `steeringGeneration` is not `"0"`) and false only at generation `"0"`; the raw baseline token is not repeated, `capturedRange.start` exact-matches the dispatch boundary and its end is strictly later, and the certified closure proof exact-matches the baseline hash, irreversible capability revocation, latest steering generation, and continuous attachment-audit proof and is not satisfiable by pane bytes alone; the envelope's required turn key/phase/status exact-match the event, capture is parsed once, and referenced artifacts are registered before this event; bounded closed capture with a valid terminal sentinel but invalid outputs becomes the byte-exact `invalidFormationOutputsProjection` defined below with `outputs={}`, while timeout, unclosed steering, unrevoked capability, foreign/unknown attachment audit, lost baseline, or unclosed output has no result/ordinary release |
| `formation_result` | `nodeId`, `attempt`, status (`done`, `needs-review`, `failed`), full durable safe canonical `outputs` map, exact-key `outputHashes`, optional `reportArtifactId`, stable-order `artifactIds` and `diffArtifactIds`, stable ascending `contributingSlotResultSeqs`, `resultEncoding=formation-result-jcs-v1`, `resultSha256`; exactly one after a complete successful schedule or its first non-`ok` result, deterministically derived from the completed prefix after output validation and prior non-Tool artifact registration; a pre-result resource block has no result |
| `tool_dispatch` | `toolLeaseId`, `nodeId`, `attempt`, `toolBindingId`, `inputManifestSha256`, `inputHashes`, `profileSha256`, `parametersSha256`, `policySha256`, `determinismPolicySha256`, `executionBundleSha256`, private lease-root authority or redacted `redactionObligationId`, `recordedBeforeExecute=true` |
| `tool_process_launch` | `toolLeaseId`, unique `launchId`, `nodeId`, `attempt`, monotonically increasing `generation` starting at 1, unique opaque non-PID `processScopeId`, opaque `deadlineAuthorityId`, `recordedBeforeSpawn=true`; private supervisor scope and immutable deadline-authority records are durable first, exact-match the open lease/launch tuple, and each launch consumes dispatch and wall-clock limits |
| `tool_result` | `toolLeaseId`, `launchId`, `generation`, `nodeId`, `attempt`, `status` (`ok`, `error`, `timeout`), full durable canonical output-projection map with explicit exactness, `outputHashes`, authoritative new `artifactRegistrations`, `artifacts`, optional bounded non-routable `displayEvidence`, `timing`; one fsynced event atomically closes the lease/latest launch and registers its new artifact ids |
| `node_output` | `nodeId`, `status` (`done`, `needs-review`, `blocked`, `failed`), durable `PayloadProjection` values keyed by stable output port id, optional `reportArtifactId`, stable-order `artifactIds` and `diffArtifactIds`, `producedBy`, `timing`, `deliveredEdges`; every top-level artifact id is already durably registered, Mission `out` requires the exact classified `rootDerivedPayloadProjection` matching `run_started.rootInputProjection`, and an ordinary Formation requires exact `formationResultSeq`/`resultSha256` plus status/output/artifact fields byte-equal to that immutable result |
| `gate_evaluating` | `gateId`, `gateAttempt`, `nodeId`, `kinds`, typed `criterionProjection` with `classification=authored_config`, `sourceKind=gate_criterion`, `encoding=gate-criterion-utf8-v1`, and `mediaType=text/plain`, `inputRef`, `judgeChain`, optional `revisionCycleId`/`triggerFeedbackId`/`priorGateSeq`; no dynamic value is interpolated into the durable criterion |
| `gate_kind_result` | `gateId`, `gateAttempt`, `kind` (`code`, `formation`), strict `verdict` (`pass`, `fail`), bounded safe `reason`, typed `evidence`, `evaluatedInputRef`, `resultEncoding=decision-result-jcs-v1`, `resultSha256`, `relatedSeqs`; code additionally requires `gateBindingId`, `inputSha256`, `profileSha256`, `evaluatorBundleSha256`, `parametersSha256`, `policySha256`, `determinismPolicySha256`; unique per Gate attempt/kind and fsynced before the next kind |
| `judge_result` | `gateId`, `gateAttempt`, `judgeNodeId`, `judgeAttempt`, `chainIndex`, `contextEncoding=judge-context-jcs-v1`, `contextSha256`, `priorResultSeqs`, strict `result`, `resultEncoding=decision-result-jcs-v1`, `resultSha256`; completes that judge Formation attempt and is fsynced before the next member dispatch |
| `judge_attempt_failed` | `gateId`, `gateAttempt`, `judgeNodeId`, `judgeAttempt`, `chainIndex`, `contextSha256`, `priorResultSeqs`, `code=invalid_judge_result`, bounded safe `reason`, `relatedSeq`; completes that judge Formation attempt as failed and blocks the Gate |
| `gate_verdict` | `gateId`, `gateAttempt`, `verdict` (`pass`, `fail`), exact all-declared-kind `perKind`, `kindResultSeqs`, `evaluatedInputRef`, `routePort` (`pass`, `fail`), `routedEdges`, bounded redaction-safe `reason`; PASS preserves the exact authorized live value and durable ref/projection, while FAIL creates exactly one stable typed `feedbackPayload` referenced by zero or more fail-edge traversals (zero blocks); invalid/waiting evaluation emits no aggregate verdict |
| `artifact_attached` | `artifactProjection` with new stable `artifactId` (available safe ref or unavailable/redacted/expired metadata), discriminated non-Tool `source` (`slot`, `gate`, or `system`); appended/fsynced before any later reference |
| `artifact_observed` | existing `artifactId`, `availability` (`available`, `unavailable`, `redacted`, `expired`), `artifact` required only for `available` with matching id and exact first-established descriptor, stable `errorCode` required for all other states, `observedAt`, `relatedSeq` |
| `escalation_raised` | `trigger`, `severity` (`info`, `needs-attention`, `stop`), bounded redaction-safe `reason`, `source` (`system`, `agent`, `human`), `nodeId`, `gateId`, `blocks` |
| `human_input_requested` | `gateId`, `gateAttempt`, `nodeId`, bounded closed fixed-system `promptProjection` using template `gate-human-verdict-v1`, exact closed fixed-system `choiceProjections` object keyed by `pass` and `fail`, `requestedBy`, `evaluatedInputRef`, exact `completedKindResultSeqs`; fields never interpolate runtime input/output/evidence/secrets, the request is unique for that Gate attempt and projects `waiting_human`, and it has no independent timeout or default verdict |
| `human_verdict_recorded` | `commandId`, `commandPayloadSha256`, `gateId`, `gateAttempt`, `nodeId`, `verdict` (`pass`, `fail`), bounded safe `reason`, `requestedSeq`, `decidedBy`; exactly once for the matching outstanding request |
| `error` | `code`, bounded redaction-safe template `message` with no raw adapter/error text, `boundary` (`engine`, `writer`, `adapter`, `tmux`, `schema`, `operator`, `evaluator`), `errorScope` (`run`, `node`, `gate`, `slot`, `tool`), conditional graph identity, `recoverable`, `relatedSeq` |
| `run_blocked` | stable bounded redaction-safe `reason`, `blockScope` (`run`, `node`, `gate`), conditional `blockedNodeId`/`blockedGateId`, `resumeAllowed`, `resumePolicy` (`retry_failed_producer`, `reattach_only`, `new_run_required`), `openDispatches`, `retryTargets`, conditional `nextEpoch`; exact policy invariants below are writer-enforced |
| `run_cancel_requested` | `commandId`, `commandPayloadSha256`, bounded redaction-safe `reason`, `requestedBy`, exact `openNodeAttempts`, exact `openSlotDispatches` including each capability/generation snapshot, exact `openToolLeases`; unique per run, appended/fsynced before cancellation work, and makes the writer reject new dispatches, launches, results, outputs/routing, ordinary replay/rerun, resume, or new Peek capability/input; the post-cancel execution allowlist is only steering drain/end, Peek revocation, reconciliation-interrupt request/outcome, target/Tool quiescence cleanup, `run_failure_reconciliation_started` escalation, and the matching final event |
| `run_canceled` | `cancelRequestSeq`, bounded redaction-safe `reason`, `requestedBy`, exact `nodeAttemptDispositions`, exact `slotDispatchDispositions`, exact `reconciledToolLeases`, `final=true`; all three arrays exactly cover the named request snapshots |
| `run_failure_reconciliation_started` | `originCancelRequestSeq` (`0` iff no prior cancel request; otherwise exact sequence of the run's unique `run_cancel_requested`), `code`, bounded redaction-safe `reason`, `unrecoverable`, `relatedSeq`, typed `failureCause`, exact `openNodeAttempts`, exact `openSlotDispatches` including capability/generation and prior interrupt request/outcome state, exact `openToolLeases`, `recordedBeforeReconciliation=true`; unique per run, fsynced before failure interrupts/cleanup, freezes the eventual failure cause/snapshots, and rejects results, outputs/routing, launches, resume, new capability/input, and another failure start; its closed allowlist is steering drain/end, Peek revocation, reconciliation-interrupt request/outcome, target/Tool quiescence cleanup, and the matching `run_failed` |
| `run_failed` | exact `failureReconciliationSeq`, `code`, bounded redaction-safe `reason`, `unrecoverable`, `relatedSeq`, typed `failureCause`, exact `nodeAttemptDispositions`, exact `slotDispatchDispositions`, exact `toolLeaseDispositions`, `final=true`; cause/header fields byte-match the named start and all three arrays exactly cover its snapshots |
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
- The two synthesized ordinary-Formation projections are byte-exact constants.
  `formationNeedsReviewProjection` is exactly
  `{availability="available",exact=true,payload={kind="unavailable",
  code="formation_needs_review",message="Formation requires review",
  retryable=true}}`. `invalidFormationOutputsProjection` is exactly
  `{availability="available",exact=true,payload={kind="error",
  code="invalid_formation_outputs",
  message="Formation outputs do not match the declared ports",retryable=true}}`.
  No locale, parser text, adapter text, or alternate message participates in
  their canonical hashes. `retryable=true` makes either completed producer
  eligible only for the existing explicit whole-producer retry selection after
  quiescence; neither routes an edge or retries automatically.
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
- `turnInputs`: exactly `{nodeStartedSeq,priorTurnResults}` with no unknown keys.
  `nodeStartedSeq` names the same Formation attempt's unique `node_started` and
  therefore its frozen ordered input refs. `priorTurnResults` is an ordered array
  of exactly `{slotResultSeq,turnResultSha256}`; every sequence names an earlier
  result in that attempt and the hash exact-matches it. The fixed phase rule below
  determines the complete array; missing, extra, duplicate, or reordered entries
  are invalid before dispatch.
- `slot-turn-result-jcs-v1`: RFC 8785 canonical UTF-8 JSON over exactly
  `{turnKey,phase,status,turnPayload,outputs,reportArtifactId,artifactIds,
  diffArtifactIds}` with no unknown keys or trailing newline. `turnKey`, `phase`,
  `status`, and `turnPayload` are required, and phase exact-matches dispatch.
  Missing optional `reportArtifactId` normalizes to `""`, missing `outputs` to `{}`, and
  missing artifact arrays to `[]`. `turnPayload` and every output are durable
  safe `PayloadProjection` values, output keys are stable declared port ids,
  and arrays preserve stable order. `turnResultSha256` hashes those exact
  canonical bytes. The parser builds this value once from the bounded closed
  turn before `slot_result`; the immutable graph snapshot's fixed
  formation-type rule consumes only ordered immutable turn results after a
  crash.
- `formation-result-jcs-v1`: RFC 8785 canonical UTF-8 JSON over exactly
  `{status,outputs,outputHashes,reportArtifactId,artifactIds,diffArtifactIds,
  contributingSlotResultSeqs}` with no unknown keys or trailing newline.
  Missing optional id/array values normalize to `""`/`[]`; output object keys
  are stable declared port ids, `outputHashes` has exactly the same keys, arrays
  preserve their declared stable order, and every output is a durable safe
  `PayloadProjection`. Status is exactly `done`, `needs-review`, or `failed` under
  the fixed turn mapping. Each `outputHashes[portId]` is the SHA-256 of RFC 8785
  canonical UTF-8 JSON for that exact closed `outputs[portId]` projection with
  no trailing newline. `resultSha256` hashes
  those exact bytes. Redact=true can retain only this safe projection; discarded
  raw output is never reconstructed from the hash.
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
- `attachment-audit-registration-v1`: writer-private RFC 8785 canonical UTF-8
  JSON over exactly
  `{monitorVersion,workspaceAuthorityId,writerFence,targetKeySha256,targetFingerprintSha256,sourceIdentitySha256,eventEpoch,startOffset,inputGateSha256}`.
  `monitorVersion` is the literal `formations-tmux-input-fence-v1`;
  `writerFence` is a positive JSON-safe integer and `startOffset` is an
  unsigned-decimal uint64 string;
  `eventEpoch` is canonical unpadded base64url for exactly 16 bytes; all hash
  fields are 64 lowercase hex. The target-key/fingerprint hashes use their exact
  private UTF-8 strings with no newline. `sourceIdentitySha256` commits to the
  certified adapter's canonical tmux server/socket/pane input boundary, and
  `inputGateSha256` commits to its enforceable route gate. The exact registration
  and its anchored first `fence_transition` record are fsynced before occupancy;
  `attachmentAuditRegistrationSha256` hashes these bytes and exact-matches the
  target record. `startOffset` is that first record's offset, and its
  `priorRecordSha256` is exactly this registration hash. There is no empty-chain
  occupancy form.
- `tmux-target-interaction-journal-v1`: a writer-private append-only hash chain
  in the registration's continuity domain. Every fixed-shape record is RFC 8785
  JSON over exactly
  `{eventEpoch,offset,priorRecordSha256,eventKind,eventIdentitySha256,routeKind,routeSeq,targetStateSha256}`.
  Epoch/offset use the registration grammar; hashes are 64 lowercase hex.
  `eventIdentitySha256` hashes exactly the domain-separation bytes
  `formations-interaction-event-v1` followed by one boundary-generated 32-byte
  CSPRNG nonce. The nonce is never derived from or replaced by input bytes,
  text, content hashes/digests, capability tokens, client ids, paths, terminal
  bytes, or any reversible/guessable derivative. `targetStateSha256` hashes RFC
  8785 canonical `target-interaction-state-v1` over exactly
  `{targetFingerprintSha256,interactionLatch,fenceState}`. The latch is `none`,
  `foreign_attachment`, `foreign_input`, or `audit_lost`; `fenceState` is
  `registration`, `dispatch_permit`, `peek`, `reconciliation_permit`, `closure`,
  `latched`, or `released`. It explicitly excludes all values prohibited above;
  neither hash is content evidence.
  `eventKind` is one of `attach`, `detach`, `select`, `resize`, `reflow`,
  `history_clear`, `pane_respawn`, `pane_kill`, `pane_move`, `pane_join`,
  `pane_break`, `pane_swap`, `target_mutation`, `input`, `dispatch_send`,
  `peek_input`, `reconciliation_interrupt`, or `fence_transition`. Every
  pane-affecting route not otherwise named is `target_mutation`, never omitted.
  `routeKind` is `foreign`, `dispatch_prompt`,
  `peek_attach`, `peek_steering`, `reconciliation_interrupt`, or `monitor`; `routeSeq` is the
  exact authorizing ledger sequence as a JSON-safe positive integer, or `0` for
  foreign/monitor events. The first record has `offset=startOffset` and
  `priorRecordSha256=attachmentAuditRegistrationSha256`; each later offset is
  the prior unsigned value plus one without wrap, and its predecessor hash
  hashes the preceding exact record. The certified boundary appends/fsyncs
  classification before allowing or rejecting the effect.
- `target-interaction-audit-evidence-v1`: RFC 8785 canonical UTF-8 JSON over
  exactly
  `{registrationSha256,eventEpoch,startOffset,endOffset,terminalRecordSha256,foreignState}`,
  where offsets use the grammar above and `foreignState` is exactly `none`,
  `foreign_attachment`, `foreign_input`, or `audit_lost`.
  Registration hash, epoch, and start offset must byte-match the named
  registration; `endOffset >= startOffset`; and every record from start through
  end is present with exact contiguous offset/predecessor linkage.
  `terminalRecordSha256` hashes the exact canonical record at `endOffset`.
  `foreignState=none` iff every validated record uses an authorized/monitor
  route. Otherwise it is `audit_lost` for any continuity failure, or—when the
  chain is intact—the first foreign record in offset order maps attach/detach/
  select to `foreign_attachment` and mutation/input routes to `foreign_input`.
  An `audit_lost` record commits to the last validated non-empty prefix and
  cannot regain `none` in that registration epoch.
  `monitorEvidenceSha256` hashes these exact bytes. Only `foreignState=none`
  can support dispatch/result proof. The exact registration, journal chain,
  evidence record, dispatch/closure barriers, and audit proof remain
  writer-private authority through release-receipt and execution-final fsync;
  recovery validates the chain and hashes rather than trusting a projection.
- `target-ready-proof-v1`: RFC 8785 canonical UTF-8 JSON over exactly
  `{targetFingerprintSha256,acquisitionChallengeSha256,dispatchInputBarrierSha256,harnessReadyEvidenceSha256}`
  with no unknown keys or trailing newline. Every value is 64 lowercase hex
  characters. `targetFingerprintSha256` hashes the exact UTF-8 bytes of the
  frozen dispatch `targetFingerprint` string with no trailing newline; the
  challenge is freshly generated for this atomic acquisition. The certified
  adapter's non-pane control channel emits this one-shot proof only for a
  closed/ready turn on that fingerprint. Pane bytes, silence, prompt text, an
  unsupported adapter, or a replayed challenge cannot produce it;
  `targetReadyProofSha256` hashes those exact canonical bytes.
- `target-dispatch-input-barrier-v1`: RFC 8785 canonical UTF-8 JSON over exactly
  `{dispatchId,targetLeaseId,targetFingerprintSha256,attachmentAuditRegistrationSha256,promptSha256,monitorEvidenceSha256}`
  with no unknown keys or trailing newline; every SHA-256 is 64 lowercase hex.
  Before this record is issued, the certified target monitor drains and
  linearizes every client/input event since its pre-occupancy registration and
  installs a per-target adapter fence. A foreign/unknown event before the
  barrier fails without `slot_dispatch` or send. After the barrier, the fence
  synchronously rejects or latches every attach/detach/target-selection,
  resize/reflow, history clear, pane lifecycle/topology/other target mutation,
  and input-capable route except one coordinator send whose exact
  dispatch/lease/fingerprint and in-memory bytes match `promptSha256`; event
  fsync precedes that one send. After the permit is consumed, the fence admits
  only attach/detach/select metadata exact-matching the latest durable
  `slot_peek_capability_issued`, exact latest-issuance steering-generation-gated Peek input, or
  the unique durable reconciliation-interrupt permit defined below. Lost barrier continuity
  leaves the dispatch unsent or unmatched and fails closed; it never authorizes
  resend. `dispatchInputBarrierSha256` hashes the exact canonical record and
  exact-matches the field in `target-ready-proof-v1`.
- `tmux-pane-history-baseline-v1`: RFC 8785 canonical UTF-8 JSON over exactly
  `{targetFingerprintSha256,historyEpoch,offset,cols,rows}` with no unknown keys
  or trailing newline. The fingerprint is the same exact hash rule above;
  `historyEpoch` is the canonical unpadded base64url encoding of exactly 16
  bytes (therefore 22 characters);
  `offset` is an unsigned decimal uint64 string with no leading zero except
  `"0"`; and `cols`/`rows` are JSON integers in `1..65535`. It is bounded
  host-private metadata, never pane/input/output bytes. The epoch identifies one
  uninterrupted capture-cursor continuity domain for the exact pane
  fingerprint; `paneHistoryBaselineSha256` hashes those exact canonical bytes.
  Pane replacement, clear-history, or adapter restart without
  durable cursor continuity changes the epoch. Trim past the baseline,
  reset, resize/reflow, missing state, or a non-unique comparison produces
  `capture_baseline_unavailable`; it cannot authorize parsing, `slot_result`, or
  ordinary release.
- `capture-cursor-v1`: exactly `{historyEpoch,offset}` using the baseline grammar.
  A valid post-baseline capture has `capturedRange.start` byte-equal to the
  dispatch cursor and `end` in the same epoch with a strictly greater offset.
- `capturedRange`: `{sessionTargetId,start,end,startedAt,endedAt}` where `start`
  and `end` are exact `capture-cursor-v1` values, not executable text.
- `clientAttachmentAuditProof`: exactly
  `{proofKind="tmux_clients_accounted",dispatchId,targetLeaseId,targetFingerprint,attachmentAuditRegistrationSha256,closureBarrierSha256,terminalCaptureEnd,monitorEvidenceSha256}`.
  The certified writer-private monitor emits it only when its continuity from
  durable occupancy through `terminalCaptureEnd` is intact and every observed
  attach, detach, target-selection, resize/reflow, history,
  pane-lifecycle/topology/other mutation, or input-capable tmux command/control
  route affecting the pane belongs to one closed authorized route for that exact
  occupancy: the barrier-bound one-shot coordinator prompt, the latest
  CHROTE-registered Peek capability issuance for attach/detach/select metadata, a steering
  generation whose inbound bytes terminate at the steering gate, or the
  unique durable reconciliation-interrupt permit. Raw `send-keys`, paste-buffer,
  control-mode, resize, select, attach, and equivalent bypasses are foreign.
  Any superseded issuance/token/generation that reaches this boundary is also
  foreign and latches the dispatch; it is never accepted as an older authorized
  route after reconnect or restart.
  Both SHA-256 fields are 64 lowercase hex; the registration hash
  exact-matches the private target record armed before occupancy, and
  `monitorEvidenceSha256` commits to the
  private ordered client-event journal; raw client ids and capability tokens
  never enter the ledger or projection. A foreign event, transient client,
  unregistered input command, monitor restart without continuity, or non-unique observation cannot
  produce this proof and durably latches the dispatch non-authorizing.
  `clientAttachmentAuditProofSha256` hashes its RFC 8785 canonical UTF-8 JSON
  with exactly the named keys and no trailing newline.
- `target-interaction-closure-barrier-v1`: before the audit proof above is
  issued, the certified monitor installs a per-target mutation/input fence and
  linearizes/drains its private event journal through one RFC 8785 canonical
  record exactly
  `{dispatchId,targetLeaseId,targetFingerprintSha256,monitorEvidenceSha256,terminalCaptureEnd}`.
  `closureBarrierSha256` hashes those exact bytes. Events before the barrier are
  included in `monitorEvidenceSha256`; after it, every unregistered pane
  mutation/input route is synchronously rejected or invalidates the proof before
  publication. The fence remains in force through
  `slot_result` and the registry transition to `result_committed`, or through
  final proof and transition to `final_quiescent`/`terminal_hold`. Receipt fsync
  is the release linearization point; a terminal hold retains the fence. Lost
  fence/barrier continuity permits no stale proof or ordinary release and enters
  quarantine unless exact pane-incarnation-gone proof makes further input
  impossible.
- `terminal-etx-v1` is exactly one byte `0x03`; `interruptSha256` hashes that
  byte. `slot_reconciliation_interrupt` is the only coordinator lifecycle route
  after dispatch. Its event must exact-match the current dispatch, target lease,
  fingerprint, and durable cancel/failure authority before the certified fence
  grants one permit. The permit is consumed at most once. A crash after intent
  fsync never retries: a missing outcome projects `send_uncertain`, and exact
  cancel/ready proof or pane-incarnation-gone proof is still required for
  release.
- `dispatch-cancel-ack-v1` is RFC 8785 canonical UTF-8 JSON over exactly
  `{ackKind="turn_canceled",interruptRequestedSeq,dispatchId,targetLeaseId,targetFingerprintSha256}`;
  `cancelAckSha256` hashes those exact bytes. `cancel-ready-evidence-v1` is the
  same encoding over exactly
  `{evidenceKind="harness_ready",interruptRequestedSeq,dispatchId,targetLeaseId,targetFingerprintSha256,terminalCaptureEnd}`;
  `harnessReadyEvidenceSha256` hashes those bytes. Both originate on the
  certified non-pane adapter channel after the unique interrupt permit; pane
  text, silence, prompt-like output, or an outcome event alone cannot produce
  them.
- `TargetFinalQuiescenceProof` is one closed writer-private union with no unknown
  keys. `proofKind="dispatch_cancel_ack"` is exactly
  `{proofKind,dispatchId,targetLeaseId,targetFingerprint,steeringGeneration,peekCapabilityRevokedSeq,interruptRequestedSeq,cancelAckSha256,terminalCaptureEnd,harnessReadyEvidenceSha256,clientAttachmentAuditProof,clientAttachmentAuditProofSha256}`.
  The nested audit proof exact-matches its hash, target, terminal end, and an
  interaction-closure barrier held through `final_quiescent` receipt fsync; all
  generation/revocation/interrupt fields exact-match the ledger. Alternatively,
  `proofKind="pane_incarnation_gone"` is exactly
  `{proofKind,targetKey,sessionTargetId,targetFingerprint,steeringGeneration,peekCapabilityRevokedSeq,observedAt}`.
  The latter is valid only when a certified resolver observation under the
  target lock proves that exact private key/fingerprint no longer exists;
  `observedAt` is canonical UTC RFC 3339, and name/PID/current-client absence is
  insufficient. A `final_quiescent` receipt stores the exact selected proof and
  retains it through execution-final fsync.
- A `result_committed` receipt contains the exact immutable `slot_result`
  sequence, turn-closure proof, and exact writer-private
  `clientAttachmentAuditProof` matching the result's durable copy/hash, plus one
  closed `releaseProof`: either
  `{proofKind="closure_barrier_held",closureBarrierSha256}` exact-matching the
  audit proof whose input fence remained continuous through receipt fsync, or
  `{proofKind="post_result_pane_incarnation_gone",targetKey,sessionTargetId,targetFingerprint,observedAt}`
  proving the original pane incarnation is gone after a post-result barrier
  fault. The latter never changes/reparses the already-fsynced result. No other
  post-result reconciliation proof is accepted.
- `turnClosureProof`: exactly
  `{proofKind="harness_turn_closed",dispatchId,targetLeaseId,targetFingerprint,paneHistoryBaselineSha256,peekCapabilityRevokedSeq,steeringGeneration,sentinelSha256,terminalCaptureEnd,harnessReadyEvidenceSha256,clientAttachmentAuditProofSha256}`.
  `steeringGeneration` uses the unsigned-decimal-uint64 grammar and
  `terminalCaptureEnd` is the exact `capture-cursor-v1` end. The certified
  adapter obtains ready/closed evidence through a channel not writable as pane
  input; terminal text, echoed sentinel bytes, silence, or generic prompt text
  cannot produce it. The proof is valid only while no steering generation is
  open and all identity/hash/generation/cursor fields exact-match the dispatch
  and candidate result under the held target occupancy. The named revocation is
  unique and irreversible, and `clientAttachmentAuditProofSha256` hashes the
  exact matching closed proof above. Missing audit continuity or foreign input
  cannot be relabeled as operator steering and cannot close a result.
  `sentinelSha256` hashes the exact UTF-8 bytes of the unique terminal sentinel
  token selected by the bounded parser, from its first `<` through its final
  `>` and excluding any line terminator; those bytes must be byte-equal to the
  `slot_result.sentinel` string encoded as UTF-8. Any normalization, duplicate
  candidate, trailing token bytes, or hash mismatch invalidates closure.
- Raw `slot_dispatch` and `slot_result` events are writer-private authority.
  Sanitized run/event/SSE/CLI/UI projections omit `dispatchInputBarrier`,
  `targetReadyProof`, `paneHistoryBaseline`, and the client-attachment proof; they expose only the
  corresponding encodings/hashes and closed validation state (`valid`,
  `unavailable`, or `invalidated`).
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
  values that exactly equals all attempts open immediately before the lifecycle
  event carrying the field: either `run_cancel_requested` or
  `run_failure_reconciliation_started`. `node_started` opens an attempt. Matching
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
  attempt in its named failure-reconciliation snapshot. It rejects missing, duplicate,
  extra, or identity-changing entries; a disposed `waiting_human` request
  cannot later accept a decision.
- `failureHeader`: exactly
  `{code,reason,unrecoverable,relatedSeq,failureCause}` with no unknown keys.
  The five corresponding `run_failed` fields must be byte-equal to those on its
  named `run_failure_reconciliation_started`; lifecycle-only origin/snapshot/
  disposition fields are not part of this header.
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
  sessionTargetId, targetFingerprint, dispatchSeq, peekCapabilityState,
  latestCapabilityGeneration, latestCapabilityIssuedSeq,
  latestSteeringGeneration, openSteeringStartedSeq?, peekCapabilityRevokedSeq?,
  interruptState, interruptRequestedSeq?, interruptOutcomeSeq?}`.
  `peekCapabilityState` is exactly `none`, `issued`, `input_open`, or `revoked`;
  capability generation is an unsigned-decimal string, and issued seq is `0`
  only when no capability exists. The sequence fields must exact-match the
  latest issuance, unique open steering generation, or irreversible revocation.
  These are safe lifecycle facts, never capability tokens or input bytes.
  `interruptState` is `none`, `requested`, `sent`, `unavailable`, or
  `unsupported`; `requested` with no outcome is the crash-safe
  `send_uncertain` state and never permits another request.
- `openSlotDispatches`: stable `dispatchSeq`-order array with the exact
  `openDispatches` item shape that equals all unmatched slot dispatches
  immediately before the lifecycle event carrying the field: either
  `run_cancel_requested` or `run_failure_reconciliation_started`.
- `slotDispatchDispositions`: stable `dispatchSeq`-order array preserving every
  open-dispatch field and adding `disposition`, `softInterrupt` (`sent`,
  `send_uncertain`, `unavailable`, or `unsupported`), exact
  `softInterruptRequestedSeq`, conditional `softInterruptOutcomeSeq`, and `targetLeaseState`
  (`released_quiescent`, `terminal_hold`, or `quarantined`), plus
  `finalPeekCapabilityState=revoked`, required `finalCapabilityGeneration`,
  `finalCapabilityIssuedSeq`, `finalSteeringGeneration`, and
  `finalPeekCapabilityRevokedSeq`. These final fields do not overwrite
  the preserved request-time capability/generation fields. An
  issued capability, open input channel, or open steering generation rejects
  the disposition and finality. The final generation/sequence exact-match the
  durable revocation event and the selected release/hold/quarantine proof.
  `released_quiescent` requires the
  exact target to be proven quiescent and its occupying host-registry record
  durably replaced by an exact non-occupying release receipt before finality.
  That receipt preserves `{targetKey, targetLeaseId, sessionTargetId, runId,
  dispatchId, targetFingerprint, releaseKind=final_quiescent, proof}` across a crash until the
  final event is fsynced. Its proof is a closed writer-validated union: either
  an exact dispatch/lease/fingerprint-bound cancel acknowledgement followed by
  certified harness-ready evidence, terminal capture boundary, irreversible
  capability revocation, and continuous client-attachment audit, or proof that
  the exact pane incarnation/fingerprint is gone. Sent Ctrl-C, silence, generic
  idle/prompt text, display name, bare PID, or an unknown proof kind is invalid.
  `terminal_hold` requires that exact record to be
  durably marked non-authorizing and busy with its Peek capability revoked and
  all steering generations closed before finality; it permits no run-bound
  input. Later
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
  failure, all are abandoned. It exactly covers the dispatches in the named
  failure-reconciliation snapshot. The writer rejects missing, duplicate,
  extra, or identity-changing entries; no disposition kills a tmux session.
  Every soft-interrupt disposition requires `softInterruptRequestedSeq` naming
  the unique exact dispatch request authorized by either the cancel request or
  failure-reconciliation start. `sent`, `unavailable`, and `unsupported` also
  require `softInterruptOutcomeSeq` naming its matching unique outcome;
  `send_uncertain` forbids an outcome sequence. `softInterrupt=sent` is valid only when the frozen binding/target is proven to
  host that exact unresolved dispatch and attempt and the unique request/outcome
  events prove the one permitted attempt. A durable request with no outcome after
  crash is `send_uncertain`, is never retried, and carries only its request seq.
  Otherwise the engine records `unavailable`/`unsupported` and sends no
  keystrokes to possibly unrelated work.
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
  values that exactly equals the open Tool leases immediately before the
  lifecycle event carrying the field: either `run_cancel_requested` or
  `run_failure_reconciliation_started`.
- `reconciledToolLeases`: stable `dispatchSeq`-order array preserving every
  field of the request's `openToolLeases` and adding exactly one `disposition`:
  `never_launched_cleaned` requires no `latestLaunch`, while
  `launch_fenced_cleaned` requires it. The writer rejects any missing,
  duplicate, extra, or changed lease/launch identity.
- `toolLeaseDispositions`: stable `dispatchSeq`-order array preserving every
  `toolLeaseSnapshot` field from the named failure-reconciliation snapshot and
  adding `disposition=failed_private_cleanup_owned` for the lease selected
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
`formation_result`, `node_output`, `judge_result`, `gate_verdict`, display evidence, diff, or
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

For an ordinary workflow Formation, each bounded closed turn is parsed once and
its exact durable payload/declared-output projections are fsynced in a hashed
`slot-turn-result-jcs-v1` envelope before the target is released. The immutable
graph snapshot fixes the turn rule: `solo` has one sole-slot terminal turn;
`flow` runs slots in persisted order, each after the first consuming the prior
turn payload, and its last slot is terminal; `peer` runs one turn per slot in
persisted order then first-slot terminal `peer-facilitator`; `orchestrated` runs
controller `leader-plan`, one `leader-worker` turn per non-controller slot in
persisted order, then controller terminal `leader-agentic`. Every turn is a
coordinator-owned leased dispatch carrying closed `turnInputs={nodeStartedSeq,
priorTurnResults}`. The prior array contains exact ordered
`{slotResultSeq,turnResultSha256}` values: empty for `solo`, first `flow-step`,
each `peer-turn`, and `leader-plan`; only the immediate predecessor for later
`flow-step`; every peer turn in persisted slot order for `peer-facilitator`;
only `leader-plan` for each worker; and plan followed by every worker in
persisted order for `leader-agentic`. Every phase also consumes the frozen node
inputs named by `nodeStartedSeq`. No agent directly mutates another bound tmux
target. Dynamic peer/leader scheduling requires an authority-schema bump.

Only an `ok` terminal turn may carry a non-empty declared-port `outputs` map, and
it must contain all and only declared outputs. Non-terminal and non-`ok` turns
use `outputs={}`. All required `ok` turns map to `done`; first `error` stops and
maps to `failed`, repeating its engine-normalized non-routable error
`turnPayload` at every declared port; first `needs-review` stops and maps to
`needs-review`, repeating exact `{availability="available",exact=true,
payload={kind="unavailable",code="formation_needs_review",
message="Formation requires review",retryable=true}}`. A bounded closed capture
with valid terminal sentinel but invalid output schema becomes exact
`{availability="available",exact=true,payload={kind="error",
code="invalid_formation_outputs",message="Formation outputs do not match the declared ports",
retryable=true}}` with `outputs={}`; timeout/unclosed output has no
result or ordinary release. Pre-result resource block uses `run_blocked` without
a Formation result. The completed successful schedule or first non-`ok` prefix
and output validation precede one unique fsynced
`formation_result` with the complete safe canonical `outputs` map,
`outputHashes`, exact contributing sequences, and only artifact ids already
registered under the rule above. The deciding last turn supplies the report;
`artifactIds`/`diffArtifactIds` are the stable first-seen union over the prefix.
`node_output` exact-matches its result
sequence/hash, status, and output/artifact fields. If a crash occurs before the
Formation result, the current fenced owner derives it once from the slot-turn
envelopes; if it occurs after, the owner appends the missing output once. Neither
path reparses capture or redispatches a completed slot. For Redact=true, a paired
exact value may remain in process memory across safe result/output/Gate/join/
dispatch fsyncs until every scheduled intra-Formation turn consumer and every
taken-edge consumer has sent once or become durably non-deliverable and no
Gate/retry path retains it; it is then erased, while
cancellation/finality/process loss always discards it. Recovery needing that
discarded projection terminates
`redacted_input_unavailable` instead of reconstructing or substituting data.

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

Canonical cancel starts with an appended and fsynced
`run_cancel_requested` bound to its command id/hash. That
intent snapshots every open node attempt, slot dispatch, and Tool lease, stops new dispatch, and forbids ordinary
replay/rerun even if the coordinator crashes before cancellation completes. The
writer also rejects new `tool_process_launch`, `tool_result`, `node_output`, edge
routing, or other execution authority except cancellation reconciliation and its
final event. Each open-slot snapshot exact-captures capability state, latest
steering generation, an optional open-generation sequence, and any prior
revocation sequence. Cancellation then serializes under target occupancy: it
stops new capability issuance/input, drains every input channel, closes an open
generation with `slot_steering_ended(reason=capability_revoked)`, and fsyncs
`slot_peek_capability_revoked(reason=cancel)` before soft interruption or final
proof validation. Those drain/revocation events are cancellation reconciliation,
not renewed execution authority. For an exact proven target, the writer then
fsyncs the unique `slot_reconciliation_interrupt` bound to this cancel-request
sequence and one `terminal-etx-v1` byte before the certified fence grants its
one coordinator-only attempt. It appends the bounded outcome after the adapter
returns. Recovery never retries a durable request, including when a crash leaves
the outcome `send_uncertain`.
Only the first accepted request appends this event; a repeated cancel, `abort`,
or `stop` alias normalizes before hashing and is idempotent against the same
canceling/final state. It cannot replace any snapshot.
`run_canceled.cancelRequestSeq` names that unique event. Every
snapshotted attempt, including a Gate waiting for human input or evaluating
without an open slot/Tool, receives a canceled/non-authorizing disposition; a
later human decision is rejected.
Active slot work is soft-interrupted without killing tmux sessions only when the
frozen target is proven to host that exact unresolved dispatch/attempt; otherwise
no keystroke is sent. Each dispatch receives a non-authorizing cancellation
disposition with the exact interrupt request/outcome or crash-derived
`send_uncertain`; none is release proof. A
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

Every terminal failure first fsyncs one
`run_failure_reconciliation_started` with the exact cause/header and complete
open attempt/slot/Tool snapshots. That event stops new execution and Peek
authority. Failure reconciliation drains/revokes Peek and uses only an exact
one-shot `slot_reconciliation_interrupt`. Direct failure, and a cancel-origin
slot with no prior request, uses failure authority naming that start event. A
cancel-origin slot with a prior cancel-authorized request reuses that exact
request instead. Both follow the same no-resend/outcome rules as cancellation.
`run_failed` must byte-match the frozen cause/header and exactly dispose those
snapshots. A crash resumes this reconciliation and never chooses a new cause or
sends a second coordinator reconciliation interrupt to a slot.
If cancellation escalates to failure (for example, Tool quiescence misses its
deadline), `originCancelRequestSeq` names that request and the failure snapshot
preserves every prior per-slot interrupt request/outcome. A slot with any durable
cancel-authorized request—including missing outcome/`send_uncertain`—reuses it
and receives no failure-authorized request; only a still-open slot with no prior
request may receive one. The eventual failure disposition carries the preserved
request/outcome and no second coordinator reconciliation interrupt request or
send can occur. User steering remains a separate generation-gated input route.

Every execution-final event revokes authority for every still-open node attempt,
slot dispatch, Tool lease, and run-bound Peek capability. Before accepting one,
the writer enumerates every occupying host target-registry record, non-occupying
release receipt, issued/revoked Peek capability, input channel, and steering
generation for the run. It rejects finality until each capability is absent or
irreversibly revoked, every input channel is drained, every generation is
closed, and each closure/final-quiescence proof exact-matches the latest closed
generation and revocation sequence. Each result-closed dispatch—one whose exact
`slot_result` is already
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
independent but do not own a pane concurrently. Before target acquisition, the
engine allocates `dispatchId` and unique `targetLeaseId`, materializes every
artifact-backed ordinary or judge Formation input through one
authorized-root-relative no-follow open, validates regular identity, media
type, size, and SHA-256 on that handle, and composes/hashes the exact in-memory
prompt bytes from that same handle without reopening. It never gives a mutable
path as input authority. Non-redacted mismatch appends node-scoped
`error(code=formation_input_integrity_failed)` and terminal failure with that
error as `failureCause`; unavailable Redact=true bytes fail terminally with
`redacted_input_unavailable`. Both send nothing and acquire no target.

Only after exact input validation does it arm the certified monitor, atomically
acquire, and fsync exactly one private
record `{targetKey, targetLeaseId, sessionTargetId, runId, dispatchId, bindingId, nodeId,
attempt, slotId, targetFingerprint, attachmentAuditRegistrationSha256,
interactionLatch=none, monitorEvidenceSha256}` referencing the separately
fsynced exact registration and anchored first `fence_transition` journal
record. The certified client/input monitor registration and that non-empty
chain genesis are armed before this occupancy
fsync, so no post-occupancy interaction gap exists. Under that acquired lease and immediately
before durable ledger dispatch/send, it exact-rechecks the frozen persona-card hash,
harness/foreground-process start identity, cwd/root, and pane incarnation. Any
drift appends a stale binding observation, releases the unsent orphan lease, and
sends nothing. Every failure after fence preparation but before ledger dispatch
atomically retires that exact unsent fence/permit with the occupancy release and
fsyncs the registry parent; no input authority survives. Under the same critical section the certified monitor drains its
journal, rejects any foreign/unknown event, and installs the exact
`target-dispatch-input-barrier-v1` fence permitting only one send of the already
hashed prompt. It then obtains a fresh one-shot `target-ready-proof-v1` bound to
that barrier; active or unknown readiness releases the unsent orphan and sends
nothing. It captures the exact `tmux-pane-history-baseline-v1` token
and hash for the unchanged target. Prior accumulated history remains agent
context, but result capture is anchored at the exact epoch/offset and cannot
parse old history as this dispatch's sentinel or output. Lost/trimmed/reset or
resize-invalidated continuity fails closed as specified above. Only an exact
match appends/fsyncs `slot_dispatch` with the barrier, ready proof, fingerprint,
and baseline before the adapter atomically consumes the one exact send permit.
The fence rejects raw client/control input through the send linearization point;
after the permit is consumed it admits only steering-generation-gated Peek
input, capability-issued attach/detach/select metadata, or the unique
reconciliation-interrupt permit. A
crash after private acquisition but before durable ledger dispatch is safe because no
prompt could have been sent; recovery exact-matches and atomically retires the
prepared fence plus orphan occupancy. Missing/conflicting fence identity enters
quarantine rather than dropping either state. A release
never deletes its only proof before ledger finality. It atomically replaces the
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
`result_committed` release receipt with certified turn-closure proof and closed
barrier-held or post-result-pane-gone `releaseProof` before any execution-final event. If the
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
only after capability issuance is stopped, every input channel is drained, no
steering generation is open, and the unique irreversible
`slot_peek_capability_revoked` is durable. Its capture starts at the exact
dispatch baseline, the sentinel is terminal, and a certified harness adapter
proves return to a closed/ready turn for that fingerprint, baseline hash, latest
closed steering generation, and revocation sequence. A certified attachment
monitor must also prove continuous accounting of every client/input event from
occupancy through the terminal capture boundary, with every event classified to
one exact authorized route: barrier-bound workflow prompt, capability-issued
attach/detach/select metadata, steering-generation-gated Peek input, or the
durable one-shot reconciliation interrupt. Under the same target critical section it
then installs the certified interaction-closure barrier, drains/linearizes the
  monitor journal, and synchronously rejects every later unregistered attach,
  select, resize/reflow, history, pane-lifecycle/topology/other mutation, or
  input route at the adapter boundary. The closed
proof binds that barrier. The closed proof comes from the certified
adapter channel and cannot be fabricated solely by typed/echoed pane bytes;
silence or generic prompt text is insufficient. Result validation and Peek input
serialize under the target occupancy: later accepted input opens a newer
generation and invalidates the proof, or input is rejected after capability
revocation. A foreign client/input route (including one that disappears before
validation) or lost attachment-monitor continuity records the stable slot error, revokes input,
and permanently forbids `slot_result` for that dispatch. The writer rechecks the
foreign/audit latch at the barrier linearization point before appending the
result. After that result is
fsynced, the registry lease may be atomically replaced by a
`releaseKind=result_committed` receipt naming the exact result sequence and
carrying that same turn-closure proof and exact durable attachment-audit proof plus
`releaseProof={proofKind=closure_barrier_held,closureBarrierSha256}`. The input
fence remains installed until the receipt fsyncs; that fsync is the target's
release linearization point. The result is not consumable by `formation_result`,
downstream routing, or finality until that receipt is durable. Success
is forbidden until that transition is durable. If cancellation or failure is
requested between result fsync and receipt creation, recovery completes that
exact transition. If the barrier/fence loses continuity or private identity is
missing/conflicting after result fsync, the immutable result remains
result-closed but uncommitted; the arbiter creates the result-closed quarantine
and permits no consumption or finality. It may replace that quarantine with the
exact `result_committed` receipt only if the original barrier is proven to have
held through receipt fsync or exact post-result proof establishes that the old
pane incarnation is gone. A post-result foreign event never reparses or changes
the result. The result-closed dispatch is not open and receives
no final slot disposition. Cancellation/failure may release an unmatched exact
active record only after Peek capability revocation is durable and the old
dispatch is proven quiescent. A correct-target interrupt alone is not proof. The
closed admissible proof is either the exact closed
`TargetFinalQuiescenceProof.proofKind=dispatch_cancel_ack`, binding the unique
interrupt request, revocation/generation, certified cancel acknowledgement,
harness-ready evidence, terminal capture boundary, and continuous interaction
audit under a closure fence held through receipt fsync, or private proof that
the exact pane incarnation/fingerprint is gone. A replacement pane gets a new
key/fingerprint and is never interrupted or released as the old target. Before
finality, the engine either atomically replaces
that record with a `releaseKind=final_quiescent` receipt and reports
`targetLeaseState=released_quiescent`, or durably marks it as a
non-authorizing terminal hold and reports
`targetLeaseState=terminal_hold`; a missing/conflicting record instead reports
the already-durable `targetLeaseState=quarantined`. Holds and quarantines remain
busy, non-interactive, and without run-bound capability until later
non-authorizing reconciliation proves quiescence. A foreign-attachment latch
linearized before `slot_result` cannot take the result path; it enters
hold/quarantine and later releases only after exact pane-incarnation-gone proof,
without promoting output. A post-result barrier fault follows
the result-closed quarantine rule above. A crash after
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

If atomic acquisition finds a lease, attachment, certified open turn, unknown
readiness, or stale fingerprint, no `slot_dispatch` or prompt is produced.
Preflight resolves unavailable with the exact stable reason
`session_target_leased`, `session_target_attached`,
`session_target_harness_busy`, `session_target_readiness_unknown`, or
`session_target_attachment_audit_unavailable`, or `session_target_stale`.
Acquisition repeats those checks with a fresh adapter
challenge. After start, the engine appends `slot_binding_observed` with that
reason, then a node-scoped `error(code=session_target_busy)` for the exact
attempt and terminal
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
   authority only for `U'`; it accepts only cancel/finality and non-authorizing
   inspection observations. The old run remains blocked until canceled. A
   separately started run is independent and neither closes nor supersedes any
   old lease; replacement-target rebinding and unresolved-lease redispatch are
   never same-run operations.
6. For `Redact=true`, if continuation requires a raw value that was
   intentionally not persisted, fsync
   `run_failure_reconciliation_started(originCancelRequestSeq=0)` with the exact
   header/cause and complete open-resource snapshots, then append `run_failed`
   naming that start with
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

- `run_started` initializes status `queued`. Unique valid fenced `run_activated`
  projects `running` and is required before graph/dispatch events. Queued cancel
  uses `run_cancel_requested` then `run_canceled`; queued failure uses
  `run_failure_reconciliation_started` then `run_failed`. Neither path requires
  activation.
- `run_blocked` projects status `blocked` for its epoch and stops automatic
  dispatch. Explicit resume may append a higher-epoch event only when
  `resumeAllowed=true`, in which case `nextEpoch` is required;
  `resumePolicy=new_run_required` omits `nextEpoch`, remains blocked, and rejects
  resume. When such a block retains non-empty `openDispatches`, it also revokes
  those leases' result/output/routing authority; only cancel/finality and
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
  `run_failure_reconciliation_started` exact-naming that cancel request,
  followed only by its matching terminal `run_failed`.
- `run_failure_reconciliation_started` projects non-final status `failing`,
  including when it escalates a prior cancel. It rejects ordinary execution and
  admits only the exact frozen failure reconciliation toward `run_failed`; a
  crash resumes those same snapshots and cannot return the run to `canceling`,
  `running`, `blocked`, or `waiting_human`.
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
- `slot_result` marks that slot complete, failed, or needs review and supplies
  its immutable parsed turn envelope for deterministic Formation continuation.
- `formation_result` closes ordinary Formation aggregation and supplies the
  exact status/output/artifact result from which a missing `node_output` is
  materialized without capture reparse or redispatch.
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
  only and is never accepted as a new append or as routing/revision authority.

## API Surface

The API is the shared UI/CLI contract. Board and layout write endpoints use
`If-Match` with the relevant ETag/revision. The retired inline-verification
removal patch is exactly
`removeVerification: {formationId, replacementGateId}`. The named Gate must
already exist on the current board and receive an existing connection from a
named output of that Formation; rejection changes neither board nor layout.
Persona-card edits use the shared
writer's stale-read conflict detection. Write conflicts return `409`.
Every runtime mutation requires a client-stable `commandId`; its canonical
request hash is journaled privately, and the response is the durable applied or
rejected receipt. The same id/hash returns that receipt, while the same id with
another hash fails `command_id_conflict`. Runtime reads and mutations fail loud
when this server is not the current workspace coordinator; no client reads the
private ledger directly.

| Route | Purpose |
|---|---|
| `GET /api/agents` | Persona roster left-joined with live Oracle sessions. |
| `GET /api/agents/{agentId}` | Deep persona inspection with pointers, not inlined sources. |
| `POST /api/agents` | Create a persona card through the shared writer. |
| `PATCH /api/agents/{agentId}` | Edit modeled card fields while preserving unknown fields. |
| `GET /api/formations/boards` | List boards by id, slug, title, and rev. |
| `GET /api/formations/boards/{boardIdOrSlug}` | Read board definition plus ETag. |
| `PATCH /api/formations/boards/{boardId}` | Field/id-addressed board mutations, including replacement-Gate-bound legacy verification removal. |
| `GET /api/formations/boards/{boardId}/layout` | Read layout sidecar plus ETag. |
| `PATCH /api/formations/boards/{boardId}/layout` | Layout-only mutations by stable ids. |
| `POST /api/formations/runs` | Journal a `start` command, preflight, fsync private authority and `run_started`, then return the durable receipt/run id without waiting for graph execution; full-queue rejection is durable before run creation. |
| `GET /api/formations/runs` | List runs from the coordinator's bounded sanitized projection, including durable queued status/order. |
| `GET /api/formations/runs/{runId}` | Project current status plus hash-linked safe binding/artifact projections from private authority; no raw route or private path. |
| `GET /api/formations/runs/{runId}/events` | Bounded sanitized event projection; never raw ledger bytes/private paths, and every artifact id is hydrated through latest authorization. |
| `GET /api/formations/runs/{runId}/stream?since=<seq>` | Sanitized SSE projection with `Last-Event-ID`; revoked artifact refs and private authority never replay. |
| `POST /api/formations/runs/{runId}/cancel` | Journal canonical cancel, append/fsync cancellation intent, stop dispatch, soft-interrupt exact slots, reconcile target/Tool leases, and finish with `run_canceled` or fail closed with `run_failed(code=tool_process_not_quiescent)`; never kills tmux sessions. |
| `POST /api/formations/runs/{runId}/abort` | Compatibility alias for `/cancel`; normalize before command hashing and never create a second cancel snapshot or state. |
| `POST /api/formations/runs/{runId}/resume` | Journal resume for the exact blocked sequence, then resume one `retry_failed_producer` or bounded `reattach_only` path; unmatched leases are never redispatched. |
| `POST /api/formations/runs/{runId}/gates/{gateId}/verdict` | Journal one verdict exact-matching the outstanding human-request sequence. |

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
- Gate-level `--onfail`; S0 used fail wiring and kept `onFail` only on retired
  inline verification. ADR-0008 requires a named, already-wired replacement
  Gate and never infers a route from that legacy field.
- General-purpose error-routing ports; the first mixed-workflow contract stops
  `unavailable` and `error` outcomes loudly.
- A second reusable formation-definition registry; embedded nodes and explicit
  copying remain the first contract.
- Broad undo matrices for every S3/S4 mutation beyond the structural cases in
  `canvas.feature`.
