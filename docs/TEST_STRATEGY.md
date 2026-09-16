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

Measurements from September 2026. Counts are executed cases, not source-file
matches. Wall time includes runner setup and varies with host load.

| suite | files | cases | wall |
| --- | --- | --- | --- |
| Go, race enabled, earlier measurement | 29 | 198 test functions | 3.9s |
| Dashboard unit, September 14 | 97 | 896 | 8.54s |
| Mocked Playwright, September 14 | 25 | 58 | 28.8s at four workers |

Choose the test layer by what the regression needs to expose it. Browser startup
and rendering cost more than component rendering, but moving a geometry check
to jsdom would lose the behavior it protects.

## Test matrix

| Layer | Owns | Command |
| --- | --- | --- |
| Go | API shapes, persistence, tmux/filesystem behaviour, concurrency | `cd src && go test -race ./...` |
| Vitest | Components, state transitions, localStorage, formatting, error states | `cd dashboard && npm run test:unit` |
| Mocked Playwright | Operator journeys through a real browser, at retries 0 | `cd dashboard && npm test` |
| Built-server contract | Embedded assets, served fonts, terminal and Files API/browser seam | `./scripts/test-built-server-contract.sh` |
| Live Playwright | Operator-approved real-backend/tmux smokes | `cd dashboard && npm run test:live` |
| Source contracts | Docs, host neutrality, embedded parity | `scripts/doc-lint.py`, `scripts/host-neutrality.py`, `scripts/check-embedded-dashboard.py` |
| Installed product | Installer routes, environment contract, unit, repeat-safe tmux grant, health build-stamp proof by default in source mode or with `CHROTE_EXPECTED_BUILD_COMMIT` in binary mode | `./scripts/test-public-install.sh <binary>` |
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

Beads map contents, ready/stale grouping, template details and lazy loading of
closed work belong to `BeadsView.test.tsx`. The browser suite covers its keyboard
entrypoints, menus, table placement and Flow geometry. Library's unconfigured
state belongs to `LibraryView.test.tsx`; canvas interaction and scroll extents
remain browser checks. Preset limits belong to `useWorkspaceLayouts.test.ts`;
the browser saves, restores and deletes a real bound layout across a reload.

Announcements are events, not readiness markers. `StatusLine.test.tsx` proves
that the newest announcement replaces the previous one. A browser journey
checks the action's result, such as a sent payload or an open drawer, without
requiring its status message to outlive unrelated background requests.
Resize checks likewise inspect each session's announced dimensions and the
rendered terminal bounds. The number of ResizeObserver deliveries is not a
protocol guarantee.

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

CI runs on pushes to `main` and `master`, pull requests targeting either branch,
and manual `workflow_dispatch`. It always runs document and host-neutrality
checks. The `changes` job selects whether the five product jobs are also needed.

`scripts/ci-product-required.py` contains the documentation allowlist. It admits
named root narrative Markdown files, Markdown under `docs/`, and images under
`docs/assets/` or `docs/images/`. Agent instructions, executable or symlink
changes, and all other paths require full product checks. Mixed changes do too.
The classifier includes deleted paths and both sides of a rename. Missing bases,
empty diffs and classification errors default to full product checks.

Push runs compare the previous branch commit with the pushed commit. PR runs
compare the target base with GitHub's proposed merge commit and test that merge.
Manual dispatch always runs all product checks, including for a documentation
commit. Use it when the exact candidate needs full-product evidence.

| job | depends on | contents |
| --- | --- | --- |
| `build` | `changes` | Node and Go setup, dashboard install, embedded bundle, stamped server binary; publishes both as artifacts |
| `go` | `build` | gofmt, vet, race tests against the downloaded bundle; installs no Node |
| `unit` | `changes` | dashboard install without a browser, vitest, eslint |
| `browser` | `changes` | dashboard install with Chromium, mocked Playwright at the runner's worker count |
| `contracts` | `build` | embedded parity, built-server contract, public installer smoke with exact-commit verification |

The `docs` job runs independently. `CI result` checks that every selected job
succeeded and that product jobs were skipped only for a documentation-only run.
Failures, cancellations and unexpected skips cannot produce a successful result.
The workflow itself is never skipped by a path filter.

The product jobs share the bundle and binary produced by `build`. Go compilation
needs the embedded bundle, so `go` and `contracts` wait for it. `unit` and
`browser` can run alongside `build` once classification finishes.

[`CONTRIBUTING.md`](../CONTRIBUTING.md#stable-local-gates) has the full local
recipe, including `scripts/build-server.sh` and explicit build-commit verification
in the installation smoke. Record the tested commit and hosted run URL. A
successful documentation-only run is not deployment evidence, and a successful
PR merge candidate does not replace validation of the resulting main commit.
Hosted CI does not deploy the service.

Dependency scans (`govulncheck`, `npm audit`) are not a CI job: a scheduled
run that fails on a transitive advisory is noise nobody acts on. Run them by
hand when a dependency changes.

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
