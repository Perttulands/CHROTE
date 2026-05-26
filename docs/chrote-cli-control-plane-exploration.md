# CHROTE CLI / Control-Plane Exploration (Superseded)

Date: 2026-05-24

> Superseded exploration. This file records a branch of thinking that is no longer the current recommendation. Current CHROTE/Gas City context lives in `docs/chrote-gascity-framing.md`: it describes what Gas City is, how it works, what it appears to afford, and the open problem of how CHROTE can best leverage it. It is not an implementation decision.

## Historical Decision (Superseded)

Build a **CHROTE-native CLI and control plane**. Use Gas City as a reference architecture, spike runtime, and optional adapter — not as CHROTE's foundation, not as a fork, and not as the required source of truth.

Short version:

> CHROTE should own the operator-facing cockpit, CLI, adapter registry, run ledger, event ledger, and safety model. Gas City can remain one orchestrator CHROTE observes and controls through a boundary.

This updates the earlier sidecar-first recommendation in `docs/gascity-meta-harness-evaluation.md` and `docs/gascity-sidecar-spike-results.md`: the sidecar was the right spike and remains useful for learning, but the long-term product direction should not make Gas City mandatory.

## Why this shifted

The earlier evaluation correctly noticed that Gas City is much closer to the desired meta-harness than a casual read suggests. It already has sessions, mail, work routing, formulas/molecules, events, supervisor API, and harness providers.

The deeper pass changes the conclusion for product fit:

1. **Gas City solves orchestration, not CHROTE's whole cockpit problem.** CHROTE also owns browser terminal lifecycle, files, Beads visibility, service proxies, Context API, TTS, tailnet/private access, and generic agent observability.
2. **Gas City's implementation is not currently a clean library dependency.** The good pieces live mostly under Go `internal/`, and the public integration boundaries are CLI/API/config/files.
3. **Gas City's vocabulary would leak a worldview.** City, rig, bead, wisp, molecule, convoy, sling, order are coherent inside Gas City, but CHROTE needs simpler host/operator nouns that work when Gas City is absent.
4. **CHROTE needs adapter neutrality.** Codex, Claude Code, Pi, OpenCode, Hermes, tmux shells, services, and Gas City should be peers behind adapters.
5. **The safety model must be CHROTE-owned.** Browser clients, Discord-driven agents, service tokens, terminal sessions, and destructive actions need one CHROTE policy/audit layer rather than inheriting Gas City's localhost-oriented assumptions.

## Gas City: what it actually affords

Gas City is a mature orchestration-builder CLI/control plane. Its core architecture is better than most agent orchestration code because it keeps infrastructure and role behavior separated.

### Five primitives worth understanding

1. **Session**
   - Durable runtime handle for start/stop/attach/peek/nudge/metadata.
   - Runtime providers include tmux, subprocess, exec, k8s, ACP/auto/hybrid layers.
   - Important lesson: tmux is a provider, not the ontology.

2. **Task Store / Beads**
   - Generic durable work records with ID, status, type, parent, dependencies, labels, assignee, metadata.
   - Messages, molecules, convoys, sessions, and tasks can all be represented as records.
   - FileStore is especially relevant: locked local JSON, atomic writes, simple durability.

3. **Event Bus**
   - Append-only JSONL/event provider with sequence cursors, filters, and Watch/SSE behavior.
   - This is the right shape for UI recovery, CLI tailing, audit, and replay.

4. **Config**
   - TOML composition, packs, provider presets, progressive activation by section presence.
   - Lesson: config presence can activate capability; avoid random feature-flag sprawl.

5. **Prompt Templates**
   - Markdown/go-template behavior lives in config, not hardcoded Go roles.
   - Lesson: Go should transport, reconcile, validate, and audit; prompts/recipes carry role behavior.

### Derived mechanisms worth copying conceptually

- **Mail**: message = durable work record, nudge = live session input.
- **Formulas / molecules / wisps**: reusable workflow definition materialized into work DAGs.
- **Sling / dispatch**: route work to an agent/session/pool and optionally wake it.
- **Health patrol**: reconcile desired session state against live runtime, enforce restart budgets and drift handling.
- **Orders**: scheduled/event-triggered dispatch, but this should be delayed in CHROTE until policy is mature.

### Strong engineering patterns

- CLI and HTTP/SSE are projections over one object model.
- Typed OpenAPI control plane generated from Go structs.
- Generated clients and tests catch API/spec drift.
- `doctor`, `config explain`, `prime`, smoke-test commands build operator trust.
- Supervisor remains localhost by default.
- Real harnesses are validated through tiny smoke prompts and constrained wrappers.

### What not to inherit wholesale

- The full command tree and terminology.
- Gas City's required city/root/rig worldview.
- Dolt/BdStore as a first CHROTE dependency.
- Orders/shell-exec automation before CHROTE has policy grants and run audit.
- Direct imports from Gas City `internal/` packages.
- Treating Gas City session ownership as the only path to agent teams.

## Current CHROTE architecture fit

CHROTE today is intentionally a durable host cockpit:

- Go server with embedded React dashboard.
- Browser terminals backed by ttyd and tmux on `/run/user/1000/chrote-tmux`.
- Files API restricted to configured roots.
- Beads API reads modern `bd --json` workspaces.
- Agents/Oracle view observes agent-like tmux sessions by prefix.
- Services API proxies TTS and Context API server-side so credentials stay off the browser.
- UI keeps terminal iframes mounted to preserve live sessions across tab switches.

This means the CLI should first be a **companion to CHROTE's existing API**, not a second server and not a direct tmux/files bypass.

Important constraints:

- Do not start or stop ttyd from the CLI.
- Do not kill tmux sessions unless explicitly commanded.
- Do not bypass service proxies for tokened upstreams.
- Do not change dashboard terminal iframe lifecycle.
- Do not expose Gas City supervisor directly through tailnet/public routes.
- Keep destructive commands behind explicit confirmation.

## Historical Recommendation By Option

The table below is superseded by `docs/chrote-gascity-framing.md`.

| Option | Verdict | Why |
|---|---|---|
| Build CHROTE directly on Gas City | No | Makes Gas City mandatory and leaks its worldview into a broader cockpit product. |
| Fork Gas City | Hell no | Large active codebase, lots of orchestration complexity, fast drift, no need. |
| Sidecar Gas City as next product layer | Good for spikes, not foundation | Fastest way to observe mature orchestration, but creates two competing control planes if promoted too far. |
| CHROTE-native CLI/control plane + Gas City adapter | Yes | Keeps CHROTE orchestrator-neutral, lets us borrow proven patterns, and preserves Gas City as optional. |

## Historical Proposed CHROTE Primitives

These names were part of the superseded native-control-plane exploration. Keep them only as vocabulary evidence.

### `WorkspaceRoot`
Allowed filesystem roots, default workdir, service visibility boundaries.

### `Adapter`
A typed integration boundary for systems CHROTE can observe/control:

- tmux
- `bd`
- Gas City
- Codex
- Claude Code
- Pi
- OpenCode
- Hermes
- Context API
- TTS Gateway
- generic shell/tmux sessions

Adapters report health, capabilities, read models, and safe actions.

### `Session`
Durable runtime handle:

- ID/name
- provider/adapter
- cwd
- attach/peek/send/interrupt/stop capabilities
- transcript/artifact references
- policy constraints

### `AgentHandle`
A projection over a session plus adapter metadata. Not a hardcoded role system.

### `WorkRef`
A reference to external or internal work:

- Beads issue
- Gas City bead
- GitHub issue
- recipe step
- arbitrary external ID

### `Run`
Audited execution instance:

- actor who started it
- recipe/manual action
- participants/sessions
- approvals
- events
- artifacts
- final status

This is CHROTE's equivalent of a molecule/run ledger, without inheriting Gas City naming.

### `Recipe`
Declarative operator-authored workflow:

- steps
- adapter targets
- required approvals
- allowed mutations
- expected artifacts
- stop/rollback behavior

Recipes coordinate transport and audit. They do not hardcode cognition in Go.

### `EventLedger`
Append-only typed event stream used by dashboard, CLI, audit, recovery, and replay.

### `PolicyGrant`
Explicit capability grant:

- path scope
- service scope
- session target
- allowed action
- approval requirement
- token/secret boundary

### `ServiceProxy`
Generalized server-side proxy model for local/private services.

## Historical CLI Shape

The CHROTE-only cockpit examples below are retained as historical exploration. They are not a current implementation plan.

```text
chrote status
chrote health
chrote events tail
chrote sessions list
chrote sessions peek <name> [--lines N]
chrote agents list
chrote beads projects
chrote beads issues --path <workspace>
chrote services list
chrote services tts health
chrote services context docs
```

Mutating commands require confirmation or explicit flags:

```text
chrote sessions create [name]
chrote sessions rename <old> <new>
chrote sessions kill <name> --yes
chrote sessions nuke --confirm DASHBOARD-NUKE-CONFIRMED
chrote services tts enqueue --text "..."
chrote services context save <path> --file <local>
```

The Gas City command examples from this exploration are superseded. Treat them as historical candidate shapes only; they are not a current implementation plan or decision about whether CHROTE should wrap, observe, adapt, or ignore any `gc` surface.

## Historical Architecture Sketch

The phased sketch below is preserved as historical context only. It is not the current plan.

### Phase 0 — ADR / source-of-truth update

- Record decision: CHROTE-native control plane, Gas City optional adapter.
- Update older Gas City docs to say sidecar-first was a spike recommendation, not final foundation.
- Non-goal: no Gas City fork, no `internal/` imports, no mandatory city.

Validation:

- CHROTE remains useful with Gas City stopped/uninstalled.
- Existing dashboard behavior unchanged.

### Phase 1 — CLI over existing CHROTE API

Add a separate `chrote` command that uses CHROTE's existing API.

Implementation direction:

- `src/cmd/chrote` for CLI.
- `src/internal/client` for HTTP client.
- Base URL default: `http://127.0.0.1:8094`.
- Optional auth: `CHROTE_URL`, `CHROTE_API_TOKEN`.
- Output modes: table/json.

Start with read-only and safe commands.

Validation:

- `chrote health` matches `/api/health`.
- `chrote sessions list` matches `/api/tmux/sessions`.
- `chrote agents list` matches `/api/oracle/agents`.
- No ttyd restart, no tmux kill, no dashboard regression.

### Phase 2 — Typed CHROTE event ledger

Add CHROTE-owned JSONL event log and streaming endpoint.

Events to record first:

- CLI command invoked
- session create/rename/kill/nuke intent/result
- service action intent/result
- adapter health change
- Gas City observation import

Validation:

- `chrote events tail` works after restart.
- Dashboard can subscribe without polling everything.
- Every mutation has actor, target, request ID, result, timestamp.

### Phase 3 — Service extraction / object model

Current server handlers contain a lot of direct business logic. Extract reusable internal packages so HTTP and CLI can be two projections over one CHROTE model.

Candidate packages:

- `internal/control` — objects, events, policy, run ledger.
- `internal/client` — typed HTTP client.
- `internal/adapters/tmux` — tmux/session operations.
- `internal/adapters/beads` — `bd` project/issues/graph reads.
- `internal/adapters/services` — TTS/Context API clients/proxies.
- `internal/adapters/gascity` — Gas City CLI/API adapter.

Validation:

- API shapes stay backward-compatible.
- CLI and browser agree on read models.
- Tests cover extracted packages, not just handlers.

### Phase 4 — Gas City read-only adapter

Integrate `/home/perttu/gascity` and supervisor `127.0.0.1:8372` as an optional adapter.

Observe:

- supervisor health/readiness
- cities
- sessions
- events
- beads/work
- mail counts
- formulas/orders

Rules:

- No direct public exposure of supervisor.
- No Gas City mutation in first adapter pass.
- No imports from Gas City `internal/`.
- Degraded state is normal if `gc` or supervisor is absent.

Validation:

- CHROTE can show Gas City state read-only.
- Stopping Gas City does not break CHROTE core.
- Adapter output maps to CHROTE primitives (`Session`, `WorkRef`, `Event`) instead of leaking raw Gas City nouns everywhere.

### Phase 5 — Run ledger + controlled actions

Add CHROTE-native `Run` records and controlled mutation actions.

Examples:

- submit text to a known session
- wake a Gas City named session
- enqueue a TTS message
- run a Gas City formula as a CHROTE-tracked run
- attach outputs/transcripts/artifacts to the run

Validation:

- Every action has an event trail.
- Dangerous actions require `--yes`, explicit target, and policy grant.
- A Pi smoke run can be started, watched, and recovered from CHROTE.

### Phase 6 — Recipes / teams

Add CHROTE-native recipes once the run ledger is proven.

Initial recipes:

- plan + two reviewers + synthesis
- review quorum
- senate discussion
- implementation + reviewer + fixer loop

Validation:

- A recipe can use different harnesses in different roles.
- Failure leaves recoverable state.
- Human can intervene, redirect, or stop without guessing tmux targets.

## Safety boundary

Before exposing mutations broadly:

- All action endpoints need request IDs and audit events.
- Browser and CLI must not see service tokens.
- Adapter capabilities must be explicit.
- Session targeting must be unambiguous.
- Destructive session/file/service actions require confirmation.
- Automated inter-agent messaging requires a run/recipe context, not ad hoc hidden prompt injection.
- Scheduled automation should wait until policy grants and run recovery exist.

## What to do with Gas City now

Keep `/home/perttu/gascity` running as a spike and test fixture.

Use it for:

- validating adapter observations
- testing session/mail/formula concepts
- comparing CHROTE run ledger with Gas City events
- trying Pi/OpenCode/other harness wrappers under controlled conditions

Do not use it for:

- CHROTE core state
- CHROTE authentication/policy
- public/tailnet supervisor exposure
- mandatory dashboard startup
- the only transcript/recovery source

## Open design questions

1. Should CHROTE's event ledger be file-backed JSONL first, SQLite, or both?
2. Should the CLI be Go-only in `src/cmd/chrote`, or should there also be a tiny shell wrapper for recovery?
3. What is the minimum adapter capability contract for harnesses: `peek`, `submit`, `interrupt`, `history`, `artifacts`, `health`?
4. How should CHROTE represent external WorkRefs without duplicating `bd`/Gas City state?
5. Should recipes be files under the workspace, records in a CHROTE store, or both?
6. What policy language is enough for first controlled actions without building a giant permissions system?

## Bottom line

Gas City is good. The ideas are absolutely worth stealing.

But CHROTE should not become a Gas City dashboard. CHROTE should become the private host control plane that can observe and operate many systems — Gas City included — through adapters, events, runs, and policy.
