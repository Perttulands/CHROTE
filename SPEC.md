# AgentArena Technical Specification

## System Overview

AgentArena is a Docker-based development environment for managing AI coding agents via tmux sessions, with a web dashboard for monitoring and control. The system runs on "Landmass" (home server) with a Tailscale sidecar container for secure remote access.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Tailscale Network                               │
│                        (Google Auth protected)                           │
└───────────────────────────────────────────────────────────────────────┬──┘
                                                                        │
                                                        ┌───────────────▼───────────┐
                                                        │   tailscale-arena         │
                                                        │   hostname: arena         │
                                                        └───────────────┬───────────┘
                                                                        │ network_mode
                                        ┌───────────────────────────────▼───────────────────────────────┐
                                        │          agent-arena (:8080)                                  │
                                        │  ┌─────────────────────────────────┐                          │
                                        │  │     nginx (reverse proxy)       │                          │
                                        │  │  /           → React Dashboard  │                          │
                                        │  │  /terminal/  → ttyd (:7681)     │                          │
                                        │  │  /api/       → Node.js (:3001)  │                          │
                                        │  │  /api/files/ → File API         │                          │
                                        │  │  /api/beads/ → Beads API        │                          │
                                        │  └─────────────────────────────────┘                          │
                                        │  ┌─────────────────────────────────┐                          │
                                        │  │  ttyd (:7681) → tmux sessions   │                          │
                                        │  └─────────────────────────────────┘                          │
                                        └───────────────────────────────────────────────────────────────┘
         │
   Volumes:
   E:/Code       → /code (RW)
   E:/Vault      → /vault (RO dev, RW root)
```

## Containers

### 1. agent-arena (agentarena-dev)

**Purpose:** Development environment for AI coding agents

**Image:** Custom Ubuntu 24.04 (`build1.dockerfile`)

**Installed Tools:**
- Claude Code (@anthropic-ai/claude-code)
- Gastown (`gt`) & Beads (`bd`) - orchestrator tools
- beads_viewer (`bv`) - TUI and robot-protocol CLI for project dependency visualization
  - `bv` - Interactive terminal UI
  - `bv --robot-triage` - JSON triage recommendations for dashboard
  - `bv --robot-insights` - JSON graph metrics for dashboard
  - `bv --robot-plan` - JSON execution plan for dashboard
- Git, tmux, Go, Node.js, Python3

**Internal Services:**
| Service | Port | Purpose |
|---------|------|---------|
| nginx | 8080 | Unified entry point, reverse proxy |
| Express API | 3001 | Tmux session management, file API |
| ttyd | 7681 | Terminal over WebSocket |
| SSH | 22 | Direct shell access |

**Exposed Ports (via Tailscale):**
| Port | Purpose |
|------|---------|
| 22 | SSH access |
| 8080 | Web dashboard |
| 3000, 5000, 8000 | Dev servers |
| 5500, 6000, 9000-9900 | Additional dev/monitoring |

**Users:**
| User | Password | Permissions |
|------|----------|-------------|
| root | root | Full access, can write to /vault |
| dev | dev | Standard user, read-only /vault, sudo access |

## Volume Layout

| Host Path | Container | Mount Path | Permissions | Purpose |
|-----------|-----------|------------|-------------|---------|
| E:/Code | agent-arena | /code | RW (dev & root) | Active coding projects |
| E:/Vault | agent-arena | /vault | RO (dev), RW (root) | Safe context files for agents |
| Named: arena_dev_home | agent-arena | /home/dev | RW | Persist dev config |
| Named: arena_root_home | agent-arena | /root | RW | Persist root config |

### /vault Security Model
- Drop files as root (SSH as root, or from Windows at E:/Vault)
- Agents (running as dev) can only read - prevents accidental corruption
- Permissions enforced at container startup via entrypoint script

---

## Dashboard Architecture

### Request Flow
```
UI (React) → nginx (/terminal/) → ttyd (port 7681) → terminal-launch.sh → tmux attach
```

### Tech Stack

| Layer | Technology |
|-------|------------|
| Frontend | React 18 + TypeScript |
| State Management | React Context + localStorage |
| Drag & Drop | @dnd-kit/core |
| Terminal | xterm.js + WebGL addon |
| Styling | CSS Variables (theming) |
| Build | Vite |
| Testing | Playwright (E2E), Jest (API) |
| API | Express.js |
| Proxy | nginx |
| Terminal Backend | ttyd |

### Component Structure

```
dashboard/src/
├── App.tsx                 # Main app with DnD context
├── types.ts                # TypeScript interfaces
├── context/
│   └── SessionContext.tsx  # Global state management
├── hooks/
│   └── useTmuxSessions.ts  # Session fetching hook
├── components/
│   ├── TabBar.tsx          # Tab navigation + music player
│   ├── SessionPanel.tsx    # Side panel with sessions + nuke button
│   ├── SessionGroup.tsx    # Collapsible session group
│   ├── SessionItem.tsx     # Draggable session item
│   ├── TerminalArea.tsx    # Layout controls + grid
│   ├── TerminalWindow.tsx  # Terminal with session tags
│   ├── FloatingModal.tsx   # Popup terminal modal
│   ├── FilesView.tsx       # Native file browser
│   ├── StatusView.tsx      # Service health display
│   ├── NukeConfirmModal.tsx# Destroy all sessions modal
│   └── MusicPlayer.tsx     # Ambient music controls
├── beads_module/           # Self-contained Beads integration
└── styles/
    └── theme.css           # Theme definitions
```

### State Management

**SessionContext** provides global state:
- `windowCount` - Number of terminal windows (1-4)
- `windows[]` - Array of window state objects
- `focusedWindowIndex` - Currently focused window
- `sidebarCollapsed` - Sidebar visibility
- `settings` - User preferences

**Window State Object:**
```typescript
{
  id: "window-0",
  boundSessions: ["hq-mayor", "gt-rig1-jack"],
  activeSession: "hq-mayor",
  colorIndex: 0
}
```

**localStorage Key:** `arena-dashboard-state`

### Session Categorization

Sessions are categorized by prefix for grouping:

| Prefix | Group | Priority | Example |
|--------|-------|----------|---------|
| `hq-*` | HQ | 0 (first) | hq-mayor |
| `main`, `shell` | Main | 1 | main |
| `gt-{rigname}-*` | gt-{rigname} | 2 | gt-gastown-jack |
| Other | Other | 3 | my-session |

### tmux Configuration

The container includes a minimal `~/.tmux.conf`:

```bash
# UTF-8 support for emojis and special characters
set -gq utf8 on
set -gq status-utf8 on
setw -gq utf8 on

# Start windows and panes at 1, not 0
set -g base-index 1
setw -g pane-base-index 1

# True color support
set -g default-terminal "tmux-256color"
set -ga terminal-overrides ",xterm-256color:Tc"

# Transparent background - inherit from terminal (enables CSS theming)
set -g window-style "bg=default"
set -g window-active-style "bg=default"
```

### Terminal Connection

**How ttyd connects to tmux:**
1. ttyd runs with URL argument support: `ttyd -a /usr/local/bin/terminal-launch.sh`
2. Dashboard requests: `/terminal/?arg={session_name}&theme={"background":"transparent"}`
3. `terminal-launch.sh` receives session name, runs `tmux attach-session -t {name}`
4. User sees tmux session in iframe
5. The transparent background in both tmux and xterm allows CSS theme colors to show through

**Critical Environment Variable:**
- `TMUX_TMPDIR=/tmp` must be set consistently across ALL processes (API, ttyd, SSH)

### Terminal Resize Handling

Both `TerminalWindow.tsx` and `FloatingModal.tsx` use ResizeObserver:
1. `triggerFit()` dispatches resize event to iframe - ttyd calls xterm's `fit()`
2. On iframe load, `triggerFit()` called at 100ms, 300ms, 500ms delays
3. ResizeObserver watches container, triggers fit with 100ms debounce

---

## API Endpoints

### Session Management

**GET /api/tmux/sessions**
```json
{
  "sessions": [
    {"name": "hq-mayor", "windows": 1, "attached": false, "group": "hq"},
    {"name": "gt-rig1-jack", "windows": 1, "attached": true, "group": "gt-rig1"}
  ],
  "grouped": {
    "hq": [...],
    "gt-rig1": [...],
    "other": [...]
  },
  "timestamp": "2026-01-14T..."
}
```

**POST /api/tmux/sessions**
```json
// Request
{"name": "shell-xyz"}

// Response
{"success": true, "session": "shell-xyz", "timestamp": "..."}
```

**DELETE /api/tmux/sessions/all**
```json
// Response
{"success": true, "killed": 5, "sessions": ["hq-mayor", "..."]}
```

**Polling:** Dashboard polls GET every 5 seconds.

### Beads Integration

| Endpoint | Purpose |
|----------|---------|
| GET /api/beads/issues | Raw issue data from .beads/issues.jsonl |
| GET /api/beads/triage | AI triage recommendations |
| GET /api/beads/insights | Graph metrics |
| GET /api/beads/plan | Parallel execution tracks |

---

## Theme System

**Available Themes:**
| Theme | Description |
|-------|-------------|
| Matrix | Green neon on black (default) |
| Dark | Blue accent on neutral dark |
| Gastown | Warm cream/russet palette |

**Implementation:**
- `data-theme` attribute on document root
- CSS custom properties define all colors
- Components use `var(--property-name)` exclusively
- Theme selection persists in localStorage

**Window Colors:**
| Window | Color | Hex |
|--------|-------|-----|
| 1 | Blue | #4a9eff |
| 2 | Purple | #9966ff |
| 3 | Green | #00ff41 |
| 4 | Orange | #ff9933 |

---

## File Browser Implementation

**Architecture:**
- Native React component (not iframe)
- Uses file API backend (`/api/files/resources/...`)
- Full CSS variable theming

**Features:**
- Navigation: Breadcrumbs, Back/Forward/Up, history
- Views: List (sortable) and Grid (thumbnails)
- Operations: Upload, Download, Rename, Delete, Create Folder
- Selection: Single, Ctrl+multi, Shift+range
- Search: Filter files in current directory
- Context Menu: Right-click actions

**Keyboard Shortcuts:**
| Key | Action |
|-----|--------|
| Enter | Open folder / Download file |
| Backspace | Parent directory |
| F2 | Rename |
| Delete | Delete |
| F5 | Refresh |
| Ctrl+A | Select all |

---

## Inbox / Package System

**Purpose:** Send files with instructions to `/code/incoming/` for agent processing.

**File Storage Pattern:**
- User uploads `report.pdf` with message "Please analyze this"
- Creates: `/code/incoming/report.pdf` (file)
- Creates: `/code/incoming/report.pdf.letter` (message text)

---

## Security Model

### Network Isolation

Containers are isolated via Tailscale ACLs:

```json
{
  "tagOwners": { "tag:container": ["autogroup:admin"] },
  "grants": [
    {"src": ["autogroup:member"], "dst": ["*"], "ip": ["*"]},
    {"src": ["tag:container"], "dst": ["tag:container"], "ip": ["*"]},
    {"src": ["tag:container"], "dst": ["autogroup:internet"], "ip": ["*"]}
  ]
}
```

| Source | Can Reach | Cannot Reach |
|--------|-----------|--------------|
| Your devices | arena, all devices | - |
| arena | internet | NAS, host, other devices |

### Sensitive File Protection

Volume overlays hide secrets from sandbox:
```yaml
volumes:
  - ./sandbox_overrides/empty_env:/code/AgentArena/.env:ro
```

---

## File Structure

```
AgentArena/
├── api/                      # Node.js API
│   ├── server.js             # Main API server
│   ├── beads-routes.js       # Beads endpoints
│   ├── file-routes.js        # File API endpoints
│   └── utils.test.js         # Unit tests
├── dashboard/                # React UI
│   ├── src/                  # Source code
│   ├── tests/                # Playwright E2E tests
│   └── dist/                 # Built assets
├── nginx/                    # nginx config
├── internal/                 # tmux helpers
├── sandbox_overrides/        # Secret overlays
├── beads_viewer_integration/ # Integration docs
├── tailscale_state/          # Tailscale identity
├── build1.dockerfile         # Main container
├── docker-compose.yml        # Orchestration
├── .env                      # Secrets (git-ignored)
├── PRD.md                    # Product requirements
└── SPEC.md                   # This file
```

---

## Development Anti-Patterns

### Avoid These

1. **Silent Fallbacks in Scripts**
   - Never add `else exec bash -l` as silent fallback
   - Never use `2>/dev/null` on tmux commands
   - Always expose errors for debugging

2. **Empty Catch Blocks**
   - Never use `.catch(() => {})` - always log errors
   - Even if degrading gracefully, log why

3. **Auto-Creating Sessions**
   - Windows should initialize empty (`activeSession: null`)
   - Let users drag sessions explicitly

4. **Timeout-Based Font Sizing**
   - Use polling approach, not fixed setTimeout delays

5. **Session Click vs Drag Confusion**
   - Click → Opens floating modal (peek)
   - Drag → Binds to window (does NOT auto-activate)

---

## Commands

```bash
# Build and start
docker compose build --no-cache
docker compose up -d

# View logs
docker compose logs -f

# SSH access
ssh dev@arena      # as dev
ssh root@arena     # as root

# Test API
curl http://arena:8080/api/tmux/sessions

# Rebuild dashboard
cd dashboard && npm run build && cd ..
docker compose restart agent-arena
```

---

## Tests

### Backend (Jest)
```bash
cd api && npm test
```
13 tests: session categorization, group priority, sorting

### Frontend (Playwright)
```bash
cd dashboard && npm test
```
38 tests: session panel, terminal layouts, drag-and-drop, search, keyboard navigation, persistence
