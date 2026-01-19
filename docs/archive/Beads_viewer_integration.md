# Beads Viewer Integration

## Status: IMPLEMENTED

The Beads tab is now integrated into the AgentArena dashboard.

## Requirements

**CRITICAL: The `bv` CLI must be installed.** There are NO fallbacks or mocks. If `bv` is not installed, you will see clear error messages.

Install bv:
```bash
go install github.com/Dicklesworthstone/beads_viewer@latest
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Dashboard (React)                         │
│  ┌─────────┬─────────┬──────────┬──────────┬──────────┐    │
│  │Terminal │ Files   │  Beads   │ Settings │  Help    │    │
│  └─────────┴─────────┴──────────┴──────────┴──────────┘    │
│                           │                                  │
│            ┌──────────────┴──────────────┐                  │
│            │       BeadsView.tsx          │                  │
│            │  ┌────────┬────────┬───────┐ │                  │
│            │  │ Kanban │ Triage │Insights│ │ (sub-tabs)      │
│            │  └────────┴────────┴───────┘ │                  │
│            └──────────────────────────────┘                  │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    API (Express)                             │
│  /api/beads/health     - check bv installation (REQUIRED)   │
│  /api/beads/projects   - discover projects with .beads       │
│  /api/beads/issues     - list issues from .beads/issues.jsonl│
│  /api/beads/triage     - bv --robot-triage (REQUIRES bv)    │
│  /api/beads/insights   - bv --robot-insights (REQUIRES bv)  │
│  /api/beads/graph      - bv --robot-graph (REQUIRES bv)     │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    bv CLI (Go binary)                        │
│  Located at: /usr/local/bin/bv                               │
│  Data: /code/<project>/.beads/issues.jsonl                   │
└─────────────────────────────────────────────────────────────┘
```

## API Endpoints

| Endpoint | Method | Description | Requires bv |
|----------|--------|-------------|-------------|
| `/api/beads/health` | GET | Check if bv is installed | Yes (returns error if not) |
| `/api/beads/projects` | GET | Find projects with `.beads` dirs | No |
| `/api/beads/issues` | GET | List issues for a project | No (reads JSONL directly) |
| `/api/beads/triage` | GET | Get triage recommendations | **Yes** |
| `/api/beads/insights` | GET | Get graph metrics | **Yes** |
| `/api/beads/graph` | GET | Get dependency graph | **Yes** |

**Query params:**
- `path` - project path (required for all except `/health` and `/projects`)

**Security:**
- Paths validated against `ALLOWED_ROOTS` (`/code`, `/workspace`)
- Uses `execFileSync` to avoid shell injection
- Strict JSONL parsing with line-by-line error reporting

## Error Codes

The API returns clear, actionable error messages:

| Code | HTTP | Description |
|------|------|-------------|
| `BAD_REQUEST` | 400 | Missing required parameter |
| `FORBIDDEN` | 403 | Path outside allowed roots |
| `NOT_FOUND` | 404 | Project/file/directory not found |
| `INVALID_JSONL` | 422 | Malformed JSONL file with line numbers |
| `BV_NOT_INSTALLED` | 503 | bv command not found |
| `BV_TIMEOUT` | 504 | bv command timed out |
| `BV_ERROR` | 502 | bv command failed |
| `BV_INVALID_OUTPUT` | 502 | bv returned invalid JSON |

## Files

**Backend:**
- `api/beads-routes.js` - Express router with all endpoints
- `api/server.js` - Mounts beads routes at `/api/beads`

**Frontend:**
```
dashboard/src/components/BeadsView/
├── index.tsx           # Main container with sub-tabs
├── ProjectSelector.tsx # Dropdown to select project
├── KanbanView.tsx      # Kanban board by status
├── TriageView.tsx      # Triage recommendations
├── InsightsView.tsx    # Metrics and health dashboard
├── IssueCard.tsx       # Reusable issue card component
├── types.ts            # TypeScript interfaces
└── hooks.ts            # useBeadsData, useProjects hooks
```

**Modified:**
- `dashboard/src/App.tsx` - Added 'beads' tab
- `dashboard/src/components/TabBar.tsx` - Added Beads tab button
- `dashboard/src/styles/theme.css` - Added beads styles for all themes

## Data Structures

### Issue (from `.beads/issues.jsonl`)

```typescript
interface BeadsIssue {
  id: string
  title: string
  status: 'open' | 'in_progress' | 'blocked' | 'closed' | 'ready' | 'wont_fix' | 'duplicate' | 'deferred'
  priority?: number  // 1 = highest
  type?: 'bug' | 'feature' | 'task' | 'chore' | 'epic'
  dependencies?: string[]  // issue IDs this blocks
  labels?: string[]
  assignee?: string
  created?: string
  updated?: string
}
```

### Triage Response (from `bv --robot-triage`)

```typescript
interface TriageResponse {
  recommendations: {
    issueId: string
    rank: number
    reasoning: string
    estimatedImpact: 'high' | 'medium' | 'low'
  }[]
  quickWins: string[]   // issue IDs
  blockers: string[]    // issue IDs blocking others
  data_hash?: string
}
```

### Insights Response (from `bv --robot-insights`)

```typescript
interface InsightsResponse {
  issueCount: number
  openCount: number
  blockedCount: number
  health: {
    score: number  // 0-100
    risks: string[]
    warnings: string[]
  }
  metrics?: {
    pageRank?: Record<string, number>
    betweenness?: Record<string, number>
    degree?: Record<string, number>
    cycles?: string[][]  // circular dependencies
    density?: number
    criticalPath?: string[]
  }
  data_hash?: string
}
```

## Usage

1. Ensure `bv` is installed in the container
2. Create a `.beads` directory in your project under `/code` or `/workspace`
3. Add issues with `bv add` or create `issues.jsonl` manually
4. Open the Beads tab in the dashboard
5. Select your project from the dropdown
6. Use Kanban, Triage, or Insights sub-tabs to view data

## No Fallbacks Policy

This implementation has **NO FALLBACKS**. If something is wrong, you get a clear error:

- **bv not installed**: `503 BV_NOT_INSTALLED: bv command not found. Install beads_viewer: go install github.com/Dicklesworthstone/beads_viewer@latest`
- **Invalid JSONL**: `422 INVALID_JSONL: JSONL parse errors in /path/to/issues.jsonl: Line 5: Unexpected token...`
- **Path not allowed**: `403 FORBIDDEN: Project path not in allowed roots: /etc. Allowed: /code, /workspace`
- **Missing .beads**: `404 NOT_FOUND: No .beads directory found in /code/myproject. Run 'bv init' to create one.`

This makes debugging straightforward - errors tell you exactly what's wrong and how to fix it.
