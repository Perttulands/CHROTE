---
title: Install CHROTE
nav: Install
summary: Build CHROTE from a checkout and run it as a user service on Linux or WSL. The installer never calls sudo.
sources: docs/installation.md, install.sh, README.md
---

# Install CHROTE

The supported installation is a **same-user Linux or WSL user service built from
the checked-out source**. The installed binary is tied to a commit you can
inspect, CHROTE runs as your own Unix user, and it sees your normal tmux server.
The installer does not use `sudo` and does not create a Unix user.

## Requirements

| Need | Why |
| --- | --- |
| Linux, or WSL with systemd enabled | CHROTE runs as a `systemd --user` service |
| tmux | CHROTE attaches to tmux sessions; it does not replace tmux |
| `setfacl`, from your distribution's `acl` package | Send to Session uses POSIX ACLs so only the target user can read what is handed over; the installer stops without it |
| Go 1.26.6 or newer | Builds the server |
| Node.js 20.19+ or 22.12+, and npm | Builds the dashboard |
| Python 3 | The dashboard build stamps the bundle it embeds |
| Git and curl | Version stamping and the post-install health check |

Go, Node.js, npm and Python are needed only to build. An install from an
already-built binary (`--binary PATH`) skips them.

> [!NOTE]
> **On WSL**, systemd must be on. Add `systemd=true` under `[boot]` in
> `/etc/wsl.conf`, then restart the distribution from Windows with
> `wsl.exe --shutdown`. Restarting WSL ends every tmux session inside it, so do
> this before you have work running.

## Install

```bash
git clone https://github.com/Perttulands/CHROTE.git
cd CHROTE
./install.sh
```

Then open <http://127.0.0.1:8094>.

The installer:

1. builds the dashboard and the Go binary with the dashboard embedded, from
   this checkout;
2. stamps the version from `VERSION` and the checkout's HEAD commit;
3. installs `chrote-server` and the `chrote-agent-event` hook script side by
   side under `$HOME/.local/bin`;
4. creates state directories for schedules, Send to Session drops and generated
   agent hooks;
5. writes `chrote.env` and an empty `secrets.env` if none exists;
6. writes the `chrote.service` user unit, enables it, starts it, and checks
   `/api/health` until it reports the version it just built.

No workspace files are copied anywhere. CHROTE does not install per-session
supervision and does not promise to recreate sessions after a reboot.

## Choose the workspace and port

| Option | Default | What it sets |
| --- | --- | --- |
| `--workspace PATH` | `$HOME` | The file root (`CHROTE_ROOTS`), the default directory for new sessions (`CHROTE_WORKDIR`), the unit's working directory, and where Beads stores are discovered |
| `--port PORT` | `8094` | The loopback port the dashboard listens on |
| `--prefix PATH` | `$HOME/.local` | Where `bin/chrote-server` and `bin/chrote-agent-event` go |
| `--binary PATH` | build from source | Install an already-built `chrote-server` instead |
| `--no-enable` | enable | Do not enable the unit at login |
| `--no-start` | start | Do not start or health-check the unit |
| `--no-systemd` | manage systemd | Write the unit and layout, but do not touch systemd |

Giving CHROTE all of `$HOME` is convenient and broad. A dedicated workspace is
narrower:

```bash
./install.sh --workspace "$HOME/work"
```

The file root bounds CHROTE's own file API. It does not sandbox anything an
agent runs in tmux; see [Security](security.md).

> [!WARNING]
> **The installer does not remember earlier choices.** Each run rewrites
> `chrote.env` from its own options and defaults. Pass the same `--workspace`
> and `--port` every time you re-run it, and keep any setting of your own in
> `secrets.env`, which the installer never overwrites and the unit loads last.
> See [Configuration](configuration.md#which-file-to-edit).

## What lands where

| Path | What it is | Rewritten by a re-install? |
| --- | --- | --- |
| `~/.local/bin/chrote-server` | The server, dashboard embedded | Yes, atomically |
| `~/.local/bin/chrote-agent-event` | The agent completion hook | Yes |
| `~/.config/chrote/chrote.env` | Managed settings: bind, roots, tmux socket, state paths, `PATH` | **Yes, completely** |
| `~/.config/chrote/secrets.env` | Your settings and secrets, loaded after `chrote.env` | Never |
| `~/.config/systemd/user/chrote.service` | The user unit | Yes |
| `~/.local/state/chrote/` | Schedules, Send to Session drops, generated agent hooks | Never |
| `~/.cache/chrote/` | The Beads tab's last read of each store; safe to delete | Never |

`XDG_CONFIG_HOME`, `XDG_STATE_HOME` and `XDG_CACHE_HOME` move these when set.

## Verify

```bash
systemctl --user status chrote.service
curl http://127.0.0.1:8094/api/health
```

The health answer names the build:

```json
{"status":"ok","version":"2.0.0-alpha.2-dev","commit":"<full commit hash>","timestamp":"…"}
```

`GET /api/theme` and `GET /api/launch` answer with built-in defaults on a fresh
install. Open the dashboard and continue with [your first session](first-session.md).

## Keep it running after you log out

A `systemd --user` service normally stops when your last login session ends,
which would take the dashboard away while you are on your phone. Lingering keeps
your user manager, and with it CHROTE, running:

```bash
loginctl enable-linger "$USER"
loginctl show-user "$USER" --property=Linger
```

Your distribution may require administrator rights to enable lingering. tmux
sessions started outside the service follow your system's own logout policy;
CHROTE does not change it.

## When the install fails

- **`setfacl is required`**: install your distribution's `acl` package.
- **`CHROTE_TMUX_SOCKET grants require a root install`**: your shell has
  `CHROTE_TMUX_SOCKET` set, and the installer treats that as a request for
  cross-user socket grants, which only a root install performs. For an ordinary
  install, run `env -u CHROTE_TMUX_SOCKET ./install.sh`. For several users, see
  [Several Unix users](multi-user.md).
- **`health endpoint responded, but not as CHROTE …`**: another process owns the
  port. Choose another with `--port`.
- **`service started but health check failed`**: read
  `journalctl --user -u chrote.service`. A setting CHROTE cannot use, such as an
  invalid launch configuration, a Library root that is not a directory, or a
  `CHROTE_LIBRARY_AUTHOR` not in `Name <email>` form, stops startup and is
  named there. [Configuration](configuration.md#what-refuses-to-start-and-what-degrades)
  lists them all.

More in [Troubleshooting](troubleshooting.md).

## Without systemd

For packaging, or to inspect the layout first:

```bash
./install.sh --no-systemd --no-start
```

That writes the binary, configuration and unit without reloading, enabling or
starting anything. To run a server straight from a source build without
installing, see [Develop](develop.md#run-a-server-from-a-source-build).
