---
title: Security model
nav: Security model
summary: CHROTE has no login. Whoever reaches it holds a shell as every configured user. The network is the boundary.
sources: SECURITY.md, PRD.md
---

# Security model

CHROTE is private infrastructure for one human operator. Nothing in the
application authenticates a request, and the design does not add a login. That
is a deliberate trade, and it puts the whole weight of security on where CHROTE
can be reached from.

## What reaching CHROTE gives

Anyone who can open the dashboard holds, at once:

- **A terminal for every Unix user** in `CHROTE_TMUX_SOCKET`: arbitrary command
  execution as each of them.
- **The file API** across everything under `CHROTE_ROOTS`, with the service
  account's Unix permissions.
- **Beads data, service controls and schedules.**

Treat exposing the dashboard as exposing the shells of every configured user.

## The boundary is the network

- The default bind is `127.0.0.1:8094`. Keep it.
- Reach CHROTE remotely only through a private network you control, with HTTPS,
  such as Tailscale Serve. See [Remote access](remote-access.md).
- Do not bind to an untrusted LAN or the public internet.
- `API_AUTH_TOKEN` and similar retired settings are ignored and protect nothing;
  the server logs a warning if one is set.

## Browser origins are not authentication

`CORS_ORIGINS` names browser origins CHROTE accepts, and is enforced in two
places. The HTTP API sends CORS headers only to listed origins. The terminal
WebSocket checks `Origin` itself and refuses a foreign one with `403` before any
tmux socket is touched; unset, only the origin the request was addressed to is
accepted, compared on host and port. A request with no `Origin` at all, as from
`curl`, is served: non-browser clients are governed by what CHROTE is bound to.

This stops a web page you did not open from driving your CHROTE. It identifies
no user and stops no direct network client.

## Terminals

- A terminal is command execution as that Unix user.
- The launcher sends a harness id, never a command, and adds nothing a terminal
  did not already allow.
- Cross-user access requires deliberate socket and tmux grants. CHROTE never
  guesses socket ownership or widens access on its own.
- Showing a session attaches beside other clients, including someone's SSH
  session, and shows what they are doing live. It detaches nobody. Claiming the
  size changes what every viewer sees, never the processes. Who else is
  attached is shown.
- Browser disconnects and CHROTE restarts do not kill tmux work.

## Files

- `CHROTE_ROOTS` bounds CHROTE's file API, after symlink resolution, for reads
  and writes. Keep it as narrow as practical; `/` grants everything the service
  account can touch.
- Inside the roots, Unix permissions decide. There is no second sensitive-path
  policy.
- The roots **do not sandbox agents**. A program in tmux keeps its Unix user's
  full filesystem access.

## Services, schedules and secrets

- Service URLs and tokens stay in server configuration. The browser calls
  CHROTE's own routes and never receives a service credential.
- The Services tab forwards only list, start and stop to the configured
  controller, and only with CHROTE's request header.
- Scheduled tasks mutate the host, so their API requires an explicit intent
  header, resolves tmux sockets on the server, and records failures in the run
  history.
- Keep `secrets.env` mode `0600` (the installer creates it so). Never commit
  tokens, private environment files, socket grants, terminal transcripts or
  private screenshots.

## Supported versions and reporting

Security fixes target `main` and the newest `2.0.0-alpha` release. Report a
vulnerability through GitHub's private security-advisory flow for the
repository, or contact the maintainer directly; do not post exploit details in a
public issue.

The authoritative statement is [`SECURITY.md`](../../../SECURITY.md).
