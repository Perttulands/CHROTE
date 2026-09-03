# CHROTE Test Strategy

## The rule

A test earns its place if it would catch a regression the operator would notice,
or a contract another component relies on.

- Tests of absence are deleted once the removal has shipped.
- Tests that assert CSS text, class names or markup shape without behaviour go.
- Playwright covers journeys, not widgets; a widget is a unit test.
- One test per behaviour; duplicates across unit and Playwright keep the cheaper one.
- A gate that takes longer than the work it protects is restructured or moved to CI only.

Every clause has a reason. Absence tests pin the past: once a removal has shipped
they can only fail if someone brings the thing back, which is not a regression the
operator would suffer. Class names and stylesheet text are not behaviour; the
operator cannot see them, and asserting them makes every rename a test failure.
Browser tests cost roughly a hundred times what a unit test costs per case, so a
widget checked in a browser is paying for a fixture it does not need. And two
tests for one behaviour means every change to that behaviour costs two edits, so
the cheaper layer keeps it.

The exception that matters: a test that looks like any of the above but is the
only available proxy for something real. Stacking order standing in for
hit-testing, a background image that IS the deterministic slot behaviour, a class
that lifts pointer events off the terminal. When a test looks stupid, say what it
protects in a comment above it, or delete it.

## What the suites cost

Measured on a 16-core host, September 2026, after the rationalization.

| suite | files | cases | wall |
| --- | --- | --- | --- |
| Go, race enabled | 29 | 198 test functions | about 3s |
| Dashboard unit | 63 | 606 | about 5.5s |
| Mocked Playwright | 20 | 33 | about 20s at four workers |

The numbers that matter about that table are the ratios. A browser case
costs roughly two seconds; a unit case costs under ten milliseconds. That is
the whole argument for the rule about journeys and widgets, and it is why
deleting a third of the unit suite saved no measurable time while cutting
the browser suite saved most of a CI run.

## Test matrix

| Layer | Owns | Command |
| --- | --- | --- |
| Go | API shapes, persistence, tmux/filesystem behaviour, concurrency | `cd src && go test -race ./...` |
| Vitest | Components, state transitions, localStorage, formatting, error states | `cd dashboard && npm run test:unit` |
| Mocked Playwright | Operator journeys through a real browser, at retries 0 | `cd dashboard && npm test` |
| Built-server contract | Embedded assets, served fonts, terminal and Files API/browser seam | `./scripts/test-built-server-contract.sh` |
| Live Playwright | Operator-approved real-backend/tmux smokes | `cd dashboard && npm run test:live` |
| Source contracts | Docs, host neutrality, embedded parity | `scripts/doc-lint.py`, `scripts/host-neutrality.py`, `scripts/check-embedded-dashboard.py` |
| Installed product | Installer routes, environment contract, unit | `./scripts/test-public-install.sh <binary>` |
| Service restart | A restart of the running service preserves live tmux sessions | `./scripts/test-systemd-restart-preserves-tmux.sh <binary>` (operator-run) |

The mocked suite owns stable browser journeys. Live tests are opt-in because they
touch an actual server and tmux substrate. Set `CHROTE_TEST_URL` to the approved
target; do not infer a deployed port from public defaults.

What belongs in a browser test is browser-only behaviour: real font metrics
deciding the terminal grid, link hit-testing on real cell geometry, granted
clipboard permission and the plain-HTTP fallback, real socket close codes and
redial on visibility change, real pointer drags through the drag library onto real
tile geometry, container queries measured by bounding boxes, focus surviving
mount, and key routing between the document listener and the terminal's hidden
textarea. Everything else is cheaper one layer down.

### The gate that is not in CI

`scripts/test-systemd-restart-preserves-tmux.sh` defends the golden invariant that
a service restart never disrupts a live tmux session. It is operator-run on the
deployment host, not wired into CI, because a hosted runner has no CHROTE unit and
no live sessions: it could only restart a synthetic unit holding a synthetic
session, and a green result would claim coverage of the operator's real restart
that it does not have. Run it on the approved local target before and after any
change to the service unit, the installer, session ownership, or process teardown.

## Feature ownership

| Product job | Primary Go/API owner | Dashboard owner |
| --- | --- | --- |
| Terminal | `internal/api/tmux_*`, `internal/proxy` | session/context unit tests and terminal Playwright journeys |
| Files | `internal/api/files*` | `FilesView` unit tests and the file browser journey |
| Beads | `internal/api/beads*` | `BeadsView` unit tests and the Beads journey |
| Scheduled | `internal/api/scheduled*`, `internal/scheduled` | `ScheduledTasksView` unit tests |
| Server | `internal/api/system*`, `health*` | `SystemStatusView` and App unit tests |
| Settings | tmux appearance/mouse/session APIs | `SettingsView` and workspace-layout unit tests |

Optional Services follows the same rule: API adapter tests, view unit tests, and
one mocked primary-action journey.

## CI

CI runs five jobs in parallel, split along their real dependencies rather than
listed in one serial script. The dashboard bundle is embedded into the Go binary
by a compile-time directive and is not tracked, so every job that compiles the
server waits for `build`; nothing else waits for anything.

| job | depends on | contents |
| --- | --- | --- |
| `build` | — | Node and Go setup, dashboard install, embedded bundle, server binary; publishes both as artifacts |
| `go` | `build` | gofmt, vet, race tests against the downloaded bundle; installs no Node |
| `unit` | — | dashboard install without a browser, vitest, eslint |
| `browser` | — | dashboard install with Chromium, mocked Playwright at the runner's worker count |
| `contracts` | `build` | source contracts, built-server contract, public installer smoke |

`govulncheck` and `npm audit` run as their own job on the weekly scheduled
invocation only.

The browser worker count is the runner's, set in `dashboard/playwright.config.ts`.
Pinning it to one worker was the single largest cost in CI: it serialised the
whole measured browser CPU into one lane while the runner's other cores idled.

## Rules

- No `t.Skip`; environment-dependent Go tests use `//go:build live`.
- Keep Playwright retries at zero and do not hide failures with timeout growth.
- Do not add a test that only asserts a retired feature is absent.
- Do not add tests of gate scripts or files named `hardening`, `baseline`,
  `fence`, `guard`, or `prototype`.
- A behaviour change needs a test that fails when the operator contract regresses.
- Prefer waiting for an event over sleeping. A fixed sleep in a test is a defect
  unless a comment says what is being waited for and why nothing signals it.
- A test that re-execs the test binary must cancel the race runtime's exit delay
  with `GORACE=atexit_sleep_ms=0`, or it pays a second per child for nothing.
- Use an alternate `CHROTE_PLAYWRIGHT_PORT` when another process owns the Vite
  test port; never kill an unrelated listener.
