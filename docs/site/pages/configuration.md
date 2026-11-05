---
title: Configuration
nav: Configuration
summary: Where CHROTE's settings live, which file to edit so a re-install keeps them, and how a change takes effect.
sources: docs/installation.md, install.sh, .env.example, ARCHITECTURE.md
---

# Configuration

CHROTE is configured by environment variables read when the server starts, and
by a few JSON files those variables point at. The browser holds only
presentation; nothing you set in the dashboard changes host configuration.

## The two environment files

An installed CHROTE runs as the `chrote.service` user unit, which loads two
files in order:

| File | Written by | Purpose |
| --- | --- | --- |
| `~/.config/chrote/chrote.env` | The installer, **every run** | Managed settings: `HOST`, `PORT`, `CHROTE_ROOTS`, `CHROTE_WORKDIR`, `CHROTE_TMUX_SOCKET`, `CHROTE_BEADS_WORKSPACES`, the state directories and `PATH` |
| `~/.config/chrote/secrets.env` | You. The installer creates it empty once and never touches it again | Every setting of your own, private or not |

systemd applies them in that order, so **a variable in `secrets.env` overrides
the same variable in `chrote.env`**. Both are `KEY=value` lines, mode `0600`.

## Which file to edit

Put your settings in `secrets.env`. That includes optional features such as the
launcher, the theme, the Library and residents, and Services, and any managed
value you want to change, such as `CHROTE_ROOTS`.

> [!WARNING]
> Edits to `chrote.env` are lost on the next `./install.sh`, which rewrites the
> file from its own options and defaults, `--workspace` and `--port` included.
> When you re-run the installer, repeat the options you installed with.

## Apply a change

```bash
$EDITOR ~/.config/chrote/secrets.env
systemctl --user restart chrote.service
journalctl --user -u chrote.service -n 50 --no-pager
```

Restarting CHROTE leaves every tmux session running; open terminals reconnect.
Every variable is read at startup, so changing one, `CHROTE_THEME_DIR`
included, needs the restart above. Some files are read only then too (the
launch configuration, `services.json`). Edits inside the theme directory need
no restart: the theme is read when the dashboard loads, so reload the page.

## What refuses to start, and what degrades

CHROTE fails loudly on a setting it cannot honour, and degrades quietly on a
feature you have not set up.

| Situation | Result |
| --- | --- |
| `CHROTE_LAUNCH_CONFIG` set but unreadable or invalid | The server refuses to start and logs why |
| `PORT` not a number from 1 to 65535 | The server refuses to start and logs why |
| `CHROTE_LIBRARY_ROOT` set but missing or not a directory | The server refuses to start and logs why |
| `CHROTE_LIBRARY_AUTHOR` set but not `Name <email>` | The server refuses to start and logs why |
| `CHROTE_LIBRARY_SEARCH_URL` set but not an `http(s)` URL with a host | The server refuses to start and logs why |
| `CHROTE_TMUX_SOCKET` entry not `user=/absolute/canonical/path` | The server refuses to start and logs why |
| A Unix user listed twice in `CHROTE_TMUX_SOCKET` or `CHROTE_TERMINAL_USER_WORKDIRS` | The server refuses to start and logs why |
| `theme.json` exists but does not validate | `/api/theme` answers `500` naming the field; the dashboard does not pretend |
| `bd` missing, no corpus, no services controller | That tab says so; terminals are unaffected |
| `services.json` invalid | Logged at startup; Services behaves as unconfigured |

## The JSON files

| File | Variable | Page |
| --- | --- | --- |
| Launch configuration | `CHROTE_LAUNCH_CONFIG` | [The launcher](launcher.md) |
| `theme.json` and its `art/` | `CHROTE_THEME_DIR` | [Theme](theme.md) |
| `services.json` | `CHROTE_SERVICES_CONFIG` | [Services](services.md#configure-it) |

Keep them in a directory of your own operator configuration, not in the CHROTE
checkout.

## All variables

Every variable, its default and its format is in the
[environment reference](environment.md). The repository's
[`.env.example`](../../../.env.example) lists the same names with comments.
