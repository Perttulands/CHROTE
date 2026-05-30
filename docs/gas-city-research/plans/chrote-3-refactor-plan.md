# CHROTE 3.0 Gas City Refactor Plan

Status: abandoned by ADR-0003 rollback.
Date: 2026-05-30

2026-05-30 rollback note: this plan is preserved as historical context only.
Do not execute it as the active CHROTE implementation plan. CHROTE has rolled
back active Gas City integration and keeps only the docs/plans/research.

## Desired Outcome

CHROTE should feel like the same session access system Perttu already uses, but
with Gas City-backed identities available as first-class sessions.

When this first refactor PR is done:

- the standalone Gas City tab is no longer the primary product surface;
- direct workflow-launcher UI is removed from the operator path;
- existing plain tmux sessions keep working and are not migrated or disrupted;
- Gas City sessions can appear in the existing session/sidebar flow;
- opening a Gas City session from CHROTE attaches through native `gc session`
  semantics instead of pretending it is an ordinary CHROTE tmux session;
- the user can create a plain tmux session or a Gas City-backed identity from
  the session creation workflow;
- durable work and status still live in Beads, not on a CHROTE dashboard.

## Non-Goals

- Do not build Team Builder yet.
- Do not build Master Board yet.
- Do not build a mail/status/events dashboard as the main Gas City value.
- Do not migrate existing running CHROTE tmux sessions.
- Do not expose the raw Gas City supervisor or mirror the `gc` command tree.
- Do not add budgets, approval gates, or special delegation policy in phase 1.
- Do not use temporary wrappers that make a CHROTE-launched process look like a
  valid Gas City identity when Gas City does not own it.

## Current State Inspected

- `PRD.md` and `docs/CHROTE_VISION.md` already state that CHROTE 3.0 should
  focus on named Gas City-backed identities rather than passive dashboards.
- `../vision/agent-orchestration-vision.md` now records the
  corrected vision and the low-structure Master Board idea.
- `dashboard/src/components/TabBar.tsx` still exposes a top-level `Gas City`
  tab.
- `dashboard/src/components/GasCityView/index.tsx` exposes health, sessions,
  mail, a Pi poem smoke form, review-quorum launch form, workflow lists, and
  events.
- `src/internal/api/gascity.go` exposes bounded observer/transcript routes, but
  also CHROTE-side poem and review-quorum launcher routes.
- `terminal-launch.sh` currently only attaches to sessions on the CHROTE tmux
  socket.
- `src/internal/proxy/terminal.go` starts one `ttyd` process with `-a`, so the
  URL `arg` reaches `terminal-launch.sh` as `$1`. That makes dispatching
  `gc:<session-id>` targets feasible, but unproven.
- Gas City provides documented primitives needed for identity access:
  `gc session new shell --alias <name> --title <title> --no-attach --json`
  and `gc session attach <identity-or-id>`. CHROTE should use only the
  configured `shell` template in this phase.

## Implementation Chunks

### 0. Prove Gas City Session Attach Through CHROTE Terminal

Before building UI on top of Gas City sessions, prove that a `gc session attach`
target can render through CHROTE's existing `ttyd` iframe path.

Likely touched files:

- `terminal-launch.sh`
- `dashboard/src/components/IframePool.tsx` only if URL target encoding needs to
  be adjusted
- a small test file if the launch-script behavior is extracted or shell-tested

Behavior change:

- Plain session names still attach to the CHROTE tmux socket as they do today.
- Targets shaped like `gc:<session-id>` cause `terminal-launch.sh` to run
  `gc --city "$CHROTE_GASCITY_CITY_DIR" session attach <session-id>`.
- The `gc` attach path must not inherit CHROTE's tmux socket in a way that
  prevents Gas City from reaching its own session runtime.

Verification:

- One real Gas City session can be attached through CHROTE's terminal iframe,
  rendered, resized, detached, and reattached.
- Existing plain tmux attach still works.
- If the attach path cannot be proven, stop and document the blocker before
  building session-list or creation UI on top of it.

Safety boundary:

- Do not kill, rename, reparent, or migrate any existing tmux or Gas City
  session during the proof.

### 1. Remove the Wrong Operator Surface

Refactor the dashboard so Gas City is not a top-level destination.

Likely touched files:

- `dashboard/src/components/TabBar.tsx`
- `dashboard/src/App.tsx`
- `dashboard/src/components/GasCityView/*`
- `dashboard/src/services/gascityClient.ts`
- `dashboard/src/styles/gascity.css`
- `src/internal/api/gascity.go`
- `src/internal/api/gascity_test.go`
- `PRD.md`
- `CLAUDE.md`

Behavior change:

- No visible `Gas City` tab in normal navigation.
- No operator-facing Pi poem smoke form.
- No operator-facing review-quorum launcher form.
- No CHROTE-side poem or review-quorum launcher endpoints.
- A minimal `/api/gascity/observer` client and endpoint remain because the
  session flow needs them in Chunk 2.
- The retained observer is read-only and limited to session/identity metadata
  needed for merging, such as id, alias/name/title, status, source, template,
  and attach target when available.
- Existing terminal, files, agents, beads, services, settings, and help views
  remain unchanged.

Verification:

- Frontend tests no longer expect the Gas City tab or launcher UI.
- `npm run build` passes in `dashboard/`.
- No terminal iframe lifecycle code is changed.

Safety boundary:

- Do not delete or close any live tmux or Gas City session.
- Do not remove the observer plumbing needed by the merged session list.
- Do not leave the old launcher as a hidden/internal product surface.

### 2. Represent Gas City Sessions In The Existing Session Flow

Make Gas City-backed sessions visible as session choices without treating them
as ordinary tmux sessions.

Depends on: Chunk 0 and Chunk 1.

Likely touched files:

- `dashboard/src/context/SessionContext.tsx`
- `dashboard/src/types.ts`
- `dashboard/src/components/SessionPanelV2.tsx`
- `dashboard/src/components/SessionGroupV2.tsx`
- `dashboard/src/components/SessionItemV2.tsx`
- `dashboard/src/components/IframePool.tsx`
- `terminal-launch.sh`

Behavior change:

- CHROTE fetches the normal `/api/tmux/sessions` list and augments it with
  Gas City sessions from `/api/gascity/observer` when Gas City is available.
- Gas City sessions have an explicit source marker and a stable attach target
  such as `gc:<session-id>`.
- Opening a Gas City session uses `terminal-launch.sh` to call
  `gc --city "$CHROTE_GASCITY_CITY_DIR" session attach <session-id>`.
- If Gas City is unavailable, existing plain tmux session behavior still works.

Verification:

- Unit tests cover merging Gas City sessions without breaking plain tmux
  sessions.
- Opening a `gc:<id>` terminal target constructs the expected terminal URL.
- `terminal-launch.sh` handles both plain session names and `gc:<id>` targets.
- Existing terminal persistence tests still pass.

Safety boundary:

- The merge is read-only.
- Do not kill, rename, or reparent Gas City sessions.
- Do not assume Gas City tmux sockets are on the CHROTE tmux socket.

### 3. Create Gas City Identities From Session Creation

Add the first useful creation path: the user can choose plain tmux session or
Gas City-backed identity from the session creation workflow.

Depends on: Chunk 0 and Chunk 2.

Likely touched files:

- `src/internal/api/gascity.go`
- `src/internal/api/gascity_test.go`
- `dashboard/src/components/SessionPanelV2.tsx`
- `dashboard/src/components/TerminalWindow.tsx`
- `dashboard/src/services/gascityClient.ts`
- `dashboard/src/types.ts`

Behavior change:

- Backend adds a narrow `POST /api/gascity/sessions` route that runs native
  `gc session new shell --alias <name> --title <title> --no-attach --json`.
- The dashboard session creation UI lets the user choose:
  - plain tmux session; or
  - Gas City identity with name/alias and optional title.
- The UI does not expose an arbitrary Gas City template chooser.
- Rig scope fails loud until CHROTE has a real configured/proven rig behavior.
- Creating a Gas City identity refreshes the session list and can bind the new
  `gc:<session-id>` target into the current terminal window.

Verification:

- Backend tests cover validation, exact `gc` argv, JSON parsing, and failure
  responses.
- Frontend tests cover both tmux and Gas City creation paths.
- The PR-level end-to-end gate proves this path: create a Gas City identity,
  list it, attach through CHROTE, and render it; otherwise the precise blocker
  is documented and the PR does not claim this path is complete.

Safety boundary:

- Only native `gc session new shell` counts as identity creation in this phase.
- Do not create a fake Gas City identity by only creating a tmux session plus
  environment variables.
- Do not change existing session names or migrate old sessions.

### 4. Preserve Agent-Side Orchestration, Not UI Launching

Keep Gas City workflow power available to agents through native `gc` and docs,
not through a user-facing CHROTE workflow launcher.

Depends on: Chunk 0 and Chunk 1.

Likely touched files:

- `src/internal/api/gascity.go`
- `src/internal/api/gascity_test.go`
- `../vision/agent-orchestration-vision.md`
- `PRD.md`
- `README.md`

Behavior change:

- CHROTE retains bounded read/recovery support that is useful near sessions.
- Docs and tests confirm CHROTE-side direct workflow launch endpoints removed
  in Chunk 1 stay gone rather than becoming hidden/internal launchers.
- During implementation, identify the actual transcript/recovery route by code
  search and its callers. Keep it only if a concrete consumer survives the new
  terminal attach path; otherwise remove it. The expected recovery path is
  opening the session.
- Docs say agents should use native `gc` primitives for mail, sling, nudge,
  formulas, and molecules.

Verification:

- Tests prove removed launcher endpoints are gone or intentionally unavailable.
- Docs no longer describe a Gas City tab as an active view.
- Go tests pass.

Safety boundary:

- Removing the UI launcher must not remove documented evidence from prior
  smokes.
- Do not delete Gas City runtime files or mail/workflow records.

### 5. File Follow-Up Beads For Real Orchestration Power

After the session/identity access PR is working, create or realign Beads for
the next layer:

- agent-to-agent mail as a durable request/reply plane;
- sling delegation between named identities;
- reusable molecules/workflows such as review-quorum;
- low-structure Master Board as team broadcast;
- Team Builder as a later CHROTE composition surface.
- agent-initiated identity spawning, including persistent "hire Lucy" identities
  and disposable polecat-style helpers.

These should not block the first identity-access PR unless they expose a direct
dependency.

Verification:

- Beads have clear acceptance criteria and dependency links.
- A beads review pass confirms they are handoff-ready.

## PR Readiness Gates

- `go test ./...` passes under `src/`.
- `go vet ./...` passes under `src/`.
- `npm run build` passes under `dashboard/`.
- Relevant frontend tests pass.
- `git diff --check` passes.
- Existing plain tmux sessions still attach through CHROTE.
- No live tmux or Gas City session was killed, renamed, reparented, or migrated
  by the refactor.
- One evidenced end-to-end Gas City identity path succeeds:
  create identity -> appears in the session list -> attach through CHROTE
  terminal -> renders. If this cannot run because Gas City is unavailable, the
  precise blocker is documented and the PR does not claim the path is complete.
- `git status` is clean except intended Beads state if Beads writes are
  intentionally separate.
- Claude reviews the plan, Beads, implementation, and verification results.
- The final PR description explains that this PR corrects the product direction:
  CHROTE is the access layer for Gas City identities, not a Gas City dashboard
  or workflow-launcher tab.
