# CLAUDE.md

Guidance for agents working on this CHROTE install.

## Product Boundary

This is the CHROTE cockpit for the configured host workspace.

Do not assume Gastown, Ralph, or vendored orchestrator components are installed. CHROTE watches tmux sessions and modern Beads state. `bv` is installed as an optional Beads TUI sidecar.

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
```

Restart only the intended service lane after rebuilding:

```bash
# /srv proving lane
sudo systemctl restart chrote-srv.service

# legacy rollback lane only
systemctl --user restart chrote.service
```

## Service

```bash
# /srv proving lane
systemctl status chrote-srv.service --no-pager
journalctl -u chrote-srv.service -f
curl http://127.0.0.1:8095/api/health

# legacy rollback lane
systemctl --user status chrote.service --no-pager
journalctl --user -u chrote.service -f
curl http://127.0.0.1:8094/api/health
```

Runtime:

```text
/srv proving lane:
  source: /srv/chrote
  data: /srv/data/chrote
  HTTP: 127.0.0.1:8095
  terminal ttyd: 127.0.0.1:7686

legacy rollback lane:
  source: /home/perttu/chrote
  HTTP: 127.0.0.1:8094
  terminal ttyd: 127.0.0.1:7683

tmux sockets/workdirs/allowed roots: configured by the active service env; do not print private env values.
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
