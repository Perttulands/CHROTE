# What We Were Trying To Do

Status: historical record, not an active roadmap.
Date: 2026-05-30

This document preserves the intent behind the CHROTE/Gas City effort before the
implementation Beads were removed from the active Beads ledger.

The active decision is still
[ADR-0003: Roll Back Active Gas City Integration](adr/0003-rollback-active-gascity-integration.md).
CHROTE is back to being the plain tmux access system. Gas City is not installed
or assumed live.

## Short Version

We were trying to find out whether Gas City could become the orchestration
plumbing underneath CHROTE while CHROTE stayed the human access layer for named
sessions and agent identities.

The desired user-facing outcome was not a Gas City dashboard. The desired
outcome was that Perttu could work with named agents, and those agents could use
Gas City primitives to collaborate: mail, nudges, sling delegation, formulas,
molecules, events, and later automation.

The felt win would have been something like:

- Perttu talks to one named agent through CHROTE.
- That agent decides another agent should help, or Perttu asks it to involve
  someone by name.
- The agents coordinate through durable mail, delegation, workflow state, and
  Beads rather than ad hoc pane scraping and manual tmux commands.
- The result is higher-quality planning, execution, and review because multiple
  agents can work together without Perttu manually carrying messages between
  them.

## Desired System Shape

The target system had these roles:

- **CHROTE:** fast access to named tmux sessions and named agent identities.
- **Gas City:** orchestration substrate for identities, mail, nudge, sling,
  formulas, molecules, events, and automation.
- **Beads:** durable work ledger for plans, tasks, blockers, discoveries, and
  verification state.
- **Context Citadel:** durable context and knowledge across time.
- **Agents:** active users of the orchestration primitives during normal work.

The important distinction was that Gas City was supposed to be agent-facing
plumbing, not a place Perttu had to visit to launch workflows. A CHROTE UI
surface would only matter when it made named sessions easier to access, helped
spawn identities, or helped recover/inspect a collaboration that had gone wrong.

## Problems We Were Trying To Solve

The work was trying to answer these problems:

- How do named agents communicate without relying on brittle transcript watching
  or manual file drops?
- How can one agent ask another agent for review, input, or execution help in a
  durable way?
- How can recurring multi-agent workflows, such as planning plus independent
  review plus synthesis, be packaged as reusable recipes?
- How can workflow state remain inspectable and recoverable across browser,
  tmux, or process disconnects?
- How can CHROTE launch or attach to sessions that are also valid orchestration
  identities?
- How can agents use shared work records through Beads while Context Citadel
  remains the long-term knowledge layer?

## What We Tried

The removed Beads covered several workstreams:

- **Decision and architecture:** whether Gas City should be central to CHROTE
  3.0, how CHROTE primitives mapped to Gas City primitives, and what the safety
  boundary should be.
- **Agent-to-agent communication:** mail, nudge, and sling/delegation smokes,
  including the simple poem-through-mail proof attempt.
- **Real identity experiments:** shell identities, rig-scoped sessions, Codex and
  helper identities, and CHROTE session attach paths.
- **Workflow/molecule experiments:** review-quorum, formula inspection,
  graph.v2 support, Dolt-backed workflow state, and restart reconciliation.
- **Harness adapter research:** Hermes, OpenCode, Codex, Claude Code direction,
  and how different harnesses could become named team members.
- **CHROTE UI attempts:** Gas City tab, workflow launcher, observer surfaces,
  identity creation flows, and session-flow integration.
- **Cleanup and rollback:** removal of mocks, duplicate runtimes, stale configs,
  runtime artifacts, and finally rollback to the plain CHROTE tmux model.

## What We Learned

Gas City's conceptual primitives were aligned with the desired system:

- durable agent mail was the right kind of communication primitive;
- sling/delegation matched the desire to tell one agent to help another;
- formulas and molecules matched reusable team workflows;
- event streams and workflow state looked useful for recovery and audit;
- identity/session concepts were promising for named agents.

The implementation path still failed the product and engineering test:

- too much effort drifted into a Gas City tab and launcher UI that Perttu did
  not want to operate;
- mocks made progress look more real than it was;
- the real tmux/session binding and identity model were not clean enough;
- rollback became safer than continuing to accumulate adapter code and runtime
  state;
- active Beads about Gas City made the home workspace look like this was still
  current work.

## Current Decision

The active state is:

- no runnable local Gas City runtime;
- no `gc` command expected in normal CHROTE work;
- no active CHROTE code path depending on Gas City;
- no active Beads that tell agents to continue the Gas City integration;
- research, conversation artifacts, and planning documents retained here for
  later evaluation.

Future Gas City work should begin with a new explicit decision and a small
evidence-backed slice. It should not continue from the deleted Beads as if they
were active backlog.

## Reintroduction Bar

If this direction is revisited, the first slice should prove the actual desired
thing:

- one named agent can involve another named or disposable agent;
- the collaboration uses durable orchestration primitives rather than a mock or
  transcript scrape;
- the user experiences the benefit through normal CHROTE session work, not by
  operating a separate Gas City dashboard;
- the work can be explained and untangled without leaving hidden runtime state.

## Beads Removed From Active Records

The Gas City implementation Beads were exported before deletion so the source
records remain recoverable without keeping them in the active Beads ledger.

Snapshot:
`/home/perttu/rollback-snapshots/gascity-beads-removal-20260530-221903/`

Important files in that snapshot:

- `candidate-ids.txt` - the deleted Bead IDs.
- `candidate-summary.tsv` - deleted IDs, status, type, and title.
- `candidate-records.jsonl` - full JSON records for the deleted Beads.
- `full-export-before-delete.jsonl` - full Beads export before deletion.
- `full-export-after-delete.jsonl` - full Beads export after deletion.
- `result-summary.txt` - deletion counts and active-reference verification.

Deletion result:

- 104 Gas City/Gastown implementation Beads deleted.
- 172 dependency links, 210 labels, and 598 events removed with those Beads.
- Text references were updated in 15 non-deleted issues.
- Eight non-deleted historical CHROTE issues were orphaned by reference cleanup:
  `home-5na1`, `home-dp5`, `home-ezd`, `home-fv6.13`, `home-ag9`,
  `home-fv6.11`, `home-fv6.12`, and `home-fv6.14`.
- Active open/in-progress Beads had zero remaining Gas City/Gastown references
  after incidental boilerplate cleanup.

Deleted Bead IDs:

```text
home-06xp, home-0oyf, home-1kxl, home-1ozg, home-22am, home-2spf, home-360,
home-4xv, home-4xv.1, home-4xv.2, home-4xv.3, home-4xv.4, home-4xv.5,
home-4xv.6, home-4xv.6.1, home-4xv.6.2, home-4xv.6.3, home-4xv.6.4,
home-4xv.6.5, home-4xv.6.6, home-4xv.6.7, home-4xv.6.8, home-5g8a, home-5qmz,
home-5ubb, home-5uh, home-5v4k, home-6p0j, home-706s, home-7a3a, home-7jk3,
home-96kc, home-9o5, home-a2vw, home-a4rf, home-a7ze, home-ai1j, home-aibh,
home-bh3w, home-bi6d, home-bi6d.1, home-bi6d.2, home-bi6d.3, home-bi6d.4,
home-bi6d.5, home-bi6d.6, home-blb9, home-blb9.1, home-bqxu, home-caiq,
home-d0lv, home-d9r, home-ddi, home-ddtc, home-dy7t, home-e54z, home-e5yp,
home-f1f9, home-fm7n, home-fo1r, home-fppe, home-gaff, home-hm0v, home-i3l,
home-iv79, home-izft, home-jca6, home-jcml, home-jka0, home-m8ds, home-mvwv,
home-n1g, home-nylk, home-p5zd, home-p7jn, home-p81n, home-pdr, home-piis,
home-pilv, home-pv11, home-qnzi, home-srd9, home-t3p9, home-t3p9.1,
home-t3p9.2, home-t3p9.3, home-t3p9.4, home-t3p9.5, home-t3p9.6, home-t3p9.7,
home-td6q, home-tsc2, home-u55t, home-ujx8, home-ukae, home-ukyt, home-ux8v,
home-vok5, home-w0wx, home-w7vq, home-wlz9, home-ws9p, home-x20y, home-ztdj
```
