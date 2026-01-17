# Arena Dashboard

React + TypeScript web UI for managing tmux sessions in the Arena container.

## Quick Start

```bash
npm install
npm run dev      # Dev server at http://localhost:5173
npm run build    # Build to dist/ (copied to container by dockerfile)
```

## Architecture

```
src/
├── App.tsx              # Main app with DnD context
├── context/
│   └── SessionContext   # Global state (sessions, workspaces, drag state)
├── components/
│   ├── TabBar           # Terminal | Terminal 2 | Files | Beads | Settings | Help tabs
│   ├── SessionPanel     # Left sidebar with session groups
│   ├── TerminalArea     # 1-4 terminal windows grid
│   ├── TerminalWindow   # Single terminal iframe + session tags
│   ├── FloatingModal    # Pop-out terminal (xterm.js + WebSocket)
│   ├── FilesView/       # Native file browser with error handling
│   │   ├── index.tsx    # Main component
│   │   ├── types.ts     # Types, error classes, path mapping
│   │   ├── fileService.ts # API layer (no silent fallbacks)
│   │   └── components/  # ErrorToast, etc.
│   └── SettingsView     # Theme and preferences
└── types.ts             # TypeScript interfaces
```

## Session Tracking

Sessions are actual tmux sessions running as `dev` user inside the container.

**How it works:**
1. API server (`/api/tmux/sessions`) lists tmux sessions
2. Dashboard polls API every 3 seconds
3. Drag session from sidebar → terminal window assigns it
4. Terminal iframe loads `/terminal/?arg=session-name`
5. ttyd receives the arg and attaches to that tmux session

**Key constraint:** Both ttyd and API must run as the same user (dev) to share the tmux socket.

## Drag and Drop

Uses `@dnd-kit/core` for drag-and-drop:

- **Drag from sidebar** → Drop on window to assign session
- **Drag session tag** → Move between windows or drop outside to remove
- **Click session tag** → Switch active session in that window

Note: The dashboard intentionally has no global keyboard shortcuts for cycling windows/sessions.

## Terminal Connection

Two modes:
- **Iframe** (`TerminalWindow`): Uses ttyd with URL args for session switching
- **WebSocket** (`FloatingModal`): Direct xterm.js connection, sends `tmux attach` command

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/tmux/sessions` | GET | List all tmux sessions |
| `/api/tmux/sessions` | POST | Create new session `{name?: string}` |
| `/api/health` | GET | Health check |

## Testing

```bash
npm run test          # Run Playwright tests
npm run test:headed   # Run with browser visible
npm run test:ui       # Interactive test UI
```

## Building for Container

The dockerfile copies `dist/` to `/usr/share/nginx/html`. After changes:

```bash
npm run build
# Then rebuild container
docker compose build agent-arena
docker compose up -d agent-arena
```
