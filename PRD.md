# AgentArena Product Requirements

## Vision

AgentArena is a multi-window terminal dashboard for monitoring and interacting with AI coding agents orchestrated by Gastown. The core use case is managing 20+ concurrent tmux sessions across multiple projects while maintaining easy access and visibility from any device.

## User Story

As a developer using Gastown (an agentic orchestration tool), I need to:
1. Run Gastown which spawns many tmux sessions (agents) automatically
2. Monitor multiple agents across different projects simultaneously
3. Quickly peek at any agent's status without disrupting my workflow
4. Group related agents together for easy navigation
5. Scale from 1-4 visible terminal windows as workload grows
6. Access everything remotely from any device on my network
7. Browse and manage files alongside terminal sessions
8. Send files with instructions to agents for processing

## Core Requirements

### Terminal Management

**Multi-Window View**
- Display 1-4 terminal windows simultaneously in a responsive grid
- Each window can hold multiple sessions (tabbed)
- Windows are color-coded for easy identification
- Layout adapts: 1=full, 2=side-by-side, 3=2+1, 4=2x2 grid

**Session Organization**
- All tmux sessions appear in a collapsible sidebar
- Sessions grouped by type: HQ (headquarters roles), Main, Rigs (project-specific), Other
- Sessions show bound status with colored indicators matching their window
- Search/filter to quickly find sessions in large lists

**Session Assignment**
- Drag sessions from sidebar to windows to bind them
- Click any session to peek at it in a popup modal (non-disruptive)
- Sessions can be moved between windows via drag
- Unbind sessions via X button on tabs
- Binding/unbinding never interrupts running sessions

**Keyboard Navigation**
- Ctrl+Up/Down to cycle focus between windows
- Ctrl+Left/Right to cycle sessions within focused window
- Tab cycling never disrupts the underlying terminal

### File Management

**File Browser**
- Native file browser integrated into dashboard
- Browse mounted code and vault directories
- Upload files via drag-and-drop
- Download, rename, delete, create folders
- List view (sortable) and grid view options
- Keyboard shortcuts for power users

**Inbox System**
- Dedicated inbox tab for sending files to agents
- Drop file + write instructions
- Creates file with accompanying .letter sidecar containing message
- Files delivered to incoming folder for agent processing

### Project Visualization (Beads)

- Interactive dependency graph showing issue relationships
- Kanban board view organized by status
- AI-powered triage recommendations
- Graph metrics and health insights

### Customization

**Themes**
- Matrix (green neon on black) - default
- Dark (blue accent on neutral)
- Gastown (warm cream/russet)
- Theme applies consistently across all components

**Settings**
- Adjustable terminal font size
- Theme selection
- Collapsible sidebar
- All preferences persist across sessions

**Music Player**
- Ambient background music for coding sessions
- Volume control, track selection
- Minimalist controls in header

### Remote Access

- Access from any device via Tailscale hostname
- No public internet exposure required
- SSH access available for direct terminal use
- Local LLM API (Ollama) accessible for agent use

### Security

- Network isolation: containers cannot reach other network devices
- Read-only vault for reference files (agents can read, not modify)
- Sensitive files hidden from sandbox via overlays

---

## User Interface

### Layout

```
┌─────────────────────────────────────────────────────────────┐
│  [Terminal] [Files] [Beads] [Status]     [Settings] [Music] │
├──────────┬──────────────────────────────────────────────────┤
│          │                                                   │
│ SIDEBAR  │              TERMINAL GRID (1-4 windows)         │
│          │                                                   │
│ [Search] │  ┌─────────────────┐  ┌─────────────────┐        │
│          │  │ Window 1 (Blue) │  │ Window 2 (Purple)│        │
│ ▼ HQ     │  │ [tab1][tab2][×] │  │ [tab1][×]        │        │
│  ●1 mayor│  │                 │  │                  │        │
│  ○  admin│  │  terminal       │  │  terminal        │        │
│          │  │                 │  │                  │        │
│ ▼ gt-rig │  └─────────────────┘  └─────────────────┘        │
│  ●2 jack │                                                   │
│  ●2 emma │  ┌─────────────────┐  ┌─────────────────┐        │
│          │  │ Window 3 (Green)│  │ Window 4 (Orange)│        │
│ ▼ Other  │  │ (empty)         │  │ [tab1][tab2][×]  │        │
│  ○  shell│  │ "Drag session"  │  │  terminal        │        │
│          │  └─────────────────┘  └─────────────────┘        │
│          │                                                   │
│[☢ Nuke]  │                                                   │
└──────────┴──────────────────────────────────────────────────┘
```

### Sidebar Indicators
- Colored dot matches window color (blue/purple/green/orange)
- Window number badge (1-4) next to bound sessions
- No dot = unbound (available to assign)

### Floating Modal
- Click any session to "peek" without affecting window bindings
- Draggable popup with full terminal functionality
- Click outside or X to close

---

## Key Behaviors

### Session Lifecycle
- Sessions exist in tmux independent of UI
- Attaching/detaching in UI does not kill sessions
- Gastown creates sessions automatically; they appear via polling
- Sessions persist until explicitly killed or server stops

### Drag-and-Drop Rules
1. Dragging to a window ADDS session to bound list
2. Dragging does NOT change active session (no interruption)
3. Dragging to empty window auto-activates (nothing to interrupt)
4. Session can only be bound to ONE window at a time
5. Moving between windows: removed from old, added to new

### Click Behaviors
- Click session in sidebar → Opens floating peek modal
- Click tab in window → Switches to that session
- Click X on tab → Unbinds session (back to sidebar-only)

### State Persistence
- Window count and layout
- Session bindings per window
- Active session per window
- Sidebar collapsed state
- Theme and font preferences

---

## What This Is NOT

- **Not a tmux replacement:** We don't manage tmux panes/windows, only sessions
- **Not session creation UI:** Gastown creates sessions; we just display them
- **Not a process manager:** Sessions run independently; UI is just a viewer
- **Not publicly accessible:** Protected by Tailscale network

---

## Acceptance Criteria

### Terminal Management
- [x] View 4 simultaneous terminals with different agent sessions
- [x] Drag session to window without disrupting any sessions
- [x] Cycle through 3+ sessions in a window using tabs
- [x] Cycle between windows using Ctrl+Up/Down
- [x] Peek at any session via popup without affecting bindings
- [x] New Gastown sessions appear in sidebar automatically
- [x] State persists across page refreshes
- [x] 20+ sessions remain manageable via grouping and search
- [x] Nuke all sessions with confirmation dialog

### File Browser
- [x] Native file browser loads and displays directory contents
- [x] File browser adapts to all three themes
- [x] Navigate directories via breadcrumbs, buttons, double-click
- [x] Upload files via drag-and-drop and file picker
- [x] Download files via double-click or context menu
- [x] Create, rename, and delete files/folders
- [x] Keyboard shortcuts work (F2, Del, F5, etc.)
- [x] Multi-select works (Ctrl+click, Shift+click, Ctrl+A)
- [x] List and Grid views both function correctly

### Inbox System
- [x] Inbox tab appears in Files view
- [x] Drag-and-drop files to upload
- [x] Write letter/instructions in textarea
- [x] Files uploaded to incoming folder
- [x] Letter saved as .letter sidecar file
- [x] Status feedback shows success/error
- [x] Adapts to all three themes

### Theme System
- [x] All components use CSS variables exclusively
- [x] Theme changes apply instantly across entire dashboard
- [x] Theme preference persists in localStorage
