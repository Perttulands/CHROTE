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

### 3. Start Services

```bash
docker compose up -d agent-arena
```

### 4. Access

Once running, access via Tailscale hostname:

| Service | URL | Description |
|---------|-----|-------------|
| Dashboard | `http://arena:8080` | Main web UI |
| Terminal | `http://arena:8080/terminal/` | Direct ttyd access |
| Files | `http://arena:8080/files/` | Filebrowser |
| SSH | `ssh dev@arena` | Password: `dev` |

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Tailscale Network                     │
│                   (Google Auth protected)                │
└─────────────────────────────┬───────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────┐
│                     nginx (:8080)                        │
│  ┌──────────┬──────────────┬─────────────┬───────────┐  │
│  │    /     │  /terminal/  │   /files/   │   /api/   │  │
│  │ Dashboard│    ttyd      │ filebrowser │  Node.js  │  │
│  └──────────┴──────────────┴─────────────┴───────────┘  │
└─────────────────────────────────────────────────────────┘
```

## Dashboard Features

- **Terminal View**: 1-4 terminal panes with drag-and-drop session assignment
- **Session Panel**: Lists all tmux sessions, drag to assign to windows
- **Files View**: Native file browser with full theme integration
- **Status View**: Service health and quick commands
- **Settings View**: Theme selection (Matrix/Dark/Gastown), font size, and preferences

### Session Management

1. Sessions appear in the left sidebar grouped by type (HQ, Main, Rigs)
2. Drag a session onto a terminal window to attach
3. Click session tags to switch between assigned sessions
4. Use ← → buttons to cycle through sessions in a window
5. Ctrl+Arrow keys for keyboard navigation (Up/Down for windows, Left/Right for sessions)

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
├── api/                  # Node.js API for tmux management
├── dashboard/            # React + TypeScript web UI
│   ├── src/
│   ├── tests/            # Playwright tests
│   └── dist/             # Built assets (copied to container)
├── nginx/                # nginx config
├── sandbox_overrides/    # Empty files overlaid on secrets in sandbox
├── build1.dockerfile     # Main container definition
├── docker-compose.yml    # Service orchestration
└── .env                  # Secrets (not in git, hidden from sandbox)
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

## License

MIT
