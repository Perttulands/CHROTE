# Data Model Comparison

> Side-by-side comparison of the two proposed on-disk formats. Both agree on the primitives (mission, formation, slot, gate, connection, run). They disagree on serialization, layout, and identity binding.

---

## The primitives (agreed)

Both packets use the same conceptual model. A **mission** is the entry point. It contains **nodes** (formations and gates) wired by **connections** (directed, port-addressed). **Slots** within formations hold references to **agent profiles**. **Runs** are append-only event logs produced from a mission snapshot.

| Primitive | Role |
|---|---|
| **Mission** | Entry point and durable definition. One per graph. |
| **Formation** | A node that coordinates work. Types: solo, peer, flow, orchestrated. |
| **Slot** | A role within a formation, referencing an agent profile. |
| **Gate** | A checkpoint between formations. Verdict belongs to the run, not the definition. |
| **Verification** | A local checkpoint inside a formation, run after the formation's work finishes. |
| **Connection** | A directed edge between ports (`nodeId:portId`). |
| **Run** | An append-only event log produced from an immutable mission snapshot. |

---

## Format choice: JSON vs TOML

### Codex: JSON

```json
{
  "schema_version": 1,
  "id": "mission_01J9XF...",
  "title": "Improve session search",
  "objective": "Make session search fuzzy and keyboard-first",
  "workspace": "/workspace/chrote",
  "status": "draft",
  "nodes": [
    {
      "id": "formation_01J9XF...",
      "type": "peer",
      "title": "Research huddle",
      "brief": { "goal": "", "beadId": "bd-204", "files": [...] },
      "inputs": [{ "id": "in", "label": "Input" }],
      "outputs": [{ "id": "out", "label": "Output" }],
      "slots": [
        { "id": "slot_a", "label": "Peer", "agent_profile_id": "codex", "controller": false },
        { "id": "slot_b", "label": "Peer", "agent_profile_id": "claude-code", "controller": false }
      ],
      "verification": { "id": "ver_x", "kinds": ["code"], "criterion": "...", "on_fail": "block" }
    },
    {
      "id": "gate_01J9XF...",
      "title": "Review gate",
      "kinds": ["human", "code"],
      "criterion": "...",
      "on_fail": "block",
      "pass_port_id": "pass",
      "fail_port_id": "fail"
    }
  ],
  "connections": [
    { "id": "edge_1", "from": "mission_01J9XF...:out", "to": "formation_01J9XF...:in" },
    { "id": "edge_2", "from": "formation_01J9XF...:out", "to": "gate_01J9XF...:in" }
  ]
}
```

**Pros:**
- Go-native structs with `json` tags — zero new parser dependencies.
- Machine edits are exact; no ambiguity about whitespace or key ordering.
- Agents generating JSON is reliable (LLMs handle JSON better than YAML).

**Cons:**
- Diffs are noisy. Adding one slot changes line numbers and brackets.
- Human review of JSON node graphs is painful.
- No comments. Agent-authored notes or reasoning are hard to preserve inline.

### Claude: TOML

```toml
schema    = 1
id        = "brd_01J9_sesssearch"
slug      = "session-search"
title     = "Improve session search"
kind      = "board"
updatedBy = "agent:scout"
updatedAt = "2026-06-02T10:14:00Z"

[[mission]]
  id = "mis_01J9_improve"; title = "Improve session search"
  goal = "Make session search fuzzy and keyboard-first"

[[formation]]
  id = "fmn_01J9_research"; type = "peer"
  title = "Research huddle"
  [formation.brief]
    goal = ""; beadId = "bd-204"; files = ["src/SessionPanel.tsx"]
  [[formation.input]]  ; id = "port_in"  ; label = "Input"
  [[formation.output]] ; id = "port_out" ; label = "Output"
  [[formation.slot]]   ; id = "slot_a" ; label = "Peer" ; agentId = "codex"       ; controller = false
  [[formation.slot]]   ; id = "slot_b" ; label = "Peer" ; agentId = "claude-code" ; controller = false
  [formation.verification]
    id = "ver_x"; kinds = ["code"]
    criterion = "both reads converge on a recommendation"
    onFail = "block"

[[gate]]
  id = "gate_01J9_review"; kinds = ["human", "code"]
  criterion = "research is sound and the plan is safe to build"
  onFail = "block"
  [gate.code]
    command = "go test ./... && npm run build"; cwd = "src"

[[connection]] ; id = "edge_1" ; from = "mis_01J9_improve:out"       ; to = "fmn_01J9_research:port_in"
[[connection]] ; id = "edge_2" ; from = "fmn_01J9_research:port_out" ; to = "gate_01J9_review:in"
```

**Pros:**
- Single-line diffs for renames, slot additions, and wire changes.
- Agents already author TOML (Gas Town formulas).
- No indentation traps (unlike YAML).
- CHROTE already transcodes TOML→JSON for `bd --json`; same move at API boundary.
- Inline comments possible (e.g., agent reasoning about why a slot was assigned).

**Cons:**
- Go needs a TOML parser (`BurntSushi/toml` or `pelletier/go-toml`).
- Array-of-tables syntax (`[[...]]`) is unfamiliar to some.
- Deterministic re-serialization requires care (key ordering, inline vs block tables).

### Verdict

TOML wins if the primary authors are agents and humans reviewing diffs. JSON wins if the primary authors are machines and exactness matters most. Given that the vision says **agents author and humans review**, TOML is the better fit. The transcoding concern is already solved by `bd` precedent.

---

## File layout: `.chrote/orchestration/` vs `.formations/` + `~/agents/`

### Codex layout

```
<workspace-root>/
└── .chrote/
    └── orchestration/
        ├── agents/
        │   ├── archon.json
        │   ├── designer-lead.json
        │   └── ...
        ├── missions/
        │   └── mission_<id>.json
        ├── runs/
        │   └── run_<id>/
        │       ├── mission.snapshot.json
        │       ├── events.jsonl
        │       └── artifacts/
        └── notices/
            └── mission_<id>.jsonl
```

### Claude layout

```
~/agents/
├── archon.toml
├── designer-lead.toml
└── ...

<workspace-root>/
├── .beads/
└── .formations/
    ├── boards/
    │   └── session-search.formation.toml
    ├── layout/
    │   └── session-search.layout.toml
    ├── runs/
    │   └── session-search/
    │       ├── run-01J9….ndjson
    │       └── latest.json
    └── board.ndjson
```

### Comparison

| | Codex | Claude |
|---|---|---|
| **Agent profiles** | Colocated with missions (`.chrote/orchestration/agents/`) | Central (`~/agents/`) |
| **Mission definitions** | `missions/*.json` | `boards/*.formation.toml` |
| **Layout** | Inline in mission JSON | `layout/*.layout.toml` sidecar |
| **Run state** | `runs/<id>/` | `runs/<board>/` |
| **Notice board** | `notices/*.jsonl` | `board.ndjson` |

**Why the layout matters:**

- **Agent profile location:** If Susie (a design lead) works on three projects, her persona card should live once, not in three `.chrote/orchestration/agents/` directories. Central `~/agents/` avoids drift. The Codex packet does not address multi-project identity.
- **Layout sidecar:** Separating x/y canvas positions from structure means a human dragging nodes never produces a git diff to the mission definition. The Codex packet inlines layout, which would dirty the mission file on every UI interaction.
- **`.formations/` as `.beads/` sibling:** CHROTE already discovers and validates `.beads/` under workspace roots. Adding `.formations/` uses the same discovery logic. `.chrote/orchestration/` is a new convention.

### Verdict

The Claude layout is more principled: split concerns (definition / layout / run state), centralize identity, and mirror existing conventions. The Codex layout is simpler to discover but conflates concerns and duplicates identity.

---

## Identity: how IDs work

### Codex IDs

- `mission_<ulid>`, `formation_<ulid>`, `slot_<ulid>`, `gate_<ulid>`, `connection_<ulid>`
- Port addressing: `node_id:port_id` where `port_id` is a string like `"in"`, `"out"`, `"pass"`, `"fail"`

### Claude IDs

- Prefixed ULIDs: `brd_`, `mis_`, `fmn_`, `slot_`, `gate_`, `ver_`, `port_`, `edge_`
- Port addressing: `nodeId:portId` with stable port IDs (`port_in`, `port_out`, `in`, `pass`, `fail`, `judge`)
- Every edge carries its own `id` so a rewire mutates one field (clean diff) rather than delete+insert

### Why this matters

The prototype uses ephemeral counters (`s1`, `f1`, `g1`). Both packets reject this. The Claude prefix system (`brd_`, `fmn_`, etc.) is self-describing — you can tell node kind from the ID, which helps debugging and log reading. The Codex flat `_<ulid>` approach requires looking up the node type in the mission file.

The Claude packet also specifies that **port IDs are stable within a node** (e.g., a gate always has `in`, `pass`, `fail`, `judge`), while formation ports are explicit arrays. This makes connection wiring predictable without reading the node definition.

### Verdict

Claude's prefixed ID scheme is better for debugging and predictable wiring. Both use ULID for collision-free concurrent authoring.

---

## Separation of concerns: the critical discipline

Both packets agree on separating definition from run state. The Claude packet adds a third separation: **layout is purely presentational** and lives in its own sidecar.

| Concern | Codex | Claude | Rule |
|---|---|---|---|
| Definition | `missions/*.json` | `boards/*.formation.toml` | Canonical. Agents write. UI reads. |
| Layout | Inline in mission | `layout/*.layout.toml` | Presentation only. Deleting it loses positions, never structure. |
| Run state | `runs/<id>/events.jsonl` | `runs/<board>/*.ndjson` + `latest.json` | Append-only. Produced by runs. Never touches definition. |

The Claude packet's three-way split means:
- A human dragging a node in the canvas produces a diff only in `layout/`.
- A run finishing produces files only in `runs/`.
- The board file only changes when structure changes (agent-authored).

The Codex packet conflates layout with definition (both in the mission JSON), so every UI drag would dirty the mission file. This undermines the "run state never pollutes definition" invariant in practice.

---

## Round-trip and concurrency

Both packets agree on:
- Atomic writes (temp file + rename).
- Optimistic concurrency (etag / revision check).
- Unknown fields survive edits (the file model stays richer than any one client).

The Claude packet adds:
- **Single writer for the format:** `fm` is the only writer of definition files. CHROTE shells out to `fm` for human tweaks, exactly as `beads.go` shells out to `bd`. This eliminates format drift between CLI and UI.
- **External-edit detection:** mtime/fsnotify → SSE `board.changed` → UI reloads. "Live-ish," not a CRDT.
- **Schema versioning:** `schema = N`; refuse newer (fail loud), up-migrate older.

The Codex packet does not specify a single-writer rule, which risks two serializers (Go JSON encoder and an agent's JSON generator) producing different key ordering.

---

## Run ledger event schema

Both packets agree the ledger is append-only NDJSON. The Claude packet specifies the event types more explicitly:

```
run_started
node_waiting
node_started
node_output
gate_evaluating
gate_verdict
verification_verdict
artifact_attached
notice_posted
human_input_requested
human_verdict_recorded
run_blocked
run_canceled
run_failed
run_succeeded
```

The Codex packet lists similar events but does not prescribe the schema as tightly. Both agree: runtime status is **projected from the event log**, not stored in a separate state table.

---

## What the first slice actually needs

Per both packets, the first slice (Phase 0 / Phase 1) only needs:

- A board file with **one formation** (slots referencing persona IDs) + a brief.
- No gates, no verification, no multi-input JOIN, no canvas layout.
- The NDJSON run ledger.
- Stable IDs, schema versioning, atomic writes.
- A round-trip test: hand-author a board with an extra agent-authored key, issue a rename via `fm`, assert the title changed, the extra key survived, and the diff is one line.

The full node menagerie (gates, judges, all four formation types, fan-out, JOIN, layout sidecar) arrives with the canvas in Phase 2.

---

## Summary table

| Decision | Codex | Claude | My read |
|---|---|---|---|
| **Format** | JSON | TOML | TOML for agent authoring + diff cleanliness; transcoding is solved |
| **Layout** | Inline in mission | Sidecar (`layout/`) | Sidecar preserves definition purity |
| **Agent profiles** | Colocated with missions | Central (`~/agents/`) | Central avoids multi-project drift |
| **Profile home** | `.chrote/orchestration/agents/` | `~/agents/` | `~/agents/` mirrors `~/skills/` convention |
| **ID scheme** | Flat `<kind>_<ulid>` | Prefixed (`brd_`, `fmn_`, etc.) | Prefixed is self-describing |
| **Port addressing** | `node_id:port_id` | `nodeId:portId` with stable gate ports | Claude's stable gate ports reduce lookup |
| **Single writer** | Not specified | `fm` owns definition writes | Prevents format drift |
| **Schema version** | `schema_version` field | `schema = N` at top of file | Both equivalent |
| **Run state** | `runs/<id>/` | `runs/<board>/` | Per-board grouping is more discoverable |

---

## The one decision that unlocks everything

**Where does the run engine live?** This is upstream of the data model. If the engine lives in `fm`, the data model must support `fm` reading/writing files and CHROTE reading them. If the engine lives in the CHROTE server, the data model must support server-state snapshots and API-driven mutations.

The data model comparison above assumes the Claude path (`fm` as the writer, files as canonical). If you choose the Codex path (server-hosted), the JSON format and `.chrote/orchestration/` layout become more defensible because the server can own serialization and the single-tree layout simplifies API routing.

**Decide engine location first. Everything else follows.**
