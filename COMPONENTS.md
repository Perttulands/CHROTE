# CHROTE Components

This is the public component map. Machine-specific service names, paths, socket
ACLs, and rollback layouts belong in private operator configuration.

## Core runtime

| Component | Role | Required? |
| --- | --- | --- |
| Go server | HTTP API, embedded dashboard, terminal proxy, scheduling, and optional adapters | Yes |
| React dashboard | Browser cockpit served from the Go binary | Yes |
| tmux | Terminal and process substrate with a lifecycle independent of the browser | Yes for terminal workspaces |
| ttyd | Browser terminal transport behind CHROTE | Yes for interactive terminals |
| Host filesystem | Files, schedules, and configuration | Yes |

The browser is a client of this runtime. It is not the source of truth. tmux
sessions have a lifecycle independent of CHROTE, and a CHROTE restart must not
deliberately terminate them. CHROTE does not recreate a workload after its
process or the host exits.

## Built-in cockpit surfaces

| Surface | Backing capability |
| --- | --- |
| Terminal 1-3 | Independent layouts over tmux sessions |
| Sessions/Files sidecar | Session discovery, Peek, assignment navigation, and workspace-local files |
| Files | Configured-root file operations and terminal handoff |
| Beads | Configured `bd` workspaces and issue data |
| Services | Optional local adapters through CHROTE-owned routes |
| Scheduled | Scheduled-task definitions, locks, runs, and history |
| Server | Health, resources, runtime events, and bounded system history |
| Settings | Appearance, terminal behavior, flags, and session cleanup |

## Optional integrations

| Component | CHROTE role |
| --- | --- |
| `bd` | Beads issue source; configured workspaces remain authoritative |
| `bv` | Optional graph-aware Beads TUI launched inside a terminal |
| TTS Gateway adapter | Optional Services console; no upstream or credentials are bundled |
| Context Citadel adapter | Optional adapter code; no current authentication or upstream is bundled |
| Tailscale or equivalent | Private HTTPS/network access outside localhost |

Optional integrations must degrade clearly when unavailable. They must not make
the core dashboard fail to load.

## Trust boundary

CHROTE runs with the Unix permissions of its service identity. Configured file
roots constrain CHROTE file APIs; they do not sandbox tmux agents. Service URLs,
tokens, socket mappings, and executable paths are private runtime configuration.
See [`SECURITY.md`](SECURITY.md).

## Not bundled or assumed

- Gastown
- Ralph
- a hosted identity provider
- autonomous agent-to-agent chat
- a mandatory cloud service
