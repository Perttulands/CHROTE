# Troubleshooting

## Service

```bash
# /srv proving lane: /srv/chrote, /srv/data/chrote, 8095/7686
systemctl status chrote-srv.service --no-pager
journalctl -u chrote-srv.service -n 100 --no-pager
journalctl -u chrote-srv.service -f

# legacy rollback lane: /home/perttu/chrote, 8094/7683
systemctl --user status chrote.service
journalctl --user -u chrote.service -n 100 --no-pager
journalctl --user -u chrote.service -f
```

Restart:

```bash
systemctl restart chrote-srv.service        # /srv proving lane
systemctl --user restart chrote.service     # legacy rollback lane
```

## Health

```bash
CHROTE_URL=http://127.0.0.1:8095      # /srv proving lane
# CHROTE_URL=http://127.0.0.1:8094    # legacy rollback lane

curl "$CHROTE_URL/api/health"
curl "$CHROTE_URL/api/tmux/sessions"
curl "$CHROTE_URL/api/oracle/status"
curl "$CHROTE_URL/api/beads/health"
```

## Tailscale

```bash
tailscale serve status
```

Expected route shape:

```text
https://<tailnet-host>:8445/
|-- / proxy http://127.0.0.1:8095
```

## Terminal

The `/srv` proving lane reads per-user tmux socket mappings from private
runtime config. The Perttu legacy rollback lane uses this interactive socket:

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
ss -ltnp | grep 7686
```

## Beads

```bash
cd <workspace-root>
bd --json list | head
curl "$CHROTE_URL/api/beads/issues?path=<workspace-root>"
```

If Beads fails in CHROTE but works in a shell, check `CHROTE_BD_COMMAND` in
`chrote-srv.service` for the `/srv` proving lane or in `chrote.service` for the
legacy rollback lane.
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
systemctl restart chrote-srv.service
```
