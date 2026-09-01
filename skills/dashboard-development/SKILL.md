---
name: dashboard-development
description: Change CHROTE dashboard views, terminal workspaces, browser state, host-setting controls, API consumption, styles, or dashboard tests.
---

# Develop the CHROTE dashboard

Read `AGENTS.md` first. Use the repository and package scripts for current build and test commands. Read `DESIGN-SYSTEM.md` only when the change affects visual design.

## Find the owner

Before editing:

1. Read the component, its callers, colocated tests and styles, the frontend request helper, and the matching Go handler.
2. Identify which layer owns the state. Extend that owner instead of creating another path.
3. Check loading, empty, degraded, and error behavior. Keep failures visible.

Shared session state belongs in `dashboard/src/context/`. Terminal iframe lifetime belongs to `IframePool.tsx`. Workspace composition and sidecars belong to `TerminalWorkspaceDock.tsx`. Keep view-specific code, tests, and styles with the view.

## Preserve the terminal model

tmux sessions exist independently of browser presentation. Hiding, moving, resetting, or closing UI state must leave the underlying session intact.

Treat browser storage as a schema. Preserve existing keys and shapes unless the Bead includes a migration and tests for it. Layouts and purely visual preferences may stay device-local. A browser load must not silently apply host-global tmux changes. Host mutations require an explicit operator action and visible result.

## Match existing interfaces

- Match the response shape already owned by the endpoint.
- Reuse current request and error helpers.
- Update `dashboard/tests/mock-api.ts` when a browser journey adds a request.
- Preserve keyboard, focus, accessible naming, narrow viewport, and reduced-motion behavior affected by the change.
- Extract shared code only when current callers already share the contract.

## Verify

Run the narrowest relevant test first. Then run the dashboard and embedded-build gates from `AGENTS.md` that the changed behavior requires.

Completion requires fresh proof of the user-visible behavior, honest failure handling, preserved tmux sessions, and an embedded dashboard that matches its source.
