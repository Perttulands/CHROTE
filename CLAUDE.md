# Claude Code Guidelines - Docker Projects

## AgentArena Dashboard

### Critical Architecture Notes

**Session Management Flow:**
```
UI (React) → nginx (/terminal/) → ttyd (port 7681) → terminal-launch.sh → tmux attach
```

**Key Environment Variable:**
- `TMUX_TMPDIR=/tmp` must be set consistently across ALL processes (API, ttyd, SSH) for tmux socket access

### Anti-Patterns to Avoid

1. **Silent Fallbacks in Scripts**
   - NEVER add `else exec bash -l` as a silent fallback when tmux commands fail
   - Always expose errors so they're visible in the terminal for debugging
   - Use `echo "Error: ..." && exit 1` instead of silently falling back
   - NEVER use `2>/dev/null` on tmux commands - expose socket/permission errors
   - NEVER use `|| true` on critical tool installations (claude-code, beads, gastown)

2. **Empty Catch Blocks in TypeScript/React**
   - NEVER use `.catch(() => {})` - always log errors with `console.warn()`
   - NEVER use `} catch {` without logging - use `} catch (e) { console.warn(...) }`
   - Even if gracefully degrading, log WHY the failure occurred

3. **Auto-Creating Sessions**
   - Don't auto-create tmux sessions when adding windows
   - Windows should initialize empty (`activeSession: null`)
   - Let users drag sessions to windows explicitly

4. **iframe Key Changes**
   - The iframe `key` prop controls when the terminal reconnects
   - Changing `activeSession` triggers iframe remount and new ttyd connection
   - This is intentional - each session switch IS a new terminal attachment

6. **Timeout-Based Font Sizing**
   - NEVER use fixed `setTimeout` delays to apply xterm font size
   - Use polling approach: poll every 50ms until `term` object is available
   - This prevents font "jerking" on session switch

5. **Session Click vs Drag**
   - Click on session → Always opens floating modal for "peek" (regardless of bound state)
   - Drag session to window → Binds session to that window (does NOT auto-activate)
   - These are intentionally separate interactions per PRD

### Terminal Resize & Fit Handling

Both `TerminalWindow.tsx` and `FloatingModal.tsx` use ResizeObserver to keep xterm.js viewport in sync:

**How it works:**
1. `triggerFit()` dispatches a `resize` event to the iframe - ttyd listens for this and calls xterm's `fit()`
2. On iframe load, `triggerFit()` is called at 100ms, 300ms, and 500ms delays to handle CSS transitions
3. ResizeObserver watches the body container and triggers fit with 100ms debounce on size changes

**Why this matters:**
- Session switching remounts the iframe (via `key={activeSession}`)
- xterm.js may calculate dimensions before container layout settles
- Without delayed fit calls, scrolling can break and content may be incorrectly positioned

### Debugging Session Issues

If sessions don't attach correctly:
1. Check ttyd is running: `ps aux | grep ttyd`
2. Check TMUX_TMPDIR: `echo $TMUX_TMPDIR` (should be `/tmp`)
3. List sessions: `TMUX_TMPDIR=/tmp tmux ls`
4. Check socket: `ls -la /tmp/tmux-*/`
5. Look at terminal-launch.sh output (errors should be visible in iframe)

### File Locations

| Component | Path |
|-----------|------|
| Dashboard React app | `AgentArena/dashboard/src/` |
| Session state | `dashboard/src/context/SessionContext.tsx` |
| Floating modal | `dashboard/src/components/FloatingModal.tsx` |
| Terminal window | `dashboard/src/components/TerminalWindow.tsx` |
| Nuke confirm modal | `dashboard/src/components/NukeConfirmModal.tsx` |
| Launch script | Embedded in `build1.dockerfile` (lines 73-109) |
| ttyd startup | `build1.dockerfile` entrypoint (line ~143) |
| nginx config | `AgentArena/nginx/nginx.conf` |
| API server | `AgentArena/api/server.js` |

### PRD Reference

See `AgentArena/PRD-SESSION-MANAGEMENT.md` for:
- Session binding rules
- Sidebar indicator specs (color dots, window badges)
- Search/filter requirements
- Window layout specifications

### Theme System

CSS variables in `dashboard/src/styles/theme.css`:
- Three themes: matrix (default), dark, gastown
- All UI must use `var(--text-primary)`, `var(--accent)`, etc.
- iframes (filebrowser, ttyd) don't inherit themes

### Music Player

Lightweight ambient music player in the tab bar (`dashboard/src/components/MusicPlayer.tsx`).

**Design principles:**
- Minimal memory footprint (2 useState, 2 useEffect, no useCallback)
- Track dropdown renders only when open (conditional rendering)
- Volume slider is always visible inline (no popup)

**Track list:**
- Files in `dashboard/public/music/`
- Tracks defined in `TRACKS` array with `{file, name}` objects

**UI behavior:**
- Track selector: Click to toggle dropdown, click track to select
- Volume: Inline slider always visible (no popup)
- Backdrop overlay closes dropdown on outside click
- Dropdown opens DOWNWARD (`top: 100%`) since player is at top of screen

### Session Drag-and-Drop Behavior

**Critical:** Dragging a session to a window adds it to `boundSessions` but does NOT change `activeSession`. This prevents disrupting the user's current terminal view.

The `addSessionToWindow` function in SessionContext.tsx:
- Removes session from any other window
- Adds to target window's `boundSessions` array
- Does NOT set `activeSession` (user must click to activate)

### Window Focus & Keyboard Navigation

- `focusedWindowIndex` tracks which window has keyboard focus
- `Ctrl+Up/Down` cycles between windows via `cycleWindow()`
- `Ctrl+Left/Right` cycles sessions within focused window via `cycleSession()`
- Focus index auto-adjusts when window count decreases

### Orphan Session Cleanup

When sessions are deleted from tmux, `refreshSessions()` automatically:
1. Detects sessions in `boundSessions` that no longer exist
2. Removes them from window bindings
3. Updates `activeSession` if the orphaned session was active

### Nuke All Sessions

The "Nuke All" feature destroys all tmux sessions at once:

**UI Flow:**
1. Red "☢ Nuke All" button appears at bottom of session sidebar (only when sessions exist)
2. Clicking opens `NukeConfirmModal` with session count warning
3. User must type "NUKE" to enable the "Destroy All" button
4. On confirm, calls `DELETE /api/tmux/sessions/all`

**API Endpoint:** `DELETE /api/tmux/sessions/all`
- Lists all current sessions
- Runs `tmux kill-server` to destroy all sessions
- Returns `{ success, killed, sessions[] }`

**Files:**
- Button & modal integration: `dashboard/src/components/SessionPanel.tsx`
- Modal component: `dashboard/src/components/NukeConfirmModal.tsx`
- API endpoint: `api/server.js` (DELETE /api/tmux/sessions/all)
- Styles: `dashboard/src/styles/theme.css` (NUKE MODAL section)
