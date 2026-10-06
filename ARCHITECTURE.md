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
| An agent's completion | Each harness's own completion hook | Install the hook per launch and keep the last report per session, in memory, until the operator looks |
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

A launched agent's completion is a fact the harness states, not one CHROTE infers. Each launch installs the harness's own completion hook through the harness's flags; the hook is the `chrote-agent-event` script installed beside the server, which asks tmux for its session's name and posts to `POST /api/agent/event`. The server keeps the last report per session in memory, attaches it to the session list, and forgets it with the session; `POST /api/agent/event/seen` records that the operator looked. No tmux activity heuristic and no poll of the harness stands in for the hook.

## Server composition

`src/cmd/server/` assembles the process and registers the runtime routes.

- `src/internal/api/` contains tmux, files, Beads, scheduled tasks, the Library, the residents, theme, launch, health, and system handlers.
- `src/internal/proxy/` owns the terminal transport and the pseudo-terminals it attaches on.
- `src/internal/dashboard/` embeds the built dashboard into the Go binary.
- `src/internal/scheduled/` contains scheduled-task persistence and execution support.
- `dashboard/src/` contains the React interface, browser-local state, API clients, and views.

The keyboard chord registry is dashboard state: the chords, their scopes, and whether CHROTE intercepts keys at all live in the browser, and the server has neither a route nor a record for them.

The build script compiles the dashboard and embeds its output into the Go server. The source tree, not a hand-copied distribution directory, is authoritative.

## Core and components

Terminal workspaces, sessions, files, server status, and settings form the core.
Beads, Scheduled and the Library are separate first-party modules.

This separation is static code organization, not a marketplace or dynamic plugin loader. Components can add routes, views, and configuration. Their failure must remain contained so the terminal core still loads and works.

## State and failure behavior

Browser-local layout state may disappear without losing host work. The server may restart without redefining sessions. A service adapter or Beads workspace may be unavailable without crashing the dashboard.

The selected object's table has one persistent content owner. Views provide
placements for that same table, so switching views preserves its drafts, loaded
content and scroll position. A table failure is contained separately from the
workspace and its terminals.

The Files, Beads, Agents, Library and Server views, and the Beads column, start
loading on first use and retain their state after that. Retained state does not
require continuous requests: the Server view reads
only while visible, with independent bounded status and history requests.
Concurrent metadata reads share in-flight work. Beads additionally retains
disposable successful reads so host and project navigation can show useful work
immediately. One reader per owning store shares a verified full snapshot across
counts, unfinished work, requested Closed work and card relationships. Source
fingerprints must agree before and after a read; a changing or unavailable store
keeps its previous snapshot with explicit pending or error state. Checking the
source and reading its records have separate timestamps. Unknown counts remain
absent, and time-based deferrals are evaluated without reopening the store.
The reader retains at most 128 stores and an estimated 128 MiB of decoded
snapshots, using four times their JSON size as its working estimate. Idle entries
expire after ten minutes. Background snapshot and prefix reads share two command
admissions; selected project and card demand promotes the existing job within
the command runner's four-slot bound. Failed reads retry through demand with
backoff from one to thirty seconds. These bounds keep the cache disposable and leave capacity for
interactive work.

The browser's Beads reader shares demand from visible reading surfaces and the
table. It publishes each project independently and checks source state about
every two seconds while the document is visible. While a demanded foreground
store or card has an active read, the same timer checks every 250 milliseconds
so completed data need not wait for another ordinary interval. Background-only
pending stores keep the ordinary cadence. Successful projections retain
their own applied generation; a failed request can retry that same generation.
Refresh keeps loaded views and the table mounted, preserving reading context
and drafts. Hidden surfaces retain their data without continuing refresh traffic.
Closed remains a lazy presentation demand. A lightweight Bead catalog supplies
terminal links independently of counts and work reads. Its projects response
distinguishes a successful empty store from a failed identity lookup with an
optional per-project `prefixError`. Known positive prefixes remain useful;
identity warnings do not make a healthy snapshot unreadable. Explicit project
refresh replaces those warnings on successful identity discovery, while cheap
workspace discovery preserves the latest identity result. Session discovery and
completion notifications retain their own background lifetime.

The snapshot log separates the reader queue from command admission, wrapped
process time, JSON decoding and snapshot construction, and counts actual source
processes and verification attempts. Its legacy `command` duration includes the
whole refresh through publication fingerprint and bookkeeping. These same-job
measurements diagnose repeated verified reads without removing the generation
guard or attributing wrapper execution to intrinsic source cost.

The command runner bounds execution and cleans its owned descendants. When a
successful command leaves an output pipe open, completed JSON may proceed after
that bounded cleanup through the existing decoder, caller shape checks and
snapshot generation guard. Failed or canceled commands and incomplete payloads
remain errors; plain version output retains the runner's transport contract.

Saved terminal bindings establish their first connection when displayed at real
layout dimensions. Once started, their pooled connection and frame survive
hiding or moving the terminal, as specified by ADR-0017.

CHROTE can store bounded history for operator visibility. That history is evidence, not a replacement for the system that produced it.

The golden failure rule is non-interference. Product code, tests, installers, restarts, and cleanup paths must never implicitly or accidentally terminate or disrupt existing tmux sessions. Exact operator-authorized deletion and exact cleanup of resources created by a failed operation or isolated test remain valid.

## Trust boundary

CHROTE assumes one trusted operator and has no internal authentication system. The server binds to loopback by default. Tailscale or another operator-controlled private network provides remote reachability.

The service needs broad Unix access to the roots and tmux sockets it exposes. CHROTE relies on configured roots, canonical-path containment, and Unix permissions. It does not attempt to sandbox the programs running inside tmux.

## Design direction

Prefer direct adapters over duplicated state. Keep host-specific deployment details outside the public repository. Add a component when it represents a coherent operator job; keep one-off recovery and repair in explicit tools or agent skills.

Consequential architectural decisions live in [`docs/adr/`](docs/adr/). The live work needed to change this architecture lives in Beads.
