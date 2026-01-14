# AgentArena Session Management PRD

## Overview

AgentArena is a multi-window terminal dashboard for monitoring and interacting with AI coding agents orchestrated by Gastown. The core use case is managing 20+ concurrent tmux sessions across multiple projects while maintaining easy access and visibility.

## User Story

As a developer using Gastown (an agentic orchestration tool), I need to:
1. Run Gastown which spawns many tmux sessions (agents) automatically
2. Monitor multiple agents across different projects simultaneously
3. Quickly peek at any agent's status without disrupting my workflow
4. Group related agents together for easy cycling
5. Scale from 1-4 visible terminal windows as workload grows

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                        Dashboard UI                          │
├──────────┬──────────────────────────────────────────────────┤
│          │                                                   │
│ SIDEBAR  │              TERMINAL GRID (1-4 windows)         │
│          │                                                   │
│ [Search] │  ┌─────────────────┐  ┌─────────────────┐        │
│          │  │ Window 1        │  │ Window 2        │        │
│ ▼ HQ     │  │ [tab1][tab2][←] │  │ [tab1][←]       │        │
│  • mayor │  │                 │  │                 │        │
│  • admin │  │  ttyd iframe    │  │  ttyd iframe    │        │
│          │  │  (tmux attach)  │  │  (tmux attach)  │        │
│ ▼ gt-rig1│  │                 │  │                 │        │
│  •1 jack │  └─────────────────┘  └─────────────────┘        │
│  •1 emma │                                                   │
│          │  ┌─────────────────┐  ┌─────────────────┐        │
│ ▼ gt-rig2│  │ Window 3        │  │ Window 4        │        │
│  • worker│  │ (empty)         │  │ [tab1][tab2][←] │        │
│          │  │                 │  │                 │        │
│ ▼ Other  │  │ "Drag session"  │  │  ttyd iframe    │        │
│  • shell │  │                 │  │                 │        │
│          │  └─────────────────┘  └─────────────────┘        │
└──────────┴──────────────────────────────────────────────────┘
```

### Key Concepts

| Term | Definition |
|------|------------|
| **Session** | A tmux session running in the container (created by user or Gastown) |
| **Window** | One of 1-4 visible terminal panels in the grid |
| **Bound** | A session assigned to a window (appears in that window's tabs) |
| **Unbound** | A session not assigned to any window (sidebar only) |
| **Active** | The currently displayed session within a window's bound sessions |

## Detailed Specifications

### 1. Sidebar

**Purpose:** Shows ALL tmux sessions, grouped by type, with visual indicators for bound sessions.

**Structure:**
```
[🔍 Search filter input]

▼ HQ (2)
  ●1 hq-mayor          <- Color dot + window number
  ○  hq-admin          <- No dot = unbound

▼ gt-rig1 (3)
  ●2 gt-rig1-jack
  ●2 gt-rig1-emma
  ○  gt-rig1-bob

▼ gt-rig2 (1)
  ○  gt-rig2-worker

▼ Other (1)
  ●4 shell-abc123
```

**Grouping Rules:**
- `hq-*` → "HQ" group (priority 0, shown first)
- `main` or `shell` → "Main" group (priority 1)
- `gt-{rigname}-*` → "gt-{rigname}" group (priority 2)
- Everything else → "Other" group (priority 3)

**Bound Session Indicators:**
- Color-coded dot matching window color (window 1=green, 2=blue, 3=orange, 4=purple)
- Window number badge (1, 2, 3, or 4) next to the session name

**Search/Filter:**
- Text input at top of sidebar
- Filters sessions by name (case-insensitive substring match)
- Groups with no matching sessions are hidden

**Interactions:**
- **Click session:** Opens floating popup modal with that session (regardless of bound state)
- **Drag session to window:** Adds session to that window's bound list (does NOT interrupt anything)
- **Collapse/expand groups:** Click group header to toggle

### 2. Terminal Windows (Grid)

**Layout based on window count:**
- **1 window:** Full width/height
- **2 windows:** Side-by-side (50/50 horizontal split)
- **3 windows:** Two on left (stacked), one on right (full height)
- **4 windows:** 2x2 grid

**Window Anatomy:**
```
┌─────────────────────────────────────────┐
│ [session-1] [session-2] [session-3] [←] │  <- Tab bar
├─────────────────────────────────────────┤
│                                         │
│           ttyd iframe                   │  <- Terminal
│        (attached to active session)     │
│                                         │
└─────────────────────────────────────────┘
```

**Tab Bar:**
- Shows all bound sessions as clickable tabs
- Active session tab is highlighted
- Each tab shows full session name
- **← button** on each tab: Unbinds session (moves back to sidebar)
- **Ctrl+Left/Right Arrow:** Cycles through tabs within the focused window
- **Ctrl+Up/Down Arrow:** Cycles focus between windows (1→2→3→4→1 or reverse)

**Empty Window State:**
- Shows message: "Drag a session here to begin"
- Also accepts dropped sessions

**Session Binding Rules:**
- A session can only be bound to ONE window at a time
- Dragging a bound session to another window moves it (removes from old, adds to new)
- Dragging does NOT change the active session or interrupt anything
- The active session continues displaying until user clicks a different tab

### 3. Floating Popup Modal

**Purpose:** Quick peek at any session without modifying window bindings.

**Trigger:** Click any session in sidebar

**Behavior:**
- Fixed center modal overlay
- Click outside or X button to close
- Shows single session only (the one clicked)
- Fully interactive ttyd terminal (can type commands)
- Does NOT affect window bindings

**Size:** ~80% viewport width/height, centered

### 4. Initial State (First Load)

**On first visit:**
- Window count: 1
- Window 1: Auto-creates a new tmux session (e.g., `shell-{timestamp}`)
- That session is bound to window 1 and active
- Sidebar shows all existing tmux sessions (may be empty)

**On subsequent visits:**
- Restore previous state from localStorage:
  - Window count
  - Which sessions bound to which windows
  - Which session is active in each window
  - Sidebar collapsed state

### 5. Window Count Changes

**Increasing windows (e.g., 2 → 3):**
- New window appears empty
- No auto-session creation (user drags sessions to it)

**Decreasing windows (e.g., 4 → 2):**
- Windows 3 and 4 disappear
- Sessions bound to removed windows become unbound (back to sidebar-only)
- Those sessions continue running in tmux, just not visible

### 6. Terminal Attachment

**How ttyd connects to tmux:**
- ttyd runs with URL argument support: `ttyd -a /usr/local/bin/terminal-launch.sh`
- Terminal URL: `/terminal/?arg={session_name}`
- `terminal-launch.sh` receives session name and runs `tmux attach-session -t {name}`
- User always sees tmux, never raw shell (unless they explicitly detach/exit)

**Session lifecycle:**
- Sessions exist in tmux independent of UI
- Attaching/detaching in UI does not kill sessions
- Gastown creates sessions automatically; they appear in sidebar via API polling

### 7. API Endpoints

**GET /api/tmux/sessions**
```json
{
  "sessions": [
    {"name": "hq-mayor", "agentName": "mayor", "windows": 1, "attached": false, "group": "hq"},
    {"name": "gt-rig1-jack", "agentName": "jack", "windows": 1, "attached": true, "group": "gt-rig1"}
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

**Polling:** Dashboard polls GET every 5 seconds to discover new sessions.

### 8. State Persistence

**Stored in localStorage (`arena-dashboard-state`):**
```json
{
  "windowCount": 2,
  "windows": [
    {
      "id": "window-0",
      "boundSessions": ["hq-mayor", "gt-rig1-jack"],
      "activeSession": "hq-mayor",
      "colorIndex": 0
    },
    {
      "id": "window-1",
      "boundSessions": ["gt-rig1-emma"],
      "activeSession": "gt-rig1-emma",
      "colorIndex": 1
    }
  ],
  "sidebarCollapsed": false,
  "settings": {...}
}
```

### 9. Color Scheme

| Window | Color | Hex |
|--------|-------|-----|
| 1 | Green | #00ff00 |
| 2 | Blue | #00bfff |
| 3 | Orange | #ff8c00 |
| 4 | Purple | #da70d6 |

Used for: Window borders, sidebar dots, tab highlights

### 10. Music Player (Bonus Feature)

**Purpose:** Provide ambient background music for coding sessions.

**Features:**
- **Floating Controls:** Play/Pause, Next/Prev, Volume, Track list
- **Tracks:** Pre-loaded lo-fi/ambient tracks
- **Persistence:** Volume and Play/Pause state saved in user settings
- **UI:** Minimalist icon in global header or sidebar footer


- **Not a tmux replacement:** We don't manage tmux panes/windows, only sessions
- **Not session creation UI:** Gastown creates sessions; we just display them
- **Not a process manager:** Sessions run independently; UI is just a viewer

## Critical Implementation Notes

1. **NEVER reset iframe on drag:** Dragging adds to bound list, does not change active session
2. **NEVER interrupt running terminals:** All window operations are additive/subtractive to bindings only
3. **Sessions are persistent:** Closing UI, removing from window, etc. does NOT kill tmux sessions
4. **API runs as `dev` user:** Must match ttyd user for consistent tmux socket access
5. **TMUX_TMPDIR=/tmp:** Required everywhere for socket consistency

## Success Criteria

- [ ] Can view 4 simultaneous terminals with different agent sessions
- [ ] Can drag session to window without disrupting any sessions
- [ ] Can cycle through 3+ sessions in a single window using tabs. Cycling through tabs does not disrupt any sessions.
- [ ] Can cycle between windows using Ctrl+Up/Down keyboard shortcuts
- [ ] Can peek at any session via popup without affecting window bindings. No disruption caused to sessions.
- [ ] New Gastown sessions appear in sidebar automatically
- [ ] State persists across page refreshes
- [ ] 20+ sessions remain manageable via grouping and search
- [ ] Creating or switching between sessions never disrupts the sessions in any way.
