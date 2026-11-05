---
title: Environment reference
nav: Environment reference
summary: Every variable the CHROTE server reads, its default, its format, and what the installer sets.
sources: .env.example, docs/installation.md, SECURITY.md
---

# Environment reference

The server reads these when it starts. On an installed host, put your values in
`~/.config/chrote/secrets.env` and restart the service; see
[Configuration](configuration.md). **Installer** marks values `install.sh`
writes into `chrote.env`.

Lists use the separator shown: most are comma-separated, but
`CHROTE_LIBRARY_LINK_ROOTS` is colon-separated, like `PATH`.

## Network

| Variable | Default | What it does |
| --- | --- | --- |
| `HOST` | `127.0.0.1` (installer) | Bind address. Anything but loopback exposes a shell to whoever can route to it; see [Security](security.md) |
| `PORT` | `8094` (installer, from `--port`) | Listen port |
| `CORS_ORIGINS` | unset | Comma-separated browser origins accepted by the HTTP API and the terminal WebSocket, for a dev server or a front door that rewrites `Host`. Unset: same origin only. Not authentication |

The binary also takes `-host` and `-port` flags; `HOST` and `PORT` override them.

## Files and new sessions

| Variable | Default | What it does |
| --- | --- | --- |
| `CHROTE_ROOTS` | Installer: the workspace. Unset: `$HOME,/code,/vault` | Comma-separated roots the file API may touch, checked after symlink resolution. Not a sandbox |
| `CHROTE_WORKDIR` | Installer: the workspace. Unset: the first root | Directory new sessions start in |
| `CHROTE_MAX_UPLOAD_BYTES` | 64 MiB | Largest file upload, in bytes |

## tmux

| Variable | Default | What it does |
| --- | --- | --- |
| `CHROTE_TMUX_SOCKET` | Installer: `<you>=<your default tmux socket>`. Unset: no terminals | Comma-separated `unixUser=/absolute/socket` entries, in order. One entry lets requests omit the user; several require it. Paths must be absolute and canonical, and a user may appear once |
| `CHROTE_TERMINAL_USER_WORKDIRS` | unset | Comma-separated `unixUser=/path` new-session directories; other users use `CHROTE_WORKDIR`. A user listed twice: the server will not start |
| `CHROTE_TMUX_BIN` | `tmux` | The tmux binary |
| `CHROTE_TERMINAL_CREATION_COLS` | `200` | Width a session CHROTE creates starts at; values below 80 are ignored |
| `CHROTE_TERMINAL_CREATION_ROWS` | `50` | Height it starts at; values below 24 are ignored |

CHROTE never discovers or falls back to a tmux server it was not given. See
[Several Unix users](multi-user.md).

## State

| Variable | Default | What it holds |
| --- | --- | --- |
| `CHROTE_SCHEDULED_TASKS_DIR` | Installer: `$XDG_STATE_HOME/chrote/scheduled-tasks` | Scheduled tasks and their run history |
| `CHROTE_SESSION_DROPS_DIR` | Installer: `$XDG_STATE_HOME/chrote/session-drops` | Text and files staged by Send to Session |
| `CHROTE_AGENT_HOOKS_DIR` | Installer: `$XDG_STATE_HOME/chrote/agent-hooks` | Generated Claude Code settings files, one per launched session name |

`$XDG_STATE_HOME` defaults to `~/.local/state`. The binary's compiled fallbacks
for these three are a fixed system path, so **set all three when you run the
server outside the installer**.

## Launcher and agent events

| Variable | Default | What it does |
| --- | --- | --- |
| `CHROTE_LAUNCH_CONFIG` | unset: the launcher offers `Shell` in `~` | JSON naming harnesses, flags and folders. Invalid: the server will not start. See [The launcher](launcher.md) |
| `CHROTE_AGENT_EVENT_HOOK` | `chrote-agent-event` beside the server binary | The completion hook script. Missing: hooks are off, and the startup log says so |
| `CHROTE_AGENT_EVENT_URL` | — | Not a server setting. The server sets it in each session it creates, for the hook to post to |

## Theme

| Variable | Default | What it does |
| --- | --- | --- |
| `CHROTE_THEME_DIR` | unset: the embedded `chrote-violet` | Directory holding `theme.json` and `art/`. See [Theme](theme.md) |

## Beads

| Variable | Default | What it does |
| --- | --- | --- |
| `CHROTE_BD_COMMAND` | `bd` | The `bd` binary; use an absolute path if it is not on the service's `PATH` |
| `CHROTE_BEADS_WORKSPACES` | Installer: the workspace | Comma-separated Beads projects to read in addition to those discovered |
| `PATH` | Installer: the prefix's `bin` and your `PATH` at install time | Where the service finds `bd`, `git` and harness binaries |

## Brain (the Library)

| Variable | Default | What it does |
| --- | --- | --- |
| `CHROTE_LIBRARY_ROOT` | unset: "No library is configured" | The corpus, a git repository of Markdown. Set but missing or not a directory: the server will not start |
| `CHROTE_LIBRARY_AUTHOR` | unset: edits refused | `Name <email>` edits are committed as. Set in any other form: the server will not start |
| `CHROTE_LIBRARY_SEARCH_URL` | unset: search not configured | Search service answering `POST <url>/v1/library/search`. Set but not an `http(s)` URL: the server will not start |
| `CHROTE_LIBRARY_LINK_ROOTS` | unset: any Markdown file outside the corpus | **Colon-separated** directories a `projects/` symlink may point into |
| `CHROTE_LIBRARIAN_SESSION` | unset: not configured | The Librarian's tmux session |
| `CHROTE_LIBRARY_BEADS` | unset | The Beads project the Librarian works from |

## Residents

| Variable | What it names |
| --- | --- |
| `CHROTE_TENDER_SESSION`, `CHROTE_TENDER_FOLDER`, `CHROTE_TENDER_BEADS` | The Agents tab's tender: session, launch folder, Beads project |
| `CHROTE_CLERK_SESSION`, `CHROTE_CLERK_FOLDER`, `CHROTE_CLERK_BEADS` | The Beads tab's Clerk: the same three |

The Librarian takes its values from the Library variables, and the Services
tab's agent from `services.json`. See [Libraries and residents](integrations.md#residents).

## Services

| Variable | Default | What it does |
| --- | --- | --- |
| `CHROTE_SERVICES_CONFIG` | `services.json` beside `CHROTE_LAUNCH_CONFIG`, else `~/.config/chrote/services.json` | The controller socket and the tab's resident. See [Services](services.md) |

## Retired

| Variable | Status |
| --- | --- |
| `API_AUTH_TOKEN` | Ignored. The server logs a warning if it is set; it protects nothing |

## Installer-only

`install.sh` reads `CHROTE_INSTALL_PREFIX`, `CHROTE_SERVICE_DIR`,
`XDG_CONFIG_HOME` and `XDG_STATE_HOME` to move what it installs. It also treats
`CHROTE_TMUX_SOCKET` in its own environment as a request for cross-user socket
grants, which only a root install performs; see
[Several Unix users](multi-user.md).
