# Formations proving missions (chrote-3af)

Status: DESIGN ONLY. Date: 2026-07-24. Branch: `design/executor-as-perttu`.
Gated on `chrote-jkk` (executor-as-perttu) landing so a live agent actually
dispatches. This spec defines the two proving missions to run *once the executor
authenticates as perttu*.

Purpose (from `chrote-3af`): prove one real agent-authored Formations workflow
end to end — an agent authors it via Archon, it runs on the cockpit tmux pool,
gates evaluate, the operator watches **honest board state** + **inspects
evidence**, and it ends in a **real deliverable**. Each mission must exercise the
three things that already shipped: the **honest board** (run state on the canvas,
not buried in logs), the **evidence inspector** (durable artifacts + node
inspection identifying the exact run attempt), and the **machine gate** (the
certified pure in-process `code` Gate profile).

Vocabulary is `FORMATIONS.md` §Core-nouns: Agent, Formation, Slot, Mission, Tool,
Board, Connection, Port, Payload, **Gate** (human/code/judge evaluator+router,
never a transformation), Run, Ledger. Gate kinds: `code` (machine, certified
pure evaluator), `judge` (agent/LLM judgment via reserved `judge` control edge),
`human` (operator verdict via `archon gate approve|reject`). Agents are
`claude-code` harness cards; the executor spawns a session per dispatch running
the card's harness-variant `launch` command, and the agent must emit its result
as a fenced ` ```chrote-outputs ` JSON block followed by the `CHROTE-DONE`
sentinel (per `tmux_executor.go` completion protocol).

Common preconditions (both missions):
- Executor authenticates as perttu (`chrote-jkk` Stage 3+ verified).
- `CHROTE_FORMATIONS_TMUX_HARNESSES` includes the `claude-code` variant used by
  the cards; `CHROTE_FORMATIONS_TMUX_CWD`/`..._ROOTS` cover the mission workspace
  and its evidence root so artifact refs hydrate.
- A `bd` proving bead exists per mission for the deliverable to close against.

---

## Mission A — Coding mission (machine + judge gates, correction loop)

**Objective:** an agent plans, implements a small real change in a scratch repo,
and the change loops through a **lint/code gate (machine)** and a **review gate
(judge)** until both pass — producing a real diff artifact.

### Board shape

```
Mission: "Implement <small feature> in <scratch repo>"
  └─out──▶ Formation "planner"        (solo, claude-code)  ── plan ──▶
           Formation "implementer"    (solo, claude-code)  ── diff ──▶
             ├──▶ Gate "lint"   (code / machine)  ──fail(gate_feedback)──┐
             │                                     └─pass─▶               │
             └──▶ Gate "review" (judge, claude-code)                      │
                    ├─fail(gate_feedback)──────────────────────────────┐ │
                    └─pass─▶ Mission deliverable (diff artifact)        │ │
   correction wire: lint.fail + review.fail ──▶ implementer.input ◀────┴─┘
```

- **planner** (Slot → agent `plan-claude`, claude-code): reads the mission
  brief + repo, emits a short plan as a `work` payload (`text/markdown`).
- **implementer** (Slot → agent `impl-claude`, claude-code): consumes the plan,
  writes code in the scratch repo, and emits the **diff as a safe file artifact**
  (registered evidence) plus a `work` payload referencing it.
- **Gate "lint" (`code` / machine):** the certified pure in-process code-Gate
  profile evaluates a deterministic lint/format/build result over the produced
  artifact. This is the **machine gate** exercise — `RunGateBinding` freezes the
  profile/policy/determinism hashes; verdict is RFC-8785 canonical
  `{verdict,reason,evidence}`. On `fail` it emits `gate_feedback` routed back to
  `implementer.input` (a `gate_feedback` Formation input) for a correction attempt.
- **Gate "review" (`judge`):** a `claude-code` reviewer agent judges the diff
  against acceptance criteria via the reserved `judge` control edge; `pass`
  delivers the diff to the Mission out, `fail` emits `gate_feedback` back to the
  implementer.
- **Correction loop:** the run engine's retry/resume policy drives the loop.
  Because retryability is engine policy (never automatic, per §Port-payload), the
  loop is designed as gate-`fail` → `gate_feedback` → implementer re-dispatch,
  bounded by immutable max-dispatch/max-attempt limits so it terminates loud
  (`run_limit_exhausted`) rather than spinning.

### Agent cards (claude-code harness)

| Card | Role | Harness | Notes |
| --- | --- | --- | --- |
| `plan-claude` | planner | claude-code variant | read-only + short plan; low blast radius |
| `impl-claude` | implementer | claude-code variant | writes in scratch repo only (cwd/roots fenced) |
| `review-claude` | judge for "review" Gate | claude-code variant | verdict + reason as `chrote-outputs` |

Scope guard: the scratch repo is a **throwaway workspace** under the configured
formations roots — never the CHROTE repo itself. The agent runs as perttu with
perttu's fs permissions (no OS sandbox); the roots fence is a CHROTE
read/write guard, so the workspace must be deliberately small.

### Success criteria (Mission A)

1. `plan-claude` produces a plan artifact; `impl-claude` produces a **diff
   artifact** that is durably registered evidence.
2. The **machine lint Gate** fails at least once on a deliberately imperfect first
   attempt, emits `gate_feedback`, and the implementer's correction attempt makes
   it pass — proving the code-Gate evaluator + correction wire, not a straight-line
   happy path.
3. The **judge review Gate** renders a verdict with a safe reason/evidence and
   routes correctly on both `pass` and `fail`.
4. Run state is legible on the **honest board** at each step (which node is
   dispatched, which gate failed, which attempt is live) — not only in logs.
5. **Evidence inspector**: the operator can open the failing gate's node and
   identify the exact run attempt, the evaluated input ref, and the durable diff
   artifact.
6. Ends `run_succeeded` with the diff as the Mission's output artifact; the loop
   terminates by success (or fails loud on limit, never silently).

---

## Mission B — Writing mission (LinkedIn post: candidates → judge → humanizer → owner verdict)

**Objective:** produce a genuinely publishable LinkedIn post about CHROTE.
Three candidate drafts → a **judge gate** picks/ranks → a **humanizer gate**
(local `humanizer` skill) de-slops the winner → an **owner human gate** renders
the final verdict → a **markdown artifact** deliverable.

### Board shape

```
Mission: "LinkedIn post about CHROTE"
  └─out──▶ Formation "drafters" (peer or 3× solo, claude-code) ── 3 candidates ──▶
           Gate "judge"  (judge, claude-code)  ── ranked winner ──▶
           Formation "humanizer" (solo, claude-code w/ humanizer skill) ── de-slopped draft ──▶
           Gate "owner"  (human)  ├─reject(gate_feedback)──▶ back to drafters/humanizer
                                  └─approve─▶ Mission deliverable (markdown artifact)
```

- **drafters** (Slot(s) → `draft-claude`, claude-code): produce **3 distinct
  candidate posts** as `work` payloads (`text/markdown`). Modeled as a `peer`
  formation (facilitator + peers) or three `solo` dispatches, each candidate a
  separate registered artifact.
- **Gate "judge" (`judge`):** a `claude-code` judge ranks the three against
  explicit criteria (accuracy about CHROTE, hook, no fabrication) and routes the
  **winner** forward; losers stay inspectable evidence, not work.
- **Formation "humanizer" (solo, claude-code):** runs the **local `humanizer`
  skill** to strip AI tells / de-slop the winning draft, emitting the humanized
  markdown as an artifact. (This is the concrete "humanizer gate" the bead names;
  implemented as a transformation Formation feeding the owner gate — a Gate is
  never a transformation, so the humanizing *work* is a Formation and the
  *decision* is the owner Gate.)
- **Gate "owner" (`human`):** the operator renders the verdict via
  `archon gate approve|reject`. `approve` delivers the markdown artifact as the
  Mission out; `reject` emits `gate_feedback` for another humanizer/drafter pass.
  This is the **needs-you / operator-pull-in** moment.

### Agent cards (claude-code harness)

| Card | Role | Harness | Notes |
| --- | --- | --- | --- |
| `draft-claude` | drafters | claude-code variant | 3 candidates; markdown only |
| `judge-claude` | judge Gate | claude-code variant | ranked verdict + reason |
| `human-claude` | humanizer Formation | claude-code variant + `humanizer` skill | de-slop winner |

### Success criteria (Mission B)

1. Exactly **3 distinct candidate** artifacts are produced and durably
   registered.
2. The **judge Gate** ranks them, routes one winner, and its reason/evidence is
   inspectable; losers remain inspectable non-work evidence.
3. The **humanizer** Formation measurably changes the winning draft (AI-tell
   reduction) and emits a humanized markdown artifact.
4. The **owner human Gate** actually pulls the operator in (a visible
   needs-you/decision on the board) and both `approve` and `reject` route
   correctly; a `reject` drives a real correction pass.
5. **Honest board**: the operator sees candidates fan in, the judge's pick, the
   humanizer pass, and the pending owner decision as board state.
6. **Evidence inspector**: every candidate, the judge verdict, the pre/post
   humanized drafts, and the final are inspectable with their exact run attempt.
7. Ends `run_succeeded` with a **markdown artifact** — a post the owner would
   actually publish.

---

## How each mission exercises the three shipped capabilities

| Shipped capability | Mission A | Mission B |
| --- | --- | --- |
| **Honest board** (run state on canvas) | plan→impl→gate loop visible; live attempt highlighted | 3-way fan-in, judge pick, humanizer pass, pending owner gate visible |
| **Evidence inspector** (durable artifacts + exact attempt) | diff artifact + failing-gate input ref + attempt id | candidates, judge verdict, pre/post humanized drafts, final |
| **Machine gate** (certified pure `code` evaluator) | the **lint Gate** (primary machine-gate proof) | judge/human gates dominate; a `code` Gate can optionally enforce length/format determinism |

Mission A is the primary **machine-gate + correction-loop** proof. Mission B is
the primary **judge + human-gate + real-deliverable + operator-pull-in** proof.
Together they cover code (`code`), agent judgment (`judge`), and operator decision
(`human`) gate kinds, plus a real correction loop and two genuinely useful
deliverables.

## Authoring path (agent-first, per ADR-0006)

Each mission's board is **authored by an agent through Archon** (the proof the
epic asks for), using the current verbs:
`archon board new` → `archon agent new` (the cards) →
`archon mission create` / `archon formation create` + `assign` / `set-brief` →
`archon gate create` (lint/review/judge/owner) → `archon formation wire` to
connect ports and the reserved `judge`/`fail` `gate_feedback` edges →
`archon board validate` → `archon mission run`, then
`archon run status|follow` and `archon gate approve|reject` for the human gate.

> Dependency on `chrote-jkk`: `archon mission run` and the gate verdict verbs are
> the runtime-authority boundary. The proving run only dispatches a **live agent**
> once the executor authenticates as perttu. Until then these missions are
> authorable/validatable offline (Archon authoring is independent of runtime) but
> not runnable end to end. Also note the doc-contract reconciliation flagged in
> the executor-as-perttu spec: `ARCHON.md` still states "Schema-2 runtime remains
> disabled" / runtime verbs "non-authorizing," which the deployed neutralized
> authority guard contradicts — the same superseding-ADR reconciliation unblocks
> both specs.

## Open items for the owner

- Confirm the **scratch repo / workspace** for Mission A (throwaway, under
  formations roots, never the CHROTE repo).
- Confirm the **CHROTE facts** the LinkedIn post may state (avoid fabrication;
  the judge criteria should include accuracy).
- Decide whether Mission A's lint Gate uses an existing certified `code` profile
  (only `json.normalize@1` is in the current closed catalog) or needs a new
  certified profile — the latter is `chrote-c3b` (machine code-Gate profiles) and
  may gate Mission A's machine-gate step. Flag: if no lint-shaped `code` profile
  exists yet, Mission A's machine-gate proof depends on `chrote-c3b` finishing, or
  substitutes a format/length-determinism `code` Gate that the current catalog
  can express.
