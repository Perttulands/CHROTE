# CHROTE Test Strategy

Tests protect operator journeys and API contracts. Value checks belong at the
lowest useful layer; browser tests prove workflows rather than rechecking every
rendered value.

## Test matrix

| Layer | Owns | Command |
| --- | --- | --- |
| Go | API shapes, persistence, tmux/filesystem behavior, concurrency | `cd src && go test -race ./...` |
| Vitest | Components, state transitions, localStorage, formatting, error states | `cd dashboard && npm run test:unit` |
| Mocked Playwright | 57 core operator journeys at retries 0 | `cd dashboard && npm test` |
| Built-server contract | Embedded assets plus terminal and Files API/browser seam | `./scripts/test-built-server-contract.sh` |
| Live Playwright | Five operator-approved real-backend/tmux smokes | `cd dashboard && npm run test:live` |
| Source contracts | Docs, host neutrality, and embedded parity | `.github/workflows/ci.yml` |

The mocked suite owns stable browser journeys. Live tests are opt-in because they
touch an actual server and tmux substrate. Set `CHROTE_TEST_URL` to the approved
target; do not infer a deployed port from public defaults.

## Feature ownership

| Product job | Primary Go/API owner | Dashboard owner |
| --- | --- | --- |
| Terminal | `internal/api/tmux_*`, `internal/proxy` | session/context unit tests and terminal Playwright journeys |
| Files | `internal/api/files*` | `FilesView` unit tests and file browser journeys |
| Beads | `internal/api/beads*` | `BeadsView` unit tests and the core-views journey |
| Scheduled | `internal/api/scheduled*`, `internal/scheduled` | `ScheduledTasksView` unit tests and the core-views journey |
| Server | `internal/api/system*`, `health*` | `SystemStatusView` unit tests and the core-views journey |
| Settings | tmux appearance/mouse/session APIs | `SettingsView`, workspace-layout units, and settings journeys |

Optional Services follows the same rule: API adapter tests, view unit tests, and
one mocked primary-action journey.

## Single CI quality job

The five-minute job performs, in order:

1. install dashboard dependencies and Chromium;
2. build the embedded dashboard and server;
3. run Go format and vet;
4. run Go tests once with race detection;
5. run Vitest, ESLint, and mocked Playwright;
6. run doc-lint, host-neutrality, and embedded parity;
7. run the built-server contract against the built artifact.

`govulncheck` and `npm audit` run only on the weekly scheduled invocation.

## Rules

- No `t.Skip`; environment-dependent Go tests use `//go:build live`.
- Keep Playwright retries at zero and do not hide failures with timeout growth.
- Do not add tests that only assert a retired feature is absent.
- Do not add tests of gate scripts or files named `hardening`, `baseline`,
  `fence`, `guard`, or `prototype`.
- A behavior change needs a test that fails when the operator contract regresses.
- Use an alternate `CHROTE_PLAYWRIGHT_PORT` when another process owns the Vite
  test port; never kill an unrelated listener.
