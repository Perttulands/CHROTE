# CHROTE Runtime Deploy Model

Durable ownership and deploy model for the CHROTE operator cockpit. This is the
authoritative reference for where CHROTE source lives, where the live service
runs, how committed code becomes the runtime binary, and what must never be
touched. It supersedes the earlier two-checkout/worktree model and satisfies the
local consolidation target tracked by `home-x2ri`.

> Golden rule: never disrupt running shells or tmux sessions. A broken deploy
> must be rolled back, never left half-applied.

## 1. Trees and Their Roles

| Tree | Role | Touch policy |
| --- | --- | --- |
| `/home/perttu/chrote` | **Canonical CHROTE source checkout and runtime deploy target.** `chrote.service` runs from this tree and executes `/home/perttu/chrote/chrote-server`. | Edit, build, and deploy CHROTE here. Git operations here are source-sync operations, not a substitute for building the runtime binary. Avoid destructive git unless explicitly approved. |
| `/home/perttu/chrote-3.0-gascity` | Temporary redundant transition clone/worktree from the old Gas City branch workflow. | Do not treat as canonical and do not deploy from it. Remove only after clean/remote-backed verification and explicit cleanup approval. |
| `/home/perttu/repos/CHROTE-public` | Temporary redundant public clone/mirror from the old release workflow. | Not the runtime source for local CHROTE. Remove after verification, or recreate later only as a deliberate release mirror. |

### Single-Checkout Rule

`/home/perttu/chrote` is both the source checkout and the live runtime directory.
The served endpoint remains `127.0.0.1:8094`; consolidation must not move the
browser/API endpoint or create a second served CHROTE instance.

The old model used `/home/perttu/chrote-3.0-gascity` as the source worktree and
`/home/perttu/chrote` as an artifact-only runtime tree. That model is obsolete.
To deploy the Gas City branch locally, first make `/home/perttu/chrote` contain
the intended branch/commit, then build and install the binary from that same
checkout.

The redundant clones are cleanup candidates only. Deleting them is a separate
reviewed cleanup step after verification; it is not part of normal deploy.

## 2. Service Facts

Expected unit facts from `services/chrote.service`:

```text
unit:        ~/.config/systemd/user/chrote.service  (systemctl --user)
WorkingDirectory: %h/chrote  (= /home/perttu/chrote)
ExecStart:   %h/chrote/chrote-server --host 127.0.0.1 --port 8094 --ttyd-port 7683
HTTP health: http://127.0.0.1:8094/api/health
ttyd:        127.0.0.1:7683
tmux socket: TMUX_TMPDIR=%t/chrote-tmux  => /run/user/1000/chrote-tmux
Gas City:    CHROTE_GASCITY_CITY_DIR=%h/gascity
private cfg: EnvironmentFile=-%h/.config/chrote/services.env   (mode 0600, never print/commit)
restart:     systemctl --user restart chrote.service
```

The source-controlled unit should keep `WorkingDirectory`, `ExecStart`,
`CHROTE_LAUNCH_SCRIPT`, and all CHROTE paths wired to `%h/chrote`. It must also
set `CHROTE_GASCITY_CITY_DIR=%h/gascity` so CHROTE points at the existing Gas
City city rather than inferring a path from the checkout.

## 3. Build Chain

The Go module root is `src/` (`module github.com/chrote/server`, `go 1.23`).
The server embeds the dashboard via `//go:embed dist/*` in
`src/internal/dashboard/embed.go`, so the built dashboard must be copied into
`src/internal/dashboard/dist` before the Go build.

```bash
cd /home/perttu/chrote/src       && go test ./...
cd /home/perttu/chrote/dashboard && npm run build
rm -rf /home/perttu/chrote/src/internal/dashboard/dist
cp -r  /home/perttu/chrote/dashboard/dist /home/perttu/chrote/src/internal/dashboard/dist
cd /home/perttu/chrote/src       && go build -o ../chrote-server ./cmd/server
```

`chrote-server`, `dashboard/dist/`, and `src/internal/dashboard/dist/` are
generated artifacts. Git state alone does not update the served app; the binary
must be rebuilt after source changes.

### Build Provenance

The single-checkout model removes the old worktree provenance problem where Go
could stamp the outer `/home/perttu` repository. The deploy should still stamp
or verify the intended CHROTE source commit explicitly, because `GET
/api/version` is the operator-visible proof of what binary is live.

Preferred explicit stamp:

```bash
go build -buildvcs=false \
  -ldflags "-X main.Version=<ver> -X main.Commit=$(git rev-parse --short HEAD)" \
  -o ../chrote-server ./cmd/server
```

The stamped commit is observable at `GET /api/version` (`{"version","commit"}`)
and should match the intended `/home/perttu/chrote` checkout commit.

### Gas City Tmux Version Dependency

The transcript route shells to `gc session peek`, and `gc` shells to `tmux` via
PATH. A prior deployed transcript 502 was caused by a tmux binary version
mismatch: `chrote.service` resolved `tmux` to `/usr/bin/tmux` (3.4), while the
Gas City supervisor used the Linuxbrew `tmux` (3.6a). tmux 3.4 cannot read a
3.6a server, so `gc session peek` failed.

The server prepends a compatible tmux bin dir to PATH for `gc` subprocesses only
(see `resolveGasCityGCExtraPath` / `gasCityChildEnv` in `gascity.go`), without
changing CHROTE's own terminal-proxy tmux sessions. Override with
`CHROTE_GASCITY_GC_PATH=<bin-dir>` or `CHROTE_GASCITY_GC_PATH=off` if the
supervisor's tmux moves.

## 4. Deploy Workflow

`scripts/deploy-local.sh` is the intended local deploy entrypoint. In the
single-checkout model it must operate from `/home/perttu/chrote`, not from the
redundant transition clone.

The deploy workflow:

1. Verifies `/home/perttu/chrote` is the canonical checkout, on the intended
   branch/commit, and clean unless an explicit dirty deploy is approved.
2. Runs `go test ./...` under `src/`.
3. Builds the dashboard with `npm run build` under `dashboard/`.
4. Refreshes `src/internal/dashboard/dist` from `dashboard/dist`.
5. Builds `chrote-server` into a staging path before touching the live binary.
6. Captures enough temporary rollback state to recover if the deploy fails.
7. Atomically installs the staged binary as `/home/perttu/chrote/chrote-server`.
8. Restarts only `chrote.service`.
9. Smokes health, version, and tmux-session preservation.
10. On success, removes backup/state/temp files created for that successful
    deploy. Persistent `.deploy-backups` dirt, `*.bak`, or `*.new.*` files
    should not remain after a successful deploy.

`--dry-run` should perform build and checks without touching the runtime binary,
writing rollback backups, or restarting the service.

The deploy must never touch the tmux socket, run `tmux kill-*`, remove
`/run/user/1000/chrote-tmux`, copy private config into the repo, or delete the
redundant clone directories. Failed deploys may keep rollback evidence until the
operator resolves the failure; successful deploys should leave no persistent
backup dirt.

## 5. Why Git Alone Is Not Deploy

A `git pull --ff-only` or branch checkout in `/home/perttu/chrote` updates source
files only. It does not rebuild the dashboard, refresh embedded assets, rebuild
`chrote-server`, or prove the live service has restarted with the intended
binary.

Therefore:

- Use Git in `/home/perttu/chrote` only to move the canonical source checkout to
  the intended commit.
- Use the build/deploy workflow to update the served binary.
- Verify the served binary through `GET /api/version` and health/smoke checks.

Destructive git operations that could lose uncommitted source or runtime state
remain out of scope: stop and report instead.

## 6. Runtime-Only / Generated Artifacts

Because source and runtime now share one checkout, the important distinction is
between tracked source, generated build output, private config, and operator
runtime state.

| Path | Class | Notes |
| --- | --- | --- |
| `/home/perttu/chrote/chrote-server` | generated deploy artifact | Runtime server binary; replace atomically after a successful build. |
| `/home/perttu/chrote/terminal-launch.sh` | tracked source and live launch script | `chrote.service` / ttyd reads this path directly. Keep it in the canonical checkout. |
| `src/internal/dashboard/dist/`, `dashboard/dist/`, `dashboard/.vite/` | generated | Rebuilt each deploy; gitignored. |
| `dashboard/node_modules/`, `*/node_modules/` | generated | Rebuild from lockfile; never treat as source. |
| `dashboard/coverage/`, `dashboard/playwright-report/`, `**/test-results/` | generated | Test output. |
| `/home/perttu/chrote/.deploy-backups/`, `*.bak`, `*.new.*` | temporary deploy state | Allowed during deploy or after failure. Must not persist after successful deploy verification. |
| `/home/perttu/.config/chrote/services.env` | private config | Loaded by the unit. Never print, sync, or commit. |
| `/home/perttu/.chrote`, `/home/perttu/.config/chrote` | private state/config | Operator runtime state outside source control. |
| `~/.local/state/chrote/gascity-transcripts/` (or `$XDG_STATE_HOME/chrote/...`) | runtime state | Transcript-recovery archive written by the live server. Operator data; never sync/delete on deploy. |
| `*.sqlite`, `*.db`, `*.db-wal`, `*.db-shm`, `.beads/`, `.dolt/` | runtime state | Beads/DB state; gitignored; never source-sync. |
| `.env`, `.env.*`, `tailscale_state/`, `filebrowser_data/` | secrets/state | Never sync or commit. |

Normal deploy should not delete operator runtime state. The only cleanup it
should perform automatically is successful-deploy cleanup of its own temporary
backup/state files. Removing `/home/perttu/chrote-3.0-gascity` and
`/home/perttu/repos/CHROTE-public` is a separate consolidation cleanup after
verification.

## 7. Verification / Smoke

Read-only smoke checks prove the deployed service is healthy:

- `chrote.service` is `active`.
- `GET http://127.0.0.1:8094/api/health` returns `status: ok`.
- `GET http://127.0.0.1:8094/api/version` reports the intended commit.
- Every chrote tmux session present in a recorded "before" list is still present
  after deploy.
- Gas City API routes return JSON API envelopes, not the SPA HTML fallback.

Safe doc/service checks for this model include `systemd-analyze verify --user
services/chrote.service`, `git diff --check`, and targeted `rg` scans for stale
two-checkout language. They do not require restarting the service or deleting
directories.

## 8. Hard Constraints

- `/home/perttu/chrote` remains the single canonical source/runtime checkout.
- The served endpoint remains `127.0.0.1:8094`.
- Do not delete `/home/perttu/chrote-3.0-gascity` or
  `/home/perttu/repos/CHROTE-public` until cleanup is explicitly approved after
  clean/remote-backed verification.
- Never touch Gas City runtime ownership: `gascity-supervisor.service`,
  `/home/perttu/gascity`, the `tmux -L gascity` socket, or its sessions.
- Never kill the chrote tmux socket/sessions; preserve
  `/run/user/1000/chrote-tmux`.
- Never print or commit secrets; never change public repo history; never push
  without explicit approval.
- No destructive git without stopping to report.
