# CLAUDE.md

Follow [`AGENTS.md`](AGENTS.md). It contains the repository-wide product boundary, source-truth map, Beads workflow, verification commands, privacy rules, and runtime-action gate.

## Project map

```text
src/cmd/server/main.go          server entry point, middleware, routes
src/internal/api/               HTTP APIs for tmux, files, Beads, recovery, and status
src/internal/proxy/terminal.go  ttyd lifecycle and terminal proxy
dashboard/src/                  React dashboard
scripts/doc-lint.py             public documentation and release-truth contract
scripts/test-public-install.sh  disposable installer smoke
```

The `/api/oracle/*` route names are compatibility names. Product UI calls this surface **Agents**.

## Working rules

- Use modern `bd` in this repository's own `.beads/` workspace for durable work state (`chrote-` prefix; `chrt-`/`ctx-` ids are imported history — `.beads/WORKSPACE.md` owns the scope and import story).
- Use `bv` only as an optional graph-aware Beads viewer, never as the issue source of truth.
- Preserve running shells, tmux sessions, and unrelated dirty work.
- Keep listeners loopback-only unless the user explicitly approves a broader binding.
- Keep Formations and Archon labeled experimental and unreleased until an explicit release decision changes that boundary.
- Keep machine-specific deployment commands in local operator documentation, not this tracked file.


<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:6cd5cc61 -->
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

**Architecture in one line:** issues live in this workspace's local embedded Dolt DB under `.beads/`; no git-remote Dolt sync is configured for this repository and there is no `.beads/issues.jsonl` export — the local database is the only copy, so treat it as primary data, not derived state.

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
