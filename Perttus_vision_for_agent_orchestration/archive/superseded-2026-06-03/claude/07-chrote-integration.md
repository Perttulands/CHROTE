# 07 · CHROTE Integration — API, Formations Tab, Feature Flags

How the prototype becomes a real (read-mostly) tab in the existing cockpit, reversibly.

> **Decisions applied (Perttu — see [08](08-open-questions.md)):** the CLI is **`archon`** (read
> `archon` wherever this doc shows `fm`); the run engine is **hosted in this server's formations
> system** (CLI + UI are both clients); the UI is built **incrementally via vertical full-stack
> slices** from slice 1 — not deferred to a later phase.

---

## 1. Mock → backend mapping

The prototype names 6 mocks. Each maps to a real CHROTE backend; the table separates *definition*
(persisted, round-trips) from *run state* (ephemeral, from the ledger/SSE):

| # | Mock | Real backend | Endpoint | Note |
|---|---|---|---|---|
| 1 | `AGENTS[]` roster + portraits | existing Oracle, left-joined with persona cards | `GET /api/oracle/agents` (reuse) → later `/api/agents` | `{id,role,av,state}`; `role`/`av` from persona cards, fallback to session name. |
| 2 | `feed()` terminal popups | **ttyd via the existing path** | `/terminal/?arg=<session>` | The popup hosts a real `<iframe>`; `?arg=` maps 1:1 to a tmux session. **Requires `slot.agentId` == tmux session name** (the "one id" spine). |
| 3 | `seed()` graph | the file-backed TOML board, composed server-side | `GET /api/formations/boards/{id}` | Server transcodes TOML→the prototype's JSON shape (like `transformIssue`). Replace `seed()` with `fetch`. |
| 4 | `mockReport()` | run ledger (last output per node) | hydrated into the board read / `…/runs/{id}` | Shape already matches `f.output`. Server pre-splits diffs into `lines[]`. |
| 5 | run timings (`setTimeout`) | real SSE run events | `GET /api/formations/boards/{id}/stream` | An event reducer sets the same `_state`/`output`/`flowing` fields + `rerender()`. The browser renders events; it never fabricates run state. |
| 6 | hard-coded verdicts | real gate results | via SSE `gate.verdict` + ledger | Gate *definition* (kinds/criterion) round-trips in TOML; *verdict* is run state. |

Cross-cutting: the prototype's client-side `s1/f1/g1` ids must become **opaque server-owned stable
ids**; the frontend never regenerates them. Edges gain a stable `id` so DELETE/PATCH are addressable.

---

## 2. Go API surface

New `src/internal/api/formations.go`, `FormationsHandler`, structurally a clone of `beads.go`
(transform-on-read, allowed-roots validation, `core.WriteSuccess/WriteError`, env config). Registered
**only when enabled** so flag-off = zero new routes and zero background goroutines:

```go
if api.FormationsEnabled() {                  // CHROTE_FORMATIONS, default off
    api.NewFormationsHandler(oracleHandler).RegisterRoutes(mux)   // reuse Oracle for the roster
}
```

Route prefix **`/api/formations/*`** — not `/api/orchestration`. (The `Agents=/api/oracle` compat name
exists only for historical reasons; there's no such legacy here — name it for the product.)

```
# v1 — read-only (the minimal first CHROTE slice)
GET  /api/formations/health
GET  /api/formations/boards
GET  /api/formations/boards/{id}                 # composed board JSON + ETag header

# v2 — light tweaks (field-level, by-id; If-Match etag; server-owned writer — see §3)
PATCH  /api/formations/boards/{id}/nodes/{nodeId}        # rename / setBrief / setCriterion / move
POST   /api/formations/boards/{id}/connections          # add edge
DELETE /api/formations/boards/{id}/connections/{edgeId}

# v3 — run + live events
GET  /api/formations/boards/{id}/stream          # SSE: board.changed + node.* + gate.* + run.*
POST /api/formations/boards/{id}/runs            # launch from {missionId|nodeId}
GET  /api/formations/boards/{id}/runs/{runId}    # ledger replay (catch-up)
DELETE /api/formations/boards/{id}/runs/{runId}  # cancel
```

The board read composes definition + layout + the latest finished run's per-node output, hydrates
`beadId` via `bd show --json`, and left-joins the live roster — one JSON response, the prototype's
exact shape.

---

## 3. Write-back / round-trip

The human-tweak set is tiny and is applied as **field-level edits by id**, never a canvas serialize:
the `PATCH` body is an allowlisted op list (`rename`, `move`, `setBrief`, `setCriterion`); `move`
touches only the **layout sidecar** (so dragging never risks the definition). Concurrency: `If-Match`
etag (mtime+hash) → `409` on mismatch, UI rebases; per-board file lock; atomic temp-rename.

**Single writer:** to avoid two serializers fighting, the **server's formations package is the one
writer of definition files.** Both the UI's `PATCH` and the `archon` CLI go through it (the CLI is a
client of `/api/formations/*`), so "UI op == CLI op" is literally true and there is no format drift.
**v1 ships read-only**, so no writer is exercised yet. External `archon`/agent edits reflect via an SSE
`board.changed` (mtime/fsnotify) → the UI refetches and `rerender()`s (or a manual Refresh button in
v1).

---

## 4. Frontend

Lives in `dashboard/src/components/FormationsView/` (`index.tsx` shell, `canvas.ts` ported logic,
`api.ts`, `types.ts`, a CSS module from the prototype's `<style>`).

**Port, don't rewrite.** The prototype is ~1400 lines of finely-tuned canvas math (pan/zoom,
Liang–Barsky obstacle-aware wire routing, pointer-drag wiring, undo). Rewriting it as idiomatic React
is multi-week work that injects bugs into working geometry for zero benefit on an inspection surface.
Instead: one React component owns a `<div ref>`; a `useEffect` mounts `mountFormations(host, deps)`
and returns `{destroy}`. Mechanical changes only: `document.getElementById` → `host.querySelector`
(scoped); `seed()` → `await api.getBoard()`; terminal popups → a standalone `<iframe src="/terminal/?arg=…">`;
run `setTimeout` theatre → the SSE reducer (v3); **scope/remove global listeners on `destroy()`**
(critical — leaked global pointer/key listeners are exactly what breaks CHROTE's tab/iframe lifecycle).

**Tab wiring** (mirror the existing `serverStatusTab` pattern exactly):

- `featureFlags.ts`: add `formationsTab: 'chrote-formations-tab'` to `FEATURE_FLAGS` and
  `formationsTab: false` to `DEFAULT_ENABLED` (default off, like `uiV2`).
- `TabBar.tsx`: add `'formations'` to the `Tab` union and a gated entry:
  `...(isFeatureEnabled('formationsTab') ? [{ id:'formations' as const, label:'Formations' }] : [])`.
- `App.tsx`: render gated, **kept-mounted** (`display:none` not unmount, like the terminals) so the
  canvas + iframes survive tab switches; wrap in `ErrorBoundary` so a canvas crash never takes down
  the dashboard.

Reuse: copy `useOracleStream` (`OracleView/hooks.ts`) → `useFormationsStream`; the
`result.success && result.data` fetch envelope; `ErrorBoundary`.

---

## 5. Feature flags & reversibility

Three independent gates, all default-off, each flippable alone:

| Layer | Mechanism | Kill switch | Effect |
|---|---|---|---|
| Backend routes | `CHROTE_FORMATIONS` env (read in `main.go`) | unset + restart | no `/api/formations/*`, no background goroutine. Identical to today. **Master switch.** |
| CLI | separate `archon` binary | don't build / `rm` | the CLI is unavailable; the hosted engine is gated by the backend row above. |
| UI tab | `chrote-formations-tab` localStorage flag | default `0` / `chroteFeatureFlags.disable(...)` | no tab; `FormationsView` never mounts. |
| Data | `<project>/.formations/` + `~/agents/` | `rm -rf .formations/` | workspace, code, sessions, `.beads/` untouched. |

Note one deliberate divergence from `oracle.go` (which `startPoller()` unconditionally in its
constructor): `FormationsHandler` must start any watcher/goroutine **only when enabled**, so flag-off
means truly zero new background activity. Enabling Formations changes **no existing default**. Rollout:
(1) read-only backend + tests → (2) tab + canvas reading one board **[ship: minimal slice]** → (3) SSE
`board.changed` → (4) write-back (server-owned writer) + etag → (5) run launch + live run SSE.

---

## 6. Build / deploy & the SSE timeout

Standard CHROTE flow (`CLAUDE.md` / `chrote-ui-work`):

```bash
cd dashboard && npm run build
cd .. && rm -rf src/internal/dashboard/dist && cp -r dashboard/dist src/internal/dashboard/dist
cd src && go test ./... && go build -o ../chrote-server ./cmd/server
systemctl --user restart chrote.service
```

The dashboard is embedded via `//go:embed dist/*` with SPA routing, so the new tab needs no server
route (it's client state). **The 30 s `WriteTimeout` vs long-lived SSE:** clear the deadline
per-connection at the top of the stream handler with
`http.NewResponseController(w).SetWriteDeadline(time.Time{})` — already proven in `services.go:374`.
Keep a 30 s heartbeat. Everything binds `127.0.0.1` and rides the existing auth+CORS middleware —
nothing new to secure.

Smoke checks: `systemctl --user status chrote.service`; `curl /api/formations/health` (on → ok, off →
404); `curl /api/formations/boards | jq`; `curl -N …/stream` stays open > 30 s; in the UI, enable the
flag, confirm the board renders, switch tabs and back (canvas survives), no console errors.

---

## 7. Minimal first slice (CHROTE side)

**Read-only inspection of one file-backed board, behind both flags. No write-back, no run engine, no
live streaming.** `formations.go` (beads.go-sized: health, list, get-board — compose definition +
layout + latest run output + left-joined roster); the `chrote-formations-tab` flag + tab wiring;
`FormationsView` mounting the ported canvas in **read-only mode** (hide run/edit affordances to avoid
implying persistence); terminal popups = standalone iframes; `ErrorBoundary`-wrapped; global listeners
scoped + removed on unmount.

Effort: backend ~1 day (a `beads.go` clone), canvas port ~2–3 days (mechanical scoping + `seed`→`fetch`
+ lifecycle cleanup; the look/interaction is already built). **This read-only tab _is_ slice 1**
(Perttu's vertical-slicing decision — [08 Q2](08-open-questions.md)): it ships together with the
`archon formation create/list/inspect` CLI and the board file format, so the file↔CLI↔UI round-trip is
proven end-to-end in the first slice. Write-back (slice 2) and the cross-harness run + live SSE
streaming (slice 5) layer on top without reworking this foundation.
