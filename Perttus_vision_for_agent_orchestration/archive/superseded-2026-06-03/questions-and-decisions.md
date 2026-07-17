# Open Questions & Decisions — Agent Orchestration System

> This is the decision gate. Implementation should not start until these questions are answered or intentionally deferred with a rationale.
> 
> The agent is not a passive participant in this system — it is a first-class entity that needs data structure, a catalogue, and systems to build, evolve, and discover itself. These questions reflect that.

---

## Tier 1 — Blocking: Cannot start without these

### Q1. Where does the run engine live?

**The question:** Does CHROTE host the scheduler, or does a standalone CLI (`fm` / `chrotectl`) own scheduling while CHROTE merely reads the ledger?

**Options:**

- **A. Server-hosted (Codex path):** New `src/internal/orchestration` Go package inside CHROTE. CHROTE API triggers runs. Server owns state snapshots.
- **B. CLI-hosted (Claude path):** `fm run` invoked by an agent in a tmux session. Append-only on-disk ledger. `fm run --resume` replays. CHROTE reads ledger over SSE.

**Why it blocks everything:** This changes the architecture diagram, the first file you write, and every subsequent boundary. If CHROTE hosts scheduling, you need server uptime, concurrency, and API design from day one. If `fm` hosts scheduling, you need a durable tmux session to host `fm run` but get recoverability for free (the ledger is the state).

**Recommendation:** `fm` owns scheduling. CHROTE should not become a competing control plane. The ledger gives recoverability without server state.

---

### Q2. What is the file format for definitions?

**The question:** JSON or TOML for mission/formation/gate definitions?

**Options:**

- **A. JSON (Codex):** Go-native, machine-safe, no new parser dependencies. LLMs handle JSON well.
- **B. TOML (Claude):** Agents already author TOML (Gas Town formulas). Single-line diffs for renames/rewires. No YAML indentation traps. CHROTE already transcodes TOML→JSON for `bd --json`.

**Why it blocks:** The schema, serializer, and round-trip tests all depend on this. Changing later means rewriting every test and sample.

**Recommendation:** TOML for definitions, transcoded to JSON at API boundary. The diff cleanliness and existing authoring precedent outweigh the parser dependency.

---

### Q3. Where do canonical files live?

**The question:** Single tree under `.chrote/orchestration/` or split `.formations/` + `~/agents/`?

**Options:**

- **A. Single tree (Codex):** `.chrote/orchestration/{agents,missions,runs,notices}/`. Simple to discover, one root to validate.
- **B. Split (Claude):** `.formations/` for mission graphs (sibling of `.beads/`), `~/agents/` for persona cards (central, cross-project). Layout in `.formations/layout/` sidecar.

**Why it blocks:** Directory discovery logic, workspace validation, and the `fm` config all depend on this.

**Recommendation:** Split layout. `.formations/` mirrors `.beads/` (CHROTE already understands this). Central `~/agents/` avoids persona drift across projects. Layout sidecar keeps definition files pure.

---

### Q4. What is the CLI called?

**Options:** `chrotectl` (descriptive, explicit) or `fm` (terse, pairs with `bd`).

**Why it blocks:** Cheap to change later, but every doc, sample, and test will reference it. Pick one and move on.

**Recommendation:** `fm`. "The `bd` of teams." Short enough to be a frequent verb.

---

### Q5. What is the first slice?

**The question:** Does Phase 0 include a real cross-harness handoff, or is that Phase 5?

**Options:**

- **A. Spine only (Claude):** One formation, slots referencing persona IDs, `fm run`, NDJSON ledger, conversational visibility. No gates, no canvas, no tab. Cross-harness acceptance is the acceptance criteria for Phase 0.
- **B. Backend contract first (Codex):** File contract + CLI skeleton (`init`, `validate`, `diff`) + read-only API + feature-flagged tab. Cross-harness adapters deferred to Phase 5.

**Why it blocks:** This determines what "done" means for the first PR. If cross-harness is deferred, Phase 0 is untestable as an end-to-end system.

**Recommendation:** Include cross-harness in Phase 0. The acceptance scenario (Archon → design lead in different harness, real artifact produced) is the most falsifiable proof the system works.

---

## Tier 2 — Load-bearing: Answer before completing first slice

### Q6. What is an agent persona's data structure?

**The question:** What fields does a persona card need to describe an agent's identity, capabilities, and runtime binding?

**Current proposals:**

Codex (JSON, colocated):
```json
{
  "id": "codex",
  "display_name": "Codex",
  "kind": "specialist",
  "capability_tags": ["typescript", "react", "go"],
  "personality_tags": ["fast", "direct"],
  "focus_tags": ["frontend", "prototyping"],
  "default_harness": "openai-codex",
  "session_match": "codex-*",
  "source_files": ["~/.codex/config.toml"],
  "notes": "..."
}
```

Claude (TOML, central `~/agents/`):
```toml
id          = "codex"
displayName = "Codex"
kind        = "specialist"

[[capability]]  ; tag = "typescript"
[[capability]]  ; tag = "react"
[[capability]]  ; tag = "go"

[[personality]] ; tag = "fast"
[[personality]] ; tag = "direct"

[[focus]]       ; tag = "frontend"
[[focus]]       ; tag = "prototyping"

[harness]
  default   = "openai-codex"
  matchStem = "codex"    ; tmux session stem Oracle binds to

[[source]]
  type = "config"
  path = "~/.codex/config.toml"

[notes]
  """..."""
```

**Why it matters:** The persona card is the bridge between the formation graph and live sessions. It needs enough structure for the Archon to say "who can do this?" and enough flexibility for humans to add fields agents will later consume.

**Open sub-questions:**

- **Q6a. What kinds exist?** Archon, leader, specialist, reviewer, maintainer, disposable? Or a tag system instead of a closed taxonomy?
- **Q6b. How are capabilities declared?** Free tags, or a structured hierarchy (language × domain × task)?
- **Q6c. What is the runtime binding model?** Session stem matching (Claude's `matchStem`) vs explicit session ID vs harness name vs profile name?
- **Q6d. What links to the agent's "source of truth"?** Hermes profile path? `CLAUDE.md` path? `.codex/config.toml`? All of the above?
- **Q6e. How does a persona card get created?** Manually by Perttu? Generated by an agent introspecting its own config? Bootstrapped from existing profiles?
- **Q6f. Can an agent have multiple persona cards?** One per harness? One per project? One canonical card with harness-specific overrides?

**Recommendation:** Start with a minimal card (id, displayName, kind, tags, harness, matchStem, source pointers). Add structure only when a real query can't be answered. Kinds as tags, not a closed enum.

---

### Q7. How does the agent catalogue work?

**The question:** How does the Archon (or a team leader) discover available agents and their capabilities?

**Options:**

- **A. File scan:** `fm who` reads `~/agents/*.toml` and `~/.hermes/profiles/*/config.yaml` to discover Hermes profiles. No registry server.
- **B. Live session join:** Oracle-detected tmux sessions are left-joined to persona cards by session stem. A session without a card is "unbound" (visible but not assignable).
- **C. Explicit registration:** Agents register themselves into a central registry file (`~/agents/registry.toml` or similar).
- **D. Hybrid:** File scan for cards + live session join for liveness + explicit registration for override/ranking.

**Why it matters:** The Archon needs to answer "who can do this?" without Perttu naming sessions. Discovery must be automatic enough to be useful, bounded enough to not be magic.

**Open sub-questions:**

- **Q7a. What happens when two agents claim the same capability?** Ranking by track record? Explicit priority? Round-robin?
- **Q7b. What is the "track record" data structure?** The ledger records every dispatch/result per agent. How is this summarized for discovery queries?
- **Q7c. How does an agent signal it is "busy" or "available"?** tmux session liveness? Explicit heartbeat? No signal at all (assign and let the adapter fail loud)?
- **Q7d. Can personas be ephemeral?** A disposable agent spawned for one task — does it get a card? Is it discoverable?

**Recommendation:** File scan + live session join. `fm who` lists cards and live sessions. Unbound sessions are visible but flagged. Track record stays in the ledger; summarization is a read projection, not a write model.

---

### Q8. How are agents built?

**The question:** What is the system for creating, configuring, and evolving an agent?

**This is the missing piece.** Both design packets assume agents exist. Neither defines the factory that creates them.

**Open sub-questions:**

- **Q8a. What is the minimum viable agent?** A Hermes profile? A tmux session + `CLAUDE.md`? A `.codex/config.toml`? All of the above are different shapes.
- **Q8b. What is the agent creation flow?**
  - Perttu describes a role to the Archon?
  - The Archon generates a persona card, picks a harness, creates a profile/session, writes the card?
  - Or: Perttu creates the profile/session manually, the Archon generates the card by introspection?
- **Q8c. What is the agent configuration surface?** Model, provider, reasoning effort, toolsets, skills, memory, personality, system prompt fragments — which are in the persona card, which in the harness config, which in the profile?
- **Q8d. How does an agent evolve?** If Susie (design lead) gets better at React over time, where is that captured? In the card? In the ledger? In a separate evaluation record?
- **Q8e. What is the relationship between persona card and harness config?** Does the card *point at* the config (reference), *contain* the config (inline), or *drive* the config (the card is the source of truth and the harness config is generated from it)?
- **Q8f. Can one persona span multiple harnesses?** Susie as a Claude Code persona and a Hermes persona — one card with harness variants, or two cards with a shared identity?
- **Q8g. What is the agent's "self-knowledge"?** Does the agent know its own card? Is the card injected into its context? Or is the agent oblivious and the system manages all metadata externally?

**Recommendation:** Define the factory separately from the orchestration system. The first slice assumes agents exist (Perttu has Hermes profiles, Claude Code sessions, Codex configs). The persona card is a *pointer*, not a *factory*. Factory work (generating profiles, creating sessions, writing configs) is Phase 1.5 or 2.

---

### Q9. What is the "one ID" spine?

**The question:** A single identifier ties cards → sessions → slots → teams → ledger. What is it?

**Current proposal (Claude):**
- The persona's `id` is simultaneously:
  - The card filename (`~/agents/susie.toml`)
  - The tmux session stem Oracle binds to (`susie` / `claude-susie`)
  - The `agentId` a formation slot references
  - The team member reference
  - The ledger key for that agent's track record

**Why it matters:** Get this right and registry ⇄ formations ⇄ live session ⇄ teams ⇄ evaluation all join for free. Get it wrong and every boundary needs a mapping layer.

**Open sub-questions:**

- **Q9a. What happens when two harnesses use the same ID?** `codex` as a session name in both Claude Code and Hermes? Prefixing? Namespacing?
- **Q9b. Are IDs human-meaningful or opaque?** `susie` vs `agent_01J9XF...`. Meaningful is easier to debug. Opaque avoids collision. Hybrid: meaningful + ULID suffix on collision?
- **Q9c. Who owns ID uniqueness?** The card author? `fm` on creation? Oracle on session detection?

**Recommendation:** Human-meaningful primary ID with optional ULID suffix for disambiguation. `susie` is the canonical ID. If `susie` is ambiguous (two sessions), `susie-codex` and `susie-hermes` are the disambiguated forms. The card lists all bound sessions.

---

### Q10. What is the formation → session binding model?

**The question:** When a formation says "slot A = susie," how does that resolve to a live tmux session?

**Options:**

- **A. Static binding:** The slot specifies `agentId = "susie"` and `fm run` resolves that to the active `susie` session at runtime.
- **B. Dynamic binding:** The slot specifies `requiredCapabilities = ["design", "react"]` and the engine picks the best available agent at runtime.
- **C. Hybrid:** Static default with dynamic fallback if the bound agent is unavailable.

**Why it matters:** This determines whether formations are "who does this" (static, agent-authored) or "what is needed" (dynamic, engine-resolved). The vision says agents staff formations, which implies static binding. But dynamic binding is more resilient.

**Recommendation:** Static binding with explicit override. The agent authoring the formation names the slot occupant. If unavailable, the engine fails loud (records an error in the ledger) rather than silently substituting. Dynamic binding is a later optimization.

---

## Tier 3 — Architectural: Answer before Phase 2

### Q11. What is the notice board?

**Options:**

- **A. File-native NDJSON (Claude):** `.formations/board.ndjson`. Append-only. `cat`-able. Separate from Beads.
- **B. Project-scoped JSONL (Codex):** `.chrote/orchestration/notices/*.jsonl`. One file per mission.

**Why it matters:** The notice board is team communication, not work tracking. Conflating it with Beads overloads both systems.

**Recommendation:** File-native NDJSON, central per workspace. Not per-mission (agents communicate across missions). Not Beads.

---

### Q12. What is the escalation channel?

**The question:** When an agent raises `CHROTE-ESCALATE`, what happens?

**Options:**

- **A. Ledger event → Archon surfaces it conversationally.** No interrupt. No TTS. The Archon mentions it when Perttu next asks.
- **B. Ledger event → mirrored to `bd` event/wisp → optional TTS interrupt via `/srv` gateway.** Loud and immediate.
- **C. Both:** Configurable per agent/persona. Some escalations are quiet (ledger only), some are loud (TTS).

**Recommendation:** Start with A (ledger + conversational). B is a nice-to-have that requires the TTS gateway and `bd` event system. Add it when a real escalation is missed because it was too quiet.

---

### Q13. What is the canvas write-back model?

**The question:** When the Formations tab (Phase 2) allows dragging nodes, what happens to the underlying files?

**Options:**

- **A. No write-back:** Canvas is purely read-only. All edits via `fm` CLI.
- **B. Layout-only write-back:** Dragging writes to the layout sidecar only. Structure is CLI-only.
- **C. Full write-back with patch:** UI generates `fm` patch commands. Server does not write files directly; it shells out to `fm`.

**Recommendation:** B for layout, C for structure (via `fm` shell-out). CHROTE already shells out to `bd` for Beads mutations. Same pattern for `fm`.

---

### Q14. What is the gate verdict model?

**The question:** Gates are checkpoints. How are they evaluated?

**Options:**

- **A. Agent-evaluated:** The slot occupant (or a designated judge slot) produces a verdict.
- **B. Code-evaluated:** A shell command runs and exit code determines pass/fail.
- **C. Human-evaluated:** Perttu is prompted for a verdict.
- **D. Hybrid:** Kind-specified per gate (`code`, `human`, `formation`).

**Recommendation:** D. Gate definition specifies kind. Stage 1 only needs `code` (exit code) and `human` (explicit verdict). `formation` (judge slot) comes later.

---

### Q15. What is the evaluation system?

**The question:** How do you know if an agent is getting better or worse?

**Open sub-questions:**

- **Q15a. What is measured?** Success rate? Artifact quality? Escalation frequency? Cycle time?
- **Q15b. Who evaluates?** The Archon? A dedicated reviewer agent? Perttu?
- **Q15c. Where does evaluation live?** In the ledger? A separate `~/agents/<id>/evaluations/` directory?
- **Q15d. How is evaluation consumed?** Ranking in `fm who`? Archon mentions it when staffing? Explicit report to Perttu?

**Recommendation:** Evaluation is Phase 3+. Phase 0 captures the raw data (ledger). Phase 1 adds summarization. Phase 2 adds consumption. Don't design the evaluation system before you have data to evaluate.

---

## Tier 4 — Deferred: Do not answer yet

### Q16. Always-on daemon?

Does `fm serve` ever make sense? For scheduled runs? For autonomous loops (integrity maintainer)?

**Deferred:** Build the ledger format so it's daemon-ready. Don't build the daemon until a real always-on need appears.

### Q17. Multi-host?

Can agents live on different machines?

**Deferred:** Localhost-only for Stage 1. Multi-host requires network transport, auth, and session proxying.

### Q18. Gas City as adapter?

Does Gas City's supervisor ever get reused?

**Deferred:** Evaluate only after CHROTE-native contracts are proven. Start read-only.

### Q19. Agent marketplace?

Can agents share persona cards? Is there a "public" agent registry?

**Deferred:** Private `~/agents/` only. Public registry is a future feature.

### Q20. Self-modifying agents?

Can an agent rewrite its own persona card? Its own harness config?

**Deferred:** This is a governance question. The system should not prevent it, but it should not encourage it either. Fail loud if it happens.

---

## The agent data structure: what we know and what we don't

### What we know

An agent persona card must describe:

1. **Identity:** `id`, `displayName`, `kind` (or tags)
2. **Capabilities:** What the agent can do (languages, domains, tasks)
3. **Personality:** How the agent behaves (tone, speed, style)
4. **Focus:** What the agent prefers or specializes in
5. **Harness:** How to reach the agent (model, provider, CLI, session stem)
6. **Sources:** Where the agent's configuration lives (profile path, `CLAUDE.md`, skill list)
7. **Notes:** Freeform context, history, known issues

### What we don't know

1. **The factory:** How does a card get created? Who creates it? What is the minimum viable creation flow?
2. **The lifecycle:** How does an agent evolve? Where is "this agent got better at X" captured?
3. **The self-model:** Does the agent know its own card? Is the card part of its system prompt?
4. **The binding:** How does a formation slot resolve to a live session? What happens when the session is dead?
5. **The track record:** How is the ledger summarized for discovery queries? What is the query interface?
6. **The namespace:** How do IDs work across harnesses? Across projects? Across hosts?

---

## Recommended resolution order

1. **Q1 (engine location)** — Unlock architecture
2. **Q2 (file format)** — Unlock schema
3. **Q3 (file layout)** — Unlock discovery/validation
4. **Q4 (CLI name)** — Unlock naming in all docs
5. **Q5 (first slice scope)** — Unlock acceptance criteria
6. **Q6 (persona data structure)** — Unlock agent model
7. **Q9 (one ID spine)** — Unlock cross-layer joins
8. **Q10 (binding model)** — Unlock runtime semantics
9. **Q7 (catalogue)** — Unlock discovery queries
10. **Q8 (build system)** — Unlock agent factory (Phase 1.5)

Q11–Q15 are Phase 2 blockers. Q16–Q20 are explicitly deferred.

---

## Status

| Question | Status | Owner | Date |
|---|---|---|---|
| Q1. Engine location | **OPEN** | Perttu | — |
| Q2. File format | **OPEN** | Perttu | — |
| Q3. File layout | **OPEN** | Perttu | — |
| Q4. CLI name | **OPEN** | Perttu | — |
| Q5. First slice scope | **OPEN** | Perttu | — |
| Q6. Persona data structure | **OPEN** | Perttu | — |
| Q7. Agent catalogue | **OPEN** | Perttu | — |
| Q8. Agent build system | **OPEN** | Perttu | — |
| Q9. One ID spine | **OPEN** | Perttu | — |
| Q10. Binding model | **OPEN** | Perttu | — |
| Q11–Q20 | **DEFERRED** | — | — |
