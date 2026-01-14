# Arena Dashboard

A unified browser-based interface for managing the AgentArena development environment, combining intelligent tmux session management, terminal access, file management, and status monitoring in a single view.

## Overview

Arena Dashboard provides:

- **Smart tmux session manager** - Side panel with auto-discovered sessions, drag-and-drop to terminal windows
- **Configurable terminal grid** - 1-4 terminal windows with distinct color themes
- **Session binding** - Assign multiple tmux sessions to windows, cycle through them with arrow buttons
- **File browser integration** - Browse and edit files through the embedded filebrowser
- **Status panel** - Quick reference for Gastown commands and system status
- **External links** - Quick access to tools like Beads Viewer

Accessible at `http://arena:8080` from any device on your Tailscale network.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Tailscale Network                        │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              nginx (port 8080)                       │   │
│  │                                                      │   │
│  │   /           → React Dashboard (static files)      │   │
│  │   /api/       → Express API (tmux session discovery)│   │
│  │   /terminal/  → ttyd WebSocket proxy               │   │
│  │   /files/     → filebrowser proxy                  │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                  │
│            ┌─────────────┼─────────────┐                   │
│            ▼             ▼             ▼                   │
│     ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│     │  ttyd    │  │filebrowser│  │  arena   │              │
│     │ :7681    │  │  :8081   │  │container │              │
│     └──────────┘  └──────────┘  └──────────┘              │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Components

### Services

| Service | Port | Purpose |
|---------|------|---------|
| nginx | 8080 | Unified entry point, reverse proxy |
| Express API | 3001 | Tmux session discovery |
| ttyd | 7681 | Terminal over WebSocket |
| filebrowser | 8081 | Web-based file management |
| agent-arena | 22 | SSH access (alternative) |

### Dashboard Tabs

**Terminal** - Intelligent tmux session manager
- Collapsible side panel lists all tmux sessions grouped by type
- HQ sessions (`hq-mayor`, `hq-deacon`) always sorted to top
- Rig sessions grouped by rig name (`gt-gastown-*`, `gt-otherrig-*`)
- Drag sessions from panel to terminal windows
- 1-4 configurable terminal windows with color-coded themes:
  - Window 1: Blue
  - Window 2: Purple
  - Window 3: Green
  - Window 4: Orange
- Session tags show bound agents with × to remove
- Arrow buttons cycle through multiple sessions bound to a window
- Click unassigned session → opens floating modal terminal
- Click assigned session → focuses that window
- Layout and bindings persist to localStorage

**Files** - Embedded filebrowser
- Browse `/srv/code` (E:/Code) and `/srv/vault` (E:/Vault)
- Upload, download, edit files
- Uses the same Matrix-green hacker theme

**Status** - System overview
- Connection status
- Quick command reference for Gastown

**Beads ↗** - External link
- Opens [Beads Viewer](https://github.com/Dicklesworthstone/beads_viewer) in new tab
- Terminal-based issue tracker for Beads project management

## Session Naming Convention

The dashboard automatically categorizes sessions by prefix:

| Prefix | Group | Example | Description |
|--------|-------|---------|-------------|
| `hq-` | HQ | `hq-mayor`, `hq-deacon` | Headquarters roles (always on top) |
| `main` | Main | `main` | Primary terminal session |
| `gt-RIGNAME-` | Rig | `gt-gastown-jack` | Gastown agents on specific rigs |

## Authentication

| Component | Auth Method |
|-----------|-------------|
| Dashboard | None (Tailscale provides network security) |
| ttyd | None (Tailscale provides network security) |
| filebrowser | None (`--noauth` flag) |

All services are protected by Tailscale network access control.

## Configuration

### Environment Variables

In `.env`:

```env
TS_AUTHKEY=tskey-auth-xxx      # Tailscale auth key
```

### Customization

**Theme colors** are defined in `dashboard/src/styles/theme.css`:

```css
:root {
  --background: #000000;
  --surface-primary: #0a0a0a;
  --text-primary: #00ff41;      /* Matrix green */
  --accent: #00ff41;
}
```

**Window colors** are defined in `dashboard/src/types.ts`:

```typescript
export const WINDOW_COLORS = [
  { accent: '#4a9eff', bg: '#0a1628', border: '#1a3a5c' },  // Blue
  { accent: '#a855f7', bg: '#1a0a28', border: '#3a1a5c' },  // Purple
  { accent: '#22c55e', bg: '#0a1a10', border: '#1a3a2c' },  // Green
  { accent: '#f97316', bg: '#1a1008', border: '#3a2a1c' },  // Orange
]
```

## Usage

### Deploying

```bash
cd e:/Docker/AgentArena

# Build the dashboard (if not already built)
cd dashboard && npm install && npm run build && cd ..

# Build and start containers
docker compose build agent-arena
docker compose up -d
```

### Accessing

Open `http://arena:8080` from any Tailscale-connected device.

### Working with Sessions

**Drag and Drop:**
1. Find your session in the side panel (grouped by type)
2. Drag it to any terminal window
3. The terminal auto-attaches to that tmux session

**Multiple Sessions per Window:**
- Drag multiple sessions to the same window
- Use ← → arrows to cycle between them
- Click a session tag to switch directly to it

**Floating Modal:**
- Click any unassigned session in the panel
- Opens a floating terminal window you can drag around
- Click × or the overlay to close

**Removing Sessions:**
- Click × on any session tag to unbind it
- Drag a tag out of its window to remove it

### Quick Commands

From any terminal pane:

```bash
# List all tmux sessions
tmux ls

# Manually attach to a session
tmux attach -t gt-polecat-1

# Detach from session (stay in pane)
# Press: Ctrl-b d

# Monitor Gastown activity
gt peek

# View agent status
gt status

# Check convoy progress
gt convoy status
```

## Performance Considerations

| Concern | Mitigation |
|---------|------------|
| Browser memory | Limited to 4 windows (~20MB total) |
| xterm.js rendering | WebGL addon for GPU acceleration |
| WebSocket overhead | Single ttyd instance, multiple connections |
| tmux history | Gastown sessions are ephemeral (auto-cleanup) |
| Session polling | 5-second refresh interval for session list |

### Recommendations

- **4 windows is the practical limit** - xterm.js is main-thread bound
- Heavy output sessions may cause lag - attach to quieter sessions
- If performance degrades, reduce window count or close floating modals

## Troubleshooting

### Terminal won't connect

1. Check ttyd is running: `docker exec agentarena-dev ps aux | grep ttyd`
2. Check nginx inside container: `docker exec agentarena-dev nginx -t`
3. Check WebSocket path: Should be `/terminal/ws`

### Sessions not appearing in panel

1. Check API is running: `curl http://arena:8080/api/tmux/sessions`
2. Verify tmux server: `tmux ls` inside container
3. Check API logs in container

### Filebrowser not loading

1. Verify port change: filebrowser should be on 8081
2. Check nginx proxy path: `/files/`

### Connection issues

1. Verify Tailscale is connected: `tailscale status`
2. Check container is running: `docker ps | grep agentarena`
3. Test API directly: `curl http://arena:8080/api/health`

### Layout not persisting

1. Check localStorage: `localStorage.getItem('arena-dashboard-state')`
2. Clear and reload: `localStorage.removeItem('arena-dashboard-state')`

### Rebuilding the dashboard

```bash
cd e:/Docker/AgentArena/dashboard
npm run build
docker compose restart nginx
```

## File Structure

```
AgentArena/
├── .env                      # Tailscale + ttyd credentials
├── docker-compose.yml        # Service orchestration
├── build1.dockerfile         # Arena container (includes ttyd + API)
├── nginx/
│   └── nginx.conf           # Reverse proxy config
├── api/
│   ├── package.json
│   └── server.js            # Tmux session discovery API
├── dashboard/
│   ├── package.json
│   ├── vite.config.ts
│   ├── playwright.config.ts # E2E test config
│   ├── src/
│   │   ├── App.tsx          # Main app with DnD context
│   │   ├── types.ts         # TypeScript interfaces
│   │   ├── context/
│   │   │   └── SessionContext.tsx  # Global state management
│   │   ├── hooks/
│   │   │   └── useTmuxSessions.ts  # Session fetching hook
│   │   ├── components/
│   │   │   ├── TabBar.tsx          # Tab navigation
│   │   │   ├── SessionPanel.tsx    # Side panel with sessions
│   │   │   ├── SessionGroup.tsx    # Collapsible session group
│   │   │   ├── SessionItem.tsx     # Draggable session item
│   │   │   ├── TerminalArea.tsx    # Layout controls + grid
│   │   │   ├── TerminalWindow.tsx  # Terminal with session tags
│   │   │   ├── FloatingModal.tsx   # Popup terminal modal
│   │   │   ├── FilesView.tsx
│   │   │   └── StatusView.tsx
│   │   └── styles/
│   │       └── theme.css    # Hacker theme + window colors
│   ├── tests/
│   │   ├── dashboard.spec.ts  # Playwright E2E tests
│   │   └── mock-api.ts        # Mock session data for tests
│   └── dist/                # Built output (served by nginx)
└── filebrowser_data/
    └── branding/
        └── custom.css       # Filebrowser theme
```

## Tech Stack

| Layer | Technology |
|-------|------------|
| Frontend | React 18 + TypeScript |
| State Management | React Context + localStorage |
| Drag & Drop | @dnd-kit/core |
| Terminal | xterm.js + WebGL addon |
| Styling | Custom CSS (Matrix theme) |
| Build | Vite |
| Testing | Playwright |
| API | Express.js |
| Proxy | nginx |
| Terminal Backend | ttyd |
| File Management | filebrowser |
| Network | Tailscale |

## Completed Features

- [x] Session auto-discovery API (list tmux sessions)
- [x] Drag-and-drop session management
- [x] Layout persistence (localStorage)
- [x] Multiple sessions per window with cycling
- [x] Floating modal for quick session preview
- [x] Color-coded window themes
- [x] Collapsible sidebar
- [x] Session grouping (HQ, Main, Rigs)
- [x] E2E test coverage (30 tests)

## Future Enhancements

- [ ] Gastown status integration (parse `gt peek` output)
- [ ] Keyboard shortcuts for session switching
- [ ] Multiple ttyd instances for true pane isolation
- [ ] Session search/filter in panel
- [ ] Custom session labels/nicknames
