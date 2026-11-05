---
title: Terminals and sessions
nav: Terminals
summary: Terminal tabs, windows, the Sessions panel, Peek, Claim, presets, and what happens when a session ends.
sources: README.md, PRD.md, DESIGN-SYSTEM.md, docs/adr/0017-terminal-viewing-model.md, docs/adr/0018-terminal-transport-ownership.md
---

# Terminals and sessions

Terminals are the product. Everything else in CHROTE exists to save a trip back
to the terminal.

## The workspace

- **Terminal tabs** are independent workspaces. Three by default, one to six in
  Settings. Rename a tab from its menu (right-click the tab).
- **Windows** are the tiles in a tab, one to four at a time. A window can hold
  several sessions and display one; you cycle through the rest without losing
  the binding.
- **The Sessions panel** (`Alt+A`) and **the Files panel** (`Alt+O`) open
  beside the windows. Both start closed and take no width while closed. Their
  state belongs to the tab.
- **The tile header** carries identity: the session name, the harness mark
  (Claude Code and Codex show theirs, any other command shows its name, a shell
  shows nothing), the Unix user, and the tile's state. What an agent is doing is
  its own to report in its status line.

## The tab's menu

Right-click a terminal tab for:

| Item | What it does |
| --- | --- |
| Rename tab | Give the tab a label of its own (device-local) |
| Windows | Choose how many windows the tab shows |
| Save layout as preset / Restore preset / Delete preset | Up to ten named layouts, stored in this browser |
| Clear tab assignments | Unbind every session from this tab's windows; sessions keep running |
| Reconnect frames | Reopen this tab's terminal connections |
| Claim all | Fit every session in the tab to this device |
| Pin sessions panel | Keep the Sessions panel open in this tab |

## A tile's menu

Right-click a tile's header for **Send to session**, **What this agent sees**,
**Open files in working directory**, **Reconnect frame**, **Claim session**,
**Refit frame**, **Rename session**, **Copy session** and **Kill session**.
Killing asks for a second press within three seconds.

## The Sessions panel

The panel lists sessions on every tmux socket CHROTE is configured to read. Its
plus opens the [launcher](first-session.md#1-launch-into-the-empty-window).
Session search is leader then `/`, or `/` when no terminal has focus.

A row click opens **Peek**. The row's menu offers:

- **Peek**; **Send to session** (`Alt+S`); **Schedule task…**, which opens the
  Send drawer with this session fixed as the target and a schedule beside it;
  **What this agent sees**;
- **Attach to window**, listing every window in every tab; **Unassign**;
- **Rename**; **Copy session**; **Kill session**.

Selecting several rows lets you kill them together, with the same two-press
confirmation.

## Peek

Peek is a floating window centred over the workspace, sized by the session it
shows: the tmux window's own columns and rows, status line included, capped at
90% of the workspace. Past the cap, or when you resize it from an edge, the font
shrinks and the grid stays. It opens with its terminal focused, so you can drive
the session from it, `Escape` included. A one-line header carries the mark, the
name, Send and Close. Peek attaches as an observer and never resizes the session.

## Sizing, Claim and other viewers

tmux draws a window once, at one size. CHROTE attaches so that a displayed
terminal is the session's one sizing client.

- Opening a session in a window takes over its size from whatever was attached
  before, CHROTE's own client or an external one. It detaches nobody.
- When another device or an SSH client takes the size, your tile says so and
  offers **Claim**, which makes the session fit this device again. The others
  keep watching at that size.
- `Alt+C` claims every connected tile on this device.
- Who else is attached is shown rather than hidden.

A session CHROTE creates starts at 200×50 (`CHROTE_TERMINAL_CREATION_COLS` and
`CHROTE_TERMINAL_CREATION_ROWS`), so an agent nobody has looked at yet writes wide
instead of wrapping at tmux's 80 columns. Nothing re-checks the size afterwards.

## When a session ends

A binding is yours until you remove it. The tile keeps its last output, says
what CHROTE knows, and offers the next move. A different session never silently
takes its place.

| The tile says | Offers |
| --- | --- |
| The session ended | **Restart**, a new session of the same name and user in this window; **Remove**, which drops the binding |
| It was detached by another client | **Claim**, which reconnects at this device's size |
| It lost its connection | **Reconnect**, which leaves the size where it is |

CHROTE does not restart processes that died and does not recreate sessions after
a reboot. If something must survive a reboot, give it a systemd unit of your own.

## Copying and links

- Painting a selection in a terminal copies it to the system clipboard, with a
  toast to confirm. Under tmux mouse mode that is the browser's own selection
  gesture; tmux's own copy-mode selection is separate and does not reach the
  system clipboard.
- A path to an image printed in a terminal opens in an image glance; a Bead id
  printed in a terminal opens on the table beside the session that named it.
- **Settings → Appearance** sets the font size (12–20 px), mouse-wheel
  scrolling, and whether the terminal scrollbar shows. These are per device.

## Agents and their events

A tile or row with an unseen **finished** or **needs input** report carries a
mark until the session is focused. The tab holding it is marked too. See
[Agent events](launcher.md#agent-events).

## On small screens

Below 768 pixels the tab bar is a menu and one window shows at a time. Peek and
Send to Session work as on desktop; layout editing is best left to a desktop.
