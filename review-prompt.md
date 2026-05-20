You are a senior Go engineer reviewing a backend feature addition to CHROTE (a tmux session dashboard). Please review the implementation on branch feature/agent-teams and provide constructive feedback.

## What was built

A minimal agent team/harness orchestration backend with NO UI changes:

- `internal/teams/models.go` — Harness templates, Team state, FlowStep rules
- `internal/teams/store.go` — File-based persistence in `.chrote/{harnesses,teams}/`
- `internal/teams/engine.go` — State machine that polls tmux and executes harness flows
- `internal/api/teams.go` — REST endpoints for `/api/harnesses` and `/api/teams`
- Wired into `cmd/server/main.go` and existing `tmux.go`

## Design principles
- Harness = collaboration pattern (e.g. builder+verifier loop)
- Team = live instance of a harness with tmux sessions as members
- Flow actions: spawn, nudge (tmux send-keys), mail (jsonl file), status
- Conditions: feedback.exists, success, failure, with negation (`!`)
- Built-in harnesses: verifier-loop, pipeline, pair-programming

## Review focus
1. **Correctness** — race conditions, error handling, resource leaks
2. **Simplicity** — is this minimal enough? What can be removed?
3. **Safety** — filesystem path validation, tmux command injection risks
4. **Extensibility** — how easy to add new actions/triggers?
5. **Integration** — does it play nicely with existing CHROTE patterns?

Please read the changed files and give specific line-level feedback where appropriate.
