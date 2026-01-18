# CHROTE

![CHROTE](CHROTE.png)

**C**ontrol **H**ub for **R**emote **O**perations & **T**mux **E**xecution

A web dashboard for managing AI coding agents via tmux sessions. Runs in WSL2 with native Linux performance.

## Quick Start

### Prerequisites

- Windows 11 with WSL2
- Ubuntu 24.04 in WSL
- Tailscale account (for secure remote access)

### Installation

**Automated setup:**

```powershell
# 1. Install Ubuntu 24.04 in WSL
wsl --install -d Ubuntu-24.04

# 2. Open WSL as root and run setup script
wsl -d Ubuntu-24.04 -u root
curl -fsSL https://raw.githubusercontent.com/peterje/chrote/main/wsl/setup-wsl.sh | bash

# 3. Restart WSL to apply changes
wsl --shutdown
```

**Manual setup:** See [docs/WSL-migration-plan.md](docs/WSL-migration-plan.md) for step-by-step instructions.

### Start Services

Services auto-start via systemd when WSL boots:

```powershell
# Option 1: Use the PowerShell toggle script
.\wsl\Chrote-Toggle.ps1

# Option 2: Manual start
wsl -d Ubuntu-24.04 echo "CHROTE started"
Start-Process "http://chrote:8080"
```

**Toggle script options:**
- `.\Chrote-Toggle.ps1` - Start and open browser (or just open if running)
- `.\Chrote-Toggle.ps1 -Stop` - Shutdown WSL completely
- `.\Chrote-Toggle.ps1 -Status` - Show service status
- `.\Chrote-Toggle.ps1 -Logs` - Stream service logs

### Optional: Developing Gastown / Beads locally

Clone your forks into the vendor directory:
```bash
git clone <your-fork-url> vendor/gastown
git clone <your-fork-url> vendor/beads
git clone <your-fork-url> vendor/beads_viewer
```

Build inside WSL:
```bash
cd vendor/gastown && go build -o ~/.local/bin/gt ./cmd/gt
cd vendor/beads && go build -o ~/.local/bin/bd ./cmd/bd
cd vendor/beads_viewer && go build -o ~/.local/bin/bv ./cmd/bv
```

### Access

Once running, access via Tailscale hostname:

| Service | URL | Description |
|---------|-----|-------------|
| Dashboard | `http://chrote:8080` | Main web UI |
| Terminal | `http://chrote:8080/terminal/` | Direct ttyd access |
| Files | `http://chrote:8080/api/files/` | File API |

## Gastown

CHROTE is the infrastructure that powers **Gastown**, an orchestration framework for running 10-30+ AI coding agents in parallel via tmux sessions. Gastown implements the "MEOW Stack" (Molecular Expression Of Work) - a workflow system where:

- **Beads** are atomic units of work
- **Epics** collect beads with parallel children
- **Molecules** chain complex workflows
- **Wisps** handle ephemeral orchestration tasks

The core philosophy is "Physics over Politeness" - sessions are ephemeral and expendable, throughput is the mission. Actions are designed to be **idempotent** (safe to repeat). The "Nuke All" button embodies this: destroy everything, start fresh.

Session naming follows the pattern `gt-{rigname}-{worker}` (e.g., `gt-gastown-jack`) for Gastown rig workers, with `hq-*` sessions for headquarters/coordination.

## Quick Reference (inside WSL)

### Web Dashboard

Access the dashboard from any Tailnet device:

```text
http://chrote:8080/
```

### Quick Reference

| What | How |
|------|-----|
| Dashboard | `http://chrote:8080/` |
| Terminal | `http://chrote:8080/terminal/` |
| Your code | `/code` → `/home/chrote/chrote` |
| Vault | `/vault` → `/mnt/e/Vault` (read-only) |

### Directory Structure

```text
/code     - Your projects (read/write, symlink to home)
/vault    - Reference files (read-only, symlink to Windows)
```

### User

All operations run as `chrote` (non-root, no sudo). This provides proper security isolation - agents cannot escalate privileges.

### tmux Sessions

All sessions use the `/run/tmux/chrote/` socket. The dashboard, API, and ttyd all run as `chrote`, so all sessions are visible.

```bash
# List all sessions
tmux list-sessions

# Ensure TMUX_TMPDIR is set
echo $TMUX_TMPDIR  # Should be /run/tmux/chrote
```

### Running Gastown

```bash
gt start gastown
gt status
gt peek
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
tmux new-session -d -s "hq-test1"
tmux new-session -d -s "gt-myrig-agent1"
tmux list-sessions
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

The system runs in WSL2 with systemd managing services:

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Tailscale Network                             │
│                      (Google Auth protected)                         │
└───────────────────────────────────────────────────────────────────┬──┘
                                                                    │
                                    ┌───────────────────────────────▼───────────────────────────────┐
                                    │                    WSL2 (Ubuntu 24.04)                         │
                                    │                   User: chrote (non-root)                      │
                                    │                                                                │
                                    │  ┌───────────────────────────────────────────────────────┐    │
                                    │  │                   systemd services                     │    │
                                    │  │  ┌─────────────────────┐  ┌─────────────────────┐     │    │
                                    │  │  │ chrote-server :8080 │  │ chrote-ttyd :7681   │     │    │
                                    │  │  │ (Go binary)         │  │ (web terminal)      │     │    │
                                    │  │  │  - Dashboard        │  │  - tmux attach      │     │    │
                                    │  │  │  - API              │  │                     │     │    │
                                    │  │  └─────────────────────┘  └─────────────────────┘     │    │
                                    │  └───────────────────────────────────────────────────────┘    │
                                    │                                                                │
                                    │  Tailscale (native) → hostname: chrote                        │
                                    └───────────────────────────────────────────────────────────────┘
```

**Services:**
- `chrote-server` - Go binary serving dashboard + API (port 8080)
- `chrote-ttyd` - Web terminal for tmux access (port 7681)
- Tailscale runs natively in WSL for network access

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
- File API has no auth (protected by Tailscale)
- Agents run as non-root `chrote` user (no sudo)
- File access restricted to `/code` and `/vault` only

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

### Rebuild Server

After code changes:

```bash
# Rebuild dashboard
cd dashboard && npm run build && cd ..

# Copy to Go server and rebuild
cp -r dashboard/dist src/internal/dashboard/
cd src && go build -o ../chrote-server ./cmd/server

# Restart service
sudo systemctl restart chrote-server
```

## File Structure

```
CHROTE/
├── src/                      # Go server (single binary)
│   ├── cmd/server/           # Entry point
│   └── internal/             # API handlers, proxy, dashboard embed
├── dashboard/                # React + TypeScript web UI
│   ├── src/
│   │   └── components/       # Core dashboard components
│   ├── tests/                # Playwright tests
│   └── dist/                 # Built assets (embedded in Go binary)
├── wsl/                      # WSL setup assets
│   ├── setup-wsl.sh          # Automated WSL setup script
│   ├── Chrote-Toggle.ps1     # Windows launcher script
│   └── wsl_assets/           # systemd services, scripts, configs
├── vendor/                   # Optional: gastown, beads, beads_viewer
├── docs/                     # Documentation
├── start-chrote.bat          # Batch start script
├── stop-chrote.bat           # Batch stop script
└── test-sessions.sh          # Creates sample tmux sessions for testing
```

## Troubleshooting

### Sessions "disappear"

All sessions should be visible via the shared socket at `/run/tmux/chrote/`. Verify:

```bash
echo $TMUX_TMPDIR  # Should be /run/tmux/chrote
ls -la /run/tmux/chrote/  # Check socket exists
```

### Services won't start

```bash
systemctl status chrote-server chrote-ttyd
journalctl -u chrote-server -f
```

### Sessions not showing in dashboard

Verify API is running:
```bash
curl http://chrote:8080/api/tmux/sessions
```

### Terminal shows black screen

Check ttyd service:
```bash
systemctl status chrote-ttyd
journalctl -u chrote-ttyd -f
```

### tmux sessions not visible between terminal and API

Ensure `TMUX_TMPDIR=/run/tmux/chrote` is set in both services and your shell:
```bash
echo $TMUX_TMPDIR  # Should be /run/tmux/chrote
```

### Tailscale not connecting

```bash
tailscale status
sudo tailscale up --hostname=chrote
```

## See Also

| Document | Description |
|----------|-------------|
| [PRD.md](PRD.md) | Product requirements - user needs and acceptance criteria |
| [SPEC.md](SPEC.md) | Technical specification - architecture, implementation, anti-patterns |
| [SECURITY.md](SECURITY.md) | Tailscale ACLs, threat model, secret protection |

## License

MIT
