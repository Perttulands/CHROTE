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

## Service Lanes

During the `/srv` migration, distinguish the active service lane before touching runtime behavior:

- `/srv` proving service: source `/srv/chrote`, data `/srv/data/chrote`, system unit `chrote-srv.service`, HTTP `127.0.0.1:8095`, ttyd `7686`.
- Legacy rollback user service: source `/home/perttu/chrote`, user unit `chrote.service`, HTTP `127.0.0.1:8094`, ttyd `7683`.

Do not restart or deploy the legacy user service while working in `/srv/chrote` unless the task explicitly says legacy, rollback, or `8094`.

## Before Editing

- Check `systemctl status chrote-srv.service --no-pager` if touching `/srv` runtime behavior.
- Check `systemctl --user status chrote.service --no-pager` only when targeting the legacy rollback lane.
- Check local deployment notes for host-specific context when available.
- Do not kill tmux sessions unless explicitly asked.

## Quality Gates

```bash
cd /path/to/chrote/src && go test ./...
cd /path/to/chrote/dashboard && npm run build
```

If changing frontend code that is served by the Go binary, rebuild and embed `dashboard/dist` into `src/internal/dashboard/dist`, then rebuild the relevant `chrote-server` binary and restart only the intended service lane.

## Decisions

If legacy CHROTE code has a good idea but assumes Gastown or Ralph, adapt the idea to an orchestrator-neutral design and capture it in `docs/legacy-ideas.md`.
