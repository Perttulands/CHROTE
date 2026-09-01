# CHROTE architecture

CHROTE is one browser application backed by one Go server. The server exposes host resources through HTTP APIs, serves the embedded React dashboard, and proxies browser terminals to ttyd. tmux remains the owner of terminal sessions.

## System shape

```text
Browser
  |-- embedded React dashboard
  |-- JSON requests under /api/
  `-- terminal WebSocket under /terminal/
             |
        CHROTE Go server
          |-- tmux and ttyd
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
| Runtime observations | CHROTE process memory and bounded history | Report health and recent events |

If CHROTE stops, tmux sessions, files, and Beads remain where they were.

## Terminal path

The dashboard asks the tmux API for sessions on configured sockets. Attaching a terminal opens the CHROTE terminal route with an exact socket and session identity.

The Go server proxies that connection to its ttyd child. ttyd runs the terminal launcher, which attaches to the requested tmux session. It fails if the socket or session is not the requested target. It never falls back to an ambient tmux server.

CHROTE may supervise the ttyd child it owns. It does not own the tmux server or the long-lived processes inside it. Shutdown and cleanup code must distinguish CHROTE-owned transport from operator-owned tmux work.

## Server composition

`src/cmd/server/` assembles the process and registers the runtime routes.

- `src/internal/api/` contains tmux, files, Beads, scheduled tasks, services, health, and system handlers.
- `src/internal/proxy/` owns the ttyd child and terminal transport.
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
