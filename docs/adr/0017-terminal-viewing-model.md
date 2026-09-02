# ADR-0017: The Terminal Viewing Model

## Status

Accepted 2026-09-01.

Scope note: this settles who owns the size of a tmux window, what a window
binding means, and how a quick look differs from a viewing session. The
transport question — how terminal bytes reach the browser — is settled
separately by [ADR-0018](0018-terminal-transport-ownership.md).

## Context

CHROTE's terminal layer accreted guards faster than it accreted a model. Five
independent parties could set the size of a tmux window: the browser's `fit()`
call, tmux's own `window-size latest` policy, a periodic server-side size guard,
whatever created the session, and any other tmux client. Nothing ever read the
resulting size back. No API returned it, no view displayed it, no test asserted
it.

### Measured, not assumed

Read-only inspection of one managed socket carrying 19 sessions found 19
attached browser clients, because a terminal released from view keeps its
connection and therefore keeps its tmux attach client. Under `window-size
latest` every one of those clients remains a sizing authority:

Retention is not the villain here, and this ADR does not remove it. What made
those clients harmful was that each one could size the window. Decision 1
removes that by attaching with `-d` and keeping one terminal per session, so a
retained connection is at most one client and it is the only one. An off-screen
tile therefore keeps its connection and its last rendered frame, which is the
most valuable thing the terminal layer holds. The cost is one process and one
tmux client per session the operator has viewed, and the operator accepted that
on 2026-09-02 in preference to a redraw on every switch.

| window | window size | attached clients |
|---|---|---|
| A | 122x59 | 56x47, 120x50, 122x60 |
| B | 200x50 (manual) | 75x60, 120x50 |
| C | 85x59 (manual) | 80x60, 111x50 |

Every row where a client width differs from the window width is a browser frame
rendering incorrectly at that moment: tmux renders once per window, so a narrow
client clips and a wide client shows dead space. The operator's experience of
this is a pane whose width changes on its own, or changes because a tab left
open on another device refit.

Six windows were `window-size manual` and therefore immune to every browser
resize; the `Refit` control on those is silently inert. Three sat at exactly
200x50, the size guard's own unobserved-window constant, with no
`@chrote-size-guard` marker — pinned by the guard before the marker existed and
then permanently orphaned by the fix that introduced it, because an unmarked
manual window is deliberately treated as operator-owned.

Six further windows had no clients at all and had drifted to whatever their last
viewer left, one of them stuck at tmux's default 80x24. The size guard cannot
reach these: it derives its window list from `list-clients`, so a window with no
clients is invisible to it.

### The binding model was a cache, not an intent

A session bound to a window was pruned when it stopped appearing in the session
poll. Because pruning ran a poll behind a candidate set, terminating a process
produced this sequence: the connection died instantly, the frame froze, and
5-10 seconds later the tag vanished and `activeSession` fell through to whatever
else was bound in that window. A different session appeared in the frame the
operator was reading. The machinery that made this survivable — a stale-candidate
set, a two-tick protection counter, alias reconciliation across three session
namespaces — existed only to stop the prune from eating sessions that had not
yet appeared in a poll.

### Verified tmux behaviour

Probed on scratch sockets, created and destroyed for the probe:

- `attach-session -d` detaches every other client, the displaced client's
  process exits cleanly, and the window adopts the new client's size exactly.
- `attach-session -f ignore-size` prevents a client from sizing a window that
  another client is already sizing, and the window holds that size even after
  the sizing client leaves. Attaching to a window with *no* clients still sizes
  it to the arriving client regardless of the flag.
- `resize-window -x -y` sets `window-size` to `manual` as a side effect. This is
  how the size guard manufactured pinned windows.
- A control-mode client (`-C attach` plus `refresh-client -C`) sizes a window
  without a pty and leaves `window-size` on `latest`. The size persists after
  that client detaches.

## Decision

1. **One sizing client at a time.** A terminal tile attaches with
   `attach-session -d`. Takeover is uniform: it displaces CHROTE's own clients
   and foreign clients such as an SSH session alike.

2. **Takeover is reversible from the losing side.** A displaced tile keeps its
   last rendered frame and offers `Reclaim`, which attaches again and takes the
   session back. No mode, no dialog, one click.

3. **A new session is sized once, at creation, and never again.** The
   create-session path attaches a transient control-mode client, sets the
   canonical size, and detaches. There is no periodic sweep, no idle
   classification, and no recurring check of any kind. After creation, the last
   viewer's size is the size, and a session nobody views keeps whatever it was
   left at.

4. **CHROTE never creates a `manual` window.** `resize-window` is not used.
   Manual windows that already exist are surfaced, not silently normalised, and
   carry an explicit operator action to release them.

5. **A binding is the operator's stated intent, not a cache of live sessions.**
   A tile has four states: `Idle` (bound, not on screen; it keeps its
   connection and its last rendered frame once it has been shown), `Live`
   (attached and sizing), `Taken over` (alive elsewhere; last frame plus
   `Reclaim`), and
   `Ended` (session gone; last frame plus `Restart` and `Remove`). Bindings,
   including ended ones, survive a reload. Nothing is ever removed from a window
   except by the operator.

6. **Peek is an observer.** It attaches with `-f ignore-size`, never displaces
   another client, and never resizes a session that has a viewer. Input is not
   suppressed.

7. **A badge means this session is not what you would assume from looking at
   it.** Pinned size, a foreign client attached, more than one tmux window or
   pane, and mouse mode off all qualify. The rule exists to keep the set from
   growing into decoration.

## Consequences

- The terminal size guard is deleted in full: the periodic sweep, idle-based
  client classification, `ignore-size` flag management, the unobserved-size
  constants, and the ownership marker.
- The prune layer is deleted in full: stale-candidate tracking, protection
  counters, workspace pruning, and the operator-facing "clear stale sessions"
  action.
- Opening a session in CHROTE disconnects an SSH client attached to it. This is
  intended, and the foreign-client badge exists so it is informed rather than
  surprising.
- A session created outside CHROTE without an explicit size starts at tmux's
  80x24 and stays there until something views it. CHROTE does not rescue it,
  because rescuing requires noticing, and noticing requires the recurring checks
  this ADR removes. Session-spawning tooling is expected to set an initial size
  the same one-shot way.
- Agent-driving tooling should stop passing `-x`/`-y` to `new-session` and size
  the session the same one-shot way instead. Corrected 2026-09-02: an earlier
  revision of this ADR claimed `-x`/`-y` pins the window at creation. It does
  not. On tmux 3.6a it sets the session's `default-size` option and leaves
  `window-size` on `latest`, and a later client resizes the window freely —
  measured on a scratch socket, where a session created at 160x48 was resized to
  96x27 by a 96x28 client. The reason to drop it is that it leaves a stale
  `default-size` and duplicates a mechanism CHROTE already owns, not that it
  pins. What actually pins is `resize-window`, and the origin of the pinned
  windows observed on the operator's sockets is therefore still unidentified;
  `ctx-1tf7` tracks finding it.

## Reversal criteria

Reopen if one-shot creation sizing proves insufficient in practice — for
example if sessions routinely reach an operator at 80x24 because they were
created by tooling outside CHROTE's control. The rejected alternative to
revisit first is a persistent control-mode client per session, which holds a
canonical size without pinning and restores it automatically on detach, at the
cost of one long-lived client per session.
