# CLAUDE.md

Guidance for agents working on this CHROTE install.

## Product Boundary

This is the CHROTE cockpit for the configured host workspace.

Do not assume Gastown, Ralph, or vendored orchestrator components are installed. CHROTE watches tmux sessions and modern Beads state. `bv` is installed as an optional Beads TUI sidecar.

For the current CHROTE/Gas City relationship, read `docs/chrote-gascity-framing.md`. Native `gc` is the Gas City operator CLI; do not mirror the `gc` command tree inside CHROTE.

Golden rule: do not disrupt running shells or tmux sessions.

## Build And Verify

```bash
cd /path/to/chrote/src
go test ./...

cd /path/to/chrote/dashboard
npm run build

cd /path/to/chrote
rm -rf src/internal/dashboard/dist
cp -r dashboard/dist src/internal/dashboard/dist

cd /path/to/chrote/src
go build -o ../chrote-server ./cmd/server
systemctl --user restart chrote.service
```

## Service

```bash
systemctl --user status chrote.service
journalctl --user -u chrote.service -f
curl http://127.0.0.1:8094/api/health
```

Runtime:

```text
HTTP: 127.0.0.1:8094
terminal ttyd: 127.0.0.1:7683
tmux socket: /run/user/1000/chrote-tmux
workdir: configured workspace root
allowed roots: configured workspace roots
```

## Architecture

```text
src/cmd/server/main.go          entry point, middleware, routes
src/internal/api/tmux.go        tmux session API
src/internal/api/files.go       file browser API
src/internal/api/beads.go       modern bd-backed Beads API
src/internal/api/oracle.go      generic agent observability API
src/internal/proxy/terminal.go  ttyd proxy
dashboard/src/                  React dashboard
```

The `/api/oracle/*` route names are compatibility names. Product UI calls this surface `Agents`.

## Active Views

- Terminal
- Terminal 2
- Files
- Agents
- Beads
- Settings
- Help

## Beads

Use modern `bd` for issue tracking. The primary project path is the configured workspace root.

```bash
bd ready
bd show <id>
bd update <id> --status in_progress
bd close <id>
```

Use `bv` for graph-aware viewing or robot insights after exporting modern bd data:

```bash
/path/to/chrote/bin/bv-refresh <workspace-root>
bv --db <workspace-root>/.beads --robot-next
```

## Cleanup Rules

- Remove old active coupling to Gastown/Ralph when found.
- Preserve good ideas in `docs/legacy-ideas.md` instead of silently deleting them.
- Keep all listeners localhost-only unless the user explicitly approves a broader binding.
- Tailscale Serve is the external access layer.
