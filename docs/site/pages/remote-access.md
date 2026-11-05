---
title: Reach CHROTE from other devices
nav: Remote access
summary: Keep the server on loopback and put an HTTPS front door on a private network you control in front of it.
sources: SECURITY.md, docs/installation.md, .env.example
---

# Reach CHROTE from other devices

CHROTE has no login. Anyone who can open it gets a terminal as every Unix user it
is configured for. So it binds to `127.0.0.1` by default, and the way to reach
it from a phone or another computer is a private network whose membership you
control, with HTTPS in front.

> [!CAUTION]
> Do not set `HOST=0.0.0.0` to "just try it" on a LAN, and never expose CHROTE
> to the public internet. Treat network access to CHROTE as shell access. See
> [Security](security.md).

## With Tailscale Serve

[Tailscale](https://tailscale.com/kb/1312/serve) is the private network the
project's own documents name. With the host on your tailnet and CHROTE still on
loopback:

```bash
tailscale serve --bg 8094
tailscale serve status
```

Tailscale terminates HTTPS with a certificate for the host's tailnet name and
forwards to `127.0.0.1:8094`. Open `https://<host>.<tailnet>.ts.net/` from any
device on the tailnet. Flags differ between Tailscale versions; check
`tailscale serve --help` on yours. Leave `HOST` at `127.0.0.1`: Serve reaches
loopback, and nothing else needs to.

## With another reverse proxy

Any front door works if it does three things:

1. **Preserves the original `Host` header.** The terminal WebSocket compares the
   browser's `Origin` with the host the request was addressed to, on host and
   port. A proxy that rewrites `Host` to `127.0.0.1:8094` makes every terminal
   answer `403`. If you cannot preserve it, list the public origin in
   `CORS_ORIGINS`, for example `CORS_ORIGINS=https://chrote.example.internal`.
2. **Passes WebSocket upgrades** on `/terminal/`, with long idle timeouts, since
   a terminal can sit quiet for hours.
3. **Keeps the server on loopback**, so the proxy is the only way in, and stays
   on a network only you can reach.

The scheme is not compared, so TLS terminated in front of a plain-HTTP loopback
bind is fine.

## Why HTTPS matters

Beyond protecting the traffic, browsers grant some features only to secure
origins. Browser notifications for [agent events](launcher.md#agent-events)
work on `https://` and on `http://127.0.0.1`, but not on plain HTTP to another
address. Clipboard copy from the docs and the dashboard is also more reliable
on a secure origin.

## Keep the service up while you are away

A user service usually stops at logout unless lingering is enabled. If you plan
to check on agents from your phone, run `loginctl enable-linger "$USER"` once;
see [Install](install.md#keep-it-running-after-you-log-out).

## On a phone

Below 768 pixels wide, the tab bar becomes a menu and the workspace shows one
window at a time. The Send drawer fills the visible screen, including when the
keyboard is up, and keeps your draft if you change target. Everything you need
to observe and steer is there: sessions, output, Peek, prompts, Send. Arranging
multi-window layouts stays a desktop job.
