# Arena Dev Environment

A sandboxed Ubuntu container for running Gastown AI agents, accessible from any device on your Tailnet.

## Web Dashboard

Access the dashboard from any Tailnet device:

```
http://arena:8080/
```

The dashboard provides:
- **Terminal** - Multi-pane tmux session viewer with drag-and-drop session assignment
- **Files** - Browse and edit files in /code and /vault
- **Status** - Service health and quick command reference

### Session Management
- Drag sessions from the side panel onto terminal windows to attach
- Click session tags to switch between assigned sessions
- Each window can have multiple sessions; click tags to switch
- Sessions are actual tmux sessions running as the `dev` user

## Quick Reference

| What | How |
|------|-----|
| Dashboard | `http://arena:8080/` (from any Tailnet device) |
| Your code | `/code` (E:/Code on host) |
| Read-only vault | `/vault` (E:/Vault on host) |
| Ollama API | `http://ollama:11434` |
| Run Claude | `claude` or `claude --dangerously-skip-permissions` |

## Directory Structure

```
/code    - Your projects (read/write)
/vault   - Reference files (read-only for dev user)
/home/dev - Your home directory (persisted across rebuilds)
```

## Users

| User | Password | Notes |
|------|----------|-------|
| dev | dev | Standard user, sudo access, cannot write to /vault |
| root | root | Can write everywhere including /vault |

## Installed Tools

- **Claude Code** - `claude --help`
- **Gastown** - `gt --help`
- **Beads** - `bd --help`
- **beads_viewer** - Visualization tool
- Git, tmux, Go, Node.js, npm, Python3, pip

## Running Gastown

Start a Gastown rig to orchestrate AI agents:

```bash
# Start gastown with a rig
gt start gastown

# Check status
gt status

# Monitor activity
gt peek
```

Agents run in tmux sessions. Use the web dashboard to monitor and interact with them from any device.

## Running AI Agents

This container is network-isolated via Tailscale ACLs. Safe to run:

```bash
claude --dangerously-skip-permissions
```

The container can only reach:
- ollama (LLM API)
- The internet (npm, pip, git, APIs)
- NOT your other Tailnet devices (NAS, other machines)

## Dev Servers

Bind to `0.0.0.0` to make accessible from your other devices:

```bash
# Vite/React
npm run dev -- --host 0.0.0.0

# Flask
flask run --host=0.0.0.0

# FastAPI
uvicorn main:app --host 0.0.0.0

# Next.js
npm run dev -- -H 0.0.0.0
```

Access from any Tailnet device at `http://arena:3000` (or other port).

## Using Ollama

```bash
# List models
curl http://ollama:11434/api/tags

# Chat
curl http://ollama:11434/api/chat -d '{
  "model": "llama2",
  "messages": [{"role": "user", "content": "Hello"}]
}'

# Generate
curl http://ollama:11434/api/generate -d '{
  "model": "llama2",
  "prompt": "Why is the sky blue?"
}'
```

## Vault Usage

Drop files into `/vault` as root (or from Windows at E:/Vault) to provide read-only context to agents:

```bash
# From Windows: copy files to E:/Vault
# From arena as root:
ssh root@arena
cp /code/important-spec.md /vault/
```

The dev user (and agents) can read but not modify vault contents.

## Troubleshooting

```bash
# Check Tailscale status
tailscale status

# Check if ollama is reachable
curl http://ollama:11434/api/tags

# Check internet
curl -I https://google.com

# Rebuild container (from Windows host)
# cd E:\Docker\AgentArena
# docker compose down
# docker compose build --no-cache agent-arena
# docker compose up -d agent-arena
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Your Devices (laptop, phone, tablet)                       │
│  Access: http://arena:8080/                                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ Tailscale
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  arena (Tailscale node)                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  agent-arena container                               │   │
│  │  - nginx (:8080) serves dashboard + proxies          │   │
│  │    └─ Dashboard UI    → /                            │   │
│  │    └─ Terminal proxy  → /terminal/ → ttyd (:7681)    │   │
│  │    └─ Files proxy     → /files/    → filebrowser     │   │
│  │    └─ API proxy       → /api/      → node (:3001)    │   │
│  │  - ttyd (web terminal, runs as dev)                  │   │
│  │  - tmux sessions (dev user, managed via API)         │   │
│  │  - API server (runs as dev, manages tmux)            │   │
│  └─────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  filebrowser container                               │   │
│  │  - Browse /code and /vault (:8081)                   │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ Tailscale
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  ollama (Tailscale node)                                    │
│  - LLM inference API at http://ollama:11434                 │
└─────────────────────────────────────────────────────────────┘
```

## Network Security

This container is isolated via Tailscale ACLs:
- Can reach: ollama, internet
- Cannot reach: Your NAS, Landmass host, other Tailnet devices

Even if compromised, blast radius is limited to this disposable container.
