# CHROTE Product Requirements

## Vision

CHROTE is the private browser platform for host-owned agentic work.

The long-term vision is one cockpit through which the human operator can
access named sessions and agent identities, orchestrate swarms of agents, drive
agentic development work, inspect durable state when needed, and support
agent-to-agent collaboration across different harnesses.

The implementation path is staged:

1. Keep the durable workspace visible and recoverable.
2. Add a Services platform for host-owned capabilities such as TTS and Context API.
3. Put Gas City under CHROTE as the orchestration substrate for named agent
   identities, mail, nudging, sling/delegation, molecules, workflows, events,
   and automation.
4. Grow toward a deliberate meta-harness with recipes, teams, run ledgers, and
   audited agent collaboration.

The browser is disposable. The Ubuntu host owns the work.

## Current Product

CHROTE is currently the durable browser cockpit for a configured host
development workspace.

The host owns terminals, agents, files, dev servers, builds, tests, Beads, logs,
and runtime state. Client devices only view and operate that state.

The product direction for CHROTE 3.0 is not "show more information about
running sessions." The direction is to make CHROTE the access layer where named
tmux sessions can become named Gas City-backed agent identities, and where the
operator can start, address, inspect, and recover those identities without
manually wiring every tmux command.

### Current Goals

1. Make durable named tmux sessions visible and controllable from a browser.
2. Keep browser or device disconnects from killing important work.
3. Provide file and Beads visibility in the same cockpit.
4. Evolve selected named sessions into explicit Gas City-backed agent
   identities.
5. Provide a path for agent collaboration through Gas City mail, nudge,
   sling/delegation, molecules, workflows, and events.
6. Wrap selected local services through CHROTE-owned server-side routes.
7. Stay private to localhost and Tailscale unless explicitly changed.

### Current Non-Goals

- CHROTE is not an IDE.
- CHROTE does not expose Windows as the workspace source of truth.
- CHROTE does not replace `bd`.
- CHROTE does not become a passive transcript-watching or status-dashboard
  product as the main value proposition.
- CHROTE does not require migration of current old tmux sessions before the
  Gas City-backed identity model can move forward.
- CHROTE does not mirror the native `gc` command tree as a second operator CLI.
- CHROTE does not replace Gas City as the owner of orchestration primitives.

## Current Views

| View | Purpose |
| --- | --- |
| Terminal | Attach panes to durable tmux sessions |
| Terminal 2 | Second independent terminal workspace |
| Files | Browse allowed host workspace files |
| Agents | Observe agent-like tmux sessions |
| Beads | Show modern `bd` project issues, ready work, health, and optional `bv` sidecar usage |
| Gas City | Early orchestration surface for Gas City-backed identities, mail, workflows, and runtime state |
| Services | Operate selected `/srv` services through CHROTE-owned proxies |
| Settings | Theme, font, session behavior |
| Help | Dashboard usage |

## Gas City Orchestration Direction

Gas City is the lower layer for orchestration. CHROTE is the access and
operator layer above it.

Gas City owns:

- valid agent identities and session ownership;
- mail as durable agent-to-agent communication;
- nudges as live delivery/wake-up;
- sling/delegation as routed work assignment;
- formulas and molecules as reusable workflow packages;
- events and supervisor state as runtime evidence.

CHROTE owns:

- the browser access layer for named sessions and named agent identities;
- safe operator controls for starting, addressing, inspecting, and recovering
  agents;
- private proxying and policy boundaries;
- the human-facing shape of workflows.

The target felt experience is direct delegation between named agents. For
example, Perttu can open a CHROTE session for an agent named Claudia, then tell
another agent identity, Codxia, "Help Claudia get this done." CHROTE should make
that action legible and reachable; Gas City should carry the identity,
mail/nudge, sling, molecule, event, and recovery mechanics.

The Gas City tab should earn its place by helping the operator use
orchestration. Merely surfacing more transcript lines, status counts, or passive
session information is not the core product win.

## Services Platform V1

The current Services stage makes selected local services legible and operable
inside CHROTE.

V1 services are:

| Service | Operator outcome |
| --- | --- |
| TTS Gateway | See agent voice messages, generation status, playback, backend/voice choices, and enqueue test messages |
| Context API | Read, edit, save, inspect history, and ask grounded questions over Markdown/Git-backed context files |

### Services Requirements

- Add a top-level `Services` tab without disrupting existing terminal iframe
  lifecycle behavior.
- Keep service runtimes outside the browser and behind CHROTE-owned routes.
- Proxy service access through CHROTE-owned API routes instead of making browser
  code call raw service ports directly.
- Keep Context API bearer tokens server-side only.
- Provide clear degraded states when a service is unavailable or not configured.
- Keep Services v1 focused on TTS and Context API.
- Do not implement Agent Teams, recipes, autonomous agent messaging, or new Gas
  City orchestration mutations as part of Services v1.

### Service Configuration

CHROTE service adapters use localhost defaults and may be overridden by runtime
environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `CHROTE_TTS_URL` | `http://127.0.0.1:3100` | TTS Gateway upstream |
| `CHROTE_CONTEXT_API_URL` | `http://127.0.0.1:3200` | Context API upstream |
| `CHROTE_CONTEXT_API_TOKEN` | unset | Server-side owner token for Context API operations |

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
starting with TTS Gateway and Context API through server-side proxies.

### Phase 3 - Named Identity Access

Current CHROTE 3.0 alignment work. CHROTE should distinguish plain named tmux
sessions from Gas City-backed named identities, without forcing migration of the
currently running sessions. The first useful operator path is addressing or
delegating to a named identity and then opening the underlying session when the
human wants to inspect or intervene.

### Phase 4 - Service Expansion

Later Services components may include image generation, Camofox browser
automation, and Ollama status if they earn their place in the operator workflow.

### Phase 5 - Meta-Harness

Planned later. CHROTE should coordinate interchangeable harnesses such as Codex,
Claude Code, Pi, OpenCode, Hermes, and generic tmux agents through explicit
Gas City-backed identities, adapters, run ledgers, recipes/molecules, and
audited control surfaces.

### Phase 6 - Agent Teams

Planned later. Agent Teams should support role topology, safe session targeting,
audited nudges/messages, durable mail threads, workflow artifacts, and human
approval boundaries. This remains roadmap work until the identity, adapter,
ledger, safety, and recovery model are implemented.

## Acceptance Criteria

- `go test ./...` passes.
- `npm run build` passes.
- `chrote.service` is active after restart.
- `/api/health` returns OK.
- `/api/tmux/sessions` returns the CHROTE tmux session list.
- `/api/oracle/status` returns OK even when no agents are running.
- `/api/beads/health` reports modern `bd`.
- Services routes report clear health/degraded states for TTS and Context API.
- Services UI can show TTS health/messages, enqueue voice output, play ready
  messages, and show generation errors.
- Services UI can list/read/save Context docs, show history, and ask grounded
  questions when a server-side Context API token is configured.
- Tailscale Serve continues to expose `:8445` only to the tailnet.
