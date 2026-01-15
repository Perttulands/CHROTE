# BOB-BEADS-INTEGRATION-MASTER-PLAN
## Comprehensive Analysis & Action Plan for beads_viewer Integration into AgentArena

> **Document Version:** 1.0
> **Created:** 2026-01-14
> **Purpose:** Evaluate and recommend integration strategies for beads_viewer as a tab in AgentArena, with options to run views in windows instead of tmux sessions

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [System Overview](#system-overview)
3. [Integration Options Matrix](#integration-options-matrix)
4. [Option 1: WebAssembly Integration](#option-1-webassembly-integration)
5. [Option 2: Native React Components](#option-2-native-react-components)
6. [Option 3: MCP/Protocol Integration](#option-3-mcpprotocol-integration)
7. [Option 4: Containerized Microservice](#option-4-containerized-microservice)
8. [Option 5: Hybrid Desktop (Tauri/Wails)](#option-5-hybrid-desktop-tauriwails)
9. [Option 6: Exotic Terminal Enhancements](#option-6-exotic-terminal-enhancements)
10. [Comparison & Evaluation](#comparison--evaluation)
11. [Recommendations](#recommendations)
12. [Implementation Roadmap](#implementation-roadmap)

---

## Executive Summary

This document analyzes **six distinct integration approaches** for embedding [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) into AgentArena's React dashboard. The goal is to:

1. Add beads_viewer as a **dedicated tab** in the dashboard
2. Enable users to **run beads views in browser windows** instead of tmux sessions
3. Provide **delightful and exotic** user experiences

### Key Finding

**Recommended Primary Approach:** Native React Components consuming beads robot JSON APIs (Option 2), combined with MCP Server integration (Option 3) for AI agent coordination.

**Exotic Enhancement:** GPU-accelerated xterm.js with CRT effects and gamification (Option 6).

### Quick Comparison

| Option | Effort | Performance | Integration | Delight Factor |
|--------|--------|-------------|-------------|----------------|
| 1. WebAssembly | 4-6 weeks | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| 2. React Components | 3-4 weeks | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| 3. MCP/Protocol | 2-3 weeks | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| 4. Microservice | 2-3 weeks | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐ |
| 5. Hybrid Desktop | 3-4 weeks | ⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ |
| 6. Exotic Terminal | 1-2 weeks | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

---

## System Overview

### beads_viewer Architecture

**beads_viewer** (`bv`) is a high-performance **graph-aware Terminal User Interface (TUI)** for the Beads issue tracking system.

```
┌─────────────────────────────────────────────────────────┐
│                    beads_viewer (bv)                    │
├─────────────────────────────────────────────────────────┤
│  TECH STACK                                             │
│  ├── Language: Go 1.25+                                 │
│  ├── TUI Framework: Bubble Tea (Elm-inspired)           │
│  ├── Styling: Lip Gloss                                 │
│  ├── Graph Analysis: Gonum (PageRank, betweenness, etc) │
│  └── Database: SQLite with FTS5                         │
├─────────────────────────────────────────────────────────┤
│  VIEWS                                                  │
│  ├── Split-view Dashboard (list + details)              │
│  ├── Kanban Board                                       │
│  ├── Visual Dependency Graph (DAG)                      │
│  ├── Insights Dashboard (9 graph metrics)               │
│  └── History View (git correlation)                     │
├─────────────────────────────────────────────────────────┤
│  ROBOT APIs (JSON output)                               │
│  ├── --robot-triage    → Prioritized recommendations    │
│  ├── --robot-insights  → Graph metrics & bottlenecks    │
│  ├── --robot-plan      → Parallel execution tracks      │
│  ├── --robot-graph     → Dependency graph (JSON/DOT)    │
│  └── --robot-search    → Semantic search                │
├─────────────────────────────────────────────────────────┤
│  EXPORTS                                                │
│  ├── Markdown reports                                   │
│  ├── Interactive HTML (force-graph, D3)                 │
│  └── SQLite databases                                   │
└─────────────────────────────────────────────────────────┘
```

### AgentArena Architecture

**AgentArena** is a Docker-based development environment with an integrated web dashboard for managing AI coding agents.

```
┌─────────────────────────────────────────────────────────┐
│                    AgentArena Dashboard                 │
├─────────────────────────────────────────────────────────┤
│  TECH STACK                                             │
│  ├── Frontend: React 18.2 + TypeScript + Vite           │
│  ├── Terminal: xterm.js 5.5 (WebSocket via ttyd)        │
│  ├── Drag & Drop: @dnd-kit                              │
│  ├── Styling: CSS variables (theme system)              │
│  └── State: React Context + localStorage                │
├─────────────────────────────────────────────────────────┤
│  CURRENT TABS                                           │
│  ├── Terminal (tmux session manager)                    │
│  ├── Files (filebrowser iframe)                         │
│  ├── Status (system health)                             │
│  ├── Settings (preferences)                             │
│  └── Beads ↗ (external link - to be replaced)           │
├─────────────────────────────────────────────────────────┤
│  BACKEND SERVICES                                       │
│  ├── nginx (port 8080) - reverse proxy                  │
│  ├── ttyd (port 7681) - WebSocket terminal              │
│  ├── filebrowser (port 8081) - file management          │
│  └── Express API (port 3001) - session management       │
├─────────────────────────────────────────────────────────┤
│  SESSION MANAGEMENT                                     │
│  ├── 1-4 configurable terminal windows                  │
│  ├── Drag-drop sessions between windows                 │
│  ├── Multiple sessions bound per window                 │
│  └── Floating modal "peek" view                         │
└─────────────────────────────────────────────────────────┘
```

---

## Integration Options Matrix

| Aspect | WASM | React | MCP | Microservice | Hybrid | Exotic |
|--------|------|-------|-----|--------------|--------|--------|
| **Binary Size** | 0.5-4 MB | N/A | N/A | N/A | 4-8 MB | N/A |
| **Load Time** | 200ms-2s | <100ms | N/A | 100ms | 1-2s | <100ms |
| **beads_viewer Unchanged** | Partial | No | Yes | Yes | No | Yes |
| **React Integration** | Good | Native | Good | Good | Poor | Good |
| **Real-time Updates** | Moderate | Excellent | Good | Moderate | Poor | Excellent |
| **AI Agent Support** | Limited | Limited | Excellent | Moderate | Poor | Limited |
| **Maintenance** | Medium | Medium | Low | Low | High | Low |
| **Delight Factor** | Medium | High | Medium | Low | Medium | Very High |

---

## Option 1: WebAssembly Integration

### Overview

Compile beads_viewer's Go analysis engine to WebAssembly and run it directly in the browser.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Browser (AgentArena Dashboard)              │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────────────────────────────────────────┐   │
│  │  React Components (BeadsViewer Tab)               │   │
│  │  ├── BeadsListView                                │   │
│  │  ├── BeadsKanbanView                              │   │
│  │  ├── BeadsGraphView                               │   │
│  │  └── BeadsInsightsView                            │   │
│  └──────────────────────────────────────────────────┘   │
│                         ↓ calls                          │
│  ┌──────────────────────────────────────────────────┐   │
│  │  WASM Module (beads_analysis.wasm)                │   │
│  │  ├── analyzeBeads(jsonData) → AnalysisResult      │   │
│  │  ├── getRelationships(beads) → GraphData          │   │
│  │  ├── computePageRank(graph) → Rankings            │   │
│  │  └── detectCycles(graph) → Cycles[]               │   │
│  └──────────────────────────────────────────────────┘   │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Technical Approach

**Sub-Option A: Analysis Engine Only (Recommended)**
- Extract `pkg/analysis` and `pkg/loader` from beads_viewer
- Compile to WASM with `go:wasmexport` directives (Go 1.24+)
- Build custom React frontend consuming WASM functions
- Binary size: ~0.5-1 MB (optimized)

**Sub-Option B: Full TUI Compilation**
- Compile entire Bubble Tea TUI to WASM
- Use xterm.js as terminal renderer
- Requires modifications for I/O handling
- Binary size: ~2-4 MB

### Go WASM Export Example

```go
// cmd/beads-wasm/main.go
package main

import (
    "encoding/json"
    "github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
)

//go:wasmexport analyzeBeads
func AnalyzeBeads(jsonData string) string {
    var beads []analysis.Bead
    json.Unmarshal([]byte(jsonData), &beads)
    result := analysis.Analyze(beads)
    bytes, _ := json.Marshal(result)
    return string(bytes)
}

//go:wasmexport computePageRank
func ComputePageRank(graphJSON string) string {
    // ... PageRank computation
}
```

### React Integration

```typescript
// hooks/useBeadsWasm.ts
export function useBeadsWasm() {
  const [wasm, setWasm] = useState<WebAssembly.Instance | null>(null);

  useEffect(() => {
    WebAssembly.instantiateStreaming(
      fetch('/beads_analysis.wasm')
    ).then(result => setWasm(result.instance));
  }, []);

  const analyzeBeads = (data: BeadsData) => {
    if (!wasm) return null;
    const result = (wasm.exports.analyzeBeads as Function)(
      JSON.stringify(data)
    );
    return JSON.parse(result);
  };

  return { analyzeBeads, ready: !!wasm };
}
```

### Pros & Cons

| Pros | Cons |
|------|------|
| Near-native analysis performance | Requires beads_viewer fork |
| No server round-trips | WASM binary adds to bundle |
| Offline-capable | TUI compilation complex |
| Browser sandboxed (secure) | File I/O requires workarounds |

### Effort Estimate

- **Analysis Engine Only:** 4-6 weeks
- **Full TUI:** 6-8 weeks (not recommended)

---

## Option 2: Native React Components

### Overview

Port beads_viewer's visualization views as native React components, consuming JSON from beads robot APIs.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│              AgentArena Dashboard                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  TabBar: Terminal | Files | [Beads] | Status | Settings │
│                              ↓                           │
│  ┌──────────────────────────────────────────────────┐   │
│  │  BeadsViewer Container                            │   │
│  │                                                   │   │
│  │  Sub-tabs: List | Kanban | Graph | Insights      │   │
│  │                                                   │   │
│  │  ┌────────────────────────────────────────────┐  │   │
│  │  │  Active View Component                     │  │   │
│  │  │  ├── BeadsListView (TanStack Table)        │  │   │
│  │  │  ├── BeadsKanbanView (@dnd-kit columns)    │  │   │
│  │  │  ├── BeadsGraphView (react-flow DAG)       │  │   │
│  │  │  └── BeadsInsightsView (Chart.js)          │  │   │
│  │  └────────────────────────────────────────────┘  │   │
│  └──────────────────────────────────────────────────┘   │
│                         ↓ HTTP                           │
│  ┌──────────────────────────────────────────────────┐   │
│  │  Beads API Wrapper (Node.js, port 3002)           │   │
│  │  ├── GET /api/beads/projects                      │   │
│  │  ├── GET /api/beads/project/:id/analysis          │   │
│  │  ├── GET /api/beads/project/:id/graph             │   │
│  │  └── GET /api/beads/project/:id/export            │   │
│  └──────────────────────────────────────────────────┘   │
│                         ↓ subprocess                     │
│  ┌──────────────────────────────────────────────────┐   │
│  │  bv --robot-* (beads_viewer CLI)                  │   │
│  └──────────────────────────────────────────────────┘   │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Recommended Libraries

| Purpose | Library | Rationale |
|---------|---------|-----------|
| **DAG Visualization** | react-flow | Best balance of features, dagre layout support |
| **Lists** | TanStack Table | Headless, works with existing CSS |
| **Kanban** | @dnd-kit/sortable | Already in AgentArena |
| **Charts** | Chart.js | Lightweight, beads_viewer uses D3 |
| **State** | Zustand | Simpler than Redux, hooks-friendly |
| **Data Fetching** | React Query | Polling, caching, refetch on focus |

### Component Structure

```
dashboard/src/
├── components/
│   └── BeadsViewer/
│       ├── BeadsViewer.tsx          # Main container
│       ├── BeadsListView.tsx        # Sortable list with filters
│       ├── BeadsKanbanView.tsx      # Drag-drop columns
│       ├── BeadsGraphView.tsx       # react-flow DAG
│       ├── BeadsInsightsView.tsx    # Chart.js metrics
│       ├── TaskCard.tsx             # Reusable task display
│       ├── DependencyEdge.tsx       # react-flow edge renderer
│       └── TaskModal.tsx            # Detail view/edit
├── hooks/
│   ├── useBeadsData.ts              # React Query hooks
│   └── useBeadsFilters.ts           # Filter state
├── stores/
│   └── beadsStore.ts                # Zustand state
└── types/
    └── beads.ts                     # TypeScript interfaces
```

### API Wrapper (Express)

```javascript
// beads-api/server.js
const express = require('express');
const { execSync } = require('child_process');
const app = express();

app.get('/api/beads/project/:id/analysis', (req, res) => {
  const projectPath = `/code/${req.params.id}`;

  try {
    const output = execSync(
      `cd "${projectPath}" && bv --robot-insights`,
      { encoding: 'utf-8', timeout: 10000 }
    );

    const data = JSON.parse(output);
    res.json({
      project: req.params.id,
      metrics: data.metrics,
      critical_path: data.critical_path,
      bottlenecks: data.bottlenecks,
      cycles: data.cycles,
      data_hash: data.data_hash,
      timestamp: new Date().toISOString()
    });
  } catch (e) {
    res.status(500).json({ error: e.message });
  }
});

app.listen(3002);
```

### React Integration Example

```tsx
// BeadsGraphView.tsx
import ReactFlow, { useNodesState, useEdgesState } from 'reactflow';
import { useBeadsGraph } from '../hooks/useBeadsData';

export function BeadsGraphView({ projectId }: { projectId: string }) {
  const { data: graph, isLoading } = useBeadsGraph(projectId);

  const [nodes, setNodes, onNodesChange] = useNodesState(
    graph?.nodes.map(n => ({
      id: n.id,
      data: { label: n.title, status: n.status },
      position: { x: 0, y: 0 }, // dagre will layout
    })) ?? []
  );

  const [edges, setEdges, onEdgesChange] = useEdgesState(
    graph?.edges.map(e => ({
      id: `${e.from}-${e.to}`,
      source: e.from,
      target: e.to,
      type: e.type,
    })) ?? []
  );

  if (isLoading) return <LoadingSpinner />;

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      fitView
    />
  );
}
```

### Pros & Cons

| Pros | Cons |
|------|------|
| Native React integration | Requires reimplementing UI |
| Full UX customization | Different look from TUI |
| Uses existing @dnd-kit | API wrapper adds latency |
| Leverages AgentArena themes | More code to maintain |
| Real-time React Query updates | No offline support |

### Effort Estimate

- **Phase 1 (List + Kanban):** 2 weeks
- **Phase 2 (Graph):** 1-2 weeks
- **Phase 3 (Insights + Polish):** 1 week
- **Total:** 3-4 weeks

---

## Option 3: MCP/Protocol Integration

### Overview

Create an MCP (Model Context Protocol) server that exposes beads_viewer's robot APIs as tools for Claude and other AI agents.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Claude Code / AI Agents                     │
├─────────────────────────────────────────────────────────┤
│                         ↓ MCP                            │
│  ┌──────────────────────────────────────────────────┐   │
│  │  Beads MCP Server (Python FastMCP)                │   │
│  │                                                   │   │
│  │  TOOLS:                                           │   │
│  │  ├── beads_triage(repo_path) → TriageResult       │   │
│  │  ├── beads_insights(repo_path) → GraphMetrics     │   │
│  │  ├── beads_plan(repo_path) → ExecutionPlan        │   │
│  │  └── beads_search(query) → SearchResults          │   │
│  │                                                   │   │
│  │  RESOURCES:                                       │   │
│  │  └── project_graph → Live dependency graph        │   │
│  └──────────────────────────────────────────────────┘   │
│                         ↓ subprocess                     │
│  ┌──────────────────────────────────────────────────┐   │
│  │  bv --robot-* (beads_viewer CLI)                  │   │
│  └──────────────────────────────────────────────────┘   │
│                                                          │
├─────────────────────────────────────────────────────────┤
│              AgentArena Dashboard                        │
│                         ↓ SSE/WebSocket                  │
│  ┌──────────────────────────────────────────────────┐   │
│  │  Real-time Updates Server                         │   │
│  │  ├── SSE: /api/beads/stream                       │   │
│  │  └── WebSocket: /ws/beads/live                    │   │
│  └──────────────────────────────────────────────────┘   │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### MCP Server Implementation

```python
# beads_mcp_server.py
from fastmcp import FastMCP, Tool
import json
import subprocess

app = FastMCP("beads-viewer-mcp")

@app.tool
def beads_triage(repo_path: str = ".") -> dict:
    """
    Get comprehensive project triage with priorities and recommendations.

    Returns prioritized recommendations, quick wins, blockers to clear,
    and project health metrics including graph analysis.
    """
    result = subprocess.run(
        ["bv", "--robot-triage"],
        cwd=repo_path,
        capture_output=True,
        text=True
    )
    return json.loads(result.stdout)

@app.tool
def beads_insights(repo_path: str = ".") -> dict:
    """
    Get graph metrics: PageRank, betweenness, critical path, cycles.

    Identifies bottlenecks, keystones, and structural issues in
    the project dependency graph.
    """
    result = subprocess.run(
        ["bv", "--robot-insights"],
        cwd=repo_path,
        capture_output=True,
        text=True
    )
    return json.loads(result.stdout)

@app.tool
def beads_plan(repo_path: str = ".") -> dict:
    """
    Generate parallelizable execution plan with independent work tracks.
    """
    result = subprocess.run(
        ["bv", "--robot-plan"],
        cwd=repo_path,
        capture_output=True,
        text=True
    )
    return json.loads(result.stdout)

if __name__ == "__main__":
    app.run()
```

### Claude Code Configuration

```json
{
  "mcpServers": {
    "beads-viewer": {
      "command": "python",
      "args": ["/srv/beads-mcp/server.py"],
      "env": {
        "BEADS_REPO": "/code"
      }
    }
  }
}
```

### Real-time Streaming (SSE)

```python
# FastAPI SSE endpoint
from fastapi import FastAPI
from fastapi.responses import StreamingResponse
from watchfiles import awatch

app = FastAPI()

async def watch_beads_updates():
    async for changes in awatch("/code/.beads"):
        triage = subprocess.run(
            ["bv", "--robot-triage"],
            capture_output=True, text=True
        )
        yield f"data: {triage.stdout}\n\n"

@app.get("/api/beads/stream")
async def stream_updates():
    return StreamingResponse(
        watch_beads_updates(),
        media_type="text/event-stream"
    )
```

### Agent-to-Agent Coordination

```
┌────────────────────────────────────────────────────────┐
│         AgentArena (tmux sessions)                     │
│                                                        │
│ ┌────────────────────────────────────────────────────┐│
│ │  HQ Agent (Coordinator)                            ││
│ │  - Runs bv --robot-triage on startup               ││
│ │  - Splits work into parallel tracks                ││
│ │  - Assigns tracks via MCP Agent Mail               ││
│ └───────────────────┬────────────────────────────────┘│
│                     │                                  │
│     ┌───────────────┼───────────────┐                  │
│     │               │               │                  │
│  ┌──▼──┐        ┌──▼──┐        ┌──▼──┐               │
│  │Agt 1│        │Agt 2│        │Agt 3│               │
│  │Track│        │Track│        │Track│               │
│  │  A  │        │  B  │        │  C  │               │
│  └─────┘        └─────┘        └─────┘               │
│                                                        │
└────────────────────────────────────────────────────────┘
```

### Pros & Cons

| Pros | Cons |
|------|------|
| beads_viewer unchanged | Requires MCP infrastructure |
| AI-native integration | Learning curve for MCP |
| Works with Claude Code | Not visual (data only) |
| Deterministic JSON APIs | Needs separate UI layer |
| Agent coordination | Additional Python service |

### Effort Estimate

- **MCP Server:** 1 week
- **SSE Streaming:** 1 week
- **Agent Coordination:** 1 week
- **Total:** 2-3 weeks

---

## Option 4: Containerized Microservice

### Overview

Run beads_viewer as a containerized service, accessible via ttyd terminal or HTTP API wrapper.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Docker Compose Stack                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌────────────────────────────────────────────────┐     │
│  │  nginx (port 8080)                              │     │
│  │  ├── /terminal/ → ttyd (7681)                   │     │
│  │  ├── /files/    → filebrowser (8081)            │     │
│  │  ├── /api/      → Express API (3001)            │     │
│  │  ├── /beads-api/ → Beads API (3002)      [NEW]  │     │
│  │  └── /beads-tui/ → ttyd-beads (7682)     [NEW]  │     │
│  └────────────────────────────────────────────────┘     │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │   ttyd   │  │filebrowser│  │  API    │              │
│  │  :7681   │  │  :8081   │  │  :3001   │              │
│  └──────────┘  └──────────┘  └──────────┘              │
│                                                          │
│  ┌──────────┐  ┌──────────┐                             │
│  │ttyd-beads│  │beads-api │   [NEW SERVICES]           │
│  │  :7682   │  │  :3002   │                             │
│  └──────────┘  └──────────┘                             │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Docker Configuration

```dockerfile
# In build1.dockerfile - add second ttyd for beads TUI
RUN printf '#!/bin/bash\n\
export BEADS_PROJECT_DIR="${1:-.}"\n\
cd "$BEADS_PROJECT_DIR"\n\
exec bv\n' > /usr/local/bin/beads-launch.sh && chmod +x /usr/local/bin/beads-launch.sh

# In entrypoint:
ttyd -p 7682 -W -a -u 1000 -g 1000 -t fontSize=14 \
  /usr/local/bin/beads-launch.sh &
```

### nginx Routing

```nginx
# Beads TUI via ttyd
location /beads-tui/ {
    proxy_pass http://127.0.0.1:7682/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 86400;
}

# Beads REST API
location /beads-api/ {
    proxy_pass http://127.0.0.1:3002/;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
}
```

### React Tab Integration

```tsx
// Simple iframe approach for TUI
function BeadsTuiTab({ projectPath }: { projectPath: string }) {
  return (
    <iframe
      src={`/beads-tui/?arg=${encodeURIComponent(projectPath)}`}
      className="beads-iframe"
      title="Beads Viewer TUI"
    />
  );
}

// Or hybrid: API data + optional TUI popup
function BeadsTab() {
  const [viewMode, setViewMode] = useState<'dashboard' | 'tui'>('dashboard');

  return viewMode === 'dashboard'
    ? <BeadsDashboardView />
    : <BeadsTuiTab />;
}
```

### Session Switching Feature

```tsx
// Allow users to switch beads views between tmux and dashboard
interface BeadsBinding {
  sessionName: string;
  projectPath: string;
  viewMode: 'tmux' | 'dashboard';
}

// In SessionPanel.tsx
<div className="beads-session-control">
  <select onChange={(e) => bindBeadsProject(session.name, e.target.value)}>
    <option value="">No beads project</option>
    {projects.map(p => <option key={p.path} value={p.path}>{p.name}</option>)}
  </select>

  <div className="view-mode-toggle">
    <button onClick={() => setViewMode('tmux')}>📱 tmux</button>
    <button onClick={() => setViewMode('dashboard')}>📊 Dashboard</button>
  </div>
</div>
```

### Pros & Cons

| Pros | Cons |
|------|------|
| beads_viewer unchanged | WebSocket latency |
| Familiar ttyd pattern | Terminal constraints |
| Easy to implement | Cannot integrate data |
| TUI keyboard support | Separate from dashboard state |

### Effort Estimate

- **ttyd Setup:** 0.5 weeks
- **API Wrapper:** 1 week
- **React Integration:** 1 week
- **Total:** 2-3 weeks

---

## Option 5: Hybrid Desktop (Tauri/Wails)

### Overview

Build a desktop application wrapper using Tauri (Rust) or Wails (Go) that provides native beads_viewer integration.

### Comparison

| Aspect | Tauri | Wails | Electron |
|--------|-------|-------|----------|
| **Binary Size** | 4-8 MB | 8 MB | 150 MB |
| **Language** | Rust + JS | Go + JS | JS only |
| **Go Integration** | Sidecar | Native | node-pty |
| **Performance** | Excellent | Excellent | Good |
| **TTY Support** | Manual | Custom | node-pty |
| **Maturity** | Stable (v2) | Alpha (v3) | Mature |

### Tauri Sidecar Pattern

```rust
// src-tauri/src/main.rs
use tauri::Manager;

fn main() {
    tauri::Builder::default()
        .setup(|app| {
            // Bundle beads_viewer binary
            let sidecar = app.shell().sidecar("bv")
                .args(["--robot-triage"])
                .spawn()
                .expect("Failed to spawn bv");
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![get_triage])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

#[tauri::command]
async fn get_triage(project_path: String) -> Result<String, String> {
    let output = std::process::Command::new("bv")
        .args(["--robot-triage"])
        .current_dir(&project_path)
        .output()
        .map_err(|e| e.to_string())?;

    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}
```

### Wails Native Go

```go
// main.go
package main

import (
    "github.com/wailsapp/wails/v2"
    "github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
)

type App struct{}

func (a *App) GetTriage(projectPath string) (string, error) {
    // Direct Go function call - no subprocess!
    issues, _ := loader.LoadIssuesFromFile(
        filepath.Join(projectPath, ".beads/beads.jsonl")
    )
    triage := analysis.ComputeTriage(issues)
    bytes, _ := json.Marshal(triage)
    return string(bytes), nil
}

func main() {
    app := &App{}
    wails.Run(&wails.AppConfig{
        Title: "AgentArena Desktop",
        Bind: []interface{}{app},
    })
}
```

### Pros & Cons

| Pros | Cons |
|------|------|
| Native performance | Desktop-only (not web) |
| Full PTY support | Additional distribution |
| Small binary (Tauri) | Different from web dashboard |
| Direct Go calls (Wails) | User must install app |

### Effort Estimate

- **Tauri Sidecar:** 3-4 weeks
- **Wails Native:** 4-5 weeks (requires beads_viewer fork)
- **Electron:** 2-3 weeks

---

## Option 6: Exotic Terminal Enhancements

### Overview

Enhance the terminal viewing experience with GPU acceleration, visual effects, and gamification.

### GPU-Accelerated Rendering

```typescript
// Use xterm-addon-webgl for 900% performance boost
import { WebglAddon } from 'xterm-addon-webgl';

const terminal = new Terminal();
terminal.loadAddon(new WebglAddon());
```

### CRT Retro Effects

```css
/* Matrix-style beads viewer theme */
.beads-terminal {
  --phosphor-color: #00ff00;
  --glow-intensity: 0.8;
  --scanline-opacity: 0.1;
  --curvature: 0.02;
}

.beads-terminal::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: repeating-linear-gradient(
    0deg,
    rgba(0, 0, 0, 0.1),
    rgba(0, 0, 0, 0.1) 1px,
    transparent 1px,
    transparent 2px
  );
  pointer-events: none;
}
```

### WebGL Shader Effects

```glsl
// CRT curvature shader
vec2 curveRemapUV(vec2 uv) {
    uv = uv * 2.0 - 1.0;
    vec2 offset = abs(uv.yx) / vec2(6.0, 4.0);
    uv = uv + uv * offset * offset;
    uv = uv * 0.5 + 0.5;
    return uv;
}

// Chromatic aberration
vec3 chromaticAberration(sampler2D tex, vec2 uv) {
    float offset = 0.003;
    float r = texture2D(tex, uv + vec2(offset, 0.0)).r;
    float g = texture2D(tex, uv).g;
    float b = texture2D(tex, uv - vec2(offset, 0.0)).b;
    return vec3(r, g, b);
}
```

### Gamification System

```typescript
// XP and achievements for beads tasks
interface BeadsProgress {
  xp: number;
  level: number;
  achievements: Achievement[];
  streak: number;
}

const ACHIEVEMENTS = {
  first_triage: { name: "First Triage", xp: 100 },
  cycle_hunter: { name: "Cycle Hunter", xp: 250, condition: "Found dependency cycle" },
  bottleneck_buster: { name: "Bottleneck Buster", xp: 500 },
  graph_master: { name: "Graph Master", xp: 1000, condition: "100+ nodes analyzed" },
};

function calculateLevel(xp: number): number {
  return Math.floor(Math.sqrt(xp / 100)) + 1;
}

// Sound effects on achievements
function playAchievementSound() {
  const audio = new Audio('/sounds/achievement.mp3');
  audio.play();
}
```

### Picture-in-Picture Terminal

```typescript
// Use Document PiP API for floating beads view
async function openBeadsPiP() {
  const pipWindow = await documentPictureInPicture.requestWindow({
    width: 400,
    height: 300,
  });

  // Clone terminal into PiP window
  const terminal = document.querySelector('.beads-terminal');
  pipWindow.document.body.appendChild(terminal.cloneNode(true));
}
```

### Inline Graphics (Sixel/Kitty Protocol)

```bash
# Future: Render beads graph inline in terminal
# When xterm.js supports Sixel/Kitty graphics protocol
bv --graph-format=sixel  # Renders DAG as inline image
```

### Collaborative Viewing

```typescript
// Real-time session sharing via WebSocket
interface CollaborativeSession {
  sessionId: string;
  participants: Participant[];
  cursors: Map<string, CursorPosition>;
}

// Show other users' cursors in beads view
function renderCollaborativeCursors(session: CollaborativeSession) {
  session.cursors.forEach((pos, odorId) => {
    renderCursor(pos, session.participants.find(p => p.id === odorId));
  });
}
```

### Pros & Cons

| Pros | Cons |
|------|------|
| Delightful UX | Experimental features |
| Performance boost | Browser compatibility |
| Unique branding | Development overhead |
| User engagement | Distraction potential |

### Effort Estimate

- **WebGL Acceleration:** 0.5 weeks
- **CRT Effects:** 1 week
- **Gamification:** 1-2 weeks
- **Total:** 1-3 weeks (incremental)

---

## Comparison & Evaluation

### Weighted Scoring Matrix

| Criterion | Weight | WASM | React | MCP | Container | Hybrid | Exotic |
|-----------|--------|------|-------|-----|-----------|--------|--------|
| **Performance** | 20% | 5 | 4 | 4 | 3 | 4 | 5 |
| **Integration** | 25% | 3 | 5 | 5 | 4 | 2 | 4 |
| **Effort** | 20% | 2 | 4 | 4 | 4 | 3 | 5 |
| **Maintainability** | 15% | 3 | 4 | 4 | 4 | 2 | 3 |
| **User Delight** | 10% | 3 | 4 | 3 | 2 | 3 | 5 |
| **AI Support** | 10% | 2 | 2 | 5 | 3 | 1 | 2 |
| **WEIGHTED TOTAL** | 100% | **3.0** | **4.1** | **4.2** | **3.5** | **2.6** | **4.1** |

### Decision Matrix

```
┌────────────────────────────────────────────────────────────────┐
│                    INTEGRATION DECISION TREE                   │
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Do you need AI agent integration (Claude Code, MCP)?           │
│     │                                                           │
│     ├── YES → Option 3: MCP Server (primary)                   │
│     │         + Option 2: React Components (visualization)      │
│     │                                                           │
│     └── NO → Do you want maximum UI flexibility?               │
│              │                                                  │
│              ├── YES → Option 2: React Components              │
│              │         + Option 6: Exotic Enhancements         │
│              │                                                  │
│              └── NO → Do you want TUI experience in browser?   │
│                       │                                         │
│                       ├── YES → Option 4: Containerized ttyd   │
│                       │                                         │
│                       └── NO → Option 1: WASM (analysis only)  │
│                                                                 │
│  Need desktop app?                                              │
│     └── Option 5: Tauri/Wails (supplementary)                  │
│                                                                 │
└────────────────────────────────────────────────────────────────┘
```

---

## Recommendations

### Primary Recommendation: Hybrid Approach

Combine **Option 2 (React Components)** + **Option 3 (MCP Server)** + **Option 6 (Exotic Enhancements)**

```
┌─────────────────────────────────────────────────────────────────┐
│                   RECOMMENDED ARCHITECTURE                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  AgentArena Dashboard                                       │ │
│  │                                                             │ │
│  │  TabBar: Terminal | Files | [Beads] | Status | Settings   │ │
│  │                              ↓                              │ │
│  │  ┌───────────────────────────────────────────────────────┐ │ │
│  │  │  BeadsViewer Tab                                      │ │ │
│  │  │                                                       │ │ │
│  │  │  ┌─────────────────────────────────────────────────┐  │ │ │
│  │  │  │  Sub-tabs: List | Kanban | Graph | Insights     │  │ │ │
│  │  │  └─────────────────────────────────────────────────┘  │ │ │
│  │  │                                                       │ │ │
│  │  │  ┌─────────────────────────────────────────────────┐  │ │ │
│  │  │  │  React Components (Option 2)                    │  │ │ │
│  │  │  │  ├── BeadsListView (TanStack Table)             │  │ │ │
│  │  │  │  ├── BeadsKanbanView (@dnd-kit)                 │  │ │ │
│  │  │  │  ├── BeadsGraphView (react-flow)                │  │ │ │
│  │  │  │  └── BeadsInsightsView (Chart.js)               │  │ │ │
│  │  │  └─────────────────────────────────────────────────┘  │ │ │
│  │  │                                                       │ │ │
│  │  │  ┌─────────────────────────────────────────────────┐  │ │ │
│  │  │  │  Exotic Enhancements (Option 6)                 │  │ │ │
│  │  │  │  ├── GPU-accelerated rendering                  │  │ │ │
│  │  │  │  ├── Matrix/CRT themes                          │  │ │ │
│  │  │  │  ├── Gamification (XP, achievements)            │  │ │ │
│  │  │  │  └── PiP floating windows                       │  │ │ │
│  │  │  └─────────────────────────────────────────────────┘  │ │ │
│  │  └───────────────────────────────────────────────────────┘ │ │
│  │                              ↓                              │ │
│  └────────────────────────────────────────────────────────────┘ │
│                               ↓ HTTP/SSE                         │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Backend Services                                           │ │
│  │  ├── Beads API (Node.js) ← calls bv --robot-*              │ │
│  │  └── MCP Server (Python) ← exposes tools for Claude        │ │
│  └────────────────────────────────────────────────────────────┘ │
│                               ↓                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  AI Agent Integration (Option 3)                            │ │
│  │  ├── Claude Code skill/tool binding                        │ │
│  │  ├── Agent-to-agent coordination                           │ │
│  │  └── Real-time SSE updates to dashboard                    │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  OPTIONAL: TUI Fallback (Option 4)                              │
│  └── ttyd endpoint for users who prefer terminal experience    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Why This Combination?

1. **React Components** provide the best dashboard integration with existing AgentArena UX
2. **MCP Server** enables AI agents to use beads_viewer data directly
3. **Exotic Enhancements** add delight without core complexity
4. **Optional TUI** preserves the original experience for power users

---

## Implementation Roadmap

### Phase 1: Foundation (Weeks 1-2)

```
Week 1:
├── [ ] Create /beads-api/ Express wrapper
│       ├── GET /api/beads/projects
│       ├── GET /api/beads/project/:id/analysis
│       └── GET /api/beads/project/:id/graph
├── [ ] Add Zustand store for beads state
├── [ ] Create useBeadsData React Query hooks
└── [ ] Add BeadsViewer tab to TabBar

Week 2:
├── [ ] Build BeadsListView with TanStack Table
├── [ ] Build BeadsKanbanView with @dnd-kit/sortable
├── [ ] Integrate task filtering and search
└── [ ] Style with existing theme CSS variables
```

### Phase 2: Visualization (Weeks 3-4)

```
Week 3:
├── [ ] Install and configure react-flow
├── [ ] Implement BeadsGraphView component
├── [ ] Add dagre layout for DAG visualization
└── [ ] Style nodes/edges for task types

Week 4:
├── [ ] Build BeadsInsightsView (Chart.js metrics)
├── [ ] Add real-time polling with React Query
├── [ ] Cross-linking: click task → show in Kanban
└── [ ] Keyboard shortcuts integration
```

### Phase 3: AI Integration (Weeks 5-6)

```
Week 5:
├── [ ] Create beads MCP server (Python FastMCP)
├── [ ] Implement beads_triage, beads_insights tools
├── [ ] Configure Claude Code skill binding
└── [ ] Test with Claude Code

Week 6:
├── [ ] Add SSE endpoint for real-time updates
├── [ ] Implement agent coordination workflow
├── [ ] Dashboard receives live updates
└── [ ] Integration testing
```

### Phase 4: Exotic Enhancements (Weeks 7-8)

```
Week 7:
├── [ ] Enable xterm-addon-webgl acceleration
├── [ ] Implement Matrix/CRT theme options
├── [ ] Add theme selector to Settings
└── [ ] WebGL shader effects (optional)

Week 8:
├── [ ] Implement gamification system (XP, achievements)
├── [ ] Add sound effects (optional)
├── [ ] Picture-in-Picture floating windows
└── [ ] Polish and testing
```

### Phase 5: Optional - TUI Fallback (Week 9)

```
Week 9:
├── [ ] Add second ttyd instance for beads TUI
├── [ ] Configure nginx routing
├── [ ] Add view mode toggle (dashboard/TUI)
└── [ ] Session binding feature
```

---

## File Structure

```
AgentArena/
├── beads_viewer_integration/
│   ├── BOB-BEADS-INTEGRATION-MASTER-PLAN.md  (this document)
│   └── README.md
├── beads-api/                                [NEW]
│   ├── server.js
│   ├── package.json
│   └── routes/
│       ├── projects.js
│       ├── analysis.js
│       └── graph.js
├── beads-mcp/                                [NEW]
│   ├── server.py
│   ├── requirements.txt
│   └── tools/
│       ├── triage.py
│       ├── insights.py
│       └── plan.py
├── dashboard/src/
│   ├── components/
│   │   └── BeadsViewer/                      [NEW]
│   │       ├── BeadsViewer.tsx
│   │       ├── BeadsListView.tsx
│   │       ├── BeadsKanbanView.tsx
│   │       ├── BeadsGraphView.tsx
│   │       ├── BeadsInsightsView.tsx
│   │       ├── TaskCard.tsx
│   │       └── TaskModal.tsx
│   ├── hooks/
│   │   ├── useBeadsData.ts                   [NEW]
│   │   └── useBeadsFilters.ts                [NEW]
│   ├── stores/
│   │   └── beadsStore.ts                     [NEW]
│   └── types/
│       └── beads.ts                          [NEW]
├── nginx/
│   └── nginx.conf                            [MODIFY]
├── build1.dockerfile                         [MODIFY]
└── docker-compose.yml                        [MODIFY]
```

---

## Conclusion

This master plan provides a comprehensive roadmap for integrating beads_viewer into AgentArena. The recommended hybrid approach (React + MCP + Exotic) balances:

- **Functionality:** Full visualization capabilities
- **Integration:** Native React components with AgentArena UX
- **AI Support:** MCP tools for Claude Code and agent coordination
- **Delight:** GPU acceleration, themes, and gamification

The phased implementation allows for incremental delivery with early value (basic views in weeks 2-3) while building toward the complete vision (AI integration and exotic features in weeks 5-8).

---

## Appendix: Resources

### beads_viewer
- Repository: https://github.com/Dicklesworthstone/beads_viewer
- Robot API Documentation: See README.md

### Libraries
- react-flow: https://reactflow.dev
- @dnd-kit: https://dndkit.com
- TanStack Table: https://tanstack.com/table
- Chart.js: https://chartjs.org
- Zustand: https://zustand-demo.pmnd.rs
- React Query: https://tanstack.com/query

### MCP
- Specification: https://modelcontextprotocol.io
- FastMCP: https://github.com/jlowin/fastmcp

### Exotic
- xterm-addon-webgl: https://www.npmjs.com/package/xterm-addon-webgl
- Cool Retro Term effects: https://github.com/HairyDuck/terminal
- Document PiP API: https://developer.mozilla.org/en-US/docs/Web/API/Document_Picture-in-Picture_API
