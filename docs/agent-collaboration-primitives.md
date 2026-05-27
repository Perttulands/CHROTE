# Agent Collaboration Primitives

CHROTE should expose durable host state and make agent work inspectable. It
should also grow into a deliberate meta-harness for orchestrating multiple AI
harnesses through named agent identities.

This note records the exploration of Disler's `pi-vs-claude-code`
communication extensions and Gastown Hall's Gas City as seeds for a CHROTE
agent-team layer. ADR-0001 has since selected Gas City as the CHROTE 3.0
orchestration substrate.

See `docs/meta-harness-desired-state.md` for the desired state captured from Perttu's clarification.

See `docs/chrote-gascity-framing.md` for the current descriptive Gas City context and CHROTE leverage problem. See `docs/gascity-meta-harness-evaluation.md` for the second-look assessment as evidence/history.

## Desired Shape

The human should be able to:

- see which agent sessions exist
- open named sessions and named agent identities
- tell one named agent to help another named agent
- see what Beads they appear to be working on
- inspect recent output
- understand team or role topology when one exists
- send an explicit, audited nudge when intervention is needed
- recover the thread of work after switching devices

The system should not:

- assume a specific AI agent implementation
- scrape agent responses as if they were a stable API
- treat passive transcript/status surfacing as the main collaboration feature
- inject prompts into arbitrary sessions without an explicit human action
- expose local IPC or agent-control endpoints outside the host trust boundary

## Disler Coms

`coms.ts` is a local peer messaging layer for the Pi VS Claude Code extension. Agents register metadata under a per-project directory, listen on local IPC, and expose operations such as list, send, get, and await.

Useful idea:

- lightweight project-local agent cards
- direct human or peer addressing by agent name
- async message/result workflow

CHROTE fit:

- useful as a read-only adapter if a project already uses it
- not suitable as CHROTE's primary collaboration layer

Primary risks:

- prompt injection is the core control mechanism
- response capture depends on extension-specific behavior
- local IPC has only local filesystem/pipe protection

## Disler Coms Net

`coms-net.ts` moves the same idea to a Bun HTTP/SSE hub with bearer auth, heartbeats, server metadata, and in-memory response tracking.

Useful idea:

- hub-and-spoke topology is easy to observe
- heartbeats give good liveness semantics
- HTTP/SSE is easier to bridge to dashboards than local sockets

CHROTE fit:

- worth studying for topology and liveness UI
- should not be exposed or managed by CHROTE by default

Primary risks:

- auth token handling becomes security-critical
- hub state is transient
- broad exposure would turn CHROTE from cockpit into control plane

## Gas City

Gas City is larger and closer to a full agent workspace runtime. The important
discovery is not only its Beads-based messaging primitive; it is the combination
of valid session identities, mail, nudging, sling/delegation, formulas,
molecules, events, and supervisor-owned runtime evidence.

Useful idea:

- valid Gas City identities make agent-to-agent mail authorship real instead of
  spoofed by arbitrary launched processes
- Beads are a plausible durable substrate for agent-visible messages
- mail/thread/read/archive semantics map well to human-agent coordination
- nudges can be explicit events rather than hidden terminal typing
- session abstractions can cover tmux, subprocesses, ACP, Kubernetes, and other providers
- formulas and molecules can package reusable workflows such as plan plus review

CHROTE fit:

- Gas City is the selected orchestration substrate for CHROTE 3.0
- CHROTE should expose named-agent access and safe operator controls above Gas
  City rather than mirroring the `gc` CLI or hiding Gas City as an invisible
  dependency
- the target operator move is named-agent collaboration, for example Codxia
  helping Claudia through Gas City primitives

Primary risks:

- too broad to vendor casually
- introduces controller/supervisor/runtime concepts beyond CHROTE's current role
- mutates Beads and runtime state, so schema and audit design must be deliberate

## Revised Decision Boundary

The earlier shorthand "CHROTE should not become the orchestrator" was too
broad. The corrected boundary is:

- CHROTE should not hide fragile or unaudited orchestration behind the cockpit.
- CHROTE can and should support deliberate orchestration as a meta-harness when
  that orchestration is inspectable, durable, and Gas City-backed.
- The meta-harness should be able to include different agent products and harnesses as interchangeable participants.

Build CHROTE's collaboration and meta-harness layer in stages:

1. Keep durable named-session access stable.
2. Bind selected named sessions to valid Gas City identities.
3. Add explicit mail/nudge/sling flows between known named identities.
4. Build recipes/molecules for repeatable workflows such as plan plus two
   reviewers and senate sessions.
5. Add team topology metadata and operator affordances in CHROTE.
6. Evaluate Pi comms as a possible adapter target or reference for team
   topology, not as the primary substrate.

Do not build automatic agent-to-agent routing, response scraping, team launchers, or a hosted communication hub without an explicit run model, audit trail, adapter boundary, and recovery story.

## Historical Candidate Beads

These pre-ADR follow-up boundaries remain useful as safety checks, but current
work should be tracked in the active Gas City/CHROTE 3.0 beads:

- Observer v2: no mutation and no agent control.
- Team topology: configuration only, rendered read-only, validated against live tmux sessions.
- Human nudge/message: explicit, audited, targeted to one known session or role.
- Beads mailbox spike: document schema compatibility with modern `bd`; no production mutation until reviewed.
- Coms adapter spike: read-only only; no CHROTE-owned hub, no token exposure.
- Meta-harness design: document registry, adapters, run ledger, Gas City
  recipe/molecule model, and safety boundaries before broad orchestration.

## References

- `https://github.com/disler/pi-vs-claude-code/blob/main/extensions/coms.ts`
- `https://github.com/disler/pi-vs-claude-code/blob/main/extensions/coms-net.ts`
- `https://github.com/gastownhall/gascity`
- Local Gas City checkout: `<workspace-root>/research/upstreams/gascity`
