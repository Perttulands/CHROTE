# ADR-0014: Persistent Agents Are Supervised by systemd, Not by the CHROTE Server

## Status

Accepted 2026-08-03 — engineering decision, recorded for `chrote-gfu.2`.

Supersedes the supervision-ownership half of
[ADR-0001 (workload-aware session recovery)](0001-workload-aware-session-recovery.md).
That ADR's descriptor model, one-owner rule, and unresolved-not-guessed
discipline survive unchanged; only its assignment of *continuous supervision* to
an in-server Persistent Agents component is replaced. Its mode/owner matrix gains
one cell, described under Decision 3.

## Context

The cockpit offers a per-session lock. Locking a session means "this agent should
still be running tomorrow" — after a crash, a service restart, or a host reboot.
Today that promise is kept by the CHROTE web server itself: a goroutine started
at `src/cmd/server/main.go:174` reconciles a desired-state file every 15 seconds,
recreates missing tmux sessions, and types a resume command into the new pane.

An inventory taken 2026-08-03 (bead `chrote-gfu.1`) measured what that costs and
what it protects:

- ~2,900 lines of production Go dedicated to the feature, ~1,389 of which exist
  only to supervise: a `ps -eo` process-table walk to prove a live pane still
  runs the expected agent (`persistent_agents.go:835-1183`), an embedded 97-line
  Python heredoc that spawns a throwaway tmux session to read `/proc/*/fd` for an
  open transcript (`:1185-1383`), pane-text screen-scraping for strings like
  "do you trust" (`:1598-1617`), a six-state ladder with 1-5 minute backoff
  arithmetic and a three-strike failure counter (`:658-762`), and the reconcile
  loop itself (`:1852-2066`).
- ~2,500 lines of tests that die with it.
- Zero live users: the configured desired-state file is `[]` on the only lane,
  so the loop iterates an empty slice every 15 seconds and has never restarted
  anything in production.

Every one of those mechanisms has a systemd equivalent that this host already
runs. `Restart=`/`RestartSec=`/`StartLimitBurst=` replace the backoff ladder.
Boot ordering replaces reboot recovery. `journalctl` replaces the last-error
field. The process-identity archaeology exists only because the supervisor
inherits panes it did not create; a launcher that starts the agent itself knows
what it started.

The pattern is not hypothetical here. Two Telegram agent lanes (`minerva`,
`marrow`) were migrated to exactly this shape on 2026-08-03: a template user
unit, a per-agent config file, and one launcher that refuses to create the tmux
server. Both survive service restart and host reboot and are observable with
`systemctl` and `journalctl`.

Two facts constrain the design more than the LOC does:

1. `ADR-0001`'s mode/owner matrix has no legal cell for what this ADR builds.
   Agent-mode descriptors require a *restarting* Session Bank or Persistent Agent
   owner; managed descriptors require a *non-restarting* external manager. A
   systemd-owned agent is agent-mode, externally managed, and restarting.
   The same constraint is pinned in code at
   `src/internal/api/managed_recovery_status.go:166-168` and in
   `scripts/tmux-recovery/recovery-manifest.schema.json:62`.
2. Half of the target architecture already ships. `scripts/tmux-recovery/restore.py:131-233`
   writes a status registry the Go server already reads
   (`managed_recovery_status.go`), enforcing `managerKind == "systemd-user"`,
   a `*.service` reference, and atomic 0600 writes. The `external_manager` lane
   is specified on both ends and connected on neither.

Notably, systemd is **not** a rejected alternative in ADR-0001. It appears once,
at line 72, only to say that ADR left systemd orchestration unchanged. There is
no prior decision against it to overturn.

## Decision

### Decision 1 — Supervision moves to systemd; the server supervises nothing

A locked session is owned by a systemd **user** unit instantiated from one
template, `chrote-agent@<name>.service`, whose configuration is a per-agent file
written by CHROTE. The CHROTE server process starts no supervision goroutine,
holds no retry state, and never recreates a session.

Rejected alternatives:

- **Delete the persistence feature entirely.** The strongest competitor, and the
  cheapest: it removes ~3,900 lines and adds nothing. Rejected because the
  affordance is a product requirement — the cockpit's promise is that the host
  owns durable work — and because the host already runs agents that must survive
  reboot. Deleting it would move the problem to hand-written units per agent,
  which is where minerva and marrow came from. What this ADR keeps is the *UI
  path to that outcome*, not a new mechanism.
- **Keep the in-server supervisor, collapse the state ladder.** Removes maybe 400
  lines and leaves the loop, the store, the process archaeology, and the
  ownership arbitration in place. It also leaves the structural defect that
  motivated the review: a web server that must stay running for durable work to
  stay durable. Rejected as the worst ratio of disruption to benefit.
- **A separate CHROTE supervisor daemon.** Rejected on the complexity budget
  (`docs/plans/2026-05-06-polis-ideas-migration.md`: zero new daemons) and
  because it is systemd with fewer features.

### Decision 2 — What systemd actually supervises: the launcher stays attached

`ExecStart` runs the ensure-launcher in the foreground for the life of the agent.
It is `Type=notify`; startup succeeds only after the launcher has independently
confirmed the running pane process and sent `READY=1`.

The launcher's lifecycle is:

1. Refuse to proceed if no tmux server answers the configured socket, naming the
   keeper unit. It never creates a tmux server. (See Decision 6.)
2. Start the pane through one fixed, trusted launcher. For a missing session,
   create it with that launcher as its initial command. For an existing session,
   adopt it only when the pane already has that exact start command and config;
   otherwise replace its sole pane and resume the same native transcript. The
   controlled replacement is necessary because a generic shell remains alive
   after its child agent exits and therefore cannot be an exact liveness signal.
   Typed config reaches pane mode through tmux's environment, not command text;
   pane mode invokes the agent with its canonical argv and `exec`, without
   `send-keys` or a rendered resume command.
3. Observe the pane's actual `/proc` command line and process start ticks. Confirm
   the configured agent kind, native session id, and Hermes profile where
   applicable; publish an invocation-bound receipt; only then notify systemd that
   startup is ready.
4. Then **watch**: block until the session disappears or the agent process in it
   exits, and exit non-zero when it does.

Step 4 is what makes `Restart=on-failure` meaningful. A launcher that created the
session and exited would leave systemd supervising a dead helper: the unit would
report `inactive (dead)` seconds after a successful start, and nothing would
notice the agent dying an hour later. This is precisely the shape of today's
`revivePersistentAgent` (`persistent_agents.go:1852-1875`), which sends its keys
and returns.

An explicit `systemctl --user stop` suppresses restart by systemd's own
semantics; the launcher does not need to distinguish operator stops from crashes.

Honest statement of the poll: watching for in-pane agent death still requires
polling — tmux offers no exit notification for a process inside a pane. The
decision is not *whether* to poll but *where*: one poll per agent, inside that
agent's own unit, visible in its journal, restartable in isolation — instead of
one loop in the web server that iterates every agent and whose failure is
invisible. The watch interval is a unit-level setting, not a global one.

Rejected alternatives:

- **`Type=oneshot` with `RemainAfterExit=yes`.** Makes "active" mean "we once ran
  the launcher", which is exactly the false-health signal the review flagged.
- **`Type=forking` with the tmux session as the daemon.** tmux's server is shared
  and outlives the unit by design; making systemd track it inverts the
  socket-keeper contract and re-creates the cgroup hazard of Decision 6.

### Decision 3 — The mode/owner matrix gains one cell; no fourth owner class

`external_manager` remains the owner kind for systemd-owned sessions. What
changes is that an external manager may be *restart-capable* when CHROTE
installed the unit:

| descriptor mode | owner kind | restart allowed |
| --- | --- | --- |
| `agent` | `session_bank` | yes |
| `agent` | `external_manager` | **yes, iff CHROTE installed the unit** (new) |
| `command`, `topology` | `session_bank` | yes |
| `managed` | `external_manager` | no |
| `unresolved` | any | no |

"CHROTE installed the unit" is not a claim taken on trust: it means the unit name
matches the template CHROTE owns (`chrote-agent@*.service`) and a corresponding
per-agent config file exists in CHROTE's state directory. A session managed by
someone else's unit — a hand-written service, a Telegram bridge lane — stays
`managed`/non-restarting and read-only, exactly as ADR-0001 requires.

This preserves ADR-0001's actual invariant (exactly one owner performs recovery)
while removing an accidental one (external managers are always passive). ADR-0001's
Rejected Alternatives entry against a session "owned by both CHROTE and an
external manager" is narrowed: CHROTE owning the *unit definition* and systemd
owning the *process lifetime* is one owner exercised through a supervisor, not
two competing recovery owners. Only systemd ever restarts.

Rejected alternative: **a fourth owner kind** (`chrote_managed`). Rejected
because it duplicates `external_manager`'s entire contract to change one boolean,
and every consumer — Go validation, the Python manifest schema, the dashboard
types — would need a new arm.

### Decision 4 — Cross-user control is permitted; the grant is narrow

The owner ruled on 2026-08-03 that CHROTE may control any user's units. The
security work is therefore mechanism correctness, not authorization:

- The grant is scoped to the verbs CHROTE needs and to the unit pattern
  `chrote-agent@*.service`. It never permits arbitrary unit names.
- Cross-user calls use `/usr/bin/sudo -n --` and the single root-owned helper at
  `/usr/local/libexec/chrote/chrote-agentctl`; neither executable is resolved
  through the service `PATH`. The tracked `services/chrote-agentctl.sudoers`
  grants the `chrote` service account only that helper, whose parser revalidates
  the target account, user-manager scope, verb, options, and exact unit pattern.
  The tracked host unit validates the grant with `visudo` before installing it.
- Session names are validated by the existing `core.ValidateSessionName`
  (`^[a-zA-Z0-9_-]+$`, max 50) *before* they are used to build a unit name, so a
  name can never inject a second unit, a flag, or a shell metacharacter.
- Desired config and reported runtime state use separate ownership domains.
  CHROTE owns each config directory; a named ACL grants the target account read
  access only. The target account owns a separate setgid receipt directory whose
  group is CHROTE's service group. Configs use mode 0640 as the ACL mask and
  receipts use mode 0640; neither domain is writable by the other owner. Receipt
  reads reject unsafe modes, ownership mismatches, non-regular files, and any
  symlinked path component.
- Config-derived invocation uses argv arrays with a timeout, per the repo's exec
  discipline. tmux accepts one fixed pane command string, containing only the
  validated installed launcher path and a constant mode flag; no config value or
  rendered resume command crosses that shell-command boundary.
- Before the HTTP server listens, a read-only `LoadState` query reaches every
  configured account through the real control path. This simultaneously proves
  the privilege grant, target user bus, and installed template. Failure is logged
  and projected to that account's sessions; the UI shows locking as unavailable
  and the mutation endpoint returns 503 before writing desired state.

This is consistent with the recorded threat model: the network perimeter is the
trust boundary, and agents have broad host access by design. Deliberately granted
authority is in scope; authority *leaking* through injection or traversal is not.

Rejected alternative: **same-user-only v1.** It would dodge the grant entirely,
but a cockpit host generally runs agents under more than one Unix account, so a
same-user cockpit could not lock the sessions the feature exists for.

### Decision 5 — Health means the unit is active *and* the launcher confirmed identity

`ActiveState=active` alone is not proof that the right agent resumed the right
transcript — it proves a process started. The status the UI renders is the
conjunction of:

- the unit's `ActiveState`/`SubState`, read live; and
- a launcher receipt derived from the actual pane process, not desired config. It
  records the observed agent kind and native session id, pane id, PID plus process
  start ticks, systemd invocation ID, and monotonic attestation time.

`healthy` = unit active **and** the receipt matches the desired identity, current
systemd invocation, current pane/process identity, and a live PID with the same
process start ticks. The attestation's monotonic time must be at or after
`ExecMainStartTimestampMonotonic`. Unit active with a stale, mismatched, missing,
or dead-process receipt is `degraded` and says so. `failed`/`inactive` are
reported verbatim from systemd. The launcher removes any prior receipt before
starting and receipt publication failure prevents `READY=1`.

Rejected alternative: **unit state alone.** Simpler, and wrong in the one case
that matters — a unit that cheerfully restarts an agent into the wrong transcript
would show green.

### Decision 6 — The launcher never creates a tmux server

The tmux server's lifetime belongs to its keeper unit -- a separate,
operator-configured unit whose only job is owning that socket. `tmux new-session` against a dead
socket forks a *server* into the caller's cgroup; if the caller is an agent unit,
a later restart of that unit kills every session on that socket. The launcher
therefore probes for a live server and fails loud, naming the keeper, rather than
reviving it.

This closes a gap that exists today and is not recorded in
[ADR-0013](0013-ttyd-restart-lifecycle-and-orphan-reaping.md)'s orphan inventory:
the current `revivePersistentAgent` runs inside the CHROTE server process, so a
revive against a dead socket would place a tmux server in `chrote-srv.service`'s
cgroup. ADR-0013's reaping inventory is extended accordingly.

### Decision 7 — Status source: live systemd reads, plus the existing registry

Health is read live from systemd at request time (Decision 5), because a written
registry can only ever be as fresh as its writer.

The existing `managed-status.json` contract is **kept**, not replaced: it remains
the interface for status published by *external* owners — the read-only
`managed` sessions of Decision 3 — and `scripts/tmux-recovery/restore.py` remains
its writer. CHROTE-installed units do not round-trip through it.

Rejected alternative: **route CHROTE's own units through the registry too.** It
would reuse a hardened serializer, but reintroduces a freshness problem the live
read does not have, and needs a writer on a timer — a poller by another name.

### Decision 8 — UI semantics of the lock

- **Lock** = write the per-agent config, then `enable --now` the unit. A session
  not already running through the trusted pane launcher is restarted once and
  resumes the same native transcript; the UI warns that in-flight terminal input
  may be interrupted. The badge appears when the unit is enabled.
- **Unlock** = `disable --now` the unit and remove the config. The tmux session
  and the running agent are **left alive**; unlocking stops the promise, not the
  work. The confirmation text says so. CHROTE removes desired state only after
  `disable --now` succeeds. A failed disable returns an error, keeps the lock and
  config registered, and projects an `unlock failed` state with the reason and
  retry action; the UI must not continue from that failure into session deletion.
- **Kill on a locked session** is no longer hidden. It is offered as "stop
  supervision and kill", which disables the unit first and then kills the
  session — the honest action. Killing a session while its unit is enabled would
  otherwise be undone by systemd within seconds, which is precisely the confusion
  the current UI avoids by hiding the button. Hiding it is replaced by doing the
  right thing.
- **Nuke All** continues to skip locked sessions.
- Badge states: locked/healthy, locked/degraded, locked/failed, locked with its
  tmux session absent, and managed (read-only, external owner). Desired locks
  are projected independently of live tmux inventory, so a failed unit remains
  visible and unlockable even when there is no session row to annotate.

## Consequences

- The CHROTE server process becomes stateless with respect to agent lifetime. A
  server restart, crash, or upgrade cannot interrupt a locked agent, and no
  locked agent depends on the cockpit being up. This is the property the feature
  always claimed and never had.
- ~1,389 lines of production Go and ~2,500 lines of tests are deleted; ~700 lines
  shrink. These figures are informational, recorded so the change can be
  recognized as the simplification it claims to be — they are not acceptance
  criteria. Acceptance is behavioral, per the implementation beads.
- One new systemd unit template is added. This spends the complexity budget's
  single permitted unit and adds no daemon, no dashboard tab, no binary, and no
  net new endpoint (the two persistence endpoints are re-implemented, not
  multiplied).
- `journalctl --user -u chrote-agent@<name>` becomes the diagnostic path for a
  misbehaving locked agent, replacing a `lastError` string in a JSON file.
- Operator burden shifts: locking now requires the target user's systemd user
  manager to be running (lingering enabled for headless users). The launcher and
  the API both fail loud naming this condition rather than silently not
  supervising.
- Session Bank is untouched. Its 279 entries, its one-shot recovery, and its
  ownership arbitration keep working; the only change is that the persistence
  store it consults for exclusions shrinks.
- The scheduler's rejection of systemd *timers*
  (`docs/SCHEDULING.md`) is unaffected and not contradicted: that decision is
  about scheduling arbitrary tasks in-process, this one is about who owns a
  long-lived process's lifetime.

## Enforcement

- A baseline test pins that the server starts no supervision goroutine.
- The launcher's refusal to create a tmux server is test-pinned.
- Descriptor validation tests cover the new matrix cell and, critically, that a
  unit CHROTE did not install cannot claim restart capability.
- Unit-name construction is tested against injection via session names.
- Cross-user and reboot behavior are proven once by an operator smoke
  (`chrote-gfu.10`), not by the disposable installer test, which deliberately
  never starts a real user service (`docs/TEST_STRATEGY.md`).
