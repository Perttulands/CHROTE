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
7. Keep file-backed Formations boards, runs, and agent authoring visible through
   one shared dashboard/Archon model.

### Current Non-Goals

- CHROTE is not an IDE.
- CHROTE does not expose Windows as the workspace source of truth.
- CHROTE does not replace `bd`.
- CHROTE does not assume Gastown, Ralph, or vendored orchestrator components.
- CHROTE does not currently own a general-purpose autonomous messaging or IPC
  fabric. Explicit, bounded dispatch inside a user-authored Formations run is a
  separate workflow contract.

## Current Views

| View | Purpose |
| --- | --- |
| Terminal | Attach panes to durable tmux sessions |
| Terminal 2 | Second independent terminal workspace |
| Files | Browse allowed host workspace files |
| Agents | Observe agent-like tmux sessions |
| Beads | Show modern `bd` project issues, ready work, health, and optional `bv` sidecar usage |
| Formations | Author and inspect file-backed agent workflows and durable runs |
| Services | Operate selected `/srv` services through CHROTE-owned proxies |
| Settings | Theme, font, session behavior |
| Help | Dashboard usage |

## Formations foundation and accepted target

The current product already has file-backed boards with missions, agent
formations, gates, stable connections/ports, Archon and API authoring, lab and
tmux execution, and append-only run ledgers. That foundation is real but not the
finished workflow cockpit.

ADR-0006 accepts the next mixed-workflow model: fixed Mission entry, Formation agent
execution, pure frozen-profile Tool transformation, and Gate evaluation/routing with
typed named ports. Agent-first authoring, deterministic Tool steps, canonical
node/attempt/artifact projection, typed gate feedback, and exact run-bound
per-slot terminal Peek are active target work and are not implemented on current main.
Current boards remain the compatibility base while those slices land behind
their explicit validation and certification gates.
Accepted runtime authority moves canonical ledgers, immutable graph/private
bindings, sealed Tool inputs, and pending raw-redaction state under the
writer-only Formations host-authority root, supplied explicitly and shared across
lanes capable of `authoritySchema=2`, outside generic Files roots. Users see sanitized
run/event/binding projections and currently authorized artifacts through the
existing cockpit/File Peek surfaces; raw authority is neither browsable nor
mutable through Files.

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
`/srv` proving lane runs from `/srv/chrote` with data under `/srv/data/chrote`,
system unit `chrote-srv.service`, HTTP `127.0.0.1:8095`, and ttyd `7686`.
Its private environment is under `/srv/chrote/config/chrote.env` and the
installed system unit reads `/etc/chrote/chrote-srv.env`.

The legacy rollback lane runs from `/home/perttu/chrote`, user unit
`chrote.service`, HTTP `127.0.0.1:8094`, ttyd `7683`, and may load
`~/.config/chrote/services.env`.

## Runtime Requirements

- Run the `/srv` proving lane as the dedicated `chrote` service identity.
- Bind `/srv` CHROTE HTTP to `127.0.0.1:8095`.
- Bind `/srv` ttyd to `127.0.0.1:7686`.
- Read per-user tmux socket mappings from private runtime configuration.
- Keep the legacy rollback lane available on `127.0.0.1:8094` and ttyd `7683` until cutover is proven.
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

### Phase 4 - Formations and Meta-Harness

The file-backed Formations foundation is present and under active stabilization.
The accepted target coordinates interchangeable harnesses such as Codex, Claude
Code, Pi, OpenCode, Hermes, and generic tmux agents through explicit adapters,
mixed workflow nodes, run ledgers, and audited control surfaces. Tool steps,
host-owned asynchronous coordination, full run inspection, and exact terminal
Peek remain target behavior until their Beads and exact-candidate gates close.

### Phase 5 - Agent Teams

Planned later. Agent Teams should support role topology, safe session targeting,
audited nudges/messages, durable transcripts, and human approval boundaries.
This remains roadmap work until the adapter, ledger, safety, and recovery model
are implemented.

## Acceptance Criteria

- `go test ./...` passes.
- `npm run build` passes.
- `chrote-srv.service` is active after restart for the `/srv` proving lane.
- `chrote.service` remains the labeled legacy rollback lane until cutover is proven.
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
