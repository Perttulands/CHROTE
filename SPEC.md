# Agent Arena & Ollama System Specification

## Overview

A Docker system on "Landmass" (home server) with **Tailscale sidecar containers** for:
1. Running AI coding agents (Claude Code, Gastown, Beads) with long-running tasks
2. Serving LLM inference to authorized remote devices

Each service gets its own Tailscale identity, enabling per-service ACL control.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ Landmass (Windows Host + Docker)                                │
│                                                                 │
│  ┌─────────────────────┐      ┌─────────────────────┐          │
│  │ tailscale-ollama    │      │ tailscale-arena     │          │
│  │ hostname: ollama    │      │ hostname: arena     │          │
│  │ (Tailnet identity)  │      │ (Tailnet identity)  │          │
│  └──────────┬──────────┘      └──────────┬──────────┘          │
│             │ network_mode               │ network_mode        │
│  ┌──────────┴──────────┐      ┌──────────┴──────────┐          │
│  │ ollama              │      │ agent-arena         │          │
│  │ :11434 (LLM API)    │      │ :22 (SSH)           │          │
│  │                     │      │ :3000-8080 (dev)    │          │
│  └──────────┬──────────┘      └──────────┬──────────┘          │
│             │                            │                      │
│  ┌──────────┴────────────────────────────┴──────────┐          │
│  │ Volumes                                          │          │
│  │ E:/LLM_models → /root/.ollama/models            │          │
│  │ E:/Code       → /code (RW)                      │          │
│  │ E:/Vault      → /vault (RO dev, RW root)        │          │
│  └──────────────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────────────┘
         │                              │
    Tailnet                        Tailnet
  "ollama:11434"               "arena:22"
         │                              │
         ▼                              ▼
   Remote Devices ──────────────────────────
   - Access ollama API at ollama:11434
   - SSH to arena:22
   - Controlled via Tailscale ACLs

## Containers

### 1. agent-arena (agentarena-dev)

**Purpose:** Development environment for AI coding agents

**Image:** Custom Ubuntu 24.04

**Installed Tools:**
- Claude Code (@anthropic-ai/claude-code)
- Gastown & Beads (orchestrator tools)
- beads_viewer
- Git, tmux, Go, Node.js, Python3

**Ports (via Tailscale):**
| Port | Purpose |
|------|---------|
| 22 | SSH access |
| 8080 | Web dashboard (nginx) |
| 7681 | ttyd web terminal (proxied via nginx) |
| 3001 | API server (proxied via nginx) |
| 3000, 5000, 8000 | Dev servers |
| 5500, 6000 | Additional dev servers |
| 9000-9900 | Monitoring & future use |

**Users:**
- `root:root` - Full access, can write to /vault
- `dev:dev` - Standard user, read-only /vault, sudo access

### 2. ollama (agentarena-ollama)

**Purpose:** LLM inference API server

**Image:** ollama/ollama:latest

**Ports:**
| Port | Purpose |
|------|---------|
| 11434 | Ollama API |

**CORS Origins (who can access):**
- http://localhost
- http://127.0.0.1
- http://arena
- https://arena

## Volume Layout

| Host Path | Container | Mount Path | Permissions | Purpose |
|-----------|-----------|------------|-------------|---------|
| E:/Code | agent-arena | /code | RW (dev & root) | Active coding projects |
| E:/Vault | agent-arena | /vault | RO (dev), RW (root) | Safe context files for agents |
| E:/LLM_models | ollama | /root/.ollama/models | RW (root) | LLM model storage |
| Named: arena_dev_home | agent-arena | /home/dev | RW | Persist dev config (.bashrc, .ssh) |
| Named: arena_root_home | agent-arena | /root | RW | Persist root config |

### /vault Security Model

The `/vault` directory is designed for safely providing context to AI agents:
- **Drop files as root** (SSH as root, or from Windows at E:/Vault)
- **Agents (running as dev) can only read** - prevents accidental corruption
- Permissions enforced at container startup via entrypoint script

## Access Points

| Service | Tailnet Access | Port |
|---------|----------------|------|
| Web Dashboard | `http://arena:8080` | 8080 |
| SSH (agent-arena) | `ssh dev@arena` | 22 |
| SSH (as root) | `ssh root@arena` | 22 |
| Ollama API | `http://ollama:11434` | 11434 |
| Dev servers | `http://arena:3000` etc. | 3000, 5000, 8000 |

### Web Dashboard

The dashboard at `http://arena:8080` provides:
- **Terminal view** with 1-4 panes, drag-and-drop session assignment
- **Files view** via filebrowser
- **Status view** with service health

**Architecture:**
- nginx serves static React dashboard and proxies:
  - `/terminal/` → ttyd (port 7681, runs as dev)
  - `/files/` → filebrowser (port 8081)
  - `/api/` → Node.js API (port 3001, runs as dev)
- ttyd accepts `?arg=session_name` to attach to tmux sessions
- API manages tmux sessions (list/create) as dev user

**Note:** No localhost port mapping. Access is exclusively via Tailscale.

## Setup

### 1. Create .env file with Tailscale auth key

```bash
cp .env.example .env
# Edit .env and add your TS_AUTHKEY
```

Generate key at: https://login.tailscale.com/admin/settings/keys
- Use **Reusable** key, or
- Use **OAuth client** with `tag:container` scope

### 2. Build and start

```bash
docker-compose build --no-cache
docker-compose up -d
```

### 3. Verify on Tailscale

Check https://login.tailscale.com/admin/machines for:
- `ollama` - LLM service
- `arena` - Dev environment

## Commands

```bash
# Build images
docker-compose build

# Start containers
docker-compose up -d

# Stop containers
docker-compose down

# View logs
docker-compose logs -f

# Rebuild from scratch
docker-compose down && docker-compose build --no-cache && docker-compose up -d

# SSH into arena
ssh dev@arena      # as dev user
ssh root@arena     # as root

# Test Ollama
curl http://ollama:11434/api/tags
```

## Files

```
E:\Docker\AgentArena\
├── build1.dockerfile      # Agent Arena image
├── ollama.dockerfile      # Ollama image  
├── docker-compose.yml     # Container orchestration
├── .env                   # Tailscale auth key (git-ignored)
├── .env.example           # Template for .env
├── SPEC.md               # This file
└── tailscale_state/      # Tailscale persistence
    ├── ollama/           # Ollama's tailscale state
    └── arena/            # Arena's tailscale state
```

## Security

1. **Tailscale ACLs** - Control which devices can reach ollama vs arena
2. **SSH passwords** are defaults (dev:dev, root:root) - change for production
3. **/vault is read-only for dev** - agents can't corrupt context files
4. **No localhost exposure** - services only accessible via authenticated Tailscale

### Tailscale ACL (grants syntax)

```json
{
  "tagOwners": {
    "tag:container": ["autogroup:admin"]
  },
  "grants": [
    // Your personal devices can reach everything
    {"src": ["autogroup:member"], "dst": ["*"], "ip": ["*"]},
    // Containers can reach each other (arena <-> ollama)
    {"src": ["tag:container"], "dst": ["tag:container"], "ip": ["*"]},
    // Containers can reach internet (npm, pip, git, APIs)
    {"src": ["tag:container"], "dst": ["autogroup:internet"], "ip": ["*"]}
    // IMPLICIT DENY: containers cannot reach your other Tailnet devices
  ]
}
```
