# Agent Collaboration Primitives

CHROTE should expose durable host state and make agent work inspectable. It should also be able to grow into a deliberate meta-harness for orchestrating multiple AI harnesses.

This note records the exploration of Disler's `pi-vs-claude-code` communication extensions and Gastown Hall's Gas City as possible seeds for a future CHROTE agent-team layer.

See `docs/meta-harness-desired-state.md` for the desired state captured from Perttu's clarification.

See `gas-city-research/evaluations/meta-harness-evaluation.md` for the second-look assessment that treats Gas City as a serious candidate SDK/component.

## Desired Shape

The human should be able to:

- see which agent sessions exist
- see what Beads they appear to be working on
- inspect recent output
- understand team or role topology when one exists
- send an explicit, audited nudge when intervention is needed
- recover the thread of work after switching devices

The system should not:

- depend on Gastown being installed
- assume a specific AI agent implementation
- scrape agent responses as if they were a stable API
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

Gas City is larger and closer to a full agent workspace runtime. The important discovery is its Beads-based messaging primitive: mail-like messages are represented as Beads, with inbox/read/archive/threading behavior layered on top. It also has a nudge mechanism that can target live sessions while keeping runtime artifacts under a project-local `.gc` area.

Useful idea:

- Beads are a plausible durable substrate for agent-visible messages
- mail/thread/read/archive semantics map well to human-agent coordination
- nudges can be explicit events rather than hidden terminal typing
- session abstractions can cover tmux, subprocesses, ACP, Kubernetes, and other providers

CHROTE fit:

- the Beads mail model is the strongest candidate for a future CHROTE collaboration primitive
- Gas City itself should stay an explored upstream, not a hidden dependency

Primary risks:

- too broad to vendor casually
- introduces controller/supervisor/runtime concepts beyond CHROTE's current role
- mutates Beads and runtime state, so schema and audit design must be deliberate

## Revised Decision Boundary

The earlier shorthand "CHROTE should not become the orchestrator" was too broad. The corrected boundary is:

- CHROTE should not hide fragile or unaudited orchestration behind the cockpit.
- CHROTE can and should support deliberate orchestration as a meta-harness when that orchestration is inspectable, durable, and adapter-based.
- The meta-harness should be able to include different agent products and harnesses as interchangeable participants.

Build CHROTE's collaboration and meta-harness layer in stages:

1. Improve observer-only agent visibility.
2. Define the meta-harness desired state and adapter boundaries.
3. Add team topology metadata.
4. Add explicit messages or nudges into known durable sessions.
5. Spike a Beads-backed mailbox/message model.
6. Evaluate Gas City as a possible orchestrator SDK or reusable component.
7. Evaluate Pi comms as a possible team communication concept or adapter target.
8. Build recipes/molecules for repeatable workflows such as plan plus two reviewers and senate sessions.

Do not build automatic agent-to-agent routing, response scraping, team launchers, or a hosted communication hub without an explicit run model, audit trail, adapter boundary, and recovery story.

## Candidate Beads

The follow-up Beads should use these acceptance boundaries:

- Observer v2: no mutation, no agent control, no dependency on Gastown.
- Team topology: configuration only, rendered read-only, validated against live tmux sessions.
- Human nudge/message: explicit, audited, targeted to one known session or role.
- Beads mailbox spike: document schema compatibility with modern `bd`; no production mutation until reviewed.
- Coms adapter spike: read-only only; no CHROTE-owned hub, no token exposure.
- Meta-harness design: document registry, adapters, run ledger, recipe/molecule model, and safety boundaries before broad orchestration.

## References

- `https://github.com/disler/pi-vs-claude-code/blob/main/extensions/coms.ts`
- `https://github.com/disler/pi-vs-claude-code/blob/main/extensions/coms-net.ts`
- `https://github.com/gastownhall/gascity`
- Local Gas City checkout: `<workspace-root>/research/upstreams/gascity`
