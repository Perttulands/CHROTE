# CHROTE Components

This is the public component map. Machine-specific service names, paths, socket
ACLs, and rollback layouts belong in private operator configuration.

## Core runtime

| Component | Role | Required? |
| --- | --- | --- |
| Go server | HTTP API, embedded dashboard, terminal proxy, operator-triggered recovery, scheduling, and optional experimental runtimes | Yes |
| React dashboard | Browser cockpit served from the Go binary | Yes |
| tmux | Terminal and process substrate with a lifecycle independent of the browser | Yes for terminal workspaces |
| ttyd | Browser terminal transport behind CHROTE | Yes for interactive terminals |
| Host filesystem | Files, schedules, recovery state, and experimental definitions when used | Yes |

The browser is a client of this runtime. It is not the source of truth. tmux
sessions have a lifecycle independent of CHROTE, and a CHROTE restart must not
deliberately terminate them. Recovery in the server is operator-triggered and
one-shot; nothing in it loops to keep a workload alive or promises host-reboot
recovery.

## Built-in cockpit surfaces

| Surface | Backing capability |
| --- | --- |
| Terminal 1-3 | Independent layouts over tmux sessions |
| Sessions/Files sidecar | Session discovery, Peek, assignment navigation, and workspace-local files |
| Files | Configured-root file operations and terminal handoff |
| Agents | Agent/persona/session observability and mission context |
| Beads | Configured `bd` workspaces and issue data |
| Formations (experimental, unreleased) | Boards, missions, ports, gates, connections, and run ledgers in development builds |
| Scheduled | Scheduled-task definitions, locks, runs, and history |
| Server | Health, resources, runtime events, and bounded system history |
| Settings | Appearance, terminal behavior, Session Bank, flags, and advanced recovery |

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

## Formations execution environments

Development builds that include Formations expose one file-backed authoring and
run-inspection surface. Formations remains unreleased. Executor promotion is a
separate safety ladder inside that experiment:

1. deterministic lab executor;
2. isolated tmux executor;
3. explicitly promoted live tmux executor.

The executor never gains permission to create or kill unrelated sessions merely
because the Formations UI is available. See [`FORMATIONS.md`](FORMATIONS.md) and
[`ARCHON.md`](ARCHON.md).

## Trust boundary

CHROTE runs with the Unix permissions of its service identity. Configured file
roots constrain CHROTE file APIs; they do not sandbox tmux agents. Service URLs,
tokens, socket mappings, and executable paths are private runtime configuration.
See [`SECURITY.md`](SECURITY.md).

## Not bundled or assumed

- Gastown
- Ralph
- a hosted identity provider
- a general-purpose IDE
- autonomous agent-to-agent chat
- a mandatory cloud service
