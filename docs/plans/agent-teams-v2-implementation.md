# Agent Teams v2 — Implementation Plan

*Companion to `agent-teams-v2.md`. Translates the architecture into concrete work units.*

## Summary

The architecture doc decided **what** changes. This doc decides **how** to ship it on `feature/agent-teams` without breaking what already works. Existing files: `src/internal/teams/{models,engine,store,engine_test}.go`, `src/internal/api/teams.go`. No frontend yet.

## Decisions resolved from the v2 open questions

| Question | Decision | Rationale |
|----------|----------|-----------|
| fsnotify vs polling | **Polling** at 1s | Engine no longer in critical path. Avoids new dep. |
| Auto-pipe on spawn | **Yes, automatic** | One less concept in harness JSON. Logs always available. |
| Workspace path injection | **`tmux new-session -e VAR=…`** | Standard tmux, no shell wrapper, env clean. |
| pipe-pane reliability | **Append mode (`cat >>`)** | Survives detach/reattach without truncation. |
| Nudge content | **`step.With` filename, content capped at 4 KB, newlines→spaces** | Keeps existing behavior for short JSON. |
| Conditions | **Keep current syntax** (file existence + `success`/`failure`) | No new evaluator. |
| `mail-*.jsonl` | **Keep as-is** | Already implemented; harmless. |

## Backend changes

### Engine (`src/internal/teams/engine.go`)

1. **Auto-pipe on spawn.** After `tmux new-session …`, run `tmux pipe-pane -t <session> "cat >> <ws>/<role>.log"`. No new action — `pipe` is a side effect of spawn.
2. **Env injection.** Add `-e CHROTE_WORKSPACE=<ws> -e CHROTE_ROLE=<role> -e CHROTE_TEAM_ID=<id>` to `new-session` args. Agents can read these to know where they live.
3. **`file:*` triggers.** Extend `teamPoller` with `knownFiles map[string]bool`. Each tick, for every `file:<name>` trigger in the harness flow, check `<ws>/<name>` existence. Edge-trigger: fire only on transitions from absent → present. Reset on team reset.
4. **Nudge respects `step.With`.** Read `<ws>/<step.With>` if set, else default `feedback.json`. Cap at 4 KB. Existing newline sanitization stays.
5. **`stop:*` is cleanup-only.** Drop the implicit "spawn next role on stop" pattern. The engine still fires `stop:<role>` triggers (so harness can mark errors), but built-in harnesses no longer chain spawns through stop.
6. **Pollerschema:** add `knownFiles` field, initialize in `RegisterTeam`/hydrate, clear in reset.
7. **Implicit roles.** If a role has no `spawnCmd`, skip spawn for that role on the `start` trigger when the team has no member for it. Allow `Team.SessionMap` to pre-bind a role to an existing session — register that as a `TeamMember` at team creation.
8. **Poll tick** 5s → 1s.

### Models (`src/internal/teams/models.go`)

1. Add `Team.SessionMap map[string]string` (role → tmux session name).
2. Update built-in harnesses to the v2 model:
   - **`ai-collab`**: spawn builder + verifier on `start`, `file:feedback.json` → nudge builder, `file:status.json` → status complete (when status=success).
   - **`verifier-loop`**: same shape as `ai-collab`, generic spawn commands.
   - **`pipeline`**: spawn extract/transform/load concurrently; sequencing is encoded in their `spawnCmd` waiting on done-files. `file:load-done` → status complete. `stop:*` → status error if !status.success.
   - **`pair-programming`**: unchanged.

### API (`src/internal/api/teams.go`)

1. `createTeamReq` gains `Sessions map[string]string` (role → existing session) for implicit roles.
2. Add `POST /api/teams/{id}/signal` taking `{"file":"<name>"}` — touches `<ws>/<name>`. This is the "Request Review" button's backend.
3. Workspace listing: `GET /api/teams/{id}/files` returns artifacts (log files, feedback.json, status.json) so the frontend can preview them.
4. `GET /api/teams/{id}/files/{name}` returns the raw file content (used for log preview / feedback display).

### Tests (`src/internal/teams/engine_test.go`)

1. `TestEngine_AutoPipe` — spawn a member, verify `pipe-pane` was invoked with the right log path.
2. `TestEngine_EnvInjection` — verify `-e CHROTE_WORKSPACE=…` etc. appear in `new-session` args.
3. `TestEngine_FileTrigger_FiresOnce` — create a workspace file, tick, verify trigger fires; tick again, verify no re-fire.
4. `TestEngine_FileTrigger_ReFiresAfterRemoval` — file appears, fires; file removed; file re-appears; fires again.
5. `TestEngine_AiCollab_ConcurrentStart` — both builder and verifier spawned by `start`, both alive, no chained `stop:` spawns.
6. `TestEngine_Nudge_ReadsStepWith` — set `step.With = "review.json"`, verify nudge reads that file.
7. `TestEngine_ImplicitRole_PreboundSession` — team with `SessionMap{builder: "my-session"}`, no spawn occurs, member registered, nudge works.
8. Update `TestEngine_StopTrigger_SpawnsVerifier` and `TestEngine_Nudge_DeadSession` to match new built-in harness shapes (or delete if no longer applicable).

API-level tests in `src/internal/api/teams_test.go` (new file): create-team with sessions map; signal endpoint; file listing.

## Frontend changes

### Tab + entry point

1. New `Tab` literal `'teams'` in `dashboard/src/components/TabBar.tsx`. Add to the visible tab list with label "Teams".
2. `App.tsx` — render `<TeamsView />` when `activeTab === 'teams'`.

### Components (`dashboard/src/components/TeamsView/`)

```
TeamsView/
├── index.tsx              # main view, three-column layout
├── TeamList.tsx           # left column: list of teams + "new" button
├── HarnessPicker.tsx      # modal: pick a harness when creating a team
├── TeamDetail.tsx         # center: members, status, controls, signal button
├── TopologyGraph.tsx      # right: SVG diagram of harness roles + flow edges
├── LogPreview.tsx         # bottom of detail: tail of selected member's log
├── api.ts                 # fetch helpers for /api/harnesses + /api/teams
├── types.ts               # TS types mirroring Go models
└── styles.css             # tab-scoped styles
```

Layout (desktop):

```
┌─────────────────────────────────────────────────────────┐
│ Teams Tab                                               │
├──────────────┬──────────────────────────┬───────────────┤
│ TeamList     │ TeamDetail               │ TopologyGraph │
│ • Team A ●   │ name, status, members    │ ┌──┐    ┌──┐  │
│ • Team B ✓   │ [start][stop][reset]     │ │bld│──▶│vrf│ │
│ • Team C ✗   │ ─────────────────        │ └──┘    └──┘  │
│              │ LogPreview (tail)        │               │
│ [+ New]      │ [Request Review] btn     │               │
└──────────────┴──────────────────────────┴───────────────┘
```

### Behavior

- Poll `/api/teams` every 2s when tab is active. Pause polling when tab not visible.
- `+ New` opens `HarnessPicker` modal: list harnesses from `/api/harnesses`, pick one, give the team a name, optionally pre-bind sessions to roles → `POST /api/teams`.
- Status badges: `pending` (gray), `running` (cyan), `reviewing` (orange), `complete` (green), `error` (red), `paused` (yellow). Reuse existing theme tokens.
- Member rows: role badge, session name, status dot, link "open in terminal" that switches to terminal tab and binds the session to a window.
- Topology graph: read `harness.flow`, lay out roles as nodes, draw arrows for `file:* → nudge:role` edges and `stop:role → status` edges. No external graph library — minimal SVG with manual layout (left-to-right by role order).
- Log preview: poll `/api/teams/{id}/files/{role}.log`, show last ~200 lines.
- "Request Review" button: `POST /api/teams/{id}/signal` with `{"file":"ready"}`.

### Tests (`dashboard/src/components/TeamsView/*.test.tsx`)

1. `TeamList.test.tsx` — renders teams, clicking selects.
2. `HarnessPicker.test.tsx` — modal flow, validates name + harness.
3. `TeamDetail.test.tsx` — renders status badge, buttons fire correct API calls, signal button posts to `/signal`.
4. `TopologyGraph.test.tsx` — renders nodes for each role, renders edges based on flow.

## Migration

The harness JSON files in `.chrote/harnesses/` are seeded once. To pick up the v2 built-in shapes on existing installs, the engine reseed should compare hashes — but for this branch, simplest is: delete `.chrote/harnesses/*.json` on dev and let `Init` re-seed. Document this in CHANGELOG.

## Done criteria

- [ ] `go build ./...` and `go test ./...` green.
- [ ] `cd dashboard && npm run build` green.
- [ ] Frontend tests pass (`npm test`).
- [ ] An `ai-collab` team can be created from the UI; both members spawn concurrently; writing `feedback.json` in the workspace nudges the builder; writing `status.json` with `{"status":"success"}` marks the team complete.
- [ ] CHANGELOG entry added.

## Out of scope (future work)

- fsnotify-based file watching.
- Topology graph editing (read-only for now).
- Inline log streaming via WebSocket (polling is fine).
- Custom harness editor UI (use API/JSON directly for v2).
