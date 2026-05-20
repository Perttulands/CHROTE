# CHROTE Vision

CHROTE is a private cockpit for durable host-owned work.

The browser is disposable. The work is not.

## Canonical Source

`../PRD.md` is the canonical product source for current behavior, Services
Platform V1, and roadmap boundaries. This document preserves the concise vision
and should not drift into a second product spec.

## Operating Principle

Everything important runs on the Ubuntu host:

- terminals
- AI agents
- dev servers
- builds
- tests
- logs
- Beads
- runtime state
- selected `/srv` services

CHROTE gives the human one browser-based control surface for that host state.

## Product Direction

CHROTE is moving in stages:

1. Durable workspace cockpit: tmux, terminals, files, Beads, and agent session
   observability.
2. Services platform: first-class operator views for selected `/srv` services,
   currently TTS Gateway and Context API.
3. Meta-harness: reusable, auditable orchestration for multiple AI harnesses and
   tools.
4. Agent Teams: team topology, typed collaboration workflows, and Beads-backed
   coordination once the substrate is stable.

See `meta-harness-desired-state.md` and
`agent-collaboration-primitives.md` for roadmap ideas. Those documents describe
future direction, not active runtime commitments.
