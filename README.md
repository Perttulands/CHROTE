# Agent Arena

A Docker-based development environment with web dashboard for managing AI coding agents via tmux sessions.

## Quick Start

### Prerequisites

- Docker Desktop
- Tailscale account (for secure remote access)

### 1. Create Environment File

```bash
cp .env.example .env
```

Edit `.env` with your credentials:

```env
TS_AUTHKEY=tskey-auth-xxxxx    # From Tailscale admin console
```

### 2. Build Dashboard (first time only)

```bash
cd dashboard
npm install
npm run build
cd ..
```

### 3. Create Desktop Shortcut (Windows)

Create a desktop shortcut for easy start/stop control:

```powershell
powershell -ExecutionPolicy Bypass -File "e:\Docker\AgentArena\Create-Shortcut.ps1"
```

This creates an **AgentArena** shortcut on your desktop that:
- Shows current status (running/stopped)
- Press Enter to toggle start/stop
- Automatically starts Docker Desktop if needed
- Shows progress and waits for services to be ready

### 4. Start Services

**Option A**: Double-click the desktop shortcut (recommended)

**Option B**: Command line
```bash
docker compose up -d agent-arena
```

### 5. Access

Once running, access via Tailscale hostname:

| Service | URL | Description |
|---------|-----|-------------|
| Dashboard | `http://arena:8080` | Main web UI |
| Terminal | `http://arena:8080/terminal/` | Direct ttyd access |
| Files | `http://arena:8080/api/files/` | File API |
| SSH | `ssh dev@arena` | Password: `dev` |

## Gastown

Agent Arena is the infrastructure that powers **Gastown**, an orchestration framework for running 10-30+ AI coding agents in parallel via tmux sessions. Gastown implements the "MEOW Stack" (Molecular Expression Of Work) - a workflow system where:

- **Beads** are atomic units of work
- **Epics** collect beads with parallel children
- **Molecules** chain complex workflows
- **Wisps** handle ephemeral orchestration tasks

The core philosophy is "Physics over Politeness" - sessions are ephemeral and expendable, throughput is the mission. Actions are designed to be **idempotent** (safe to repeat). The "Nuke All" button embodies this: destroy everything, start fresh.

Session naming follows the pattern `gt-{rigname}-{worker}` (e.g., `gt-gastown-jack`) for Gastown rig workers, with `hq-*` sessions for headquarters/coordination.

## In-Container Guide (also copied to /README.md)

This section is intended to be the quick reference you can read from inside the container.

### Web Dashboard

Access the dashboard from any Tailnet device:

```text
http://arena:8080/
```

### Quick Reference

| What | How |
|------|-----|
| Dashboard | `http://arena:8080/` |
| Terminal | `http://arena:8080/terminal/` |
| Your code | `/code` (E:/Code on host) |
| Vault | `/vault` (E:/Vault on host) |
| SSH (dev) | `ssh dev@arena` |

### Directory Structure

```text
/code     - Your projects (read/write)
/vault    - Reference files (read-only for dev user)
/home/dev - Your home directory (persisted across rebuilds)
```

### Users

| User | Password | Notes |
|------|----------|-------|
| dev | dev | Standard user, sudo access |
| root | root | Can write everywhere including /vault |

### Important: tmux sessions are per-user

tmux runs a separate server per user (socket lives under `/tmp/tmux-<uid>/`). The dashboard/API/ttyd run as `dev` (uid 1000).

If you create sessions as `root` (uid 0), they will not show up in the dashboard and it can look like sessions “randomly disappear”.

Use the built-in helpers (work even if you are logged in as root):

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

### Running Gastown

```bash
gt-dev start gastown
gt-dev status
gt-dev peek
```

### Session Naming Conventions

| Prefix | Example | Dashboard Group |
|--------|---------|-----------------|
| `hq-` | `hq-mayor` | HQ |
| `gt-rigname-` | `gt-gastown-jack` | Rig (`gt-gastown`) |
| `main`, `shell` | `main` | Main |
| Other | `my-session` | Other |

### Testing Session Display

```bash
# Create sample sessions (run inside the container)
bash /code/AgentArena/test-sessions.sh

# Or manually (recommended: use dev wrappers so they appear in the dashboard)
tmux-dev new-session -d -s "hq-test1"
tmux-dev new-session -d -s "gt-myrig-agent1"
tmux-dev list-sessions
```

### Vault Usage

Drop files into `/vault` as root (or from Windows at E:/Vault) to provide read-only context to agents:

```bash
# From arena as root:
cp /code/important-spec.md /vault/
```

### Dev Servers

Bind to `0.0.0.0` to make accessible from your other devices:

```bash
# Vite/React
npm run dev -- --host 0.0.0.0

# Flask
flask run --host=0.0.0.0

# FastAPI
uvicorn main:app --host 0.0.0.0
```

## Architecture

The system runs as Docker containers with a Tailscale sidecar for secure networking:

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Tailscale Network                             │
│                      (Google Auth protected)                         │
└───────────────────────────────────────────────────────────────────┬──┘
                                                                    │
                                                    ┌───────────────▼───────────┐
                                                    │   tailscale-arena         │
                                                    │   (network sidecar)       │
                                                    └───────────────┬───────────┘
                                                                    │
                                    ┌───────────────────────────────▼───────────────────────────────┐
                                    │          agent-arena (:8080)                                  │
                                    │  ┌─────────────────────────────────┐                          │
                                    │  │         nginx (reverse proxy)   │                          │
                                    │  │  ┌────────┬─────────┬────────┐  │                          │
                                    │  │  │   /    │/terminal│ /api/  │  │                          │
                                    │  │  │  React │  ttyd   │Node.js │  │                          │
                                    │  │  │        │         │(files, │  │                          │
                                    │  │  │        │         │ tmux)  │  │                          │
                                    │  │  └────────┴─────────┴────────┘  │                          │
                                    │  └─────────────────────────────────┘                          │
                                    └───────────────────────────────────────────────────────────────┘
```

**Containers:**
- `agent-arena` - Main dev environment (nginx, ttyd, API, SSH)
- `tailscale-arena` - Network sidecar for secure access

## Dashboard Features

- **Terminal View**: 1-4 terminal panes with drag-and-drop session assignment
- **Session Panel**: Lists all tmux sessions, drag to assign to windows
- **Files View**: Native file browser with full theme integration
- **Settings View**: Theme selection (Matrix/Dark/Gastown), font size, and preferences
- **Music Player**: Ambient background music in tab bar (Gastown soundtrack)
- **Nuke All Sessions**: Quick destroy all tmux sessions
- **Floating Modal**: Peek at sessions without leaving current view

### Session Management

1. Sessions appear in the left sidebar grouped by type (HQ, Main, Rigs)
2. Drag a session onto a terminal window to attach
3. Click session tags to switch between assigned sessions
4. Use ← → buttons to cycle through sessions in a window
5. **Nuke All** button destroys all sessions at once (with confirmation)

Layout state persists in localStorage.

### File Browser

The native file browser provides full access to mounted volumes with theme-adaptive styling:

**Features:**
- Browse `/code` (E:/Code) and `/vault` (E:/Vault) directories
- List view with sortable columns (Name, Size, Modified)
- Grid view with icon thumbnails
- Upload files via drag-and-drop or click-to-select
- Download, rename, delete files/folders
- Create new folders
- Filter/search files in current directory
- Breadcrumb navigation with history (Back/Forward/Up)

**Keyboard Shortcuts:**
- `Enter` - Open folder / Download file
- `Backspace` - Go to parent directory
- `F2` - Rename selected item
- `Delete` - Delete selected item
- `F5` - Refresh
- `Ctrl+A` - Select all

**Selection:**
- Click to select single item
- Ctrl+Click for multi-select
- Shift+Click for range select
- Right-click for context menu

## Security

This setup is designed for use behind Tailscale with Google Auth:

- No services exposed to public internet
- All access requires Tailscale network membership
- SSH available only within tailnet
- File API has no auth (protected by Tailscale)
- Sensitive files (`.env`) are hidden from sandbox via volume overlays

**Do not expose port 8080 to the public internet.**

See [SECURITY.md](SECURITY.md) for ACL configuration and threat model.

## Development

### Run Dashboard Locally

```bash
cd dashboard
npm install
npm run dev    # Dev server at localhost:5173
```

### Run Tests

```bash
cd dashboard
npm run test
```

### Rebuild Container

After code changes:

```bash
cd dashboard && npm run build && cd ..
docker compose build --no-cache agent-arena
docker compose up -d agent-arena
```

## File Structure

```
AgentArena/
├── api/                      # Node.js API server
│   ├── server.js             # Main server (tmux, files)
│   └── file-routes.js        # File operations API
├── dashboard/                # React + TypeScript web UI
│   ├── src/
│   │   └── components/       # Core dashboard components
│   ├── tests/                # Playwright tests
│   └── dist/                 # Built assets (copied to container)
├── nginx/                    # nginx config
├── sandbox_overrides/        # Empty files overlaid on secrets in sandbox
├── tailscale_state/          # Persisted Tailscale identity (preserves hostname)
├── build1.dockerfile         # Main container definition
├── docker-compose.yml        # Service orchestration
├── AgentArena-Toggle.ps1     # PowerShell start/stop toggle script
├── Create-Shortcut.ps1       # Creates desktop shortcut
├── start-arena.bat           # Batch start script
├── stop-arena.bat            # Batch stop script
├── test-sessions.sh          # Creates sample tmux sessions for testing
└── .env                      # Secrets (not in git, hidden from sandbox)
```

## Troubleshooting

### Sessions "disappear" (root vs dev tmux split)

tmux runs a separate server per user (socket lives under `/tmp/tmux-<uid>/`). If you create sessions as `root` (uid 0) but the Arena dashboard/API runs as `dev` (uid 1000), it will look like sessions randomly disappear.

Use these commands inside the container:

- Show both servers and sockets: `arena-sessions`
- List dev sessions (recommended): `tmux-dev ls`
- Run Gastown/Beads as dev even from a root shell: `gt-dev ...`, `bd-dev ...`

Root interactive shells default to these wrappers to prevent accidental root-owned tmux servers. If you truly need root tmux (rare), use `tmux-root`.

### Container won't start

```bash
docker compose logs agent-arena
```

### Sessions not showing in dashboard

Verify API is running:
```bash
curl http://arena:8080/api/tmux/sessions
```

### Terminal shows black screen

Check ttyd logs inside container:
```bash
docker exec -it agentarena-dev ps aux | grep ttyd
```

### tmux sessions not visible between terminal and API

Ensure `TMUX_TMPDIR=/tmp` is set consistently. The API, ttyd, and SSH all need the same socket path:
```bash
docker exec -it agentarena-dev bash -c 'echo $TMUX_TMPDIR'  # Should be /tmp
docker exec -it agentarena-dev ls /tmp/tmux-*/              # Check socket exists
```

### Tailscale not connecting

```bash
docker compose logs tailscale-arena
docker exec -it tailscale-arena tailscale status
```

## See Also

| Document | Description |
|----------|-------------|
| [PRD.md](PRD.md) | Product requirements - user needs and acceptance criteria |
| [SPEC.md](SPEC.md) | Technical specification - architecture, implementation, anti-patterns |
| [SECURITY.md](SECURITY.md) | Tailscale ACLs, threat model, secret protection |

## License

MIT
