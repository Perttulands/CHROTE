# CHROTE Formations - Scenario To Slice Map

This is the S0 acceptance map for `home-7kc4.1`. It pins how the
behavioral spec in this directory is divided across the Formations child beads.

Authoritative sources:
- `../DECISIONS-LOCKED.md` wins over sibling docs.
- `../03-formations.html` and `../03-formations.js` are the canonical canvas
  behavior and visual reference. The React UI must be implemented fresh, not by
  porting the vanilla JS.
- `contracts.md` is the active schema, addressing, API, and write contract.
- Beads tracks work only. Formations graph state lives in files under
  `.formations/` and persona cards under `~/agents/`.

## Slice Ownership

| Slice | Bead | Primary files/scenarios |
|---|---|---|
| S0 - baseline | `home-7kc4.1` | The whole `Perttus_vision_for_agent_orchestration/` packet, excluding `*:Zone.Identifier`; this map; D1-D7; feature index and active contracts. |
| Foundations | `home-7kc4.2` | `reversibility-persistence.feature`; `contracts.md` file layout, write rules, API gating, rev/ETag, schema versioning, definition/layout/run separation; `visibility.feature` default-off Formations tab scenario. |
| S1 - agents | `home-7kc4.3` | `agents.feature`; `agent-factory.feature`; `contracts.md` persona-card schema and one-id spine; agent roster/menu actions from `context-menus.feature`. |
| S2 - formation authoring | `home-7kc4.4` | Formation creation, types, default slots, board load/save, layout, canvas pan/zoom/fit/arrange, node move as layout-only, formation inspect/list/create, and the non-run visual canvas reference from `canvas.feature` and `formations-and-slots.feature`. |
| S3 - mission graph authoring | `home-7kc4.5` | Agent slot assignment, slot add/remove/order, brief editing, `connections.feature`, `gates-and-judges.feature`, `verification.feature` authoring, `mission.feature`, context menus for mutating graph elements, and undo for structural mutations. |
| S4 contract pinning | `home-7kc4.6` | The `contracts.md` "Deferred From S0" runtime edges: `run_blocked` epoch/resume semantics, full ledger payload schemas, projection mapping, dispatch lease/idempotency, replay reattach-or-error, never blind redispatch, immutable run snapshot. |
| S4 - run engine | `home-7kc4.7` | `run-execution.feature`; `run-recovery.feature` engine/replay scenarios; `terminals.feature`; run streaming scenarios from `visibility.feature`; escalation sentinels from `escalation.feature`; NDJSON ledger and SSE/API run routes from `contracts.md`. |
| S5 - recover and decide | `home-7kc4.8` | Human verdict and decision scenarios from `escalation.feature`; run resume/abort/operator decision surfaces from `run-recovery.feature`; gate approve/reject CLI/API/UI flows. |
| E2E and rollback | `home-7kc4.9` | `career-web-experience.feature`, especially the staffed mission demo and clean rollback; verifies all prior slices behind both feature gates. |

## Scenario Split Notes

- `formations-and-slots.feature` spans S2 and S3. Creation, types, default slot
  shape, controller invariants, and board persistence belong to S2. Assigning
  agents, adding/removing/reordering slots, and brief linkage belong to S3.
- `canvas.feature` spans S2, S3, and S4. Pan/zoom/fit/arrange/layout is S2.
  Undo for graph mutations is S3. On-canvas terminals are S4.
- `context-menus.feature` is implemented with the feature it invokes. Empty
  board and formation creation menus start in S2; assignment, wire, gate,
  verification, mission, and destructive/undoable actions land in S3; live
  terminal or run actions land in S4/S5.
- `briefs-and-io.feature` spans S3 and S4. Brief input editing and bead/file
  references are S3. Runtime output reports, artifacts, diffs, and clearing run
  state without changing definitions are S4.
- `visibility.feature` spans Foundations through S4. The default-off tab gate is
  Foundations, progressive disclosure follows the UI slices, external-change
  reflection depends on the writer/API, and live run streaming is S4.
- `escalation.feature` spans S4 and S5. Escalation sentinel capture and ledger
  events are S4. Human verdict capture, routing, and operator decisions are S5.
- `run-recovery.feature` spans S4 contract pinning, S4 engine implementation,
  and S5 decisions. The lease/idempotency/resume rules must be pinned in
  `home-7kc4.6` before `home-7kc4.7` implements them.

## Baseline File Set

The S0 baseline is the full Formations design packet:

- `../DECISIONS-LOCKED.md`
- `../03-formations.html`
- `../03-formations.js`
- `../03-formations-paper.html`
- `../perttus_vision_for_agent_teams_and_orchestration.md`
- `../assets/*`
- `../archive/superseded-2026-06-03/**`
- `README.md`
- `contracts.md`
- `SCENARIO-SLICE-MAP.md`
- `features/*.feature`

Exclude all `*:Zone.Identifier` files. They are Windows metadata, not
Formations requirements or assets.

## Closure Evidence For S0

Before closing `home-7kc4.1`, collect:

- A stable local reference for the baseline, preferably a commit containing only
  the Formations packet files above and no unrelated staged CHROTE changes.
- `git show --name-only --oneline <baseline-ref> -- Perttus_vision_for_agent_orchestration`
  proves the baseline is referenceable.
- A feature coverage check proves this map names every file in `features/`.
- A staged/commit file check proves no `*:Zone.Identifier` file was included.
- A read-only review confirms S0 did not change runtime code, Beads graph
  storage, CHROTE service state, or tmux sessions.
