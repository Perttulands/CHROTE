# Synthesis: Codex vs Claude Design Packets

Both packets read the same sources — the vision interview, the `03-formations` prototype, the existing CHROTE codebase, and the prior meta-harness/Gas City research — and converged on the same core principles. The disagreements are structural, not philosophical. This doc surfaces only the forks that change what gets built.

---

## What both agree on (non-negotiables)

- **Agent-driven authoring.** Agents create formations and missions through a CLI. Perttu does not hand-build node graphs.
- **Files are canonical.** Definitions and run history live on disk. Every other component (CLI, engine, CHROTE) is a *client* of those files.
- **Conversational visibility first.** Stage 1 has no required UI. You ask the agents; they read the ledger and answer.
- **Formations tab is Phase 2, default-off, read-only first.** It inspects agent-authored structure, never replaces the CLI.
- **Beads stays work-only.** Do not turn Beads into the graph store or the comms channel.
- **No Gas City as first-class.** Gas City lessons inform the design, but the native contract comes first.
- **Feature-flagged and reversible.** Every layer has a kill switch. Default-off changes nothing.
- **Append-only run ledger.** Run state is produced, never authored. Never pollutes the definition file.

---

## The four disagreements that change the build

### 1. Where does the run engine live?

| | Codex | Claude |
|---|---|---|
| **Location** | `src/internal/orchestration` — a new Go package inside the CHROTE server | Inside a standalone **`fm`** CLI binary |
| **Invocation** | Server-hosted; CHROTE API triggers runs | Agent-invoked via `fm run` in a tmux session |
| **Ledger read** | CHROTE API streams from its own store | CHROTE reads the on-disk ledger over SSE; `fm` owns write |
| **Recovery** | Server state + snapshots | `fm run --resume` replays the on-disk NDJSON |
| **Philosophy** | CHROTE grows orchestration as a server capability | CHROTE stays a cockpit; `fm` is the control plane |

**Why it matters:** The Codex path makes CHROTE the orchestration authority. The Claude path keeps CHROTE as a reader and makes `fm` the authority. If CHROTE becomes the scheduler, you need server uptime, API design, and concurrency handling from day one. If `fm` is the scheduler, you get recoverability for free (the ledger is the state) but you need a durable tmux session to host `fm run`.

**Decision needed:** Do you want CHROTE to host scheduling, or do you want a CLI-invoked engine that CHROTE merely observes?

---

### 2. What is the file format for definitions?

| | Codex | Claude |
|---|---|---|
| **Format** | JSON | TOML |
| **Why** | Go-native, machine-safe, avoids parser dependencies | Agents already author TOML (Gas Town); LLM-robust (no YAML indentation traps); single-line diffs for renames/rewires |
| **API boundary** | JSON directly served | TOML transcoded to JSON at API boundary (same pattern `bd --json` already uses) |
| **First slice** | One `mission_<id>.json` per mission | One `.formation.toml` "board" per mission graph + a `.layout.toml` sidecar |

**Why it matters:** JSON is safer for exact machine edits. TOML is safer for agent authoring and human diffs. The Claude packet explicitly notes that CHROTE already transcodes TOML→JSON for `bd`, so this is not a new wire-format problem.

**Decision needed:** JSON (safer for machine) or TOML (safer for agents/humans, with existing transcoding precedent)?

---

### 3. What is the CLI called?

| Codex | Claude |
|---|---|
| `chrotectl` | `fm` |

**Why it matters:** `chrotectl` is descriptive but long. `fm` pairs with `bd` ("the `bd` of teams") and is short enough to be a frequent verb. Both packets agree the CLI is the primary interface; the name is cheap to change later.

**Decision needed:** `chrotectl` (explicit) or `fm` (terse, pairs with `bd`)?

---

### 4. Where do canonical files live?

| | Codex | Claude |
|---|---|---|
| **Orchestration files** | `.chrote/orchestration/` under workspace root | `.formations/` under workspace root (sibling of `.beads/`) |
| **Agent profiles** | `.chrote/orchestration/agents/*.json` | `~/agents/*.toml` (central, identities span projects) |
| **Run state** | `.chrote/orchestration/runs/` | `.formations/runs/<board>/` |
| **Notice board** | `.chrote/orchestration/notices/*.jsonl` | `.formations/board.ndjson` |

**Why it matters:** The Claude path mirrors `.beads/` exactly (CHROTE already validates sibling directories) and centralizes personas outside any project. The Codex path keeps everything under `.chrote/` for discoverability but colocates agent profiles with missions (which may not make sense if the same agent works across projects).

**Decision needed:** `.chrote/orchestration/` (single tree) or `.formations/` + `~/agents/` (split by concern, mirroring existing `.beads/` convention)?

---

## Secondary disagreements (matter, but don't block the first slice)

### 5. First slice: headless only, or pull the tab forward?

- **Codex:** Headless first, but the tab can be Phase 2 alongside the spine.
- **Claude:** Strictly headless first. The tab is explicitly *last* and must "earn its place." Canvas-first is the Gas City trap.

Both agree the tab is read-only and default-off. The question is whether seeing the prototype rendered is urgent enough to parallelize with the spine build.

### 6. Cross-harness mechanics

- **Codex:** Adapter layer in the Go backend (Phase 5).
- **Claude:** `tmux send-keys` + `capture-pane` + sentinel completion line (`<<<CHROTE-DONE …>>>`) from day one.

The Claude packet treats cross-harness as the *acceptance criteria* for Phase 0. The Codex packet defers adapters to Phase 5. This is the biggest scope difference between the two packets.

### 7. Notice board

- **Codex:** Include it in v0 as file-backed JSONL.
- **Claude:** Defer to Phase 1.5 — build it the moment a real two-agent async note is needed.

### 8. Persona registry binding

- **Codex:** Profile JSON files + live tmux/Oracle session matching.
- **Claude:** TOML "cards" in `~/agents/` that *point at* existing definitions (Hermes profiles, `CLAUDE.md`, skills). The "one ID" spine ties cards → tmux sessions → slots → teams.

---

## Recommended resolution

If you want to start building today, the decisions in order of leverage:

1. **Engine location** — This changes the architecture diagram and the first file you write.
2. **File format** — This changes the schema and the serializer.
3. **File layout** — This changes where `fm` looks and where agents write.
4. **CLI name** — Cheap to change; pick one and move on.
5. **First slice scope** — Does cross-harness acceptance happen in Phase 0 or Phase 5?

My recommendation (as the synthesizer, not the designer):

- **Engine in `fm`**, not the server. The Claude packet's argument is stronger: CHROTE should not become a competing control plane. The ledger gives you recoverability without server state. `fm run --resume` is simpler than server snapshot recovery.
- **TOML for definitions.** You already have agents authoring TOML (Gas Town). The transcoding precedent exists (`bd --json`). Single-line diffs matter for git review.
- **`.formations/` + `~/agents/` layout.** It mirrors `.beads/`, which CHROTE already understands. Central personas make sense for cross-project identity.
- **`fm` as the CLI name.** Pairs with `bd`. Short. Agents will type it often.
- **Headless first, cross-harness in Phase 0.** The Claude packet's acceptance criteria (real Claude Code → Codex handoff via sentinel) is the most falsifiable proof that the system works. Deferring it to Phase 5 makes Phase 0 untestable.

---

## What to read next

- **Data model deep-dive:** `data-model-comparison.md` (generated alongside this file) — side-by-side schema, ID strategy, and separation-of-concerns comparison.
- **Claude master plan:** `claude/00-master-plan.md` — the most coherent single-document expression of the vision.
- **Codex master plan:** `codex/master-plan.md` — the most implementation-phased expression, with explicit verification gates per phase.
- **Open questions:** `claude/08-open-questions.md` — 20 questions tiered by blocking power. Tier 1 (Q1–Q5) are the decisions above.
