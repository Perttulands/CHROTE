---
title: Several Unix users
nav: Several Unix users
summary: Front several Unix users' tmux servers from one CHROTE, through explicit socket mappings and deliberate grants.
sources: SECURITY.md, .env.example, install.sh, scripts/lib/tmux-server-access.sh
---

# Several Unix users

The generic installer runs CHROTE as you, against your own tmux server, and
that is the supported path. Some operators run agents under separate Unix
accounts, to separate harness configuration or process ownership, and want one
CHROTE in front of all of them. CHROTE supports that through explicit
configuration, never by discovery.

> [!WARNING]
> Every mapped user's terminal is reachable by anyone who reaches CHROTE.
> Mapping a user is giving the dashboard that user's shell. These accounts are
> operational identities, not mutually hostile tenants. Read
> [Security](security.md) first.

## Map each user to a socket

```bash
CHROTE_TMUX_SOCKET=alice=/run/user/2001/tmux-2001/default,build=/tmp/tmux-2002/default
CHROTE_TERMINAL_USER_WORKDIRS=alice=/home/alice/work,build=/home/build/work
```

- Entries are `unixUser=/absolute/socket`, comma-separated, in the order the
  dashboard lists them. Paths must be absolute and canonical; a user appears
  once. A bad entry stops the server at startup.
- With one entry, requests may leave the user out. With several, every request
  names its user, and the launcher asks which.
- A session is attached only on its own user's socket. If it is not there, the
  attach fails; CHROTE never tries another tmux server.
- Settings sets a launch user per terminal tab, a name prefix per user, and the
  identity colours come from the theme in this order.

## Give the server access

The CHROTE service account needs, for each mapped socket:

1. permission to traverse to the socket's directory and to read and write the
   socket file, usually through POSIX ACLs; and
2. tmux's own permission to attach: `tmux server-access -a -w <service-user>`,
   run as the socket's owner. `server-access` needs tmux 3.3 or newer.

CHROTE may add an explicitly configured grant, but it never removes or narrows
the owner's access and never reshapes permissions to manufacture isolation.
Missing access is reported, not repaired.

### The installer's grant path

Run as root with `CHROTE_TMUX_SOCKET` in its environment, `install.sh` applies
those grants for each mapping:

| Variable | Default | Meaning |
| --- | --- | --- |
| `CHROTE_TMUX_GRANT_USER` | `chrote` | The service account to grant |
| `CHROTE_TMUX_GRANT_SOCKET_ROOTS` | `/run/user/%u:/tmp/tmux-%u` | Colon-separated directories a socket must live under; `%u` is the owner's uid |

It checks that each owner exists, that the socket lies under an allowed root and
belongs to its owner, adds traverse ACLs on the path, a default ACL on the
socket's directory and read-write on the socket, then the tmux write grant.
A socket that is not running yet is reported; re-run after it starts.

> [!NOTE]
> This path grants access. It does not create the service account, and the unit
> it writes is a per-user unit for the account that ran it. Running CHROTE as a
> dedicated system service is host configuration you own; keep its unit, paths
> and grants in your operator configuration, not in this repository.

An ordinary, non-root install with `CHROTE_TMUX_SOCKET` set in your shell stops
with *grants require a root install*. Unset it for that run:
`env -u CHROTE_TMUX_SOCKET ./install.sh`.
