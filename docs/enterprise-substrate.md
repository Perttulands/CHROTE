---
type: reference
status: active
authority: source-of-truth
workspace: chrote
enforced_by: scripts/doc-lint.py
---

# Enterprise Substrate (Parked)

CHROTE ships as a **solo, single-operator** product: one trusted operator driving
trusted agents behind a network perimeter (Tailscale / localhost). In that model,
security machinery that isolates users, governs permissions, and audits access is
pure friction — so it is deliberately **off**.

A second point on the axis exists: an **enterprise / multiplayer** mode with real
admin control (multiple users, roles, tenant isolation, audit). The intended
architecture is **one codebase with an enterprise mode toggle** (the GitLab
CE/EE pattern): enterprise features are additive or seam-gated and **default off**,
so the solo user never pays for them.

Substantial run-data governance work toward that mode was built (by the `gpt-5.6-sol`
lane) and is **parked, not lost**. This note is the map back to it.

## Parked work (preserved by exact annotated tags)

| Tag | Branch | What it is |
| --- | --- | --- |
| `enterprise-substrate/run-view-projection` | `feat/formations-run-view` (91 commits) | Canonical **sanitizing run-view projection**: per-consumer data isolation, closed event unions, bounded/paginated + reconnectable transport, artifact authorization with revocation. The run-data **tenant-isolation read boundary**. |
| `enterprise-substrate/workspace-authority` | `feat/formations-workspace-authority` (69 commits) | Schema-2 **workspace authority writer + records**: durable, hash-linked, revision-controlled who-can-act-on-a-run. The **roles/permissions writer** that makes the runtime guard enforce. |

Both were shelved on 2026-07-22 as over-engineered *for the solo product* — the
board already works with the direct projection, and the runtime guard was
neutralized under the trust model. They are the seed of the enterprise mode, not
dead code.

## The toggle seams that already exist in `main`

When the security was trimmed for the solo model, the **seams were kept** so
enterprise mode can be switched on later without re-plumbing:

- **Authority enforcement** — `Store.RequireRuntimeAuthority()`
  (`src/internal/formations/runtime_authority.go`) is wired into all 15 runtime
  effect sites (run start/resume/dispatch/executors/ledger/escalation/archon). It
  currently returns `nil` (authorize) with a trust-model comment; the
  `runtimeAuthorityBoundary` struct, `NewRuntimeStore`, and the
  `RuntimeAuthorityNonAuthorizingError` types are retained. Enterprise mode makes
  this enforce (fed by the parked workspace-authority writer).
- **Sanitizing projection** — the direct board projection is the solo read path;
  the parked canonical run-view projection is the enterprise read path, selected
  by mode.
- **Identity / audit / tenant filtering** — additive: absent in solo mode
  (perimeter is the boundary), added as middleware/filters in enterprise mode.

## Reviving for enterprise mode

1. Create revival branches from the parked tags and refresh them against current
   `main` first — the code **drifts** the longer it sits (that is the cost of
   parking).
2. Introduce an explicit mode flag (env/config), **off by default**.
3. Behind the flag: flip `RequireRuntimeAuthority` to enforce, wire the
   workspace-authority writer, select the sanitizing projection, add identity +
   tenant filtering + audit.
4. Keep every enterprise path gated so the solo user experiences zero change.

The exact annotated tags are the durable preservation refs. Before removing an
old parked branch, verify that its tag peels to the recorded branch tip. The
branch may then be removed; do **not** delete the tags.
