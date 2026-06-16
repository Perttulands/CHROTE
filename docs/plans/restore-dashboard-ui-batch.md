# Plan: Restore the lost CHROTE dashboard UI batch (non‑Gas‑City)

> Status: proposed (not yet executed). Working document — uncommitted.
> Durable task record: bead `home-hjfi`. Source ref for all restores: `cc8cb6b`.

## Context

A feature‑flagged dashboard UI improvement batch was developed but **never merged to `main`**. During
the Gas City scrub the repo was reset back to `origin/main` (`586f67a`), abandoning that work. Two
losses were reported — a terminal **"refit" button** (re‑fits the live terminal when the agent's
input scrolls out of view) and weaker **file‑browser state durability** — but those are two items in
a larger UI batch to be fully restored, **without** any Gas City code.

Scope decision, 2026-06-01: this is intentionally the expanded `cc8cb6b` UI batch, not the earlier
7-file surgical restore. It includes System Status, V2 sidebar code, Beads issue detail/comments,
all-status Beads loading, session location badges, terminal refit, and Files durability.

### Forensic findings (verified read‑only)

| Ref | What it is | Buildable? | Gas City? |
|---|---|---|---|
| `main` = `586f67a` | current base; minimal `package.json` (`@dnd-kit/core`, `react`, `react-dom`) | yes | no |
| `3f3c827` (tag `chrote-2.0-baseline`) | first "feature‑flagged dashboard review batch" | **NO** — `TerminalArea.tsx` imports `lucide-react` absent from its `package.json`; `App.tsx` imports `OracleView/V2` which doesn't exist yet | no |
| `cc8cb6b` (`feature/chrote-ui-review-batch` HEAD) | batch completed | **YES** — adds `lucide-react`, `OracleView/V2.tsx`, `SystemStatusView`, `system.go` | **no entanglement** |
| `9ca2de9` (`chrote-3.0-gascity`) | superset + Gas City | yes | **yes** — gascity code/imports first appear here |

Conclusions that shape this plan:
- **Source every expanded-batch restore from `cc8cb6b`**, never `3f3c827` (WIP/non‑building) or
  `9ca2de9` (Gas City).
- At `cc8cb6b`: UI files contain no `gascity`; `system.go` imports only `internal/core`;
  `src/cmd/server/main.go` registers `systemHandler` with **no** `gasCityHandler`; `go.mod` unchanged
  vs main → the only Go change to wire System Status is additive.
- The CRLF churn in `cc8cb6b` is confined to files we will NOT bring; the INCLUDE files are already LF.
- `main` uses Unicode‑glyph icons (sidebar refresh is `↻`); the `chrote-ui-work` skill also endorses
  lucide. The V2 components genuinely need `lucide-react` (~18 icons), so it is added; the **refit
  button reuses `↻`**.

Outcome: a fresh branch off `main` with the expanded clean UI batch, building green, embedded into
`chrote-server`, deployed to the live service (:8094), smoke‑checked — V2 sidebar behind its existing
`uiV2` flag (default off), Beads detail/comments intentionally restored, and every user-visible
behavior behind its restored feature flag.

## Goal / success criteria

1. New branch off `main` builds green: dashboard (`tsc && vite build`) and Go (`go test ./...`,
   `go build`).
2. `git diff main...HEAD` contains **no** `gascity` path, import, or route.
3. After embed + `systemctl --user restart chrote.service`, the live dashboard shows:
   - terminal toolbar **`↻` refit button** that re‑fits the active session on click;
   - **Files** tab: draggable preview pane whose width persists across reload, and tab state that
     survives switching tabs;
   - **Beads** tab: all-status loading, clickable issue detail modal, readable comments, and
     intentionally write-capable "Add comment" flow through CHROTE's Beads API;
   - **System Status** tab backed by `GET /api/system/status`;
   - session location badges where restored;
   - V2 session sidebar when `uiV2` is enabled (off by default).
4. Every new user‑visible behavior is flag‑gated with a documented rollback.

---

## Execution runbook

> All commands assume `cd /home/perttu/chrote` unless noted. Source ref for every restored file is
> `cc8cb6b`. Checkpoint after each phase before proceeding.

### Phase 0 — Pre‑flight & bead

```bash
cd /home/perttu/chrote
git status --short                       # clean / only known drift; do NOT sweep unrelated changes
git rev-parse --abbrev-ref HEAD
sed -n '1,80p' AGENTS.md
bd prime && bd ready --json
# This restore executes bead home-hjfi (home workspace) → mark in_progress.
```

### Phase 1 — Branch off main

```bash
git switch -c feat/restore-dashboard-ui-batch main
```

### Phase 2 — Dependency (lucide-react)

```bash
git checkout cc8cb6b -- dashboard/package.json   # adds only "lucide-react": "^1.16.0"
cd dashboard && npm install && cd ..             # regenerates package-lock.json
```

### Phase 3 — Backend (Go): System Status + Beads detail/comments + router

```bash
git checkout cc8cb6b -- \
  src/internal/api/system.go \
  src/internal/api/system_test.go \
  src/internal/api/beads.go \
  src/internal/api/beads_test.go \
  src/cmd/server/main.go

if git grep -i 'gascity\|gas city\|GasCity' -- src/internal/api src/cmd/server; then
  echo "ERROR: Gas City reference found in restored Go files" >&2
  exit 1
else
  echo "OK: no gascity in Go"
fi

cd src && go test ./... && mkdir -p "$HOME/.local/state/chrote/builds" && go build -o "$HOME/.local/state/chrote/builds/chrote-server-check" ./cmd/server && cd ..
```

This phase intentionally restores the Beads detail and comments endpoints, including
`POST /api/beads/comments`. That write-capable route is part of the expanded UI batch and must remain
covered by backend tests before deploy.

Do **not** restore `src/internal/api/services.go` or `services_test.go` from `cc8cb6b`; that diff only
regresses current "Context Citadel" wording back to the older "Context API" label.

### Phase 4 — Frontend new components

```bash
git checkout cc8cb6b -- \
  dashboard/src/featureFlags.ts \
  dashboard/src/services/systemClient.ts \
  dashboard/src/components/LoadingSkeleton.tsx \
  dashboard/src/components/SystemStatusView/index.tsx \
  dashboard/src/components/OracleView/V2.tsx \
  dashboard/src/components/SessionPanelV2.tsx \
  dashboard/src/components/SessionItemV2.tsx \
  dashboard/src/components/SessionGroupV2.tsx \
  dashboard/src/components/BeadsView/IssueDetailModal.tsx
```

`IssueDetailModal.tsx` is intentionally included: it is the UI for Beads detail, comment reading, and
comment creation.

### Phase 5 — Frontend modified files (real changes only)

```bash
git checkout cc8cb6b -- \
  dashboard/src/App.tsx \
  dashboard/src/components/TerminalArea.tsx \
  dashboard/src/components/TerminalWindow.tsx \
  dashboard/src/components/FilesView/index.tsx \
  dashboard/src/components/SessionItem.tsx \
  dashboard/src/components/TabBar.tsx \
  dashboard/src/components/BeadsView/index.tsx \
  dashboard/src/components/BeadsView/IssueCard.tsx \
  dashboard/src/components/BeadsView/KanbanView.tsx \
  dashboard/src/components/BeadsView/hooks.ts \
  dashboard/src/components/BeadsView/types.ts
```

### Phase 6 — Styles

```bash
git checkout cc8cb6b -- \
  dashboard/src/styles/system-status.css \
  dashboard/src/styles/session-panel.css \
  dashboard/src/styles/beads.css \
  dashboard/src/styles/file-browser.css \
  dashboard/src/styles/terminal.css \
  dashboard/src/styles/base.css \
  dashboard/src/styles/index.css
```

### Phase 7 — Manual edit: refit‑button icon (reuse `↻`, drop lucide in TerminalArea only)

Edit `dashboard/src/components/TerminalArea.tsx`:

1. Delete: `import { RefreshCw } from 'lucide-react'`
2. Replace **every** occurrence (mobile + desktop branches) of `<RefreshCw size={14} />` with:
   ```tsx
   <span aria-hidden="true">↻</span>
   ```
   Keep the surrounding `<button className="layout-btn terminal-refit-btn" title="Refit terminal layout" …>` and `onClick={() => setRefitNonce(n => n + 1)}` as restored.

```bash
git grep -n 'lucide-react' -- dashboard/src/components/TerminalArea.tsx || echo "OK: TerminalArea lucide-free"
```
(The V2 components keep their lucide icons — expected and skill‑endorsed.)

### Phase 8 — What we did NOT touch

- **Gas City (exclude):** none exist at `cc8cb6b`; never source `gascityClient`/`sessionMerge`/
  `gascity*.go` from `9ca2de9`.
- **CRLF‑only files (skip — leave main's versions):** ~35 files whose only diff vs main is line
  endings (`context/SessionContext.tsx`, `context/ToastContext.tsx`, `hooks/useKeyboardShortcuts.ts`,
  `KeyboardShortcutsOverlay.tsx`, `LayoutPresetsPanel.tsx`, `ToastNotification.tsx`, most of
  `FilesView/components/*`, big CSS like `delightful.css`/`settings.css`/`help-view.css`/`inbox.css`,
  and `dashboard/tests/*.spec.ts`). Detect with `git diff --ignore-all-space main cc8cb6b -- <file>`
  (empty ⇒ skip). Leaving main's versions is functionally identical and avoids whitespace pollution.
- **Stale service wording (skip):** `src/internal/api/services.go` and `services_test.go` in
  `cc8cb6b` only rename Context Citadel back to the older Context API wording. Leave main's versions.
- **Unrelated ServicesView service options (skip):** `dashboard/src/components/ServicesView/index.tsx`
  in `cc8cb6b` only adds a Kokoro TTS backend option and regresses Context Citadel labels. Leave
  main's version.
- **MusicPlayer:** `App.tsx@cc8cb6b` no longer imports it; leave `MusicPlayer.tsx` orphaned.

```bash
git diff --stat
git diff --check
if git grep -i 'gascity\|gas city\|GasCity' -- dashboard/src src/internal src/cmd; then
  echo "ERROR: Gas City reference found in restored application files" >&2
  exit 1
else
  echo "OK: no gascity anywhere"
fi
```

### Phase 9 — Build dashboard + embed + Go build (per `chrote-ui-work` skill)

```bash
cd dashboard && npm run build && cd ..            # tsc typecheck + vite build (the gate that fails 3f3c827)
find src/internal/dashboard/dist -mindepth 1 -delete
cp -a dashboard/dist/. src/internal/dashboard/dist/
cd src && go test ./... && go build -o ../chrote-server ./cmd/server && cd ..
```

### Phase 10 — Deploy + smoke‑check (:8094)

```bash
systemctl --user status chrote.service --no-pager
systemctl --user restart chrote.service && sleep 1
systemctl --user status chrote.service --no-pager

curl -sS http://127.0.0.1:8094/api/beads/health | jq '.'
curl -sS http://127.0.0.1:8094/api/system/status | jq '.'    # NEW endpoint
curl -sS http://127.0.0.1:8094/ | rg 'Chrote Dashboard|/assets/'
```
Manual UI (`http://127.0.0.1:8094`):
- Terminal tab → `↻` refit button; click → active terminal re‑fits.
- Files tab → drag preview divider; reload → width persists; switch tabs and back → state kept.
- Beads tab → issue cards open detail modal; comments load; adding a comment succeeds on a disposable
  or approved bead and refreshes the modal.
- System Status tab present, populating from `/api/system/status`.
- Sessions show restored location badges where the flag is enabled.
- `localStorage.setItem('chrote-ui-v2','1'); location.reload()` → V2 sidebar; `'0'` → classic.

### Phase 11 — Commit / PR / close‑out

```bash
git status --short                                # only task files; surface unrelated drift, don't hide it
git add \
  dashboard/package.json \
  dashboard/package-lock.json \
  dashboard/src/featureFlags.ts \
  dashboard/src/services/systemClient.ts \
  dashboard/src/components/LoadingSkeleton.tsx \
  dashboard/src/components/SystemStatusView/index.tsx \
  dashboard/src/components/OracleView/V2.tsx \
  dashboard/src/components/SessionPanelV2.tsx \
  dashboard/src/components/SessionItemV2.tsx \
  dashboard/src/components/SessionGroupV2.tsx \
  dashboard/src/components/BeadsView/IssueDetailModal.tsx \
  dashboard/src/App.tsx \
  dashboard/src/components/TerminalArea.tsx \
  dashboard/src/components/TerminalWindow.tsx \
  dashboard/src/components/FilesView/index.tsx \
  dashboard/src/components/SessionItem.tsx \
  dashboard/src/components/TabBar.tsx \
  dashboard/src/components/BeadsView/index.tsx \
  dashboard/src/components/BeadsView/IssueCard.tsx \
  dashboard/src/components/BeadsView/KanbanView.tsx \
  dashboard/src/components/BeadsView/hooks.ts \
  dashboard/src/components/BeadsView/types.ts \
  dashboard/src/styles/system-status.css \
  dashboard/src/styles/session-panel.css \
  dashboard/src/styles/beads.css \
  dashboard/src/styles/file-browser.css \
  dashboard/src/styles/terminal.css \
  dashboard/src/styles/base.css \
  dashboard/src/styles/index.css \
  src/internal/api/system.go \
  src/internal/api/system_test.go \
  src/internal/api/beads.go \
  src/internal/api/beads_test.go \
  src/cmd/server/main.go \
  src/internal/dashboard/dist
git diff --cached --stat
git diff --cached --check
if git grep -i 'gascity\|gas city\|GasCity' -- dashboard/src src/internal src/cmd; then
  echo "ERROR: Gas City reference found in staged application files" >&2
  exit 1
fi
git commit -m "Restore feature-flagged dashboard UI batch (refit button, files durability, V2, system status) without Gas City"
git push -u origin feat/restore-dashboard-ui-batch
gh pr create --repo Perttulands/CHROTE --base main --head feat/restore-dashboard-ui-batch --draft \
  --title "Restore dashboard UI batch (non-Gas-City)" \
  --body "Restores abandoned cc8cb6b UI batch onto main without Gas City. Flags restored: terminalRefitButton/filesPersistTabState/filesResizablePreview/serverStatusTab/beadsAllStatuses/beadsDetailModal/sessionLocationBadges on; uiV2 off. Beads comments are intentionally write-capable through CHROTE's Beads API."
```
Update bead `home-hjfi` with outcome, rollback flags, and smoke‑check results.

---

## Feature‑flag defaults (`dashboard/src/featureFlags.ts`, from `cc8cb6b`)

| Flag | localStorage key | Default | Effect |
|---|---|---|---|
| `terminalRefitButton` | `chrote-terminal-refit-button` | on | shows `↻` refit button |
| `filesPersistTabState` | `chrote-files-persist-tab-state` | on | Files tab kept mounted across tab switches |
| `filesResizablePreview` | `chrote-files-resizable-preview` | on | draggable + persisted preview width |
| `serverStatusTab` | `chrote-server-status-tab` | on | System Status tab visible |
| `beadsAllStatuses` | `chrote-beads-all-statuses` | on | Beads issue list can load all statuses |
| `beadsDetailModal` | `chrote-beads-detail-modal` | on | issue cards open detail/comment modal |
| `sessionLocationBadges` | `chrote-session-location-badges` | on | session cards show location/workspace badges |
| `uiV2` | `chrote-ui-v2` | **off** | V2 session sidebar (opt‑in) |

## Rollback

Per‑feature, no redeploy (value `0`/`false`/`off` disables):
```js
localStorage.setItem('chrote-terminal-refit-button','0'); location.reload()
localStorage.setItem('chrote-files-persist-tab-state','0'); location.reload()
localStorage.setItem('chrote-files-resizable-preview','0'); location.reload()
localStorage.setItem('chrote-server-status-tab','0');     location.reload()
localStorage.setItem('chrote-beads-all-statuses','0');    location.reload()
localStorage.setItem('chrote-beads-detail-modal','0');    location.reload()
localStorage.setItem('chrote-session-location-badges','0'); location.reload()
localStorage.setItem('chrote-ui-v2','0');                 location.reload()
```
Full rollback: `git switch main` → rebuild/embed/restart, or redeploy the previous `chrote-server`.
Work is isolated on `feat/restore-dashboard-ui-batch`; nothing on `main` is force‑changed.

## Risks / caveats

- **Large surface (~30 files + a Go endpoint + 1 dependency).** `tsc` is the safety net — exactly what
  would have failed `3f3c827`; sourcing from `cc8cb6b` avoids the dangling imports.
- **`uiV2` default off** → restoring V2 sidebar code does not change the visible sidebar until flipped.
- **`go test ./...`** now includes `system_test.go`/`beads_test.go`; environment‑tied failures are real
  signals — fix/narrow, don't skip silently.
- **Git hygiene:** commit only task files; do not sweep parent `/home/perttu` repo or CHROTE drift.
- **`system.go`** shells out for process/disk info; review for input validation before deploy
  (ships unchanged from `cc8cb6b`).

## Critical files reference

- Feature flags: `dashboard/src/featureFlags.ts`
- Refit button: `components/TerminalArea.tsx` (manual `↻` swap), `components/TerminalWindow.tsx`
  (`refitNonce` prop + `triggerFit` effect), `styles/terminal.css`
- Files durability: `App.tsx` (`filesPersistTabState`), `components/FilesView/index.tsx`
  (`chrote.files.previewWidthPercent`), `styles/file-browser.css`
- System Status: `components/SystemStatusView/index.tsx`, `services/systemClient.ts`,
  `src/internal/api/system.go`, `src/cmd/server/main.go`
- V2 sidebar (`uiV2`): `components/Session{PanelV2,ItemV2,GroupV2}.tsx`, `styles/session-panel.css`
- Embed target: `src/internal/dashboard/dist` · Service: `chrote.service` (:8094)
