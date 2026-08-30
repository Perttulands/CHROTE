# CHROTE

![CHROTE](CHROTE.png)

**C**ontrol **H**ub for **R**emote **O**perations & **T**mux **E**xecution

> **WARNING:** CHROTE is for people who opened several AI coding agents, lost
> track of which terminal was doing what, and decided the sensible answer was a
> browser cockpit.
>
> **CAUTION:** This is private infrastructure for a machine you own. It exposes
> terminal-grade capabilities and assumes you are comfortable with tmux.

CHROTE is one person's browser cockpit for host-owned work. tmux sessions,
files, Beads, schedules, builds, tests, and agent harness state stay on the host.
The browser is glass, not the vault.

[![CHROTE terminal workspace with Codex and Claude Code attached](docs/assets/readme/terminal-agents.png)](docs/assets/readme/attach-hermes-workflow.mp4)

<p align="center"><strong><a href="docs/assets/readme/attach-hermes-workflow.mp4">Watch the 18-second workflow</a></strong>: two attached agents, add a third window, attach Hermes.</p>

## Why this exists

One agent in one terminal is a workflow. Several agents across tmux sessions, a
Beads backlog, a file tree, and half-finished builds are a control-room problem.

CHROTE gives that work handles without taking ownership away from the tools that
created it. Close the browser, change devices, and reconnect: externally owned
tmux work remains because the browser never owned it. CHROTE itself does not
recreate a process that dies or a workload lost with the host.

This is a supervised cockpit, not an autonomous software factory. The operator
decides what runs, what changes, and what ships.

## What is on `main`

| Surface | Operator job |
| --- | --- |
| **Terminal 1–3** | Independent workspaces with one to four windows attached to real tmux sessions |
| **Sessions / Files sidecars** | Find, Peek, attach, navigate, and inspect files beside the active terminal |
| **Files** | Browse, inspect, edit, compare, and send configured workspace files to a session |
| **Beads** | Inspect configured `bd` workspaces, issues, ready work, triage, and health |
| **Scheduled** | Create and inspect literal prompt deliveries to configured tmux sessions |
| **Server** | Inspect health, resources, runtime events, and bounded history |
| **Settings** | Configure appearance, terminal behavior, and browser-owned layout preferences |
| **Services** | Use optional server-side adapters without putting private tokens in the browser |

The terminal is the heart of the product. A few interaction rules are
deliberate:

- Session-row clicks mean **Peek**; they do not reassign a window.
- Location chips navigate to already attached sessions.
- Sessions and Files sidecars are closed by default and reserve no width while
  closed.
- Sessions presentation persists application-wide; Files state persists per
  terminal workspace.
- Narrow layouts overlay sidecars rather than crushing terminals.
- Browser disconnects and CHROTE restarts must not kill external tmux work.

Rare recovery, restore, cleanup, migration, and repair remain agent skills or
operator procedures. Experimental Formations and Archon work lives separately
in [chrote-agent-formations](https://github.com/Perttulands/chrote-agent-formations),
not in CHROTE's release promise.

## Beads without leaving the cockpit

CHROTE reads configured modern `bd` workspaces and surfaces issue state, ready
work, triage, dependencies, and health. `bd` remains the source of truth; CHROTE
does not invent a second issue database.

![CHROTE Beads Kanban with synthetic public demo issues](docs/assets/readme/beads-kanban.png)

## Who this is for

This is probably for you if you already run agent CLIs in tmux, want terminals,
files, Beads, and operational state in one private surface, and prefer supervised
work to agent mythology.

It is not for you if you want hosted SaaS, built-in accounts, an OS sandbox, or
the browser to become the workspace source of truth.

## Quick start

The supported public path is a same-user Linux or WSL user service built from an
inspectable checkout.

Requirements: Linux or WSL with user systemd, Go 1.26.6+, Node.js 20.19+ or
22.12+, npm, tmux, curl, and Git.

```bash
git clone https://github.com/Perttulands/CHROTE.git
cd CHROTE
./install.sh --workspace "$HOME/work"
```

Open <http://127.0.0.1:8094>.

The installer builds the dashboard and embedded Go binary, installs a user
service, starts it, and checks `/api/health`. It does not require `sudo`.

Read [the installation guide](docs/installation.md) before changing ports,
prefixes, or remote access. Use [troubleshooting](docs/troubleshooting.md) when a
supported install is not healthy.

## Trust boundary

CHROTE has **no built-in login**. Anyone who can reach it is inside the trusted
operator boundary and can reach terminal-grade capabilities.

```text
browser
  |
private network / HTTPS
  |
loopback CHROTE server
  |
  +-- tmux + ttyd
  +-- configured file roots
  +-- configured Beads workspaces
  +-- optional local adapters
```

Keep CHROTE on loopback and place remote access behind an operator-controlled
private network. CORS is not authentication. File roots constrain CHROTE APIs;
they do not sandbox commands running in tmux. Read [SECURITY.md](SECURITY.md).

## Release status

CHROTE v2 is alpha. The latest tagged alpha is `v2.0.0-alpha.1`; current `main`
is `2.0.0-alpha.2-dev`. An older downloadable binary is not equivalent to
current source.

## Architecture

CHROTE is a Go server with an embedded React dashboard. One process serves the
UI, APIs, scheduled tasks, optional service adapters, and loopback terminal
proxy. Durable state stays inspectable: tmux, configured files, `bd` workspaces,
host-owned configuration, and Git.

## Documentation

| Need | Start here |
| --- | --- |
| Product and roadmap boundary | [PRD.md](PRD.md) |
| Install or upgrade | [docs/installation.md](docs/installation.md) |
| Troubleshoot | [docs/troubleshooting.md](docs/troubleshooting.md) |
| Security model | [SECURITY.md](SECURITY.md) |
| Contribute and reproduce CI | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Component map | [COMPONENTS.md](COMPONENTS.md) |
| Documentation authority | [docs/source-truth-index.md](docs/source-truth-index.md) |

## Development

```bash
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh
cd src
go test ./...
go build ./cmd/server
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full verification contract.

## License

MIT. Open source for people who want their own cockpit, not somebody else's
control plane.
