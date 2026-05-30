# CHROTE 3.0 Gas City Substrate Map

Inspected: 2026-05-27.

2026-05-30 rollback note: ADR-0003 superseded the Gas City substrate decision.
This map is retained as historical evidence only. It is not an active CHROTE
implementation map.

Related work:

- `home-e5yp`: prove Gas City can use shared Beads truth.
- `home-bi6d.4`: map CHROTE 2.0 primitives to Gas City 3.0 primitives.
- `home-4xv.4`: Dolt/quorum validation; see
  `../experiments/dolt-quorum-validation.md`.
- ADR: `../adr/0001-chrote-3-gas-city-substrate.md`.

## Assumptions And Scope

This document covers the documentation and proof boundary only. It does not
change CHROTE code, Gas City code, Beads state, Gas City runtime state, tmux
sessions, systemd units, or supervisor registration.

Historical source-of-truth rule considered during the Gas City work:

- `/home/perttu` Beads remains canonical for CHROTE 3.0 workflow/work state:
  durable issues, dependencies, acceptance criteria, ownership, lifecycle
  status, closure reasons, and follow-up work.
- Gas City may own orchestration/runtime records: valid Gas City identities,
  sessions, mail, nudges, formulas, molecules, convoys, orders, and events.
- Gas City must project or link orchestration state to canonical `home-*` Beads
  records. It must not silently compete with `/home/perttu` Beads as a second
  durable work truth.
- Context Citadel remains canonical for durable personal/project context.
- CHROTE owns authenticated access, operator UI, policy, and recovery surfaces.

## Current Gas City State Boundary

The active sidecar is `/home/perttu/gascity`.

Verified local configuration:

- Installed `gc`: `/home/perttu/.local/bin/gc`.
- `city.toml` sets `[beads] provider = "file"`.
- `gc config show` resolves the same file-backed provider.
- `gc bd` exits with: only supported for bd-backed beads providers, resolved
  `"file"` for `/home/perttu/gascity`.
- `gc beads health` reports healthy for the file provider. CLI help states
  file-provider health is a no-op.
- `gc doctor --verbose` passed 46 checks in the live sidecar. The Dolt checks
  were skipped because this is file-backed or Dolt was skipped.

Runtime state observed under `.gc/`:

| Path | Role | Boundary |
| --- | --- | --- |
| `.gc/beads.json` | File-backed Gas City bead store | Current canonical store for Gas City sidecar runtime records only |
| `.gc/events.jsonl` | Append-only Gas City event log | Gas City runtime evidence and observer input |
| `.gc/pi-sessions/*.jsonl` | Pi harness session transcripts from smoke work | Runtime transcript evidence, not canonical work truth |
| `.gc/nudges/state.json` | Nudge/poller state | Gas City runtime state |
| `.gc/runtime/*` | Reconciler/runtime traces | Gas City runtime state |
| `.gc/controller.token`, locks | Controller/private runtime internals | Do not print, commit, or expose |
| `.beads/` | Separate modern Beads workspace marker in the repo | Present, but current `gc` scope still resolves provider `file` |

Bounded `.gc/beads.json` summary at inspection time:

- `seq`: 52197.
- bead records: 52197.
- open records: 33.
- dependencies: 24.
- observed record families: order tracking, sessions, formulas/molecules,
  sling/convoy records, and bead-backed mail.
- sample open records included formula steps with `gc.step_ref`, routed work
  with `gc.routed_to`, sessions with `gc:session` labels, and mail threads.

Event summary at inspection time:

- `.gc/events.jsonl` had 104354 lines.
- Recent events were controller order fire/complete events.

This means the current sidecar has real Gas City runtime evidence, but the
file-backed store is not a production shared-work proof and is not a CHROTE 3.0
work-truth candidate.

## Supported Beads Provider Options

Verified from Gas City CLI, docs, and source:

- Provider selection priority is `GC_BEADS` env var, then
  `city.toml [beads].provider`, then default `"bd"`.
- Supported provider names documented for the bead store are:
  - `bd`: default, Dolt-backed database through the `bd` CLI.
  - `file`: JSON file on disk, useful for tutorials and small setups.
  - `exec:/path/to/script`: custom provider script.
- `gc bd` is available only for bd-backed scopes. It rejects file-backed scopes.
- `gc beads city use-managed` and `gc beads city use-external` are only for
  bd-backed stores. They manage canonical managed/external Dolt endpoint
  topology and support `--dry-run`.
- Source markers:
  - `.gc/beads.json` marks a file-backed scope.
  - `.beads/metadata.json` marks a bd/Dolt-backed scope.

Local support status:

- `bd` exists: version 1.0.3.
- `flock` exists.
- standalone `dolt` was initially missing, then installed through Linuxbrew as
  `dolt version 2.0.6` for the disposable proof.
- Gas City docs require `dolt` for provider `bd`, even though `bd` itself can
  use embedded Dolt for direct Beads work.

## Disposable bd/Dolt Proof Attempt

The requirement was to test a supported bd/Dolt-backed or projection approach
on disposable data if local tooling supports it safely.

Initial safe temp-city proof:

1. Created a temp Gas City city under `/tmp/chrote-gc-bd-proof.*` with
   `gc init --provider codex --skip-provider-readiness`.
2. The city scaffold was created, but `gc init` reported missing dependency
   `dolt` and advised installing it before `gc start`.
3. `gc beads health` in the temp city reported an unhealthy bd provider because
   the managed Dolt server was not reachable and recovery failed.
4. `gc bd create ...` in the temp city failed because the database was not
   initialized (`issue_prefix` missing).

Rejected direct Beads initialization path:

- Running direct `bd` from the temp tree was not safe in this environment:
  `bd where` resolved to `/home/perttu/.beads`, not the temp `.beads`
  directory.
- Explicit `BEADS_DIR=/tmp/.../.beads bd where` still resolved to
  `/home/perttu/.beads`.
- Therefore direct `bd init` from `/tmp` is not a safe disposable proof path
  here unless a future agent first proves a reliable isolation flag or wrapper.

Retried with standalone Dolt:

1. Installed user-local `dolt` through Linuxbrew.
2. Set Dolt author identity from the existing Git identity.
3. Created `/tmp/chrote-gc-bd-proof.0tkSSO` with
   `gc init --provider codex --skip-provider-readiness`.
4. `gc beads health` reported `Beads provider: healthy`.
5. `gc bd where` reported `/tmp/chrote-gc-bd-proof.0tkSSO/.beads` and
   `database: /tmp/chrote-gc-bd-proof.0tkSSO/.beads/dolt`.
6. `gc bd create "Disposable Gas City bd proof" ... --json` created a
   disposable `cgbp-*` bead in the temp city.
7. `gc bd list --json` returned the disposable bead plus Gas City session
   records from the temp city.
8. The temp city was then suspended, stopped, and unregistered. `gc cities`
   returned only the live `/home/perttu/gascity` city afterward.

Safety finding:

- `gc init` is not a pure offline scaffold in this setup. It registered the temp
  city with the machine-wide supervisor and waited for the supervisor to start
  the city.
- The default temp city used provider `codex` and materialized a `mayor` session
  record whose command included Codex dangerous bypass flags. No new tmux
  session appeared under the Gas City tmux socket during the proof, but this
  default is not an acceptable unattended proof path for real harness work.
- `/home/perttu/.beads/issues.jsonl` hash stayed unchanged during the proof.
  `/home/perttu/gascity/.gc/beads.json` hash changed during supervisor activity,
  so future proofs must avoid supervisor registration/startup or snapshot and
  review live sidecar runtime changes explicitly.

Conclusion:

- The supported bd/Dolt-backed Gas City approach works on disposable data once
  standalone Dolt and Dolt identity are available.
- The current `gc init` path is not sufficiently isolated for repeated
  automated proofs because it can register/start a temp city with the live
  supervisor. Resolved by the GC_HOME-isolated inert-config path below
  (see "Safe Disposable City Proof Path (home-d0lv)").
- Do not migrate `/home/perttu/gascity/.gc/beads.json` until a disposable
  bd-backed city or rig projection can run without live-supervisor side effects.

## Safe Disposable City Proof Path (home-d0lv)

Resolved on 2026-05-27. `gc init` has no `--no-register` / `--no-start` /
`--offline` flag: it always registers the new city, writes a per-`GC_HOME`
systemd user unit, and starts a supervisor that launches the city. A precise
negative result on a pure offline scaffold flag, but a fully safe disposable
path exists by isolating `GC_HOME` and using an inert mayor config.

Isolation mechanism:

- The machine-wide supervisor registry is `${GC_HOME}/cities.toml`. The live
  supervisor runs with `GC_HOME=/home/perttu/.gc`, so the live registry is
  `/home/perttu/.gc/cities.toml`.
- Setting `GC_HOME=<tmp>/.gc` redirects the registry, supervisor socket, API
  port, and the generated systemd unit name (`gascity-supervisor-gc-<hash>`)
  entirely into the temp tree. The live registry and live supervisor are never
  touched.
- `HOME` must stay `/home/perttu`. The binary refuses a `HOME` override for the
  platform supervisor ("Keep HOME unchanged and use GC_HOME for isolated runs").
- `TMUX_TMPDIR=<tmp>/tmux` isolates any tmux socket away from the live
  `gascity` socket. With an inert mayor no tmux session is created at all.

Avoiding paid/credentialed harness sessions:

- Do NOT use `--provider codex` (or any real-harness provider). That path
  materializes a `mayor` session whose command carries
  `--dangerously-bypass-approvals-and-sandbox` (and equivalent yolo /
  `--approval-mode auto_edit` flags for other providers).
- Instead init from an inert `city.toml` with `--file`:
  `start_command = "true"` (no-op mayor) and `[beads] provider = "file"`. This
  still materializes the full builtin packs, including the `core` formulas
  (`mol-review-quorum`, `mol-scoped-work`), but creates no harness session and
  no dangerous flags anywhere in the city tree.

Supported safe sequence (verified):

```bash
TMPROOT="$(mktemp -d /tmp/gc-disposable.XXXXXX)"
export HOME=/home/perttu                 # keep real HOME (required)
export GC_HOME="$TMPROOT/gchome/.gc"     # isolate registry/supervisor/unit
export TMUX_TMPDIR="$TMPROOT/tmux"       # isolate tmux socket
mkdir -p "$GC_HOME" "$TMUX_TMPDIR"

cat > "$TMPROOT/inert-city.toml" <<'TOML'
[workspace]
start_command = "true"
max_active_sessions = 1
[beads]
provider = "file"
TOML

CITY="$TMPROOT/city"
gc init --file "$TMPROOT/inert-city.toml" "$CITY"   # auto-registers+starts in GC_HOME only

# Read-only inspection works even after stopping the isolated supervisor:
gc --city "$CITY" stop && gc --city "$CITY" unregister && gc supervisor stop
gc --city "$CITY" formula list                       # works offline, no supervisor
gc --city "$CITY" formula show mol-review-quorum      # reproduces home-706s offline

# Teardown: remove the per-GC_HOME systemd unit, then the temp tree.
systemctl --user disable --now gascity-supervisor-gc-*.service 2>/dev/null || true
rm -f /home/perttu/.local/share/systemd/user/gascity-supervisor-gc-*.service
systemctl --user daemon-reload
rm -rf "$TMPROOT"
```

Notes:

- `gc init` will print "Registered city ..." and write/start the isolated unit.
  This is expected and safe: it all lives under `GC_HOME`. Always remove the
  `gascity-supervisor-gc-<hash>.service` unit during teardown so it does not
  linger in user systemd.
- Read-only `gc --city <dir> formula list/show` and file-provider bead reads do
  NOT require a running supervisor, so pure inspection work (e.g. home-706s) can
  skip the start/register entirely after scaffolding — or even reuse a scaffold
  without ever starting it.
- For bd/Dolt-backed disposable proofs, set `provider = "bd"` in the inert
  `city.toml` and supply standalone Dolt as in the prior section; the same
  `GC_HOME` isolation keeps it off live state.

Live-state evidence (2026-05-27 run):

- `/home/perttu/.gc/cities.toml` byte-identical before/after; registered cities
  list unchanged (only `gascity`); live supervisor PID unchanged (3826452).
- Live `/home/perttu/gascity/.gc` file set identical (427 files, 0 added,
  0 removed); no live `.gc` path referenced the temp tree. The live
  `beads.json` / `events.jsonl` contents change continuously from the live
  supervisor's own activity, independent of the proof.

## Canonical Store For CHROTE 3.0 Work

Canonical CHROTE work state is `/home/perttu` Beads.

Gas City records can be authoritative only for Gas City-owned orchestration
facts, such as:

- session identity and lifecycle state;
- mail delivery/read/archive state;
- nudge and live-delivery attempts;
- formula/molecule/convoy materialization state;
- supervisor/controller events;
- runtime reconciliation evidence.

Gas City records are not authoritative for:

- whether a CHROTE work item exists;
- acceptance criteria;
- dependencies between `home-*` work items;
- priority/owner/assignee of the CHROTE task;
- whether the task is complete;
- durable project context.

Required link shape for CHROTE 3.0:

- Every Gas City workflow that corresponds to a CHROTE work item should carry a
  stable `home-*` reference, for example `home_bead=home-e5yp` or a namespaced
  metadata key such as `chrote.home_bead=home-e5yp`.
- Gas City `gc-*` IDs should be stored as projection/runtime references on the
  canonical `home-*` bead only through normal Beads tooling, never by editing
  `.beads/issues.jsonl` by hand.
- If a Gas City workflow has no `home-*` reference, CHROTE should display it as
  unlinked runtime work, not silently treat it as backlog truth.

## Split-Brain Failure And Recovery

Split brain means `/home/perttu` Beads and Gas City disagree about work state,
or one system has durable-looking state the other cannot reconcile.

Common failure modes:

| Failure | Example | Recovery rule |
| --- | --- | --- |
| Gas City open, Beads closed | `gc-*` workflow remains open after `home-*` bead closes | `/home/perttu` Beads wins. Mark or archive the Gas City runtime record through supported `gc` commands after review, or leave it as historical runtime evidence. |
| Beads open, Gas City missing | A `home-*` bead has no corresponding `gc-*` workflow | Treat Gas City as missing projection. Recreate or relink Gas City orchestration from the Beads record if still needed. |
| Both open, fields disagree | Gas City routed target/status differs from Beads owner/status | Beads wins for work truth. Gas City may explain orchestration status only. Human review if changing target could affect a live harness. |
| Gas City mail exists without Beads task | A thread has useful outcome but no `home-*` link | Preserve as runtime evidence. Create or link a Beads follow-up through normal `bd` workflow if the outcome affects work. |
| File store lost/corrupt | `.gc/beads.json` damaged or stale locks remain | Do not infer task loss. Recover CHROTE work from `/home/perttu` Beads and context from Context Citadel. Archive damaged `.gc` files before cleanup. |
| Beads unavailable, Gas City running | `bd` commands fail but Gas City can still dispatch | Freeze CHROTE 3.0 work mutations that would create/close/claim canonical work. Gas City can continue only bounded runtime observation. |
| Gas City unavailable, Beads healthy | Supervisor/city down | Continue work through Beads/CHROTE/tmux. Rebuild Gas City projection later from Beads plus saved `.gc` evidence. |
| Context mismatch | Agent output references stale context | Context Citadel wins for durable context. Store new durable claims through Context Citadel contribution/append paths, not Gas City mail alone. |

Recovery procedure:

1. Stop treating Gas City status as canonical for work.
2. Read the `home-*` bead and its dependencies from `/home/perttu`.
3. Read linked `gc-*` records and events if a link exists.
4. Compare only explicit IDs and metadata; do not match by vague title alone
   except as a candidate for human review.
5. Decide one of: relink, recreate projection, close/archive runtime record,
   or leave as historical evidence.
6. Record the reconciliation result in Beads notes or a close reason through
   normal `bd` tooling.
7. Never repair by hand-editing `.gc/beads.json` or `.beads/issues.jsonl`.

## Primitive Ownership Map

| CHROTE 2.0 primitive | Current CHROTE behavior | Gas City 3.0 primitive | Owner classification | 3.0 rule |
| --- | --- | --- | --- | --- |
| Durable terminal sessions | CHROTE lists/creates/kills/renames tmux sessions and proxies terminal panes | Gas City sessions backed by runtime providers | Shared | CHROTE keeps human shell/cockpit sessions; Gas City owns orchestrated agent sessions. |
| Tmux socket | CHROTE uses `TMUX_TMPDIR=/run/user/1000/chrote-tmux` and default socket | Gas City sidecar uses socket name `gascity` under same root | Shared | Socket root is host/CHROTE infrastructure; Gas City owns its own socket namespace. |
| Agent session detection | Agents view watches tmux prefixes like `codex`, `claude-`, `opencode` | Gas City has named sessions, agent configs, and session records | Shared | CHROTE can observe both, but real Gas City agents must be valid Gas City identities. |
| Prompts and intervention | Browser terminal input and tmux capture/submit patterns | Prompt templates, `gc session submit`, `gc session nudge`, formulas | Shared | CHROTE owns human input UI; Gas City owns role prompts and routed work delivery. |
| Mail/communication | No native CHROTE mail; mostly terminal text and external tools | `gc mail`, bead-backed messages, threads, read/archive, nudge notification | Gas City-owned | CHROTE should render/proxy mail through policy, not invent a parallel mailbox. |
| Lifecycle | CHROTE controls tmux sessions and its own service | Gas City controller, session state, sleep/wake/pin, health patrol | Shared | Gas City owns orchestrated harness lifecycle; CHROTE owns cockpit lifecycle and direct operator shell controls. |
| Transcript recovery | CHROTE capture-pane, terminal iframe continuity, some logs | `gc session peek`, provider logs, Pi session logs, `gc session logs` where supported | Shared | Recovery display is CHROTE-owned; source material may come from both. Gas City transcript coverage is still a gap. |
| Workflows/recipes | Roadmap only; old agent-team ideas are not current implementation | Formulas, molecules, wisps, convoys, sling, orders | Gas City-owned in the superseded substrate model | Historical rule: Gas City would own workflow graph execution; CHROTE would own launch/observe/approval UI. |
| Beads | CHROTE calls `bd --json` against configured workspaces; `/home/perttu` is work truth | Gas City can use file, bd, or exec bead providers | Shared, Beads-canonical | `/home/perttu` Beads is canonical. Gas City must link/project. |
| Context Citadel | CHROTE Services proxy reads/writes/asks with server-side credentials | Not a Gas City orchestration goal | CHROTE-owned access, Context-owned truth | Context Citadel remains durable context truth. Gas City mail is not context memory. |
| Auth/access policy | CHROTE is private cockpit via localhost/tailnet and server-side service tokens | Gas City supervisor is localhost and not user-authenticated for browser exposure | CHROTE-owned | CHROTE must proxy any Gas City surface through CHROTE auth/policy. Do not expose raw supervisor. |
| Operator UI | CHROTE dashboard views for Terminal, Files, Agents, Beads, Services | Native `gc` CLI and possible Gas City dashboard/API | CHROTE-owned | CHROTE is the operator cockpit. Do not mirror the full `gc` command tree. |
| Events/status stream | Oracle SSE infers agent state from tmux and Beads IDs | Gas City events log and supervisor API/SSE | Shared | Gas City owns orchestration events; CHROTE owns operator-friendly read model. |
| Files/artifacts | CHROTE Files view over allowed roots | Gas City rigs/work dirs and runtime artifacts | Shared | CHROTE owns file browsing policy. Gas City may point to artifacts but should not widen file roots. |
| Real harness auth | CHROTE keeps service credentials server-side and private | Gas City can launch providers/wrappers | CHROTE-owned policy, Gas City-owned runtime | Real harness adapters need explicit credential/env boundaries before mutation. |
| External/mobile access | CHROTE is the browser/tailnet cockpit | Gas City is local substrate | CHROTE-owned | Client remains disposable; Gas City supervisor remains local-only. |

## Implementation Guardrails

- Start with read-only CHROTE observation of Gas City city/session/mail/workflow
  state.
- Add mutating CHROTE controls only after the real-harness safety boundary is
  accepted.
- Never count a CHROTE-launched process calling `gc mail` as proof of Gas
  City-owned identity unless the sender is a valid Gas City session identity.
- Do not use `gc-*` records as CHROTE backlog.
- Do not hand-edit `.gc/beads.json`.
- Do not hand-edit `.beads/issues.jsonl`.
- Do not expose the raw Gas City supervisor outside localhost or outside a
  CHROTE-owned authenticated proxy.

## Follow-Up Bead Status

Linked follow-up beads exist where this map intentionally stops short of broad
implementation.

| Bead | Status | Notes |
| --- | --- | --- |
| `home-6p0j` | Completed | Installed Dolt and proved a bd-backed temp-city path, but found live-supervisor side effects. |
| `home-d0lv` | Verified, pending close | Safe disposable path resolved: `gc init` has no offline/no-register flag, but `GC_HOME`-isolated init from an inert `city.toml` keeps all register/start side effects off live state and creates no harness session. See "Safe Disposable City Proof Path (home-d0lv)". |
| `home-p81n` | Completed | Projection schema is in `projection-schema.md`. |
| `home-4xv.2` | Completed | CHROTE observer is implemented as `GET /api/gascity/observer` plus the dashboard Gas City view. |
| `home-ujx8` | Completed | Split-brain guidance is in `../runbooks/split-brain-runbook.md`. |
| `home-ukyt` | In progress | Restart reconciliation evidence is in `../experiments/restart-reconciliation-drill.md`; close with the CHROTE count-level caveat after review. |
| `home-4xv.3` | In progress | Transcript recovery is implemented through `gc session peek`, documented in `../runbooks/transcript-recovery.md`, and exposed through the dashboard sessions panel. |
| `home-bi6d.5` | Completed | First identity proof succeeded through a Pi-owned Gas City mail fallback; evidence is in `../experiments/real-identity-smoke.md`. |
| `home-bi6d.6` | Completed | Minimal CHROTE control surface can observe sessions/mail/workflows/events and send one bounded Pi poem request. |
| `home-4xv.4` | In progress | Dolt/quorum validation is documented in `../experiments/dolt-quorum-validation.md`; live state remains file-backed. |
| `home-706s` | Open | Follow-up for silent `graph.v2` / `mol-review-quorum` formula inspection failure. |
