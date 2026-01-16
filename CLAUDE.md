# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

AgentArena is a Docker-based development environment for managing AI coding agents via tmux sessions, with a React web dashboard for monitoring and control. The system runs behind Tailscale for secure remote access.

## Build & Run Commands

**Recommended:** Use the interactive control script `AgentArena-Toggle.ps1` which provides a menu for all common operations:
- Start/Stop/Restart
- Rebuild (with cache) - fast, for code changes
- Rebuild (no cache) - full rebuild, for dependency/apt changes
- View/follow logs, shell access, status, cleanup

```bash
# Or use docker compose directly:

# Start the full stack
docker compose up -d agent-arena

# Rebuild after code changes (uses cached layers, fast)
cd dashboard && npm run build && cd ..
docker compose build agent-arena
docker compose up -d --force-recreate agent-arena

# Full rebuild (no cache, use after dependency changes)
cd dashboard && npm run build && cd ..
docker compose build --no-cache agent-arena
docker compose up -d --force-recreate agent-arena

# View logs
docker compose logs -f agent-arena

# Dashboard development (localhost:5173)
cd dashboard && npm install && npm run dev

# Run tests
cd dashboard && npm test           # Playwright E2E (38 tests)
cd dashboard && npm run test:ui    # Playwright with UI
cd api && npm test                 # Jest unit tests (17 tests)
```

## Architecture

The system consists of Docker containers orchestrated via docker-compose.yml with Tailscale sidecars:

- **agent-arena** (build1.dockerfile): Ubuntu 24.04 container with nginx, ttyd, Express API, SSH
- **ollama**: Local LLM inference at `http://ollama:11434`
- **filebrowser**: Web file manager proxied at `/files/`
- **tailscale-arena/tailscale-ollama**: Network sidecars for secure access

### Request Flow
```
Browser → nginx (:8080)
         ├── /           → React dashboard (static files)
         ├── /terminal/  → ttyd (:7681) → terminal-launch.sh → tmux attach
         ├── /api/       → Express API (:3001) for session management
         └── /files/     → filebrowser (:8081)
```

### Key Environment Variable
`TMUX_TMPDIR=/tmp` must be consistent across API, ttyd, and SSH for tmux socket discovery.

## Gastown

Agent Arena powers **Gastown**, an orchestration framework for running many AI coding agents (10-30+) in parallel. Gastown uses the "MEOW Stack" (Molecular Expression Of Work) with Beads (atomic tasks), Epics, Molecules, and Wisps. Philosophy: "Physics over Politeness" - sessions are ephemeral, throughput is king, all actions are idempotent.

Session naming: `gt-{rigname}-{worker}` for rig workers, `hq-*` for coordination sessions.

## Dashboard (React + TypeScript)

**Tech Stack:** React 18, TypeScript, Vite, xterm.js, @dnd-kit (drag-drop)

**Entry Point:** `dashboard/src/App.tsx`

**State Management:** `SessionContext.tsx` manages:
- Window count (1-4 terminal panes)
- Window states with bound sessions
- Sidebar visibility, theme settings

**Session Categorization by prefix:**
- `hq-*` → HQ group (priority 0)
- `main`, `shell` → Main group (priority 1)
- `gt-{rigname}-*` → Gastown rig groups (priority 2)
- Other → Other group (priority 3)

**Views:** Terminal, Files (native React), Beads (visualization), Status, Settings

## API (Express.js)

**Entry Point:** `api/server.js`

**Key Endpoints:**
- `GET /api/tmux/sessions` - List sessions (polled every 5s)
- `POST /api/tmux/sessions` - Create session
- `PATCH /api/tmux/sessions/:name` - Rename session
- `DELETE /api/tmux/sessions/:name` - Delete specific session
- `DELETE /api/tmux/sessions/all` - Kill all sessions
- `GET /api/beads/*` - Beads viewer integration

## Session Panel Features

**Side Panel (260px width):**
- Sessions grouped by category with colored text when assigned to a window
- Search/filter sessions
- "+" button creates sessions with auto-incrementing names (tmux1, tmux2, etc.)
- "Nuke All" button to kill all sessions

**Right-Click Context Menu:**
- **Rename** - Inline rename with Enter to save, Escape to cancel
- **Assign to Window →** - Submenu to assign session to Window 1-4
- **Unassign** - Remove from window without deleting (only shows if assigned)
- **Delete Session** - Kill the tmux session

**Drag-and-Drop:**
- Drag sessions to terminal windows to assign them
- Drag session tags between windows to move assignments

## Anti-Patterns to Avoid

1. **No silent fallbacks** - Never add `else exec bash -l` or `2>/dev/null` on tmux commands; expose errors
2. **No empty catch blocks** - Always log errors even when degrading gracefully
3. **No auto-creating sessions** - Windows initialize empty; users drag sessions explicitly
4. **Click vs Drag behavior** - Click opens floating modal (peek), Drag binds to window

## Testing

Playwright tests in `dashboard/tests/` cover: session panel, terminal layouts, drag-and-drop, search, keyboard navigation, state persistence.

Jest tests in `api/utils.test.js` cover: session categorization, agent name extraction, group priority.

## Volume Mounts (Windows Host)

- `E:/Code` → `/code` (RW)
- `E:/Vault` → `/vault` (RO for dev, RW for root)
- `E:/LLM_models` → Ollama models

## Access URLs (via Tailscale)

- Dashboard: `http://arena:8080`
- SSH: `ssh dev@arena`
