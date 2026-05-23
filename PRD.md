# CHROTE Product Requirements

## Vision

CHROTE is the private browser platform for host-owned agentic work.

The long-term vision is one cockpit through which the human operator can
orchestrate swarms of agents, drive agentic development work, inspect durable
state, and support agent-to-agent collaboration across different harnesses.

The implementation path is staged:

1. Keep the durable workspace visible and recoverable.
2. Add a Services platform for host-owned capabilities such as TTS and Context Citadel.
3. Grow toward a deliberate meta-harness with recipes, teams, run ledgers, and
   audited agent collaboration.

The browser is disposable. The Ubuntu host owns the work.

## Current Product

CHROTE is currently the durable browser cockpit for a configured host
development workspace.

The host owns terminals, agents, files, dev servers, builds, tests, Beads, logs,
and runtime state. Client devices only view and operate that state.

### Current Goals

1. Make durable tmux sessions visible and controllable from a browser.
2. Keep browser or device disconnects from killing important work.
3. Provide file and Beads visibility in the same cockpit.
4. Surface agent-like sessions without depending on a specific orchestrator.
5. Wrap selected local services through CHROTE-owned server-side routes.
6. Stay private to localhost and Tailscale unless explicitly changed.

### Current Non-Goals

- CHROTE is not an IDE.
- CHROTE does not expose Windows as the workspace source of truth.
- CHROTE does not replace `bd`.
- CHROTE does not assume Gastown, Ralph, or vendored orchestrator components.
- CHROTE does not currently own agent-to-agent IPC or autonomous team routing.

## Current Views

| View | Purpose |
| --- | --- |
| Terminal | Attach panes to durable tmux sessions |
| Terminal 2 | Second independent terminal workspace |
| Files | Browse allowed host workspace files |
| Agents | Observe agent-like tmux sessions |
| Beads | Show modern `bd` project issues, ready work, health, and optional `bv` sidecar usage |
| Services | Operate selected `/srv` services through CHROTE-owned proxies |
| Settings | Theme, font, session behavior |
| Help | Dashboard usage |

## Services Platform V1

The current Services stage makes selected local services legible and operable
inside CHROTE.

V1 services are:

| Service | Operator outcome |
| --- | --- |
| TTS Gateway | See agent voice messages, generation status, playback, backend/voice choices, and enqueue test messages |
| Context Citadel | Read, edit, save, inspect history, and ask grounded questions over Markdown/Git-backed context files |

### Services Requirements

- Add a top-level `Services` tab without disrupting existing terminal iframe
  lifecycle behavior.
- Keep service runtimes outside the browser and behind CHROTE-owned routes.
- Proxy service access through CHROTE-owned API routes instead of making browser
  code call raw service ports directly.
- Keep Context Citadel bearer tokens server-side only.
- Provide clear degraded states when a service is unavailable or not configured.
- Keep Services v1 focused on TTS and Context Citadel.
- Do not implement Agent Teams, recipes, autonomous agent messaging, or Gas City
  orchestration as part of Services v1.

### Service Configuration

CHROTE service adapters use localhost defaults and may be overridden by runtime
environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `CHROTE_TTS_URL` | `http://127.0.0.1:3100` | TTS Gateway upstream |
| `CHROTE_CONTEXT_API_URL` | `http://127.0.0.1:3200` | Context Citadel upstream |
| `CHROTE_CONTEXT_API_TOKEN` | unset | Server-side owner token for Context Citadel operations |

Secrets must live in private runtime configuration, not tracked docs. The
current deployment unit loads `~/.config/chrote/services.env` if present.

## Runtime Requirements

- Run as the configured host user.
- Bind CHROTE HTTP to `127.0.0.1:8094`.
- Bind ttyd to `127.0.0.1:7683`.
- Use `TMUX_TMPDIR=/run/user/1000/chrote-tmux`.
- Restrict file roots to configured workspace roots.
- Expose the dashboard through a private access layer such as Tailscale Serve.
- Keep all service adapters private to localhost or tailnet-safe routes.

## Agent Observability Requirements

- Detect sessions using configurable prefixes.
- Show counts for total, working, idle, and Beads-linked agents.
- Show last non-empty terminal lines.
- Infer simple state from recent terminal output.
- Extract modern Beads IDs such as `home-fv6.9`.
- Treat absence of agent sessions as normal.
- Treat absence of a default tmux server as normal.

## Beads Requirements

- Use configured `bd`.
- Use `bd --json` output.
- Do not require `bv` for dashboard operation.
- Support `bv` as an optional TUI sidecar generated from modern `bd export`.
- Detect configured Beads workspaces.
- Provide issues, triage, and insights endpoints for the dashboard.

## Roadmap

### Phase 1 - Durable Cockpit

Completed foundation. CHROTE exposes durable terminals, files, Beads, agent
observability, and private tailnet access.

### Phase 2 - Services Platform

Current phase. CHROTE is a component host for selected local services,
starting with TTS Gateway and Context Citadel through server-side proxies.

### Phase 3 - Service Expansion

Later Services components may include image generation, Camofox browser
automation, Ollama status, and Gas City read-only observation if they earn their
place in the operator workflow.

### Phase 4 - Meta-Harness

Planned later. CHROTE should coordinate interchangeable harnesses such as Codex,
Claude Code, Pi, OpenCode, Hermes, and generic tmux agents through explicit
adapters, run ledgers, recipes, and audited control surfaces.

### Phase 5 - Agent Teams

Planned later. Agent Teams should support role topology, safe session targeting,
audited nudges/messages, durable transcripts, and human approval boundaries.
This remains roadmap work until the adapter, ledger, safety, and recovery model
are implemented.

## Acceptance Criteria

- `go test ./...` passes.
- `npm run build` passes.
- `chrote.service` is active after restart.
- `/api/health` returns OK.
- `/api/tmux/sessions` returns the CHROTE tmux session list.
- `/api/oracle/status` returns OK even when no agents are running.
- `/api/beads/health` reports modern `bd`.
- Services routes report clear health/degraded states for TTS and Context Citadel.
- Services UI can show TTS health/messages, enqueue voice output, play ready
  messages, and show generation errors.
- Services UI can list/read/save Context docs, show history, and ask grounded
  questions when a server-side Context Citadel token is configured.
- Tailscale Serve continues to expose `:8445` only to the tailnet.
