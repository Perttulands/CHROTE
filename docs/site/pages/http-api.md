---
title: HTTP API
nav: HTTP API
summary: The routes the dashboard uses, grouped by what they touch. Useful for scripts and agents on the same host.
sources: src/cmd/server/main.go, ARCHITECTURE.md, docs/scheduled-tasks.md, docs/installation.md
---

# HTTP API

The dashboard is the API's main client, and the API is not versioned yet: v2 is
alpha, and routes other than those with their own contract pages may change.
Every route below has the same trust model as the dashboard: whoever can reach
it holds a shell. See [Security](security.md).

JSON routes live under `/api/`. The terminal is a WebSocket under `/terminal/`.
Mutations that matter carry an intent header: `X-Requested-With: chrote` for
service actions, `X-Chrote-Intent: scheduled-task` for scheduled-task changes,
and `X-Nuke-Confirm: DASHBOARD-NUKE-CONFIRMED` for killing all sessions; without
it that route answers `403`.

## Health and build

| Route | Answers |
| --- | --- |
| `GET /api/health` | `{ status, version, commit, timestamp }` |
| `GET /api/version` | The version and build details |
| `GET /api/system/status` | Host, CPU, memory, disks, network, GPUs, warnings |
| `GET /api/system/history` | The bounded sample history |

## tmux sessions

| Route | Does |
| --- | --- |
| `GET /api/tmux/sessions` | Sessions on every configured socket, with each one's last agent event |
| `POST /api/tmux/sessions` | Create a session: name, Unix user, harness id, optional flags |
| `PATCH /api/tmux/sessions/{name}` | Rename |
| `DELETE /api/tmux/sessions/{name}` | Kill exactly that session |
| `DELETE /api/tmux/sessions/all` | Kill every session on one user's tmux server (Settings' emergency action). Needs `X-Nuke-Confirm: DASHBOARD-NUKE-CONFIRMED`; takes `?unixUser=`, required when several users are configured |
| `GET /api/tmux/sessions/{name}/panes` | The session's panes |
| `GET /api/tmux/sessions/{name}/capture` | Captured pane text |
| `POST /api/tmux/sessions/{name}/send` | Send to Session: text and files, guarded paste, optional submit |
| `POST /api/tmux/mouse` | Set tmux's mouse mode, an explicit operator action |

## Launch, theme, residents

| Route | Answers |
| --- | --- |
| `GET /api/launch` | Harness ids, labels, binaries, default flags, catalogues, and folders |
| `GET /api/theme` | The active theme document; `500` naming the field if it does not validate |
| `GET /api/theme/art/{name}` | One art file from the theme directory |
| `GET /api/residents` | `[{ tab, label, session, folder, beads }]`, one per resident tab |
| `GET /api/workspaces` | Folders that are workspaces: session folders, Beads projects, git roots |

## Agent events and context

| Route | Does |
| --- | --- |
| `POST /api/agent/event` | `{ session, unixUser, event, summary? }`, `event` is `finished` or `needs-input`; `404` for an unknown session |
| `POST /api/agent/event/seen` | `{ session, unixUser? }` marks the last event seen |
| `GET /api/agent/context` | What a harness would load in a folder: instructions, skills, memories |
| `GET /api/agent/file` | One of those files |

## Files

| Route | Does |
| --- | --- |
| `GET /api/files/resources/{path…}` | List a directory or read file metadata |
| `POST /api/files/resources/{path…}` | Create a folder, upload, or save a file |
| `PATCH /api/files/resources/{path…}` | Rename or move |
| `DELETE /api/files/resources/{path…}` | Delete |
| `GET /api/files/raw/{path…}` | File bytes, for viewing and download |
| `GET /api/files/find` | Find by name under the roots |
| `GET /api/files/diff` | The file against its committed state |

Every path must resolve inside `CHROTE_ROOTS` after symlinks.

## Beads

| Route | Does |
| --- | --- |
| `GET /api/beads/health` | Whether `bd` is usable |
| `GET /api/beads/host` | The host table: every store's last read |
| `GET /api/beads/projects` | Configured and manual projects |
| `GET /api/beads/search`, `/text`, `/issue` | Search ids by text; one Bead's long text; one Bead as its card reads it |
| `GET /api/beads/work`, `/state` | One project's unfinished work; store state |
| `POST /api/beads/issues` | File a Bead (the report box) |

## Brain (Library)

| Route | Does |
| --- | --- |
| `GET /api/library/shelves` | Root, shelves and counts, and the Librarian's names |
| `GET /api/library/pages`, `/page`, `/changes`, `/graph` | The tree, a page, recent changes, the link graph |
| `GET /api/library/search` | Forwarded to the configured search service |
| `PUT /api/library/page` | Save one page, committed as the configured author |

## Scheduled tasks

| Route | Does |
| --- | --- |
| `GET /api/scheduled-tasks`, `GET /api/scheduled-tasks/{id}` | List, read |
| `POST /api/scheduled-tasks` | Create |
| `PATCH /api/scheduled-tasks/{id}` | Update; `targets` replaces the list |
| `POST /api/scheduled-tasks/{id}/pause`, `/resume`, `/run-now` | Act |
| `DELETE /api/scheduled-tasks/{id}` | Delete |

The schema, examples and safety contract are in the
[scheduled tasks reference](../../scheduled-tasks.md).

## Services

| Route | Does |
| --- | --- |
| `GET /api/services` | The controller's catalog |
| `POST /api/services/{id}/start`, `/stop` | Forwarded to the controller; requires `X-Requested-With: chrote` |

See [Services](services.md#the-controllers-contract).

## Terminal

`/terminal/` upgrades to a WebSocket that relays a pseudo-terminal running
`tmux attach` for one named session on one configured socket. The `Origin`
check is described in [Security](security.md#browser-origins-are-not-authentication).
The transport is owned by
[ADR-0018](../../adr/0018-terminal-transport-ownership.md).
