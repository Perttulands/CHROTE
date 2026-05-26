# CHROTE UX Improvements

Last updated: 2026-01-18

## Overview

This document captures UX gaps and improvement recommendations for the CHROTE dashboard, based on analysis against the PRD requirements and real-world usage patterns.

**Current UX Score: 6.5/10** — Strong technical foundation, needs better onboarding and feedback.

For context on Gastown (the orchestration framework CHROTE supports), see [GASTOWN.md](GASTOWN.md).

---

## UX Strengths (What Works)

### State Management
- Robust localStorage persistence with version migration
- Orphan session cleanup on reconnect
- Layout survives page refresh

### Drag-and-Drop
- 4px activation threshold (responsive, not twitchy)
- Visual overlay during drag
- "Drop here" hints on targets

### Theme System
- 3 themes (Matrix/Dark/Gastown) with instant switching
- Consistent CSS variables
- Good contrast ratios

### File Browser
- Full keyboard support (F5, F2, Ctrl+A, Delete)
- Multiple selection modes (click, Ctrl+click, Shift+click)
- Breadcrumb navigation with history

### Help System
- Comprehensive 6-section reference
- tmux command cheatsheet
- Feature overview cards

---

## UX Gaps

### 1. No First-Time Onboarding

**Problem**: New users see empty dashboard with no guidance.

**Current state**:
- SessionPanel shows "No tmux sessions" but doesn't suggest next step
- [+] button exists but lacks context for tmux-unfamiliar users
- Help tab is passive (user must navigate there)

**Impact**: Users who don't know tmux are immediately lost.

### 2. Missing Feedback for Async Operations

**Problem**: Operations feel silent — no confirmation of success/failure.

**Missing toasts**:
- "Created session: tmux1" (success)
- "Failed to create: <error>" (failure)
- "Uploaded file.txt" (success)
- "Destroyed 15 sessions" (after Nuke)

**Impact**: Users don't know if their actions worked.

### 3. No Connection Health Indicator

**Problem**: When API or ttyd fails, user doesn't know why.

**Current state**:
- API timeout shows generic error
- ttyd iframe fails silently (blank terminal)
- No status badge showing system health

**Needed**: Green/yellow/red indicator in TabBar.

### 4. Keyboard Shortcuts Not Discoverable

**Problem**: Power features exist but are invisible.

**Issues**:
- ~~Ctrl+Left/Right for cycling~~ (removed — tmux captures these)
- No `?` key for shortcut overlay
- Buttons lack tooltips with keyboard hints
- No legend in Help showing active shortcuts per tab

### 5. Session Panel Lacks Visual Clarity at Scale

**Problem**: With 20+ sessions, it's hard to see assignment status.

**Missing**:
- Group counts: `▼ HQ (1)`, `▼ Rigs (10)`, `▼ Other (5)`
- Clear visual distinction: assigned vs unassigned
- Summary badge: "3/20 assigned"

### 6. Terminal Window Focus Unclear

**Problem**: Can't tell which window has keyboard focus.

**Missing**:
- Border highlight on focused window
- Visual indicator for cycling availability
- "Click to focus" hint for new users

### 7. Empty States Need Animation

**Problem**: "Drag session here" is static and easy to miss.

**Better**: Subtle pulsing effect, arrow hint, or count of available sessions.

---

## Mobile Considerations

### Design Philosophy

**95% of usage is on large screens.** We must NOT sacrifice the desktop experience to accommodate mobile. However, mobile has legitimate use cases:

- Quick status checks from phone
- Sending messages to agents while away from desk
- Monitoring build/test progress

### The Wrong Approach

Converting the current UI to responsive would:
- Cram 4 terminal windows into phone viewport (unusable)
- Force complex drag-drop onto touch (frustrating)
- Reduce information density for desktop users

### The Right Approach: Messenger-Style Mobile Interface

Instead of responsive terminals, provide a **separate mobile-optimized view** focused on the [Gastown mail system](GASTOWN.md#the-mail-system):

```
┌─────────────────────────────┐
│  CHROTE          [≡]   │
├─────────────────────────────┤
│  🎩 Mayor (gt-gastown)      │
│  ────────────────────────── │
│  [14:32] Mayor: Finished    │
│  convoy gt-abc12. 3 beads   │
│  completed, 1 blocked.      │
│                             │
│  [14:28] You: Focus on the  │
│  auth refactor first        │
│                             │
│  [14:25] Mayor: Starting    │
│  convoy with 4 polecats...  │
├─────────────────────────────┤
│  📧 Rigs: gastown | myproj  │
├─────────────────────────────┤
│  [Type message to Mayor...] │
│                      [Send] │
└─────────────────────────────┘
```

**Benefits**:
- Natural "texting" metaphor everyone understands
- Works perfectly on phone
- Leverages existing `gt mail` infrastructure
- Doesn't compromise desktop UI
- Could also be useful as a secondary panel on desktop

### Implementation Path

1. **New `/mobile` route** — Separate React view, not responsive breakpoints
2. **Message API** — Wrapper around `gt mail send/inbox`
3. **WebSocket for updates** — Real-time message delivery
4. **Rig selector** — Quick switch between project Mayors
5. **Status summary** — Convoy progress, agent counts

---

## Priority Recommendations

### P1 — Critical (Blocking Issues)

| Item | Description | Effort |
|------|-------------|--------|
| Toast notifications | Success/failure feedback for all async ops | Medium |
| Connection status | Green/yellow/red badge in TabBar | Small |
| First-time onboarding | Modal with 3-step guide on empty state | Medium |

### P2 — Important (Significant UX Lift)

| Item | Description | Effort |
|------|-------------|--------|
| Session group badges | Show counts per category | Small |
| Shortcut overlay | `?` key toggles cheatsheet modal | Small |
| Window focus indicator | Border highlight on active window | Small |
| Button tooltips | Add `title` attributes with shortcuts | Small |

### P3 — Polish (Nice to Have)

| Item | Description | Effort |
|------|-------------|--------|
| Animated empty states | Pulsing drop zones, arrow hints | Small |
| Message panel (desktop) | Gastown mail sidebar for quick comms | Large |
| Mobile messenger view | Dedicated `/mobile` route | Large |

---

## Removed Features

### Keyboard Session Cycling

**Original PRD**: "Ctrl+Left/Right: cycle active session in focused window"

**Removed because**: When a tmux session is active in the terminal iframe, all keyboard input goes to tmux. Ctrl+arrow combinations are captured by tmux for word navigation, not passed to the dashboard.

**Alternative**: Click session tags in window header to switch, or use session panel.

---

## Acceptance Criteria Updates

Based on this analysis, the following PRD acceptance criteria should be revised:

### Terminal
- [x] View 1-4 windows in two independent workspaces
- [x] Drag sessions from sidebar to windows
- [x] Move sessions between windows via tag drag
- [ ] ~~Cycle sessions with Ctrl+Left/Right~~ → **Remove** (tmux captures)
- [x] Peek session via floating modal
- [x] Create, rename, delete sessions
- [x] Nuke all with confirmation
- [x] Layout persists across refresh
- [ ] **Add**: Toast feedback for session operations
- [ ] **Add**: Connection health indicator

---

## Scorecard

| Criteria | Current | Target |
|----------|---------|--------|
| User Journeys | 7/10 | 9/10 |
| Visual Design | 8/10 | 9/10 |
| Interaction Patterns | 6/10 | 8/10 |
| Error Feedback | 5/10 | 9/10 |
| Discoverability | 6/10 | 8/10 |
| Mobile/Tablet | 3/10 | 7/10 |
| State Persistence | 9/10 | 9/10 |

**Current Overall: 6.5/10 → Target: 8.5/10**
