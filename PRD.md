# CHROTE Product Requirements

## Vision

CHROTE is a private browser cockpit for host-owned work. The browser is
disposable glass; the configured Linux or WSL host owns the terminals, agents,
files, builds, tests, Beads, schedules, runtime history, and local services.

CHROTE gives one trusted operator a coherent view of that state without moving
its source of truth into the browser. tmux owns terminal sessions, the filesystem
owns files, `bd` owns issues, and explicit host configuration owns durable
workloads and integrations.

CHROTE grows only where it makes host-owned work easier to understand and
deliberately coordinate. It does not become a second IDE, orchestration harness,
host supervisor, or pile of dashboards for work already clearer in a terminal.

## Product contract

### Current goals

1. Make tmux sessions visible and controllable from a browser.
2. Keep browser disconnects and CHROTE restarts from deliberately terminating
   externally owned tmux work.
3. Put terminal sessions and configured files beside each other without
   pretending to be an IDE.
4. Surface Beads, scheduled tasks, server health, and optional local adapters in
   the cockpit.
5. Keep deployment private to localhost or an operator-controlled private
   network unless explicitly configured otherwise.

### Current non-goals

- hosted multi-tenant SaaS, an application identity system, or an OS sandbox;
- replacing tmux, `bd`, Git, or an agent's native harness;
- autonomous agent society, implicit agent-to-agent chat, or a required harness;
- automatic reconstruction of ordinary work after process death or host reboot;
- recovery, restore, cleanup, migration, or one-off repair in request-path code;
- restricting accessible host state beyond configured roots and Unix permissions.

Rare, judgment-heavy operations are agent skills. A workload that truly needs
reboot durability belongs in explicit operator-owned host configuration.

## Current views

The core product jobs are Terminal, Files, Beads, Scheduled, Server, and
Settings. Three configurable terminal workspaces present the Terminal job.
Services is an optional adapter surface and is not a prerequisite for the core.

| View | Operator job |
| --- | --- |
| Terminal 1 | First independent terminal workspace with one to four tmux-backed windows |
| Terminal 2 | Second independent terminal workspace |
| Terminal 3 | Third independent terminal workspace |
| Files | Browse, inspect, edit, compare, and send configured workspace files |
| Beads | Inspect configured `bd` workspaces, issues, ready work, and health |
| Services | Operate explicitly configured local adapters through CHROTE-owned routes |
| Scheduled | Inspect and manage scheduled prompt tasks and their run history |
| Server | Inspect server health, resources, events, and bounded history |
| Settings | Configure appearance, terminal behavior, feature flags, and session cleanup |

Help and keyboard guidance live in the application shell rather than a persistent
workspace.

## Durable terminal workspace

- Each terminal workspace owns its layout, attached sessions, labels, and Files
  state. Sessions presentation is application-global.
- A workspace can show one to four terminal windows.
- tmux owns process and session lifetime; browser storage owns presentation, not
  shell state.
- A disconnect or CHROTE restart must not terminate external tmux work. A
  session may still exit naturally, be stopped externally, or disappear with
  its process or host.
- Session-row selection means **Peek**. Navigation and assignment are explicit
  actions.
- Sessions and Files sidecars are closed by default. Sessions state is shared
  across workspaces; Files state persists independently per workspace.
- Refit, reconnect, bulk cleanup, and Send to Session remain explicit operator
  actions.

## Files

- `CHROTE_ROOTS` plus canonical-path checks define the application boundary.
- CHROTE exposes everything under those roots that its service identity can
  access. The only cross-user asymmetry is Unix permissions; CHROTE does not
  encode a second access policy.
- Symlinks and mutations must remain within configured roots after resolution.
- Unix permission errors are reported plainly, not disguised as empty or
  missing paths.
- Files is a terminal companion, not a general-purpose IDE.

## Beads and agent observability

- CHROTE uses configured modern `bd` commands and JSON output.
- Multiple Beads workspaces may be configured; an unavailable workspace is a
  degraded integration, not a dashboard crash.
- `bv` is optional and remains a terminal-side viewer, not the issue source of
  truth.
- Agent work is observed through tmux and native harness state without requiring
  one agent runtime or persona model.

## Services

Services hosts adapters for explicitly configured local capabilities. The public
build bundles no upstream, service URL, or working credential.

- Browser code calls CHROTE-owned routes; tokens remain server-side.
- Missing or unhealthy upstreams render a clear degraded state.
- Optional adapters do not become hidden prerequisites for core views.

## Scheduled tasks and Server status

- Scheduled tasks are host-owned definitions with explicit enabled, paused,
  run, and history state.
- A task sends its prompt literally through CHROTE's guarded tmux path. Delivery
  receipts do not claim that the terminal application consumed the prompt.
- Server status exposes health, resource observations, runtime events, and
  bounded history. Telemetry is operational evidence, not analytics.

## Security and deployment boundary

- Default HTTP binding is loopback-only.
- CHROTE has no built-in login. Anyone who can reach it is inside the trusted
  operator boundary and can reach terminal-grade capabilities.
- Remote access belongs behind operator-controlled private networking and HTTPS;
  CORS is not authentication.
- Configured roots constrain file APIs but do not sandbox tmux agents.
- Access is broad by design. CHROTE never tightens ownership, modes, or ACLs to
  manufacture isolation; explicit grants may only add access.
- Secrets live in private runtime configuration, never tracked docs or browser
  storage.

See [`SECURITY.md`](SECURITY.md) for the public security contract.

## Roadmap boundary

CHROTE may improve the six core jobs and add local adapters that earn a real
operator workflow. Experimental Formations and Archon orchestration contracts
were extracted to
[chrote-agent-formations](https://github.com/Perttulands/chrote-agent-formations)
and are not part of this product's release promise.

Roadmap text is not permission to claim unshipped behavior in the README.

## Acceptance criteria

- The dashboard and Go API build reproducibly from one source tree.
- The single CI quality job runs Go format/vet/race coverage, dashboard unit,
  lint and browser tests, source contracts, and the built-server contract.
- `/api/health` returns a healthy response on a supported installation.
- Browser disconnects and CHROTE restarts do not terminate external tmux work.
- Optional integrations degrade clearly.
- File access remains under configured roots and Unix permissions.
- Public docs stay generic and host-neutral.
