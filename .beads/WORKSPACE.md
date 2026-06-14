---
workspace: chrt
prefix: chrt
root: /home/perttu/chrote/.beads
scope: >-
  CHROTE product Beads workspace. Durable task tracking for the CHROTE cockpit:
  dashboard, Formations, Archon, server/API, terminal/ttyd, Oracle/Agents, Beads
  view, Services, deploy/runtime, and CHROTE code review/test/quality work.
not_for:
  - Personal/home work with no dedicated workspace — use the home workspace at /home/perttu/.beads.
  - /srv service-infrastructure work — use the ctx workspace at /srv/.beads.
  - The /srv/.labs Tower/SHOWCALL lineage — that is separate from the CHROTE cockpit; it stays in home.
---

# CHROTE Beads Workspace

Manifest for the `chrt` Beads workspace. The `bd` issue prefix here is `chrt-`
(e.g. `chrt-vdki`, `chrt-idhj.17`).

## Origin

Created 2026-06-14 by migrating the 271 CHROTE-associated beads out of the
shared `home` workspace (prefix `home-`), so CHROTE Beads state lives and
versions with the CHROTE code instead of borrowing the personal workspace.
IDs preserved their suffixes (`home-vdki` → `chrt-vdki`); six beads orphaned
from non-CHROTE dotted parents were flattened (`home-34h.9` → `chrt-34h9`,
`home-fv6.11..15` → `chrt-fv611..615`).

## Tracked with the repo

`.beads/issues.jsonl` (+ `metadata.json`, `config.yaml`, `README.md`,
`WORKSPACE.md`) are committed to the CHROTE repo; auto-export keeps the JSONL
fresh. The embedded Dolt database and runtime files stay git-ignored
(see `.beads/.gitignore`).

## CLI

`bd` resolves its workspace by directory — the nearest `.beads` up the tree,
stopping at the git-repo root — so a plain `bd` anywhere inside the chrote tree
targets this `chrt` workspace; `/home/perttu` → home, `/srv` → ctx. (The old
`BEADS_DIR=/home/perttu/.beads` pin in `~/.bashrc` was removed 2026-06-14 so this
"just works"; a session started before that change keeps the stale pin until
relaunched.)

See also: `/home/perttu/.beads/WORKSPACE.md` (home) and
`/srv/.beads/WORKSPACE.md` (the `ctx` workspace).
