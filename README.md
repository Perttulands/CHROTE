# CHROTE

CHROTE is a browser cockpit for a host-owned development workspace.

## Version Line

CHROTE v2 is the current next-generation architecture. It is a major-version
continuation of the original CHROTE system, not a compatibility patch to v1.

The original code line is preserved for reference:

- branch: `legacy/v1`
- release: `v1.0.0`

Its job is narrow:

- show durable tmux sessions
- attach browser terminal panes to those sessions
- browse configured workspace roots
- show modern Beads issues from `bd`
- run `beads_viewer` (`bv`) as an optional tmux TUI sidecar
- surface agent-like tmux sessions in the Agents view
- wrap selected local services through a Services view without exposing local
  service secrets to the browser

The workspace source of truth is the host environment. Client devices are disposable viewports.

`PRD.md` is the canonical product source. This README is the operator quick
reference for a generic install.

## Access

Tailnet URL format:

```text
https://<tailnet-host>:<tailnet-port>/
```

Local service:

```text
http://127.0.0.1:8094/
```

Service:

```bash
systemctl --user status chrote.service
systemctl --user restart chrote.service
journalctl --user -u chrote.service -f
```

## Runtime

CHROTE can run through user systemd.

```text
CHROTE HTTP: 127.0.0.1:8094
terminal ttyd: 127.0.0.1:7683
tmux socket: /run/user/1000/chrote-tmux
workdir: <workspace-root>
allowed roots: <workspace-root>
```

The terminal proxy is loopback-only. Tailscale Serve exposes only the CHROTE HTTP server.

## Tmux

CHROTE uses a dedicated tmux socket:

```bash
TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux ls
```

The baseline session is:

```bash
TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux new-session -A -s main -c "$HOME"
```

## Agents

The Agents view is orchestrator-neutral. It watches tmux sessions whose names look like agent sessions:

```text
agent-*
claude-*
codex*
gemini-*
hermes-*
opencode*
```

Override with:

```bash
CHROTE_AGENT_PREFIXES=claude-,codex,opencode,agent-
```

The view infers simple state from recent terminal output, shows context percentage if visible in scrollback, and extracts Beads IDs such as `home-fv6.9`.

## Beads And BV

Modern `bd` remains the source of truth. `beads_viewer` (`bv`) is installed as a useful graph-aware TUI sidecar.

CHROTE discovers Beads workspaces from `CHROTE_BEADS_WORKSPACES` when set. This is intentionally separate from `CHROTE_ROOTS`: `/srv` service beads can appear in the Beads view without giving the Files view access to `/srv`.

```bash
bd version
bv --version
./bin/bv-refresh "$HOME"
curl 'http://127.0.0.1:8094/api/beads/issues?path=<workspace-root>'
```

CHROTE has a durable `bv-home` tmux session on the CHROTE socket:

```bash
TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux new-session -A -s bv-home -c "$HOME" ./bin/bv-session
```

## Services

The Services view is the CHROTE surface for selected host services.
The first services are:

| Service | Default URL | CHROTE role |
| --- | --- | --- |
| TTS Gateway | `http://127.0.0.1:3100` | enqueue spoken responses, inspect health, queue, voices, and playback state |
| Context API | `http://127.0.0.1:3200` | list, read, edit, save, inspect history, and ask grounded questions over context docs |

CHROTE talks to these services server-side. Browser clients call
`/api/services/...`, and CHROTE injects service credentials where needed.

Runtime overrides:

```bash
CHROTE_TTS_URL=http://127.0.0.1:3100
CHROTE_CONTEXT_API_URL=http://127.0.0.1:3200
CHROTE_CONTEXT_API_TOKEN=...
```

Private service adapter values live in:

```text
~/.config/chrote/services.env
```

The systemd unit loads that file if present. It must not be committed; keep
owner tokens there and use the public defaults in tracked docs and service
templates.

## Build

```bash
cd /path/to/chrote/dashboard
npm run build

cd /path/to/chrote
rm -rf src/internal/dashboard/dist
cp -r dashboard/dist src/internal/dashboard/dist

cd /path/to/chrote/src
go test ./...
go build -o ../chrote-server ./cmd/server
systemctl --user restart chrote.service
```

## Product Boundary

This install does not assume Gastown, Ralph, or vendored orchestrator components. Agent Teams and the broader meta-harness remain roadmap work outside Services V1. Old ideas that are still useful are captured in `docs/legacy-ideas.md`.
