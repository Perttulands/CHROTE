# Beads Viewer Integration

The Chrote dashboard integrates with [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) (`bv`) to display issue tracking data from `.beads` directories.

## Overview

The Beads tab in the dashboard provides three views:
- **Kanban** - Issues organized by status columns
- **Triage** - AI-recommended priorities, quick wins, and blockers
- **Insights** - Health scores, metrics, and breakdowns

## Requirements

The `bv` CLI must be installed for Triage and Insights views:

```bash
go install github.com/Dicklesworthstone/beads_viewer@latest
```

Kanban works without `bv` (reads `issues.jsonl` directly).

## Data Format

### JSONL Format (issues.jsonl)

Issues are stored in `.beads/issues.jsonl` with this structure:

```json
{
  "id": "hq-028lw",
  "title": "EPIC: Merge Existing Work",
  "description": "...",
  "status": "open",
  "priority": 1,
  "issue_type": "task",
  "assignee": "Chrote/polecats/onyx",
  "owner": "user@example.com",
  "created_at": "2026-01-21T10:57:34.182640539+02:00",
  "updated_at": "2026-01-21T10:57:34.182640539+02:00",
  "labels": ["P1", "epic", "testing"],
  "dependencies": [
    {
      "issue_id": "hq-028lw.1",
      "depends_on_id": "hq-028lw",
      "type": "parent-child",
      "created_at": "2026-01-21T10:57:51.060249127+02:00",
      "created_by": "Chrote/crew/Ronja"
    }
  ]
}
```

### Backend Transformation

The Go backend (`beads.go`) transforms raw JSONL to match the frontend interface:

| JSONL Field | Frontend Field | Notes |
|-------------|----------------|-------|
| `id` | `id` | Direct copy |
| `title` | `title` | Direct copy |
| `status` | `status` | Direct copy |
| `issue_type` | `type` | Field rename |
| `created_at` | `created` | Field rename |
| `updated_at` | `updated` | Field rename |
| `dependencies[].depends_on_id` | `dependencies[]` | Extract IDs only |

The `dependencies` array is flattened from objects to string IDs.

### Supported Status Values

The frontend recognizes these status values:

| Status | Kanban Column |
|--------|---------------|
| `open` | Open |
| `ready` | Ready |
| `in_progress` | In Progress |
| `hooked` | Hooked |
| `blocked` | Blocked |
| `closed` | Closed |
| `wont_fix` | Closed (same column) |
| `duplicate` | Closed (same column) |
| `deferred` | Other |

Any unrecognized status goes to the "Other" column.

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/beads/health` | GET | Check if `bv` is installed |
| `/api/beads/projects` | GET | List projects with `.beads` directories |
| `/api/beads/issues?path=X` | GET | Get transformed issues from `issues.jsonl` |
| `/api/beads/triage?path=X` | GET | Run `bv --robot-triage` |
| `/api/beads/insights?path=X` | GET | Run `bv --robot-insights` |
| `/api/beads/graph?path=X` | GET | Run `bv --robot-graph` |

## bv Command Integration

The backend shells out to `bv` for advanced analysis:

```go
// Triage recommendations
cmd := exec.Command("bv", "--robot-triage", projectPath)

// Insights/metrics
cmd := exec.Command("bv", "--robot-insights", projectPath)

// Dependency graph
cmd := exec.Command("bv", "--robot-graph", projectPath)
```

Commands have a 60-second timeout.

## Project Discovery

Projects are auto-discovered by scanning `/code` and `/vault` for directories containing `.beads/`. Users can also configure additional paths in Settings.

## Error Handling

The frontend handles missing/malformed data gracefully:

- Missing `recommendations`, `quickWins`, `blockers` arrays default to `[]`
- Missing `health` object defaults to `{ score: 0, risks: [], warnings: [] }`
- Missing numeric fields default to `0`

This ensures views render even when `bv` returns incomplete data.

## Files

### Backend
- `src/internal/api/beads.go` - API handlers and data transformation

### Frontend
- `dashboard/src/components/BeadsView/index.tsx` - Main container
- `dashboard/src/components/BeadsView/KanbanView.tsx` - Status columns
- `dashboard/src/components/BeadsView/TriageView.tsx` - Recommendations
- `dashboard/src/components/BeadsView/InsightsView.tsx` - Metrics dashboard
- `dashboard/src/components/BeadsView/IssueCard.tsx` - Issue display
- `dashboard/src/components/BeadsView/types.ts` - TypeScript interfaces
- `dashboard/src/components/BeadsView/hooks.ts` - Data fetching hooks
