# Agent instructions

Use these rules for work in the CHROTE repository. Keep public instructions host-neutral; deployment-specific paths, ports, sockets, service identities, and credentials belong in untracked operator configuration.

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

Use `bd` for durable task state in the owning workspace:

```bash
bd prime
bd ready --json
bd show <id>
bd update <id> --claim --json
```

Create discovered work as linked Beads rather than burying it in prose or unrelated changes.

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
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

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
   bd dolt push
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->
