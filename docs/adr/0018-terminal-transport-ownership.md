# ADR-0018: CHROTE Owns the Terminal Transport

## Status

Accepted 2026-09-01. Amends [ADR-0013](0013-ttyd-restart-lifecycle-and-orphan-reaping.md),
whose ttyd lifecycle analysis stops applying once ttyd is removed.

Scope note: this settles how terminal bytes reach the browser and what renders
them. Who owns the *size* of a tmux window is settled by
[ADR-0017](0017-terminal-viewing-model.md).

## Context

CHROTE renders each tmux session by embedding ttyd's own web client in an
`<iframe>`. ttyd serves a self-contained page; CHROTE proxies it and then
reaches through the document boundary to control it.

That boundary, not tmux, is the source of most of the terminal layer's
complexity. An audit found the following exists solely because the terminal
lives in a separate document:

- A paste bridge that intercepts the paste chord and calls `term.paste()`,
  because ttyd's hidden textarea is not the focus target across the boundary.
- A scrollbar toggle implemented by **injecting a `<style>` element into ttyd's
  document**, because CSS cannot cross documents.
- **Two duplicated 20-attempt, 50ms polling loops** that busy-wait for ttyd's
  `window.term` global to appear, because there is no readiness event.
- A `moveBefore` reparenting trick, because `appendChild` reloads an iframe's
  document and kills the WebSocket.
- Reconnect implemented as a cache-busting URL change that reloads a document.
- Deferred `src` assignment, an offscreen parking container, parked-versus-
  claimed inline style juggling, and retention of each iframe's last real
  viewport so parking does not push a small grid into the shared tmux window.
- A `useLayoutEffect`-not-`useEffect` requirement, because a passive cleanup
  runs after React detaches the subtree, by which point the iframe can only be
  re-inserted, which reloads it.
- Six `try`/`catch` blocks whose only purpose is swallowing cross-document
  access errors.

Roughly 230-280 lines of production code exist only for these reasons, with
about 120 more shaped by them; the pool component is roughly three times the
size it needs to be, and about 584 lines of test are devoted to iframe
mechanics rather than terminal behaviour.

Critically, the boundary also denies CHROTE the one signal
[ADR-0017](0017-terminal-viewing-model.md) depends on: a connection-closed
event. Without it, a tile cannot distinguish "taken over" from "ended" from
"still connecting".

### The protocol is small and frozen

ttyd's browser protocol is undocumented but has not moved. Its command
constants are byte-identical across 1.6.3, 1.7.0, 1.7.7 and `main`; the header
defining them has had no protocol-affecting change since December 2020; and
1.7.7, released March 2024, is the newest version. Frames are binary with a
one-byte command prefix: client sends `0` input, `1` resize as JSON, `2`/`3`
flow control, and an unprefixed JSON object as the opening handshake. Server
sends `0` output, `1` window title, `2` preferences.

There is no middle path. ttyd's client is a preact application inlined by its
build into a single self-contained HTML file compiled into the binary. It is not
published, has no separate script URL, and assumes it owns the document root.
The only npm wrapper is a single-maintainer pre-1.0 package and is not a
credible dependency. Speaking the protocol directly is approximately 60-80 lines.

## Decision

1. **The dashboard owns the terminal.** CHROTE renders xterm.js in its own page
   and speaks the terminal protocol itself. The iframe, and every workaround
   listed above, is deleted.

2. **The end state removes ttyd.** The Go server spawns the tmux attach on a pty
   directly and relays it over its own WebSocket, removing the vendored ttyd
   binary, the ttyd process lifecycle, the port-based reaper, the launch script
   and its duplicated socket-resolution rules.

3. **This lands in two stages.** Stage one keeps ttyd as the pty host and
   replaces only the browser side. Stage two replaces the server side. Stage one
   is independently verifiable and captures most of the deletion; stage two is
   then a contained change to what sits at the far end of one WebSocket.

## Consequences

- xterm.js moves into CHROTE's bundle and therefore into the Go binary. This is
  not new weight on the wire: the browser already downloads xterm today, inlined
  in ttyd's page. No build gate constrains bundle size.
- Stage two adopts a class of correctness risk that ttyd currently absorbs: pty
  lifecycle, signal handling, EOF, flow control and resize. The surface is small
  and frozen, but it is real, and it lands in the component the product is built
  around.
- Stage two closes `chrote-bgp` and `chrote-xrw` by deleting the code they
  describe, and retires the duplicated socket-resolution logic that currently
  exists in both Go and shell and must be kept in sync by hand.
- The restart behaviour recorded in ADR-0013 is unchanged by stage one. After
  stage two, terminal attaches still do not survive a service restart, for the
  same reason: the pty children die with the server.

## Reversal criteria

Reopen stage two if pty ownership produces correctness bugs that ttyd did not
have — garbled output, lost signals, resize races, or leaked processes — that
are not fixed within one iteration. Stage one is not reversible in practice and
should not be: it removes the boundary that made the terminal layer opaque.
