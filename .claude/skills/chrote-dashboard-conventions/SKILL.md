---
name: chrote-dashboard-conventions
description: Use when changing CHROTE dashboard views, terminal workspaces, state ownership, API consumption, styles, or dashboard tests. Do not use as product or API source truth.
---

# CHROTE dashboard conventions

Verified: 2026-08-30

Read `AGENTS.md`, `PRD.md`, `DESIGN-SYSTEM.md`, `dashboard/README.md`, and
`docs/TEST_STRATEGY.md` before editing. Those tracked files override this
workflow when they change.

## Find the owner first

1. Locate the shipped view in the six-view source map in `AGENTS.md`.
2. Read the component, its immediate callers, its colocated tests and styles,
   the frontend request helper, and the matching Go handler.
3. Identify the existing state owner before adding state. Prefer the narrowest
   owner already responsible for that lifecycle.
4. Preserve explicit loading, empty, degraded, and error states. Never replace
   unavailable data with a synthetic success.

## State and terminal rules

- `dashboard/src/context/SessionContext.tsx` owns shared session state.
- Session polling, workspace layout persistence, and send-to-session behavior
  live under `dashboard/src/context/`; extend those owners instead of creating
  a second path.
- Treat browser storage as a schema. Preserve existing keys and shapes unless
  the task explicitly includes a migration and migration tests.
- `IframePool.tsx` owns terminal iframe lifetime. Views and sidecars select or
  reveal terminals; they do not remount an iframe to express visibility.
- `TerminalWorkspaceDock.tsx` owns the terminal workspace composition and its
  sidecars. Keep layout behavior there unless a current caller proves otherwise.
- Keep tmux sessions independent from browser presentation state. Closing or
  resetting UI state must not kill external work.

## API and component rules

- Match the response shape already used by the endpoint. Do not normalize a
  deliberately flat tmux response into another endpoint's envelope.
- Reuse existing request and error helpers before adding a new client layer.
- Keep view-specific code, tests, and styles colocated with the view.
- Reuse design tokens and established classes from the active design system.
- Preserve keyboard, focus, accessible-name, narrow-viewport, and reduced-motion
  behavior affected by the change.
- Avoid speculative abstractions. Extract only when multiple current callers
  already share the same contract.

## Test the intent

1. Add or update colocated Vitest coverage for state and component behavior.
   Encode why the behavior matters, including failure or migration paths.
2. Update `dashboard/tests/mock-api.ts` when a browser journey makes a new
   request. Unmocked requests are fixture failures, not harmless noise.
3. Add or update Playwright coverage only for a user-visible journey that unit
   tests cannot prove.
4. Run the narrow test first, then the dashboard gates:

   ```bash
   cd dashboard
   npm run test:unit -- --coverage
   npm run lint
   npm test
   cd ..
   ./scripts/build-embedded-dashboard.sh
   python3 scripts/check-embedded-dashboard.py
   git diff --check
   ```

If the browser-test port is occupied, set `CHROTE_PLAYWRIGHT_PORT` to an unused
port. Do not stop an unrelated process.

## Done when

- State has one clear owner and persistence compatibility is proven where used.
- The UI exposes honest async and error behavior and keeps terminal lifetime
  separate from presentation.
- Relevant unit and browser journeys pass, the embed matches source, and the
  handoff reports any skipped broader gate.
