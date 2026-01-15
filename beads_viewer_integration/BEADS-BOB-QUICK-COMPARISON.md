# Beads Viewer Integration Options

## Quick Reference Guide

A concise comparison of six approaches for integrating [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) into AgentArena.

---

## Executive Summary

**Recommended Approach:** Combine **Option 2 (React Components)** with **Option 3 (MCP Protocol)** for the best balance of functionality and AI integration. Add **Option 6 (Exotic Enhancements)** for visual polish.

---

## Option Overview

| # | Option | Description | Development Time | User Experience |
|:-:|--------|-------------|:----------------:|:---------------:|
| 1 | **WASM** | Compile Go analysis engine to WebAssembly | 4-6 weeks | Good |
| 2 | **React** | Build native React components from scratch | 3-4 weeks | Excellent |
| 3 | **MCP** | Model Context Protocol server for AI agents | 2-3 weeks | Good |
| 4 | **Container** | Wrap TUI in Docker with ttyd web terminal | 2-3 weeks | Fair |
| 5 | **Hybrid** | Desktop app via Tauri or Wails framework | 3-4 weeks | Good |
| 6 | **Exotic** | GPU rendering, CRT effects, gamification | 1-2 weeks | Excellent |

---

## When to Use Each Option

### Decision Guide

```
Do you need AI agent integration (Claude Code)?
├── YES → Option 3 (MCP) + Option 2 (React)
│         Consider adding Option 6 for visual polish
│
└── NO
    │
    ├── Do you need full control over the UI?
    │   └── YES → Option 2 (React) + Option 6 (Exotic)
    │
    ├── Do you want to preserve the original TUI experience?
    │   └── YES → Option 4 (Container with ttyd)
    │
    └── Do you need offline/edge computing support?
        └── YES → Option 1 (WASM)
```

### Use Case Mapping

| Use Case | Recommended Options |
|----------|---------------------|
| Dashboard with custom visualizations | Option 2 (React) |
| Claude Code / AI assistant integration | Option 3 (MCP) |
| Quick deployment, minimal changes | Option 4 (Container) |
| Offline-first or edge deployment | Option 1 (WASM) |
| Desktop application distribution | Option 5 (Hybrid) |
| Enhanced visual experience | Option 6 (Exotic) |

---

## Feature Comparison Matrix

| Feature | WASM | React | MCP | Container | Hybrid | Exotic |
|---------|:----:|:-----:|:---:|:---------:|:------:|:------:|
| **Browser-based** | Yes | Yes | No | Yes | No | Yes |
| **DAG visualization** | Yes | Yes | No | Yes | Yes | Yes |
| **Kanban board** | No | Yes | No | Yes | No | No |
| **AI agent tools** | No | No | Yes | No | No | No |
| **Real-time updates** | Partial | Yes | Yes | Partial | No | Yes |
| **Keyboard navigation** | Partial | Yes | No | Yes | Yes | Yes |
| **GPU acceleration** | No | No | No | No | No | Yes |
| **CRT/retro themes** | No | No | No | No | No | Yes |
| **Gamification** | No | Partial | No | No | No | Yes |
| **Offline capable** | Yes | No | No | No | Yes | No |
| **No upstream changes** | No | No | Yes | Yes | No | Yes |

**Legend:** Yes = Full support | Partial = Requires additional work | No = Not supported

---

## Recommended Technology Stack

### For AgentArena Integration

**Primary Layer: Option 2 (React Components)**
- `reactflow` - Interactive DAG visualization
- `@dnd-kit` - Drag-and-drop Kanban board
- `chart.js` + `react-chartjs-2` - Metrics and analytics charts
- `@tanstack/react-query` - Data fetching with automatic refresh

**Secondary Layer: Option 3 (MCP Server)**
- `FastMCP` (Python) - MCP server framework
- Claude Code skill binding for AI integration
- Server-Sent Events (SSE) for real-time updates

**Enhancement Layer: Option 6 (Exotic)**
- `xterm-addon-webgl` - GPU-accelerated terminal rendering
- Custom GLSL shaders for CRT effects
- Achievement/XP system for gamification

### Package Dependencies

```json
{
  "dependencies": {
    "reactflow": "^11.10.0",
    "@tanstack/react-table": "^8.x",
    "@tanstack/react-query": "^5.x",
    "chart.js": "^4.4.0",
    "react-chartjs-2": "^5.2.0",
    "zustand": "^4.4.0",
    "xterm-addon-webgl": "^0.16.0"
  }
}
```

---

## Implementation Roadmap

### Phase 1: Core Foundation (Weeks 1-4)

| Priority | Task | Status |
|:--------:|------|:------:|
| P0 | Beads API wrapper (Node.js backend) | Required |
| P0 | BeadsListView component | Required |
| P0 | BeadsKanbanView component | Required |
| P0 | BeadsGraphView with react-flow | Required |

### Phase 2: AI Integration (Weeks 5-6)

| Priority | Task | Status |
|:--------:|------|:------:|
| P1 | MCP Server for Claude Code | Recommended |
| P1 | Real-time SSE updates | Recommended |
| P1 | BeadsInsightsView (metrics dashboard) | Recommended |

### Phase 3: Visual Polish (Weeks 7-8)

| Priority | Task | Status |
|:--------:|------|:------:|
| P2 | GPU-accelerated rendering | Optional |
| P2 | CRT/Matrix visual themes | Optional |
| P2 | Gamification system (XP, achievements) | Optional |

### Phase 4: Extended Features (Week 9+)

| Priority | Task | Status |
|:--------:|------|:------:|
| P3 | TUI fallback via ttyd | Future |
| P3 | WebAssembly analysis engine | Future |
| P3 | Collaborative multi-user viewing | Future |

---

## Code Examples

### 1. Add Beads Tab to Navigation

```tsx
// dashboard/src/components/TabBar.tsx
const tabs: TabConfig[] = [
  { id: 'terminal', label: 'Terminal' },
  { id: 'files', label: 'Files' },
  { id: 'beads', label: 'Beads' },    // Add this line
  { id: 'status', label: 'Status' },
  { id: 'settings', label: 'Settings' },
];
```

### 2. Backend API Endpoint

```javascript
// api/beads-routes.js
app.get('/api/beads/project/:id/analysis', (req, res) => {
  const projectPath = `/code/${req.params.id}`;
  const output = execSync(`cd ${projectPath} && bv --robot-insights`);
  res.json(JSON.parse(output));
});
```

### 3. React Query Data Hook

```typescript
// hooks/useBeadsData.ts
import { useQuery } from '@tanstack/react-query';

export function useBeadsAnalysis(projectId: string) {
  return useQuery({
    queryKey: ['beads', projectId, 'analysis'],
    queryFn: async () => {
      const response = await fetch(`/api/beads/project/${projectId}/analysis`);
      if (!response.ok) throw new Error('Failed to fetch analysis');
      return response.json();
    },
    refetchInterval: 5000,  // Poll every 5 seconds
  });
}
```

### 4. MCP Server Tool Definition

```python
# beads-mcp/server.py
import subprocess
import json
from fastmcp import FastMCP

app = FastMCP("beads-server")

@app.tool
def beads_triage(repo_path: str = ".") -> dict:
    """Run beads triage analysis on a repository."""
    result = subprocess.run(
        ["bv", "--robot-triage"],
        cwd=repo_path,
        capture_output=True,
        text=True
    )
    return json.loads(result.stdout)
```

---

## Return on Investment Analysis

| Option | Development Effort | Business Value | ROI Score |
|--------|:------------------:|:--------------:|:---------:|
| React Components | 3-4 weeks | High | Excellent |
| MCP Server | 2-3 weeks | High | Excellent |
| Exotic Enhancements | 1-2 weeks | Medium | Very Good |
| Container/ttyd | 2-3 weeks | Medium | Good |
| WASM | 4-6 weeks | Medium | Good |
| Hybrid Desktop | 3-4 weeks | Low | Fair |

**Optimal Investment:** React + MCP + Exotic = 6-9 weeks total for maximum value

---

## Getting Started

1. **Review** the detailed analysis in [BEADS-INTEGRATION-MASTER-PLAN.md](./BEADS-BOB-BEADS-INTEGRATION-MASTER-PLAN.md)
2. **Select** your integration approach based on requirements above
3. **Implement** Phase 1 components (API wrapper and basic views)
4. **Iterate** by adding features incrementally based on user feedback

---

## Related Documentation

- [Integration Instructions](./INTEGRATION_INSTRUCTIONS.md) - Step-by-step setup guide
- [Removal Guide](./REMOVAL_GUIDE.md) - How to cleanly remove the integration
- [Master Plan](./BEADS-BOB-BEADS-INTEGRATION-MASTER-PLAN.md) - Comprehensive technical analysis

---

*Last updated: 2026-01-15*
