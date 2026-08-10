# Legacy Ideas Worth Keeping

This CHROTE install is not the old Gastown-oriented stack. The useful ideas from that stack should be retained deliberately, without carrying stale dependencies.

## Keep

- Tmux as the universal session bus.
- Browser as a disposable cockpit, not the durability boundary.
- Agent observability: list agent sessions, show status, show recent output, show related Beads IDs, and allow human intervention.
- Beads in the cockpit: Kanban, ready work, blockers, triage, and health.
- File access in the same cockpit.
- Iframe lifecycle care: hidden terminal iframes must not corrupt tmux dimensions.
- Team topology as metadata: roles, edges, launch plans, and visual relationships between sessions.
- Remote proxy-operator idea: chat or voice can query the same CHROTE API later.

## Do Not Carry Forward As Assumptions

- Gastown is installed.
- Session names use `gt-*`.
- `bv` is the Beads source of truth. In this install, modern `bd` is the source of truth and `bv` is a sidecar.
- Ralph projects exist.
- Agent communication is owned by CHROTE.
- Team launch/harness code is production-ready.

## Historical Formations Experiments

- The redaction/replay quarry explored preventing agent-controlled output and
  completion references from persisting, then failing replay when authoritative
  raw input was unavailable. The owner cut it: CHROTE is a solo,
  trusted-operator system, its proving missions had no must-scrub-secrets
  workflow, and the recovery/authority machinery added no current product value.
  Reopen only for a concrete secret-bearing workflow.
- `feat/formations-solid-ground` was an explicitly unreviewed WIP lane around
  the same work. Its reusable CI assertion already landed on `main`; the
  remaining redaction/replay material shares the cut above, so preserve the
  decision here rather than the branch.

## Future Agent Monitor Direction

The old Oracle concept should become a generic Agents view:

- Detect agent-like tmux sessions by configurable prefixes.
- Show state inferred from terminal output, not from a specific orchestrator.
- Extract modern Beads IDs such as `home-fv6.9`.
- Add explicit agent metadata later through a small sidecar file or `bd` labels.
- Add message/nudge support only after the command channel is safe and auditable.

## Future Team Direction

The old Teams concept should return only as declarative metadata:

```yaml
name: verifier-loop
members:
  - session: claude-builder
    role: builder
  - session: codex-reviewer
    role: reviewer
edges:
  - from: claude-builder
    to: codex-reviewer
    type: review-after-change
```

CHROTE should visualize and navigate this topology. The agent harnesses should own IPC, prompts, sockets, and stop hooks.
