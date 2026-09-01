# PRD: Terminal Iframe Lifecycle & Session Resize

**Status:** Historical. Both mechanisms this document specifies are gone. The
iframe pool was replaced by an in-page terminal, and ttyd by a pseudo-terminal
the Go server owns, under
[ADR-0018](adr/0018-terminal-transport-ownership.md); window sizing is settled
by [ADR-0017](adr/0017-terminal-viewing-model.md). Kept as a record of the
problem, not as requirements.
**Author:** polis + claude
**Date:** 2026-03-16

---

## Overview

CHROTE's terminal system uses an IframePool to persist terminal connections across tab switches and preset loads. Currently, the pool pre-creates iframes in a hidden 0x0 container, causing xterm.js to initialize with 2x1 terminal dimensions. This silently resizes tmux sessions to 2 columns x 1 row, breaking running processes and causing garbled input when users interact with these sessions. Additionally, when grid layouts change (e.g., 1 window to 2x2), the resize signal to xterm may not reliably propagate, leaving terminals at stale dimensions.

## Goals

1. **Terminal dimensions always match the visible container** - when an iframe is displayed, the terminal's cols/rows correspond to its actual pixel dimensions within 500ms.
2. **Pre-created iframes never corrupt tmux session dimensions** - sessions in the pool must not attach 2x1 clients to tmux.
3. **Grid layout changes reliably resize all visible terminals** - switching from grid-1 to grid-4 resizes all 4 terminals, not just the previously-active one.
4. **Session creation is atomic and robust** - creating a session, connecting an iframe, and achieving correct dimensions happens as a reliable unit of work.

## Users & Use Cases

**Primary users:** Human operator viewing the CHROTE dashboard

| Use Case | Actor | Scenario | Expected Outcome |
|----------|-------|----------|-----------------|
| UC-1 | Operator | Switches from 1-window to 4-window grid | All 4 terminals resize to ~half-width, ~half-height. Text wraps correctly. Input works. |
| UC-2 | Operator | Loads a preset with 4 sessions they haven't viewed yet | Sessions connect and display at correct dimensions. No 2x1 corruption. |
| UC-3 | Operator | Creates a new session via "New Session" button | Session appears with correct dimensions. `cd ..` and normal commands work immediately. |
| UC-4 | Operator | Switches tabs (Terminal -> Files -> Terminal) | Terminal returns at same dimensions, no resize flash or corruption. |
| UC-5 | Operator | Has sessions in presets but not currently displayed | Those tmux sessions are NOT attached with 2x1 clients. They retain their natural dimensions. |

## Requirements

### Functional

- **REQ-1: Deferred connection** — Iframes for sessions that are NOT currently displayed in any visible window MUST NOT connect to ttyd. The iframe element may exist in the DOM, but `src` must not be set until the iframe is claimed into a visible container. This prevents 2x1 client attachments to tmux.
- **REQ-2: Connect on first claim** — When an iframe is claimed into a visible window for the first time, set its `src` to the terminal URL. The iframe loads and connects to ttyd.
- **REQ-3: Keep connection on release** — When a claimed iframe is released back to the pool (e.g., preset switch), maintain the ttyd WebSocket connection. The iframe stays loaded but is hidden. This enables instant re-claim without reload.
- **REQ-4: Minimum pool container size** — The hidden pool container must have minimum dimensions (e.g., 200x150px) so that released iframes don't trigger a resize to 2x1. The pool container is visually hidden (`visibility: hidden; overflow: hidden`) but has enough size that xterm.fit() calculates reasonable dimensions (~25 cols, ~10 rows).
- **REQ-5: Aggressive fit on claim** — When an iframe is claimed into a visible container, trigger `fit()` at multiple intervals (50ms, 200ms, 500ms after claim) to handle layout settling. The last fit at 500ms is the authoritative one.
- **REQ-6: Fit on visibility change** — When a TerminalWindow transitions from `display:none` to `display:flex` (grid layout expansion), its ResizeObserver fires and triggers `fit()` for all bound sessions in that window, not just the active one.
- **REQ-7: Fit verification** — After calling `fit()`, verify the terminal dimensions changed by comparing pre/post cols/rows. If dimensions didn't change (stale), retry after 100ms.
- **REQ-8: No CSS transition on grid** — Remove `transition: all 0.2s ease` from `.terminal-grid` to eliminate the animation window where intermediate dimensions could cause incorrect fit calculations. Grid changes should be instant.

### Non-Functional

- **NFR-1: Resize latency** — Terminal dimensions must reach their final correct value within 500ms of a grid layout change.
- **NFR-2: No SIGWINCH storm** — Debounce resize events so tmux receives at most 2-3 SIGWINCH signals per layout change, not dozens.
- **NFR-3: Golden rule compliance** — No change may disrupt a running shell or tmux session. Specifically, releasing an iframe to the pool must not crash the session's shell.

## Success Metrics

| Metric | Baseline | Target | How Measured |
|--------|----------|--------|-------------|
| Sessions with 2x1 clients | 7 of 13 (54%) | 0 | `tmux list-clients` shows no 2x1 entries |
| Resize correctness after grid change | Inconsistent | 100% of visible terminals resize within 500ms | Playwright test: change grid, verify iframe dimensions match container |
| Session crash on `cd ..` | Reproducible | 0 occurrences | Manual test + Playwright: create session, cd .., verify session alive |
| Learning: iframe lifecycle | Unclear | Documented in tests | Unit + integration tests cover claim/release/resize cycle |

## Risks & Dependencies

| Risk | Impact | Likelihood | Mitigation |
|------|--------|-----------|------------|
| Deferred src breaks instant preset switching | Medium - preset load takes 1-2s instead of instant | Medium | Only defer for sessions not in current workspace. Previously-claimed sessions keep their connections. |
| Pool container min-size causes layout shifts | Low | Low | Container uses `visibility:hidden` + `position:fixed` offscreen, won't affect layout |
| Removing grid transition feels jarring | Low | Medium | Test without transition first. Can add back a very short transition (50ms) if needed. |
| Multiple fit() calls cause SIGWINCH storm | Medium | Low | Debounce in triggerFit already handles this. Multiple calls to triggerFit within 100ms collapse to one. |

**Dependencies:**
- ttyd v1.7.7 client-side code listens for `window` resize events and calls `fitAddon.fit()` (verified)
- xterm.js FitAddon correctly measures parent element dimensions (standard behavior)
- tmux `aggressive-resize off` (current default, verified)
