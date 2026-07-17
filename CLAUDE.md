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

- Use modern `bd` in the owning workspace for durable work state.
- Use `bv` only as an optional graph-aware Beads viewer, never as the issue source of truth.
- Preserve running shells, tmux sessions, and unrelated dirty work.
- Keep listeners loopback-only unless the user explicitly approves a broader binding.
- Keep Formations and Archon labeled experimental and unreleased until an explicit release decision changes that boundary.
- Keep machine-specific deployment commands in local operator documentation, not this tracked file.
