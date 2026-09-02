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

Shared session state belongs in `dashboard/src/context/`. The terminal is in-page: `dashboard/src/terminal/terminalSession.ts` owns an xterm instance and its WebSocket, `TerminalPool.tsx` keeps one of those per bound session, `TerminalSurface.tsx` is the only place a terminal goes on screen, and `dashboard/src/terminal/tileState.ts` derives which state a tile is in. Workspace composition and sidecars belong to `TerminalWorkspaceDock.tsx`. Keep view-specific code, tests, and styles with the view.

## Preserve the terminal model

tmux sessions exist independently of browser presentation. Hiding, moving, resetting, or closing UI state must leave the underlying session intact.

[ADR-0017](../../docs/adr/0017-terminal-viewing-model.md) decides who owns a tmux window's size and what a binding means. [ADR-0018](../../docs/adr/0018-terminal-transport-ownership.md) decides how terminal bytes reach the browser. Read both before changing terminal behavior, and link to them instead of restating them. What they mean while editing:

- A binding is the operator's stated intent, not a cache of live sessions. Only the operator removes one. Do not reintroduce a prune, a stale-candidate set, or a protection counter.
- A tile is in exactly one of five states: `Idle` (bound, off screen), `Live` (attached, sizing the window or watching at another client's size), `Taken over` (a client outside CHROTE detached this terminal; last frame plus `Claim`), `Lost` (the connection went, the session did not; last frame plus `Reconnect`), `Ended` (session gone; last frame plus `Restart` and `Remove`). A tile that has been shown keeps its connection and its last rendered frame off screen, so do not add a disconnect on hide.
- Nothing attaches with `-d`. A tile takes the sizing seat when no other client holds it and attaches with `-f ignore-size` when one does, so a second device watches instead of evicting. `Claim` moves the seat by flagging the other sizing clients before clearing its own. Peek never takes the seat.
- The server sizes a new session once, at creation. There is no periodic sweep, idle classification, or recurring size check, and adding one reopens ADR-0017. A session nobody views keeps the size it was left at.
- CHROTE never runs `resize-window`, which pins a window to `window-size manual`. Session facts that contradict appearances are surfaced as badges, not silently corrected.
- Fit a terminal only when its container has real layout. A detached or `display:none` container measures zero, and fitting it pushes a bogus size into the shared tmux window.

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
