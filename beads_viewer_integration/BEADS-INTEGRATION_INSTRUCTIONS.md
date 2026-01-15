# Beads Module Integration Instructions

This document provides step-by-step instructions to integrate the beads_viewer module into AgentArena.

## Prerequisites

1. **bv (beads_viewer) binary** - Must be installed in the container
2. **Node.js** - Already present in AgentArena
3. **React 18+** - Already present in AgentArena dashboard

## Integration Steps

### Step 1: Install bv Binary in Docker

Add to your `Dockerfile`:

```dockerfile
# Install beads_viewer (bv)
RUN curl -fsSL https://raw.githubusercontent.com/Dicklesworthstone/beads_viewer/main/install.sh | bash \
    || echo "bv installation failed - will use fallback API"
```

Or build from source (requires Go 1.21+):

```dockerfile
# Build beads_viewer from source
RUN go install github.com/Dicklesworthstone/beads_viewer/cmd/bv@latest
```

---

### Step 2: Add Backend API Routes

**File:** `api/server.js`

Add these lines after the existing route definitions:

```javascript
// ============================================================================
// BEADS MODULE INTEGRATION - START
// Add this block to integrate beads_viewer API routes
// To remove: Delete this block and api/beads-routes.js
// ============================================================================
const beadsRoutes = require('./beads-routes');
app.use('/api/beads', beadsRoutes);
// ============================================================================
// BEADS MODULE INTEGRATION - END
// ============================================================================
```

---

### Step 3: Update App.tsx

**File:** `dashboard/src/App.tsx`

**Add import at the top:**
```typescript
// BEADS MODULE INTEGRATION - START
import { BeadsTab } from './beads_module'
// BEADS MODULE INTEGRATION - END
```

**Update Tab type:**
```typescript
// Change from:
type Tab = 'terminal' | 'files' | 'status' | 'settings'

// To:
type Tab = 'terminal' | 'files' | 'status' | 'settings' | 'beads'
```

**Add render condition (inside DashboardContent, after existing tab conditions):**
```typescript
{activeTab === 'terminal' && (
  <>
    <SessionPanel />
    <TerminalArea />
  </>
)}
{activeTab === 'files' && <FilesView />}
{activeTab === 'status' && <StatusView />}
{activeTab === 'settings' && <SettingsView />}
{/* BEADS MODULE INTEGRATION - START */}
{activeTab === 'beads' && <BeadsTab />}
{/* BEADS MODULE INTEGRATION - END */}
```

---

### Step 4: Update TabBar.tsx

**File:** `dashboard/src/components/TabBar.tsx`

**Update the tabs array:**

```typescript
const tabs: TabConfig[] = [
  { id: 'terminal', label: 'Terminal' },
  { id: 'files', label: 'Files' },
  { id: 'beads', label: 'Beads' },  // BEADS MODULE INTEGRATION - ADD THIS
  { id: 'status', label: 'Status' },
  { id: 'settings', label: 'Settings' },
  // Remove or keep the external link as desired:
  // { id: 'beads-ext', label: 'Beads ↗', external: true, url: 'https://github.com/Dicklesworthstone/beads_viewer' },
]
```

---

### Step 5: Import CSS Styles

**Option A: Import in theme.css (Recommended)**

**File:** `dashboard/src/styles/theme.css`

Add at the top of the file:
```css
/* BEADS MODULE INTEGRATION - START */
@import '../beads_module/beads.css';
/* BEADS MODULE INTEGRATION - END */
```

**Option B: Import in BeadsTab.tsx**

The CSS import can also be added directly in `BeadsTab.tsx`:
```typescript
import '../beads.css';
```

---

### Step 6: Verify Installation

1. **Rebuild the frontend:**
   ```bash
   cd dashboard && npm run build
   ```

2. **Restart the application**

3. **Navigate to the Beads tab** - You should see the sub-navigation with Graph, Kanban, Triage, and Insights views

4. **Test with a .beads project:**
   - Create a test `.beads/issues.jsonl` file in `/workspace`
   - Or point to an existing beads project

---

## Configuration

### Environment Variables

The beads API routes support these environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `BEADS_PROJECT_PATH` | `/workspace` | Default project path |
| `BV_COMMAND` | `bv` | Path to bv binary |

### API Endpoints

After integration, these endpoints are available:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/beads/health` | GET | Health check |
| `/api/beads/issues` | GET | Fetch all issues |
| `/api/beads/triage` | GET | AI triage recommendations |
| `/api/beads/insights` | GET | Graph metrics |
| `/api/beads/plan` | GET | Execution plan |
| `/api/beads/projects` | GET | Discover beads projects |

Query parameter: `?path=/path/to/project`

---

## Fallback Mode

If the `bv` binary is not installed, the API will:

1. **For `/api/beads/issues`**: Read directly from `.beads/issues.jsonl`
2. **For `/api/beads/triage`**: Generate basic recommendations from issues
3. **For `/api/beads/insights`**: Calculate basic metrics from issues
4. **For `/api/beads/plan`**: Return empty plan

This allows the module to function (with reduced features) even without bv installed.

---

## Troubleshooting

### "bv command not found"

The bv binary is not installed. Install it or rely on fallback mode.

### "No .beads directory found"

The specified project path doesn't have a `.beads` folder. Create one or specify a different path.

### "Failed to parse issues"

The `issues.jsonl` file has invalid JSON. Each line must be a valid JSON object.

### TypeScript Errors

If you see TypeScript errors after integration:

1. Check that imports are correct
2. Check that the Tab type includes 'beads'
3. Run `npm run type-check` for specific errors

### Styling Issues

If styles don't appear:

1. Verify the CSS import path is correct
2. Check browser console for CSS loading errors
3. Try hard-refreshing the page (Ctrl+Shift+R)

---

## Module Architecture

```
dashboard/src/beads_module/
├── index.ts              # Main entry point, exports everything
├── types.ts              # TypeScript types and constants
├── api.ts                # API client functions
├── context.tsx           # React context for state management
├── beads.css             # All styles for the module
└── components/
    ├── index.ts          # Component barrel exports
    ├── BeadsTab.tsx      # Main tab container
    ├── BeadsGraphView.tsx    # Dependency graph visualization
    ├── BeadsKanbanView.tsx   # Kanban board
    ├── BeadsTriageView.tsx   # AI triage recommendations
    └── BeadsInsightsView.tsx # Graph metrics and health

api/
└── beads-routes.js       # Express routes for beads API
```

---

## What's Included

### Features

- **Graph View**: Interactive canvas-based dependency graph with force-directed layout
- **Kanban Board**: Issues organized by status with drag-and-drop support
- **Triage View**: AI-powered recommendations (uses bv --robot-triage or fallback)
- **Insights View**: Graph metrics, health score, critical path, cycle detection

### Not Included

- Direct issue editing (read-only view)
- Real-time sync with bv TUI
- WebSocket updates
- Issue creation/deletion

These features would require additional development.

---

## Next Steps

After basic integration:

1. **Add project discovery**: Auto-populate project selector with found .beads directories
2. **Add real-time updates**: Poll for changes or implement WebSocket
3. **Add issue editing**: Allow status changes from the UI
4. **Add window mode**: Allow beads views to replace tmux sessions in terminal windows
