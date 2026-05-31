# Interview Plan For New Agent Orchestration Direction

Status: research/interview plan, not an implementation plan.
Date: 2026-05-30

This document distills why Gas City/Gastown looked attractive, why the attempt
did not work, and how to interview Perttu before making a new plan.

The interview is not primarily requirements gathering. It is vision discovery.
The interviewer should help Perttu uncover why he wants this system, what desire
and frustration sit underneath it, what kind of future he is trying to create,
and what agents can infer or elaborate beyond what he has already articulated.

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

The attractive idea was not "integrate Gas City." The attractive idea was a
more powerful way to work with agents: named collaborators, team formation,
delegation, shared awareness, reusable review loops, and higher-quality outcomes
without Perttu manually carrying context between terminal panes.

Gas City looked useful because it contained primitives that seemed to point
toward that future: identity, mail, nudge, delegation, formulas, molecules,
events, workflow state, and runtime supervision. Those are implementation
clues, not the starting point.

The starting point is the why:

1. What frustration in the current agent workflow makes this worth pursuing?
2. What kind of work does Perttu want to become capable of?
3. Why are solo agents, one-off subagents, and manual tmux choreography not
   enough?
4. What would feel different if agents became a real working team?
5. What is the larger personal AI infrastructure vision this belongs to?
6. What does Perttu sense but not yet have words for?

The failed direction was partly technical, but the root failure was sequencing:
we moved from interesting primitives to implementation before fully discovering
the desire, taste, ambition, and lived workflow that should shape the system.

The next planning process must invert that. Interview first for motive and
vision. Translate that into capabilities only after the desire is clearer. Ask
implementation questions last.

## Interview Stance

The interviewer is not a product manager collecting feature requests.

The interviewer should act as a sharp collaborator trying to help Perttu
discover and articulate a vision he may not yet be able to state directly.

Good interview behavior:

- ask "why?" repeatedly, but not mechanically;
- ask for stories from real work, not abstract preferences;
- notice emotional charge, frustration, excitement, and aversion;
- surface contradictions instead of smoothing them over;
- propose language back to Perttu and ask whether it lands;
- distinguish what Perttu explicitly says from what agents infer;
- help elaborate the vision beyond current vocabulary;
- postpone implementation until the desired transformation is clear.

Bad interview behavior:

- asking "how should we implement this?" too early;
- forcing choices between storage, APIs, UI components, or dependencies;
- treating Gas City primitives as the ontology of the vision;
- turning every desire into a feature;
- assuming the current CHROTE/Gas City vocabulary is the right vocabulary;
- optimizing for an easy plan instead of a true one.

## Working Hypotheses To Explore

Use the interview to test these hypotheses. Do not treat them as settled.

### H1: The Desire Is For A Higher-Order Working Relationship With Agents

The deeper goal may be that agents stop feeling like isolated tools and start
feeling like members of a working system: named, addressable, cooperative, and
able to improve each other's work.

### H2: The Pain Is Manual Coordination And Lost Leverage

Perttu may already be getting value from multiple agents, but the coordination
cost is too high: copying context, nudging panes, remembering who knows what,
and manually turning separate outputs into a better whole.

### H3: The Prize Is Not Automation For Its Own Sake

The point may not be "agents do more without me." The point may be better
thinking, better review, better continuity, and better execution because agents
can collaborate in patterns Perttu trusts.

### H4: CHROTE Is Valuable When It Supports The Relationship

CHROTE matters when it gives Perttu access to the working system: named
sessions, recovery, intervention, and maybe team composition. It stops mattering
when it becomes a dashboard Perttu must operate instead of working with agents.

### H5: Gas City Was A Glimpse Of A Missing Layer

Gas City may have been attractive because it revealed a missing layer in the
personal AI stack: not chat, not terminal access, not memory, not task tracking,
but orchestration between working agents.

### H6: The Vision May Need New Vocabulary

"Gas City," "Gastown," "molecules," "mail," "Master Board," "Team Builder," and
"identity" may be temporary handles. The interview should allow better language
to emerge.

## Interview Goals

The interview should produce enough clarity to write a new plan without
repeating the failed pattern.

By the end, we should know:

1. Why Perttu wants agent orchestration at all.
2. What current pain, ceiling, or ambition creates the desire.
3. What kind of future working relationship with agents he is imagining.
4. What new capability would feel meaningfully different.
5. What examples from recent work reveal the desired pattern.
6. What qualities the system must have to feel trustworthy and worth using.
7. Which concepts from Gas City remain resonant after stripping away
   implementation details.
8. Which questions are still too implementation-shaped and should wait.

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

### Part 1: Why This Matters

Goal: uncover the motivation before naming mechanisms.

Questions:

1. Why do you want agents to collaborate?
2. Why is one strong agent not enough?
3. Why is manual coordination through tmux, files, and chat not acceptable as
   the long-term way?
4. What frustration keeps recurring in your current agent workflow?
5. What do you keep trying to make happen that the current setup resists?
6. If this system worked, what would become easier, deeper, faster, or more
   ambitious?
7. Why does this matter enough to build infrastructure around it?

Interview notes:

```text
Core frustration:
Core desire:
Why current tools are insufficient:
What becomes possible:
Emotional charge / repeated language:
```

### Part 2: The Desired Future

Goal: let the vision expand before narrowing it into a first slice.

Questions:

1. Imagine this system working a year from now. What are you doing with it?
2. What are agents doing that they cannot do for you today?
3. What kind of projects become possible?
4. What kind of thinking becomes possible?
5. What would make this feel like a personal AI infrastructure breakthrough
   rather than another tool?
6. What would surprise or delight you if agents could do it together?
7. What would make you trust the system enough to lean on it?

Interview notes:

```text
Future scene:
New project capability:
New thinking capability:
Trust condition:
Words/images/metaphors Perttu uses:
```

### Part 3: Stories From Real Work

Goal: ground the vision in lived examples.

Questions:

1. Tell the story of a recent task where multiple agents helped.
2. Where did collaboration improve the result?
3. Where did you become the manual message bus?
4. Where did context get lost?
5. Where did a second agent catch something important?
6. Where did the process feel powerful despite being awkward?
7. What did you wish the agents could do for each other in that moment?

Interview notes:

```text
Story:
Moment of leverage:
Moment of friction:
What agents could not do:
What the operator had to carry:
```

### Part 4: Quality, Taste, And Trust

Goal: define "better" in human terms before defining mechanisms.

Questions:

1. What does "higher quality output" mean to you?
2. Is the value better correctness, better judgment, broader perspective,
   creativity, verification, confidence, speed, or something else?
3. What makes agent collaboration feel serious rather than noisy?
4. What kind of agent disagreement is useful?
5. What kind of agent collaboration wastes your time?
6. What makes you trust an agent team's result?
7. What makes you distrust it?

Interview notes:

```text
Quality means:
Useful disagreement:
Noise pattern:
Trust signal:
Distrust signal:
```

### Part 5: Agent Relationships And Roles

Goal: discover the human mental model behind names, teams, and identities.

Questions:

1. Why do names like Claudia, Codxia, or Lucy matter?
2. Do you imagine agents as tools, workers, collaborators, specialists,
   apprentices, reviewers, rivals, or something else?
3. What makes an agent worth keeping as a persistent identity?
4. When should an agent be disposable?
5. What should one agent know about another?
6. What does it mean for one agent to "help" another?
7. What does a leader agent actually do?

Interview notes:

```text
Role vocabulary:
Persistent identity criteria:
Disposable identity criteria:
Meaning of help:
Leader model:
```

### Part 6: Shared Awareness

Goal: understand the Master Board desire as a group-awareness concept, not a
storage problem.

Questions:

1. Why does one-to-one communication not cover all collaboration needs?
2. What kinds of things should be surfaced to the whole group?
3. What should a team know without any one agent being directly addressed?
4. What does "shared context for thinking" mean in practice?
5. What is the difference between a board post and a Bead?
6. What would make a shared board useful?
7. What would make it become clutter?

Interview notes:

```text
Need for group awareness:
Useful broadcast examples:
Difference from Beads:
Clutter risk:
```

### Part 7: Anti-Vision

Goal: sharpen the vision by naming what must not happen.

Questions:

1. What version of this system would make you angry?
2. What would feel like agents creating work instead of removing work?
3. What would feel like a dashboard replacing judgment?
4. What would make CHROTE worse?
5. What would make agent collaboration feel fake?
6. What would make the system impossible to untangle?
7. What must future agents remember not to do?

Interview notes:

```text
Anti-vision:
Dashboard trap:
Fake collaboration signal:
Untangling risk:
Hard no:
```

### Part 8: Translate Vision Into Capabilities

Goal: only now translate desire into candidate capabilities.

Questions:

1. Given the why, what capability seems most central?
2. Which capability is merely nice to have?
3. Which previous Gas City/Gastown idea still resonates after removing the
   implementation details?
4. Which term should we stop using because it pulls us toward the wrong shape?
5. What is the smallest capability that would prove we are moving toward the
   vision?
6. What should agents be able to do for each other first?

Interview notes:

```text
Central capability:
Secondary capability:
Resonant old primitive:
Vocabulary to retire:
Smallest meaningful proof:
```

### Part 9: Mechanism Questions For Later

Goal: keep implementation questions available, but clearly downstream.

Ask these only after Parts 1-8 produce a strong motivation and vision memo.

Questions:

1. What evidence would count as a real agent collaboration proof?
2. Which harnesses must be involved for the proof to feel real?
3. Should named identities be born as identities, or can existing sessions be
   promoted?
4. Should disposable helpers exist in the first slice?
5. Where should any shared board or broadcast surface live?
6. What should CHROTE show by default, if anything?
7. Which Gas City/Gastown primitives should be copied, adapted, reintroduced,
   or discarded?

Interview notes:

```text
Proof standard:
Harnesses:
Identity lifecycle:
Shared surface:
Minimal UI:
Primitive disposition:
```

## Interview Output Template

At the end of the interview, write a short decision memo in this shape:

```markdown
# New Agent Orchestration Direction - Vision Interview Result

Date:
Status: interview result, not implementation plan

## Why This Matters

## Desired Future

## Real Work Stories

## Quality, Taste, And Trust

## Agent Relationships And Roles

## Shared Awareness

## Anti-Vision

## Candidate Capabilities

## Open Questions

## Questions Deferred Until Planning
```

Only after that memo exists should a new implementation plan or Beads epic be
written.

## Planning Guardrails After The Interview

Any new plan should obey these guardrails:

- Start with one real-agent vertical slice.
- Do not build a standalone orchestration tab.
- Do not use mocks as acceptance evidence for real collaboration.
- Do not start by asking implementation questions.
- Treat identity/session binding as the central design problem.
- Keep Beads as work truth and Context Citadel as context truth.
- Do not recreate deleted Gas City implementation Beads as active backlog.
- Make rollback/untangling simple from the first slice.
- Prefer fewer primitives with a clear felt win over a broad orchestration
  platform that is hard to trust.
