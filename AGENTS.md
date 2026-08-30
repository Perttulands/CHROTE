# Agent instructions

Use these rules for work in CHROTE. Keep public instructions host-neutral;
deployment paths, ports, sockets, service identities, and credentials belong in
untracked operator configuration.

## Operating contract

The host workspace's `CLAUDE.md` and `AGENTS.md` one directory up name the
operator's canonical contract. Read them before non-trivial work. Follow that
pointer rather than copying deployment-specific paths or duplicating the rules.

## Product boundary

CHROTE is a private browser cockpit for host-owned work:

- tmux sessions and browser terminal windows;
- files under configured roots;
- Beads through modern `bd`;
- harness-neutral agent observability;
- scheduling, server status, and optional local adapters.

CHROTE is not a hosted service, an IDE, or an OS sandbox. Do not assume
Gastown, Ralph, or any single agent harness is installed.

Access is broad by design: everything CHROTE can reach, it shows. The only
asymmetry is Unix permissions — a secondary account cannot read the owner's
work, the owner can read the secondary's — and it is never encoded in CHROTE.
Never tighten ownership, modes, or ACLs to make code safer; CHROTE's value is
access. `SECURITY.md` owns the additive-grant and missing-access rules; follow
it instead of reshaping the permission topology.

Rare, judgment-heavy recovery, restore, cleanup, migration, and repair are
agent skills, not request-path code. CHROTE must not kill external tmux work on
disconnect or restart, but it does not recreate ordinary work after process
death or reboot. Durable workloads belong in operator-owned host configuration.
Experimental orchestration contracts live in `chrote-agent-formations`, not
this product.

## Complexity budget

Every change is measured by what it removes.

- Report the net line delta of every change.
- No new environment variable without removing one.
- No new CI step, gate script, or test-of-a-gate without removing one.
- No test file named `hardening`, `baseline`, `fence`, `guard`, or `prototype`.
- No test asserting the absence of a feature.
- No `t.Skip`. Opt-in or environment-dependent tests use a Go build tag such as `//go:build live`.
- Rollback is `git revert` plus a rebuild, never a binary swap.
- Never `git add -A` in this shared tree; commit small, scoped sets of files.

## Source truth

Read [`docs/source-truth-index.md`](docs/source-truth-index.md) before changing product or architecture claims.

- [`PRD.md`](PRD.md) owns the current product and roadmap boundary.
- [`SECURITY.md`](SECURITY.md) owns the public trust boundary.
- [chrote-agent-formations](https://github.com/Perttulands/chrote-agent-formations) owns experimental orchestration contracts.
- [`docs/legacy-ideas.md`](docs/legacy-ideas.md) is non-current context only.

### Six-view source map

| View | Dashboard | API |
| --- | --- | --- |
| Terminal | `dashboard/src/components/TerminalWorkspaceDock.tsx` | `src/internal/api/tmux_sessions.go`, `src/internal/proxy/terminal.go` |
| Files | `dashboard/src/components/FilesView/` | `src/internal/api/files.go` |
| Beads | `dashboard/src/components/BeadsView/` | `src/internal/api/beads.go` |
| Scheduled | `dashboard/src/components/ScheduledTasksView.tsx` | `src/internal/api/scheduled.go` |
| Server | `dashboard/src/components/SystemStatusView/` | `src/internal/api/system.go` |
| Settings | `dashboard/src/components/SettingsView.tsx` | `src/internal/api/tmux_sessions.go` |

## Work state

Use `bd` from this repo root so it resolves the local `.beads/` workspace. New
issues use `chrote-`; `chrt-` and `ctx-` are imported history described by the
untracked `.beads/WORKSPACE.md`. Do not file CHROTE work elsewhere.

Create discovered work as linked Beads, not prose or unrelated changes.

## Git checkpoints

Frequent local commits are part of normal Beads work. This repository rule
overrides the generated conservative fallback below; leave that block intact.

- Claim the Bead before substantive work; commit each coherent verified increment.
- Stage only the files belonging to the current Bead, and preserve unrelated dirty work.
- Pass generated/Markdown commit messages with `git commit -F`; never put
  backticks or `$()` in a double-quoted `-m` argument.
- Report the commit, Bead state, and verification together at handoff.
- A commit does not authorize push, rebase, merge, or deployment.
- Honor an explicit no-commit instruction and report the uncommitted scope.

## Before editing

- Read nearby code, callers, tests, and the active source-truth document.
- Define done and preserve unrelated dirty work.
- Do not kill, rename, or restart tmux sessions unless the task explicitly requires it.
- Do not assume a service name, port, socket, or deployment lane from tracked files. Discover the approved target from local operator configuration before runtime actions.
- Never commit private topology, credentials, terminal transcripts, or operator-specific recovery procedures.
  `scripts/host-neutrality.py` checks every tracked file. Use neutral fixtures:
  `alice`/`build`, `/run/user/<uid>/...`, `/tmp/tmux-<uid>/...`.

## Build and verify

Use the repository scripts instead of hand-copying embedded assets:

```bash
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh

cd src
GOTOOLCHAIN=go1.26.6 go vet ./...
GOTOOLCHAIN=go1.26.6 go test ./...
GOTOOLCHAIN=go1.26.6 go test -race ./...

cd ../dashboard
npm run lint
npm run test:unit -- --coverage
npm test

cd ..
python3 scripts/doc-lint.py
```

Set `CHROTE_PLAYWRIGHT_PORT` when the default Vite test port is already owned by another process. Do not kill an unrelated listener merely to run tests.

For installer changes, run both disposable modes:

```bash
./scripts/test-public-install.sh /path/to/chrote-server
./scripts/test-public-install.sh
```

Before completion, run `git diff --check`, compare `dashboard/dist` with
`src/internal/dashboard/dist`, and report warnings or skipped checks plainly.

## Runtime actions

Runtime deployment is separate from repository verification. Before any restart
or install: identify the operator-approved service and endpoint, inspect current
health, preserve tmux and unrelated work, touch only that service, then verify
the live endpoint. Ask if untracked operator context does not identify it.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:970c3bf2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for shared persistent knowledge. Harness-private memory files may exist, but anything another agent needs must land in `bd remember` or a bead — never only in a tool-private file

**Architecture in one line:** issues live in this workspace's local embedded Dolt DB under `.beads/`; the public source repository intentionally ignores that directory. Owner/maintainer checkouts replicate native Dolt history to a separately authorized private Git sidecar and pair portable exports with source revisions there. Never push tracker data or `refs/dolt/data` to the public source origin; verify the Beads remote before `bd dolt push`. See [`docs/private-beads-sidecar.md`](docs/private-beads-sidecar.md).

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->
