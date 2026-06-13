# 02 · Data Model & On-Disk Format

The canonical, file-backed representation. This is the source of truth; the CLI, engine, and cockpit
are all clients of it.

> **Decisions applied (Perttu — see [08](08-open-questions.md)):** the CLI is **`archon`** (read
> `archon` wherever this doc shows `fm`); the single writer is the **server's formations package** (CLI
> + UI both go through it); missions are **bead-backed** (a mission's bead id keys its run and logs).

---

## 1. Format & layout

**TOML for definitions, NDJSON for append-only logs, a TOML sidecar for canvas layout.**

Why TOML for definitions: agents already author TOML (Gas Town formulas); array-of-tables maps 1:1
to nodes and is the most LLM-robust nested structure (no YAML indentation traps, no deep-JSON
punctuation churn); renames/rewires are single-line diffs. CHROTE transcodes TOML→JSON at the API
boundary — the same move it already makes for `bd --json`, so no new wire format reaches the
frontend.

**Layout — mirror `.beads/` exactly** (it's the sibling precedent CHROTE already validates):

```
<workspace-root>/
├── .beads/                              # existing
└── .formations/                         # NEW — sibling, same allowed-roots discovery
    ├── boards/
    │   └── session-search.formation.toml      # one board = one canonical graph
    ├── layout/
    │   └── session-search.layout.toml         # x/y + hand-routed wire lanes (presentation only)
    ├── runs/
    │   └── session-search/
    │       ├── run-01J9XF….ndjson             # one file per run, append-only event log
    │       └── latest.json                    # derived per-node snapshot (cache; regenerable)
    └── board.ndjson                           # per-board notice board (see doc 06)
```

Personas and teams live centrally in `~/agents/` (see [doc 05](05-registry-and-personas.md)), not in
`.formations/`, because identities span projects.

---

## 2. Separation of concerns (the critical discipline)

Three kinds of data, three physical homes, so a human tweak is a safe diff and run state never
pollutes the definition:

| Concern | Home | Writer | Rule |
|---|---|---|---|
| **(a) Definition** — missions, formations, slots, briefs, gates, verifications, connections | `boards/*.formation.toml` | agents (via `archon`) | Rich and canonical. The UI may write here *only* through a small allowlist, as field-level edits by id. Never reconstructed from the canvas. |
| **(b) Layout** — node x/y, hand-routed wire lanes | `layout/*.layout.toml` | UI (drag), `archon` | Purely presentational. Missing entry ⇒ auto-placed. Deleting it loses positions, never structure. |
| **(c) Run state** — outputs, verdicts, status, timing, events | `runs/<board>/*.ndjson` (+ `latest.json` cache) | the run engine | Produced *by a run, never authored*. Append-only. Never touches the definition file. |

The UI overlays run state at render time by **joining** definition (by id) + `latest.json` (by id) +
layout (by id) — three clean reads composed into the prototype's JSON shape. A run never produces a
git diff to a board file.

---

## 3. Identity

The prototype mints ephemeral `s1/f1/g1` counters — fatal for round-trip (collide across files,
agents, writers). Replace with **content-free, sortable, collision-free IDs minted once at creation
and never reused**:

- Shape: `<prefix>_<ulid>` — `brd | mis | fmn | slot | gate | ver | port | edge`. Self-describing
  (you can tell node-kind from a reference) and ULID gives time-sortable stable serialization order.
- **Ports are addressed durably as `nodeId:portId`.** Within a node, `portId` is stable
  (`port_in_main`, or fixed `in/pass/fail/judge` for gates).
- **Every edge carries its own `id`**, so a human rewire mutates `connection.to` on a known edge
  (clean diff) rather than delete+insert (noisy, loses provenance).
- Human-friendly slugs/titles/labels are display-only; nothing structural references them.
- ID minting needs no central allocator — ULIDs don't collide, which is what makes concurrent
  CLI+UI authoring safe.

---

## 4. The schema (annotated)

```toml
# ── BOARD HEADER ──
schema    = 1                          # version → migration gate; reader refuses schema > known max (fail loud)
id        = "brd_01J9_sesssearch"
slug      = "session-search"           # == filename basename; rename-safe (structure refs the id)
title     = "Improve session search"   # human-tweakable
kind      = "board"                    # "board" (live) | "template" (reusable, empty agentIds, copy-to-instantiate)
updatedBy = "agent:scout"              # provenance: "agent:<id>" | "human" | "run"
updatedAt = "2026-06-02T10:14:00Z"     # cheap optimistic-concurrency signal alongside mtime/etag

# ── MISSION (entry point; one implicit output port "<id>:out") ──
[[mission]]
  id = "mis_01J9_improve"; title = "Improve session search"
  goal = "Make session search fuzzy and keyboard-first"   # objective = seed input to the first node

# ── FORMATION ──
[[formation]]
  id = "fmn_01J9_research"; type = "peer"   # solo | peer | flow | orchestrated (execution-style HINT)
  title = "Research huddle"
  [formation.brief]                          # the manual task spec (prototype f.input)
    goal = ""; beadId = "bd-204"; files = ["src/SessionPanel.tsx", "https://…"]
  [[formation.input]]  ; id = "port_in"  ; label = "Input"     # N inputs; 2+ = a JOIN
  [[formation.output]] ; id = "port_out" ; label = "Output"    # N outputs; multiple = fan-out
  [[formation.slot]]   ; id = "slot_a" ; label = "Peer" ; agentId = "codex"       ; controller = false
  [[formation.slot]]   ; id = "slot_b" ; label = "Peer" ; agentId = "claude-code" ; controller = false
  [formation.verification]                   # optional; runs at END of the formation's work
    id = "ver_x"; kinds = ["code"]           # subset of {code, human, formation}
    criterion = "both reads converge on a recommendation"
    onFail = "block"                          # block | pushback (re-run own slots with feedback)

# ── GATE (first-class checkpoint between formations) ──
[[gate]]
  id = "gate_01J9_review"; kinds = ["human", "code"]   # any combination of {code, human, formation}
  criterion = "research is sound and the plan is safe to build"
  onFail = "block"                            # block | pushback; UNWIRED fail port ⇒ block
  judgeFormationId = ""                        # set ⇒ adds "formation" to kinds; a judge formation decides
  # implicit ports: "<id>:in", "<id>:pass", "<id>:fail", "<id>:judge"
  [gate.code]                                  # agent-authored; UI shows only `criterion` (richer-than-UI)
    command = "go test ./... && npm run build"; cwd = "src"

# ── CONNECTIONS (directed, port-addressed; each has a stable id) ──
[[connection]] ; id = "edge_1" ; from = "mis_01J9_improve:out"        ; to = "fmn_01J9_research:port_in"
[[connection]] ; id = "edge_2" ; from = "fmn_01J9_research:port_out"  ; to = "gate_01J9_review:in"
[[connection]] ; id = "edge_3" ; from = "gate_01J9_review:pass"       ; to = "fmn_01J9_ship:port_in"
# a fail edge to an EARLIER node = a revise loop; tolerated via the wire while the forward graph stays acyclic
```

Notes: `type` is a hint the engine reads, not machinery the file encodes. Verification is a sub-table
(can't be wired independently). Gate ports are fixed; formation ports are explicit arrays
(variable-count). Exactly one slot may be `controller = true` (validated on write). `judgeFormationId`
and the `"formation"` kind are kept in sync by the writer.

### Links out (id references resolved at the API boundary, never denormalized)

- `brief.beadId` → a bead id string; hydrated on read via `bd show --json` (CHROTE's existing
  pattern). Empty/omitted = no link.
- `gate.code.command` → the executable check an agent wired up (the gate's "Code" kind).
- `slot.agentId` → a persona/agent id (the "one id" spine). Thin by design: liveness comes from
  Oracle at render time. Same agent may fill many slots (reference, not ownership).
- `judgeFormationId` → another formation in the same board (cross-board judges deferred).

---

## 5. Round-trip & concurrency rules

The same files are touched by `archon`, running agents, and the cockpit concurrently.

1. **Write = load → mutate-by-id → re-serialize with deterministic key order.** Never serialize from
   the canvas. Unknown/extra keys are carried through verbatim → the file model stays richer than the
   UI, and fields the UI never shows survive an edit untouched.
2. **Optimistic concurrency** via `If-Match: <etag>` (etag = hash of bytes + mtime). Mismatch → `409`;
   the client re-reads and replays its one field edit. No lock daemon.
3. **External-edit detection** via mtime/fsnotify → an SSE `board.changed`; the UI reloads (the
   prototype already does a full `rerender()`). "Live-ish," not a CRDT.
4. **Field-level last-writer-wins**, never whole-file. An agent adding a slot and a human renaming the
   board don't conflict.
5. **Atomic writes**: temp file + `rename()`. Fail loud on write error.
6. **Versioning**: `schema = N`; refuse newer (fail loud), up-migrate older. The one piece of
   forward-proofing kept, because silently changing a user's source-of-truth format is how you corrupt
   it.
7. **Runs never contend with definitions** — append-only NDJSON per run; the engine rewrites only the
   `latest.json` cache, atomically.

---

## 6. Single writer for the format

To avoid two serializers fighting over key order/comments, **the server's formations package is the
only writer of definition files.** Both the cockpit UI and the `archon` CLI go through it (the CLI is a
client of `/api/formations/*`), so "the UI and the CLI issue the same operation" is literally true and
there is no format drift. (Reads can be direct-file for speed; writes always go through the one
writer.)

---

## 7. Minimal first slice

Per the master plan, the first slice is "see a formation" — an agent authors a formation via `archon`
and it renders read-only in the cockpit. So the schema needs:

- A board TOML with **one** formation (type + slots referencing agent ids) + brief. No gates, no
  verification, no multi-input JOIN yet.
- The **layout sidecar** (x/y) so the canvas can place the node (auto-placed if absent).
- Stable ULID ids; `schema = 1`; atomic writes; the read composition into the prototype's JSON shape.

The `runs/` NDJSON ledger arrives with the **run** slice (slice 5); gates, judges, all four formation
types, fan-out, and JOIN come in later slices. Ship the **round-trip test** now (Rule 7): hand-author a
board with an extra agent-authored key the writer doesn't model, issue a rename via `archon`, assert
the title changed, the extra key survived byte-for-byte, and the diff is one line.
