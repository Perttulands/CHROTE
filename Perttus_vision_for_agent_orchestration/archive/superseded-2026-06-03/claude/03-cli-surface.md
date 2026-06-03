# 03 · CLI Surface — `archon`

The **`archon`** CLI is the primary interface to the agent organization. Agents — and occasionally
Perttu — drive the whole system through it. The cockpit UI issues the *same* operations against the
*same* system (see [01-architecture §3](01-architecture.md)); CLI and UI are two clients of one
backend.

> Naming (decided): **`archon`**, not `fm`. "Formation" is only one of the nouns; the CLI commands the
> whole organization, so it takes the organization's name — the same name as the root persona you talk
> to. `archon` *is* how you (or an agent) command the org.

---

## 1. Shape

`archon <noun> <verb> [args] [--flags]`, `--json` everywhere, typed fail-loud errors with actionable
hints, idempotent on identity, `--dry-run` on mutations. It mirrors `bd`'s ergonomics (the project's
agents already live in `bd`). Built as `src/cmd/archon` → `bin/archon`.

**It is a thin client of the formations system** hosted in the CHROTE server (`/api/formations/*` +
`/api/agents`). Authoring, running, and inspecting all go through that one system, so the CLI and the
UI can never diverge. (The CHROTE service is always up; if it's down, `archon` fails loud with
`SYSTEM_UNAVAILABLE`.)

---

## 2. The vocabulary (canonical — from Perttu)

```
archon agent list                                  # the roster: personas + live sessions (progressive disclosure)
archon agent spawn codex --name scout              # launch a fresh agent from a harness, named scout
archon agent attach scout                          # attach to its terminal (tmux attach / open in UI)
archon agent inspect scout [--deep]                # drill into one agent (tags → config/prompt/skills)

archon formation list
archon formation create peer --agents codex,claude # type ∈ solo|peer|flow|orchestrated; agents inline
archon formation inspect research-peer             # by name

archon mission list
archon mission run "make-session-search-fuzzy"     # start a run; ensures a backing bead (see §4)
archon mission inspect bd-204                       # mission + chain + live state, by its bead id

archon run list
archon run logs bd-204                              # the run ledger for that mission
archon run resume bd-204                            # resume after disconnect/crash (ledger-backed)
archon run abort bd-204                             # cancel — does NOT kill agent sessions

archon gate approve bd-204 research-check
archon gate reject  bd-204 safety-check --reason "plan is sloppy"
```

Five nouns: **agent** (roster + spawn/attach — the registry + launch primitive), **formation** (team
structure), **mission** (bead-backed objective + its formation chain), **run** (an execution: logs /
resume / abort), **gate** (human approve/reject of a checkpoint). This is the whole surface; it maps
1:1 onto the data model and the run engine.

### Authoring verbs (the natural extensions, kept minimal)

Perttu's examples cover the common path. Authoring a multi-step mission also needs (small, additive):

```
archon formation assign research-peer scout         # put/replace an agent in a slot
archon mission create "make-session-search-fuzzy"    # create a mission (creates its backing bead)
archon mission wire <mission> <formation|gate> ...   # wire the chain (mission → formation → gate → …)
```

These are authoring operations agents use to *build* a mission before `mission run`. They're delivered
in their own vertical slices (see [00 §5](00-master-plan.md)), each landing in CLI **and** UI together.

---

## 3. `agent spawn` & `attach` — the launch primitives

`archon agent spawn <harness> --name <name>` launches a fresh agent session:

- `<harness>` ∈ `codex | claude | pi | opencode | hermes | tmux`.
- `--name` is the session name (the "one id" — it's the tmux session stem, the slot reference, the
  ledger key). A spawned agent with **no persona card is disposable**; one whose name matches a card
  (or `--persona susie`) **binds to that persistent identity** (see [05](05-registry-and-personas.md)).
- Spawning runs on the CHROTE tmux socket with that session name. It **never disrupts existing
  sessions** (golden rule).

`archon agent attach <name>` opens the agent's terminal — `tmux attach` in a shell, or the ttyd
terminal popup in the UI (`/terminal/?arg=<name>`). Same operation, two surfaces.

`archon agent list` is the discovery primitive (progressive disclosure): persona cards left-joined
with live Oracle sessions; `--cap design --available --json` filters; `agent inspect <name> --deep`
drills into config/prompt/skills.

---

## 4. Missions are bead-backed

Perttu's commands key missions and runs by **Beads IDs** (`bd-204`): `mission inspect bd-204`,
`run logs bd-204`, `gate approve bd-204 …`. This is a deliberate integration, not incidental:

- A **mission is a unit of work**, so it is backed by a Beads issue. `mission create` / `mission run`
  ensures a backing bead; its **bead id is the durable handle** for the mission, its run, its logs, and
  its gate decisions.
- The **formation chain** (team structure, wiring) lives in the TOML board
  ([02](02-data-model.md)); the **mission/work** lives as a bead; the **run mechanics** live in the
  NDJSON ledger keyed by the bead id. Three stores, one handle.
- This cleanly honors "Beads = work": the mission objective is the bead; the formation is *how* it
  gets done; the run is *the doing*, logged and resumable.

So `archon mission run "make-session-search-fuzzy"` resolves the named mission, ensures its bead
(`bd-204`), launches the run, and everything downstream (`run logs/resume/abort`, `gate approve/reject`)
references `bd-204`.

---

## 5. The organization patterns as commands

**The Archon assembles and launches** (the career/web-experience scenario):

```bash
archon agent list --cap design --available                 # who can design, right now?
archon agent spawn claude --name susie --persona susie     # bring the persistent designer online
archon formation create peer --agents susie,scout          # a design huddle
archon formation assign research-peer susie
archon mission create "career-web-experience"               # → backing bead bd-301
archon mission wire career-web-experience design-huddle      # build the chain
archon mission run career-web-experience                    # → run keyed by bd-301
archon mission inspect bd-301                                # "what's happening?" — chain + live state
```

**A leader hands work cross-harness and stays involved:** the leader spawns/【attaches to】 a Codex
specialist and a Claude specialist in its formation, runs the formation, and watches via
`archon run logs <bead>`. The cross-harness collaboration (Codex + Claude as peers) is the run engine
delivering the brief to both sessions and capturing both replies ([04](04-run-engine-and-adapters.md)).

**A human decision at a gate:**

```bash
archon gate approve bd-301 design-review
archon gate reject  bd-301 safety-check --reason "the plan is sloppy; tighten the data contract first"
```

`approve`/`reject` are the human verdict (clearer than pass/fail); `reject --reason` feeds the pushback
loop. These are exactly the "light tweaks" the UI also offers — same operation, two surfaces.

---

## 6. Fail-loud & discoverability

- No bare `exit 1`: non-zero always carries `{code,message,hint}`. `SYSTEM_UNAVAILABLE` if the CHROTE
  service is down; `NO_AGENT` if a slot is empty; `DEAD_SESSION` if a spawn/attach target vanished;
  `DELIVERY_UNCONFIRMED` if a brief didn't land. Each with an actionable hint.
- Partial success is reported, never hidden (`mission run` with an empty slot refuses with `INCOMPLETE`
  listing the gaps, or runs with `--allow-partial` + a `warnings[]` block).
- `archon` with no args = a one-screen, example-first cheatsheet (Perttu's vocabulary above).
  `archon <noun> --help` shows a runnable example + the JSON shape + the errors it can raise, generated
  from the schema so it can't drift. `--dry-run` previews any mutation.

---

## 7. Slicing (per Perttu: full-stack vertical slices)

The CLI is **not** built in one go ahead of the UI. Each capability lands in CLI **and** UI in the
same slice, so integration issues surface early. The natural order (each a vertical slice — see
[00 §5](00-master-plan.md)):

1. `agent list` + `formation list/create/inspect` — an agent creates a formation; it exists in files
   **and** renders in the UI canvas.
2. `formation assign` — assign an agent to a slot, in CLI **and** via the UI drag gesture (the
   write-back round-trip, [02 §5](02-data-model.md)).
3. `agent spawn` / `attach` — launch and open a real session, from CLI **and** the UI.
4. `mission create/wire` — build a chain, both surfaces.
5. `mission run` + `run logs` (live SSE) — the cross-harness execution, both surfaces. *(The fragile
   tmux/run part, tackled once the structural round-trip is proven full-stack.)*
6. `run resume/abort`, `gate approve/reject` — recovery + human verdicts, both surfaces.

Deferred (Perttu): the notice board (`board …`) — a different system. Per-harness native adapters
beyond tmux. `agent inspect --deep`.
