# CHROTE Formations - Active Contracts

This file is the active S0 implementation contract. The archived packets are
background only. Where this document conflicts with archived `fm`,
`.chrote/orchestration`, dot-separated event names, `judgeFormationId`, mock
verdicts, or prototype counter ids, this document wins with
[DECISIONS-LOCKED.md](../DECISIONS-LOCKED.md).

`03-formations.js` remains a behavior and visual reference for the canvas. Its
`s1`/`f1`/`g1` counters, in-memory connection shape, stored gate verdicts, and
canned terminals are not persistence or run contracts.

## Files

```text
~/agents/<id>.toml
<workspace>/.formations/boards/<slug>.formation.toml
<workspace>/.formations/layout/<slug>.layout.toml
<workspace>/.formations/runs/<slug>/<run-id>.ndjson
<workspace>/.formations/runs/<slug>/<run-id>.snapshot.toml
<workspace>/.formations/runs/<slug>/<run-id>.bindings.toml
<workspace>/.formations/runs/<slug>/<run-id>.refs/
<workspace>/.formations/runs/<slug>/latest.json
```

The shared formations package is the only writer for persona cards, board, and
layout files. The UI, `archon`, and agents all call that writer. Runs append
their own NDJSON ledger, write immutable run snapshots and payload refs under the
same slug run folder, and atomically rewrite only regenerable run caches.

## Identity and Addressing

Stable ids are real addresses:

```text
brd_...  board
mis_...  mission
fmn_...  formation
slot_... slot
gate_... gate
ver_...  verification
port_... dynamic formation port
edge_... connection
run_...  run
```

Board slugs, node titles, and `in[N]` / `out[N]` are aliases for examples,
tests, and human display. A writer resolves aliases once to stable ids before a
mutation. Persisted references use `<nodeId>:<portId>` and never `in[0]` or
`out[0]`.

Formation input ports accept one incoming edge. JOIN is represented by adding
input ports and wiring one upstream edge to each. A second edge to the same
formation input is a write conflict, not an implicit multi-edge JOIN.

Slot identity is:

```text
slot.agentId       required persona card id
slot.harness       optional harness variant id
```

The slot never stores a tmux session name. At run time, `agentId` plus the
optional harness resolves through the persona card's harness variant and
`session_stem`. If the selected variant is missing, matches multiple sessions,
or has no live session, dispatch records a loud error. Implicit Oracle prefix
stripping such as deriving `susie` from `claude-susie` without a card-declared
variant is deferred until Perttu explicitly chooses that rule.

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
schema   = 1
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
id    = "port_01J9_research_in"
label = "Input"

[[formation.output]]
id    = "port_01J9_research_out"
label = "Output"

[[formation.slot]]
id         = "slot_01J9_peer_a"
label      = "Peer A"
agentId    = "susie"
harness    = "claude-code"
controller = false

[formation.verification]
id        = "ver_01J9_research"
kinds     = ["code"]
criterion = "Both reads converge on a recommendation."
onFail    = "block" # block | pushback

[[gate]]
id        = "gate_01J9_review"
title     = "Review gate"
kinds     = ["human", "code"]
criterion = "Research is sound and safe to build."

[gate.code]
command = "go test ./..."
cwd     = "src"

[[connection]]
id   = "edge_01J9_start"
from = "mis_01J9_improve:out"
to   = "fmn_01J9_research:port_01J9_research_in"

[[connection]]
id   = "edge_01J9_gate"
from = "fmn_01J9_research:port_01J9_research_out"
to   = "gate_01J9_review:in"
```

Missions have fixed port `out`. Gates have fixed ports `in`, `pass`, `fail`,
and `judge`. Formation ports are explicit arrays with stable `port_...` ids.

Gate definitions do not store run verdicts, default verdicts, or gate-level
`onFail`. Fail behavior is determined by wiring: an unwired `fail` port blocks;
a fail edge routes down that wire, including explicit pushback loops. In-formation
verification keeps `onFail` because it has no pass/fail output ports.

Judge chains are normal connections through the `judge` socket:

```toml
[[connection]]
id = "edge_01J9_judge_send"
from = "gate_01J9_review:judge"
to = "fmn_01J9_judge_a:port_01J9_judge_a_in"

[[connection]]
id = "edge_01J9_judge_mid"
from = "fmn_01J9_judge_a:port_01J9_judge_a_out"
to = "fmn_01J9_judge_b:port_01J9_judge_b_in"

[[connection]]
id = "edge_01J9_judge_return"
from = "fmn_01J9_judge_b:port_01J9_judge_b_out"
to = "gate_01J9_review:judge"
```

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

## Run Ledger Envelope

Each run is append-only NDJSON. Every line is one JSON object with this minimum
envelope:

```json
{
  "ts": "2026-06-03T16:05:00Z",
  "runId": "run_01J9...",
  "seq": 1,
  "type": "run_started",
  "actor": "agent:archon",
  "boardId": "brd_01J9_sesssearch",
  "boardRev": 7,
  "missionId": "mis_01J9_improve",
  "beadId": "bd-204",
  "snapshot": ".formations/runs/session-search/run_01J9.snapshot.toml",
  "bindingsSnapshot": ".formations/runs/session-search/run_01J9.bindings.toml",
  "epoch": 0,
  "attempt": 0,
  "data": {}
}
```

All events include `ts`, `runId`, monotonic `seq`, `type`, and `actor`. Events
that touch graph objects include the relevant `boardId`, `boardRev`, `missionId`,
`nodeId`, `slotId`, `gateId`, `edgeId`, `attempt`, or `epoch`. `run_started`
is the first event and must include the board id, board revision or snapshot
path, mission id, and run id.

Run state is epoch-aware:

```text
run_started seq=1, epoch=0
run_blocked closes an epoch and may be resumed explicitly
run_resumed opens the next epoch after an operator resume
run_succeeded, run_failed, and run_canceled are final for the whole run
```

`seq` is monotonic for the whole run and never resets. `run_started` appears
exactly once at `seq=1`. `epoch` starts at `0` and increments only when an
operator explicitly resumes a blocked run. A `run_blocked` event stops dispatch
for that epoch. `archon run resume <runId>` or the equivalent API action is
accepted only when the latest projection is blocked and `resumeAllowed=true`;
it appends `run_resumed` as the first event in `epoch + 1` with
`data.resumedFromSeq` set to the `run_blocked` sequence. The engine must not
append after `run_succeeded`, `run_failed`, or `run_canceled`.

Ledger replay is idempotent. A `slot_dispatch` is recorded before the prompt is
sent. If the recovery reconciler sees a dispatch without a `slot_result`, it
re-attaches capture or records a loud `error`; it never blindly re-delivers the
prompt. See `docs/adr/0001-formations-run-recovery-contract.md`.

## Run Snapshot And Revision Rules

Run start reads the current board definition, validates the selected mission,
resolves every staffed slot to a persona card and harness variant, and writes
immutable board and binding snapshots before appending `run_started`. The board
snapshot is copied to `<run-id>.snapshot.toml` under the slug run folder and is
the graph the engine executes. The binding snapshot is copied to
`<run-id>.bindings.toml` and records the resolved `agentId`, harness variant,
`sessionStem`, card path, card hash, and launch/source pointers needed for this
run. It does not inline source files, prompts, skill bodies, or secrets.

`run_started` records the source board path, board id, board rev, board snapshot
path, binding snapshot path, and mission id. If a caller supplies an expected
board rev or ETag and it does not match the current board, run start fails with
the same conflict semantics as a board write and appends no event. Resume replays
the board and binding snapshots and never revalidates or rewrites current
board/layout/persona files. Board, layout, and persona edits after `run_started`
apply only to future runs. Runs may atomically rewrite regenerable run caches
such as `latest.json`, but they never write persona cards, board definitions, or
layout definitions.

## Event Payload Schemas

Each event uses the envelope above. Fields listed here are inside `data` unless
the envelope already names the field. Unknown `data` keys are allowed for
forward-compatible readers, but the required keys below must be present.

| Event type | Required payload |
|---|---|
| `run_started` | `boardSlug`, `boardPath`, `boardRev`, `snapshot`, `bindingsSnapshot`, `missionId`, `beadId`, `objective`, `limits` (`maxDispatch`, `wallClockSeconds`, `redact`) |
| `run_resumed` | `resumedFromSeq`, `resumedBy`, `resumeMode` (`reattach`, `redispatch`, `operator-input`), `reason`, `openDispatches` |
| `node_waiting` | `nodeId`, `neededInputs`, `readyInputs`, `totalInputs`, `waitingFor` (`edgeId` or `portId` list) |
| `node_started` | `nodeId`, `nodeKind` (`mission`, `formation`, `gate`), `attempt`, `inputRefs`, `reason` (`initial`, `resume`, `pushback`, `judge`) |
| `slot_dispatch` | `dispatchId`, `nodeId`, `slotId`, `agentId`, `harness`, `sessionStem`, `sessionRef`, `promptSha256`, `promptRef`, `nativeAck`, `recordedBeforeSend=true` |
| `slot_result` | `dispatchId`, `nodeId`, `slotId`, `agentId`, `status` (`ok`, `error`, `timeout`, `needs-review`), `capturedRange`, `textRef`, `artifacts`, `sentinel` |
| `node_output` | `nodeId`, `status` (`done`, `needs-review`, `blocked`), `reportRef`, `artifacts`, `diffs`, `producedBy`, `timing`, `deliveredEdges` |
| `gate_evaluating` | `gateId`, `nodeId`, `kinds`, `criterion`, `inputRef`, `judgeChain` |
| `gate_verdict` | `gateId`, `verdict` (`pass`, `fail`), `perKind`, `routePort` (`pass`, `fail`, `none`), `routedEdges`, `reason` |
| `verification_verdict` | `verificationId`, `nodeId`, `verdict` (`pass`, `fail`), `kinds`, `criterion`, `onFail`, `feedback` |
| `artifact_attached` | `nodeId`, `slotId`, `name`, `type`, `ref`, `sha256` |
| `escalation_raised` | `trigger`, `severity` (`info`, `needs-attention`, `stop`), `reason`, `source` (`system`, `agent`, `human`), `nodeId`, `gateId`, `blocks` |
| `human_input_requested` | `gateId`, `nodeId`, `prompt`, `choices`, `requestedBy`, `timeoutSeconds` |
| `human_verdict_recorded` | `gateId`, `nodeId`, `verdict` (`pass`, `fail`), `reason`, `requestedSeq`, `decidedBy` |
| `error` | `code`, `message`, `boundary` (`engine`, `writer`, `adapter`, `tmux`, `schema`, `operator`), `nodeId`, `slotId`, `recoverable`, `relatedSeq` |
| `run_blocked` | `reason`, `blockedNodeId`, `blockedGateId`, `resumeAllowed`, `resumePolicy`, `openDispatches`, `nextEpoch` |
| `run_canceled` | `reason`, `requestedBy`, `softInterruptedSlots`, `final=true` |
| `run_failed` | `code`, `reason`, `unrecoverable`, `relatedSeq`, `final=true` |
| `run_succeeded` | `summaryRef`, `outputRefs`, `artifactRefs`, `final=true` |

`promptRef`, `textRef`, `reportRef`, and `summaryRef` may point to files under
`<run-id>.refs/` when payloads are large. If `redact` is true, durable payloads
may keep a hash, but a ref may remain only after its target is sanitized or
replaced inside an authorized root. A ref to unsanitized prompt, reply, report,
or artifact content is invalid even when the ledger omits the inline text. The
run-private pending-redaction registry is the only temporary exception: its
cleanup locator is written and fsynced before raw bytes reach the target and is
never exposed as an output ref or graph input.

Structured payload fields use these shapes:

- `inputRefs`: array of `{edgeId, fromNodeId, outputSeq, ref}`.
- `capturedRange`: `{sessionRef, start, end, startedAt, endedAt}` where `start`
  and `end` are adapter-defined capture cursors, not executable text.
- `artifacts` and `artifactRefs`: array of `{name, type, ref, sha256}`.
- `diffs`: array of `{path, ref, sha256}`.
- `timing`: `{startedAt, finishedAt, durationMs}`.
- `deliveredEdges`: array of `{edgeId, toNodeId, toPortId, ref}`.
- `openDispatches`: array of `{dispatchId, nodeId, slotId, agentId, sessionRef,
  dispatchSeq}`.

## Dispatcher Lease And Idempotency

`slot_dispatch.dispatchId` is the durable lease for one attempted delivery to
one slot. It is unique within a run and includes the run id, epoch, node id, slot
id, and attempt in its generated value or payload. The engine must append
`slot_dispatch` and fsync the ledger before calling the adapter boundary that can
send text to tmux. For tmux adapters, `nativeAck` is always `false`; a future
adapter may set a native acknowledgement token, but it never replaces
ledger-before-send ordering.

Replay handles an open dispatch lease as follows:

1. If a matching `slot_result` exists, the slot is complete and is never
   redispatched.
2. If the recorded qualified `sessionRef` is live and the reconciler proves it
   still identifies the same unresolved dispatch and attempt, the engine
   reattaches capture and waits for that attempt's matching sentinel or
   idle-timeout result.
3. If capture proves the agent finished while the engine was down, append
   `slot_result` from captured output.
4. If the session is missing, dead, ambiguous, or capture cannot be reattached,
   append `error` and determine whether continuation requires a discarded
   execution-authoritative value.
5. For an ordinary recoverable boundary, append `run_blocked`. An explicit
   operator resume may append `run_resumed`, advance the epoch, and create a new
   `slot_dispatch`.
6. For `Redact=true`, if continuation requires a raw value that was
   intentionally not persisted, append `run_failed` with
   `code=redacted_input_unavailable`,
   `reason=redacted_input_unavailable`, `unrecoverable=true`, `relatedSeq` set to
   the exact source event whose authoritative value was required, and
   `final=true`. Do not append `run_blocked`, open another epoch, or dispatch a
   marker, hash, summary, pending cleanup target, capture, report, or artifact
   as input.
7. Never send the original prompt a second time except through a valid explicit
   resume of a genuinely blocked, resumable run.

Automatic replay or process reconnect does not advance the epoch. It may append
`slot_result` for a proven completed dispatch. When recovery cannot be proven,
it appends `error` followed by either resumable `run_blocked` or the terminal
redacted-input failure above. Explicit resume from a blocked run always advances
the epoch, even when the first resumed action is reattaching capture rather than
redispatching; a final redacted-input failure rejects resume.

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
  dispatch until explicit resume appends a higher-epoch event.
- `run_resumed` projects status `running` for the new epoch and records the
  blocked sequence being resumed.
- `run_succeeded`, `run_failed`, and `run_canceled` project final statuses and
  reject further appends.
- An open `slot_dispatch` without a result projects `running` with open
  dispatches from the ledger. If recovery cannot reattach or prove completion,
  the reconciler appends `error` and then either resumable `run_blocked` or
  terminal `run_failed` with `code=redacted_input_unavailable` when a redacted
  run cannot recover required raw input.

Node projection:

- `node_waiting` projects waiting readiness counts.
- `node_started` projects `running` and the current `attempt`.
- `slot_dispatch` marks a slot in flight.
- `slot_result` marks that slot complete, failed, or needs review.
- `node_output` projects `done`, `needs-review`, or `blocked` and delivers the
  listed output edges.
- `gate_evaluating`, `gate_verdict`, `verification_verdict`,
  `human_input_requested`, and `human_verdict_recorded` project gate or
  verification state from the latest event for that object in the current epoch.

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
| `POST /api/formations/runs` | Start a run from mission/bead selector; returns `runId`. |
| `GET /api/formations/runs/{runId}` | Project current status from the ledger. |
| `GET /api/formations/runs/{runId}/events` | Bounded ledger read. |
| `GET /api/formations/runs/{runId}/stream?since=<seq>` | SSE replay with `Last-Event-ID` support. |
| `POST /api/formations/runs/{runId}/abort` | Append `run_canceled`; never kills tmux sessions. |
| `POST /api/formations/runs/{runId}/resume` | Resume/re-attach from ledger replay. |
| `POST /api/formations/runs/{runId}/gates/{gateId}/verdict` | Human verdict for one waiting gate instance. |

Run operations target `runId` once a run exists. Mission or bead selectors are
allowed only when they resolve to exactly one active run; otherwise the command
or API call fails loud.

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
- Gate-level `--onfail`; S0 uses fail wiring and keeps `onFail` only on
  verification.
- Multiple edges to one formation input port; S0 uses one edge per input port.
- Interactive keystroke forwarding inside canvas terminal popups.
- Broad undo matrices for every S3/S4 mutation beyond the structural cases in
  `canvas.feature`.
