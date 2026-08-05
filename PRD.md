# CHROTE Product Requirements

## Vision

CHROTE is the private browser cockpit for host-owned agentic work.

The host owns the terminals, agents, files, dev servers, builds, tests, Beads,
and recovery state. Browsers and client devices are replaceable windows onto
that work.

The browser is disposable. The work is not.

## Product contract

CHROTE gives one trusted operator a coherent control surface over a configured
Linux or WSL workspace. It keeps durable terminal work visible, makes local
state inspectable, and adds explicit orchestration primitives without moving the
source of truth into the browser.

### Current goals

1. Make durable tmux sessions visible and controllable from a browser.
2. Keep device disconnects and CHROTE restarts from silently killing important work.
3. Put terminal sessions and relevant files beside each other without pretending
   to be an IDE.
4. Surface agent sessions, Beads work, scheduled tasks, services, and server
   health in one cockpit.
5. Keep experimental orchestration work explicit and isolated from the shipped
   cockpit until it earns a release.
6. Keep the deployment private to localhost or an operator-controlled private
   network unless explicitly configured otherwise.

### Current non-goals

- CHROTE is not a hosted multi-tenant SaaS product.
- CHROTE is not an OS sandbox or a security boundary around tmux agents.
- CHROTE is not an IDE and does not make the browser the workspace source of truth.
- CHROTE does not replace `tmux`, `bd`, Git, or the underlying AI harnesses.
- CHROTE does not promise autonomous agent society, implicit agent-to-agent chat,
  or magic recovery of arbitrary shell state.
- CHROTE does not require Gastown, Ralph, or a single preferred agent harness.

## Current views

| View | Operator job |
| --- | --- |
| Terminal 1 | First independent terminal workspace with 1-4 durable tmux windows |
| Terminal 2 | Second independent terminal workspace |
| Terminal 3 | Third independent terminal workspace |
| Files | Browse, inspect, edit, compare, and send configured workspace files |
| Agents | Inspect agent personas/sessions and the mission/run context around them |
| Beads | Inspect configured `bd` workspaces, issues, ready work, and health |
| Formations | Develop and inspect file-backed orchestration contracts without presenting them as shipped product |
| Services | Operate explicitly configured local service adapters through CHROTE-owned routes |
| Scheduled | Inspect and manage CHROTE scheduled tasks and their run history |
| Server | Inspect server health, resources, events, and recovery history |
| Settings | Configure appearance, terminal behavior, Session Bank, feature flags, and recovery actions |

Help and keyboard guidance are available from the application shell rather than
as a separate persistent workspace.

## Durable terminal workspace

### Terminal panes

- Each terminal workspace owns its own layout, attached sessions, labels, and
  sidecar state.
- A workspace can show one to four terminal windows.
- tmux remains the durable process/session substrate; CHROTE does not copy shell
  state into browser storage.
- Browser/device disconnects must not terminate tmux sessions.
- Refit and recovery controls are explicit operator actions.

### Sessions and Files sidecar

Sessions and Files are independent peer sidecars within each terminal workspace.

- Both sidecars are closed by default and reserve no permanent terminal width.
- Wide layouts may pin a sidecar; when both are open they occupy adjacent pinned
  rails so neither obscures the other. Narrow layouts overlay the open sidecar
  views.
- Session row selection means **Peek**. It must not detach, reassign, or mutate
  terminal-window assignment metadata.
- Navigating an attached session occurs through its explicit location chip.
- Each sidecar's open state, the pin preference, and separate Sessions/Files
  widths persist per terminal workspace.
- The `/` shortcut opens Sessions for the active terminal workspace and focuses
  its search when no visible dialog or menu owns the key.

### Session Bank and workload recovery

- CHROTE records previously observed sessions so an operator can distinguish
  live, offline, recoverable, and unmanaged work after a restart.
- Typed recovery descriptors may reconstruct supported agent or command
  workloads using canonical argument vectors and constrained paths.
- Legacy or unsafe entries remain inspection-only or require explicit operator
  action; CHROTE must not fabricate arbitrary shell recovery.
- Recovery plans are host-owned state, not browser-local state.
- Bulk destruction remains an advanced emergency action.

### Persistent agents

- Locking a session is the operator's promise that this agent should still be
  running tomorrow. The lock is the cockpit affordance for that promise; it is
  not itself the supervisor.
- Supervision belongs to the host init system. CHROTE writes a per-agent
  configuration and enables a systemd user unit for it. The server runs no
  supervision loop, holds no retry state, and never recreates a session, so a
  CHROTE restart, crash, or upgrade cannot interrupt a locked agent.
- Locking replaces an unmanaged pane once with CHROTE's fixed pane launcher and
  resumes the same native agent session. The UI warns that current in-flight
  terminal input may be interrupted. The launcher invokes typed agent arguments
  directly; it never types a rendered command into a fresh shell.
- Lifecycle state is read from the unit rather than tracked by CHROTE. The unit
  does not finish starting until its launcher observes the actual pane process
  and publishes proof bound to that systemd invocation, pane, PID, process start,
  and native transcript. Reported health revalidates that proof; an active unit
  without current matching evidence is degraded rather than healthy.
- Unlocking withdraws the promise, not the work. The tmux session and the
  running agent are left alive, and the confirmation says so.
- Sessions owned by units CHROTE did not install remain read-only and are never
  restarted by CHROTE.
- Persistent state does not imply autonomous authority to message, claim, or
  mutate unrelated work.

## Files

- File access is constrained to configured roots and the Unix permissions of the
  CHROTE process.
- The Files view is a terminal companion: browse, inspect, edit, compare, and
  send context to a session without becoming a general-purpose IDE.
- Symlinks and mutations must remain within configured roots after resolution.
- Browser convenience does not weaken the host filesystem boundary.

## Beads and agent observability

- CHROTE uses configured `bd` commands and JSON output.
- Multiple Beads workspaces may be configured; absence of Beads is a degraded
  integration, not a dashboard crash.
- `bv` is optional and remains a terminal-side graph viewer, not CHROTE's issue
  source of truth.
- Agent observation is harness-neutral and may use configured session prefixes
  and persona metadata.
- Absence of agent sessions is normal.

## Experimental Formations and missions

Formations is implemented on `main` as an unreleased experimental orchestration
surface. Its presence in a development build is not a promise that it ships in
the latest tagged alpha or belongs in the public quick-start path.

- Boards, layouts, personas, missions, formations, gates, connections, and run
  ledgers remain durable host-owned files.
- Inputs and outputs are typed ports; fan-in waits and fan-out follows explicit
  output connections.
- Gates produce explicit verdicts and ledger events.
- Mission and formation runs are inspectable and resumable only within their
  documented execution contract.
- The deterministic lab executor is the safest proving environment.
- Isolated and live tmux execution require explicit configuration and promotion;
  Formations must not create or kill unrelated tmux sessions.
- Formations is not a text-routing fallback and does not silently invent missing
  connections, workers, or gate outcomes.

`FORMATIONS.md`, `ARCHON.md`, and `DATA-MODEL.md` own the detailed experimental
contracts. Promotion requires an explicit release decision, current media, and
installer/runtime evidence.

## Services

The Services view hosts adapters for explicitly configured local capabilities.
The public build does not bundle an upstream service, service URL, or working
credential. Adapter code in the tree is not evidence that an integration is
configured or currently authenticated.

- Browser code calls CHROTE-owned routes rather than raw upstream ports.
- Tokens stay in server-side runtime configuration.
- Missing or unhealthy upstream services render a clear degraded state.
- Service adapters do not become hidden prerequisites for terminals, Files,
  Beads, Formations, or Server health.

## Scheduled tasks and Server status

- Scheduled tasks are stored as host-owned definitions with explicit enabled,
  run, lock, and history state.
- Stale locks may be diagnosed and recovered without silently double-running a
  task.
- Server status exposes health, resource observations, runtime events, and
  bounded history useful for diagnosing restarts and degradation.
- Telemetry is operational evidence, not an external analytics pipeline.

## Security and deployment boundary

- Default HTTP binding is loopback-only.
- CHROTE has no built-in application login. Anyone who can reach the dashboard is
  inside the trusted operator boundary and can reach terminal-grade capabilities.
- Remote access belongs behind operator-controlled private networking and HTTPS.
- CORS configuration is not authentication.
- File roots constrain CHROTE APIs; they do not sandbox tmux agents or their Unix
  user.
- Secrets live in private runtime configuration, never tracked docs or browser
  storage.

See `SECURITY.md` for the public security contract.

## Roadmap boundary

### Shipped foundation

- Three durable terminal workspaces
- Unified Sessions/Files sidecars
- Files, Agents, Beads, Services, Scheduled, and Server views
- Session Bank and typed workload recovery

### Deliberate next steps

- Promote Formations only after its authoring, execution, security, install, and
  recovery contracts pass an explicit release gate
- Broader clean-install and release portability
- Stronger reusable harness adapters and run provenance
- Better agent-team ownership, reservation, handoff, and human-approval flows
- Additional local service adapters only when they earn a place in the operator workflow

Roadmap text is not permission to claim unshipped behavior in the README.

## Acceptance criteria

- The embedded dashboard and Go API build from one reproducible source tree.
- Go tests, race tests, vet, dashboard lint/unit/browser tests, dependency audits,
  and documentation checks pass in CI.
- `/api/health` returns a healthy response on a supported installation.
- Terminal sessions survive browser disconnects.
- Unknown or unavailable optional integrations degrade clearly.
- File access remains under configured roots.
- Recovery, Formations execution, and destructive actions fail loud at their
  safety boundaries.
- Public documentation describes supported generic installation and behavior,
  not one operator's private host layout.
