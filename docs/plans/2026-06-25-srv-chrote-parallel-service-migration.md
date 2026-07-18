# Plan: move CHROTE toward a `/srv` service without touching the current user service

Date: 2026-06-25
Planning bead: `chrt-5p5h`

> **Product-topology correction (2026-07-17):** the dedicated `/run` Formations
> tmux socket described below is historical and no longer an executable
> promotion path. Current dogfood uses only test-owned paths under fixed `/tmp`. It is
> not the production Formations design. Production Formations
> borrows eligible existing sessions through the same configured Terminal-session
> resolver and inventory used by Terminal tabs, including its explicit user/socket
> sources, with fail-loud busy/attached arbitration
> and same-session-lineage evidence. ADR-0007 is authoritative where this older
> plan differs.

## Success state

A new CHROTE instance runs from `/srv` as a real local service, in parallel with the current `perttu` user service. The current service on `127.0.0.1:8094` remains untouched until the new one proves parity.

The new instance:

- binds only to loopback on a different port during proving, e.g. `127.0.0.1:8095`;
- runs from `/srv/chrote`, with data under `/srv/data/chrote`;
- runs as a dedicated non-root service identity, not as `perttu`, `tavern`, or root;
- exposes both `perttu` and `tavern` terminal sessions through deliberate socket ACLs/shared socket paths;
- has shared Archon/Formations state under `/srv/data/chrote`, not hidden in one user's home;
- keeps service adapter credentials server-side and out of git;
- updates the agent-facing skills, project docs, runbooks, and connected-system references that currently assume `/home/perttu/chrote`, the user `chrote.service`, or port `8094` as the primary CHROTE path;
- has an explicit cutover/rollback procedure, so switching back to the current user service is one command or one Tailscale/reverse-proxy pointer change.

## Current inspected facts

Current live service:

```text
unit:        /home/perttu/.config/systemd/user/chrote.service
status:      active
http:        127.0.0.1:8094
health:      /api/health OK
workdir:     /home/perttu/chrote
binary:      /home/perttu/chrote/chrote-server
ttyd:        7683
runtime:     perttu user systemd
```

Important current env shape:

```text
CHROTE_WORKDIR=/home/perttu
CHROTE_ROOTS=/
CHROTE_TERMINAL_USERS=perttu,tavern
CHROTE_TERMINAL_USER_SOCKETS=perttu=/run/user/1000/chrote-tmux/tmux-1000/default,tavern=/tmp/tmux-1001/default
CHROTE_TERMINAL_USER_WORKDIRS=perttu=/home/perttu,tavern=/home/tavern
CHROTE_FORMATIONS_TMUX_SOCKET=/run/user/1000/chrote-formations-tmux/default
CHROTE_FORMATIONS_TMUX_CWD=/home/perttu
CHROTE_FORMATIONS_TMUX_ROOTS=/home/perttu
CHROTE_AGENTS_DIR currently defaults to the service user's home/agents unless set
```

Those legacy Formations values describe the rollback unit's historical
dedicated socket. They are not an accepted promotion path: current source
rejects non-temporary Formations tmux sockets and roots, and no `DEDICATED` or
`PROD_SMOKE` value grants live access.

`/srv` currently has a Docker Compose stack for Camofox, TTS, Context Citadel, and Ollama. CHROTE is not currently part of that compose file. Because CHROTE needs host tmux sockets, ptys, ttyd, home/workspace files, and per-user ACLs, the first `/srv` deployment should be a host systemd service, not a container. Containerizing first would make tmux/socket/user boundaries worse, not cleaner.

## Non-goals for the first migration slice

- Do not stop, disable, or mutate the existing `chrote.service` on `8094`.
- Do not expose CHROTE on a public interface.
- Do not run CHROTE as root.
- Do not copy secrets from user homes into `/srv` blindly.
- Do not pretend cross-user file writes are safe until ownership behavior is tested.
- Do not claim Formations real-agent execution is service-ready until service-owned agent credentials/sessions are deliberately provisioned and smoked.
- Do not bulk-rewrite archived/historical design documents as if they are current runbooks. Classify them, add archive/superseded labels when needed, and update only active instruction surfaces.

## Recommended target architecture

Use a dedicated service account:

```text
user:  chrote
home:  /srv/chrote
shell: nologin or locked shell unless interactive debugging is explicitly needed
group: chrote, plus a shared access group if needed
```

Recommended layout:

```text
/srv/chrote/                         # CHROTE source checkout and runtime scripts
/srv/chrote/chrote-server             # built binary for the /srv instance
/srv/chrote/terminal-launch.sh        # launch script used by ttyd
/srv/chrote/config/chrote.env         # private env, 0640 root:chrote or chrote:chrote
/srv/chrote/systemd/chrote-srv.service# unit source copied to /etc/systemd/system/

/srv/data/chrote/workspace/           # service-owned CHROTE workspace
/srv/data/chrote/workspace/.formations# shared Formations boards/layout/runs
/srv/data/chrote/agents/              # shared Archon/Agents persona cards
/srv/data/chrote/evidence/            # optional smoke/evidence output

/run/chrote/                          # systemd RuntimeDirectory for sockets/ttyd helpers
/run/chrote/tmux/                     # service-owned default tmux socket if needed
```

First parallel ports:

```text
existing user service: 127.0.0.1:8094, ttyd 7683
new /srv service:      127.0.0.1:8095, ttyd 7686
```

Initial `/srv/chrote/config/chrote.env` shape:

```bash
HOST=127.0.0.1
PORT=8095
TTYD_PORT=7686
TMUX_TMPDIR=/run/chrote/tmux

CHROTE_WORKDIR=/srv/data/chrote/workspace
CHROTE_ROOTS=/srv,/home/perttu,/home/tavern
CHROTE_LAUNCH_SCRIPT=/srv/chrote/terminal-launch.sh

CHROTE_TERMINAL_USERS=perttu,tavern
CHROTE_TERMINAL_USER_SOCKETS=perttu=/run/user/1000/chrote-tmux/tmux-1000/default,tavern=/tmp/tmux-1001/default
CHROTE_TERMINAL_USER_WORKDIRS=perttu=/home/perttu,tavern=/home/tavern

CHROTE_AGENTS_DIR=/srv/data/chrote/agents
CHROTE_BEADS_WORKSPACES=/srv,/home/perttu/chrote,/home/perttu,/home/tavern,/home/tavern/velvetwood
CHROTE_BEADS_AUTO_DISCOVER=0
CHROTE_BD_COMMAND=/home/perttu/.local/bin/bd

# Do not configure the stock Formations tmux executor on the long-lived service.
# Isolated dogfood injects CHROTE_FORMATIONS_TMUX_* only into a bounded test
# process, with its socket, cwd, and roots all beneath one mktemp directory.

CHROTE_TTS_URL=http://127.0.0.1:3100
CHROTE_CONTEXT_API_URL=http://127.0.0.1:3200
# CHROTE_CONTEXT_API_TOKEN belongs here only if the server-side adapter needs it.
```

The exact roots can be narrowed after a capability matrix. Do not use `CHROTE_ROOTS=/` on the `/srv` service unless we intentionally accept “service account can browse every file it can read.”

## Phase 0 — inventory and freeze the baseline

Goal: know exactly what must be reproduced before touching deployment.

Steps:

1. Capture current service health and config:
   - `systemctl --user status chrote.service --no-pager`
   - `curl -fsS http://127.0.0.1:8094/api/health`
   - current env from the unit, redacting private env files.
2. Capture current repo/build state:
   - `git status --short`
   - `npm run test:unit`
   - `npm run build`
   - embed dashboard dist
   - `go test ./...`
   - `go build -o ../chrote-server ./cmd/server`
3. Capture a capability matrix for current `8094`:
   - dashboard root loads;
   - terminal session list for `perttu` and `tavern`;
   - create+attach new session as `perttu`;
   - create+attach new session as `tavern`;
   - Files read/write smoke for intended roots;
   - Beads health/workspace list;
   - Agents list;
   - Formations board list;
   - Services panel routes for Context/TTS/etc.

Verification:

- Evidence file under `/srv/data/chrote/evidence/preflight-<timestamp>/` or existing CHROTE evidence root.
- No mutation to current service except safe smoke-created sessions that are cleaned up.

## Phase 0B — inventory active skills, docs, and connected-system references

Goal: know which agent-facing instructions and runbooks will keep steering people back to the old user-scoped CHROTE service after the `/srv` instance exists.

Initial scan already shows many references under both `/home/perttu/skills` and `/home/perttu/chrote` to `CHROTE`, Archon, Formations, `/home/perttu/chrote`, `chrote.service`, and `127.0.0.1:8094`. Treat this as a real migration surface, not cosmetic documentation cleanup.

Inventory buckets:

1. **Hot skills and agent instructions** — update before cutover:
   - `/home/perttu/skills/chrote-ui-work/SKILL.md` and its active references;
   - `/home/perttu/skills/tmux-agent-driving/` references that locate CHROTE sockets/sessions;
   - `/home/perttu/skills/codex/` CHROTE durable-worker references;
   - `/home/perttu/skills/beads-workflow/references/chrote-formations-beads-readiness.md`;
   - `/home/perttu/skills/personal-ai-infrastructure/` CHROTE/Gas City framing references;
   - Pi/Claude/OpenCode review skills only where they invoke CHROTE, Archon, Formations, or tmux paths.
2. **Active CHROTE project docs** — update or split into user-service vs `/srv` sections:
   - `README.md`, `AGENTS.md`, `.env.example`, `SECURITY.md`, `CONTRIBUTING.md`;
   - `ARCHON.md`, `FORMATIONS.md`, `DATA-MODEL.md`, `docs/source-truth-index.md`;
   - `docs/installation.md`, `dashboard/README.md`, `docs/TEST_STRATEGY.md`;
   - Windows helpers and install/uninstall scripts if they are still presented as current operator paths.
3. **Connected-system docs** — update only where CHROTE is the caller or control plane:
   - `/srv/AGENTS.md` or a new `/srv` CHROTE runbook;
   - Context Citadel, TTS, Camofox, Beads, Archon/Formations service-adapter notes;
   - service health/runbook docs that name CHROTE ports, unit names, or data paths.
4. **Archive/prototype/history** — do not rewrite as current truth:
   - `docs/archive/**`;
   - `Perttus_vision_for_agent_orchestration/archive/**`;
   - old Gas City research/framing docs;
   - old PR/review incident references in skills.

For each match, classify it as one of:

```text
current-user-service baseline       # still valid while 8094 is the old service
new-/srv-service target             # should point at /srv paths, chrote-srv.service, 8095/7686
dual-mode transition                # must mention both and say which one to use when
historical/archive                  # leave alone or label; not an operator source of truth
wrong/stale                         # remove or replace
```

Verification:

- Save a machine-readable inventory under `/srv/data/chrote/evidence/source-truth-<timestamp>/` or the CHROTE evidence root.
- Every hot skill/doc with deployment commands says whether it targets the old user service or the new `/srv` service.
- No active instruction says plain `restart chrote.service` or `open 8094` after cutover without labeling it as the legacy user service.
- Changed shared skills pass the skill validator/health check for the touched skill directories.
- Project docs pass `scripts/doc-lint.py`; extend doc-lint if needed so future agents do not reintroduce unlabeled `8094`, `/home/perttu/chrote`, or user-systemd commands into active `/srv` runbooks.

## Phase 1 — create `/srv` layout and service identity

Goal: create a place where CHROTE can live without yet starting a daemon.

Steps:

1. Create `chrote` service user and group.
2. Create directories:
   - `/srv/chrote`
   - `/srv/chrote/config`
   - `/srv/data/chrote/workspace`
   - `/srv/data/chrote/agents`
   - `/srv/data/chrote/evidence`
3. Make `/srv/data/chrote` group-writable where shared state is intended:
   - setgid directories;
   - `umask 0002` or systemd `UMask=0002` for shared files.
4. Put CHROTE source/build in `/srv/chrote`.

Strong recommendation: start with a real git checkout at `/srv/chrote`, not a copied binary-only release. Once the service shape is stable, add release/current symlinks if we actually need deployment promotion mechanics.

Safety boundary:

- Do not copy `/home/perttu/.config/chrote/services.env` or any profile secrets wholesale.
- Do not change `/srv/docker-compose.yml` yet. CHROTE should join `/srv` as a host systemd service first.

Verification:

- `sudo -u chrote test -x /srv/chrote/chrote-server` after build.
- `sudo -u chrote test -r /srv/chrote/config/chrote.env` without printing env contents.
- `sudo -u chrote test -w /srv/data/chrote/workspace`.

## Phase 2 — build and smoke the `/srv` binary without systemd

Goal: prove the binary can start from `/srv` on alternate ports before making it a service.

Steps:

1. Build dashboard and embed assets inside `/srv/chrote`.
2. Build `/srv/chrote/chrote-server`.
3. Start it in a foreground one-shot smoke environment on `8095`/`7686` as `chrote`.
4. Hit:
   - `http://127.0.0.1:8095/api/health`
   - `http://127.0.0.1:8095/`
   - `http://127.0.0.1:8095/api/agents`
   - `http://127.0.0.1:8095/api/formations/boards`

Safety boundary:

- It must not bind `8094` or `7683`.
- It must not read private env from user homes.

Verification:

- `/srv` instance health is green while current `8094` remains green.

## Phase 3 — install parallel system service

Goal: make the new service persistent while still isolated from the current one.

Systemd shape:

```ini
[Unit]
Description=CHROTE /srv workspace cockpit
After=network.target

[Service]
Type=simple
User=chrote
Group=chrote
WorkingDirectory=/srv/chrote
EnvironmentFile=/srv/chrote/config/chrote.env
RuntimeDirectory=chrote
RuntimeDirectoryMode=0750
UMask=0002
ExecStart=/srv/chrote/chrote-server --host 127.0.0.1 --port 8095 --ttyd-port 7686
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Open question: name it `chrote-srv.service` during proving, not `chrote.service`, to avoid confusion with the current user unit.

Verification:

- `systemctl status chrote-srv.service --no-pager`
- `curl -fsS http://127.0.0.1:8095/api/health`
- `curl -fsS http://127.0.0.1:8094/api/health` still OK.

## Phase 4 — terminal access for both users

Goal: the `/srv` service can list, create, and attach `perttu` and `tavern` sessions without becoming either user and without granting either user access to the other's private control socket beyond what is intended.

Recommended two-step path:

### 4A. Temporary ACL parity

Grant the `chrote` service account minimum access to existing tmux socket paths:

```bash
# Example shape only; exact paths must be verified live.
setfacl -m u:chrote:x /run/user/1000/chrote-tmux/tmux-1000
setfacl -m u:chrote:rw /run/user/1000/chrote-tmux/tmux-1000/default
setfacl -m u:chrote:x /tmp/tmux-1001
setfacl -m u:chrote:rw /tmp/tmux-1001/default
```

Then smoke:

- list `perttu` sessions via `/api/tmux/sessions` or selected user route;
- list `tavern` sessions;
- create a `perttu` smoke session and verify it runs as `perttu`;
- create a `tavern` smoke session and verify it runs as `tavern`;
- attach each through ttyd;
- delete smoke sessions.

### 4B. Durable shared socket paths

Temporary ACLs on `/tmp/tmux-1001` are brittle. The clean end state should expose per-user tmux sockets at deliberate paths, for example:

```text
/run/chrote/user-tmux/perttu/default
/run/chrote/user-tmux/tavern/default
```

Those sockets should be created by per-user user services or startup scripts, with default ACLs for `chrote`. This avoids relying on `/tmp/tmux-$uid` parent permissions and avoids giving `tavern` access to CHROTE's own cockpit socket.

Safety boundary:

- Never give `tavern` access to the `perttu`/CHROTE cockpit socket.
- Never make `chrote` root just to solve socket permissions.
- Do not kill live tmux sessions to move sockets unless explicitly approved; prefer new shared sockets and gradual session migration.

## Phase 5 — shared Archon/Formations state

Goal: Archon and Formations become service-level shared surfaces, not state hidden in `/home/perttu`.

Steps:

1. Set `CHROTE_AGENTS_DIR=/srv/data/chrote/agents`.
2. Use `CHROTE_WORKDIR=/srv/data/chrote/workspace`, making Formations state live at:
   - `/srv/data/chrote/workspace/.formations/boards`
   - `/srv/data/chrote/workspace/.formations/layout`
   - run ledgers/artifacts under the same service workspace.
3. Seed or migrate current useful persona cards and boards deliberately:
   - copy only stable catalog/persona/board data;
   - do not copy noisy smoke runs unless archived as evidence.
4. Add a wrapper for CLI use, e.g. `/srv/chrote/bin/archon-srv`, that exports the same env as the service and runs `/srv/chrote/archon` or the built archon command.
5. Decide executor mode:
   - `lab` mode for API/cockpit behavior proof with no agent credentials;
   - bounded tmux dogfood only in a test process whose socket, cwd, and roots all resolve under fixed `/tmp`; never configure that adapter on the long-lived service;
   - the shared-cockpit production resolver only after the ADR-0007 input fence and coordinator integration are implemented and certified;
   - per-user execution only if CHROTE grows a first-class per-user formations executor model. Current env is single-socket, so pretending it is multi-user would be lying.

Verification:

- `/api/agents` shows shared `/srv/data/chrote/agents` catalog.
- `/api/formations/boards` shows service-owned boards.
- `archon-srv agent list --json` matches the UI roster.
- `archon-srv board list --json` matches the UI board list.
- If real executor is enabled, a minimal real run writes a ledger under `/srv/data/chrote/workspace/.formations` and no user-home state unexpectedly changes.

## Phase 6 — Files, Beads, and service adapters

Goal: prove every non-terminal surface under the new service identity.

Files:

- Start with read checks on `/srv`, `/home/perttu`, `/home/tavern`.
- For write checks, use throwaway files under explicit test directories and verify owner/group/mode afterwards.
- If service-owned writes create bad ownership in user homes, do not cut over. Either keep writes scoped to `/srv`, or add a per-user mutation broker/helper as a separate design.

Beads:

- Verify `CHROTE_BD_COMMAND` works under the `chrote` service account.
- Verify read/list for configured workspaces.
- Verify a throwaway create/update/close in a dedicated test workspace, not random project beads.
- For real project beads, decide whether `chrote` is an acceptable writer or whether Beads writes need a per-user/project ownership strategy.

Services:

- Verify service proxies against current `/srv` services:
  - Context Citadel `3200`
  - TTS gateway `3100`
  - Camofox `9377`
- Keep tokens in `/srv/chrote/config/chrote.env` or another private env file, never in git.

## Phase 6B — update skills, docs, and source-of-truth links

Goal: after the `/srv` instance exists and before cutover, make active instructions point at the correct service shape so agents stop operating from stale muscle memory.

Order matters:

1. Keep `/home/perttu/chrote` and `8094` documented as the **legacy/current user service** until cutover.
2. Add `/srv/chrote`, `/srv/data/chrote`, `chrote-srv.service`, `8095`, and `7686` as the **parallel proving service** while both run.
3. After cutover, flip active runbooks and hot skills so `/srv` is primary and `8094` is rollback/legacy only.
4. Only then consider pruning obsolete user-service install helpers.

Required updates before cutover:

- `chrote-ui-work` skill and active references: default workdir, build/embed/deploy commands, old/new service status checks, live smoke ports, and `/srv` migration reference.
- `tmux-agent-driving` skill references: CHROTE socket discovery must include `/run/chrote/user-tmux/*` or the final shared socket contract, not just the old Chrote socket/user tmpdir paths.
- `codex`, Pi review, and agent-worker skills: any durable CHROTE worker pattern must say whether it targets the user service, `/srv` service, Archon CLI wrapper, or direct tmux.
- CHROTE project docs: README/installation/security/contributing/source-truth index must make the deployment modes explicit.
- Archon/Formations docs: state location, CLI wrapper, executor socket, evidence/runs directory, and credential ownership must match the `/srv` service.
- `/srv` docs: add or update a service runbook that treats CHROTE as part of the `/srv` stack even though it is host systemd, not Docker Compose.

Do not update by blind search-and-replace. `8094` and `/home/perttu/chrote` remain valid as baseline/rollback references until the old service is retired. The bug is unlabeled current-path advice, not the mere existence of historical references.

Verification:

- A fresh-agent read-through from `/home/perttu/RULES.md` → project `AGENTS.md` → `chrote-ui-work` → CHROTE README/runbook leads to the intended service for the current phase.
- `search_files`/`rg` inventory for `/home/perttu/chrote`, `chrote.service`, `127.0.0.1:8094`, `/srv/chrote`, `chrote-srv.service`, `127.0.0.1:8095`, `7686`, `CHROTE_WORKDIR`, `CHROTE_AGENTS_DIR`, and `CHROTE_FORMATIONS_*` has every active match classified.
- Changed skills validate with the skill gate, and changed docs pass markdown/readback plus `scripts/doc-lint.py`.
- The parity gauntlet includes a docs/skills row: an agent using only current docs can build, restart, smoke, and roll back the correct service without guessing.

## Phase 7 — parity gauntlet before cutover

Required side-by-side checks:

```text
8094 current user service: green
8095 /srv service:         green
```

Minimum parity matrix:

- dashboard root loads;
- static assets served from embedded dist;
- terminal session list for `perttu`;
- terminal session list for `tavern`;
- create+attach new `perttu` session, verify target user;
- create+attach new `tavern` session, verify target user;
- side-panel session creation and empty-window attach semantics still work;
- Files read smoke for intended roots;
- Files write smoke in approved throwaway roots, with ownership readback;
- Beads workspace list and health;
- Agents roster;
- Formations board list;
- Archon CLI wrapper parity;
- service panel routes;
- active skills/docs/runbooks point to the right old/new service for the current phase;
- browser Playwright smoke for terminal attach and key dashboard tabs;
- secret scan over diff/config templates;
- journal scan for permission errors and leaked env values.

Do not cut over if any row is red or unproven.

## Phase 8 — cutover and rollback

Cutover should be boring:

1. Stop sending traffic to `8094`; point Tailscale Serve/reverse proxy/bookmark to `8095`.
2. Leave the old user service running for an observation window unless port/conflict cleanup requires stopping it.
3. After observation, disable the old user service only with explicit approval:

```bash
systemctl --user disable --now chrote.service
```

Rollback:

- Repoint traffic/bookmark/Tailscale Serve back to `8094`.
- If needed, stop only the new system service:

```bash
sudo systemctl stop chrote-srv.service
```

No data migration should be destructive in the first cutover. Current `/home/perttu` state remains intact until `/srv` state has proven itself.

## Implementation beads to file if approved

1. `Prepare /srv CHROTE service account and directory layout`
2. `Build CHROTE from /srv and run foreground smoke on alternate ports`
3. `Install chrote-srv.service parallel systemd unit`
4. `Wire durable per-user tmux socket access for chrote service account`
5. `Move shared Archon/Formations state to /srv/data/chrote`
6. `Verify Files/Beads ownership behavior under chrote service account`
7. `Audit and update CHROTE skills/docs/runbooks for /srv service migration`
8. `Run /srv CHROTE parity gauntlet against current 8094 service`
9. `Cut over CHROTE traffic from user service to /srv service`

## Main risks

- **File ownership:** a service account writing inside user homes can create `chrote`-owned files. This is the biggest hidden footgun.
- **Tmux socket brittleness:** `/tmp/tmux-$uid` ACLs are not a durable contract. Shared socket paths or user services are cleaner.
- **Agent credentials:** service-owned Formations agents need deliberate Claude/Codex config. Do not copy user credentials casually.
- **Single Formations executor socket:** current Formations executor config is one socket, not per-user. Shared service-owned execution is straightforward; per-user execution is a later feature.
- **No browser auth:** CHROTE is still a high-authority localhost/private-network cockpit. Moving to `/srv` does not make it a multi-tenant web app.
- **Stale instructions:** skills and runbooks that keep saying `/home/perttu/chrote`, user `chrote.service`, or `8094` without context will cause agents to mutate or verify the wrong service. This is a cutover blocker, not nice-to-have doc polish.

## Recommendation

Do this, but do it in parallel and host-systemd-first. Do not Dockerize CHROTE yet. The clean migration is:

1. `/srv/chrote` source/build + `/srv/data/chrote` state;
2. dedicated `chrote` service account;
3. alternate ports until parity is proven;
4. deliberate tmux socket ACL/shared-socket contract;
5. shared Archon/Formations state under `/srv/data/chrote`;
6. skills/docs/runbooks updated and validated as part of the migration;
7. only then switch traffic away from the user service.

That gets the cleanup benefit without breaking the working cockpit mid-flight.
