---
title: Troubleshooting
nav: Troubleshooting
summary: Start at the boundary that is failing. Do not kill tmux, reboot WSL or wipe browser state as a first move.
sources: docs/troubleshooting.md, docs/installation.md
---

# Troubleshooting

CHROTE is a replaceable client in front of tmux. A broken dashboard is almost
never a reason to touch the sessions behind it. Work from the outside in, and
stop at the first boundary that fails.

> [!NOTE]
> These commands assume the default from-scratch install: the
> `chrote.service` user unit on port `8094`. If your host runs CHROTE under
> another unit or port, use those.

## Find the failing boundary

| Symptom | First check |
| --- | --- |
| Nothing loads | `systemctl --user status chrote.service`, then `journalctl --user -u chrote.service -n 100 --no-pager` |
| The service will not start | The last lines of the journal name the setting: launch configuration, Library root, tmux socket mapping, or port |
| Dashboard loads, terminals fail | `tmux list-sessions` and `curl http://127.0.0.1:8094/api/tmux/sessions` |
| Terminals fail only remotely (`403`) | The proxy rewrites `Host`; see [Remote access](remote-access.md#with-another-reverse-proxy) |
| Port in use | `ss -ltnp \| grep -E ':8094\b'` |
| Files empty or denied | `grep -E '^CHROTE_(ROOTS\|WORKDIR)=' ~/.config/chrote/*.env` |
| Beads unavailable | `command -v bd` and `curl http://127.0.0.1:8094/api/beads/health` |
| Brain empty or refuses edits | `CHROTE_LIBRARY_ROOT`, `CHROTE_LIBRARY_AUTHOR`, and whether the service can run `git` in the corpus |
| Theme wrong, launcher offers only Shell | `curl -i http://127.0.0.1:8094/api/theme` and `curl http://127.0.0.1:8094/api/launch` |
| A setting "did not stick" after a re-install | It was in `chrote.env`. Move it to `secrets.env`; see [Configuration](configuration.md#which-file-to-edit) |
| Dashboard gone after logout | Lingering is off; see [Install](install.md#keep-it-running-after-you-log-out) |
| Old interface after an upgrade | Hard refresh, then compare `/api/health` with `git rev-parse HEAD` |

## Terminals

CHROTE runs the tmux attach itself, on a pseudo-terminal it allocates. In the
journal, look for:

- `terminal attach refused: …`
- `tmux session … is not available on configured socket …`
- `terminal WebSocket upgrade failed`

A fresh install maps your normal tmux socket explicitly in
`CHROTE_TMUX_SOCKET`. If you start tmux with a different socket (`-L`, `-S` or
`TMUX_TMPDIR`), CHROTE will not see those sessions until that socket is mapped.
CHROTE never falls back to another tmux server.

## Changing port or workspace

```bash
./install.sh --port 8096   # add --workspace PATH if you installed with one
```

The installer does not remember the previous run: a `--port` alone resets the
workspace to `$HOME`, and a `--workspace` alone resets the port to 8094. Pass
the workspace you installed with, not a new one, unless the point is to change
it. Do not widen roots to `/` to debug a denied path; a denied path
is a Unix permission, reported as one.

## Browser state

Hard refresh first, and check `/api/health` reports the build you expect. Only
then consider clearing CHROTE's local storage: it holds layouts, labels and
preferences, so clearing it resets your arrangement and touches no session or
file.

## Scheduled tasks

Read the task's run history in the Scheduled tab and the journal. Do not delete
lock or state files while another process might be running the task.

## Reinstall without losing work

`./install.sh` with your original options keeps the workspace, the state
directory and `secrets.env`. `./uninstall.sh` keeps them too unless you pass
`--purge-state` or `--purge-private-config`.

The complete, boundary-by-boundary guide is the canonical
[troubleshooting reference](../../troubleshooting.md).
