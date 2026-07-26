# ADR-0011: Leader-Agentic Orchestration with Per-Worker Ledger Evidence

## Status

Accepted 2026-07-26 (owner decision, Formations scope session). Supersedes the
2026-06-11 owner correction on chrt-9hjn that required an Archon-mediated
leader command protocol ("the leader should not get raw tmux control").

## Context

Orchestrated formations appoint one agent as leader of bound worker sessions.
Two shapes competed:

- **Archon-mediated**: the leader issues commands through a validated Archon
  interface; Archon executes them against tmux and records ledger evidence.
  Required by the 2026-06-11 correction.
- **Leader-agentic**: the leader is dispatched with the tmux socket and worker
  session names in its prompt and steers workers with native tmux directly.
  This is what main shipped (`executeOrchestratedFormation`,
  `leaderAgenticExtraLines`).

Adversarial review established two facts. First, ADR-0009 does not rule
mediation out — it addresses kernel-level fencing of untrusted same-UID
processes, a different problem from offering a cooperative command interface.
Second, the shipped path records almost no per-worker evidence: the ledger
holds one team-binding event plus the leader's final text, and the prompt asks
the leader to self-report which workers it touched. A hung or never-prompted
worker is invisible to Archon.

Under the recorded CHROTE threat model, agents are trusted and the network
perimeter — not same-UID isolation — is the security boundary. A mediation
layer therefore buys no enforcement, only observability, at the cost of
rebuilding the deployed path and constraining orchestration strategies to a
fixed protocol (the concern chrt-7d7p records).

## Decision

1. Leader-agentic is the accepted contract. The leader gets raw tmux access to
   bound worker sessions and decides the orchestration strategy dynamically.
   The Archon-mediated command protocol is rejected.
2. Observability is recovered in the ledger, not by mediation: orchestrated
   runs must record per-worker evidence independent of leader self-report —
   worker binding, each prompt dispatched to a worker (or a capture-derived
   equivalent), and a captured output/result event sufficient to inspect what
   each worker produced.
3. Worker failure modes fail loud with ledger evidence: a missing worker
   session, a worker that is never prompted, and a worker timeout/hang are
   distinct visible events, not silence.
4. Archon remains the control plane for reproducible setup, safe defaults,
   redaction/output caps, and provenance; helpers add value where formation
   lookup, artifact collection, or UI projection needs them (chrt-7d7p).

## Consequences

- chrt-9hjn is reset with acceptance criteria matching this shape and owns
  the per-worker evidence surface (chrt-zqnq was folded into it); chrt-7d7p
  is the toolset/observability slice on top of it.
- The leader's tool power stays bounded by scope conventions (formation
  sessions and workspace by default), not by an enforcement fence — consistent
  with ADR-0009's finding that no same-UID fence exists to build on.
