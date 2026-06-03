# Subagent Review Synthesis

Date: 2026-06-02
Status: read-only review synthesis

Five parallel review lanes were used:

1. Prototype and vision artifact.
2. Current CHROTE dashboard/UI integration.
3. Backend/API/storage architecture.
4. Agent-native CLI and lifecycle surfaces.
5. Rollout safety and feature-flag strategy.

No subagent edited files.

## Shared Conclusions

The strongest product signal is that formations and missions are agent-authored through files and CLI operations. The UI is an inspection and light-tweak surface.

The right first slice is not a full scheduler, not Gas City adoption, and not a browser workflow builder. It is a durable file contract plus CLI/API validation that CHROTE can render.

Stable identity is essential:

- node IDs;
- port IDs;
- edge IDs;
- slot IDs;
- run IDs;
- artifact refs;
- profile IDs.

Definitions and runtime state must be separate. Mission files describe what should happen. Run ledgers record what did happen.

Agents are live/discovered participants, not objects created by the dashboard. Persist profiles, capabilities, and slot references. Bind to tmux/Oracle/harness sessions at runtime.

Beads should remain work tracking. A notice board is communication. Context Citadel is context memory, not the orchestration store.

## Lane 1: Prototype And Vision

Preserve:

- agent organization over workflow automation;
- Archon as main touchpoint;
- team leaders as access points into grouped capability;
- persistent specialists for taste, continuity, standards, and stewardship;
- progressive discovery of agents and capabilities;
- Beads for work tracking and a separate notice board for shared awareness;
- prototype primitives: Mission, Formation, Slot, Gate, Verification, Connection, Run.

Do not copy directly:

- mocked roster;
- mocked terminal feed;
- seed graph;
- generated IDs from JS arrays;
- `setTimeout` run simulation;
- hard-coded gate verdicts;
- browser canvas as primary authoring tool.

## Lane 2: Dashboard Integration

Recommended integration points:

- `dashboard/src/featureFlags.ts` for frontend flags.
- `dashboard/src/components/TabBar.tsx` for the tab union and conditional tab entry.
- `dashboard/src/App.tsx` for mounting the view behind an `ErrorBoundary`.
- `dashboard/src/services/` for typed API clients.
- `dashboard/src/styles/index.css` plus a dedicated CSS file for the new view.

Default UI flag should be off:

```js
window.chroteFeatureFlags.disable('formationsTab')
location.reload()
```

Use the Server/System Status tab pattern if state should stay warm while hidden. Use Beads/Agents/Services conditional rendering if reset-on-switch is acceptable.

## Lane 3: Backend/API/Storage

Current backend shape:

- `src/cmd/server/main.go` wires handlers.
- tmux state is owned by `src/internal/api/tmux.go`.
- agent observability is inferred through `src/internal/api/oracle.go`.
- Beads, Files, Services, System, and terminal proxy are separate handlers.
- No current `teams` runtime package exists in `src/internal`.

Recommended new boundary:

- `src/internal/orchestration`
- `src/internal/api/orchestration.go`

Backend components:

- file-backed store;
- registry merging profiles and live sessions;
- tmux, Beads, and artifact adapters;
- append-only run ledger;
- graph/schema validator.

Important constraints:

- reuse existing tmux handler/environment path instead of spawning tmux independently;
- validate roots with the same seriousness as the Files API;
- do not build a Beads mailbox until Beads argv confinement is fixed;
- keep artifacts outside graph JSON;
- preserve unknown fields during UI patch operations.

## Lane 4: CLI/API Surface

The CLI should be host-side and agent-friendly.

Recommended command groups:

- `agents list`
- `agents inspect`
- `mission init`
- `mission validate`
- `mission diff`
- `mission patch`
- `formation add`
- `gate add`
- `connect`
- `run start`
- `run watch`
- `run cancel`
- `run verdict`

API should be a separate `/api/orchestration` family. Do not overload `/api/oracle` or `/api/beads`.

Lifecycle recommendation:

- Mission: `draft -> validated -> ready -> running -> succeeded | failed | blocked | canceled -> archived`
- Formation run state: `waiting_inputs -> runnable -> assigned -> running -> done | needs_review | blocked | failed`
- Gate run state: `pending -> evaluating -> pass | fail | blocked`

## Lane 5: Rollout Safety

The safety reviewer focused on live CHROTE runtime boundaries. The exact assumption was broader than this planning task, but the gates apply.

High-risk boundaries:

- live tmux sessions;
- ttyd;
- terminal iframe lifecycle;
- terminal proxy WebSocket behavior;
- service restart;
- Beads mutations;
- file/root policy.

Concrete gates:

```bash
systemctl --user status chrote.service --no-pager
curl -sS http://127.0.0.1:8094/api/health
curl -sS http://127.0.0.1:8094/api/tmux/sessions
curl -sS http://127.0.0.1:8094/api/oracle/status
curl -sS http://127.0.0.1:8094/api/beads/health
cd /home/perttu/chrote/src && go test ./...
cd /home/perttu/chrote/dashboard && npm run test:unit && npm run build
```

Backend behavior-risk flags must be server/env controlled. localStorage flags are fine for UI visibility but not enough for mutation safety.

Do not restart or kill tmux sessions as part of normal verification.

## Cross-Lane Risks

1. The prototype's visual richness may tempt a dashboard-first build. That would violate the product intent.
2. Older Gas City docs may tempt a sidecar-first build. That would delay the native contract and import an external worldview too early.
3. Beads mailbox ideas are attractive but currently unsafe to prioritize while Beads argv confinement remains open.
4. Current dashboard auth is not enough to treat browser mutation as low risk. Mutating orchestration endpoints need explicit server mode and audit.
5. A "formation" could mean both template and concrete node. V0 should use it only for concrete mission nodes.
6. Full transcript storage has privacy and retention questions. V0 should not silently persist every terminal transcript.

## Recommended Next Move

Accept or revise the decisions in `open-questions.md`.

Then create an implementation Beads epic with these first children:

- Define orchestration file contract and validator.
- Build `chrotectl` mission skeleton commands.
- Add read-only orchestration API.
- Add feature-flagged Formations tab.
- Add run ledger projection.

Do not start with autonomous agent routing, broad browser authoring, Gas City sidecar control, or real harness spawning.
