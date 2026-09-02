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
- tmux
- curl
- Git

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
| Server user unit | `$XDG_CONFIG_HOME/systemd/user/chrote.service` |

The installer:

1. builds the dashboard and exact embedded Go binary from the checkout;
2. injects the version from `VERSION`;
3. installs `chrote-server` under the user prefix;
4. writes XDG-scoped state paths for schedules and session drops;
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

Edit the managed non-secret values in:

```text
~/.config/chrote/chrote.env
```

Put optional tokens and private service URLs in:

```text
~/.config/chrote/secrets.env
```

Then reload:

```bash
systemctl --user restart chrote.service
```

Common advanced settings are documented in [`.env.example`](../.env.example).
Cross-user terminal sockets require deliberate host setup and are not enabled by
the generic installer. CHROTE may apply explicitly configured additive tmux
access grants, but must not remove or narrow the session owner's access.

## Environment

Two optional variables move presentation and launch choices out of the binary
and into operator configuration. Set them in `chrote.env` and restart the
service. Both are unset on a fresh install, and CHROTE runs without either.

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

A JSON file naming what a new session may start and where.

```json
{
  "harnesses": [
    {"id": "example-agent", "label": "Example Agent", "command": "example-agent --flag"},
    {"id": "shell", "label": "Shell", "command": ""}
  ],
  "folders": ["/absolute/path/to/project", "~"]
}
```

Ids match `[a-z0-9-]+` and are unique. `shell` is the bare login shell and must
have an empty command; it is offered whether or not the file lists it. A folder
is absolute, or starts with `~` and resolves against the target Unix user's
home. Commands never reach the browser: `GET /api/launch` returns ids, labels
and folders, and a session is created by naming a harness id.

Unset: the launcher offers `Shell` in `~`. Set but unreadable or invalid: the
server refuses to start and logs the reason, instead of running a launcher that
cannot launch.

## Upgrade

```bash
cd CHROTE
git pull --ff-only
./install.sh
```

The installer rebuilds from the checked-out commit, atomically replaces the
managed binary, rewrites managed non-secret configuration, preserves
`secrets.env` and durable state, then restarts and health-checks the user service.

Review release notes and `git diff` before upgrading alpha builds.

## Uninstall

```bash
./uninstall.sh
```

The default uninstall removes:

- the managed CHROTE executable, and any ttyd and terminal launcher left by an
  install that predates the built-in terminal transport;
- `chrote.service`;
- managed `chrote.env`.

It preserves:

- the configured workspace;
- CHROTE durable state;
- `secrets.env`.

Explicit cleanup is available:

```bash
./uninstall.sh --purge-state --purge-private-config
```

The workspace is never deleted.

## Source build without installation

```bash
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh

cd src
go test ./...
version="$(tr -d '\r\n' < ../VERSION)"
go build \
  -trimpath \
  -ldflags "-X main.Version=$version" \
  -o ../chrote-server \
  ./cmd/server

cd ..
./chrote-server
```

Do not build a release from stale embedded assets. The canonical embed script and
`diff -qr dashboard/dist src/internal/dashboard/dist` must agree first.

## Remote access

CHROTE has no application login. Treat network access as shell access.

Keep the server on loopback and put remote access behind an operator-controlled
private network and HTTPS, such as Tailscale Serve. See
[`SECURITY.md`](../SECURITY.md).
