# CHROTE Dashboard

React + TypeScript cockpit embedded in the CHROTE Go server.

## Local development

From `dashboard/`:

```bash
npm ci
npm run dev
npm run lint
npm run test:unit
npm test -- --project=chromium
npm run build
```

The Vite development server defaults to `http://127.0.0.1:5173` and proxies API
requests to the configured CHROTE backend. See `vite.config.ts` and
`playwright.config.ts` rather than hard-coding a private deployment port in
contributor docs.

## Shipped navigation

The application shell currently exposes:

- Terminal 1
- Terminal 2
- Terminal 3
- Files
- Beads
- Library
- Scheduled
- Server (default-enabled feature flag)
- Settings

Help is an application-shell dialog, not a persistent tab.

## Architecture map

```text
src/
├── App.tsx                         application shell and keep-alive view ownership
├── context/
│   ├── SessionContext.tsx          session actions and composed state surface
│   ├── useSessionsPoll.ts          session discovery and stale-state handling
│   ├── useWorkspaceLayouts.ts      layouts, settings, and persistence
│   ├── useSendToSession.ts         guarded send workflow client
│   └── ToastContext.tsx            operator notifications
├── components/
│   ├── TabBar.tsx                  top-level navigation
│   ├── TerminalArea.tsx            1-4 terminal-window workspace
│   ├── TerminalWorkspaceDock.tsx   unified Sessions/Files sidecar owner
│   ├── SessionPanel.tsx            grouped sessions and Peek
│   ├── TerminalWindow.tsx          assignment, location, and tile states
│   ├── TerminalPool.tsx            one terminal per bound session
│   ├── TerminalSurface.tsx         the only place a terminal goes on screen
│   ├── FilesView/                  full Files workspace
│   ├── TerminalFilesPanel.tsx      terminal-companion Files sidecar
│   ├── BeadsView/                  Beads workspace and issue surfaces
│   ├── LibraryView/                the context corpus as a reading room
│   ├── ScheduledTasksView.tsx      schedules and run history
│   ├── SystemStatusView/           health, resources, and system history
│   └── SettingsView.tsx            appearance, flags, and session cleanup
├── terminal/                       xterm session, tile state, and wire protocol
├── hooks/                          keyboard, drag, polling, and layout behavior
├── utils/                          shared parsing and UI utilities
└── types.ts                        dashboard contracts and persisted settings
```

## Terminal workspace contract

Each of the three terminal tabs owns an independent workspace with one to four
terminal windows.

Sessions and Files are peer views of one sidecar surface with different state
ownership:

- closed by default;
- zero permanent width while closed;
- pinnable on wide layouts;
- overlayed at `768px` and below;
- application-global Sessions presentation and width;
- workspace-local Files presentation and width.

A session-row click means **Peek**. It does not detach, reassign, or alter the
window assignment. Attached-window navigation uses the explicit location chip.
The `/` shortcut opens the global Sessions surface in the active terminal
workspace and focuses its search when no visible dialog or menu owns the key.

Do not duplicate dock ownership inside `TerminalWindow` or build a second Files
sidebar. `TerminalWorkspaceDock` is the layout owner.

## State and lifecycle rules

- A terminal must survive tab switches and unrelated React renders. Its element
  and WebSocket are owned outside React, and a tile that has been shown keeps
  both while it is off screen.
- A window binding is operator intent. Nothing but the operator removes one, and
  a tile whose session ended keeps its last frame.
- Browser state stores presentation and assignment metadata, not durable process
  state.
- tmux sessions and host files remain authoritative.
- Hidden keep-alive views must not steal keyboard events from visible dialogs.
- Async views need explicit loading, empty, degraded, error, and stale states.
- Destructive operations require visible operator intent and fail-loud feedback.

See [`../docs/adr/0017-terminal-viewing-model.md`](../docs/adr/0017-terminal-viewing-model.md)
for the viewing model,
[`../docs/adr/0018-terminal-transport-ownership.md`](../docs/adr/0018-terminal-transport-ownership.md)
for the transport, and [`../DESIGN-SYSTEM.md`](../DESIGN-SYSTEM.md).

## Files

The full Files view and terminal Files sidecar share file-service contracts but
serve different jobs:

- **Files view:** browse, edit, compare, pin, and manage configured workspace
  files.
- **Files sidecar:** compact terminal companion for selecting and sending useful
  context without turning the terminal layout into an IDE.

Filesystem errors must remain visible. Never fall back silently to fake data or
an unconstrained root.

## Testing

```bash
# Static and component checks
npm run lint
npm run test:unit
npm run test:unit -- --coverage

# Deterministic mocked browser suite
npm test

# Interactive debugging
npm run test:headed
npm run test:ui
```

Live backend/terminal integration is intentionally separate from the default
mocked browser gate:

```bash
CHROTE_TEST_URL=http://127.0.0.1:8094 npm run test:live
```

Run live tests only against an approved disposable or operator-controlled CHROTE
instance. They are not a substitute for the deterministic suite.

`npm audit --audit-level=moderate` runs in the weekly CI dependency scan.

## Production embedding

From the repository root:

```bash
./scripts/build-embedded-dashboard.sh
```

That script builds the dashboard and replaces
`src/internal/dashboard/dist` with the exact generated output. Verify parity
before building a release binary:

```bash
diff -qr dashboard/dist src/internal/dashboard/dist
```

Do not hand-maintain embedded asset filenames.
