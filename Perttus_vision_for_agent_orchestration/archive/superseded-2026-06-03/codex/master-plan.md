# CHROTE Agent Orchestration Master Plan

Date: 2026-06-02
Status: planning draft for review
Scope: agent orchestration primitives, file-backed mission definitions, agent-native CLI/API surfaces, and a feature-flagged CHROTE Formations tab

## Success Criteria

This plan is good enough to implement when it lets a future agent:

- build the first orchestration substrate without rereading the whole prototype;
- preserve the core product idea: agents and team leaders create formations and missions, not Perttu manually drawing workflows;
- keep CHROTE modular and reversible through clear feature flags and rollback paths;
- separate durable definitions from runtime state, artifacts, transcripts, and UI layout;
- answer which questions still need Perttu's decision before code starts.

## Source Read

Primary sources:

- `Perttus_vision_for_agent_orchestration/perttus_vision_for_agent_teams_and_orchestration.md`
- `Perttus_vision_for_agent_orchestration/03-formations.js`
- `Perttus_vision_for_agent_orchestration/03-formations.html`
- `PRD.md`
- `docs/CHROTE_VISION.md`
- `docs/meta-harness-desired-state.md`
- `docs/agent-collaboration-primitives.md`
- `docs/gascity-meta-harness-evaluation.md`
- `/srv/context-citadel/docs/VISION.md`

Current CHROTE reality:

- CHROTE has tmux, Oracle/Agents, Files, Beads, Services, System Status, terminal proxy, and dashboard tabs.
- There is no current `teams` runtime package in `src/internal`.
- Context Citadel explicitly treats CHROTE orchestration as out of scope for Citadel itself. Use it for context retrieval/contribution, not as the orchestration store.

## Core Interpretation

The system being designed is an agent organization layer, not a dashboard workflow builder.

The important product idea is that Perttu talks to the Archon or a team leader, and those agents discover capabilities, assemble formations, create missions, wire gates, and coordinate work. The human should not be expected to hand-build the system in a UI.

The prototype is still valuable. It identifies the right primitives and inspection surface:

- Mission: entry point and objective.
- Formation: coordination unit.
- Slot: role/reference to an agent or session.
- Gate: pass/fail checkpoint between formations.
- Verification: local checkpoint inside a formation.
- Connection: port-addressed edge between nodes.
- Run: async cascade of work, events, outputs, gates, artifacts, diffs, and reports.

But the prototype should not become the backend schema or primary authoring model. Its own header says the real definitions live as files on disk and are primarily authored by agents through a command line interface.

## Resolved Conflicts

### New Tab Versus "Do Not Build Dashboard First"

The interview says first-stage visibility should be conversational, not dashboard-first. The prototype asks for a Formations tab.

Decision: build the file/CLI substrate first. Then add a feature-flagged Formations tab that reads and explains the system. The tab is an inspection and light-tweak surface, not the authoring center.

### Gas City Sidecar Versus First-Principles CHROTE Primitives

Older CHROTE docs treat Gas City as a plausible sidecar runtime. The new vision says the Gas City experiment failed as a destination, but some primitives were useful.

Decision: start CHROTE-native for the first slice: file-backed mission definitions, run ledgers, registry, and explicit adapters. Keep Gas City as reference material and optional future adapter/sidecar only after the native contract is clear.

### Beads Versus Notice Board

Beads already tracks work. The notice board is team communication.

Decision: missions may link to Beads and create child Beads for real work. Do not turn Beads into the graph store. Do not make the notice board another issue tracker.

### Feature Flags

Dashboard localStorage flags are useful for UI rollout but insufficient for backend mutation safety.

Decision: use both:

- frontend flags for tab and UI controls;
- server/env flags for API exposure and mutating behavior.

## Recommended Architecture

Layer 1: canonical files

- Store definitions under a workspace-local path, recommended `.chrote/orchestration/`.
- Definitions are the source of truth for missions, formations, gates, slots, ports, and connections.
- Runs are append-only ledgers produced from definition snapshots.
- UI edits must be patch-style updates that preserve unknown fields.

Layer 2: agent-native CLI

- Provide a host-side CLI for agents and humans.
- The CLI authors and validates mission files.
- It should support JSON output by default for agent use.
- It should not require browser interaction.

Layer 3: Go backend package

Add a new package, likely `src/internal/orchestration`, with these internal boundaries:

- `Store`: safe workspace path resolution, atomic file writes, revision checks, schema validation.
- `Registry`: merges persisted profile metadata with live tmux/Oracle sessions.
- `Adapters`: tmux, Beads, artifact files, and later harness-specific adapters.
- `Runs`: run snapshots, event ledger, status projection, SSE/event streaming.
- `Validator`: graph validation, port references, join readiness, gate policies, and schema version checks.

Layer 4: CHROTE API

Add a separate API family instead of overloading `/api/oracle` or `/api/beads`:

```text
GET  /api/orchestration/health
GET  /api/orchestration/agents
GET  /api/orchestration/missions
POST /api/orchestration/missions
GET  /api/orchestration/missions/{id}
PATCH /api/orchestration/missions/{id}
POST /api/orchestration/missions/{id}/validate
POST /api/orchestration/missions/{id}/runs
GET  /api/orchestration/runs/{id}
GET  /api/orchestration/runs/{id}/events
GET  /api/orchestration/runs/{id}/stream
POST /api/orchestration/runs/{id}/cancel
POST /api/orchestration/runs/{id}/verdicts
```

Start read-only except for validation and explicitly safe draft creation. Add run controls later behind a stricter server flag.

Layer 5: Formations tab

- Add a top-level Formations tab behind `chrote-formations-tab`, default off.
- First version renders existing missions and run state.
- Controls are limited to inspect, open linked terminal/session, open Bead, validate, start/cancel run when enabled, and record explicit human verdicts.
- Full graph authoring remains CLI/agent-owned.

## Data Model V0

Use JSON for v0. It is less pleasant than YAML for humans, but it avoids adding parser dependencies and is safer for exact machine edits. Agents can still generate it cleanly.

Recommended layout:

```text
.chrote/orchestration/
  agents/
    archon.json
    designer-lead.json
  missions/
    mission_<id>.json
  runs/
    run_<id>/
      mission.snapshot.json
      events.jsonl
      artifacts/
  notices/
    mission_<id>.jsonl
```

### Agent Profile

Agent profiles describe durable identity and capability. They do not own live processes.

Fields:

- `id`
- `display_name`
- `kind`: `archon`, `leader`, `specialist`, `reviewer`, `maintainer`, `disposable`
- `capability_tags`
- `personality_tags`
- `focus_tags`
- `default_harness`
- `session_match`
- `source_files`
- `notes`

Live sessions come from tmux/Oracle/adapters and bind to profiles at runtime.

### Mission

Mission is the entry point and durable definition.

Fields:

- `schema_version`
- `id`
- `title`
- `objective`
- `workspace`
- `owner_bead_id`
- `status`: `draft`, `validated`, `ready`, `running`, `succeeded`, `failed`, `blocked`, `canceled`, `archived`
- `nodes`
- `connections`
- `layout`
- `created_by`
- `updated_by`
- `revision`

### Formation Node

Formation is a concrete mission node. Reusable templates can come later.

Fields:

- `id`
- `type`: `solo`, `peer`, `flow`, `orchestrated`
- `title`
- `brief`
- `inputs`
- `outputs`
- `slots`
- `verification`
- `bead_refs`
- `artifact_expectations`

### Slot

Fields:

- `id`
- `label`
- `controller`
- `agent_profile_id`
- `session_binding`
- `required_capabilities`
- `optional`

An agent can appear in many slots. Slot placement is a reference, not ownership.

### Gate

Fields:

- `id`
- `title`
- `criterion`
- `kinds`: `code`, `human`, `formation`
- `judge_formation_id`
- `on_fail`: `block`, `pushback`, `route`
- `pass_port_id`
- `fail_port_id`

Gate verdicts belong to runs, not reusable gate definitions.

### Verification

Fields:

- `id`
- `criterion`
- `kinds`
- `on_fail`: `block`, `pushback`

Verification is local to a formation and runs after formation work finishes.

### Connection

Fields:

- `id`
- `from_node_id`
- `from_port_id`
- `to_node_id`
- `to_port_id`

Port IDs are stable. Canvas lanes and wire shapes are layout-only.

### Run

At start, create an immutable mission snapshot and append events to `events.jsonl`.

Run events should include:

- `run_started`
- `node_waiting`
- `node_started`
- `node_output`
- `gate_evaluating`
- `gate_verdict`
- `verification_verdict`
- `artifact_attached`
- `notice_posted`
- `human_input_requested`
- `human_verdict_recorded`
- `run_blocked`
- `run_canceled`
- `run_failed`
- `run_succeeded`

Runtime status is projected from the event log. Do not mutate the mission definition to store output, timing, reports, diffs, or verdicts.

### Notice Board

Use JSONL per mission for communication and shared awareness.

This is not the issue tracker. Use Beads for work items and blockers.

Notice fields:

- `id`
- `mission_id`
- `run_id`
- `sender`
- `audience`
- `kind`: `status`, `question`, `decision_needed`, `risk`, `opportunity`, `handoff`
- `body`
- `refs`
- `created_at`

## CLI Surface V0

Prefer `chrotectl` unless a broader `chrote` CLI already exists by implementation time. `chrotectl` avoids confusing the operator CLI with `chrote-server`.

Candidate commands:

```bash
chrotectl agents list --json
chrotectl agents inspect archon --json

chrotectl mission init "Improve session search" \
  --workspace /workspace/chrote \
  --bead home-abc.1 \
  --json

chrotectl mission validate mission_123 --json
chrotectl mission diff mission_123
chrotectl mission patch mission_123 --ops patch.json --json

chrotectl formation add mission_123 \
  --type orchestrated \
  --title "Implement and review" \
  --slot controller=codex \
  --slot reviewer=claude-code \
  --input brief \
  --output patch \
  --json

chrotectl gate add mission_123 \
  --after formation_impl \
  --kind code \
  --command "cd src && go test ./..." \
  --on-fail pushback:formation_impl \
  --json

chrotectl connect mission_123:out formation_impl:input --json
chrotectl connect formation_impl:patch gate_tests:in --json
chrotectl connect gate_tests:pass formation_review:input --json

chrotectl run start mission_123 --json
chrotectl run watch run_456 --jsonl
chrotectl run cancel run_456 --reason "superseded by user direction" --json
chrotectl run verdict run_456 gate_human --pass --note "approved by Perttu" --json
```

CLI rules:

- Use JSON output for all agent-facing commands.
- Validate workspace roots before reading/writing.
- Write atomically: temp file, fsync where practical, rename.
- Refuse to overwrite a changed mission unless the caller passes the current revision.
- Preserve unknown fields during patch operations.
- Never shell-interpolate user-provided IDs, commands, paths, Beads IDs, or comments.

## Dashboard Tab Plan

Feature flags:

- `formationsTab`: localStorage key `chrote-formations-tab`, default off.
- `formationsRunControls`: localStorage key `chrote-formations-run-controls`, default off.
- `formationsEditControls`: localStorage key `chrote-formations-edit-controls`, default off.

Server flags:

- `CHROTE_ORCHESTRATION_ENABLED=0|1`
- `CHROTE_ORCHESTRATION_MODE=off|read_only|drafts|mutating`
- `CHROTE_ORCHESTRATION_ROOT=.chrote/orchestration`

Rollback examples:

```js
window.chroteFeatureFlags.disable('formationsTab')
window.chroteFeatureFlags.disable('formationsRunControls')
window.chroteFeatureFlags.disable('formationsEditControls')
location.reload()
```

Server rollback:

```bash
CHROTE_ORCHESTRATION_MODE=off
systemctl --user restart chrote.service
```

Only use the service restart rollback during a planned deployment window. Do not kill tmux sessions.

Initial tab capabilities:

- Mission list with status, owning Bead, last run, and revision.
- Read-only graph/canvas from mission definitions.
- Run event timeline.
- Agent roster from registry plus live Oracle/tmux status.
- Node details panel: brief, slots, inputs, outputs, gate criteria, linked Beads, artifacts.
- "Open terminal", "Open Bead", "Open file/artifact" links.
- Clear disabled states when backend flag is off.

Later tab capabilities:

- Start/cancel run.
- Record human verdict on a gate.
- Rename mission/node.
- Apply small patch operations generated by the CLI or an agent.
- Rewire only through patch operations and revision checks.

Do not ship browser-first graph creation in v0.

## Implementation Phases

### Phase 0: Decision Pass

Output:

- This plan reviewed.
- Questions in `open-questions.md` answered or intentionally deferred.
- Beads epic/tasks created from the final chosen architecture.

Verification:

- No code changed.
- Plan and Beads agree.

### Phase 1: File Contract And CLI Skeleton

Build:

- `src/internal/orchestration` store and validator.
- JSON schema structs or typed validation helpers.
- `chrotectl` with `agents list`, `mission init`, `mission validate`, `mission diff`.
- Sample mission generated from the prototype concepts.

Flags:

- Backend API still off.
- No dashboard tab yet.

Verification:

- `cd src && go test ./...`
- CLI validates good sample and rejects broken ports, duplicate IDs, bad roots, and unknown node references.

### Phase 2: Read-Only API And Tab

Build:

- `/api/orchestration/health`
- `/api/orchestration/agents`
- `/api/orchestration/missions`
- `/api/orchestration/missions/{id}`
- Formations tab behind `chrote-formations-tab`, default off.

Verification:

- `cd src && go test ./...`
- `cd dashboard && npm run test:unit && npm run build`
- Tab hidden by default.
- Enabling flag shows read-only mission/sample state.
- Disabling flag removes the tab after reload.

### Phase 3: Run Ledger Without Autonomous Routing

Build:

- `run start` creates a mission snapshot and append-only event log.
- API can read run status and events.
- UI can stream/project run status.
- Manual event injection for local testing is allowed only through test fixtures or CLI dev mode.

Verification:

- Event projection tests.
- Cancel state tests.
- Run snapshot immutability tests.

### Phase 4: Explicit Human And Agent Actions

Build:

- Narrow run controls behind backend `mutating` mode and frontend control flags.
- `cancel`
- `human verdict`
- `notice post`
- bounded `patch mission` with revision check.

Verification:

- Audit/event log records every mutation.
- UI cannot mutate when server is read-only.
- Unknown mission fields survive patch operations.

### Phase 5: Adapter-Based Execution

Build one adapter at a time:

- tmux session binding and safe nudge.
- Beads reference creation only after the open Beads argv confinement issue is fixed.
- Code gate command runner with explicit allowlist/config.
- Artifact store with allowed-root validation.
- Optional harness adapters for Codex, Claude Code, Pi, OpenCode, Hermes.

Verification:

- No hidden prompt injection.
- No generic command execution endpoint.
- Adapter tests cover dead session, wrong workspace, canceled run, and unavailable harness.
- Live CHROTE/tmux smoke before deployment.

### Phase 6: Gas City Or External Sidecar Evaluation

Only after CHROTE-native mission/run contracts exist:

- Evaluate Gas City as an adapter or sidecar.
- Start read-only.
- Proxy only through CHROTE-owned API.
- Do not expose raw supervisor ports.
- Do not connect paid/real harnesses until transcript, environment, and audit boundaries are proven.

## Safety Gates

Before implementation:

- Check `systemctl --user status chrote.service` if touching runtime behavior.
- Do not restart or kill live tmux sessions without explicit approval.
- Keep unrelated dirty files untouched.

Before deployment:

```bash
systemctl --user status chrote.service --no-pager
curl -sS http://127.0.0.1:8094/api/health
curl -sS http://127.0.0.1:8094/api/tmux/sessions
curl -sS http://127.0.0.1:8094/api/oracle/status
curl -sS http://127.0.0.1:8094/api/beads/health
```

Build/test:

```bash
cd /workspace/chrote/src && go test ./...
cd /workspace/chrote/dashboard && npm run test:unit && npm run build
```

If frontend is embedded:

```bash
cd /workspace/chrote
find src/internal/dashboard/dist -mindepth 1 -delete
cp -a dashboard/dist/. src/internal/dashboard/dist/
find src/internal/dashboard/dist -type d -empty -delete
cd src && go test ./...
```

Rollback:

- UI rollback: localStorage flags off plus reload.
- Backend rollback: server env mode `off` or binary/dist restore during a planned service restart.
- Never roll back by manipulating tmux sessions.

## First Implementation Recommendation

Build the smallest real slice:

1. JSON file contract under `.chrote/orchestration`.
2. `chrotectl mission init/validate/diff`.
3. Read-only `/api/orchestration/missions`.
4. Feature-flagged Formations tab that renders a sample mission from disk.

Do not build autonomous routing, browser graph authoring, Gas City sidecar control, Beads mailbox, or harness spawning in the first slice.

This proves the critical contract: agents can create the structure natively, and CHROTE can inspect it without becoming the authoring tool.
