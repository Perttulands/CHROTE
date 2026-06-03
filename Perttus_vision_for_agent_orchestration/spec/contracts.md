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
<workspace>/.formations/runs/<slug>/latest.json
```

The shared formations package is the only writer for persona cards, board, and
layout files. The UI, `archon`, and agents all call that writer. Runs append
their own NDJSON ledger and atomically rewrite only regenerable run caches.

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

Terminal run events are:

```text
run_succeeded
run_failed
run_blocked
run_canceled
```

Replay is idempotent. A `slot_dispatch` is recorded before the prompt is sent.
If replay sees a dispatch without a `slot_result`, it re-attaches capture or
records a loud `error`; it never blindly re-delivers the prompt.

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
- Full S4 ledger payload schemas beyond the envelope and terminal events above.
- Interactive keystroke forwarding inside canvas terminal popups.
- Broad undo matrices for every S3/S4 mutation beyond the structural cases in
  `canvas.feature`.
