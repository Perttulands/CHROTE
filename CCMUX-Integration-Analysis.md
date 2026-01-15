# CCMUX Integration Analysis

## Comprehensive Analysis and Action Plan for Integrating ccmux into AgentArena

**Document Version:** 1.0
**Date:** 2026-01-14
**Project:** AgentArena Terminal Multiplexer Integration

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [What is ccmux?](#what-is-ccmux)
3. [Current AgentArena Architecture](#current-agentarena-architecture)
4. [Integration Options Overview](#integration-options-overview)
5. [Frontend Integration Approaches](#frontend-integration-approaches)
6. [Backend Integration Approaches](#backend-integration-approaches)
7. [Deployment Considerations](#deployment-considerations)
8. [Comparison Matrix](#comparison-matrix)
9. [Recommendations](#recommendations)
10. [Implementation Roadmap](#implementation-roadmap)

---

## Executive Summary

This document provides a comprehensive analysis of integrating [ccmux](https://github.com/brendanbecker/ccmux), a Claude Code-aware terminal multiplexer, into the AgentArena platform. ccmux offers significant advantages over the current tmux-based implementation, including:

- **Claude State Detection**: Identifies whether Claude is idle, thinking, using tools, streaming, or complete
- **Session Recovery**: Write-ahead logging enables crash recovery with scrollback preservation
- **MCP Integration**: 30 tools across 7 categories for programmatic control
- **Sideband Protocol**: Claude can emit control commands directly in output

The analysis evaluates multiple frontend and backend integration approaches, weighing complexity, performance, security, and user experience trade-offs.

**Key Recommendation**: Adopt a **phased hybrid approach** starting with a drop-in tmux replacement using ttyd/iframe embedding (minimal changes), then evolving to a native xterm.js + WebSocket implementation with full MCP integration.

---

## What is ccmux?

### Core Features

| Feature | Description |
|---------|-------------|
| **State Detection** | Detects 5 Claude states: Idle, Thinking, ToolUse, Streaming, Complete |
| **Session Isolation** | Each pane gets unique `CLAUDE_CONFIG_DIR` preventing conflicts |
| **WAL Persistence** | Write-ahead logging preserves sessions and 1000 lines scrollback |
| **MCP Bridge** | JSON-RPC protocol with 30 tools for programmatic control |
| **Sideband Protocol** | XML-like commands (`<ccmux:spawn>`) in stdout for direct control |

### Architecture Components

```
┌─────────────────────────────────────────────────────────────────────┐
│                        ccmux System                                 │
├─────────────────────────────────────────────────────────────────────┤
│  ccmux-client     │ Terminal UI (ratatui + crossterm)               │
│  ccmux-server     │ Daemon managing PTY allocation + MCP bridge     │
│  ccmux-protocol   │ Message types + bincode serialization           │
│  ccmux-session    │ Hierarchy: sessions → windows → panes           │
│  ccmux-utils      │ Shared utilities                                │
│  ccmux-persistence│ Write-ahead logging for crash recovery          │
└─────────────────────────────────────────────────────────────────────┘
```

### Communication Model

- **Client-Server**: Unix domain sockets (`~/.ccmux/ccmux.sock`)
- **Serialization**: bincode for efficient binary encoding
- **MCP Bridge**: JSON-RPC protocol translation

### MCP Tool Categories (30 tools)

| Category | Tools | Description |
|----------|-------|-------------|
| Sessions | 5 | Create, list, attach, detach, destroy |
| Windows | 4 | Create, navigate, rename, close |
| Panes | 5 | Split, navigate, resize, close, zoom |
| I/O | 3 | Send keys, capture output, paste |
| Layout | 3 | Apply layouts, get/set dimensions |
| Environment | 2 | Get/set environment variables |
| Orchestration | 6 | Agent spawning, coordination |

---

## Current AgentArena Architecture

### Tech Stack Overview

| Layer | Technology |
|-------|------------|
| **Frontend** | React 18.2 + TypeScript 5.3, Vite 5.0 |
| **Terminal UI** | xterm.js 5.5 (via ttyd iframe) |
| **Backend API** | Node.js/Express 4.18 (197 lines) |
| **Multiplexer** | tmux (current) |
| **Web Terminal** | ttyd 7681 |
| **Reverse Proxy** | nginx 8080 |
| **Container** | Docker (Ubuntu 24.04 base) |

### Current Terminal Flow

```
React UI → iframe URL with ?arg=session_name
         → nginx /terminal/ proxy (WebSocket)
         → ttyd port 7681 (web terminal)
         → terminal-launch.sh script
         → tmux attach-session
```

### Key Integration Points for ccmux

1. **API Server** (`api/server.js`): Replace tmux commands with ccmux equivalents
2. **Terminal Launch Script**: Update to use `ccmux attach-session`
3. **Session Listing**: Query ccmux instead of tmux
4. **Environment Variables**: Replace `TMUX_TMPDIR` with ccmux socket path

### Current Files to Modify

| File | Purpose | Changes Needed |
|------|---------|----------------|
| `api/server.js` | Session API | ccmux command translation |
| `build1.dockerfile` | Container setup | Install ccmux, update entrypoint |
| `nginx/nginx.conf` | Routing | Potentially add MCP proxy route |
| `dashboard/src/hooks/useTmuxSessions.ts` | Data fetching | Update API response handling |

---

## Integration Options Overview

### Frontend Options

| Approach | Complexity | UX Quality | Performance | Cross-Platform |
|----------|------------|------------|-------------|----------------|
| **A. xterm.js + WebSocket** | Medium | Excellent | Good | Excellent |
| **B. Electron Desktop** | Medium-High | Excellent | Excellent | Good |
| **C. Browser Extension** | High | Good | Good | Medium |
| **D. Iframe/ttyd (Current)** | Low | Good | Good | Excellent |

### Backend Options

| Approach | Complexity | Scalability | Latency | Maintenance |
|----------|------------|-------------|---------|-------------|
| **1. Unix Socket Proxy** | Medium | Medium | Low | Medium |
| **2. MCP Bridge** | Low-Medium | High | Low | Low |
| **3. REST API Wrapper** | Low | Medium | Medium | Low |
| **4. gRPC/GraphQL** | Medium-High | High | Low | Medium |
| **5. Docker Container** | High | High | Medium | Medium-High |

---

## Frontend Integration Approaches

### Option A: xterm.js + WebSocket Bridge

**Architecture:**
```
┌─────────────────────────────────────────────────────────────┐
│                      Browser                                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                   xterm.js                          │   │
│  │  - Terminal rendering (WebGL accelerated)           │   │
│  │  - Input handling                                   │   │
│  │  - ANSI escape sequence parsing                     │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                  │
│                    WebSocket                                │
└──────────────────────────┼──────────────────────────────────┘
                           │
┌──────────────────────────┼──────────────────────────────────┐
│              ccmux WebSocket Bridge                         │
│  - Session management                                       │
│  - PTY multiplexing                                         │
│  - State synchronization                                    │
│  - bincode ↔ JSON translation                               │
└──────────────────────────┼──────────────────────────────────┘
                           │
┌──────────────────────────┼──────────────────────────────────┐
│                    ccmux daemon                             │
└─────────────────────────────────────────────────────────────┘
```

**Implementation:**

```typescript
// React component
import { Terminal } from 'xterm';
import { WebglAddon } from 'xterm-addon-webgl';
import { FitAddon } from 'xterm-addon-fit';

class CcmuxTerminal {
  private term: Terminal;
  private ws: WebSocket;
  private fitAddon: FitAddon;

  constructor(container: HTMLElement, sessionId: string) {
    this.term = new Terminal({
      cursorBlink: true,
      fontFamily: 'JetBrains Mono, monospace',
      fontSize: 14,
      theme: { background: '#1e1e1e', foreground: '#d4d4d4' }
    });

    this.fitAddon = new FitAddon();
    this.term.loadAddon(this.fitAddon);
    this.term.loadAddon(new WebglAddon());

    this.term.open(container);
    this.fitAddon.fit();
    this.connect(sessionId);
  }

  private connect(sessionId: string) {
    this.ws = new WebSocket(`wss://arena/ccmux/${sessionId}`);

    this.ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      if (msg.type === 'output') {
        this.term.write(msg.data);
      } else if (msg.type === 'state') {
        this.onStateChange(msg.claudeState);
      }
    };

    this.term.onData((data) => {
      this.ws.send(JSON.stringify({ type: 'input', data }));
    });
  }

  private onStateChange(state: 'Idle' | 'Thinking' | 'ToolUse' | 'Streaming' | 'Complete') {
    // Update UI indicators based on Claude state
  }
}
```

**Dependencies:**
- `xterm` (~300KB)
- `xterm-addon-fit`
- `xterm-addon-webgl`
- `xterm-addon-unicode11`

**Pros:**
- Full control over terminal rendering and behavior
- Direct access to Claude state information
- Native WebGL acceleration
- No iframe limitations
- Excellent for custom UI features

**Cons:**
- Requires building WebSocket bridge server
- More complex than iframe approach
- Need to handle reconnection logic

---

### Option B: Electron Desktop App

**Architecture:**
```
┌─────────────────────────────────────────────────────────────┐
│                    Electron App                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │               Renderer Process                      │   │
│  │  - xterm.js / React                                 │   │
│  │  - UI rendering                                     │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │ IPC                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                Main Process                         │   │
│  │  - node-pty / Native Module                         │   │
│  │  - Direct PTY communication                         │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                  │
│                    Unix Socket                              │
└──────────────────────────┼──────────────────────────────────┘
                           │
┌──────────────────────────┼──────────────────────────────────┐
│                      ccmux daemon                           │
└─────────────────────────────────────────────────────────────┘
```

**Pros:**
- Native performance (direct PTY)
- Full OS integration (notifications, clipboard, global shortcuts)
- Offline capable
- Auto-update mechanism

**Cons:**
- Large distribution size (~150MB)
- Memory overhead (Chromium)
- Build complexity (native modules per platform)
- Separate install required

**Best For:** Power users, developers who need native desktop experience

---

### Option C: Browser Extension

**Architecture:**
```
┌─────────────────────────────────────────────────────────────┐
│                      Browser Extension                      │
│  ┌───────────────┐  ┌───────────────────────────┐          │
│  │ Background    │  │    Content Script         │          │
│  │ Service Worker│  │    (claude.ai page)       │          │
│  │ - Native      │  │  - UI injection           │          │
│  │   messaging   │  │  - Terminal panel         │          │
│  └───────────────┘  └───────────────────────────┘          │
│          │                                                  │
│          │ Native Messaging Protocol                        │
└──────────┼──────────────────────────────────────────────────┘
           │
┌──────────┼──────────────────────────────────────────────────┐
│          │ ccmux-browser-bridge                             │
│          │ - JSON message handling                          │
│          │ - Session management                             │
└──────────┼──────────────────────────────────────────────────┘
           │
┌──────────┼──────────────────────────────────────────────────┐
│                      ccmux daemon                           │
└─────────────────────────────────────────────────────────────┘
```

**Pros:**
- Browser integration with claude.ai
- Lightweight footprint
- Cross-browser potential

**Cons:**
- Complex setup (extension + native host)
- Extension store review process
- Manifest V3 limitations
- Trust concerns

**Best For:** Direct claude.ai integration scenarios

---

### Option D: Iframe/ttyd (Current Pattern)

**Architecture:**
```
┌─────────────────────────────────────────────────────────────┐
│                    Host Application                         │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                Parent Window                        │   │
│  │  - Navigation, session management UI                │   │
│  │  ┌───────────────────────────────────────────────┐ │   │
│  │  │              <iframe>                         │ │   │
│  │  │  ┌─────────────────────────────────────────┐ │ │   │
│  │  │  │           ttyd                          │ │ │   │
│  │  │  │  - Terminal emulation                   │ │ │   │
│  │  │  │  - Runs: ccmux attach --session X       │ │ │   │
│  │  │  └─────────────────────────────────────────┘ │ │   │
│  │  └───────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Implementation Changes for ccmux:**

```bash
# Update terminal-launch.sh
#!/bin/bash
SESSION_NAME="${1:-main}"
ccmux attach-session -t "$SESSION_NAME" 2>/dev/null || ccmux new-session -s "$SESSION_NAME"
```

**Pros:**
- Minimal code changes from current implementation
- Leverage battle-tested ttyd
- Quick to implement
- Separation of concerns

**Cons:**
- Limited customization
- postMessage communication barrier
- iframe style isolation
- Cannot expose Claude state easily

**Best For:** Rapid migration, minimal disruption

---

## Backend Integration Approaches

### Option 1: Unix Socket Proxy (WebSocket Bridge)

**Architecture:**
```
Browser ←→ WebSocket Server ←→ Unix Socket ←→ ccmux daemon
```

**Rust Implementation:**

```rust
use tokio::net::{TcpListener, UnixStream};
use tokio_tungstenite::accept_async;
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize)]
#[serde(tag = "type")]
enum WebMessage {
    Input { data: String },
    Resize { cols: u16, rows: u16 },
    State { claude_state: String },
    Output { data: String },
}

async fn handle_connection(tcp_stream: tokio::net::TcpStream, session_id: String) {
    let ws_stream = accept_async(tcp_stream).await.unwrap();
    let unix_stream = UnixStream::connect("/home/dev/.ccmux/ccmux.sock").await.unwrap();

    // Bridge WebSocket ↔ Unix socket with bincode ↔ JSON translation
    // ...
}
```

**Pros:**
- Low latency, direct connection
- Full control over protocol
- Rust implementation shares types with ccmux

**Cons:**
- Requires understanding ccmux internal protocol
- bincode-JSON translation complexity
- Unix-only (no Windows named pipes)

---

### Option 2: MCP Bridge Integration

**Architecture:**
```
Claude/Client ←→ JSON-RPC ←→ ccmux MCP Server ←→ ccmux daemon
```

**Configuration (`~/.claude/mcp.json`):**

```json
{
  "mcpServers": {
    "ccmux": {
      "command": "/usr/local/bin/ccmux-server",
      "args": ["mcp-bridge"]
    }
  }
}
```

**Available MCP Tools:**

```typescript
// Example: Using ccmux MCP tools from Claude
const tools = {
  // Session management
  'ccmux_create_session': { name: string },
  'ccmux_list_sessions': {},
  'ccmux_attach_session': { session_id: string },

  // Pane operations
  'ccmux_split_pane': { direction: 'horizontal' | 'vertical' },
  'ccmux_send_keys': { pane_id: string, keys: string },

  // Orchestration
  'ccmux_spawn_agent': { name: string, command: string },
  'ccmux_coordinate_agents': { strategy: string },
};
```

**Pros:**
- Native Claude Code integration
- Standard JSON-RPC protocol
- Growing MCP ecosystem
- Clear tool abstractions

**Cons:**
- Request-response model less ideal for streaming
- JSON-only (no binary efficiency)
- Some security concerns with tool permissions

---

### Option 3: REST API Wrapper

**Architecture:**
```
Browser ←→ REST API ←→ ccmux CLI/socket ←→ ccmux daemon
```

**API Endpoints:**

```
GET    /api/ccmux/sessions           # List all sessions
POST   /api/ccmux/sessions           # Create new session
GET    /api/ccmux/sessions/:id       # Get session details
DELETE /api/ccmux/sessions/:id       # Close session
POST   /api/ccmux/sessions/:id/input # Send input
GET    /api/ccmux/sessions/:id/output # Get output buffer (polling)
POST   /api/ccmux/sessions/:id/resize # Resize terminal
GET    /api/ccmux/sessions/:id/state # Get Claude state
```

**Express.js Implementation:**

```javascript
// api/server.js updates
const { execSync, spawn } = require('child_process');

app.get('/api/ccmux/sessions', (req, res) => {
  try {
    const output = execSync('ccmux list-sessions --json').toString();
    res.json(JSON.parse(output));
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

app.post('/api/ccmux/sessions', (req, res) => {
  const { name, command } = req.body;
  try {
    execSync(`ccmux new-session -d -s "${name}" ${command || ''}`);
    res.json({ success: true, name });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

app.get('/api/ccmux/sessions/:id/state', (req, res) => {
  // Get Claude state from ccmux
  const state = execSync(`ccmux get-state -t "${req.params.id}"`).toString().trim();
  res.json({ session: req.params.id, claudeState: state });
});
```

**Pros:**
- Simple, well-understood
- Easy to test and document
- Stateless scaling

**Cons:**
- Polling required for output (inefficient)
- No native streaming
- Higher latency

---

### Option 4: gRPC with Streaming

**Architecture:**
```
Browser ←→ grpc-web ←→ Envoy Proxy ←→ gRPC Server ←→ ccmux daemon
```

**Protocol Definition:**

```protobuf
syntax = "proto3";

service CcmuxService {
  rpc CreateSession(CreateSessionRequest) returns (Session);
  rpc ListSessions(Empty) returns (SessionList);
  rpc StreamOutput(StreamRequest) returns (stream OutputChunk);
  rpc SendInput(InputRequest) returns (Empty);
  rpc GetClaudeState(StateRequest) returns (ClaudeState);
  rpc WatchState(StateRequest) returns (stream ClaudeState);
}

message ClaudeState {
  string session_id = 1;
  enum State {
    IDLE = 0;
    THINKING = 1;
    TOOL_USE = 2;
    STREAMING = 3;
    COMPLETE = 4;
  }
  State state = 2;
}

message OutputChunk {
  bytes data = 1;
  int64 timestamp = 2;
}
```

**Pros:**
- Strong typing
- Native bidirectional streaming
- Efficient binary protocol
- Code generation

**Cons:**
- Higher initial complexity
- Requires grpc-web + proxy for browsers
- Schema management overhead

---

### Option 5: Docker Container Approach

**Dockerfile:**

```dockerfile
FROM rust:alpine AS builder
WORKDIR /app
RUN git clone https://github.com/brendanbecker/ccmux.git .
RUN cargo build --release

FROM alpine:latest
RUN apk add --no-cache ttyd
COPY --from=builder /app/target/release/ccmux /usr/local/bin/
COPY --from=builder /app/target/release/ccmux-server /usr/local/bin/

EXPOSE 7681 8080
VOLUME /home/dev/.ccmux

ENTRYPOINT ["/entrypoint.sh"]
```

**docker-compose.yml Update:**

```yaml
services:
  agent-arena:
    build:
      context: .
      dockerfile: build1.dockerfile
    environment:
      - CCMUX_SOCKET=/home/dev/.ccmux/ccmux.sock
      - CCMUX_CONFIG_DIR=/home/dev/.config/ccmux
    volumes:
      - ccmux_state:/home/dev/.ccmux
      - ccmux_config:/home/dev/.config/ccmux
```

**Pros:**
- Process isolation
- Easy deployment (K8s compatible)
- Resource limits

**Cons:**
- PTY forwarding complexity
- Added latency
- Container orchestration overhead

---

## Deployment Considerations

### Session Isolation

| Deployment | Isolation Mechanism | Use Case |
|------------|---------------------|----------|
| Development | None (shared) | Local dev |
| Staging | Namespace + cgroups | Testing |
| Production | gVisor/Kata containers | Multi-tenant |

### Authentication Flow

```
User Login → JWT Token → Session Manager → ccmux Session
                ↓                              ↓
          Token Store               Session Registry
         (Redis/DB)                (In-Memory/DB)
```

### Security Hardening Checklist

- [ ] TLS for all WebSocket connections
- [ ] JWT validation on every message
- [ ] Rate limiting (sessions/min, commands/min)
- [ ] Input sanitization for terminal commands
- [ ] Audit logging for all session operations
- [ ] Network isolation (PTY tier separate from web tier)
- [ ] Seccomp profiles for container runtime

### Monitoring Metrics

```prometheus
# Session metrics
ccmux_sessions_active{node="node1"} 42
ccmux_sessions_total 1523

# Claude state metrics
ccmux_claude_state{session="main",state="thinking"} 1

# Performance metrics
ccmux_output_bytes_total 1048576
ccmux_websocket_latency_seconds 0.005
```

---

## Comparison Matrix

### Frontend Approaches

| Criteria | xterm.js WebSocket | Electron | Browser Ext | Iframe/ttyd |
|----------|-------------------|----------|-------------|-------------|
| **Implementation Effort** | Medium | High | Very High | Low |
| **User Experience** | Excellent | Excellent | Good | Good |
| **Performance** | Good | Excellent | Good | Good |
| **Cross-Platform** | Excellent | Good | Medium | Excellent |
| **Claude State Access** | Full | Full | Full | Limited |
| **Customization** | Full | Full | Medium | Limited |
| **Maintenance** | Medium | High | High | Low |

### Backend Approaches

| Criteria | Unix Socket | MCP Bridge | REST API | gRPC | Docker |
|----------|-------------|------------|----------|------|--------|
| **Implementation Effort** | Medium | Low | Low | Medium | High |
| **Latency** | Very Low | Low | Medium | Low | Medium |
| **Streaming Support** | Full | Limited | Polling | Full | Full |
| **Claude Integration** | Manual | Native | Manual | Manual | Manual |
| **Scalability** | Medium | High | Medium | High | High |
| **Maintenance** | Medium | Low | Low | Medium | High |

### Recommended Combinations

| Scenario | Frontend | Backend | Rationale |
|----------|----------|---------|-----------|
| **Quick Migration** | Iframe/ttyd | REST API | Minimal changes, fast deployment |
| **Full Featured** | xterm.js | Unix Socket + MCP | Best UX, full Claude state |
| **Enterprise** | xterm.js | gRPC + Docker | Scalability, strong typing |
| **Claude-First** | xterm.js | MCP Bridge | Native Claude orchestration |

---

## Recommendations

### Phase 1: Drop-in Replacement (1-2 weeks)

**Goal:** Replace tmux with ccmux with minimal frontend changes

**Changes Required:**

1. **Install ccmux in Dockerfile:**
```dockerfile
# Add to build1.dockerfile
RUN git clone https://github.com/brendanbecker/ccmux.git /tmp/ccmux && \
    cd /tmp/ccmux && \
    cargo build --release && \
    cp target/release/ccmux /usr/local/bin/ && \
    cp target/release/ccmux-server /usr/local/bin/ && \
    rm -rf /tmp/ccmux
```

2. **Update terminal-launch.sh:**
```bash
#!/bin/bash
SESSION_NAME="${1:-main}"
ccmux attach-session -t "$SESSION_NAME" 2>/dev/null || ccmux new-session -s "$SESSION_NAME"
```

3. **Update API server:**
```javascript
// api/server.js - replace tmux commands
const listSessions = () => {
  const output = execSync('ccmux list-sessions -F "#{session_name}:#{session_windows}:#{session_attached}"')
    .toString().trim();
  // Parse output same as before
};
```

4. **Start ccmux daemon in entrypoint:**
```bash
# Add to entrypoint
ccmux-server &
```

**Benefits:**
- Minimal frontend changes
- Quick validation of ccmux
- Preserves existing UX

---

### Phase 2: Enhanced API + State (2-3 weeks)

**Goal:** Expose Claude state and add ccmux-specific features

**New API Endpoints:**

```javascript
// GET /api/ccmux/sessions/:id/state
app.get('/api/ccmux/sessions/:id/state', (req, res) => {
  const state = execSync(`ccmux get-claude-state -t "${req.params.id}"`).toString().trim();
  res.json({
    session: req.params.id,
    claudeState: state,
    timestamp: Date.now()
  });
});

// POST /api/ccmux/sessions/:id/spawn-agent
app.post('/api/ccmux/sessions/:id/spawn-agent', (req, res) => {
  const { agentName, command } = req.body;
  execSync(`ccmux spawn-agent -t "${req.params.id}" -n "${agentName}" "${command}"`);
  res.json({ success: true });
});
```

**Frontend Updates:**

```typescript
// useCcmuxSessions.ts - add state polling
const [claudeStates, setClaudeStates] = useState<Record<string, ClaudeState>>({});

useEffect(() => {
  const stateInterval = setInterval(async () => {
    for (const session of sessions) {
      const res = await fetch(`/api/ccmux/sessions/${session.name}/state`);
      const { claudeState } = await res.json();
      setClaudeStates(prev => ({ ...prev, [session.name]: claudeState }));
    }
  }, 1000);
  return () => clearInterval(stateInterval);
}, [sessions]);
```

**UI Indicators:**

```tsx
// SessionItem.tsx
const stateColors = {
  Idle: 'green',
  Thinking: 'yellow',
  ToolUse: 'blue',
  Streaming: 'purple',
  Complete: 'gray'
};

<span className="state-indicator" style={{ color: stateColors[claudeState] }}>
  ● {claudeState}
</span>
```

---

### Phase 3: Native WebSocket Integration (3-4 weeks)

**Goal:** Replace iframe with native xterm.js for full control

**Components to Build:**

1. **WebSocket Bridge Server (Rust):**
```rust
// ccmux-web-bridge/src/main.rs
// Bridges WebSocket connections to ccmux Unix socket
```

2. **React Terminal Component:**
```typescript
// components/CcmuxTerminal.tsx
// Native xterm.js with WebSocket to bridge
```

3. **Claude State Subscription:**
```typescript
// Real-time state updates via WebSocket
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === 'state_change') {
    updateClaudeState(msg.session, msg.state);
  }
};
```

**Benefits:**
- Full Claude state visibility
- Real-time updates (no polling)
- Custom keyboard shortcuts
- Better resize handling
- Session recovery support

---

### Phase 4: MCP Integration (2-3 weeks)

**Goal:** Enable Claude-driven session management

**Configuration:**

```json
// ~/.claude/mcp.json
{
  "mcpServers": {
    "ccmux": {
      "command": "/usr/local/bin/ccmux-server",
      "args": ["mcp-bridge"]
    }
  }
}
```

**Use Cases:**
- Claude spawns sub-agents in new panes
- Claude monitors other agents' states
- Declarative layout creation
- Cross-session coordination

---

## Implementation Roadmap

```
Week 1-2:  Phase 1 - Drop-in Replacement
           ├── Install ccmux in container
           ├── Update terminal-launch.sh
           ├── Modify API for ccmux commands
           └── Test existing functionality

Week 3-4:  Phase 2 - Enhanced API
           ├── Add state endpoint
           ├── Add spawn-agent endpoint
           ├── Frontend state indicators
           └── State polling integration

Week 5-7:  Phase 3 - WebSocket Integration
           ├── Build Rust WebSocket bridge
           ├── Create CcmuxTerminal component
           ├── Real-time state subscriptions
           └── Migrate from iframe to native

Week 8-9:  Phase 4 - MCP Integration
           ├── Configure MCP server
           ├── Test Claude orchestration
           ├── Document MCP tools
           └── Integration testing

Week 10:   Polish & Documentation
           ├── Performance optimization
           ├── Security audit
           ├── User documentation
           └── Deployment guide
```

---

## Appendix: Quick Reference

### ccmux Commands (tmux equivalents)

| tmux | ccmux | Description |
|------|-------|-------------|
| `tmux new-session -s name` | `ccmux new-session -s name` | Create session |
| `tmux attach -t name` | `ccmux attach -t name` | Attach to session |
| `tmux list-sessions` | `ccmux list-sessions` | List sessions |
| `tmux kill-session -t name` | `ccmux kill-session -t name` | Kill session |
| `tmux split-window -h` | `ccmux split -h` | Horizontal split |
| N/A | `ccmux get-claude-state` | Get Claude state |
| N/A | `ccmux spawn-agent` | Spawn sub-agent |

### Configuration Files

| File | Purpose |
|------|---------|
| `~/.config/ccmux/config.toml` | ccmux configuration |
| `~/.ccmux/ccmux.sock` | Unix domain socket |
| `~/.claude/mcp.json` | MCP server config |

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `CCMUX_SOCKET` | Custom socket path |
| `CCMUX_CONFIG_DIR` | Config directory |
| `CLAUDE_CONFIG_DIR` | Claude isolation per pane |

---

*Document prepared for AgentArena ccmux integration project.*
