# Agent Teams in CHROTE

*Strategy document — 2026-05-06*

## Problem

CHROTE makes it easy to run and monitor many individual tmux sessions. The next level is **agent teams**: multiple agents with defined roles that collaborate — executor + verifier, orchestrator + workers, reviewer + coder. Today CHROTE has no concept of a team, no way to define relationships between agents, and no way to launch a team as a unit.

## Guiding principle

CHROTE does not own agent-to-agent communication. Harnesses (Claude Code, Pi, Codex, Gemini) handle their own IPC — stop hooks, socket injection, prompt relays. CHROTE's job is narrower: **define topology → launch team → visualize status → navigate sessions**.

---

## Architecture

### 1. Team definition — YAML files

Teams live in `/code/teams/` (next to the code agents work on, inside the existing `/code` filesystem root).

```yaml
# /code/teams/verifier.yaml
name: verifier
agents:
  - session: hq-executor
    role: executor
    harness: claude-code
  - session: hq-verifier
    role: verifier
    harness: claude-code
edges:
  - from: hq-verifier
    to: hq-executor
    type: stop-hook
```

That's the full data model. Harness-agnostic. No new protocol. Roles and edges are metadata for visualization and launch — the harness config handles the actual IPC.

**Supported harness values:** `claude-code`, `pi`, `codex`, `gemini`, `raw` (extensible).

### 2. Team launcher — `team-launch.sh`

A new launch script that reads a team YAML and starts each agent session with the correct harness command:

```
team-launch.sh <team.yaml>
```

Internally maps harness → command:
- `claude-code` → `claude [flags]`
- `pi` → `pi [flags]`
- `codex` → `codex [flags]`
- etc.

Each agent gets its own named tmux session (matching the `session:` field). The dashboard "Launch Team" button fires this script.

### 3. Dashboard visualization — extend what exists

No new tab. Extend the existing session sidebar:

**Teams section** (above the flat session list):
- One card per discovered team YAML
- Each card shows agents with role badges: `executor`, `verifier`, `worker`, `orchestrator`
- Live session status (running / stopped) pulled from existing tmux polling
- "Launch" button → fires `team-launch.sh <team.yaml>`
- "Stop" button → kills all sessions in the team

**Workspace integration:**
- Drag a team card onto a workspace → auto-assigns each agent to a window, preserving topology order
- Window headers show role badge alongside the session name
- When two windows in the same workspace hold agents with a defined edge between them, draw a subtle arrow between those window headers

**Topology modal:**
- Click team card → opens modal with a simple SVG node/edge graph of the team
- Nodes: agent name + role + status dot
- Edges: labeled with type (`stop-hook`, `prompt-relay`, etc.)
- No new library needed — plain SVG

---

## What CHROTE does NOT own

| Concern | Owner |
|---------|-------|
| Stop hook wiring | Harness config (`.claude/settings.json`, etc.) |
| Agent prompts | Team YAML or harness config |
| IPC protocol (sockets, files, stdin injection) | Harness |
| Verification logic | Harness |
| Agent memory / state | Harness |

---

## Open question: where do team YAMLs live?

| Location | Pro | Con |
|----------|-----|-----|
| `/code/teams/` | Co-located with project code; per-project teams | Requires `/code` mount |
| `~/chrote/teams/` | Always available; shared across projects | Disconnected from project context |

**Recommendation:** `/code/teams/` as primary, with optional fallback scan of `~/chrote/teams/`. This matches the existing `/code` filesystem model and puts team definitions where agents can read them.

---

## Milestone plan

**M1 — Team config + launcher**
- Define YAML schema
- `team-launch.sh` with harness dispatch
- Go API endpoint: `GET /api/teams` → scans `/code/teams/*.yaml`
- Go API endpoint: `POST /api/teams/:name/launch` → fires launch script
- Go API endpoint: `POST /api/teams/:name/stop` → kills sessions

**M2 — Dashboard Teams section**
- Teams card list in session sidebar
- Role badges on session entries
- Launch / Stop buttons

**M3 — Workspace topology**
- Drag team → auto-assign to windows with role order
- Role badge in window headers
- Arrow between windows with a defined edge

**M4 — Topology modal**
- Click team → SVG graph of nodes and edges
- Live status dots

---

## Reference: the verifier pattern

The pattern demonstrated in [the-verifier-agent](https://github.com/disler/the-verifier-agent):
- Builder (executor) and Verifier run as separate tmux sessions
- Builder's `stop` lifecycle event triggers verifier via Unix socket
- Verifier injects corrective prompts back into builder via `sendUserMessage()`
- Up to 3 correction rounds before escalating to human
- ~2x token cost; saves ~50% of human review time

In CHROTE terms: two sessions, one edge of type `stop-hook`, both harness `pi`. The YAML definition captures this completely. CHROTE launches the sessions and shows the topology; Pi handles the socket and prompt injection.
