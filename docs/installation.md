# Installation

This guide documents the current `/srv` CHROTE proving service. Use the legacy
user service only when explicitly doing rollback work.

## Runtime

Active `/srv` lane:

```bash
systemctl status chrote-srv.service --no-pager
curl http://127.0.0.1:8095/api/health
```

Service shape:

```text
source: /srv/chrote
data: /srv/data/chrote
unit: chrote-srv.service
HTTP: 127.0.0.1:8095
ttyd: 127.0.0.1:7686
```

Legacy rollback lane, not the active install target:

```text
source: /home/perttu/chrote
unit: chrote.service
HTTP: 127.0.0.1:8094
ttyd: 127.0.0.1:7683
```

## Rebuild

Build from the intended `/srv` checkout and restart only the `/srv` lane when
deployment is explicitly approved.

```bash
cd /srv/chrote/dashboard
npm ci
npm run build

cd /srv/chrote
rm -rf src/internal/dashboard/dist
cp -r dashboard/dist src/internal/dashboard/dist

cd /srv/chrote/src
go test ./...
go build -o ../chrote-server ./cmd/server
systemctl restart chrote-srv.service
```

## Service Configuration

Private runtime values belong in a service env file, not in git:

```text
/srv/chrote/config/chrote.env
/etc/chrote/chrote-srv.env
```

Typical `/srv` values:

```text
HOST=127.0.0.1
PORT=8095
TTYD_PORT=7686
CHROTE_WORKDIR=<workspace-root>
CHROTE_ROOTS=<workspace-root>
CHROTE_FORMATIONS_DATA_ROOT=/srv/data/chrote/formations
CHROTE_BEADS_WORKSPACES=<workspace-root>
CHROTE_BD_COMMAND=bd
CHROTE_SESSION_BANK_PATH=/srv/data/chrote/session-bank/sessions.json
CHROTE_PERSISTENT_AGENTS_PATH=/srv/data/chrote/persistent-agents/agents.json
CHROTE_SESSION_DROPS_DIR=/srv/data/chrote/session-drops
CHROTE_MANAGED_RECOVERY_STATUS_PATH=/srv/data/chrote/tmux-recovery/managed-status.json
```

Keep `CHROTE_FORMATIONS_DATA_ROOT` outside the workspace and generic Files
roots. The server treats it as host-private authority state and denies its
configured and canonical paths through the Files API.

Do not print or commit secrets from service env files.

## Managed Recovery Status

Externally managed workloads are read-only in CHROTE. They stay out of Session
Bank and Persistent desired-state stores, and CHROTE must not restart them.

The restore-side owner can publish a strict managed-status registry:

```bash
scripts/tmux-recovery/restore.py \
  --api-url http://127.0.0.1:8095 \
  --manifest <accepted-manifest.json> \
  --managed-status-output /srv/data/chrote/tmux-recovery/managed-status.json \
  ...
```

The output file is atomically replaced with mode `0600`. It contains only
identity, external owner, manager kind/ref, storage/source kind, and health
status. It must not contain descriptors, argv, env, tokens, or restart
instructions. CHROTE rejects the registry if the path is a symlink, is not a
regular file, or is group/world-readable or writable. It intentionally does not
require the same UID for the writer and `chrote-srv.service`; use the configured
path as the trust boundary and keep the parent directory writable only by the
trusted owner-side publisher and service administration.

## Rollback Lane

Use this only for legacy rollback checks:

```bash
systemctl --user status chrote.service --no-pager
curl http://127.0.0.1:8094/api/health
```

Do not restart `chrote.service` while working on the `/srv` lane unless the task
explicitly says legacy, rollback, or `8094`.
