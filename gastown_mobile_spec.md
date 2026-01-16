# Gastown Mobile: Mobile Alternative for AgentArena

## Executive Summary

A mobile-optimized version of AgentArena using a **shared codebase** approach. Instead of building a separate React Native app, we'll use a **PWA (Progressive Web App) with responsive design** - sharing the same React/Vite codebase, theme system, and components between desktop and mobile.

**Key Insight**: Since the terminal functionality requires server access anyway (ttyd over WebSocket), and @dnd-kit already supports touch events, a PWA approach provides near-zero additional code while keeping everything in one codebase.

---

## Code Sharing Strategy

### Why PWA Over React Native

| Factor | PWA | React Native |
|--------|-----|--------------|
| Theme sharing | Same CSS variables | Must convert to StyleSheet |
| Component sharing | 100% shared | ~60% rewrite needed |
| @dnd-kit | Works with touch | Incompatible, need rewrite |
| xterm.js/ttyd | Works unchanged | WebView wrapper needed |
| File drop | Same code + touch fallback | Different native picker |
| Development effort | Low (responsive CSS) | High (new codebase) |
| Maintenance | Single codebase | Two codebases |

### What Gets Shared (Everything!)

- `theme.css` - All 3 themes work on mobile unchanged
- `types.ts` - TypeScript interfaces
- `SessionContext.tsx` - State management and API client
- `FilesView.tsx` - File drop/upload (add touch-friendly button)
- All React components - Just add responsive breakpoints

---

## Core Design Philosophy

### Gastown Principles Embodied in Mobile UI

| Gastown Principle | Mobile Implementation |
|-------------------|----------------------|
| **Physics over Politeness** | No confirmation dialogs for routine actions; swipe to nudge, tap to refresh; haptic feedback for immediate confirmation |
| **Nondeterministic but Idempotent (NDI)** | Nudge actions are always safe to repeat; state syncs to Git/beads truth, not local cache |
| **Sessions are Ephemeral** | Sessions displayed as temporary workers; emphasis on Beads (work) over terminal output; session death/respawn shown as normal |
| **Throughput is the Mission** | Information-dense displays; aggressive pull-to-refresh; quick-action gestures |

### Mobile-Responsive Constraints

- **Responsive terminal**: Smaller font, single-window focus on mobile
- **Adaptive navigation**: Bottom nav on mobile, sidebar on desktop
- **Thumb-zone design**: Critical actions in bottom 60% of screen
- **Touch targets**: Minimum 44x44px for all interactive elements

---

## Information Architecture

```
Gastown Mobile
├── Dashboard (Home)
│   ├── Fleet Overview (aggregate stats bar)
│   ├── Active Sessions Carousel
│   └── Recent Activity Feed
│
├── Sessions
│   ├── Grouped List (HQ, Main, Gastown rigs, Other)
│   ├── Search/Filter
│   └── Session Detail → Terminal Glimpse
│
├── Beads
│   ├── Kanban (swipeable columns)
│   └── Triage (AI recommendations)
│
├── Health
│   ├── Connection Status
│   ├── Nudge Intervals
│   └── Recent Events Log
│
└── Settings
    ├── Theme Toggle (Matrix/Dark/Gastown)
    └── Notifications
```

---

## Key Screens

### 1. Dashboard (Home)

```
┌─────────────────────────────────────┐
│  GASTOWN MOBILE         [pull↓]    │
├─────────────────────────────────────┤
│  ┌─────────────────────────────┐   │
│  │  FLEET: ████████░░  12/20   │   │
│  │  ⚡3 working  🔴2 blocked   │   │
│  └─────────────────────────────┘   │
│                                     │
│  ACTIVE SESSIONS ──────────────    │
│  ┌──────┐ ┌──────┐ ┌──────┐       │
│  │gt-001│ │gt-002│ │hq-eng│ swipe→│
│  └──────┘ └──────┘ └──────┘       │
│                                     │
│  RECENT ACTIVITY ──────────────    │
│  • BUG-042 closed by gt-003        │
│  • TASK-108 → in_progress          │
│                                     │
│  ┌─────────────────────────────┐   │
│  │     [ 🔄 NUDGE ALL ]        │   │
│  └─────────────────────────────┘   │
│                                     │
├─────────────────────────────────────┤
│  🏠    📟    📋    💓    ⚙️       │
└─────────────────────────────────────┘
```

### 2. Session Detail / Terminal Glimpse

- **Read-only** terminal output (last 50 lines)
- Quick command buttons: Nudge, Pause, Refresh
- Linked Beads issues shown below
- Pinch-to-zoom for font size

### 3. Beads Kanban

- Swipeable columns (Open → In Progress → Blocked → Done)
- Tap card to view details
- Dot indicators for current column

---

## Gestures & Interactions

| Gesture | Context | Action |
|---------|---------|--------|
| Pull down | Any list | Refresh |
| Swipe right | Session row | Nudge session |
| Swipe left | Session row | Reveal Kill button |
| Tap | Session/Issue | Open detail |
| Long press | Session | Context menu sheet |
| Horizontal swipe | Kanban | Change column |

---

## Feature Matrix

### Included in Mobile

- View session list with grouping
- Session status (real-time polling)
- Create/Kill/Rename sessions
- Terminal glimpse (read-only)
- Beads Kanban and Triage views
- All 3 themes (Matrix, Dark, Gastown)
- Search and filter
- Nudge actions (primary interaction)
- Push notifications (new)

### Excluded from Mobile

| Desktop Feature | Reason |
|-----------------|--------|
| Multi-window terminal grid | Screen size |
| Drag-and-drop assignment | Touch imprecision |
| Full terminal input | Mobile keyboard UX |
| Beads Graph visualization | Complex, needs large screen |
| Beads Insights | Information density |
| File browser | Scope - use desktop |
| Music player | Desktop ambiance feature |

---

## Technical Architecture

### Frontend Stack (Same Codebase!)

**Existing React + Vite stack** with PWA additions:

```
dashboard/
├── src/
│   ├── styles/
│   │   ├── theme.css          # Add responsive breakpoints
│   │   └── mobile.css         # Mobile-specific overrides (new)
│   ├── components/
│   │   ├── MobileNav.tsx      # Bottom nav for mobile (new)
│   │   └── ResponsiveLayout.tsx # Adaptive layout wrapper (new)
│   └── hooks/
│       └── useIsMobile.ts     # Viewport detection hook (new)
├── public/
│   ├── manifest.json          # PWA manifest (new)
│   └── icons/                 # App icons for home screen (new)
└── vite.config.ts             # Add PWA plugin
```

### PWA Setup (Vite PWA Plugin)

```typescript
// vite.config.ts additions
import { VitePWA } from 'vite-plugin-pwa'

plugins: [
  VitePWA({
    registerType: 'autoUpdate',
    manifest: {
      name: 'Gastown Mobile',
      short_name: 'Gastown',
      theme_color: '#000000',
      display: 'standalone',
    }
  })
]
```

### Deployment (Same Container, Same URL)

```
┌──────────────────────────────────────────┐
│           Tailscale Network              │
├──────────────────────────────────────────┤
│                                          │
│   arena:8080 (nginx)                     │
│   ├── Desktop browsers → full layout     │
│   └── Mobile browsers → responsive PWA   │
│                                          │
│   Same React app, responsive CSS         │
│   Same API, same ttyd terminal           │
│                                          │
└──────────────────────────────────────────┘
```

No separate mobile container needed - one URL works everywhere.

---

## Responsive CSS Changes

Add to `theme.css`:

```css
/* Mobile breakpoints */
@media (max-width: 768px) {
  /* Hide desktop sidebar, show bottom nav */
  .session-panel { display: none; }
  .mobile-nav { display: flex; }

  /* Single terminal window on mobile */
  .terminal-grid { grid-template-columns: 1fr; }

  /* Touch-friendly targets */
  .session-item { min-height: 48px; padding: 12px; }
  .tab { padding: 16px 20px; }

  /* Safe areas for notched phones */
  .mobile-nav {
    padding-bottom: env(safe-area-inset-bottom);
  }
}

@media (min-width: 769px) {
  .mobile-nav { display: none; }
}
```

---

## Critical Files to Modify

| File | Changes |
|------|---------|
| `dashboard/src/styles/theme.css` | Add responsive breakpoints, mobile nav styles |
| `dashboard/src/App.tsx` | Add ResponsiveLayout wrapper, conditional MobileNav |
| `dashboard/vite.config.ts` | Add vite-plugin-pwa |
| `dashboard/src/components/FilesView.tsx` | Add prominent upload button for touch |

### New Files to Create

| File | Purpose |
|------|---------|
| `src/components/MobileNav.tsx` | Bottom navigation bar (5 tabs) |
| `src/components/MobileSessionList.tsx` | Full-screen session list for mobile |
| `src/hooks/useIsMobile.ts` | Viewport detection hook |
| `public/manifest.json` | PWA manifest with app name, icons, theme |
| `public/icons/*.png` | App icons (192x192, 512x512)

---

## Implementation Phases

### Phase 1: PWA Foundation
- Add `vite-plugin-pwa` to vite.config.ts
- Create `manifest.json` with app metadata
- Generate app icons for home screen
- Add `useIsMobile` hook for viewport detection

### Phase 2: Responsive Layout
- Add responsive breakpoints to `theme.css`
- Create `MobileNav.tsx` bottom navigation
- Modify `App.tsx` to show MobileNav on mobile
- Hide sidebar on mobile, use full-screen views

### Phase 3: Mobile Views
- Create `MobileSessionList.tsx` (full-screen, touch-friendly)
- Adapt terminal view for single-window mobile display
- Add prominent file upload button (not just drop zone)
- Optimize Beads kanban for touch swipe

### Phase 4: Polish
- Test on iOS Safari and Android Chrome
- Ensure touch targets are 44px minimum
- Add pull-to-refresh on session list
- Test "Add to Home Screen" flow

---

## Verification Plan

1. **Desktop Regression**: Ensure desktop layout unchanged at >768px
2. **Mobile Layout**: Test at 375px (iPhone SE) and 414px (iPhone 14)
3. **PWA Install**: Test "Add to Home Screen" on iOS Safari and Android Chrome
4. **Touch Interactions**: Verify 44px touch targets, swipe gestures work
5. **Theme Consistency**: All 3 themes render correctly on mobile
6. **Tailscale Access**: Confirm PWA works over Tailscale from mobile device

---

## Decisions Made

- **App Type**: PWA with responsive design (single codebase)
- **Code Sharing**: 100% - same React components, CSS, API client
- **Hosting**: Same URL (arena:8080), responsive layout adapts
- **Offline Mode**: Minimal - connection required for terminal access anyway
