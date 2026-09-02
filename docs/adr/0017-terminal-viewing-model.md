# ADR-0017: The Terminal Viewing Model

## Status

Accepted 2026-09-01; amended 2026-09-02 by decision 8 and the `Lost` tile state
in decision 5, after a deploy showed twenty tiles claiming a takeover that had
not happened; amended again 2026-09-02 by decision 1, which now holds one
*sizing* client rather than one client, and by the parts of decisions 2, 5, 6
and 8 that only existed to work around `-d`.

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
- Measured again 2026-09-02, on tmux 3.6a, for the observer model in decision
  1. `ignore-size` is per client and can be moved after the fact:
  `refresh-client -t <tty> -f ignore-size` sets it and `-f '!ignore-size'`
  clears it, each toggling only the flag named and leaving a client's other
  flags alone. A client that is the *only* candidate sizes the window whether
  it carries the flag or not, and keeps tracking its own resizes. When every
  attached client carries the flag, tmux falls back to sizing by the most
  recent one — the `window-size latest` flapping this ADR exists to stop, which
  is why exactly one client is left unflagged rather than none. Addressing a
  client that has already detached fails with `can't find client` and changes
  nothing.
- `resize-window -x -y` sets `window-size` to `manual` as a side effect. This is
  how the size guard manufactured pinned windows.
- A control-mode client (`-C attach` plus `refresh-client -C`) sizes a window
  without a pty and leaves `window-size` on `latest`. The size persists after
  that client detaches.

## Decision

1. **One sizing client at a time — not one client.** Amended 2026-09-02. Many
   clients may watch one tmux window; only one may set its size. A tile attaches
   with `-f ignore-size` when another client is already sizing the window and
   with no flags when none is, so it takes the sizing seat when the seat is free
   and watches at the current size when it is not. Nothing CHROTE does attaches
   with `-d`, so no client of any origin is ever displaced.

   The first revision made takeover uniform, because the measured harm was three
   clients fighting over one window's width. That conflated two things tmux keeps
   separate — how many clients may attach, and how many may size — and eviction
   answered the second by forbidding the first. It cost a capability worth having:
   a second device could not watch a session at all, and neither could a second
   person. The seat is read back from `list-clients` at attach, so a window whose
   clients all carry the flag is repaired by the next tile to arrive.

2. **The sizing seat moves by `Claim`, and moving it detaches nobody.**
   Amended 2026-09-02. A tile offers `Claim`: the host flags every other sizing
   client and only then clears its own, so the window is never momentarily
   sized by two clients. The other viewers keep watching, at the claiming
   device's size. It is one frame on the connection the tile already has — no
   redial, no mode, no dialog — and a tile with no connection dials first and
   claims on arrival. No mode, no dialog, one click.

   The honest limit is that a tmux window is drawn once, at one size, for every
   client watching it. An observer therefore sees the sizing client's
   dimensions, and a phone watching a 200-column desktop pane is unreadable
   until it claims. No interface can make one grid two sizes, so CHROTE says so
   instead: the `Claim` control states what claiming does to the other viewers,
   and a session with more than one viewer carries a badge saying it is drawn
   once at the claiming viewer's size.

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
   A tile has five states: `Idle` (bound, not on screen; it keeps its
   connection and its last rendered frame once it has been shown), `Live`
   (attached, sizing the window or watching at somebody else's size),
   `Taken over` (another client detached this terminal; last frame plus
   `Claim`), `Lost` (the connection went, the session did not; last frame plus
   `Reconnect`), and
   `Ended` (session gone; last frame plus `Restart` and `Remove`). Bindings,
   including ended ones, survive a reload. Nothing is ever removed from a window
   except by the operator.

   `Lost` was added 2026-09-02. A tile with no connection and a live session was
   read as `Taken over` outright, which is false for the commonest cause of one:
   restarting `chrote-srv` kills every pty, so every open tile loses its
   connection while every session stays alive ([ADR-0013](0013-ttyd-restart-lifecycle-and-orphan-reaping.md)).
   Measured minutes after a deploy — twenty tiles saying the session was
   attached elsewhere while the socket carried one client, and twenty `Reclaim`
   clicks to recover.

   `Taken over` narrowed 2026-09-02 with decision 1. CHROTE no longer attaches
   with `-d`, so it can no longer detach its own terminals; the state now means
   a client from outside CHROTE attached with `-d` and took this pty away.

6. **Peek is an observer.** It attaches with `-f ignore-size`, never displaces
   another client, never resizes a session that has a viewer, and cannot claim
   the sizing seat. Input is not suppressed. Since decision 1 a tile no longer
   displaces anyone either, so the two modes now differ in one thing only:
   whether they take a free sizing seat.

7. **A badge means this session is not what you would assume from looking at
   it.** Pinned size, a foreign client attached, more than one tmux window or
   pane, and mouse mode off all qualify. The rule exists to keep the set from
   growing into decoration.

8. **A lost connection is told from an ended terminal by the close frame, and
   dials again by itself; a session another client holds does not.** Added
   2026-09-02. CHROTE closes with a close frame when the pty hangs up and when
   an attach is refused, and sends nothing at all when the process dies or the
   network goes, so the two causes are separable in the browser before tmux is
   asked anything. A tile whose connection was *lost* dials again when it is put
   on screen — one attempt per such moment, no retry, no timer, no recurring
   check of any kind.

   The carve-out this decision arrived with is gone as of decision 1: a tile
   whose session had a foreign client was left alone, because dialling again
   attached with `-d` and would have evicted them. Dialling now attaches
   alongside them without the sizing seat, so it costs them neither their client
   nor their size, and the tile dials for the operator like any other.

## Consequences

- The terminal size guard is deleted in full: the periodic sweep, idle-based
  client classification, `ignore-size` flag management, the unobserved-size
  constants, and the ownership marker.
- The prune layer is deleted in full: stale-candidate tracking, protection
  counters, workspace pruning, and the operator-facing "clear stale sessions"
  action.
- Opening a session in CHROTE no longer disconnects an SSH client attached to
  it. Both watch it live; the SSH client keeps the size until the operator
  claims it, and claiming leaves them attached at the new size. The
  foreign-client badge stays, because who else is watching is still not
  something a glance can tell.
- A retained off-screen tile keeps its tmux client, so a session can accumulate
  one client per tile that has ever shown it. Exactly one of them is unflagged,
  and it is whichever attached to a free seat — which may be an off-screen tile,
  in which case the on-screen one watches at its size until it is claimed. The
  operator has `Claim` for that, and no recurring check looks for it.
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
