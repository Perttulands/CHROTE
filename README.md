# CHROTE

![CHROTE](CHROTE.png)

**C**ontrol **H**ub for **R**emote **O**perations & **T**mux **E**xecution

> **WARNING:** CHROTE is for people who looked at one AI coding agent, thought
> "good start," opened five more terminals, lost track of which one was doing
> what, and decided the correct answer was obviously a browser cockpit.
>
> **CAUTION:** This is private infrastructure for a machine you own. It assumes
> you are comfortable with tmux, local services, weird agent output, and the
> occasional need to kill everything and start over.

CHROTE is a browser cockpit for durable, host-owned AI development work. Your
terminal sessions, files, Beads, agents, builds, and recovery state live on the
host. The browser is just glass.

Close the laptop. Change devices. Reconnect later. The work should still be
there, because a browser tab was never qualified to own it.

[![CHROTE terminal workspace with Codex and Claude Code attached](docs/assets/readme/terminal-agents.png)](docs/assets/readme/attach-hermes-workflow.mp4)

<p align="center"><strong><a href="docs/assets/readme/attach-hermes-workflow.mp4">Watch the 18-second workflow</a></strong>: two attached agents, add a third window, attach Hermes.</p>

## Why this exists

One agent in one terminal is a workflow.

Five agents across tmux sessions, a Beads backlog, a file tree, recovery state,
and three half-finished builds is a control-room problem.

CHROTE is the control room. It does not replace the terminal, move durable state
into the browser, or pretend agent work is calm and linear. It gives the mess
handles.

This is not an autonomous software factory. It is not a chat app wearing a
trench coat. It is a cockpit for the host where the work actually lives.

## What is on `main`

| Surface | Operator job |
| --- | --- |
| **Terminal 1-3** | Three independent workspaces, each with one to four windows attached to real tmux sessions |
| **Sessions / Files sidecar** | Find, Peek, attach, navigate, and inspect files without leaving the active terminal workspace |
| **Files** | Browse, inspect, edit, compare, and send configured workspace files to a session |
| **Beads** | Inspect configured `bd` workspaces, issues, ready work, triage, and health |
| **Agents** | Observe agent-like tmux sessions without requiring one blessed harness |
| **Recovery** | Distinguish live, offline, recoverable, and unmanaged work through Session Bank and typed recovery plans |
| **Scheduled / Server** | Inspect scheduled tasks, health, resources, runtime events, and bounded history |
| **Services** | Host optional server-side adapters without putting private tokens in the browser |

The terminal remains the heart of the thing. tmux owns process durability;
CHROTE owns the browser view and explicit operator actions.

A few interaction rules are deliberate:

- Session-row clicks mean **Peek**. They do not silently reassign a window.
- The location chip navigates to an already attached session.
- The Sessions/Files sidecar is closed by default and takes zero width while
  closed. Pinning and width persist per terminal workspace.
- At `768px` and below, the sidecar overlays the terminals instead of crushing
  them.
- Browser disconnects must not kill tmux work.

Empty terminal windows have guardians. They do not execute code or solve race
conditions. They sit there judging the empty pane until you attach something.
Some things are load-bearing.

## Beads without leaving the cockpit

Agents can ramble. Beads are supposed to say what the work actually is.

CHROTE reads configured `bd` workspaces and surfaces issue state, ready work,
triage, dependencies, and project health. `bd` remains the source of truth;
CHROTE does not invent a second issue database.

![CHROTE Beads Kanban with synthetic public demo issues](docs/assets/readme/beads-kanban.png)

## Who this is for

**This is probably for you if:**

- you already run agent CLIs in tmux;
- closing a laptop should not kill the work;
- you want terminals, files, Beads, recovery, and local services in one private
  browser surface;
- you think agents need supervision, not mythology;
- you want your own cockpit, not somebody else's control plane.

**This is not for you if:**

- you want a hosted multi-tenant SaaS product;
- you expect built-in accounts, password resets, and public-internet hardening;
- you want the browser to become the workspace source of truth;
- you need an OS sandbox around terminal agents;
- you have never used tmux and have no interest in starting.

## Quick start

The supported public path is a same-user Linux or WSL user service built from an
inspectable checkout.

Requirements: Linux or WSL with user systemd, Go 1.26.5+, Node.js 20.19+ or 22.12+,
npm, tmux, curl, and Git.

```bash
git clone https://github.com/Perttulands/CHROTE.git
cd CHROTE
./install.sh --workspace "$HOME/work"
```

Open <http://127.0.0.1:8094>.

The installer builds the dashboard and embedded Go binary from the checkout,
installs a user service, starts it, and checks `/api/health`. It does not require
`sudo`. Giving CHROTE all of `$HOME` is possible, but a narrower workspace is the
better default.

Read [the installation guide](docs/installation.md) before changing ports,
prefixes, service configuration, or remote access. Use
[troubleshooting](docs/troubleshooting.md) when a supported install is not
healthy.

## Trust boundary

CHROTE has **no built-in application login**. Anyone who can reach it is inside
the trusted operator boundary and can reach terminal-grade capabilities.

The sane deployment shape is:

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

Keep CHROTE bound to loopback. Put remote access behind an
operator-controlled private network such as Tailscale. Do not expose it directly
to the public internet. CORS is not authentication, and configured file roots do
not sandbox commands running in tmux.

Read [SECURITY.md](SECURITY.md) before treating this as anything less powerful
than remote shell access.

## Release status

CHROTE v2 is alpha.

- Latest tagged alpha: `v2.0.0-alpha.1`
- Current `main` development version: `2.0.0-alpha.2-dev`
- Legacy v1: preserved for archaeology, not supported

This README describes current `main`. A development version is not a published
release, and an older downloadable binary is not equivalent to current source.

Formations and Archon work exists on `main`, but it is **experimental and
unreleased**. Its specs describe the experiment; they do not put it in the
supported install promise or latest tagged alpha.

## Architecture

CHROTE is a Go server with an embedded React dashboard. The dashboard is built,
copied into the Go tree, and baked into the server binary with `go:embed`. One
process serves the UI, API, recovery state, and loopback terminal proxy.

The hard state stays boring and inspectable:

- tmux sessions for durable terminal work;
- files under configured roots;
- `bd` workspaces for issue state;
- host-owned configuration and recovery records;
- Git for source and history.

The browser is disposable. The work is not.

## Documentation

| Need | Start here |
| --- | --- |
| Current product and roadmap boundary | [PRD.md](PRD.md) |
| Install or upgrade | [docs/installation.md](docs/installation.md) |
| Troubleshoot | [docs/troubleshooting.md](docs/troubleshooting.md) |
| Security model | [SECURITY.md](SECURITY.md) |
| Contribute and reproduce CI | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Component map | [COMPONENTS.md](COMPONENTS.md) |
| Release history | [CHANGELOG.md](CHANGELOG.md) |
| Documentation authority | [docs/source-truth-index.md](docs/source-truth-index.md) |

Plans and archives are context, not product authority. When docs disagree, the
[source-truth index](docs/source-truth-index.md) says which one wins.

## Development

```bash
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh

cd src
GOTOOLCHAIN=go1.26.5 go test ./...
go build ./cmd/server
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full local and CI verification
contract. Do not build a release from stale embedded dashboard assets.

## License

MIT. Open source for people who want their own cockpit, not somebody else's
control plane.
