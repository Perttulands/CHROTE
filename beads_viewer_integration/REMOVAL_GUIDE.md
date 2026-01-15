# Beads Module Removal Guide

This document provides step-by-step instructions to completely remove the beads_viewer integration from AgentArena.

## Overview

The beads module was designed as a self-contained, removable feature. All module code is isolated in dedicated folders, with minimal integration points in the main application.

## Quick Removal (TL;DR)

```bash
# 1. Remove frontend module
rm -rf dashboard/src/beads_module

# 2. Remove backend API routes
rm api/beads-routes.js

# 3. Revert App.tsx changes (see detailed instructions below)

# 4. Revert TabBar.tsx changes (see detailed instructions below)

# 5. Revert server.js changes (see detailed instructions below)

# 6. Revert theme.css import (if added)
```

---

## Detailed Removal Instructions

### Step 1: Remove the Frontend Module

Delete the entire beads_module folder:

```bash
rm -rf dashboard/src/beads_module
```

**Files being removed:**
- `dashboard/src/beads_module/index.ts`
- `dashboard/src/beads_module/types.ts`
- `dashboard/src/beads_module/api.ts`
- `dashboard/src/beads_module/context.tsx`
- `dashboard/src/beads_module/beads.css`
- `dashboard/src/beads_module/components/index.ts`
- `dashboard/src/beads_module/components/BeadsTab.tsx`
- `dashboard/src/beads_module/components/BeadsGraphView.tsx`
- `dashboard/src/beads_module/components/BeadsKanbanView.tsx`
- `dashboard/src/beads_module/components/BeadsTriageView.tsx`
- `dashboard/src/beads_module/components/BeadsInsightsView.tsx`

---

### Step 2: Remove Backend API Routes

Delete the beads routes file:

```bash
rm api/beads-routes.js
```

---

### Step 3: Revert App.tsx Changes

**File:** `dashboard/src/App.tsx`

**Remove this import:**
```typescript
// BEADS MODULE INTEGRATION - REMOVE THIS LINE
import { BeadsTab } from './beads_module'
```

**Remove from Tab type (if modified):**
```typescript
// Change from:
type Tab = 'terminal' | 'files' | 'status' | 'settings' | 'beads'

// To:
type Tab = 'terminal' | 'files' | 'status' | 'settings'
```

**Remove from render section:**
```typescript
// REMOVE THIS BLOCK:
{activeTab === 'beads' && <BeadsTab />}
```

---

### Step 4: Revert TabBar.tsx Changes

**File:** `dashboard/src/components/TabBar.tsx`

**Remove the beads tab from the tabs array:**
```typescript
// Remove this entry from the tabs array:
{ id: 'beads', label: 'Beads' },
```

The original TabBar.tsx should have this tabs array:
```typescript
const tabs: TabConfig[] = [
  { id: 'terminal', label: 'Terminal' },
  { id: 'files', label: 'Files' },
  { id: 'status', label: 'Status' },
  { id: 'settings', label: 'Settings' },
  { id: 'beads', label: 'Beads ↗', external: true, url: 'https://github.com/Dicklesworthstone/beads_viewer' },
]
```

Note: The external link to beads_viewer was already present. You can keep or remove it as desired.

---

### Step 5: Revert server.js Changes

**File:** `api/server.js`

**Remove these lines (if added):**
```javascript
// BEADS MODULE INTEGRATION - REMOVE THESE LINES
const beadsRoutes = require('./beads-routes');
app.use('/api/beads', beadsRoutes);
```

---

### Step 6: Revert CSS Import (if added)

**File:** `dashboard/src/styles/theme.css`

**Remove this import (if present at the top of the file):**
```css
/* BEADS MODULE INTEGRATION - REMOVE THIS LINE */
@import '../beads_module/beads.css';
```

---

### Step 7: Remove Documentation (Optional)

Delete the integration documentation:

```bash
rm -rf beads_viewer_integration/
```

---

## Verification

After removal, verify the application still works:

1. **Build the frontend:**
   ```bash
   cd dashboard && npm run build
   ```

2. **Check for TypeScript errors:**
   ```bash
   cd dashboard && npm run type-check
   ```

3. **Start the application:**
   ```bash
   # Your usual start command
   ```

4. **Verify tabs work correctly** - Terminal, Files, Status, Settings should all function normally.

---

## Files Modified During Integration

This is the complete list of files that were modified to integrate the beads module:

### Files Added (delete these)
```
dashboard/src/beads_module/           # Entire folder
api/beads-routes.js                   # API routes file
beads_viewer_integration/             # Documentation folder (optional)
```

### Files Modified (revert these)

| File | Changes Made |
|------|--------------|
| `dashboard/src/App.tsx` | Added import, Tab type, conditional render |
| `dashboard/src/components/TabBar.tsx` | Added 'beads' tab to tabs array |
| `api/server.js` | Added require and app.use for beads routes |
| `dashboard/src/styles/theme.css` | Added @import for beads.css (optional) |

---

## Rollback Commit

If you're using git, you can create a rollback script:

```bash
#!/bin/bash
# rollback-beads-module.sh

# Remove module files
rm -rf dashboard/src/beads_module
rm -f api/beads-routes.js
rm -rf beads_viewer_integration

# Git commands to revert file changes
git checkout HEAD -- dashboard/src/App.tsx
git checkout HEAD -- dashboard/src/components/TabBar.tsx
git checkout HEAD -- api/server.js
git checkout HEAD -- dashboard/src/styles/theme.css

echo "Beads module removed successfully"
```

---

## Troubleshooting

### Build Errors After Removal

If you see errors about missing imports:

1. Check that all imports referencing `beads_module` are removed
2. Check that the Tab type doesn't include 'beads'
3. Run `npm run build` to see specific errors

### Runtime Errors

If the app crashes after removal:

1. Check browser console for errors
2. Verify no components are trying to render `BeadsTab`
3. Clear browser cache and localStorage

### API Errors

If you see 404 errors for `/api/beads/*`:

1. This is expected - the routes were removed
2. If something is still calling these endpoints, check for leftover code

---

## Support

If you encounter issues during removal, check:

1. The git diff of your changes
2. TypeScript compilation errors
3. Browser console errors

The module was designed for clean removal, so any issues likely indicate incomplete removal of integration points.
