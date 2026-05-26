# CHROTE Cleanup TODO

**Created:** 2026-01-18
**Last Updated:** 2026-01-18
**Purpose:** Track all naming inconsistencies and fixes needed after Ralph removal and Arena→CHROTE rename.

---

## Critical (Security)

| File | Issue | Action |
|------|-------|--------|
| `.env` | Tailscale auth key in git history | Rotate key, scrub history with BFG |
| `build1.dockerfile:22` | Hardcoded SSH password `root:root` | Consider key-only auth |

---

## Folder Renames (Blocked - Restart Required)

| Current | Target | Notes | Status |
|---------|--------|-------|--------|
| `AgentArena_go/` | `src/` | Main Go server | ⏳ Pending (close VS Code first) |
| Repo folder `AgentArena/` | `CHROTE/` | Optional - update all paths | Deferred |

---

## Naming: Arena → CHROTE

### Config Files ✅ Complete

| File | Change | Status |
|------|--------|--------|
| `docker-compose.yml` | `tailscale-arena` → `tailscale-chrote`, `hostname: chrote`, `chrote_root_home` | ✅ |
| `.env.example` | Removed filebrowser vars, updated CORS example | ✅ |

### Package Names ✅ Complete

| File | Change | Status |
|------|--------|--------|
| `api/package.json` | `arena-api` → `chrote-api` | ✅ |
| `api/package-lock.json` | `arena-api` → `chrote-api` | ✅ |
| `dashboard/package.json` | `arena-dashboard` → `chrote-dashboard` | ✅ |

### Source Code ✅ Complete

| File | Change | Status |
|------|--------|--------|
| `dashboard/src/context/SessionContext.tsx` | `arena-dashboard-state` → `chrote-dashboard-state` | ✅ |
| `api/beads-routes.js` | `AgentArena` → `CHROTE` | ✅ |
| `api/server.test.js` | `http://arena:8080` → `http://chrote:8080` | ✅ |
| `api/server.js` | Comment + startup log updated | ✅ |
| `api/smoke.test.sh` | Header updated | ✅ |

### WSL Assets ✅ Complete

| File | Change | Status |
|------|--------|--------|
| `wsl/wsl_assets/systemd/*.service` | Renamed to `chrote-*`, user/group → `chrote`, TMUX_TMPDIR → `/run/tmux/chrote` | ✅ |

---

## Documentation Updates ✅ Complete

| File | Status |
|------|--------|
| `README.md` | ✅ Updated for WSL deployment |
| `PRD.md` | ✅ Updated architecture, security model |
| `CLAUDE.md` | ✅ Updated commands, paths |
| `CODE_REVIEW.md` | ✅ Updated status |
| `docs/UX_IMPROVEMENTS.md` | ✅ Renamed AgentArena → CHROTE |
| `docs/GASTOWN.md` | ✅ Already updated |
| `docs/WSL-migration-plan.md` | ✅ Contains CHROTE references |

---

## Dead Code / Unused Files ✅ Complete

| Item | Action | Status |
|------|--------|--------|
| `.claude/ralph-loop.local.md` | Deleted | ✅ |
| `MusicPlayer.tsx` | Already wired in TabBar.tsx | ✅ (was not dead) |

---

## Code Quality Fixes ✅ Complete

| File | Fix | Status |
|------|-----|--------|
| `api/file-routes.js` | Added try-catch around `decodeURIComponent()` | ✅ |

---

## Features to Add

| Feature | Status | Notes |
|---------|--------|-------|
| MusicPlayer in dashboard | ✅ Complete | Already wired into TabBar |
| Toast notifications | Deferred | Per UX_IMPROVEMENTS.md |
| Connection health indicator | Deferred | Per UX_IMPROVEMENTS.md |

---

## Build Checklist

Before WSL deployment:

1. [x] Update `docker-compose.yml` arena→chrote references
2. [x] Update package.json names
3. [x] Update SessionContext storage key
4. [x] Delete `.claude/ralph-loop.local.md`
5. [x] Wire up MusicPlayer (already done)
6. [x] Fix decodeURIComponent error handling
7. [ ] Rename `AgentArena_go` → `src` (after restart)
8. [ ] `cd dashboard && npm run build`
9. [ ] Copy `dashboard/dist/` → `src/internal/dashboard/dist/`
10. [ ] Test all tabs work

---

## Decisions Made

| Question | Decision |
|----------|----------|
| Go server vs Node.js? | **Go is active**, Node.js is deprecated |
| Rename repo folder to CHROTE? | **Deferred** - would require updating all paths |
| WSL migration now or later? | **In progress** - see `docs/WSL-EXECUTION-CHECKLIST.md` |

---

## Remaining Arena References

Some files still contain "arena" in:
- CSS class names (can be left as-is, internal)
- Test file descriptions (non-critical)
- Path strings containing `AgentArena` (will change with folder rename)
- WSL asset files referencing paths under `/mnt/e/Docker/AgentArena`

These are low priority and can be addressed during the folder rename or left as-is.
