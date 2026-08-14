# ADR-0009: Record the Same-Pool tmux Input Fence as Infeasible

## Status

Accepted 2026-07-21 — infeasible under the accepted topology; descendants blocked.
Partially superseded 2026-07-26 by
[ADR-0010](0010-formations-agent-user-socket-ownership.md): the blanket
configured-socket refusal and the prohibition on a Formations-only socket no
longer hold; the same-UID input-fence infeasibility analysis stands.

## Context

ADR-0007 requires Terminal and Formations to resolve the same accumulated
cockpit session lineage while a run-bound authority accounts for every input,
attachment, target, resize, history, lifecycle and topology effect. Stock tmux
does not make an owner-accessible socket an enforcement boundary. Any process
with the server owner's UID can open that socket and bypass a CHROTE mutex,
wrapper or API.

CHROTE does not yet have one production resolver shared by Terminal and
Formations. There are three current paths:

1. Terminal tabs reach `proxy.NewTerminalProxy`, its launch environment, and the
   root `terminal-launch.sh`. That script independently validates the configured
   Unix user, maps it to a socket, and attaches to the named session.
2. Terminal API inventory and mutation use `TmuxHandler.targetForUnixUser`,
   `listSessionsForTarget`, and the exact-pane `resolveSendPane` path in
   `src/internal/api/tmux.go`.
3. `TmuxFormationExecutor` uses its separate `config.Socket` directly. Its
   production guard deliberately rejects configured cockpit sockets with
   `session_target_attachment_audit_unavailable` before a tmux client call.
   (Superseded: since ADR-0010 the executor accepts any configured socket
   whose backing server the configured agent-user owns.)

Resolver convergence remains mandatory for any future candidate. It is not
implemented today, and the infeasible result below blocks `ctx-ug7.22` on the
new operation-time kernel-boundary decision `ctx-ug7.37` rather than permitting
these paths to converge behind an unproven interaction seam.

This decision is based on CHROTE at commit
`9d5d119610c87688843b80455d0d09deed92fbdc` and upstream tmux 3.6a at commit
`cc117b5048f77a4842820f8ebbe3a86e5c077224`. The upstream source establishes
these boundary facts:

- tmux automatically gives the server owner write access in
  [`server_acl_init`](https://github.com/tmux/tmux/blob/3.6a/server-acl.c#L52-L61),
  and `server-access` refuses to change the owner's access in
  [`cmd_server_access_exec`](https://github.com/tmux/tmux/blob/3.6a/cmd-server-access.c#L97-L100);
- `server-access -d` marks every client of that UID for exit and removes its
  server ACL in
  [`cmd_server_access_deny`](https://github.com/tmux/tmux/blob/3.6a/cmd-server-access.c#L47-L67);
- the server obtains the connecting peer UID from the kernel in
  [`proc_add_peer`](https://github.com/tmux/tmux/blob/3.6a/proc.c#L313-L319);
- a read-only peer is rejected unless every parsed command has
  `CMD_READONLY`, and attached key and paste events are discarded, in
  [`server_client_dispatch_command`](https://github.com/tmux/tmux/blob/3.6a/server-client.c#L3502-L3507)
  and
  [`server_client_handle_key`](https://github.com/tmux/tmux/blob/3.6a/server-client.c#L2587-L2599);
- input can reach a pane through more than `send-keys`: `paste-buffer` writes
  directly in
  [`cmd_paste_buffer_exec`](https://github.com/tmux/tmux/blob/3.6a/cmd-paste-buffer.c#L87-L106),
  and `pipe-pane -I` writes pipe input in
  [`window_pane_read_callback`](https://github.com/tmux/tmux/blob/3.6a/cmd-pipe-pane.c#L188-L200).
- terminal queries parsed from pane output call `input_send_reply`, which writes
  reply bytes directly to the pane bufferevent, in
  [`input_reply`](https://github.com/tmux/tmux/blob/3.6a/input.c#L1133-L1164).

The OS route analysis is also source-backed. Linux Yama
`ptrace_scope=2` permits attach operations only to `CAP_SYS_PTRACE` callers;
both `process_vm_writev` and `pidfd_getfd` use
`PTRACE_MODE_ATTACH_REALCREDS`. Linux 6.6 rejects unprivileged `TIOCSTI` when
`dev.tty.legacy_tiocsti=0` in
[`tiocsti`](https://github.com/torvalds/linux/blob/v6.6/drivers/tty/tty_io.c#L2259-L2280).
An open slave PTY still admits `TIOCSWINSZ`; Linux updates the tty size and
signals the foreground process group in
[`tty_do_resize`](https://github.com/torvalds/linux/blob/v6.6/drivers/tty/tty_io.c#L2319-L2372).
Yama mode 2 and disabled legacy `TIOCSTI` would be host-wide prerequisites for
one rejected seal direction, not settings CHROTE may silently change; they are
insufficient to revoke the open-file descriptions identified below.

Stock read-only access is a negative security boundary, not the CHROTE
observation API. In 3.6a its `CMD_READONLY` command set is attach, detach,
switch-client and list-clients; even `list-sessions`, `list-panes` and
`capture-pane` are rejected. Attach and switch are target routes, and a client
without `ignore-size` can alter calculated pane size. An untrusted identity
therefore receives no raw socket access, including read-only access.

## Route inventory

### Reachable CHROTE routes

| Consumer | Current route | Relevant tmux effects |
| --- | --- | --- |
| Terminal launch | `src/internal/proxy/terminal.go` to root `terminal-launch.sh` | source/session selection, `has-session`, `attach-session`, attached key, paste, mouse, focus and client-size traffic |
| Terminal API | `src/internal/api/tmux.go` | inventory and capture; create, split, layout, buffer/paste/send, kill, rename, key and option changes; recovery creation and resume |
| Scheduled sends | `src/internal/api/scheduled.go` | `has-session`, literal `send-keys`, Enter |
| Oracle | `src/internal/api/oracle.go` | list and capture through the ambient Terminal handler |
| Formations adapter | `src/internal/formations/tmux_executor.go` | separate configured-socket list, describe, buffer, send and capture path; production currently rejects it before these calls |
| Formations-generated agent routes | `peerTurnExtraLines`, `leaderAgenticExtraLines`, and `appendOrchestrationTeamEvent` in `src/internal/formations/tmux_executor.go` | puts the raw `config.Socket` plus native capture/send instructions into peer/leader prompts and run artifacts, making spawned agents direct protocol clients |
| Archon dogfood | `src/cmd/archon/main.go` | list, create and attach on an explicitly disposable Formations socket |
| Operator grant helper | `scripts/chrote-tmux-grants.sh` | filesystem ACLs and writable `server-access -a` grant |
| Recovery tooling | `scripts/tmux-recovery/` and API restore handlers | disposable create/kill plus API-owned restore, topology and resume effects |

The former Persistent Agents route was retired by
[ADR-0015](0015-access-first-non-interference.md) and is not part of the current
tmux command surface.

The current raw Terminal attach and broad writable grant helper cannot be used
by any future complete fence. Merely routing read-only collectors through a
broker does not close the remaining pane-slave routes.

### Complete tmux 3.6a command surface

`tmux list-commands` reports the following 90 commands. This grouping accounts
for every command; it is an inventory, not a safe-command allowlist.

| Class | Commands | Fence relevance |
| --- | --- | --- |
| Input and byte ingress (4) | `send-keys`, `send-prefix`, `paste-buffer`, `pipe-pane` | May write bytes, control keys or fan-out input. |
| Attach, target, client and pane mode (21) | `attach-session`, `detach-client`, `switch-client`, `select-pane`, `last-pane`, `select-window`, `last-window`, `next-window`, `previous-window`, `find-window`, `choose-client`, `choose-tree`, `display-panes`, `copy-mode`, `clock-mode`, `customize-mode`, `lock-client`, `lock-session`, `lock-server`, `suspend-client`, `refresh-client` | Changes attachment, selected target, mode, client geometry or the path receiving later input. |
| Resize, topology and lifecycle (26) | `break-pane`, `join-pane`, `move-pane`, `swap-pane`, `resize-pane`, `kill-pane`, `respawn-pane`, `split-window`, `new-window`, `kill-window`, `respawn-window`, `link-window`, `unlink-window`, `move-window`, `swap-window`, `rotate-window`, `resize-window`, `select-layout`, `next-layout`, `previous-layout`, `new-session`, `kill-session`, `kill-server`, `rename-session`, `rename-window`, `start-server` | Changes pane incarnation, target identity, layout, dimensions or server/session/window lifetime. |
| History, buffer and prompt state (11) | `clear-history`, `capture-pane`, `choose-buffer`, `load-buffer`, `set-buffer`, `delete-buffer`, `save-buffer`, `show-buffer`, `list-buffers`, `clear-prompt-history`, `show-prompt-history` | Mutates or exposes evidence and stages later input. |
| Programmable and administrative indirection (14) | `bind-key`, `unbind-key`, `command-prompt`, `confirm-before`, `display-menu`, `display-popup`, `if-shell`, `run-shell`, `source-file`, `set-hook`, `set-option`, `set-window-option`, `set-environment`, `server-access` | Installs deferred commands, changes synchronization or aliases, spawns shell paths, changes ACLs or recursively invokes tmux. |
| Observation and synchronization (14) | `display-message`, `has-session`, `list-clients`, `list-commands`, `list-keys`, `list-panes`, `list-sessions`, `list-windows`, `show-environment`, `show-hooks`, `show-messages`, `show-options`, `show-window-options`, `wait-for` | Usually observes or synchronizes, but formats, hooks and `#()` jobs make raw admission unsafe. |

The non-command surface is also in scope:

- attached terminal key, paste, mouse and focus events, including control-mode
  and transient clients;
- automatic client-size reflow on attach, detach and `refresh-client -C`;
- `synchronize-panes`, key bindings, hooks, command aliases, menus, choosers,
  prompts, shell commands and `#()` format jobs;
- natural pane exit, tmux server exit/restart/socket recreation, owner-UID OS
  signals, application terminal sequences, slave-PTY output, and slave ioctls
  including winsize, termios and line-discipline changes that alter input,
  application, history or screen state; terminal-emulator-generated reply bytes
  (for example status, device or cursor reports) solicited by pane output;
- direct raw socket/protocol clients, socket hard links or path replacement,
  and a client connection or descriptor opened before a seal transition; and
- same-UID `/proc` or `pidfd_getfd` descriptor duplication, ptrace/
  `process_vm_writev`, terminal `TIOCSTI` input injection, filesystem ACLs,
  permission-bypassing capabilities, user/mount namespaces and a sibling socket
  or file sharing the sealed directory.

The required fence would have to mediate both the socket and pane-slave input
boundaries and continuously account for tmux effects. It cannot attribute
ordinary pane output to a particular writer once a slave description is open:
tmux sees bytes on the PTY master, not the process that wrote them. A pathname
seal can deny new opens, but it cannot prove every pre-seal open description is
gone. The infeasible result below identifies kernel-held references that evade
that drain. Any future operation-time monitor must trust only the registered
pane lineage for its own application state/output; terminal replies are baseline
evidence, not workflow/human input or closure authority.
Filtering a command subset or intercepting only named CHROTE routes cannot meet
the narrower input-plane contract either.

## Disposable evidence

No prototype used `/run/user/2001/chrote-tmux` or a live session. Each tmux
server used a fresh `/tmp/chrote-tmux.*` `TMUX_TMPDIR` or a fresh equivalent
inside a disposable container, explicit sockets, and exact-session cleanup. No
prototype used `kill-server`.

The test-only prototype in
`src/internal/api/tmux_input_fence_prototype_test.go` and
`src/internal/api/tmux_input_fence_prototype_support_test.go`:

- invokes the exact root `terminal-launch.sh` source/session resolver with a
  fake tmux, invokes `targetForUnixUser` plus `resolveSendPane`, and compares
  both current Terminal routes to one test-only broker resolver consumed through
  explicit Terminal and Formations adapters across two configured sources;
- proves duplicate names across sources fail before journal creation;
- returns a unique-epoch registration over the non-empty genesis and monotonic
  tail cursor/hash checkpoint to an immutable coordinator fixture rather than
  storing a sibling self-anchor, and rejects missing genesis, joint journal/
  local-anchor replacement, and closed-journal prefix rollback;
- serializes each target under one mutex, proves concurrent duplicate dispatch
  produces one pane effect, and latches the running process immediately after a
  post-intent failure;
- sends one workflow dispatch, opens and closes Peek generation 1, opens
  generation 2, rejects generation 1 before input, accepts generation 2, then
  durably closes generation 2, rejects replayed generation 1 before occupancy
  close, and proves generation 1 remains absent after recovery; and
- stores only non-secret generation record IDs: no Peek credential, dispatch or
  Peek bytes, or byte-derived hash appears in the journal.

Its six-field
`{unixUser,serverPid,sessionId,windowId,paneId,panePid}` value is deliberately a
**prototype observation tuple**, not ADR-0007's canonical authority fingerprint.
A future candidate must also bind `tmuxServerId`, race-safe socket identity,
`resolvedCwd`, `resolvedRoot`, harness/foreground-process start evidence,
target key and canonical `targetFingerprint` before any permit or journal
effect.

The bounded test serializes one target. It establishes dispatch/Peek/closure
ordering and recovery, not source-wide multi-target atomicity or an OS fence.
No production interface is selected; the reopening requirements below describe
what a later evidence-backed decision must prove before `.22` can proceed.

Run the test only under explicit disposable-server approval:

```bash
cd src
CHROTE_INPUT_FENCE_PROTOTYPE=1 GOTOOLCHAIN=go1.26.6 \
  go test -race ./internal/api -run '^TestSamePoolInputFencePrototype' -count=1 -v
```

The ownership boundary was separately exercised with tmux 3.6a in the
network-disabled image
`codercom/example-universal:ubuntu@sha256:90cb6666ca898f9f9870b835ea72607dff5641c1bbd911edc810bc017e2469b5`.
The container had only a read-only Homebrew tmux mount plus `CAP_SYS_ADMIN` for
container-internal bind mounts. The exact procedure was:

1. UID 2000 started one tmux session on a fresh socket and recorded
   `#{pid}|#{session_id}|#{window_id}|#{pane_id}|#{pane_pid}`.
2. UID 2000 granted UID 2001 tmux access.
3. Root self-bind-mounted the live socket directory, bind-mounted the same
   directory at a UID-2001-only path, then changed the directory and socket to
   UID 2001 mode `0700`/`0600`.
4. UID 2000 failed both parent-directory rename and raw `send-keys`; UID 2001
   delivered `BROKER_FENCED_INPUT` through the alternate path.
5. The exact tuple remained
   `124|$0|@0|%0|125` before and after sealing. UID 2001 killed only the exact
   disposable session, the server exited, both mounts were removed, and Docker
   removed the container.

The final sentinel was `TMUX_MOUNT_SEALED_SOCKET_PROTOTYPE_OK`. This proves the
stock server and pane can survive a socket ownership/mount seal; it does not
claim a complete adversarial boundary. The infeasibility analysis below rejects
that socket-only direction before `ctx-ug7.23`.

## Result: infeasible under the accepted topology

The bounded Go prototype establishes useful but limited facts: both current
Terminal resolver routes and a Formations test consumer can converge on one
multi-source observation, an immutable caller-held current high-water checkpoint
can detect replacement or rollback of that acknowledged tail, and serialized
dispatch/Peek/closure logic can fail closed. It does not model a valid
journal-ahead/coordinator-checkpoint-lag crash. The mount prototype also shows that a live stock tmux
server and pane survive a socket-directory ownership seal. Neither result proves
a complete OS input fence.

The socket seal fails on the pane slave. An ordinary source-UID process can open
`/dev/pts/N` before a seal, write terminal queries that make tmux inject reply
bytes, or issue `TIOCSWINSZ` and termios/line-discipline ioctls. Changing the
slave owner and scanning `/proc/*/fd` cannot prove those references are gone:

- Linux `SCM_RIGHTS` passes a reference to an open file description and is
  semantically equivalent to `dup` into the receiver, according to
  [unix(7)](https://man7.org/linux/man-pages/man7/unix.7.html). A sender can put
  the slave reference in an AF_UNIX receive queue and close its table entry
  before the scan; the receiver obtains a usable descriptor after the seal.
- `io_uring_register` takes long-term kernel references to registered files,
  according to
  [io_uring_register(2)](https://man7.org/linux/man-pages/man2/io_uring_register.2.html).
  A fixed-file reference is likewise not established absent merely because a
  process FD-table scan is clean.
- The recovered open file description bypasses later pathname ownership. It can
  elicit tmux's direct `input_send_reply` pane writes or reach Linux's
  `TIOCSWINSZ` resize/SIGWINCH path cited above.

This is not the disposable-server startup quirk named in the bead claim. It is a
stock Linux open-file-description and PTY-ownership property. Linux
`vhangup` is not a selective answer: it hangs up the tty for the registered
pane workload as well, breaking the exact live lineage the decision must
preserve. No finite userspace FD drain can distinguish and revoke every queued
or kernel-registered reference while retaining the authorized inherited pane
descriptors.

### Decision

Record the same-pool input fence as **infeasible under the accepted current
topology**. No socket, userspace broker, pathname ownership transition, tmux
hook, wrapper or in-process lock is selected for production. Production
Formations remains fail-closed with
`session_target_attachment_audit_unavailable` before every tmux client call.
(Superseded by ADR-0010: production Formations now runs on any agent-user-owned
configured socket; that refusal survives only for socket-identity pinning
failures, and slot attachment to pre-existing sessions remains unimplemented.)

An operation-time kernel reference monitor might form a future primitive because
it could decide every slave write/ioctl by trusted pane lineage even when the
caller received a pre-opened file description. That is a new kernel-policy and
process-classification architecture, not a proven detail of this decision.
BPF LSM, SELinux or another mechanism is deliberately not selected without a
disposable-VM prototype covering `SCM_RIGHTS`, `io_uring`, cgroup/process
lineage, fork/exec, crash and rollback. Treating that research direction as the
chosen primitive would move the load-bearing decision into `.22`.

### Trust boundary required to reopen the decision

A future candidate must preserve the existing tmux server, sessions, panes and
pane processes. Raw root and compromise of the exact registered pane workload
for one target remain outside the product threat model. Every other process
sharing the source UID, including a registered pane lineage for a different
target, is untrusted for:

- tmux socket connect/protocol, inherited or queued client descriptors;
- slave-PTY open, write, `TIOCSTI`, winsize, termios and line-discipline
  operations, including descriptors transferred or registered before arming;
- target, attachment, resize, history, lifecycle and topology effects; and
- generated terminal replies that enter pane input.

Enforcement must run at each effect on the open file description, not merely at
path open or during a point-in-time FD scan. It must bind an exact, root-owned
pane-lineage identity that untrusted same-UID processes cannot join. Unknown
lineage, policy drift, monitor loss or unsupported kernel state is unavailable
before effect and latches every affected target after occupancy.

### Required future interface

Any new decision must freeze one concrete kernel mechanism behind one semantic
Terminal/Formations authority with no generic escape:

```text
Resolve(source, selector) -> CanonicalTargetBinding
Inventory(source) -> []CanonicalTargetBinding
EstablishInteractionFence(source, targetSet, kernelPolicyManifest, maintenancePermit) -> FenceReceipt
ArmAudit(target, fenceReceipt, writerFence, routeGate, journalGenesisAnchor) -> AuditRegistrationReceipt
AcquireOccupancy(target, auditRegistration, fenceReceipt, occupancyPermit) -> OccupancyReceipt
OpenPresentation(target, ordinaryTerminal | runBoundPeek, permit) -> PresentationHandle
ClosePresentation(target, presentationHandle, permit) -> PresentationClosureReceipt
Dispatch(target, oneShotDispatchPermit, bytes) -> EffectReceipt
ReconcileInterrupt(target, oneShotInterruptPermit, bytes) -> EffectReceipt
OpenPeekGeneration(target, peekCapability, priorClosure) -> GenerationPermit
SendPeek(target, generationPermit, bytes) -> EffectReceipt
CloseGeneration(target, generationPermit) -> ClosureReceipt
CloseOccupancy(target, closurePermit) -> ContinuousAuditReceipt
ApplyTerminalOperation(source, targetSet, typedOperation, ordinaryPermit) -> EffectReceipt
RemoveInteractionFence(source, fenceReceipt, maintenancePermit) -> RemovalReceipt
```

`EstablishInteractionFence` must complete before `ArmAudit`; `ArmAudit`
must externally anchor a non-empty genesis before `AcquireOccupancy`. One
serialized source event loop and generation must own all target leases and every
one-target, target-set or source-wide effect. Unseal/removal requires every
registration, presentation, client, occupancy and barrier on the source to be
closed.

Presentation close is explicit. Normal EOF, abrupt relay loss and broker crash
do not produce a closure receipt until the exact broker-side tmux client is gone,
the output/input route is drained, and the content-free detach barrier is
durable. Run-bound presentation handles bind the latest Peek generation.
Durable records use opaque non-secret record IDs, never tmux client IDs or
authorization credentials.

The journal must keep a coordinator-anchored monotonic high-water cursor and
tail hash, not only a genesis hash. Peek generations use a durable monotonic
number plus exact prior-close receipt; journal record IDs are separate from
non-journaled RPC credentials. Unknown outcomes never resend, unknown schemas
never downgrade, and reserved tmux buffers are quarantined and removed before
any later authorization. Recovery must distinguish acknowledged-tail rollback
from a journal legitimately ahead of the last coordinator checkpoint: the
former rejects, while the latter verifies the anchored prefix and latches for
no-resend reconciliation before accepting any input or closure.

### Failure modes and operational cost

The current topology's failure mode is deterministic: the complete capability
cannot register, so all stock cockpit targets remain unavailable to Formations
and its adapter makes zero tmux calls. Ordinary Terminal remains unchanged. It
is not permissible to enable a partial socket-only fence.

A future operation-time kernel monitor would add a pinned kernel/LSM feature,
root-owned policy loader, protected lineage/cgroup lifecycle, per-effect policy
maps, kernel-event audit, broker RPC/output relay, durable journal, typed
replacement for every raw route, and disposable-VM certification. Kernel,
policy, cgroup or audit drift would latch the whole source. That cost and host
compatibility impact must be measured by a new decision; this ADR does not
authorize it.

### Rollback boundary

This decision performed no live mount, ownership, ACL, sysctl, cgroup, LSM,
socket, service, session or pane change. Its rollback is therefore the existing
stock fail-closed adapter and removal of test-owned disposable state only. The
prototype tests kill their exact disposable sessions and remove only their fresh
`/tmp/chrote-tmux.*` roots; no prototype uses `kill-server`.

A future kernel candidate must specify rollback before implementation. At
minimum it must stop admissions, close all presentations and occupancies, fsync
the terminal high-water receipt, prove every old-pane latch, remove policy only
under a root maintenance permit, and verify no raw socket or slave route becomes
reachable during partial rollback. An identity change cannot resume a run.

## Rejected alternatives

- **Owner-owned raw socket plus mutex, wrapper or hooks.** The owner UID can
  connect directly and tmux refuses to make its owner read-only.
- **Socket ownership/mount seal plus userspace broker.** It leaves the pane slave
  and terminal-reply route outside the broker.
- **Socket plus slave pathname ownership and FD-table drain.** `SCM_RIGHTS`
  queues and `io_uring` registered files retain pre-seal open file
  descriptions outside the scan.
- **An unsealed proxy.** A process being fenced can bypass it and reach the
  upstream socket.
- **A broker-owned replacement tmux server.** It creates a new server and pane
  incarnation, loses accumulated exact lineage, and requires a privileged UID
  transition for source-owner workloads.
- **A patched tmux fork.** Existing servers cannot adopt it without restart and
  direct slave-PTY effects remain outside tmux's client parser.
- **Pane-local PTY or shell relay.** It cannot retrofit exclusive ownership of
  already-open descriptions or mediate attachment, history, topology and server
  lifecycle.
- **Yama, disabled legacy `TIOCSTI`, seccomp, `LD_PRELOAD` or client-binary
  filtering alone.** These do not mediate every write/ioctl on transferred or
  registered slave file descriptions.
- **A speculative BPF-LSM/SELinux design.** Operation-time kernel mediation is
  the only remaining direction, but choosing a specific policy without a
  disposable-VM bypass prototype would be speculation.
- **A Formations-only server or socket.** It violates the same-pool and
  accumulated-context contract.

## Exact handoff for `ctx-ug7.22`

`.22` has a durable Beads dependency on `ctx-ug7.37` and must not implement the
rejected root-sealed broker from earlier notes. **ADR-0010 (2026-07-26)
supersedes parts of this handoff: item 1's blanket refusal and item 2's
"Formations-only pool" prohibition no longer bind — read ADR-0010 first. The
kernel-mechanism requirement and everything else stand.** Its exact contract
is:

1. Keep every non-temporary or configured cockpit target returning
   `session_target_attachment_audit_unavailable` through the Formations executor
   before its list, describe, capture, send, attach, detach, create or kill
   calls. Ordinary Terminal remains unchanged.
2. Do not migrate a production server/socket, change live socket or slave
   ownership, load kernel policy, alter sysctls/cgroups, or introduce a
   Formations-only pool.
3. Before implementation, obtain a superseding decision that selects one exact
   operation-time kernel mechanism. The decision must name kernel/config
   prerequisites, policy code and loader identity, process-lineage primitive,
   source/target serialization, trust boundary, operational cost and reversible
   rollback.
4. Prove that mechanism in a disposable VM against a pre-seal slave FD hidden in
   an `SCM_RIGHTS` receive queue and an `io_uring` fixed-file table, including a
   reference sent from another registered target on the same source. After
   arming, external and cross-target same-UID receivers must fail write,
   terminal-query reply injection, `TIOCSWINSZ`, termios and line-discipline
   effects while only the exact target's pre-existing pane lineage continues
   input/output.
5. In the same candidate, close raw tmux client/descriptor, attach, target,
   resize, history, lifecycle, topology, prompt-generated native-tmux and
   presentation routes. Terminal and Formations must consume one converged
   production resolver and full canonical DATA-MODEL fingerprint.
6. Only after that new decision may `.22` implement the complete interface
   above, anchored tail high-water receipts, explicit presentation closure,
   monotonic Peek, content-free journals, no-resend recovery, schema downgrade
   rejection and typed operation scopes.
7. Leave `.23` an immutable manifest containing source/kernel/policy/loader/
   broker/RPC/journal/resolver/typed-route identities and every changed path.
   Every must-fix edit changes candidate identity.

## Exact handoff for `ctx-ug7.23`

`.23` is blocked until `.22` has a candidate permitted by a superseding
decision. Certification then must:

1. Exact-match the immutable `.22` manifest before testing.
2. Use fresh disposable multi-UID, multi-source, multi-target VM state and the
   exact production Terminal resolver for both Terminal and Formations.
3. Attempt all 90 tmux commands inventoried above plus attached key/paste/mouse/
   focus, control mode, client size, synchronized input, `pipe-pane -I`,
   hook/key/alias/menu/chooser/prompt/shell/source-file/`#()`, direct protocol,
   hard-link/path replacement and pre-opened tmux clients.
4. Exercise fresh and pre-opened slave descriptors; same-target, cross-target
   and external `SCM_RIGHTS` queued and `io_uring` registered references;
   `/proc`/pidfd duplication; ptrace/
   process-vm; `TIOCSTI`; winsize, termios and line discipline; terminal-query
   replies; fork/exec/lineage migration; signal, natural exit, server restart and
   socket recreation routes.
5. Crash before and after kernel-policy arm, lineage-map update, external audit
   anchor, occupancy, every input intent/effect/barrier, presentation close,
   Peek close/reissue, result, release and policy removal. Prove no resend,
   stale generation, prefix rollback, lost latch or partial unseal.
6. Re-prove stock unavailable/zero-call behavior without the exact registered
   capability, unknown-schema high-water rejection, prompt/env/artifact socket
   non-disclosure, content-free records and exact cleanup.
7. Run focused/full/race Go gates, vet, doc lint, diff check, secret scan and
   independent architecture/security reviews with zero P0/P1 and no unexplained
   skip. Record explicitly that no live tmux, service, deploy, merge or push
   occurred.

## Consequences

The same cockpit lineage requirement remains intact; it is not weakened into a
separate Formations pool or a partial broker claim. Implementation stops at the
real kernel boundary instead of transferring an unproven decision to descendants.
A future operation-time reference monitor requires the new, evidence-backed
`ctx-ug7.37` decision before `.22` or `.23` may proceed.
