# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CHROTE (**C**ontrol **H**ub for **R**emote **O**perations & **T**mux **E**xecution) is a WSL2-based environment for managing AI coding agents via tmux sessions, with a React web dashboard for monitoring and control. The system runs behind Tailscale for secure remote access.

**GOLDEN RULE:** NEVER DISRUPT RUNNING SESSIONS

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

## Theming

The dashboard supports multiple themes (Matrix, Dark, Gastown) via CSS variables in `dashboard/src/styles/theme-colors.css`. When adding new UI elements, reuse existing component classes (like `.tab`) rather than creating custom styles - this ensures automatic theme support.

## Gastown Integration Constraints

Gastown sessions run Claude Code instances. You cannot inject commands via `tmux send-keys` into Gastown sessions (they're running Claude, not a shell).

## Anti-Patterns

1. **No silent fallbacks** - Never `2>/dev/null` on tmux commands
2. **No auto-creating sessions** - Windows start empty; users drag sessions
3. **Click = peek (modal), Drag = bind** - Distinct behaviors
4. **No nginx** - Go server handles all routing (single binary)
5. **Reuse existing styles** - Use existing component classes for theme consistency
6. **No injecting into Gastown sessions** - They run Claude Code, not interactive shells

## Filesystem

```
/code   → /home/chrote/chrote    (symlink, read/write)
/vault  → /mnt/e/Vault           (symlink, read-only)
```

File API only allows access to these paths.

## Dev Mode vs Production

**Production** - dashboard baked into binary:
```bash
systemctl start chrote-server
# Access at localhost:8080 or chrote:8080
```

**Development** - live reload while hacking:
```bash
cd dashboard
npm run dev   # Vite dev server on :5173
```

Change a file, browser updates instantly. When you're done, rebuild and restart:
```bash
npm run build
cp -r dist/* ../src/internal/dashboard/dist/
cd ../src && go build -o ../chrote-server ./cmd/server
sudo systemctl restart chrote-server
```

**CRITICAL for Claude:** When debugging issues, FIRST ask which mode the user is running:
- **localhost:5173** = Vite dev server. Routing is controlled by `dashboard/vite.config.ts` proxy settings. The Go binary's embedded assets are IRRELEVANT.
- **localhost:8080** = Production Go server. Dashboard is embedded in the binary.

If adding new backend routes (like `/bv-terminal/`), you MUST add them to BOTH:
1. Go backend (for production)
2. `vite.config.ts` proxy section (for dev mode)

Don't assume "restart the server" or "rebuild the binary" will fix dev mode issues - check the vite proxy config first.

## Access URLs (via Tailscale)

- Dashboard: `http://chrote:8080`
