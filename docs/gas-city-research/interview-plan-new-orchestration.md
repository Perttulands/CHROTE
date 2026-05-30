# Interview Plan For New Agent Orchestration Direction

Status: research/interview plan, not an implementation plan.
Date: 2026-05-30

This document distills why Gas City/Gastown looked attractive, why the attempt
did not work, and how to interview Perttu before making a new plan.

The active decision remains
[ADR-0003: Roll Back Active Gas City Integration](adr/0003-rollback-active-gascity-integration.md).
Gas City is not installed or assumed live. Nothing in this file reintroduces a
Gas City runtime, `gc` command, CHROTE Gas City tab, or implementation backlog.

## Inputs Used

This plan was generated from:

- the retained Gas City research folder;
- current CHROTE product docs;
- removed Gas City Beads snapshot records;
- three focused Codex subagent reviews;
- one Claude Code `opus` / `max` read-only synthesis.

Most important source files:

- `what-we-were-trying-to-do.md`
- `framing.md`
- `vision/agent-orchestration-vision.md`
- `evaluations/meta-harness-evaluation.md`
- `adr/0001-chrote-3-gas-city-substrate.md`
- `adr/0002-gascity-real-harness-safety-boundary.md`
- `adr/0003-rollback-active-gascity-integration.md`
- `architecture/substrate-map.md`
- `architecture/projection-schema.md`
- `experiments/sidecar-spike-results.md`
- `experiments/real-identity-smoke.md`
- `experiments/dolt-quorum-validation.md`
- `/home/perttu/chrote/docs/meta-harness-desired-state.md`
- `/home/perttu/chrote/docs/agent-collaboration-primitives.md`
- `/home/perttu/rollback-snapshots/gascity-beads-removal-20260530-221903/`

## Core Synthesis

Perttu did not want Gas City as a destination or dashboard.

The attractive idea was agent-facing orchestration plumbing underneath CHROTE:
named identities, durable mail, live nudges, delegation, formulas, molecules,
events, and workflow state that agents could use while doing normal work.

The felt win was:

1. Perttu works with one named agent through CHROTE.
2. That agent involves another named or disposable agent when useful.
3. Agents coordinate through durable primitives instead of manual tmux pane
   targeting, transcript scraping, or copy/paste between sessions.
4. The collaboration improves the quality of planning, Bead creation, execution,
   review, and verification.

The failed direction was:

1. Building a Gas City tab, workflow launcher, and passive dashboard before
   proving the real collaboration loop.
2. Treating mock-agent plumbing as stronger evidence than it was.
3. Not solving the central identity/session binding problem cleanly enough.
4. Letting adapters, runtime state, and rollback complexity accumulate before a
   small real-agent slice proved value.
5. Polluting active Beads and docs so future agents could not easily tell what
   was live versus historical.

The next plan should invert that failure pattern. It should start with a tiny
real-agent collaboration proof, keep UI minimal, and make identity binding the
central design problem rather than an implementation detail.

## Working Hypotheses To Verify

Use the interview to test these hypotheses. Do not treat them as settled.

### H1: The First Valuable Slice Is Quality Amplification

The first useful workflow is not "launch a workflow from a UI." It is the lived
pattern where a primary agent is improved by another agent at each stage:

- planning;
- Bead creation;
- execution;
- verification;
- synthesis/closure.

### H2: CHROTE Should Mainly Provision And Access Identities

If orchestration returns, CHROTE's first useful role is not observability. It is
making named agent identities easy to create, access, and recover.

Possible actions:

- create a plain tmux session;
- create a named agent identity;
- identify whether a session is plain or identity-backed;
- let an agent spawn a disposable teammate.

### H3: The Missing Primitive Is Group Awareness

One-to-one mail and delegation are not enough for teams. The Master Board idea
may matter because it gives one agent, Perttu, or a strategy agent a way to
surface something to the wider group.

It should be a low-structure team noticeboard, not a task tracker. Work remains
in Beads.

### H4: Most Web UI Should Fail The "Just Open The Session" Test

If Perttu would rather ask an agent to explain or simply open the session, do
not build a CHROTE web surface for it.

Useful UI is likely limited to:

- access to named sessions;
- identity creation when spawning sessions;
- team composition if it beats doing the same verbally;
- pull-based search/recovery once real communications exist.

### H5: Gas City May Be A Reference, Not The Answer

Gas City had strong primitives, but direct adoption created confusion and
complexity. The interview should separate:

- primitives Perttu still wants;
- product experience Perttu wants;
- whether those primitives should come from Gas City, CHROTE-native code, Beads,
  a smaller adapter, or a future different system.

## Interview Goals

The interview should produce enough clarity to write a new plan without
repeating the failed pattern.

By the end, we should know:

1. The smallest valuable real-agent collaboration slice.
2. What "real proof" means and what evidence does not count.
3. The identity lifecycle model.
4. The Master Board ownership and boundaries.
5. The minimal CHROTE UI surface that is worth building.
6. Which Gas City concepts remain valuable even if Gas City is not reused.
7. Which implementation routes are acceptable to consider next.

## Things Not To Re-Litigate

These points were already clarified and should be treated as defaults unless
Perttu changes them:

- Gas City should not be a CHROTE destination tab.
- Perttu does not want to launch workflows from a Gas City web form.
- Passive transcript watching is not the purpose.
- Existing old tmux sessions do not need migration.
- Beads remains the work ledger.
- Context Citadel remains durable context and knowledge.
- CHROTE's baseline job is easy access to named tmux sessions.
- If collaboration goes wrong, Perttu expects to ask agents to explain before
  CHROTE tries to diagnose it automatically.
- Phase 1 should not overbuild budgets, approval gates, or speculative security
  policy around ordinary agent spawning/delegation.

## Interview Structure

### Part 1: Reconfirm The Felt Win

Goal: pin down the first human-visible success.

Questions:

1. If we built only one thing next, which outcome would make you say "yes, this
   captures why Gas City was attractive"?
2. Is the first win:
   - one agent asking another agent for help;
   - a second-agent review loop through planning, Beads, execution, and
     verification;
   - agent-spawned disposable teammates;
   - Team Builder;
   - Master Board;
   - something else?
3. In that first success, what do you personally do in CHROTE?
4. What do the agents do without you manually carrying messages?
5. What output quality difference would make the collaboration obviously worth
   the extra machinery?

Decision output:

```text
First felt-win slice:
Why it matters:
What Perttu does:
What agents do:
What proves quality improved:
```

### Part 2: Define Real Evidence

Goal: avoid mock-driven overclaiming.

Questions:

1. What evidence would count as a real agent collaboration proof?
2. Which harnesses must be involved for the proof to feel real: Claude Code,
   Codex, Pi, OpenCode, Hermes, or plain tmux?
3. Is a shell script or mock agent ever acceptable in the proof, or only as a
   supporting unit test?
4. Does the result need to return through a durable message/mail system, or is a
   verified transcript response acceptable for slice 1?
5. What should we explicitly refuse to claim until proven?

Decision output:

```text
Required real harnesses:
Allowed test doubles:
Required return path:
Evidence that counts:
Evidence that does not count:
```

### Part 3: Identity And Session Lifecycle

Goal: resolve the hard problem the rollback exposed.

Questions:

1. Should named identities be born as identities, or can an existing plain tmux
   session be promoted?
2. Who creates identities in phase 1: Perttu, agents, or both?
3. Should disposable helpers exist in phase 1, or only long-lived named agents?
4. What makes an identity valid: name, mailbox, tmux session, harness config,
   environment, Beads ownership, something else?
5. What should an agent be able to say to involve another agent?
6. Does the phrase "Codxia, help Claudia get this done" imply a standing
   identity registry, ad hoc spawning, or both?

Decision output:

```text
Identity types:
Creation paths:
Promotion allowed:
Minimum valid identity fields:
Agent invocation language:
```

### Part 4: Master Board And Team Awareness

Goal: clarify the broadcast primitive without creating a second task tracker.

Questions:

1. What should agents post to the Master Board?
2. What must never live on the Master Board because it belongs in Beads?
3. Is the board per project, per workflow, per team, or global?
4. Who can post: Perttu, leader agents, all team agents, disposable helpers?
5. Should board posts be ephemeral awareness signals or durable records?
6. Where should it live conceptually: Beads message records, CHROTE-native file,
   Context Citadel candidate/context, or a new lightweight store?
7. How should an agent know when to read the board?

Decision output:

```text
Board purpose:
Allowed post types:
Forbidden post types:
Scope:
Writers:
Storage candidate:
Relationship to Beads:
```

### Part 5: Team Builder And Leader-Orchestrated Work

Goal: decide whether a team composition surface is useful now or later.

Questions:

1. Would you use a graphical Team Builder before agents can already collaborate
   through prompts?
2. What does Team Builder do that a conversation with a leader agent cannot do?
3. Is the common pattern one leader orchestrating others?
4. Should the leader be chosen by Perttu, suggested by an agent, or implied by
   the current session?
5. Should team composition create durable identities, temporary identities, or
   only a plan?

Decision output:

```text
Team Builder phase:
Leader selection:
Team composition output:
Reason UI beats conversation, if any:
```

### Part 6: Delegation, Autonomy, And Boundaries

Goal: clarify phase-1 freedom without overbuilding guardrails.

Questions:

1. Can agents spawn teammates freely in phase 1?
2. Can agents spend model calls freely in phase 1?
3. Can agents assign or claim Beads for themselves or others?
4. Can agents edit files as part of delegated work?
5. Are there any hard "never autonomously do this" actions, such as pushing,
   deploying, killing sessions, changing services, or touching secrets?
6. When should an agent ask Perttu before delegating?

Decision output:

```text
Allowed autonomous actions:
Ask-first actions:
Never-autonomous actions:
Phase-1 non-goals:
```

### Part 7: Build, Borrow, Or Revisit Gas City

Goal: separate desired primitives from a specific dependency.

Questions:

1. Which Gas City/Gastown primitives still feel important?
2. Which primitives are only interesting because Gas City had them already?
3. If we do not reinstall Gas City, what is the smallest native CHROTE/Beads
   equivalent worth considering?
4. If we do revisit Gas City, should the route be:
   - sidecar/API;
   - CLI adapter;
   - SDK/library;
   - copied concept only;
   - upstream contribution first;
   - no reuse for now?
5. Are you willing to take on Dolt or another runtime dependency if it buys real
   orchestration leverage?
6. What would make us decide "do not use Gas City, just keep the ideas"?

Decision output:

```text
Primitives to keep:
Preferred reuse route:
Dependencies acceptable:
Reasons to reject Gas City:
```

### Part 8: Minimal CHROTE Surface

Goal: prevent another Gas City tab mistake.

Questions:

1. What should CHROTE show by default when agent collaboration exists?
2. What should only be searchable/pull-based?
3. What should never be in CHROTE because an agent should explain it instead?
4. Should "spawn session" become "spawn plain session or named identity"?
5. Would a communications search be useful only after there is real traffic?
6. What is the smallest UI change that would help the first felt-win slice?

Decision output:

```text
Default UI:
Pull/search UI:
No-build UI:
First UI change:
```

## Interview Output Template

At the end of the interview, write a short decision memo in this shape:

```markdown
# New Agent Orchestration Direction - Interview Result

Date:
Status: interview result, not implementation plan

## First Felt-Win Slice

## Proof Standard

## Identity Model

## Master Board Boundary

## Team/Leader Model

## Autonomy Boundary

## Gas City Concept Disposition

## Minimal CHROTE Surface

## Open Questions

## Next Planning Step
```

Only after that memo exists should a new implementation plan or Beads epic be
written.

## Planning Guardrails After The Interview

Any new plan should obey these guardrails:

- Start with one real-agent vertical slice.
- Do not build a standalone orchestration tab.
- Do not use mocks as acceptance evidence for real collaboration.
- Treat identity/session binding as the central design problem.
- Keep Beads as work truth and Context Citadel as context truth.
- Do not recreate deleted Gas City implementation Beads as active backlog.
- Make rollback/untangling simple from the first slice.
- Prefer fewer primitives with a clear felt win over a broad orchestration
  platform that is hard to trust.
