# CHROTE Runtime Deploy Model

Durable ownership and deploy model for the CHROTE operator cockpit. This is the
authoritative reference for *where source lives*, *where the live service runs*,
*how to get committed code into the runtime safely*, and *what must never be
touched*. It satisfies Beads `home-gia` (epic), `home-ksz` (ownership), and
`home-j7x` (sync exclusions).

> Golden rule (inherited from `CLAUDE.md`): never disrupt running shells or tmux
> sessions. A broken deploy must be rolled back, never left half-applied.

## 1. Trees and their roles

| Tree | Role | Git | Touch policy |
| --- | --- | --- | --- |
| `/home/perttu/chrote-3.0-gascity` | **Canonical source for the CHROTE 3.0 Gas City work.** Build artifacts here. Worktree on branch `chrote-3.0-gascity`. | Worktree of `/home/perttu/chrote/.git` (shared object store) | Edit source here. Never run a branch-switch/reset that would disturb the shared store. |
| `/home/perttu/chrote` | **Runtime deploy tree** used by `chrote.service`. Branch `feature/chrote-ui-review-batch`. `WorkingDirectory=%h/chrote`. Holds the live `chrote-server` binary. | Real repo; `.git` object store is shared by the worktree above | Do **not** hand-edit source here. Update only via the deploy script (artifacts). No `git pull`/`reset`/`checkout` here — see Section 5. |
| `/home/perttu/repos/CHROTE-public` | **Public release clone.** Clean public GitHub mirror (branch e.g. `docs/readme-service-labels`). Does **not** contain the CHROTE 3.0 Gas City commits. | Independent clone | Source of public releases only. Not the source for *this* runtime deploy. |

### Why two trees that share a `.git`
`/home/perttu/chrote-3.0-gascity` is a `git worktree` of `/home/perttu/chrote`
(`.git` -> `/home/perttu/chrote/.git/worktrees/chrote-3.0-gascity`). They share
one object store but check out different branches. This is why git operations on
the runtime tree are risky (Section 5) and why we deploy **built artifacts**, not
git state.

### Canonical source disambiguation
The older `home-gia` text named `/home/perttu/repos/CHROTE-public` as canonical.
For the CHROTE 3.0 Gas City release the canonical source is
`/home/perttu/chrote-3.0-gascity @ chrote-3.0-gascity` (commit `ee585b1` at first
deploy), because the transcript-recovery code (`src/internal/api/gascity*.go`,
`dashboard/`) and the harness-adapter docs live only on that branch.
`CHROTE-public` remains the public *release* source and should be reconciled
separately (it does not yet carry these commits).

## 2. Service facts (verified from the unit and live process)

```text
unit:        ~/.config/systemd/user/chrote.service  (systemctl --user)
WorkingDirectory: %h/chrote  (= /home/perttu/chrote)
ExecStart:   %h/chrote/chrote-server --host 127.0.0.1 --port 8094 --ttyd-port 7683
HTTP health: http://127.0.0.1:8094/api/health  -> {"status":"ok",...}
ttyd:        127.0.0.1:7683
tmux socket: TMUX_TMPDIR=%t/chrote-tmux  => /run/user/1000/chrote-tmux
             (tmux socket file: /run/user/1000/chrote-tmux/tmux-1000/default)
private cfg: EnvironmentFile=-%h/.config/chrote/services.env   (mode 0600, never print/commit)
restart:     systemctl --user restart chrote.service
```

## 3. Build chain (authoritative, from `CLAUDE.md`)

The Go module root is `src/` (`module github.com/chrote/server`, `go 1.23`).
The server embeds the dashboard via `//go:embed dist/*` in
`src/internal/dashboard/embed.go`, so the built dashboard must be copied into
`src/internal/dashboard/dist` **before** the Go build.

```bash
cd <canonical>/src      && go test ./...
cd <canonical>/dashboard && npm run build            # -> dashboard/dist
rm -rf <canonical>/src/internal/dashboard/dist
cp -r  <canonical>/dashboard/dist <canonical>/src/internal/dashboard/dist
cd <canonical>/src      && go build -o ../chrote-server ./cmd/server
# deploy: copy built chrote-server -> /home/perttu/chrote/chrote-server
systemctl --user restart chrote.service
```

Both `chrote-server` and `src/internal/dashboard/dist/` are **gitignored build
artifacts** (see `.gitignore`). Git operations never update them; the binary
must be rebuilt and copied for any code change to go live.

### Build provenance (home-altx)

The canonical tree is a git **worktree** whose `.git` is a file, so Go's
automatic VCS stamp walks up to the OUTER `/home/perttu` repo and records the
wrong commit (`vcs.revision` of the home-repo HEAD, `vcs.modified=true`). The
deploy therefore builds with `-buildvcs=false` and stamps the real CHROTE
source commit explicitly:

```bash
go build -buildvcs=false \
  -ldflags "-X main.Version=<ver> -X main.Commit=$(git rev-parse --short HEAD)" \
  -o <staging> ./cmd/server
```

The stamped commit is observable at `GET /api/version` (`{"version","commit"}`)
and in the startup log line, and is recorded in the deploy state receipt as
`source_commit` / `binary_provenance`.

### Gas City tmux version dependency (home-5ubb)

The transcript route shells to `gc session peek`, and `gc` shells to `tmux` via
PATH. The **root cause** of the deployed transcript 502 was a tmux **binary
version mismatch**: the chrote.service minimal PATH resolved `tmux` to
`/usr/bin/tmux` (3.4), but the Gas City supervisor created its `-L gascity`
server with the Linuxbrew `tmux` (3.6a). tmux 3.4 cannot read a 3.6a server
("server exited unexpectedly"), so `gc session peek` failed and the handler
returned `502 GASCITY_TRANSCRIPT_UNAVAILABLE`. It worked from an interactive
shell only because that shell had Linuxbrew on PATH.

Fix: the server prepends a compatible-tmux bin dir to PATH **for gc
subprocesses only** (see `resolveGasCityGCExtraPath`/`gasCityChildEnv` in
`gascity.go`), without changing CHROTE's own `/usr/bin/tmux` terminal-proxy
sessions (`tmux.go`). Override with `CHROTE_GASCITY_GC_PATH=<bin-dir>` (or `off`)
if the supervisor's tmux moves. This is a deliberate, documented version
dependency on the supervisor's tmux build.

## 4. Deploy workflow

Use `scripts/deploy-local.sh` (in the canonical tree). It:

1. Verifies it is running from the canonical worktree and the worktree is clean
   (refuses a dirty source tree unless `--allow-dirty`).
2. Runs `go test ./...` (skippable with `--skip-tests`, not recommended).
3. Builds the dashboard (`npm run build`) and refreshes
   `src/internal/dashboard/dist` from `dashboard/dist`.
4. Builds `chrote-server` into a staging path (never overwrites the runtime
   binary until the build fully succeeds).
5. **Backs up the current runtime binary** to a timestamped file and records
   rollback state (runtime git HEAD, service status, tmux session list).
6. Atomically swaps the new binary into `/home/perttu/chrote/chrote-server`.
7. Restarts **only** `chrote.service` (`systemctl --user`).
8. Smokes `/api/health` and verifies the chrote tmux session list is unchanged.

`--dry-run` performs build + checks and prints the planned actions without
touching the runtime binary or restarting the service.

The deploy **never** touches the tmux socket, never runs `tmux kill-*`, never
removes `/run/user/1000/chrote-tmux`, never edits the runtime git tree, and never
copies private config into the repo.

## 5. Why we do NOT use a git operation to deploy

A `git pull --ff-only` in the runtime tree would *appear* clean (runtime HEAD is
an ancestor of the canonical commit), but:

- It updates tracked source files only; the **binary and embedded dist are
  gitignored** and would stay stale, so the service would not actually change.
- The runtime tree shares its object store with the canonical worktree; branch
  ref churn there is unnecessary risk for zero runtime benefit.

Therefore: **build artifacts, swap the binary.** No runtime git op is part of the
deploy. If a future operator wants the runtime branch to also track the new
commits for bookkeeping, do that as a separate, reviewed step — it is not
required for the code to be live.

Destructive git (force-reset of runtime, branch deletion, anything that could
lose uncommitted runtime state) is out of scope: stop and report instead.

## 6. Runtime-only / generated artifacts (sync exclusions) — `home-j7x`

Because we deploy a single built binary (not an rsync of the source tree), the
classic rsync-exclusion risk is largely avoided. The classification below
documents what is runtime-only and what must never be synced or deleted, and it
governs any future tree-sync variant of the deploy.

| Path | Class | Notes |
| --- | --- | --- |
| `/home/perttu/chrote/chrote-server` | **deploy target** | The one artifact the deploy replaces (with backup). |
| `src/internal/dashboard/dist/`, `dashboard/dist/`, `dashboard/.vite/` | generated | Rebuilt each deploy; gitignored. Never source-sync. |
| `dashboard/node_modules/`, `*/node_modules/` | generated | Rebuild from lockfile; never sync. |
| `dashboard/coverage/`, `dashboard/playwright-report/`, `**/test-results/` | generated | Test output; never sync. |
| `/home/perttu/.config/chrote/services.env` | **private config** | Loaded by the unit. Never print, sync, or commit. Lives outside both trees. |
| `/home/perttu/.chrote`, `/home/perttu/.config/chrote` | private state/config | Operator runtime state; outside source sync. |
| `~/.local/state/chrote/gascity-transcripts/` (or `$XDG_STATE_HOME/chrote/...`) | runtime state | Transcript-recovery archive written by the live server. Operator data; never sync/delete on deploy. |
| `*.sqlite`, `*.db`, `*.db-wal`, `*.db-shm`, `.beads/`, `.dolt/` | runtime state | Beads/DB state; gitignored; never source-sync. |
| `.env`, `.env.*`, `tailscale_state/`, `filebrowser_data/` | secrets/state | Never sync or commit. |
| Large repo media (`*.png` at repo root, `bg_*.png`, `*.mp3`) | tracked assets | Embedded via dist where relevant; not part of binary swap. |

No runtime files are deleted by the deploy. Any cleanup of stale runtime
artifacts must be a separate, reviewed step (per `home-j7x`).

## 7. Verification / smoke — `scripts/smoke.sh`

`scripts/smoke.sh` is read-only and proves a deploy is healthy:

- (a) `chrote.service` is `active`.
- (b) `GET http://127.0.0.1:8094/api/health` returns `status: ok`.
- (c) Every chrote tmux session present in a recorded "before" list is still
  present after (no session lost). It reads
  `TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux list-sessions`.
- (d) Transcript-recovery is live: `GET /api/gascity/sessions/<id>/transcript`
  returns the JSON API envelope (`Content-Type: application/json`,
  `{"success":...}`), not the SPA HTML fallback. On the old binary this route
  fell through to `index.html` (HTML); on the new binary it is a real API route
  (`gascity.go` -> `GET /api/gascity/sessions/{id}/transcript`). The archive /
  stale-recovery branch is exercised read-only by requesting a session id the
  supervisor will not resolve — it still returns the JSON envelope.

## 8. Hard constraints

- Never touch Gas City: `gascity-supervisor.service`, `/home/perttu/gascity`,
  the `tmux -L gascity` socket, or its sessions (`planner`, `reviewer-*`,
  `s-gc-*`, `*-smoke`, ...). NB: a CHROTE session named `gascity-considering`
  lives on the **CHROTE** socket and is a CHROTE session to preserve — it is not
  the Gas City substrate.
- Never kill the chrote tmux socket/sessions; preserve
  `/run/user/1000/chrote-tmux`.
- Never print or commit secrets; never change public repo history; never push.
- No destructive git without stopping to report.
