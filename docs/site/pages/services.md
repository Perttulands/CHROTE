---
title: Services
nav: Services
summary: A host-owned catalog of services, each with its state and one action, Start or Stop, forwarded to a controller you run.
sources: PRD.md, SECURITY.md, DESIGN-SYSTEM.md, src/internal/api/services.go
---

# Services

The Services tab lists the services a host chooses to expose, with a description,
the current state, an optional link, and one action per row: **Start** or
**Stop**. Restart, Open and Send are in the row's menu. A service may be a group
of components; the controller reports partial or failed state and starts or
stops the group.

The tab changes **current state only**. Whether a service starts at boot is
startup configuration, and CHROTE never touches it.

## How it works

CHROTE does not know how to start anything. It forwards three kinds of request
to a **controller** you run, over a Unix socket:

```text
Browser ──► CHROTE  GET  /api/services                ──► controller  GET  /services
                    POST /api/services/{id}/start     ──►             POST /services/{id}/start
                    POST /api/services/{id}/stop      ──►             POST /services/{id}/stop
```

The host owns the catalog, the supervisor and its credentials, and the socket's
permissions. The browser supplies only a service id. Actions require CHROTE's
own request header. If no controller is configured, or it is unreachable, the
tab says so and nothing else in CHROTE is affected.

## Configure it

Create `services.json`. CHROTE looks, in order, at `CHROTE_SERVICES_CONFIG`; at
`services.json` beside `CHROTE_LAUNCH_CONFIG` when that is set; and otherwise at
`~/.config/chrote/services.json`. The file is read once, at startup.

```json
{
  "socket": "/absolute/path/to/services.sock",
  "resident": {
    "session": "service-agent",
    "folder": "/absolute/path/to/service-agent",
    "beads": "/absolute/path/to/store"
  }
}
```

`socket` must be an absolute path. `resident` is optional and names the
tab's resident agent, as for the other [residents](integrations.md#residents).
A file that is not valid JSON, or names a relative socket, is reported in the
service log at startup and the tab behaves as unconfigured. Restart CHROTE after
editing the file.

## The controller's contract

This is the shape the dashboard reads today, documented from the code
(`src/internal/api/services.go` and `dashboard/src/components/ServicesView/servicesApi.ts`).
It is a contract between your controller and CHROTE, so check it against the
version you run.

`GET /services` answers with the catalog in display order:

```json
{
  "services": [
    {
      "id": "search",
      "name": "Search",
      "description": "The corpus search service",
      "link": "http://127.0.0.1:7700",
      "state": "running",
      "controllable": true
    }
  ]
}
```

- `id` matches `^[a-z0-9][a-z0-9_-]*$`.
- `state` is one of `running`, `stopped`, `starting`, `stopping`, `partial`,
  `failed`, `unknown`. The tab reads more often while a service is starting or
  stopping.
- `controllable: false` shows the status with no action.

`POST /services/{id}/start` and `/stop` answer with `{ "service": { … } }`, the
service's new state. A failure is a non-2xx status or a body carrying
`"error": "…"` or `"errors": ["…"]`; the row shows the controller's own words
in place of its description. CHROTE allows an action up to 90 seconds and caps a
response at 2 MiB.

## The service agent

When `resident` is set, a tmux session lives behind the spine at the tab's
right edge. `Alt+S` pastes the selected service into its prompt.
