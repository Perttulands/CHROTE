# Troubleshooting CHROTE

> **Scope: the from-scratch install this repo's `install.sh` creates** — the
> `chrote.service` user unit with the compiled default port `8094`. An
> already-operated deployment may run under a different
> unit and port known only to local operator configuration; health-checking or
> restarting the defaults on such a host inspects the wrong process and can
> touch a retired service. Resolve the real unit and port from local operator
> context before running anything below, and ask instead of guessing.

Start at the boundary that is actually failing. Do not reboot WSL, kill tmux, or
wipe browser state as a first move.

## 1. Is the user service healthy?

```bash
systemctl --user status chrote.service
journalctl --user -u chrote.service -n 100 --no-pager
curl -v http://127.0.0.1:8094/api/health
```

Expected health shape:

```json
{"status":"ok","timestamp":"...","version":"2.0.0-alpha.2-dev"}
```

A tagged release reports its tag-derived version instead of the development
suffix.

If the port was customized, read `PORT` from
`~/.config/chrote/chrote.env` and use that value.

## 2. Dashboard loads, but terminals fail

CHROTE serves the terminal itself, running the tmux attach on a pseudo-terminal
it allocates. Check the service log for:

- `terminal attach refused: ...`
- `tmux session ... is not available on configured socket ...`
- `terminal WebSocket upgrade failed`

Verify the installed pieces:

```bash
command -v tmux
test -x ~/.local/bin/chrote-server
```

Then verify the same user's tmux server:

```bash
tmux list-sessions
curl http://127.0.0.1:8094/api/tmux/sessions
```

A fresh install writes the installing user's normal tmux socket as an explicit
`CHROTE_TMUX_SOCKET=unixUser=/absolute/socket` mapping. For cross-user mappings,
verify each exact socket path and its filesystem permissions. CHROTE intentionally
fails loud instead of discovering or falling back to another ambient server.

Do **not** kill a healthy tmux server merely because the browser terminal is
broken. tmux owns the session lifetime; CHROTE is a replaceable client.

## 3. Port already in use

```bash
ss -ltnp | grep -E ':8094\b'
```

Choose unused loopback ports and reinstall:

```bash
./install.sh --port 8096
```

## 4. Files view is empty or denied

Inspect the configured roots:

```bash
grep -E '^CHROTE_(ROOTS|WORKDIR)=' ~/.config/chrote/chrote.env
```

Rules:

- requested paths must resolve under `CHROTE_ROOTS`;
- reads and mutations must remain under those roots after symlink resolution;
- the CHROTE Unix user still needs ordinary filesystem permission; a denied
  path is reported as a permission error, not an empty root.

Re-run the installer with the intended workspace instead of widening roots to
`/` as a debugging shortcut:

```bash
./install.sh --workspace "$HOME/work"
```

## 5. Beads is unavailable

```bash
command -v bd
bd version
curl http://127.0.0.1:8094/api/beads/health
```

If `bd` is outside the service `PATH`, set an absolute `CHROTE_BD_COMMAND` in
`chrote.env`. Configure `CHROTE_BEADS_WORKSPACES` to actual Beads roots.

`bv` is optional. Its absence must not break Beads API or dashboard operation.

## 6. Services show degraded

Services cards are optional adapters. No upstream service, URL, or credential is
bundled with CHROTE, and a degraded card does not mean the core is unhealthy.

- Put private URLs/tokens in `~/.config/chrote/secrets.env`.
- Restart `chrote.service` after changing server-side environment.
- Check the upstream directly from the host without printing tokens.
- Never paste bearer values into browser storage, screenshots, issues, or logs.

## 7. Scheduled task is stuck

Inspect the Scheduled view and service logs for lock age, last run, and failure
history. CHROTE may reclaim a stale lock according to its scheduling contract;
it must not silently double-run a task with a fresh lock.

Do not delete lock/state files until you understand whether another process is
still running.

## 8. Browser state looks stale

First:

1. hard refresh;
2. verify `/api/health` reports the expected build version;
3. check whether a service worker or reverse proxy is caching old assets;
4. compare the loaded asset names with the current embedded build.

Only then consider clearing CHROTE local storage. Local storage owns presentation
preferences and workspace assignments; clearing it should not kill tmux sessions
or delete host files, but it will reset layout state.

## 9. The theme looks wrong, or the launcher offers only Shell

Both come from host configuration described under
[Environment](installation.md#environment).

```bash
grep -E '^CHROTE_(THEME_DIR|LAUNCH_CONFIG)=' ~/.config/chrote/chrote.env
curl -i http://127.0.0.1:8094/api/theme
curl http://127.0.0.1:8094/api/launch
```

- `/api/theme` answers `500` with the offending field named: a `theme.json`
  exists but is not schema 1. Fix the field the response names. CHROTE does not
  fall back to the default here, because a silent fallback would hide a broken
  edit behind a palette that looks deliberate.
- `/api/theme` answers with `chrote-dark` and an empty `art` list: that is the
  embedded default, so `CHROTE_THEME_DIR` is unset or holds no `theme.json`.
- The launcher offers only `Shell`: `CHROTE_LAUNCH_CONFIG` is unset. If it is
  set and the service will not start at all, the startup log names the parse or
  validation error in that file.
- The look did not change after an apply: the dashboard reads the theme once, at
  load. Reload the page.

## 10. Reinstall without losing work

```bash
cd CHROTE
./install.sh
```

Reinstall preserves the configured workspace, XDG state, and `secrets.env`.
Uninstall also preserves them by default:

```bash
./uninstall.sh
```

Never use `--purge-state` unless you deliberately want to remove schedules and
other CHROTE-owned state. Purging CHROTE state does not stop externally managed
workloads.
