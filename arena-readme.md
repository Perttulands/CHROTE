## Deprecated

The content of this file has been merged into the canonical repository README.

- Read in repo: `README.md`
- Read in container: `/README.md`

This stub exists only to avoid breaking older links.

## Web Dashboard

Access the dashboard from any Tailnet device:

```
http://arena:8080/
```

The dashboard provides:
- **Terminal** - Multi-pane tmux session viewer with drag-and-drop session assignment
- **Files** - Browse and edit files in /code and /vault
- **Status** - Service health and quick command reference

### Session Management
- **Click sessions** in the sidebar to open them in the first window (swap view)
- **Drag sessions** onto terminal windows to assign them
- **Click session tags** in window headers to switch between assigned sessions
- **Use ← → buttons** to cycle through sessions in a window
- Sessions are actual tmux sessions running as the `dev` user
- Switching views does NOT interrupt running agents - they continue in the background

### Settings
- **Font Size**: Adjustable terminal font (12-20px)
- **Theme**: Matrix (green hacker), Dark (neutral), or Gastown (warm coffee/gold)
- **Session Prefix**: Default naming for new sessions (e.g., `Terminal-1`)

Settings are automatically persisted to browser localStorage.

## Quick Reference

| What | How |
|------|-----|
| Dashboard | `http://arena:8080/` (from any Tailnet device) |
| Your code | `/code` (E:/Code on host) |
| Read-only vault | `/vault` (E:/Vault on host) |
| Run Claude | `claude` or `claude --dangerously-skip-permissions` |

## Directory Structure

```
/code    - Your projects (read/write)
/vault   - Reference files (read-only for dev user)
/home/dev - Your home directory (persisted across rebuilds)
```

## Users

| User | Password | Notes |
|------|----------|-------|
| dev | dev | Standard user, sudo access, cannot write to /vault |
| root | root | Can write everywhere including /vault |

### Important: tmux sessions are per-user

tmux runs a separate server per user (socket lives under `/tmp/tmux-<uid>/`).

The dashboard/API/ttyd run as `dev` (uid 1000). If you create sessions as `root` (uid 0), they will *not* show up in the dashboard and it can look like Gastown sessions “randomly disappear”.

Use these helpers (work even if you are logged in as root):

```bash
# Show both dev + root tmux servers + sockets
arena-sessions

# Always interact with the dev tmux server
tmux-dev ls
tmux-dev a -t gt-gastown-jack

# Always run orchestrators as dev
gt-dev status
bd-dev --help
```

If you truly need root's tmux (rare), use `tmux-root`.

## Installed Tools

- **Claude Code** - `claude --help`
- **Gastown** - `gt --help`
- Git, tmux, Go, Node.js, npm, Python3, pip

## Running Gastown

Start a Gastown rig to orchestrate AI agents:

```bash
# Start gastown with a rig
gt start gastown

# Check status
gt status

# Monitor activity
gt peek
```

Agents run in tmux sessions. Use the web dashboard to monitor and interact with them from any device.

### Session Naming Conventions

Gastown creates sessions with specific prefixes that the dashboard groups automatically:

| Prefix | Example | Dashboard Group |
|--------|---------|-----------------|
| `hq-` | `hq-mayor` | HQ |
| `gt-rigname-` | `gt-gastown-jack` | Gastown (rig name) |
| `main`, `shell` | `main` | Main |
| `Terminal-` | `Terminal-1` | Other (dashboard-created) |
| Other | `my-session` | Other |

### Testing Session Display

To test the dashboard without running Gastown:

```bash
# Create test sessions (run inside the container)
bash /code/AgentArena/test-sessions.sh

# Or manually (recommended: use dev wrappers so they appear in the dashboard):
tmux-dev new-session -d -s "hq-test1"
tmux-dev new-session -d -s "gt-myrig-agent1"
tmux-dev list-sessions
```

## Running AI Agents

This container is network-isolated via Tailscale ACLs. Safe to run:

```bash
claude --dangerously-skip-permissions
```

The container can only reach:
- The internet (npm, pip, git, APIs)
- NOT your other Tailnet devices (NAS, other machines)

## Dev Servers

Bind to `0.0.0.0` to make accessible from your other devices:

```bash
# Vite/React
npm run dev -- --host 0.0.0.0

# Flask
flask run --host=0.0.0.0

# FastAPI
uvicorn main:app --host 0.0.0.0

# Next.js
npm run dev -- -H 0.0.0.0
```

Access from any Tailnet device at `http://arena:3000` (or other port).

## Vault Usage

Drop files into `/vault` as root (or from Windows at E:/Vault) to provide read-only context to agents:

```bash
# From Windows: copy files to E:/Vault
# From arena as root:
ssh root@arena
cp /code/important-spec.md /vault/
```

The dev user (and agents) can read but not modify vault contents.

## Troubleshooting

```bash
# Check Tailscale status
tailscale status

# Check internet
curl -I https://google.com

# Rebuild container (from Windows host)
# cd E:\Docker\AgentArena
# docker compose down
# docker compose build --no-cache agent-arena
# docker compose up -d agent-arena
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Your Devices (laptop, phone, tablet)                       │
│  Access: http://arena:8080/                                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ Tailscale
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  arena (Tailscale node)                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  agent-arena container                               │   │
│  │  - nginx (:8080) serves dashboard + proxies          │   │
│  │    └─ Dashboard UI    → /                            │   │
│  │    └─ Terminal proxy  → /terminal/ → ttyd (:7681)    │   │
│  │    └─ API proxy       → /api/      → node (:3001)    │   │
│  │  - ttyd (web terminal, runs as dev)                  │   │
│  │  - tmux sessions (dev user, managed via API)         │   │
│  │  - API server (runs as dev, manages tmux)            │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Network Security

This container is isolated via Tailscale ACLs:
- Can reach: internet
- Cannot reach: Your NAS, host machine, other Tailnet devices

Even if compromised, blast radius is limited to this disposable container.
