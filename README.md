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
                                    │  │  │        │         │ tmux,  │  │                          │
                                    │  │  │        │         │ beads) │  │                          │
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

### Beads Viewer

The Beads tab integrates [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) for project issue tracking and dependency visualization:

**Sub-views:**
- **Graph**: Force-directed dependency graph showing issue relationships
- **Kanban**: Drag-and-drop board with columns by status (Open, In Progress, Blocked, Closed)
- **Triage**: AI-generated prioritization with quick wins and blockers identified
- **Insights**: PageRank scores, critical path analysis, cycle detection

**Pointing to Your Beads Project:**

The beads viewer can point to **any directory** accessible via the mounted volumes. To set up:

1. **Using the Path Selector**: Click the folder icon in the Beads tab header and either:
   - Enter a path directly (e.g., `/code/my-project`)
   - Use the file browser to navigate to your project folder

2. **Required Structure**: Your project needs a `.beads/` directory containing `issues.jsonl`

3. **Mounted Volumes**: The following paths are available:
   - `/code` → `E:/Code` (Read/Write)
   - `/vault` → `E:/Vault` (Read-Only)

**bv CLI (beads_viewer):**

The `bv` command-line tool provides:
- `bv` - Interactive TUI for browsing issues
- `bv --robot-triage` - AI-powered triage recommendations (used by dashboard)
- `bv --robot-insights` - Graph metrics and health analysis
- `bv --robot-plan` - Parallel execution planning

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
│   ├── server.js             # Main server (tmux, files, beads)
│   ├── file-routes.js        # File operations API
│   └── beads-routes.js       # Beads viewer API
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
