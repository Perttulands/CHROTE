# Agent instructions

Use these rules for work in the CHROTE repository. Keep public instructions host-neutral; deployment-specific paths, ports, sockets, service identities, and credentials belong in untracked operator configuration.

## Operating contract

This repository sits inside a host workspace whose root instruction files (`CLAUDE.md` and `AGENTS.md` one directory up) name the operator's canonical operating contract. Read it before non-trivial work; it is the authority these repository rules sit under, not a duplicate of them. In short: define done before changing things, read before writing, keep changes small, tests must prove intent, verify before claiming done, fail loud.

The contract's own location is deployment-specific, so it is deliberately not spelled out here — naming it would put an operator path in a public file, which the host-neutrality rule above and `scripts/host-neutrality.py` both forbid. Follow the pointer up rather than copying the rules down: restating them here is how two contracts drift apart.

## Product boundary

CHROTE is a private browser cockpit for host-owned work:

- durable tmux sessions and browser terminal windows;
- files under configured roots;
- Beads through modern `bd`;
- harness-neutral agent observability;
- recovery, scheduling, server status, and optional local adapters.

CHROTE is not a hosted service, an IDE, or an OS sandbox. Do not assume Gastown, Ralph, or any single agent harness is installed.

Formations and Archon are experimental and unreleased. Their active specs define development contracts, not a supported release promise.

## Source truth

Read [`docs/source-truth-index.md`](docs/source-truth-index.md) before changing product or architecture claims.

- [`PRD.md`](PRD.md) owns the current product and roadmap boundary.
- [`SECURITY.md`](SECURITY.md) owns the public trust boundary.
- [`FORMATIONS.md`](FORMATIONS.md), [`ARCHON.md`](ARCHON.md), and [`DATA-MODEL.md`](DATA-MODEL.md) own experimental orchestration contracts.
- Plans and archives are context, not current authority.

## Work state

Use `bd` for durable task state in this repository's own `.beads/` workspace —
run `bd` from the repo root so it resolves there. New issues get the `chrote-`
prefix. `chrt-` and `ctx-` ids are imported history living in this same
database; `.beads/WORKSPACE.md` (workspace-local, untracked) owns the scope,
the import story, and what does not belong here. Do not file CHROTE work in any
other workspace:

```bash
bd prime
bd ready --json
bd show <id>
bd update <id> --claim --json
```

Create discovered work as linked Beads rather than burying it in prose or unrelated changes.

## Git checkpoints

This project uses frequent local Git commits as part of normal Beads-backed
work. This project-specific rule is more specific than the generated Beads
profile's conservative Git fallback below; leave the generated block intact.

- Create and claim the relevant Bead before substantive implementation, then
  commit each coherent, verified increment before moving to an independent
  concern.
- Keep commits small and single-purpose. Stage only the files belonging to the
  current Bead, and preserve unrelated dirty work.
- For generated, multiline, or Markdown commit messages, pass message bytes
  with `git commit -F <file>` or `git commit -F - <<'EOF'`; never interpolate
  message text into a double-quoted `-m` argument, where backticks and `$()`
  execute in the shell.
- Do not leave substantive completed work only in the working tree at handoff.
  Report the commit hash, Bead id/status, and verification receipt together.
- A local commit does not authorize a push, rebase, merge, or deployment. Those
  remain separate actions requiring explicit authority.
- If the user explicitly says not to commit yet, honor that exception and
  report the exact uncommitted scope at handoff.

## Before editing

- Read nearby code, callers, tests, and the active source-truth document.
- Define done before changing files.
- Preserve unrelated dirty work.
- Do not kill, rename, or restart tmux sessions unless the task explicitly requires it.
- Do not assume a service name, port, socket, or deployment lane from tracked files. Discover the approved target from local operator configuration before runtime actions.
- Never commit private topology, credentials, terminal transcripts, or operator-specific recovery procedures.
  `python3 scripts/host-neutrality.py` enforces this over every tracked file and runs in CI. It fails on real
  usernames, home directories, uid-scoped socket paths, tailnet or host names, and host-only unit names.
  Use neutral fixtures instead: `alice`/`build`, `/run/user/<uid>/...`, `/tmp/tmux-<uid>/...`.

## Build and verify

Use the repository scripts instead of hand-copying embedded assets:

```bash
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh

cd src
GOTOOLCHAIN=go1.26.5 go vet ./...
GOTOOLCHAIN=go1.26.5 go test ./...
GOTOOLCHAIN=go1.26.5 go test -race ./...

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

Before completion, run `git diff --check`, compare `dashboard/dist` with `src/internal/dashboard/dist`, inspect every touched repository, and report warnings or skipped checks plainly.

## Runtime actions

Runtime deployment is separate from repository verification. Before restarting or installing anything:

1. identify the exact operator-approved service and endpoint;
2. inspect current status and health;
3. preserve tmux sessions and unrelated work;
4. restart only the intended service;
5. verify the live endpoint after activation.

If the runtime target is not discoverable from local, untracked operator context, ask instead of guessing.

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
