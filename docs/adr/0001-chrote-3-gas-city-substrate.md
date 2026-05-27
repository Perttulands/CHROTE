# ADR-0001: CHROTE 3.0 Gas City Substrate

## Status

Accepted

## Context

CHROTE is moving toward a host-owned meta-harness where Perttu can run,
observe, recover, and redirect workflows across multiple AI harnesses. Gas City
already provides primitives that overlap with the desired lower layer: sessions,
mail, dispatch, formulas, workflows, and events.

The `gc mail` poem smoke test is the key pressure against treating Gas City as
only a command-line helper. A CHROTE-launched Codex process could call `gc mail`
and return a poem, but it could only send as the default or human identity.
Arbitrary CHROTE-launched senders were not valid Gas City identities. That means
a thin CHROTE wrapper around `gc` commands would hide the hard part instead of
solving it: identity, mail, session ownership, dispatch, and recovery need to be
native Gas City concepts if Gas City is the substrate.

The current CHROTE 2.0 baseline remains the rollback point tracked by
`home-bi6d.1`. The CHROTE 3.0 workstream should not erase or silently mutate that
baseline.

## Decision

CHROTE 3.0 uses Gas City as the underlying session, mail, dispatch, workflow,
and event substrate. CHROTE remains the authenticated access, UI, operator
cockpit, recovery, and policy layer on top.

The product consequence is that CHROTE should grow from "open named tmux
sessions" toward "open named Gas City-backed agent identities." A target
operator action is: tell one named agent, such as Codxia, to help another named
agent, such as Claudia. Gas City carries the identity, mail/nudge,
sling/delegation, molecule/workflow, event, and recovery mechanics. CHROTE makes
that action reachable and inspectable.

Source-of-truth boundaries are explicit:

- Beads remains the durable work truth: issues, dependencies, acceptance
  criteria, ownership, lifecycle state, and follow-up work.
- Context Citadel remains the durable context truth: personal and project
  context, grounded retrieval, sourced contributions, and context history.
- Gas City owns orchestration: sessions, valid Gas City identities, mail,
  dispatch, formulas, molecules/workflows, nudges, events, and supervisor-owned
  runtime state.
- CHROTE owns access and operation: authentication, UI, cockpit workflows,
  recovery affordances, read/write policy, private proxying, and operator
  controls.

CHROTE may observe and proxy Gas City through bounded interfaces, but it should
not mirror the native `gc` command tree as a separate CHROTE command surface.

## Alternatives Considered

### Keep CHROTE-native orchestration

This keeps today approach closer to CHROTE's existing tmux-centric model, but it
duplicates Gas City's session, mail, dispatch, workflow, and event primitives.
It also risks rebuilding a second orchestration system while still needing to
interoperate with Gas City later.

### Build a thin wrapper around `gc`

This is attractive because it looks small, but the poem smoke showed the failure
mode: CHROTE can launch a process that calls `gc`, yet arbitrary
CHROTE-launched senders are not valid Gas City identities. A wrapper can pass
commands through, but it does not make session identity, mail authorship,
dispatch, and recovery coherent.

### Keep CHROTE 2.0 indefinitely

This is the safest operational option and remains the rollback baseline through
`home-bi6d.1`, but it does not deliver the desired reusable multi-harness
workflow substrate. It keeps CHROTE useful as a cockpit, not as the broader
meta-harness direction.

### Make Gas City own everything

This avoids a split orchestration model, but it overreaches. Gas City should not
become the work truth, durable context truth, authentication layer, or full
operator cockpit. Beads, Context Citadel, and CHROTE each keep a narrower source
of truth.

## Consequences

What gets simpler:

- CHROTE does not need to invent its own full session, mail, dispatch, workflow,
  and event ontology.
- Gas City-owned identities give mail and dispatch a real substrate instead of
  ad hoc terminal automation.
- Named-agent collaboration can be built as an operator workflow instead of a
  transcript-watching feature.
- CHROTE can focus on authenticated access, visibility, recovery, and safe
  operator controls.
- Future harnesses can follow one substrate pattern instead of one-off CHROTE
  wrappers.

What gets harder:

- CHROTE must integrate with Gas City's identity and session model rather than
  treating any launched process as a valid sender.
- Existing CHROTE views and controls need mapping onto Gas City sessions, mail,
  events, and workflows.
- Mutating controls need stricter policy, audit, and recovery behavior because
  they can drive real paid or credentialed harnesses.
- The first vertical slice must prove a real Gas City-owned identity end to end,
  not only mock sessions or CHROTE-launched shell commands.

What remains risky:

- Real harness behavior is not proven by the mock-agent or poem smoke evidence.
- Restart, transcript recovery, and workflow reconciliation still need evidence.
- Shared Beads truth must avoid split-brain state between Gas City records and
  the existing Beads workspace.
- Supervisor access must stay private and bounded; exposing raw Gas City
  mutation surfaces would bypass CHROTE policy.

## Safety And Rollback

Gas City supervisor access stays localhost-only. CHROTE must not expose the raw
supervisor directly on public or tailnet surfaces.

No broad real-harness mutation should happen before `home-4xv.5` defines the
real-harness safety boundary. Broad rollout waits until the first real identity
slice proves that one Codex, Pi, OpenCode, Claude Code, or other harness can be
owned by Gas City, receive work through Gas City primitives, and return results
through valid Gas City mail.

Rollback uses the CHROTE 2.0 baseline tracked by `home-bi6d.1`. If the Gas
City-native path fails, abandon or disable the CHROTE 3.0 workstream, return to
the 2.0 baseline, keep Beads and Context Citadel as their existing sources of
truth, and leave Gas City artifacts as experimental evidence rather than
production workflow state.
