# Agent Instructions

Use `bd` for task tracking and keep CHROTE aligned with the configured host workspace.

## Current Role

CHROTE is a durable browser cockpit for:

- tmux sessions
- browser terminal panes
- files under the configured workspace roots
- modern Beads via `bd`
- optional Beads Viewer via `bv`
- generic agent observability

It is not currently a Gastown or Ralph deployment. `bv` is available as a Beads TUI sidecar, not as the source of truth.

## Before Editing

- Check `systemctl --user status chrote.service` if touching runtime behavior.
- Check local deployment notes for host-specific context when available.
- Do not kill tmux sessions unless explicitly asked.

## Quality Gates

```bash
cd /path/to/chrote/src && go test ./...
cd /path/to/chrote/dashboard && npm run build
```

If changing frontend code that is served by the Go binary, rebuild and embed `dashboard/dist` into `src/internal/dashboard/dist`.

## Decisions

If legacy CHROTE code has a good idea but assumes Gastown or Ralph, adapt the idea to an orchestrator-neutral design and capture it in `docs/legacy-ideas.md`.
