# CHROTE Agent Orchestration Interview Report

Date: 2026-05-31
Status: interview capture and synthesis, not implementation plan
Scope: agent collaboration, CHROTE, Archon, team leaders, persistent agent identities, progressive disclosure, and first-stage operating principles

## Evidence quality note

This report is based primarily on the direct dictated answers given after switching away from live voice mode. The earlier live voice-mode exchange is not treated as a reliable verbatim source because the exact spoken transcript was not available. The direct dictated answers are treated as the reliable source.

The user requested that their input be captured more or less as-is. The raw capture below is therefore lightly structured for readability but intentionally not rewritten into polished product language.

---

# Part 1: Near-Verbatim Interview Capture

## 1. Why agents should collaborate

By having these agents collaborate, I believe that I can reach higher quality outcomes and unlock new capabilities.

I believe that when I can chain together different harnesses and agents from different providers, I can reach better outcomes. And we've already had some pretty interesting experiments with this by, for example, having in a shared team session a Claude Code agent and a Codex agent collaborate.

So one can, for example, provide an outside perspective or act as a squire or a co-pilot, looking up information or surfacing some new ideas.

And this works differently than using sub-agents, which all of these harnesses can do locally. But the sub-agent system requires for the orchestrator agent to kind of guide the entire thing. And it's not so native to have an actual agent be a collaborator with the driver or the orchestrator agent.

But now, if I have multiple harnesses that can do this, then this is possible.

This also creates a much more fluid way to create specialists who can have their own skills, their own personalities, and have them collaborate and allows me to, over time, build an organization and maybe even start adding layers like creating evaluations or ways for these systems to start learning.

So we can evaluate, like, okay, how is Susie, the designer agent, doing here? What kind of outputs are we getting? Can we do something to improve the tools this agent uses or the system prompt or add some more hooks, maybe try a different harness for this or a different model?

This kind of flexibility is very appealing to me.

---

## 2. Current pain and why the shared tmux setup is not enough

As I said, we currently have the shared tmux socket, so the agents can, it is possible for them to view each other's sessions, and this is useful, but it will still require for me to give quite specific directions that, okay, find this socket, this is the session name, look at what it's doing, and help it out, and then instruct this to the other agent, perhaps, so it's not very fluid.

And we explored an orchestrator builder called Gas City. It has some of these ready primitives in place, like agent-to-agent communications and such, but this was a failure, this experiment. That's why we're now thinking about this from the first principles.

What I liked about Gas City is the ability to have molecules, which is a Gas City primitive, and a molecule is a predefined template that can, for example, include specific workflows.

But I'd actually like to go beyond that and start thinking about pre-configured agent teams, so that I could, for example, talk to a specific team leader who has certain agents that are part of its group, or assemble these groups on the fly by, for example, having a conversation with a team leader type agent and have it assemble the right team for the job.

And to have this available for me and the agent as a very simple, perhaps a command line interface tool that they can call or having a simple place where they can view what agents are available for them to use in the team.

And yeah, so not having that infrastructure makes this kind of ad hoc, and I feel like we're leaving some value on the table.

Over time, what I think could happen with a system like this is that they, this gives the system more abilities to be proactive.

They could over time, by understanding me, my needs, my business context better, start to suggest new things we could build and even go beyond that to take all the ideas I have or thoughts I have and to kind of run with them that, OK, it could be like an example workflow, like to research a theme, conduct some research, think about how we could integrate in our systems, create some prototypes and material for me to review and stuff like this.

---

## 3. What a team leader agent should do

The primary idea of a leader is to access the capabilities of a larger group of agents while maintaining the touchpoint of a single agent. That's the kind of entire point.

Also, a leader is somebody who can stay on a slightly higher level.

I mean, even these can collaborate, like there could be, like if you think about how human organizations work, there are functions, there are teams that are organized around business problems or capabilities, and these groups can collaborate in order to reach some business outcomes or even from their perspectives, provide input to the question of what actually is worth building and what should we be doing in the first place.

And this pattern, I think, is extremely interesting.

And then also the moving from this world where I have to be, I hold all of the context and spawn deliberately spawn an agent and tell it to work with this other agent or design something and then have agents help me craft it.

To go to more towards a world where I almost have co-workers, and these having personalities, I think is quite important.

It's not just picking this one agent, a team leader who is my touchpoint is actually somebody who I'm having fun with. I like the way it thinks, I like the way it communicates with me.

I suppose there's also a level of responsibility almost. Like this leader takes responsibility for the actions of the group, but perhaps that's not so vital to highlight here, it might be a little bit misleading.

---

## 4. What makes a persistent agent worth keeping

Taste, style, relationships, this is I think a good place to start.

If it's only sort of mechanical execution, then perhaps you don't really need this kind of elaborate custom agent for it. For example, executing on a coding task doesn't require a special kind of agent.

But perhaps on the other hand, having an agent who is the senior architect of a project could be a very useful thing, because an agent like this would sort of hold the golden thread throughout my days and weeks and months of what is the thing we're building and what are the key architectural principles.

Instead of writing these in documents and then hoping the agents understand them every time when they load up with the fresh context, it would be much preferable if they could, for example, make a plan and then ask the master architect that is this plan now in line with the grand vision, and the architect could reason about it and see if the actual architecture should be changed with these new ideas or what would be the optimal path forward for the project as a whole.

And I think especially in tasks like writing, creating visual assets, creating dashboards, user interfaces, having an agent who can do it well, reliably, is very much just like super valuable.

And again, like the way you would use it, you do this usually is that you create a skill.md file about like a writing style.

But again, if we can have this agent who can hold the voice and who can hold the standards, that would be so much better because we could have this special character to review this kind of work.

And then for the leaders who would be my touch points, then here the personality will start to shine and be more important so that the collaboration is nice and feels good.

Also these agents with specific roles would fit very well in this almost independent role.

Like as an example, there could be an agent whose responsibility is to maintain the integrity of the documents and maintain the integrity of the skill.md files, for example, or maintain the integrity of the configurations of the working agents.

And these could run in preset loops, surfacing any mismatch, any issues they see, reason about how things could be improved.

And again, I believe this will be more powerful than just having a generic Claude Code run cron jobs.

---

## 5. What agents should know about each other

There should be progressive disclosure, which I think is a key principle in building any kind of systems.

So any agent can query the system to see who's there.

I think a capability or personality or focus tagging would be helpful so that the agent can, for example, look at who to consider for a job.

And then you can go deeper.

And of course, you can go to inspect the specific configuration system prompts and everything for these agents or their agent.md file or claude.md file or specific skills, whatever.

This will enable an agent to answer the question, who should we include in this project? Who should we have run this task?

Or more meta level, what agents should we build that we have not yet worked up?

---

## 6. Ideal collaboration flow

I have a conversation with my main touchpoint, who I would like to call the Archon, and the Archon, with me, has a brief discussion and understands what I'm going for.

An example could be that, okay, I want to find a job at a moving AI company, and the main reason that a good resume could be an interesting web experience that highlights my capabilities in agentic development and engineering.

The Archon will then think about the good team to start building what this experience could look like, and will identify a good team lead for this project.

I can then have a brief discussion with the team leader, who will then assemble a group of designers, for example, three designers, who will then work on the general flow and produce some prototypes for me to have a look at.

I also could bring in the Archon to this, and then we could then agree on a kind of design idea, and then we could build a new setup.

And actually this could be quite interesting if we then think that now we have the design, so we have the design team, and maybe we could bring in like an architecture team and a development team on board of this.

And the team leaders could collaborate, agree on like a blueprint for this thing, and then it could go into the development so that the designer, the design lead, and the architecture lead remain as active members in the group, and then there's a group of developers who are managed by a developer lead.

And then the developer has a conversation with the developer lead and has a conversation with the developers.

They collaborate, they could have like a shared state for that.

We already have something called Beads for issue tracking.

They could also have the shared notice board for any communications.

The architect and designer could proactively review what's going on and that the developers, if they have any questions, they could surface those and ask for input from the designers, design lead or architectural lead.

The Archon would also be participating in this, so it would have a look at how things are going and then communicate with me if we status updates, surface anything that might need my input.

---

## 7. Visibility in the first stage

In the first stage, I don't really expect there to be anything in terms of making this visible.

If I want to know something, I can just ask the agents.

---

## 8. When the system should interrupt

The system should proactively interrupt for:

* blocked work
* decision needing your taste/judgment
* team disagreement
* architectural drift
* cost/risk concerns
* surprising opportunity
* “we found something better than the original direction”
* a leader spots that things must be stopped and something clarified or things are going in a bad direction

We will not build any safeguards at first. We rely on the agents being smart about this.

---

# Part 2: Interview Report

## 1. Core read

The desired system is not primarily an automation framework, task runner, tmux dashboard, or workflow launcher.

The desired system is a way to create a working agent organization around Perttu. This organization should contain named agents, team leaders, specialists, persistent standards-holders, and possibly independent maintainer agents. It should allow Perttu to interact through a small number of trusted touchpoints while still accessing the capabilities of many agents and harnesses underneath.

The strongest emerging concept is the **Archon**: a main personal touchpoint that understands Perttu’s intention, helps frame what should happen, identifies or assembles a suitable team, remains involved at the right level, and surfaces issues or decisions that need Perttu’s input.

The purpose is not just to make agents do more work. The purpose is to reach higher quality outcomes, unlock new capabilities, and build a more flexible agent organization that can improve over time.

---

## 2. Why collaboration matters

The first stated reason for agent collaboration is quality.

Perttu believes that better outcomes can be reached when agents from different providers and harnesses can work together. This is based partly on experiments where Claude Code and Codex collaborated in a shared team session. In that experiment pattern, one agent can act as the main driver while another agent provides an outside perspective, acts as a squire or co-pilot, looks up information, or surfaces additional ideas.

This is meaningfully different from built-in sub-agent systems. In the sub-agent model, the orchestrator agent usually has to direct the entire process from inside one harness. The sub-agents are subordinate to that harness and that orchestration frame. They are useful, but they do not feel like independent collaborators with the main agent.

The desired system makes it possible for multiple harnesses to participate in collaboration. This matters because different harnesses, models, tool environments, and agent identities may have different strengths. A Codex agent, a Claude Code agent, a designer agent, a writing agent, and an architect agent should not have to collapse into one provider’s local sub-agent mechanism.

The second stated reason is capability expansion. Collaboration enables agent roles that are not just short-lived execution helpers. It makes it possible to create specialists with their own skills and personalities, and to build something closer to an organization over time.

The third stated reason is learning and improvement. If agents are persistent and differentiated, they can be evaluated. Their outputs can be reviewed over time. Their prompts, tools, hooks, harnesses, and models can be improved. This makes the system evolvable.

The appeal is flexibility: different agents, different roles, different harnesses, different models, different tools, and the ability to improve them as their behavior becomes visible.

---

## 3. Current pain

The current shared tmux socket proves that technical visibility is possible. Agents can, in principle, inspect each other’s sessions. This is already useful.

But the current workflow is not fluid.

Perttu still has to give specific operational instructions:

* find this socket
* here is the session name
* inspect what this agent is doing
* help it out
* tell the other agent what to do

This means Perttu remains the coordinator. The agents do not naturally discover each other, understand each other’s roles, or involve each other through a native collaboration layer. The system can be made to work, but it requires too much manual choreography.

The pain is not that agents cannot technically see each other. The pain is that collaboration is ad hoc. The missing layer is an agent-facing collaboration and team layer.

This is why the current setup leaves value on the table. The necessary ingredients exist in scattered form: tmux sessions, named agents, possible cross-agent inspection, Beads, skill files, harnesses, and prior Gas City ideas. But there is no simple native way for agents or Perttu to discover available agents, assemble teams, initiate collaboration, and preserve reusable patterns.

---

## 4. What Gas City revealed and why it failed

Gas City was attractive because it had some primitives that seemed relevant:

* agent-to-agent communication
* workflow-like structures
* molecules as predefined templates
* orchestration concepts

However, the experiment is described as a failure. This is important. The next direction should not simply revive Gas City with a different UI. The value was not Gas City itself. The value was the glimpse of reusable collaboration primitives.

The particularly resonant primitive was the molecule: a predefined template that could include a workflow. But Perttu now wants to go beyond that.

The next idea is not just predefined workflows. It is pre-configured agent teams and dynamically assembled teams.

This means the new starting point should not be “what primitives do we have?” but “what agent organization should exist, and how should agents discover, assemble, and work with each other?”

---

## 5. Desired direction: agent teams, not just workflows

The important shift is from workflow templates to agent teams.

A predefined workflow might say: do research, then review, then synthesize.

A preconfigured agent team is richer. It can contain agents with roles, personalities, standards, tools, and relationships. A team leader can know which agents belong to its group. A team can be organized around a business problem, capability, or function. Teams can collaborate with other teams.

Perttu wants both:

1. **Preconfigured teams**
   For example, a team leader with known agents already part of its group.

2. **Dynamically assembled teams**
   For example, a team leader agent can assemble the right team for the job after understanding the goal.

This should be available both to Perttu and to agents through a simple mechanism. The mechanism might be a CLI tool or a simple registry where agents can see what agents are available to use in a team.

The important point is that the collaboration mechanism should be agent-usable. It should not require Perttu to operate a dashboard or manually copy context between agents.

---

## 6. The Archon

The Archon is the strongest named concept from the interview.

The Archon is Perttu’s main touchpoint. It is the agent he talks to first. The Archon has a brief discussion with Perttu, understands what he is trying to do, and helps decide what team or team lead should be involved.

The Archon is not just a router. It has a higher-level role. It helps frame the work, identify what kind of capabilities are needed, and remain involved as work proceeds.

The Archon gives Perttu access to a larger agent organization while preserving a single-agent touchpoint.

This is the key interaction pattern:

> Perttu should not have to directly operate the whole agent organization. He should be able to work through an agent he likes, trusts, and enjoys interacting with.

The Archon can also remain involved later:

* checking how work is going
* receiving status
* surfacing anything that needs Perttu’s input
* potentially noticing if the direction is going wrong
* participating in design/architecture discussions when useful

The Archon is the top-level relationship interface between Perttu and the agent organization.

---

## 7. Team leaders

A team leader gives access to a larger group while preserving a single-agent touchpoint at the team level.

The main purpose of a leader is not command-and-control. It is to make a group of agents usable without making Perttu operate the group manually.

A team leader should be able to:

* understand a goal at a slightly higher level
* know or discover relevant agents
* assemble a team
* coordinate work inside the team
* collaborate with other team leaders
* help produce a blueprint or direction
* keep specialists involved across phases
* surface questions or issues to Perttu or the Archon

The human organization analogy matters. Functions and teams can be organized around capabilities or business problems. These groups can collaborate not only to execute work, but also to help decide what is worth building in the first place.

This is a crucial point: the team system should not only execute ideas. It should also help evaluate and shape ideas.

There is also a relationship and personality element. A leader should be someone Perttu enjoys talking to. The leader’s way of thinking and communication style matter.

Responsibility may be part of the leader concept, but Perttu flagged it as potentially misleading. It should not be overemphasized yet.

---

## 8. Persistent identities versus disposable helpers

Persistent identity is valuable when the agent does something beyond mechanical execution.

Mechanical coding execution often does not require an elaborate persistent agent. A generic coding agent may be enough for a bounded implementation task.

Persistence becomes valuable when an agent holds:

* taste
* style
* relationships
* standards
* project continuity
* long-term architectural judgment
* a specific domain of responsibility
* a characteristic way of communicating or thinking

The distinction is not “named versus unnamed.” The distinction is whether there is something worth preserving.

---

## 9. Senior architect agent

The senior architect is one of the clearest examples of a persistent agent worth keeping.

A senior architect agent would hold the golden thread of a project over days, weeks, and months. It would understand what is being built and preserve the key architectural principles.

This avoids a weak current pattern: writing architecture principles in documents and hoping fresh-context agents read and apply them properly every time.

Instead, another agent could make a plan and ask the master architect:

* is this plan in line with the grand vision?
* should the architecture change because of these new ideas?
* what is the optimal path forward for the project as a whole?

The architect is not just a documentation reader. It is an active holder of architectural continuity.

---

## 10. Taste, voice, and standards agents

Persistent identity is also valuable in domains where quality depends on taste and consistency:

* writing
* visual assets
* dashboards
* user interfaces

Today these standards are often captured in files like skill.md. That is useful, but limited.

Perttu’s stronger idea is an agent that can hold the voice or hold the standard. This agent could review work through a specific taste lens or character.

This is different from just loading a style guide. The agent becomes a persistent reviewer and quality-holder for a domain.

This matters because some types of work are not just about correctness. They are about judgment, consistency, feel, style, and taste.

---

## 11. Independent maintainer or integrity agents

Another proposed class of persistent agent is an independent maintainer or integrity agent.

Such an agent might be responsible for:

* maintaining document integrity
* maintaining skill.md file integrity
* maintaining working agent configurations
* surfacing mismatches
* detecting drift
* identifying issues
* reasoning about improvements

These agents could run in preset loops.

The important distinction is that this is not just a generic Claude Code cron job. The value comes from the agent having a persistent role, standard, and responsibility.

This suggests a future system where some agents are not only called during projects, but have ongoing stewardship responsibilities.

---

## 12. Progressive disclosure as an agent discovery principle

Progressive disclosure is a key design principle for the agent registry and collaboration layer.

Any agent should be able to query the system to see who exists. But the first view should not dump all details.

The initial layer should likely expose enough metadata for candidate selection:

* capability tags
* personality tags
* focus tags
* possibly role or team tags

This lets an agent ask:

* who should we include in this project?
* who should run this task?
* who should we consider for this job?

Then the agent can go deeper if needed.

Deeper inspection could include:

* specific configuration
* system prompts
* agent.md files
* claude.md files
* skill files
* tool setup
* other agent definitions

The registry should also support meta-level reasoning:

* what agents should we build that do not exist yet?
* what capabilities are missing?
* what kind of team should exist for this kind of work?

This implies the registry is not merely an address book. It is a reflective surface for the agent organization.

---

## 13. Ideal project flow

The concrete example used in the interview was career positioning: finding a job at a fast-moving AI company.

The initial idea is that a good resume may not be enough. A more interesting artifact could be a web experience that highlights Perttu’s capabilities in agentic development and engineering.

The ideal flow:

1. Perttu talks with the Archon.
2. The Archon understands the goal.
3. The Archon thinks about what team would be useful.
4. The Archon identifies a good team lead.
5. Perttu talks briefly with the team leader.
6. The team leader assembles designers.
7. The designers work on the general flow and produce prototypes.
8. Perttu reviews prototypes.
9. The Archon can be brought into the discussion.
10. A design direction is chosen.
11. Architecture and development teams are brought in.
12. Team leaders collaborate on a blueprint.
13. Development begins.
14. Design lead and architecture lead remain active.
15. Developers work under a developer lead.
16. Developers surface questions to design or architecture when needed.
17. Designers and architects proactively review ongoing work.
18. Beads can be used for issue tracking.
19. A shared notice board can be used for communication.
20. The Archon monitors and sends status or escalation to Perttu.

This is not just a waterfall process. It is a coordinated agent organization where different roles remain active and can interact as the work evolves.

---

## 14. Shared state

Two shared-state concepts were mentioned:

1. **Beads**
   Existing issue tracking / work tracking.

2. **Shared notice board**
   A possible communication surface for team messages.

Beads already exists and should likely remain the work/issue tracking layer.

The shared notice board is conceptually different. It would be for communications, notices, updates, questions, and perhaps team-level context. It should not necessarily become another issue tracker.

The exact boundary remains open, but the distinction matters:

* Beads = work tracking
* notice board = team communication / shared awareness

---

## 15. Visibility and UI

The first stage should not prioritize making collaboration visible through a dashboard or live UI.

Perttu explicitly said that in the first stage he does not expect anything in terms of making this visible.

If he wants to know something, he can ask the agents.

This is a major product constraint.

The first version should not become a control cockpit. It should not become an orchestration dashboard. It should not spend its energy making every collaboration step visible by default.

The useful first-stage interaction model is:

> Ask agents what is happening when you need to know.

This implies that the system needs to be explainable on request, not constantly observable by default.

---

## 16. Escalation

The system should proactively interrupt Perttu only when there is a meaningful reason.

Escalation triggers include:

* blocked work
* a decision needing Perttu’s taste or judgment
* team disagreement
* architectural drift
* cost or risk concerns
* surprising opportunity
* discovering something better than the original direction
* a leader sees that work should stop
* something needs clarification
* things are going in a bad direction

This gives the first-stage leader/Archon behavior a simple operating rule:

> Keep work moving unless blocked, drifting, risky, meaningfully conflicted, or in need of Perttu’s taste/judgment.

Perttu explicitly does not want to build safeguards at first. The first version should rely on agents being smart enough to surface these issues.

That means the first version should not overbuild approval gates, governance, or safety machinery. The expectation is judgment and escalation, not formal policy enforcement.

---

# Part 3: Key Findings

## Finding 1: The real goal is agent organization, not automation

The desired system is closer to an organization than a tool.

It should contain:

* main touchpoint agents
* team leaders
* specialists
* persistent standards-holders
* independent maintainers
* possible future evaluators
* dynamically assembled teams
* preconfigured teams

The organization should help Perttu create better outcomes and eventually become proactive.

---

## Finding 2: The Archon is the central user-facing pattern

The Archon is the main personal touchpoint.

It protects Perttu from having to operate the whole agent organization directly.

The Archon should:

* understand Perttu’s goal
* identify what kind of team is needed
* find or assign a suitable team leader
* remain involved at the right level
* communicate status and escalations
* help stop or redirect work when needed

The Archon is not merely a router. It is the interface to the system.

---

## Finding 3: Team leaders are interfaces to grouped capability

Team leaders are useful because they let Perttu or the Archon access a group without manually coordinating every agent.

A team leader should:

* represent a capability area or problem-oriented group
* assemble or use relevant specialists
* coordinate their work
* collaborate with other team leaders
* maintain the single-agent touchpoint for that group

This lets the system scale without making Perttu manage every agent directly.

---

## Finding 4: Persistent agents are justified by taste, continuity, and standards

A persistent identity is not needed for every task.

It becomes valuable when an agent holds:

* taste
* style
* relationships
* project continuity
* architectural principles
* quality standards
* domain stewardship

This suggests a clear split between disposable execution helpers and persistent judgment-holders.

---

## Finding 5: The registry should use progressive disclosure

Agents should be able to discover each other.

But they should not need to ingest every detail upfront.

The registry should expose:

1. who exists
2. what they are good for
3. what personality/focus/capability tags they have
4. deeper configuration only when needed
5. meta-level gaps in the agent organization

This lets agents answer not only “who can do this?” but also “what agent should exist for this?”

---

## Finding 6: First-stage visibility should be conversational, not dashboard-based

Perttu does not want a visible orchestration cockpit in the first stage.

The system should be inspectable by asking agents.

This supports a product direction where CHROTE remains a working surface and access layer, not a dashboard for watching agents shuffle messages around.

---

## Finding 7: First-stage safety should be agent judgment, not heavy safeguards

Perttu does not want safeguards built at first.

The expectation is that leaders and the Archon should use judgment and interrupt when needed.

This means the first stage should focus on:

* good escalation prompts
* clear leader responsibilities
* simple shared state
* explainability on request

Not:

* policy engines
* approval gates
* elaborate risk controls
* enterprise workflow theater

---

# Part 4: Emerging System Concepts

## Archon

Main touchpoint agent. Understands Perttu’s goals and routes/frames work through the agent organization.

Possible responsibilities:

* goal clarification
* team selection
* team leader selection
* high-level monitoring
* escalation to Perttu
* participation in key direction-setting moments

## Team leader

Single touchpoint into a group of agents.

Possible responsibilities:

* assembling specialists
* coordinating a team
* working with other team leaders
* maintaining progress
* surfacing questions
* preserving the team-level context

## Specialist agent

Agent with a narrower domain capability, skill, personality, or standard.

Examples:

* designer
* writer
* visual asset creator
* dashboard/UI specialist
* researcher
* developer
* reviewer

## Senior architect

Persistent project-level standards and architecture holder.

Possible responsibilities:

* holding the golden thread
* reviewing plans against the grand vision
* deciding whether architecture should evolve
* helping choose the optimal path forward

## Taste/voice/standards agent

Persistent quality-holder for subjective or style-heavy domains.

Possible responsibilities:

* holding voice
* reviewing writing
* reviewing UI
* reviewing visual assets
* preserving style and standards

## Integrity maintainer

Independent role responsible for drift detection and upkeep.

Possible responsibilities:

* checking document integrity
* checking skill file integrity
* checking agent configuration integrity
* surfacing mismatches
* suggesting improvements

## Agent registry

Progressively disclosed directory of available agents.

Possible layers:

1. roster
2. capability/personality/focus tags
3. team membership
4. configuration
5. prompts and skill files
6. gap analysis

## Shared notice board

Communication layer for agent teams.

Possible uses:

* team updates
* questions
* notices
* cross-team messages
* lightweight shared awareness

## Beads

Existing issue/work tracking layer.

Likely role:

* tasks
* issues
* work records
* blockers
* project state

---

# Part 5: Product Principles

## 1. Preserve a single-agent touchpoint into larger capability

The user should not have to manually operate a swarm.

The Archon and team leaders are important because they preserve a coherent conversational surface.

## 2. Use agents as collaborators, not only sub-agents

The system should support real collaboration between agents and harnesses, not merely local sub-agent delegation inside one provider.

## 3. Make team assembly fluid

The system should support both predefined teams and ad hoc team formation.

## 4. Let agents discover each other

Agents should be able to query who exists and who might be useful.

## 5. Use progressive disclosure

Start with tags and high-level identity. Go deeper only when needed.

## 6. Keep visibility conversational at first

No big UI requirement in the first stage. Ask the agents if you want to know.

## 7. Interrupt only for meaningful reasons

Escalate for blockers, decisions, drift, disagreement, risk, opportunity, or bad direction.

## 8. Do not overbuild safeguards at first

Rely initially on agent judgment and leader escalation.

## 9. Persistent identity must earn its keep

Do not create persistent agents for generic mechanical execution.

Persistent agents are valuable when they hold taste, standards, continuity, or domain judgment.

## 10. The system should become improvable

Over time, agent outputs, tools, prompts, hooks, harnesses, and models should be evaluated and improved.

---

# Part 6: Anti-Vision

The interview implies several things the system should avoid.

## Avoid a dashboard-first product

Perttu does not want visibility for its own sake in the first stage.

A dashboard would likely be premature.

## Avoid manually choreographed tmux collaboration

The current shared socket is useful but too ad hoc.

The new system should reduce the need for Perttu to tell agents which socket, which session, and what to inspect.

## Avoid treating Gas City as the destination

Gas City provided useful inspiration but the experiment failed.

Do not restart by rebuilding Gas City integration.

## Avoid generic cron-job automation as the model for maintainer agents

A persistent integrity agent should have a role and standard, not just run generic scheduled commands.

## Avoid making every agent persistent

Mechanical execution does not always require named identity.

## Avoid exposing all internals immediately

Progressive disclosure matters. Agents should not need to ingest every prompt/config/skill file just to find a collaborator.

## Avoid formal safeguards in the first slice

Do not build heavy approval or governance machinery before the collaboration model itself works.

---

# Part 7: Candidate First-Stage Direction

This section is interpretation, not a decision.

A plausible first-stage system should prove the following:

1. Perttu can speak to the Archon.
2. The Archon can understand a goal and suggest a team leader.
3. The team leader can inspect available agents through a simple registry.
4. The team leader can assemble a small group.
5. The group can produce a useful output.
6. A persistent specialist or reviewer can improve that output.
7. The team can use Beads for work/issues and possibly a notice board for communication.
8. Perttu does not need to watch a dashboard.
9. Perttu can ask what is happening.
10. The Archon or leader interrupts only when needed.

A strong first test could use the career/web-experience example:

* Perttu tells the Archon he wants to create a web experience to support a job search at a fast-moving AI company.
* The Archon identifies a suitable project/team lead.
* The team lead assembles a small design team.
* The design team produces flows/prototypes.
* The Archon and/or a design lead helps review direction.
* Architecture and development leads are added later only if the initial design direction is promising.

The important proof is not that many agents run. The proof is that the system feels like access to a working team through coherent touchpoints.

---

# Part 8: Open Questions

These are not answered yet.

## First slice

* What exact first slice would be small enough to build but real enough to matter?
* Should the first slice use Archon + one team leader + two specialists?
* Should the first slice focus on design, architecture review, or development?

## Agent registry

* What is the minimal registry format?
* What tags are needed first?
* How should agents query it?
* Is the registry a CLI, file, API, or all of these eventually?

## Shared state

* What belongs in Beads?
* What belongs on a notice board?
* What belongs in long-term context?
* What should remain only in session transcripts?

## Persistent identity

* What is the minimal definition of a persistent agent?
* Does persistent identity require a session, config, memory, role file, or all of these?
* How should agent track record be captured?

## Evaluation

* What does it mean to evaluate “Susie the designer agent”?
* What outputs should be reviewed?
* Who reviews them?
* How are improvements made to prompts, tools, models, or harnesses?

## Escalation

* How should the leader know when to interrupt?
* How much should this be prompt-level guidance versus system behavior?
* What does “things are going in a bad direction” mean in practice?

## CHROTE role

* Is CHROTE only the access layer?
* Does CHROTE expose the registry?
* Does CHROTE expose notice boards?
* Does CHROTE only provide named session access and leave agent collaboration to files/CLI?

---

# Part 9: Planning Guardrails

Any next implementation plan should obey these guardrails.

1. Do not start from Gas City.
2. Do not build a dashboard first.
3. Do not make visibility the product.
4. Do not make every agent persistent.
5. Start with an agent registry or discovery primitive only if it directly supports team formation.
6. Start with real agents/harnesses, not mocks, if the goal is to prove real collaboration.
7. Preserve the Archon as the main touchpoint concept.
8. Preserve team leaders as access points into groups.
9. Use progressive disclosure.
10. Keep Beads as issue/work tracking unless there is a strong reason not to.
11. Treat a shared notice board as communication, not as a replacement for Beads.
12. Use agent judgment for first-stage escalation instead of building heavy safeguards.
13. Make the first slice feel like “I can access a team through one agent,” not “I can watch a workflow execute.”
14. Capture enough state that agents can explain what is happening when asked.
15. Prefer one working team flow over a broad orchestration platform.

---

# Part 10: Compact Summary

Perttu wants to move from manually coordinating individual AI agents toward an agent organization.

The current shared tmux socket proves that agents can technically inspect each other, but the workflow is still ad hoc and requires Perttu to choreograph collaboration manually.

The desired system has a main touchpoint called the Archon. The Archon understands Perttu’s goal, identifies or assembles the right team, and remains involved enough to surface important decisions, blockers, opportunities, or drift.

Team leaders provide single-agent touchpoints into groups of specialists. Specialists may include designers, developers, architects, writers, reviewers, and maintainers.

Persistent agents are valuable when they hold taste, style, standards, continuity, relationships, or project-level judgment. They are less necessary for purely mechanical execution.

Agents should discover each other through progressive disclosure: first seeing who exists and what they are good for, then drilling into configuration, prompts, skill files, and deeper details only when needed.

The first stage does not need a dashboard. If Perttu wants to know what is happening, he can ask the agents.

The system should proactively interrupt only for meaningful reasons: blocked work, decisions needing Perttu’s taste, team disagreement, architectural drift, cost/risk issues, surprising opportunities, better directions, or signs that work should stop and be clarified.

The first implementation should prove that Perttu can access the capabilities of a larger agent group through coherent touchpoints, without manually operating the collaboration machinery himself.
