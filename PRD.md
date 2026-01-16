# AgentArena — Product Requirements Document (PRD)

Last updated: 2026-01-16

## 1) Summary (What / Why)

**AgentArena** is a Docker-based “agent workbench” for running and supervising many AI coding agents that live in **tmux sessions** (often created by Gastown). It provides a single, Tailnet-accessible web dashboard to:

- **Watch and interact with many tmux sessions at once** (1–4 panes, tabbed sessions per pane).
- **Peek into any session without disrupting your layout** (floating modal).
- **Move sessions between panes quickly** (drag-and-drop).
- **Manage files in the mounted workspace** (browse, upload, download, rename, delete, create folders).
- **Send “packages” to agents** via an incoming folder workflow.
- Optionally visualize Beads projects (issues/dependencies) via a self-contained Beads module.

The product exists to solve a very specific pain: once an orchestrator spawns 10–30+ agent sessions, native tmux alone becomes hard to monitor remotely and hard to “sample” without constantly detaching/attaching. AgentArena makes the tmux swarm legible, navigable, and safe to access from any device.

## 2) Goals

1. **High-signal supervision** of many concurrent agent tmux sessions.
2. **Non-disruptive interaction**: rebind/peek/switch sessions without killing or interrupting agents.
3. **Remote access by default**: usable from phone/tablet/laptop over Tailscale without exposing ports publicly.
4. **Practical file ops** for the mounted workspace and a dead-simple “send to agents” path.
5. **Low operational overhead**: start via Docker Compose, minimal moving parts.

## 3) Non-goals

- Not a tmux replacement (no pane/window management UI; sessions are the unit).
- Not a full process supervisor (agents may fail; the UI is primarily for observation and manual control).
- Not a public multi-tenant SaaS (assume a single trusted Tailnet; optional token auth exists for hardening).
- Not an IDE (no in-browser editor requirements; file operations only).

## 4) Target users

- A developer/operator running agentic coding workflows (often Gastown) who needs to supervise many agent sessions across multiple projects.
- A developer who wants the same view from any device (e.g., kitchen tablet monitoring builds).

## 5) Product surface area (What exists today)

### 5.1 Docker stack

AgentArena runs as multiple containers orchestrated by Docker Compose:

- **agent-arena**: nginx reverse proxy, ttyd web terminal, Node API, SSH server
- **filebrowser**: filebrowser backend (also used by the native Files UI via its API)
- **ollama**: local inference API (optional, for agent use)
- **tailscale sidecars**: provide Tailnet access for arena + ollama

### 5.2 Web dashboard

Tabs:

- **Terminal**: multi-pane tmux session viewer + session sidebar
- **Files**: native file browser UI + Inbox/package workflow
- **Beads**: optional project visualization (graph/kanban/triage/insights)
- **Settings**: theme, terminal font size, polling interval, session prefix, tmux appearance
- **Help**: lightweight guidance

Persistent UI state is stored in browser localStorage.

### 5.3 API

Node/Express API behind nginx under `/api/`:

- tmux session listing + grouping
- create/rename/delete sessions
- nuke all sessions
- apply tmux appearance settings (hot-reload)
- optional Beads endpoints (`/api/beads/*`)

## 6) Key workflows (How users use it)

### 6.1 Start the environment

1. Start Docker Compose services.
2. Join the same Tailnet.
3. Open the dashboard at `http://arena:8080/`.

### 6.2 Run agents (Gastown or manual)

1. Run Gastown inside the container (or SSH in) to spawn tmux sessions.
2. AgentArena automatically discovers tmux sessions via polling.

### 6.3 Monitor and interact

1. Use the sidebar to find sessions (grouped + filterable).
2. Drag sessions onto one of the terminal panes.
3. Use tabs within each pane to switch active session.
4. Use floating modal to “peek” at any session without changing pane bindings.

### 6.4 Send files to agents (Inbox)

1. Drop files into the Inbox panel.
2. Add an optional note.
3. The UI writes files into `/code/incoming` and writes a sidecar note file alongside (current implementation: `filename.note`).

## 7) Functional requirements

### 7.1 Terminal supervision

**Multi-pane layout**
- Support 1–4 panes in a responsive grid.
- Each pane can have 0..N bound tmux sessions.

**Session discovery + grouping**
- The system polls tmux sessions and groups them by naming convention (HQ / rig / main / other).
- A text filter narrows sessions by name/agent.

**Binding model**
- A session can be bound to at most one pane at a time.
- Dragging a session to a pane binds it.
- Dragging a session tag between panes moves the binding.
- Removing a tab unbinds the session from the pane.

**Active session behavior**
- If a pane is empty and a session is bound, that session becomes active.
- If a pane already has sessions, binding an additional session must not switch the active session.

**Peek modal**
- Clicking a session in the sidebar opens a floating modal showing that session.
- Peek does not change pane bindings.

**Keyboard navigation**
- `Ctrl+Up/Down`: cycle focused pane
- `Ctrl+Left/Right`: cycle active session in focused pane

**Session lifecycle operations**
- Create a new tmux session from the UI (using a configurable prefix).
- Rename and delete individual sessions.
- “Nuke all” destroys all tmux sessions (with confirmation UI).

### 7.2 Settings + persistence

**Persisted state**
- Pane count
- Pane bindings and active session per pane
- Sidebar collapsed state
- Theme and font size
- Auto-refresh/polling interval
- Default session name prefix
- tmux appearance settings

**Theme system**
- Themes: matrix / dark / gastown.
- Use CSS variables so the whole dashboard re-themes consistently.

**tmux appearance**
- Provide presets (matrix/dark/gastown) and custom color controls.
- Apply changes via API so existing tmux sessions update without restarting.

### 7.3 Files

**Native file browser**
- Browse mounted volumes exposed through filebrowser.
- List and grid view.
- Sort by name/size/modified.
- Filter within the current directory.
- Upload (button and drag/drop).
- Download (double click / context menu).
- Rename, delete, create folder.
- Multi-select (Ctrl/Shift) and keyboard shortcuts.

**Inbox / package workflow**
- Provide a dedicated “Inbox” drop area.
- Target path: `/code/incoming` (host mapped to `E:/Code/incoming`).
- When a note is provided, write a sidecar note file next to the first file (current: `originalFilename.note`).

**External filebrowser access**
- Provide a control to open the filebrowser UI in a new tab at `/files/`.

### 7.4 Beads (optional module)

Beads integration is a removable module.

**Project selection**
- Allow selecting a project path under allowed roots.

**Data sources**
- Read `.beads/issues.jsonl` from the selected project.

**Robot protocol (best-effort)**
- If `bv` is installed, call robot commands for triage/insights/plan.
- If not installed, provide graceful fallback responses.

## 8) Non-functional requirements

### 8.1 Remote access + security posture

- Primary access is over Tailscale (no public internet exposure by default).
- Containers are intended to be network-isolated via Tailnet ACLs.
- `/vault` should be read-only for the `dev` user to protect reference materials.

### 8.2 Safety + failure modes

- If tmux is not running, the dashboard should show “no sessions” rather than error-spam.
- Polling should be inexpensive (API caching is acceptable).
- File operations must surface errors clearly (no silent fallbacks).

### 8.3 Performance

- Terminal panes should remain responsive with 20+ sessions present.
- UI should not re-render excessively on polling.

## 9) How to get there (Roadmap)

This section describes the intended path from “working tool” → “reliable daily driver”.

### Milestone A — PRD/Docs alignment (now)
- Ensure the PRD describes the real behavior and naming (tabs, Inbox note extension, tmux appearance settings).
- Keep README/SPEC consistent with PRD (follow-up work).

### Milestone B — Reliability hardening
- Add clear health indicators for: API reachable, ttyd reachable, filebrowser reachable.
- Make polling interval and error retry behavior explicit and testable.
- Confirm persistence behaviors across refreshes and container restarts.

### Milestone C — Security hardening (optional, Tailnet-dependent)
- Document and optionally enable API bearer token auth (`API_AUTH_TOKEN`).
- Document restricted CORS allowlist (`CORS_ORIGINS`) when used outside strict Tailnet assumptions.
- Re-validate file path restrictions for Beads project selection.

### Milestone D — Workflow polish
- Decide on the canonical Inbox sidecar format (keep `.note` or switch to `.letter`) and standardize across docs + any downstream agent tooling.
- Decide whether the dashboard needs a dedicated “Status” tab or whether Help/Settings cover it.

## 10) Acceptance criteria (minimum)

### Terminal
- View 1–4 panes simultaneously.
- Drag sessions from sidebar into panes.
- Move sessions between panes via drag.
- Switch sessions within a pane via tabs and via `Ctrl+Left/Right`.
- Peek a session via modal without changing bindings.
- Create, rename, and delete sessions.
- Nuke all sessions with confirmation.
- Layout + bindings + settings persist across refresh.

### Files
- Load directory contents and navigate via breadcrumbs + back/forward/up.
- Upload/download/rename/delete/create folder.
- Multi-select and shortcuts (F2/Del/F5/Ctrl+A).
- Inbox writes files to `/code/incoming` and writes a note sidecar.

### Beads (when enabled)
- Can load issues from `.beads/issues.jsonl` under an allowed root.
- Triage/insights/plan endpoints return usable data (robot or fallback).
# AgentArena — Product Requirements Document (PRD)

Last updated: 2026-01-16

## 1) Summary (What / Why)

**AgentArena** is a Docker-based “agent workbench” for running and supervising many AI coding agents that live in **tmux sessions** (often created by Gastown). It provides a single, Tailnet-accessible web dashboard to:

- **Watch and interact with many tmux sessions at once** (1–4 panes, tabbed sessions per pane).
- **Peek into any session without disrupting your layout** (floating modal).
- **Move sessions between panes quickly** (drag-and-drop).
- **Manage files in the mounted workspace** (browse, upload, download, rename, delete, create folders).
- **Send “packages” to agents** via an incoming folder workflow.
- Optionally visualize Beads projects (issues/dependencies) via a self-contained Beads module.

The product exists to solve a very specific pain: once an orchestrator spawns 10–30+ agent sessions, native tmux alone becomes hard to monitor remotely and hard to “sample” without constantly detaching/attaching. AgentArena makes the tmux swarm legible, navigable, and safe to access from any device.

## 2) Goals

1. **High-signal supervision** of many concurrent agent tmux sessions.
2. **Non-disruptive interaction**: rebind/peek/switch sessions without killing or interrupting agents.
3. **Remote access by default**: usable from phone/tablet/laptop over Tailscale without exposing ports publicly.
4. **Practical file ops** for the mounted workspace and a dead-simple “send to agents” path.
5. **Low operational overhead**: start via Docker Compose, minimal moving parts.

## 3) Non-goals

- Not a tmux replacement (no pane/window management UI; sessions are the unit).
- Not a full process supervisor (agents may fail; the UI is primarily for observation and manual control).
- Not a public multi-tenant SaaS (assume a single trusted Tailnet; optional token auth exists for hardening).
- Not an IDE (no in-browser editor requirements; file operations only).

## 4) Target users

- A developer/operator running agentic coding workflows (often Gastown) who needs to supervise many agent sessions across multiple projects.
- A developer who wants the same view from any device (e.g., kitchen tablet monitoring builds).

## 5) Product surface area (What exists today)

### 5.1 Docker stack

AgentArena runs as multiple containers orchestrated by Docker Compose:

- **agent-arena**: nginx reverse proxy, ttyd web terminal, Node API, SSH server
- **filebrowser**: filebrowser backend (also used by the native Files UI via its API)
- **ollama**: local inference API (optional, for agent use)
- **tailscale sidecars**: provide Tailnet access for arena + ollama

### 5.2 Web dashboard

Tabs:

- **Terminal**: multi-pane tmux session viewer + session sidebar
- **Files**: native file browser UI + Inbox/package workflow
- **Beads**: optional project visualization (graph/kanban/triage/insights)
- **Settings**: theme, terminal font size, polling interval, session prefix, tmux appearance
- **Help**: lightweight guidance

Persistent UI state is stored in browser localStorage.

### 5.3 API

Node/Express API behind nginx under `/api/`:

- tmux session listing + grouping
- create/rename/delete sessions
- nuke all sessions
- apply tmux appearance settings (hot-reload)
- optional Beads endpoints (`/api/beads/*`)

## 6) Key workflows (How users use it)

### 6.1 Start the environment

1. Start Docker Compose services.
2. Join the same Tailnet.
3. Open the dashboard at `http://arena:8080/`.

### 6.2 Run agents (Gastown or manual)

1. Run Gastown inside the container (or SSH in) to spawn tmux sessions.
2. AgentArena automatically discovers tmux sessions via polling.

### 6.3 Monitor and interact

1. Use the sidebar to find sessions (grouped + filterable).
2. Drag sessions onto one of the terminal panes.
3. Use tabs within each pane to switch active session.
4. Use floating modal to “peek” at any session without changing pane bindings.

### 6.4 Send files to agents (Inbox)

1. Drop files into the Inbox panel.
2. Add an optional note.
3. The UI writes files into `/code/incoming` and writes a sidecar note file alongside (current implementation: `filename.note`).

## 7) Functional requirements

### 7.1 Terminal supervision

**Multi-pane layout**
- Support 1–4 panes in a responsive grid.
- Each pane can have 0..N bound tmux sessions.

**Session discovery + grouping**
- The system polls tmux sessions and groups them by naming convention (HQ / rig / main / other).
- A text filter narrows sessions by name/agent.

**Binding model**
- A session can be bound to at most one pane at a time.
- Dragging a session to a pane binds it.
- Dragging a session tag between panes moves the binding.
- Removing a tab unbinds the session from the pane.

**Active session behavior**
- If a pane is empty and a session is bound, that session becomes active.
- If a pane already has sessions, binding an additional session must not switch the active session.

**Peek modal**
- Clicking a session in the sidebar opens a floating modal showing that session.
- Peek does not change pane bindings.

**Keyboard navigation**
- `Ctrl+Up/Down`: cycle focused pane
- `Ctrl+Left/Right`: cycle active session in focused pane

**Session lifecycle operations**
- Create a new tmux session from the UI (using a configurable prefix).
- Rename and delete individual sessions.
- “Nuke all” destroys all tmux sessions (with confirmation UI).

### 7.2 Settings + persistence

**Persisted state**
- Pane count
- Pane bindings and active session per pane
- Sidebar collapsed state
- Theme and font size
- Auto-refresh/polling interval
- Default session name prefix
- tmux appearance settings

**Theme system**
- Themes: matrix / dark / gastown.
- Use CSS variables so the whole dashboard re-themes consistently.

**tmux appearance**
- Provide presets (matrix/dark/gastown) and custom color controls.
- Apply changes via API so existing tmux sessions update without restarting.

### 7.3 Files

**Native file browser**
- Browse mounted volumes exposed through filebrowser.
- List and grid view.
- Sort by name/size/modified.
- Filter within the current directory.
- Upload (button and drag/drop).
- Download (double click / context menu).
- Rename, delete, create folder.
- Multi-select (Ctrl/Shift) and keyboard shortcuts.

**Inbox / package workflow**
- Provide a dedicated “Inbox” drop area.
- Target path: `/code/incoming` (host mapped to `E:/Code/incoming`).
- When a note is provided, write a sidecar note file next to the first file (current: `originalFilename.note`).

**External filebrowser access**
- Provide a control to open the filebrowser UI in a new tab at `/files/`.

### 7.4 Beads (optional module)

Beads integration is a removable module.

**Project selection**
- Allow selecting a project path under allowed roots.

**Data sources**
- Read `.beads/issues.jsonl` from the selected project.

**Robot protocol (best-effort)**
- If `bv` is installed, call robot commands for triage/insights/plan.
- If not installed, provide graceful fallback responses.

## 8) Non-functional requirements

### 8.1 Remote access + security posture

- Primary access is over Tailscale (no public internet exposure by default).
- Containers are intended to be network-isolated via Tailnet ACLs.
- `/vault` should be read-only for the `dev` user to protect reference materials.

### 8.2 Safety + failure modes

- If tmux is not running, the dashboard should show “no sessions” rather than error-spam.
- Polling should be inexpensive (API caching is acceptable).
- File operations must surface errors clearly (no silent fallbacks).

### 8.3 Performance

- Terminal panes should remain responsive with 20+ sessions present.
- UI should not re-render excessively on polling.

## 9) How to get there (Roadmap)

This section describes the intended path from “working tool” → “reliable daily driver”.

### Milestone A — PRD/Docs alignment (now)
- Ensure the PRD describes the real behavior and naming (tabs, Inbox note extension, tmux appearance settings).
- Keep README/SPEC consistent with PRD (follow-up work).

### Milestone B — Reliability hardening
- Add clear health indicators for: API reachable, ttyd reachable, filebrowser reachable.
- Make polling interval and error retry behavior explicit and testable.
- Confirm persistence behaviors across refreshes and container restarts.

### Milestone C — Security hardening (optional, Tailnet-dependent)
- Document and optionally enable API bearer token auth (`API_AUTH_TOKEN`).
- Document restricted CORS allowlist (`CORS_ORIGINS`) when used outside strict Tailnet assumptions.
- Re-validate file path restrictions for Beads project selection.

### Milestone D — Workflow polish
- Decide on the canonical Inbox sidecar format (keep `.note` or switch to `.letter`) and standardize across docs + any downstream agent tooling.
- Decide whether the dashboard needs a dedicated “Status” tab or whether Help/Settings cover it.

## 10) Acceptance criteria (minimum)

### Terminal
- View 1–4 panes simultaneously.
- Drag sessions from sidebar into panes.
- Move sessions between panes via drag.
- Switch sessions within a pane via tabs and via `Ctrl+Left/Right`.
- Peek a session via modal without changing bindings.
- Create, rename, and delete sessions.
- Nuke all sessions with confirmation.
- Layout + bindings + settings persist across refresh.

### Files
- Load directory contents and navigate via breadcrumbs + back/forward/up.
- Upload/download/rename/delete/create folder.
- Multi-select and shortcuts (F2/Del/F5/Ctrl+A).
- Inbox writes files to `/code/incoming` and writes a note sidecar.

### Beads (when enabled)
- Can load issues from `.beads/issues.jsonl` under an allowed root.
- Triage/insights/plan endpoints return usable data (robot or fallback).
