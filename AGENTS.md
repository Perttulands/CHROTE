# CHROTE Agent Guide

This file is the CHROTE-specific override. It inherits the broader
`/home/perttu/AGENTS.md` rules for Beads, Context Citadel, caution, and
verification; do not duplicate those rules here.

## Start Here

- Before starting CHROTE work, read `/home/perttu/RULES.md`.
- `PRD.md` is the canonical current product source.
- `README.md` is the operator quick reference for access, runtime, build, and product boundary.
- `docs/TEST_STRATEGY.md` has the fuller test matrix and copy-pasteable commands.
- `docs/troubleshooting.md` has runtime health checks and common service diagnostics.
- `CLAUDE.md` may contain useful historical agent guidance, but Codex does
  not load it unless fallback names are configured. Keep durable
  CHROTE-specific guidance in this file.

## Product Boundary

CHROTE is a private browser cockpit for durable host-owned work. The Ubuntu
host owns terminals, agents, files, dev servers, builds, tests, Beads, logs,
Gas City runtime state, and selected `/srv` service access. Browser clients are
disposable viewports.

Current CHROTE surfaces:

- durable tmux sessions and browser terminal panes
- files under configured workspace roots
- modern Beads via `bd`
- optional Beads Viewer via `bv`
- generic agent observability from tmux sessions
- selected local services through CHROTE-owned server-side routes
- early Gas City orchestration access for named agent identities

CHROTE 3.0 intentionally uses Gas City as the orchestration substrate while
CHROTE remains the access/operator layer for named sessions and named agent
identities. Do not turn this into a passive transcript/status dashboard; the
product goal is addressable agent collaboration through Gas City primitives.
Preserve useful legacy ideas in `docs/legacy-ideas.md`; treat
`docs/archive/*` as historical unless proven current.

For the current CHROTE/Gas City relationship, read
`docs/chrote-gascity-framing.md`. Native `gc` is the Gas City operator CLI;
do not mirror the `gc` command tree inside CHROTE.

## Architecture Map

- `src/cmd/server/main.go` - Go server entry point, flags, middleware, and route wiring.
- `src/internal/api/` - HTTP APIs for health, tmux, files, Beads, agents, and services.
- `src/internal/core/` - path, response, and session utilities shared by APIs.
- `src/internal/proxy/terminal.go` - ttyd terminal proxy behavior.
- `src/internal/dashboard/` - embedded production dashboard assets served by the Go binary.
- `dashboard/src/` - React dashboard source.
- `dashboard/tests/` - mocked Playwright browser tests.
- `dashboard/tests/integration/` - live backend/terminal Playwright smoke tests.
- `services/chrote.service` - current user systemd service template for this install.

## Runtime Safety

- Check `systemctl --user status chrote.service` before changing runtime behavior.
- Current service should bind CHROTE to `127.0.0.1:8094` and ttyd to `127.0.0.1:7683`.
- Use the CHROTE tmux socket, not the default socket, when inspecting live sessions:

```bash
TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux ls
tmux -S /run/user/1000/chrote-tmux/tmux-1000/default ls
```

- Do not kill tmux sessions unless the user explicitly asks.
- Keep listeners private to localhost or the configured tailnet access layer
  unless the user explicitly approves broader exposure.
- Private service adapter values live in `~/.config/chrote/services.env`;
  never print, commit, or document token values such as
  `CHROTE_CONTEXT_API_TOKEN`.
- Treat `services/chrote.service` and the README runtime docs as current.
  `src/deploy.sh` creates an older system service on different ports and
  should not be run or copied without first reconciling that conflict.

Runtime health checks:

```bash
systemctl --user status chrote.service
curl http://127.0.0.1:8094/api/health
curl http://127.0.0.1:8094/api/tmux/sessions
curl http://127.0.0.1:8094/api/oracle/status
curl http://127.0.0.1:8094/api/beads/health
```

## Before Editing

- Read the immediate caller/export/shared utility context for the area you
  touch.
- Keep changes surgical. Do not clean up adjacent docs, archived material, or
  old plans unless the bead or user request includes that scope.
- If changing frontend code served by the Go binary, rebuild `dashboard/dist`
  and copy it into `src/internal/dashboard/dist` before rebuilding the server.
- If touching runtime behavior, verify the service and health endpoints after the final change.

## Verification Gates

Choose the smallest gate that proves the change.

Backend Go changes:

```bash
cd /path/to/chrote/src
test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"
go vet ./...
go test ./...
```

Frontend React/TypeScript changes:

```bash
cd /path/to/chrote/dashboard
npm run lint
npm run test:unit
npm run build
```

Browser workflow changes:

```bash
cd /path/to/chrote/dashboard
npm test
```

Live backend or terminal behavior changes:

```bash
cd /path/to/chrote/dashboard
CHROTE_TEST_URL=http://127.0.0.1:8094 npm run test:live
```

Production dashboard asset changes:

```bash
cd /path/to/chrote/dashboard
npm run build
cd /path/to/chrote
rm -rf src/internal/dashboard/dist
cp -r dashboard/dist src/internal/dashboard/dist
cd src
go test ./...
go build -o ../chrote-server ./cmd/server
```

Docs-only changes:

- Inspect the rendered Markdown or source diff.
- Run no code tests unless the docs change includes commands or generated artifacts that need validation.
- Say explicitly that code tests were not run because the change was docs-only.
