---
title: CHROTE documentation
nav: Overview
summary: CHROTE is a browser-based agentic IDE for one operator on one host. tmux owns the sessions; CHROTE is the way in.
sources: README.md, VISION.md, PRD.md
---

<div class="tile">
<div class="tile-head"><span>Terminal 1</span><span class="dim">window 1 · empty</span></div>
<div class="tile-body">
<img src="../../../CHROTE.png" alt="A sign reading CHROTE, Control Hub for Remote Operations and Tmux Execution, on a rock above a desert refinery with tanker trucks driving towards it." width="1408" height="736">
<div class="tile-caption">
<h1 id="chrote-documentation">CHROTE documentation</h1>
<p>Control Hub for Remote Operations and Tmux Execution. A browser-based agentic IDE for one trusted operator: see every tmux session on the host, arrange them side by side, and steer them from a laptop, tablet or phone on your own private network.</p>
</div>
</div>
</div>

<ul class="start">
<li><a href="install.md"><span class="step">1 · Install</span><span class="title">Install CHROTE</span><span class="text">Build from source and run it as a user service on Linux or WSL. No sudo.</span></a></li>
<li><a href="first-session.md"><span class="step">2 · Launch</span><span class="title">Your first session</span><span class="text">Launch an agent into a window, look at another with Peek, and hand it work.</span></a></li>
<li><a href="remote-access.md"><span class="step">3 · Reach</span><span class="title">Reach it from your phone</span><span class="text">Keep CHROTE on loopback and put HTTPS on your private network in front of it.</span></a></li>
<li><a href="concepts.md"><span class="step">Learn</span><span class="title">Concepts</span><span class="text">Sessions, windows, tabs, Peek, Claim, residents and the table, in one page.</span></a></li>
<li><a href="configuration.md"><span class="step">Configure</span><span class="title">Configuration</span><span class="text">Where settings live, which survive a reinstall, and every variable.</span></a></li>
<li><a href="security.md"><span class="step">Trust</span><span class="title">Security model</span><span class="text">No login. Whoever reaches CHROTE holds a shell. Read this before exposing it.</span></a></li>
</ul>

## What CHROTE is

One agent in one terminal needs no dashboard. Ten agents across a dozen tmux
sessions is a different problem: you lose track of which one is waiting on an
answer, which one exited an hour ago, and which one has been rewriting the same
file since lunch. SSH and tmux handle that fine at a desk. They handle it badly
from a phone.

CHROTE puts handles on the work without taking it over. It shows the tmux
sessions on the host, arranges them into windows you watch side by side, and
lets you type into any of them. Close the browser, switch devices or restart
the server, and the sessions keep running, because the browser never owned
them.

Any command-line program runs in a CHROTE window: Codex, Claude Code, a build,
a log tail, `psql`. A window is a tmux session running your shell, and CHROTE
does not need to know which is which.

<pre class="diagram" role="img" aria-label="Diagram of the CHROTE workspace: a tab bar, the Sessions panel at the left, two terminal windows side by side, the table at the right and a resident's spine at the far right edge.">
<b>Terminal 1</b>  Terminal 2   Files   Beads   Brain   Agents   Scheduled   Services   Server   Settings
<i>──────────</i>───────────────────────────────────────────────────────────────────────────────────────────
┌ Sessions ────┐┌ api-refactor · claude ─────────┐┌ docs-pass · codex ──────────┐┌ The table ───┐┃
│ <i>●</i> api-refac… ││ › Running tests…               ││ › Rewrote install section.  ││ chrote-x1k   │┃
│   docs-pass  ││   42 passed, 1 failed          ││   Waiting for review.       ││ Fix reinstall│┃ C
│   build-log  ││                                ││                             ││ wording      │┃ L
│   psql       ││                                ││ ›                           ││              │┃ E
│ + Launch     ││ › <i>█</i>                            ││                             ││ Send  Alt+S  │┃ R
└──────────────┘└────────────────────────────────┘└─────────────────────────────┘└──────────────┘┃ K
</pre>

## How it fits together

- **tmux owns the sessions.** CHROTE discovers sessions on the tmux sockets it
  is configured to read, attaches to them on pseudo-terminals it allocates, and
  never falls back to another tmux server.
- **The host owns the work.** Files stay in the filesystem, issues in each
  project's Beads store, schedules and service catalogs in host configuration.
  CHROTE keeps no shadow copy.
- **The browser owns only presentation.** Layouts, tab labels and presets live
  in `localStorage`. Clear it and you lose an arrangement, never any work.
- **The network is the boundary.** There is no login. CHROTE binds to
  `127.0.0.1` and you reach it through a private network you control.

## Find your way

<ul class="rows">
<li><a href="terminals.md"><span class="row-title">Use</span><span class="row-text">Terminals, Send to Session, Files, Beads, Brain, Agents, Scheduled, Services, Server, Settings and the keyboard.</span></a></li>
<li><a href="environment.md"><span class="row-title">Configure</span><span class="row-text">The environment reference, the launcher, the theme, the Library and residents, Services, and several Unix users.</span></a></li>
<li><a href="upgrade.md"><span class="row-title">Operate</span><span class="row-text">Upgrade, uninstall, troubleshoot, and know where state lives before you back it up.</span></a></li>
<li><a href="http-api.md"><span class="row-title">Reference</span><span class="row-text">The HTTP routes, the development loop, and the canonical documents this site is built from.</span></a></li>
</ul>

CHROTE v2 is alpha. The canonical documents, such as
[`PRD.md`](../../../PRD.md), [`ARCHITECTURE.md`](../../../ARCHITECTURE.md) and
[`SECURITY.md`](../../../SECURITY.md), are rendered in this site under
[Source documents](documents.md). Where a guide page and a canonical document
disagree, the canonical document wins.
