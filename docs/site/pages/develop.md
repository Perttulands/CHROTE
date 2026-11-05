---
title: Develop
nav: Develop
summary: Build CHROTE from source, run it without installing, work on the dashboard, and run the checks CI runs.
sources: CONTRIBUTING.md, README.md, dashboard/README.md, docs/TEST_STRATEGY.md, AGENTS.md
---

# Develop

Small, reviewable, verified changes. Before changing behaviour, read
[`CONTRIBUTING.md`](../../../CONTRIBUTING.md), the spec that owns it (the
[source-truth index](../../source-truth-index.md) says which), and
[`docs/TEST_STRATEGY.md`](../../TEST_STRATEGY.md) for where its test belongs.

## Layout

| Path | Owns |
| --- | --- |
| `dashboard/src/` | The React interface and device-local presentation |
| `src/cmd/server/` | Server assembly |
| `src/internal/api/` | Host resources: tmux, files, Beads, schedules, services, Library, residents, theme, launch, health, system |
| `src/internal/proxy/` | Terminal transport and the pseudo-terminals it attaches on |
| `src/internal/dashboard/` | The embedded dashboard the binary serves |
| `scripts/` | Build, install and validation entrypoints |
| `docs/site/` | This documentation site |

## Build

```bash
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh
python3 scripts/check-embedded-dashboard.py
./scripts/build-server.sh ./chrote-server
```

The dashboard is embedded in the binary. Always produce it with
`build-embedded-dashboard.sh`, which also writes the stamp
`check-embedded-dashboard.py` verifies; copying `dashboard/dist` by hand
bypasses that contract. `build-server.sh` stamps the version from `VERSION` and
the checkout's commit.

## Run a server from a source build

A bare `./chrote-server` uses compiled defaults meant for an installed host: no
tmux socket, so terminals answer "no tmux sockets are configured", and state
directories outside your home. Give it what the installer would:

```bash
state="$PWD/.dev-state"
mkdir -p "$state"
tmux_tmp="${TMUX_TMPDIR:-/tmp}"
HOST=127.0.0.1 PORT=8090 \
CHROTE_ROOTS="$HOME/work" CHROTE_WORKDIR="$HOME/work" \
CHROTE_TMUX_SOCKET="$(id -un)=${tmux_tmp%/}/tmux-$(id -u)/default" \
CHROTE_SCHEDULED_TASKS_DIR="$state/scheduled-tasks" \
CHROTE_SESSION_DROPS_DIR="$state/session-drops" \
CHROTE_AGENT_HOOKS_DIR="$state/agent-hooks" \
./chrote-server
```

Port 8090 is the backend the dashboard's Vite dev server proxies to (below),
and stays clear of an installed CHROTE on 8094. The socket path drops a trailing
slash from `TMUX_TMPDIR`, as the installer does: the server refuses a path that
is not canonical. This server
attaches to your real tmux server: it shows and can kill your real sessions.
For anything destructive, point `CHROTE_TMUX_SOCKET` at a scratch socket
(`tmux -S /tmp/chrote-dev.sock new -d`) instead.

## Dashboard development

```bash
cd dashboard
npm run dev
```

Vite serves on `http://localhost:5173` and proxies `/api` and `/terminal` to
`http://localhost:8090`, as set in `vite.config.ts`: run the source server
above on `PORT=8090`, or the dashboard has no backend. Change dashboard dependencies with npm 11
(`npx npm@11 install <pkg>`); `npm ci` works on either line.

## Checks

For documentation:

```bash
python3 scripts/doc-lint.py
python3 scripts/host-neutrality.py
python3 scripts/build-docs-site.py --check
git diff --check
```

For code, the narrow test first, then the gates CI runs:

```bash
cd dashboard && npm run test:unit && npm run lint && npm test && cd ..
cd src && go vet ./... && go test -race ./... && cd ..
CHROTE_SERVER_BINARY="$PWD/chrote-server" ./scripts/test-built-server-contract.sh
```

The built-server and installer checks create and clean up their own tmux
sessions on disposable sockets. The full recipe, including the installer smoke
test, is in [`CONTRIBUTING.md`](../../../CONTRIBUTING.md#stable-local-gates).

## This documentation site

The site is built with the standard library from Markdown in this repository:

| Path | Holds |
| --- | --- |
| `docs/site/pages/*.md` | The guide pages, each with a short header: `title`, `nav`, `summary`, `sources` |
| `docs/site/nav.json` | Sections and page order |
| `docs/site/template.html`, `docs/site/static/` | The page frame, styles and script |
| `scripts/build-docs-site.py` | The builder |

The canonical documents (README, PRD, ARCHITECTURE, SECURITY, the installation
and troubleshooting references, ADRs and the rest) are rendered as they are
under **Source documents**, so a guide links to the source of truth instead of
copying it. Fonts come from `dashboard/public/fonts`.

```bash
python3 scripts/build-docs-site.py --serve   # build into docs/site/_site/ and serve on 127.0.0.1:8000
python3 scripts/build-docs-site.py --check   # build to a temporary directory; fail on a broken link or anchor
```

The output is static, uses relative links only, and opens straight from disk.
`docs/site/_site/` is ignored by git. When a guide page and a canonical document
disagree, fix the guide, or fix the canonical document if it is the one that
is wrong.
