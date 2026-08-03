# Troubleshooting CHROTE

> **Scope: the from-scratch install this repo's `install.sh` creates** — the
> `chrote.service` user unit with the compiled default ports `8094` (dashboard)
> and `7683` (ttyd). An already-operated deployment may run under a different
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

CHROTE starts and proxies one loopback ttyd child. Check the service log for:

- `Started ttyd on port ...`
- `failed to start ttyd`
- `Terminal proxy error`
- `tmux session ... is not available`

Verify the installed pieces:

```bash
command -v tmux
~/.local/bin/ttyd --version
test -x ~/.local/lib/chrote/terminal-launch.sh
```

Then verify the same user's tmux server:

```bash
tmux list-sessions
curl http://127.0.0.1:8094/api/tmux/sessions
```

A fresh install uses the installing user's normal tmux server. If you configured
`CHROTE_DEFAULT_TMUX_SOCKET` or cross-user socket mappings, verify the exact
socket path and filesystem permissions. CHROTE intentionally fails loud instead
of falling back to a different user's ambient server.

Do **not** kill a healthy tmux server merely because the browser terminal is
broken. tmux owns the durable work; ttyd and CHROTE are replaceable clients.

## 3. Port already in use

```bash
ss -ltnp | grep -E ':(8094|7683)\b'
```

Choose unused loopback ports and reinstall:

```bash
./install.sh --port 8096 --ttyd-port 7685
```

The dashboard and ttyd ports must differ.

## 4. Files view is empty or denied

Inspect the configured roots:

```bash
grep -E '^CHROTE_(ROOTS|WRITE_ROOTS|WORKDIR)=' ~/.config/chrote/chrote.env
```

Rules:

- requested paths must resolve under `CHROTE_ROOTS`;
- mutations must also remain under `CHROTE_WRITE_ROOTS`;
- symlink resolution must not escape those roots;
- the CHROTE Unix user still needs ordinary filesystem permission.

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

## 7. Formations exists but cannot execute

That is often correct. Formations authoring and run inspection are always
available; execution promotion is separately gated.

1. Start with the deterministic lab executor.
2. Use an isolated temp tmux socket for executor dogfood.
3. Promote to a live socket only with explicit configuration and an approved
   operator boundary.

Check `FORMATIONS.md` and the relevant ledger events. Do not enable script gates
or live tmux execution merely to make a button look green.

## 8. Recovery entries are limited or blocked

Session Bank distinguishes supported typed recovery from unmanaged or unsafe
entries.

- `Recover workload` appears only when CHROTE has a valid typed descriptor.
- Managed external supervisors remain read-only unless their contract permits a
  restart action.
- Arbitrary shell state cannot be reconstructed safely.
- Recovery failures should appear in the API response, supervisor status, or
  durable recovery evidence.

Use explicit Refit/reconnect actions before deleting stale sessions. Bulk
session destruction is an advanced emergency action in Settings.

## 9. A locked agent is failed, degraded, or not coming back

A locked session is supervised by its own systemd user unit, not by CHROTE, so
the unit's journal is the diagnostic record — not a status string in CHROTE.
Read it as the account that owns the agent:

```bash
systemctl --user status chrote-agent@<session>.service
journalctl --user -u chrote-agent@<session>.service -n 100
```

Read the reported state literally:

- `failed` or `inactive` come straight from systemd; the journal holds the exit
  status and the launcher's own refusal messages.
- `degraded` means the unit is active but the launcher receipt does not match the
  session CHROTE expected. The unit is running something; treat it as the wrong
  something until the journal says otherwise.
- A launcher that refuses at startup naming a tmux socket is working as
  designed: it never creates a tmux server, so a dead socket is the keeper
  unit's problem, not the agent's.

Locking needs the owning account's systemd user manager to be running, which for
a headless account means lingering. A lock that fails immediately with no unit
in the journal usually means there was no user manager to accept it.

Unlocking disables the unit and leaves the agent and its tmux session running.
If work is still alive after an unlock, that is the contract, not a bug.

## 10. Scheduled task is stuck

Inspect the Scheduled view and service logs for lock age, last run, and failure
history. CHROTE may reclaim a stale lock according to its scheduling contract;
it must not silently double-run a task with a fresh lock.

Do not delete lock/state files until you understand whether another process is
still running.

## 11. Browser state looks stale

First:

1. hard refresh;
2. verify `/api/health` reports the expected build version;
3. check whether a service worker or reverse proxy is caching old assets;
4. compare the loaded asset names with the current embedded build.

Only then consider clearing CHROTE local storage. Local storage owns presentation
preferences and workspace assignments; clearing it should not kill tmux sessions
or delete host files, but it will reset layout state.

## 12. Reinstall without losing work

```bash
cd CHROTE
./install.sh
```

Reinstall preserves the configured workspace, XDG state, and `secrets.env`.
Uninstall also preserves them by default:

```bash
./uninstall.sh
```

Never use `--purge-state` unless you deliberately want to remove Session Bank,
schedules, per-agent lock configuration and launcher receipts, and other
CHROTE-owned state. Purging the configuration of a locked agent does not by
itself stop its systemd unit; disable the unit first if you want supervision to
end, and expect the agent itself to keep running until you stop it.
