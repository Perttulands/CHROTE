# CHROTE — Product Requirements Document (PRD)

Last updated: 2026-01-18

## 1) Summary (What / Why)

**CHROTE** (Claude Has Root Over This Environment) is an agent workbench for running and supervising many AI coding agents that live in **tmux sessions**. It provides a single, Tailnet-accessible web dashboard to:

- **Watch and interact with many tmux sessions at once** (dual workspaces, 1–4 panes each, tabbed sessions per pane)
- **Track project issues** via Beads integration with AI-powered triage recommendations
- **Manage files** in mounted workspaces (browse, upload, download, rename, delete)
- **Send "packages" to agents** via an incoming folder workflow

The product exists to solve a very specific pain: once an orchestrator spawns 10–30+ agent sessions, native tmux alone becomes hard to monitor remotely and hard to "sample" without constantly detaching/attaching. CHROTE makes the tmux swarm legible, navigable, and controllable from any device.

---

## 2) Goals

1. **High-signal supervision** of many concurrent agent tmux sessions
2. **Non-disruptive interaction**: rebind/peek/switch sessions without killing or interrupting agents
3. **Remote access by default**: usable from phone/tablet/laptop over Tailscale
4. **Project visibility**: track issues and get AI-powered work recommendations via Beads
5. **Practical file ops** for the mounted workspace and a dead-simple "send to agents" path
6. **Low operational overhead**: single binary deployment, minimal moving parts

---

## 3) Non-goals

- Not a tmux replacement (no pane/window management UI; sessions are the unit)
- Not a full process supervisor (agents may fail; the UI is primarily for observation and manual control)
- Not a public multi-tenant SaaS (assume a single trusted Tailnet; optional token auth exists for hardening)
- Not an IDE (no in-browser editor; file operations only)

---

## 4) Target Users

**Primary:** A developer/operator running agentic coding workflows (often via Gastown) who needs to supervise many agent sessions across multiple projects from a single command center.

**Secondary:** A developer who wants the same view from any device (e.g., kitchen tablet monitoring builds, phone checking agent status).

---

## 4.5) Security Model

CHROTE runs agents as a **non-root Linux user** (`chrote`) with no sudo access. This provides proper security isolation:

| Aspect | Implementation |
|--------|----------------|
| **User** | `chrote` (UID 1000, no sudo) |
| **File access** | Only `/code`, `/vault` via symlinks |
| **Process isolation** | User permissions, not container boundary |
| **Escalation** | Not possible - no sudo, no setuid binaries |

### File Ingestion Model

Since agents cannot access the host filesystem directly, files are **uploaded via the dashboard**:

```
Windows Desktop                    WSL (chrote user)
────────────────                   ─────────────────
E:\MyProject\file.txt
        │
        │ (drag to browser)
        ▼
┌─────────────────────┐
│  CHROTE Dashboard   │─────────►  /code/incoming/file.txt
│  (Files Tab)        │            (owned by chrote:chrote)
└─────────────────────┘
        │
        │ (with optional note)
        ▼
                                   /code/incoming/file.txt.note
```

**Benefits:**
- Files land directly on native Linux filesystem (ext4) - fast
- Correct ownership (chrote:chrote) automatically
- Explicit sandbox boundary - agents only see what you give them
- No accidental access to Windows system files

---

## 5) Architecture

### 5.1 System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Browser (Any Device)                      │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                    React Dashboard                          ││
│  │  ┌──────────┬──────────┬───────┬───────┬──────────┐        ││
│  │  │Terminal 1│Terminal 2│ Files │ Beads │ Settings │        ││
│  │  └──────────┴──────────┴───────┴───────┴──────────┘        ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       WSL2 (Ubuntu 24.04)                        │
│                      User: chrote (non-root)                     │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                  systemd services                            ││
│  │  ┌───────────────────────┐  ┌───────────────────────┐       ││
│  │  │ chrote-server.service │  │ chrote-ttyd.service   │       ││
│  │  │ (Go binary :8080)     │  │ (web terminal :7681)  │       ││
│  │  └───────────────────────┘  └───────────────────────┘       ││
│  └─────────────────────────────────────────────────────────────┘│
│                              │                                   │
│  ┌─────────────┬─────────────┬─────────────────┐               │
│  │ tmux API    │  Beads API  │   Files API     │               │
│  │ sessions    │  issues     │   browse/upload │               │
│  └─────────────┴─────────────┴─────────────────┘               │
│                                                                  │
│  Tailscale (native) → hostname: chrote                          │
└─────────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
   ┌─────────┐          ┌─────────┐          ┌──────────────────┐
   │  tmux   │          │  ttyd   │          │ Filesystem       │
   │ sessions│          │ :7681   │          │ /code (native)   │
   │ socket: │          └─────────┘          │ /vault (mounted) │
   │ /run/   │                               │ /incoming        │
   │ tmux/   │                               └──────────────────┘
   │ chrote/ │
   └─────────┘
```

### 5.2 Single Binary Design

CHROTE runs as a single Go binary (`chrote-server`) that provides:

- **HTTP server** on port 8080 serving all endpoints
- **Embedded React dashboard** (no separate static file serving needed)
- **Terminal proxy** to ttyd for web-based tmux access
- **API layer** for tmux, Beads, and file operations
- **Graceful shutdown** with signal handling

### 5.3 Deployment (WSL)

CHROTE runs in WSL2 with systemd managing services:

| Service | Purpose | Port |
|---------|---------|------|
| `chrote-server.service` | Go API + dashboard | 8080 |
| `chrote-ttyd.service` | Web terminal | 7681 |

**Startup:**
```powershell
# Windows - WSL starts services automatically via systemd
wsl -d Ubuntu-24.04 echo "CHROTE started"
Start-Process "http://chrote:8080"
```

**Filesystem Layout:**
```
/home/chrote/chrote/       # Git repo, native Linux (fast)
├── src/                   # Go server source
├── dashboard/             # React source
└── vendor/                # gastown, beads, beads_viewer

/code   → /home/chrote/chrote    (symlink, read-write)
/vault  → /mnt/e/Vault           (symlink, read-only)
```

### 5.4 Data Flow

```
User Action → React Component → API Call → Go Handler → System Command → Response
     │              │               │            │              │
     │              │               │            │              ▼
     │              │               │            │         tmux/bv
     │              │               │            │              │
     │              │               │            ▼              │
     │              │               │      JSON Response ◄──────┘
     │              │               │            │
     │              ▼               ▼            │
     │         State Update ◄── useEffect ◄─────┘
     │              │
     ▼              ▼
   UI Re-render ← Context
```

---

## 6) Dashboard Design

### 6.1 Tab Structure

```
┌────────────┬────────────┬───────┬───────┬──────────┬──────┐
│ Terminal 1 │ Terminal 2 │ Files │ Beads │ Settings │ Help │
└────────────┴────────────┴───────┴───────┴──────────┴──────┘
                                                ┌─────────────┐
                                                │ 🎵 Music    │
                                                │   Player    │
                                                └─────────────┘
```

### 6.2 Terminal Workspace Layout

Each terminal tab contains an independent workspace:

```
┌─────────────────────────────────────────────────────────────────────┐
│ [+] Create  [🔍 Search...]  [☠️ Nuke All]              [◀ Collapse] │
├─────────────┬───────────────────────────────────────────────────────┤
│             │  ┌─────────────────────┬─────────────────────┐        │
│  Sessions   │  │ Window 1 (blue)     │ Window 2 (purple)   │        │
│  ─────────  │  │ [tag1] [tag2]       │ [tag3]              │        │
│             │  │                     │                     │        │
│  ▼ HQ       │  │   ┌─────────────┐   │   ┌─────────────┐   │        │
│    hq-main  │  │   │   ttyd      │   │   │   ttyd      │   │        │
│    hq-coord │  │   │   iframe    │   │   │   iframe    │   │        │
│             │  │   │             │   │   │             │   │        │
│             │  │   └─────────────┘   │   └─────────────┘   │        │
│             │  ├─────────────────────┼─────────────────────┤        │
│             │  │ Window 3 (green)    │ Window 4 (orange)   │        │
│  ▼ Rigs     │  │ [tag4] [tag5]       │                     │        │
│    gt-rig-1 │  │                     │     (empty)         │        │
│    gt-rig-2 │  │   ┌─────────────┐   │                     │        │
│             │  │   │   ttyd      │   │   "Drag session     │        │
│  ▼ Other    │  │   │   iframe    │   │    here"            │        │
│    shell1   │  │   │             │   │                     │        │
│    tmux-0   │  │   └─────────────┘   │                     │        │
│             │  └─────────────────────┴─────────────────────┘        │
└─────────────┴───────────────────────────────────────────────────────┘
```

### 6.3 Session Panel Design

**Grouping by prefix:**
| Prefix | Group | Priority | Color |
|--------|-------|----------|-------|
| `hq-*` | HQ | 0 | — |
| `main`, `shell` | Main | 1 | — |
| `gt-{rig}-*` | Gastown Rigs | 3 | — |
| Other | Other | 4 | — |

**Session states:**
- Unassigned (no color) → Click to peek in modal
- Assigned to window → Colored dot matching window
- Active in window → Bold text

### 6.4 Window Design

**Window count options:** 1, 2, 3, or 4 windows per workspace

**Layouts:**
```
1 window:     2 windows:      3 windows:       4 windows:
┌─────────┐   ┌─────┬─────┐   ┌───┬───┬───┐   ┌─────┬─────┐
│         │   │     │     │   │   │   │   │   │     │     │
│    1    │   │  1  │  2  │   │ 1 │ 2 │ 3 │   │  1  │  2  │
│         │   │     │     │   │   │   │   │   ├─────┼─────┤
└─────────┘   └─────┴─────┘   └───┴───┴───┘   │  3  │  4  │
                                              └─────┴─────┘
```

**Window header:**
- Window number with color indicator
- Session tags (draggable)
- Active session highlighted
- Click tag to switch active session
- X button to remove session from window

### 6.5 Drag-and-Drop System

```
Drag Sources              Drop Targets
─────────────             ────────────
Session in sidebar   →    Terminal window (assign)
Session tag          →    Another window (move)
Session tag          →    Trash zone (unassign)
Files                →    Upload zone (upload)
```

---

## 7) User Journeys

### 7.1 Journey: First-Time Setup

```
1. User starts chrote-server (WSL systemd auto-starts)
2. Opens http://chrote:8080 in browser (via Tailscale)
3. Sees empty Terminal 1 tab with sidebar
4. Clicks [+] to create first session "shell1"
5. Session appears in sidebar under "Main" group
6. Drags session to Window 1
7. Terminal loads in iframe, ready for use
```

### 7.2 Journey: Multi-Agent Monitoring (Gastown)

```
1. Gastown spawns 20 sessions: hq-main, gt-rig1-1..5, gt-rig2-1..5, etc.
2. User opens CHROTE dashboard, sees sessions auto-discovered
3. Sessions grouped: HQ (1), Rigs (rig1: 5, rig2: 5), etc.
4. User configures Terminal 1: 4 windows
   - Window 1: hq-main (coordination)
   - Window 2: gt-rig1-1, gt-rig1-2 (rig 1 workers)
   - Window 3: gt-rig2-1, gt-rig2-2 (rig 2 workers)
   - Window 4: gt-rig1-3 (overflow)
5. User configures Terminal 2 for remaining workers
6. Uses Ctrl+Left/Right to cycle through sessions per window
7. Clicks session in sidebar to peek without disrupting layout
8. Layout persists across browser refresh
```

### 7.3 Journey: Issue Triage (Beads)

```
1. User has project with .beads/ directory
2. Opens Beads tab
3. Selects project from dropdown
4. Views Kanban: sees issues organized by status
   - Open (5) → Ready (3) → In Progress (2) → Blocked (1) → Closed (10)
5. Switches to Triage tab
6. Sees AI-ranked recommendations:
   - #1: "Fix auth bug" - HIGH impact, blocks 3 issues
   - #2: "Add logging" - MEDIUM impact, quick win
   - #3: "Refactor API" - LOW impact, high effort
7. Switches to Insights tab
8. Reviews health score: 72/100
   - Warnings: 2 stale issues
   - Risks: Circular dependency detected
9. Uses graph metrics to understand issue relationships
```

### 7.4 Journey: File Management

```
1. User opens Files tab
2. Sees root: /code, /vault directories
3. Navigates: /code → myproject → src
4. Uses keyboard shortcuts:
   - F5 to refresh
   - Ctrl+A to select all
   - F2 to rename selected
5. Uploads file via drag-drop to upload zone
6. Creates new folder with [+📁] button
7. Downloads file via right-click → Download
8. Uses breadcrumbs to navigate back
```

### 7.5 Journey: Sending Package to Agents (Inbox)

```
1. User has files to send to agents
2. Opens Files tab → Inbox panel
3. Drags files into Inbox drop zone
4. Adds optional note: "Please review these specs"
5. Clicks [Send]
6. Files written to /code/incoming/
7. Note written as filename.note sidecar
8. Agents pick up files from incoming folder
```

### 7.6 Journey: Customizing Appearance

```
1. User opens Settings tab
2. Selects theme: Matrix (green) / Dark (blue) / Gastown (gold)
3. Adjusts font size: 14px
4. Customizes tmux colors:
   - Status bar: bg=#1a1a2e, fg=#00ff00
   - Active pane border: #00ff00
5. Changes apply immediately (hot-reload)
6. All terminal iframes update with new font size
7. tmux sessions update with new colors
```

---

## 8) Functional Requirements

### 8.1 Terminal Supervision

**Dual Workspaces**
- Two independent terminal tabs (Terminal 1, Terminal 2)
- Each workspace has its own 1-4 window configuration
- Sessions can be assigned to windows in either workspace
- State persists separately per workspace

**Multi-Window Layout**
- Support 1–4 windows per workspace in responsive grid
- Each window can have 0..N bound tmux sessions
- Windows have distinct colors: blue, purple, green, orange

**Session Discovery + Grouping**
- Poll tmux sessions every 5 seconds (configurable)
- Group by naming convention with priority ordering
- Text filter narrows sessions by name in real-time
- Show session count per group

**Binding Model**
- Session bound to at most one window at a time (across both workspaces)
- Drag session to window → binds it
- Drag session tag between windows → moves binding
- Click X on tag → unbinds without deleting session
- Click unassigned session → opens peek modal

**Session Lifecycle**
- Create new session with configurable prefix (e.g., shell1, shell2)
- Rename session inline (Enter to save, Escape to cancel)
- Delete individual session with confirmation
- Nuke all sessions with confirmation modal + header protection

**Keyboard Navigation**
- `Ctrl+Left/Right`: cycle active session in focused window
- Click window to focus

### 8.2 Beads Integration

**Kanban View**
- Columns: Open → Ready → In Progress → Blocked → Closed
- Issue cards with: title, ID, type, priority, labels, assignee
- Priority sorting within columns
- Status color coding

**Triage View**
- AI-ranked recommendations
- Quick wins identification
- Blocker highlighting
- Impact levels: high / medium / low
- Reasoning for each recommendation

**Insights View**
- Project metrics: total, open, blocked, closed counts
- Health score (0-100) with risk assessment
- Warnings and risks identified
- Graph metrics when available:
  - PageRank importance
  - Betweenness centrality
  - Dependency cycles
  - Critical path analysis

**Project Discovery**
- Auto-discover .beads directories in /code and /workspace
- Manual path configuration in Settings
- Project selector dropdown

### 8.3 File Browser

**Navigation**
- Breadcrumb path with click navigation
- Back / Forward / Up buttons with history
- Path bar showing current location
- Roots: /code (read-write), /vault (read-only)

**Views**
- List view: name, size, modified columns
- Grid view: icon-based thumbnails
- Sort by: name, size, modified (ascending/descending)
- Directories always listed first

**Operations**
- Upload: drag-drop zone + button
- Download: double-click or context menu
- Create folder: button + dialog
- Rename: F2 or context menu, inline editing
- Delete: Delete key or context menu with confirmation
- Copy path: context menu

**Selection**
- Single click: select one
- Ctrl+Click: toggle selection
- Shift+Click: range selection
- Ctrl+A: select all

**Inbox Panel**
- Dedicated drop area for agent packages
- Target path: /code/incoming
- Optional note creates sidecar file (filename.note)

### 8.4 Music Player

**Controls**
- Play / Pause toggle
- Previous / Next track
- Track selector dropdown
- Volume slider (0-100%)
- Volume persists in settings

**Tracks (10 built-in)**
- Convoy Run, Deacon's Revenge, Design Phase, March of the Polecats
- Mayor's Introspection, Merge Push, Polecat Danceparty
- The Idle Polecat, Vibes at the HQ, Who Ate My PRD?

**Behavior**
- Auto-advance on track completion
- Graceful handling of playback failures
- Embedded in tab bar (always accessible)

### 8.5 Settings

**Appearance**
- Theme: Matrix (green) / Dark (blue) / Gastown (gold)
- Font size: 12-20px slider
- Applies globally to dashboard and terminal iframes

**tmux Appearance (Hot-Reload)**
- Presets: Matrix / Dark / Gastown
- Custom colors:
  - Status bar: background, foreground
  - Pane borders: active, inactive
  - Selection mode: background, foreground
- Changes apply immediately to running tmux sessions

**Session Defaults**
- Auto-refresh interval: 1-30 seconds
- Default session prefix: customizable

**Beads Configuration**
- Manual project path management
- Add/remove nested project paths

**Persistence**
- All settings saved to localStorage
- State migration between versions
- Graceful handling of missing fields

### 8.6 Help System

**Sections:**
1. Overview - Quick start guide with feature cards
2. Terminal - Window layouts, controls, session management
3. Sessions - Panel features, drag-and-drop, keyboard shortcuts
4. Files - Browser features, operations, shortcuts
5. tmux Commands - Complete reference (60+ shortcuts)
6. Tips & Tricks - Pro tips for power users

---

## 9) API Reference

### 9.1 tmux API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/tmux/sessions` | List all sessions with grouping |
| POST | `/api/tmux/sessions` | Create new session |
| PATCH | `/api/tmux/sessions/:name` | Rename session |
| DELETE | `/api/tmux/sessions/:name` | Delete session |
| DELETE | `/api/tmux/sessions/all` | Nuke all (requires X-Nuke-Confirm header) |
| POST | `/api/tmux/appearance` | Apply color settings (hot-reload) |

### 9.2 Beads API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/beads/health` | Check if bv CLI installed |
| GET | `/api/beads/projects` | Discover .beads directories |
| GET | `/api/beads/issues` | List issues for project |
| GET | `/api/beads/triage` | Get AI recommendations |
| GET | `/api/beads/insights` | Project metrics and health |
| GET | `/api/beads/graph` | Dependency graph data |

### 9.3 Files API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/files/resources/` | Root listing (code, vault) |
| GET | `/api/files/resources/{path}` | Directory listing or file info |
| POST | `/api/files/resources/{path}` | Upload file or create folder |
| PATCH | `/api/files/resources/{path}` | Rename/move |
| DELETE | `/api/files/resources/{path}` | Delete file/folder |
| GET | `/api/files/raw/{path}` | Download file |

### 9.4 Health API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | Server health check |

---

## 10) Non-Functional Requirements

### 10.1 Security

- Primary access over Tailscale (no public internet exposure)
- Optional bearer token auth (`API_AUTH_TOKEN`)
- Optional CORS allowlist (`CORS_ORIGINS`)
- Path traversal protection in file API
- Nuke protection via confirmation header
- /vault mounted read-only

### 10.2 Performance

- Terminal panes responsive with 20+ sessions
- Session polling cached (1s TTL)
- UI uses memoization to prevent excessive re-renders
- Lazy iframe loading for terminals
- Debounced resize events (100ms)

### 10.3 Reliability

- No sessions = "no sessions" message (not error spam)
- Error boundaries isolate component failures
- Toast notifications for operation feedback
- Graceful degradation (e.g., music playback failures)
- State persists across refresh and restart

---

## 11) Roadmap

### Milestone A — Go Rewrite ✅ Complete
- Single Go binary replacing Node.js + nginx
- Embedded React dashboard
- Full API parity: tmux, files, beads
- ttyd integration for terminal proxy

### Milestone B — WSL Migration ⏳ In Progress
- Migrate from Docker to native WSL2
- Non-root user (`chrote`) with no sudo
- systemd service management
- Native Linux filesystem for performance
- Tailscale native (not sidecar container)

### Milestone C — Reliability Hardening
- Health indicators for API and ttyd reachability
- Explicit polling and retry behavior
- Persistence verification across restarts
- Toast notifications for all async operations

### Milestone D — Security Hardening
- Document bearer token auth
- Document CORS configuration
- Audit path validation

### Milestone E — Workflow Polish
- Standardize Inbox sidecar format
- Wire up Music Player to dashboard
- Mobile-responsive improvements

---

## 12) Acceptance Criteria

### Terminal
- [ ] View 1–4 windows in two independent workspaces
- [ ] Drag sessions from sidebar to windows
- [ ] Move sessions between windows via tag drag
- [ ] Cycle sessions with Ctrl+Left/Right
- [ ] Peek session via floating modal
- [ ] Create, rename, delete sessions
- [ ] Nuke all with confirmation
- [ ] Layout persists across refresh

### Beads
- [ ] Discover projects with .beads/
- [ ] View issues in Kanban layout
- [ ] Get AI triage recommendations
- [ ] Review project insights and health score
- [ ] Navigate graph metrics

### Files
- [ ] Browse /code and /vault
- [ ] Navigate via breadcrumbs and history
- [ ] Upload via drag-drop
- [ ] Download, rename, delete files
- [ ] Create folders
- [ ] Send packages via Inbox with notes

### Settings
- [ ] Switch themes (Matrix/Dark/Gastown)
- [ ] Adjust font size
- [ ] Customize tmux colors with hot-reload
- [ ] Configure refresh interval and session prefix

### Music
- [ ] Play/pause, previous/next
- [ ] Select track from dropdown
- [ ] Adjust and persist volume
