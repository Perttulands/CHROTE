# ADR-0013: Browser Terminal Attaches Do Not Survive a chrote-srv Restart

## Status

Accepted 2026-07-27; amended 2026-08-30 after the Formations extraction recorded
by [ADR-0016](0016-core-boundary-and-formations-extraction.md), and again
2026-09-01 by [ADR-0018](0018-terminal-transport-ownership.md), which removed
ttyd.

**What still holds:** the decision. Browser terminal attaches are not required
to survive a `chrote-srv` restart, and they do not. **What is now history:** the
ttyd process, its `fuser -k` port reaper and `terminal-launch.sh`. CHROTE runs
each attach on a pseudo-terminal it owns, so the kernel hangs that attach up
when the server process exits by any means. The measurements below were taken
under ttyd and are kept because they are what the decision rests on.

Scope note: this settles the ttyd lifecycle and the orphan-reaping story. The one
operator-facing consequence — a visible terminal blink on every deploy — is
called out under Consequences so it can be overruled without reopening the
analysis.

## Context

`KillMode=process` on `chrote-srv.service` does not preserve browser terminal
attaches, because the loss is application-level rather than cgroup-level. Two
code paths caused it at the time of this decision:

1. The SIGTERM handler called `TerminalProxy.Stop()`, which sent
   `os.Interrupt` to ttyd. ttyd then killed every pty child, i.e. every
   `terminal-launch.sh` and its tmux attach client.
2. `TerminalProxy.Start()` ran `fuser -k <ttyd-port>/tcp` before spawning ttyd,
   so a ttyd that somehow survived step 1 was killed by the incoming instance.

### Measured, not assumed

A restart on 2026-07-27 with four terminals attached, comparing by pid rather
than by count:

| | before | after |
|---|---|---|
| attach client pids | 2055653, 2055663, 2055673, 2055683 | 2391810, 2391820, 2391830, 2391840 |
| ttyd pids | 2055599, 2055621, 2391068, 2391094 | none of the previous pids |

The pid intersection is empty in both rows: nothing survived. The client *count*
stayed at four and the sessions re-attached within seconds, which is precisely
why a count-based check would have reported success. The tty↔session mapping
also reshuffles across a restart (`claude-vw-1` moved from `/dev/pts/33` to
`/dev/pts/15`), so tty identity is not a stable handle either.

Blast radius is small by construction: the tmux **servers** live outside this
unit's cgroup under their own units, so no agent session is lost — only the
attach clients are.

### Why adoption is not the fix

Adopting an already-running ttyd on startup would preserve the attaches, but an
adopted process keeps its **old environment**. An env correction — the socket
dedup of `chrote-5mj.1.2` is the worked example — would then silently not take
effect until something replaced ttyd. That converts a visible, self-healing
blink into an invisible, indefinite correctness drift. A stale-env terminal that
looks healthy is worse than a terminal that visibly reconnects.

### The harm that mattered is already fixed elsewhere

The real cost of re-attaching was not the blink. Browser clients reconnect at
ttyd's default 80x24 before the page reports its viewport, and under
`window-size latest` they clamped live agent windows to 80 columns — truncating
TUI output that the Telegram bridge relays. That is fixed at the tmux layer by
the terminal size guard (`chrote-5mj.6.7`), which refuses sizing input from an
idle, still-un-negotiated client and widens a session left without a sizing
client.

Verified on the same restart as the pid table above: the three agent windows
stayed at 200x50 across the restart instead of collapsing to 80x23, while their
clients did re-attach at 80x24. With the clamping gone, what remains of a
restart is cosmetic.

## Decision

1. **Browser terminal attaches are not required to survive a `chrote-srv`
   restart.** Terminal attaches are a view onto a session, not the session. The
   durable thing is the tmux server, and that already lives outside the unit.
2. **Do not adopt an existing ttyd on startup.** Replacing ttyd on every start
   is what guarantees the process environment matches the deployed
   configuration. This is the deliberate trade the bug asked to be written down:
   a visible reconnect is accepted in exchange for never running a terminal
   whose environment silently predates the current config.
3. `KillMode=process` stays. It exists for the tmux servers and other long-lived
   children that must outlive a restart, not for ttyd.

## Orphan reaping under KillMode=process

`KillMode=process` means systemd stops only the main process, so this unit must
say what may outlive it and what cleans up:

- **tmux attach clients** — each runs on a pseudo-terminal CHROTE holds the
  master of, so closing that master hangs it up. The master is closed when the
  browser disconnects, and by the kernel when the server process exits, so
  there is no orphan to reap and no port-based reaper. This is the mechanism
  behind this whole ADR; before [ADR-0018](0018-terminal-transport-ownership.md)
  ttyd held the master and the same hangup applied.
- **tmux servers** — deliberately outside this cgroup and owned by their own
  units. Not this unit's to reap; reaping them here is what `KillMode=process`
  exists to prevent.
- **Agent-revival tmux servers** — retired 2026-08-09 by
  [ADR-0015](0015-access-first-non-interference.md). CHROTE no longer has an
  agent-revival path that creates replacement tmux servers.

The sharp edge this ADR left as follow-up — `fuser -k <port>/tcp` killing
whatever held the port rather than specifically our ttyd — is gone with the
reaper it described, closing `chrote-bgp`.

## Consequences

- Every deploy shows a brief terminal reconnect in open browser tabs. Accepted.
  If that is unacceptable to the operator, the alternative is ttyd adoption plus
  an explicit mechanism to force replacement whenever the environment changes —
  reopen this ADR rather than adopting silently.
- Anything asserting terminal survival across restarts must compare pids, not
  counts. Counts are preserved by the reconnect and will report false success.
- Agent panes keep their width across restarts only because the size guard runs.
  If that guard is disabled (`CHROTE_TERMINAL_SIZE_GUARD=off`), restart-induced
  clamping returns.

## Reversal criteria

Reopen if terminal reconnect stops being self-healing — for example if a
reconnect begins losing scrollback or leaving sessions detached rather than
re-attached.
