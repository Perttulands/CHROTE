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
TTYD_PASSWORD=your-password     # For ttyd basic auth (optional)
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
| Files | `http://arena:8080/files/` | Filebrowser |
| SSH | `ssh dev@arena` | Password: `dev` |

## Architecture

The system runs as multiple Docker containers with Tailscale sidecars for secure networking:

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Tailscale Network                             │
│                      (Google Auth protected)                         │
└───────────────────┬─────────────────────────────┬───────────────────┘
                    │                             │
        ┌───────────▼───────────┐     ┌───────────▼───────────┐
        │   tailscale-arena     │     │   tailscale-ollama    │
        │   (network sidecar)   │     │   (network sidecar)   │
        └───────────┬───────────┘     └───────────┬───────────┘
                    │                             │
┌───────────────────▼───────────────────┐   ┌─────▼─────┐
│          agent-arena (:8080)          │   │  ollama   │
│  ┌─────────────────────────────────┐  │   │  (:11434) │
│  │         nginx (reverse proxy)   │  │   │           │
│  │  ┌────────┬─────────┬────────┐  │  │   │  LLM API  │
│  │  │   /    │/terminal│ /api/  │  │  │   └───────────┘
│  │  │  React │  ttyd   │Node.js │  │  │
│  │  └────────┴─────────┴────────┘  │  │
│  └─────────────────────────────────┘  │
│  ┌─────────────────────────────────┐  │
│  │  filebrowser (:8081 → /files/) │  │
│  └─────────────────────────────────┘  │
└───────────────────────────────────────┘
```

**Containers:**
- `agent-arena` - Main dev environment (nginx, ttyd, API, SSH)
- `ollama` - Local LLM inference server
- `filebrowser` - Web-based file manager
- `tailscale-arena` / `tailscale-ollama` - Network sidecars for secure access

## Ollama Integration

The system includes a local LLM server accessible at `http://ollama:11434` from within containers.

**From inside agent-arena:**
```bash
curl http://ollama:11434/api/tags          # List available models
curl http://ollama:11434/api/generate -d '{"model":"llama3","prompt":"Hello"}'
```

**From Tailscale network:**
```bash
curl http://ollama:11434/api/tags          # Direct access via tailscale-ollama
```

**Model storage:** Models are persisted in `E:/LLM_models` on the host.

## Dashboard Features

- **Terminal View**: 1-4 terminal panes with drag-and-drop session assignment
- **Session Panel**: Lists all tmux sessions, drag to assign to windows
- **Files View**: Native file browser with full theme integration
- **Beads View**: Project dependency visualization with multiple sub-views:
  - **Graph**: Interactive dependency DAG with zoom/pan
  - **Kanban**: Issue board organized by status
  - **Triage**: AI-powered prioritization recommendations
  - **Insights**: Graph metrics, health score, and critical path analysis
- **Status View**: Service health and quick commands
- **Settings View**: Theme selection (Matrix/Dark/Gastown), font size, and preferences
- **Music Player**: Ambient background music in tab bar (Gastown soundtrack)
- **Nuke All Sessions**: Quick destroy all tmux sessions from Status view
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
- Browse `/srv/code` (E:/Code) and `/srv/vault` (E:/Vault) directories
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

### Beads Viewer

The Beads tab integrates [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) for project issue tracking and dependency visualization:

**Sub-views:**
- **Graph**: Force-directed dependency graph showing issue relationships
- **Kanban**: Drag-and-drop board with columns by status (Open, In Progress, Blocked, Closed)
- **Triage**: AI-generated prioritization with quick wins and blockers identified
- **Insights**: PageRank scores, critical path analysis, cycle detection

**Data Source:**
- Reads from `.beads/issues.jsonl` in your project directory
- Uses `bv` CLI robot protocol for AI-powered analysis
- Supports multiple projects via project selector

**API Endpoints:**
- `GET /api/beads/issues` - Raw issue data
- `GET /api/beads/triage` - AI triage recommendations
- `GET /api/beads/insights` - Graph metrics
- `GET /api/beads/plan` - Parallel execution tracks

## Security

This setup is designed for use behind Tailscale with Google Auth:

- No services exposed to public internet
- All access requires Tailscale network membership
- SSH available only within tailnet
- Filebrowser runs with `--noauth` (protected by Tailscale)
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
├── api/                      # Node.js API for tmux management
│   ├── server.js             # Main API server
│   └── beads-routes.js       # Beads API endpoints
├── dashboard/                # React + TypeScript web UI
│   ├── src/
│   │   ├── beads_module/     # Self-contained Beads integration
│   │   └── components/       # Core dashboard components
│   ├── tests/                # Playwright tests
│   └── dist/                 # Built assets (copied to container)
├── nginx/                    # nginx config
├── internal/                 # Internal tmux helpers
├── sandbox_overrides/        # Empty files overlaid on secrets in sandbox
├── beads_viewer_integration/ # Integration analysis docs
├── tailscale_state/          # Persisted Tailscale identity (preserves hostname)
├── filebrowser_data/         # Filebrowser config & database
├── build1.dockerfile         # Main container definition
├── ollama.dockerfile         # Ollama LLM container
├── docker-compose.yml        # Service orchestration
├── AgentArena-Toggle.ps1     # PowerShell start/stop toggle script
├── Create-Shortcut.ps1       # Creates desktop shortcut
├── start-arena.bat           # Batch start script
├── stop-arena.bat            # Batch stop script
├── test-sessions.sh          # Creates sample tmux sessions for testing
└── .env                      # Secrets (not in git, hidden from sandbox)
```

## Troubleshooting

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

### Ollama not responding

```bash
curl http://ollama:11434/api/tags                           # From inside arena
docker compose logs ollama                                  # Check ollama logs
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
