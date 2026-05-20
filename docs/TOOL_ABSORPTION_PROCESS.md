# Tool Absorption Process

Repeatable workflow for evaluating and integrating new tools into CHROTE.

## Phase 1: Discovery & Research

- [ ] **Source**: Where did you find it? (HN, GitHub trending, recommendation, etc.)
- [ ] **First impression**: What problem does it claim to solve?
- [ ] **Quick stats**: Stars, activity, language, maintainers
- [ ] **Read the README**: Understand core architecture and capabilities
- [ ] **Check the docs**: Look for integration points, APIs, extension systems

## Phase 2: Fit Evaluation

Answer these questions:

1. **What pain does this solve?** (Be specific—"ChroteChat is shit" not "improves chat")
2. **What do we throw away if we adopt this?** (Sunk cost is real, but so is opportunity cost)
3. **Where does it sit in the architecture?**
   - Interface layer? (like Clawdbot)
   - Execution layer? (runs inside tmux)
   - Core infrastructure? (replaces something in CHROTE itself)
4. **What's the integration surface?** (API? File system? WebSocket? Skill system?)
5. **What's the maintenance burden?** (Active project? Breaking changes likely?)

## Phase 3: Challenge

Before committing, actively challenge the idea:

- [ ] Is this solving a real problem or shiny object syndrome?
- [ ] Are we building something we should buy/adopt, or adopting something we should build?
- [ ] Does this add complexity that isn't justified?
- [ ] What's the rollback plan if it doesn't work?

## Phase 4: Spike

- [ ] Clone/install in isolated environment
- [ ] Get basic functionality working
- [ ] Test the specific integration point you need
- [ ] Document: what works, what doesn't, what's surprising

## Phase 5: Integration

- [ ] Define where it lives in the CHROTE structure
- [ ] Build the connection layer (skill, API bridge, etc.)
- [ ] Update architecture docs
- [ ] Add to session templates if it runs in tmux

## Phase 6: Document & Evolve

- [ ] Write up findings (what it does, how we use it, gotchas)
- [ ] Update CHROTE_VISION.md if it changes the architecture
- [ ] Create runbook for common operations
- [ ] Revisit periodically—is it still earning its place?

---

## Template: Tool Evaluation Card

```
# [Tool Name]

## Source
Where found, date discovered

## Problem It Solves
One sentence

## Architecture Fit
- Layer: [Interface / Execution / Core]
- Integration point: [API / Files / WebSocket / Other]
- Replaces: [Nothing / ChroteChat / etc.]

## Evaluation Status
[ ] Researched
[ ] Challenged
[ ] Spiked
[ ] Integrated
[ ] Documented

## Decision
[Adopt / Reject / Defer]

## Notes
Findings, gotchas, links to spikes
```

---

## Example: Clawdbot

**Source**: GitHub, 2025-01-25

**Problem**: ChroteChat doesn't work well. Need mobile/voice access to CHROTE when on the run.

**Architecture Fit**:
- Layer: Interface
- Integration point: Clawdbot skill → CHROTE API
- Replaces: ChroteChat (for mobile use case)

**Decision**: Spike it. Build a skill that talks to CHROTE API.
