# 08 · Open Questions & Decisions to Nail Down

Consolidated from all seven design passes. **Tier 1 is now RESOLVED by Perttu** (recorded below,
verbatim intent). Tier 2 is decided during implementation; Tier 3 is settled-with-a-default.

---

## Tier 1 — RESOLVED (Perttu, this session)

### Q1. Execution model → **one hosted system, accessed by CLI and UI alike**
Perttu: *"This question seems confused. The mission execution is a system that can be accessed through
the CLI and the UI same as anything else."*
**Decision:** mission execution is not an engine that lives off in the CLI with CHROTE spectating. It
is part of **the formations system**, hosted once (in the CHROTE server, exposing `/api/formations/*`),
and both `archon` (CLI) and the cockpit UI are clients of it. Files + NDJSON ledger remain the source
of truth; the server replays the ledger on restart, so runs survive disconnect/restart. This
supersedes the earlier "engine inside `fm`, CHROTE reads only" framing. The "don't become a control
plane" guard becomes "don't grow a Gas-City worldview" (stay file-backed, simple, fail-loud,
session-non-managing) — not "don't host execution." See [01-architecture §3](01-architecture.md).

### Q2. Phasing → **vertical full-stack slices, not headless-then-UI**
Perttu: *"We should slice this in a full-stack way. An example slice is making a [formation] by an
agent so it exists and is also there on the UI. Another slice could be making the assignment of one
agent to a formation both in CLI and in UI. And so forth. That surfaces issues early."*
**Decision:** each slice delivers one capability **end-to-end** — an agent does it via `archon` (CLI) +
it round-trips through files + it appears/works in the cockpit UI — before moving to the next
capability. The UI is built incrementally *alongside* the CLI from slice 1, not deferred. The fragile
cross-harness *run* is its own later full-stack slice, after the structural round-trip (create / assign
/ wire) is proven full-stack. See the revised [00-master-plan §5](00-master-plan.md).

### Q3. CLI name → **`archon`**
Perttu: *"`fm` is wrong. Formation is one of the things we have here."* — and gave the vocabulary:
`archon agent list|spawn|attach`, `archon formation list|create|inspect`, `archon mission list|run|
inspect`, `archon run list|logs|resume|abort`, `archon gate approve|reject --reason`.
**Decision:** the CLI is **`archon`** (same name as the root persona — it *is* how you command the
org). Missions and runs are keyed by **Beads IDs** (`bd-204`) — a mission is bead-backed work. See the
rewritten [03-cli-surface.md](03-cli-surface.md).

### Q4. Notice board → **defer**
Perttu: *"defer notice board now — it's a very different system."*
**Decision:** the notice board is out of the near-term plan. Stage-1 communication is the brief
(inline) + the run ledger + conversational status via the Archon. Escalation stays minimal
(ledger + the Archon surfacing it; TTS later). See [06](06-shared-state-and-escalation.md), now marked
deferred.

### Q5. First cross-harness pair (still open, low-stakes)
Which two harnesses for the first real run. **Recommendation: Claude Code + Codex** (the proven pair),
optionally proving the sentinel/timeout mechanics same-harness first. Not blocking.

### Q5. First cross-harness pair
Which two harnesses for the proof? Claude Code + Codex is the pair you've already run together.
**Recommendation: Claude Code + Codex.** Optionally de-risk by proving the sentinel/timeout mechanics
*same-harness* first (Claude→Claude with a different persona card), then immediately do the
cross-harness run in the same phase. Headline acceptance stays cross-harness.

---

## Tier 2 — decide during implementation (doesn't block starting)

### Q6. Write strategy for human tweaks → **settled by Q1**
Because execution is hosted in the server, the **server's formations package is the single writer** of
definition files; the UI's `PATCH` and the `archon` CLI both go through it (the CLI is a client of
`/api/formations/*`). No cross-process shelling, no format drift. v1 is read-only, so no writer is
exercised yet.

### Q7. Escalation reach beyond TTS
TTS is great at the machine, useless when you're away. Add a Discord/voice-channel mirror (the `/srv`
voice infra exists) or phone push for AFK escalations?
**Recommendation: TTS-only for slice 1; add a Discord mirror as a tiny second fan-out target only if
you find you miss escalations while away.** Don't build push infra speculatively.

### Q8. Sentinel-line convention
Completion/escalation are detected via a sentinel line the brief instructs agents to emit
(`<<<CHROTE-DONE turn=…>>>` / `<<<CHROTE-ESCALATE …>>>`). It's a small prompt-discipline tax on every
brief.
**Recommendation: yes** — it's the highest-leverage reliability mechanism (the only robust completion
signal absent a native ACK) and doubles as the artifact hand-off channel (`artifact=/path`). Trivially
removed once native adapters give real ACKs.

### Q9. Multi-kind gate verdict
When a gate combines kinds (e.g. Code + Human), is the verdict strict AND, or can a human override a
failing Code check?
**Recommendation: strict AND, fail-loud.** Override is a "safeguard" you said to skip; a human who
disagrees with a red test edits the criterion and re-runs (visible), rather than us building an
override path. (Moot until gates ship — post-spine.)

### Q10. Ledger text storage
Store full prompt+reply text in the ledger (best audit + offline "ask the agents"), or just references
(capture-file paths)?
**Recommendation: full text by default** (localhost, single-user), with a per-run `redact` flag that
swaps bodies for length+hash. Revisit before any non-localhost exposure.

### Q11. Per-run fail-loud limits
A per-run max-dispatch count + wall-clock timeout prevent runaway loops/cost. They *record-and-stop*,
they don't *ask permission*.
**Recommendation: yes — these honor "no safeguards"** (no gating/approval) while preventing the
runaway-cost failure mode (itself a named escalation trigger in vision §16). Confirm this reading of
"no safeguards."

---

## Tier 3 — settled with a default (object if you disagree)

| # | Question | Default |
|---|---|---|
| Q12 | Serialization format | **TOML** for definitions/cards/teams, **NDJSON** for ledger/board. (Data-model + registry agents aligned.) |
| Q13 | Persona home | **central `~/agents/`** (identities span projects); allow a project-local override only if a real need appears. |
| Q14 | Archon session in CHROTE | **show + launch-on-demand** (no always-on process you didn't ask for). |
| Q15 | Dynamically-assembled teams | **persist-on-demand** (`archon team save`); ephemeral by default, so `~/agents/teams/` stays curated. |
| Q16 | Evaluation ledger granularity | **one shared `interactions.jsonl`** + a `persona` field; derive per-persona views by filtering. Don't fragment. |
| Q17 | Reusable formation templates | **same schema** as a live board, flagged `kind="template"`; instantiate by copy + fresh ids. No second schema. |
| Q18 | Cross-board references (a judge/handoff in another board) | **deferred**; the id scheme is already forward-compatible (`<boardId>/<nodeId>:<port>`). |
| Q19 | Board scoping | **per-mission board + one global board**; no per-team boards. |
| Q20 | Who launches Hermes-profile sessions | **the run engine** (via the card's `launch`); CHROTE owns the card + reference, not the launch. |

---

## Next step — the Beads epic (vertical slices)

The work becomes a Beads epic with the [master-plan §5](00-master-plan.md) **vertical-slice** structure
(tracked in `bd`, per `CLAUDE.md` — not markdown). Each child bead is one full-stack slice (CLI + files
+ API + UI together):

1. **Slice 1 — see a formation:** `.formations/` schema + server formations package (read/write owner)
   + `archon agent list` / `formation create|list|inspect` + `GET /api/formations/boards/{id}` +
   read-only Formations tab (flag `chrote-formations-tab`) + the round-trip test. *(includes 3 real
   persona cards incl. `archon`, and the `/api/agents` left-join onto Oracle.)*
2. **Slice 2 — assign an agent:** `archon formation assign` + the UI drag gesture → field-level
   write-back round-trip (If-Match etag, layout sidecar for moves).
3. **Slice 3 — spawn & attach:** `archon agent spawn <harness> --name` / `attach` + the UI equivalents.
4. **Slice 4 — build a mission:** `archon mission create|wire` + its backing bead + the UI chain view.
5. **Slice 5 — run it (cross-harness):** run engine + tmux adapter + sentinel + ledger + SSE streaming;
   `archon mission run` / `run logs`; peer formation, JOIN-of-two; the crash-resume test.
6. **Slice 6 — recover & decide:** `archon run resume|abort`, `archon gate approve|reject` + UI.

Acceptance for the whole: the career/web-experience scenario (master-plan §11) and the clean-rollback
test, each behind the feature flags.
