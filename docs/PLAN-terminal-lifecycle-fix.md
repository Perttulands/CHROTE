# Plan: Fix Terminal Iframe Lifecycle (pol-8989, pol-1ccd, pol-5733)

## Context

The IframePool pre-creates iframes for ALL sessions (current workspaces + saved presets) in a hidden `div` with `width:0; height:0`. When xterm.js loads inside these zero-pixel iframes, it initializes with 2 columns x 1 row — the minimum terminal size. ttyd sends this resize to the PTY, and the tmux session obediently shrinks to 2x1.

**Evidence:** `tmux list-clients` shows 7 of 13 polis sessions at 2x1:
```
/dev/pts/11:2x1:br-compat
/dev/pts/12:2x1:br-manager
/dev/pts/13:2x1:br-validator
/dev/pts/16:2x1:master-chiron
/dev/pts/21:2x1:master-p3
/dev/pts/22:2x1:master-p4
/dev/pts/23:2x1:master-p2b
```

These are sessions whose iframes were pre-created in the pool but never claimed into a visible window. Their tmux sessions are running with 2-column terminals — any process inside them sees garbled output.

A secondary issue: when the grid layout changes (e.g., 1 window → 2x2), the CSS `transition: all 0.2s ease` on `.terminal-grid` creates a 200ms animation window of intermediate dimensions. The ResizeObserver fires during this animation, and the 100ms debounce may settle on an intermediate size rather than the final one.

**PRD:** `docs/PRD-terminal-lifecycle.md`
**Beads:** pol-8989 (critical: 2x1 pool), pol-1ccd (grid resize), pol-5733 (session crash on cd ..)

---

## Step 1: Remove CSS grid transition

**File:** `dashboard/src/styles/terminal.css`

**What:** Remove `transition: all 0.2s ease;` from `.terminal-grid` (line 56). This eliminates the 200ms animation window where intermediate dimensions can confuse xterm.fit().

**Why:** The grid transition is purely cosmetic. Without it, layout changes are instant — the ResizeObserver fires once at the final size. The 100ms debounce in TerminalWindow's ResizeObserver then works correctly because there's only one size change to debounce, not a continuous stream of intermediate values.

**Also:** Narrow the `.terminal-window` transition (line 88) from `transition: all 0.2s ease` to `transition: border-color 0.2s ease, box-shadow 0.2s ease;`. This preserves the hover/focus visual effects but prevents the window element itself from transitioning its width/height during grid changes.

**Risk:** Layout changes feel slightly more abrupt. This is acceptable — terminal correctness matters more than animation smoothness.

---

## Step 2: Deferred iframe connection (core fix)

**File:** `dashboard/src/components/IframePool.tsx`

**What:** Don't set `iframe.src` when the iframe is created. Only set it when the iframe is first claimed into a visible container.

**Current behavior (broken):**
```
iframe created → src set immediately → xterm loads in 0x0 pool → 2x1 resize → tmux corrupted
```

**New behavior:**
```
iframe created (no src) → sits empty in pool → claimed into visible window → src set → xterm loads at correct size
```

**Detailed changes:**

1. Add a tracking ref near line 66:
   ```typescript
   const connectedRef = useRef<Set<string>>(new Set())
   ```

2. In the iframe creation effect (lines 98-129), remove line 107:
   ```typescript
   // REMOVE: iframe.src = getTerminalUrl(sessionName)
   ```
   The iframe is created as a DOM element with styles but no src. It sits in the pool as an empty shell.

3. In `claimIframe` (lines 156-178), set src on first claim:
   ```typescript
   const claimIframe = useCallback((sessionName: string, container: HTMLElement): (() => void) => {
     claimsRef.current.set(sessionName, container)
     const iframe = iframeRefs.current.get(sessionName)
     if (iframe) {
       container.appendChild(iframe)
       iframe.style.position = ''
       iframe.style.visibility = ''
       // Deferred connection: set src only on first claim
       if (!connectedRef.current.has(sessionName)) {
         connectedRef.current.add(sessionName)
         iframe.src = getTerminalUrl(sessionName)
       }
     }
     return () => {
       claimsRef.current.delete(sessionName)
       const iframe = iframeRefs.current.get(sessionName)
       const pool = poolContainerRef.current
       if (iframe && pool) {
         iframe.style.position = 'absolute'
         iframe.style.visibility = 'hidden'
         pool.appendChild(iframe)
       }
     }
   }, [])
   ```

4. In the cleanup effect (lines 74-96), when removing iframes for sessions no longer needed, also clean up the connected tracking:
   ```typescript
   connectedRef.current.delete(sessionName)
   ```

**What this preserves:**
- Iframe DOM elements are still pre-created for all sessions (instant switching for previously-viewed sessions)
- Once connected, releasing back to pool keeps the WebSocket alive (no reload on re-claim)
- The `isLoaded` tracking still works — it fires on the iframe's `load` event, which only happens after src is set

**What this changes:**
- First time viewing a preset-only session has a ~1-2s load delay (iframe must connect to ttyd)
- This is acceptable because the alternative is a 2x1 corrupted session

---

## Step 3: Pool container with minimum dimensions

**File:** `dashboard/src/components/IframePool.tsx`

**What:** Replace the 0x0 pool container with a fixed-position offscreen container that has real dimensions (400x300).

**Current (broken):**
```tsx
<div ref={poolContainerRef}
  style={{ position: 'absolute', width: 0, height: 0, overflow: 'hidden', pointerEvents: 'none' }}
/>
```

**New:**
```tsx
<div ref={poolContainerRef}
  style={{
    position: 'fixed',
    left: '-9999px',
    top: '-9999px',
    width: '400px',
    height: '300px',
    overflow: 'hidden',
    pointerEvents: 'none',
    visibility: 'hidden',
  }}
/>
```

**Why:** When an iframe is released back to the pool (e.g., preset switch), its WebSocket is still alive. If xterm.js's internal ResizeObserver fires on the iframe, it will see 400x300 and calculate ~50 cols x 15 rows — a reasonable fallback instead of 2x1. The session remains usable when re-claimed.

**Why `position: fixed` + offscreen instead of `width: 0`:** Some browsers report 0x0 to children even if the child has explicit dimensions, when the parent uses `overflow: hidden` with 0 size. Offscreen positioning with real dimensions is more reliable.

**Why `visibility: hidden`:** Prevents rendering overhead while keeping layout dimensions intact for iframe contents.

---

## Step 4: Aggressive staggered fit on active session change

**File:** `dashboard/src/components/TerminalWindow.tsx`

**What:** Replace the single 50ms fit attempt with staggered attempts at 50ms, 200ms, and 500ms.

**Current (lines 195-201):**
```typescript
useEffect(() => {
  if (activeSession && pool.isLoaded(activeSession)) {
    setTimeout(() => pool.triggerFit(activeSession), 50)
  }
}, [activeSession])
```

**New:**
```typescript
useEffect(() => {
  if (!activeSession) return
  const timeouts: ReturnType<typeof setTimeout>[] = []
  const fitIfLoaded = () => {
    if (pool.isLoaded(activeSession)) pool.triggerFit(activeSession)
  }
  timeouts.push(setTimeout(fitIfLoaded, 50))
  timeouts.push(setTimeout(fitIfLoaded, 200))
  timeouts.push(setTimeout(fitIfLoaded, 500))
  return () => timeouts.forEach(t => clearTimeout(t))
}, [activeSession])
```

**Why staggered:** The iframe layout may not have settled at 50ms, especially on first claim when the iframe is loading. The 500ms attempt is the authoritative one — by then, xterm.js is fully initialized and the container has its final dimensions.

**Precedent:** `FloatingModal.tsx` (lines 62-64) already uses this exact pattern with 100ms/300ms/500ms delays.

---

## Step 5: Fit trigger after visibility effect

**File:** `dashboard/src/components/TerminalWindow.tsx`

**What:** Add a fit trigger at the end of the visibility effect (lines 182-193).

**Current:** The visibility effect sets `display`, `position`, `width`, `height` on iframes based on which is active, but doesn't trigger a fit.

**New:** Append after the forEach loop:
```typescript
if (activeSession && pool.isLoaded(activeSession)) {
  setTimeout(() => pool.triggerFit(activeSession), 50)
}
```

**Why:** When a session becomes active (user clicks its tag), the iframe transitions from `display:none` to `display:block`. The iframe now has visible dimensions but xterm hasn't been told to re-fit. Without this trigger, the terminal keeps its old col/row count until the next window resize.

---

## Step 6: Dimension guard in triggerFit

**File:** `dashboard/src/components/IframePool.tsx`

**What:** Add a guard in `triggerFit` to skip iframes with no meaningful dimensions.

**Current (lines 194-201):**
```typescript
const triggerFit = useCallback((sessionName: string) => {
  try {
    const iframe = iframeRefs.current.get(sessionName)
    if (iframe?.contentWindow) {
      iframe.contentWindow.dispatchEvent(new Event('resize'))
    }
  } catch { /* cross-origin */ }
}, [])
```

**New:**
```typescript
const triggerFit = useCallback((sessionName: string) => {
  try {
    const iframe = iframeRefs.current.get(sessionName)
    if (!iframe?.contentWindow) return
    // Don't trigger fit if iframe has no meaningful dimensions
    if (iframe.offsetWidth < 10 || iframe.offsetHeight < 10) return
    iframe.contentWindow.dispatchEvent(new Event('resize'))
  } catch { /* cross-origin */ }
}, [])
```

**Why:** This is a safety net. Even if some code path manages to call triggerFit on an iframe that's in the pool or has `display:none`, it won't send a 2x1 resize to tmux. The `< 10` threshold avoids false negatives from subpixel rendering.

---

## Files Modified Summary

| File | Change | Risk |
|------|--------|------|
| `dashboard/src/styles/terminal.css` | Remove grid transition, narrow window transition | Low — cosmetic only |
| `dashboard/src/components/IframePool.tsx` | Deferred src, pool container size, triggerFit guard | Medium — core behavior change |
| `dashboard/src/components/TerminalWindow.tsx` | Staggered fit, visibility-change fit | Low — additive changes |

No backend (Go) changes. No terminal-launch.sh changes. No tmux configuration changes.

---

## Test Strategy

### Unit Tests (IframePool.test.tsx)

1. **Deferred connection test:** After rendering the provider with sessions, verify iframes exist but have no `src` attribute set
2. **Connection on claim test:** After calling `claimIframe`, verify the iframe's `src` is set to the terminal URL
3. **Connection retained on release test:** After claiming and releasing, verify `src` is still set (connection preserved)

### Integration Tests (iframe-pool.spec.ts)

1. **No 2x1 dimensions:** Create a session, wait for iframe load, access xterm instance via `iframe.contentFrame()`, verify `cols > 10 && rows > 5`
2. **Resize on layout change:** Start with 1 window, record dimensions, switch to 4 windows, wait 600ms, verify dimensions changed (should be roughly half width)
3. **Existing tests pass:** The iframe persistence and tab-switch tests should continue passing without modification

### Manual Verification

```bash
# After deploying changes, check tmux clients:
TMUX_TMPDIR=/home/polis/.tmux-socket tmux list-clients -F "#{client_width}x#{client_height}:#{client_session}"
# Expected: NO 2x1 entries

# Test grid resize:
# 1. Open dashboard at localhost:5173
# 2. Create a session in window 1
# 3. Switch to Layout: 4
# 4. Type in the terminal — text should wrap correctly at the visible width
# 5. Run `stty size` — should show reasonable rows/cols matching the visible area

# Test session creation:
# 1. Create new session via + button
# 2. Run `cd ..` — session should NOT crash
# 3. Run `stty size` — should show reasonable dimensions
```

---

## Edge Cases

| Scenario | Expected Behavior |
|----------|------------------|
| **Preset switch** | New claims set src; released iframes keep connection in 400x300 pool |
| **Tab switch (terminal1 ↔ terminal2)** | Both TerminalAreas stay mounted (CSS visibility). ResizeObserver fires on re-show. Staggered fit corrects dimensions. |
| **Tab switch (terminal → files → terminal)** | Same as above. Existing integration test covers this. |
| **Page reload** | Fresh start. Iframes created without src. Src set on first claim into visible window. Loads at correct size. |
| **Rapid layout changes (1→2→4)** | No CSS transition → instant size changes. ResizeObserver fires per change, 100ms debounce collapses rapid changes. |
| **Session in preset but never viewed** | Iframe exists as empty DOM element (no src). No ttyd connection. No 2x1 corruption. Connects on first view. |
| **Mobile (grid forced to 1)** | Only 1 window visible. Other windows have `display:none`. triggerFit guard prevents fitting invisible iframes. |

---

## Migration

The 7 sessions currently at 2x1 will self-heal:
- When the user views them in a window, the staggered fit (Step 4) sends correct dimensions
- No manual intervention needed
- No need to restart tmux or kill sessions (GOLDEN RULE respected)
