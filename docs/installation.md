# Installation

This guide describes a generic local CHROTE install. Replace `/path/to/chrote`
and `<workspace-root>` with the paths for your host.

## Runtime

```bash
systemctl --user status chrote.service
curl http://127.0.0.1:8094/api/health
tailscale serve status
```

Tailnet URL format:

```text
https://<tailnet-host>:8445/
```

## Rebuild

```bash
cd /path/to/chrote/dashboard
npm ci
npm run build

cd /path/to/chrote
rm -rf src/internal/dashboard/dist
cp -r dashboard/dist src/internal/dashboard/dist

cd /path/to/chrote/src
go test ./...
go build -o ../chrote-server ./cmd/server
systemctl --user restart chrote.service
```

## Service Configuration

The service file is:

```text
/path/to/chrote/services/chrote.service
```

The user-systemd symlink is:

```text
~/.config/systemd/user/chrote.service
```

Current important environment:

```text
TMUX_TMPDIR=/run/user/1000/chrote-tmux
CHROTE_WORKDIR=<workspace-root>
CHROTE_ROOTS=<workspace-root>
CHROTE_BEADS_WORKSPACES=<workspace-root>
CHROTE_BD_COMMAND=bd
```

## Not Installed By This Setup

- Gastown
- Ralph
- Beads Viewer `bv`
- Teams harness launcher
- Chat/nudge integrations
