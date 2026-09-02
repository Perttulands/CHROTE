# CHROTE architecture

CHROTE is one browser application backed by one Go server. The server exposes host resources through HTTP APIs, serves the embedded React dashboard, and hosts browser terminals on pseudo-terminals it owns. tmux remains the owner of terminal sessions.

## System shape

```text
Browser
  |-- embedded React dashboard
  |-- JSON requests under /api/
  `-- terminal WebSocket under /terminal/
             |
        CHROTE Go server
          |-- tmux
          |-- configured filesystem roots
          |-- project Beads stores
          |-- scheduled-task state
          `-- configured service adapters
```

The browser never talks directly to a local service or receives its credentials. The Go server is the boundary between browser code and host resources.

## Sources of truth

CHROTE reads and controls existing resources instead of copying them into a central database.

| State | Authority | CHROTE's role |
| --- | --- | --- |
| Live terminals and processes | tmux | Discover, create, attach, display, and explicitly control sessions |
| Files | Host filesystem | Expose operations within configured roots |
| Project work | Each project's Beads store | Present and mutate work through `bd` |
| Schedules and service configuration | Host configuration and state directories | Provide an interface and run the configured behavior |
| Layouts and presentation | Browser storage | Render device-local workspaces |
| The interface theme | Host theme directory | Serve the active theme and the art it names |
| Launchable harnesses and folders | Host launch configuration | Offer the choices and start the chosen one in a new session |
| Runtime observations | CHROTE process memory and bounded history | Report health and recent events |

If CHROTE stops, tmux sessions, files, and Beads remain where they were.

## Terminal path

The dashboard asks the tmux API for sessions on configured sockets. Attaching a terminal opens the CHROTE terminal route with an exact socket and session identity.

The Go server runs the tmux attach on a pseudo-terminal it allocates and relays that pseudo-terminal over the WebSocket. It fails if the socket or session is not the requested target. It never falls back to an ambient tmux server.

CHROTE owns the pseudo-terminal and the attach client on it, and nothing else. It does not own the tmux server or the long-lived processes inside it. Shutdown and cleanup code must distinguish CHROTE-owned transport from operator-owned tmux work.

A displayed terminal is the session's one sizing client, so opening a session takes it over from whatever was attached, CHROTE's own client or an external one. Peek attaches as an observer and sizes nothing. A new session is sized once, at creation; no server-side loop revisits a window's size afterwards. Window bindings are operator intent held in browser storage, not a cache of live sessions, and the server holds no binding or tile state. [ADR-0017](docs/adr/0017-terminal-viewing-model.md) owns this model and [ADR-0018](docs/adr/0018-terminal-transport-ownership.md) owns the transport.

## Theme and launch configuration

Three parties share the interface's look, and each does one thing. A host apply script writes the active theme into the directory CHROTE reads; the server serves that document, unmodified, and refuses to guess when it is malformed; the dashboard applies it to its custom properties and to the terminal. CHROTE never writes a theme, and it no longer pushes appearance into tmux: the same apply script sets the tmux status bar and the agents' own settings once, in ANSI colour names, so a session looks the same to an SSH client in its own palette.

The launch configuration names what may be started and where. It is read once at startup; the browser learns only harness ids, labels and folders, and the commands stay on the server. An unreadable or invalid launch configuration stops startup rather than presenting a launcher that cannot launch.

## Server composition

`src/cmd/server/` assembles the process and registers the runtime routes.

- `src/internal/api/` contains tmux, files, Beads, scheduled tasks, services, theme, launch, health, and system handlers.
- `src/internal/proxy/` owns the terminal transport and the pseudo-terminals it attaches on.
- `src/internal/dashboard/` embeds the built dashboard into the Go binary.
- `src/internal/scheduled/` contains scheduled-task persistence and execution support.
- `dashboard/src/` contains the React interface, browser-local state, API clients, and views.

The build script compiles the dashboard and embeds its output into the Go server. The source tree, not a hand-copied distribution directory, is authoritative.

## Core and components

Terminal workspaces, sessions, files, server status, and settings form the core. Beads, Scheduled, Services, and future Formations integration are separate first-party modules.

This separation is static code organization, not a marketplace or dynamic plugin loader. Components can add routes, views, and configuration. Their failure must remain contained so the terminal core still loads and works.

## State and failure behavior

Browser-local layout state may disappear without losing host work. The server may restart without redefining sessions. A service adapter or Beads workspace may be unavailable without crashing the dashboard.

CHROTE can store bounded history for operator visibility. That history is evidence, not a replacement for the system that produced it.

The golden failure rule is non-interference. Product code, tests, installers, restarts, and cleanup paths must never implicitly or accidentally terminate or disrupt existing tmux sessions. Exact operator-authorized deletion and exact cleanup of resources created by a failed operation or isolated test remain valid.

## Trust boundary

CHROTE assumes one trusted operator and has no internal authentication system. The server binds to loopback by default. Tailscale or another operator-controlled private network provides remote reachability.

The service needs broad Unix access to the roots and tmux sockets it exposes. CHROTE relies on configured roots, canonical-path containment, and Unix permissions. It does not attempt to sandbox the programs running inside tmux.

## Design direction

Prefer direct adapters over duplicated state. Keep host-specific deployment details outside the public repository. Add a component when it represents a coherent operator job; keep one-off recovery and repair in explicit tools or agent skills.

Consequential architectural decisions live in [`docs/adr/`](docs/adr/). The live work needed to change this architecture lives in Beads.
