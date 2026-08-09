# Installing CHROTE

> **Scope: a fresh, from-scratch install.** Everything below — the
> `chrote.service` user unit, port `8094`, ttyd port `7683` — describes what
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

The installer does not use `sudo`, create a dedicated Unix user, or start a
second ttyd service. The Go server owns one loopback ttyd child and connects to
the installing user's normal tmux server.

## Requirements

- Linux or WSL with user systemd available
- Go 1.26.5+
- Node.js 20.19+ or 22.12+
- npm
- tmux
- curl
- Git

`ttyd` 1.7.7 is copied from the current `PATH` or downloaded into the user
prefix when absent.

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
| ttyd child | `127.0.0.1:7683` |
| Workspace/file root | `$HOME` |
| Binary prefix | `$HOME/.local` |
| Managed config | `$XDG_CONFIG_HOME/chrote/chrote.env` or `$HOME/.config/chrote/chrote.env` |
| Private overrides | `$XDG_CONFIG_HOME/chrote/secrets.env` |
| Durable state | `$XDG_STATE_HOME/chrote` or `$HOME/.local/state/chrote` |
| Server user unit | `$XDG_CONFIG_HOME/systemd/user/chrote.service` |

The installer:

1. builds the dashboard and exact embedded Go binary from the checkout;
2. injects the version from `VERSION`;
3. installs `chrote-server`, ttyd, and `terminal-launch.sh` under the user prefix;
4. writes XDG-scoped state paths for Session Bank, schedules, recovery status,
   and agent cards;
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
- `CHROTE_WRITE_ROOTS`
- `CHROTE_WORKDIR`
- the default tmux working directory
- the initial Beads discovery root
- the Formations workspace

Configured roots constrain CHROTE file APIs. They do not sandbox commands or AI
agents running in tmux.

## Custom ports or prefix

```bash
./install.sh \
  --workspace "$HOME/work" \
  --port 8094 \
  --ttyd-port 7683 \
  --prefix "$HOME/.local"
```

The dashboard and ttyd ports must differ. Both remain loopback-only.

## Verify

```bash
systemctl --user status chrote.service
curl http://127.0.0.1:8094/api/health
```

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
Cross-user terminal sockets, Formations tmux execution, and script gates require
deliberate host setup. They are not enabled by the generic installer. CHROTE may
apply explicitly configured additive tmux access grants, but must not remove or
narrow the session owner's access.

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

- managed CHROTE and ttyd executables;
- the managed terminal launcher;
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
