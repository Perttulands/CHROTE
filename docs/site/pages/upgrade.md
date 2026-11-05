---
title: Upgrade and uninstall
nav: Upgrade and uninstall
summary: Move to a newer commit without losing sessions or settings, go back to an older one, or remove CHROTE.
sources: docs/installation.md, install.sh, uninstall.sh, CHANGELOG.md
---

# Upgrade and uninstall

## Upgrade

CHROTE v2 is alpha: read [the changelog](../../../CHANGELOG.md) and `git log`
before you move.

```bash
cd CHROTE
git pull --ff-only
npm ci --prefix dashboard
./install.sh   # add the --workspace and --port you installed with, if any
```

- `npm ci` brings the dashboard's dependencies in line with the new lockfile.
  The build only installs them when `dashboard/node_modules` is missing, so
  without this step an upgrade can build against stale packages.
- Repeat your original `--workspace` and `--port`, and only those. The
  installer rewrites `chrote.env` from its options and defaults; it does not
  read the old one. A `--workspace` you did not install with is not harmless:
  the installer creates that directory and narrows the file root, the working
  directory and the Beads workspaces to it.
- The installer rebuilds from the checked-out commit, replaces the binary
  atomically, keeps `secrets.env` and the state directory, restarts the service
  and checks that `/api/health` reports the new version.

Restarting the service leaves tmux sessions running. Open dashboards reconnect;
reload them to load the new interface.

Confirm what is running:

```bash
curl http://127.0.0.1:8094/api/health
git rev-parse HEAD
```

The `commit` in the health answer should match.

## Go back

Check out the commit you want and install it the same way:

```bash
git checkout <previous-commit>
npm ci --prefix dashboard
./install.sh   # with the same options as above
```

State formats can change between alpha builds; check the changelog between the
two commits.

## Uninstall

```bash
./uninstall.sh
```

It asks for confirmation; `-y` skips the question. If you installed with
`--prefix`, pass the same `--prefix` again.

| Removed | Kept |
| --- | --- |
| `chrote-server` and `chrote-agent-event` | Your workspace, always |
| `chrote.service`, after stopping and disabling it | `secrets.env` |
| `chrote.env` | The state directory: schedules, drops, agent hooks |
| Leftovers from installs that predate the built-in terminal transport | The Beads cache in `~/.cache/chrote` |
| | Every tmux session |

To remove more, ask for it:

```bash
./uninstall.sh --purge-state --purge-private-config
rm -rf "${XDG_CACHE_HOME:-$HOME/.cache}/chrote"
```

`--purge-state` deletes schedules and other CHROTE-owned state; it does not stop
anything running outside CHROTE. `--no-systemd` leaves the service manager alone.
If you enabled lingering only for CHROTE, `loginctl disable-linger "$USER"`
turns it off.
