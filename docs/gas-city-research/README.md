# Gas City Research and Experiments

This folder preserves the thinking, decisions, experiments, and conversation
artifacts from the CHROTE/Gas City investigation.

It is historical research, not an active implementation area. The local Gas
City runtime and `gc` command were removed on 2026-05-30 to reduce confusion.
CHROTE currently runs as the plain tmux access system again. Future Gas City
work should start from a fresh decision and a new evidence-backed slice rather
than treating anything here as already live.

Removal manifest: `/home/perttu/rollback-snapshots/gascity-runtime-removal-20260530-215854/manifest.txt`.

## What Belongs Here

- Why Gas City looked useful for agent-to-agent collaboration.
- What the team tried while evaluating it.
- What worked, what failed, and what was learned.
- Superseded ADRs and rollback decisions.
- Experiment records, smoke-test notes, runbooks, and prototype research notes.

## What Does Not Belong Here

- Runnable Gas City city roots, supervisor configs, sockets, or runtime state.
- `gc` binaries, installers, or active adapter packages.
- Current CHROTE product requirements unless Gas City is explicitly
  reintroduced.
- New implementation tasks that assume Gas City is installed.

## How To Read This Folder

- Start with [what-we-were-trying-to-do.md](what-we-were-trying-to-do.md) for
  the concise historical record of the goal, what was attempted, what was
  learned, and which Gas City Beads were removed from the active ledger.
- Use [interview-plan-new-orchestration.md](interview-plan-new-orchestration.md)
  to interview Perttu before turning the retained lessons into any new plan.
- Read [framing.md](framing.md) for the longer research framing behind that
  record.
- Read [adr/0003-rollback-active-gascity-integration.md](adr/0003-rollback-active-gascity-integration.md)
  before assuming any Gas City plan is current.
- Treat [adr/0001-chrote-3-gas-city-substrate.md](adr/0001-chrote-3-gas-city-substrate.md)
  and [adr/0002-gascity-real-harness-safety-boundary.md](adr/0002-gascity-real-harness-safety-boundary.md)
  as superseded archaeology.
- Use the `experiments/` and `runbooks/` folders for evidence from specific
  attempts.

## Layout

- `adr/` - decisions, including the rollback ADR.
- `architecture/` - maps and schemas from the substrate design attempt.
- `vision/` - operator vision and collaboration-product thinking.
- `evaluations/` - broader assessment of Gas City as a meta-harness component.
- `experiments/` - smoke tests, parity checks, Dolt/quorum validation, and
  restart drills.
- `runbooks/` - split-brain and transcript-recovery notes from the old setup.
- `plans/` - abandoned implementation plans kept for context.
- `adapter-research/` - markdown-only harness-adapter research; executable
  prototypes were intentionally removed.

## Retained Outside This Folder

Raw conversation artifacts remain in agent history locations such as Claude,
Pi, and Codex session logs. This folder is the cleaned CHROTE-side research
index, not a complete transcript archive.
