# Beads Viewer Integration Analysis for AgentArena

## Executive Summary

This document provides a comprehensive analysis for integrating [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) into AgentArena as a new tab, with the additional capability to use beads_viewer views as replacements for tmux sessions in the terminal windows.

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Integration Approaches](#2-integration-approaches)
3. [Approach Comparison Matrix](#3-approach-comparison-matrix)
4. [Detailed Implementation Plans](#4-detailed-implementation-plans)
5. [Recommendations](#5-recommendations)
6. [Risk Assessment](#6-risk-assessment)
7. [Action Plan](#7-action-plan)

---

## 1. System Overview

### 1.1 AgentArena Architecture

AgentArena is a containerized web application with:

| Component | Technology | Port |
|-----------|------------|------|
| Frontend | React 18 + TypeScript + Vite | 8080 (nginx) |
| Backend API | Node.js + Express | 3001 |
| Terminal Server | ttyd | 7681 |
| File Browser | FileBrowser | 8081 |
| Session Manager | tmux | Socket |
| Reverse Proxy | nginx | 8080 |

**Current Tab System:**
- `terminal` - Main view with SessionPanel + TerminalArea
- `files` - FileBrowser iframe
- `status` - API health monitoring
- `settings` - User preferences

**Window System:**
- 1-4 configurable terminal windows
- Each window can bind multiple sessions
- Sessions displayed via ttyd iframes pointing to tmux sessions

### 1.2 Beads Viewer Architecture

beads_viewer (`bv`) is a Go-based terminal UI with:

| Feature | Implementation |
|---------|----------------|
| Core | Go 1.25 + Charmbracelet TUI libraries |
| Data Format | `.beads/issues.jsonl` (JSONL) |
| Visualization | Terminal UI, Web Preview, SVG, Mermaid |
| AI Integration | Robot protocol with JSON output |
| Web Export | force-graph.js interactive graphs |
| Embedding | Built-in HTTP preview server |

**Key Integration Points:**
1. **Preview Server** (`pkg/export/preview.go`) - HTTP server with auto-port discovery (9000-9100)
2. **Robot Protocol** - `--robot-triage`, `--robot-insights`, `--robot-plan` commands
3. **WebAssembly Module** (`bv-graph-wasm/`) - Browser-based graph visualization
4. **Export Formats** - HTML bundles, SVG, Markdown, Mermaid

---

## 2. Integration Approaches

### Approach A: Iframe Embedding (Simple)

Embed beads_viewer's preview server output directly in an iframe.

```
┌─────────────────────────────────────────────────────────┐
│ AgentArena                                              │
│  ┌─────────────────────────────────────────────────┐   │
│  │ TabBar: [Terminal] [Files] [Beads] [Status]     │   │
│  └─────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────┐   │
│  │ <iframe src="http://localhost:9000">            │   │
│  │   ┌─────────────────────────────────────────┐   │   │
│  │   │ beads_viewer preview                    │   │   │
│  │   │ (force-graph.js visualization)          │   │   │
│  │   └─────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

**Implementation Steps:**
1. Add `bv` binary to Docker container
2. Create API endpoint to start/stop preview server
3. Add new "Beads" tab with iframe pointing to preview server
4. Handle lifecycle (start on tab open, stop on close)

**Pros:**
- Minimal code changes
- Leverages existing beads_viewer features
- Independent updates

**Cons:**
- Limited state sharing
- Additional process to manage
- Communication only via postMessage

---

### Approach B: TUI in tmux Session (Minimal Change)

Run `bv` TUI directly in a tmux session, displayed via existing ttyd infrastructure.

```
┌─────────────────────────────────────────────────────────┐
│ AgentArena                                              │
│  ┌───────────────────┬─────────────────────────────┐   │
│  │ Sessions:         │ Terminal Window             │   │
│  │  ☐ hq-mayor       │ ┌─────────────────────────┐ │   │
│  │  ☐ gt-gastown-... │ │ bv TUI in tmux          │ │   │
│  │  ★ beads-viewer   │ │ ┌───────────────────┐   │ │   │
│  │                   │ │ │ Issue Dashboard   │   │ │   │
│  │                   │ │ │ [Tab] [Tab] [Tab] │   │ │   │
│  │                   │ │ └───────────────────┘   │ │   │
│  │                   │ └─────────────────────────┘ │   │
│  └───────────────────┴─────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

**Implementation Steps:**
1. Add `bv` binary to Docker container
2. Create special session type: `tmux new-session -d -s beads-viewer "bv"`
3. Display in existing terminal window system
4. Add quick-launch button in sidebar

**Pros:**
- Zero frontend changes needed
- Uses existing infrastructure
- Full TUI functionality
- Keyboard-driven (matches bv design)

**Cons:**
- TUI, not web visualization
- No force-graph interactive views
- Standard terminal limitations

---

### Approach C: Hybrid Web Component (Recommended)

Create a dedicated React component that:
1. Runs `bv` commands via API to get data
2. Renders custom web visualization using fetched data
3. Can replace tmux sessions in window slots

```
┌─────────────────────────────────────────────────────────┐
│ AgentArena                                              │
│  ┌───────────────────┬─────────────────────────────┐   │
│  │ Sessions:         │ Window 1 (Beads View)       │   │
│  │  ☐ hq-mayor       │ ┌─────────────────────────┐ │   │
│  │  ☐ gt-gastown     │ │ [BeadsViewerComponent]  │ │   │
│  │ Views:            │ │ ┌───────────────────┐   │ │   │
│  │  ★ Beads Graph    │ │ │ React + D3.js     │   │ │   │
│  │  ★ Beads Kanban   │ │ │ Graph Viz         │   │ │   │
│  │                   │ │ └───────────────────┘   │ │   │
│  │                   │ └─────────────────────────┘ │   │
│  │                   ├─────────────────────────────┤   │
│  │                   │ Window 2 (tmux session)    │   │
│  │                   │ ┌─────────────────────────┐ │   │
│  │                   │ │ ttyd iframe             │ │   │
│  │                   │ └─────────────────────────┘ │   │
│  └───────────────────┴─────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

**Implementation Steps:**
1. Add `bv` binary to Docker container
2. Create API endpoints for robot protocol commands
3. Build React components for visualization
4. Extend window system to support "beads views" alongside tmux sessions
5. Add view selection in sidebar

---

### Approach D: Full Tab with Multiple Views

Add a comprehensive "Beads" tab with multiple sub-views and its own navigation.

```
┌─────────────────────────────────────────────────────────┐
│ AgentArena                                              │
│  ┌─────────────────────────────────────────────────┐   │
│  │ [Terminal] [Files] [★Beads★] [Status] [Settings]│   │
│  └─────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Beads Tab                                        │   │
│  │  ┌──────────────────────────────────────────┐   │   │
│  │  │ Sub-tabs: [Graph] [Kanban] [Triage] [Raw]│   │   │
│  │  └──────────────────────────────────────────┘   │   │
│  │  ┌──────────────────────────────────────────┐   │   │
│  │  │ Graph View:                              │   │   │
│  │  │  ┌────────────────────────────────────┐  │   │   │
│  │  │  │ force-graph.js / D3.js             │  │   │   │
│  │  │  │ Interactive dependency graph       │  │   │   │
│  │  │  └────────────────────────────────────┘  │   │   │
│  │  │  [Refresh] [Export] [Filter: ▼]          │   │   │
│  │  └──────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

---

## 3. Approach Comparison Matrix

| Criteria | A: Iframe | B: TUI in tmux | C: Hybrid Component | D: Full Tab |
|----------|-----------|----------------|---------------------|-------------|
| **Implementation Effort** | Low | Very Low | Medium | High |
| **Frontend Changes** | Minimal | None | Moderate | Extensive |
| **Backend Changes** | Low | None | Moderate | Moderate |
| **Visualization Quality** | High (native) | Medium (TUI) | High (custom) | Highest |
| **State Integration** | Poor | None | Excellent | Excellent |
| **Replace tmux Windows** | No | N/A (IS tmux) | Yes | Partial |
| **Maintenance** | Low | Very Low | Medium | High |
| **User Experience** | Good | Fair | Very Good | Excellent |
| **Keyboard Navigation** | Limited | Excellent | Moderate | Moderate |
| **Performance** | Good | Excellent | Good | Good |
| **Offline Support** | Yes | Yes | Yes | Yes |

### Scoring (1-10)

| Criteria (Weight) | A | B | C | D |
|-------------------|---|---|---|---|
| Ease of Implementation (3) | 9 | 10 | 6 | 4 |
| Feature Completeness (3) | 6 | 7 | 8 | 10 |
| Integration Quality (2) | 4 | 3 | 9 | 8 |
| Maintainability (2) | 8 | 10 | 7 | 5 |
| **Weighted Total** | **67** | **72** | **74** | **68** |

---

## 4. Detailed Implementation Plans

### 4.1 Plan A: Iframe Embedding

#### Backend Changes

**File: `/api/server.js`**
```javascript
// Add new endpoints
const { spawn, execSync } = require('child_process');
let bvProcess = null;
let bvPort = null;

app.post('/api/beads/start', (req, res) => {
  if (bvProcess) {
    return res.json({ status: 'running', port: bvPort });
  }

  const projectPath = req.body.projectPath || '/workspace';
  bvPort = 9000 + Math.floor(Math.random() * 100);

  bvProcess = spawn('bv', [
    '--preview',
    `--preview-port=${bvPort}`,
    '--no-auto-launch',
    projectPath
  ]);

  bvProcess.on('exit', () => {
    bvProcess = null;
    bvPort = null;
  });

  res.json({ status: 'started', port: bvPort });
});

app.post('/api/beads/stop', (req, res) => {
  if (bvProcess) {
    bvProcess.kill();
    bvProcess = null;
  }
  res.json({ status: 'stopped' });
});

app.get('/api/beads/status', (req, res) => {
  res.json({
    running: bvProcess !== null,
    port: bvPort
  });
});
```

#### Frontend Changes

**File: `src/components/BeadsView.tsx`**
```typescript
import { useState, useEffect } from 'react';

export function BeadsView() {
  const [port, setPort] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    startBeadsViewer();
    return () => stopBeadsViewer();
  }, []);

  const startBeadsViewer = async () => {
    try {
      const res = await fetch('/api/beads/start', { method: 'POST' });
      const data = await res.json();
      setPort(data.port);
      setLoading(false);
    } catch (e) {
      setError('Failed to start beads viewer');
      setLoading(false);
    }
  };

  const stopBeadsViewer = async () => {
    await fetch('/api/beads/stop', { method: 'POST' });
  };

  if (loading) return <div className="loading">Starting Beads Viewer...</div>;
  if (error) return <div className="error">{error}</div>;

  return (
    <div className="beads-view">
      <iframe
        src={`http://localhost:${port}`}
        className="beads-iframe"
        title="Beads Viewer"
      />
    </div>
  );
}
```

**File: `src/App.tsx` (additions)**
```typescript
import { BeadsView } from './components/BeadsView';

// In render:
{activeTab === 'beads' && <BeadsView />}
```

**File: `src/components/TabBar.tsx` (additions)**
```typescript
const tabs: TabConfig[] = [
  { id: 'terminal', label: 'Terminal' },
  { id: 'files', label: 'Files' },
  { id: 'beads', label: 'Beads' },  // NEW
  { id: 'status', label: 'Status' },
  { id: 'settings', label: 'Settings' },
];
```

#### Docker Changes

**File: `Dockerfile` (additions)**
```dockerfile
# Install beads_viewer
RUN curl -fsSL https://raw.githubusercontent.com/Dicklesworthstone/beads_viewer/main/install.sh | bash
# Or build from source:
# RUN go install github.com/Dicklesworthstone/beads_viewer/cmd/bv@latest
```

---

### 4.2 Plan B: TUI in tmux Session

#### Backend Changes

**File: `/api/server.js` (additions)**
```javascript
app.post('/api/tmux/sessions/beads', (req, res) => {
  const sessionName = 'beads-viewer';
  const projectPath = req.body.projectPath || '/workspace';

  try {
    // Check if session exists
    try {
      execSync(`tmux has-session -t ${sessionName} 2>/dev/null`);
      res.json({ status: 'exists', name: sessionName });
      return;
    } catch (e) {
      // Session doesn't exist, create it
    }

    execSync(`tmux new-session -d -s ${sessionName} "bv ${projectPath}"`);
    res.json({ status: 'created', name: sessionName });
  } catch (e) {
    res.status(500).json({ error: e.message });
  }
});
```

#### Frontend Changes

**File: `src/components/SessionPanel.tsx` (additions)**
```typescript
const launchBeadsViewer = async () => {
  const res = await fetch('/api/tmux/sessions/beads', { method: 'POST' });
  const data = await res.json();
  // Auto-add to focused window
  addSessionToWindow(focusedWindowIndex, data.name);
};

// In render, add button:
<button onClick={launchBeadsViewer} className="beads-launcher">
  Launch Beads Viewer
</button>
```

---

### 4.3 Plan C: Hybrid Component (RECOMMENDED)

This approach provides the best balance of integration and functionality.

#### Backend API Endpoints

**File: `/api/server.js` (new endpoints)**
```javascript
// Get issues data via robot protocol
app.get('/api/beads/triage', async (req, res) => {
  const projectPath = req.query.path || '/workspace';
  try {
    const result = execSync(`bv --robot-triage ${projectPath}`, {
      encoding: 'utf-8',
      maxBuffer: 10 * 1024 * 1024
    });
    res.json(JSON.parse(result));
  } catch (e) {
    res.status(500).json({ error: e.message });
  }
});

app.get('/api/beads/insights', async (req, res) => {
  const projectPath = req.query.path || '/workspace';
  try {
    const result = execSync(`bv --robot-insights ${projectPath}`, {
      encoding: 'utf-8',
      maxBuffer: 10 * 1024 * 1024
    });
    res.json(JSON.parse(result));
  } catch (e) {
    res.status(500).json({ error: e.message });
  }
});

app.get('/api/beads/plan', async (req, res) => {
  const projectPath = req.query.path || '/workspace';
  try {
    const result = execSync(`bv --robot-plan ${projectPath}`, {
      encoding: 'utf-8',
      maxBuffer: 10 * 1024 * 1024
    });
    res.json(JSON.parse(result));
  } catch (e) {
    res.status(500).json({ error: e.message });
  }
});

// Get raw issues for custom visualization
app.get('/api/beads/issues', async (req, res) => {
  const projectPath = req.query.path || '/workspace';
  const beadsPath = path.join(projectPath, '.beads', 'issues.jsonl');

  try {
    const content = fs.readFileSync(beadsPath, 'utf-8');
    const issues = content.trim().split('\n').map(line => JSON.parse(line));
    res.json({ issues });
  } catch (e) {
    res.status(500).json({ error: e.message });
  }
});
```

#### Frontend Components

**File: `src/types.ts` (additions)**
```typescript
export interface BeadsIssue {
  id: string;
  title: string;
  description: string;
  status: 'open' | 'in_progress' | 'blocked' | 'deferred' | 'pinned' | 'hooked' | 'closed' | 'tombstone';
  priority: number;
  type: 'bug' | 'feature' | 'task' | 'epic' | 'chore';
  dependencies: string[];
  blocking: string[];
  labels: string[];
  assignees: string[];
  created: string;
  updated: string;
}

export interface BeadsTriage {
  recommendations: Array<{
    issueId: string;
    rank: number;
    reasoning: string;
    unblockChain: string[];
  }>;
  quickWins: string[];
  blockers: string[];
}

export interface BeadsInsights {
  metrics: {
    pageRank: Record<string, number>;
    betweenness: Record<string, number>;
    criticalPath: string[];
    cycles: string[][];
  };
  health: {
    score: number;
    risks: string[];
  };
}

// Extend window system for beads views
export type WindowContentType = 'tmux' | 'beads-graph' | 'beads-kanban' | 'beads-triage';

export interface TerminalWindow {
  id: string;
  contentType: WindowContentType;  // NEW
  boundSessions: string[];
  activeSession: string | null;
  beadsView?: {  // NEW
    type: 'graph' | 'kanban' | 'triage';
    projectPath: string;
  };
  colorIndex: number;
}
```

**File: `src/components/BeadsGraphView.tsx`**
```typescript
import { useEffect, useRef, useState } from 'react';
import ForceGraph2D from 'react-force-graph-2d';
import { BeadsIssue } from '../types';

interface Props {
  projectPath: string;
}

interface GraphNode {
  id: string;
  name: string;
  status: string;
  priority: number;
  type: string;
}

interface GraphLink {
  source: string;
  target: string;
  type: 'blocks' | 'depends';
}

export function BeadsGraphView({ projectPath }: Props) {
  const [issues, setIssues] = useState<BeadsIssue[]>([]);
  const [graphData, setGraphData] = useState<{ nodes: GraphNode[], links: GraphLink[] }>({ nodes: [], links: [] });
  const [loading, setLoading] = useState(true);
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
  const graphRef = useRef<any>();

  useEffect(() => {
    fetchIssues();
  }, [projectPath]);

  const fetchIssues = async () => {
    try {
      const res = await fetch(`/api/beads/issues?path=${encodeURIComponent(projectPath)}`);
      const data = await res.json();
      setIssues(data.issues);
      buildGraph(data.issues);
      setLoading(false);
    } catch (e) {
      console.error('Failed to fetch issues:', e);
      setLoading(false);
    }
  };

  const buildGraph = (issues: BeadsIssue[]) => {
    const nodes: GraphNode[] = issues.map(issue => ({
      id: issue.id,
      name: issue.title,
      status: issue.status,
      priority: issue.priority,
      type: issue.type
    }));

    const links: GraphLink[] = [];
    issues.forEach(issue => {
      issue.dependencies?.forEach(depId => {
        links.push({ source: depId, target: issue.id, type: 'depends' });
      });
      issue.blocking?.forEach(blockId => {
        links.push({ source: issue.id, target: blockId, type: 'blocks' });
      });
    });

    setGraphData({ nodes, links });
  };

  const getNodeColor = (node: GraphNode) => {
    const statusColors: Record<string, string> = {
      open: '#4CAF50',
      in_progress: '#2196F3',
      blocked: '#f44336',
      closed: '#9E9E9E',
      deferred: '#FF9800'
    };
    return statusColors[node.status] || '#666';
  };

  if (loading) return <div className="beads-loading">Loading graph...</div>;

  return (
    <div className="beads-graph-view">
      <div className="beads-graph-toolbar">
        <button onClick={fetchIssues}>Refresh</button>
        <span>{issues.length} issues</span>
      </div>
      <ForceGraph2D
        ref={graphRef}
        graphData={graphData}
        nodeColor={getNodeColor}
        nodeLabel={(node: any) => node.name}
        linkColor={() => 'rgba(255,255,255,0.2)'}
        linkDirectionalArrowLength={3}
        linkDirectionalArrowRelPos={1}
        onNodeClick={(node: any) => setSelectedNode(node)}
        width={800}
        height={600}
      />
      {selectedNode && (
        <div className="beads-node-detail">
          <h4>{selectedNode.name}</h4>
          <p>Status: {selectedNode.status}</p>
          <p>Priority: {selectedNode.priority}</p>
          <p>Type: {selectedNode.type}</p>
        </div>
      )}
    </div>
  );
}
```

**File: `src/components/BeadsTriageView.tsx`**
```typescript
import { useEffect, useState } from 'react';
import { BeadsTriage } from '../types';

interface Props {
  projectPath: string;
}

export function BeadsTriageView({ projectPath }: Props) {
  const [triage, setTriage] = useState<BeadsTriage | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchTriage();
  }, [projectPath]);

  const fetchTriage = async () => {
    try {
      const res = await fetch(`/api/beads/triage?path=${encodeURIComponent(projectPath)}`);
      const data = await res.json();
      setTriage(data);
      setLoading(false);
    } catch (e) {
      console.error('Failed to fetch triage:', e);
      setLoading(false);
    }
  };

  if (loading) return <div className="beads-loading">Running triage analysis...</div>;
  if (!triage) return <div className="beads-error">Failed to load triage data</div>;

  return (
    <div className="beads-triage-view">
      <section className="quick-wins">
        <h3>Quick Wins</h3>
        <ul>
          {triage.quickWins.map(id => (
            <li key={id}>{id}</li>
          ))}
        </ul>
      </section>

      <section className="blockers">
        <h3>Current Blockers</h3>
        <ul>
          {triage.blockers.map(id => (
            <li key={id}>{id}</li>
          ))}
        </ul>
      </section>

      <section className="recommendations">
        <h3>Recommendations</h3>
        {triage.recommendations.map((rec, idx) => (
          <div key={rec.issueId} className="recommendation">
            <span className="rank">#{rec.rank}</span>
            <span className="issue-id">{rec.issueId}</span>
            <p className="reasoning">{rec.reasoning}</p>
          </div>
        ))}
      </section>
    </div>
  );
}
```

**File: `src/components/TerminalWindow.tsx` (modified)**
```typescript
import { BeadsGraphView } from './BeadsGraphView';
import { BeadsTriageView } from './BeadsTriageView';
import { BeadsKanbanView } from './BeadsKanbanView';

export function TerminalWindow({ window, index }: Props) {
  // Existing code...

  // NEW: Render beads views instead of ttyd iframe
  const renderContent = () => {
    if (window.contentType === 'beads-graph') {
      return <BeadsGraphView projectPath={window.beadsView?.projectPath || '/workspace'} />;
    }
    if (window.contentType === 'beads-kanban') {
      return <BeadsKanbanView projectPath={window.beadsView?.projectPath || '/workspace'} />;
    }
    if (window.contentType === 'beads-triage') {
      return <BeadsTriageView projectPath={window.beadsView?.projectPath || '/workspace'} />;
    }

    // Default: tmux session via ttyd
    return (
      <iframe
        src={`/terminal/?arg=${activeSession}`}
        className="terminal-iframe"
        title={`Terminal ${index}`}
      />
    );
  };

  return (
    <div className={`terminal-window window-${colorIndex}`}>
      <div className="window-header">
        {/* Session tags or beads view title */}
      </div>
      <div className="window-content">
        {renderContent()}
      </div>
    </div>
  );
}
```

**File: `src/context/SessionContext.tsx` (additions)**
```typescript
// Add actions for beads views
const setWindowContent = (windowIndex: number, contentType: WindowContentType, beadsView?: BeadsViewConfig) => {
  setState(prev => ({
    ...prev,
    windows: prev.windows.map((w, i) =>
      i === windowIndex
        ? { ...w, contentType, beadsView }
        : w
    )
  }));
};

// Expose in context
return (
  <SessionContext.Provider value={{
    ...existingValue,
    setWindowContent,  // NEW
  }}>
    {children}
  </SessionContext.Provider>
);
```

**File: `src/components/SessionPanel.tsx` (additions)**
```typescript
// Add beads view launchers
const beadsViews = [
  { id: 'beads-graph', label: 'Dependency Graph', icon: '🕸️' },
  { id: 'beads-kanban', label: 'Kanban Board', icon: '📋' },
  { id: 'beads-triage', label: 'Triage View', icon: '🎯' },
];

// In render:
<div className="beads-views-section">
  <h4>Beads Views</h4>
  {beadsViews.map(view => (
    <div
      key={view.id}
      className="beads-view-item"
      onClick={() => setWindowContent(focusedWindowIndex, view.id, {
        type: view.id.replace('beads-', ''),
        projectPath: '/workspace'
      })}
    >
      <span className="icon">{view.icon}</span>
      <span className="label">{view.label}</span>
    </div>
  ))}
</div>
```

#### Package.json Dependencies

```json
{
  "dependencies": {
    "react-force-graph-2d": "^1.25.0",
    "d3": "^7.8.5"
  }
}
```

---

### 4.4 Plan D: Full Tab Implementation

Extends Plan C with a dedicated tab and sub-navigation.

**File: `src/components/BeadsTab.tsx`**
```typescript
import { useState } from 'react';
import { BeadsGraphView } from './BeadsGraphView';
import { BeadsKanbanView } from './BeadsKanbanView';
import { BeadsTriageView } from './BeadsTriageView';
import { BeadsInsightsView } from './BeadsInsightsView';

type SubTab = 'graph' | 'kanban' | 'triage' | 'insights';

export function BeadsTab() {
  const [activeSubTab, setActiveSubTab] = useState<SubTab>('graph');
  const [projectPath, setProjectPath] = useState('/workspace');

  const subTabs: { id: SubTab; label: string }[] = [
    { id: 'graph', label: 'Dependency Graph' },
    { id: 'kanban', label: 'Kanban Board' },
    { id: 'triage', label: 'AI Triage' },
    { id: 'insights', label: 'Insights' },
  ];

  return (
    <div className="beads-tab">
      <div className="beads-tab-header">
        <div className="sub-tabs">
          {subTabs.map(tab => (
            <button
              key={tab.id}
              className={`sub-tab ${activeSubTab === tab.id ? 'active' : ''}`}
              onClick={() => setActiveSubTab(tab.id)}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="project-selector">
          <label>Project:</label>
          <select value={projectPath} onChange={e => setProjectPath(e.target.value)}>
            <option value="/workspace">/workspace</option>
            {/* Dynamically populate with discovered .beads directories */}
          </select>
        </div>
      </div>

      <div className="beads-tab-content">
        {activeSubTab === 'graph' && <BeadsGraphView projectPath={projectPath} />}
        {activeSubTab === 'kanban' && <BeadsKanbanView projectPath={projectPath} />}
        {activeSubTab === 'triage' && <BeadsTriageView projectPath={projectPath} />}
        {activeSubTab === 'insights' && <BeadsInsightsView projectPath={projectPath} />}
      </div>
    </div>
  );
}
```

---

## 5. Recommendations

### Primary Recommendation: Plan C (Hybrid Component)

**Rationale:**
1. **Best Integration** - Beads views can replace tmux sessions in windows
2. **Balanced Effort** - Moderate implementation cost with high value
3. **Future-Proof** - Can evolve into Plan D if needed
4. **Maintains UX** - Consistent with existing window management paradigm
5. **Data Control** - Full control over visualization and state

### Secondary Recommendation: Start with Plan B for Quick Win

If time-constrained, implement Plan B first:
- Zero frontend changes
- Immediate functionality
- Can coexist with Plan C implementation

### Phased Implementation Strategy

```
Phase 1 (Quick Win):
├── Install bv binary in container
├── Add API endpoint for beads session
└── Add launcher button in sidebar

Phase 2 (Core Integration):
├── Add robot protocol API endpoints
├── Create BeadsGraphView component
├── Extend window system for beads content types
└── Add view selection in sidebar

Phase 3 (Full Feature):
├── Add BeadsKanbanView component
├── Add BeadsTriageView component
├── Add BeadsInsightsView component
├── Create dedicated Beads tab with sub-navigation
└── Add project selector for multi-repo support
```

---

## 6. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| bv binary compatibility in container | Low | High | Test in Docker build early |
| Robot protocol JSON changes | Medium | Medium | Version pin bv, add error handling |
| Performance with large issue sets | Medium | Medium | Pagination, lazy loading |
| force-graph.js bundle size | Low | Low | Code splitting, lazy import |
| tmux/beads mode confusion | Medium | Low | Clear UI indicators |

---

## 7. Action Plan

### Immediate Actions (Phase 1)

1. **Docker Configuration**
   - [ ] Add bv installation to Dockerfile
   - [ ] Verify bv runs correctly in container
   - [ ] Test robot protocol commands

2. **API Development**
   - [ ] Add `/api/beads/issues` endpoint
   - [ ] Add `/api/beads/triage` endpoint
   - [ ] Add `/api/beads/insights` endpoint
   - [ ] Add `/api/tmux/sessions/beads` endpoint

3. **Frontend Foundation**
   - [ ] Update types.ts with beads types
   - [ ] Add contentType to TerminalWindow interface
   - [ ] Add setWindowContent action to context

### Short-term Actions (Phase 2)

4. **Core Components**
   - [ ] Create BeadsGraphView component
   - [ ] Install react-force-graph-2d dependency
   - [ ] Modify TerminalWindow to render beads views
   - [ ] Add beads views section to SessionPanel

5. **Testing**
   - [ ] Test with sample .beads/issues.jsonl
   - [ ] Verify graph rendering performance
   - [ ] Test window switching between tmux and beads

### Medium-term Actions (Phase 3)

6. **Additional Views**
   - [ ] Create BeadsKanbanView component
   - [ ] Create BeadsTriageView component
   - [ ] Create BeadsInsightsView component

7. **Full Tab**
   - [ ] Create BeadsTab component with sub-navigation
   - [ ] Add to TabBar
   - [ ] Add project discovery API

8. **Polish**
   - [ ] Add theming support for beads components
   - [ ] Add keyboard shortcuts
   - [ ] Add documentation

---

## Appendix A: File Structure After Implementation

```
AgentArena/
├── api/
│   └── server.js                    # + beads endpoints
├── dashboard/
│   └── src/
│       ├── types.ts                 # + beads types
│       ├── context/
│       │   └── SessionContext.tsx   # + setWindowContent
│       └── components/
│           ├── BeadsGraphView.tsx   # NEW
│           ├── BeadsKanbanView.tsx  # NEW
│           ├── BeadsTriageView.tsx  # NEW
│           ├── BeadsInsightsView.tsx # NEW
│           ├── BeadsTab.tsx         # NEW (Phase 3)
│           ├── TerminalWindow.tsx   # modified
│           ├── SessionPanel.tsx     # modified
│           └── TabBar.tsx           # modified
├── Dockerfile                       # + bv installation
└── beads_viewer_integration/
    └── INTEGRATION_ANALYSIS.md      # this file
```

---

## Appendix B: API Reference

### Beads API Endpoints

| Endpoint | Method | Description | Response |
|----------|--------|-------------|----------|
| `/api/beads/issues` | GET | Get all issues | `{ issues: BeadsIssue[] }` |
| `/api/beads/triage` | GET | Get AI triage recommendations | `BeadsTriage` |
| `/api/beads/insights` | GET | Get graph metrics | `BeadsInsights` |
| `/api/beads/plan` | GET | Get execution plan | `BeadsPlan` |
| `/api/tmux/sessions/beads` | POST | Launch bv TUI session | `{ name: string }` |

**Query Parameters:**
- `path` - Project path containing .beads directory (default: `/workspace`)

---

## Appendix C: beads_viewer CLI Reference

```bash
# Robot protocol commands (JSON output)
bv --robot-triage /path      # Prioritized recommendations
bv --robot-insights /path    # Graph metrics
bv --robot-plan /path        # Execution tracks
bv --robot-priority /path    # Priority analysis

# Preview server
bv --preview --preview-port=9000 --no-auto-launch /path

# Export formats
bv --export-mermaid /path    # Mermaid diagram
bv --export-html /path       # Interactive HTML
bv --export-svg /path        # Static SVG graph
```

---

*Document generated: 2026-01-14*
*AgentArena Version: Current*
*beads_viewer Version: Latest (GitHub main)*
