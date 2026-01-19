# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CHROTE (**C**ontrol **H**ub for **R**emote **O**perations & **T**mux **E**xecution) is a WSL2-based environment for managing AI coding agents via tmux sessions, with a React web dashboard for monitoring and control. The system runs behind Tailscale for secure remote access.

**Primary Use Case:** Run [Gastown](https://github.com/steveyegge/gastown), an orchestration framework for 10-30+ Claude Code instances in parallel on a home server via Tailscale.

## Build & Run Commands

```bash
# Primary control script (Windows) - interactive menu
./Chrote-Toggle.ps1

# Service management (WSL)
systemctl status chrote-server chrote-ttyd     # Status
systemctl restart chrote-server                # Restart
journalctl -u chrote-server -f                 # Logs

# Dashboard development (localhost:5173)
cd dashboard && npm install && npm run dev

# Dashboard tests (Playwright E2E)
cd dashboard && npm test                       # Run all tests
cd dashboard && npm run test:ui                # Interactive test UI
cd dashboard && npx playwright test dashboard.spec.ts  # Single test file

# Go backend tests
cd src && go test ./...                        # All tests
cd src && go test ./internal/api/...           # API tests only
cd src && go test -v ./internal/api/ -run TestFilesHandler  # Single test

# Build and deploy
cd dashboard && npm run build && cd ..
cp -r dashboard/dist src/internal/dashboard/
cd src && go build -o ../chrote-server ./cmd/server
sudo systemctl restart chrote-server
```

## Architecture

**Go Single Binary Backend** (`src/`):

```
cmd/server/main.go          # Entry point, middleware, routes
internal/
  api/                      # HTTP handlers
    tmux.go                 # Session management, appearance
    files.go                # File browser API (security-critical)
    beads.go                # Beads issue tracking
    health.go               # Health checks
  core/                     # Shared utilities
    session.go              # Session parsing, categorization
    response.go             # JSON response helpers
    pathutil.go             # Path utilities
  proxy/
    terminal.go             # ttyd WebSocket proxy
  dashboard/
    embed.go                # Embedded React dashboard
```

**Key Patterns:**
- Handlers register routes via `RegisterRoutes(mux *http.ServeMux)`
- File API restricts access to `/code` and `/vault` only (`resolveSafePath()`)
- Session cache with 1s TTL to reduce tmux calls
- `core.WriteJSON()` and `core.WriteError()` for consistent responses

### React Dashboard (`dashboard/`)

```
src/
  App.tsx                   # Main component, tab routing
  context/SessionContext.tsx # Global state (windows, sessions, theme)
  components/
    TerminalWindow.tsx      # xterm.js terminal with ttyd WebSocket
    SessionPanel.tsx        # Left sidebar with session list
    FilesView/              # Native file browser (not filebrowser app)
    BeadsView/              # Kanban, Triage, Insights views
    MusicPlayer.tsx         # Ambient music in tab bar
```

**Views (Tab Bar):**
- Terminal 1 & 2 - Dual independent terminal workspaces
- Files - Native React file browser for /code and /vault
- Beads - Issue tracking with Kanban/Triage/Insights
- Settings - Theme, font size, tmux appearance
- Help

### Request Flow

```
Browser → Go Server (:8080)
         ├── /           → Embedded React dashboard
         ├── /terminal/  → ttyd proxy → terminal-launch.sh → tmux
         └── /api/       → Go handlers (tmux, files, beads)
```

## Session Categorization

Session naming determines dashboard grouping:
- `hq-*` → HQ group (priority 0)
- `main`, `shell` → Main group (priority 1)
- `gt-{rigname}-*` → Gastown rig groups (priority 3)
- Other → Other group (priority 4)

Implemented in `src/internal/core/session.go:CategorizeSession()`.

## Security Model

All operations run as **`chrote` user** (non-root, no sudo):
- Tmux sockets at `/run/tmux/chrote/` - dedicated socket directory
- User permissions provide isolation (not container boundary)
- Agents cannot escalate privileges

## Anti-Patterns

1. **No silent fallbacks** - Never `2>/dev/null` on tmux commands
2. **No auto-creating sessions** - Windows start empty; users drag sessions
3. **Click = peek (modal), Drag = bind** - Distinct behaviors
4. **No nginx** - Go server handles all routing (single binary)

## Filesystem

```
/code   → /home/chrote/chrote    (symlink, read/write)
/vault  → /mnt/e/Vault           (symlink, read-only)
```

File API only allows access to these paths.

## Access URLs (via Tailscale)

- Dashboard: `http://chrote:8080`
