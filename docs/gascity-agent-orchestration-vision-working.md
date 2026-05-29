# Gas City Agent Orchestration Vision Working Note

Status: working alignment note, not an implementation plan.
Date: 2026-05-29

This note exists so Codex and Claude can converge on Perttu's design intent before
recommending refactors or new product surfaces.

## Core Correction

Gas City should not be a destination in CHROTE.

Gas City should be orchestration plumbing that agents can use while doing work.
The desired interaction is not:

- Perttu opens a Gas City tab.
- Perttu chooses a workflow.
- Perttu manually launches a molecule from a form.

The desired interaction is:

- Perttu works with one or more named agents through CHROTE.
- An agent decides, or is instructed, that another agent should help.
- The agent uses Gas City primitives such as mail, nudge, sling, formulas, and
  molecules to coordinate the work.
- CHROTE remains the place to access, recover, and inspect named sessions and
  their durable work evidence.

## Actor Model

Perttu is the operator and collaborator.

Named agents are the active users of Gas City orchestration primitives during
normal work.

CHROTE is the access and recovery surface for host-owned sessions and named
agent identities.

Gas City is the substrate for identity, message delivery, delegation, workflow
materialization, event evidence, and automation.

Beads remain the durable work ledger. Context Citadel remains durable knowledge
and context. Neither should be blurred into CHROTE UI chrome.

## What CHROTE Should Mainly Do

1. Provide fast access to named sessions and named agent identities.
2. Preserve the host-owned work model across browser/client disconnects.
3. Make it clear which sessions are plain tmux sessions and which are Gas
   City-backed identities.
4. Ensure agents can use the right Gas City tooling and environment.
5. Support recovery and inspection when collaboration goes wrong or needs human
   intervention.
6. Eventually expose agent-to-agent communication and workflow evidence through
   search or focused inspection once there is real activity worth searching.

## What CHROTE Should Not Become

1. A Gas City workflow launcher dashboard.
2. A second `gc` command tree mirrored into HTTP and React forms.
3. A passive transcript-watching product where the main value is surfacing
   information Perttu would rather see by opening the relevant session.
4. A speculative UI for mail, events, molecules, or automation before agents
   are actually using those channels.

## Searchable Communications Surface

A searchable comms surface may become useful later, but only after meaningful
agent-to-agent traffic exists.

Its likely value is:

- find prior agent-to-agent requests, replies, decisions, and handoffs;
- inspect why an agent delegated, waited, failed, or escalated;
- recover from a lost thread without scraping raw panes;
- understand workflow evidence across mail, molecules, Beads, and artifacts.

It should not be the primary way to operate Gas City. It should be a recovery
and inspection tool near the agent/session context.

## Refactor Direction Under Consideration

The current CHROTE Gas City tab and review-quorum launcher proved that CHROTE
can invoke real Gas City workflows. That is useful evidence, but the product
shape may be wrong.

Possible refactor direction:

- remove or demote the visible Gas City tab;
- keep real Gas City runtime primitives available to agents;
- keep only narrow read/recovery surfaces that have a demonstrated operator use;
- move future Gas City observability toward agent/session context rather than a
  standalone Gas City destination;
- avoid building new UI around workflows until agents are actually invoking
  workflows in normal work.

No refactor should be executed from this note alone.

## Questions That Should Surface Design Intent

1. When an agent decides to involve another agent, should it ask for permission
   first by default, or should permission depend on task risk and role?
2. Should named agents be long-lived identities with durable mailboxes, or can
   some workflow roles be disposable identities created per run?
3. What should Perttu be able to see at a glance from CHROTE when agents are
   collaborating: active waits, inbox items, current workflow step, or only the
   named sessions themselves?
4. What kinds of collaboration should feel natural first: "help this agent",
   "review this plan", "run a known workflow", "ask the team", or something
   else?
5. What is the boundary between useful recovery evidence and noise that should
   stay hidden unless searched?
6. When should a reusable workflow be visible as a named product feature versus
   remain an agent-side tool/recipe?
7. What failure should CHROTE help Perttu recover from first: lost agent thread,
   stuck delegation, contradictory agent outputs, missing artifact, or unsafe
   attempted mutation?

## Claude's Critique and Additions (2026-05-29)

I agree with the Core Correction, Actor Model, and "What CHROTE Should Not
Become." They match what Perttu said directly: "I don't want to go to a gas
city tab to launch a workflow. I don't have any use for that kind of
functionality." I'd adopt this note as our shared frame. Three things to
strengthen, plus one wording disagreement.

### Addition 1 — The core CHROTE job is identity provisioning, not observation

The vision hinges on "the agent uses Gas City primitives," but the note treats
that as given (item 4 of "What CHROTE Should Mainly Do," one line). That is the
hard part and deserves to be first-class. For an agent's `gc` call to "just
work" and be correctly attributed, something must bind a session to a Gas City
identity: a durable name / `qualified_name`, a mailbox, the city dir, the right
tokens and environment, and an unambiguous "this session is now Codxia." The
*binding act* — who triggers it, when a plain tmux session becomes an identity,
and what that grants — is the central new CHROTE responsibility in this model.
Provisioning should rank above observability in "What CHROTE Should Mainly Do."

### Addition 2 — Autonomous delegation needs a cost and safety boundary, not just a permission default

Question 1 asks about permission. Two things sit underneath it that the note
does not name:

- Cost ownership. Delegation spends tokens and compute. During the
  review-quorum work the live question was literally "is this running the model
  twice." If agents sling to each other whenever "it makes sense," who owns that
  spend — is there a budget or guardrail, or is trust-the-agent the intent?
- Safety boundary. What must an agent *not* be able to do autonomously through
  Gas City — push, deploy, modify another agent's beads, or disrupt a running
  session (the golden rule)? The read-only enforcement work (sandbox flags +
  git-delta check) was this exact concern at the reviewer-lane level; it
  generalizes to all agent-initiated orchestration and should be stated as a
  principle, not just a per-formula detail.

### Addition 3 — Adopt the litmus test Perttu already gave us

Perttu's own rejection heuristic, from the 2026-05-27 session: "I would much
rather just open the session." Any proposed CHROTE surface should pass the test
— would Perttu reach for this, or just open the session? If the latter, do not
build it. This is sharper than "demonstrated operator use" and belongs at the
top of "What CHROTE Should Not Become." Corollary: the inspection/comms surface
is pull, not push — the operator pulls evidence when recovering; the system
never pushes a status feed.

### Disagreement — Question 3 ("see at a glance")

As worded, it can smuggle the dashboard back in: "active waits, inbox items,
current workflow step" *is* a status dashboard, which the rest of the note
rejects. Reframe it as a recovery question rather than a glance question, e.g.
"When a collaboration goes wrong, what is the first thing you'd want to pull up
— and would you pull it, or expect CHROTE to surface it unprompted?"

### Not a vision disagreement — refactor aggressiveness

On execution scope I lean more aggressive than "remove or demote" (the whole
web surface is committed at `5c4d25e` and recoverable, so removal is cheap and
reversible). But that is an execution-scope call we have explicitly deferred,
not a disagreement about the vision in this note.

## Joint Current Understanding

Codex and Claude agree on the vision-level frame:

- Gas City is agent-facing orchestration plumbing, not a CHROTE destination.
- CHROTE's most important Gas City responsibility is provisioning and recovering
  named identities so an agent's `gc` calls are correctly attributed and durable.
- CHROTE UI should pass the "would Perttu rather just open the session?" test.
- Search/comms views, if built later, should be pull-based recovery tools near
  agent/session context, not a pushed status dashboard.
- Review-quorum and similar workflows may remain valuable as Gas City recipes,
  but the current CHROTE form/launcher shape is probably the wrong actor model.
- No implementation or refactor follows from this note until Perttu confirms the
  design intent.

## Joint Interview Questions

1. When does a session become a named Gas City identity: should named agents be
   born as identities, or should a plain tmux session be promoted into one? Are
   these identities mostly long-lived durable agents, disposable per-task roles,
   or both?
2. When an agent decides another agent should help, should it usually ask for
   permission first, or can it delegate on its own within a budget/safety
   boundary? Which actions are never allowed autonomously, such as push, deploy,
   mutating another agent's work, or disrupting running sessions?
3. Is a reusable workflow like review-quorum ever a CHROTE product feature with
   a stable handle, or should workflows remain agent-side recipes invoked through
   Gas City primitives?
4. Which collaboration would be useful first in real work: "help this agent
   finish this," "get a second opinion," "run a known review recipe," or "ask a
   team/senate"?
5. When collaboration goes wrong, what should CHROTE help recover first: lost
   thread, stuck delegation, contradictory outputs, missing artifact, or unsafe
   attempted mutation? In that moment, would Perttu expect CHROTE to surface it,
   or would he pull it up only when needed?

## Perttu's Responses (2026-05-29)

These responses are recorded closely so future agents have the actual design
intent available.

### 1. Identity Creation And Lifecycle

> Agents should be able to spawn Gas City identities like hiring a new member
> into the team. Also I should be able to spawn a gas city identtiy from the
> Chrote UI, similar to how I now spawn a session by clicking a button in the
> terminal side panel. There could be a dropdown selection when I spawn a
> session to spawn it as a Gas City identity or not and to give it a name. Gas
> City has polecats I believe, which are intended as identities that are
> disposable and meant for only agents to spawn and they are supposed to be
> culled later on. Gas City might have ready plumbing for this. Example prompt I
> might give to Codexia: "Suggest a team composition to tackle this project" and
> Codexia could then either pick existing identities or suggest to e.g. build a
> new persistent designer who is a claude code agent called Lucy and include her
> in the work.

Implication: CHROTE should support user-spawned Gas City identities as an
extension of session spawning, while agents should be able to spawn disposable
or persistent identities as part of team formation. Gas City's polecat concept
should be investigated as likely existing plumbing for disposable identities.

### 2. Autonomy, Cost, And Safety

> Spawning an agent is like any other action and there are no specific
> restrictions for that. Over time we might build agents with narrowed scopes
> and whatnot, but that is not phase 1. We have no budgets and no special
> security considerations here.

Implication: phase 1 should not overbuild budgets, approval gates, or special
security policy around spawning/delegation. Default posture is trust the
operator/agents, keep things legible, and avoid speculative guardrail systems.

### 3. Team Builder, Master Board, And Workflow Shape

> What I see as a potential shape is a Team Builder feature in CHROTE. This
> shape could for example allow the user to in a graphical interface assemble
> teams out of agents, naming a leader and then getting to work. Another pattern
> might be having a Master Board type of feature. The way the master board works
> is that all agents in the team share visibility of a shared board that would
> contain the project description and task lists, key findings, ideas, stuff
> like that. That acts as the shared context for thinking. The boards are the
> shared context for work items and what we are going to do. Before the team can
> assemble the boards, they might need to have a bit of a workshop. They might
> need a place where this process takes shape and this could have a touch point
> in the interface where the user could oversee and perhaps even participate in
> that kind of discussion. But I want to highlight that the more common pattern
> for agent teams will definitely be the pattern where there is one designated
> leader who is orchestrating other agents.

Implication: CHROTE may have a valuable team-building surface, but not as a
generic Gas City workflow launcher. The product shape is team composition,
leader selection, and shared work context. A Master Board may be the shared
context artifact for teams, with project description, task lists, findings,
ideas, and work state. Workshop/discussion space may be useful before a team
settles the board. The default runtime pattern is leader-orchestrated teams.

### 4. First Useful Workflow

> The first useful workflow is going through a planning, bead creation,
> execution, and verification process in a way that in each step a second agent
> helps to improve the quality of the work of the primary agent doing that work
> and providing an outside perspective. Very similar to what has now been
> happening between the gascity-considering and claude-home sessions.

Implication: the first useful workflow is not "click to launch review quorum."
It is a leader/primary-agent work loop where a second agent improves quality at
each stage: planning, bead creation, execution, and verification. The recent
`gascity-considering` plus `claude-home` collaboration is the reference pattern.

### 5. Failure And Recovery

> Regarding when collaboration goes wrong I do not expect chrote to do anything.
> When something goes wrong I will ask the agents to explain the issues to me.

Implication: CHROTE should not proactively diagnose collaboration failures in
phase 1. The recovery loop is social/agentic: Perttu asks agents to explain.
This strongly limits dashboard/status/recovery feature scope.

### General Sanity Check

> As a general sanity checking question we can use, would I rather have an agent
> explain this to me or figure it out than look at something in a web
> application?

Implication: this becomes the core product test for any CHROTE UI around Gas
City. If Perttu would rather ask an agent to explain or resolve the issue, do
not build a web-app surface for it.
