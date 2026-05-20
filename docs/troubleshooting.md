# Troubleshooting

## Service

```bash
systemctl --user status chrote.service
journalctl --user -u chrote.service -n 100 --no-pager
journalctl --user -u chrote.service -f
```

Restart:

```bash
systemctl --user restart chrote.service
```

## Health

```bash
curl http://127.0.0.1:8094/api/health
curl http://127.0.0.1:8094/api/tmux/sessions
curl http://127.0.0.1:8094/api/oracle/status
curl http://127.0.0.1:8094/api/beads/health
```

## Tailscale

```bash
tailscale serve status
```

Expected route shape:

```text
https://<tailnet-host>:8445/
|-- / proxy http://127.0.0.1:8094
```

## Terminal

CHROTE uses a dedicated tmux socket:

```bash
TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux ls
```

If the dashboard has no sessions:

```bash
TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux new-session -d -s main -c "$HOME"
```

The ttyd listener should be loopback-only:

```bash
ss -ltnp | grep 7683
```

## Beads

```bash
cd <workspace-root>
bd --json list | head
curl 'http://127.0.0.1:8094/api/beads/issues?path=<workspace-root>'
```

If Beads fails in CHROTE but works in a shell, check `CHROTE_BD_COMMAND` in `chrote.service`.
If a workspace is missing from the Beads selector, check `CHROTE_BEADS_WORKSPACES`.
Those paths are separate from `CHROTE_ROOTS`; adding `/srv` there does not widen Files view access.

## Frontend Changes

Production CHROTE serves embedded dashboard assets. After frontend edits:

```bash
cd /path/to/chrote/dashboard
npm run build
cd /path/to/chrote
rm -rf src/internal/dashboard/dist
cp -r dashboard/dist src/internal/dashboard/dist
cd src
go build -o ../chrote-server ./cmd/server
systemctl --user restart chrote.service
```
