# Installing CHROTE

> **Scope: a fresh, from-scratch install.** Everything below — the
> `chrote.service` user unit and port `8094` — describes what
> `install.sh` creates on a machine that has never run CHROTE, using the
> compiled defaults. It is **not** a description of any already-operated
> deployment: an existing host may run a different unit, port, and socket,
> recorded only in local operator configuration. Diagnosing or restarting an
> operated deployment from this page targets the wrong process — resolve the
> real target from local operator context first, and ask instead of guessing.

CHROTE v2 is alpha. The canonical public installation is a **same-user Linux or
WSL user service built from the checked-out source**. This keeps the installed
binary tied to an inspectable commit instead of silently selecting an older
prerelease artifact.

The installer does not use `sudo` or create a dedicated Unix user. The Go
server serves terminals itself and connects to the installing user's normal tmux
server.

## Requirements

- Linux or WSL with user systemd available
- Go 1.26.6+
- Node.js 20.19+ or 22.12+
- npm
- Python 3, which the dashboard build uses to stamp the embedded bundle
- tmux
- `setfacl`, from the distribution's `acl` package: Send to Session grants
  each drop to its target Unix user with a POSIX ACL, and the installer stops
  without it
- curl
- Git

Go, Node.js, npm and Python 3 are build requirements; `./install.sh --binary
PATH` installs an already-built server without them.

If `CHROTE_TMUX_SOCKET` is set in the installing shell, the installer treats it
as a request for cross-user socket grants, which only a root install performs,
and stops. Run an ordinary install with `env -u CHROTE_TMUX_SOCKET ./install.sh`.

A user service normally stops when the user's last login session ends. To keep
CHROTE reachable while logged out, enable lingering with
`loginctl enable-linger "$USER"`, which some distributions reserve for
administrators.

## Install

```bash
git clone https://github.com/Perttulands/CHROTE.git
cd CHROTE
./install.sh
```

Defaults:

| Setting | Default |
| --- | --- |
| Dashboard | `http://127.0.0.1:8094` |
| Workspace/file root | `$HOME` |
| Binary prefix | `$HOME/.local` |
| Managed config | `$XDG_CONFIG_HOME/chrote/chrote.env` or `$HOME/.config/chrote/chrote.env` |
| Private overrides | `$XDG_CONFIG_HOME/chrote/secrets.env` |
| Durable state | `$XDG_STATE_HOME/chrote` or `$HOME/.local/state/chrote` |
| Disposable cache | `$XDG_CACHE_HOME/chrote` or `$HOME/.cache/chrote`: the Beads tab's last read of each store |
| Server user unit | `$XDG_CONFIG_HOME/systemd/user/chrote.service` |

The installer:

1. builds the dashboard and exact embedded Go binary from the checkout;
2. stamps the version from `VERSION` and the checkout's HEAD commit;
3. installs `chrote-server` and the `chrote-agent-event` hook script under the
   user prefix, side by side;
4. writes XDG-scoped state paths for schedules, session drops and generated
   agent hooks;
5. writes the `chrote.service` user unit that runs the cockpit itself;
6. enables, starts, and health-checks the service.

No workspace files are copied into CHROTE state. CHROTE does not install
per-session supervision units or promise to recreate ordinary sessions after
process death or host reboot. Workloads with a demonstrated durability need use
explicit operator-owned host configuration.

## Choose a narrower workspace

Giving CHROTE all of `$HOME` is convenient but broad. Prefer a dedicated
workspace when practical:

```bash
./install.sh --workspace "$HOME/work"
```

That path becomes:

- `CHROTE_ROOTS`
- `CHROTE_WORKDIR`
- the default tmux working directory
- the initial Beads discovery root

Configured roots constrain CHROTE file APIs. They do not sandbox commands or AI
agents running in tmux.

## Custom ports or prefix

```bash
./install.sh \
  --workspace "$HOME/work" \
  --port 8094 \
  --prefix "$HOME/.local"
```

The dashboard port remains loopback-only.

The installer does not read an earlier install's choices: every run rewrites
`chrote.env` from its own options and defaults. Repeat `--workspace` and
`--port` whenever you re-run it, or the file root returns to `$HOME` and the
port to `8094`.

## Verify

```bash
systemctl --user status chrote.service
curl http://127.0.0.1:8094/api/health
curl http://127.0.0.1:8094/api/theme
curl http://127.0.0.1:8094/api/launch
```

`GET /api/theme` returns the active theme, `GET /api/launch` the harnesses and
folders the launcher offers. On a fresh install both answer with built-in
defaults; see [Environment](#environment).

Open `http://127.0.0.1:8094` and create or select a tmux session. CHROTE runs as
the same Unix user and sees that user's normal tmux server by default.

To test without changing systemd state, packaging and contributors may use:

```bash
./install.sh --no-systemd --no-start
```

That still writes the unit and install layout but does not reload, enable, or
start the user service.

## Configure optional capabilities

The installer writes the managed values to:

```text
~/.config/chrote/chrote.env
```

It rewrites that file completely on every run, so an edit there lasts only
until the next install or upgrade. Put every setting of your own, optional
capabilities and tokens alike, in:

```text
~/.config/chrote/secrets.env
```

The installer creates `secrets.env` once and never rewrites it. The unit loads
it after `chrote.env`, so a value there also overrides a managed one, such as
`CHROTE_ROOTS`.

Then reload:

```bash
systemctl --user restart chrote.service
```

Common advanced settings are documented in [`.env.example`](../.env.example).
Cross-user terminal sockets require deliberate host setup and are not enabled by
the generic installer. CHROTE may apply explicitly configured additive tmux
access grants, but must not remove or narrow the session owner's access.

## Environment

A few optional variables move presentation, launch and library choices out of
the binary and into operator configuration. Set them in `secrets.env` and
restart the service. All are unset on a fresh install, and CHROTE runs without any of
them.

### `CHROTE_THEME_DIR`

The directory holding the one active theme: a `theme.json` with an `art/`
directory beside it, written by whatever host tooling authors themes.

```json
{
  "schema": 1,
  "name": "example-dark",
  "ui": {
    "background": "#0f0f0f", "surface": "#1a1a1a", "surfaceRaised": "#252525",
    "divider": "#3a3a3a", "text": "#e5e5e5", "textSecondary": "#a3a3a3",
    "textDim": "#737373", "accent": "#6b9fff", "error": "#f87171"
  },
  "terminal": {
    "background": "#0a0a0a", "foreground": "#e5e5e5", "cursor": "#e5e5e5",
    "selectionBackground": "#6b9fff40",
    "ansi": [
      "#0f0f0f", "#f87171", "#8bd450", "#e5c07b", "#6b9fff", "#c084fc", "#45d6d6", "#a3a3a3",
      "#737373", "#ff8a8a", "#a6e37a", "#f0d48a", "#8fb5ff", "#d3a4ff", "#7ae2e2", "#ffffff"
    ]
  },
  "identity": ["#4f6d8f", "#8f6f3a"],
  "art": ["example.webp"]
}
```

Every colour is `#rrggbb` or `#rrggbbaa`. `terminal.ansi` has exactly sixteen
entries, `identity` at least one, and identity colours are assigned to the
configured terminal users in order. Art names match `[A-Za-z0-9._-]+`, hold no
path separator, and are served from `art/` under the same directory by
`GET /api/theme/art/{name}`.

Unset, or naming a directory with no `theme.json`: the server serves its
embedded default, the same document with no art. A `theme.json` that exists but
does not validate is an error rather than a fallback, so a broken edit is
visible instead of looking deliberate.

### `CHROTE_LAUNCH_CONFIG`

A JSON file naming what a new session may start, with which flags, and where.

```json
{
  "harnesses": [
    {
      "id": "example-agent",
      "label": "Example Agent",
      "command": "example-agent",
      "defaultFlags": "--flag",
      "flags": [
        {"name": "--flag", "description": "What the flag does"},
        {"name": "--model", "short": "-m", "value": "<model>", "description": "Model to use"},
        {"name": "--mode", "value": "<MODE>", "values": ["a", "b"], "description": "One of two modes"}
      ]
    },
    {"id": "shell", "label": "Shell", "command": ""}
  ],
  "folders": ["/absolute/path/to/project", "~"]
}
```

Ids match `[a-z0-9-]+` and are unique. `command` is the binary; `defaultFlags`
is the line the launcher prefills, which the operator may edit before launching;
`flags` is an optional catalogue the launcher's panel lists and searches, where
only `name` and `description` are required. An entry that still carries flags
inside `command` is split on load: first token as the binary, the rest as
`defaultFlags`. `shell` is the bare login shell with an empty command, no
default flags and no catalogue; it is offered whether or not the file lists it.
A folder is absolute, or starts with `~` and resolves against the target Unix
user's home. `GET /api/launch` returns each harness's id, label, binary,
default flags and catalogue, and the folders. A session is created by naming a
harness id and, optionally, a one-line `flags` string; the server types
`binary flags` into the new session. Control characters in `flags` are
refused.

Unset: the launcher offers `Shell` in `~`. Set but unreadable or invalid: the
server refuses to start and logs the reason, instead of running a launcher that
cannot launch.

### Agent events

CHROTE learns that an agent finished a turn, or is waiting on the operator,
from the harness's own completion hook, never from guessing at terminal output.
The hook is the `chrote-agent-event` script, a POSIX `sh` script the installer
places beside `chrote-server`. It asks tmux for the name of the session it runs
in and posts `POST /api/agent/event` with `{ session, unixUser, event, summary? }`,
where `event` is `finished` or `needs-input`. It bounds its request to two
seconds and always exits 0: no notification is worth failing the harness for.

The launcher installs the hook per launch when its **Notify on completion**
setting is on (the default), through the harness's own flags: Claude Code
(harness id `claude-code`) gets `--settings <file>` naming a generated settings
file whose `Stop` hook reports `finished` and whose `Notification` hook reports
`needs-input`; Codex (harness id `codex`) gets `-c notify=[...]` naming the
script. Other harness ids launch as they are.

| Variable | What it names |
| --- | --- |
| `CHROTE_AGENT_EVENT_HOOK` | The script's path. Unset: `chrote-agent-event` beside the server binary. Missing there: hooks are off, and the startup log says so. |
| `CHROTE_AGENT_HOOKS_DIR` | Where the generated Claude Code settings files are written, one per Unix user and session name, world-readable, overwritten by the next launch of that name. The installer sets it under the state directory. |
| `CHROTE_AGENT_EVENT_URL` | Not a server setting: the server sets it in every session it creates while hooks are configured, as the address the script posts to (`http://<bind host>:<port>/api/agent/event`, loopback for a bind to every interface). A hook wired by hand in a session CHROTE did not create must set it itself. |

The server keeps each session's last event in memory as `lastEvent:
{ event, time, summary?, seen }` on `GET /api/tmux/sessions`, and forgets it
with the session. `POST /api/agent/event/seen` with `{ session, unixUser? }`
marks it seen; the dashboard calls that when the session is focused. A report
about a session that does not exist on the user's socket is answered `404`.
Nothing is persisted and nothing polls the harness.

How a device passes a report on is that device's own choice, under Settings,
Agent events. Marking the session's row and the terminal tab holding it, and
naming the session in the toast, are on; a tone and a browser notification
while the tab is hidden are off. The browser grants notifications only on a
secure origin, and the setting reads back what it answered. The status line
keeps the last report whatever is turned on, and focusing the session marks it
seen for every device.

### The Library

Six variables describe the context corpus the Library tab reads. The corpus is
a Markdown tree under git whose top-level directories are its shelves.

| Variable | What it names |
| --- | --- |
| `CHROTE_LIBRARY_ROOT` | The corpus directory. Unset: the tab reads "No library is configured" and nothing else. |
| `CHROTE_LIBRARY_AUTHOR` | The git identity the operator's edits are committed as, as `Name <email>`. Unset: an edit is refused with that reason. |
| `CHROTE_LIBRARIAN_SESSION` | The tmux session shown in the Library's resident column, the Librarian. Unset: the column says the Librarian is not configured. |
| `CHROTE_LIBRARY_BEADS` | The Beads project the Librarian works from, named as his store on `/api/residents`. Unset: he is named with no store. |
| `CHROTE_LIBRARY_SEARCH_URL` | The brain service whose ranked search the tab's search asks (`POST <url>/v1/library/search`, as Context Citadel serves it on loopback). Unset: a search answers that brain search is not configured. |
| `CHROTE_LIBRARY_LINK_ROOTS` | Where a page under the corpus's `projects/` shelf may point. That shelf holds each project's own README as a symlink under a Library path, never a copy; the Library lists, reads and maps it there, read-only, with the corpus's own history of the link. A symlink anywhere else, or to anything but a regular Markdown file outside the corpus, is not a page. Colon-separated directories. Unset: any Markdown file outside the corpus. |

```bash
CHROTE_LIBRARY_ROOT=/absolute/path/to/corpus
CHROTE_LIBRARY_AUTHOR="Example Operator <operator@example.invalid>"
CHROTE_LIBRARIAN_SESSION=librarian
CHROTE_LIBRARY_BEADS=/absolute/path/to/corpus
CHROTE_LIBRARY_SEARCH_URL=http://127.0.0.1:<port>
CHROTE_LIBRARY_LINK_ROOTS=/absolute/path/to/projects:/absolute/path/to/more
```

`GET /api/library/shelves` reports the root, the shelves and their page counts,
and the two names above; `pages`, `page`, `changes` and `graph` read the tree
and its git log; `search` asks the search service; `PUT /api/library/page` writes one page and commits it
as the
configured author, refusing a page that already carries somebody else's
uncommitted change. Every path is validated inside the root, and CHROTE writes
nowhere else in the corpus.

A root that is set but is not a directory is an operator mistake: the server
refuses to start and logs it, rather than serving a library that answers
nothing.

### The residents

Four tabs each host a resident agent as a tmux session in a column at the
tab's right edge: the Librarian in Brain (the Library), the tender in Agents,
the Clerk in Beads, and the service agent in Services. Each resident is a
session name, the folder its launcher starts in when the session is absent, and
the Beads project the resident works from. The Librarian takes its three from
the Library variables above, with the corpus root as its folder; the tender and
the Clerk have three variables each; the service agent is the `resident` object
of the Services configuration below.

| Variable | What it names |
| --- | --- |
| `CHROTE_TENDER_SESSION` | The tender's tmux session, shown in the Agents tab. Unset: the column says the tender is not configured. |
| `CHROTE_TENDER_FOLDER` | The folder the launcher offers when the tender's session is absent. |
| `CHROTE_TENDER_BEADS` | The Beads project the tender works from. |
| `CHROTE_CLERK_SESSION` | The Clerk's tmux session, shown in the Beads tab. Unset: the column says the Clerk is not configured. |
| `CHROTE_CLERK_FOLDER` | The folder the launcher offers when the Clerk's session is absent. |
| `CHROTE_CLERK_BEADS` | The Beads project the Clerk works from. |

```bash
CHROTE_TENDER_SESSION=tender
CHROTE_TENDER_FOLDER=/absolute/path/to/tender
CHROTE_TENDER_BEADS=/absolute/path/to/store
CHROTE_CLERK_SESSION=clerk
CHROTE_CLERK_FOLDER=/absolute/path/to/clerk
CHROTE_CLERK_BEADS=/absolute/path/to/store
```

`GET /api/residents` reports all four as
`[{ tab, label, session, folder, beads }]`, one entry per tab in tab order,
whether or not anything is configured for it; the dashboard reads the residents
from this route alone. Each resident's charter lives on the host outside this
repository, and the sessions are started from the column's Launch, not by the
server.

### Services

The Services tab forwards list, start and stop requests to a controller the
host runs on a Unix socket. CHROTE reads where that socket is from a JSON file:
`CHROTE_SERVICES_CONFIG` when set, otherwise `services.json` beside
`CHROTE_LAUNCH_CONFIG` when that is set, otherwise
`$XDG_CONFIG_HOME/chrote/services.json`. The file is read at startup; a missing
file leaves the tab explaining that no controller is configured, and an invalid
one is logged and treated the same way.

```json
{
  "socket": "/absolute/path/to/services.sock",
  "resident": {"session": "service-agent", "folder": "/absolute/path", "beads": "/absolute/path/to/store"}
}
```

The controller answers `GET /services` with `{"services": [...]}`, each entry
`{ id, name, description, link, state, controllable }`, and
`POST /services/{id}/start` or `/stop` with `{"service": {...}}`, or with
`error` or `errors` on failure. `state` is one of `running`, `stopped`,
`starting`, `stopping`, `partial`, `failed` or `unknown`.

## Upgrade

```bash
cd CHROTE
git pull --ff-only
npm ci --prefix dashboard
./install.sh   # repeat the --workspace and --port you installed with
```

The dashboard build installs dependencies only when `dashboard/node_modules` is
missing, so `npm ci` keeps an upgrade from building against packages the new
lockfile no longer names.

The installer rebuilds from the checked-out commit, atomically replaces the
managed binary, rewrites `chrote.env` from its options and defaults, preserves
`secrets.env` and durable state, then restarts and health-checks the user
service.

Review release notes and `git diff` before upgrading alpha builds.

## Uninstall

```bash
./uninstall.sh
```

It asks before removing anything; `-y` skips the question. An install made with
`--prefix` needs the same `--prefix` here.

The default uninstall removes:

- the managed CHROTE executable, and any ttyd and terminal launcher left by an
  install that predates the built-in terminal transport;
- `chrote.service`;
- managed `chrote.env`.

It preserves:

- the configured workspace;
- CHROTE durable state;
- `secrets.env`;
- the Beads tab's disposable cache under `$XDG_CACHE_HOME/chrote`, which can be
  deleted by hand.

Explicit cleanup is available:

```bash
./uninstall.sh --purge-state --purge-private-config
```

The workspace is never deleted.

## Source build without installation

```bash
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh
python3 scripts/check-embedded-dashboard.py

cd src
go test ./...
cd ..
./scripts/build-server.sh ./chrote-server
```

The server build script stamps the version from `VERSION` and the HEAD commit of
this checkout. Do not build a release from stale embedded assets:
`check-embedded-dashboard.py` must pass first.

A bare `./chrote-server` uses compiled defaults meant for an installed host. It
has no tmux socket mapping, so every terminal request fails with "no tmux
sockets are configured", and its state directories are a fixed system path.
Give it what the installer would:

```bash
state="$PWD/.dev-state"
tmux_tmp="${TMUX_TMPDIR:-/tmp}"
HOST=127.0.0.1 PORT=8090 \
CHROTE_ROOTS="$HOME/work" CHROTE_WORKDIR="$HOME/work" \
CHROTE_TMUX_SOCKET="$(id -un)=${tmux_tmp%/}/tmux-$(id -u)/default" \
CHROTE_SCHEDULED_TASKS_DIR="$state/scheduled-tasks" \
CHROTE_SESSION_DROPS_DIR="$state/session-drops" \
CHROTE_AGENT_HOOKS_DIR="$state/agent-hooks" \
./chrote-server
```

Port 8090 is the backend `npm run dev` in `dashboard/` proxies to, and stays
clear of an installed CHROTE on 8094. The socket path drops a trailing slash
from `TMUX_TMPDIR`, as the installer does. Unset, `CHROTE_ROOTS` defaults to
`$HOME,/code,/vault`.

## Remote access

CHROTE has no application login. Treat network access as shell access.

Keep the server on loopback and put remote access behind an operator-controlled
private network and HTTPS, such as Tailscale Serve. See
[`SECURITY.md`](../SECURITY.md).

A front door must pass WebSocket upgrades on `/terminal/` and keep the
browser's original `Host` header: the terminal WebSocket compares the `Origin`
with the host the request was addressed to, and answers `403` when a proxy has
rewritten it. Tailscale Serve keeps it. A proxy that cannot must have its
public origin listed in `CORS_ORIGINS`. Browser notifications for agent events
need a secure origin, which HTTPS provides.
